package main

// ฝั่งอ่าน + concurrency: B1 isolation · B2 lock/deadlock · B3 EXPLAIN · B4 shared_buffers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// sqlState ดึงรหัส error ของ PG ออกมา (40001 = serialization failure, 40P01 = deadlock)
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func seedAcct(ctx context.Context, c *pgx.Conn) {
	exec(ctx, c, `DROP TABLE IF EXISTS acct`)
	exec(ctx, c, `CREATE TABLE acct(id int PRIMARY KEY, bal bigint)`)
	exec(ctx, c, `INSERT INTO acct VALUES (1, 100), (2, 100)`)
}

// ══════════════════════════════════════════════════════════════
// B1 · Isolation
//
// สองคนถอนเงินคนละ 50 จากบัญชีที่มี 100 พร้อมกัน ผลที่ถูกคือ 0
// แต่ละ isolation level จะจัดการเรื่องนี้ต่างกัน
//
// จุดที่น่าสนใจที่สุด: REPEATABLE READ ไม่ได้ "ป้องกัน" ให้เรา
// มันแค่ "ไม่ยอมให้ผ่าน" แล้วโยน error 40001 กลับมาให้เรา retry เอง
// → นี่คือ optimistic concurrency แบบเดียวกับ TP restart ของ YottaDB
//   ต่างกันตรงที่ YottaDB รัน closure ให้ใหม่เอง ส่วน PG ให้เราเขียน loop เอง
// ══════════════════════════════════════════════════════════════

func demoIsolation(ctx context.Context) {
	header("B1 · Isolation — ใครกันไม่ให้สองคนเขียนทับกัน")

	lostUpdate(ctx, "READ COMMITTED")
	lostUpdate(ctx, "REPEATABLE READ")
	retryLoop(ctx)
	writeSkew(ctx, "REPEATABLE READ")
	writeSkew(ctx, "SERIALIZABLE")

	note("READ COMMITTED (default ของ PG) = เงินหายเงียบๆ DB ไม่บอกอะไรเลย")
	note("REPEATABLE READ = DB จับได้ โยน 40001 ให้เรา retry — ข้อมูลไม่พัง แต่เราต้องเขียน retry เอง")
	note("SERIALIZABLE = จับได้แม้แต่ write skew (สองคนแก้คนละแถว แต่พังเงื่อนไขร่วมกัน)")
	note("เทียบ YottaDB: TP restart รัน closure ให้ใหม่อัตโนมัติ — PG โยน error ให้ 'เรา' จัดการ")
	note("  นี่คือเหตุผลว่าทำไมโค้ดที่คุย PG ต้องมี retry loop รอบ transaction เสมอ")
}

func lostUpdate(ctx context.Context, iso string) {
	section("ถอน 50 สองคนพร้อมกันจากยอด 100 · isolation = " + iso)
	s := connect(ctx)
	defer s.Close(ctx)
	seedAcct(ctx, s)

	a, b := connect(ctx), connect(ctx)
	defer a.Close(ctx)
	defer b.Close(ctx)

	exec(ctx, a, `BEGIN ISOLATION LEVEL `+iso)
	exec(ctx, b, `BEGIN ISOLATION LEVEL `+iso)

	balA := scan1[int64](ctx, a, `SELECT bal FROM acct WHERE id = 1`)
	balB := scan1[int64](ctx, b, `SELECT bal FROM acct WHERE id = 1`)
	fmt.Printf("   ทั้งคู่อ่านได้ยอด = %d / %d\n", balA, balB)

	exec(ctx, a, `UPDATE acct SET bal = $1 WHERE id = 1`, balA-50)
	fmt.Printf("   A เขียน %d แล้ว (ยังไม่ commit)\n", balA-50)

	// B จะโดนบล็อกรอ A — ต้องยิงใน goroutine ไม่งั้นค้างทั้งโปรแกรม
	errCh := make(chan error, 1)
	go func() {
		_, err := b.Exec(ctx, `UPDATE acct SET bal = $1 WHERE id = 1`, balB-50)
		errCh <- err
	}()

	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-errCh:
		fmt.Printf("   B ไม่บล็อก?! err=%v\n", err)
	default:
		fmt.Printf("   B ถูกบล็อกรอ A อยู่ (รอมา 200ms แล้ว)\n")
	}

	exec(ctx, a, `COMMIT`)
	err := <-errCh

	if err != nil {
		fmt.Printf("   A commit → B ได้ error: [%s] %v\n", sqlState(err), err)
		exec(ctx, b, `ROLLBACK`)
	} else {
		exec(ctx, b, `COMMIT`)
		fmt.Printf("   A commit → B เขียน %d ผ่านฉลุย แล้ว commit\n", balB-50)
	}

	final := scan1[int64](ctx, s, `SELECT bal FROM acct WHERE id = 1`)
	verdict := "❌ เงินหาย 50 (lost update)"
	if final == 0 {
		verdict = "✅ ถูกต้อง"
	} else if err != nil {
		verdict = "⚠️  B ถูกปฏิเสธ — ยอดถูกตามจำนวนที่สำเร็จ แต่ B ต้อง retry เอง"
	}
	fmt.Printf("   ยอดสุดท้าย = %d  (ที่ถูกคือ 0)  %s\n", final, verdict)
}

