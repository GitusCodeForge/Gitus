package routes

import "github.com/GitusCodeForge/Gitus/pkg/gitus"

// DefaultMaxConcurrentClone is the number of HTTP clone requests that may
// spawn a `git upload-pack` process at the same time when the configuration
// does not set gitConfig.maxConcurrentClone.
const DefaultMaxConcurrentClone = gitus.DefaultMaxConcurrentClone

// CloneLimiter bounds the number of concurrent `git upload-pack` processes.
// Serving a clone costs a process (and the pack generation behind it), so
// without a bound a burst of clones can exhaust the host even when the HTTP
// server itself is fine.
type CloneLimiter struct {
	slots chan struct{}
}

// NewCloneLimiter returns a limiter with n slots. n <= 0 uses
// DefaultMaxConcurrentClone.
func NewCloneLimiter(n int) *CloneLimiter {
	if n <= 0 {
		n = DefaultMaxConcurrentClone
	}
	return &CloneLimiter{slots: make(chan struct{}, n)}
}

// TryAcquire takes a slot without waiting. It returns a release function and
// true on success, or nil and false when every slot is in use. Callers must
// call release once the clone request is done.
func (l *CloneLimiter) TryAcquire() (func(), bool) {
	if l == nil {
		return func() {}, true
	}
	select {
	case l.slots <- struct{}{}:
		return func() { <-l.slots }, true
	default:
		return nil, false
	}
}

// AcquireCloneSlot is the nil-safe wrapper for handlers. A context without a
// limiter (some tests build one by hand) is unlimited.
func (ctx *RouterContext) AcquireCloneSlot() (func(), bool) {
	if ctx == nil || ctx.CloneLimiter == nil {
		return func() {}, true
	}
	return ctx.CloneLimiter.TryAcquire()
}
