package pair

// PairForBudget returns indexes i < j such that prices[i]+prices[j] == budget.
// ok is false when no such pair exists.
func PairForBudget(prices []int, budget int) (i, j int, ok bool) {
	seen := make(map[int]int, len(prices)) // ราคา → index ที่เคยเห็น
	for j, p := range prices {
		if i, found := seen[budget-p]; found {
			return i, j, true
		}
		seen[p] = j
	}
	return 0, 0, false
}