func retryLoop(ctx context.Context) {
	section("REPEATABLE READ + retry loop (เลียนแบบสิ่งที่ TP restart ของ YottaDB ทำให้ฟรีๆ)")
	s := connect(ctx)
	defer s.Close(ctx)
	seedAcct(ctx, s)

	a, b := connect(ctx), connect(ctx)
	defer a.Close(ctx)
	defer b.Close(ctx)

	// จัดจังหวะให้ชนกันแน่นอน: ระหว่างที่ B อ่านเสร็จแต่ยังไม่เขียน
	// ให้ A แทรกเข้ามาถอนจนจบรอบแรกพอดี
	interfered := false
	tries := withdraw(ctx, b, 50, "B", func(try int) {
		if interfered {
			return
		}
		interfered = true
		fmt.Printf("   ← ตรงนี้ A แทรกเข้ามาถอน 50 แล้ว commit จบไปเลย\n")
		withdraw(ctx, a, 50, "A", nil)
	})

	final := scan1[int64](ctx, s, `SELECT bal FROM acct WHERE id = 1`)
	ok := map[bool]string{true: "✅ ถูกต้อง", false: "❌"}[final == 0]
	fmt.Printf("   ยอดสุดท้าย = %d  (B ต้องลอง %d รอบถึงจะผ่าน)  %s\n", final, tries, ok)
}

// withdraw = pattern ที่โค้ดฝั่ง PG ของจริงต้องเขียนเอง: ห่อ transaction ด้วย retry loop
// afterRead คือจุดแทรกสำหรับ demo เท่านั้น ของจริงไม่มี
func withdraw(ctx context.Context, c *pgx.Conn, amt int64, who string, afterRead func(try int)) int {
	for try := 1; try <= 5; try++ {
		exec(ctx, c, `BEGIN ISOLATION LEVEL REPEATABLE READ`)
		bal := scan1[int64](ctx, c, `SELECT bal FROM acct WHERE id = 1`)
		if afterRead != nil {
			afterRead(try)
		}
		_, err := c.Exec(ctx, `UPDATE acct SET bal = $1 WHERE id = 1`, bal-amt)
		if err == nil {
			_, err = c.Exec(ctx, `COMMIT`)
		}
		if err == nil {
			fmt.Printf("   %s: อ่านยอดได้ %d → เขียน %d → commit ผ่านในรอบที่ %d\n",
				who, bal, bal-amt, try)
			return try
		}
		exec(ctx, c, `ROLLBACK`)
		fmt.Printf("   %s: รอบที่ %d ชน [%s] → ทิ้งผลที่คำนวณไว้ แล้วเริ่มใหม่ทั้งก้อน\n",
			who, try, sqlState(err))
	}
	return -1
}

