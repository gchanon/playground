// bench — ตัววัดกลางของ phpdemo: ยิงโหลดใส่ server ตัวไหนก็ได้ แล้วรายงานผลแบบเดียวกัน
//
// ใช้ตัวนี้ยิงทั้ง srv.php (PHP) และ gosrv (Go) ด้วยโหลดชุดเดียวกันเป๊ะ
// ตัวเลขที่ออกมาถึงเทียบกันได้จริง ไม่ใช่เทียบคนละเครื่องมือ
//
//	go run ./phpdemo/bench -url http://127.0.0.1:8080/ping -c 50 -d 3s
//
// -uniq ใช้ตอนอยากรู้ว่า "คำตอบที่ได้มีกี่ค่าที่ไม่ซ้ำกัน"
// เช่น /db คืน pid ของ backend ฝั่ง PG — นับค่าไม่ซ้ำ = ใช้ connection ไปกี่เส้น
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/ping", "endpoint ที่จะยิง")
	c := flag.Int("c", 10, "จำนวน request ที่ยิงพร้อมกัน (concurrency)")
	d := flag.Duration("d", 3*time.Second, "ยิงนานเท่าไหร่")
	n := flag.Int("n", 0, "ยิงกี่ request (ถ้าตั้ง จะใช้แทน -d)")
	label := flag.String("label", "", "ชื่อที่จะโชว์ในผล")
	uniq := flag.Bool("uniq", false, "นับค่าคำตอบที่ไม่ซ้ำกัน (ใช้กับ /db เพื่อนับ connection)")
	warmup := flag.Int("warmup", 20, "ยิงอุ่นเครื่องกี่ครั้งก่อนเริ่มจับเวลา")
	flag.Parse()

	// client ตัวเดียวใช้ร่วมกันทุก worker — มี connection pool ในตัว
	// MaxConnsPerHost ต้องกว้างพอ ไม่งั้นจะวัด "คิวของ client" แทน "คิวของ server"
	tr := &http.Transport{
		MaxIdleConns:        *c * 2,
		MaxIdleConnsPerHost: *c * 2,
		MaxConnsPerHost:     *c * 2,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{Transport: tr, Timeout: 60 * time.Second}

	for i := 0; i < *warmup; i++ {
		_, _ = fetch(client, *url)
	}

	var (
		mu      sync.Mutex
		lats    []time.Duration
		bodies  = map[string]int{}
		okCnt   atomic.Int64
		errCnt  atomic.Int64
		errMsg  atomic.Value // error ข้อความแรกที่เจอ — benchmark ที่ซ่อน error คือ benchmark ที่เชื่อไม่ได้
		stopAt  = time.Now().Add(*d)
		budget  atomic.Int64
		useBudg = *n > 0
	)
	budget.Store(int64(*n))

	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < *c; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]time.Duration, 0, 1024)
			localBodies := map[string]int{}
			for {
				if useBudg {
					if budget.Add(-1) < 0 {
						break
					}
				} else if time.Now().After(stopAt) {
					break
				}

				t0 := time.Now()
				body, err := fetch(client, *url)
				el := time.Since(t0)

				if err != nil {
					errCnt.Add(1)
					errMsg.CompareAndSwap(nil, err.Error())
					continue
				}
				okCnt.Add(1)
				local = append(local, el)
				if *uniq {
					localBodies[strings.TrimSpace(body)]++
				}
			}
			mu.Lock()
			lats = append(lats, local...)
			for k, v := range localBodies {
				bodies[k] += v
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	if len(lats) == 0 {
		fmt.Fprintf(os.Stderr, "!! ไม่มี request สำเร็จเลย (error %d ครั้ง) — server ขึ้นรึยัง: %s\n",
			errCnt.Load(), *url)
		os.Exit(1)
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })

	name := *label
	if name == "" {
		name = *url
	}
	fmt.Printf("%-22s c=%-4d  %8.0f req/s │ p50 %8s │ p95 %8s │ p99 %8s │ err %d",
		name, *c, float64(okCnt.Load())/elapsed.Seconds(),
		pct(lats, 0.50).Round(10*time.Microsecond),
		pct(lats, 0.95).Round(10*time.Microsecond),
		pct(lats, 0.99).Round(10*time.Microsecond),
		errCnt.Load())
	if *uniq {
		fmt.Printf(" │ คำตอบไม่ซ้ำ %d ค่า", len(bodies))
	}
	fmt.Println()
	if m, ok := errMsg.Load().(string); ok {
		if len(m) > 160 {
			m = m[:160] + "..."
		}
		fmt.Printf("%-22s   error แรกที่เจอ: %s\n", "", m)
	}
}

func fetch(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	return string(b), nil
}

// pct คืนค่า percentile จาก slice ที่เรียงแล้ว
func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(float64(len(sorted)-1) * p)
	return sorted[i]
}
