package lru

// Cache keeps at most capacity quote prices. When it is full, adding a new
// key evicts the key that was used least recently. Both Get and Put count as
// a use.
type Cache struct {
	// TODO
}

// New returns an empty cache that holds at most capacity keys (capacity >= 1).
func New(capacity int) *Cache {
	// TODO
	return &Cache{}
}

// Get returns the value for key and marks key as most recently used.
func (c *Cache) Get(key string) (int, bool) {
	// TODO
	return 0, false
}

// Put sets the value for key and marks key as most recently used.
func (c *Cache) Put(key string, value int) {
	// TODO
}

// Len returns the number of keys in the cache.
func (c *Cache) Len() int {
	// TODO
	return 0
}
