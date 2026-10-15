# ex02 — ช่วงออเดอร์ยาวสุดที่ไม่มีร้านซ้ำ (sliding window)

**เวลา:** 20 นาที · **ระดับ:** medium

## โจทย์

ทีม growth อยากรู้ว่าผู้ใช้คนหนึ่ง "ลองร้านใหม่ต่อเนื่อง" ได้นานแค่ไหน
ให้ประวัติการสั่งเป็นลำดับ restaurant ID หาความยาวของ**ช่วงที่ติดกัน**ที่ยาวที่สุด
ซึ่งไม่มีร้านไหนซ้ำในช่วงนั้น

```go
func LongestDistinctRun(ids []string) int
```

## ตัวอย่าง

| ids | ผลที่ถูก | ช่วงนั้นคือ |
|---|---|---|
| `kfc mk kfc mk som kfc` | 3 | `mk som kfc` |
| `p w w k e w` | 3 | `w k e` |
| `a b b a` | 2 | `a b` หรือ `b a` |
| `x x x x` | 1 | |
| (ว่าง) | 0 | |

## ข้อจำกัด

- `len(ids)` สูงสุด 200,000
- **เป้า:** O(n) เทสมี input ใหญ่ที่จับเวลา 2 วินาที

## รัน

```bash
go test ./ex02_distinct_run/
```

## คำถามต่อยอดที่ interviewer มักถาม

1. ถ้ายอมให้**แต่ละร้านโผล่ในช่วงได้ไม่เกิน k ครั้ง** (โจทย์เดิมคือ k = 1) ต้องเปลี่ยนอะไร
2. ถ้า ID เป็นตัวอักษร a–z อย่างเดียว ทำให้เร็วขึ้นหรือใช้หน่วยความจำน้อยลงได้ไหม
3. ถ้าข้อมูลไหลเข้ามาเรื่อยๆ (stream) ไม่มีจุดจบ ต้องเก็บอะไรไว้บ้าง

---

## บันทึกหลังทำ (14 ต.ค. 2026)

### ภาพที่ใช้คิด: หนอนกินใบไม้

ออเดอร์คือใบไม้เรียงเป็นแถว หนอนคลานจากซ้ายไปขวา **ในตัวหนอนห้ามมีร้านซ้ำ**
- หัวกินไปข้างหน้าทีละใบเสมอ (= index ของลูป)
- ถ้าใบที่หัวจะกินมีร้านนี้อยู่ในตัวแล้ว → ตัวหนอนต้องเริ่มใหม่**ถัดจากร้านเก่าตัวนั้น**
- ทุกครั้งที่กินเสร็จ จดความยาวที่ยาวสุดไว้ = คำตอบ

### โค้ดที่ทำผ่าน

```go
func LongestDistinctRun(ids []string) int {
	bestDist := 0
	normalDist := 0
	seenMap := make(map[string]int)

	for index, shopData := range ids {
		lastSeenIndex, found := seenMap[shopData]

		if found {
			if normalDist >= (index - lastSeenIndex) {
				normalDist = index - lastSeenIndex
			} else {
				normalDist++
			}
		} else {
			normalDist++
		}

		bestDist = max(bestDist, normalDist)
		seenMap[shopData] = index
	}

	return bestDist
}
```

| ตัวแปร | ในภาพ |
|---|---|
| `normalDist` | ความยาวหนอนตอนนี้ |
| `bestDist` | หนอนยาวสุดที่เคยเห็น |
| `seenMap[ร้าน]` | ร้านนี้โผล่ล่าสุดที่ index ไหน |
| `index - lastSeenIndex` | ความยาวตั้งแต่**ถัดจากร้านเก่า**จนถึงร้านใหม่ (รวมทุกร้านที่คั่นอยู่) |
| `normalDist >= index - lastSeenIndex` | ร้านเก่าตัวนั้น**ยังอยู่ในหน้าต่าง**ไหม |

