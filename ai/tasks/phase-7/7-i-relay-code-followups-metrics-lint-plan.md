# 7-i Relay Code — FOLLOWUPS + Prometheus Metrics + Lint Configs + Initial-Fix Sweep + CI Extension — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lock the Phase 7 wire contract so 7-ii (release artifacts), 7-iii (demo stack), 7-iv (docs + governance) can land in parallel. Pin one new external Go module (`github.com/prometheus/client_golang`, exact-pinned) and extend `scripts/check-pins.sh` to enforce it. Ship a new `relay/internal/metrics/` package exposing the frozen surface declared in README §8.3 — `Registry`, `New(enabled bool, addr string) *Registry`, `Middleware() func(http.Handler) http.Handler`, `ListenAndServe(ctx context.Context) error`, `RecordLLMTokens(provider, direction string, count int)`, `RecordAdapterCall(adapterName, result string)` — backing four Prometheus collectors (`intake_http_requests_total{path,status}`, `intake_http_request_duration_seconds{path}`, `intake_llm_tokens_total{provider,direction}`, `intake_adapter_calls_total{adapter,result}`). Disabled mode (`Enabled=false`, the default) returns a literal-passthrough middleware, no-op `Record*` methods, and an immediate-return `ListenAndServe` — zero observable cost. Enabled mode binds a separate `*http.Server` on `cfg.Observability.Metrics.Addr` (default `:9090`) serving `/metrics` via `promhttp.HandlerFor`; a port-bind failure logs at Error but does NOT crash the main relay (independence invariant). Add `Observability ObservabilityConfig` + `MetricsConfig` to `relay/internal/config/config.go` (off-by-default invariant: `Enabled bool` defaults false). Add `Metrics *metrics.Registry` to `relay/internal/server/deps.go` (nil-safe via the package's passthrough/no-op semantics). Close all four Phase 6 FOLLOWUPS in this sub-plan: I1 — refactor `buildRegistry` to return `([]adapter.Adapter, []string)` so per-adapter `Configure()` failures, license-gate "no adapters enabled", and other registry problems all flow into the consolidated startup-problems slice instead of `os.Exit(1)`; I2 — extract `accumulateStartupProblems(cfg, licState, logger) (Deps, []string)` from `main()` for unit testability and lock the L022 contract gap closed; M2 — `validateAttachments` returns zero-value `config.AttachmentsConfig{}` when `Enabled=false`; M4 — `run-q9-smoke.sh` uses `(cd relay && ...)` subshells. Add metric-record sites — single-line additive calls — in `turn.go` (on `SSEDone`) and `submit.go` (after `adapter.Create`). Prepend `deps.Metrics.Middleware()` to the chi chain at the FRONT (BEFORE the per-IP rate limiter so 429s are still counted — observability sees ALL inbound traffic). Author repo-root lint configs (`.golangci.yml`, `.eslintrc.cjs`, `.prettierrc`, plus ignore files) using curated rulesets (NOT `--enable-all`). Run an initial-fix sweep BEFORE wiring CI gates: triage every finding (real bug → fix; false positive → targeted `//nolint:rule // reason` or `// eslint-disable-next-line rule -- reason`; style preference → reformat or narrow). Extend `.github/workflows/ci.yml` with four new jobs — `lint-go`, `lint-ts`, `test-go`, `test-ts` — pinned to exact tool versions; `goreleaser-check` + `snapshot-build` are deferred to 7-ii (they need `.goreleaser.yaml` 7-ii authors). After this sub-plan: Deps shape is frozen, the metrics surface is frozen, the L022 contract gap is closed, every linter reports zero issues, and 7-ii/7-iii/7-iv have a stable seam to build on.

**Architecture:** Three additive surfaces plus four FOLLOWUPS plus lint integration. (1) `metrics` package is structurally independent: `New(false, "")` returns a `*Registry` whose `Middleware` is a literal `func(h http.Handler) http.Handler { return h }` passthrough, whose `Record*` methods short-circuit on a nil internal collector pointer, and whose `ListenAndServe` returns nil immediately. `New(true, addr)` constructs the four collectors via `prometheus.NewCounterVec` / `prometheus.NewHistogramVec`, registers them on a fresh `*prometheus.Registry` (NOT the default global, to keep tests isolated and to avoid `MustRegister` panics on re-init), and stores a `*http.Server` whose handler is `promhttp.HandlerFor(reg, promhttp.HandlerOpts{})`. `Middleware` wraps the next handler with a `httpsnoop`-style status-code-capturing wrapper (implemented inline as a `responseWriter` wrapper — no new dep), reads `chi.RouteContext(r.Context()).RoutePattern()` for the bounded `path` label, calls `r.Inc()` on the request counter and `r.Observe()` on the histogram. (2) Config addition is purely additive: `ObservabilityConfig` + `MetricsConfig` slot beside Phase 5 `RateLimit` and Phase 6 `Attachments` in `Config`; defaults applied in `applyDefaults` (LogLevel="info", LogFormat="json", Metrics.Enabled=false, Metrics.Addr=":9090"). The Q9 startup gate does NOT validate the metrics block — port-bind failures are runtime warnings, not startup errors (per design spec §3.2). (3) Deps gains exactly one field (`Metrics *metrics.Registry`); existing unit tests that construct `Deps{}` literally remain valid because nil-safety lives inside the metrics package, not at the call sites. (4) FOLLOWUPS land as `accumulateStartupProblems` — a pure function taking `(cfg, licState, logger)` returning `(Deps, []string)` — composed of the existing `startupProblems`, the refactored `buildRegistry` (now returning `([]adapter.Adapter, []string)` instead of `(map[string]adapter.Adapter, error)`), and the existing `validateAttachments` (with the M2 zero-value short-circuit). `main()` calls it once, logs one consolidated line + one `os.Exit(1)`, then starts the metrics server in a goroutine (independent of the main HTTP server). (5) Lint integration runs sweep-first: `.golangci.yml` enables `errcheck`, `govet`, `staticcheck`, `gosec`, `ineffassign`, `unused`; `.eslintrc.cjs` uses `eslint-plugin-vue` recommended + `@typescript-eslint/recommended` strict; `.prettierrc` matches the existing two-space-indent style in `core/src/client.ts`. The sweep produces 1+ `chore(7-i): initial lint sweep — <linter> findings (...)` commits BEFORE the CI gate commit, so reverting the gate (if it surfaces unexpected noise) doesn't lose the fixes.

