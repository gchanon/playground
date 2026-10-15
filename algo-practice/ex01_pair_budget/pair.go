package pair

// PairForBudget returns indexes i < j such that prices[i]+prices[j] == budget.
// ok is false when no such pair exists.
func PairForBudget(prices []int, budget int) (i, j int, ok bool) {

	seenMap := make(map[int]int)

	for i, priceData := range prices {

		rest := budget - priceData

		if indexSeen, ok := seenMap[rest]; ok {
			return indexSeen, i, true
		}
		seenMap[priceData] = i

	}

	return 0, 0, false
}
