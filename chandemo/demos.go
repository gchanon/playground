package main

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// ─────────────────────────────────────────────────────────────
// 1) buffer ใหญ่ขึ้น = เร็วขึ้นจริงมั้ย
//
//	unbuffered ต้อง "นัดเจอกัน" ทุกครั้งที่ส่ง 1 ค่า
//	buffered วางไว้บนชั้นแล้วเดินไปทำอย่างอื่นต่อได้
//
// ─────────────────────────────────────────────────────────────
func demoBuffer(n int) {
	fmt.Printf("\n── 1. ขนาด buffer มีผลกับความเร็วแค่ไหน (ส่ง %d ค่า) ──\n\n", n)
	fmt.Println("  buffer │     เวลา │ ต่อ 1 ค่า │ เทียบ unbuffered")
	fmt.Println("  ───────┼──────────┼───────────┼─────────────────")

	var base time.Duration
	for _, size := range []int{0, 1, 10, 100, 1000} {
		ch := make(chan int, size)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range ch {
			}
		}()

		start := time.Now()
		for i := 0; i < n; i++ {
			ch <- i
		}
		close(ch)
		wg.Wait()
		elapsed := time.Since(start)

		if size == 0 {
			base = elapsed
		}
		fmt.Printf("  %6d │ %8v │ %7.0f ns │ %.2fx\n",
			size, elapsed.Round(time.Millisecond),
			float64(elapsed.Nanoseconds())/float64(n),
			float64(base)/float64(elapsed))
	}
	fmt.Println("\n  อ่านผล: buffer ช่วยลด \"จำนวนครั้งที่ต้องนัดเจอกัน\" ไม่ได้ทำให้ CPU เร็วขึ้น")
	fmt.Println("          พอ buffer ใหญ่พอที่ผู้รับตามทัน เพิ่มต่อก็ไม่ช่วยแล้ว")
}

// ─────────────────────────────────────────────────────────────
// 2) unbuffered = ผู้ส่งค้างรอจนผู้รับมารับจริงๆ
//
//	วัดว่า `ch <- x` คืนค่าตอนไหน เทียบกับตอนที่ผู้รับมาถึง
//
// ─────────────────────────────────────────────────────────────
func demoRendezvous() {
	fmt.Println("\n── 2. ส่งเข้า channel แล้ว \"คืนค่า\" ตอนไหน ──")
	fmt.Println("\n  ผู้รับจะหลับ 300ms ก่อนค่อยมารับ แล้วดูว่าผู้ส่งค้างอยู่กี่ ms")
	fmt.Println()
	fmt.Println("  แบบ          │ ผู้ส่งค้าง │ แปลว่า")
	fmt.Println("  ─────────────┼───────────┼──────────────────────────────")

	for _, c := range []struct {
		label string
		size  int
	}{{"unbuffered", 0}, {"buffer 1", 1}} {
		ch := make(chan int, c.size)

		done := make(chan struct{})
		go func() {
			defer close(done)
			time.Sleep(300 * time.Millisecond) // ผู้รับมาสาย
			<-ch
		}()

		start := time.Now()
		ch <- 1
		blocked := time.Since(start)
		<-done

		meaning := "วางบนชั้นแล้วเดินต่อได้เลย"
		if blocked > 100*time.Millisecond {
			meaning = "ต้องยืนถือจานรอจนคนมารับ"
		}
		fmt.Printf("  %-12s │ %9v │ %s\n", c.label, blocked.Round(time.Millisecond), meaning)
	}
	fmt.Println("\n  อ่านผล: unbuffered ไม่ใช่ \"buffer ขนาด 0\" แต่คือ \"ต้องส่งมือต่อมือ\"")
	fmt.Println("          ผู้ส่งกับผู้รับต้องมาเจอกันพอดี ถึงจะผ่านไปได้ทั้งคู่")
}