func writeSkew(ctx context.Context, iso string) {
	section("write skew: หมอ 2 คนอยู่เวร ต้องเหลืออย่างน้อย 1 · isolation = " + iso)
	s := connect(ctx)
	defer s.Close(ctx)
	exec(ctx, s, `DROP TABLE IF EXISTS oncall`)
	exec(ctx, s, `CREATE TABLE oncall(doctor text PRIMARY KEY, on_call bool)`)
	exec(ctx, s, `INSERT INTO oncall VALUES ('alice', true), ('bob', true)`)

	a, b := connect(ctx), connect(ctx)
	defer a.Close(ctx)
	defer b.Close(ctx)

	exec(ctx, a, `BEGIN ISOLATION LEVEL `+iso)
	exec(ctx, b, `BEGIN ISOLATION LEVEL `+iso)

	// ทั้งคู่เช็คเงื่อนไขเดียวกัน ได้ 2 เหมือนกัน → คิดว่าปลอดภัยที่จะลาตัวเอง
	nA := scan1[int64](ctx, a, `SELECT count(*) FROM oncall WHERE on_call`)
	nB := scan1[int64](ctx, b, `SELECT count(*) FROM oncall WHERE on_call`)
	fmt.Printf("   alice เห็นคนอยู่เวร %d คน · bob เห็น %d คน → ทั้งคู่คิดว่าลาได้\n", nA, nB)

	// แก้คนละแถว จึงไม่มีการบล็อกกันเลย
	exec(ctx, a, `UPDATE oncall SET on_call = false WHERE doctor = 'alice'`)
	exec(ctx, b, `UPDATE oncall SET on_call = false WHERE doctor = 'bob'`)

	_, errA := a.Exec(ctx, `COMMIT`)
	_, errB := b.Exec(ctx, `COMMIT`)
	report := func(who string, err error) {
		if err != nil {
			fmt.Printf("   %s commit ไม่ผ่าน: [%s] %v\n", who, sqlState(err), err)
		} else {
			fmt.Printf("   %s commit ผ่าน\n", who)
		}
	}
	report("alice", errA)
	report("bob", errB)

	left := scan1[int64](ctx, s, `SELECT count(*) FROM oncall WHERE on_call`)
	v := "❌ เหลือ 0 คน — เงื่อนไขพังทั้งที่แต่ละ txn ถูกหมด"
	if left >= 1 {
		v = "✅ เงื่อนไขยังอยู่"
	}
	fmt.Printf("   เหลือคนอยู่เวร = %d  %s\n", left, v)
}

// ══════════════════════════════════════════════════════════════
// B2 · Lock & deadlock
//
// กฎทองของ PG: "ผู้อ่านไม่บล็อกผู้เขียน ผู้เขียนไม่บล็อกผู้อ่าน"
// ทำได้เพราะ MVCC — คนอ่านไปหยิบใบเก่าที่ยังไม่ถูกแก้มาอ่านได้เลย
// ที่บล็อกกันจริงมีแค่ เขียน vs เขียน แถวเดียวกัน
//
// เทียบ YottaDB: crit มีตัวเดียวต่อ region จับตอน commit
//                PG ล็อกเป็นราย "แถว" และมี LWLock ภายในหลายร้อยตัว
// ══════════════════════════════════════════════════════════════

func demoLock(ctx context.Context) {
	header("B2 · Lock — ใครบล็อกใคร และ deadlock")
	lockBasics(ctx)
	deadlock(ctx)

	note("ผู้อ่านไม่เคยรอ — ได้ค่าเก่า (ใบก่อนแก้) กลับไปทันที นี่คือของแถมจาก MVCC")
	note("ผู้เขียนรอผู้เขียนแถวเดียวกันเท่านั้น")
	note("deadlock PG ตรวจเจอเองแล้วฆ่าตัวหนึ่งทิ้ง (SQLSTATE 40P01) ไม่ค้างยาว")
	note("  เวลาที่ใช้ตรวจ ≈ deadlock_timeout (ตั้งไว้ 1s ใน compose.yml)")
}

