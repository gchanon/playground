package run

// LongestDistinctRun returns the length of the longest contiguous run of
// orders in which no restaurant ID appears twice.
func LongestDistinctRun(ids []string) int {

	//"a b c b d e"
	bestDist := 0
	normalDist := 0
	seenMap := make(map[string]int)

	for index, shopData := range ids {

		lastSeenIndex, found := seenMap[shopData]

		if found {

			if normalDist >= (index - lastSeenIndex) {
				normalDist = index - lastSeenIndex
			} else {
				normalDist++
			}

		} else {
			normalDist++
		}

		bestDist = max(bestDist, normalDist)

		seenMap[shopData] = index

		if len(seenMap) > 2*normalDist { // map ใหญ่เกินหน้าต่าง 2 เท่า → กวาดทีนึง -> ต้องใช้ 2x เพราะจะได้ไม่ต้องกวาดทุกรอบของ loop นานๆกวาดออกทีนึง
			left := index - normalDist + 1
			for shop, lastSeenIndex := range seenMap {
				if lastSeenIndex < left {
					delete(seenMap, shop)
				}
			}
		}

	}

	return bestDist
}
