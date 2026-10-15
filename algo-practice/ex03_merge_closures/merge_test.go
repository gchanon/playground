package merge

import (
	"math/rand"
	"slices"
	"testing"
	"time"

	assert "github.com/go-playground/assert/v2"
	"github.com/golf/playground/algo-practice/internal/timing"
)

func TestMergeClosures(t *testing.T) {
	tests := []struct {
		name string
		in   []Interval
		want []Interval
	}{
		{"overlap", []Interval{{60, 120}, {90, 180}}, []Interval{{60, 180}}},
		{"unsorted input", []Interval{{600, 660}, {60, 120}, {100, 200}}, []Interval{{60, 200}, {600, 660}}},
		{"touching windows merge", []Interval{{60, 120}, {120, 180}}, []Interval{{60, 180}}},
		{"gap of one minute stays apart", []Interval{{60, 120}, {121, 180}}, []Interval{{60, 120}, {121, 180}}},
		{"nested", []Interval{{0, 600}, {60, 120}, {300, 310}}, []Interval{{0, 600}}},
		{"later start, earlier end", []Interval{{0, 100}, {10, 20}, {90, 95}, {150, 200}}, []Interval{{0, 100}, {150, 200}}},
		{"single", []Interval{{5, 10}}, []Interval{{5, 10}}},
		{"empty", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeClosures(tt.in)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("MergeClosures(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMergeClosuresDoesNotModifyInput(t *testing.T) {
	in := []Interval{{600, 660}, {60, 120}, {100, 200}}
	before := slices.Clone(in)
	MergeClosures(in)
	if !slices.Equal(in, before) {
		t.Fatalf("input was modified: got %v, want %v", in, before)
	}
}

func TestMergeClosuresLargeInput(t *testing.T) {
	const n = 200_000
	r := rand.New(rand.NewSource(1))
	in := make([]Interval, n)
	for k := range in {
		in[k] = Interval{Start: 3 * k, End: 3*k + 2} // ห่างกันทุกช่วง ไม่มีอันไหนรวมกัน
	}
	r.Shuffle(n, func(a, b int) { in[a], in[b] = in[b], in[a] })
	timing.Within(t, 2*time.Second, func() {
		if got := MergeClosures(in); len(got) != n {
			t.Errorf("got %d windows, want %d", len(got), n)
		}
	})
}

func TestMergeClosuresRiders(t *testing.T) {
	tests := []struct {
		name string
		in   []Interval
		want int
	}{
		{"overlap", []Interval{{0, 100}, {10, 20}, {30, 40}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MinRidersSweep(tt.in)
			if got == 0 && tt.want == 0 {
				return
			}
			assert.Equal(t, got, tt.want)
		})
	}
}
