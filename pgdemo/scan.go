package main

// B3 EXPLAIN · B4 shared_buffers

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ══════════════════════════════════════════════════════════════
// B3 · EXPLAIN
//
// PG ไม่ได้ทำตามที่เราสั่งตรงๆ — มันมี "planner" ที่ประมาณว่าทางไหนถูกกว่า
// แล้วเลือกเอง การอ่าน EXPLAIN ให้เป็นคือทักษะพื้นฐานของคนใช้ PG
//
// ของเด็ดใน demo นี้คือ Index Only Scan:
//   index รู้ค่าในคอลัมน์อยู่แล้ว แต่ PG ยัง "ไม่กล้า" ตอบจาก index เฉยๆ
//   เพราะ index ไม่รู้ว่าแถวนั้นยังมีชีวิตอยู่มั้ย (เป็นผลพวงของ MVCC)
//   มันต้องไปเปิด heap ดู — ยกเว้นหน้านั้นถูกทำเครื่องหมายว่า "ทั้งหน้ามองเห็นได้หมด"
//   ซึ่งคนที่ทำเครื่องหมายนั้นให้คือ VACUUM
// ══════════════════════════════════════════════════════════════

func demoExplain(ctx context.Context) {
	header("B3 · EXPLAIN — planner เลือกแผนยังไง และ Index Only Scan ต้องพึ่ง VACUUM")
	c := connect(ctx)
	defer c.Close(ctx)

	exec(ctx, c, `SET max_parallel_workers_per_gather = 0`) // กันผลวัดเพี้ยนจาก worker
	exec(ctx, c, `DROP TABLE IF EXISTS scan_demo`)
	exec(ctx, c, `CREATE TABLE scan_demo(id bigint, grp int, payload text)
	              WITH (autovacuum_enabled = false)`)
	exec(ctx, c, `INSERT INTO scan_demo
	              SELECT g, g % 5000, repeat('x', 40) FROM generate_series(1, 200000) g`)
	exec(ctx, c, `CREATE INDEX scan_demo_grp ON scan_demo(grp)`)
	exec(ctx, c, `CREATE INDEX scan_demo_id ON scan_demo(id)`)
	// ANALYZE เก็บสถิติให้ planner — แต่ "ไม่" ไปตั้ง visibility map (คนละงานกับ VACUUM)
	exec(ctx, c, `ANALYZE scan_demo`)

	fmt.Printf("\nตาราง 200,000 แถว · grp มี 5,000 ค่า (ค่าละ 40 แถว) · มี index บน grp\n")
	fmt.Printf("heap %s  index %s\n",
		fmtBytes(relSize(ctx, c, "scan_demo")), fmtBytes(idxSize(ctx, c, "scan_demo")))

	explain(ctx, c, "1 แถว จาก index ที่เรียงตรงกับลำดับในลัง (id)",
		`SELECT * FROM scan_demo WHERE id = 123456`)
	explain(ctx, c, "40 แถว แต่กระจายคนละหน้ากันหมด (grp)",
		`SELECT * FROM scan_demo WHERE grp = 7`)
	explain(ctx, c, "4,000 แถว = 2%",
		`SELECT * FROM scan_demo WHERE grp < 100`)
	explain(ctx, c, "160,000 แถว = 80%",
		`SELECT * FROM scan_demo WHERE grp < 4000`)

	section("Index Only Scan — ก่อน VACUUM")
	explain(ctx, c, "นับอย่างเดียว คอลัมน์ที่ต้องใช้มีอยู่ใน index ครบแล้ว",
		`SELECT count(*) FROM scan_demo WHERE grp < 100`)
	note("สังเกตว่า planner 'ไม่ยอม' ใช้ Index Only Scan เลย ทั้งที่ index มีค่าที่ต้องใช้ครบ")
	note("  เพราะ index เก็บแค่ค่า ไม่ได้เก็บว่าแถวนั้นยังมีชีวิตอยู่มั้ย (ผลพ่วงจาก MVCC)")
	note("  ตัวที่บอกได้คือ visibility map ซึ่งยังว่างเปล่าอยู่ → PG ประเมินว่ายังไงก็ต้องเปิด heap ทุกแถว")
	note("  เลยเลือก Bitmap Heap Scan ไปเลยดีกว่า")

	exec(ctx, c, `VACUUM scan_demo`)
	section("Index Only Scan — หลัง VACUUM (คำสั่งเดิมเป๊ะ)")
	explain(ctx, c, "คำสั่งเดียวกับข้างบน",
		`SELECT count(*) FROM scan_demo WHERE grp < 100`)
	note("คำสั่งไม่เปลี่ยน ข้อมูลไม่เปลี่ยน แต่แผนเปลี่ยน — เพราะ VACUUM ไปตั้ง visibility map ให้")
	note("  Heap Fetches: 0 = ไม่ต้องแตะ heap เลยสักหน้า · เทียบ Buffers ของสองแผนดู")

	note("")
	note("สรุป node ทั้ง 4 แบบ")
	note("  Index Scan        เปิดสารบัญแล้ววิ่งไปหยิบทีละชิ้นตามลำดับที่สารบัญบอก")
	note("  Bitmap Heap Scan  เปิดสารบัญจดก่อนว่าต้องไปหน้าไหนบ้าง แล้วค่อยเดินเก็บ 'เรียงหน้า'")
	note("  Seq Scan          เดินอ่านทั้งลังตั้งแต่ต้นจนจบ")
	note("  Index Only Scan   ตอบจากสารบัญล้วนๆ ไม่ต้องเปิดลังเลย (ต้อง VACUUM ก่อน)")
	note("")
	note("ทำไม 1 แถว (id) ได้ Index Scan แต่ 40 แถว (grp) ได้ Bitmap")
	note("  id เรียงตรงกับลำดับที่ของวางอยู่ในลัง → กระโดดครั้งเดียวจบ")
	note("  grp=7 อยู่ที่แถว 7, 5007, 10007, ... กระจายคนละหน้ากันหมด 40 หน้า")
	note("  Bitmap จึงคุ้มกว่า: จดไว้ก่อนว่าต้องไปหน้าไหน แล้วเดินเรียงหน้าเอา ไม่ต้องกระโดดสุ่ม")
	note("")
	note("ตัวเลข Buffers: shared hit = เจอใน shared_buffers · read = ต้องไปเอามาจากดิสก์/OS cache")
}

