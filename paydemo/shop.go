package main

// shop.go — ระบบของเรา (ฝั่งขายประกัน): order, เริ่มจ่ายเงิน, รับ webhook, ออกกรมธรรม์, กระทบยอด
//
// เก็บข้อมูลใน map เพื่อให้เดโมรันได้ทันที ของจริงต้องเป็น database ที่มี transaction
// (และ job ที่ต้องทำต่อควรเขียนลงตารางในธุรกรรมเดียวกับการเปลี่ยนสถานะ = outbox pattern)

import (
	"bytes"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type order struct {
	ID       string   `json:"id"`
	Insurer  string   `json:"insurer"`
	Amount   int64    `json:"amount"` // สตางค์
	Method   string   `json:"method,omitempty"`
	ChargeID string   `json:"charge_id,omitempty"`
	State    string   `json:"state"`
	PolicyNo string   `json:"policy_no,omitempty"`
	History  []string `json:"history"`

	rejectByInsurer bool // เฉพาะเดโม: ให้บริษัทประกันปฏิเสธ order นี้
	updatedAt       time.Time
}

// state machine ของ order: เปลี่ยนได้เฉพาะตามลูกศรนี้เท่านั้น
// webhook ที่ส่งซ้ำ มาช้า หรือมาผิดลำดับ จะเปลี่ยนสถานะไม่ได้เอง เพราะไม่มีลูกศรรองรับ
//
//	CREATED → AWAITING_PAYMENT → PAID → ISSUING → ISSUED
//	                    ↓                   ↓
//	             PAYMENT_FAILED        CANCELLING → CANCELLED (คืนเงิน/ยกเลิกการกันวงเงินแล้ว)
var transitions = map[string][]string{
	"CREATED":          {"AWAITING_PAYMENT"},
	"AWAITING_PAYMENT": {"PAID", "PAYMENT_FAILED"},
	"PAID":             {"ISSUING"},
	"ISSUING":          {"ISSUED", "CANCELLING"},
	"CANCELLING":       {"CANCELLED"},
}

type shop struct {
	mu         sync.Mutex
	seq        int
	orders     map[string]*order
	seenEvents map[string]bool // event id ที่ประมวลผลแล้ว (ของจริงเก็บใน DB พร้อม unique index)
	jobs       chan string     // order ที่จ่ายแล้ว รอออกกรมธรรม์ (ของจริงคือ queue/outbox)
	pspURL     string
	secret     []byte
	insurer    *insurer
}

var pspClient = &http.Client{Timeout: 5 * time.Second}

func newShop(pspURL string, secret []byte) *shop {
	return &shop{orders: map[string]*order{}, seenEvents: map[string]bool{}, jobs: make(chan string, 100),
		pspURL: pspURL, secret: secret, insurer: &insurer{issued: map[string]string{}}}
}

func (s *shop) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", s.createOrder)
	mux.HandleFunc("GET /orders/{id}", s.getOrder)
	mux.HandleFunc("POST /orders/{id}/pay", s.pay)
	mux.HandleFunc("POST /webhooks/psp", s.webhook)
	return mux
}

// transition ต้องเรียกตอนถือ s.mu อยู่ คืน false ถ้าไม่มีลูกศรจากสถานะปัจจุบันไป to
func (s *shop) transition(o *order, to, why string) bool {
	for _, next := range transitions[o.State] {
		if next == to {
			logf("ORDER", "%s: %s → %s (%s)", o.ID, o.State, to, why)
			o.History = append(o.History, fmt.Sprintf("%s → %s (%s)", o.State, to, why))
			o.State, o.updatedAt = to, time.Now()
			return true
		}
	}
	logf("ORDER", "%s: ข้าม %s → %s ไม่ได้ (%s) ถือว่าทำไปแล้ว", o.ID, o.State, to, why)
	return false
}

