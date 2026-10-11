package main

// kitsecure.go — ก๊อปพฤติกรรมของ kit/v2/database/secure (v2.30.0) มาทั้งดุ้น
// เพื่อให้ playground นี้รันได้เองโดยไม่ต้องดึง private repo
//
// *** ห้ามแก้ให้ "ถูกต้องขึ้น" *** — ของจริงมันเป็นแบบนี้ ถ้าแก้แล้วเลขจะไม่ตรงกับ production

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sync"
)

type secureOption struct {
	PassPhrase string
	IsEncrypt  bool
}

var SecureOption secureOption

// constReader — ตัวต้นเหตุทั้งหมด
//
// สัญญาของ io.Reader คือ "เขียนลง p แล้วคืนจำนวน byte ที่เขียน"
// ตัวนี้โกหก: คืน 1 ทุกครั้งแต่ไม่แตะ p เลย
// io.ReadFull เลยนับครบ 12 แล้วพอใจ ทั้งที่ p ยังเป็นศูนย์ล้วน
type constReader struct{}

func (r *constReader) Read(p []byte) (n int, err error) {
	return 1, nil
}

func createHashByt(key string) (hash []byte, nonce []byte) {
	h := sha256.Sum256([]byte(key))
	hash = h[:]
	block, _ := aes.NewCipher(hash)
	gcm, _ := cipher.NewGCM(block)
	nonce = make([]byte, gcm.NonceSize())
	var rd constReader
	io.ReadFull(&rd, nonce) // ตั้งใจจะสุ่ม แต่ไม่เกิดอะไรขึ้น
	return hash, nonce
}

func Encrypt(data string) string {
	if !SecureOption.IsEncrypt {
		return data
	}
	hash, nonce := createHashByt(SecureOption.PassPhrase)
	block, _ := aes.NewCipher(hash)
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return data
	}
	return hex.EncodeToString(gcm.Seal(nil, nonce, []byte(data), nil))
}

func Decrypt(data string) string {
	if !SecureOption.IsEncrypt {
		return data
	}
	hash, nonce := createHashByt(SecureOption.PassPhrase)
	block, err := aes.NewCipher(hash)
	if err != nil {
		return data
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return data
	}
	enc, err := hex.DecodeString(data)
	if err != nil {
		return data // ← กลืนเงียบ
	}
	plaintext, err := gcm.Open(nil, nonce, enc, nil)
	if err != nil {
		return data // ← กลืนเงียบ ไม่มี log
	}
	return string(plaintext)
}

// fastDecrypt — ของจริงอยู่ใน cust360 decryptsActualFile/decrypt.go
// ต่างจาก Decrypt ตรงที่สร้าง cipher ครั้งเดียวด้วย sync.Once แทนที่จะสร้างใหม่ทุก call
var (
	gcmOnce  sync.Once
	gcmAEAD  cipher.AEAD
	gcmNonce []byte
)

func fastDecrypt(s string) string {
	if !SecureOption.IsEncrypt {
		return s
	}
	gcmOnce.Do(func() {
		hash := sha256.Sum256([]byte(SecureOption.PassPhrase))
		block, _ := aes.NewCipher(hash[:])
		gcmAEAD, _ = cipher.NewGCM(block)
		gcmNonce = make([]byte, gcmAEAD.NonceSize()) // ทำได้เพราะ nonce ของ kit เป็นศูนย์อยู่แล้ว
	})
	enc, err := hex.DecodeString(s)
	if err != nil {
		return s
	}
	plain, err := gcmAEAD.Open(nil, gcmNonce, enc, nil)
	if err != nil {
		return s
	}
	return string(plain)
}
