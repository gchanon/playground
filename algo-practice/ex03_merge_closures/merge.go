package merge

import (
	"container/heap"
	"slices"
	"sort"
)

// Interval is a half-open range [Start, End) in minutes since midnight.
type Interval struct {
	Start int
	End   int
}

// MergeClosures merges overlapping or touching closure windows and returns
// them sorted by Start. The input slice must not be modified.
func MergeClosures(in []Interval) []Interval {
	if len(in) == 0 {
		return nil
	}

	//[]Interval{{0, 600}, {60, 120}, {300, 310}}
	sorted := make([]Interval, len(in))
	copy(sorted, in)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start < sorted[j].Start
	})

	result := []Interval{
		sorted[0],
	}

	for _, intervalData := range sorted[1:] {
		if intervalData.Start <= result[len(result)-1].End {
			if intervalData.End > result[len(result)-1].End {
				result[len(result)-1].End = intervalData.End
			}
		} else {
			result = append(result, intervalData)
		}
	}
	return result
}

func MinRidersSweep(jobs []Interval) int {

	// []Interval{{0, 100}, {10, 20}, {30, 40}}
	starts := make([]int, len(jobs))
	ends := make([]int, len(jobs))
	for i, job := range jobs {
		starts[i] = job.Start
		ends[i] = job.End
	}
	slices.Sort(starts)
	slices.Sort(ends)

	working, bestRiders := 0, 0
	e := 0
	for _, start := range starts {
		for ends[e] <= start { // งานที่ส่งเสร็จแล้ว ณ นาทีนี้ (ชนพอดีก็นับว่าว่าง)
			working--
			e++
		}
		working++
		bestRiders = max(bestRiders, working)
	}
	return bestRiders
}

type endTimes []int

func (h endTimes) Len() int           { return len(h) }
func (h endTimes) Less(i, j int) bool { return h[i] < h[j] } // เสร็จเร็วกว่าอยู่ใกล้หัวแถวกว่า
func (h endTimes) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *endTimes) Push(x any)        { *h = append(*h, x.(int)) }
func (h *endTimes) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
func MinRiders(jobs []Interval) int {
	sorted := make([]Interval, len(jobs))
	copy(sorted, jobs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start < sorted[j].Start
	})

	onDuty := &endTimes{} // min-heap: เวลาส่งเสร็จของไรเดอร์ทุกคน
	bestRiders := 0
	for _, job := range sorted {
		if onDuty.Len() > 0 && (*onDuty)[0] <= job.Start { // คนที่เสร็จเร็วสุดว่างแล้ว
			heap.Pop(onDuty) // ให้คนนั้นรับงานนี้
		}
		heap.Push(onDuty, job.End) // จดเวลาเสร็จของงานใหม่
		bestRiders = max(bestRiders, onDuty.Len())
	}
	return bestRiders
}
