package path

// ShortestPath returns the minimum number of moves from 'S' to 'E' on the
// grid, moving up, down, left or right through '.' cells and never through
// '#'. It returns -1 when 'E' cannot be reached.
func ShortestPath(grid []string) int {
	rows := len(grid)
	if rows == 0 {
		return -1
	}
	cols := len(grid[0])

	type cell struct{ r, c int }
	dist := make([][]int, rows) // -1 = ยังไม่เคยไปถึง
	var start cell
	for r := range dist {
		dist[r] = make([]int, cols)
		for c := range dist[r] {
			dist[r][c] = -1
			if grid[r][c] == 'S' {
				start = cell{r, c}
			}
		}
	}

	queue := []cell{start}
	dist[start.r][start.c] = 0
	moves := []cell{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for head := 0; head < len(queue); head++ {
		cur := queue[head]
		if grid[cur.r][cur.c] == 'E' {
			return dist[cur.r][cur.c]
		}
		for _, m := range moves {
			nr, nc := cur.r+m.r, cur.c+m.c
			if nr < 0 || nr >= rows || nc < 0 || nc >= cols {
				continue
			}
			if grid[nr][nc] == '#' || dist[nr][nc] != -1 {
				continue
			}
			dist[nr][nc] = dist[cur.r][cur.c] + 1 // ทำเครื่องหมายตอนใส่คิว ไม่ใช่ตอนหยิบออก
			queue = append(queue, cell{nr, nc})
		}
	}
	return -1
}
