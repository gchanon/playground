// ssedemo — ระบบเทียบราคาประกันรถแบบ SSE (Server-Sent Events) ย่อส่วน
//
// ผู้ใช้ขอราคา 1 ครั้ง → server ยิงถามบริษัทประกัน 20 เจ้าพร้อมกัน
// แล้ว "ส่งราคาทีละเจ้าทันทีที่ได้" ผ่าน HTTP connection เดียวที่เปิดค้างไว้
// ไม่ต้องรอให้ครบทุกเจ้า
//
// บริษัทประกันเป็นของปลอม: แต่ละเจ้าแค่ sleep ตามเวลาที่กำหนดแล้วคืนราคา
// cache เป็น map ใน memory (ของจริงควรเป็น Redis + TTL 24 ชม.)
//
// รัน:   go run ./ssedemo
// เปิด:  http://localhost:8080
// ดู stream ดิบ:  curl -N "localhost:8080/quotes?car=yaris-2020"
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"sync"
	"time"
)

//go:embed index.html
var indexHTML []byte

// timeout สองตัว ตอบคนละคำถาม
const (
	userDeadline   = 5 * time.Second  // ผู้ใช้รอได้แค่นี้ แล้วปิดรายการ
	insurerTimeout = 20 * time.Second // การเรียกประกันทำต่อเบื้องหลังได้นานกว่า เพื่อเอาราคาลง cache
)

type insurer struct {
	name    string
	latency time.Duration // เวลาที่เจ้านี้ใช้ตอบ (ปลอม)
}

// 20 เจ้า เร็วช้าไม่เท่ากัน 2 เจ้าสุดท้ายช้ากว่า userDeadline ผู้ใช้จึงไม่เห็นในรอบแรก
// แต่ยังทัน insurerTimeout จึงเข้า cache ให้คนถัดไป
var insurers = []insurer{
	{"Alpha", 300 * time.Millisecond}, {"Bravo", 450 * time.Millisecond},
	{"Charlie", 600 * time.Millisecond}, {"Delta", 700 * time.Millisecond},
	{"Echo", 800 * time.Millisecond}, {"Foxtrot", 900 * time.Millisecond},
	{"Golf", 1000 * time.Millisecond}, {"Hotel", 1100 * time.Millisecond},
	{"India", 1200 * time.Millisecond}, {"Juliet", 1300 * time.Millisecond},
	{"Kilo", 1500 * time.Millisecond}, {"Lima", 1600 * time.Millisecond},
	{"Mike", 1800 * time.Millisecond}, {"November", 2000 * time.Millisecond},
	{"Oscar", 2200 * time.Millisecond}, {"Papa", 2500 * time.Millisecond},
	{"Quebec", 3000 * time.Millisecond}, {"Romeo", 4000 * time.Millisecond},
	{"Sierra", 8 * time.Second}, {"Tango", 15 * time.Second},
}

