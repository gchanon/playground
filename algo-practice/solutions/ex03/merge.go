package merge

import (
	"slices"
	"sort"
)

// Interval is a half-open range [Start, End) in minutes since midnight.
type Interval struct {
	Start, End int
}

// MergeClosures merges overlapping or touching closure windows and returns
// them sorted by Start. The input slice must not be modified.
func MergeClosures(in []Interval) []Interval {
	if len(in) == 0 {
		return nil
	}
	sorted := slices.Clone(in) // เรียงบนสำเนา ไม่แตะของ caller
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Start < sorted[b].Start })

	out := []Interval{sorted[0]}
	for _, cur := range sorted[1:] {
		last := &out[len(out)-1]
		if cur.Start <= last.End { // ทับหรือชนพอดี
			last.End = max(last.End, cur.End)
		} else {
			out = append(out, cur)
		}
	}
	return out
}