- **O(n) time, O(จำนวนร้านไม่ซ้ำ) space** ผ่านเทสทั้งหมด รวม `-race`
- เป็นแบบ "กระโดดทีเดียว" ไม่ต้องให้หางหดทีละใบ (หางแปลงจากตัวแปรได้: หาง = `index - normalDist + 1`)
- เคส `a b b a` ผ่านถูก เพราะเงื่อนไขในกิ่ง `if` เช็กว่าร้านเก่ายังอยู่ในหน้าต่างไหม ถ้าหลุดไปแล้วก็ยืดต่อ

### บั๊กรอบแรก: `normalDist = 1`

ตอนแรกตั้งค่าเป็น `1` เมื่อเจอร้านซ้ำในหน้าต่าง เทสเดิมผ่านทั้งหมด แต่ `a b c b d e` ได้ 3 แทน 4

```
index:  0  1  2  3
        a  b  c  b      ← หัวจะกิน b ตัวที่สอง
              [====]    ← หนอนใหม่คือ c b ยาว 2 ไม่ใช่ 1
```

`= 1` ถูกเฉพาะตอนร้านเก่าอยู่**ติดกัน**กับร้านใหม่ ถ้ามีร้านคั่นจะทิ้งร้านที่คั่นไปด้วย แก้เป็น `index - lastSeenIndex`
เทสเดิมผ่านเพราะทุกเคส **คำตอบที่ยาวสุดเกิดขึ้นก่อนจังหวะที่ reset ผิด** (ตอนนี้เพิ่มเคส `a b c b d e` ในเทสแล้ว)

**บทเรียน:** เทสผ่านไม่ได้แปลว่าถูก ตอนไล่ด้วยมือ ให้เลือกเคสที่บังคับให้ทุกกิ่งของ `if` ทำงาน

### เขียนให้สั้นลง (ไม่บังคับ)

```go
if found {
	normalDist = min(normalDist+1, index-lastSeenIndex)
} else {
	normalDist++
}
```

- ร้านเก่าอยู่ในหน้าต่าง → `index-lastSeenIndex` น้อยกว่า จึงถูกเลือก (reset)
- ร้านเก่าหลุดไปแล้ว → `normalDist+1` น้อยกว่าหรือเท่ากัน จึงถูกเลือก (ยืดต่อ)

### คำถามต่อยอด 1: แต่ละร้านโผล่ได้ไม่เกิน k ครั้ง

```
ids = a b a c a, k = 2  →  4 ("b a c a")
```

**คำตอบรอบแรก:** เก็บ `[lastIndex, numberOfFound]` แล้วเทียบตัวนับกับ k ยังไม่ได้ เพราะ
1. ตัวนับนับตั้งแต่ต้น ไม่ได้นับเฉพาะในหน้าต่าง พอร้านหลุดหน้าต่างไปแล้วตัวนับก็ไม่ลด
2. `lastIndex` บอกจุดเริ่มหน้าต่างใหม่ไม่ได้: ตอน index 4 ของตัวอย่าง ต้องเริ่มถัดจาก `a` **ตัวแรก** (index 0) ถึงจะเหลือ `a` 2 ตัว ถ้าใช้ `lastIndex` (= 2) จะได้แค่ `c a` = 2

**แก้โดยคงโครงเดิมไว้:** เปลี่ยนจากเก็บ index ล่าสุด 1 ตัว เป็นเก็บ index ล่าสุด **k ตัว** ของแต่ละร้าน แล้วกระโดดไปถัดจาก**ตัวที่เก่าที่สุดในนั้น** (ตัวที่ k นับย้อนหลัง) พอ k = 1 จะกลายเป็นโค้ดเดิมพอดี

```go
func LongestRunAtMostK(ids []string, k int) int {
	bestDist := 0
	normalDist := 0
	seenMap := make(map[string][]int) // ร้าน → index ล่าสุดไม่เกิน k ตัว เรียงจากเก่าไปใหม่

	for index, shopData := range ids {
		seen := seenMap[shopData]
		if len(seen) == k {
			// seen[0] คือร้านนี้ตัวที่ k นับย้อนหลัง ถ้าหน้าต่างรวมตัวนี้ด้วยจะมี k+1 ตัว
			normalDist = min(normalDist+1, index-seen[0])
			seen = seen[1:]
		} else {
			normalDist++
		}
		bestDist = max(bestDist, normalDist)
		seenMap[shopData] = append(seen, index)
	}

	return bestDist
}
```

