# Phase 7 — Release & Ops

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement the sub-plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Ships the v0 release & operations infrastructure behind the **frozen** Phase 0-6 seams: Prometheus metrics endpoint (off-by-default, 4 series, separate HTTP server), `golangci-lint` + `eslint` + `prettier` integrated via initial-fix sweep then CI gate, `relay/Dockerfile` (multi-stage distroless), `relay/.goreleaser.yaml` (5 platforms + dockers block), `.github/workflows/release.yml` (authored, never executed in Phase 7), `examples/docker-compose/` demo stack, four operator-facing docs (`quickstart.md`, `self-hosting.md`, `license.md`, `adapters.md`) + repo README rewrite, four governance files (LICENSE, CONTRIBUTING, SECURITY, COMMERCIAL), and resolution of all four Phase 6 FOLLOWUPS (I1+I2+M2+M4). One new external Go module (`github.com/prometheus/client_golang`, exact-pinned). No `adapter.Adapter` interface change, no schema change, no public publish.

**IN SCOPE — generate artifacts locally:** `goreleaser release --snapshot --clean` (5 binaries + docker image in `./dist/`), `docker build` (local daemon), `npm pack` / `npm publish --dry-run` (tarballs in working tree), `docker-compose up` (local stack).

**OUT OF SCOPE — never published in Phase 7:** no `docker push`, no `npm publish`, no `gh release create`, no git tag push, no mutation of any remote.

## 1. Spec link

- Phase 7 design: [docs/specs/2026-05-29-phase-7-release-ops-design.md](../../../docs/specs/2026-05-29-phase-7-release-ops-design.md)
- Parent decomposition: [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](../../../docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 7 row + "everything → P7" dependency)
- Source of truth for scope/contracts: [docs/PROJECT.md](../../../docs/PROJECT.md) §14 (repo layout), §15 (build/release), §10 (observability config)
- Closes: [ai/tasks/phase-6/FOLLOWUPS.md](../phase-6/FOLLOWUPS.md) (I1, I2, M2, M4 all folded into Phase 7-i; renamed to `FOLLOWUPS-resolved.md` after 7-i)
- Phase 4-6 patterns mirrored: Phase 6's L022-consolidated startup gate (Phase 7-i closes the contract gap by folding `buildRegistry` failures in), Phase 6's per-sub-plan implementer + spec review + code quality review cadence
- Phase 0-6 frozen seams (unchanged here): `relay/internal/adapter/adapter.go`, `relay/internal/payload/types.go`, `schema/payload.v1.json`, `relay/internal/auth/middleware.go` (Handler signature + SessionContext), `relay/internal/server/server.go` (chi route shape), `relay/internal/attachvalidate/` + `relay/internal/adapter/capabilities.go`

## 2. Architectural Decision Record (ADR) summary

- **Snapshot-only release verification, no public publish.** Phase 7's final smoke runs `goreleaser release --snapshot --clean`, `npm pack`/`npm publish --dry-run`, `docker build` (no push), `docker-compose up` (local stack). Every artifact lands in `./dist/` or the local Docker daemon. The actual public release is a separate maintainer-driven Phase 7.5+ action gated on Q1 final product name + GitHub remote + ghcr/npm tokens. Revisit trigger: maintainer locks Q1 + sets up remote → runs the same workflows against real registries.

- **Prometheus metrics: separate HTTP server, opt-in, 4 core series.** New `relay/internal/metrics/` package; dedicated `*http.Server` on `cfg.Observability.Metrics.Addr` (default `:9090`, **disabled by default**). Four series: `intake_http_requests_total{path,status}`, `intake_http_request_duration_seconds{path}`, `intake_llm_tokens_total{provider,direction}`, `intake_adapter_calls_total{adapter,result}`. `path` label uses chi's `RoutePattern()` for cardinality safety. Disabled mode returns passthrough middleware + no-op record hooks (zero observable cost). Metrics server lives independently — a port-bind failure does NOT crash the main relay (observability shouldn't be able to brick the service it observes). Revisit trigger: per-tenant breakdown OR histogram for adapter call durations needed → v1+ adds labels + a histogram.

