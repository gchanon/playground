// cryptodemo — วัดพฤติกรรมของ encrypt/decrypt ที่ cust360 ใช้ ด้วยการทดลองจริง ไม่ใช่ท่องตำรา
//
// อุปมาที่ใช้ทั้งไฟล์: การเข้ารหัส = "เอาแผ่นใสที่พิมพ์เลขมั่วๆ มาทาบบนข้อความ"
//
//	keystream = แผ่นใสที่มีเลขมั่วๆ พิมพ์อยู่
//	XOR       = การทาบแล้วผสมกัน (ทาบสองครั้ง = ได้ของเดิม)
//	key       = ตัวบอกว่า "ใครมีสิทธิ์สร้างแผ่นใสนี้"
//	nonce     = ตัวบอกว่า "ข้อความนี้ใช้แผ่นใสแผ่นไหน"  ← ของเราใช้แผ่นเดียวตลอด นั่นคือปัญหา
//
// รัน: go run ./cryptodemo                  (ทุก demo)
//
//	go run ./cryptodemo -demo nonce      (เฉพาะอันเดียว)
//	go run ./cryptodemo -value 1234567890123 -pass mykey
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"flag"
	"fmt"
	"strings"
)

var (
	value string // plaintext ที่ใช้ทดลอง
	pass  string // passphrase ที่ใช้ทดลอง
)

// ksBlock — สร้าง keystream 1 block (16 bytes) จาก nonce + ตัวนับ
//
// GCM ไม่ได้เอาข้อมูลเราเข้า AES ตรงๆ แต่เอา "counter block" เข้าแทน
// counter block = nonce(12 bytes) || ตัวนับ(4 bytes) = 16 bytes พอดีกับที่ AES รับ
func ksBlock(nonce []byte, ctr uint32) []byte {
	cb := make([]byte, 16)
	copy(cb, nonce)
	cb[12] = byte(ctr >> 24)
	cb[13] = byte(ctr >> 16)
	cb[14] = byte(ctr >> 8)
	cb[15] = byte(ctr)

	h := sha256.Sum256([]byte(SecureOption.PassPhrase))
	blk, _ := aes.NewCipher(h[:])
	out := make([]byte, 16)
	blk.Encrypt(out, cb)
	return out
}

// gcmOf — คืนเครื่อง GCM กับ nonce ที่ kit ใช้จริง (nonce ศูนย์ล้วน)
func gcmOf() (cipher.AEAD, []byte) {
	hash, nonce := createHashByt(SecureOption.PassPhrase)
	blk, _ := aes.NewCipher(hash)
	g, _ := cipher.NewGCM(blk)
	return g, nonce
}

func xor(a, b []byte) []byte {
	o := make([]byte, len(a))
	for i := range a {
		o[i] = a[i] ^ b[i]
	}
	return o
}

func head(title string) {
	fmt.Printf("\n\n=== %s ===\n", title)
}

func  sub(title string) {
	fmt.Printf("\n-- %s\n", title)
}

// hx — hex แบบเว้นวรรคทีละ byte ให้คนอ่านออก
func hx(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}
	return strings.Join(parts, " ")
}

func main() {
	demo := flag.String("demo", "all", "all | walk | hash | keystream | nonce | leak | tamper | bench")
	flag.StringVar(&value, "value", "3101700912345", "ค่าที่เอามาทดลอง (เลขบัตรปลอม)")
	flag.StringVar(&pass, "pass", "demo-passphrase", "passphrase สาธิต — ห้ามใส่ของ production")
	flag.Parse()

	SecureOption.PassPhrase = pass
	SecureOption.IsEncrypt = true

	fmt.Printf("\n=== ของเล่นชุดนี้: value=%q  pass=%q ===", value, pass)
	fmt.Println("\n(ค่า default ตรงกับที่เขียนไว้ในเพจ Notion — เทียบเลขกันได้)")

	run := func(name string, f func()) {
		if *demo == "all" || *demo == name {
			f()
		}
	}

	run("walk", demoWalk)           // เดินตั้งแต่ต้นจนจบ 1 รอบ
	run("hash", demoHash)           // ข้างใน createHashByt ทีละบรรทัด
	run("keystream", demoKeystream) // keystream มาจากไหน ทำไมตัวนับเริ่มที่ 2
	run("nonce", demoNonce)         // nonce มีหน้าที่อะไร ซ้ำแล้วพังยังไง
	run("leak", demoLeak)           // ราคาที่จ่ายจาก nonce ศูนย์
	run("tamper", demoTamper)       // แก้ ciphertext แล้วเกิดอะไร
	run("bench", demoBench)         // Decrypt vs fastDecrypt

	fmt.Println()
}