// ─────────────────────────────────────────────────────────────
// 3) ★ G ที่รอ channel ไม่กิน M เลย
//
//	จุดเชื่อมกับ GMP model: "รอ channel" กับ "ติด syscall" คนละเรื่องกันคนละโลก
//	ทั้งคู่คือ G ที่ยังไม่ได้ทำงาน แต่ราคาที่ runtime จ่ายต่างกันลิบ
//
// ─────────────────────────────────────────────────────────────
func demoPark(n int) {
	fmt.Printf("\n── 3. ★ goroutine %d ตัว \"รออยู่\" พร้อมกัน ใช้ thread กี่ตัว ──\n\n", n)

	fmt.Println("  แบบที่รอ         │ G เพิ่ม │ M เพิ่ม │ แปลว่า")
	fmt.Println("  ─────────────────┼─────────┼─────────┼────────────────────────────")

	// (ก) รอ channel — runtime จอด G ไว้เอง
	gate := make(chan struct{})
	dg, dm := measureWait(n, func() { <-gate }, func() { close(gate) })
	fmt.Printf("  %-16s │ %+7d │ %+7d │ %s\n", "รอ channel", dg, dm, "M ว่างทันที ไปหยิบ G อื่นทำต่อ")

	// (ข) ติด syscall จริง — kernel ยึด thread ไว้ runtime แตะไม่ได้
	//     select(2) ที่ไม่เฝ้า fd เลย = นอนในเคอร์เนลเฉยๆ (ไม่ใช่ time.Sleep ของ Go)
	//     หมดเวลาเองใน 300ms ไม่ต้องมีคนปลุก
	dg2, dm2 := measureWait(n, func() {
		tv := syscall.Timeval{Sec: 0, Usec: 300000}
		_ = sysSelect(&tv)
	}, nil)
	fmt.Printf("  %-16s │ %+7d │ %+7d │ %s\n", "ติด syscall จริง", dg2, dm2, "M ติดไปด้วย ต้องจ้าง M ใหม่มาแทน")

	fmt.Println("\n  อ่านผล: G ที่รอ channel ถูก runtime \"จอด\" (park) ไว้เอง ไม่ได้นั่งทับ M")
	fmt.Println("          → รอกี่ตัวก็ได้ ไม่เปลืองอะไรนอกจาก memory ของ G เอง")
	fmt.Println("          แต่ G ที่หายเข้าไปใน kernel จะลาก M ติดไปด้วย runtime เรียกกลับไม่ได้")
	fmt.Println("          → sysmon เห็นว่า P ว่าง ก็ต้องสร้าง M ใหม่มานั่งโต๊ะแทน")
}

// measureWait = ปล่อย goroutine n ตัวไป "รอ" ด้วยวิธี wait แล้ววัดว่า G / M ขยับเท่าไร
// release = วิธีปลุกให้ทุกตัวเลิกรอ (nil ถ้า wait หมดเวลาเอง)
func measureWait(n int, wait func(), release func()) (dG, dM int) {
	beforeM, beforeG := threadsCreated(), runtime.NumGoroutine()

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			wait()
		}()
	}

	// รอจนทุกตัวเกิดครบและเข้าสู่สภาพ "รอ" จริงๆ
	for runtime.NumGoroutine() < beforeG+n {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)

	dG = runtime.NumGoroutine() - beforeG
	dM = threadsCreated() - beforeM

	if release != nil {
		release()
	}
	wg.Wait()
	return dG, dM
}

// ─────────────────────────────────────────────────────────────
// 4) select สุ่มจริงมั้ย เมื่อหลาย case พร้อมพร้อมกัน
// ─────────────────────────────────────────────────────────────
func demoSelectRandom(rounds int) {
	fmt.Printf("\n── 4. select สุ่มจริงมั้ย (ทั้ง 2 case พร้อมเสมอ %d รอบ) ──\n\n", rounds)

	a := make(chan int, rounds)
	b := make(chan int, rounds)
	for i := 0; i < rounds; i++ {
		a <- 1
		b <- 1
	}

	ca, cb := 0, 0
	for i := 0; i < rounds; i++ {
		select {
		case <-a:
			ca++
		case <-b:
			cb++
		}
	}

	fmt.Printf("  case A ถูกเลือก %d ครั้ง (%.1f%%)\n", ca, float64(ca)*100/float64(rounds))
	fmt.Printf("  case B ถูกเลือก %d ครั้ง (%.1f%%)\n", cb, float64(cb)*100/float64(rounds))
	fmt.Println("\n  อ่านผล: ใกล้ 50/50 → select สุ่มจริง ไม่ได้ไล่จากบนลงล่างเหมือน switch")
	fmt.Println("          ถ้าไม่สุ่ม case บนจะกินรวบ case ล่างจนอดตาย (starvation)")
}