- **`golangci-lint` + `eslint` + `prettier` ship in CI; initial-fix sweep precedes the gate.** Curated rulesets (NOT `--enable-all`). Phase 7-i includes an initial-fix sweep task: run each linter locally, triage every finding (real bug → fix; false positive → targeted `//nolint`/`eslint-disable` with reason; style preference → narrow rule), land fixes BEFORE wiring the lint jobs into CI gates. Prevents the gate from being a Day-1 barrier to all future PRs. Revisit trigger: chosen rulesets miss a class of bug → tighten; OR lint noise overwhelms PR signal → narrow.

- **Single Dockerfile in `relay/`, multi-stage, distroless target.** Stage 1: `golang:1.23.2-alpine` builds the static binary. Stage 2: `gcr.io/distroless/static-debian12:nonroot` runs it. No shell, no package manager. Image total < 50 MB (enforced by build-fail discipline). Default exposed ports: 8080 (relay) + 9090 (metrics, when enabled). Runs as `nonroot` user (UID 65532). Revisit trigger: an adapter needs CGO → switch base to `alpine` or `debian:bookworm-slim`.

- **`goreleaser` builds 5 platforms; `release.yml` authored but never executed in Phase 7.** `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`. `goreleaser` exact-pinned. `release.yml` triggered only on tag push `v[0-9]+.[0-9]+.[0-9]+`; correctness verified via `goreleaser check` + snapshot build smoke. `archives.files` uses explicit allowlist to prevent secret leaks. Revisit trigger: 6th platform OR npm scope `@intake` changes → update both goreleaser and the npm publish step.

- **Phase 6 FOLLOWUPS fold into Phase 7-i.** I1: refactor `buildRegistry` to return `([]adapter.Adapter, []string)` — per-adapter Configure failures + "no adapters enabled" flow into the consolidated startup-problems slice, closing the L022 contract gap. I2: extract `accumulateStartupProblems(cfg, licState, logger) (Deps, []string)` from `main()` for unit testability. M2: `validateAttachments` returns zero-value when `Enabled=false`. M4: `run-q9-smoke.sh` subshell cleanup. After 7-i, `FOLLOWUPS.md` is renamed to `FOLLOWUPS-resolved.md` (preserves audit trail). Revisit trigger: none.

This phase does NOT add: actual public release artifacts (npm tarballs uploaded, ghcr images pushed, GitHub Releases — all gated on the separate Phase 7.5+ maintainer action), `golangci-lint --enable-all` (curated set only), per-adapter doc pages (overview only in `docs/adapters.md`), `vue-sso` example (deferred to v1+), per-provider/per-auth-mode/widget-integration deep docs (v1+), Grafana dashboards (the 4 metric series support any dashboard; defer to v1+), CRL/license-revocation (v1+), persistent attachment storage (v1+), additional LLM providers, additional auth modes, additional adapters.

## 3. Sub-plan index

| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 7-i | [Relay code: FOLLOWUPS + Prometheus metrics + lint configs + initial-fix sweep + CI extension](7-i-relay-code-followups-metrics-lint-plan.md) | the seam | L | Not started |
| 7-ii | [Release artifacts: Dockerfile + .goreleaser.yaml + release.yml + goreleaser pin](7-ii-release-artifacts-plan.md) | release config | M | Not started |
| 7-iii | [Demo stack: examples/docker-compose/ (3 services, builds from 7-ii Dockerfile)](7-iii-docker-compose-demo-plan.md) | demo | M | Not started |
| 7-iv | [Docs + governance: 4 docs + repo README rewrite + LICENSE + CONTRIBUTING + SECURITY + COMMERCIAL](7-iv-docs-governance-plan.md) | docs + governance | M | Not started |
| 7-v | [Final smoke + drive-docker-compose.ts + LESSONS L024-L028 + README evidence + FOLLOWUPS rename](7-v-smoke-docs-plan.md) | live evidence | S | Not started |

## 4. Dependency graph

