package main

// B5 page — ข้างใน 8KB หน้าหนึ่งมีอะไร

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ══════════════════════════════════════════════════════════════
// B5 · PAGE
//
// page (PG เรียกสลับกับคำว่า block) = หน่วยที่เล็กที่สุดที่ PG ทำอะไรก็ตาม
// อ่านดิสก์ · อยู่ใน shared_buffers · checksum · full page image ใน WAL
// ล้วนคิดเป็น "หน้า" ทั้งหมด แก้ 1 บาทในคอลัมน์เดียวก็ยังต้องขยับทั้ง 8KB
//
// โครงข้างในหน้าหนึ่ง — โตสองทางเข้าหากัน เพื่อให้ที่ว่างเป็นก้อนเดียวตรงกลาง
//
//	┌─ PageHeader 24 B ────────────────────┐
//	├─ ItemId array →→→ (4 B ต่อช่อง)       │  pd_lower ไล่ลงมา
//	│              ที่ว่าง                   │
//	├─ ←←← tuples                           │  pd_upper ไล่ขึ้นไป
//	└─ special space ──────────────────────┘  heap ไม่ใช้ · B-tree ใช้
//
// ของเด็ดใน demo นี้คือ LP_REDIRECT:
//   ItemId ไม่ใช่แค่ตัวเลข offset เฉยๆ แต่เป็น "ชั้นอ้อม" ที่ทำให้ย้าย tuple
//   ภายในหน้าได้โดยที่ index ซึ่งชี้มาที่ (page, ช่อง) ไม่ต้องแก้อะไรเลย
//   YottaDB ไม่มีชั้นนี้เพราะมันเขียนทับที่เดิม
// ══════════════════════════════════════════════════════════════

const pageDemoTable = "pg_demo"

type lineItem struct {
	lp, off, length int
	flags           string
	xmin, xmax      string
	ctid            string
	hotUpdated      bool
	heapOnly        bool
}

// heapItems งัดหน้าที่ 0 ของ heap ออกมาดูทีละช่อง
// แปลงทุกคอลัมน์เป็น text ตั้งแต่ใน SQL เพราะหลัง VACUUM บางช่องเป็น NULL
func heapItems(ctx context.Context, c *pgx.Conn, t string) []lineItem {
	rows, err := c.Query(ctx, `
		SELECT lp, lp_off, lp_len,
		       CASE lp_flags WHEN 0 THEN 'UNUSED'
		                     WHEN 1 THEN 'NORMAL'
		                     WHEN 2 THEN 'REDIRECT'
		                     ELSE 'DEAD' END,
		       COALESCE(t_xmin::text, '-'),
		       COALESCE(t_xmax::text, '-'),
		       COALESCE(t_ctid::text, '-'),
		       COALESCE((t_infomask2 & 16384) <> 0, false),
		       COALESCE((t_infomask2 & 32768) <> 0, false)
		  FROM heap_page_items(get_raw_page($1, 0))
		 ORDER BY lp`, t)
	must(err)
	defer rows.Close()

	var out []lineItem
	for rows.Next() {
		var it lineItem
		must(rows.Scan(&it.lp, &it.off, &it.length, &it.flags,
			&it.xmin, &it.xmax, &it.ctid, &it.hotUpdated, &it.heapOnly))
		out = append(out, it)
	}
	must(rows.Err())
	return out
}

func printItems(items []lineItem) {
	fmt.Println("   ช่อง | offset | ยาว | สถานะ    | xmin   | xmax   | ctid  | หมายเหตุ")
	for _, it := range items {
		memo := ""
		switch {
		case it.flags == "REDIRECT":
			memo = fmt.Sprintf("→ ชี้ต่อไปช่อง %d", it.off)
		case it.flags == "UNUSED":
			memo = "ว่างแล้ว ใช้ซ้ำได้"
		case it.hotUpdated:
			memo = "ถูก UPDATE แล้ว ตัวใหม่อยู่ที่ ctid"
		case it.heapOnly:
			memo = "ตัวใหม่จาก HOT — index ไม่รู้จัก"
		case it.xmax != "-" && it.xmax != "0":
			memo = "ตายแล้ว รอ VACUUM"
		}
		fmt.Printf("   %4d | %6d | %3d | %-8s | %-6s | %-6s | %-5s | %s\n",
			it.lp, it.off, it.length, it.flags, it.xmin, it.xmax, it.ctid, memo)
	}
}

// pageHdr คืนค่าขอบเขตของที่ว่างในหน้า
func pageHdr(ctx context.Context, c *pgx.Conn, rel string, blk int) (lower, upper, special, size int) {
	must(c.QueryRow(ctx,
		`SELECT lower, upper, special, pagesize FROM page_header(get_raw_page($1, $2))`,
		rel, blk).Scan(&lower, &upper, &special, &size))
	return
}

