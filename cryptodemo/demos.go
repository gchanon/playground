package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"testing"
)

// ───────────────────────────────────────────────────────────────────
// 1. walk — เดินเลข 1 ค่า ตั้งแต่ต้นจนจบ แล้วเดินกลับ
// ───────────────────────────────────────────────────────────────────

func demoWalk() {
	head("WALK — ตามค่า 1 ตัว ตั้งแต่ต้นจนจบ")

	pb := []byte(value)
	g, nonce := gcmOf()

	fmt.Println("Pure bytes of value (decimal) : ", pb)

	sub("ขั้น 1 — ข้อความกลายเป็นตัวเลข (คอมไม่รู้จักตัวอักษร)")
	fmt.Printf("  plain = %q  (%d bytes)\n", value, len(pb))
	fmt.Printf("  bytes (hexagonal) = %s\n", hx(pb))

	sub("ขั้น 2 — passphrase กลายเป็นกุญแจ 32 bytes (AES-256 ต้องการเป๊ะ 32)")
	hash, _ := createHashByt(SecureOption.PassPhrase)
	fmt.Printf("  %q --SHA-256--> %s\n", SecureOption.PassPhrase, hex.EncodeToString(hash))

	sub("ขั้น 3 — nonce")
	fmt.Printf("  nonce = %s   ← ศูนย์ล้วน ดู demo nonce ว่าทำไม\n", hx(nonce))

	sub("ขั้น 4 — กุญแจ + nonce ปั่นออกมาเป็น keystream (ไม่เกี่ยวกับข้อมูลเลย)")
	ks := ksBlock(nonce, 2)
	fmt.Printf("  keystream = %s\n", hx(ks))

	sub("ขั้น 5 — XOR ทีละตัว  (plain XOR keystream = cipher)")
	fmt.Println("  pos ตัว   plain   keystream   cipher")
	body := make([]byte, len(pb))
	for i := range pb {
		k := ksBlock(nonce, 2+uint32(i/16))[i%16]
		body[i] = pb[i] ^ k
		if i < 16 {
			fmt.Printf("  %2d   %c    0x%02x      0x%02x        0x%02x\n", i, pb[i], pb[i], k, body[i])
		}
	}
	if len(pb) > 16 {
		fmt.Printf("  ... (อีก %d ตัว ข้ามไป)\n", len(pb)-16)
	}
	fmt.Printf("  body = %s\n", hx(body))

	sub("ขั้น 6 — ต่อ tag 16 bytes ท้าย (ซีลกันแกะ)")
	sealed := g.Seal(nil, nonce, pb, nil)
	fmt.Printf("  body = %s\n", hex.EncodeToString(sealed[:len(pb)]))
	fmt.Printf("  tag  = %s\n", hex.EncodeToString(sealed[len(pb):]))
	fmt.Printf("  body ที่ XOR เองตรงกับของ gcm.Seal ไหม: %v\n",
		hex.EncodeToString(sealed[:len(pb)]) == hex.EncodeToString(body))

	sub("ขั้น 7 — แปลงเป็น hex แล้วเก็บลง YottaDB")
	stored := Encrypt(value)
	fmt.Printf("  %s\n", stored)
	fmt.Printf("  ยาว %d ตัว   สูตร (%d + 16) * 2 = %d\n", len(stored), len(pb), (len(pb)+16)*2)
	fmt.Printf("  ^CCCI(\"CITIZEN_ID_IDX\", %q, <CIF>) = \"\"\n", stored[:12]+"...")

	sub("ขากลับ — Decrypt เดินย้อนทาง ใช้ keystream ชุดเดิมเป๊ะ")
	fmt.Println("  pos cipher  keystream   plain  ตัว")
	for i := 0; i < len(body) && i < 16; i++ {
		k := ksBlock(nonce, 2)[i]
		p := body[i] ^ k
		fmt.Printf("  %2d   0x%02x     0x%02x       0x%02x   %c\n", i, body[i], k, p, p)
	}
	fmt.Printf("  Decrypt(stored) = %q\n", Decrypt(stored))
	fmt.Printf("  ได้ค่าเดิมกลับมาไหม: %v\n", Decrypt(stored) == value)
}