**Tech Stack:** Go 1.23.2 (relay), `github.com/prometheus/client_golang` v1.20.5 (verify exact latest stable as of 2026-06-01 in Task 1; do NOT use `@latest`), `golangci-lint` v1.62.2 (verify in Task 12), `eslint` v9.16.0 + `eslint-plugin-vue` v9.32.0 + `@typescript-eslint/parser` v8.18.0 + `@typescript-eslint/eslint-plugin` v8.18.0 (verify in Task 12), `prettier` v3.4.2 (verify in Task 12). Zero new TS modules for the metrics surface (TS doesn't see metrics). `go mod tidy` after Task 1 must produce exactly one new module entry (`prometheus/client_golang`) plus its transitive indirect deps in the `require ( ... // indirect )` block.

---

## Design References

- README §8.2 — `ObservabilityConfig` + `MetricsConfig` (frozen here)
- README §8.3 — `metrics.Registry` package surface (frozen here)
- README §8.4 — `Deps.Metrics` field (frozen here)
- README §8.5 — `accumulateStartupProblems` shape (frozen here, mirrors design spec §5.4)
- README §8.6 — endpoint contract: `/metrics` on a SEPARATE server (no new endpoints on the main HTTP server)
- README §6 — build-fail checklist items 1, 3, 5, 6, 9, 23–26 (off-by-default invariant, independence invariant, lint zero-issues, FOLLOWUPS closure, Phase 1+4+5+6 regression)
- README §7 final-smoke items 1 (Q9 startup gate combined fixture), 2 (metrics endpoint smoke), 7 (Phase 1+4+5+6 regression), 8 (lint smoke)
- Design spec §3.2 — Prometheus metrics: separate HTTP server, opt-in, 4 core series + cardinality rationale
- Design spec §3.3 — initial-fix sweep precedes the CI gate
- Design spec §3.6 — Phase 6 FOLLOWUPS fold into 7-i (closes the L022 contract gap)
- Design spec §5.1 — `metrics` package layout + exports
- Design spec §5.2 — Config addition
- Design spec §5.3 — Deps extension
- Design spec §5.4 — `main.go` orchestration after FOLLOWUPS I1+I2
- Design spec §5.5 — metric-record sites (single-line additive calls)
- Design spec §7.2 — metrics observation in the request path (chi `RoutePattern()` for cardinality safety)
- Design spec §8.2 — startup gate consolidation (closes L022 contract gap)
- Design spec §8.3 — metrics endpoint failure modes (independence invariant)
- Design spec §8.4 — lint signal management (sweep before gate)
- LESSONS L010 — PowerShell `Set-Content -Encoding ascii` for YAML written via PS (prefer Bash)
- LESSONS L016 — return parsed values from the startup gate (no re-parse-with-discarded-error)
- LESSONS L022 — ONE consolidated log line, ONE `os.Exit(1)`; 7-i closes the contract gap by folding `buildRegistry` failures into the slice
- LESSONS L023 — unused-but-parsed body fields are load-bearing assertions (applies inside `metrics` to the `success` field semantics of `MustRegister` — handled package-internal)
- Reference: `relay/cmd/relay/main.go:74-138` (the current 3-gate orchestration — startupProblems, buildRegistry+len(registry)==0, validateAttachments — being extracted into `accumulateStartupProblems`)
- Reference: `relay/cmd/relay/main.go:467-570` (current `buildRegistry` signature returning `(map[string]adapter.Adapter, error)` — refactor target for FOLLOWUPS I1)
- Reference: `relay/cmd/relay/main.go:671-717` (current `validateAttachments` — M2 zero-value-on-disabled target)
- Reference: `relay/internal/config/config.go:13-22` (the `Config` struct where `Observability` slots in after `Attachments`)
- Reference: `relay/internal/server/deps.go:27-118` (current Deps shape — `Metrics *metrics.Registry` field appended at the end)
- Reference: `relay/internal/server/server.go:22-43` (chi chain composition — `r.Use(perIPLimitMiddleware(...))` line; `deps.Metrics.Middleware()` is prepended BEFORE it)
- Reference: `relay/internal/server/turn.go:290-310` (SSEDone callsite where `RecordLLMTokens` lands)
- Reference: `relay/internal/server/submit.go:107-112` (post-`adapter.Create` site where `RecordAdapterCall` lands)
- Reference: `relay/cmd/relay/smoke/run-q9-smoke.sh:11-22, 50-77` (M4 subshell cleanup target — every `cd relay && go run ...` becomes `(cd relay && go run ...)`)
- Reference: `.github/workflows/ci.yml` (existing CI shape — single `contract` job; 7-i adds 4 sibling jobs)
- Reference: `scripts/check-pins.sh:30-49` (the `golang.org/x/time` pin-enforcement style — copy verbatim for `prometheus/client_golang`)
- Reference: `core/src/client.ts` (existing two-space-indent style for `.prettierrc` to mirror)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/go.mod` | Modify | Add `github.com/prometheus/client_golang v1.20.5` to the require block (exact pin) |
| `relay/go.sum` | Modify | `go mod tidy` writes the checksum entries for prometheus + its indirect deps |
| `scripts/check-pins.sh` | Modify | Add a gate that fails if `prometheus/client_golang` is caret/latest-pinned |
| `relay/internal/metrics/metrics.go` | Create | `Registry`, `New`, `Middleware`, `ListenAndServe`, `RecordLLMTokens`, `RecordAdapterCall` + 4 collectors |
| `relay/internal/metrics/metrics_test.go` | Create | Disabled-no-op, enabled-increments-via-testutil, port-conflict-survival, RoutePattern cardinality |
| `relay/internal/config/config.go` | Modify | Add `Config.Observability`, `ObservabilityConfig`, `MetricsConfig`; extend `applyDefaults` |
| `relay/internal/config/config_test.go` | Modify | Tests for `Observability` defaults + explicit values |
| `relay/internal/config/testdata/sample.yaml` | Modify | Append an `observability:` block to the sample fixture |
| `relay/internal/server/deps.go` | Modify | Add `Metrics *metrics.Registry` field at the end of `Deps` |
| `relay/cmd/relay/main.go` | Modify | Refactor `buildRegistry` (FOLLOWUPS I1); extract `accumulateStartupProblems` (FOLLOWUPS I2); short-circuit `validateAttachments` on disabled (FOLLOWUPS M2); wire metrics server in goroutine |
| `relay/cmd/relay/main_test.go` | Modify | New `TestAccumulateStartupProblems_*` cases (7 sub-tests) |
| `relay/internal/server/turn.go` | Modify | One-line additive `deps.Metrics.RecordLLMTokens` at the SSEDone branch |
| `relay/internal/server/turn_test.go` | Modify | `TestTurnHandler_RecordsLLMTokensOnSSEDone` |
| `relay/internal/server/submit.go` | Modify | One-block additive `deps.Metrics.RecordAdapterCall` after `adapter.Create` (success and error paths) |
| `relay/internal/server/submit_test.go` | Modify | `TestSubmitHandler_RecordsAdapterCallSuccess`, `TestSubmitHandler_RecordsAdapterCallError` |
| `relay/internal/server/server.go` | Modify | Prepend `deps.Metrics.Middleware()` to the `/v1/intake` chain BEFORE `perIPLimitMiddleware` |
| `relay/internal/server/server_test.go` | Modify | `TestServer_MetricsMiddlewareCountsRateLimited429s` (counts the 429 from the per-IP limiter) |
| `relay/cmd/relay/smoke/run-q9-smoke.sh` | Modify | FOLLOWUPS M4 — wrap every `cd relay && go run ...` in `(cd relay && go run ...)` subshells |
| `.golangci.yml` | Create | Curated linter set: errcheck, govet, staticcheck, gosec, ineffassign, unused |
| `.eslintrc.cjs` | Create | `eslint-plugin-vue` recommended + `@typescript-eslint/recommended` strict |
| `.prettierrc` | Create | Two-space indent, single-quote, no trailing comma in objects, semi true |
| `.eslintignore` | Create | dist, node_modules, generated codegen output, local-dev |
| `.prettierignore` | Create | Same allowlist plus `*.generated.*` |
| `core/package.json` | Modify | Add `eslint`, `prettier`, `@typescript-eslint/*` devDependencies (exact-pinned) |
| `vue/package.json` | Modify | Add `eslint`, `eslint-plugin-vue`, `prettier`, `@typescript-eslint/*` devDependencies (exact-pinned) |
| `package.json` (repo root) | Modify | Add top-level `prettier` devDependency + `lint:prettier` script |
| Any source files surfaced by the initial-fix sweep | Modify | Triage + fix per design spec §3.3 (real bug → fix; false positive → targeted `//nolint`/`eslint-disable-next-line`; style preference → reformat) |
| `.github/workflows/ci.yml` | Modify | Add four new jobs: `lint-go`, `lint-ts`, `test-go`, `test-ts` (each tool exact-pinned) |

---

## Tasks

### Task 1: Pin `prometheus/client_golang` exact + extend `scripts/check-pins.sh`

**Files:** Modify `relay/go.mod`, `relay/go.sum`, `scripts/check-pins.sh`

- [ ] **Step 1: Verify the latest stable release**

Open `https://pkg.go.dev/github.com/prometheus/client_golang?tab=versions` in a browser (or `curl https://api.github.com/repos/prometheus/client_golang/releases/latest`) and read the latest stable tag as of 2026-06-01. Expected: `v1.20.5` (verify; if a later v1.20.x patch exists, use that — the API surface used by 7-i is stable across the v1.20.x line). Record the exact version chosen in this checkbox; subsequent steps reference `PROM_VERSION` as a placeholder.

- [ ] **Step 2: Add the exact-pinned require entry**

```bash
cd relay
go get github.com/prometheus/client_golang@v1.20.5  # use the version verified in Step 1
cd ..
```

Open `relay/go.mod`. Confirm the new line inside `require ( ... )` reads exactly:

```
github.com/prometheus/client_golang v1.20.5
```

NOT `^1.20.5`, NOT `latest`. The line lives in the FIRST `require` block (direct deps), not the `// indirect` block.

- [ ] **Step 3: Run `go mod tidy` and inspect the diff**

```bash
cd relay
go mod tidy
cd ..
git diff --stat relay/go.mod relay/go.sum
```

Expected: `relay/go.mod` gains one line in the direct-require block (prometheus) plus several indirect lines (`prometheus/client_model`, `prometheus/common`, `prometheus/procfs`, possibly `beorn7/perks`, `cespare/xxhash`, `golang/protobuf`, `google.golang.org/protobuf` already present). `relay/go.sum` gains 10–20 checksum lines. NO unexpected upgrades to existing pinned deps. If any existing direct-require version changed, revert that change manually — `go mod tidy` should add prometheus only.

- [ ] **Step 4: Extend `scripts/check-pins.sh` with the prometheus gate**

In `scripts/check-pins.sh`, immediately AFTER the `golang.org/x/time` block (line 49), INSERT this exact block (mirrors the Phase 5 style verbatim):

```bash
# Gate: github.com/prometheus/client_golang must be exact-pinned (no caret, no @latest) in go.mod. Phase 7.
if grep -E 'prometheus/client_golang' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/prometheus/client_golang is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
```

- [ ] **Step 5: Verify the gate works (negative test, then revert)**

Edit `relay/go.mod` to temporarily change the prometheus line to `github.com/prometheus/client_golang ^v1.20.5`. Run `bash scripts/check-pins.sh`. Expected: exit code 1 with the new ERROR line on stderr. Revert the edit. Re-run `bash scripts/check-pins.sh`. Expected: exit 0 with `OK: all codegen tools are exact-pinned`.

- [ ] **Step 6: Commit**

```bash
git add relay/go.mod relay/go.sum scripts/check-pins.sh
git commit -m "$(cat <<'EOF'
feat(7-i): pin prometheus/client_golang v1.20.5 + extend check-pins.sh

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Create the `metrics` package — `metrics.go` + `metrics_test.go`

**Files:** Create `relay/internal/metrics/metrics.go`, `relay/internal/metrics/metrics_test.go`

- [ ] **Step 1: Write the failing tests**

Create `relay/internal/metrics/metrics_test.go`:

```go
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

	"intake/internal/metrics"
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
```

- [ ] **Step 2: Run to verify the tests fail to compile**

Run: `cd relay && go test ./internal/metrics/... -v && cd ..`
Expected: FAIL — `metrics: package not found`.

- [ ] **Step 3: Create `metrics.go`**

Create `relay/internal/metrics/metrics.go`:

```go
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
	"github.com/prometheus/client_golang/promhttp"
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
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/metrics/... -v && cd ..`
Expected: all six sub-tests pass.

NOTE: The port-conflict test is platform-sensitive. On Linux the error message is `address already in use`; on Windows it is `Only one usage of each socket address is normally permitted`. The test's `strings.Contains` check covers both.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/metrics/
git commit -m "$(cat <<'EOF'
feat(7-i): metrics package — Registry, 4 collectors, passthrough on disabled

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Extend `config.go` with `ObservabilityConfig` + defaults

**Files:** Modify `relay/internal/config/config.go`, `relay/internal/config/config_test.go`, `relay/internal/config/testdata/sample.yaml`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/config/config_test.go` (after the last existing test):

```go
func TestLoad_AppliesPhase7DefaultsForObservability(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Observability.LogLevel != "info" {
		t.Errorf("default LogLevel = %q; want \"info\"", cfg.Observability.LogLevel)
	}
	if cfg.Observability.LogFormat != "json" {
		t.Errorf("default LogFormat = %q; want \"json\"", cfg.Observability.LogFormat)
	}
	if cfg.Observability.Metrics.Enabled {
		t.Error("default Metrics.Enabled = true; want false (off-by-default invariant)")
	}
	if cfg.Observability.Metrics.Addr != ":9090" {
		t.Errorf("default Metrics.Addr = %q; want \":9090\"", cfg.Observability.Metrics.Addr)
	}
}

func TestLoad_ExplicitObservabilityHonored(t *testing.T) {
	tmp := t.TempDir() + "/observ.yaml"
	body := []byte("observability:\n  log_level: \"debug\"\n  log_format: \"text\"\n  metrics:\n    enabled: true\n    addr: \":12345\"\n")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	cfg, err := config.Load(tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Observability.LogLevel != "debug" {
		t.Errorf("LogLevel = %q; want \"debug\"", cfg.Observability.LogLevel)
	}
	if cfg.Observability.LogFormat != "text" {
		t.Errorf("LogFormat = %q; want \"text\"", cfg.Observability.LogFormat)
	}
	if !cfg.Observability.Metrics.Enabled {
		t.Error("Metrics.Enabled = false; want true")
	}
	if cfg.Observability.Metrics.Addr != ":12345" {
		t.Errorf("Metrics.Addr = %q; want \":12345\"", cfg.Observability.Metrics.Addr)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/config/... -run TestLoad_.+Phase7|TestLoad_ExplicitObservability -v && cd ..`
Expected: FAIL — `cfg.Observability undefined`.

- [ ] **Step 3: Add the new structs to `config.go`**

In `relay/internal/config/config.go`, modify the `Config` struct (look for the existing struct around line 13–22) to ADD the new field after `Attachments`:

```go
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	LLM           LLMConfig           `yaml:"llm"`
	Auth          AuthConfig          `yaml:"auth"`
	Adapters      AdaptersConfig      `yaml:"adapters"`
	Routing       RoutingConfig       `yaml:"routing"`
	License       LicenseConfig       `yaml:"license"`
	Captcha       CaptchaConfig       `yaml:"captcha"`        // Phase 5
	RateLimit     RateLimitConfig     `yaml:"ratelimit"`      // Phase 5
	Attachments   AttachmentsConfig   `yaml:"attachments"`    // Phase 6
	Observability ObservabilityConfig `yaml:"observability"`  // Phase 7
}
```

Append the two new types at the bottom of `config.go`, AFTER the Phase 6 attachment types and BEFORE the final `applyDefaults` (or wherever the file ends; placement is non-load-bearing as long as they're at top level):

```go
// ObservabilityConfig configures the relay's structured logging + Prometheus
// metrics surface. Phase 7 (7-i). LogLevel + LogFormat are reserved for v1+
// fine-tuning — Phase 7 wires them as no-ops other than reading the values
// (slog is already constructed with JSON+info before config load).
type ObservabilityConfig struct {
	LogLevel  string        `yaml:"log_level"`  // default "info"
	LogFormat string        `yaml:"log_format"` // default "json"
	Metrics   MetricsConfig `yaml:"metrics"`
}

// MetricsConfig controls the optional Prometheus metrics endpoint. OFF by
// default per the off-by-default observability invariant — operators opt in.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"` // default false (off-by-default invariant)
	Addr    string `yaml:"addr"`    // default ":9090"
}
```

In `applyDefaults`, append at the END (after the Phase 6 attachment defaults block):

```go
	// Phase 7 observability defaults
	if c.Observability.LogLevel == "" {
		c.Observability.LogLevel = "info"
	}
	if c.Observability.LogFormat == "" {
		c.Observability.LogFormat = "json"
	}
	if c.Observability.Metrics.Addr == "" {
		c.Observability.Metrics.Addr = ":9090"
	}
	// Metrics.Enabled defaults to the Go zero (false), which is exactly the
	// off-by-default invariant. No explicit set needed.
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/config/... -v && cd ..`
Expected: all existing Phase 1+4+5+6 tests + the two new Phase 7 tests pass.

- [ ] **Step 5: Extend `testdata/sample.yaml`**

Read the existing `relay/internal/config/testdata/sample.yaml`. APPEND (do not replace) the new block (2-space YAML indent):

```yaml
observability:
  log_level: "info"
  log_format: "json"
  metrics:
    enabled: false
    addr: ":9090"
```

Add a sample-parsing test to `config_test.go`:

```go
func TestLoad_ParsesSampleYAMLPhase7ObservabilityBlock(t *testing.T) {
	cfg, err := config.Load("testdata/sample.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Observability.LogLevel != "info" {
		t.Errorf("sample.yaml log_level = %q; want info", cfg.Observability.LogLevel)
	}
	if cfg.Observability.Metrics.Enabled {
		t.Error("sample.yaml metrics.enabled true; want false")
	}
	if cfg.Observability.Metrics.Addr != ":9090" {
		t.Errorf("sample.yaml metrics.addr = %q; want :9090", cfg.Observability.Metrics.Addr)
	}
}
```

Run: `cd relay && go test ./internal/config/... -v && cd ..`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add relay/internal/config/config.go relay/internal/config/config_test.go relay/internal/config/testdata/sample.yaml
git commit -m "$(cat <<'EOF'
feat(7-i): ObservabilityConfig + MetricsConfig + defaults + sample.yaml block

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Extend `server.Deps` with `Metrics *metrics.Registry`

**Files:** Modify `relay/internal/server/deps.go`

- [ ] **Step 1: Add the new field**

In `relay/internal/server/deps.go`, add the `metrics` import to the import block:

```go
import (
	"log/slog"
	"net/netip"

	"intake/internal/auth"
	"intake/internal/budget"
	"intake/internal/captcha"
	"intake/internal/classify"
	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/metrics"
	"intake/internal/payloadbuild"
	"intake/internal/ratelimit/perip"
	"intake/internal/router"
	"intake/internal/version"
)
```

Append at the END of the `Deps` struct (after `BodyCapBytes`, the last Phase 6 field around line 117):

```go
	// from 7-i (Phase 7):

	// Metrics is the Prometheus metrics registry. nil-safe: when main.go
	// populates this from a disabled config, the *Registry's Middleware()
	// returns a literal passthrough and Record* methods are no-ops, so all
	// existing tests that construct Deps{} without setting this field
	// continue to work without modification.
	Metrics *metrics.Registry
```

- [ ] **Step 2: Build — must pass**

Run: `cd relay && go build ./... && cd ..`
Expected: build passes. Existing tests that construct `Deps{}` literally leave `Metrics` nil; the metrics package's nil-receiver checks (`if r == nil || !r.enabled`) handle this safely.

- [ ] **Step 3: Commit**

```bash
git add relay/internal/server/deps.go
git commit -m "$(cat <<'EOF'
feat(7-i): Deps gains Metrics *metrics.Registry (nil-safe via passthrough)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: FOLLOWUPS I1 — refactor `buildRegistry` to return `([]adapter.Adapter, []string)`

**Files:** Modify `relay/cmd/relay/main.go`, `relay/cmd/relay/main_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/cmd/relay/main_test.go`:

```go
// TestBuildRegistry_PerAdapterConfigureFailureContributesProblem asserts
// FOLLOWUPS I1: a chatwoot adapter with api_token_env pointing at an unset env
// var produces a problem entry, NOT an os.Exit(1). The function returns the
// registry slice (possibly empty) and the problems slice; the caller decides.
func TestBuildRegistry_PerAdapterConfigureFailureContributesProblem(t *testing.T) {
	cfg := &config.Config{}
	cfg.Adapters.Chatwoot.Enabled = true
	cfg.Adapters.Chatwoot.BaseURL = "https://example.com"
	cfg.Adapters.Chatwoot.AccountID = "1"
	cfg.Adapters.Chatwoot.InboxID = "1"
	cfg.Adapters.Chatwoot.APITokenEnv = "INTAKE_TEST_NEVER_SET_XYZ"
	licState := freeLicenseState(t)
	logger := discardLogger()

	registry, problems := buildRegistry(cfg, licState, logger)
	if len(problems) == 0 {
		t.Fatal("expected at least one problem for chatwoot api_token_env unset; got none")
	}
	foundChatwootProblem := false
	for _, p := range problems {
		if strings.Contains(p, "chatwoot") {
			foundChatwootProblem = true
			break
		}
	}
	if !foundChatwootProblem {
		t.Errorf("expected a problem mentioning chatwoot; got %v", problems)
	}
	// Registry should NOT contain chatwoot (Configure failed) and may be empty.
	for _, ad := range registry {
		if ad.Name() == "chatwoot" {
			t.Error("chatwoot adapter present in registry despite Configure failure")
		}
	}
}

// TestBuildRegistry_NoAdaptersEnabledContributesProblem asserts the second
// FOLLOWUPS I1 case: cfg with NO adapters enabled produces a problem entry,
// not os.Exit(1).
func TestBuildRegistry_NoAdaptersEnabledContributesProblem(t *testing.T) {
	cfg := &config.Config{} // all adapters disabled by default
	licState := freeLicenseState(t)
	logger := discardLogger()

	registry, problems := buildRegistry(cfg, licState, logger)
	if len(registry) != 0 {
		t.Errorf("registry len = %d; want 0", len(registry))
	}
	foundNoAdaptersProblem := false
	for _, p := range problems {
		if strings.Contains(p, "no adapters enabled") {
			foundNoAdaptersProblem = true
			break
		}
	}
	if !foundNoAdaptersProblem {
		t.Errorf("expected a problem mentioning 'no adapters enabled'; got %v", problems)
	}
}

// TestBuildRegistry_FreeAdapterEnabled is the happy-path baseline: webhook
// enabled with valid config → registry has webhook, problems is empty.
func TestBuildRegistry_FreeAdapterEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com/intake"
	licState := freeLicenseState(t)
	logger := discardLogger()

	registry, problems := buildRegistry(cfg, licState, logger)
	if len(problems) != 0 {
		t.Errorf("happy path: problems = %v; want empty", problems)
	}
	if len(registry) != 1 || registry[0].Name() != "webhook" {
		t.Errorf("registry = %v; want [webhook]", adapterListNames(registry))
	}
}

// adapterListNames is a small test helper to extract adapter names for assertion messages.
func adapterListNames(reg []adapter.Adapter) []string {
	out := make([]string, 0, len(reg))
	for _, ad := range reg {
		out = append(out, ad.Name())
	}
	return out
}
```

Add these helpers near the top of `main_test.go` if not already present:

```go
// freeLicenseState returns a license.State in free mode (paid adapters
// silently skipped via the licensed() helper).
func freeLicenseState(t *testing.T) *licensemgr.State {
	t.Helper()
	return &licensemgr.State{Mode: licensemgr.ModeFree, Message: "no license file"}
}

// discardLogger returns a slog.Logger that discards all output (test-only).
func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}
```

Ensure imports include `"io"`, `"log/slog"`, `"strings"`, and `licensemgr "intake/internal/license"`.

- [ ] **Step 2: Run to verify they fail to compile**

Run: `cd relay && go test ./cmd/relay/... -run TestBuildRegistry -v && cd ..`
Expected: FAIL — `buildRegistry returns (map[string]adapter.Adapter, error), not ([]adapter.Adapter, []string)`.

- [ ] **Step 3: Refactor `buildRegistry`**

In `relay/cmd/relay/main.go`, REPLACE the entire `buildRegistry` function (lines ~461–570) with this:

```go
// buildRegistry constructs the set of enabled adapters. Each adapter that
// passes the license gate has its Configure() called; failures contribute to
// the returned problems slice rather than calling os.Exit(1) (FOLLOWUPS I1 —
// closes the L022 contract gap by routing per-adapter Configure failures
// through the same consolidated startup-problems slice as Phase 5 and Phase 6
// gates).
//
// Returns:
//   - []adapter.Adapter — slice (NOT map) of successfully-configured adapters;
//     order is webhook, chatwoot, fider, zendesk, linear, matching the order
//     adapters are tried below. Caller may convert to a map via router.New.
//   - []string — problems slice. Empty on success. Each entry is self-describing.
//
// "No adapters enabled" is added to the problems slice (NOT os.Exit'd) so it
// composes with Phase 5+6 problems in the consolidated log line.
//
// License-gate "paid adapter without license" stays as slog.Warn (NOT a problem
// entry) — free-mode is a valid operating state per the design spec.
func buildRegistry(cfg *config.Config, licState *licensemgr.State, logger *slog.Logger) ([]adapter.Adapter, []string) {
	var reg []adapter.Adapter
	var problems []string

	// webhook (1-iv) — free.
	if cfg.Adapters.Webhook.Enabled {
		wh := webhook.New()
		if licensed(wh, licState, logger) {
			if err := wh.Configure(map[string]any{
				"url":     cfg.Adapters.Webhook.URL,
				"headers": cfg.Adapters.Webhook.Headers,
				"retry": map[string]any{
					"max_attempts": cfg.Adapters.Webhook.Retry.MaxAttempts,
					"backoff":      cfg.Adapters.Webhook.Retry.Backoff,
				},
			}); err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: %v", wh.Name(), err))
			} else {
				reg = append(reg, wh)
				logger.Info("relay: adapter enabled", "adapter", wh.Name())
			}
		}
	}

	// chatwoot (3-ii) — free.
	if cfg.Adapters.Chatwoot.Enabled {
		cw := chatwoot.New()
		if licensed(cw, licState, logger) {
			token, err := config.RequireSecret(cfg.Adapters.Chatwoot.APITokenEnv)
			if err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: api_token_env=%q: %v", cw.Name(), cfg.Adapters.Chatwoot.APITokenEnv, err))
			} else if err := cw.Configure(map[string]any{
				"base_url":   cfg.Adapters.Chatwoot.BaseURL,
				"account_id": cfg.Adapters.Chatwoot.AccountID,
				"inbox_id":   cfg.Adapters.Chatwoot.InboxID,
				"api_token":  token,
			}); err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: %v", cw.Name(), err))
			} else {
				reg = append(reg, cw)
				logger.Info("relay: adapter enabled", "adapter", cw.Name())
			}
		}
	}

	// fider (3-iii) — free.
	if cfg.Adapters.Fider.Enabled {
		fd := fider.New()
		if licensed(fd, licState, logger) {
			key, err := config.RequireSecret(cfg.Adapters.Fider.APIKeyEnv)
			if err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: api_key_env=%q: %v", fd.Name(), cfg.Adapters.Fider.APIKeyEnv, err))
			} else if err := fd.Configure(map[string]any{
				"base_url": cfg.Adapters.Fider.BaseURL,
				"api_key":  key,
			}); err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: %v", fd.Name(), err))
			} else {
				reg = append(reg, fd)
				logger.Info("relay: adapter enabled", "adapter", fd.Name())
			}
		}
	}

	// zendesk (3-iv) — PAID; gated generically via RequiresLicense().
	if cfg.Adapters.Zendesk.Enabled {
		zd := zendesk.New()
		if licensed(zd, licState, logger) {
			token, err := config.RequireSecret(cfg.Adapters.Zendesk.APITokenEnv)
			if err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: api_token_env=%q: %v", zd.Name(), cfg.Adapters.Zendesk.APITokenEnv, err))
			} else if err := zd.Configure(map[string]any{
				"subdomain":        cfg.Adapters.Zendesk.Subdomain,
				"email":            cfg.Adapters.Zendesk.Email,
				"api_token":        token,
				"default_priority": cfg.Adapters.Zendesk.DefaultPriority,
			}); err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: %v", zd.Name(), err))
			} else {
				reg = append(reg, zd)
				logger.Info("relay: adapter enabled", "adapter", zd.Name())
			}
		}
	}

	// linear (3-v) — PAID; gated generically via RequiresLicense().
	if cfg.Adapters.Linear.Enabled {
		ln := linear.New()
		if licensed(ln, licState, logger) {
			key, err := config.RequireSecret(cfg.Adapters.Linear.APIKeyEnv)
			if err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: api_key_env=%q: %v", ln.Name(), cfg.Adapters.Linear.APIKeyEnv, err))
			} else if err := ln.Configure(map[string]any{
				"api_key": key,
				"team_id": cfg.Adapters.Linear.TeamID,
			}); err != nil {
				problems = append(problems, fmt.Sprintf("adapter %q: %v", ln.Name(), err))
			} else {
				reg = append(reg, ln)
				logger.Info("relay: adapter enabled", "adapter", ln.Name())
			}
		}
	}

	// "No adapters enabled" is a problem (not os.Exit) so it composes with
	// Phase 5+6 problems in the consolidated log line.
	if len(reg) == 0 {
		problems = append(problems, "no adapters enabled — enable at least one in config.adapters")
	}

	return reg, problems
}
```

Update the callsite in `main()` (the section between current lines ~93–123) — this is replaced wholesale in Task 6's `accumulateStartupProblems` extraction, but Task 5 leaves a temporary intermediate form. For now, REPLACE lines 93–101 (`registry, err := buildRegistry(...)` through the `if len(registry) == 0` block) with:

```go
	// FOLLOWUPS I1 (Phase 7): buildRegistry returns problems slice instead of
	// os.Exit. Append into the shared `problems` slice; consolidated exit fires
	// after all gates have run (the original code path further down is replaced
	// by Task 6's accumulateStartupProblems extraction).
	registry, regProblems := buildRegistry(cfg, licState, logger)
	problems = append(problems, regProblems...)
```

The downstream `enabledList` construction (current lines 109–112) also needs updating because `registry` is now `[]adapter.Adapter`, not `map[string]adapter.Adapter`. REPLACE:

```go
	enabledList := make([]adapter.Adapter, 0, len(registry))
	for _, ad := range registry {
		enabledList = append(enabledList, ad)
	}
	attachmentsCfg, attProblems := validateAttachments(cfg, enabledList)
```

WITH:

```go
	attachmentsCfg, attProblems := validateAttachments(cfg, registry)
```

Update `validateAttachments`'s signature in this same file to accept `[]adapter.Adapter` directly (it already iterates as a slice; this is essentially a no-op refactor):

```go
func validateAttachments(cfg *config.Config, enabled []adapter.Adapter) (config.AttachmentsConfig, []string) {
```

(The current signature already matches — no change. The change is purely at the call site that was building `enabledList` from the map.)

Similarly, `ComputeAttachmentsCaps` (called next, current line 128) takes `[]adapter.Adapter` — no change needed.

`router.New` (current line 287) takes a `map[string]adapter.Adapter` and a default-adapter name. Add a tiny helper at the bottom of `main.go` to convert the slice back to a map for `router.New`:

```go
// adapterRegistryFromSlice (already exists from Phase 6 — reused here) converts
// []adapter.Adapter to map[string]adapter.Adapter for callers (router.New)
// that consume the map shape. Confirmed in the codebase; no new function.
```

Actually the existing `adapterRegistryFromSlice` (current `main.go` lines ~722–728) does exactly this. Reuse it at the `router.New` callsite:

```go
	rtr, err := router.New(adapterRegistryFromSlice(registry), rules, cfg.Routing.DefaultAdapter, logger)
```

(Replace the existing `router.New(registry, ...)` with this; the conversion is O(N) and runs once at startup.)

Also: the old `adapterNames(registry)` log line at the end of router setup needs to change because `registry` is no longer a map. The simplest fix is to use the existing `adapterRegistryFromSlice` helper there too:

```go
	logger.Info("relay: router ready", "default_adapter", cfg.Routing.DefaultAdapter, "adapters", adapterNames(adapterRegistryFromSlice(registry)))
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./cmd/relay/... -run TestBuildRegistry -v && cd ..`
Expected: all three Task 5 tests pass.

Run: `cd relay && go build ./... && cd ..`
Expected: build passes.

- [ ] **Step 5: Commit**

```bash
git add relay/cmd/relay/main.go relay/cmd/relay/main_test.go
git commit -m "$(cat <<'EOF'
fix(7-i): FOLLOWUPS I1 — buildRegistry returns ([]Adapter, []string), no os.Exit

Per-adapter Configure failures, secret-resolution failures, and the
"no adapters enabled" case now contribute to the shared problems slice
instead of calling os.Exit(1) inline. Closes the L022 contract gap.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: FOLLOWUPS I2 — extract `accumulateStartupProblems` from `main()`

**Files:** Modify `relay/cmd/relay/main.go`, `relay/cmd/relay/main_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/cmd/relay/main_test.go`:

```go
// TestAccumulateStartupProblems_Empty: clean cfg + free license + no enabled
// adapters → 1 problem ("no adapters enabled"). Asserts the function is pure
// (no os.Exit) and the Deps return value is populated for the fields that
// don't depend on a registry.
func TestAccumulateStartupProblems_Empty(t *testing.T) {
	cfg := minimalValidCfg(t) // helper builds the smallest cfg that parses cleanly
	licState := freeLicenseState(t)
	logger := discardLogger()

	deps, problems := accumulateStartupProblems(cfg, licState, logger)
	// "no adapters enabled" is the one expected problem.
	if len(problems) != 1 {
		t.Errorf("len(problems) = %d; want 1; got %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "no adapters enabled") {
		t.Errorf("expected 'no adapters enabled'; got %v", problems)
	}
	// Metrics is populated (nil-safe disabled mode is fine).
	if deps.Metrics == nil {
		t.Error("Deps.Metrics is nil; want non-nil even in disabled mode")
	}
}

// TestAccumulateStartupProblems_Phase5Only: anonymous-no-captcha + bad CIDR +
// at least one valid adapter → exactly 2 Phase-5 problems, NO Phase-7
// (registry) problems.
func TestAccumulateStartupProblems_Phase5Only(t *testing.T) {
	cfg := minimalValidCfg(t)
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	cfg.Server.TrustedProxies = []string{"not-a-cidr"}
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com"
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, problems := accumulateStartupProblems(cfg, licState, logger)
	hasAnon := false
	hasCIDR := false
	for _, p := range problems {
		if strings.Contains(p, "anonymous") {
			hasAnon = true
		}
		if strings.Contains(p, "not-a-cidr") {
			hasCIDR = true
		}
	}
	if !hasAnon {
		t.Errorf("expected an 'anonymous' problem; got %v", problems)
	}
	if !hasCIDR {
		t.Errorf("expected a 'not-a-cidr' problem; got %v", problems)
	}
}

// TestAccumulateStartupProblems_Phase6Only: storage.mode=s3 + cap inverted +
// webhook enabled (so Phase 7 contributes nothing) → exactly 2 Phase-6 problems.
func TestAccumulateStartupProblems_Phase6Only(t *testing.T) {
	cfg := minimalValidCfg(t)
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com"
	cfg.Attachments.Enabled = true
	cfg.Attachments.Storage.Mode = "s3"
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, problems := accumulateStartupProblems(cfg, licState, logger)
	hasStorage := false
	hasCapInverted := false
	for _, p := range problems {
		if strings.Contains(p, "storage.mode") {
			hasStorage = true
		}
		if strings.Contains(p, "max_size_bytes") {
			hasCapInverted = true
		}
	}
	if !hasStorage {
		t.Errorf("expected a 'storage.mode' problem; got %v", problems)
	}
	if !hasCapInverted {
		t.Errorf("expected a 'max_size_bytes' problem; got %v", problems)
	}
}

// TestAccumulateStartupProblems_AdapterConfigureFails: chatwoot api_token_env
// unset → 1 problem mentioning chatwoot, NO "no adapters enabled" (because
// webhook is also enabled and configures cleanly).
func TestAccumulateStartupProblems_AdapterConfigureFails(t *testing.T) {
	cfg := minimalValidCfg(t)
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com"
	cfg.Adapters.Chatwoot.Enabled = true
	cfg.Adapters.Chatwoot.BaseURL = "https://chat.example.com"
	cfg.Adapters.Chatwoot.AccountID = "1"
	cfg.Adapters.Chatwoot.InboxID = "1"
	cfg.Adapters.Chatwoot.APITokenEnv = "INTAKE_TEST_NEVER_SET_XYZ"
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, problems := accumulateStartupProblems(cfg, licState, logger)
	hasChatwoot := false
	hasNoAdapters := false
	for _, p := range problems {
		if strings.Contains(p, "chatwoot") {
			hasChatwoot = true
		}
		if strings.Contains(p, "no adapters enabled") {
			hasNoAdapters = true
		}
	}
	if !hasChatwoot {
		t.Errorf("expected a chatwoot problem; got %v", problems)
	}
	if hasNoAdapters {
		t.Errorf("did NOT expect 'no adapters enabled' (webhook is configured); got %v", problems)
	}
}

// TestAccumulateStartupProblems_NoAdaptersEnabled: cfg with all adapters
// disabled → "no adapters enabled" problem.
func TestAccumulateStartupProblems_NoAdaptersEnabled(t *testing.T) {
	cfg := minimalValidCfg(t)
	// no adapter enabled
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, problems := accumulateStartupProblems(cfg, licState, logger)
	hasNoAdapters := false
	for _, p := range problems {
		if strings.Contains(p, "no adapters enabled") {
			hasNoAdapters = true
		}
	}
	if !hasNoAdapters {
		t.Errorf("expected 'no adapters enabled' problem; got %v", problems)
	}
}

// TestAccumulateStartupProblems_LicenseGateWarnsNotFails: a PAID adapter
// (zendesk) enabled in FREE mode → registry skips it via licensed() with a
// Warn log; NO problem entry contributed. With webhook also enabled, the
// final registry has webhook only, and problems is empty.
func TestAccumulateStartupProblems_LicenseGateWarnsNotFails(t *testing.T) {
	cfg := minimalValidCfg(t)
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com"
	cfg.Adapters.Zendesk.Enabled = true
	cfg.Adapters.Zendesk.Subdomain = "example"
	cfg.Adapters.Zendesk.Email = "agent@example.com"
	cfg.Adapters.Zendesk.APITokenEnv = "INTAKE_TEST_NEVER_SET_XYZ"
	licState := freeLicenseState(t) // free mode → zendesk skipped (paid)
	logger := discardLogger()

	_, problems := accumulateStartupProblems(cfg, licState, logger)
	for _, p := range problems {
		if strings.Contains(p, "zendesk") {
			t.Errorf("did NOT expect a zendesk problem (license gate is a Warn, not a problem); got %v", problems)
		}
	}
	if len(problems) != 0 {
		t.Errorf("expected zero problems (webhook configures, zendesk skipped by license); got %v", problems)
	}
}

// TestAccumulateStartupProblems_AllCombined: Phase 5 (anon-no-captcha + bad
// CIDR) + Phase 6 (cap inverted) + Phase 7 (chatwoot Configure failure) all
// in ONE cfg → consolidated problems slice contains every distinct issue,
// with count >= 4. Asserts the L022 contract end-to-end.
func TestAccumulateStartupProblems_AllCombined(t *testing.T) {
	cfg := minimalValidCfg(t)
	// Phase 5: anon-no-captcha
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	// Phase 5: bad CIDR
	cfg.Server.TrustedProxies = []string{"not-a-cidr"}
	// Phase 6: cap inverted
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000
	// Phase 7: chatwoot Configure fails
	cfg.Adapters.Chatwoot.Enabled = true
	cfg.Adapters.Chatwoot.BaseURL = "https://chat.example.com"
	cfg.Adapters.Chatwoot.AccountID = "1"
	cfg.Adapters.Chatwoot.InboxID = "1"
	cfg.Adapters.Chatwoot.APITokenEnv = "INTAKE_TEST_NEVER_SET_XYZ"
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, problems := accumulateStartupProblems(cfg, licState, logger)
	if len(problems) < 4 {
		t.Errorf("AllCombined: len(problems) = %d; want >= 4; got %v", len(problems), problems)
	}
	want := []string{"anonymous", "not-a-cidr", "max_size_bytes", "chatwoot"}
	for _, w := range want {
		found := false
		for _, p := range problems {
			if strings.Contains(p, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a problem containing %q; got %v", w, problems)
		}
	}
}
```

Add the `minimalValidCfg` helper near the top of `main_test.go`:

```go
// minimalValidCfg returns the smallest *config.Config that's parseable but
// has no adapters enabled and no Phase 5/6 misconfigs. Tests selectively
// flip fields to assert per-gate behavior.
func minimalValidCfg(t *testing.T) *config.Config {
	t.Helper()
	c := &config.Config{}
	// Apply the same defaults the YAML loader would apply (durations etc.).
	c.RateLimit.PerSession.SessionTTL = "1h"
	c.RateLimit.PerIP.IdleTTL = "15m"
	return c
}
```

- [ ] **Step 2: Run to verify they fail to compile**

Run: `cd relay && go test ./cmd/relay/... -run TestAccumulateStartupProblems -v && cd ..`
Expected: FAIL — `accumulateStartupProblems undefined`.

- [ ] **Step 3: Extract `accumulateStartupProblems` from `main()`**

Add `import "intake/internal/metrics"` and `import "intake/internal/server"` (the latter is already imported as `"intake/internal/server"`; confirm) at the top of `main.go`.

INSERT this function in `main.go` BETWEEN `validateAttachments` (currently ~lines 671–717) and `adapterRegistryFromSlice` (currently ~lines 720–728):

```go
// accumulateStartupProblems runs all startup gates (Phase 5 startupProblems,
// Phase 7-i refactored buildRegistry, Phase 6 validateAttachments) and
// returns the consolidated Deps + problems slice. PURE FUNCTION: no os.Exit,
// no slog beyond the gates' own internal Info/Warn calls. Phase 6's L022
// contract is honored at the single call site in main(): ONE consolidated
// log line, ONE os.Exit(1).
//
// Returns:
//   - server.Deps with all fields the gates produce: TrustedProxies, Registry,
//     AttachmentsCfg, AttachmentMIMEs, BodyCapBytes, Metrics. Fields the
//     caller (main) populates separately — Provider, Auth, EmailService,
//     Captcha*, Budget, PerIP, etc. — are left as Go zero values.
//   - []string — the consolidated problems slice. Empty on success.
//
// Closes FOLLOWUPS I2: the cross-phase wiring is now unit-testable via
// the TestAccumulateStartupProblems_* suite, NOT only via the shell smoke.
func accumulateStartupProblems(
	cfg *config.Config,
	licState *licensemgr.State,
	logger *slog.Logger,
) (server.Deps, []string) {
	var problems []string

	// 1. Phase 5 gate (anonymous/SSO/CIDR/budget/durations).
	p5Problems, trustedProxies := startupProblems(cfg)
	problems = append(problems, p5Problems...)

	// 2. FOLLOWUPS I1: buildRegistry returns problems (NOT os.Exit) for
	//    per-adapter Configure failures + "no adapters enabled".
	registry, regProblems := buildRegistry(cfg, licState, logger)
	problems = append(problems, regProblems...)

	// 3. Phase 6 attachments gate (with M2 zero-value-on-disabled — see Task 7).
	attachmentsCfg, attProblems := validateAttachments(cfg, registry)
	problems = append(problems, attProblems...)

	// 4. Compute derived Deps values.
	attCaps := server.ComputeAttachmentsCaps(attachmentsCfg, registry)
	var attachmentMIMEs []string
	if attCaps != nil {
		attachmentMIMEs = attCaps.AllowedMIMETypes
	}
	bodyCapBytes := int64(1 << 20)
	if attachmentsCfg.Enabled {
		bodyCapBytes = 14 * (1 << 20)
	}
	metricsReg := metrics.New(cfg.Observability.Metrics.Enabled, cfg.Observability.Metrics.Addr)

	return server.Deps{
		// Gate-produced fields. Caller fills in the rest.
		TrustedProxies:  trustedProxies,
		AttachmentsCfg:  attachmentsCfg,
		AttachmentMIMEs: attachmentMIMEs,
		BodyCapBytes:    bodyCapBytes,
		Metrics:         metricsReg,
	}, problems
}
```

Now REWRITE the orchestration block in `main()` (currently lines ~74–138) — REPLACE this entire block:

```go
	// --- Q9 consolidated startup gate (Phase 5) ---
	// ... (the existing 3-gate dance from current main.go) ...
	problems, trustedProxies := startupProblems(cfg)
	// ...
	registry, regProblems := buildRegistry(cfg, licState, logger)
	problems = append(problems, regProblems...)
	// ...
	attachmentsCfg, attProblems := validateAttachments(cfg, registry)
	problems = append(problems, attProblems...)
	if len(problems) > 0 {
		logger.Error("relay: startup config errors", "count", len(problems), "problems", problems)
		os.Exit(1)
	}
	// ... attCaps, attachmentMIMEs, bodyCapBytes computation ...
```

WITH this consolidated form:

```go
	// --- Consolidated Q9 startup gate (Phase 5+6+7, FOLLOWUPS I1+I2) ---
	// One call collects all startup misconfigs across every subsystem; one
	// consolidated log line lists every problem; one os.Exit(1).
	startupDeps, problems := accumulateStartupProblems(cfg, licState, logger)
	if len(problems) > 0 {
		logger.Error("relay: startup config errors", "count", len(problems), "problems", problems)
		os.Exit(1)
	}

	// Re-derive the registry for downstream consumers (router.New) that
	// accept the map shape. buildRegistry ran inside accumulateStartupProblems
	// — we need its result here. Cleanest factoring: have accumulateStartupProblems
	// also return the registry slice. Refactor:
```

Adjust `accumulateStartupProblems` to ALSO return the registry slice for downstream consumers:

```go
func accumulateStartupProblems(
	cfg *config.Config,
	licState *licensemgr.State,
	logger *slog.Logger,
) (server.Deps, []adapter.Adapter, []string) {
	// ... same body ...
	return server.Deps{
		TrustedProxies:  trustedProxies,
		AttachmentsCfg:  attachmentsCfg,
		AttachmentMIMEs: attachmentMIMEs,
		BodyCapBytes:    bodyCapBytes,
		Metrics:         metricsReg,
	}, registry, problems
}
```

(Update the test calls — the seven `TestAccumulateStartupProblems_*` tests now unpack three return values: `_, _, problems := accumulateStartupProblems(...)` for tests that only assert on problems; `deps, registry, problems := ...` for tests that assert on Deps or registry.)

Now in `main()`, the consolidated form becomes:

```go
	startupDeps, registry, problems := accumulateStartupProblems(cfg, licState, logger)
	if len(problems) > 0 {
		logger.Error("relay: startup config errors", "count", len(problems), "problems", problems)
		os.Exit(1)
	}
```

`startupDeps` carries `TrustedProxies`, `AttachmentsCfg`, `AttachmentMIMEs`, `BodyCapBytes`, `Metrics`. The remainder of `main()` continues to construct LLM provider, captcha, store, etc. and then merges into the final `deps`:

```go
	deps := server.Deps{
		Version:      version.Info(),
		CORSOrigins:  cfg.Server.CORSOrigins,
		Logger:       logger,
		Auth:         middleware,
		Provider:     provider,
		SystemPrompt: systemPrompt,
		Model:        model,
		MaxTokens:    maxTokens,
		Router:       rtr,
		Classifier:   classifier,
		Builder:      builder,
		AuthCfg:      cfg.Auth,
		EmailService: emailSvc,

		// Phase 5
		CaptchaCfg:      cfg.Captcha,
		CaptchaVerifier: captchaVerifier,
		Budget:          budgetTracker,
		PerIP:           perIPLimiter,

		// Carried forward from startupDeps:
		TrustedProxies:  startupDeps.TrustedProxies,
		AttachmentsCfg:  startupDeps.AttachmentsCfg,
		AttachmentMIMEs: startupDeps.AttachmentMIMEs,
		BodyCapBytes:    startupDeps.BodyCapBytes,
		Metrics:         startupDeps.Metrics,
	}
```

After `deps` is built but BEFORE the main `srv.ListenAndServe()` call, INSERT the metrics server goroutine:

```go
	// Phase 7 (7-i): metrics server runs in a goroutine, independent of the
	// main HTTP server. A port-bind failure is logged at Error but does NOT
	// crash the main relay (independence invariant — observability shouldn't
	// be able to brick the service it observes).
	metricsCtx, cancelMetrics := context.WithCancel(context.Background())
	defer cancelMetrics()
	go func() {
		if err := deps.Metrics.ListenAndServe(metricsCtx); err != nil {
			logger.Error("metrics: ListenAndServe failed", "err", err)
			// Main relay continues.
		}
	}()
```

(The existing `context` import is already present; confirm.)

Update `router.New` (currently around line 287) — it still receives the registry. Since `registry` is now `[]adapter.Adapter`, use `adapterRegistryFromSlice`:

```go
	rtr, err := router.New(adapterRegistryFromSlice(registry), rules, cfg.Routing.DefaultAdapter, logger)
```

Update the `logger.Info("relay: router ready", ...)` line similarly:

```go
	logger.Info("relay: router ready", "default_adapter", cfg.Routing.DefaultAdapter, "adapters", adapterNames(adapterRegistryFromSlice(registry)))
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./cmd/relay/... -run TestAccumulateStartupProblems -v && cd ..`
Expected: all seven sub-tests pass.

Run: `cd relay && go test ./cmd/relay/... -v && cd ..`
Expected: every Phase 4+5+6 main_test.go test continues to pass.

Run: `cd relay && go build ./... && cd ..`
Expected: build passes.

- [ ] **Step 5: Commit**

```bash
git add relay/cmd/relay/main.go relay/cmd/relay/main_test.go
git commit -m "$(cat <<'EOF'
fix(7-i): FOLLOWUPS I2 — extract accumulateStartupProblems for unit testability

Cross-phase startup-gate wiring (Phase 5 + Phase 6 + Phase 7) lives in a
pure function returning (Deps, []Adapter, []string). main() becomes:
1 call → 1 log line → 1 os.Exit. Seven new TestAccumulateStartupProblems_*
cases cover Empty, Phase5Only, Phase6Only, AdapterConfigureFails,
NoAdaptersEnabled, LicenseGateWarnsNotFails, AllCombined.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: FOLLOWUPS M2 — `validateAttachments` returns zero-value when `Enabled=false`

**Files:** Modify `relay/cmd/relay/main.go`, `relay/cmd/relay/main_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/cmd/relay/main_test.go`:

```go
// TestValidateAttachments_DisabledReturnsZeroValue asserts FOLLOWUPS M2:
// when cfg.Attachments.Enabled=false, validateAttachments returns a
// zero-value AttachmentsConfig (NOT cfg.Attachments) so a bad Storage.Mode
// or inverted caps in the disabled block can't leak to a future consumer.
func TestValidateAttachments_DisabledReturnsZeroValue(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = false
	cfg.Attachments.MaxSizeBytes = 99 // garbage value
	cfg.Attachments.MaxTotalBytes = 1 // would normally trigger cap-inverted gate
	cfg.Attachments.AllowedMIMETypes = []string{"junk/type"}
	cfg.Attachments.Storage.Mode = "s3" // would normally fail gate

	parsed, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("disabled path produced problems %v; want none", problems)
	}
	if parsed.Enabled {
		t.Error("parsed.Enabled = true; want false")
	}
	if parsed.MaxSizeBytes != 0 {
		t.Errorf("M2: parsed.MaxSizeBytes = %d; want 0 (zero-value, garbage 99 must be discarded)", parsed.MaxSizeBytes)
	}
	if parsed.MaxTotalBytes != 0 {
		t.Errorf("M2: parsed.MaxTotalBytes = %d; want 0", parsed.MaxTotalBytes)
	}
	if len(parsed.AllowedMIMETypes) != 0 {
		t.Errorf("M2: parsed.AllowedMIMETypes = %v; want empty", parsed.AllowedMIMETypes)
	}
	if parsed.Storage.Mode != "" {
		t.Errorf("M2: parsed.Storage.Mode = %q; want empty (s3 in disabled block must not leak)", parsed.Storage.Mode)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./cmd/relay/... -run TestValidateAttachments_DisabledReturnsZeroValue -v && cd ..`
Expected: FAIL — current `validateAttachments` returns `parsed` (carrying garbage) when `!parsed.Enabled`.

- [ ] **Step 3: Apply the M2 fix**

In `relay/cmd/relay/main.go`, in the `validateAttachments` function, REPLACE this opening:

```go
func validateAttachments(cfg *config.Config, enabled []adapter.Adapter) (config.AttachmentsConfig, []string) {
	parsed := cfg.Attachments
	if !parsed.Enabled {
		return parsed, nil
	}
```

WITH:

```go
func validateAttachments(cfg *config.Config, enabled []adapter.Adapter) (config.AttachmentsConfig, []string) {
	parsed := cfg.Attachments
	if !parsed.Enabled {
		// FOLLOWUPS M2 (Phase 7-i): return a zero-value AttachmentsConfig so a
		// bad Storage.Mode or inverted caps in the disabled block can't leak
		// to a future consumer. Downstream code already gates on Enabled
		// first; this is defensive future-proofing.
		return config.AttachmentsConfig{}, nil
	}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./cmd/relay/... -v && cd ..`
Expected: TestValidateAttachments_DisabledReturnsZeroValue passes; all other Phase 6 validateAttachments tests still pass (none assert on the disabled-path return value — they all set Enabled=true).

- [ ] **Step 5: Commit**

```bash
git add relay/cmd/relay/main.go relay/cmd/relay/main_test.go
git commit -m "$(cat <<'EOF'
fix(7-i): FOLLOWUPS M2 — validateAttachments returns zero-value when disabled

Bad Storage.Mode or inverted caps in a disabled attachments block no
longer leak to downstream consumers via the returned parsed cfg.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: FOLLOWUPS M4 — `run-q9-smoke.sh` subshell cleanup

**Files:** Modify `relay/cmd/relay/smoke/run-q9-smoke.sh`

- [ ] **Step 1: Apply the subshell refactor**

In `relay/cmd/relay/smoke/run-q9-smoke.sh`, REPLACE this section (around line 22):

```bash
  output=$(cd relay && go run ./cmd/relay --config "../$fixture" 2>&1 || true)
```

WITH:

```bash
  output=$( ( cd relay && go run ./cmd/relay --config "../$fixture" ) 2>&1 || true )
```

REPLACE this section (around line 51):

```bash
combined_output=$(cd relay && go run ./cmd/relay --config "../relay/cmd/relay/smoke/combined.yaml" 2>&1 || true)
```

WITH:

```bash
combined_output=$( ( cd relay && go run ./cmd/relay --config "../relay/cmd/relay/smoke/combined.yaml" ) 2>&1 || true )
```

REPLACE this section (around line 76):

```bash
ac_output=$(cd relay && go run ./cmd/relay --config "../relay/cmd/relay/smoke/attachments-combined.yaml" 2>&1 || true)
```

WITH:

```bash
ac_output=$( ( cd relay && go run ./cmd/relay --config "../relay/cmd/relay/smoke/attachments-combined.yaml" ) 2>&1 || true )
```

The outer `( ... )` makes each `cd relay` execute in a subshell — the parent script's CWD is unaffected and the next `run_misconfig` call doesn't accumulate `cd`s. M4 was about removing the implicit cd-stacking; the explicit subshell makes the intent clear.

- [ ] **Step 2: Re-run the Q9 smoke; output unchanged**

```bash
bash relay/cmd/relay/smoke/run-q9-smoke.sh
```

Expected: ALL existing assertions pass. The combined fixture produces ONE `relay: startup config errors` log line listing every misconfig from every gate (Phase 5+6+7-i). No regression.

- [ ] **Step 3: Commit**

```bash
git add relay/cmd/relay/smoke/run-q9-smoke.sh
git commit -m "$(cat <<'EOF'
fix(7-i): FOLLOWUPS M4 — run-q9-smoke.sh uses subshells for cd relay

Each `cd relay && go run ...` is wrapped in `( ... )` so the parent
script's CWD is unaffected.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Metric-record sites in `turn.go` + `submit.go`

**Files:** Modify `relay/internal/server/turn.go`, `relay/internal/server/turn_test.go`, `relay/internal/server/submit.go`, `relay/internal/server/submit_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/server/turn_test.go`:

```go
// TestTurnHandler_RecordsLLMTokensOnSSEDone asserts that when the LLM
// streams to completion (SSEDone with chunk.Done=true and chunk.Err=nil),
// deps.Metrics.RecordLLMTokens is called with the provider name + "input"
// and "output" directions + the token counts from chunk.
func TestTurnHandler_RecordsLLMTokensOnSSEDone(t *testing.T) {
	reg := metrics.New(true, ":0")
	deps := newTestTurnDeps(t) // helper builds Deps with a fake LLM provider
	deps.Metrics = reg
	// fake provider's Name() returns "fake-provider"; configure it to emit
	// one Delta chunk and one Done chunk with InputTokens=42, OutputTokens=17.
	deps.Provider = &fakeProvider{
		name: "fake-provider",
		chunks: []llm.Chunk{
			{Delta: "hello"},
			{Done: true, InputTokens: 42, OutputTokens: 17},
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	r = r.WithContext(auth.WithContext(r.Context(), auth.SessionContext{SessionID: "ssn-test", AuthMode: "anonymous", Verified: true}))
	w := httptest.NewRecorder()

	turnHandler(deps).ServeHTTP(w, r)

	if got := testutil.ToFloat64(reg.LLMTokensTotalForTest("fake-provider", "input")); got != 42 {
		t.Errorf("input tokens = %v; want 42", got)
	}
	if got := testutil.ToFloat64(reg.LLMTokensTotalForTest("fake-provider", "output")); got != 17 {
		t.Errorf("output tokens = %v; want 17", got)
	}
}

// TestTurnHandler_DoesNotRecordLLMTokensOnError asserts the negative path:
// chunk.Err set → no RecordLLMTokens call.
func TestTurnHandler_DoesNotRecordLLMTokensOnError(t *testing.T) {
	reg := metrics.New(true, ":0")
	deps := newTestTurnDeps(t)
	deps.Metrics = reg
	deps.Provider = &fakeProvider{
		name: "fake-provider",
		chunks: []llm.Chunk{
			{Err: errors.New("upstream timeout")},
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	r = r.WithContext(auth.WithContext(r.Context(), auth.SessionContext{SessionID: "ssn-test", AuthMode: "anonymous", Verified: true}))
	w := httptest.NewRecorder()

	turnHandler(deps).ServeHTTP(w, r)

	if got := testutil.ToFloat64(reg.LLMTokensTotalForTest("fake-provider", "input")); got != 0 {
		t.Errorf("input tokens on error path = %v; want 0", got)
	}
}
```

Add the import `"github.com/prometheus/client_golang/prometheus/testutil"` and `"intake/internal/metrics"` to the test file's import block.

The `fakeProvider` helper may not exist yet in `turn_test.go`. If a similar fake exists, reuse it; otherwise add this minimal version:

```go
type fakeProvider struct {
	name   string
	chunks []llm.Chunk
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Chat(ctx context.Context, msgs []llm.Message, opts llm.ChatOpts) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}
```

Append to `relay/internal/server/submit_test.go`:

```go
// TestSubmitHandler_RecordsAdapterCallSuccess asserts that when adapter.Create
// succeeds, RecordAdapterCall(adapterName, "success") fires exactly once.
func TestSubmitHandler_RecordsAdapterCallSuccess(t *testing.T) {
	reg := metrics.New(true, ":0")
	deps := newTestSubmitDeps(t) // existing or newly-added Phase 1 test helper
	deps.Metrics = reg
	// Configure deps.Router to return a fake adapter whose Create returns a
	// non-error result with AdapterName "test-adapter".

	body := minimalSubmitBody(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", strings.NewReader(body))
	r = r.WithContext(auth.WithContext(r.Context(), auth.SessionContext{SessionID: "ssn-test", AuthMode: "anonymous", Verified: true}))
	w := httptest.NewRecorder()

	submitHandler(deps).ServeHTTP(w, r)

	if got := testutil.ToFloat64(reg.AdapterCallsTotalForTest("test-adapter", "success")); got != 1 {
		t.Errorf("success count = %v; want 1", got)
	}
	if got := testutil.ToFloat64(reg.AdapterCallsTotalForTest("test-adapter", "error")); got != 0 {
		t.Errorf("error count = %v; want 0", got)
	}
}

// TestSubmitHandler_RecordsAdapterCallError asserts that when adapter.Create
// returns a non-nil error, RecordAdapterCall(adapterName, "error") fires
// exactly once.
func TestSubmitHandler_RecordsAdapterCallError(t *testing.T) {
	reg := metrics.New(true, ":0")
	deps := newTestSubmitDeps(t)
	deps.Metrics = reg
	// Configure deps.Router to return a fake adapter whose Create returns an error.

	body := minimalSubmitBody(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", strings.NewReader(body))
	r = r.WithContext(auth.WithContext(r.Context(), auth.SessionContext{SessionID: "ssn-test", AuthMode: "anonymous", Verified: true}))
	w := httptest.NewRecorder()

	submitHandler(deps).ServeHTTP(w, r)

	if got := testutil.ToFloat64(reg.AdapterCallsTotalForTest("test-adapter", "error")); got != 1 {
		t.Errorf("error count = %v; want 1", got)
	}
	if got := testutil.ToFloat64(reg.AdapterCallsTotalForTest("test-adapter", "success")); got != 0 {
		t.Errorf("success count = %v; want 0", got)
	}
}
```

If `newTestSubmitDeps` and `minimalSubmitBody` don't already exist in `submit_test.go`, they should be derived from existing Phase 1+6 test setup — read `submit_test.go` to locate the existing test setup pattern and reuse it. If a refactor is needed, extract the existing inline setup into these helpers as part of this commit.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/server/... -run TestTurnHandler_Records|TestSubmitHandler_RecordsAdapterCall -v && cd ..`
Expected: FAIL — `deps.Metrics.RecordLLMTokens` / `deps.Metrics.RecordAdapterCall` not yet called.

- [ ] **Step 3: Add the record sites**

In `relay/internal/server/turn.go`, locate the `if chunk.Done {` branch (around line 290). INSIDE the branch, AFTER the existing `if deps.Budget != nil { deps.Budget.Commit(...) }` block and BEFORE the `writeSSEFrame(w, SSEDone{...})` call, INSERT:

```go
				// Phase 7 (7-i): record LLM token counts. Nil-safe on disabled
				// or unset Metrics — the package's Record* methods short-circuit.
				deps.Metrics.RecordLLMTokens(deps.Provider.Name(), "input", chunk.InputTokens)
				deps.Metrics.RecordLLMTokens(deps.Provider.Name(), "output", chunk.OutputTokens)
```

In `relay/internal/server/submit.go`, locate the `result, err := ad.Create(ctx, p)` block (around line 107–112). REPLACE the existing 6-line block:

```go
		result, err := ad.Create(ctx, p)
		if err != nil {
			slog.ErrorContext(ctx, "adapter create failed", "adapter", ad.Name(), "error", err)
			writeError(w, http.StatusBadGateway, "adapter_error", "downstream adapter unavailable")
			return
		}
```

WITH:

```go
		result, err := ad.Create(ctx, p)
		if err != nil {
			slog.ErrorContext(ctx, "adapter create failed", "adapter", ad.Name(), "error", err)
			// Phase 7 (7-i): record the adapter failure. Nil-safe.
			deps.Metrics.RecordAdapterCall(ad.Name(), "error")
			writeError(w, http.StatusBadGateway, "adapter_error", "downstream adapter unavailable")
			return
		}
		// Phase 7 (7-i): record the adapter success. Nil-safe.
		deps.Metrics.RecordAdapterCall(ad.Name(), "success")
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/server/... -v && cd ..`
Expected: all existing tests still pass; the new metric-record tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/server/turn.go relay/internal/server/turn_test.go relay/internal/server/submit.go relay/internal/server/submit_test.go
git commit -m "$(cat <<'EOF'
feat(7-i): metric-record sites in turn.go (SSEDone) + submit.go (post-Create)

deps.Metrics.RecordLLMTokens fires on SSEDone (input + output);
deps.Metrics.RecordAdapterCall fires after adapter.Create (success or error).
All calls nil-safe via the package's passthrough semantics.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: chi middleware wiring — prepend `deps.Metrics.Middleware()`

**Files:** Modify `relay/internal/server/server.go`, `relay/internal/server/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/internal/server/server_test.go`:

```go
// TestServer_MetricsMiddlewarePrependedBeforePerIP asserts the metrics
// middleware runs BEFORE the per-IP rate limiter — a request that gets
// rate-limited (429) is still counted. This is the "observability sees ALL
// inbound traffic" invariant.
func TestServer_MetricsMiddlewareCountsRateLimited429s(t *testing.T) {
	reg := metrics.New(true, ":0")
	// PerIP limiter with rps=0 (impossible) and burst=0 → first request 429s.
	limiter := perip.New(0, 0, 1*time.Minute, time.Now)

	cfg := &config.Config{}
	deps := Deps{
		Logger:         slog.New(slog.NewJSONHandler(io.Discard, nil)),
		PerIP:          limiter,
		Metrics:        reg,
		TrustedProxies: nil,
	}
	h := New(cfg, deps)

	req := httptest.NewRequest(http.MethodGet, "/v1/intake/init", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; want 429", rec.Code)
	}
	// Metrics middleware ran BEFORE the per-IP gate, so the 429 is counted.
	// Path label uses chi's RoutePattern — for a request that hits a chi
	// route, the pattern is /v1/intake/init.
	got := testutil.ToFloat64(reg.HTTPRequestsTotalForTest("/v1/intake/init", "429"))
	if got != 1 {
		t.Errorf("counter for {path=/v1/intake/init, status=429} = %v; want 1", got)
	}
}
```

Add imports as needed: `"io"`, `"log/slog"`, `"time"`, `"intake/internal/metrics"`, `"intake/internal/ratelimit/perip"`, `"github.com/prometheus/client_golang/prometheus/testutil"`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./internal/server/... -run TestServer_MetricsMiddleware -v && cd ..`
Expected: FAIL — current `server.New` does not wire metrics middleware.

- [ ] **Step 3: Prepend the metrics middleware**

In `relay/internal/server/server.go`, MODIFY the `/v1/intake` group block (lines 36–40):

```go
	r.Route("/v1/intake", func(r chi.Router) {
		r.Use(clientIPMiddleware(deps.TrustedProxies))
		r.Use(perIPLimitMiddleware(deps.PerIP))
		registerIntakeRoutes(r, deps)
	})
```

REPLACE with:

```go
	r.Route("/v1/intake", func(r chi.Router) {
		// Phase 7 (7-i): metrics middleware runs FIRST so even rate-limited
		// 429s are counted. Disabled-mode is a literal passthrough — zero
		// observable cost when cfg.Observability.Metrics.Enabled=false.
		r.Use(deps.Metrics.Middleware())
		r.Use(clientIPMiddleware(deps.TrustedProxies))
		r.Use(perIPLimitMiddleware(deps.PerIP))
		registerIntakeRoutes(r, deps)
	})
```

Note: `/v1/health` and `/v1/version` outside the `/v1/intake` group are intentionally NOT counted in the metrics middleware. Operator liveness probes shouldn't drive metric series. This matches the design spec §3.2.

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/server/... -v && cd ..`
Expected: all existing server tests pass; the new metrics-middleware test passes.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/server/server.go relay/internal/server/server_test.go
git commit -m "$(cat <<'EOF'
feat(7-i): prepend Metrics.Middleware() to chi /v1/intake chain

Runs BEFORE perIPLimitMiddleware so rate-limited 429s are counted.
Disabled-mode is a literal passthrough — zero observable cost.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: Lint configs — `.golangci.yml`, `.eslintrc.cjs`, `.prettierrc`, ignore files

**Files:** Create `.golangci.yml`, `.eslintrc.cjs`, `.prettierrc`, `.eslintignore`, `.prettierignore`; modify `core/package.json`, `vue/package.json`, root `package.json`

- [ ] **Step 1: Verify the latest stable lint tool versions**

Open each release page (or `curl https://api.github.com/repos/<owner>/<repo>/releases/latest`) and record:

- `golangci-lint` — expected `v1.62.2` (verify; pin to whatever is the actual latest stable as of 2026-06-01)
- `eslint` — expected `v9.16.0` (verify)
- `eslint-plugin-vue` — expected `v9.32.0` (verify; for ESLint 9 / flat-config era)
- `@typescript-eslint/parser` + `@typescript-eslint/eslint-plugin` — expected `v8.18.0` (verify)
- `prettier` — expected `v3.4.2` (verify)

Record the exact verified versions before proceeding to Step 2. The remainder of this task assumes the placeholder versions above; substitute the real numbers throughout.

- [ ] **Step 2: Create `.golangci.yml`**

Create `.golangci.yml` at the repo root:

```yaml
# .golangci.yml — Phase 7 (7-i) lint config for the Go relay.
# Curated ruleset (NOT --enable-all). Signal over noise.
# See ai/tasks/phase-7/README.md §6 build-fail item 3.
run:
  go: "1.23"
  timeout: 5m

linters:
  disable-all: true
  enable:
    - errcheck      # unchecked error returns
    - govet         # standard go vet
    - staticcheck   # SA-prefixed bugs
    - gosec         # security issues (G-prefixed)
    - ineffassign   # ineffectual assignments
    - unused        # unused vars, consts, funcs

linters-settings:
  errcheck:
    check-type-assertions: false
    check-blank: false
  gosec:
    # G101 (hardcoded credentials) flags test fixtures and the example
    # license templates. Each false positive gets a targeted //nolint:gosec.
    # G304 (file inclusion via variable) hits config loading via a CLI flag —
    # operator-supplied paths are intentional; nolint at the call site.
    excludes: []

issues:
  exclude-rules:
    # Test files are allowed to ignore errors from cleanup deferrals.
    - path: _test\.go
      linters:
        - errcheck
        - gosec
  max-issues-per-linter: 0
  max-same-issues: 0
```

- [ ] **Step 3: Create `.eslintrc.cjs`**

Create `.eslintrc.cjs` at the repo root:

```js
// .eslintrc.cjs — Phase 7 (7-i) lint config for TypeScript + Vue.
// eslint-plugin-vue recommended + @typescript-eslint/recommended strict.
// See ai/tasks/phase-7/README.md §6 build-fail item 5.
module.exports = {
  root: true,
  env: {
    browser: true,
    node: true,
    es2022: true,
  },
  parser: 'vue-eslint-parser',
  parserOptions: {
    parser: '@typescript-eslint/parser',
    ecmaVersion: 2022,
    sourceType: 'module',
    extraFileExtensions: ['.vue'],
  },
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:vue/vue3-recommended',
  ],
  plugins: ['@typescript-eslint', 'vue'],
  rules: {
    // Curated overrides — narrow rather than blanket-disable.
    '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    '@typescript-eslint/no-explicit-any': 'warn',
    'vue/multi-word-component-names': 'off', // Widget components are single-word by convention.
    'no-console': ['warn', { allow: ['warn', 'error'] }],
  },
  overrides: [
    {
      // Smoke drivers may use console.log for progress output.
      files: ['core/smoke/**/*.ts', 'vue/smoke/**/*.ts'],
      rules: {
        'no-console': 'off',
      },
    },
    {
      // Generated codegen output is exempt — covered by .eslintignore.
      files: ['**/generated/**/*.ts'],
      rules: {
        '@typescript-eslint/no-explicit-any': 'off',
      },
    },
  ],
};
```

- [ ] **Step 4: Create `.prettierrc`**

Open `core/src/client.ts` and inspect the indentation + quoting style. The verified style is two-space indent, single quotes for strings, trailing commas only in multi-line arrays/objects, semicolons present, max-width 100.

Create `.prettierrc` at the repo root:

```json
{
  "printWidth": 100,
  "tabWidth": 2,
  "useTabs": false,
  "semi": true,
  "singleQuote": true,
  "trailingComma": "all",
  "bracketSpacing": true,
  "arrowParens": "always",
  "endOfLine": "lf"
}
```

- [ ] **Step 5: Create `.eslintignore`**

Create `.eslintignore`:

```
node_modules/
dist/
**/dist/
**/generated/
local-dev/
*.generated.*
relay/
schema/
docs/
ai/
.github/
```

- [ ] **Step 6: Create `.prettierignore`**

Create `.prettierignore`:

```
node_modules/
dist/
**/dist/
**/generated/
local-dev/
*.generated.*
relay/
schema/payload.v1.json
ai/
.github/
```

(The `schema/payload.v1.json` exclusion preserves the canonical JSON schema's whitespace exactly as authored.)

- [ ] **Step 7: Add lint devDependencies to `core/package.json`**

In `core/package.json`, in the `devDependencies` block (or create one if missing), ADD these exact-pinned entries:

```json
{
  "devDependencies": {
    "eslint": "9.16.0",
    "@typescript-eslint/parser": "8.18.0",
    "@typescript-eslint/eslint-plugin": "8.18.0",
    "prettier": "3.4.2"
  }
}
```

Merge with existing devDependencies (do not overwrite). Add a `"lint"` script:

```json
{
  "scripts": {
    "lint": "eslint src/ smoke/ --max-warnings=0",
    "lint:prettier": "prettier --check src/ smoke/"
  }
}
```

- [ ] **Step 8: Add lint devDependencies to `vue/package.json`**

Same as core, plus `eslint-plugin-vue`:

```json
{
  "devDependencies": {
    "eslint": "9.16.0",
    "eslint-plugin-vue": "9.32.0",
    "@typescript-eslint/parser": "8.18.0",
    "@typescript-eslint/eslint-plugin": "8.18.0",
    "prettier": "3.4.2"
  },
  "scripts": {
    "lint": "eslint src/ smoke/ --max-warnings=0",
    "lint:prettier": "prettier --check src/ smoke/"
  }
}
```

- [ ] **Step 9: Add root `package.json` prettier devDependency + script**

In the root `package.json`, ADD prettier as a devDependency (single source of pin) plus a `"lint:prettier"` workspace-aggregating script:

```json
{
  "devDependencies": {
    "prettier": "3.4.2"
  },
  "scripts": {
    "lint:prettier": "prettier --check ."
  }
}
```

- [ ] **Step 10: Install + sanity-check (no fixes yet — Task 13 sweeps)**

```bash
npm install
```

Expected: clean install with the new lint deps. No package-lock divergence other than the new entries.

- [ ] **Step 11: Commit the lint configs (NOT the fixes — those land in Task 13)**

```bash
git add .golangci.yml .eslintrc.cjs .prettierrc .eslintignore .prettierignore core/package.json vue/package.json package.json package-lock.json
git commit -m "$(cat <<'EOF'
chore(7-i): lint configs — .golangci.yml + .eslintrc.cjs + .prettierrc + ignores

