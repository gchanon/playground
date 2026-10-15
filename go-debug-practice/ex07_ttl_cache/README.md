# ex07 — Session cache: panic เป็นระยะ ๆ ตอนโหลดสูง

**ระดับความรุนแรง:** P0 · **คนแจ้ง:** On-call

## อาการที่เจอ

session cache ของ API gateway ปกติดีบน staging และปกติดีตอนทราฟฟิกน้อย
แต่ตอน peak (ประมาณ 09:00 กับ 18:00) pod จะตายด้วยข้อความใดข้อความหนึ่งนี้:

```
fatal error: concurrent map writes
fatal error: concurrent map read and map write
```

ไม่ได้ตายทุก pod และไม่ได้ตายทุก peak บางวันผ่าน peak ไปได้โดยไม่ crash เลย
จุดที่ crash ไม่เคยซ้ำที่เดิมสองครั้ง และมันเป็น `fatal error` ไม่ใช่ `panic`
ดังนั้น recover middleware ของเราจึงจับไม่ได้

ตัวนับ hit/miss บน dashboard ก็เพี้ยนด้วย: ผลบวกของ hit กับ miss น้อยกว่า
จำนวนครั้งที่ request log บอกว่ามีการค้น cache อยู่ตลอด

cache ตัวนี้มี `sync.RWMutex` และทุก exported method ก็ล็อกหมด ซึ่งเป็นเหตุผล
ที่วิศวกรสองคนแรกที่มาดูเรื่องนี้ผ่านไปเฉย ๆ

## สิ่งที่ควรจะเป็น

cache ต้องปลอดภัยเมื่อถูกใช้พร้อมกันจาก goroutine กี่ตัวก็ตาม และตัวนับต้องแม่นยำ

## วิธีทำให้เกิดซ้ำ

```bash
go test -race ./ex07_ttl_cache/
```

(`go test` แบบไม่ใส่ `-race` อาจจะผ่าน — นั่นก็เป็นส่วนหนึ่งของบทเรียน)
