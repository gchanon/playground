package top

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golf/playground/algo-practice/internal/timing"
)

func orders(s string) []string { return strings.Fields(s) }

func TestTopKRestaurants(t *testing.T) {
	tests := []struct {
		name   string
		orders []string
		k      int
		want   []string
	}{
		{"basic", orders("kfc mk kfc som mk kfc"), 2, []string{"kfc", "mk"}},
		{"tie is broken by name", orders("zen bonchon zen bonchon aroi"), 2, []string{"bonchon", "zen"}},
		{"tie at the cut-off", orders("a b c d d"), 3, []string{"d", "a", "b"}},
		{"k larger than restaurants", orders("x y x"), 5, []string{"x", "y"}},
		{"k is one", orders("p q q r r r"), 1, []string{"r"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TopKRestaurants(tt.orders, tt.k); !slices.Equal(got, tt.want) {
				t.Fatalf("TopKRestaurants(%v, %d) = %v, want %v", tt.orders, tt.k, got, tt.want)
			}
		})
	}
}

func TestTopKRestaurantsEmpty(t *testing.T) {
	if got := TopKRestaurants(nil, 3); len(got) != 0 {
		t.Fatalf("no orders: got %v, want empty", got)
	}
	if got := TopKRestaurants(orders("a b"), 0); len(got) != 0 {
		t.Fatalf("k = 0: got %v, want empty", got)
	}
}

func TestTopKRestaurantsLargeInput(t *testing.T) {
	const distinct, k = 100_000, 10
	var in []string
	for r := 0; r < distinct; r++ {
		times := 5
		if r < k {
			times = 100 - r // r00000 ถูกสั่ง 100 ครั้ง, r00001 99 ครั้ง, ...
		}
		for i := 0; i < times; i++ {
			in = append(in, fmt.Sprintf("r%05d", r))
		}
	}
	rand.New(rand.NewSource(1)).Shuffle(len(in), func(a, b int) { in[a], in[b] = in[b], in[a] })
	want := make([]string, k)
	for r := range want {
		want[r] = fmt.Sprintf("r%05d", r)
	}
	timing.Within(t, 3*time.Second, func() {
		if got := TopKRestaurants(in, k); !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}
