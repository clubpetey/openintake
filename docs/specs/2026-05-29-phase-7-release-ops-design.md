# Phase 7 — Release & Ops — Design Spec

> **Status:** Approved design (brainstorming output), pre-planning
> **Date:** 2026-05-29
> **Implements:** [docs/PROJECT.md](../PROJECT.md) §14 (repo layout), §15 (build/release), §10 (observability config)
> **Decomposes:** [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](2026-05-26-v0-decomposition-and-phasing-design.md) Phase 7 row + "everything → P7" dependency
> **Inherits seams from:** Phase 0 (schema + codegen), Phase 1 (chi server + Deps + middleware chain), Phase 3 (adapter Configure + license gate), Phase 4 (auth dispatcher), Phase 5 (abuse gates + consolidated Q9 startup gate), Phase 6 (attachments + Capabilities + the L022 consolidated-problems pattern)
> **Companion phase doc:** `ai/tasks/phase-7/README.md` (authored via writing-plans after this spec is approved)
> **Closes:** [ai/tasks/phase-6/FOLLOWUPS.md](../../ai/tasks/phase-6/FOLLOWUPS.md) (I1, I2, M2, M4 all folded into Phase 7-i)

---

## 1. Goal

Ship the v0 release & operations infrastructure: every artifact a public release needs (multi-platform binaries, Docker image, npm packages, demo stack, governance files, operator docs, CI/CD pipeline, observability endpoint), authored and verified locally. Phase 7's final smoke proves the pipeline works end-to-end via snapshot/dry-run modes; nothing is actually published. After Phase 7, the public release is gated on a separate maintainer-driven action that locks the final product name (Q1 from the decomposition spec), sets up the GitHub remote, and tags `vX.Y.Z`.

---

## 2. Scope and non-scope

### IN SCOPE — generate artifacts locally

