package higher

import (
	"slices"
	"testing"
	"time"

	"github.com/golf/playground/algo-practice/internal/timing"
)

func TestDaysUntilHigher(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want []int
	}{
		{"mixed", []int{73, 74, 75, 71, 69, 72, 76, 73}, []int{1, 1, 4, 2, 1, 1, 0, 0}},
		{"rising", []int{1, 2, 3, 4}, []int{1, 1, 1, 0}},
		{"falling", []int{4, 3, 2, 1}, []int{0, 0, 0, 0}},
		{"equal is not higher", []int{5, 5, 5, 6}, []int{3, 2, 1, 0}},
		{"valley", []int{30, 10, 20, 40}, []int{3, 1, 1, 0}},
		{"single", []int{9}, []int{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DaysUntilHigher(tt.in); !slices.Equal(got, tt.want) {
				t.Fatalf("DaysUntilHigher(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDaysUntilHigherEmpty(t *testing.T) {
	if got := DaysUntilHigher(nil); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestDaysUntilHigherLargeInput(t *testing.T) {
	const n = 200_000
	in := make([]int, n)
	for k := range in {
		in[k] = n - k // ลดลงเรื่อยๆ แล้วพุ่งขึ้นวันสุดท้าย = กรณีแย่สุดของวิธีไล่ทีละคู่
	}
	in[n-1] = 2 * n
	timing.Within(t, 2*time.Second, func() {
		got := DaysUntilHigher(in)
		if len(got) != n || got[0] != n-1 || got[n-2] != 1 || got[n-1] != 0 {
			t.Errorf("wrong answer on large input")
		}
	})
}