```
7-i (relay code: FOLLOWUPS + Prometheus + lint configs + CI extension)
      │
      ├──► 7-ii (release artifacts: Dockerfile + .goreleaser.yaml + release.yml)
      │       │
      │       ▼
      │     7-iii (docker-compose demo — builds the relay image from 7-ii Dockerfile)   ┐
      │                                                                                  │ parallelizable
      └──► 7-iv (docs + governance — only depends on 7-i for the metrics endpoint URL   ┘ with 7-iii
                  in self-hosting.md; otherwise independent)
                                                                  ▼
                                                                7-v (final smoke + LESSONS + evidence)
```

7-i locks the wire contract: Deps shape (adds `Metrics *metrics.Registry`), the `accumulateStartupProblems` extracted function, the lint rulesets, the metrics middleware + record sites. 7-ii produces the Dockerfile + goreleaser config that 7-iii consumes via `build: ../../relay`. 7-iv writes docs + governance files that only reference what 7-i (and earlier phases) shipped. 7-v consumes outputs from all prior waves. 7-iii and 7-iv touch fully disjoint file territories — dispatched as parallel subagents under subagent-driven-development.

## 5. Tool version pin list

Phase 7 introduces **one** new external Go module (`prometheus/client_golang`) and several new CI tools (goreleaser, golangci-lint, eslint, prettier, optionally actionlint). All exact-pinned; caret-versioning forbidden per PHASE_PLANNING §5.

| Tool | Version | Reason |
|---|---|---|
| `github.com/prometheus/client_golang` | exact-pin at install time (latest stable as of 2026-05-29; verify in 7-i first task) | First new Go module since Phase 5's `golang.org/x/time` promotion. Observability surface is load-bearing for operators — a silent API break would hide regressions. |
| `goreleaser` | exact-pin (latest stable as of 2026-05-29; pin in `scripts/check-pins.sh` + both `ci.yml` and `release.yml`) | Deploy-time CLI; caret-versioning forbidden per PHASE_PLANNING §5 — this is the exact failure mode that motivated PHASE_PLANNING. |
| `golangci-lint` | exact-pin in `ci.yml` lint-go job | Lint output must be reproducible across CI runs and developer machines. |
| `eslint` + `eslint-plugin-vue` + `@typescript-eslint/parser` + `@typescript-eslint/eslint-plugin` | exact-pin in `core/package.json` + `vue/package.json` | Same — lint reproducibility. |
| `prettier` | exact-pin in repo root `package.json` | Same. |
| `actionlint` (optional, CI-only) | exact-pin | Validates `.github/workflows/*.yml` structure. Optional per the spec — include if it's worth the dep. |

`scripts/check-pins.sh` extended with one line per new pinned tool/module, mirroring the existing `golang-jwt`/`keyfunc/v3`/`golang.org/x/time`/`html2canvas` style.

## 6. Build-fail checklist

