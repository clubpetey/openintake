package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metricstestutil "github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/clubpetey/openintake/relay/internal/auth"
	"github.com/clubpetey/openintake/relay/internal/config"
	metricsregistry "github.com/clubpetey/openintake/relay/internal/metrics"
	"github.com/clubpetey/openintake/relay/internal/ratelimit/perip"
	"github.com/clubpetey/openintake/relay/internal/server"
	"github.com/clubpetey/openintake/relay/internal/version"
)

// ---- helpers ----

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("JSON decode failed: %v\nbody: %s", err, body)
	}
}

func newTestServer(t *testing.T, corsOrigins []string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr:        ":8080",
			ExternalURL: "http://localhost:8080",
			CORSOrigins: corsOrigins,
		},
	}
	deps := server.Deps{
		Version:     version.Info(),
		CORSOrigins: corsOrigins,
	}
	return server.New(cfg, deps)
}

// ---- Task 6: /v1/health ----

func TestHealth_Returns200(t *testing.T) {
	h := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
	}
	var body map[string]string
	decodeJSON(t, w.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("body.status = %q; want %q", body["status"], "ok")
	}
}

// ---- Task 6: /v1/version ----

func TestVersion_ReturnsBuildInfo(t *testing.T) {
	h := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
	}
	var info version.BuildInfo
	decodeJSON(t, w.Body.Bytes(), &info)
	if info.Version == "" {
		t.Error("version.version is empty")
	}
}

// ---- Task 6: CORS — allowed origin gets ACAO header ----

func TestCORS_AllowedOriginGetsHeader(t *testing.T) {
	allowed := "http://localhost:5173"
	h := newTestServer(t, []string{allowed})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", allowed)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != allowed {
		t.Errorf("ACAO = %q; want %q", acao, allowed)
	}
}

// ---- Task 6: CORS — disallowed origin does NOT get ACAO header ----

func TestCORS_DisallowedOriginNoHeader(t *testing.T) {
	h := newTestServer(t, []string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "" {
		t.Errorf("ACAO = %q; want empty for disallowed origin", acao)
	}
}

// ---- Task 6: CORS — preflight for allowed origin returns 204 ----

func TestCORS_PreflightAllowedOrigin(t *testing.T) {
	allowed := "http://localhost:5173"
	h := newTestServer(t, []string{allowed})

	req := httptest.NewRequest(http.MethodOptions, "/v1/intake/init", nil)
	req.Header.Set("Origin", allowed)
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d; want %d", w.Code, http.StatusNoContent)
	}
}

// ---- Task 6: CORS — preflight for disallowed origin returns 403 ----

func TestCORS_PreflightDisallowedOrigin(t *testing.T) {
	h := newTestServer(t, []string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodOptions, "/v1/intake/init", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("preflight status = %d; want %d", w.Code, http.StatusForbidden)
	}
}

// ---- Fix 2: OPTIONS without Origin must NOT be 403 (not a CORS preflight) ----

func TestCORS_OptionsWithoutOriginPassesThrough(t *testing.T) {
	h := newTestServer(t, []string{"http://localhost:5173"})

	// OPTIONS /v1/health with no Origin header — this is a plain HTTP OPTIONS
	// request, not a CORS preflight. The CORS middleware must not short-circuit
	// it with a 403; it must be handled by the router.
	req := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Errorf("OPTIONS without Origin returned 403; want router response (e.g. 405), not a CORS block")
	}
}

// ---- Task 7 (5-i): /v1/intake group wires clientIP + perIP middlewares ----