func lockBasics(ctx context.Context) {
	section("เขียนบล็อกเขียน แต่อ่านไม่โดนบล็อก")
	s := connect(ctx)
	defer s.Close(ctx)
	seedAcct(ctx, s)

	a, b, r, w := connect(ctx), connect(ctx), connect(ctx), connect(ctx)
	for _, c := range []*pgx.Conn{a, b, r, w} {
		defer c.Close(ctx)
	}

	exec(ctx, a, `BEGIN`)
	exec(ctx, a, `UPDATE acct SET bal = 999 WHERE id = 1`)
	fmt.Printf("   A: BEGIN + UPDATE id=1 → 999 (ยังไม่ commit)\n")

	_ = scan1[int64](ctx, r, `SELECT bal FROM acct WHERE id = 2`) // อุ่นเครื่อง
	t0 := time.Now()
	got := scan1[int64](ctx, r, `SELECT bal FROM acct WHERE id = 1`)
	fmt.Printf("   ผู้อ่าน: ได้ %d ใน %v  ← ไม่รอเลย และได้ค่า 'เก่า'\n",
		got, time.Since(t0).Round(time.Microsecond))

	errCh := make(chan error, 1)
	t1 := time.Now()
	go func() {
		_, err := b.Exec(ctx, `UPDATE acct SET bal = 111 WHERE id = 1`)
		errCh <- err
	}()
	time.Sleep(500 * time.Millisecond)

	// ส่องจาก connection ที่ 4 ว่าตอนนี้ใครรอใคร
	rows, err := w.Query(ctx, `
		SELECT pid, wait_event_type || ':' || wait_event, pg_blocking_pids(pid)::text
		FROM pg_stat_activity
		WHERE datname = current_database()
		  AND cardinality(pg_blocking_pids(pid)) > 0`)
	must(err)
	for rows.Next() {
		var pid int32
		var ev, by string
		must(rows.Scan(&pid, &ev, &by))
		fmt.Printf("   pg_stat_activity: pid %d กำลังรอ (%s) ถูกบล็อกโดย pid %s\n", pid, ev, by)
	}
	rows.Close()

	exec(ctx, a, `COMMIT`)
	must(<-errCh)
	waited := time.Since(t1)
	exec(ctx, b, `COMMIT`)
	fmt.Printf("   ผู้เขียนคนที่สอง: รอ %v จนกว่า A จะ commit\n", waited.Round(time.Millisecond))
}

func deadlock(ctx context.Context) {
	section("สร้าง deadlock จริง")
	s := connect(ctx)
	defer s.Close(ctx)
	seedAcct(ctx, s)

	a, b := connect(ctx), connect(ctx)
	defer a.Close(ctx)
	defer b.Close(ctx)

	exec(ctx, a, `BEGIN`)
	exec(ctx, a, `UPDATE acct SET bal = bal + 1 WHERE id = 1`)
	exec(ctx, b, `BEGIN`)
	exec(ctx, b, `UPDATE acct SET bal = bal + 1 WHERE id = 2`)
	fmt.Printf("   A ถือแถว 1 · B ถือแถว 2\n")
	fmt.Printf("   แล้วให้ A ขอแถว 2 พร้อมกับ B ขอแถว 1 → วนรอกันเป็นวง\n")

	type res struct {
		who  string
		err  error
		took time.Duration
	}
	ch := make(chan res, 2)
	t0 := time.Now()
	go func() {
		_, err := a.Exec(ctx, `UPDATE acct SET bal = bal + 1 WHERE id = 2`)
		ch <- res{"A", err, time.Since(t0)}
	}()
	go func() {
		_, err := b.Exec(ctx, `UPDATE acct SET bal = bal + 1 WHERE id = 1`)
		ch <- res{"B", err, time.Since(t0)}
	}()

	// ตัวที่โดนฆ่าจะ error ทันที ตัวที่รอดจะไปต่อได้เพราะอีกตัวปล่อยล็อกแล้ว
	// จึงรอครบทั้งคู่ได้ ไม่ค้าง
	r1, r2 := <-ch, <-ch
	victim, survivor := r1, r2
	if r1.err == nil {
		victim, survivor = r2, r1
	}
	if victim.err == nil {
		fmt.Printf("   ไม่เกิด deadlock (จังหวะไม่ชนกัน) ลองรันใหม่อีกที\n")
	} else {
		fmt.Printf("   PG ฆ่า %s ทิ้งที่ %v: [%s] %v\n",
			victim.who, victim.took.Round(time.Millisecond), sqlState(victim.err), victim.err)
		fmt.Printf("   %s รอดและทำงานต่อได้ที่ %v (พอ %s ถูกฆ่า ล็อกก็ถูกปล่อย)\n",
			survivor.who, survivor.took.Round(time.Millisecond), victim.who)
	}
	_, _ = a.Exec(ctx, `ROLLBACK`)
	_, _ = b.Exec(ctx, `ROLLBACK`)
	detected := victim.took
	fmt.Printf("   เวลาที่ PG ใช้ตรวจเจอ deadlock = %v (deadlock_timeout = %s)\n",
		detected.Round(time.Millisecond), scan1[string](ctx, s, `SHOW deadlock_timeout`))
}
