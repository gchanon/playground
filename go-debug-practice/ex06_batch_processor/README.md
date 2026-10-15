# ex06 — Batch processor: งาน export กลางคืนค้าง

**ระดับความรุนแรง:** P1 · **คนแจ้ง:** On-call

## อาการที่เจอ

worker ที่ export ข้อมูลตอนกลางคืนจะส่งงานของวันนั้นไปยัง sink ปลายทาง
เป็น batch แบบจำกัดจำนวน concurrency ตั้งแต่เราเพิ่ม batch size จาก 4 เป็น 500
งานนี้ไม่เคยเสร็จเลย: มันส่งได้ไม่กี่ record แล้วก็ค้างอยู่อย่างนั้นจนกระทั่ง
context timeout 10 วินาทีทำงาน แล้วเราก็ได้ `context deadline exceeded`
โดยที่ของส่วนใหญ่ใน batch ยังไม่ถูกส่ง

สิ่งที่ on-call สังเกตเห็น:

- ถ้า `concurrency = 4` และ batch มี 4 งาน → ทำงานได้สมบูรณ์
- ถ้า `concurrency = 4` และ batch มี 12 งาน → ค้าง
- goroutine dump ที่เก็บตอนค้าง แสดงว่า goroutine ที่เรียกใช้จอดอยู่ใน `Run`
  ส่วน sink ยังปกติดีและว่างอยู่
- ถ้าเพิ่ม `concurrency` ให้เท่ากับ batch size แล้วมันกลับมาทำงานได้
  ซึ่งเป็น workaround ที่ on-call ใช้อยู่ทุกคืนตอนนี้

## สิ่งที่ควรจะเป็น

batch size เท่าไหร่ก็ต้องทำงานได้กับ concurrency limit เท่าไหร่ก็ได้
`Run` ต้องส่งงานครบทุกชิ้นแล้ว return

## วิธีทำให้เกิดซ้ำ

```bash
go test ./ex06_batch_processor/
```