Curated rulesets (NOT --enable-all):
- golangci-lint: errcheck, govet, staticcheck, gosec, ineffassign, unused
- eslint: vue3-recommended + @typescript-eslint/recommended
- prettier: 2-space, single-quote, trailing-comma-all (matches core/src/client.ts)

CI gate wired in Task 14 AFTER the initial-fix sweep in Task 13.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: Pin `golangci-lint` exact + extend `scripts/check-pins.sh`

**Files:** Modify `scripts/check-pins.sh`

- [ ] **Step 1: Decide the install strategy for golangci-lint in CI**

`golangci-lint` ships as a binary release (no Go-module pin). The CI job (Task 14) uses the official action `golangci/golangci-lint-action@v6` with `version: v1.62.2`. The pin enforcement lives in `scripts/check-pins.sh`: grep `.github/workflows/ci.yml` for the version string; fail if it's `latest` or caret-prefixed.

- [ ] **Step 2: Extend `scripts/check-pins.sh`**

In `scripts/check-pins.sh`, AFTER the prometheus pin block (added in Task 1 Step 4), INSERT:

```bash
# Gate: golangci-lint version in ci.yml must be exact-pinned (no @latest, no caret). Phase 7.
if [ -f .github/workflows/ci.yml ]; then
  if grep -E 'golangci-lint(-action)?.*version:.*(\^|latest)' .github/workflows/ci.yml; then
    echo "ERROR: golangci-lint is caret/latest-pinned in .github/workflows/ci.yml; PHASE_PLANNING §5 requires exact pins" >&2
    fail=1
  fi
fi
# Gate: eslint + prettier + @typescript-eslint/* in core/package.json + vue/package.json + root must be exact-pinned. Phase 7.
for pkg in package.json core/package.json vue/package.json; do
  if [ -f "$pkg" ]; then
    if grep -E '"(eslint|prettier|eslint-plugin-vue|@typescript-eslint/parser|@typescript-eslint/eslint-plugin)":\s*"[\^~]' "$pkg"; then
      echo "ERROR: lint tool in $pkg is caret/tilde-pinned; PHASE_PLANNING §5 requires exact pins" >&2
      fail=1
    fi
  fi
done
```

