package main

// ฝั่งเขียน: A1 MVCC/bloat · A2 COPY vs INSERT · A3 ทำไม commit ถึงแพง

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ══════════════════════════════════════════════════════════════
// A1 · MVCC & bloat
//
// ใน PG การ UPDATE ไม่เคยเขียนทับที่เดิม — มันเขียน "แถวใหม่" เสมอ
// แล้วทำเครื่องหมายแถวเก่าว่าตายแล้ว (นี่คือสิ่งที่ YottaDB ไม่มีเลย)
//
// คำถามที่ demo นี้ตอบ: ถ้าอย่างนั้นทำไมตารางถึงไม่โตทุกครั้งที่ UPDATE?
// คำตอบ: PG มี HOT pruning — ถ้าไม่มีใคร "ยังมองเห็น" ใบเก่า มันกวาดทิ้ง
//        ในหน้าเดิมได้ทันที ตารางเลยแทบไม่โต
//        bloat ของจริงเกิดตอนมีใครเปิด transaction ค้างไว้
// ══════════════════════════════════════════════════════════════

func demoMVCC(ctx context.Context, n int) {
	header("A1 · MVCC & bloat — UPDATE สร้างแถวใหม่เสมอ แต่ทำไมไฟล์ไม่โต")
	fmt.Printf("\nUPDATE แถวเดียวกัน %d ครั้ง เทียบ 2 สถานการณ์\n", n)

	a := mvccRound(ctx, n, false)
	b := mvccRound(ctx, n, true)

	section("สรุป")
	fmt.Printf("  %-34s %14s %14s\n", "", "ไม่มีใครค้าง", "มี txn ค้าง")
	fmt.Printf("  %-34s %14s %14s\n", "ctid ก่อน UPDATE", a.ctidBefore, b.ctidBefore)
	fmt.Printf("  %-34s %14s %14s\n", "ctid หลัง UPDATE", a.ctidAfter, b.ctidAfter)
	fmt.Printf("  %-34s %14s %14s\n", "ขนาด heap หลัง UPDATE", fmtBytes(a.szAfter), fmtBytes(b.szAfter))
	fmt.Printf("  %-34s %14d %14d\n", "  = กี่หน้า (หน้าละ 8KB)", pages(a.szAfter), pages(b.szAfter))
	fmt.Printf("  %-34s %14d %14d\n", "dead tuple (pgstattuple)", a.dead, b.dead)
	fmt.Printf("  %-34s %14s %14s\n", "ขนาดหลัง VACUUM", fmtBytes(a.szVac), fmtBytes(b.szVac))
	fmt.Printf("  %-34s %14d %14d\n", "dead tuple หลัง VACUUM", a.deadVac, b.deadVac)
	fmt.Printf("  %-34s %14s %14s\n", "ขนาดหลัง VACUUM FULL", fmtBytes(a.szFull), fmtBytes(b.szFull))

	note("ctid = (หน้าที่, ช่องที่) — มันเปลี่ยนทุกครั้งที่ UPDATE ทั้งสองคอลัมน์")
	note("  นั่นคือหลักฐานว่า PG ไม่เคยเขียนทับที่เดิม")
	note("คอลัมน์ซ้าย: ไฟล์แทบไม่โต เพราะ HOT pruning กวาดใบเก่าในหน้าเดิมได้ทันที")
	note("คอลัมน์ขวา: มี REPEATABLE READ ค้างอยู่ → ใบเก่าทุกใบยังมีคนมองเห็น → กวาดไม่ได้ → ไฟล์ระเบิด")
	note("VACUUM ทำให้ dead=0 แต่ขนาดไฟล์ไม่ลด (คืนเป็นที่ว่างให้ใช้ซ้ำ ไม่คืน OS)")
	note("VACUUM FULL เขียนตารางใหม่ทั้งใบ → ไฟล์เล็กลงจริง แต่ล็อกทั้งตาราง ห้ามทำตอน prod ใช้งาน")
	note("YottaDB เขียนทับที่เดิม จึงไม่มีทั้ง dead tuple ไม่มี VACUUM ไม่มี bloat")
}

