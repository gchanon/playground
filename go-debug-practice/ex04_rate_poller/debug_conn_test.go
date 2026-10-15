package fx

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptrace"
	"strings"
	"testing"
)

// TestDebugConnectionReuse ไม่ได้ทดสอบอะไร — มันพิมพ์ให้ดูว่าแต่ละ request
// ได้ connection ใหม่หรือใช้ของเดิมซ้ำ ไฟล์นี้ลบทิ้งได้ตลอด
//
//	go test -v -run TestDebugConnectionReuse ./ex04_rate_poller/
func TestDebugConnectionReuse(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("รันด้วย -v เพื่อดูผล: go test -v -run TestDebugConnectionReuse ./ex04_rate_poller/")
	}

	srv, counter := newRateServer(t)
	p := NewPoller(srv.URL)

	// 8 คู่ สลับมีราคา/ไม่มีราคา เหมือนในเทสจริง
	pairs := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		pair := fmt.Sprintf("USDTHB%02d", i)
		if i%2 == 1 {
			pair += "X" // provider ไม่มีราคาคู่นี้
		}
		pairs = append(pairs, pair)
	}

	fmt.Println()
	fmt.Printf("%-4s %-11s %-15s %-22s %-18s %s\n",
		"req", "คู่เงิน", "ผลลัพธ์", "สายที่ใช้ (local addr)", "สถานะสาย", "สายรวม")
	fmt.Println(strings.Repeat("─", 96))

	for i, pair := range pairs {
		var localAddr string
		var reused bool

		// ดักเหตุการณ์ฝั่ง client: GotConn ถูกเรียกตอนได้ connection มาใช้
		trace := &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) {
				localAddr = info.Conn.LocalAddr().String()
				reused = info.Reused // true = หยิบจาก pool, false = เพิ่งเปิดใหม่
			},
		}
		ctx := httptrace.WithClientTrace(context.Background(), trace)

		// เรียกโค้ดจริงใน poller.go ไม่ได้แก้อะไรเลย
		_, err := p.Rate(ctx, pair)

		result := "200 มีราคา"
		switch {
		case errors.Is(err, ErrPairNotQuoted):
			result = "404 ไม่มีราคา"
		case err != nil:
			result = "error: " + err.Error()
		}

		state := "*** เปิดสายใหม่ ***"
		if reused {
			state = "ใช้สายเดิมซ้ำ"
		}

		fmt.Printf("%-4d %-11s %-15s %-22s %-18s %d\n",
			i+1, pair, result, localAddr, state, counter.count())
	}

	fmt.Println(strings.Repeat("─", 96))
	fmt.Printf("ยิงไป %d requests -> เซิร์ฟเวอร์เห็น connection ใหม่ทั้งหมด %d เส้น\n\n",
		len(pairs), counter.count())
}
