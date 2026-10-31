package routes

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
)

func newTestRateLimiter() *RateLimiter {
	return NewRateLimiter(&gitus.GitusConfig{MaxRequestInSecond: 1000})
}

func TestRateLimiterReusesAndEvicts(t *testing.T) {
	rl := newTestRateLimiter()
	rl.idleTTL = 20 * time.Millisecond
	rl.sweepEvery = 1

	rl.IsIPAllowed("10.0.0.1")
	rl.mutex.RLock()
	first := rl.limiter["10.0.0.1"]
	rl.mutex.RUnlock()
	rl.IsIPAllowed("10.0.0.1")
	rl.mutex.RLock()
	same := rl.limiter["10.0.0.1"]
	rl.mutex.RUnlock()
	if same != first { t.Fatal("the same client should reuse its limiter") }

	time.Sleep(40 * time.Millisecond)
	rl.IsIPAllowed("10.0.0.2") // this insertion also triggers the sweep
	rl.mutex.RLock()
	_, stale := rl.limiter["10.0.0.1"]
	_, fresh := rl.limiter["10.0.0.2"]
	rl.mutex.RUnlock()
	if stale { t.Fatal("the idle limiter was not evicted") }
	if !fresh { t.Fatal("the active limiter was evicted") }
}

func TestRateLimiterConcurrent(t *testing.T) {
	rl := newTestRateLimiter()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				rl.IsIPAllowed(fmt.Sprintf("10.0.%d.%d", i, j%32))
			}
		}(i)
	}
	wg.Wait()
	rl.mutex.RLock()
	n := len(rl.limiter)
	rl.mutex.RUnlock()
	if n == 0 { t.Fatal("no limiter was created") }
}
