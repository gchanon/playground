package lru

import (
	"strconv"
	"testing"
	"time"

	"github.com/golf/playground/algo-practice/internal/timing"
)

func mustGet(t *testing.T, c *Cache, key string, want int) {
	t.Helper()
	got, ok := c.Get(key)
	if !ok || got != want {
		t.Fatalf("Get(%q) = (%d, %v), want (%d, true)", key, got, ok, want)
	}
}

func mustMiss(t *testing.T, c *Cache, key string) {
	t.Helper()
	if got, ok := c.Get(key); ok {
		t.Fatalf("Get(%q) = (%d, true), want a miss", key, got)
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New(2)
	c.Put("a", 1)
	c.Put("b", 2)
	mustGet(t, c, "a", 1) // a ถูกใช้ล่าสุด b จึงเก่าสุด
	c.Put("c", 3)         // เต็ม → ต้องไล่ b
	mustMiss(t, c, "b")
	mustGet(t, c, "a", 1)
	mustGet(t, c, "c", 3)
	if c.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", c.Len())
	}
}

func TestPutExistingKeyUpdatesAndRefreshes(t *testing.T) {
	c := New(2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("a", 10) // แก้ค่า และ a กลายเป็นล่าสุด
	if c.Len() != 2 {
		t.Fatalf("Len() = %d after updating an existing key, want 2", c.Len())
	}
	c.Put("c", 3) // ต้องไล่ b ไม่ใช่ a
	mustMiss(t, c, "b")
	mustGet(t, c, "a", 10)
	mustGet(t, c, "c", 3)
}

func TestCapacityOne(t *testing.T) {
	c := New(1)
	c.Put("a", 1)
	c.Put("b", 2)
	mustMiss(t, c, "a")
	mustGet(t, c, "b", 2)
	c.Put("b", 3)
	mustGet(t, c, "b", 3)
	if c.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", c.Len())
	}
}

func TestMissDoesNotChangeAnything(t *testing.T) {
	c := New(2)
	c.Put("a", 1)
	c.Put("b", 2)
	mustMiss(t, c, "zzz")
	c.Put("c", 3) // a ยังเก่าสุด ต้องถูกไล่
	mustMiss(t, c, "a")
	mustGet(t, c, "b", 2)
}

func TestManyOperations(t *testing.T) {
	const capacity, ops = 50_000, 500_000
	timing.Within(t, 3*time.Second, func() {
		c := New(capacity)
		for i := 0; i < ops; i++ {
			c.Put(strconv.Itoa(i), i)
			if i >= 10 {
				c.Get(strconv.Itoa(i - 10))
			}
		}
		if c.Len() != capacity {
			t.Errorf("Len() = %d, want %d", c.Len(), capacity)
			return
		}
		if v, ok := c.Get(strconv.Itoa(ops - 1)); !ok || v != ops-1 {
			t.Errorf("newest key missing")
		}
		if _, ok := c.Get(strconv.Itoa(ops - capacity - 1)); ok {
			t.Errorf("a key that should have been evicted is still there")
		}
	})
}
