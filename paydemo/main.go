// paydemo — ระบบรับชำระเงินด้วยบัตรและ QR PromptPay แบบเสมือนจริง พร้อม webhook
//
// มี 2 ฝั่งรันใน process เดียว:
//
//	PSP ปลอม (payment gateway)  :9090  — psp.go
//	ร้านของเรา (ขายประกัน)       :8090  — shop.go  รับ webhook ที่ /webhooks/psp
//
// รัน:            go run ./paydemo            (เล่น 8 สถานการณ์ให้ดูแล้วจบ)
// เปิดค้างไว้ลองเอง: go run ./paydemo -serve     (ดูตัวอย่าง curl ใน README)
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var webhookSecret = []byte("whsec_demo_only") // ของจริงอ่านจาก secret store ห้าม hardcode

func main() {
	serve := flag.Bool("serve", false, "เปิด server ค้างไว้ ไม่เล่นสถานการณ์")
	flag.Parse()

	p := newPSP("http://localhost:8090/webhooks/psp", webhookSecret)
	s := newShop("http://localhost:9090", webhookSecret)
	go func() { log.Fatal(http.ListenAndServe(":9090", p.routes())) }()
	go func() { log.Fatal(http.ListenAndServe(":8090", s.routes())) }()
	go s.worker()
	go s.reconcile()
	time.Sleep(200 * time.Millisecond)

	if *serve {
		logf("MAIN", "PSP :9090, shop :8090 พร้อมแล้ว")
		select {}
	}
	runScenarios()
}

func runScenarios() {
	const price = 1_590_000 // 15,900.00 บาท เก็บเป็นสตางค์

	scenario("1. บัตร: จ่ายผ่าน ออกกรมธรรม์ได้ → ตัดเงินจริง (capture)", func() {
		tok := cardToken("4242424242424242")
		id := newOrder("Alpha", price, false)
		pay(id, "card", tok, "")
		waitFinal(id)
	})
	scenario("2. บัตร: บริษัทประกันปฏิเสธ → ยกเลิกการกันวงเงิน ลูกค้าไม่เคยโดนตัด", func() {
		tok := cardToken("4242424242424242")
		id := newOrder("Bravo", price, true)
		pay(id, "card", tok, "")
		waitFinal(id)
	})
	scenario("3. บัตร: ธนาคารปฏิเสธบัตร → PAYMENT_FAILED", func() {
		tok := cardToken(declinedCard)
		id := newOrder("Charlie", price, false)
		pay(id, "card", tok, "")
		waitFinal(id)
	})
	scenario("4. QR: ลูกค้าสแกนจ่าย PSP ส่ง webhook ซ้ำ 2 รอบ → ประมวลผลครั้งเดียว", func() {
		id := newOrder("Delta", price, false)
		c := pay(id, "promptpay", "", "duplicate")
		fmt.Printf("        หน้าเว็บแสดง QR: %s…\n", c.QRPayload[:20])
		time.Sleep(300 * time.Millisecond) // ลูกค้าหยิบมือถือ เปิดแอปธนาคาร
		post("http://localhost:9090/simulate/scan/"+c.ID, nil, nil)
		waitFinal(id)
	})
	scenario("5. QR: เงินออกแล้วแต่บริษัทประกันปฏิเสธ → ต้องคืนเงิน (refund)", func() {
		id := newOrder("Echo", price, true)
		c := pay(id, "promptpay", "", "")
		post("http://localhost:9090/simulate/scan/"+c.ID, nil, nil)
		waitFinal(id)
	})
	scenario("6. ลูกค้ากดปุ่มจ่ายรัว 2 ครั้งพร้อมกัน → ได้ charge เดียว", func() {
		tok := cardToken("4242424242424242")
		id := newOrder("Foxtrot", price, false)
		var wg sync.WaitGroup
		ids := make([]string, 2)
		for i := range ids {
			wg.Add(1)
			go func() { defer wg.Done(); ids[i] = pay(id, "card", tok, "").ID }()
		}
		wg.Wait()
		fmt.Printf("        กดครั้งที่ 1 ได้ %s / ครั้งที่ 2 ได้ %s\n", ids[0], ids[1])
		waitFinal(id)
	})
	scenario("7. webhook หายระหว่างทาง → reconciler ไปถาม PSP เองแล้วเดินต่อได้", func() {
		tok := cardToken("4242424242424242")
		id := newOrder("Golf", price, false)
		pay(id, "card", tok, "drop")
		waitFinal(id)
	})
	scenario("8. QR แสดงอยู่แต่ยังไม่มีใครจ่าย แล้วมีคนยิง webhook ปลอม (ลายเซ็นผิด) อ้างว่าจ่ายแล้ว", func() {
		id := newOrder("Hotel", price, false)
		pay(id, "promptpay", "", "") // order รอเงินอยู่จริง ถ้าไม่ตรวจลายเซ็น webhook ปลอมจะทำให้ได้กรมธรรม์ฟรี
		body, _ := json.Marshal(event{ID: "evt_fake", Type: "charge.successful",
			Data: charge{OrderID: id, Status: "successful"}})
		req, _ := http.NewRequest("POST", "http://localhost:8090/webhooks/psp", bytes.NewReader(body))
		req.Header.Set("X-PSP-Timestamp", fmt.Sprint(time.Now().Unix()))
		req.Header.Set("X-PSP-Signature", "deadbeef")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		fmt.Printf("        server ตอบ %d\n", resp.StatusCode)
		var o order
		get("http://localhost:8090/orders/"+id, &o)
		fmt.Printf("        ผล: %s ยังเป็น %s\n", id, o.State)
	})
}

// ---------- helper ฝั่ง "browser" ----------

func cardToken(number string) string {
	var out struct{ Token string }
	post("http://localhost:9090/tokens", map[string]string{"number": number}, &out) // browser → PSP ตรง
	return out.Token
}

func newOrder(insurer string, amount int64, reject bool) string {
	var o order
	post("http://localhost:8090/orders", map[string]any{"insurer": insurer, "amount": amount, "reject_by_insurer": reject}, &o)
	return o.ID
}

func pay(orderID, method, token, chaos string) charge {
	var c charge
	post("http://localhost:8090/orders/"+orderID+"/pay", map[string]string{"method": method, "token": token, "chaos": chaos}, &c)
	return c
}

func waitFinal(id string) {
	deadline := time.Now().Add(6 * time.Second)
	var o order
	for time.Now().Before(deadline) {
		get("http://localhost:8090/orders/"+id, &o)
		if o.State == "ISSUED" || o.State == "CANCELLED" || o.State == "PAYMENT_FAILED" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond) // รอ log ที่ตามมา (เช่น webhook ซ้ำ) ให้ขึ้นครบ
	fmt.Printf("        ผล: %s = %s %s\n", o.ID, o.State, o.PolicyNo)
}

// ---------- util ----------

var t0 atomic.Int64

func scenario(title string, fn func()) {
	fmt.Printf("\n=== %s ===\n", title)
	t0.Store(time.Now().UnixNano())
	fn()
}

func logf(tag, format string, args ...any) {
	ms := (time.Now().UnixNano() - t0.Load()) / int64(time.Millisecond)
	fmt.Printf("%6dms [%-7s] %s\n", ms, tag, fmt.Sprintf(format, args...))
}

func baht(satang int64) string { return fmt.Sprintf("%d.%02d บาท", satang/100, satang%100) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func post(url string, in, out any) {
	b, _ := json.Marshal(in)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
}

func get(url string, out any) {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(out)
}
