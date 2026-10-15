# ex01 — เขียนลง map ที่เป็น nil

## บั๊กคืออะไร

`Order.reserved` เป็น `map[string]int` ที่ไม่ถูก export

- ทาง `NewOrder()` → สร้าง map ให้แล้ว (`make(map[string]int)`)
- ทาง `LoadDraft()` → ใช้ `json.Unmarshal` ซึ่ง **ไม่แตะ field ที่ไม่ถูก export** เลย
  `reserved` จึงเป็น `nil`

พอ `Reserve()` ทำ `o.reserved[sku] += qty` ก็ panic:
`assignment to entry in nil map`

## ทำไมถึงเกิด

ใน Go, `nil` map **อ่านได้ แต่เขียนไม่ได้**

```go
var m map[string]int
fmt.Println(m["a"])  // 0  — ไม่ error
m["a"] = 1           // panic
```

อุปมา: `nil` map เหมือน **ที่อยู่ของตู้เอกสารที่ยังไม่ได้ซื้อตู้**
ไปยืนถามว่า "ในลิ้นชัก A มีอะไร" → ตอบว่า "ไม่มีอะไร" ได้ (ไม่มีตู้ ก็ไม่มีของ)
แต่ถ้าจะ "ยัดเอกสารเข้าลิ้นชัก A" → พัง เพราะไม่มีตู้ให้ยัด

ที่หลอกตาคือ `o.available(sku)` เรียก `o.reserved[sku]` ก่อนหน้า **และผ่านฉลุย**
เพราะเป็นการอ่าน บรรทัดที่พังคือบรรทัดถัดไปที่เป็นการเขียน

## วิธีแก้ (น้อยที่สุด)

ให้ทุกทางที่สร้าง `Order` การันตีว่า map ถูกสร้างแล้ว

```go
 	if o.Labels == nil {
 		o.Labels = make(map[string]string)
 	}
+	if o.reserved == nil {
+		o.reserved = make(map[string]int)
+	}
```

สังเกตว่า `Labels` มีโค้ดกันไว้อยู่แล้ว — แปลว่าเคยมีคนเจอบั๊กนี้มาก่อน
แล้วแก้เฉพาะ field ที่ตัวเองเจอ ไม่ได้ไล่ดูให้ครบ

ทางเลือกที่แข็งแรงกว่า (ถ้าสัมภาษณ์ถามต่อ):
- ทำ method `func (o *Order) ensureMaps()` แล้วเรียกจากทุก constructor
- หรือใช้ `UnmarshalJSON` ของตัวเองเพื่อ init หลัง decode เสมอ
- หรือทำให้ `Reserve` lazy-init: `if o.reserved == nil { o.reserved = ... }`

## สิ่งที่ควรพูดตอนสัมภาษณ์

1. **อ่าน panic ก่อน** — `assignment to entry in nil map` บอกชนิดบั๊กตรง ๆ
   ไม่ต้องเดา ให้ไปที่ stack trace บรรทัดบนสุดที่เป็นโค้ดเรา
2. **ตั้งคำถามที่ถูก** — "ทำไมเทสนึงผ่าน อีกเทสพัง ทั้งที่เรียก method เดียวกัน"
   คำตอบอยู่ที่ **ทางสร้าง object ต่างกัน** ไม่ใช่ที่ `Reserve`
3. **พูดถึง unexported field กับ json** — `json.Unmarshal` มองไม่เห็น field
   ตัวพิมพ์เล็ก นี่เป็นกับดักคลาสสิกของ Go
4. **เสนอการป้องกันไม่ให้เกิดซ้ำ** — ไม่ใช่แค่แก้บรรทัดเดียว ให้พูดว่าจะรวม
   logic การ init ไว้ที่เดียว เพราะบั๊กนี้คือ "มี constructor หลายทาง
   แต่ invariant ถูกบังคับแค่ทางเดียว"
