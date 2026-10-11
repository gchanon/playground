// chandemo — วัดพฤติกรรมของ channel ด้วยการทดลองจริง ไม่ใช่ท่องตำรา
//
// อุปมาที่ใช้ทั้งไฟล์: channel = "ช่องส่งอาหาร" ระหว่างครัวกับหน้าร้าน
//
//	ผู้ส่ง (sender)   = เชฟที่ยกจานมาวาง
//	ผู้รับ (receiver) = เด็กเสิร์ฟที่มาหยิบ
//	buffer            = ชั้นวางจานในช่อง (ขนาด N จาน)
package main

import (
	"flag"
	"fmt"
	"runtime"
	"runtime/pprof"
	"time"
)

// threadsCreated = จำนวน OS thread (M) ที่ runtime เคยสร้าง — สะสม ไม่ลดลง
// ตัวเดียวกับที่ใช้ใน gmpdemo
func threadsCreated() int {
	return pprof.Lookup("threadcreate").Count()
}

// probe = ลองทำ op ดูว่าจบทันที / ค้าง / พัง
// ใช้ตอบคำถาม "ทำแบบนี้แล้วบล็อกมั้ย" โดยไม่ต้องเดา
func probe(op func()) string {
	res := make(chan string, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				res <- fmt.Sprintf("panic: %v", r)
			}
		}()
		op()
		res <- "ผ่านทันที"
	}()

	select {
	case s := <-res:
		return s
	case <-time.After(50 * time.Millisecond):
		// goroutine ตัวนั้นค้างอยู่จริงๆ ปล่อยทิ้งไว้ — โปรแกรมจบแล้วมันก็หายไปเอง
		return "บล็อกค้าง"
	}
}

func main() {
	demo := flag.String("demo", "all", "all | buffer | rendezvous | park | random | rules | fair")
	// n := flag.Int("n", 100000, "จำนวนข้อความที่ส่ง (demo buffer / random)")
	// g := flag.Int("g", 1000, "จำนวน goroutine ที่จอดรอ (demo park)")
	// r := flag.Int("r", 5, "จำนวน receiver (demo fair)")
	flag.Parse()

	fmt.Printf("\n=== เครื่องนี้: NumCPU=%d  GOMAXPROCS=%d ===\n",
		runtime.NumCPU(), runtime.GOMAXPROCS(0))

	run := func(name string, f func()) {
		if *demo == "all" || *demo == name {
			f()
		}
	}

	run("buffer", func() { demoBuffer(5) })
	// run("rendezvous", func() { demoRendezvous() })
	// run("park", func() { demoPark(*g) })
	// run("random", func() { demoSelectRandom(*n) })
	// run("rules", func() { demoRules() })
	// run("fair", func() { demoFairness(*r, 1000) })

	fmt.Println()
}