- [ ] **Step 3: Verify the gate works (negative tests)**

Temporarily edit `core/package.json` to make `"eslint": "^9.16.0"` (with caret). Run `bash scripts/check-pins.sh`. Expected: exit 1 with the new ERROR. Revert. Re-run; expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add scripts/check-pins.sh
git commit -m "$(cat <<'EOF'
chore(7-i): extend check-pins.sh for golangci-lint + lint npm pins

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: Initial-fix sweep — run every linter, triage every finding, land fixes

**Files:** Any source file the linters flag; one or more `chore(7-i): initial lint sweep — <linter> findings (...)` commits

- [ ] **Step 1: Run `golangci-lint` and capture every finding**

```bash
cd relay
# Install golangci-lint locally (exact version) if not already present.
# Recommended: use the official installer with the pinned version.
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b "$(go env GOPATH)/bin" v1.62.2

"$(go env GOPATH)/bin/golangci-lint" run ./... > ../ai/tasks/phase-7/lint-sweep-go.txt 2>&1 || true
cd ..
```

Open `ai/tasks/phase-7/lint-sweep-go.txt` and triage. Expected categories of findings:

- **errcheck**: ignored error returns in adapters' `defer resp.Body.Close()`, ignored returns in test setup. Real fix: `defer func() { _ = resp.Body.Close() }()` OR explicit log. False positive in tests: handled by `.golangci.yml` exclude-rules block.
- **govet**: shadowing in for-range loops (Go 1.22+ scope semantics already mitigate most). Real fix: rename inner var.
- **staticcheck**: deprecated `io/ioutil` calls (none expected; the project uses `os` + `io` already). SA1019 deprecations, SA4006 (unused-write) — case-by-case.
- **gosec**: G304 (file inclusion via variable) on `config.Load(*configPath)` — operator-supplied path is intentional → `//nolint:gosec // G304: configPath is operator-supplied via CLI flag, by design`. G101 (hardcoded credentials) on test fixtures with dummy tokens → `//nolint:gosec // G101: test fixture; not a real credential`.
- **ineffassign**: `err := ... ; err = ...` shadowing where the first assignment is never read. Real fix: collapse.
- **unused**: leftover helpers from earlier phases. Real fix: remove if truly unused; OR add `//nolint:unused // kept for Phase X test fixture` if needed for tests (rare).