- [ ] `go build ./... && go vet ./...` fails in `relay/` → **Fail**
- [ ] `go test -race ./...` fails → **Fail**
- [ ] `golangci-lint run ./...` reports any issue → **Fail** (after initial sweep wired)
- [ ] `npm run type-check && npm run build && npm run test` fails in `core/` or `vue/` → **Fail**
- [ ] `eslint .` reports any error → **Fail** (after initial sweep wired)
- [ ] `prettier --check .` reports any unformatted file → **Fail** (after initial sweep wired)
- [ ] `scripts/verify-contract.sh` regresses → **Fail** (no schema change in Phase 7)
- [ ] `scripts/check-pins.sh` regresses → **Fail** (must extend for `prometheus/client_golang` + `goreleaser` + lint tools)
- [ ] `go mod tidy` produces any diff to `relay/go.mod` or `relay/go.sum` → **Fail** (one new module: `prometheus/client_golang`)
- [ ] `goreleaser check` reports any issue on `relay/.goreleaser.yaml` → **Fail**
- [ ] `goreleaser release --snapshot --clean` fails to produce all 5 archives + SHA256SUMS.txt + docker image → **Fail**
- [ ] `docker build -t intake-relay relay/` fails → **Fail**
- [ ] `docker inspect intake-relay --format '{{.Config.User}}'` returns root or empty → **Fail** (distroless nonroot invariant)
- [ ] `docker images intake-relay --format '{{.Size}}'` shows > 50 MB → **Fail** (distroless target)
- [ ] `npm publish --dry-run -w @intake/core` or `-w @intake/vue` fails → **Fail**
- [ ] `tar -tf dist/intake-*.tgz | grep -E '\.env|secrets|local-dev'` returns any line → **Fail** (secret leak in package)
- [ ] `actionlint .github/workflows/*.yml` reports any issue → **Fail** (when actionlint adopted)
- [ ] `cd examples/docker-compose && docker-compose config` fails → **Fail** (YAML or service-graph error)
- [ ] `cfg.Observability.Metrics.Enabled=false` AND `/metrics` reachable on `:9090` → **Fail** (off-by-default invariant)
- [ ] `cfg.Observability.Metrics.Enabled=true` AND `/metrics` returns anything other than text/plain with `# HELP intake_` prefix → **Fail**
- [ ] Metrics port conflict causes the main HTTP listener to fail to start → **Fail** (independence invariant; metrics-port failure must NOT propagate)
- [ ] After 7-i: any of Phase 6 FOLLOWUPS I1/I2/M2/M4 not marked closed in `ai/tasks/phase-6/FOLLOWUPS-resolved.md` → **Fail**
- [ ] Phase 1+4+5+6 regression: any existing smoke driver (`drive-attachments.ts`, `drive-abuse.ts`, `drive-auth-email.ts`, `drive-auth-sso.ts`) fails under Phase 7's middleware chain → **Fail**
- [ ] Existing Phase 1-6 unit tests fail under Phase 7 → **Fail**
- [ ] `core/smoke/drive-docker-compose.ts` fails on a clean machine → **Fail** (Phase 7's load-bearing demo smoke)

## 7. Final smoke (mandatory)

Proves the Phase 7 deliverable end-to-end. ALL 8 items are self-runnable; **no maintainer-paused live smokes** (per the scope-boundary decision — Phase 7 generates artifacts locally, never publishes).

```
1. Q9 startup gate smoke (no LLM credit; self-runnable; after 7-i):
   Combined misconfig fixture covers Phase 5 + Phase 6 + Phase 7 (adapter
   Configure failure). Assert ONE log line with count >= 6 problems listing
   every distinct issue including:
     - auth.modes.anonymous=true without captcha
     - server.trusted_proxies invalid CIDR
     - ratelimit.daily_llm_budget.action_on_exceeded=queue
     - adapter chatwoot api_token_env=NONEXISTENT_VAR
     - attachments.storage.mode=s3
     - attachments.max_size_bytes > max_total_bytes
   Then exit 1. Operators fix every misconfig in one restart cycle.
   Closes Phase 6 FOLLOWUPS I1+I2 (buildRegistry now contributes, not exits;
   accumulateStartupProblems is unit-tested).

2. Metrics endpoint smoke (no LLM credit; self-runnable; after 7-i):
   a. With cfg.Observability.Metrics.Enabled=false:
      relay starts, /metrics not reachable (connection refused on :9090).
   b. With Enabled=true:
      relay starts, /metrics returns 200 with all 4 series declared
      (curl http://localhost:9090/metrics | grep '# HELP intake_').
      Drive 5 /v1/intake/init requests; assert
      intake_http_requests_total{path="/v1/intake/init",status="200"}
      counter increased by exactly 5.

3. Snapshot release smoke (no public artifacts; after 7-ii):
   `goreleaser release --snapshot --clean` produces ./dist/ with:
     - intake-relay_<version>_linux_amd64.tar.gz
     - intake-relay_<version>_linux_arm64.tar.gz
     - intake-relay_<version>_darwin_amd64.tar.gz
     - intake-relay_<version>_darwin_arm64.tar.gz
     - intake-relay_<version>_windows_amd64.zip
     - SHA256SUMS.txt covering all 5
     - ghcr.io/intake/intake-relay:snapshot tag in local docker daemon
     - CHANGELOG.md (NOT uploaded; --snapshot skips upload)
   Each archive extracted and the binary's `--version` printout matches.

4. npm pack dry-run smoke (after 7-ii):
   `npm pack -w @intake/core` produces intake-core-<version>.tgz.
   `npm publish -w @intake/core --dry-run` exits 0.
   Same for @intake/vue.
   The tarball contents inspected (`tar -tf`):
     - asserts no .env or local-dev/ in either tarball
     - asserts no secrets or credentials anywhere
     - asserts package.json has required fields (description, repository, license)

5. docker-compose demo smoke (after 7-iii):
   `cd examples/docker-compose && docker-compose up -d` then
   drive-docker-compose.ts asserts:
     - /v1/health 200
     - /init returns session_id + capabilities (metrics enabled in demo config)
     - /turn SSE stream completes via fake-llm
     - /submit returns external_id (webhook adapter)
     - webhook-receiver logged the canonical payload with messages/client/user
     - /metrics on :19090 returns 4 series
     - docker exec intake-relay id -u returns 65532 (nonroot)
   Then `docker-compose down -v`.

6. Docs walkthrough smoke (manual; after 7-iv):
   Maintainer follows docs/quickstart.md from a hypothetical fresh clone
   (or temp directory) and reaches "ticket in webhook log" within 30 min.
   Same for docs/self-hosting.md docker-compose path.

7. Phase 1+4+5+6 regression (after 7-v wiring):
   - drive-attachments.ts (Phase 6) passes unchanged under Phase 7's
     middleware chain (metrics.Middleware() added at front; transparent)
   - drive-abuse.ts (Phase 5) passes unchanged
   - existing chatwoot/fider/linear/zendesk/webhook unit tests pass
   - go test -race ./... full suite green
   - core/ + vue/ npm test green (no widget changes in Phase 7)
   - scripts/verify-contract.sh + scripts/check-pins.sh green
   - go mod tidy is a no-op (single new module: prometheus/client_golang)

8. Lint smoke (after 7-i sweep + CI gate):
   - golangci-lint run ./... in relay → 0 issues
   - eslint . in core/ + vue/ → 0 errors
   - prettier --check . → 0 unformatted files
   - All three integrated as CI jobs that pass on the merge commit
```

A phase is NOT done until this smoke passes from a clean state. ALL 8 items are self-runnable; no maintainer pauses required.

## 8. Shared Contracts (SINGLE SOURCE OF TRUTH)

These shapes are **frozen** in the noted sub-plan; later sub-plans consume them unchanged.

### 8.1 The frozen Phase 0-6 seams (UNCHANGED)

- `adapter.Adapter` interface (`relay/internal/adapter/adapter.go`) — Phase 6 frozen
- `payload.IntakePayload` / `payload.Attachment` generated types (`relay/internal/payload/types.go`) — Phase 0 frozen
- `schema/payload.v1.json` — Phase 0 frozen; NO schema change in Phase 7
- `auth.Middleware.Handler` signature, `auth.SessionContext`, `auth.Store`, `auth.NewMiddleware/NewMiddlewareWithModes` — Phase 4+5 frozen
- Phase 5 abuse gates (per-IP, per-session, daily budget, CAPTCHA) — Phase 5 frozen
- `intake/license` canonicalization — Phase 3 frozen
- `payloadbuild.Builder` constructor — Phase 6 additive only
- `attachvalidate.ValidateAll` / `DecodeOne` signatures + 6 sentinel errors — Phase 6 frozen
- Each adapter's `Capabilities()` shape (returning `adapter.Capabilities{AcceptedMIMETypes []string}`) — Phase 6 frozen
- The chi route-registration shape (`registerIntakeRoutes`) — additive only; Phase 7 adds `metrics.Middleware()` to the chain; NO new endpoints on the relay's main HTTP server (the `/metrics` endpoint lives on a SEPARATE server on a different port)
- The 8 attachment wire-level error codes (`attachment_too_large`, `attachments_exceed_total`, `attachment_mime_not_allowed`, `attachment_mime_mismatch`, `attachment_malformed`, `attachment_type_unsupported`, `attachments_disabled`, `request_body_too_large`) — Phase 6 frozen

### 8.2 Config addition (additive — `relay/internal/config/config.go`, FROZEN in 7-i)

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
    Observability ObservabilityConfig `yaml:"observability"`  // 7-i NEW
}

