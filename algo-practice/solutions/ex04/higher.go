package higher

// DaysUntilHigher returns, for each day, how many days the customer must wait
// for a strictly higher premium. A day with no higher premium later gets 0.
func DaysUntilHigher(premiums []int) []int {
	result := make([]int, len(premiums))
	waiting := []int{} // index ของวันที่ยังไม่เจอวันแพงกว่า ราคาจากล่างขึ้นบนไม่เพิ่มขึ้น
	for today, price := range premiums {
		for len(waiting) > 0 && premiums[waiting[len(waiting)-1]] < price {
			day := waiting[len(waiting)-1]
			waiting = waiting[:len(waiting)-1]
			result[day] = today - day
		}
		waiting = append(waiting, today)
	}
	return result // วันที่ยังค้างใน waiting ไม่มีวันแพงกว่า = 0 อยู่แล้ว
}