Apply fixes file-by-file. After each batch of related fixes, commit:

```bash
git add <files>
git commit -m "$(cat <<'EOF'
chore(7-i): initial lint sweep — golangci-lint findings (<N_fixed> fixed, <N_nolint> nolint'd, <N_narrowed> narrowed)

<one-line summary of category — e.g. "errcheck on adapter response-body defers; gosec G304 nolint'd on configPath">

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Re-run `golangci-lint run ./...` after each commit; iterate until zero findings.

- [ ] **Step 2: Run `eslint` and capture every finding**

```bash
cd core
npx eslint src/ smoke/ > ../ai/tasks/phase-7/lint-sweep-eslint-core.txt 2>&1 || true
cd ../vue
npx eslint src/ smoke/ > ../ai/tasks/phase-7/lint-sweep-eslint-vue.txt 2>&1 || true
cd ..
```

Triage:

- **@typescript-eslint/no-unused-vars**: real bugs (leftover imports), real fixes (remove).
- **@typescript-eslint/no-explicit-any**: warnings, not errors. If the `any` is intentional (interop with html2canvas etc.), narrow with `// eslint-disable-next-line @typescript-eslint/no-explicit-any -- html2canvas types are external`.
- **vue/no-mutating-props**: real bugs; rewrite to emit events.
- **vue/no-v-html**: legitimate XSS gate. If the component intentionally renders trusted HTML (the redactor preview probably does), `// eslint-disable-next-line vue/no-v-html -- redactor preview takes sanitized output from DOMPurify`.
- **no-console**: warnings in `src/`, allowed in smoke drivers (already in overrides).

