// Package perip is the per-IP token-bucket rate limiter for the relay's
// /v1/intake/* endpoints. Backed by golang.org/x/time/rate.Limiter per key,
// with eager GC of buckets whose last-seen exceeds idleTTL.
//
// L014: injectable clock for deterministic tests; eager-eviction matches
// the auth/emailcode pattern (no background goroutine).
package perip

import (
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// entry holds one client IP's rate.Limiter and the last time it was touched.
type entry struct {
	bucket   *rate.Limiter
	lastSeen time.Time
}

// Limiter holds a per-client-IP token bucket. Buckets are eagerly GC'd when
// a key's last-seen exceeds the configured idle TTL.
type Limiter struct {
	mu        sync.Mutex
	buckets   map[string]*entry
	rate      rate.Limit // events per second
	burst     int
	idleTTL   time.Duration
	now       func() time.Time
	retryWait time.Duration // pre-computed: 1/rate, floor 1s
}

// New constructs a Limiter. reqsPerSecond is the steady-state rate; burst is
// the bucket capacity. idleTTL is how long an unused key is retained before GC.
// now is injectable for tests (production: time.Now).
//
// reqsPerSecond <= 0 is treated as "no rate" — every Allow returns ok=true
// (degraded safe; main.go's startup gate should have rejected this config,
// but the constructor remains permissive).
func New(reqsPerSecond float64, burst int, idleTTL time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	r := rate.Limit(reqsPerSecond)
	// retryWait: how long until the bucket has another token at the configured rate.
	// At rate=0, retryWait is irrelevant (Allow always returns ok=true below).
	retry := time.Second
	if reqsPerSecond > 0 {
		retry = time.Duration(math.Ceil(1.0/reqsPerSecond*float64(time.Second))) * time.Nanosecond
		// Floor at 1s for HTTP Retry-After numeric form (RFC 9110).
		if retry < time.Second {
			retry = time.Second
		}
	}
	return &Limiter{
		buckets:   make(map[string]*entry),
		rate:      r,
		burst:     burst,
		idleTTL:   idleTTL,
		now:       now,
		retryWait: retry,
	}
}

// Allow reports whether ip may proceed now. On reject, retryAfter is the bucket
// refill interval (1/reqsPerSecond) rounded UP to seconds, floor 1.
// An empty ip shares a single bucket (degraded safe; caller's clientIPMiddleware
// stashed "" because RemoteAddr was unparseable).
func (l *Limiter) Allow(ip string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.gcLocked(now)

	if l.rate <= 0 {
		// "No rate" → always allow; still track lastSeen for consistency.
		e, exists := l.buckets[ip]
		if !exists {
			e = &entry{bucket: rate.NewLimiter(rate.Inf, l.burst), lastSeen: now}
			l.buckets[ip] = e
		}
		e.lastSeen = now
		return true, 0
	}

	e, exists := l.buckets[ip]
	if !exists {
		e = &entry{bucket: rate.NewLimiter(l.rate, l.burst), lastSeen: now}
		l.buckets[ip] = e
	}
	e.lastSeen = now

	// rate.Limiter is internally clocked from time.Now; for deterministic tests
	// we use AllowN(now, 1) which consumes from the bucket at the supplied time.
	if e.bucket.AllowN(now, 1) {
		return true, 0
	}
	return false, l.retryWait
}

// gcLocked evicts buckets whose lastSeen is older than idleTTL.
// Caller MUST hold l.mu.
func (l *Limiter) gcLocked(now time.Time) {
	if l.idleTTL <= 0 {
		return
	}
	cutoff := now.Add(-l.idleTTL)
	for k, e := range l.buckets {
		if e.lastSeen.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}