type mvccResult struct {
	ctidBefore, ctidAfter  string
	szAfter, szVac, szFull int64
	dead, deadVac          int64
	took                   time.Duration
}

func mvccRound(ctx context.Context, n int, holdSnapshot bool) mvccResult {
	c := connect(ctx)
	defer c.Close(ctx)

	exec(ctx, c, `DROP TABLE IF EXISTS acct`)
	// ปิด autovacuum ระดับตารางซ้ำอีกชั้น กันผลวัดเพี้ยน
	exec(ctx, c, `CREATE TABLE acct(id int PRIMARY KEY, bal bigint)
	              WITH (autovacuum_enabled = false)`)
	exec(ctx, c, `INSERT INTO acct VALUES (1, 0)`)

	label := "ไม่มีใครถือ snapshot ค้าง"
	var holder *pgx.Conn
	if holdSnapshot {
		label = "มีอีก connection เปิด BEGIN ISOLATION LEVEL REPEATABLE READ ค้างไว้"
		holder = connect(ctx)
		exec(ctx, holder, `BEGIN ISOLATION LEVEL REPEATABLE READ`)
		// ใน REPEATABLE READ snapshot เกิดตอน "คำสั่งแรก" ไม่ใช่ตอน BEGIN
		_ = scan1[int](ctx, holder, `SELECT 1`)
	}
	section("รอบ: " + label)

	var r mvccResult
	r.ctidBefore = scan1[string](ctx, c, `SELECT ctid::text FROM acct WHERE id = 1`)

	start := time.Now()
	for i := 0; i < n; i++ {
		exec(ctx, c, `UPDATE acct SET bal = bal + 1 WHERE id = 1`)
	}
	r.took = time.Since(start)

	r.ctidAfter = scan1[string](ctx, c, `SELECT ctid::text FROM acct WHERE id = 1`)
	r.szAfter = relSize(ctx, c, "acct")
	_, r.dead = pgstat(ctx, c, "acct")
	fmt.Printf("   UPDATE %d ครั้ง ใช้เวลา %v (%.0f ครั้ง/วิ)\n", n, r.took.Round(time.Millisecond), rate(n, r.took))
	fmt.Printf("   ctid %s → %s   heap %s   dead=%d\n",
		r.ctidBefore, r.ctidAfter, fmtBytes(r.szAfter), r.dead)

	// ปล่อย snapshot ก่อน VACUUM — ไม่งั้น VACUUM เก็บอะไรไม่ได้เลย
	if holder != nil {
		exec(ctx, holder, `COMMIT`)
		holder.Close(ctx)
		note("ปิด transaction ที่ค้างแล้ว VACUUM ถึงจะเก็บกวาดได้")
	}

	exec(ctx, c, `VACUUM acct`)
	r.szVac = relSize(ctx, c, "acct")
	_, r.deadVac = pgstat(ctx, c, "acct")

	exec(ctx, c, `VACUUM FULL acct`)
	r.szFull = relSize(ctx, c, "acct")
	fmt.Printf("   VACUUM → heap %s dead=%d   |   VACUUM FULL → heap %s\n",
		fmtBytes(r.szVac), r.deadVac, fmtBytes(r.szFull))
	return r
}

// ══════════════════════════════════════════════════════════════
// A2 · COPY vs INSERT
//
// ยัดข้อมูลชุดเดียวกันด้วย 6 วิธี วัด 2 อย่าง
//   1. เร็วแค่ไหน (rows/sec)
//   2. สร้าง WAL กี่ byte — WAL คือสมุดที่ต้องจดก่อนทำจริง
//      ยิ่งจดเยอะ ยิ่งต้องเขียนดิสก์เยอะ
// ══════════════════════════════════════════════════════════════