func explain(ctx context.Context, c *pgx.Conn, label, q string) {
	fmt.Printf("\n   %s\n   %s\n", label, q)
	rows, err := c.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS, COSTS OFF) `+q)
	must(err)
	defer rows.Close()
	for rows.Next() {
		var line string
		must(rows.Scan(&line))
		fmt.Printf("     | %s\n", line)
	}
	must(rows.Err())
}

// ══════════════════════════════════════════════════════════════
// B4 · shared_buffers
//
// shared_buffers = cache กลางที่ทุก backend process แชร์กัน (= buffer pool ของ YottaDB)
// ตั้งไว้ 64MB ใน compose.yml = 8,192 หน้า หน้าละ 8KB
//
// สิ่งที่คนมักไม่รู้: PG ไม่ยอมให้ Seq Scan ตารางใหญ่มาล้าง cache ทิ้ง
// ถ้าตารางใหญ่กว่า shared_buffers/4 มันจะสลับไปใช้ "ring buffer" ขนาด 256KB
// คือวนใช้แค่ 32 หน้าแล้วทิ้ง — ของเดิมใน cache จึงรอด
// ══════════════════════════════════════════════════════════════

func demoBuffers(ctx context.Context) {
	header("B4 · shared_buffers — cache hit / miss และกับดัก ring buffer")
	c := connect(ctx)
	defer c.Close(ctx)

	exec(ctx, c, `SET max_parallel_workers_per_gather = 0`)
	total := scan1[int64](ctx, c, `SELECT count(*) FROM pg_buffercache`)
	fmt.Printf("\nshared_buffers = %s = %d หน้า · ring buffer เริ่มทำงานเมื่อตารางใหญ่กว่า %s\n",
		scan1[string](ctx, c, `SHOW shared_buffers`), total,
		fmtBytes(total*8192/4))

	buildBufTable(ctx, c, "buf_small", 15000)
	buildBufTable(ctx, c, "buf_big", 300000)

	type bufStat struct {
		rows, pages, hit, read int64
		last                   time.Duration
	}
	st := map[string]bufStat{}

	for _, t := range []string{"buf_small", "buf_big"} {
		sz := relSize(ctx, c, t)
		fits := "เล็กกว่า 1/4 ของ shared_buffers → ใช้กลยุทธ์ปกติ"
		if sz > total*8192/4 {
			fits = "ใหญ่เกิน 1/4 ของ shared_buffers → สแกนจะใช้ ring buffer"
		}
		section(fmt.Sprintf("%s · %s (%d หน้า) · %s", t, fmtBytes(sz), pages(sz), fits))

		exec(ctx, c, `SELECT pg_prewarm($1)`, t) // พยายามโหลดเข้า cache ให้หมดก่อน
		fmt.Printf("   หลัง pg_prewarm: อยู่ใน shared_buffers %d หน้า จาก %d หน้า\n",
			cachedPages(ctx, c, t), pages(sz))

		fmt.Printf("   %-6s %10s %12s %12s\n", "รอบ", "เวลา", "cache hit", "ต้องอ่านใหม่")
		var s bufStat
		s.pages = pages(sz)
		s.rows = scan1[int64](ctx, c, `SELECT count(*) FROM `+t)
		for i := 1; i <= 3; i++ {
			hit0, read0 := ioStats(ctx, c, t)
			t0 := time.Now()
			_ = scan1[int64](ctx, c, `SELECT count(*) FROM `+t)
			el := time.Since(t0)
			hit1, read1 := ioStats(ctx, c, t)
			fmt.Printf("   %-6d %10v %12d %12d\n",
				i, el.Round(time.Millisecond), hit1-hit0, read1-read0)
			s.hit, s.read, s.last = hit1-hit0, read1-read0, el
		}
		st[t] = s
		fmt.Printf("   หลังสแกน 3 รอบ: ยังอยู่ใน shared_buffers %d หน้า\n", cachedPages(ctx, c, t))
	}

	// ── เทียบต้นทุนต่อแถว ระหว่างตารางที่ hit 100% กับตารางที่ hit แค่บางส่วน
	//    (วัดต่อ "แถว" ไม่ใช่ต่อ "หน้า" เพราะ buf_small เล็กเกินไป
	//     เวลา 1-2 ms ของมันถูก overhead คงที่ของการยิง query กลืนไปเกือบหมด)
	sm, bg := st["buf_small"], st["buf_big"]
	nsPerRow := func(b bufStat) float64 { return float64(b.last.Nanoseconds()) / float64(b.rows) }
	hitRate := func(b bufStat) float64 { return 100 * float64(b.hit) / float64(b.hit+b.read) }

	section("ต้นทุนต่อแถว เทียบกัน")
	fmt.Printf("   %-11s %10s %8s %9s %12s %11s\n", "ตาราง", "แถว", "หน้า", "เวลา", "cache hit", "ns/แถว")
	for _, b := range []struct {
		n string
		s bufStat
	}{{"buf_small", sm}, {"buf_big", bg}} {
		fmt.Printf("   %-11s %10d %8d %9v %11.0f%% %11.0f\n",
			b.n, b.s.rows, b.s.pages, b.s.last.Round(time.Millisecond),
			hitRate(b.s), nsPerRow(b.s))
	}

	note("buf_small เข้า cache ครบ → รอบ 2-3 เป็น hit ล้วน นี่คือภาพที่ทุกคนคาดไว้")
	note("")
	note("buf_big ต่างออกไป และเป็นจุดที่คนมักเข้าใจผิด")
	note(fmt.Sprintf("  หลัง prewarm มี %d หน้าค้างใน cache แล้ว 'ค้างอยู่เท่านั้นตลอดไป'", bg.hit))
	note(fmt.Sprintf("  อีก %d หน้าโดนอ่านใหม่ทุกรอบ ไม่มีวันเข้า cache ได้เลย", bg.read))
	note("  ring buffer ไม่ได้แปลว่า cache เหลือแค่ 32 หน้า — หน้าที่อยู่ใน cache แล้วยังใช้ได้ปกติ")
	note("  แต่หน้าที่ 'ต้องอ่านเข้ามาใหม่' จะถูกยัดลงวงแหวน 32 หน้าแล้วทิ้ง ไม่เบียดของเดิมออก")
	note("  → สแกนซ้ำกี่รอบก็ไม่มีวันเร็วขึ้น เพราะส่วนที่ขาดไม่เคยถูกเก็บไว้")
	note("  PG ตั้งใจทำแบบนี้ ไม่ใช่บั๊ก — กันไม่ให้รายงานตัวเดียวมาล้าง cache ของทั้งระบบทิ้ง")
	note("")
	note("ผลที่ควรสะดุดตา: ต้นทุนต่อแถวของสองตารางแทบเท่ากัน")
	note(fmt.Sprintf("  ทั้งที่ buf_small hit 100%% ส่วน buf_big hit แค่ %.0f%%", hitRate(bg)))
	note("  → บนเครื่องนี้ cache miss แทบไม่มีราคาเลย")
	note("")
	note("ข้อจำกัดของตัวเลขชุดนี้ (สำคัญมาก อย่าเอาไปอ้างลอยๆ)")
	note("  miss ที่วัดได้ไม่ได้ลงไปถึงดิสก์จริง — Linux ใน VM ยังมี OS page cache รับไว้อีกชั้น")
	note("  ถ้าต้องอ่านจาก NVMe จริงจะอยู่หลัก ~100 µs/หน้า ไม่ใช่หลัก ns แบบนี้")
	note("  ล้าง OS page cache ใน Docker Desktop ไม่ได้ จึงวัดต้นทุนดิสก์จริงจากตรงนี้ไม่ได้")
	note("  สิ่งที่ demo นี้พิสูจน์ได้จริงคือ 'พฤติกรรมของ cache' (ring buffer) ไม่ใช่ 'ราคาของ I/O'")
	note("")
	note("เทียบ YottaDB: ydb_buffers คือ shared_buffers ตัวเดียวกัน")
	note("  แต่ YottaDB ไม่มีกลไก ring buffer แยกสำหรับงานสแกนใหญ่")

	// เก็บกวาด — ตารางพวกนี้ใหญ่ ไม่ต้องเก็บไว้กินดิสก์
	exec(ctx, c, `DROP TABLE buf_small, buf_big`)
}

func buildBufTable(ctx context.Context, c *pgx.Conn, name string, n int) {
	exec(ctx, c, `DROP TABLE IF EXISTS `+name)
	exec(ctx, c, `CREATE TABLE `+name+`(id bigint, payload text)
	              WITH (autovacuum_enabled = false)`)
	exec(ctx, c, `INSERT INTO `+name+`
	              SELECT g, repeat('x', 100) FROM generate_series(1, $1) g`, n)
	// VACUUM เพื่อให้ hint bit ถูกตั้ง ไม่งั้นสแกนรอบแรกจะเขียนกลับลงหน้า ทำให้ตัวเลขเพี้ยน
	exec(ctx, c, `VACUUM `+name)
}

// ioStats ต้องเรียกจาก connection เดียวกับที่ยิง query เพราะ backend เป็นคนส่งสถิติเอง
func ioStats(ctx context.Context, c *pgx.Conn, t string) (hit, read int64) {
	exec(ctx, c, `SELECT pg_stat_force_next_flush()`)
	must(c.QueryRow(ctx,
		`SELECT heap_blks_hit, heap_blks_read FROM pg_statio_user_tables WHERE relname = $1`,
		t).Scan(&hit, &read))
	return
}

func cachedPages(ctx context.Context, c *pgx.Conn, t string) int64 {
	return scan1[int64](ctx, c, `
		SELECT count(*) FROM pg_buffercache b
		JOIN pg_class c ON b.relfilenode = pg_relation_filenode(c.oid)
		WHERE c.relname = $1`, t)
}
