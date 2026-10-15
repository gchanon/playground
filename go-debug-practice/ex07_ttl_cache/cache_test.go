package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCacheGetSetSingleGoroutine(t *testing.T) {
	c := New(time.Minute, 128)
	defer c.Close()

	c.Set("user:1", []byte("alice"))

	v, ok := c.Get("user:1")
	if !ok || string(v) != "alice" {
		t.Fatalf("Get(user:1) = %q, %v; want alice, true", v, ok)
	}
	if _, ok := c.Get("user:2"); ok {
		t.Fatal("Get(user:2) reported a hit for a key that was never set")
	}

	st := c.Stats()
	if st.Hits != 1 || st.Misses != 1 {
		t.Fatalf("Stats() = %+v, want 1 hit and 1 miss", st)
	}
}

func TestCacheUnderConcurrentLoad(t *testing.T) {
	c := New(50*time.Millisecond, 256)
	defer c.Close()

	const (
		readers = 8
		writers = 4
		ops     = 500
	)

	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("user:%d", i%64)
				c.Set(key, []byte(fmt.Sprintf("writer-%d-%d", w, i)))
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				c.Get(fmt.Sprintf("user:%d", i%64))
			}
		}()
	}

	wg.Wait()

	st := c.Stats()
	if st.Hits+st.Misses == 0 {
		t.Fatal("cache recorded no hits and no misses")
	}
	if c.Len() > 256 {
		t.Fatalf("cache holds %d items, above the 256 limit", c.Len())
	}
}