const ingestDDL = `(id bigint, cif text, name text, amt numeric(15,2), ts timestamptz)`

var ingestCols = []string{"id", "cif", "name", "amt", "ts"}

func makeRows(n int) [][]any {
	now := time.Now()
	rows := make([][]any, n)
	for i := range rows {
		rows[i] = []any{
			int64(i),
			fmt.Sprintf("CIF%09d", i),
			fmt.Sprintf("ลูกค้าหมายเลข %d", i),
			float64(i%100000) + 0.25,
			now,
		}
	}
	return rows
}

type ingestResult struct {
	name string
	took time.Duration
	wal  int64
}

func demoCopyVsInsert(ctx context.Context, n int) {
	header("A2 · COPY vs INSERT — ยัดข้อมูลชุดเดียวกัน 6 วิธี")
	c := connect(ctx)
	defer c.Close(ctx)

	exec(ctx, c, `DROP TABLE IF EXISTS ingest`)
	exec(ctx, c, `CREATE TABLE ingest `+ingestDDL)
	exec(ctx, c, `DROP TABLE IF EXISTS ingest_unlogged`)
	exec(ctx, c, `CREATE UNLOGGED TABLE ingest_unlogged `+ingestDDL)

	rows := makeRows(n)
	fmt.Printf("\nแถวละ ~%d byte × %d แถว\n", 60, n)

	methods := []struct {
		name  string
		table string
		fn    func(*pgx.Conn, string, [][]any)
	}{
		{"1. INSERT ทีละแถว (commit ทุกแถว)", "ingest", insertAutocommit},
		{"2. INSERT ทีละแถว (txn เดียว)", "ingest", insertOneTxn},
		{"3. INSERT หลายแถวต่อคำสั่ง (1000)", "ingest", insertMultiRow},
		{"4. pgx SendBatch (pipeline 1000)", "ingest", insertBatch},
		{"5. COPY (pgx CopyFrom)", "ingest", copyFrom},
		{"6. COPY ลงตาราง UNLOGGED", "ingest_unlogged", copyFrom},
	}

	var res []ingestResult
	for _, m := range methods {
		exec(ctx, c, `TRUNCATE `+m.table)
		before := walLSN(ctx, c)
		start := time.Now()
		m.fn(c, m.table, rows)
		took := time.Since(start)
		wal := walDiff(ctx, c, before, walLSN(ctx, c))

		got := scan1[int64](ctx, c, `SELECT count(*) FROM `+m.table)
		if got != int64(n) {
			must(fmt.Errorf("%s: ได้ %d แถว คาดว่า %d", m.name, got, n))
		}
		res = append(res, ingestResult{m.name, took, wal})
		fmt.Printf("   %-36s %9v  %10.0f rows/s  WAL %9s\n",
			m.name, took.Round(time.Millisecond), rate(n, took), fmtBytes(wal))
	}

	base := res[0]
	section("เทียบกับวิธีที่ 1")
	fmt.Printf("  %-36s %10s %10s %12s\n", "วิธี", "เร็วขึ้น", "WAL/แถว", "WAL เทียบ")
	for _, r := range res {
		fmt.Printf("  %-36s %9.1fx %9s %11.0f%%\n",
			r.name,
			base.took.Seconds()/r.took.Seconds(),
			fmtBytes(r.wal/int64(n)),
			100*float64(r.wal)/float64(base.wal))
	}

	note("วิธี 1 → 2 เร็วขึ้นเพราะ commit น้อยลง ไม่ใช่เพราะ INSERT เร็วขึ้น (ดู demo commit)")
	note("วิธี 2 → 3/4 เร็วขึ้นเพราะลดจำนวน round-trip ไป-กลับ server")
	note("COPY เร็วที่สุดเพราะเป็น protocol คนละเส้น — ไม่ต้อง parse/plan SQL ทีละคำสั่ง")
	note("UNLOGGED = ไม่จดสมุด WAL → เร็วขึ้นอีก แต่ถ้า server crash ตารางนี้โดนล้างทิ้งทั้งใบ")
}

