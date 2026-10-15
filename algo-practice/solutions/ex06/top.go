package top

import "container/heap"

type entry struct {
	name  string
	count int
}

// worse reports whether a ranks below b: fewer orders, or same orders and a
// later name.
func worse(a, b entry) bool {
	if a.count != b.count {
		return a.count < b.count
	}
	return a.name > b.name
}

// minHeap keeps the worst of the current top k at index 0.
type minHeap []entry

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return worse(h[i], h[j]) }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(entry)) }
func (h *minHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

// TopKRestaurants returns the k most ordered restaurants, most orders first.
// Restaurants with the same number of orders are ordered by name, A to Z.
// If there are fewer than k restaurants, all of them are returned.
func TopKRestaurants(orders []string, k int) []string {
	if k <= 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, name := range orders {
		counts[name]++
	}

	h := &minHeap{}
	for name, count := range counts {
		heap.Push(h, entry{name, count})
		if h.Len() > k {
			heap.Pop(h) // ทิ้งตัวที่แย่ที่สุด เหลือ k ตัวที่ดีที่สุดเสมอ
		}
	}

	out := make([]string, h.Len())
	for i := len(out) - 1; i >= 0; i-- { // Pop ได้ตัวแย่สุดก่อน จึงเติมจากท้าย
		out[i] = heap.Pop(h).(entry).name
	}
	return out
}
