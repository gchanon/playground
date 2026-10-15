package run

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golf/playground/algo-practice/internal/timing"
)

// ids("a b c") = []string{"a", "b", "c"}
func ids(s string) []string { return strings.Fields(s) }

func TestLongestDistinctRun(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want int
	}{
		{"repeat pattern", ids("kfc mk kfc mk som kfc"), 3},
		{"all distinct", ids("a b c d e"), 5},
		{"all same", ids("x x x x"), 1},
		{"repeat inside", ids("p w w k e w"), 3},
		{"old duplicate behind the window", ids("a b b a"), 2},
		{"best run at the end", ids("a a b c d"), 4},
		{"window keeps the part after the old duplicate", ids("a b c b d e"), 4},
		{"single", ids("only"), 1},
		{"empty", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LongestDistinctRun(tt.in); got != tt.want {
				t.Fatalf("LongestDistinctRun(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestLongestDistinctRunLargeInput(t *testing.T) {
	const n, distinct = 200_000, 5_000
	in := make([]string, n)
	for k := range in {
		in[k] = "r" + strconv.Itoa(k%distinct)
	}
	timing.Within(t, 2*time.Second, func() {
		if got := LongestDistinctRun(in); got != distinct {
			t.Errorf("got %d, want %d", got, distinct)
		}
	})
}