// ───────────────────────────────────────────────────────────────────
// 2. keystream — มันถูกปั่นออกมายังไง
// ───────────────────────────────────────────────────────────────────

func demoKeystream() {
	head("KEYSTREAM — แผ่นใสถูกปั่นออกมายังไง")

	_, nonce := gcmOf()

	sub("counter block = nonce(12 bytes) || ตัวนับ(4 bytes) = 16 bytes พอดีที่ AES รับ")
	for _, c := range []uint32{1, 2, 3, 4} {
		cb := make([]byte, 16)
		copy(cb, nonce)
		cb[15] = byte(c)
		fmt.Printf("  ตัวนับ=%d  block=%s\n            keystream=%s\n",
			c, hex.EncodeToString(cb), hex.EncodeToString(ksBlock(nonce, c)))
	}
	fmt.Println("  → เลื่อนตัวนับไป 1 ได้ keystream คนละเรื่องเลย ไม่ได้คล้ายกันสักนิด")

	sub("ข้อมูลใช้ตัวนับ 2 ไม่ใช่ 1 — ตัวนับ 1 ถูกจองไว้ทำ tag (พิสูจน์ด้วยการลอง)")
	g, _ := gcmOf()
	pb := []byte(value)
	real := g.Seal(nil, nonce, pb, nil)[:len(pb)]
	for _, c := range []uint32{1, 2} {
		got := xor(pb, ksBlock(nonce, c)[:min(len(pb), 16)])
		mark := "ไม่ตรง"
		if hex.EncodeToString(got) == hex.EncodeToString(real[:len(got)]) {
			mark = "ตรงเป๊ะ  ← ข้อมูลใช้ชุดนี้"
		}
		fmt.Printf("  plain XOR ks(ตัวนับ=%d) = %s  %s\n", c, hex.EncodeToString(got), mark)
	}
	fmt.Printf("  ciphertext จริง        = %s\n", hex.EncodeToString(real))

	sub("ค่ายาวเกิน 16 bytes → หมุนตัวนับไปเรื่อยๆ ต่อ block กันเป็นสายยาว")
	long := []byte("99/123 Moo 5 Bangrak Road, Muang, Nonthaburi 11000")
	nblk := (len(long) + 15) / 16
	fmt.Printf("  plaintext %d bytes → ใช้ %d block\n", len(long), nblk)

	var stitched []byte
	for i := 0; i < nblk; i++ {
		ctr := uint32(2 + i)
		lo, hi := i*16, min((i+1)*16, len(long))
		fmt.Printf("    byte %2d-%2d  ตัวนับ=%d  ks=%s\n", lo, hi-1, ctr, hex.EncodeToString(ksBlock(nonce, ctr)))
		stitched = append(stitched, ksBlock(nonce, ctr)...)
	}
	lc := g.Seal(nil, nonce, long, nil)[:len(long)]
	fmt.Printf("  เอา %d block มาต่อกันแล้ว XOR ตรงกับของจริงไหม: %v\n",
		nblk, hex.EncodeToString(xor(long, stitched[:len(long)])) == hex.EncodeToString(lc))
	fmt.Println("  → block สุดท้ายใช้ไม่ครบ ที่เหลือทิ้ง จึงไม่มี padding → ciphertext ยาวเท่า plaintext เป๊ะ")
}

// ───────────────────────────────────────────────────────────────────
// 3. nonce — หน้าที่ของมัน และราคาของการซ้ำ
// ───────────────────────────────────────────────────────────────────

