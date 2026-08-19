package tcache

import (
	"time"
	"sync"
)

// temporary cache.
// used to store kv pairs that expires after a set amount of time.

type tCacheVal struct {
	timer *tCacheTimer
	timeout time.Duration
	value string
}

type TCache struct {
	defaultTimeout time.Duration
	val map[string]*tCacheVal
	timerPool sync.Pool
	lock sync.RWMutex
}

type tCacheTimer struct {
	t *time.Timer
	associatedKey string
}

func newTCacheTimer(d time.Duration) *tCacheTimer {
	return &tCacheTimer{
		t: time.NewTimer(d),
		associatedKey: "",
	}
}

func (tc *TCache) timerFunc(t *tCacheTimer) {
	<-t.t.C
	delete(tc.val, t.associatedKey)
	tc.timerPool.Put(t)
}

func NewTCache(d time.Duration) *TCache {
	res := &TCache{
		defaultTimeout: d,
		val: make(map[string]*tCacheVal, 0),
		timerPool: sync.Pool{
			New: func() any {
				return newTCacheTimer(d)
			},
		},
	}
	return res
}

func (tc *TCache) Register(key string, value string, d time.Duration) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	_, ok := tc.val[key]
	if ok {
		tc.val[key].value = value
		tc.val[key].timer.t.Reset(d)
		return
	}
	t := tc.timerPool.Get().(*tCacheTimer)
	t.associatedKey = key
	tc.val[key] = &tCacheVal{
		timer: t,
		timeout: d,
		value: value,
	}
	t.t.Reset(d)
	go tc.timerFunc(t)
}

func (tc *TCache) Get(key string) (string, bool) {
	tc.lock.RLock()
	defer tc.lock.RUnlock()
	v, ok := tc.val[key]
	if !ok { return "", ok }
	return v.value, true
}

func (tc *TCache) Delete(key string) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	tm := tc.val[key].timer
	tm.t.Stop()
	tc.timerPool.Put(tm)
	delete(tc.val, key)
}