Apply fixes; commit each batch:

```bash
git add <files>
git commit -m "$(cat <<'EOF'
chore(7-i): initial lint sweep — eslint findings (<N_fixed> fixed, <N_disabled> eslint-disable'd, <N_narrowed> narrowed)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Re-run `eslint` after each commit; iterate to zero ERRORS (warnings are allowed; --max-warnings=0 in CI makes warnings fatal there).

- [ ] **Step 3: Run `prettier --check` and capture every finding**

```bash
npx prettier --check . > ai/tasks/phase-7/lint-sweep-prettier.txt 2>&1 || true
```

Apply fixes:

```bash
npx prettier --write .
```

Verify the diff is style-only (no semantic change). Commit:

```bash
git add <changed files>
git commit -m "$(cat <<'EOF'
chore(7-i): initial lint sweep — prettier formatting (<N> files reformatted)

No semantic change; style only.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Re-run `npx prettier --check .` — must exit 0.

- [ ] **Step 4: Re-run all three linters with zero findings**

```bash
cd relay && "$(go env GOPATH)/bin/golangci-lint" run ./... && cd ..
cd core && npx eslint src/ smoke/ --max-warnings=0 && cd ..
cd vue && npx eslint src/ smoke/ --max-warnings=0 && cd ..
npx prettier --check .
```

