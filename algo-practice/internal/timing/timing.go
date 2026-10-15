// Package timing helps tests fail fast when a solution has the wrong complexity.
package timing

import (
	"testing"
	"time"
)

// Within runs f and fails the test if it does not finish within limit.
func Within(t *testing.T, limit time.Duration, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("ช้าเกิน %v กับ input ใหญ่ — complexity น่าจะสูงกว่าที่โจทย์ต้องการ", limit)
	}
}
