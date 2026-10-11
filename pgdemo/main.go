// pgdemo — ดูความสามารถของ PostgreSQL ด้วยการวัดจริง ไม่ใช่ท่องตำรา
//
// เปิด server ก่อน:
//
//	cd pgdemo && docker compose up -d
//	go run ./pgdemo                 # รันทุก demo
//	go run ./pgdemo -demo mvcc      # รันทีละตัว
//
// อุปมาที่ใช้ทั้งชุด
//
//	heap    = ลังเก็บของ ไม่เรียง ช่องไหนว่างก็ยัดลงไป
//	index   = สารบัญที่เรียงแล้ว บอกว่าของชิ้นนั้นอยู่ลังไหน ช่องไหน
//	MVCC    = แก้ของ ไม่ได้ลบของเก่า แต่เขียนใบใหม่วางข้างๆ ใบเก่ารอแม่บ้านมาเก็บ
//	VACUUM  = แม่บ้านที่มาเก็บใบเก่า
//	WAL     = สมุดที่ต้องจดว่า "จะทำอะไร" ก่อนเสมอ แล้วค่อยไปทำของจริง
//	commit  = ตอนที่ต้องมั่นใจว่าสมุดถึงดิสก์จริงแล้ว (fsync) — ตรงนี้แหละที่แพง
package main

import (
	"context"
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all",
		"all | mvcc | copy | commit | isolation | lock | explain | buffers | page")
	n := flag.Int("n", 10000, "จำนวนแถวที่ยัด (demo copy)")
	u := flag.Int("u", 20000, "จำนวนครั้งที่ UPDATE (demo mvcc)")
	cn := flag.Int("cn", 3000, "จำนวนแถว (demo commit) — ต่ำไว้เพราะ fsync ต่อแถวช้ามาก")
	flag.Parse()

	ctx := context.Background()
	c := connect(ctx)
	defer c.Close(ctx)

	fmt.Printf("\n=== PostgreSQL %s | shared_buffers=%s | autovacuum=%s | fsync=%s ===\n",
		scan1[string](ctx, c, `SHOW server_version`),
		scan1[string](ctx, c, `SHOW shared_buffers`),
		scan1[string](ctx, c, `SHOW autovacuum`),
		scan1[string](ctx, c, `SHOW fsync`))

	run := func(name string, f func()) {
		if *demo == "all" || *demo == name {
			f()
		}
	}

	run("mvcc", func() { demoMVCC(ctx, *u) })
	run("copy", func() { demoCopyVsInsert(ctx, *n) })
	run("commit", func() { demoCommitCost(ctx, *cn) })
	run("isolation", func() { demoIsolation(ctx) })
	run("lock", func() { demoLock(ctx) })
	run("explain", func() { demoExplain(ctx) })
	run("buffers", func() { demoBuffers(ctx) })
	run("page", func() { demoPage(ctx) })

	fmt.Println()
}
