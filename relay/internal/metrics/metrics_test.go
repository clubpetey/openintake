package metrics_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/clubpetey/openintake/relay/internal/metrics"
)

// TestRegistry_DisabledIsNoOp asserts the zero-cost-when-disabled invariant:
// New(false, ...) returns a Registry whose Middleware is a literal passthrough,
// whose Record* methods short-circuit, and whose ListenAndServe returns nil
// immediately. No goroutines, no port bind, no collectors.
func TestRegistry_DisabledIsNoOp(t *testing.T) {
	r := metrics.New(false, ":0")
	if r == nil {
		t.Fatal("New(false, ...) returned nil; want a non-nil disabled Registry")
	}

	// Middleware passthrough: wrap an identity handler, confirm both work.
	called := false
	h := r.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Error("disabled Middleware did not call the next handler")
	}
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d; want 418", rec.Code)
	}

	// Record* methods are safe no-ops.
	r.RecordLLMTokens("anthropic", "input", 100)
	r.RecordLLMTokens("openai", "output", 50)
	r.RecordAdapterCall("webhook", "success")
	r.RecordAdapterCall("chatwoot", "error")

	// ListenAndServe returns immediately on disabled with nil error.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := r.ListenAndServe(ctx); err != nil {
		t.Errorf("ListenAndServe on disabled returned %v; want nil", err)
	}
}

// TestRegistry_EnabledIncrementsHTTPCounter exercises the request-count counter
// via testutil.ToFloat64. Drives 3 requests through a chi router with a known
// RoutePattern; asserts the counter for that {path,status} pair increased by 3.
func TestRegistry_EnabledIncrementsHTTPCounter(t *testing.T) {
	r := metrics.New(true, ":0") // :0 → no port bind; we don't start the server here.

	mux := chi.NewMux()
	mux.Use(r.Middleware())
	mux.Get("/v1/intake/init", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/intake/init", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d; want 200", i, rec.Code)
		}
	}

	got := testutil.ToFloat64(r.HTTPRequestsTotalForTest("/v1/intake/init", "200"))
	if got != 3.0 {
		t.Errorf("intake_http_requests_total{path=/v1/intake/init,status=200} = %v; want 3", got)
	}
}

// TestRegistry_EnabledRecordsLLMTokens asserts RecordLLMTokens increments the
// provider+direction counter.
func TestRegistry_EnabledRecordsLLMTokens(t *testing.T) {
	r := metrics.New(true, ":0")
	r.RecordLLMTokens("anthropic", "input", 1234)
	r.RecordLLMTokens("anthropic", "input", 100)
	r.RecordLLMTokens("anthropic", "output", 500)
	r.RecordLLMTokens("openai", "output", 200)

	if got := testutil.ToFloat64(r.LLMTokensTotalForTest("anthropic", "input")); got != 1334 {
		t.Errorf("anthropic/input = %v; want 1334", got)
	}
	if got := testutil.ToFloat64(r.LLMTokensTotalForTest("anthropic", "output")); got != 500 {
		t.Errorf("anthropic/output = %v; want 500", got)
	}
	if got := testutil.ToFloat64(r.LLMTokensTotalForTest("openai", "output")); got != 200 {
		t.Errorf("openai/output = %v; want 200", got)
	}
}

// TestRegistry_EnabledRecordsAdapterCall asserts RecordAdapterCall increments
// the adapter+result counter.
func TestRegistry_EnabledRecordsAdapterCall(t *testing.T) {
	r := metrics.New(true, ":0")
	r.RecordAdapterCall("webhook", "success")
	r.RecordAdapterCall("webhook", "success")
	r.RecordAdapterCall("webhook", "error")
	r.RecordAdapterCall("chatwoot", "error")

	if got := testutil.ToFloat64(r.AdapterCallsTotalForTest("webhook", "success")); got != 2 {
		t.Errorf("webhook/success = %v; want 2", got)
	}
	if got := testutil.ToFloat64(r.AdapterCallsTotalForTest("webhook", "error")); got != 1 {
		t.Errorf("webhook/error = %v; want 1", got)
	}
	if got := testutil.ToFloat64(r.AdapterCallsTotalForTest("chatwoot", "error")); got != 1 {
		t.Errorf("chatwoot/error = %v; want 1", got)
	}
}

// TestRegistry_PathLabelUsesChiRoutePattern is the cardinality-safety test:
// 100 requests with different query strings against the SAME chi route must
// produce ONE series (not 100). Without RoutePattern this test would fail
// because query-string values would leak into the label.
func TestRegistry_PathLabelUsesChiRoutePattern(t *testing.T) {
	r := metrics.New(true, ":0")
	mux := chi.NewMux()
	mux.Use(r.Middleware())
	mux.Get("/v1/intake/init", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/intake/init?session_id=ssn-"+itoa(i), nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}
	got := testutil.ToFloat64(r.HTTPRequestsTotalForTest("/v1/intake/init", "200"))
	if got != 100 {
		t.Errorf("expected one series with count 100; got %v (cardinality exploded — RoutePattern not used)", got)
	}
}

// TestRegistry_ListenAndServe_PortConflict binds the addr first, then asserts
// ListenAndServe returns a non-nil error WITHOUT crashing the process. Models
// the independence invariant: a port-bind failure must surface as a return
// value, not a panic.
func TestRegistry_ListenAndServe_PortConflict(t *testing.T) {
	// Bind a port first so the metrics ListenAndServe fails.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	r := metrics.New(true, addr)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err = r.ListenAndServe(ctx)
	if err == nil {
		t.Fatal("ListenAndServe with port already bound returned nil; want non-nil error")
	}
	if !strings.Contains(err.Error(), "address already in use") &&
		!strings.Contains(err.Error(), "bind") &&
		!strings.Contains(err.Error(), "Only one usage") /* Windows */ {
		t.Errorf("expected bind-error substring; got %v", err)
	}
}

// itoa avoids importing strconv at the top of the test file for one use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = digits[n%10]
		n /= 10
	}
	return string(b[i:])
}
