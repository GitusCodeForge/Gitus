package tcache

import (
	"fmt"
	"testing"
	"time"
)

func TestExpiryIsSafeWithConcurrentAccess(t *testing.T) {
	c := NewTCache(time.Minute)
	for i := 0; i < 200; i++ {
		c.Register(fmt.Sprintf("k%d", i), "v", time.Millisecond)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		for i := 0; i < 200; i++ {
			c.Get(fmt.Sprintf("k%d", i))
			if i%20 == 0 { c.Register(fmt.Sprintf("k%d", i), "w", time.Millisecond) }
		}
	}
	time.Sleep(20 * time.Millisecond)
	for i := 0; i < 200; i++ {
		if _, ok := c.Get(fmt.Sprintf("k%d", i)); ok {
			t.Fatalf("entry k%d should have expired", i)
		}
	}
	// Deleting a key that is already gone must not panic.
	c.Delete("k0")
}

func TestDeleteMissingKey(t *testing.T) {
	c := NewTCache(time.Minute)
	c.Delete("nope")
	c.Register("a", "1", -1)
	c.Delete("a")
	if _, ok := c.Get("a"); ok { t.Fatal("Delete did not remove the entry") }
}

func TestNoExpiry(t *testing.T) {
	c := NewTCache(time.Minute)
	c.Register("a", "1", -1)
	time.Sleep(20 * time.Millisecond)
	if v, ok := c.Get("a"); !ok || v != "1" { t.Fatalf("entry without TTL disappeared: %q %v", v, ok) }
}

func TestReRegisterSkipsTheOldExpiry(t *testing.T) {
	c := NewTCache(time.Minute)
	c.Register("a", "old", 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	c.Register("a", "new", time.Hour)
	time.Sleep(20 * time.Millisecond)
	if v, ok := c.Get("a"); !ok || v != "new" {
		t.Fatalf("an old expiry removed the new value: %q %v", v, ok)
	}
}

func TestDeleteThenReRegister(t *testing.T) {
	c := NewTCache(time.Minute)
	c.Register("a", "old", 5*time.Millisecond)
	c.Delete("a")
	c.Register("a", "new", time.Hour)
	time.Sleep(20 * time.Millisecond)
	if v, ok := c.Get("a"); !ok || v != "new" {
		t.Fatalf("a stale timer removed the new value: %q %v", v, ok)
	}
}

func TestExtendAndShortenTTL(t *testing.T) {
	c := NewTCache(time.Minute)
	c.Register("a", "1", time.Hour)
	c.Register("a", "2", 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get("a"); ok { t.Fatal("shortened TTL did not expire") }

	c.Register("b", "1", 5*time.Millisecond)
	c.Register("b", "2", time.Hour)
	time.Sleep(20 * time.Millisecond)
	if v, ok := c.Get("b"); !ok || v != "2" { t.Fatalf("extended TTL expired: %q %v", v, ok) }
}

func TestReRegisterDoesNotGrowTheHeapForever(t *testing.T) {
	c := NewTCache(time.Minute)
	for i := 0; i < 1000; i++ {
		c.Register("a", "v", time.Hour)
	}
	time.Sleep(20 * time.Millisecond)
	c.lock.Lock()
	n := len(c.expiries)
	c.lock.Unlock()
	if n > 32 { t.Fatalf("heap kept %d entries for one live key", n) }
	if v, ok := c.Get("a"); !ok || v != "v" { t.Fatalf("entry lost during compaction: %q %v", v, ok) }
}