- `goreleaser release --snapshot --clean` → 5 platform binaries (`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`) sitting in `./dist/` with checksums
- `docker build` → multi-arch image tagged `ghcr.io/clubpetey/openintake-relay:snapshot` in the local Docker daemon
- `npm pack` (or `npm publish --dry-run`) → `intake-core-0.0.0.tgz` + `intake-vue-0.0.0.tgz` tarballs in the working tree, validated against the registry's manifest rules
- `docker-compose up` → full stack boots locally (`examples/docker-compose/`), processes a real ticket end-to-end
- Prometheus metrics endpoint on a separate HTTP server (off-by-default), exporting 4 series
- Phase 6 FOLLOWUPS (I1+I2+M2+M4) fully resolved
- Lint integration: `golangci-lint` + `eslint` + `prettier` in CI (after initial-fix sweep)
- New operator-facing docs (`quickstart.md`, `self-hosting.md`, `license.md`, `adapters.md`) + repo `README.md` rewrite
- Governance files (`LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, `COMMERCIAL.md`)

### OUT OF SCOPE — never published in Phase 7

- No `docker push` to ghcr or any registry
- No `npm publish` (only `--dry-run` and `npm pack`)
- No `gh release create` or git tag push
- No mutation of any remote (the repo stays local-only)
- No `vue-sso` example (deferred to v1+; requires operator-supplied IDP infrastructure)
- No per-adapter deep doc pages (`docs/adapters.md` is overview-only)
- No `llm-providers.md` / `auth-modes.md` / `widget-integration.md` deep pages (deferred to v1+)
- No Grafana dashboards (the 4 metric series support any standard dashboard; defer dashboard-writing to v1+)
- No `golangci-lint --enable-all` (curated set only; see §3.3)
- No CRL / license-revocation (v1+)
- No hosted-relay multi-tenant configuration (separate project)
- No mobile SDKs, no React widget, no additional adapters/providers (per the decomposition spec's v0 scope)

---

## 3. Architectural Decision Record

Six decisions lock in here. Each carries a trigger for revisiting.

### 3.1 Snapshot-only release verification, no public publish

Phase 7's final smoke runs `goreleaser release --snapshot --clean`, `npm pack`/`npm publish --dry-run`, `docker build` (no push), and `docker-compose up` (local stack). All artifacts land in `./dist/` or the local Docker daemon. NO push to ghcr, NO npm publish, NO GitHub Release, NO git tag pushed to any remote.

**Revisit trigger:** maintainer locks Q1 final product name + sets up GitHub remote + provisions npm/ghcr tokens → "Phase 7.5 — first public release" is a separate manual action that runs the same workflows against real registries. Phase 7 is the unconditional prerequisite for Phase 7.5; the snapshot smoke is the load-bearing proof that Phase 7.5 will succeed when invoked.

### 3.2 Prometheus metrics: separate HTTP server, opt-in, 4 core series

A dedicated `*http.Server` on `cfg.Observability.Metrics.Addr` (default `:9090`, **disabled by default** via `cfg.Observability.Metrics.Enabled bool`). Lives in a new `relay/internal/metrics/` package, exports four series via `github.com/prometheus/client_golang`:

- `intake_http_requests_total{path,status}` (counter)
- `intake_http_request_duration_seconds{path}` (histogram)
- `intake_llm_tokens_total{provider,direction}` (counter; reuses `budget.Tracker` counts)
- `intake_adapter_calls_total{adapter,result}` (counter)

Implemented as a passive observer middleware in the chi chain — does NOT modify Phase 1+4+5 abuse-gate code or the auth dispatcher. The `path` label uses chi's `RoutePattern()` to bound cardinality. Disabled mode returns a passthrough middleware + no-op record hooks (zero observable cost).

**Revisit trigger:** operators need per-tenant breakdown OR a histogram for adapter call durations → v1+ adds labels + a histogram. Also: cardinality of `provider`/`adapter`/`result` is bounded by enums today; if v1+ introduces dynamic providers/adapters, the cardinality plan needs review.

### 3.3 `golangci-lint` + `eslint` + `prettier` ship in CI; initial-fix sweep precedes the gate

PROJECT.md §15 expects all three linters. Phase 7 adds them via:

- `.golangci.yml` enables a curated ruleset: `errcheck`, `govet`, `staticcheck`, `gosec`, `ineffassign`, `unused`. NOT `--enable-all` — signal over noise.
- `.eslintrc.cjs` uses `eslint-plugin-vue` recommended + TS strict.
- `.prettierrc` matches the project's existing two-space style.

Phase 7-i includes an **initial-fix sweep task** that runs each linter locally, triages every finding (real bug → fix; false positive → targeted `//nolint` or `eslint-disable-next-line` comment with reason; style preference → either accept or narrow the rule), and lands fixes BEFORE wiring the lint jobs into CI gates. This prevents the gate from being a barrier to all future PRs from day 1.

**Revisit trigger:** the chosen rulesets miss a class of bug found in production → tighten by enabling additional checks; OR lint noise overwhelms PR signal → narrow the ruleset.

### 3.4 Single `Dockerfile` in `relay/`, multi-stage build, distroless target

- **Stage 1**: `golang:1.23.2-alpine` builds the static binary via `go build -ldflags '-s -w' -trimpath ./cmd/relay`.
- **Stage 2**: `gcr.io/distroless/static-debian12:nonroot` runs it. No shell, no package manager, no apt CVEs to track.
- Binary is `~10MB` static. Image total `< 50 MB` (enforced by build-fail discipline).
- Default exposed ports: `8080` (relay HTTP) + `9090` (metrics, when enabled).
- Runs as `nonroot` user (UID 65532) by default.

**Revisit trigger:** an adapter needs CGO (none today) → switch base to `alpine` or `debian:bookworm-slim`. Or: a security advisory affects the distroless base → update the pinned base image SHA.

### 3.5 `goreleaser` builds the 5 platforms per spec; release gated on tag push

- Platforms: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`.
- `goreleaser` pinned exact (latest stable as of 2026-05-29; verify and pin in 7-ii first task) per `scripts/check-pins.sh`.
- `release.yml` workflow is **authored in Phase 7-ii but never EXECUTED in Phase 7** — its correctness is verified via `goreleaser check` + the snapshot build smoke. Triggered only on tag push matching `v[0-9]+.[0-9]+.[0-9]+`.
- Includes a `dockers:` block that builds + tags the multi-arch image as `ghcr.io/clubpetey/openintake-relay:vX.Y.Z` + `:latest` — but never `docker push` in Phase 7's flows.
- `archives.files` uses an explicit allowlist (LICENSE, README.md, CHANGELOG.md, docs/) to prevent accidental inclusion of `.env`, `local-dev/`, etc.

**Revisit trigger:** add a 6th platform (`linux/arm/v7`, `freebsd`) → extend the matrix; OR the npm scope `@intake` needs to change → update both goreleaser AND the npm publish step.

### 3.6 Phase 6 FOLLOWUPS fold into Phase 7-i (closes the L022 contract gap)

- **I1**: refactor `buildRegistry` to return `([]adapter.Adapter, []string)`. Per-adapter `Configure()` failures + license-gate warnings + "no adapters enabled" all flow into a problems slice, NOT `os.Exit(1)`.
- **I2**: extract gate orchestration from `main()` into `accumulateStartupProblems(cfg, licState, logger) (Deps, []string)`. Makes the cross-phase wiring unit-testable (Phase 6 only had the shell smoke for this).
- **M2**: `validateAttachments` returns zero-value `AttachmentsConfig` when `Enabled=false` (defensive future-proofing).
- **M4**: `run-q9-smoke.sh` uses `(cd relay && ...)` subshells instead of repeated `cd` dance.

After Phase 7-i lands, `ai/tasks/phase-6/FOLLOWUPS.md` is renamed to `FOLLOWUPS-resolved.md` (preserves audit trail).

**Revisit trigger:** none expected — these are pure cleanups.

---

## 4. Open-question resolutions

All design-time questions raised during brainstorming were resolved; none are deferred.

| # | Question | Resolution |
|---|---|---|
| Q-A | Scope boundary (provisional infra vs. real publish) | **Provisional infrastructure only** — generate artifacts locally; never publish. Public release is a separate Phase 7.5+ action gated on Q1 + remote + tokens. |
| Q-B | Phase 6 FOLLOWUPS | **Fold all 4 into 7-i**. FOLLOWUPS.md renamed to FOLLOWUPS-resolved.md after 7-i. |
| Q-C | Prometheus metrics scope | **Production-ready minimal**: 4 series (HTTP request count, HTTP request duration, LLM tokens, adapter calls), separate HTTP server, off-by-default. New dep: `prometheus/client_golang` (exact-pinned). |
| Q-D | User docs scope | **Minimal essential set**: `quickstart.md`, `self-hosting.md`, `license.md`, `adapters.md` (overview only). Defers per-adapter / per-provider / per-auth-mode deep pages to v1+. |
| Q-E | Examples scope | **docker-compose demo only**. `vue-anonymous` + `webhook-receiver` (Phase 1) stay; `vue-sso` deferred to v1+. |
| Q-F | CI hardening | **Full**: golangci-lint + eslint + prettier + go test + npm test + goreleaser check + snapshot build. Includes initial-fix sweep before wiring the lint gates. |
| Q-G | Decomposition | **Approach A — 5 sub-plans**: 7-i seam, 7-ii release artifacts, 7-iii demo, 7-iv docs+governance, 7-v final smoke. 7-iii + 7-iv parallel after 7-ii. |
| Q-H | Maintainer pause? | **None.** Phase 7 is fully self-runnable. No live smoke, no credentials, no external endpoints. |

---

## 5. Components

### 5.1 New Go packages

```
relay/internal/metrics/
    metrics.go              Registry + 4 collectors + Middleware() + ListenAndServe() + Record* hooks
    metrics_test.go         disabled-is-no-op, enabled-increments, port-conflict-survival, cardinality safety
```

Exports:

```go
package metrics

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
func (r *Registry) RecordAdapterCall(adapter, result string)
```

### 5.2 Config addition

```go
// relay/internal/config/config.go — NEW top-level block (7-i)

type Config struct {
    // ... existing fields ...
    Observability ObservabilityConfig `yaml:"observability"`  // 7-i NEW
}

type ObservabilityConfig struct {
    LogLevel  string         `yaml:"log_level"`   // default "info"; reserved for v1+ fine-tuning
    LogFormat string         `yaml:"log_format"`  // default "json"; "text" for human-readable dev
    Metrics   MetricsConfig  `yaml:"metrics"`
}

type MetricsConfig struct {
    Enabled bool   `yaml:"enabled"`  // default false (off-by-default invariant)
    Addr    string `yaml:"addr"`     // default ":9090"
}
```

Defaults applied in `config.applyDefaults`. The Q9-consolidated startup gate in `main.go` does NOT validate the metrics block in Phase 7 (no fatal misconfigs — port-bind failures are runtime warnings, not startup errors per §3.2).

### 5.3 Deps extension (additive)

```go
// relay/internal/server/deps.go — additive (7-i)

type Deps struct {
    // ... all existing Phase 1-6 fields unchanged ...

    // 7-i NEW:
    Metrics *metrics.Registry  // populated by main.go; nil-safe via the Middleware() passthrough + no-op Record* hooks
}
```

### 5.4 main.go orchestration (after FOLLOWUPS I1+I2 land)

```go
// relay/cmd/relay/main.go — extract gate orchestration

// accumulateStartupProblems runs all startup gates (Phase 5, Phase 6, Phase 7
// adapter Configure) and returns the consolidated Deps + problems slice.
//
// This is a PURE FUNCTION (takes cfg + licState + logger; returns Deps + problems;
// no os.Exit). Phase 6's L022 contract is honored: ONE consolidated log line at
// the call site, ONE os.Exit(1). Unit-testable in isolation.
func accumulateStartupProblems(
    cfg config.Config,
    licState *license.State,
    logger *slog.Logger,
) (Deps, []string) {
    var problems []string

    // 1. Phase 5 gate (anonymous/SSO/CIDR/budget)
    p5Problems, trustedProxies := startupProblems(cfg, logger)
    problems = append(problems, p5Problems...)

    // 2. buildRegistry — FOLLOWUPS I1: returns []string instead of os.Exit
    registry, regProblems := buildRegistry(cfg, licState, logger)
    problems = append(problems, regProblems...)

    // 3. Phase 6 attachments gate
    attCfg, attProblems := validateAttachments(cfg.Attachments, registry)
    problems = append(problems, attProblems...)

    // 4. Compute Deps from the validated outputs
    attCaps := server.ComputeAttachmentsCaps(attCfg, registry)
    bodyCapBytes := int64(1 << 20)
    if attCfg.Enabled {
        bodyCapBytes = int64((1 << 20) * 14)
    }
    metricsReg := metrics.New(cfg.Observability.Metrics.Enabled, cfg.Observability.Metrics.Addr)

    return Deps{
        // ... all existing Phase 1-6 fields ...
        TrustedProxies:  trustedProxies,
        Registry:        registry,
        AttachmentsCfg:  attCfg,
        AttachmentMIMEs: attCaps.AllowedMIMETypes,
        BodyCapBytes:    bodyCapBytes,
        Metrics:         metricsReg,
    }, problems
}

func main() {
    // ... cfg + license load (unchanged) ...

    deps, problems := accumulateStartupProblems(cfg, licState, logger)
    if len(problems) > 0 {
        logger.Error("relay: startup config errors", "count", len(problems), "problems", problems)
        os.Exit(1)
    }

    // Start metrics server in a goroutine (no-op if disabled)
    ctx := context.Background()
    go func() {
        if err := deps.Metrics.ListenAndServe(ctx); err != nil {
            logger.Error("metrics: ListenAndServe failed", "err", err)
        }
    }()

    // Start main HTTP server (unchanged from Phase 1-6 + metrics.Middleware() prepended to the chain)
    server.New(deps).ListenAndServe(...)
}
```

### 5.5 Metric-record sites (additive, single-line)

- **`relay/internal/server/turn.go`** — on SSEDone:
  ```go
  deps.Metrics.RecordLLMTokens(providerName, "input",  inputTokens)
  deps.Metrics.RecordLLMTokens(providerName, "output", outputTokens)
  ```
- **`relay/internal/server/submit.go`** — after `adapter.Create`:
  ```go
  if err != nil {
      deps.Metrics.RecordAdapterCall(adapterName, "error")
      // existing error wrapping unchanged
  } else {
      deps.Metrics.RecordAdapterCall(adapterName, "success")
  }
  ```

Disabled-mode no-op makes both sites safe regardless of cfg.

### 5.6 Release artifacts (7-ii)

- `relay/Dockerfile` (multi-stage, distroless/static-debian12:nonroot, < 50 MB).
- `relay/.goreleaser.yaml` (5-platform matrix + dockers block + archives + checksum + release config).
- `.github/workflows/release.yml` (gated on tag push; authored but never executed in Phase 7).
- `scripts/check-pins.sh` extension for `goreleaser` exact-pin enforcement.

### 5.7 Demo stack (7-iii)

```
examples/docker-compose/
    README.md              what this is + run instructions
    docker-compose.yml     3 services: relay, fake-llm, webhook-receiver
    config.yaml            relay config with attachments enabled, webhook adapter
    .env.example           ANTHROPIC_API_KEY optional (defaults to fake-llm)
```

Service breakdown:
- **relay**: `build: ../../relay` (uses 7-ii Dockerfile)
- **fake-llm**: `build: ../../relay`, `command: ["/fake-llm", ...]` (reuses the same image with a different entrypoint)
- **webhook-receiver**: `build: ../webhook-receiver` (existing example; just containerize it)

No chatwoot / fider / linear / zendesk containers — those have their own setup steps and an "intake demo" shouldn't double as "set up chatwoot." Per-adapter setup is documented in `docs/self-hosting.md` and the per-adapter sections of `docs/adapters.md`.

### 5.8 Docs + governance (7-iv)

**New `docs/`:**
- `quickstart.md` — clone → docker-compose up → submit a ticket → see it in the webhook log. Target: 30 minutes.
- `self-hosting.md` — bare-metal binary path + docker path + reverse-proxy section + env-var reference + secret management.
- `license.md` — free vs paid model, license-file format, trial mode, expiry behavior.
- `adapters.md` — overview matrix (5 adapters × tier × config keys × notes).

**Repo-root files:**
- `README.md` — rewrite to cover what intake is, canonical demo command, links to all docs.
- `LICENSE` — Apache 2.0 canonical text + project copyright line.
- `CONTRIBUTING.md` — branch model, commit conventions, the `ai/tasks/` phase model, local pre-commit commands.
- `SECURITY.md` — supported versions, how to report a vulnerability, expected response time.
- `COMMERCIAL.md` — terms for paid-adapter use, contact for licensing.

### 5.9 Final smoke driver (7-v)

```
core/smoke/drive-docker-compose.ts
```

Boots `examples/docker-compose/` via `docker-compose up -d`, polls `/v1/health`, drives a full `/init → /turn → /submit` flow via the webhook adapter, asserts the receiver logged the canonical payload, tears down with `docker-compose down -v`. Self-contained, no credentials.

---

## 6. Tool version pins (per PHASE_PLANNING §5)

| Tool | Version | Reason |
|---|---|---|
| `github.com/prometheus/client_golang` | exact-pin at install time (latest stable as of 2026-05-29; verify in 7-i first task) | First new Go module since Phase 5's `golang.org/x/time` promotion. Observability surface is load-bearing — a silent API break would hide regressions. Caret forbidden. |
| `goreleaser` | exact-pin (latest stable as of 2026-05-29; pin in `scripts/check-pins.sh` + both workflow files) | Deploy-time CLI — caret forbidden per PHASE_PLANNING §5 (the exact failure mode that motivated PHASE_PLANNING). |
| `golangci-lint` | exact-pin in `ci.yml` job | Lint output must be reproducible across CI runs and developer machines. |
| `eslint` + `eslint-plugin-vue` + `@typescript-eslint/parser` + `@typescript-eslint/eslint-plugin` | exact-pin in `core/package.json` + `vue/package.json` | Same — lint reproducibility. |
| `prettier` | exact-pin in repo root `package.json` | Same. |
| `actionlint` (optional, CI-only) | exact-pin | Same. Validates `.github/workflows/*.yml` structure. |

`scripts/check-pins.sh` extended with one line per new pinned tool/module, mirroring the existing `golang-jwt`/`keyfunc/v3`/`golang.org/x/time`/`html2canvas` style.

---

## 7. Data flow

### 7.1 Startup gate consolidation (after FOLLOWUPS I1+I2 land)

```
main()
  │  cfg = config.Load(...)
  │  licState = license.Load(...)
  ▼
accumulateStartupProblems(cfg, licState, logger)
  │
  │  1. p5Problems, trustedProxies = startupProblems(cfg, logger)
  │
  │  2. registry, regProblems = buildRegistry(cfg, licState, logger)   ← I1
  │     (per-adapter Configure failures + license-gate warnings + no-adapters check
  │      all return as []string entries, NOT os.Exit)
  │
  │  3. attCfg, attProblems = validateAttachments(cfg.Attachments, registry)   ← M2
  │     (returns zero-value when Enabled=false)
  │
  │  4. problems = p5Problems + regProblems + attProblems
  │
  │  5. Build Deps (Registry, AttachmentsCfg, AttachmentMIMEs, BodyCapBytes,
  │                 Metrics, TrustedProxies, ...)
  │
  │  return Deps, problems
  ▼
if len(problems) > 0:
    logger.Error("relay: startup config errors", count=N, problems=[...])
    os.Exit(1)        ← ONE site, ALL problems

go deps.Metrics.ListenAndServe(ctx)   ← independent goroutine, port-bind failure logged but not fatal
server.New(deps).ListenAndServe(...)
```

**Single restart cycle reveals every misconfig** across every subsystem — Phase 5 (auth/CIDR/budget), Phase 6 (storage.mode/cap-inverted), Phase 7 (adapter Configure failures, "no adapters enabled"). License-gate warnings stay as `slog.Warn` (NOT problem entries — they're informational; free-mode is a valid operating state).

### 7.2 Metrics observation (request path)

```
HTTP request
  │
  ▼ chi router
  │
  │ ─── metrics.Middleware() ──────────────────────────────┐
  │     startTime := time.Now()                            │
  │     next.ServeHTTP(rec, r)  // rec wraps w for status  │
  │     duration := time.Since(startTime)                  │
  │     intake_http_requests_total{path,status}.Inc()      │
  │     intake_http_request_duration_seconds{path}         │
  │       .Observe(duration.Seconds())                     │
  │ ────────────────────────────────────────────────────── │
  │
  ▼ per-IP rate limiter      (Phase 5; unchanged)
  ▼ auth middleware          (Phase 4; unchanged)
  ▼ per-session counter      (Phase 5; unchanged)
  ▼ budget gate              (Phase 5; unchanged)
  ▼ endpoint handler
        │
        │ /turn: on SSEDone:
        │   deps.Metrics.RecordLLMTokens(provider, "input",  inputTokens)
        │   deps.Metrics.RecordLLMTokens(provider, "output", outputTokens)
        │
        │ /submit: after adapter.Create:
        │   deps.Metrics.RecordAdapterCall(adapter, "success" or "error")
```

**`path` label normalization:** the middleware reads `chi.RouteContext(r.Context()).RoutePattern()` to bound cardinality. Without normalization, `/v1/intake/submit?session_id=abc` and `/v1/intake/submit?session_id=def` would produce two distinct label values, exploding cardinality. With chi's pattern, every endpoint is one label value regardless of query string or path parameters.

**Disabled-mode cost:** `metrics.New(false, ...)` returns a Registry whose `Middleware()` returns a literal passthrough and whose `Record*` methods are inlined no-ops. Zero observable cost vs. Phase 6.

### 7.3 Release artifact generation (provisional, never published)

```
maintainer runs (Phase 7-v final smoke):

  $ goreleaser release --snapshot --clean
   │
   ▼
goreleaser reads relay/.goreleaser.yaml
   │
   ├─► builds 5 platform binaries via `go build`
   │     → dist/openintake-relay_<version>_<os>_<arch>/openintake-relay[.exe]
   │
   ├─► creates platform-specific archives (tar.gz on linux/darwin, zip on windows)
   │     → dist/openintake-relay_<version>_<os>_<arch>.{tar.gz,zip}
   │
   ├─► generates SHA256SUMS.txt
   │
   ├─► (dockers block) builds multi-arch image LOCALLY
   │     → tag: ghcr.io/clubpetey/openintake-relay:snapshot
   │     → NEVER pushed (--snapshot disables push)
   │
   └─► (release block) generates CHANGELOG.md from git log
         → dist/CHANGELOG.md   (NOT uploaded; --snapshot skips upload)


  $ cd core && npm pack && cd ../vue && npm pack
   │
   ▼
npm packs each workspace
   │
   ├─► dist/intake-core-0.0.0.tgz
   └─► dist/intake-vue-0.0.0.tgz
         (tarballs validated against npm's manifest schema; NEVER uploaded)


  $ cd examples/docker-compose && docker-compose up -d
   │
   ▼
docker-compose builds the relay image from relay/Dockerfile, starts:
   │
   ├─► relay (port 18080 — relay HTTP, port 19090 — metrics)
   ├─► fake-llm (port 11434)
   └─► webhook-receiver (port 19099)
   │
   ▼
core/smoke/drive-docker-compose.ts polls /v1/health,
runs /init → /turn → /submit, asserts the webhook-receiver
logged the canonical payload, tears down with docker-compose down -v.
```

**No network egress to ghcr / npm / GitHub in any of the above.** Every artifact is generated in the working tree (`./dist/`) or local Docker daemon and verified locally.

---

## 8. Error handling

### 8.1 New error envelope codes

Phase 7 introduces **NONE**. No new HTTP endpoints, no new request/response shapes (the `/metrics` endpoint lives on a separate server and returns Prometheus text format, not the relay's `ErrorEnvelope`).

### 8.2 Startup gate consolidation (closes L022 contract gap)

After 7-i, the single error log line covers EVERY startup misconfig across every subsystem:

```json
{
  "time": "2026-05-29T14:23:01Z",
  "level": "ERROR",
  "msg": "relay: startup config errors",
  "count": 6,
  "problems": [
    "auth.modes.anonymous=true requires captcha.enabled=true OR auth.anonymous.allow_without_captcha=true",
    "server.trusted_proxies contains an invalid CIDR \"not-a-cidr\"",
    "ratelimit.daily_llm_budget.action_on_exceeded=\"queue\" is not supported in v0 (only \"reject\")",
    "adapter \"chatwoot\": api_token_env=\"NONEXISTENT_VAR\" is not set in the environment",
    "attachments.storage.mode=\"s3\" is not supported in v0 (only \"\" or \"forward\")",
    "attachments.max_size_bytes=20000000 exceeds attachments.max_total_bytes=10000000"
  ]
}
```

Operators fix every misconfig in one restart cycle. License-gate failures stay as `slog.Warn` (free-mode is valid). "No adapters enabled" IS fatal (added to the consolidated slice; without an adapter the relay is non-functional).

### 8.3 Metrics endpoint failures

The metrics server is operationally independent of the main relay:

| Failure | Behavior |
|---|---|
| `cfg.Observability.Metrics.Enabled=false` | `ListenAndServe` returns nil immediately; main HTTP server starts normally |
| Port already bound | metrics server exits with error; main relay continues; logged as `slog.Error("metrics: ListenAndServe failed: %v")` once |
| `/metrics` request during scrape | always 200 (passthrough to `promhttp.Handler()`) |
| OOM from cardinality explosion | impossible by design (bounded RoutePattern + enum labels); documented in self-hosting.md |

**Independence invariant**: observability shouldn't be able to brick the service it observes. A misconfigured metrics port doesn't take the relay down.

### 8.4 Lint signal management

Adding `golangci-lint` + `eslint` + `prettier` to CI will surface false-positives on Phase 0-6 code. Phase 7-i includes an **initial-fix sweep** BEFORE wiring the lint jobs into CI:

1. Run each linter locally; capture every finding.
2. Triage each finding:
   - **Real bug** → fix it (commit message includes the lint rule name).
   - **False positive on intentional code** → add a targeted `//nolint:rule // reason` (Go) or `// eslint-disable-next-line rule -- reason` (TS) with the reason. No blanket disables.
   - **Style preference difference** → either accept (reformat) or narrow the rule in `.golangci.yml` / `.eslintrc.cjs`.
3. **Only after the sweep is clean**, wire the lint jobs into `ci.yml`.

This is L023-adjacent — every lint finding deserves a deliberate verdict before becoming a CI gate. The sweep results land as one or more `chore(7-i): initial lint sweep` commits BEFORE the CI gate commits, so reverting the gate (if it turns out too noisy) doesn't lose the actual fixes.

### 8.5 Snapshot-build failures

`goreleaser release --snapshot --clean` is the Phase 7-v final smoke target. Failure modes:

| Failure | Cause | Resolution |
|---|---|---|
| `unsupported GOOS/GOARCH` | typo in matrix | Fix `.goreleaser.yaml` |
| `docker build failed` | Dockerfile syntax error OR base image unavailable | Fix Dockerfile or pin a working base image SHA |
| `archive: file not found: README.md` | `archives.files` references missing file | Update `archives.files` |
| `npm pack: package.json missing required field` | TS package manifest incomplete | Fill in `description`, `repository`, `license`, etc. |
| `goreleaser version mismatch` | local goreleaser ≠ pinned version | Reinstall the pinned version |

A failing snapshot smoke is a build-fail in Phase 7-v.

### 8.6 docker-compose demo failures

| Failure | Cause | Resolution |
|---|---|---|
| `relay: cannot find /etc/openintake/config.yaml` | mount path wrong | Fix `volumes:` in `docker-compose.yml` |
| `webhook-receiver returns 502` | network alias misconfigured | Confirm `depends_on` + service names match `config.yaml`'s `adapters.webhook.url` |
| `port 18080 in use` on host | conflict with developer's other processes | Document port-remap workaround in README.md |

The demo smoke handles teardown via `defer docker-compose down -v` so a failed run doesn't leave containers running.

### 8.7 Observability semantics (operator-facing)

**What the 4 metrics tell an operator:**

| Metric | Operational meaning | Common queries |
|---|---|---|
| `intake_http_requests_total{path,status}` | Total request volume per endpoint by status code | `rate(intake_http_requests_total{status=~"5.."}[5m])` → 5xx error rate |
| `intake_http_request_duration_seconds{path}` | Latency distribution per endpoint | `histogram_quantile(0.95, rate(..._bucket{path="/v1/intake/submit"}[5m]))` → p95 submit latency |
| `intake_llm_tokens_total{provider,direction}` | LLM cost tracking | `rate(..._total{direction="output"}[1h])` → output token burn rate |
| `intake_adapter_calls_total{adapter,result}` | Adapter health | `rate(..._total{result="error"}[5m])` → adapter failure rate |

Documented in `docs/self-hosting.md` with PromQL examples. No Grafana dashboards in Phase 7 (defer to v1+).

**Intentional v0 gaps** (one-line additions for v1+): per-tenant breakdown, per-IP request rate, captcha pass/fail counts, attachment validation outcomes. Phase 7 keeps the metric surface minimal so we don't ship metrics no one queries.

---

## 9. Frozen seams Phase 7 must NOT modify

Phase 7 inherits the cumulative frozen-seam set from Phases 0-6:

- `adapter.Adapter` interface — Phase 6 frozen
- `payload.IntakePayload` / `payload.Attachment` generated types — Phase 0 frozen
- `schema/payload.v1.json` — Phase 0 frozen; NO schema change in Phase 7
- `auth.Middleware.Handler` signature, `auth.SessionContext`, `auth.Store`, `auth.NewMiddleware*` — Phase 4+5 frozen
- Phase 5 abuse gates (per-IP, per-session, daily budget, CAPTCHA) — Phase 5 frozen
- `intake/license` canonicalization — Phase 3 frozen
- `payloadbuild.Builder` constructor — Phase 6 additive only
- `attachvalidate.ValidateAll` / `DecodeOne` signatures + sentinel errors — Phase 6 frozen
- Each adapter's `Capabilities()` shape — Phase 6 frozen
- The chi route-registration shape (`registerIntakeRoutes`) — additive only; Phase 7 adds `metrics.Middleware()` to the chain; NO new endpoints on the relay's main HTTP server; `/metrics` lives on a SEPARATE server
- The 8 wire-level attachment error codes — Phase 6 frozen

**What Phase 7 IS allowed to modify:**

- `relay/cmd/relay/main.go` orchestration (explicit goal of FOLLOWUPS I1+I2)
- `relay/internal/config/config.go` ADDITIVELY (adds `ObservabilityConfig` + `MetricsConfig`)
- `relay/internal/server/deps.go` ADDITIVELY (adds `Metrics *metrics.Registry` field)
- `relay/internal/server/{turn,submit}.go` for single-line metric-record sites (no behavior change)
- `relay/cmd/relay/main_test.go` (adds `TestAccumulateStartupProblems_*` tests)
- `relay/cmd/relay/smoke/run-q9-smoke.sh` (M4 subshell cleanup; output unchanged)
- All existing source files for lint initial-fix sweep findings (whitespace, ordering, unused-variable cleanups — no semantic change; verified by full test suite still green)
- `.github/workflows/ci.yml` (adds lint-go, lint-ts, test-go, test-ts, goreleaser-check, snapshot-build jobs)

---

## 10. Sub-plan decomposition

| # | Plan | Driver | Effort |
|---|---|---|---|
| **7-i** | Relay code: FOLLOWUPS (I1+I2+M2+M4) + Prometheus metrics package + middleware wiring + record sites + lint configs + initial-fix sweep + CI extension | the seam | L |
| **7-ii** | Release artifacts: `relay/Dockerfile` + `relay/.goreleaser.yaml` + `.github/workflows/release.yml` (authored, never executed) + `goreleaser` pin extension to `scripts/check-pins.sh` | release config | M |
| **7-iii** | Demo stack: `examples/docker-compose/` (3 services, builds relay image from 7-ii Dockerfile, full stack boots locally) | demo | M |
| **7-iv** | Docs + governance: `docs/{quickstart,self-hosting,license,adapters}.md` + repo `README.md` rewrite + `LICENSE` + `CONTRIBUTING.md` + `SECURITY.md` + `COMMERCIAL.md` | docs + governance | M |
| **7-v** | Final smoke + LESSONS + README evidence: `core/smoke/drive-docker-compose.ts` + run all 8 §11 final-smoke items + write Phase 7 README §7 evidence + append LESSONS + rename FOLLOWUPS.md → FOLLOWUPS-resolved.md | live evidence | S |

### Dependency graph

```
7-i (relay code: FOLLOWUPS + Prometheus + lint configs + CI extension)
      │
      ├──► 7-ii (release artifacts: Dockerfile + .goreleaser.yaml + release.yml)
      │       │
      │       ▼
      │     7-iii (docker-compose demo)                                ┐
      │                                                                 │ parallelizable
      └──► 7-iv (docs + governance)                                    ┘ with 7-iii
                                                  ▼
                                                7-v (final smoke + LESSONS + evidence)
```

Wave-by-wave execution under subagent-driven-development:

| Wave | Sub-plans | Why |
|---|---|---|
| 1 | 7-i | Locks Deps shape + lint rulesets + metrics surface |
| 2 | 7-ii | Locks Dockerfile + goreleaser config that 7-iii consumes |
| 3 | 7-iii + 7-iv | Disjoint file territory; both dispatched as parallel subagents |
| 4 | 7-v | Consumes outputs from all prior waves |

---

## 11. Testing strategy

### 11.1 Credit-free unit + integration coverage

Every line of new code has a Go unit test (httptest mocks) or a Vitest. Zero paid-credit consumption. New configs (Dockerfile, goreleaser.yaml, docker-compose.yml, lint configs) verified by their respective `check` tools and a snapshot smoke.

#### 7-i — Relay code

| Package / file | Tests |
|---|---|
| `relay/internal/metrics/metrics_test.go` (new) | `TestRegistry_DisabledIsNoOp`, `TestRegistry_EnabledIncrementsCounters` (via `testutil.ToFloat64`), `TestRegistry_LLMTokens_*` (provider+direction label combos), `TestRegistry_AdapterCall_*` (adapter+result label combos), `TestRegistry_ListenAndServe_PortConflict` (binds addr first, asserts ListenAndServe returns error without killing process), `TestRegistry_PathLabelUsesChiRoutePattern` (cardinality safety: 100 requests with different query strings → 1 series) |
| `relay/internal/config/config_test.go` (extend) | `TestLoad_ObservabilityDefaults`, `TestLoad_ObservabilityExplicit` |
| `relay/cmd/relay/main_test.go` (extend; closes I2) | `TestAccumulateStartupProblems_Empty`, `TestAccumulateStartupProblems_Phase5Only`, `TestAccumulateStartupProblems_Phase6Only`, `TestAccumulateStartupProblems_AdapterConfigureFails`, `TestAccumulateStartupProblems_NoAdaptersEnabled`, `TestAccumulateStartupProblems_LicenseGateWarnsNotFails`, `TestAccumulateStartupProblems_AllCombined` |
| `relay/internal/server/turn_test.go` (extend) | `TestTurnHandler_RecordsLLMTokensOnSSEDone` |
| `relay/internal/server/submit_test.go` (extend) | `TestSubmitHandler_RecordsAdapterCallSuccess`, `TestSubmitHandler_RecordsAdapterCallError` |
| `relay/cmd/relay/smoke/run-q9-smoke.sh` (M4) | Subshell refactor verified by re-running existing Q9 smoke; output unchanged |

#### 7-ii — Release artifacts

| Artifact | "Test" |
|---|---|
| `relay/Dockerfile` | `docker build -t openintake-relay:test relay/` exits 0; image runs as `nonroot`; image size < 50 MB |
| `relay/.goreleaser.yaml` | `goreleaser check` exits 0; `goreleaser release --snapshot --clean` produces all 5 archives + image in `./dist/` |
| `.github/workflows/release.yml` | `actionlint` exits 0; YAML structurally valid; secret references syntactically present |
| `.github/workflows/ci.yml` extension | `actionlint` exits 0 on new jobs; each new job's `run:` works when executed locally |

#### 7-iii — Docker-compose demo

| Test | Mechanism |
|---|---|
| `core/smoke/drive-docker-compose.ts` | `docker-compose up -d` → poll `/v1/health` → POST `/init` → SSE `/turn` → POST `/submit` → assert webhook-receiver logged the canonical payload → `docker-compose down -v` |
| Metrics endpoint reachable | `curl http://localhost:19090/metrics` returns text starting with `# HELP intake_http_requests_total` |
| Containers run as nonroot | `docker exec openintake-relay id -u` prints `65532` |

#### 7-iv — Docs + governance

Internal-consistency checks: `license.md` claims match `LICENSE` + `COMMERCIAL.md`; `adapters.md` matrix matches each adapter's actual `Configure()` keys; `README.md` links work; `CONTRIBUTING.md` "Run `npm test`" actually works. Manual walkthrough verification during 7-v.

#### 7-v — Final smoke (mandatory per PHASE_PLANNING §7)

```
1. Startup gate smoke (no LLM credit; self-runnable; after 7-i):
   Combined misconfig fixture covers Phase 5 + Phase 6 + Phase 7 (adapter
   Configure failure). Assert ONE log line with count >= 6 problems listing
   every distinct issue, then exit 1.

2. Metrics endpoint smoke (no LLM credit; self-runnable; after 7-i):
   a. Enabled=false → /metrics not reachable (connection refused on :9090).
   b. Enabled=true → /metrics returns 200 with all 4 series declared.
      Drive 5 /init requests; assert intake_http_requests_total counter
      increased by exactly 5 for path=/v1/intake/init,status=200.

3. Snapshot release smoke (no public artifacts; after 7-ii):
   `goreleaser release --snapshot --clean` produces ./dist/ with all 5
   archives + SHA256SUMS.txt + ghcr.io/clubpetey/openintake-relay:snapshot tag in
   local docker daemon + CHANGELOG.md (NOT uploaded). Each archive extracted
   and the binary's --version printout matches the tag string.

4. npm pack dry-run smoke (after 7-ii):
   `npm pack -w @openintake/core` → intake-core-0.0.0.tgz; `npm publish
   -w @openintake/core --dry-run` exits 0. Same for @openintake/vue. Tarball
   contents inspected: no .env, no local-dev/, no secrets.

5. docker-compose demo smoke (after 7-iii):
   `cd examples/docker-compose && docker-compose up -d` then
   drive-docker-compose.ts asserts /health, /init, /turn SSE, /submit,
   webhook-receiver log. Then `docker-compose down -v`.

6. Docs walkthrough smoke (manual; after 7-iv):
   Maintainer follows docs/quickstart.md from a fresh clone and reaches
   "ticket in webhook log" within 30 min. Same for self-hosting.md docker
   path.

7. Phase 1-6 regression (after 7-v wiring):
   drive-attachments.ts + drive-abuse.ts + drive-auth-email.ts pass
   unchanged under Phase 7's middleware chain (metrics.Middleware()
   transparent). Existing per-adapter unit tests pass. go test -race ./...
   green. core/ + vue/ npm test green. verify-contract + check-pins green.
   go mod tidy is a no-op.

8. Lint smoke (after 7-i sweep + CI gate):
   golangci-lint run ./... in relay → 0 issues
   eslint . in core/ + vue/ → 0 errors
   prettier --check . → 0 unformatted files
   All three integrated as CI jobs that pass on the merge commit.
```

A phase is NOT done until this smoke passes from a clean state. ALL 8 items are self-runnable; no maintainer pauses.

### 11.2 No maintainer-paused live smokes

Per the scope-boundary decision (provisional infrastructure only), Phase 7 has NO live smokes that touch external services. Everything verified locally:
- snapshot artifacts to `./dist/`
- docker images in local daemon
- npm packs in working tree
- docker-compose on `localhost`

The only "live" thing is Docker Desktop / Linux Docker engine — already on every developer's machine, no creds, no external endpoints.

---

## 12. Build-fail discipline (extends Phase 6 checklist)

Every silent-failure shape gets a CI gate. Phase 7 additions:

- [ ] `go build ./... && go vet ./...` fails in `relay/` → **Fail**
- [ ] `go test -race ./...` fails → **Fail**
- [ ] `golangci-lint run ./...` reports any issue → **Fail** (after initial sweep)
- [ ] `npm run type-check && npm run build && npm run test` fails in `core/` or `vue/` → **Fail**
- [ ] `eslint .` or `prettier --check .` reports any issue → **Fail** (after initial sweep)
- [ ] `scripts/verify-contract.sh` regresses → **Fail** (no schema change in Phase 7)
- [ ] `scripts/check-pins.sh` regresses → **Fail** (extends for `prometheus/client_golang` + `goreleaser`)
- [ ] `go mod tidy` produces any diff → **Fail** (one new module: `prometheus/client_golang`)
- [ ] `goreleaser check` reports any issue → **Fail**
- [ ] `goreleaser release --snapshot --clean` fails to produce any of the 5 archives → **Fail**
- [ ] `docker build -t openintake-relay relay/` fails → **Fail**
- [ ] `docker inspect openintake-relay --format '{{.Config.User}}'` returns root or empty → **Fail** (distroless nonroot)
- [ ] `docker images openintake-relay --format '{{.Size}}'` shows > 50 MB → **Fail**
- [ ] `npm publish --dry-run -w @openintake/core` or `-w @openintake/vue` fails → **Fail**
- [ ] `tar -tf dist/intake-core-*.tgz | grep -E '\.env|secrets|local-dev'` returns any line → **Fail** (secret leak)
- [ ] `actionlint .github/workflows/*.yml` reports any issue → **Fail**
- [ ] `cd examples/docker-compose && docker-compose config` fails → **Fail**
- [ ] `cfg.Observability.Metrics.Enabled=false` AND `/metrics` reachable on `:9090` → **Fail** (off-by-default invariant)
- [ ] `cfg.Observability.Metrics.Enabled=true` AND `/metrics` returns anything other than text/plain with `# HELP intake_` prefix → **Fail**
- [ ] Metrics port conflict crashes the main HTTP listener → **Fail** (independence invariant)
- [ ] Phase 6 FOLLOWUPS I1+I2+M2+M4 not all marked closed in `ai/tasks/phase-6/FOLLOWUPS-resolved.md` after 7-i → **Fail**
- [ ] Phase 1+4+5+6 regression: existing smokes pass unchanged → **Fail otherwise**

---

## 13. Final smoke (mandatory per PHASE_PLANNING §7)

See §11.1's items 1-8. The Phase 7 README's §7 evidence subsection (written during 7-v) records the actual command output + verdict for each item.

---

## 14. New patterns Phase 7 establishes

These become referenceable patterns for any future phase that adds release/ops surface:

- **Snapshot-then-publish split**: every release artifact tool has a `--snapshot` / `--dry-run` mode. Phase 7-v exercises ALL of them and produces ZERO publish-side mutations. The actual publish is a deliberate, separately-gated action.
- **Initial lint sweep before CI gate**: never enable a lint as a CI gate against existing code without first running the sweep + triaging every finding. Otherwise the gate becomes a barrier to all future PRs from day 1.
- **Metrics server lives independently from main HTTP**: observability shouldn't be able to brick the service it observes. Metrics-port conflicts are warnings, not fatal.
- **Off-by-default for new observability surface**: every new operator-facing thing in Phase 7 (`metrics.enabled`) defaults to off; operators opt in.
- **Distroless multi-stage Docker**: the relay's distroless Dockerfile + `nonroot` user pattern is the template for any future Go binary in this monorepo.

These should become L024-L028 lessons in `ai/LESSONS.md` (authored during 7-v).

---

## 15. Inconsistencies to fix in PROJECT.md (deferred, not blocking)

Two contradictions noticed during Phase 7 design — fix when convenient, does not block:

- §14 repo layout lists `core/src/screenshot.ts` and `core/src/auth.ts` files that never existed (capture+attachments+client are the actual core/ files). The §14 listing also predates Phase 6's redactor + strip components. Refresh §14 to match the current state.
- §15 lists `openintake-license` (CLI) in the build/release section but `license-tool/` is excluded from goreleaser per the decomposition spec (Q10 resolution: maintainer-only). §15 should explicitly note the exclusion.

These are doc-only inconsistencies; the actual build pipeline is unaffected. A short PROJECT.md cleanup task can fold these into 7-iv if convenient.

---

## 16. Next step

Per the brainstorming → writing-plans workflow, the next planning action is to author Phase 7 under `ai/tasks/phase-7/` per [ai/PHASE_PLANNING.md](../../ai/PHASE_PLANNING.md): the README (this spec's index) plus five sub-plans (7-i through 7-v). Each sub-plan has its own mandatory smoke; the phase README carries the final §11 / §13 smoke.

*End of Phase 7 design spec.*