// drawPage วาดสัดส่วนจริงของหน้าให้เห็นว่าที่ว่างอยู่ตรงกลาง
func drawPage(lower, upper, special, size int) {
	fmt.Printf("   ┌─ PageHeader 24 B ─┬─ ItemId %d ช่อง ─┬─ ว่าง %d B ─┬─ tuples %d B ─┐\n",
		(lower-24)/4, upper-lower, size-upper)
	fmt.Printf("   0                  24            lower=%-5d        upper=%-5d   %d\n",
		lower, upper, size)
	if special < size {
		fmt.Printf("   special=%d → มี special space ท้ายหน้า %d B\n", special, size-special)
	}
}

func demoPage(ctx context.Context) {
	header("B5 · PAGE — ข้างใน 8KB หน้าหนึ่งมีอะไร")
	c := connect(ctx)
	defer c.Close(ctx)

	exec(ctx, c, `CREATE EXTENSION IF NOT EXISTS pageinspect`)

	// ── 1. หน่วยพื้นฐาน
	section("1. block_size — หน่วยที่ PG ทำทุกอย่างทีละก้อน")
	bs := scan1[string](ctx, c, `SHOW block_size`)
	fmt.Printf("   block_size = %s byte\n", bs)
	note("ตั้งตอน compile ทั้ง cluster เปลี่ยนทีหลังไม่ได้ — ต่างจาก YottaDB ที่ตั้งต่อ region ตอน MUPIP CREATE")
	note("อ่านดิสก์ · อยู่ใน shared_buffers · checksum · full page image ใน WAL — คิดเป็น 'หน้า' ทั้งหมด")

	// ── 2. heap page หลัง INSERT
	exec(ctx, c, `DROP TABLE IF EXISTS `+pageDemoTable)
	exec(ctx, c, `CREATE TABLE `+pageDemoTable+`(id int, name text)
	              WITH (autovacuum_enabled = false)`) // ปิดแม่บ้านอัตโนมัติ ไม่งั้นเก็บกวาดกลางคัน
	exec(ctx, c, `INSERT INTO `+pageDemoTable+` VALUES (1,'alice'),(2,'bob'),(3,'carol')`)
	exec(ctx, c, `CREATE INDEX pg_demo_id ON `+pageDemoTable+`(id)`)

	section("2. หลัง INSERT 3 แถว — หน้าหน้าตาแบบนี้")
	lower0, upper0, special0, size0 := pageHdr(ctx, c, pageDemoTable, 0)
	drawPage(lower0, upper0, special0, size0)
	printItems(heapItems(ctx, c, pageDemoTable))
	note(fmt.Sprintf("lower=%d = header 24 B + ItemId %d ช่อง × 4 B", lower0, (lower0-24)/4))
	note(fmt.Sprintf("ที่ว่าง = upper - lower = %d B และเป็น 'ก้อนเดียวตรงกลาง' เสมอ ไม่แตกกระจาย", upper0-lower0))
	note("tuple ตัวแรกอยู่ท้ายหน้า ตัวถัดไปไล่ขึ้นมาข้างบน — ItemId ไล่ลงมาจากข้างบน")

	// ── 3. UPDATE
	section("3. UPDATE 1 แถว — MVCC ที่ระดับ byte")
	exec(ctx, c, `UPDATE `+pageDemoTable+` SET name = 'ALICE' WHERE id = 1`)
	lower1, upper1, _, _ := pageHdr(ctx, c, pageDemoTable, 0)
	printItems(heapItems(ctx, c, pageDemoTable))
	fmt.Printf("   lower %d → %d (+%d = ItemId ใหม่ 1 ช่อง)   upper %d → %d (−%d = tuple ใหม่)\n",
		lower0, lower1, lower1-lower0, upper0, upper1, upper0-upper1)
	note("แถวเก่าไม่ได้ถูกลบ — มันได้ xmax แล้ว ctid ชี้ไปบอกว่า 'ตัวใหม่อยู่ช่องไหน'")
	note("นี่คือกลไกที่ demo mvcc วัดได้ว่า ctid เปลี่ยนจาก (0,1) → (0,81) ตอน UPDATE รัวๆ")
	note("โตสองทางเข้าหากัน — ที่ว่างตรงกลางหดลงจากทั้งสองด้าน")

	// ── 4. DELETE
	section("4. DELETE 1 แถว — ไม่มีอะไรหายไปจากหน้า")
	exec(ctx, c, `DELETE FROM `+pageDemoTable+` WHERE id = 2`)
	printItems(heapItems(ctx, c, pageDemoTable))
	note("DELETE = เขียน xmax ลงไปเฉยๆ ไฟล์ไม่เล็กลงแม้แต่ byte เดียว")
	note("ctid ของแถวที่ถูก DELETE ชี้กลับหาตัวเอง — ต่างจาก UPDATE ที่ชี้ไปตัวใหม่")

	// ── 5. VACUUM
	section("5. VACUUM — แม่บ้านมาเก็บ")
	idxBefore := indexPointers(ctx, c)
	exec(ctx, c, `VACUUM `+pageDemoTable)
	lower2, upper2, _, _ := pageHdr(ctx, c, pageDemoTable, 0)
	printItems(heapItems(ctx, c, pageDemoTable))
	fmt.Printf("   lower %d → %d (ไม่ลด!)   upper %d → %d (คืนที่ว่าง %d B)\n",
		lower1, lower2, upper1, upper2, upper2-upper1)
	note("tuple ที่ตายถูกบีบออก แถวที่เหลือถูกเลื่อนไปชิดท้ายหน้า — offset เปลี่ยนหมด")
	note("lower ไม่ลดลง = ItemId array ไม่หด นี่คือ 'line pointer bloat' ที่ VACUUM ธรรมดาเก็บไม่ได้")

	// ── 6. ทำไม index ไม่ต้องรู้เรื่อง
	section("6. ชั้นอ้อมของ ItemId — ทำไมแถวขยับแล้ว index ไม่ต้องแก้")
	idxAfter := indexPointers(ctx, c)
	fmt.Printf("   index ก่อน VACUUM : %v\n", idxBefore)
	fmt.Printf("   index หลัง VACUUM : %v\n", idxAfter)

	// แยกให้ชัดว่าอันไหนรอด อันไหนหาย — สองเคสนี้คนละเรื่องกันคนละสาเหตุ
	kept := map[string]bool{}
	for _, p := range idxAfter {
		kept[p] = true
	}
	for _, p := range idxBefore {
		if kept[p] {
			fmt.Printf("   %-6s รอด  — แถวยังมีชีวิต (ถึงของจริงจะย้ายช่อง/ย้าย offset ไปแล้วก็ตาม)\n", p)
		} else {
			fmt.Printf("   %-6s หาย  — แถวถูก DELETE จริง VACUUM ต้องตามไปลบใน index ด้วย\n", p)
		}
	}
	note("(0,1) รอด ทั้งที่ของจริงย้ายไปอยู่ช่อง 4 แล้ว — เพราะช่อง 1 กลายเป็น REDIRECT ส่งต่อให้")
	note("index ชี้ที่ 'ช่อง' ไม่ได้ชี้ที่ 'offset' → ย้าย tuple ภายในหน้าได้ฟรี ไม่ต้องแตะ index")
	note("index ถูกแก้เฉพาะตอน 'แถวตายจริง' เท่านั้น ไม่ใช่ทุกครั้งที่แถวขยับ")
	note("นี่คือหัวใจของ HOT update: UPDATE ที่ไม่แตะคอลัมน์ที่มี index → ไม่ต้องเขียน index เลยสักตัว")
	note("YottaDB ไม่มีชั้นอ้อมนี้เพราะเขียนทับที่เดิมอยู่แล้ว — แลกกับที่ต้องไล่แก้ index global เองทุกครั้ง")

	// ── 7. special space
	section("7. special space — ที่ที่ heap ไม่ใช้ แต่ B-tree ใช้")
	_, _, hSpecial, hSize := pageHdr(ctx, c, pageDemoTable, 0)
	iLower, iUpper, iSpecial, iSize := pageHdr(ctx, c, "pg_demo_id", 1)
	fmt.Printf("   heap page  : special=%d  pagesize=%d  → special space %d B\n",
		hSpecial, hSize, hSize-hSpecial)
	fmt.Printf("   index page : special=%d  pagesize=%d  → special space %d B\n",
		iSpecial, iSize, iSize-iSpecial)
	drawPage(iLower, iUpper, iSpecial, iSize)
	var prev, next, level int
	must(c.QueryRow(ctx,
		`SELECT btpo_prev, btpo_next, btpo_level FROM bt_page_stats('pg_demo_id', 1)`).
		Scan(&prev, &next, &level))
	fmt.Printf("   ข้างใน special space: btpo_prev=%d  btpo_next=%d  btpo_level=%d\n", prev, next, level)
	note("16 byte ท้ายหน้าเก็บ pointer ไปหน้าพี่น้องซ้าย/ขวาในชั้นเดียวกันของ B-tree")
	note("เพราะมี pointer นี้ สแกน index แบบช่วง (range scan) จึงเดินขวางไปได้เลย ไม่ต้องย้อนขึ้น root")
	note("heap ไม่ต้องใช้เพราะ heap ไม่ใช่ต้นไม้ — เป็นแค่ถุงใส่ของที่ index ชี้เข้ามา")

	// เก็บกวาด — ดิสก์ของ VM เหลือน้อย
	exec(ctx, c, `DROP TABLE IF EXISTS `+pageDemoTable)
}

// indexPointers = รายการ ctid ที่ leaf page ของ index ชี้ไป
func indexPointers(ctx context.Context, c *pgx.Conn) []string {
	rows, err := c.Query(ctx,
		`SELECT ctid::text FROM bt_page_items('pg_demo_id', 1) ORDER BY itemoffset`)
	must(err)
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s string
		must(rows.Scan(&s))
		out = append(out, s)
	}
	must(rows.Err())
	return out
}
