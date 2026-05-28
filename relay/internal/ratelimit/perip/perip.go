// Package perip is the per-IP token-bucket rate limiter for the relay's
// /v1/intake/* endpoints. Phase 5-i exports the empty type and the New
// constructor as placeholders so the server chain compiles; 5-ii fills in
// the actual implementation backed by golang.org/x/time/rate.
package perip

import "time"

// Limiter holds a per-client-IP token bucket. 5-i placeholder; 5-ii implements.
type Limiter struct {
	// Phase 5-i: no fields. Phase 5-ii populates with rps/burst/idleTTL/now
	// and a sync.Mutex-guarded map[string]*entry.
}

// New constructs a Limiter. 5-i placeholder; 5-ii implements.
// Until 5-ii lands, the only safe constructor is one that returns a zero-value
// Limiter (so perIPLimitMiddleware's nil-Limiter short-circuit never fires
// from a real construction path). main.go in 5-i passes nil to Deps.PerIP;
// do NOT call this constructor in 5-i.
func New(reqsPerSecond float64, burst int, idleTTL time.Duration, now func() time.Time) *Limiter {
	// 5-ii implements. Returning the zero value here is acceptable only
	// because no 5-i code path calls New (main.go passes nil to Deps.PerIP).
	return &Limiter{}
}

// Allow is the gate the middleware calls. 5-i placeholder; 5-ii implements.
func (l *Limiter) Allow(ip string) (ok bool, retryAfter time.Duration) {
	// 5-i: an all-allow stub keeps the chain compiling. 5-ii replaces with
	// the real rate.Limiter call.
	return true, 0
}