// ─────────────────────────────────────────────────────────────
// 5) ตารางกฎ: ทำอะไรแล้วบล็อก / พัง / ผ่าน
//
//	ไม่ท่องจำ — ยิง probe ดูจริงทีละเคส
//
// ─────────────────────────────────────────────────────────────
func demoRules() {
	fmt.Println("\n── 5. ตารางกฎ channel (วัดจริงทุกช่อง ไม่ได้ลอกตำรา) ──")
	fmt.Println()
	fmt.Println("  สภาพ channel        │ ทำอะไร      │ ผลที่วัดได้")
	fmt.Println("  ────────────────────┼─────────────┼──────────────────────────")

	row := func(state, op string, f func()) {
		fmt.Printf("  %-19s │ %-11s │ %s\n", state, op, probe(f))
	}

	var nilCh chan int
	row("nil (ไม่ได้ make)", "ส่ง", func() { nilCh <- 1 })
	row("nil (ไม่ได้ make)", "รับ", func() { <-nilCh })
	row("nil (ไม่ได้ make)", "close", func() { close(nilCh) })

	row("unbuffered ไม่มีคู่", "ส่ง", func() { ch := make(chan int); ch <- 1 })
	row("unbuffered ไม่มีคู่", "รับ", func() { ch := make(chan int); <-ch })

	row("buffered ว่าง", "ส่ง", func() { ch := make(chan int, 1); ch <- 1 })
	row("buffered ว่าง", "รับ", func() { ch := make(chan int, 1); <-ch })

	row("buffered เต็ม", "ส่ง", func() { ch := make(chan int, 1); ch <- 1; ch <- 2 })
	row("buffered เต็ม", "รับ", func() { ch := make(chan int, 1); ch <- 1; <-ch })

	row("ปิดแล้ว (ว่าง)", "รับ", func() { ch := make(chan int); close(ch); <-ch })
	row("ปิดแล้ว (มีของ)", "รับ", func() { ch := make(chan int, 1); ch <- 9; close(ch); <-ch })
	row("ปิดแล้ว", "ส่ง", func() { ch := make(chan int); close(ch); ch <- 1 })
	row("ปิดแล้ว", "close ซ้ำ", func() { ch := make(chan int); close(ch); close(ch) })

	// รับจาก channel ที่ปิดแล้ว ได้ค่าอะไร
	closed := make(chan int, 1)
	closed <- 42
	close(closed)
	v1, ok1 := <-closed
	v2, ok2 := <-closed
	fmt.Printf("\n  รับจาก channel ที่ปิดแล้ว: ครั้งที่ 1 → v=%d ok=%v (ของที่ค้างอยู่)\n", v1, ok1)
	fmt.Printf("                              ครั้งที่ 2 → v=%d ok=%v (หมดแล้ว = zero value)\n", v2, ok2)

	fmt.Println("\n  อ่านผล: \"ปิดแล้ว\" ไม่ได้แปลว่าของหาย — ของที่ค้างใน buffer ยังรับได้จนหมด")
	fmt.Println("          ok=false คือสัญญาณเดียวที่บอกว่า \"หมดจริงและปิดแล้ว\"")
	fmt.Println("          กฎนิ้วโป้ง: ฝั่งที่ส่งเป็นคนปิด ฝั่งรับห้ามปิด (ไม่งั้นผู้ส่ง panic)")
}

// sysSelect = นอนในเคอร์เนลเฉยๆ ตามเวลาที่กำหนด โดยไม่ผ่าน scheduler ของ Go
// ใช้ select(2) ที่ไม่เฝ้า fd อะไรเลย — ได้ผลเหมือน sleep แต่ Go แตะ thread นี้ไม่ได้
// (ต่างจาก time.Sleep ที่ runtime จัดการเองด้วย timer ภายใน)
func sysSelect(tv *syscall.Timeval) error {
	return syscall.Select(0, nil, nil, nil, tv)
}
