package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func mustPrefix(t *testing.T, s string) netip.Prefix {
	t.Helper()
	p, err := netip.ParsePrefix(s)
	if err != nil {
		t.Fatalf("ParsePrefix(%q): %v", s, err)
	}
	return p
}

func newRequestWithRemoteAndXFF(remote, xff string) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	return req
}

func TestClientIP_EmptyTrustedProxies_UsesRemoteAddrVerbatim(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware(nil)
	req := newRequestWithRemoteAndXFF("203.0.113.5:12345", "1.2.3.4")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "203.0.113.5" {
		t.Errorf("ClientIP = %q; want 203.0.113.5 (XFF must be ignored when no trusted proxies)", captured)
	}
}

func TestClientIP_TrustedProxy_RightmostUntrustedXFFHop(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{mustPrefix(t, "10.0.0.0/8")})
	// RemoteAddr is trusted (10.0.0.1); XFF chain: original=203.0.113.7,
	// then a trusted internal proxy 10.0.0.2, then the trusted edge 10.0.0.1.
	req := newRequestWithRemoteAndXFF("10.0.0.1:12345", "203.0.113.7, 10.0.0.2")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "203.0.113.7" {
		t.Errorf("ClientIP = %q; want 203.0.113.7 (rightmost untrusted XFF hop)", captured)
	}
}

func TestClientIP_UntrustedRemoteAddr_IgnoresSpoofedXFF(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{mustPrefix(t, "10.0.0.0/8")})
	// RemoteAddr is NOT in any trusted CIDR → XFF is ignored even if present.
	req := newRequestWithRemoteAndXFF("203.0.113.5:12345", "1.2.3.4")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "203.0.113.5" {
		t.Errorf("ClientIP = %q; want 203.0.113.5 (untrusted RemoteAddr must use RemoteAddr verbatim)", captured)
	}
}

func TestClientIP_AllHopsTrusted_FallsBackToLeftmost(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{mustPrefix(t, "10.0.0.0/8")})
	// Every hop is trusted — fall back to the leftmost (original client per RFC 7239).
	req := newRequestWithRemoteAndXFF("10.0.0.1:12345", "10.1.1.1, 10.2.2.2")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "10.1.1.1" {
		t.Errorf("ClientIP = %q; want 10.1.1.1 (all-trusted falls back to leftmost)", captured)
	}
}

func TestClientIP_MalformedRemoteAddr_StashesEmpty(t *testing.T) {
	var captured string
	hit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware(nil)
	req := newRequestWithRemoteAndXFF("not-an-address", "")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if !hit {
		t.Fatal("next handler not invoked")
	}
	if captured != "" {
		t.Errorf("ClientIP = %q; want \"\" (malformed RemoteAddr → empty)", captured)
	}
}

func TestClientIP_NoMiddleware_ReturnsEmpty(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if ip := ClientIPFromContext(req.Context()); ip != "" {
		t.Errorf("ClientIPFromContext on bare ctx = %q; want \"\"", ip)
	}
}

func TestClientIP_IPv6RemoteAddr(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware(nil)
	req := newRequestWithRemoteAndXFF("[2001:db8::1]:9999", "")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "2001:db8::1" {
		t.Errorf("ClientIP = %q; want 2001:db8::1 (IPv6 from bracketed RemoteAddr)", captured)
	}
}

func TestClientIP_TrustedRemoteAddr_EmptyXFF_ReturnsRemoteAddr(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{mustPrefix(t, "10.0.0.0/8")})
	// RemoteAddr is trusted but XFF is absent — fall back to RemoteAddr.
	req := newRequestWithRemoteAndXFF("10.0.0.1:12345", "")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "10.0.0.1" {
		t.Errorf("ClientIP = %q; want 10.0.0.1 (trusted RemoteAddr + empty XFF)", captured)
	}
}

func TestClientIP_TrailingCommaAndWhitespaceXFF(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{mustPrefix(t, "10.0.0.0/8")})
	// Trailing comma + extra whitespace before/after hops — RTL scan must skip
	// the empty trailing element and trim spaces before CIDR membership check.
	req := newRequestWithRemoteAndXFF("10.0.0.1:12345", "203.0.113.7 ,  10.0.0.2 ,")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "203.0.113.7" {
		t.Errorf("ClientIP = %q; want 203.0.113.7 (RTL skips trailing-comma empty + trims whitespace)", captured)
	}
}

func TestClientIP_MultipleTrustedCIDRs(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{
		mustPrefix(t, "10.0.0.0/8"),
		mustPrefix(t, "192.168.0.0/16"),
	})
	// RemoteAddr in the second CIDR; XFF chain mixes hops from both trusted
	// CIDRs plus one untrusted. RTL scan must skip both trusted hops and return
	// the untrusted one.
	req := newRequestWithRemoteAndXFF("192.168.5.10:12345", "203.0.113.7, 10.0.0.2, 192.168.5.99")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "203.0.113.7" {
		t.Errorf("ClientIP = %q; want 203.0.113.7 (RTL skips both trusted-CIDR hops)", captured)
	}
}

func TestPerIPLimit_NilLimiter_AllowsAll(t *testing.T) {
	hit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true })
	mw := perIPLimitMiddleware(nil) // nil → "no limiter wired" → always allow
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)
	if !hit {
		t.Fatal("next handler not invoked when limiter is nil")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want 200 (default OK when next does not write)", rec.Code)
	}
}