type ObservabilityConfig struct {
    LogLevel  string        `yaml:"log_level"`   // default "info"; reserved for v1+
    LogFormat string        `yaml:"log_format"`  // default "json"
    Metrics   MetricsConfig `yaml:"metrics"`
}

type MetricsConfig struct {
    Enabled bool   `yaml:"enabled"` // default false (off-by-default invariant)
    Addr    string `yaml:"addr"`    // default ":9090"
}
```

Defaults applied in `config.applyDefaults` (7-i). The Q9-consolidated startup gate does NOT validate the metrics block in Phase 7 (no fatal misconfigs — port-bind failures are runtime warnings, not startup errors).

### 8.3 Metrics package (FROZEN in 7-i — `relay/internal/metrics/`)

```go
package metrics

import (
    "context"
    "net/http"
)

// Registry holds the package-level prometheus.Registerer + the 4 v0 collectors.
// Disabled mode (Enabled=false) makes ListenAndServe a no-op and Record* methods no-ops.
type Registry struct { /* unexported */ }

// New constructs a Registry. enabled=false makes ListenAndServe + all Record* methods no-ops.
// addr defaults to ":9090" when empty.
func New(enabled bool, addr string) *Registry

// Middleware observes HTTP request count + duration via chi's RoutePattern (bounded cardinality).
// In disabled mode returns a literal passthrough: func(h http.Handler) http.Handler { return h }.
func (r *Registry) Middleware() func(http.Handler) http.Handler

