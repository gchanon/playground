# paydemo — ระบบรับชำระเงินด้วยบัตรและ QR พร้อม webhook

เดโมนี้จำลองระบบรับชำระเงินให้เหมือนของจริงมากที่สุดเท่าที่รันบนเครื่องเดียวได้ ใช้ประกอบโจทย์ system design ข้อ 2 (ซื้อประกันและชำระเงิน)

| ฝั่ง | ไฟล์ | port | ในโลกจริงคือ |
|---|---|---|---|
| PSP ปลอม (payment gateway) | `psp.go` | 9090 | บริษัทอื่น เช่น Omise, 2C2P, Stripe |
| ร้านของเรา | `shop.go` | 8090 | service ของเรา รับ webhook ที่ `/webhooks/psp` |
| "browser" และสถานการณ์ทดสอบ | `main.go` | - | หน้าเว็บกับลูกค้า |

## รัน

เล่น 8 สถานการณ์ต่อกันแล้วจบ:

```bash
go run ./paydemo
```

เปิด server ค้างไว้เพื่อยิง curl เอง (ตัวอย่างอยู่ท้ายไฟล์):

```bash
go run ./paydemo -serve
```

## เส้นทางของเงิน

**บัตรเครดิต:** กันวงเงินก่อน แล้วค่อยตัดจริงหลังออกกรมธรรม์ได้

```
browser ──เลขบัตร──▶ PSP                     ← เลขบัตรไม่ผ่าน server ร้านเลย
browser ◀──token──── PSP
browser ──token────▶ ร้าน POST /orders/{id}/pay
ร้าน ──สร้าง charge (capture=false, Idempotency-Key)──▶ PSP     → pending
PSP ──webhook charge.authorized (มีลายเซ็น)──▶ ร้าน            → PAID (กันวงเงินแล้ว ยังไม่ตัด)
ร้าน ──ออกกรมธรรม์──▶ บริษัทประกัน
   ได้ → ร้าน ──capture──▶ PSP (ตัดเงินจริง)  → ISSUED
   ไม่ได้ → ร้าน ──reverse──▶ PSP (ปล่อยวงเงิน) → CANCELLED  ลูกค้าไม่เคยโดนตัด
```

**QR PromptPay:** แยกกันวงเงินกับตัดจริงไม่ได้ สแกนเมื่อไหร่เงินออกทันที

```
ร้าน ──สร้าง charge (promptpay)──▶ PSP → ได้ QR → หน้าเว็บแสดง QR
ลูกค้าสแกนในแอปธนาคาร (กี่วินาทีหรือกี่นาทีก็ได้ ร้านไม่รู้)
PSP ──webhook charge.successful──▶ ร้าน → PAID (เงินเข้าแล้ว)
ร้าน ──ออกกรมธรรม์──▶ บริษัทประกัน
   ได้ → ISSUED
   ไม่ได้ → ร้าน ──refund──▶ PSP → CANCELLED  ต้องคืนเงินจริง
```

## 8 สถานการณ์ และผลที่วัดได้จริง

| # | สถานการณ์ | ผลลัพธ์ | สิ่งที่ทำให้ถูก |
|---|---|---|---|
| 1 | บัตร ทุกอย่างผ่าน | `ISSUED` ใน ~715 ms, capture หลังได้กรมธรรม์ | แยก authorize กับ capture |
| 2 | บัตร บริษัทประกันปฏิเสธ | `CANCELLED` ด้วย reverse ลูกค้าไม่เคยโดนตัด | ตัดเงินจริงเป็นขั้นสุดท้าย |
| 3 | บัตร ธนาคารปฏิเสธ | `PAYMENT_FAILED` | webhook `charge.failed` |
| 4 | QR PSP ส่ง webhook ซ้ำ 2 รอบ | ประมวลผลครั้งเดียว ครั้งที่สองตอบ 200 เฉย ๆ | กันซ้ำด้วย event id |
| 5 | QR บริษัทประกันปฏิเสธ | `CANCELLED` ด้วย refund | QR ตัดไปแล้ว ต้องคืนเงิน |
| 6 | กดจ่ายรัว 2 ครั้งพร้อมกัน | ได้ `chrg_15` ทั้งสองครั้ง | `Idempotency-Key: pay-{order_id}` |
| 7 | webhook หาย | reconciler เจอที่ ~1.9 s แล้วเดินต่อจน `ISSUED` | ไล่ถาม PSP เอง |
| 8 | webhook ปลอม ลายเซ็นผิด | `401` order ยังเป็น `AWAITING_PAYMENT` | ตรวจ HMAC + timestamp |

รันด้วย `go run -race` แล้วไม่เจอ data race

## สิ่งที่ระบบจ่ายเงินจริงต้องมี (และตรงไหนในโค้ด)