func insertAutocommit(c *pgx.Conn, t string, rows [][]any) {
	ctx := context.Background()
	q := `INSERT INTO ` + t + ` VALUES ($1,$2,$3,$4,$5)`
	for _, r := range rows {
		exec(ctx, c, q, r...)
	}
}

func insertOneTxn(c *pgx.Conn, t string, rows [][]any) {
	ctx := context.Background()
	q := `INSERT INTO ` + t + ` VALUES ($1,$2,$3,$4,$5)`
	exec(ctx, c, `BEGIN`)
	for _, r := range rows {
		exec(ctx, c, q, r...)
	}
	exec(ctx, c, `COMMIT`)
}

func insertMultiRow(c *pgx.Conn, t string, rows [][]any) {
	ctx := context.Background()
	const per = 1000
	exec(ctx, c, `BEGIN`)
	for i := 0; i < len(rows); i += per {
		end := min(i+per, len(rows))
		var sb strings.Builder
		sb.WriteString(`INSERT INTO ` + t + ` VALUES `)
		args := make([]any, 0, (end-i)*5)
		for j := i; j < end; j++ {
			if j > i {
				sb.WriteByte(',')
			}
			b := (j-i)*5 + 1
			fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d)", b, b+1, b+2, b+3, b+4)
			args = append(args, rows[j]...)
		}
		exec(ctx, c, sb.String(), args...)
	}
	exec(ctx, c, `COMMIT`)
}

func insertBatch(c *pgx.Conn, t string, rows [][]any) {
	ctx := context.Background()
	const per = 1000
	q := `INSERT INTO ` + t + ` VALUES ($1,$2,$3,$4,$5)`
	exec(ctx, c, `BEGIN`)
	for i := 0; i < len(rows); i += per {
		end := min(i+per, len(rows))
		b := &pgx.Batch{}
		for j := i; j < end; j++ {
			b.Queue(q, rows[j]...)
		}
		must(c.SendBatch(ctx, b).Close())
	}
	exec(ctx, c, `COMMIT`)
}

func copyFrom(c *pgx.Conn, t string, rows [][]any) {
	ctx := context.Background()
	_, err := c.CopyFrom(ctx, pgx.Identifier{t}, ingestCols, pgx.CopyFromRows(rows))
	must(err)
}

// ══════════════════════════════════════════════════════════════
// A3 · แยกต้นทุนของ "commit ทุกแถว" ออกเป็นชิ้นๆ
//
// คนมักพูดลอยๆ ว่า "transaction แพงเพราะ fsync" — demo นี้วัดว่าจริงแค่ไหน
// โดยแยก 3 ตัวแปรออกจากกัน ไม่ให้ปนกัน
//
//	ก. จำนวน round-trip ต่อแถว  (BEGIN/COMMIT = ยิงเพิ่มอีก 2 คำสั่ง)
//	ข. จำนวน commit ต่อแถว      (ลดได้ด้วยการ commit เป็นก้อน)
//	ค. ต้องรอ fsync มั้ย         (synchronous_commit on/off)
//
// วิธีแยก: เทียบแถวที่ต่างกันทีละตัวแปรเดียว
//
//	autocommit (rt=1, commit=1) vs BEGIN+COMMIT (rt=3, commit=1) → ได้ต้นทุน ก.
//	autocommit (commit=1)       vs ทุก 1000 แถว (commit=0.001)   → ได้ต้นทุน ข.
//	แถวเดียวกัน sync=on vs off                                    → ได้ต้นทุน ค.
// ══════════════════════════════════════════════════════════════

type commitShape struct {
	name     string
	rtPerRow float64
	cmPerRow float64
	fn       func(context.Context, *pgx.Conn, [][]any)
}