// ListenAndServe starts the metrics HTTP server. Returns when ctx is cancelled.
// No-op when Enabled=false. A port-bind failure is logged at Error level
// but does NOT crash the main relay (independence invariant).
func (r *Registry) ListenAndServe(ctx context.Context) error

// RecordLLMTokens is called from the turn handler on SSEDone.
// direction is "input" or "output".
func (r *Registry) RecordLLMTokens(provider, direction string, count int)

// RecordAdapterCall is called from the submit handler after adapter.Create.
// result is "success" or "error".
func (r *Registry) RecordAdapterCall(adapterName, result string)
```

Four series exported by `New()`:
- `intake_http_requests_total{path,status}` (counter)
- `intake_http_request_duration_seconds{path}` (histogram; default buckets)
- `intake_llm_tokens_total{provider,direction}` (counter)
- `intake_adapter_calls_total{adapter,result}` (counter)

All label values are bounded enums (chi RoutePattern, HTTP status codes 2xx-5xx, `provider` ∈ {anthropic, openai, gemini, ollama}, `direction` ∈ {input, output}, `adapter` ∈ {webhook, chatwoot, fider, linear, zendesk}, `result` ∈ {success, error}). No cardinality explosion possible.

### 8.4 Deps extension (FROZEN in 7-i — `relay/internal/server/deps.go`)

```go
type Deps struct {
    // ... all existing Phase 1-6 fields unchanged ...

    // 7-i NEW:
    Metrics *metrics.Registry  // populated by main.go; nil-safe via the
                               // Middleware() passthrough + no-op Record* hooks.
}
```

### 8.5 main.go orchestration (after FOLLOWUPS I1+I2 land in 7-i)

```go
// accumulateStartupProblems runs all startup gates (Phase 5, Phase 6, Phase 7
// adapter Configure) and returns the consolidated Deps + registry + problems.
//
// PURE FUNCTION: takes cfg + licState + logger; returns Deps + registry + problems; no os.Exit.
// Phase 6's L022 contract is honored: ONE consolidated log line at the call site,
// ONE os.Exit(1). Unit-testable in isolation (closes FOLLOWUPS I2).
//
// Returns a 3-tuple (Deps, []adapter.Adapter, []string) — the registry slice is
// returned separately so main() can pass it to router.New via adapterRegistryFromSlice.
// (Updated per 7-i implementation: cfg is *config.Config; licState is *licensemgr.State.)
func accumulateStartupProblems(
    cfg *config.Config,
    licState *licensemgr.State,
    logger *slog.Logger,
) (server.Deps, []adapter.Adapter, []string) {
    var problems []string

    // 1. Phase 5 gate (anonymous/SSO/CIDR/budget) — returns parsed trustedProxies.
    p5Problems, trustedProxies := startupProblems(cfg)
    problems = append(problems, p5Problems...)

    // 2. buildRegistry — FOLLOWUPS I1: returns ([]adapter.Adapter, []string),
    //                                  NOT (registry, error) with os.Exit on error.
    //    Per-adapter Configure() failures + "no adapters enabled" + license-gate
    //    warnings all flow through here.
    registry, regProblems := buildRegistry(cfg, licState, logger)
    problems = append(problems, regProblems...)

    // 3. Phase 6 attachments gate — M2: returns zero-value when Enabled=false.
    attachmentsCfg, attProblems := validateAttachments(cfg, registry)
    problems = append(problems, attProblems...)

    // 4. Compute Deps from the validated outputs.
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
        TrustedProxies:  trustedProxies,
        AttachmentsCfg:  attachmentsCfg,
        AttachmentMIMEs: attachmentMIMEs,
        BodyCapBytes:    bodyCapBytes,
        Metrics:         metricsReg,
    }, registry, problems
}