Expected: every command exits 0. The sweep-log files in `ai/tasks/phase-7/lint-sweep-*.txt` may be kept as audit trail OR deleted before the final commit; either is fine.

- [ ] **Step 5: Re-run the entire test suite to confirm no regression**

```bash
cd relay && go test -race ./... && cd ..
cd core && npm test && cd ..
cd vue && npm test && cd ..
```

Expected: all green. Any test regression from a sweep fix → revert that fix, re-triage, choose a different remediation.

---

### Task 14: CI extension — add `lint-go`, `lint-ts`, `test-go`, `test-ts` jobs

**Files:** Modify `.github/workflows/ci.yml`

- [ ] **Step 1: Append the four new jobs**

In `.github/workflows/ci.yml`, append (preserving the existing `contract` job) the new jobs:

```yaml
  lint-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.23.2"
          cache-dependency-path: relay/go.sum
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.62.2
          working-directory: relay
          args: --timeout=5m

  lint-ts:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
          cache: npm
      - name: Install deps
        run: npm ci
      - name: ESLint (core)
        run: npm run lint -w @intake/core
      - name: ESLint (vue)
        run: npm run lint -w @intake/vue
      - name: Prettier check
        run: npm run lint:prettier

  test-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.23.2"
          cache-dependency-path: relay/go.sum
      - name: Go test (race)
        working-directory: relay
        run: go test -race ./...

  test-ts:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
          cache: npm
      - name: Install deps
        run: npm ci
      - name: Type-check (core)
        run: npm run -w @intake/core type-check
      - name: Test (core)
        run: npm test -w @intake/core
      - name: Test (vue)
        run: npm test -w @intake/vue
```

