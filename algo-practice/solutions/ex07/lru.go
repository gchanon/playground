package lru

import "container/list"

type entry struct {
	key   string
	value int
}

// Cache keeps at most capacity quote prices. When it is full, adding a new
// key evicts the key that was used least recently. Both Get and Put count as
// a use.
type Cache struct {
	capacity int
	order    *list.List               // หน้าสุด = ใช้ล่าสุด, ท้ายสุด = เก่าสุด
	items    map[string]*list.Element // key → ตำแหน่งในแถว
}

// New returns an empty cache that holds at most capacity keys (capacity >= 1).
func New(capacity int) *Cache {
	return &Cache{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

// Get returns the value for key and marks key as most recently used.
func (c *Cache) Get(key string) (int, bool) {
	el, ok := c.items[key]
	if !ok {
		return 0, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*entry).value, true
}

// Put sets the value for key and marks key as most recently used.
func (c *Cache) Put(key string, value int) {
	if el, ok := c.items[key]; ok {
		el.Value.(*entry).value = value
		c.order.MoveToFront(el)
		return
	}
	if c.order.Len() == c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*entry).key) // ต้องลบจาก map ด้วย จึงต้องเก็บ key ไว้ใน entry
	}
	c.items[key] = c.order.PushFront(&entry{key, value})
}

// Len returns the number of keys in the cache.
func (c *Cache) Len() int {
	return c.order.Len()
}