func demoNonce() {
	head("NONCE — ไม่ต้องลับ แต่ห้ามซ้ำ")

	sub("หน้าที่: key บอกว่า 'ใครเปิดได้'  /  nonce บอกว่า 'ใช้แผ่นใสแผ่นไหน'")
	fmt.Println("  nonce ไม่ใช่ความลับ — ระบบมาตรฐานเก็บมันเปิดเผยข้างๆ ciphertext เลย")
	fmt.Println("  กฎข้อเดียวของมันคือ: ห้ามซ้ำภายใต้ key เดียวกัน")

	sub("เปลี่ยน nonce แค่ bit เดียว → keystream คนละเรื่อง")
	n0 := make([]byte, 12)
	n1 := make([]byte, 12)
	n1[11] = 1
	a, b := ksBlock(n0, 2), ksBlock(n1, 2)
	fmt.Printf("  nonce %s → %s\n", hex.EncodeToString(n0), hex.EncodeToString(a))
	fmt.Printf("  nonce %s → %s\n", hex.EncodeToString(n1), hex.EncodeToString(b))
	diff := 0
	for _, v := range xor(a, b) {
		for bit := 0; bit < 8; bit++ {
			diff += int(v>>bit) & 1
		}
	}
	fmt.Printf("  ต่างกัน %d จาก 128 bits — ครึ่งนึงพอดี เท่ากับเดาสุ่มเอา\n", diff)

	sub("แต่ของเรา nonce ซ้ำทุกแถว → two-time pad")
	fmt.Println("  c1 XOR c2 = (p1 XOR ks) XOR (p2 XOR ks) = p1 XOR p2   ← keystream หักล้างทิ้ง")
	fmt.Println()

	p1 := []byte(value)
	p2 := partnerOf(value) // ค่าปลอมอีกใบ ยาวเท่ากัน มีบางหลักเหมือนกัน
	g, nonce := gcmOf()
	c1 := g.Seal(nil, nonce, p1, nil)[:len(p1)]
	c2 := g.Seal(nil, nonce, p2, nil)[:len(p2)]

	fmt.Printf("  p1 = %s  →  c1 = %s\n", string(p1), hex.EncodeToString(c1))
	fmt.Printf("  p2 = %s  →  c2 = %s\n", string(p2), hex.EncodeToString(c2))
	fmt.Println()
	fmt.Printf("  c1 XOR c2 = %s\n", hex.EncodeToString(xor(c1, c2)))
	fmt.Printf("  p1 XOR p2 = %s   ← เท่ากันเป๊ะ กุญแจหายไปจากสมการ\n", hex.EncodeToString(xor(p1, p2)))

	sub("ตำแหน่งที่เป็น 00 แปลว่าสองค่านั้นมีตัวนั้นเหมือนกัน — อ่านออกโดยไม่มีกุญแจ")
	d := xor(p1, p2)
	fmt.Print("  XOR  ")
	for _, v := range d {
		fmt.Printf("%02x ", v)
	}
	fmt.Print("\n  p1   ")
	for _, v := range p1 {
		fmt.Printf(" %c ", v)
	}
	fmt.Print("\n  p2   ")
	for _, v := range p2 {
		fmt.Printf(" %c ", v)
	}
	fmt.Print("\n       ")
	for _, v := range d {
		if v == 0 {
			fmt.Print("^^ ")
		} else {
			fmt.Print("   ")
		}
	}
	fmt.Println("  ← ตรงนี้เหมือนกัน")

	sub("รู้ p1 ใบเดียว → ได้ p2 ทันที")
	fmt.Printf("  p2 = (c1 XOR c2) XOR p1 = %q\n", string(xor(xor(c1, c2), p1)))

	fmt.Println("\n  หมายเหตุ: ยังมีผลงานชื่อ forbidden attack (Joux) บอกว่า nonce ซ้ำใน GCM")
	fmt.Println("  ทำให้ปลอม tag ได้ด้วย — demo นี้ไม่ได้พิสูจน์ข้อนั้น (ต้องหารากของ polynomial ใน GF(2^128))")
}

// partnerOf — สร้างค่าปลอมอีกใบ ยาวเท่าเดิม ให้บางหลักเหมือนบางหลักต่าง
// เพื่อให้เห็นผลของ two-time pad ชัดๆ กับ -value อะไรก็ได้
func partnerOf(s string) []byte {
	out := []byte(s)
	for i, c := range out {
		if c >= '0' && c <= '9' && i%3 != 0 {
			out[i] = '0' + (c-'0'+byte(i%7)+1)%10
		}
	}
	return out
}

// ───────────────────────────────────────────────────────────────────
// 4. leak — ราคาที่จ่ายจาก nonce ศูนย์
// ───────────────────────────────────────────────────────────────────

