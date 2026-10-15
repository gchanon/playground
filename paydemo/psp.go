package main

// psp.go — payment gateway ปลอม (PSP = Payment Service Provider) แบบ Omise / 2C2P / Stripe ย่อส่วน
//
// ในโลกจริงนี่คือ "บริษัทอื่น" ที่เราเรียกผ่าน HTTPS ไม่ใช่โค้ดของเรา
// แยกไว้ใน process เดียวกันแค่เพื่อให้รันเดโมได้ง่าย (ฟังที่ port 9090 คนละ port กับร้านเรา)

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// บัตรทดสอบ: ใบนี้ธนาคารจะปฏิเสธ ใบอื่นผ่านหมด
const declinedCard = "4000000000000002"

type charge struct {
	ID            string `json:"id"`
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`  // หน่วยสตางค์ (int) ห้ามใช้ float กับเงิน
	Method        string `json:"method"`  // card | promptpay
	Capture       bool   `json:"capture"` // false = กันวงเงินไว้ก่อน (authorize) ยังไม่ตัดจริง
	Status        string `json:"status"`  // pending → authorized/successful/failed → reversed/refunded
	QRPayload     string `json:"qr_payload,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	chaos         string // เฉพาะเดโม: "duplicate" ส่ง webhook ซ้ำ, "drop" ไม่ส่งเลย
	cardNumber    string
}

type event struct {
	ID   string `json:"id"`
	Type string `json:"type"` // charge.authorized | charge.successful | charge.failed
	Data charge `json:"data"`
}

type psp struct {
	mu         sync.Mutex
	seq        int
	charges    map[string]*charge
	idem       map[string]string // Idempotency-Key → charge id
	tokens     map[string]string // token → เลขบัตร (มีแค่ PSP ที่รู้เลขบัตร)
	webhookURL string
	secret     []byte
}

func newPSP(webhookURL string, secret []byte) *psp {
	return &psp{charges: map[string]*charge{}, idem: map[string]string{}, tokens: map[string]string{},
		webhookURL: webhookURL, secret: secret}
}

func (p *psp) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tokens", p.createToken)
	mux.HandleFunc("POST /charges", p.createCharge)
	mux.HandleFunc("GET /charges/{id}", p.getCharge)
	mux.HandleFunc("POST /charges/{id}/capture", p.action("authorized", "successful"))
	mux.HandleFunc("POST /charges/{id}/reverse", p.action("authorized", "reversed"))
	mux.HandleFunc("POST /charges/{id}/refund", p.action("successful", "refunded"))
	mux.HandleFunc("POST /simulate/scan/{id}", p.simulateScan)
	return mux
}

// createToken: ของจริงเรียกจาก JavaScript ของ PSP ใน browser ตรง ๆ
// เลขบัตรจึงวิ่งจาก browser ไป PSP โดยไม่ผ่าน server ของร้านเลย ร้านได้แค่ token กลับมา
func (p *psp) createToken(w http.ResponseWriter, r *http.Request) {
	var in struct{ Number string }
	json.NewDecoder(r.Body).Decode(&in)
	p.mu.Lock()
	p.seq++
	tok := fmt.Sprintf("tok_%d", p.seq)
	p.tokens[tok] = in.Number
	p.mu.Unlock()
	writeJSON(w, 201, map[string]string{"token": tok})
}

