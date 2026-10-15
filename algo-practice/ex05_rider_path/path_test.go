package path

import (
	"strings"
	"testing"
	"time"

	"github.com/golf/playground/algo-practice/internal/timing"
)

func TestShortestPath(t *testing.T) {
	tests := []struct {
		name string
		grid []string
		want int
	}{
		{"straight line", []string{
			"S..E",
		}, 3},
		{"next to each other", []string{
			"SE",
		}, 1},
		{"around a wall", []string{
			"S.#.",
			"..#E",
			"....",
		}, 6},
		{"short way is right, long way is down", []string{
			"S....",
			".###.",
			".#E..",
			".###.",
			".....",
		}, 8},
		{"short way is down, long way is right", []string{
			"S....",
			".###.",
			".#E#.",
			".#.#.",
			".....",
		}, 8},
		{"walled off", []string{
			"S.#..",
			"..#.E",
			"###..",
		}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortestPath(tt.grid); got != tt.want {
				t.Fatalf("ShortestPath() = %d, want %d\n%s", got, tt.want, strings.Join(tt.grid, "\n"))
			}
		})
	}
}

func TestShortestPathLargeOpenGrid(t *testing.T) {
	const n = 1000
	grid := make([]string, n)
	row := strings.Repeat(".", n)
	for r := range grid {
		grid[r] = row
	}
	grid[0] = "S" + row[1:]
	grid[n-1] = row[:n-1] + "E"
	timing.Within(t, 3*time.Second, func() {
		if got := ShortestPath(grid); got != 2*(n-1) {
			t.Errorf("got %d, want %d", got, 2*(n-1))
		}
	})
}