func demoLeak() {
	head("LEAK — ราคาที่จ่ายจาก nonce ศูนย์")

	sub("6.1 ค่าเดิมได้ ciphertext เดิมเสมอ (นี่คือสิ่งที่ index ต้องการ แต่ก็คือรูรั่ว)")
	for i := 0; i < 3; i++ {
		fmt.Printf("  เข้ารหัสรอบที่ %d: %s\n", i+1, Encrypt(value))
	}

	sub("6.2 ความยาวคำนวณกลับได้ 100%")
	for _, p := range []string{"1", "12345", "0812345678", value} {
		fmt.Printf("  plain=%-16s len(plain)=%2d  len(hex)=%d\n", p, len(p), len(Encrypt(p)))
	}
	fmt.Println("  สูตร: len(hex) = (len(plain) + 16) * 2  — เดาความยาวค่าจริงได้โดยไม่มีกุญแจ")

	sub("6.2 prefix รั่ว — เหมือนกันกี่ตัว ciphertext ก็เหมือนกันเท่านั้น")
	base := Encrypt(value)
	for _, v := range variantsOf(value) {
		e := Encrypt(v)
		fmt.Printf("  %s → %s\n", v, e[:32]+"...")
		fmt.Printf("  %*s   ตรงกับของเดิม %d hex (%d bytes) แรก\n",
			len(v), "", commonPrefix(base, e), commonPrefix(base, e)/2)
	}

	sub("6.3 รู้ plaintext แค่คู่เดียว → ถอดค่าอื่นได้โดยไม่มีกุญแจ")
	known := "1111111111111111111111111111111111111111"
	kct, _ := hex.DecodeString(Encrypt(known))
	ks := make([]byte, len(known))
	for i := range known {
		ks[i] = kct[i] ^ known[i] // กู้ keystream ออกมา ไม่ใช้กุญแจเลย
	}
	target, _ := hex.DecodeString(Encrypt(value))
	out := make([]byte, len(target)-16)
	for i := range out {
		out[i] = target[i] ^ ks[i]
	}
	fmt.Printf("  ผู้โจมตีรู้: %q + ciphertext ของมัน\n", known[:12]+"...")
	fmt.Printf("  ได้มาเพิ่ม : ciphertext ของค่าที่ไม่รู้\n")
	fmt.Printf("  ถอดออกมา  : %q   ← ไม่ได้ใช้กุญแจเลยสักครั้ง\n", string(out))

	fmt.Println("\n  อ่านให้ครบก่อนตกใจ: ทั้งหมดนี้ต้องให้ ciphertext หลุดออกไปก่อน")
	fmt.Println("  ชั้นนี้ยังกันคนเปิด .dat มาดูเฉยๆ ได้ — แต่ถ้าไฟล์หลุด ให้ถือว่าเลขบัตรหลุดด้วย")
}

// variantsOf — ค่าที่ต่างจากเดิมคนละตำแหน่ง เพื่อดูว่า prefix รั่วแค่ไหน
func variantsOf(s string) []string {
	if len(s) < 6 {
		return nil
	}
	b1 := []byte(s)
	b1[len(b1)-1] = bump(b1[len(b1)-1]) // ต่างตัวสุดท้าย
	b2 := []byte(s)
	for i := 5; i < len(b2); i++ { // ต่างตั้งแต่ตัวที่ 5
		b2[i] = bump(b2[i])
	}
	return []string{string(b1), string(b2)}
}

func bump(c byte) byte {
	if c >= '0' && c <= '9' {
		return '0' + (c-'0'+1)%10
	}
	return c
}

// commonPrefix — นับว่า hex สองเส้นเหมือนกันกี่ตัวแรก
func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// ───────────────────────────────────────────────────────────────────
// 5. tamper — tag จับได้ แต่ kit กลืน error
// ───────────────────────────────────────────────────────────────────