func (s *shop) createOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Insurer         string
		Amount          int64
		RejectByInsurer bool `json:"reject_by_insurer"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	s.seq++
	o := &order{ID: fmt.Sprintf("ord_%d", s.seq), Insurer: in.Insurer, Amount: in.Amount, State: "CREATED",
		History: []string{}, rejectByInsurer: in.RejectByInsurer, updatedAt: time.Now()}
	s.orders[o.ID] = o
	out := *o
	s.mu.Unlock()
	logf("ORDER", "สร้าง %s ประกัน %s ราคา %s", o.ID, o.Insurer, baht(o.Amount))
	writeJSON(w, 201, out)
}

func (s *shop) getOrder(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[r.PathValue("id")]
	if !ok {
		http.Error(w, "not found", 404)
		return
	}
	writeJSON(w, 200, *o)
}

// pay: ร้านไม่เคยเห็นเลขบัตร ได้แค่ token จาก PSP แล้วสั่ง "สร้าง charge" ด้วย Idempotency-Key ที่ผูกกับ order
// ราคาเอาจาก order ใน DB ของเรา ห้ามเชื่อจำนวนเงินที่ browser ส่งมา
func (s *shop) pay(w http.ResponseWriter, r *http.Request) {
	var in struct{ Method, Token, Chaos string }
	json.NewDecoder(r.Body).Decode(&in)

	s.mu.Lock()
	o, ok := s.orders[r.PathValue("id")]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "not found", 404)
		return
	}
	if o.State != "CREATED" && o.State != "AWAITING_PAYMENT" {
		s.mu.Unlock()
		http.Error(w, "order state "+o.State, 409)
		return
	}
	id, amount := o.ID, o.Amount
	s.mu.Unlock() // ห้ามถือ lock ระหว่างเรียก network

	var c charge
	err := s.callPSP("POST", "/charges", "pay-"+id, map[string]any{
		"order_id": id, "amount": amount, "method": in.Method, "token": in.Token, "chaos": in.Chaos,
		"capture": in.Method != "card", // บัตร: กันวงเงินก่อน ตัดจริงหลังได้กรมธรรม์ / QR: ตัดทันที (แยกไม่ได้)
	}, &c)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}

	s.mu.Lock()
	if o.State == "CREATED" {
		o.Method, o.ChargeID = in.Method, c.ID
		s.transition(o, "AWAITING_PAYMENT", "สร้าง "+c.ID)
	}
	s.mu.Unlock()
	writeJSON(w, 200, c) // QR ส่งกลับไปให้หน้าเว็บแสดง
}

// webhook: ตรวจลายเซ็น → กันซ้ำ → อัปเดตสถานะ → ตอบ 200 ให้เร็ว (งานหนักโยนเข้า queue)
func (s *shop) webhook(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	ts := r.Header.Get("X-PSP-Timestamp")

	// 1) ลายเซ็นต้องตรง ใครก็ยิง URL นี้ได้ ถ้าไม่ตรวจ คนนอกยิง "จ่ายแล้ว" มาเองได้ฟรี
	//    ใช้ hmac.Equal (เทียบแบบเวลาคงที่) ไม่ใช้ == เพื่อกันเดาลายเซ็นจากเวลาที่ใช้เทียบ
	if !hmac.Equal([]byte(r.Header.Get("X-PSP-Signature")), []byte(sign(s.secret, ts, body))) {
		logf("WEBHOOK", "ลายเซ็นไม่ตรง → 401 ไม่ประมวลผล")
		http.Error(w, "bad signature", 401)
		return
	}
	// 2) timestamp ต้องไม่เก่าเกิน 5 นาที กันเอา webhook จริงที่ดักได้มายิงซ้ำ (replay)
	if t, err := strconv.ParseInt(ts, 10, 64); err != nil || time.Since(time.Unix(t, 0)).Abs() > 5*time.Minute {
		http.Error(w, "stale", 401)
		return
	}
	var ev event
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "bad body", 400)
		return
	}

	// 3) กันซ้ำด้วย event id: PSP ส่งแบบ at-least-once ได้ event เดิมสองรอบเป็นเรื่องปกติ
	s.mu.Lock()
	if s.seenEvents[ev.ID] {
		s.mu.Unlock()
		logf("WEBHOOK", "%s เคยประมวลผลแล้ว → ตอบ 200 แต่ไม่ทำซ้ำ", ev.ID)
		w.WriteHeader(200) // ต้องตอบ 2xx ไม่งั้น PSP จะ retry ไม่เลิก
		return
	}
	s.seenEvents[ev.ID] = true
	s.mu.Unlock()

	logf("WEBHOOK", "รับ %s %s ของ %s", ev.ID, ev.Type, ev.Data.OrderID)
	s.applyCharge(ev.Data, "webhook")
	w.WriteHeader(200)
}

// applyCharge ใช้ร่วมกันทั้ง webhook และ reconciler ใครมาก่อนก็เปลี่ยนสถานะ อีกคนจะถูก state machine กันไว้
func (s *shop) applyCharge(c charge, src string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[c.OrderID]
	if !ok {
		return
	}
	switch c.Status {
	case "authorized", "successful":
		if s.transition(o, "PAID", src+": "+c.Status) {
			s.jobs <- o.ID
		}
	case "failed":
		s.transition(o, "PAYMENT_FAILED", src+": "+c.FailureReason)
	}
}

// worker: ออกกรมธรรม์ แล้วค่อยตัดเงินจริง หรือถ้าออกไม่ได้ก็คืนเงิน/ยกเลิกการกันวงเงิน
func (s *shop) worker() {
	for id := range s.jobs {
		s.fulfil(id)
	}
}

func (s *shop) fulfil(id string) {
	s.mu.Lock()
	o := s.orders[id]
	if !s.transition(o, "ISSUING", "ส่งคำขอออกกรมธรรม์") {
		s.mu.Unlock()
		return
	}
	method, chargeID, reject := o.Method, o.ChargeID, o.rejectByInsurer
	s.mu.Unlock()

	policy, err := s.insurer.issue(id, reject)
	if err == nil {
		if method == "card" { // ได้กรมธรรม์แล้วค่อยตัดเงินจริง
			if err := s.callPSP("POST", "/charges/"+chargeID+"/capture", "capture-"+id, nil, nil); err != nil {
				logf("WORKER", "%s capture ไม่สำเร็จ: %v → ค้างไว้ที่ ISSUING ให้ retry หรือ ops ตรวจ", id, err)
				return
			}
		}
		s.mu.Lock()
		o.PolicyNo = policy
		s.transition(o, "ISSUED", "ได้กรมธรรม์ "+policy)
		s.mu.Unlock()
		logf("EMAIL", "ส่งกรมธรรม์ %s ให้ลูกค้าของ %s", policy, id)
		return
	}

	s.mu.Lock()
	s.transition(o, "CANCELLING", "บริษัทประกันปฏิเสธ: "+err.Error())
	s.mu.Unlock()
	path, what := "/refund", "คืนเงิน"
	if method == "card" {
		path, what = "/reverse", "ยกเลิกการกันวงเงิน (ไม่เคยตัดจริง)"
	}
	if err := s.callPSP("POST", "/charges/"+chargeID+path, path[1:]+"-"+id, nil, nil); err != nil {
		logf("WORKER", "%s %s ไม่สำเร็จ: %v → ค้างไว้ที่ CANCELLING ให้ retry หรือ ops ตรวจ", id, what, err)
		return
	}
	s.mu.Lock()
	s.transition(o, "CANCELLED", what)
	s.mu.Unlock()
	logf("EMAIL", "แจ้งลูกค้าของ %s: ออกกรมธรรม์ไม่ได้ %s แล้ว", id, what)
}

// reconcile: ตาข่ายกันตก ถ้า webhook หาย order จะค้าง AWAITING_PAYMENT ไปตลอด
// จึงต้องไล่ถาม PSP เองเป็นระยะ (ของจริงทำทุกไม่กี่นาที และมี report กระทบยอดรายวันอีกชั้น)
func (s *shop) reconcile() {
	for range time.Tick(500 * time.Millisecond) {
		var stuck []*order
		s.mu.Lock()
		for _, o := range s.orders {
			if o.State == "AWAITING_PAYMENT" && time.Since(o.updatedAt) > 1500*time.Millisecond {
				stuck = append(stuck, o)
			}
		}
		s.mu.Unlock()
		for _, o := range stuck {
			var c charge
			if err := s.callPSP("GET", "/charges/"+o.ChargeID, "", nil, &c); err != nil || c.Status == "pending" {
				continue
			}
			logf("RECON", "%s ค้าง AWAITING_PAYMENT แต่ PSP บอกว่า %s → อัปเดตเอง (webhook อาจหาย)", o.ID, c.Status)
			s.applyCharge(c, "reconcile")
		}
	}
}

func (s *shop) callPSP(method, path, idemKey string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, s.pspURL+path, body)
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := pspClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("psp %s: %s", resp.Status, bytes.TrimSpace(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// insurer ปลอม: ออกกรมธรรม์แบบ idempotent ด้วย order id ถ้าเรียกซ้ำได้เลขเดิม ไม่ออกสองฉบับ
type insurer struct {
	mu     sync.Mutex
	seq    int
	issued map[string]string
}

func (in *insurer) issue(orderID string, reject bool) (string, error) {
	time.Sleep(400 * time.Millisecond)
	in.mu.Lock()
	defer in.mu.Unlock()
	if p, ok := in.issued[orderID]; ok {
		return p, nil
	}
	if reject {
		logf("INSURER", "ปฏิเสธ %s (ข้อมูลผู้ขับไม่ผ่านเกณฑ์)", orderID)
		return "", errors.New("underwriting_rejected")
	}
	in.seq++
	p := fmt.Sprintf("POL-%04d", in.seq)
	in.issued[orderID] = p
	logf("INSURER", "ออกกรมธรรม์ %s ให้ %s", p, orderID)
	return p, nil
}
