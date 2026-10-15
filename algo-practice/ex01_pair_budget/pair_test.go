package pair

import (
	"testing"
	"time"

	"github.com/golf/playground/algo-practice/internal/timing"
)

func TestPairForBudget(t *testing.T) {
	tests := []struct {
		name   string
		prices []int
		budget int
		wantOK bool
	}{
		{"basic", []int{120, 70, 250, 30}, 100, true},
		{"pair at the end", []int{5, 9, 14, 40, 60}, 100, true},
		{"same price twice", []int{50, 10, 50}, 100, true},
		{"zero price", []int{0, 80, 15}, 80, true},
		{"no pair", []int{10, 20, 30}, 100, false},
		{"one item cannot pair with itself", []int{50}, 100, false},
		{"empty", nil, 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i, j, ok := PairForBudget(tt.prices, tt.budget)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if i < 0 || j >= len(tt.prices) || i >= j {
				t.Fatalf("got (%d, %d), want 0 <= i < j < %d", i, j, len(tt.prices))
			}
			if got := tt.prices[i] + tt.prices[j]; got != tt.budget {
				t.Fatalf("prices[%d]+prices[%d] = %d, want %d", i, j, got, tt.budget)
			}
		})
	}
}

func TestPairForBudgetLargeInput(t *testing.T) {
	const n = 200_000
	prices := make([]int, n)
	for k := range prices {
		prices[k] = 2 * k // ทุกตัวเป็นเลขคู่ คู่เดียวที่ได้คือสองตัวสุดท้าย
	}
	prices[n-1] = 3
	budget := prices[n-2] + 3
	timing.Within(t, 2*time.Second, func() {
		i, j, ok := PairForBudget(prices, budget)
		if !ok || i != n-2 || j != n-1 {
			t.Errorf("got (%d, %d, %v), want (%d, %d, true)", i, j, ok, n-2, n-1)
		}
	})
}
