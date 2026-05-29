package server

import (
	"net/http"

	"intake/internal/ratelimit/perip"
)

// perIPLimitMiddleware enforces the per-IP token bucket via the supplied
// Limiter. When limiter is nil, all requests pass through (the chain is wired
// but the gate is inert — used in 5-i before 5-ii lands the real Limiter,
// and in unit tests that don't exercise rate-limiting).
//
// On reject: writes 429 + Retry-After: <secs> + the standard ErrorEnvelope
// with code "rate_limited". The client IP is read from r.Context() via
// ClientIPFromContext; an empty IP shares one bucket (degraded safe).
func perIPLimitMiddleware(limiter *perip.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}
			ip := ClientIPFromContext(r.Context())
			ok, retryAfter := limiter.Allow(ip)
			if ok {
				next.ServeHTTP(w, r)
				return
			}
			setRetryAfter(w, retryAfter)
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests; slow down")
		})
	}
}
