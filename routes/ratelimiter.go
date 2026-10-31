package routes

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"golang.org/x/time/rate"
)

type rateLimiterEntry struct {
	limiter *rate.Limiter
	// lastSeen is written from the read path, so it is atomic rather than
	// guarded by the (read) lock. Unix nanoseconds.
	lastSeen atomic.Int64
}

type RateLimiter struct {
	limiter map[string]*rateLimiterEntry
	mutex *sync.RWMutex
	limit rate.Limit
	cap int
	// inserts counts new entries since the last sweep. Guarded by mutex.
	inserts int
	// idleTTL and sweepEvery control eviction; a limiter that has not been
	// asked about for idleTTL is dropped every sweepEvery new clients.
	idleTTL time.Duration
	sweepEvery int
}

const (
	defaultRateLimiterIdleTTL = 10 * time.Minute
	defaultRateLimiterSweepEvery = 256
)

func NewRateLimiter(cfg *gitus.GitusConfig) *RateLimiter {
	return &RateLimiter{
		limiter: make(map[string]*rateLimiterEntry, 0),
		mutex: &sync.RWMutex{},
		limit: rate.Limit(cfg.MaxRequestInSecond),
		cap: 1,
		idleTTL: defaultRateLimiterIdleTTL,
		sweepEvery: defaultRateLimiterSweepEvery,
	}
}

func ResolveMostPossibleIP(w http.ResponseWriter, r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	netip := net.ParseIP(ip)
	if netip != nil { return netip.String() }

	ips := r.Header.Values("X-Forwarded-For")
	// one should know that this is never going to be 100% correct.
	for _, ip := range ips {
		for k := range strings.SplitSeq(ip, ",") {
			netip = net.ParseIP(strings.TrimSpace(k))
			if netip != nil { return netip.String() }
		}
	}

	// TODO: handle "Forward" header as well.

	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil { return r.RemoteAddr }
	return h
}

func (rl *RateLimiter) IsIPAllowed(s string) bool {
	now := time.Now().UnixNano()
	// Fast path: the entry almost always exists after the first request.
	rl.mutex.RLock()
	e, ok := rl.limiter[s]
	if ok { e.lastSeen.Store(now) }
	rl.mutex.RUnlock()
	if !ok {
		rl.mutex.Lock()
		e, ok = rl.limiter[s]
		if !ok {
			e = &rateLimiterEntry{limiter: rate.NewLimiter(rl.limit, rl.cap)}
			e.lastSeen.Store(now)
			rl.limiter[s] = e
			rl.inserts++
			if rl.sweepEvery > 0 && rl.inserts % rl.sweepEvery == 0 {
				rl.sweepLocked(now)
			}
		}
		e.lastSeen.Store(now)
		rl.mutex.Unlock()
	}
	return e.limiter.Allow()
}

// sweepLocked drops the limiters of clients that have not made a request for a
// while, so the map does not grow by one entry per address the instance has
// ever seen. Dropping an idle limiter only resets its token bucket.
func (rl *RateLimiter) sweepLocked(now int64) {
	cutoff := now - int64(rl.idleTTL)
	for k, e := range rl.limiter {
		if e.lastSeen.Load() < cutoff {
			delete(rl.limiter, k)
		}
	}
}
