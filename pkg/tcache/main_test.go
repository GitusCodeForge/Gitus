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
