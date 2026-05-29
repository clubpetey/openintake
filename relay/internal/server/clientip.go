package server

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// clientIPCtxKey is the unexported context key for the resolved client IP.
type clientIPCtxKey struct{}

// clientIPMiddleware resolves the request's client IP per the trusted-proxies
// allowlist and stashes it in r.Context() under clientIPCtxKey{}.
//
// Resolution:
//   - If RemoteAddr is in any CIDR of trustedProxies, walk X-Forwarded-For
//     right-to-left and take the first hop NOT in trustedProxies. If every
//     hop is trusted, use the leftmost hop (the original client per
//     RFC 7239 standard).
//   - Otherwise (or if trustedProxies is empty), use RemoteAddr verbatim.
//
// The stashed value is the IP only (no port). If RemoteAddr cannot be parsed,
// the empty string is stashed — the per-IP limiter will treat all such
// requests as a single bucket (safe degraded behavior).
func clientIPMiddleware(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trustedProxies)
			ctx := context.WithValue(r.Context(), clientIPCtxKey{}, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientIPFromContext returns the IP stashed by clientIPMiddleware.
// Returns "" if not set (or if the middleware stashed "" for an
// unparseable RemoteAddr).
func ClientIPFromContext(ctx context.Context) string {
	v, _ := ctx.Value(clientIPCtxKey{}).(string)
	return v
}

// resolveClientIP applies the rules in the clientIPMiddleware doc.
func resolveClientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	remoteIP := parseHostFromRemoteAddr(r.RemoteAddr)
	if remoteIP == "" {
		return ""
	}
	if !ipInAnyPrefix(remoteIP, trustedProxies) {
		return remoteIP
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remoteIP
	}
	// Right-to-left scan for the first untrusted hop.
	hops := strings.Split(xff, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(hops[i])
		if hop == "" {
			continue
		}
		if !ipInAnyPrefix(hop, trustedProxies) {
			return hop
		}
	}
	// All hops trusted → return the leftmost (original client per RFC 7239).
	leftmost := strings.TrimSpace(hops[0])
	if leftmost == "" {
		return remoteIP
	}
	return leftmost
}

// parseHostFromRemoteAddr extracts just the host portion of an "ip:port" pair,
// handling IPv6 bracketed form. Returns "" if r.RemoteAddr is unparseable.
func parseHostFromRemoteAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// Try as a bare IP (some test paths set RemoteAddr without a port).
		if ip, perr := netip.ParseAddr(remoteAddr); perr == nil {
			return ip.String()
		}
		return ""
	}
	return host
}

// ipInAnyPrefix reports whether ip is contained in any of the given prefixes.
// Returns false for an unparseable ip.
func ipInAnyPrefix(ip string, prefixes []netip.Prefix) bool {
	if len(prefixes) == 0 {
		return false
	}
	parsed, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, p := range prefixes {
		if p.Contains(parsed) {
			return true
		}
	}
	return false
}