func main() {
    // ... cfg + license load (unchanged) ...

    startupDeps, registry, problems := accumulateStartupProblems(cfg, licState, logger)
    if len(problems) > 0 {
        logger.Error("relay: startup config errors",
            "count", len(problems), "problems", problems)
        os.Exit(1)        // ONE site, ALL problems
    }

    // Start metrics server in a goroutine (no-op when disabled)
    ctx := context.Background()
    go func() {
        if err := startupDeps.Metrics.ListenAndServe(ctx); err != nil {
            logger.Error("metrics: ListenAndServe failed", "err", err)
            // Main relay continues — observability shouldn't be able to brick it.
        }
    }()

    // main() converts registry to a map via adapterRegistryFromSlice for router.New,
    // then starts the main HTTP server (unchanged from Phase 1-6 +
    // metrics.Middleware() prepended to the chain).
    // server.New(startupDeps, adapterRegistryFromSlice(registry), ...).ListenAndServe(...)
}
```

### 8.6 Endpoint contract shapes (FROZEN in 7-i)

**No new endpoints on the relay's main HTTP server.** Phase 7 adds a SEPARATE HTTP server (on `cfg.Observability.Metrics.Addr`, default `:9090`) with:

```
GET /metrics
  200 text/plain; version=0.0.4 (Prometheus exposition format)
  Body: # HELP intake_http_requests_total ...
        # TYPE intake_http_requests_total counter
        intake_http_requests_total{path="/v1/intake/init",status="200"} 5
        ...
```

No authentication on `/metrics` in v0 — operators are expected to put it behind a private network or a reverse proxy that restricts access. Documented in `docs/self-hosting.md` (authored in 7-iv).

## 9. Notes

- Module path remains `intake`. Go 1.23.2. No go.mod path change.
- L010 (PS 5.1 BOM) applies to any new smoke YAML written via `Set-Content` — use `-Encoding ascii`.
- L016 (return parsed values from startup gate) is preserved by `accumulateStartupProblems` — every gate returns its parsed outputs, no re-parse-with-discarded-error.
- L022 (consolidate Q9 startup-gate problems before single os.Exit) is honored AND its contract gap is closed by FOLLOWUPS I1 (buildRegistry now contributes problems instead of exiting).
- L023 (read unused-but-parsed success body fields) applies to the metrics package: the prometheus client's `Inc()` returns no error, but `MustRegister` panics on duplicate registration — handled by package-internal initialization, not exposed to callers.
- L024-L028 (new lessons authored during 7-v): snapshot-then-publish split, initial lint sweep before CI gate, metrics server independence, off-by-default observability, distroless multi-stage Docker template.
- Schema is unchanged in Phase 7. `scripts/verify-contract.sh` passes without re-running codegen.
- `go mod tidy` after Phase 7 must produce zero diff beyond the one new module (`prometheus/client_golang`).
- `local-dev/` files (smoke-chatwoot-attachments.{yaml,ps1} etc.) are gitignored — Phase 7 does NOT touch them.
- `examples/vue-anonymous/` and `examples/webhook-receiver/` (Phase 1 examples) stay as-is. Phase 7 only ADDS `examples/docker-compose/`.
