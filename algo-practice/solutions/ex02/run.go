package run

// LongestDistinctRun returns the length of the longest contiguous run of
// orders in which no restaurant ID appears twice.
func LongestDistinctRun(ids []string) int {
	lastSeen := make(map[string]int) // ร้าน → index ล่าสุดที่เจอ
	best, left := 0, 0
	for right, id := range ids {
		if prev, ok := lastSeen[id]; ok && prev >= left {
			left = prev + 1 // ร้านนี้ซ้ำในหน้าต่าง → เลื่อนขอบซ้ายข้ามตัวเก่าไป
		}
		lastSeen[id] = right
		best = max(best, right-left+1)
	}
	return best
}