- O(n) time, O(จำนวนร้าน × k) space
- อีกทางที่ตรงไปตรงมากว่า: กลับไปใช้หนอนหดทีละใบ เก็บ `count[ร้าน]` เฉพาะในหน้าต่าง ถ้าร้านที่หัวเพิ่งกินเกิน k ให้หางหดทีละใบ (ลด count ของใบที่หลุด) จนกลับมาไม่เกิน ก็ได้ O(n) เหมือนกัน
- ทดสอบแล้ว: ผลตรงกับวิธีลองทุกช่วง (brute force) บน input สุ่ม 3,000 ชุด ด้วย k = 1 ถึง 4

### คำถามต่อยอด 2: ID เป็น a–z อย่างเดียว

**คำตอบรอบแรก:** `make(map[string]int, 26)` ช่วยแค่ไม่ให้ map ต้องขยายตัวกลางทาง ซึ่งเป็นเรื่องเล็ก
(และ 26 key ไม่ได้อยู่ใน bucket เดียว map ของ Go 1.24 แต่ละ group มี 8 ช่อง)

**สิ่งที่ควรพูดให้ชัด: space เปลี่ยนจาก O(n) เป็น O(1)**
- โจทย์ปกติ space = O(จำนวนร้านไม่ซ้ำ) แย่สุดคือ O(n)
- พอรู้ว่ามีได้แค่ 26 แบบ ที่เก็บก็ไม่มีทางเกิน 26 = **O(1)**

**คำตอบที่ดีกว่า: ใช้ array 26 ช่องแทน map** ไม่ต้อง hash และไม่ต้อง allocate

```go
func LongestDistinctRunAZ(ids []string) int {
	bestDist := 0
	normalDist := 0
	var seenAt [26]int // เก็บ index+1 เพราะค่าเริ่มต้น 0 ต้องแปลว่า "ยังไม่เคยเห็น"

	for index, shopData := range ids {
		slot := shopData[0] - 'a'
		if seenAt[slot] > 0 {
			lastSeenIndex := seenAt[slot] - 1
			normalDist = min(normalDist+1, index-lastSeenIndex)
		} else {
			normalDist++
		}
		bestDist = max(bestDist, normalDist)
		seenAt[slot] = index + 1
	}

	return bestDist
}
```

วัดด้วย 200,000 ID สุ่ม a–z (ผลตรงกับแบบ map):

| | เวลา | หน่วยความจำ |
|---|---|---|
| `map[string]int` | 11.06 ms | 1,384 B, 5 allocs |
| `[26]int` | **0.35 ms** (เร็วกว่า 31 เท่า) | **0 B** |

Big-O เท่ากัน แต่ค่าคงที่ต่างกันมาก เป็นจุดที่ Big-O ไม่ได้บอก

### คำถามต่อยอด 3: ข้อมูลมาแบบ stream

**คำตอบรอบแรก:** ต้องเก็บ `bestDist` กับ `normalDist` ✅ ถูก แต่ "`seenMap` โตเรื่อยๆ ทิ้งร้านไหนไม่ได้ ต้องลง disk" ไม่จริง

**ทิ้งได้:** ร้านที่ index ล่าสุด**อยู่นอกหน้าต่างแล้ว** ลบออกจาก map ได้เลย เพราะถ้าร้านนั้นกลับมาอีก โค้ดจะเข้ากิ่ง "หลุดหน้าต่างแล้ว" แล้วทำ `normalDist++` ซึ่งได้ผลเหมือนกรณี "ไม่เคยเจอ" พอดี map จึงเก็บแค่ร้านที่อยู่ในหน้าต่าง ไม่โตตามความยาว stream

