// Package metrics is the Phase 7 (7-i) observability surface.
//
// One Registry, four collectors, one optional separate HTTP server. The public
// surface is FROZEN in ai/tasks/phase-7/README.md §8.3 — 7-ii, 7-iii, 7-iv
// consume this package unchanged.
//
// Disabled mode (Enabled=false, the default) returns a literal-passthrough
// Middleware, no-op Record* methods, and an immediate-return ListenAndServe.
// Zero observable cost vs. Phase 6.
//
// Enabled mode (Enabled=true) registers four Prometheus collectors on a
// dedicated *prometheus.Registry (NOT the default global — this keeps tests
// hermetic and prevents MustRegister panics on accidental re-init) and binds
// a separate *http.Server on addr serving /metrics via promhttp.HandlerFor.
//
// Independence invariant: a port-bind failure surfaces as an error return
// from ListenAndServe and is logged at slog.Error in main.go, but the main
// relay HTTP server continues to serve. Observability shouldn't be able to
// brick the service it observes.
package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds the four v0 collectors plus the optional *http.Server. The
// zero value is NOT usable; callers MUST construct via New.
type Registry struct {
	enabled bool
	addr    string

	// reg is the package-local prometheus registry. nil when enabled=false.
	reg *prometheus.Registry

	// The four v0 collectors. nil when enabled=false (Record* methods check).
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	llmTokensTotal      *prometheus.CounterVec
	adapterCallsTotal   *prometheus.CounterVec
}

// New constructs a Registry. enabled=false makes ListenAndServe + all Record*
// methods no-ops; the returned Middleware is a literal passthrough.
// addr defaults to ":9090" when empty.
func New(enabled bool, addr string) *Registry {
	if !enabled {
		return &Registry{enabled: false}
	}
	if addr == "" {
		addr = ":9090"
	}

	reg := prometheus.NewRegistry()

	httpRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "intake_http_requests_total",
			Help: "Total HTTP requests handled by the intake relay, labelled by chi route pattern and status code.",
		},
		[]string{"path", "status"},
	)
	httpRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "intake_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds, labelled by chi route pattern. Default Prometheus buckets.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path"},
	)
	llmTokensTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "intake_llm_tokens_total",
			Help: "Total LLM tokens, labelled by provider and direction (input|output). Reported by the turn handler on SSEDone.",
		},
		[]string{"provider", "direction"},
	)
	adapterCallsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "intake_adapter_calls_total",
			Help: "Total adapter Create calls, labelled by adapter and result (success|error). Reported by the submit handler.",
		},
		[]string{"adapter", "result"},
	)

	reg.MustRegister(httpRequestsTotal, httpRequestDuration, llmTokensTotal, adapterCallsTotal)

	return &Registry{
		enabled:             true,
		addr:                addr,
		reg:                 reg,
		httpRequestsTotal:   httpRequestsTotal,
		httpRequestDuration: httpRequestDuration,
		llmTokensTotal:      llmTokensTotal,
		adapterCallsTotal:   adapterCallsTotal,
	}
}

// Middleware observes HTTP request count + duration. In disabled mode it
// returns a literal passthrough; in enabled mode it wraps the response writer
// to capture the status code and reads chi's RoutePattern for cardinality
// safety (the `path` label is bounded by chi's route table — NOT by query
// strings or path parameters).
func (r *Registry) Middleware() func(http.Handler) http.Handler {
	if r == nil || !r.enabled {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, req)
			pattern := chi.RouteContext(req.Context()).RoutePattern()
			if pattern == "" {
				// chi sets the pattern after the handler matches; if the route
				// did not match (404 from chi.NotFound), bucket it under
				// "unmatched" so cardinality stays bounded.
				pattern = "unmatched"
			}
			status := strconv.Itoa(rec.status)
			r.httpRequestsTotal.WithLabelValues(pattern, status).Inc()
			r.httpRequestDuration.WithLabelValues(pattern).Observe(time.Since(start).Seconds())
		})
	}
}

// ListenAndServe starts the metrics HTTP server on r.addr serving /metrics
// via promhttp. Returns when ctx is cancelled. Disabled-mode no-op.
// A port-bind failure returns the underlying net error — main.go logs it at
// slog.Error and continues; the main relay HTTP server is unaffected.
func (r *Registry) ListenAndServe(ctx context.Context) error {
	if r == nil || !r.enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{
		Registry: r.reg,
	}))

	srv := &http.Server{
		Addr:              r.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Shutdown the server when the caller's ctx is cancelled.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		// Either a bind error (immediate) or a real listen error.
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	}
}

// RecordLLMTokens is called from the turn handler on SSEDone.
// direction must be "input" or "output"; count is added to the counter.
// Disabled-mode no-op.
func (r *Registry) RecordLLMTokens(provider, direction string, count int) {
	if r == nil || !r.enabled {
		return
	}
	r.llmTokensTotal.WithLabelValues(provider, direction).Add(float64(count))
}

// RecordAdapterCall is called from the submit handler after adapter.Create.
// result must be "success" or "error". Disabled-mode no-op.
func (r *Registry) RecordAdapterCall(adapterName, result string) {
	if r == nil || !r.enabled {
		return
	}
	r.adapterCallsTotal.WithLabelValues(adapterName, result).Inc()
}

// statusRecorder wraps http.ResponseWriter to capture the status code for the
// `status` label. We intentionally do NOT pull in github.com/felixge/httpsnoop
// to avoid a new transitive dep — the four lines of wrapping below are enough.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// Flush implements http.Flusher when the underlying ResponseWriter does. SSE
// handlers rely on this; without it, /v1/intake/turn streaming would buffer
// the whole response before flushing.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---- Test accessors. Exported only for use by metrics_test.go.
// These are needed because testutil.ToFloat64 takes a prometheus.Collector,
// and our four collectors are otherwise unexported.

// HTTPRequestsTotalForTest exposes the HTTP request counter for testutil. Test-only.
func (r *Registry) HTTPRequestsTotalForTest(path, status string) prometheus.Counter {
	return r.httpRequestsTotal.WithLabelValues(path, status)
}

// LLMTokensTotalForTest exposes the LLM-token counter for testutil. Test-only.
func (r *Registry) LLMTokensTotalForTest(provider, direction string) prometheus.Counter {
	return r.llmTokensTotal.WithLabelValues(provider, direction)
}

// AdapterCallsTotalForTest exposes the adapter-call counter for testutil. Test-only.
func (r *Registry) AdapterCallsTotalForTest(adapter, result string) prometheus.Counter {
	return r.adapterCallsTotal.WithLabelValues(adapter, result)
}
