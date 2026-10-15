# ex04 — FX rate poller: file descriptor หมด

**ระดับความรุนแรง:** P1 · **คนแจ้ง:** SRE

## อาการที่เจอ

pricing service ตายประมาณทุก 40 นาทีภายใต้โหลดปกติ ด้วยข้อความ:

```
http: Accept error: accept tcp 0.0.0.0:8080: accept4: too many open files
```

สั่ง `lsof` บน pod ที่กำลังรันอยู่ เห็น socket ที่ต่อไปหา rate provider
เป็นพัน ๆ เส้นในสถานะ `ESTABLISHED` / `CLOSE_WAIT` และไต่ขึ้นเรื่อย ๆ
ตั้งแต่ pod เริ่มทำงาน การเพิ่ม `ulimit` แค่เลื่อนเวลาที่จะ crash ออกไป

ฝั่ง rate provider ก็เปิด ticket มาหาเราด้วย เขาเห็นว่าแต่ละ request จาก IP
ของเราเปิด TCP connection ใหม่ทุกครั้ง แทนที่จะใช้ keep-alive connection ซ้ำ
และเขาใกล้จะ rate-limit เราที่ระดับ connection แล้ว

service นี้ poll คู่สกุลเงินประมาณ 24 คู่วนเป็นลูป แบบเรียงทีละตัว จาก
goroutine เดียว ไม่มีอะไรใน service นี้ที่ทำงานแบบ concurrent เลย นั่นคือ
เหตุผลที่ไม่มีใครสงสัยว่าจะมีการรั่ว

ข้อมูลเพิ่มอีกอย่างจาก SRE: จำนวน socket ไต่ขึ้นเป็นสัดส่วนคร่าว ๆ กับจำนวน
คู่สกุลเงินที่ตั้งค่าไว้แล้ว provider *ไม่* มีราคาให้ และทีม pricing เพิ่งเพิ่ม
คู่สกุลเงินแปลก ๆ เข้ามาชุดหนึ่งในสัปดาห์ที่เริ่ม crash พอดี

## สิ่งที่ควรจะเป็น

การ poll แบบเรียงทีละตัวจาก goroutine เดียว ต้องใช้ connection ขาออกเส้นเดียวซ้ำ
การดึงราคา 24 คู่สองรอบ ไม่ควรเปิด socket ถึง 48 เส้น

## วิธีทำให้เกิดซ้ำ

```bash
go test ./ex04_rate_poller/
```
