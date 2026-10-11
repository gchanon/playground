# pgdemo

ดูความสามารถของ PostgreSQL ด้วยการ **วัดจริง** ไม่ใช่ท่องตำรา
เขียนคู่กับเพจ Notion ชุด [PostgreSQL](https://app.notion.com/p/3e73b15f732481f7aa09d7ffe4503584)

```bash
cd pgdemo && docker compose up -d     # เปิด server (PG 17 ที่ port 55432)
go run ./pgdemo                       # รันทุก demo
go run ./pgdemo -demo mvcc            # รันทีละตัว
docker compose down -v                # ปิดและลบข้อมูลทิ้ง
```

flag: `-n` จำนวนแถว (demo copy) · `-u` จำนวน UPDATE (demo mvcc) · `-cn` จำนวนแถว (demo commit)

## demo ทั้ง 7 ตัว

| `-demo` | เพจ Notion | ตอบคำถามว่า |
|---|---|---|
| `mvcc` | 4 MVCC กับ VACUUM | UPDATE สร้างแถวใหม่เสมอ แล้วทำไมไฟล์ส่วนใหญ่ไม่โต |
| `copy` | 6 COPY vs INSERT | ยัดข้อมูล 6 วิธี ต่างกันกี่เท่า และสร้าง WAL กี่ byte |
| `commit` | 3 Journey ของข้อมูล | "transaction แพงเพราะ fsync" จริงแค่ไหน |
| `isolation` | 4 MVCC | lost update · 40001 · write skew · retry loop |
| `lock` | 2 พื้นฐาน | ใครบล็อกใคร · อ่านไม่เคยรอ · deadlock |
| `explain` | 5 Index + Heap | Seq / Index / Bitmap / Index Only · ทำไมต้อง VACUUM ก่อน |
| `buffers` | 2, 7 | cache hit/miss · กับดัก ring buffer |

## ตั้งค่าอะไรไว้บ้าง และทำไม

ใน `compose.yml` fix ค่าไว้เพื่อให้ผลวัด **ซ้ำได้** — ไม่ใช่ค่าที่ควรใช้บน prod

| ค่า | เหตุผล |
|---|---|
| `shared_buffers=16MB` | เล็กพอให้สร้างตารางใหญ่กว่า cache ได้โดยไม่กินดิสก์ (ของจริง ~25% ของ RAM) |
| `autovacuum=off` | ไม่งั้นแม่บ้านมาเก็บกวาดกลางคัน `-demo mvcc` จะวัดไม่ได้ |
| `deadlock_timeout=1s` | ให้ `-demo lock` ไม่ต้องรอนาน |
| `max_wal_size=256MB` | กัน WAL บวมกินดิสก์ตอนรันซ้ำๆ |

## ตัวเลขที่วัดได้จริง (Apple Silicon / Docker Desktop / PG 17.11)

**A1 — UPDATE แถวเดียว 20,000 ครั้ง**

| | ไม่มีใครถือ snapshot | มีอีก txn เปิดค้าง |
|---|---|---|
| ctid | `(0,1)` → `(0,81)` | `(0,1)` → `(108,21)` |
| ขนาด heap | **8 KB** | **872 KB** |
| dead tuple | 80 | 20,000 |
| หลัง VACUUM | 8 KB | 872 KB (dead=0 แต่ไฟล์ไม่ลด) |
| หลัง VACUUM FULL | 8 KB | 8 KB |

ข้อมูลเหมือนกันเป๊ะ ต่างกัน 109 เท่า เพราะมีคนเปิด transaction ค้างไว้

**A2 — ยัด 10,000 แถว**

| วิธี | rows/s | เร็วขึ้น | WAL/แถว |
|---|---|---|---|
| INSERT ทีละแถว commit ทุกแถว | 2,529 | 1.0x | 183 B |
| INSERT ทีละแถว txn เดียว | 5,016 | 2.0x | 143 B |
| INSERT 1000 แถว/คำสั่ง | 159,690 | 63x | 143 B |
| pgx SendBatch 1000 | 135,073 | 53x | 143 B |
| **COPY** | **611,604** | **242x** | 96 B |
| COPY ลง UNLOGGED | 619,481 | 245x | 0 B |

**A3 — แยกต้นทุนของ commit (INSERT 3,000 แถว)**

| แบบ | rt/แถว | commit/แถว | sync=on | sync=off | ต่าง = fsync |
|---|---|---|---|---|---|
| autocommit INSERT | 1.0 | 1.0 | 314 µs | 200 µs | 114 µs |
| BEGIN+INSERT+COMMIT | 3.0 | 1.0 | 636 µs | 560 µs | 76 µs |
| commit ทุก 100 แถว | 1.02 | 0.01 | 171 µs | 207 µs | ~0 |
| commit ทุก 1000 แถว | 1.002 | 0.001 | 199 µs | 201 µs | ~0 |

ถอดออกมา: round-trip 178 µs (57%) + fsync 114 µs (36%) ≈ เวลาทั้งหมด 314 µs
→ **"transaction แพงเพราะ fsync" จริงแค่ครึ่งเดียว** อีกครึ่งคือแค่คุยกับ server

**B4 — ring buffer**

`buf_big` 40 MB บน `shared_buffers` 16 MB — สแกนกี่รอบก็ได้ **hit 2011 / read 3162 เท่าเดิมทุกรอบ**
pg_prewarm ช่วยไม่ได้ เพราะหน้าที่ต้องอ่านเข้ามาใหม่จะถูกยัดลงวงแหวน 32 หน้าแล้วทิ้ง

## ข้อจำกัดที่ต้องรู้ก่อนเอาตัวเลขไปอ้าง

1. **fsync บน Docker Desktop (macOS) ถูกกว่าของจริง** — ลงดิสก์เสมือนใน VM
   วัดดิบด้วย `docker compose exec pg pg_test_fsync` เครื่องนี้ได้ fdatasync ≈ 113 µs/op
   บน NVMe จริง สัดส่วน fsync จะสูงกว่าที่ตารางนี้แสดง
2. **round-trip ผ่าน Docker bridge แพงกว่า unix socket** — วัดได้ ~180-205 µs
   ถ้าต่อผ่าน unix socket บนเครื่องเดียวกันจะเหลือหลักสิบ µs
3. **`-demo buffers` วัด "ราคาของ I/O" ไม่ได้** — cache miss ที่เห็นยังถูกรับโดย
   OS page cache ของ Linux ใน VM ไม่ได้ลงดิสก์จริง สิ่งที่ demo นี้พิสูจน์ได้คือ
   *พฤติกรรม* ของ cache (ring buffer) ไม่ใช่ *ราคา* ของการพลาด cache
4. ค่าทั้งหมดวัดบนเครื่องเดียว ไม่ได้รันหลายรอบหาค่าเฉลี่ย ยกเว้น `-demo commit`
   ที่วัด 3 รอบเอาค่าต่ำสุด
