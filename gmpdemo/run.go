package main

import (
	"sync"
	"time"
)

// runScenario รัน work() พร้อมกัน n ตัว แล้ววัดว่า runtime สร้าง thread เพิ่มกี่ตัว
func runScenario(name string, n int, note string, work func(id int)) result {

	before := threadsCreated()
	w := watchPeak()
	start := time.Now()

	tm := newThreadMap()
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(id int) {
			defer wg.Done()
			work(id)
			tm.record(threadID()) // วัดตอนจบ = thread ที่ goroutine นี้ลงเอย
		}(i)
	}
	wg.Wait()

	elapsed := time.Since(start)
	peak := w.finish()
	after := threadsCreated()

	return result{name, n, before, after, peak, elapsed, note, tm}
}