NOTE: The `goreleaser-check` and `snapshot-build` jobs are intentionally NOT added in 7-i — they need `relay/.goreleaser.yaml` which 7-ii authors. 7-ii's plan extends `ci.yml` with both jobs.

- [ ] **Step 2: Validate the workflow locally (optional but recommended)**

Install `actionlint` if not present (e.g. `go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.4`) and run:

```bash
actionlint .github/workflows/*.yml
```

Expected: exit 0 with no issues.

- [ ] **Step 3: Run `scripts/check-pins.sh`**

```bash
bash scripts/check-pins.sh
```

Expected: exit 0. The golangci-lint pin (`v1.62.2`) and lint npm pins are exact-pinned.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "$(cat <<'EOF'
chore(7-i): CI — add lint-go, lint-ts, test-go, test-ts jobs

Each tool is exact-pinned. goreleaser-check + snapshot-build deferred
to 7-ii (they need relay/.goreleaser.yaml that 7-ii authors).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Smoke (mandatory)

Phase 7-i is NOT complete until ALL six smoke items below pass from a clean state. Each is self-runnable; no external services, no maintainer pauses.

### Smoke 1 — Metrics endpoint disabled-vs-enabled behavior

```bash
# Disabled (default): /metrics not reachable on :9090
cd relay
go build ./cmd/relay -o /tmp/intake-relay
cat > /tmp/intake-test-cfg.yaml <<EOF
server:
  addr: ":18080"
  cors_origins: ["*"]
adapters:
  webhook:
    enabled: true
    url: "https://hooks.example.com/intake"
ratelimit:
  per_session: { session_ttl: "1h" }
  per_ip: { idle_ttl: "15m" }
auth:
  modes: { anonymous: true }
  anonymous: { allow_without_captcha: true }
observability:
  metrics: { enabled: false }
EOF
/tmp/intake-relay --config /tmp/intake-test-cfg.yaml &
RELAY_PID=$!
sleep 1
if curl -sf http://127.0.0.1:9090/metrics; then
  echo "FAIL: /metrics reachable on :9090 with Enabled=false (off-by-default invariant)"
  kill $RELAY_PID
  exit 1
fi
echo "OK: /metrics not reachable when disabled"
kill $RELAY_PID

# Enabled: /metrics returns 200 with the 4 series declared
sed -i 's/enabled: false/enabled: true/' /tmp/intake-test-cfg.yaml
/tmp/intake-relay --config /tmp/intake-test-cfg.yaml &
RELAY_PID=$!
sleep 1
curl -sf http://127.0.0.1:9090/metrics | grep -E '^# HELP intake_(http_requests_total|http_request_duration_seconds|llm_tokens_total|adapter_calls_total)' | wc -l | grep -q '4' || { echo "FAIL: not all 4 series declared"; kill $RELAY_PID; exit 1; }
echo "OK: all 4 series declared in /metrics"

# Drive 5 /v1/intake/init requests and assert the counter increased by 5
for i in 1 2 3 4 5; do
  curl -sf -X POST http://127.0.0.1:18080/v1/intake/init -H 'Content-Type: application/json' -d '{}' >/dev/null
done
sleep 0.5
COUNT=$(curl -sf http://127.0.0.1:9090/metrics | grep '^intake_http_requests_total{path="/v1/intake/init",status="200"}' | awk '{print $2}')
if [ "$COUNT" != "5" ]; then
  echo "FAIL: expected counter == 5; got $COUNT"
  kill $RELAY_PID
  exit 1
fi
echo "OK: counter increased by exactly 5"
kill $RELAY_PID
cd ..
```

### Smoke 2 — `TestAccumulateStartupProblems_AllCombined` passes

```bash
cd relay && go test -run TestAccumulateStartupProblems_AllCombined -v ./cmd/relay/... && cd ..
```

Expected: PASS. Asserts the L022 contract: one slice contains Phase 5 (anonymous, not-a-cidr) + Phase 6 (max_size_bytes) + Phase 7 (chatwoot) problems.

### Smoke 3 — `run-q9-smoke.sh` produces one consolidated log line

```bash
bash relay/cmd/relay/smoke/run-q9-smoke.sh
```

Expected: `All Q9 smokes passed.` ALL existing assertions (Phase 5+6 combined fixtures) pass. M4 subshell refactor doesn't break the smoke.

### Smoke 4 — Three linters report zero findings

```bash
cd relay && "$(go env GOPATH)/bin/golangci-lint" run ./... && cd ..
cd core && npx eslint src/ smoke/ --max-warnings=0 && cd ..
cd vue && npx eslint src/ smoke/ --max-warnings=0 && cd ..
npx prettier --check .
```

Expected: all four commands exit 0.

### Smoke 5 — Phase 1+4+5+6 regression

```bash
cd relay && go test -race ./... && cd ..
cd core && npm test && cd ..
cd vue && npm test && cd ..
bash scripts/check-pins.sh
bash scripts/verify-contract.sh
cd relay && go mod tidy && git diff --exit-code go.mod go.sum && cd ..
```

Expected: every command exits 0. `go mod tidy` is a no-op (the one new module landed in Task 1; nothing should drift).

### Smoke 6 — CI workflow validates locally

```bash
actionlint .github/workflows/*.yml
```

Expected: exit 0. The four new jobs structurally valid.

---

## Done criteria

- [ ] `relay/go.mod` declares `github.com/prometheus/client_golang` at the exact verified version; `relay/go.sum` carries the checksums; no other direct-require deps changed.
- [ ] `scripts/check-pins.sh` enforces the prometheus pin, the golangci-lint pin in `.github/workflows/ci.yml`, and the eslint/prettier pins in `package.json` files.
- [ ] `relay/internal/metrics/` package exists with `metrics.go` + `metrics_test.go`; all six test cases pass (`DisabledIsNoOp`, `EnabledIncrementsHTTPCounter`, `EnabledRecordsLLMTokens`, `EnabledRecordsAdapterCall`, `PathLabelUsesChiRoutePattern`, `ListenAndServe_PortConflict`).
- [ ] `relay/internal/config/config.go` carries `ObservabilityConfig` + `MetricsConfig` with defaults applied in `applyDefaults`; `testdata/sample.yaml` carries the new block; both new config tests pass.
- [ ] `relay/internal/server/deps.go` carries `Metrics *metrics.Registry` field; existing Phase 1–6 tests pass unchanged.
- [ ] `relay/cmd/relay/main.go` exposes `accumulateStartupProblems(cfg, licState, logger) (Deps, []Adapter, []string)`; `buildRegistry` returns `([]adapter.Adapter, []string)`; `validateAttachments` returns zero-value `AttachmentsConfig{}` when `Enabled=false`; `main()` has exactly ONE `os.Exit(1)` site for startup errors; the metrics server runs in an independent goroutine.
- [ ] All seven `TestAccumulateStartupProblems_*` cases pass.
- [ ] FOLLOWUPS I1, I2, M2, M4 are each closed by a dedicated `fix(7-i): FOLLOWUPS <ID> — ...` commit.
- [ ] `turn.go` calls `deps.Metrics.RecordLLMTokens` on SSEDone; `submit.go` calls `deps.Metrics.RecordAdapterCall` after `adapter.Create`; both nil-safe.
- [ ] `server.go` prepends `deps.Metrics.Middleware()` to the `/v1/intake` chi chain BEFORE `perIPLimitMiddleware`.
- [ ] `run-q9-smoke.sh` uses `( cd relay && ... )` subshells.
- [ ] Repo-root `.golangci.yml`, `.eslintrc.cjs`, `.prettierrc`, `.eslintignore`, `.prettierignore` exist with the curated rulesets in the spec.
- [ ] `core/package.json`, `vue/package.json`, root `package.json` carry exact-pinned eslint + prettier + (vue) eslint-plugin-vue + @typescript-eslint/* devDependencies.
- [ ] Initial-fix sweep complete: `golangci-lint run ./...` in `relay/` reports 0 issues; `eslint src/ smoke/ --max-warnings=0` in `core/` and `vue/` reports 0 errors; `prettier --check .` reports 0 unformatted files. Sweep commits land BEFORE the CI gate commit.
- [ ] `.github/workflows/ci.yml` has the four new jobs: `lint-go`, `lint-ts`, `test-go`, `test-ts`. Each tool exact-pinned. `goreleaser-check` + `snapshot-build` deferred to 7-ii.
- [ ] All six Smoke items pass.
- [ ] `go mod tidy` is a no-op (only the one prometheus module added in Task 1 + its transitive indirects).
- [ ] `scripts/verify-contract.sh` exits 0 (no schema change in Phase 7).
- [ ] All Phase 1+4+5+6 unit tests + smoke drivers (`drive-attachments.ts`, `drive-abuse.ts`, `drive-auth-email.ts`, `drive-auth-sso.ts`) pass unchanged under Phase 7's middleware chain.
- [ ] Branch is `phase-7`; commits are not pushed.

*End of Phase 7-i sub-plan.*
