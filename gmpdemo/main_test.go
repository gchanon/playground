package main

// test อยู่ใน package main เดียวกัน → เรียกฟังก์ชันตัวเล็ก ๆ ข้างในได้หมด
// รวมถึง main() เองด้วย (ดู TestMain_Direct)

import (
	"flag"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

// 1) จุดที่เล็กที่สุด — แค่ดูว่าตอนนี้ runtime สร้าง M ไปกี่ตัว
func TestThreadsCreated_Baseline(t *testing.T) {
	t.Logf("GOMAXPROCS (P) = %d", runtime.GOMAXPROCS(0))
	t.Logf("NumGoroutine (G) = %d", runtime.NumGoroutine())
	t.Logf("threadsCreated (M) = %d", threadsCreated())
}

//  2. ★ ยิง cgo แค่ "ครั้งเดียว" แล้วดูทีละจุดว่าเกิดอะไร
//     วาง breakpoint ตรงบรรทัด callCgo ได้เลย
func TestSingleCgoCall_StepByStep(t *testing.T) {
	before := threadsCreated()
	t.Logf("[1] ก่อนเรียก cgo      M=%d", before)

	start := time.Now()
	callCgo(50) // <-- วาง breakpoint ตรงนี้ แล้ว step เข้าไปดู
	elapsed := time.Since(start)

	after := threadsCreated()
	t.Logf("[2] หลังเรียก cgo      M=%d  (ใช้เวลา %v)", after, elapsed)
	t.Logf("[3] M เพิ่มขึ้น        %+d", after-before)

	if elapsed < 40*time.Millisecond {
		t.Errorf("C function น่าจะ block อย่างน้อย 50ms แต่ได้ %v", elapsed)
	}
}

// 3) เทียบ pure-Go กับ cgo ด้วย n เล็ก ๆ จะได้ step ไหว
func TestCompare_PureGo_vs_Cgo(t *testing.T) {
	const n = 20
	const ms = 50

	pure := runScenario("pure Go", n, "", func(id int) {
		deadline := time.Now().Add(ms * time.Millisecond)
		x := 0
		for time.Now().Before(deadline) {
			x++
		}
		_ = x
	})
	t.Logf("pure Go : G=%d  M %d -> %d  (เพิ่ม %+d)", n, pure.before, pure.peak, pure.peak-pure.before)

	cgo := runScenario("cgo", n, "", func(id int) {
		callCgo(ms)
	})
	t.Logf("cgo     : G=%d  M %d -> %d  (เพิ่ม %+d)", n, cgo.before, cgo.peak, cgo.peak-cgo.before)

	deltaPure := pure.peak - pure.before
	deltaCgo := cgo.peak - cgo.before
	if deltaCgo <= deltaPure {
		t.Errorf("คาดว่า cgo ต้องสร้าง thread มากกว่า pure Go: cgo=%+d pure=%+d", deltaCgo, deltaPure)
	}
}

// 4) ดูว่า goroutine ที่ "จอด" ไม่กิน M จริงไหม
func TestParkedGoroutines_DoNotEatThreads(t *testing.T) {
	const n = 500

	before := threadsCreated()
	t.Logf("[1] ก่อนแตก goroutine  G=%d  M=%d", runtime.NumGoroutine(), before)

	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() { defer wg.Done(); <-release }()
	}

	time.Sleep(100 * time.Millisecond) // ให้ทุกตัวไปจอดก่อน
	during := threadsCreated()
	t.Logf("[2] ระหว่างจอด %d ตัว   G=%d  M=%d  (เพิ่ม %+d)", n, runtime.NumGoroutine(), during, during-before)

	close(release)
	wg.Wait()
	t.Logf("[3] หลังปล่อยหมด       G=%d  M=%d", runtime.NumGoroutine(), threadsCreated())

	if during-before > 10 {
		t.Errorf("goroutine ที่จอดไม่ควรสร้าง thread เยอะขนาดนี้: +%d", during-before)
	}
}

// 5) เรียก child() ตรง ๆ — เหมือนที่ process ลูกทำ
func TestChild_Direct(t *testing.T) {
	child(99, 30)
}

// 6) ★ เรียก main() จาก test ได้จริง — แค่ต้องจัดการ flag ให้ถูก
func TestMain_Direct(t *testing.T) {
	oldArgs := os.Args
	oldFlags := flag.CommandLine
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldFlags
	}()

	// testing ลงทะเบียน flag ของตัวเองไว้ใน flag.CommandLine แล้ว
	// ถ้าไม่สร้างใหม่ main() จะเจอ flag แปลกปลอมแล้ว exit
	os.Args = []string{"gmpdemo", "-n=5", "-ms=20"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	main() // <-- วาง breakpoint ตรงนี้ แล้ว step เข้า main() ได้เลย
}