func demoTamper() {
	head("TAMPER — มีคนไปแก้ ciphertext ใน DB")

	stored := Encrypt(value)
	raw, _ := hex.DecodeString(stored)
	bad := make([]byte, len(raw))
	copy(bad, raw)
	bad[0] ^= 0x01 // พลิก bit เดียว
	badHex := hex.EncodeToString(bad)

	fmt.Printf("  ของดี  : %s → Decrypt = %q\n", stored[:20]+"...", Decrypt(stored))
	fmt.Printf("  ถูกแก้ : %s → Decrypt = %q\n", badHex[:20]+"...", Decrypt(badHex))

	sub("tag จับได้จริง")
	g, nonce := gcmOf()
	_, err := g.Open(nil, nonce, bad, nil)
	fmt.Printf("  gcm.Open คืน error: %v\n", err)

	sub("แต่ kit กลืน error ทิ้ง")
	fmt.Println("    if err != nil { return data }   ← คืน hex ดิบกลับไป ไม่มี log ไม่ panic")
	fmt.Printf("  → ค่าที่เสียไหลต่อเป็น string ยาว %d ตัว แทนที่จะระเบิด\n", len(Decrypt(badHex)))
	fmt.Println("  → ต้องไปเจอเอาตอน validate ปลายทาง ซึ่งอาจไม่มีใครเช็ค")

	sub("เคสที่เงียบกว่า: ค่าที่ไม่ใช่ hex เลย (เช่นเขียนตอน ENCRYPT_BYPASS_FLAG=true)")
	fmt.Printf("  Decrypt(%q) = %q   ← ไหลผ่านเฉยๆ\n", "3101700912345", Decrypt("3101700912345"))
}

// ───────────────────────────────────────────────────────────────────
// 6. bench — ทำไม cust360 ต้องเขียน fastDecrypt เอง
// ───────────────────────────────────────────────────────────────────

func demoBench() {
	head("BENCH — kit Decrypt vs fastDecrypt")

	addr := "ที่อยู่ 123/45 หมู่ 6 ต.บางรัก อ.เมือง จ.นนทบุรี 11000"
	enc := Encrypt(addr)
	fastDecrypt(enc) // warm up sync.Once ก่อนจับเวลา

	fmt.Printf("  string ที่ใช้วัด: %q (%d bytes)\n\n", addr, len(addr))

	kit := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Decrypt(enc)
		}
	})
	fast := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			fastDecrypt(enc)
		}
	})

	fmt.Printf("  kit Decrypt : %8.0f ns/op  %5d B/op  %2d allocs/op\n",
		float64(kit.NsPerOp()), kit.AllocedBytesPerOp(), kit.AllocsPerOp())
	fmt.Printf("  fastDecrypt : %8.0f ns/op  %5d B/op  %2d allocs/op\n",
		float64(fast.NsPerOp()), fast.AllocedBytesPerOp(), fast.AllocsPerOp())
	fmt.Printf("\n  เร็วขึ้น %.1f เท่า, memory ลดลง %.1f เท่า\n",
		float64(kit.NsPerOp())/float64(fast.NsPerOp()),
		float64(kit.AllocedBytesPerOp())/float64(fast.AllocedBytesPerOp()))

	fmt.Println("\n  ต่างกันแค่ตรงนี้: kit hash passphrase + สร้าง AES + สร้าง GCM ใหม่ทุก call")
	fmt.Println("  fastDecrypt ทำครั้งเดียวด้วย sync.Once แล้วใช้ซ้ำ")
	fmt.Println("  พอ 1 record ถอด ~20 ฟิลด์ x หลายสิบล้าน record งานซ้ำนี้กลายเป็นต้นทุนหลัก")
	fmt.Println("\n  ผลเท่ากันทุก input ไหม (รวม edge case):")
	for _, s := range []string{enc, "not-hex!!", "abcd", "", Encrypt("")} {
		label := s
		if len(label) > 20 {
			label = label[:20] + "..."
		}
		fmt.Printf("    in=%-26q kit=%q fast=%q ตรงกัน=%v\n",
			label, trunc(Decrypt(s)), trunc(fastDecrypt(s)), Decrypt(s) == fastDecrypt(s))
	}
}

// trunc — ตัดตาม rune ไม่ใช่ byte ไม่งั้นภาษาไทยขาดกลางตัวแล้วเพี้ยน
func trunc(s string) string {
	r := []rune(s)
	if len(r) > 10 {
		return string(r[:10]) + "..."
	}
	return s
}

// ───────────────────────────────────────────────────────────────────
// 7. hash — เปิดฝา createHashByt ดูทีละบรรทัด
// ───────────────────────────────────────────────────────────────────

