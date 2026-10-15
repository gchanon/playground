# ex07 — เขียนของที่แชร์กันใต้ `RLock` (data race)

## บั๊กคืออะไร

`Get` ขอ **read lock** แต่ข้างในมีการ **เขียน** ถึง 4 จุด

```go
func (c *Cache) Get(key string) ([]byte, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    e, ok := c.items[key]
    if !ok {
        c.stats["miss"]++        // เขียน map
        return nil, false
    }
    if time.Now().After(e.expiresAt) {
        delete(c.items, key)     // เขียน map
        c.stats["expired"]++     // เขียน map
        c.stats["miss"]++        // เขียน map
        return nil, false
    }
    e.hits++                     // เขียน struct ที่แชร์ผ่าน pointer
    c.stats["hit"]++             // เขียน map
    return e.value, true
}
```

`RLock` ยอมให้ **หลาย goroutine เข้าพร้อมกัน** ดังนั้น reader หลายตัว
เขียน `c.stats` และ `c.items` พร้อมกันได้จริง → `fatal error: concurrent map writes`
และตัวนับ hit/miss ตกหล่น (`++` ไม่ใช่ atomic: อ่าน-บวก-เขียน 3 จังหวะ)
ตรงกับ ticket ที่บอกว่าผลรวม hit+miss น้อยกว่าจำนวน lookup จริง

## ทำไมถึงเกิด

`sync.RWMutex` มีสองโหมด:

| | ใครเข้าได้พร้อมกัน | ใช้เมื่อ |
|---|---|---|
| `RLock` / `RUnlock` | reader หลายตัวพร้อมกัน | **อ่านอย่างเดียว** |
| `Lock` / `Unlock` | ได้ทีละตัว | มีการเขียน |

อุปมา: `RLock` คือ **บัตรเข้าห้องสมุดเพื่ออ่านหนังสือ**
ออกได้พร้อมกันร้อยใบ เพราะอ่านพร้อมกันไม่ชนกัน
แต่โค้ดนี้คือคนถือบัตรอ่าน **แอบเอาปากกาไปขีดเขียนในหนังสือ** ทุกคน
พร้อมกัน — หน้าหนังสือเละ

ที่ทำให้สองคนแรกที่ดูบั๊กนี้ "มองผ่าน" คือ **ทุก method มี lock จริง ๆ**
การเห็น `c.mu.RLock()` อยู่บรรทัดบนสุดทำให้สมองตัดสินว่า "มี lock แล้ว ผ่าน"
สิ่งที่ต้องตรวจไม่ใช่ "มี lock ไหม" แต่คือ **"lock ชนิดถูกกับสิ่งที่ทำไหม"**

อีกจุดที่ลึกกว่า: `c.items[key]` คืน `*entry` (pointer) การอ่าน map
ปลอดภัยใต้ `RLock` แต่ `e.hits++` คือการเขียน **ของที่ pointer ชี้ไป**
ซึ่งอยู่นอกการคุ้มครองของ read lock — เป็นกับดักที่เจอบ่อยในโค้ดจริง

## ทำไม `go test` เฉย ๆ ถึงจับไม่ได้

race detector ไม่ได้เปิดโดยปริยาย เพราะมันทำให้ช้าลง ~5–10 เท่า
และกินหน่วยความจำมากขึ้น ถ้าไม่ใส่ `-race` โปรแกรมอาจผ่านได้เป็นสิบรอบ
แล้วพังใน production ตอนโหลดพีค — ตรงกับ ticket ที่บอกว่า staging ปกติ

```bash
go test -race ./ex07_ttl_cache/
```

## วิธีแก้ (น้อยที่สุด)

`Get` มีการเขียน → ต้องใช้ write lock

```go
 func (c *Cache) Get(key string) ([]byte, bool) {
-	c.mu.RLock()
-	defer c.mu.RUnlock()
+	c.mu.Lock()
+	defer c.mu.Unlock()
```

ถ้า interviewer ถามว่า "แล้วไม่เสีย performance เหรอ" — คำตอบที่ดี:

- ถูกต้องก่อน แล้วค่อยวัด ถ้า `Get` ที่เป็น write lock กลายเป็นคอขวดจริง
  ค่อยปรับ
- ทางที่ยังเก็บ `RLock` ไว้ได้: ย้ายตัวนับออกจาก map ไปเป็น
  `atomic.Int64` คนละตัว (`hits`, `misses`, `expired`) แล้วเปลี่ยน
  `e.hits` เป็น `atomic.Int64` ใน struct — แบบนี้ `Get` ที่เจอ cache hit
  ยังใช้ read lock ได้
- ส่วนการ `delete` ของหมดอายุไม่จำเป็นต้องทำใน `Get` เลย ปล่อยให้
  janitor ที่มีอยู่แล้วเก็บกวาดก็พอ แค่ return miss ออกไป
- ถ้าอ่านหนักมากจริง ๆ ให้พิจารณา sharded map (แบ่งเป็น N bucket
  แต่ละ bucket มี mutex ของตัวเอง)

## สิ่งที่ควรพูดตอนสัมภาษณ์

1. **เปิด `-race` เป็นอย่างแรก** — เห็นคำว่า "intermittent", "เฉพาะตอนพีค",
   "fatal error ไม่ใช่ panic" ให้พูดทันทีว่าจะรันด้วย `-race`
   แล้วอ่าน report ที่บอกทั้งจุดเขียนและจุดอ่านพร้อม stack ของทั้งคู่
2. **แยก `panic` กับ `fatal error`** — `fatal error: concurrent map writes`
   เป็น runtime throw ที่ `recover()` จับไม่ได้ อธิบายตรงนี้ได้จะดูแน่น
   และตอบคำถามใน ticket ว่าทำไม middleware ถึงไม่ช่วย
3. **เปลี่ยนคำถามจาก "มี lock ไหม" เป็น "lock ถูกชนิดไหม"** — ไล่ทุก
   statement ใน critical section แล้วถามว่าอันไหนเป็นการเขียน
4. **ชี้ประเด็น pointer** — `map[string]*entry` ทำให้ "อ่าน map"
   กับ "เขียนของใน entry" เป็นคนละเรื่อง แต่คนมักคิดว่า lock เดียวคุมหมด
5. **เสนอให้ใส่ `-race` ใน CI** — อย่างน้อยใน job ที่รัน integration test
   เพื่อไม่ให้บั๊กแบบนี้หลุดไป production อีก
