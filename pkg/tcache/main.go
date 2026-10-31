package tcache

import (
	"container/heap"
	"sync"
	"time"
)

// temporary cache.
// used to store kv pairs that expires after a set amount of time.
//
// Expiry is handled by one sweeper goroutine per cache and a min-heap of
// (key, expiry) entries. The previous implementation started one time.Timer
// and one goroutine per key, which is a lot of goroutines for a session cache
// and leaked the goroutine when the entry was deleted before it expired.

type tCacheVal struct {
	value string
	// zero means "no expiry".
	expiresAt time.Time
	// identifies this version of the entry; heap items carrying an older seq
	// belong to a value that has been replaced or deleted.
	seq uint64
}

type expiryItem struct {
	key string
	at time.Time
	seq uint64
}

type expiryHeap []expiryItem

func (h expiryHeap) Len() int { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h expiryHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *expiryHeap) Push(x any) { *h = append(*h, x.(expiryItem)) }
func (h *expiryHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type TCache struct {
	defaultTimeout time.Duration
	val map[string]*tCacheVal
	expiries expiryHeap
	seq uint64
	lock sync.RWMutex
	// wake tells the sweeper that the earliest expiry may have changed.
	wake chan struct{}
}

func NewTCache(d time.Duration) *TCache {
	res := &TCache{
		defaultTimeout: d,
		val: make(map[string]*tCacheVal, 0),
		wake: make(chan struct{}, 1),
	}
	go res.sweepLoop()
	return res
}

// `d = -1` registers the kvpair w/o a TTL.
func (tc *TCache) Register(key string, value string, d time.Duration) {
	tc.lock.Lock()
	v, ok := tc.val[key]
	if !ok {
		v = &tCacheVal{}
		tc.val[key] = v
	}
	tc.seq++
	v.value = value
	v.seq = tc.seq
	if d == -1 {
		v.expiresAt = time.Time{}
	} else {
		v.expiresAt = time.Now().Add(d)
		heap.Push(&tc.expiries, expiryItem{key: key, at: v.expiresAt, seq: v.seq})
	}
	tc.wakeLocked()
	tc.lock.Unlock()
}

func (tc *TCache) Get(key string) (string, bool) {
	tc.lock.RLock()
	v, ok := tc.val[key]
	if !ok {
		tc.lock.RUnlock()
		return "", false
	}
	value := v.value
	expired := !v.expiresAt.IsZero() && !time.Now().Before(v.expiresAt)
	tc.lock.RUnlock()
	if expired { return "", false }
	return value, true
}

func (tc *TCache) Delete(key string) {
	tc.lock.Lock()
	delete(tc.val, key)
	tc.wakeLocked()
	tc.lock.Unlock()
}

// wakeLocked asks the sweeper to recompute its timer. The channel has room for
// one pending notification, which is all that is needed: the sweeper reads the
// heap under the lock after waking, so it sees every change anyway.
func (tc *TCache) wakeLocked() {
	select {
	case tc.wake <- struct{}{}:
	default:
	}
}

// dropExpiredLocked removes the expired entries at the front of the heap and
// discards heap items that no longer match the current value.
func (tc *TCache) dropExpiredLocked(now time.Time) {
	for len(tc.expiries) > 0 {
		e := tc.expiries[0]
		v, ok := tc.val[e.key]
		if !ok || v.seq != e.seq {
			heap.Pop(&tc.expiries)
			continue
		}
		if e.at.After(now) { break }
		delete(tc.val, e.key)
		heap.Pop(&tc.expiries)
	}
}

// compactLocked rebuilds the heap from the live entries. Re-registering a key
// leaves its previous heap item behind (it is ignored because its seq no
// longer matches), so a key that is refreshed often would otherwise grow the
// heap for a whole TTL.
func (tc *TCache) compactLocked() {
	h := make(expiryHeap, 0, len(tc.val))
	for k, v := range tc.val {
		if !v.expiresAt.IsZero() {
			h = append(h, expiryItem{key: k, at: v.expiresAt, seq: v.seq})
		}
	}
	heap.Init(&h)
	tc.expiries = h
}

func (tc *TCache) sweepLoop() {
	for {
		tc.lock.Lock()
		tc.dropExpiredLocked(time.Now())
		if len(tc.expiries) > 2*len(tc.val)+16 {
			tc.compactLocked()
		}
		wait := time.Hour
		if len(tc.expiries) > 0 {
			wait = time.Until(tc.expiries[0].at)
			if wait < 0 { wait = 0 }
		}
		tc.lock.Unlock()

		if wait <= 0 { continue }
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-tc.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}
}