func demoHash() {
	head("HASH — ข้างใน createHashByt ทีละบรรทัด")
	fmt.Println("createHashByt มีบรรทัดที่ทำงานจริง 6 บรรทัด — demo นี้เดินซ้ำบรรทัดเดิมแล้วพิมพ์ของกลางให้ดู")
	fmt.Println("(ไม่ได้ไปแก้ kitsecure.go ให้พิมพ์ — ท้าย demo มีเช็คว่าที่เดินเองตรงกับของจริง)")

	key := SecureOption.PassPhrase

	// ── 1 ────────────────────────────────────────────────────────────
	sub("บรรทัด 1 — h := sha256.Sum256([]byte(key))")
	kb := []byte(key)
	fmt.Printf("  เข้า : %q  (%d bytes)\n", key, len(kb))
	fmt.Printf("         %s\n", hx(kb))
	h := sha256.Sum256(kb)
	fmt.Printf("  ออก  : %s\n", hx(h[:8]))
	fmt.Printf("         %s   (ต่อจากบรรทัดบน)\n", hx(h[8:16]))
	fmt.Printf("         %s\n", hx(h[16:24]))
	fmt.Printf("         %s\n", hx(h[24:]))
	fmt.Printf("  type : [%d]byte — SHA-256 คืน 32 bytes เสมอ passphrase จะยาว 3 ตัวหรือ 3000 ตัวก็ได้เท่านี้\n", len(h))

	fmt.Println("\n  พลิก 1 bit ใน passphrase แล้วดูว่า hash ขยับแค่ไหน:")
	alt := []byte(key)
	alt[len(alt)-1] ^= 0x01
	h2 := sha256.Sum256(alt)
	diff := 0
	for i := range h {
		if h[i] != h2[i] {
			diff++
		}
	}
	fmt.Printf("    %q  -> %s...\n", key, hex.EncodeToString(h[:10]))
	fmt.Printf("    %q  -> %s...\n", string(alt), hex.EncodeToString(h2[:10]))
	fmt.Printf("    ต่างกัน %d/32 bytes ← ขยับนิดเดียวเปลี่ยนยกชุด เลยเดาย้อนกลับไม่ได้\n", diff)
	fmt.Println("    แต่ SHA-256 เปล่าๆ ไม่มี salt ไม่มีวนซ้ำ -> passphrase สั้นก็ยังไล่เดาได้เร็วอยู่ดี")

	// ── 2 ────────────────────────────────────────────────────────────
	sub("บรรทัด 2 — hash = h[:]")
	hash := h[:]
	fmt.Println("  ไม่ได้คำนวณอะไรเพิ่ม แค่เปลี่ยนรูป:")
	fmt.Printf("    h    = [32]byte  array — ส่งต่อแล้ว copy ทั้งก้อน\n")
	fmt.Printf("    hash = []byte    slice — ชี้เข้าไปที่ h ก้อนเดิม ไม่ copy\n")
	fmt.Printf("    len=%d cap=%d  ค่าเหมือนกันทุก byte: %v\n",
		len(hash), cap(hash), hex.EncodeToString(hash) == hex.EncodeToString(h[:]))
	fmt.Println("  ที่ต้องแปลง เพราะ aes.NewCipher รับ []byte ไม่รับ array")

	// ── 3 ────────────────────────────────────────────────────────────
	sub("บรรทัด 3 — block, _ := aes.NewCipher(hash)")
	block, err := aes.NewCipher(hash)
	fmt.Printf("  เข้า : hash ยาว %d bytes\n", len(hash))
	fmt.Printf("  ออก  : block.BlockSize() = %d   err = %v\n", block.BlockSize(), err)
	fmt.Println("  ความยาวกุญแจเป็นตัวตัดสินว่าได้ AES รุ่นไหน — ไม่ได้ตั้งค่าที่อื่น:")
	fmt.Println("     16 bytes -> AES-128")
	fmt.Println("     24 bytes -> AES-192")
	fmt.Println("     32 bytes -> AES-256   ← ของเรา (เพราะ SHA-256 คืน 32 พอดี)")
	fmt.Println("  BlockSize 16 คือ 'AES กินทีละ 16 bytes' — ไม่ใช่ความยาวกุญแจ")
	for _, n := range []int{15, 16, 31, 32} {
		_, e := aes.NewCipher(make([]byte, n))
		fmt.Printf("     ลองกุญแจ %2d bytes -> err = %v\n", n, e)
	}

	// ── 4 ────────────────────────────────────────────────────────────
	sub("บรรทัด 4 — gcm, _ := cipher.NewGCM(block)")
	gcm, err := cipher.NewGCM(block)
	fmt.Printf("  ออก  : gcm.NonceSize() = %d    err = %v\n", gcm.NonceSize(), err)
	fmt.Printf("         gcm.Overhead()  = %d\n", gcm.Overhead())
	fmt.Println("  Overhead 16 คือ tag ที่จะถูกต่อท้าย — นี่คือที่มาของ +16 ในสูตร (len(plain)+16)*2")
	fmt.Printf("  เช็คสูตรกับของจริง: value %d bytes -> hex ยาว %d, สูตรบอก %d\n",
		len(value), len(Encrypt(value)), (len(value)+gcm.Overhead())*2)

	// ── 5 ────────────────────────────────────────────────────────────
	sub("บรรทัด 5 — nonce = make([]byte, gcm.NonceSize())")
	nonce := make([]byte, gcm.NonceSize())
	fmt.Printf("  ออก  : %s   (%d bytes)\n", hx(nonce), len(nonce))
	fmt.Println("  Go รับประกันว่า make() เคลียร์เป็นศูนย์ให้เสมอ — ตรงนี้ยังไม่ผิดอะไร")

	// ── 6 ────────────────────────────────────────────────────────────
	sub("บรรทัด 6 — io.ReadFull(&rd, nonce)   ← บรรทัดที่พัง")
	before := hx(nonce)
	var rd constReader
	n, rerr := io.ReadFull(&rd, nonce)
	fmt.Printf("  ก่อนเรียก : %s\n", before)
	fmt.Printf("  คืนค่า    : n = %d, err = %v   ← อ้างว่าเขียนครบ 12 แล้ว\n", n, rerr)
	fmt.Printf("  หลังเรียก : %s\n", hx(nonce))
	fmt.Printf("  nonce ขยับไหม: %v\n", before != hx(nonce))
	fmt.Println()
	fmt.Println("  เพราะ constReader.Read คืน (1, nil) โดยไม่แตะ p เลย:")
	fmt.Println("     func (r *constReader) Read(p []byte) (n int, err error) { return 1, nil }")
	fmt.Println("  io.ReadFull วนเรียกจนกว่าจะครบ 12 — มันนับจากเลขที่ Read คืนมา")
	fmt.Println("  ไม่ได้ไปดูว่า p เปลี่ยนจริงไหม วน 12 รอบ ได้ 1+1+...=12 ก็จบงาน")
	fmt.Println()
	fmt.Println("  ถ้า constReader ทำตามสัญญา (ลองสลับไปใช้ crypto/rand ดู):")
	for i := 0; i < 3; i++ {
		rn := make([]byte, 12)
		rand.Read(rn)
		fmt.Printf("     เรียกครั้งที่ %d -> %s\n", i+1, hx(rn))
	}
	fmt.Println("  ^ ควรได้ไม่ซ้ำกันแบบนี้ แต่ของจริงได้ 00 ล้วนทุกครั้ง")

	// ── ตรวจ ─────────────────────────────────────────────────────────
	sub("ตรวจว่าที่เดินเองข้างบนตรงกับ createHashByt ของจริง")
	rh, rn := createHashByt(key)
	fmt.Printf("  hash  ตรงกัน: %v\n", hex.EncodeToString(rh) == hex.EncodeToString(hash))
	fmt.Printf("  nonce ตรงกัน: %v\n", hx(rn) == hx(nonce))

	rh2, rn2 := createHashByt(key)
	fmt.Printf("\n  เรียก createHashByt ซ้ำอีกรอบ ได้เท่าเดิมไหม:\n")
	fmt.Printf("    hash  : %v\n", hex.EncodeToString(rh) == hex.EncodeToString(rh2))
	fmt.Printf("    nonce : %v\n", hx(rn) == hx(rn2))
	fmt.Println("  ^ ทั้งฟังก์ชันไม่มีอะไรสุ่มเลย ผลเลยซ้ำเดิมตลอดกาล")
	fmt.Println("    นี่คือรากของทุกอย่างใน demo อื่น — ดู demo nonce ต่อว่าทำไมถึงแก้ไม่ได้")
}
