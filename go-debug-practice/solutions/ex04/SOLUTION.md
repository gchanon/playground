# ex04 — ไม่ปิด (และไม่อ่านให้จบ) response body

## บั๊กคืออะไร

```go
resp, err := p.client.Do(req)
if err != nil { ... }

if resp.StatusCode == http.StatusNotFound {
    return Rate{}, fmt.Errorf(...)      // <-- ออกเลย ไม่ปิด ไม่อ่าน body
}
```

ไม่มี `resp.Body.Close()` ที่ไหนเลยในฟังก์ชัน ทุกครั้งที่ provider ตอบ 404
(คู่เงินที่เขาไม่ quote) connection จะค้างและไม่ถูกคืน → socket งอกเรื่อย ๆ
จนชน ulimit → `too many open files`

## ทำไมถึงเกิด (และทำไม "เส้นทางสำเร็จ" ถึงไม่พัง)

จุดที่คนส่วนใหญ่ตอบไม่ครบ: Go คืน connection เข้า pool เมื่อ
**อ่าน body จนถึง EOF _แล้ว_ ปิด** ไม่ใช่แค่ปิดอย่างเดียว

ทดสอบจริงด้วย `httptest` + `ConnState` นับ connection จาก 20 requests:

| พฤติกรรม | จำนวน connection ที่เปิด |
|---|---|
| decode จน EOF แต่ไม่ `Close()` | 1 |
| ไม่อ่าน ไม่ `Close()` | 20 |
| `Close()` เฉย ๆ ไม่อ่านให้จบ | 20 |
| อ่านจนจบ + `Close()` | 1 |

(ตัวเลขนี้วัดจากเครื่องที่สร้างแบบฝึกนี้ Go 1.24.0 darwin/arm64 — พฤติกรรมอาจต่างกันได้
ตามเวอร์ชันและขนาด body ถ้าจะอ้างในสัมภาษณ์ให้บอกว่า "เคยวัดเอง" ไม่ใช่
"สเปกการันตี")

อุปมา: connection คือ **โต๊ะในร้านอาหาร** ที่ร้านอยากหมุนเวียนใช้ซ้ำ
- ลูกค้ากินจนหมดจาน (อ่านจน EOF) → พนักงานรู้ว่าโต๊ะว่าง เก็บไปใช้ต่อได้
- ลูกค้าลุกไปเฉย ๆ โดยเหลืออาหารค้างโต๊ะ (ปิดโดยไม่อ่านจบ) → ร้านไม่กล้า
  ใช้โต๊ะซ้ำ เพราะไม่รู้ว่าของบนโต๊ะเป็นของใคร ต้องทิ้งโต๊ะนั้นไปเลย
- ลูกค้าไม่ลุกเลย (ไม่ปิด) → โต๊ะโดนจองค้างถาวร

เหตุผลเชิงเทคนิค: HTTP keep-alive ใช้ connection เดิมส่ง request ถัดไป
ถ้า byte ของ response เก่ายังค้างในสาย Go ไม่รู้ว่าเหลืออีกกี่ byte
จึงเลือกทิ้ง connection แทนที่จะเสี่ยงอ่านข้อมูลปนกัน

## วิธีแก้ (น้อยที่สุด)

ใส่ `defer` ทันทีหลังเช็ค `err` ของ `Do` — ก่อน `return` ทุกเส้นทาง
และ **drain ก่อนปิด**

```go
 	resp, err := p.client.Do(req)
 	if err != nil {
 		return Rate{}, fmt.Errorf("fetch %s: %w", pair, err)
 	}
+	defer func() {
+		_, _ = io.Copy(io.Discard, resp.Body)
+		_ = resp.Body.Close()
+	}()
```

ข้อควรระวัง: `defer resp.Body.Close()` ต้องอยู่ **หลัง** เช็ค `err`
เพราะเมื่อ `Do` คืน error `resp` จะเป็น `nil` → `defer` จะ panic

ถ้า body อาจใหญ่มาก อย่า drain แบบไม่จำกัด ให้ใช้
`io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))` แล้วปิด
ยอมทิ้ง connection ถ้าเหลือเกินนั้น ดีกว่าเสียเวลาอ่านทิ้งเป็นสิบ MB

## สิ่งที่ควรพูดตอนสัมภาษณ์

1. **บอกกฎทองก่อน** — "ทุกครั้งที่ `http.Client.Do` คืน error เป็น nil
   เราเป็นเจ้าของ `resp.Body` และต้องปิดเสมอ"
2. **ไล่ทุก return path** — ชี้ว่าบั๊กซ่อนอยู่ใน early return ของ 404
   ไม่ใช่ happy path นี่คือเหตุผลที่ `defer` ดีกว่าการปิดด้วยมือ
3. **แยกสองปัญหาให้ชัด** — (ก) fd รั่ว (ข) keep-alive ใช้ซ้ำไม่ได้
   คนส่วนใหญ่ตอบแค่ (ก) การพูดถึง drain จะทำให้ดูระดับ senior
4. **บอกวิธีพิสูจน์** — `lsof -p <pid> | wc -l` ดูแนวโน้ม,
   `curl localhost:6060/debug/pprof/goroutine?debug=1` ดู goroutine ของ
   transport, หรือเขียนเทสนับ connection ด้วย `httptest` + `ConnState`
   แบบที่เทสในโจทย์ทำ
5. **ถ้าถูกถามต่อ** — `http.Client` ที่ไม่ตั้ง `Timeout` ก็อันตรายเท่ากัน
   โค้ดนี้ตั้งไว้ 5s แล้ว ชมได้ และให้พูดถึง `MaxIdleConnsPerHost`
   ที่ต้องสูงพอถ้ายิงไปโฮสต์เดียวเยอะ ๆ