**ลบยังไงให้เร็ว:** เก็บ ID ในหน้าต่างไว้ใน queue ด้วย พอหน้าต่างหด ให้ดึงตัวที่หลุดออกจากหัว queue แล้วลบจาก map

```go
type Stream struct {
	index      int
	normalDist int
	bestDist   int
	seenMap    map[string]int
	window     []string // ID ในหน้าต่างตามลำดับ ใช้รู้ว่าใครหลุดออกไปแล้ว
}

func NewStream() *Stream { return &Stream{seenMap: make(map[string]int)} }

// Add รับออเดอร์ใหม่ 1 รายการ แล้วคืนช่วงยาวสุดเท่าที่เห็นมา
func (s *Stream) Add(shopData string) int {
	if lastSeenIndex, found := s.seenMap[shopData]; found {
		s.normalDist = min(s.normalDist+1, s.index-lastSeenIndex)
	} else {
		s.normalDist++
	}
	s.bestDist = max(s.bestDist, s.normalDist)
	s.seenMap[shopData] = s.index
	s.window = append(s.window, shopData)

	left := s.index - s.normalDist + 1 // index แรกของหน้าต่าง
	for len(s.window) > s.normalDist {
		old := s.window[0]
		s.window = s.window[1:]
		if s.seenMap[old] < left { // ตัวล่าสุดของร้านนี้หลุดหน้าต่างแล้ว ลบทิ้งได้
			delete(s.seenMap, old)
		}
	}
	s.index++
	return s.bestDist
}
```

- ทดสอบแล้ว: ทุกจังหวะของ stream ได้ค่าตรงกับ brute force และ `len(seenMap)` ไม่เคยเกินความยาวหน้าต่าง
- แต่ละ ID ถูกใส่และถูกลบอย่างละครั้ง = O(1) ต่อออเดอร์โดยเฉลี่ย, space O(ความยาวหน้าต่าง)
- **มุมงานจริง:** จำนวนร้านในระบบมีจำกัด (หลักแสน) map มีเพดานอยู่แล้ว ไม่ต้องลง disk สิ่งที่โตไม่สิ้นสุดคือ `index` ซึ่ง int64 รับได้สบาย

**ทำไมต้องมี `window` ใช้โค้ดเดิมตรงๆ ไม่ได้หรอ:** โค้ดเดิมที่ไม่ลบอะไรเลย**ตอบถูกเหมือนกัน** `window` มีไว้แก้เรื่อง memory อย่างเดียว ถ้าจะลบร้านที่หลุดหางหนอนต้องรู้ว่า "ร้านอะไร" หลุด ในข้อปกติอ่าน `ids[left-1]` ได้ แต่ stream ไม่มี `ids` ให้ย้อนดู `index` กับ `normalDist` บอกได้แค่ตำแหน่ง ไม่บอกชื่อร้าน `window` คือการเก็บตัวหนอนไว้เองแทน

วัด (ร้าน 1 ล้านร้าน สุ่มออเดอร์ 2 ล้านครั้ง ทุกแบบตอบ 3849 เท่ากัน):

| แบบ | ขนาด `seenMap` |
|---|---|
| โค้ดเดิม ไม่ลบอะไร | 864,164 และโตไปเรื่อยๆ |
| มี `window` queue | สูงสุด 3,849 |
| กวาดเป็นครั้งคราว (ด้านล่าง) | สูงสุด 5,036 |

#### ทางเลือก: ไม่ต้องมี queue แต่กวาด map เป็นครั้งคราว

ใกล้โค้ดเดิมกว่า เติมแค่ `if` เดียว: พอ map ใหญ่เกิน 2 เท่าของหน้าต่าง ให้ไล่ทั้ง map แล้วลบร้านที่ index ล่าสุดอยู่นอกหน้าต่าง