func (p *psp) createCharge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrderID              string `json:"order_id"`
		Method, Token, Chaos string
		Amount               int64
		Capture              bool
	}
	json.NewDecoder(r.Body).Decode(&in)
	key := r.Header.Get("Idempotency-Key")

	p.mu.Lock()
	// Idempotency: key เดิม = คืน charge เดิม ไม่สร้างใหม่ (กันกดจ่ายซ้ำ / retry ตอน network หลุด)
	if id, ok := p.idem[key]; ok && key != "" {
		c := *p.charges[id]
		p.mu.Unlock()
		logf("PSP", "Idempotency-Key %s ซ้ำ → คืน %s ตัวเดิม ไม่สร้าง charge ใหม่", key, c.ID)
		writeJSON(w, 200, c)
		return
	}
	p.seq++
	c := &charge{ID: fmt.Sprintf("chrg_%d", p.seq), OrderID: in.OrderID, Amount: in.Amount,
		Method: in.Method, Capture: in.Capture, Status: "pending", chaos: in.Chaos, cardNumber: p.tokens[in.Token]}
	if c.Method == "promptpay" {
		c.QRPayload = "00020101021230...5303764540" + strconv.FormatInt(c.Amount, 10) // QR ปลอม
	}
	p.charges[c.ID] = c
	if key != "" {
		p.idem[key] = c.ID
	}
	out := *c
	p.mu.Unlock()
	logf("PSP", "สร้าง %s (%s, %s) status=pending", c.ID, c.Method, baht(c.Amount))

	// บัตร: ธนาคารใช้เวลาตรวจ (3-D Secure/OTP) แล้วค่อยแจ้งผลทาง webhook
	// QR: รอจนลูกค้าเปิดแอปธนาคารสแกน (/simulate/scan) ไม่รู้ว่าจะกี่วินาทีหรือกี่นาที
	if c.Method == "card" {
		go func() {
			time.Sleep(300 * time.Millisecond)
			p.mu.Lock()
			if c.cardNumber == declinedCard {
				c.Status, c.FailureReason = "failed", "insufficient_funds"
			} else if c.Capture {
				c.Status = "successful"
			} else {
				c.Status = "authorized"
			}
			p.mu.Unlock()
			p.sendWebhook(c)
		}()
	}
	writeJSON(w, 201, out)
}

// simulateScan แทน "ลูกค้าสแกน QR ในแอปธนาคารแล้วกดยืนยัน"
func (p *psp) simulateScan(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	c, ok := p.charges[r.PathValue("id")]
	if ok && c.Status == "pending" {
		c.Status = "successful"
	}
	p.mu.Unlock()
	if !ok {
		http.Error(w, "not found", 404)
		return
	}
	logf("BANK", "ลูกค้าสแกน QR ของ %s แล้ว เงินออกจากบัญชี", c.ID)
	go p.sendWebhook(c)
	w.WriteHeader(204)
}

func (p *psp) getCharge(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.charges[r.PathValue("id")]
	if !ok {
		http.Error(w, "not found", 404)
		return
	}
	writeJSON(w, 200, *c)
}

// action เปลี่ยนสถานะ charge ได้เฉพาะจากสถานะที่ถูกต้อง เช่น capture ได้เฉพาะ charge ที่ authorized
func (p *psp) action(from, to string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		c, ok := p.charges[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", 404)
			return
		}
		if c.Status != from {
			http.Error(w, fmt.Sprintf("cannot %s→%s", c.Status, to), 409)
			return
		}
		c.Status = to
		logf("PSP", "%s: %s → %s", c.ID, from, to)
		writeJSON(w, 200, *c)
	}
}

// sendWebhook: ยิงไปบอกร้าน พร้อมลายเซ็น HMAC ถ้าร้านตอบไม่ใช่ 2xx ก็ retry แบบ backoff
// ของจริงส่งแบบ "at-least-once": ร้านต้องรับมือกับการส่งซ้ำ ส่งช้า และสลับลำดับได้เสมอ
func (p *psp) sendWebhook(c *charge) {
	p.mu.Lock()
	p.seq++
	typ := "charge." + c.Status
	ev := event{ID: fmt.Sprintf("evt_%d", p.seq), Type: typ, Data: *c}
	chaos := c.chaos
	p.mu.Unlock()

	if chaos == "drop" {
		logf("PSP", "webhook %s ของ %s หายระหว่างทาง (จำลอง)", ev.ID, c.ID)
		return
	}
	times := 1
	if chaos == "duplicate" {
		times = 2
	}
	body, _ := json.Marshal(ev)
	for i := 0; i < times; i++ {
		for attempt, wait := 1, 200*time.Millisecond; attempt <= 5; attempt, wait = attempt+1, wait*2 {
			ts := strconv.FormatInt(time.Now().Unix(), 10)
			req, _ := http.NewRequest("POST", p.webhookURL, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-PSP-Timestamp", ts)
			req.Header.Set("X-PSP-Signature", sign(p.secret, ts, body))
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode/100 == 2 {
					logf("PSP", "ส่ง webhook %s (%s) สำเร็จ ครั้งที่ %d", ev.ID, typ, i+1)
					break
				}
			}
			logf("PSP", "ส่ง webhook %s ไม่สำเร็จ retry ใน %v", ev.ID, wait)
			time.Sleep(wait)
		}
	}
}

// sign = HMAC-SHA256(secret, timestamp + "." + body) ใส่ timestamp ด้วยเพื่อกันคนเอา webhook เก่ามายิงซ้ำ (replay)
func sign(secret []byte, ts string, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(ts + "."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}
