# ex03 — Payment client: ออเดอร์ถูกมาร์กว่าจ่ายแล้วทั้งที่ไม่เคยถูกตัดเงิน

**ระดับความรุนแรง:** P0 · **คนแจ้ง:** Finance / reconciliation

## อาการที่เจอ

ระหว่างที่ gateway ล่มเมื่อวันอังคารที่แล้ว (09:12–09:31) job กระทบยอดของเรา
เจอออเดอร์ 418 ใบอยู่ในสถานะ `PAID` โดยที่ `transaction_id` ว่างเปล่า ฝั่ง
gateway ไม่มีบันทึกของรายการพวกนี้เลยสักใบ แต่ลูกค้าได้รับอีเมล "payment
successful" ไปแล้ว

ฝั่ง checkout service เรียกใช้แบบปกติทั่วไป:

```go
receipt, err := payments.Charge(ctx, req)
if err != nil {
    return markFailed(order, err)
}
return markPaid(order, receipt.TransactionID)
```

มี pod อยู่ไม่กี่ตัวที่ crash ด้วย nil pointer dereference ที่บรรทัดถัดจาก
การเช็ค error ซึ่งเป็นเหตุผลเดียวที่ทำให้เราสังเกตเห็นเรื่องนี้

เคส decline (402) ทำงานถูกต้อง — ออเดอร์พวกนั้นถูกมาร์กว่า failed
มีแต่ช่วงเวลาที่ gateway ล่มเท่านั้นที่มีปัญหา

## สิ่งที่ควรจะเป็น

เมื่อการเรียก gateway ล้มเหลวทุกครั้ง `Charge` ต้องคืน error ที่ไม่ใช่ nil
และคืน receipt เป็น nil ส่วนเคส decline ต้องไม่ถูก retry เหมือนเดิม

## วิธีทำให้เกิดซ้ำ

```bash
go test ./ex03_payment_client/
```