```go
type Stream struct {
	index      int
	normalDist int
	bestDist   int
	seenMap    map[string]int
}

func (s *Stream) Add(shopData string) int {
	if lastSeenIndex, found := s.seenMap[shopData]; found {
		s.normalDist = min(s.normalDist+1, s.index-lastSeenIndex)
	} else {
		s.normalDist++
	}
	s.bestDist = max(s.bestDist, s.normalDist)
	s.seenMap[shopData] = s.index

	if len(s.seenMap) > 2*s.normalDist { // ขยะสะสมถึงครึ่ง map แล้ว → กวาดทีนึง
		left := s.index - s.normalDist + 1
		for shop, lastSeenIndex := range s.seenMap {
			if lastSeenIndex < left {
				delete(s.seenMap, shop)
			}
		}
	}
	s.index++
	return s.bestDist
}
```

**ทำไม 2 เท่า ไม่ใช่ `> normalDist` เฉยๆ:** หลังกวาดเสร็จ map เหลือแค่ร้านในหน้าต่าง = `normalDist` พอดี ถ้าเกณฑ์คือ `> normalDist` แปลว่า**ขยะ 1 ร้านก็กวาดแล้ว** ไล่ทั้ง map W ช่องเพื่อลบ 1 ร้าน ถ้ามีร้านหลุดหางบ่อยๆ รวมเป็น O(n × W) ตอบถูกแต่ช้า

วัด 2 แสนออเดอร์ (ตำแหน่งคู่ = ร้านใหม่ไม่กลับมาอีก, ตำแหน่งคี่ = ร้านประจำ 1,000 ร้านวนซ้ำ → หน้าต่างค้างที่ ~2,000 และมีร้านหลุดหางทุก 2 ก้าว):

| เกณฑ์ | กวาดกี่ครั้ง | ไล่ดูต่อออเดอร์ | map ใหญ่สุด | เวลา |
|---|---|---|---|---|
| `> 1 × normalDist` | 99,000 | 990 ช่อง | 2,001 | **2.99 s** |
| `> 2 × normalDist` | 49 | 1 ช่อง | 4,001 | **20 ms** |

เกณฑ์ 2 เท่า: กว่าจะกวาดต้องมีขยะประมาณครึ่ง map → กวาด 1 ครั้งไล่ 2W ช่อง ลบได้ ≥ W ช่อง ค่ากวาดหารเฉลี่ยกับร้านที่ถูกลบ (แต่ละร้านลบได้ครั้งเดียว) = **O(1) amortized** หลักเดียวกับ `append` ที่ขยาย cap เป็นเท่าตัวไม่ใช่ +1 เลข 2 ไม่ศักดิ์สิทธิ์ ใช้ 1.5 หรือ 3 ก็ได้ ขอให้ > 1 แบบคูณ (ต่ำ = ประหยัด memory แต่กวาดบ่อย, สูง = กลับกัน)

| | `window` queue | กวาดเป็นครั้งคราว |
|---|---|---|
| time | O(1) เฉลี่ยทุกครั้ง สม่ำเสมอ | O(1) amortized แต่บางครั้งกระตุกตอนกวาดทั้ง map |
| memory | map + queue อย่างละ ~W | map อย่างเดียว ≤ ~2W |
| โค้ด | เพิ่ม field + loop | เพิ่ม `if` เดียว |

ทดสอบแล้ว: ทั้ง 3 แบบ (ไม่ลบ, queue, กวาด 1 เท่า/2 เท่า) ตอบตรงกันทุกจังหวะบน stream สุ่ม 3,000 ชุด

### สรุปสิ่งที่ต้องฝึกเพิ่มจากข้อนี้
1. **ไล่มือด้วยเคสที่ทุกกิ่งของ `if` ได้ทำงาน** ก่อนเชื่อว่าเทสผ่านแล้วถูก
2. เวลาโจทย์ให้ข้อจำกัดเพิ่ม (a–z) ให้ถามตัวเองว่า **Big-O เปลี่ยนไหม** ไม่ใช่แค่ "จูน" โค้ดเดิม
3. "ทิ้งไม่ได้" ให้พิสูจน์ก่อนพูด: ข้อมูลที่ไม่มีผลกับคำตอบอีกแล้วทิ้งได้เสมอ
