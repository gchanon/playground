// gmpdemo — จำลองให้เห็นว่า GMP model ทำงานยังไง
// และทำไม goroutine ที่เรียก cgo ถึงบังคับให้ runtime สร้าง OS thread เพิ่ม
//
// G = goroutine        (งาน)
// M = OS thread        (คนงาน — OS เห็นแค่ตัวนี้)
// P = logical processor (โต๊ะ = GOMAXPROCS)
package main

/*
#include <unistd.h>
#include <pthread.h>
#include <stdint.h>

// c_thread_id = เลขประจำตัวของ OS thread ที่กำลังรันอยู่
// ต้องถามผ่าน C เพราะ Go ไม่เปิดเผยเลข M ให้โค้ด Go เห็น
static unsigned long long c_thread_id(void) {
    return (unsigned long long)(uintptr_t)pthread_self();
}

// จำลอง YottaDB call: โค้ด C ที่ "ทำงานนาน" อยู่ฝั่ง C
// ระหว่างอยู่ในฟังก์ชันนี้ Go แตะ thread ที่เรียกมันไม่ได้เลย
static void c_blocking_work(int ms) {
    usleep(ms * 1000);
}
*/
import "C"

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// callCgo = ห่อการเรียก C ไว้ในฟังก์ชัน Go ธรรมดา
// ทำให้วาง breakpoint / step ได้ง่าย (delve step เข้าโค้ด C ไม่ได้)
func callCgo(ms int) {
	C.c_blocking_work(C.int(ms))
}

// baselineM = จำนวน M ตอนโปรแกรมเพิ่งเริ่ม ก่อนรัน scenario ใดๆ
// เป็นค่าคงที่ของ runtime เอง ไม่เกี่ยวกับงานที่เราสั่ง
var baselineM int

// threadID = เลข OS thread ที่ goroutine นี้กำลังนั่งอยู่ ณ ตอนที่เรียก
func threadID() uint64 { return uint64(C.c_thread_id()) }

// threadMap = นับว่า thread ไหนรับ goroutine ไปกี่ตัว
type threadMap struct {
	mu    sync.Mutex
	count map[uint64]int
}

func newThreadMap() *threadMap { return &threadMap{count: map[uint64]int{}} }

func (t *threadMap) record(tid uint64) {
	t.mu.Lock()
	t.count[tid]++
	t.mu.Unlock()
}