| ต้องมี | ทำไม | ในเดโม |
|---|---|---|
| **ไม่แตะเลขบัตรเอง** ใช้ token | ถ้า server เห็นเลขบัตร ต้องผ่านมาตรฐาน PCI DSS เต็มรูปแบบ ซึ่งยากและแพงมาก | `psp.go createToken` |
| **เงินเป็นจำนวนเต็มหน่วยย่อย** (สตางค์) | `float` บวกลบแล้วเพี้ยน เช่น 0.1+0.2 | `Amount int64` |
| **ราคาเอาจาก DB ของเรา** | ห้ามเชื่อจำนวนเงินที่ browser ส่งมา แก้ได้ง่ายมาก | `shop.pay` ใช้ `o.Amount` |
| **Idempotency-Key** ทุกครั้งที่สั่งให้เงินเคลื่อน | network หลุดแล้ว retry หรือกดซ้ำ ต้องไม่ตัดเงินสองครั้ง | `pay-{order_id}` |
| **state machine** | webhook ซ้ำ มาช้า หรือมาผิดลำดับ ต้องเปลี่ยนสถานะผิดไม่ได้ | `transitions` ใน `shop.go` |
| **ตรวจลายเซ็น webhook** (HMAC, เทียบแบบเวลาคงที่, timestamp) | URL webhook เปิดสาธารณะ ใครก็ยิงได้ | `shop.webhook` |
| **กันซ้ำด้วย event id** | PSP ส่งแบบ at-least-once | `seenEvents` |
| **ตอบ webhook ให้เร็ว** งานหนักโยนเข้า queue | ช้าเกินไป PSP จะ timeout แล้ว retry ซ้ำ | `s.jobs <- id` |
| **reconciliation** | webhook หายได้ ถ้าไม่มีตัวนี้ order ค้างตลอดไป | `shop.reconcile` |
| **ไม่เชื่อหน้า "ชำระสำเร็จ"** ที่ browser ถูก redirect กลับมา | ผู้ใช้พิมพ์ URL นั้นเองได้ ต้องเชื่อ webhook หรือการถาม PSP เท่านั้น | ร้านเปลี่ยนสถานะจาก webhook/reconcile เท่านั้น |
| **ปลายทางที่ทำซ้ำได้** (บริษัทประกันออกกรมธรรม์ซ้ำไม่ได้) | worker อาจรันซ้ำหลัง crash | `insurer.issue` ใช้ order id เป็น key |

## สิ่งที่ตั้งใจไม่ใส่ (ของจริงต้องมี)

- **Database + transaction + outbox:** เปลี่ยนสถานะกับใส่งานลง queue ต้องอยู่ในธุรกรรมเดียวกัน ไม่งั้น crash ระหว่างสองขั้นจะได้ order ที่ PAID แต่ไม่มีใครออกกรมธรรม์ ในเดโมใช้ map กับ channel ซึ่งหายหมดเมื่อ restart
- **unique index ของ event id ใน DB** แทน `map`
- **retry ของ capture/refund** และหน้าจอให้ ops: ตอนนี้ถ้า capture พัง order จะค้างที่ `ISSUING` แล้วแค่ log ไว้
- **QR หมดอายุ** (มักราว 15 นาที) และ**การกันวงเงินบัตรหมดอายุ** (หลายวัน แล้วแต่ธนาคาร) ถ้าออกกรมธรรม์ช้ากว่านั้นต้อง capture ไม่ทัน
- **3-D Secure (OTP):** ของจริง browser ต้องถูก redirect ไปหน้าธนาคารก่อน ในเดโมแทนด้วยการรอ 300 ms
- **บริษัทประกันแบบ async** (ส่งกรมธรรม์ทาง callback ทีหลัง) ต้องมีสถานะรอและ webhook อีกเส้น
- **รายงานกระทบยอดรายวัน** กับ PSP และบริษัทประกัน, chargeback/dispute, คืนเงินบางส่วน, การหมุน webhook secret, alert เมื่อมี order ค้างนานเกินกำหนด

## ลองเองด้วย curl (`go run ./paydemo -serve`)

สร้าง order:
```bash
curl -s localhost:8090/orders -d '{"insurer":"Alpha","amount":1590000}'
```

จ่ายด้วย QR (ได้ `id` ของ charge กลับมา):
```bash
curl -s localhost:8090/orders/ord_1/pay -d '{"method":"promptpay"}'
```

จำลองว่าลูกค้าสแกนจ่าย (ใช้ `id` ของ charge ที่ได้จากขั้นก่อน):
```bash
curl -s -X POST localhost:9090/simulate/scan/chrg_1
```

ดูสถานะและประวัติของ order:
```bash
curl -s localhost:8090/orders/ord_1
```
