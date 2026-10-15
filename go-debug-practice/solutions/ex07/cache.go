package cache

import (
	"sync"
	"time"
)

type entry struct {
	value     []byte
	expiresAt time.Time
	hits      int
}

type Stats struct {
	Hits      int
	Misses    int
	Expired   int
	Evictions int
}

type Cache struct {
	mu    sync.RWMutex
	items map[string]*entry
	stats map[string]int

	ttl      time.Duration
	maxItems int

	stop chan struct{}
	once sync.Once
}

func New(ttl time.Duration, maxItems int) *Cache {
	c := &Cache{
		items:    make(map[string]*entry),
		stats:    make(map[string]int),
		ttl:      ttl,
		maxItems: maxItems,
		stop:     make(chan struct{}),
	}
	go c.janitor(ttl / 2)
	return c
}

func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[key]
	if !ok {
		c.stats["miss"]++
		return nil, false
	}

	if time.Now().After(e.expiresAt) {
		delete(c.items, key)
		c.stats["expired"]++
		c.stats["miss"]++
		return nil, false
	}

	e.hits++
	c.stats["hit"]++
	return e.value, true
}

func (c *Cache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.items) >= c.maxItems {
		if _, exists := c.items[key]; !exists {
			c.evictOldestLocked()
		}
	}

	c.items[key] = &entry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time

	for k, e := range c.items {
		if oldestKey == "" || e.expiresAt.Before(oldest) {
			oldestKey, oldest = k, e.expiresAt
		}
	}
	if oldestKey != "" {
		delete(c.items, oldestKey)
		c.stats["evict"]++
	}
}

func (c *Cache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Stats{
		Hits:      c.stats["hit"],
		Misses:    c.stats["miss"],
		Expired:   c.stats["expired"],
		Evictions: c.stats["evict"],
	}
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *Cache) Close() {
	c.once.Do(func() { close(c.stop) })
}

func (c *Cache) janitor(every time.Duration) {
	if every <= 0 {
		every = time.Second
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case now := <-ticker.C:
			c.mu.Lock()
			for k, e := range c.items {
				if now.After(e.expiresAt) {
					delete(c.items, k)
					c.stats["expired"]++
				}
			}
			c.mu.Unlock()
		}
	}
}