// callInsurer แทนการเรียก API จริง: รอ latency แล้วคืนราคา หรือเลิกเมื่อ ctx ถูก cancel
func callInsurer(ctx context.Context, ins insurer, car string) (int, error) {
	select {
	case <-time.After(ins.latency):
		h := fnv.New32a()
		h.Write([]byte(ins.name + car))
		return 12000 + int(h.Sum32()%8000), nil // ราคาเดิมทุกครั้งสำหรับรถคันเดิม
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// cache แบบ cache-aside: key แยกต่อ "รถ + เจ้า" คำขอเดียวจึง hit บางเจ้า miss บางเจ้าได้
var (
	cacheMu sync.Mutex
	cache   = map[string]int{}
)

// result คือสิ่งที่ goroutine ของแต่ละเจ้าส่งกลับมา 1 ชิ้นเสมอ ไม่ว่าสำเร็จหรือพลาด
type result struct {
	Insurer string `json:"insurer"`
	Price   int    `json:"price,omitempty"`
	Source  string `json:"source,omitempty"` // "cache" หรือ "live"
	Ms      int64  `json:"ms"`               // นับจากตอนผู้ใช้กดขอราคา
	Err     string `json:"error,omitempty"`
}

func quotesHandler(w http.ResponseWriter, r *http.Request) {
	car := r.URL.Query().Get("car")
	if car == "" {
		http.Error(w, "missing ?car=", http.StatusBadRequest)
		return
	}

	// 1) บอก browser ว่านี่คือ stream ไม่ใช่ response ก้อนเดียว
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)

	// 2) ctx ของการเรียกประกัน "ไม่ผูก" กับ connection ของผู้ใช้ (WithoutCancel)
	//    ผู้ใช้ปิดแท็บหรือเลย userDeadline ไปแล้ว การเรียกก็ยังทำต่อจนได้ราคาลง cache
	//    เพราะ token และโควตาของบริษัทประกันถูกใช้ไปตั้งแต่ตอนส่งแล้ว cancel ไม่ได้อะไรคืน
	//    แต่ยังมีเพดาน insurerTimeout ของตัวเอง goroutine จึงไม่ค้างไม่รู้จบ
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), insurerTimeout)
	var wg sync.WaitGroup

	// 3) fan-out: 1 goroutine ต่อเจ้า
	//    buffer = จำนวนเจ้า → goroutine ส่งผลได้เสมอแม้ handler เลิกอ่านไปแล้ว ไม่ค้าง (ไม่ leak)
	start := time.Now()
	results := make(chan result, len(insurers))
	for _, ins := range insurers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := car + "|" + ins.name

			cacheMu.Lock()
			price, hit := cache[key]
			cacheMu.Unlock()
			if hit {
				results <- result{Insurer: ins.name, Price: price, Source: "cache", Ms: time.Since(start).Milliseconds()}
				return
			}

			price, err := callInsurer(ctx, ins, car)
			if err != nil {
				results <- result{Insurer: ins.name, Err: err.Error(), Ms: time.Since(start).Milliseconds()}
				return
			}
			cacheMu.Lock()
			cache[key] = price // เขียนลง cache ให้คนถัดไปได้ใช้
			cacheMu.Unlock()
			log.Printf("cached %s after %v", key, time.Since(start).Round(time.Millisecond))
			results <- result{Insurer: ins.name, Price: price, Source: "live", Ms: time.Since(start).Milliseconds()}
		}()
	}
	// คืน timer ของ ctx เมื่อครบทุกเจ้า ไม่ใช่ตอน handler return (ห้าม defer cancel() ไม่งั้นงานเบื้องหลังโดนตัด)
	go func() { wg.Wait(); cancel() }()

	// 4) รับผลทีละเจ้าตามลำดับที่เสร็จ แล้วส่งออกไปทันที จนครบหรือถึง userDeadline
	//    ทุก goroutine ส่งผลมา 1 ชิ้นแน่นอน (สำเร็จ, error หรือ timeout) จึงวนไม่เกิน 20 รอบ
	got := 0
	deadline := time.After(userDeadline)
	for i := range insurers {
		var res result
		select {
		case res = <-results:
		case <-deadline:
			// ผู้ใช้รอครบแล้ว ปิดรายการ ส่วนเจ้าที่ยังไม่ตอบทำต่อเบื้องหลัง
			pending := len(insurers) - i
			fmt.Fprintf(w, "event: done\ndata: {\"got\":%d,\"total\":%d,\"ms\":%d,\"pending\":%d}\n\n",
				got, len(insurers), time.Since(start).Milliseconds(), pending)
			rc.Flush()
			log.Printf("car=%s got=%d/%d at deadline, %d still running in background", car, got, len(insurers), pending)
			return
		}
		event := "quote"
		if res.Err != "" {
			event = "skip"
		} else {
			got++
		}
		data, _ := json.Marshal(res)
		// รูปแบบของ SSE: "event: ชื่อ\ndata: ข้อมูล\n\n" (บรรทัดว่างคือจบ 1 event)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		if err := rc.Flush(); err != nil { // ดันออกไปเลย ไม่รอ buffer เต็ม
			log.Printf("client gone: %v", err)
			return
		}
	}

	// 5) บอก browser ว่าครบแล้ว ให้ปิด connection เอง (ไม่งั้น EventSource จะ reconnect อัตโนมัติ)
	fmt.Fprintf(w, "event: done\ndata: {\"got\":%d,\"total\":%d,\"ms\":%d}\n\n",
		got, len(insurers), time.Since(start).Milliseconds())
	rc.Flush()
	log.Printf("car=%s got=%d/%d in %v", car, got, len(insurers), time.Since(start).Round(time.Millisecond))
}

func main() {
	http.HandleFunc("/quotes", quotesHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	log.Println("open http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