func demoCommitCost(ctx context.Context, n int) {
	header("A3 · แยกต้นทุนของ commit ออกเป็นชิ้นๆ")
	c := connect(ctx)
	defer c.Close(ctx)

	exec(ctx, c, `DROP TABLE IF EXISTS commit_demo`)
	exec(ctx, c, `CREATE TABLE commit_demo `+ingestDDL)
	rows := makeRows(n)

	// ── วัดพื้นฐานก่อน: ยิงคำสั่งเปล่าๆ ไป-กลับครั้งนึงแพงแค่ไหน
	const warm = 2000
	t0 := time.Now()
	for i := 0; i < warm; i++ {
		_ = scan1[int](ctx, c, `SELECT 1`)
	}
	rt := time.Since(t0) / warm
	fmt.Printf("\nพื้นฐานของเครื่องนี้\n")
	fmt.Printf("   round-trip เปล่า (SELECT 1 ×%d)   %v / ครั้ง\n", warm, rt)
	fmt.Printf("   fsync ดิบ                         ดูด้วย `docker compose exec pg pg_test_fsync`\n")

	shapes := []commitShape{
		{"autocommit INSERT", 1, 1, insertEveryN(1, false)},
		{"BEGIN + INSERT + COMMIT", 3, 1, insertEveryN(1, true)},
		{"commit ทุก 100 แถว", 1.02, 0.01, insertEveryN(100, true)},
		{"commit ทุก 1000 แถว", 1.002, 0.001, insertEveryN(1000, true)},
	}

	fmt.Printf("\nINSERT %d แถว\n\n", n)
	fmt.Printf("  %-26s %7s %10s %11s %11s %11s\n",
		"แบบ", "rt/แถว", "commit/แถว", "sync=on", "sync=off", "ต่าง = fsync")

	per := map[string][2]time.Duration{}
	for _, s := range shapes {
		var d [2]time.Duration
		for i, sync := range []string{"on", "off"} {
			// วัด 3 รอบเอาค่าต่ำสุด — รอบที่ช้ากว่าคือรอบที่โดนอย่างอื่นมาแทรก
			for try := 0; try < 3; try++ {
				exec(ctx, c, `TRUNCATE commit_demo`)
				exec(ctx, c, `SET synchronous_commit = `+sync)
				start := time.Now()
				s.fn(ctx, c, rows)
				if el := time.Since(start) / time.Duration(n); try == 0 || el < d[i] {
					d[i] = el
				}
			}
		}
		per[s.name] = d
		fmt.Printf("  %-26s %7.3f %10.3f %11v %11v %11v\n",
			s.name, s.rtPerRow, s.cmPerRow,
			d[0].Round(time.Microsecond), d[1].Round(time.Microsecond),
			fmtDur(d[0]-d[1]))
	}
	exec(ctx, c, `SET synchronous_commit = on`)

	// ── ถอดสมการ
	auto := per["autocommit INSERT"][0]
	expl := per["BEGIN + INSERT + COMMIT"][0]
	k1000 := per["commit ทุก 1000 แถว"][0]
	fsyncCost := per["autocommit INSERT"][0] - per["autocommit INSERT"][1]

	residual := auto - rt - fsyncCost

	section("ถอดว่าเวลาหายไปไหน (ใช้แบบ autocommit เป็นฐาน)")
	fmt.Printf("  เวลาทั้งหมดต่อแถว                          %v\n", auto.Round(time.Microsecond))
	fmt.Printf("    - รอ fsync  (ส่วนต่าง sync on/off)       %v  %4.0f%%\n",
		fsyncCost.Round(time.Microsecond), 100*float64(fsyncCost)/float64(auto))
	fmt.Printf("    - round-trip (วัดจาก SELECT 1)           %v  %4.0f%%\n",
		rt.Round(time.Microsecond), 100*float64(rt)/float64(auto))
	fmt.Printf("    - ที่เหลือ (parse/plan/execute + WAL)     %v  %4.0f%%\n",
		residual.Round(time.Microsecond), 100*float64(residual)/float64(auto))

	note(fmt.Sprintf("สองตัวแรกรวมกันแล้วเกือบเท่าเวลาทั้งหมด (%v + %v = %v เทียบกับที่วัดได้ %v)",
		rt.Round(time.Microsecond), fsyncCost.Round(time.Microsecond),
		(rt + fsyncCost).Round(time.Microsecond), auto.Round(time.Microsecond)))
	note("  แปลว่างาน parse/plan/execute ของ INSERT ธรรมดา แทบไม่มีต้นทุนเลยเมื่อเทียบกับสองตัวนั้น")
	note("")
	note("สิ่งที่ตัวเลขชุดนี้บอก")
	note(fmt.Sprintf("  1) \"transaction แพงเพราะ fsync\" จริงแค่ครึ่งเดียว — fsync %.0f%%, round-trip %.0f%%",
		100*float64(fsyncCost)/float64(auto), 100*float64(rt)/float64(auto)))
	note(fmt.Sprintf("  2) BEGIN+INSERT+COMMIT ช้ากว่า autocommit %.1f เท่า (%v → %v) ทั้งที่ commit เท่ากันเป๊ะ",
		float64(expl)/float64(auto), auto.Round(time.Microsecond), expl.Round(time.Microsecond)))
	note("     ต้นทุนล้วนๆ คือ round-trip ที่เพิ่มมา 2 ครั้ง → ห่อ transaction รอบคำสั่งเดียวมีแต่เสีย")
	note(fmt.Sprintf("  3) พอ commit เป็นก้อน fsync แทบหายไป (%v → %s ต่อแถว)",
		fsyncCost.Round(time.Microsecond),
		fmtDur(per["commit ทุก 1000 แถว"][0]-per["commit ทุก 1000 แถว"][1])))
	note(fmt.Sprintf("     เหลือแต่ round-trip ที่ลดไม่ได้อีก (%v ≈ %v)",
		k1000.Round(time.Microsecond), rt.Round(time.Microsecond)))
	note("     → เพดานของการ batch คือ 1 round-trip ต่อแถว")
	note("       ต้องข้ามไปใช้ COPY ที่ส่งทุกแถวในการคุยครั้งเดียว ถึงจะทะลุเพดานนี้ (ดู -demo copy)")
	note("")
	note("ข้อจำกัด: วัดบน Docker Desktop (macOS)")
	note("  fsync ลงดิสก์เสมือนใน VM ถูกกว่า NVMe จริงบน bare metal — สัดส่วน fsync บน prod จะสูงกว่านี้")
	note("  round-trip ผ่าน Docker bridge ก็แพงกว่า unix socket บนเครื่องเดียวกัน")
	note("  → อ่าน \"ทิศทาง\" ได้ อย่าเอาเปอร์เซ็นต์ไปอ้างเป็นค่าสากล")
}

// insertEveryN คืน func ที่ยัดข้อมูลโดย commit ทุกๆ per แถว
// explicit=false คือปล่อยให้ PG เปิด-ปิด transaction ให้เอง (autocommit) = ยิงแค่คำสั่งเดียว
func insertEveryN(per int, explicit bool) func(context.Context, *pgx.Conn, [][]any) {
	return func(ctx context.Context, c *pgx.Conn, rows [][]any) {
		q := `INSERT INTO commit_demo VALUES ($1,$2,$3,$4,$5)`
		if !explicit {
			for _, r := range rows {
				exec(ctx, c, q, r...)
			}
			return
		}
		open := false
		for i, r := range rows {
			if i%per == 0 {
				exec(ctx, c, `BEGIN`)
				open = true
			}
			exec(ctx, c, q, r...)
			if i%per == per-1 {
				exec(ctx, c, `COMMIT`)
				open = false
			}
		}
		if open {
			exec(ctx, c, `COMMIT`)
		}
	}
}