// perThread = จำนวน goroutine ต่อ thread เรียงมากไปน้อย
func (t *threadMap) perThread() []int {
	out := make([]int, 0, len(t.count))
	for _, c := range t.count {
		out = append(out, c)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

// threadsCreated = จำนวน OS thread ที่ runtime เคยสร้าง (สะสม)
// เป็นตัวเลขเดียวที่ดูได้แบบ portable ทั้ง mac/linux
func threadsCreated() int {
	return pprof.Lookup("threadcreate").Count()
}

func snapshot(label string) {
	fmt.Printf("  %-22s P=%-3d G=%-5d M(created)=%-4d\n",
		label, runtime.GOMAXPROCS(0), runtime.NumGoroutine(), threadsCreated())
}

// peakWatcher คอยสุ่มดูจำนวน thread ระหว่าง scenario ทำงาน
type peakWatcher struct {
	peak int64
	stop chan struct{}
	done chan struct{}
}

func watchPeak() *peakWatcher {
	w := &peakWatcher{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		t := time.NewTicker(2 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-t.C:
				if n := int64(threadsCreated()); n > atomic.LoadInt64(&w.peak) {
					atomic.StoreInt64(&w.peak, n)
				}
			}
		}
	}()
	return w
}

func (w *peakWatcher) finish() int {
	close(w.stop)
	<-w.done
	return int(atomic.LoadInt64(&w.peak))
}

type result struct {
	name     string
	n        int
	before   int
	after    int
	peak     int
	duration time.Duration
	note     string
	tm       *threadMap
}

func printTable(rs []result) {
	fmt.Println()
	fmt.Printf("  ตัวเลข M ข้างล่างหักฐาน baseline (%d) ออกให้แล้ว = ส่วนที่งานทำให้งอกจริงๆ\n", baselineM)
	fmt.Println("┌──────────────────────────┬───────┬──────────┬──────────┬──────────┬───────────┐")
	fmt.Println("│ scenario                 │  G    │ M ก่อน   │ M หลัง   │ M เพิ่ม  │  เวลา     │")
	fmt.Println("├──────────────────────────┼───────┼──────────┼──────────┼──────────┼───────────┤")
	for _, r := range rs {
		fmt.Printf("│ %-24s │ %5d │ %8d │ %8d │ %+8d │ %9s │\n",
			r.name, r.n, r.before-baselineM, r.peak-baselineM, r.peak-r.before, r.duration.Round(time.Millisecond))
	}
	fmt.Println("└──────────────────────────┴───────┴──────────┴──────────┴──────────┴───────────┘")
	fmt.Println()
	for _, r := range rs {
		fmt.Printf("  • %-24s %s\n", r.name, r.note)
	}
}

// printThreadMap สรุปว่าแต่ละ scenario ใช้ thread กี่ตัว และกระจายงานยังไง
func printThreadMap(rs []result) {
	fmt.Println()
	fmt.Println("  ── thread ไหนรัน goroutine ไหนบ้าง (สรุป) ──")
	for _, r := range rs {
		per := r.tm.perThread()
		show := per
		more := ""
		if len(per) > 8 {
			show = per[:8]
			more = fmt.Sprintf(" …อีก %d ตัว", len(per)-8)
		}
		parts := make([]string, len(show))
		for i, c := range show {
			parts[i] = strconv.Itoa(c)
		}
		fmt.Printf("  %-24s ใช้ %3d thread │ G ต่อ thread: %s%s\n",
			r.name, len(per), strings.Join(parts, " "), more)
	}
}

func scenarios(n int, workMS int) {
	fmt.Printf("\n=== เครื่องนี้: NumCPU=%d  GOMAXPROCS=%d  PID=%d ===\n",
		runtime.NumCPU(), runtime.GOMAXPROCS(0), os.Getpid())
	fmt.Printf("=== ทุก scenario แตก goroutine %d ตัว งานละ ~%dms ===\n\n", n, workMS)

	baselineM = threadsCreated()
	snapshot("ตอนเริ่ม (baseline)")
	fmt.Printf("  └─ M=%d นี้คือ \"พนักงานประจำ\" ของ runtime เอง มีมาก่อน main() จะรัน:\n", baselineM)
	fmt.Println("     thread แรกของ process + ยาม(sysmon) + worker ว่างๆ ของ GC")
	fmt.Println("     + อีก 2 ที่ cgo บังคับให้มี (1 ตัวเป็นแค่ struct ไม่มี thread จริง)")

	var rs []result

	// 1) งาน Go ล้วน — CPU bound
	//    scheduler สลับ G ลง P ได้ตามปกติ → thread ไม่บาน
	rs = append(rs, runScenario("1. pure Go (CPU)", n,
		fmt.Sprintf("โค้ด Go ล้วน → scheduler ยัดหลาย G ลงไม่กี่ M ได้ → thread ไม่บาน\n    (เวลานานกว่า %dms เพราะ %d G แย่ง P แค่ %d ตัว — ต้องต่อคิวกัน)",
			workMS, n, runtime.GOMAXPROCS(0)),
		func(id int) {
			deadline := time.Now().Add(time.Duration(workMS) * time.Millisecond)
			x, rounds := 0, 0
			for time.Now().Before(deadline) {
				for i := 0; i < 5000; i++ {
					x += i * i
				}

				rounds++
			}

			if id == 99 {
				fmt.Printf("  [G99] วนได้ %d ชุด · ใช้เวลาจริง %v (เกิน deadline %v)\n",
					rounds, time.Since(deadline.Add(-time.Duration(workMS)*time.Millisecond)),
					time.Since(deadline))
			}
			_ = x
		}))

	// 2) รอ channel — G ถูก park ไว้ ไม่กิน M เลย
	rs = append(rs, runScenario("2. channel wait", n,
		"G จอด (park) → M ไม่ลุกจากโต๊ะ ไปหยิบ G อื่นมาทำต่อ → M น้อยที่สุด",
		func(id int) {
			ch := make(chan struct{})
			go func() {
				time.Sleep(time.Duration(workMS) * time.Millisecond)
				close(ch)
			}()
			<-ch
		}))

	// 3) time.Sleep — runtime timer ไม่ใช่ syscall ต่อ goroutine
	rs = append(rs, runScenario("3. time.Sleep", n,
		"runtime จัดการเองด้วย timer → ไม่ต้องมี thread ต่อ G",
		func(id int) {
			time.Sleep(time.Duration(workMS) * time.Millisecond)
		}))

	// 4) ★ cgo — จุดสำคัญ
	//    ระหว่างอยู่ใน C: M ตัวนั้นถูกยึด เอา G อื่นมาทำแทนไม่ได้
	//    runtime ต้องสร้าง M ใหม่มานั่งโต๊ะ (P) แทน
	rs = append(rs, runScenario("4. ★ cgo (blocking C)", n,
		"★ G ติดอยู่ใน C → ยึด M ทั้งตัว → runtime ต้องสร้าง M ใหม่ ≈ จำนวน G",
		func(id int) {
			callCgo(workMS)
		}))

	printTable(rs)
	printThreadMap(rs)

	fmt.Println()
	fmt.Println("  อ่านผล: ดูคอลัมน์ 'M เพิ่ม'")
	fmt.Println("    scenario 1-3  → เพิ่มนิดเดียว (ไม่กี่ตัว) เพราะ G ไม่ได้ยึด M")
	fmt.Printf("    scenario 4    → เพิ่มพุ่งเข้าใกล้ %d เพราะทุก G ยึด M ของตัวเองไว้\n", n)
	fmt.Println()
	fmt.Printf("  และตลอดทั้งโปรแกรมนี้ OS เห็นแค่ 1 process เดียว (PID %d)\n", os.Getpid())
	fmt.Println("  — goroutine ไม่เคยกลายเป็น process ไม่ว่าจะแตกกี่ตัว")
}

// multiProc สาธิตว่า "8 process" ที่งาน ingest ใช้ หน้าตาเป็นยังไงในสายตา OS
func multiProc(n int, workMS int) {
	self, err := os.Executable()
	if err != nil {
		fmt.Println("หา executable ไม่เจอ:", err)
		return
	}

	fmt.Printf("\n=== โหมด multi-process: แม่ PID=%d กำลังจะ fork ลูก %d ตัว ===\n\n", os.Getpid(), n)

	var wg sync.WaitGroup
	pids := make([]int, n)
	var mu sync.Mutex

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(part int) {
			defer wg.Done()
			cmd := exec.Command(self, "-mode=child", "-part="+strconv.Itoa(part), "-ms="+strconv.Itoa(workMS))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				fmt.Println("start ลูกไม่สำเร็จ:", err)
				return
			}
			mu.Lock()
			pids[part] = cmd.Process.Pid
			mu.Unlock()
			_ = cmd.Wait()
		}(i)
	}

	// รอให้ลูกเกิดครบก่อนไปนับ
	time.Sleep(time.Duration(workMS/2) * time.Millisecond)

	strPids := make([]string, 0, n)
	for _, p := range pids {
		if p > 0 {
			strPids = append(strPids, strconv.Itoa(p))
		}
	}
	fmt.Printf("\n--- OS เห็นอะไร (ps) : %d process ---\n", len(strPids))
	out, _ := exec.Command("ps", "-o", "pid,ppid,%cpu,comm", "-p", strings.Join(strPids, ",")).CombinedOutput()
	fmt.Print(string(out))

	wg.Wait()
	fmt.Printf("\nลูกทั้ง %d ตัวจบแล้ว — แต่ละตัวมี heap / GC / GOMAXPROCS / M เป็นของตัวเอง\n", n)
	fmt.Println("นี่คือรูปแบบที่งาน ingest ใช้: 8 process แยกกัน ไม่ใช่ 8 goroutine")
}

func child(part, workMS int) {
	// ลูกแต่ละตัวมี runtime ของตัวเองครบชุด
	fmt.Printf("  [child part=%d] PID=%d  GOMAXPROCS=%d  M(created)=%d\n",
		part, os.Getpid(), runtime.GOMAXPROCS(0), threadsCreated())
	callCgo(workMS) // จำลองการยิง YottaDB ผ่าน cgo
}

func main() {
	mode := flag.String("mode", "scenarios", "scenarios | multiproc | child")
	n := flag.Int("n", 200, "จำนวน goroutine (หรือจำนวน process ในโหมด multiproc)")
	ms := flag.Int("ms", 200, "ความนานของงานแต่ละชิ้น (ms)")
	part := flag.Int("part", 0, "เลข part (ใช้ภายในโหมด child)")
	flag.Parse()

	switch *mode {
	case "child":
		child(*part, *ms)
	case "multiproc":
		multiProc(*n, *ms)
	default:
		scenarios(*n, *ms)
	}
}