// TestServerNew_HealthAndIntakeRouteRegistration verifies that:
//   - /v1/health responds 200 (registered at the top level, outside /v1/intake)
//   - /v1/intake/init responds 200 (registered inside the /v1/intake group)
//
// Both new Phase 5 middlewares (clientIPMiddleware, perIPLimitMiddleware) are
// wired into the /v1/intake group but are no-ops with PerIP=nil and
// TrustedProxies=nil, so this test does NOT verify they actually run.
// The behavioral verification lives in 5-ii Task 1's perip.Limiter tests,
// which exercise the full chain with a rejecting Limiter: /v1/intake/init
// returns 429 while /v1/health continues to return 200.
func TestServerNew_HealthAndIntakeRouteRegistration(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{}}}
	deps := server.Deps{
		Auth:           auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg:        config.AuthConfig{Modes: config.AuthModes{Anonymous: true}},
		TrustedProxies: nil, // empty list — default behavior
		PerIP:          nil, // nil limiter → always-allow
		Version:        version.BuildInfo{Version: "test"},
	}
	srv := server.New(cfg, deps)

	// /v1/health is OUTSIDE /v1/intake — no rate limit, no client-IP middleware.
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/health", nil)
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("/v1/health status = %d; want 200", rec.Code)
		}
	}

	// /v1/intake/init flows through clientIPMiddleware + perIPLimitMiddleware.
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
		req.RemoteAddr = "203.0.113.10:12345"
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("/v1/intake/init status = %d; want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	}
}

// ---- Fix 3: Vary: Origin is set on every response ----

func TestCORS_VaryOriginAlwaysSet(t *testing.T) {
	allowed := "http://localhost:5173"
	h := newTestServer(t, []string{allowed})

	// Verify Vary: Origin on a request with an allowed origin.
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", allowed)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	vary := w.Header().Get("Vary")
	if vary == "" {
		t.Error("Vary header is absent; want Vary: Origin on CORS responses")
	}
}

// ---- Phase 7-i: metrics middleware wiring ----

// TestServer_MetricsMiddlewareCountsRateLimited429s asserts the metrics
// middleware runs BEFORE the per-IP rate limiter — a request that gets
// rate-limited (429) is still counted. This is the "observability sees ALL
// inbound traffic" invariant.
func TestServer_MetricsMiddlewareCountsRateLimited429s(t *testing.T) {
	reg := metricsregistry.New(true, ":0")
	// PerIP limiter with a tiny non-zero RPS but burst=0 → first request 429s.
	// (rps=0 in perip means "no rate" / always allow; we need a non-zero rate
	// with zero burst to force a reject on the first request.)
	limiter := perip.New(0.001, 0, 1*time.Minute, time.Now)

	cfg := &config.Config{}
	deps := server.Deps{
		Logger:         slog.New(slog.NewJSONHandler(io.Discard, nil)),
		PerIP:          limiter,
		Metrics:        reg,
		Auth:           auth.NewMiddleware(auth.NewStore(), nil, nil),
		TrustedProxies: nil,
	}
	h := server.New(cfg, deps)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/init", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; want 429", rec.Code)
	}
	// chi resolves RoutePattern incrementally as a request descends through
	// nested r.Route() groups. When a middleware INSIDE /v1/intake (the
	// per-IP limiter) short-circuits with 429, the leaf route /v1/intake/init
	// has not yet matched, so chi's RoutePattern is the group prefix wildcard
	// "/v1/intake/*". Either form is bounded (cardinality-safe — the set of
	// possible patterns is the set of chi route declarations), so we accept
	// both. The invariant under test: a 429 IS counted (observability sees
	// ALL inbound traffic, including rate-limited rejects).
	pat := metricstestutil.ToFloat64(reg.HTTPRequestsTotalForTest("/v1/intake/init", "429"))
	groupWildcard := metricstestutil.ToFloat64(reg.HTTPRequestsTotalForTest("/v1/intake/*", "429"))
	unmatched := metricstestutil.ToFloat64(reg.HTTPRequestsTotalForTest("unmatched", "429"))
	if pat+groupWildcard+unmatched != 1 {
		t.Errorf("counter for 429 total = %v (init=%v, wildcard=%v, unmatched=%v); want 1", pat+groupWildcard+unmatched, pat, groupWildcard, unmatched)
	}
}
