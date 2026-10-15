# Go cheatsheet สำหรับโจทย์ algorithm

ไวยากรณ์ที่ต้องใช้บ่อยตอนเขียนโจทย์ ถ้าต้องมานั่งนึกตอนสัมภาษณ์จะเสียเวลา (Go 1.21+)

## Slice

```go
s := make([]int, n)            // ยาว n ค่าเริ่มต้นเป็น 0
s := make([]int, 0, n)         // ยาว 0 จองที่ไว้ n
s = append(s, x)
last := s[len(s)-1]
s = s[:len(s)-1]               // pop ท้าย (ใช้เป็น stack)
c := slices.Clone(s)           // สำเนา ไม่แชร์ array กับตัวเดิม
slices.Reverse(s)
slices.Equal(a, b)
i, found := slices.BinarySearch(sorted, x)
```

**ระวัง:** `b := a[1:3]` แชร์ array กับ `a` แก้ `b` แล้ว `a` เปลี่ยนด้วย

## เรียง

```go
slices.Sort(nums)                                   // เลข/สตริง เรียงน้อยไปมาก
sort.Slice(xs, func(i, j int) bool { return xs[i].Start < xs[j].Start })
slices.SortFunc(xs, func(a, b T) int { return cmp.Compare(a.Start, b.Start) })
```

เรียงหลายเงื่อนไข (มากไปน้อย แล้วชื่อ A → Z):

```go
sort.Slice(xs, func(i, j int) bool {
    if xs[i].Count != xs[j].Count {
        return xs[i].Count > xs[j].Count
    }
    return xs[i].Name < xs[j].Name
})
```

## Map

```go
m := make(map[string]int)
m[k]++                          // key ที่ไม่มีอ่านได้ 0 เลยใช้นับได้ทันที
v, ok := m[k]                   // แยก "ไม่มี" กับ "มีค่าเป็น 0"
delete(m, k)
seen := map[int]bool{}          // ใช้เป็น set
```

**ระวัง:** ลำดับการวน `range m` สุ่มทุกครั้ง ถ้าผลต้องเรียง ให้เรียงเอง

## Stack / Queue

```go
stack = append(stack, x)                         // push
top := stack[len(stack)-1]; stack = stack[:len(stack)-1]   // pop

queue := []T{start}
for head := 0; head < len(queue); head++ {       // BFS: เดินด้วยตัวชี้ ไม่ต้องตัดหัว slice
    cur := queue[head]
    queue = append(queue, next)
}
```

## Grid

```go
rows, cols := len(grid), len(grid[0])
dirs := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
nr, nc := r+d[0], c+d[1]
if nr < 0 || nr >= rows || nc < 0 || nc >= cols { continue }

seen := make([][]bool, rows)
for r := range seen { seen[r] = make([]bool, cols) }

grid[r][c] == '#'               // string ดัชนีได้ byte เทียบกับ rune literal ได้เลยถ้าเป็น ASCII
```

## Heap (`container/heap`)

```go
type IntHeap []int // min-heap

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] } // > = max-heap
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
    old := *h
    x := old[len(old)-1]
    *h = old[:len(old)-1]
    return x
}

h := &IntHeap{}
heap.Push(h, 5)            // ต้องเรียกผ่าน heap.Push ไม่ใช่ h.Push
top := (*h)[0]             // ดูตัวบนสุดโดยไม่เอาออก
x := heap.Pop(h).(int)
```

## Linked list (`container/list`)

```go
l := list.New()
e := l.PushFront(v)        // คืน *list.Element เก็บไว้ใน map ได้
l.MoveToFront(e)
back := l.Back()
l.Remove(back)
v := e.Value.(T)           // Value เป็น any ต้อง type assert
```

## ตัวเลข / สตริง

```go
max(a, b), min(a, b)       // built-in ตั้งแต่ Go 1.21
math.MaxInt, math.MinInt
strconv.Itoa(n); n, err := strconv.Atoi(s)
var b strings.Builder; b.WriteString(s); b.String()
for i, r := range s { }    // r เป็น rune (ตัวอักษร) ส่วน s[i] เป็น byte
```
