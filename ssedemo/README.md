# ssedemo — เทียบราคาประกันรถแบบ SSE

โค้ดย่อส่วนของแบบ C จากการซ้อม system design โจทย์ 1: ผู้ใช้ขอราคา 1 ครั้ง server ถามบริษัทประกัน 20 เจ้าพร้อมกัน แล้วส่งราคากลับ**ทีละเจ้าทันทีที่ได้** ผ่าน connection เดียวที่เปิดค้างไว้

บริษัทประกันเป็นของปลอม (sleep ตามเวลาที่กำหนดแล้วคืนราคา) และ cache เป็น map ใน memory

## รัน

```bash
go run ./ssedemo
```

แล้วเปิด http://localhost:8080 หรือดู stream ดิบด้วย

```bash
curl -N "localhost:8080/quotes?car=yaris-2020"
```

(`-N` = ไม่ buffer ให้เห็นแต่ละ event ตอนที่มันมาถึงจริง)

## ลองดู 3 อย่าง

| ลอง | จะเห็น |
|---|---|
| ขอราคารถคันใหม่ | ราคาทยอยมาที่ 301, 451, 601 … 4001 ms รายการด้านซ้ายเรียงใหม่ทุกครั้ง ครบ 5 s ปิดรายการที่ 18/20 และบอกว่าอีก 2 เจ้า (8 s, 15 s) กำลังดึงต่อเบื้องหลัง |
| รอ 15 s แล้วกดซ้ำด้วยรถคันเดิม | ครบ 20 เจ้าจาก `cache` ภายใน 0 ms เพราะ Sierra กับ Tango ทำต่อจนเสร็จแล้วเขียนลง cache |
| `curl -N --max-time 1 ...` (ปิดกลางทาง) | log ขึ้น `client gone` แต่การเรียกประกันไม่ถูก cancel ยังเขียน cache ต่อจนครบ ขอซ้ำทีหลังได้ 20/20 จาก cache |

ผลที่วัดได้จาก curl:

```
รอบ 1 (รถคันใหม่ wrv-2025)
event: quote  data: {"insurer":"Alpha","price":17772,"source":"live","ms":301}
event: quote  data: {"insurer":"Bravo","price":12340,"source":"live","ms":451}
...
event: quote  data: {"insurer":"Romeo","price":13614,"source":"live","ms":4000}
event: done   data: {"got":18,"total":20,"ms":5001,"pending":2}

server log ระหว่างนั้น
cached wrv-2025|Sierra after 8.001s
cached wrv-2025|Tango after 15.001s

รอบ 2 (หลัง 16 s)
event: quote  data: {"insurer":"Tango","price":14869,"source":"cache","ms":0}
event: quote  data: {"insurer":"Sierra","price":16748,"source":"cache","ms":0}
...
event: done   data: {"got":20,"total":20,"ms":0}
```

## ทำไมมี timeout สองตัว

| | ตอบคำถาม | ค่า |
|---|---|---|
| `userDeadline` | ผู้ใช้รอได้นานแค่ไหนก่อนปิดรายการ (UX) | 5 s |
| `insurerTimeout` | การเรียกประกันควรทำได้นานแค่ไหนก่อนยอมแพ้ (cache, token) | 20 s |

ถ้าใช้ตัวเดียว 5 s เจ้าที่ตอบ 8 s กับ 15 s จะถูกตัดทุกครั้ง เสีย token ไปแล้วแต่ไม่เคยได้ราคาลง cache

ctx ของการเรียกประกันจึงสร้างจาก `context.WithoutCancel(r.Context())` ไม่ตายตาม connection ของผู้ใช้ (ปิดแท็บแล้วก็ยังทำต่อ เพราะ token กับโควตาของเขาถูกใช้ไปแล้ว cancel ไม่ได้อะไรคืน) แต่มีเพดาน `insurerTimeout` ของตัวเอง และห้าม `defer cancel()` ใน handler เพราะ handler return ตอนถึง `userDeadline` จะตัดงานเบื้องหลังทิ้ง จึงใช้ `go func() { wg.Wait(); cancel() }()` แทน

## SSE คืออะไรในระดับ HTTP

เป็น HTTP response ธรรมดาที่ไม่จบ ต่างกันแค่ 3 จุด

1. header `Content-Type: text/event-stream`
2. body เขียนเป็นก้อน ๆ รูปแบบ `event: ชื่อ\ndata: ข้อมูล\n\n` (บรรทัดว่างคือจบ 1 event)
3. เขียนแต่ละก้อนแล้ว **Flush ทันที** ไม่รอให้ buffer เต็ม

ฝั่ง browser ใช้ `EventSource` ซึ่ง reconnect ให้เองเมื่อ connection หลุด จึงต้อง `close()` เองตอนได้ `done` ไม่งั้นจะขอราคาใหม่วนไปเรื่อย ๆ

## โค้ดตรงไหนตรงกับแบบระบบ

| ในแบบระบบ | ใน `main.go` |
|---|---|
| hit/miss แยกต่อเจ้า | key `car + "|" + ins.name` ใน goroutine ของแต่ละเจ้า |
| cache-aside + เขียนกลับ cache | อ่าน `cache[key]` ก่อน, miss แล้วเรียกเสร็จค่อย `cache[key] = price` |
| timeout ของการเรียกประกัน (เพดาน W) แยกจาก connection ผู้ใช้ | `context.WithTimeout(context.WithoutCancel(r.Context()), insurerTimeout)` |
| เวลาที่ผู้ใช้รอ | `select` ระหว่าง `results` กับ `time.After(userDeadline)` |
| fan-out 20 ทาง | `for _, ins := range insurers { go func() {...} }` |
| ไม่ให้ goroutine ค้างถ้าผู้ใช้หนีไป | `make(chan result, len(insurers))` buffer เท่าจำนวนเจ้า ส่งได้เสมอ |
| ส่งทันทีที่ได้ | `fmt.Fprintf(w, "event: ...")` แล้ว `rc.Flush()` |
| เรียงตามราคา | ฝั่ง browser `quotes.sort(...)` ทุกครั้งที่มีราคาใหม่ |

## สิ่งที่ตั้งใจไม่ใส่ (ของจริงต้องมี)

- **Redis** แทน map: แชร์ข้าม pod และมี TTL 24 ชม.
- **token bucket ต่อเจ้า** ใน Redis: กันยิงเกิน limit ของบริษัทประกัน
- **`quote_id` + เก็บผลใน Redis**: ให้ reconnect ไปตก pod อื่นแล้วยังได้ผลเดิม
- **singleflight**: ระหว่าง 15 s ที่ Tango ยังไม่ตอบ ถ้ามีคนขอรถคันเดียวกันอีก ตอนนี้จะยิงซ้ำ ควรรวมให้เรียกจริงครั้งเดียว
- **circuit breaker**: เจ้าที่ล่มจริง (timeout ติดกันแม้ให้ 20 s) ควรหยุดเรียกชั่วคราว ไม่เสีย token ฟรี
- **gateway/LB**: ต้องตั้ง idle timeout ให้นานกว่า 5 s และปิด response buffering (เช่น nginx ใช้ header `X-Accel-Buffering: no`)
