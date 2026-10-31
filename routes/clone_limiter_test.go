package routes

import "testing"

func TestCloneLimiterTryAcquire(t *testing.T) {
	l := NewCloneLimiter(2)
	r1, ok := l.TryAcquire()
	if !ok {
		t.Fatal("first acquire failed")
	}
	r2, ok := l.TryAcquire()
	if !ok {
		t.Fatal("second acquire failed")
	}
	if _, ok := l.TryAcquire(); ok {
		t.Fatal("acquired more slots than the limiter has")
	}
	r1()
	r3, ok := l.TryAcquire()
	if !ok {
		t.Fatal("slot was not released")
	}
	r2()
	r3()
	r4, ok := l.TryAcquire()
	if !ok {
		t.Fatal("release did not return the slots")
	}
	r4()
}

func TestCloneLimiterDefault(t *testing.T) {
	l := NewCloneLimiter(0)
	for i := 0; i < DefaultMaxConcurrentClone; i++ {
		if _, ok := l.TryAcquire(); !ok {
			t.Fatalf("acquire %d failed before the default limit", i+1)
		}
	}
	if _, ok := l.TryAcquire(); ok {
		t.Fatal("acquired more than DefaultMaxConcurrentClone slots")
	}
}

func TestNilCloneLimiterIsUnlimited(t *testing.T) {
	var ctx *RouterContext
	release, ok := ctx.AcquireCloneSlot()
	if !ok {
		t.Fatal("a context without a limiter refused a slot")
	}
	release()
}
