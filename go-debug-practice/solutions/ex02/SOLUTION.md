# ex02 — slice aliasing: `append` ไปทับข้อมูลของ slice แม่

## บั๊กคืออะไร

```go
batch := sorted[start:end]          // slice ย่อย — ใช้ array เดียวกับ sorted
batch = append(batch, p.handlingLine())
```

`sorted[start:end]` ได้ slice ที่ `len = end-start` แต่ **`cap` ยาวไปจนสุด
array เดิม** พอ `append` แล้วยังมี cap เหลือ Go จะ **ไม่สร้าง array ใหม่**
แต่เขียนทับช่องถัดไป ซึ่งก็คือ `sorted[end]` — ของ batch ถัดไปนั่นเอง

ผลที่เห็น (picklist 7 ชิ้น, batchSize 3):

| รอบ | batch | append ไปเขียนที่ | ของที่หาย |
|---|---|---|---|
| 1 | `sorted[0:3]` cap 7 | `sorted[3]` | SKU-D |
| 2 | `sorted[3:6]` cap 4 | `sorted[6]` | SKU-G |
| 3 | `sorted[6:7]` cap 1 | cap เต็ม → copy ใหม่ | ไม่หาย |

จึงเห็น handling line โผล่หัว manifest ที่ 2 (เพราะ `sorted[3]` ถูกทับ)
และ SKU-D, SKU-G หายไปเฉย ๆ

## ทำไมถึงเกิด

slice คือ **ป้ายชี้ 3 ค่า: pointer ไป array, len, cap** — ไม่ใช่ตัวข้อมูล

อุปมา: array คือ **แถวตู้ล็อกเกอร์ยาว ๆ แถวเดียว**
`sorted[0:3]` ไม่ใช่การยกล็อกเกอร์ 3 ตู้ออกมา แต่คือ **บอกว่า "ฉันดูแลตู้ 0–2"**
`append` แปลว่า "เอาของใส่ตู้ถัดไป" — ก็คือตู้ 3 ที่เพื่อนกำลังดูแลอยู่

`cap` คือ "ฉันเอื้อมถึงตู้ได้ถึงเบอร์ไหน" ตราบใดที่ยังเอื้อมถึง Go จะไม่ย้ายแถวใหม่

ที่หลอกตามากคือ **`TestPlanDoesNotMutateInput` ผ่าน** เพราะ `Plan` copy
picklist ของ caller ไว้แล้วจริง ๆ ความเสียหายเกิดกับ `sorted` ซึ่งเป็นของภายใน

## วิธีแก้ (น้อยที่สุด)

copy batch ออกมาก่อน append และจอง cap เผื่อ handling line 1 ช่อง

```go
-		batch := sorted[start:end]
+		batch := make([]Line, end-start, end-start+1)
+		copy(batch, sorted[start:end])
```

ทางเลือกอื่นที่ต้องรู้จัก — **full slice expression** `a[low:high:max]`
ซึ่งจำกัด cap ไว้ ทำให้ `append` ถูกบังคับให้ copy ใหม่เสมอ

```go
batch := sorted[start:end:end]   // cap = end-start → append จะ copy ให้เอง
```

วิธีนี้สั้นกว่า แต่จงใจให้ append จัดสรรใหม่ทุกครั้ง ซึ่งอ่านยากกว่าถ้าคนอ่าน
ไม่รู้จักไวยากรณ์ 3 ช่อง — ในสัมภาษณ์ควรพูดถึงทั้งสองแบบ

## สิ่งที่ควรพูดตอนสัมภาษณ์

1. **บอกชื่อปัญหาให้ตรง** — "slice aliasing" หรือ "append ที่มี spare capacity
   ไปเขียนทับ backing array ร่วม"
2. **อธิบายด้วย len vs cap** — ชี้ให้เห็นว่า `sorted[3:6]` มี `len 3` แต่ `cap 4`
   ใช้ `fmt.Println(len(b), cap(b))` ระหว่าง debug ได้เลย
3. **โยงกับอาการ** — "ของหายพอดีกับจำนวน batch - 1" เป็นเบาะแสว่าแต่ละรอบ
   ทำลาย 1 ชิ้น ไม่ใช่ตรรกะ fee ผิด
4. **เตือนเรื่องกฎทั่วไป** — ฟังก์ชันที่รับ slice แล้ว `append` ต่อ
   ถือว่าอันตรายเสมอถ้าไม่ได้เป็นเจ้าของ backing array
   ควร copy หรือ จำกัด cap ตั้งแต่จุดที่ตัด slice
