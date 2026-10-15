# ex08 — goroutine รั่วตอน early return + ไม่ส่ง context ต่อ

PR นี้เขียนสะอาดมาก ตั้งชื่อดี มี options pattern มี `defer resp.Body.Close()`
ครบ — แต่มี **2 บั๊กที่ block merge ได้**

## บั๊กที่ 1 — early return ทิ้ง goroutine ไว้บน unbuffered channel

```go
results := make(chan Result)          // unbuffered

for _, url := range urls {
    go func(url string) {
        sem <- struct{}{}
        defer func() { <-sem }()
        res, err := f.fetchOne(url)
        if err != nil { errs <- err; return }
        results <- res                // <-- ไม่มีคนรับ = ค้างตลอดกาล
    }(url)
}

for i := 0; i < len(urls); i++ {
    select {
    case res := <-results: out = append(out, res)
    case err := <-errs:  return nil, err    // <-- ออกกลางคัน
    case <-ctx.Done():   return nil, ctx.Err()
    }
}
```

พอ URL ใดพัง `FetchAll` return ทันที **ไม่มีใครรับจาก `results` อีกเลย**
worker ที่ fetch สำเร็จจะค้างที่ `results <- res` ตลอดไป และเพราะมันไม่
return `defer func() { <-sem }()` ก็ไม่ทำงาน → ช่อง semaphore ไม่ถูกคืน →
worker ที่เหลือค้างที่ `sem <- struct{}{}` ต่อกันเป็นลูกโซ่

เรียก 1 ครั้ง 32 URL ที่มี 1 ตัวพัง = รั่วประมาณ 31 goroutine **ถาวร**
ตรงกับ ticket เป๊ะ: กราฟ goroutine ขึ้นอย่างเดียว และชันกว่าตอน
provider ไม่เสถียร

อุปมา: หัวหน้าสั่งงานลูกน้อง 32 คน บอกว่า "ทำเสร็จแล้วมา **ยื่นรายงานให้ผม
กับมือ**" (unbuffered = ต้องส่งมือต่อมือ) พอได้ยินว่ามีคนทำพลาด 1 คน
หัวหน้าก็เดินออกจากตึกไปเลย ลูกน้องที่เหลือถือรายงานยืนรอหน้าห้องตลอดกาล
และเพราะยังไม่ได้ส่งงาน ก็ไม่คืนบัตรเข้าห้องให้คิวถัดไปด้วย

## บั๊กที่ 2 — `ctx` ไม่ถูกส่งเข้า request

```go
func (f *Fetcher) fetchOne(url string) (Result, error) {
    req, err := http.NewRequest(http.MethodGet, url, nil)   // <-- ไม่มี ctx
```

`FetchAll` รับ `ctx` มา และ `select` ก็ดู `ctx.Done()` จริง
แต่ **ตัว HTTP request ไม่รู้จัก ctx เลย** พอ client ยกเลิก (ผู้ใช้ปิดแท็บ
หรือ gateway หมดเวลา) `FetchAll` คืนค่าเร็วก็จริง แต่ request ที่ค้างอยู่
ยังวิ่งต่อจนครบ `http.Client.Timeout` = 30 วินาที กิน connection
กินโควต้า provider แล้วค่อยไปค้างที่ `results <- res` ตามบั๊กที่ 1

นี่คือสาเหตุที่ ticket บอกว่า "timeout ฝั่ง client ก็ทำให้ตัวเลขกระโดด"

อุปมา: ยกเลิกออเดอร์กับพนักงานหน้าร้านแล้ว แต่ไม่มีใครเดินไปบอกครัว
ครัวก็ทำอาหารต่อจนเสร็จ แล้วอาหารก็ไปกองทิ้งไว้เฉย ๆ

## วิธีแก้

1. `fetchOne` รับ `ctx` แล้วใช้ `http.NewRequestWithContext`
2. ทำ `results` ให้ buffered เท่าจำนวน url — worker จะส่งผลลัพธ์แล้ว
   จบตัวเองได้เสมอ ไม่ต้องรอคนรับ
3. `wg.Wait()` ก่อน return — การันตีว่าไม่มี goroutine ไหนรอดออกไป
4. เจอ error → `cancel()` ctx ลูก เพื่อหยุด request ที่เหลือทันที

```go
ctx, cancel := context.WithCancel(ctx)
defer cancel()

results := make(chan Result, len(urls))   // buffered
var wg sync.WaitGroup
var once sync.Once
var firstErr error
fail := func(err error) { once.Do(func() { firstErr = err; cancel() }) }

for _, url := range urls {
    wg.Add(1)
    go func(url string) {
        defer wg.Done()
        select {
        case sem <- struct{}{}:
        case <-ctx.Done():
            fail(ctx.Err())
            return
        }
        defer func() { <-sem }()

        res, err := f.fetchOne(ctx, url)
        if err != nil { fail(err); return }
        results <- res
    }(url)
}

wg.Wait()
close(results)
if firstErr != nil { return nil, firstErr }
```

ของจริงในโปรดักชันจะใช้ `errgroup.WithContext` + `g.SetLimit(n)`
ซึ่งทำทั้ง 4 ข้อนี้ให้แล้ว — พูดถึงได้เลย

## สิ่งที่ควรพูดตอนรีวิว PR นี้

1. **ชมก่อน** — `defer resp.Body.Close()`, `io.LimitReader` กัน body
   ระเบิด, options pattern, ตั้งชื่อดี ของพวกนี้ถูกต้องหมด
   การเริ่มด้วยสิ่งที่ถูกทำให้ feedback น่าฟังขึ้นมาก
2. **ตั้งคำถามหลักของ concurrency review** — ถามสองคำถามนี้กับทุก PR
   ที่มี goroutine:
   - "ถ้า function นี้ return **ก่อนเวลา** goroutine ทุกตัวจบไหม"
   - "ใครเป็นคนรับจาก channel นี้ และรับครบไหม"
   ในเคสนี้คำตอบคือ "ไม่" ทั้งสองข้อ
3. **โยงบั๊กกับกราฟ** — "goroutine ขึ้นอย่างเดียว ชันกว่าตอน provider พัง"
   = goroutine leak ที่ผูกกับ error path ไม่ใช่ memory leak ธรรมดา
   พูดแบบนี้แสดงว่าอ่าน metric เป็น
4. **กฎ context** — "ถ้า function รับ `ctx` มา ทุก I/O ข้างในต้องได้รับ
   `ctx` ตัวนั้นต่อ" การมี `ctx` ใน signature แต่ไม่ส่งต่อ แย่กว่าไม่รับเลย
   เพราะหลอกให้คนอ่านคิดว่ายกเลิกได้
5. **เสนอวิธีพิสูจน์** — `runtime.NumGoroutine()` ก่อน/หลังในเทส
   (แบบที่เทสในโจทย์ทำ), `go.uber.org/goleak` ใน `TestMain`,
   หรือ `/debug/pprof/goroutine?debug=2` บน staging จะเห็น stack
   ซ้ำ ๆ หลายพันอันปักที่ `chan send` บรรทัดเดียวกัน
6. **สรุปให้เป็นคำตัดสิน** — "block merge: goroutine leak ที่ error path
   กับ ctx ที่ไม่ถูกส่งต่อ ทั้งสองอย่างแก้ได้ใน PR นี้ ไม่ต้องรื้อ design"
