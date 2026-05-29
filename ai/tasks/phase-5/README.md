# Phase 5 — Abuse & Spend Control

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement the sub-plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Adds the four guardrails from PROJECT.md §10 behind the **frozen** Phase 1+4 seams: **per-IP token bucket**, **per-session turn/token caps**, **daily LLM spend cap**, and **CAPTCHA** at `/v1/intake/init`. Tightens Q9 to fail-closed: the dispatcher rejects anonymous sessions when `auth.modes.anonymous=false`, and a single consolidated startup gate covers every combinable misconfig (anonymous-without-captcha + sso both/neither set + invalid trusted-proxy CIDR + unsupported `action_on_exceeded`). No new external Go modules — `golang.org/x/time` (already indirect via `keyfunc/v3 v3.8.0`) is promoted to direct require.

## 1. Spec link

- Phase 5 design: [docs/specs/2026-05-28-phase-5-abuse-and-spend-control-design.md](../../../docs/specs/2026-05-28-phase-5-abuse-and-spend-control-design.md)
- Parent decomposition: [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](../../../docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 5 row + §4 Q9 resolution)
- Source of truth for scope/contracts: [docs/PROJECT.md](../../../docs/PROJECT.md) §10 (rate limiting + abuse), §17 (security), §19 Q9 (anonymous-without-CAPTCHA)
- Secrets seam: [docs/specs/2026-05-27-configuration-and-secrets-design.md](../../../docs/specs/2026-05-27-configuration-and-secrets-design.md) (`config.ResolveSecret`)
- Phase 4 patterns mirrored: [docs/specs/2026-05-28-phase-4-auth-breadth-design.md](../../../docs/specs/2026-05-28-phase-4-auth-breadth-design.md) (middleware composition + Deps + factory-with-mutual-exclusivity)
- Phase-1+4 frozen seams: `relay/internal/auth/middleware.go` (Handler signature unchanged; new `modesAnonymous` field added via a wider constructor), `relay/internal/server/server.go` (chi route shape unchanged — Phase 5 adds two new middlewares in the existing `/v1/intake` group)

## 2. Architectural Decision Record (ADR) summary

- **`golang.org/x/time/rate` as the per-IP limiter primitive.** Already indirect via `keyfunc/v3 v3.8.0`; Phase 5 promotes it to a direct require. Zero new download — already in `go.sum`. Caret forbidden (abuse-control boundary). Revisit trigger: multi-instance deployment requires a shared limiter (then: Redis + an external token-bucket impl).
- **Eager-GC + injectable-clock pattern for all in-memory stores (perip, budget, sessionMeta).** Matches L014; no background goroutines; deterministic tests. Revisit trigger: per-op cost becomes measurable under load (then: amortize the GC pass).
- **Reserve-then-Commit budget semantics with estimates.** Gates spend on conservative estimates; reconciles with actuals on `SSEDone`. Tolerates a small race window for concurrent Reserves (acceptable for v0 soft cap). Revisit trigger: hosted-relay multi-tenant billing requires transactional accuracy (then: per-tenant mutex + held reservations).
- **CAPTCHA secret never appears in any error or log line.** L005 redact-before-truncate pattern applied to siteverify response bodies. Replay protection layered inside the verifier independent of the provider's own single-use semantics (defense in depth).
- **Two-call `/init` dance when CAPTCHA is required.** Widget calls `/init` once with no `captcha_token`; relay returns 400 `captcha_required` with the same `capabilities` + `captcha` discovery fields the success path returns; widget solves the challenge and re-calls `/init`. One new error code. Revisit trigger: v1 may add `GET /v1/intake/capabilities` for cleaner discovery.
- **Q9 strict-anonymous + consolidated startup gate.** Dispatcher rejects anonymous when `auth.modes.anonymous=false`; startup gate emits a single consolidated error covering anonymous-without-captcha + sso-both + sso-neither + invalid CIDR + unsupported `action_on_exceeded`. Operators fix all misconfigs in one restart cycle. Revisit trigger: a new auth mode or guardrail introduces another misconfig class (extend the same `problems` slice).
- **Per-IP middleware sits BEFORE auth.** Prevents unauthenticated abusers from exhausting the auth-store or email rate-limiter with bursts. `/v1/health` and `/v1/version` stay outside `/v1/intake` and are NOT rate-limited (load-balancer liveness probes must always succeed).

This phase does NOT add: persistent/shared rate-limit state (v1+), per-tenant CAPTCHA thresholds (v0 one global config), CAPTCHA on `/turn`/`/submit` (v0 one-shot at `/init`), cookie-based long-lived CAPTCHA bypass (v1+), sliding-window per-IP limiter (v0 token bucket only), `action_on_exceeded: "queue"` (v0 only ships `"reject"`), or an additional `GET /v1/intake/capabilities` endpoint (v1+).

## 3. Sub-plan index

| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 5-i | [Config + middleware chain seam + Q9 startup gate + dispatcher hardening](5-i-config-middleware-seam-Q9-plan.md) | the seam | M | Live + smoked |
| 5-ii | [Per-IP limiter + per-session counters + daily budget tracker](5-ii-ratelimit-budget-plan.md) | rate-limit primitives | M | Live + smoked |
| 5-iii | [CAPTCHA verifier + /init two-call dance](5-iii-captcha-plan.md) | CAPTCHA | M | Live + smoked |
| 5-iv | [Final smoke + docs](5-iv-smoke-docs-plan.md) | live abuse + maintainer-paused CAPTCHA | S | Live + smoked |

## 4. Dependency graph

```
5-i (config + middleware chain seam + Q9 + dispatcher hardening)
      │
      ├──► 5-ii (per-IP + per-session + daily budget)   ┐
      │                                                 │  mutually independent;
      └──► 5-iii (captcha verifier + /init integration) ┘  parallelizable after 5-i
                  │
                  ▼
            5-iv (final smoke + docs)
```

5-i locks the config shape, adds the two new middlewares (clientIP resolver + per-IP limiter stub), wires the Q9 consolidated startup gate, hardens the auth dispatcher with `modesAnonymous`, and extends `Deps` with optional `CaptchaVerifier`/`Budget`/`TrustedProxies`/`CaptchaCfg` fields. 5-ii and 5-iii each populate one slot of the Deps surface (5-ii: per-IP Limiter + Budget Tracker + Store-with-caps; 5-iii: CaptchaVerifier + /init body parsing). 5-iv records the live evidence + LESSONS.

## 5. Tool version pin list

Phase 5 introduces **zero** new Go modules. `golang.org/x/time` is already in `relay/go.sum` at `v0.9.0` as an indirect dep of `MicahParks/keyfunc/v3 v3.8.0`. The only `go.mod` edit is promoting it from the `// indirect` block to the primary `require` block — a one-line move; `go mod tidy` after the edit is a no-op confirming the existing pin.

| Tool | Version | Reason |
|---|---|---|
| `golang.org/x/time` | promote from indirect to direct at `v0.9.0` (matches existing go.sum) | `rate.Limiter` backs the per-IP token bucket. Already downloaded; promotion locks the API contract as Phase 5's. Caret forbidden — abuse-control surface. |

`scripts/check-pins.sh` extended with a one-line check for `golang.org/x/time` matching the existing `golang-jwt`/`keyfunc/v3` style.

## 6. Build-fail checklist

- [ ] `go build ./...` / `go vet ./...` fails in `relay/`. **Fail.**
- [ ] Any Go test fails (`go test ./...`). **Fail.**
- [ ] The Phase-0 contract gate regresses (`scripts/verify-contract.sh`). **Fail.**
- [ ] A CAPTCHA secret key appears in a log line, error string, or response body. **Fail.**
- [ ] `golang.org/x/time` pinned with a caret or `@latest` in `relay/go.mod`. **Fail** (check-pins gate).
- [ ] `auth.modes.anonymous=true` AND NOT (`captcha.enabled=true` AND `"anonymous" ∈ captcha.required_for`) AND `auth.anonymous.allow_without_captcha` absent/false → relay starts. **Fail** (must fatal at startup; covers `enabled:false`, `required_for:[]`, and `required_for` without `"anonymous"`).
- [ ] `auth.modes.sso=true` with both `jwks_url` AND `hs256_secret_env` set → relay starts. **Fail** (consolidated Q9 gate must list this in the problems slice).
- [ ] `auth.modes.sso=true` with neither set → relay starts. **Fail** (same gate).
- [ ] An invalid CIDR in `server.trusted_proxies` → relay starts. **Fail.**
- [ ] `daily_llm_budget.action_on_exceeded` set to anything other than `"reject"` → relay starts. **Fail** (Phase 5 only ships "reject"; "queue" documented as v1+).
- [ ] Per-IP burst exceeded → response is anything other than 429 + `Retry-After: 1`. **Fail.**
- [ ] Daily budget exceeded → response is anything other than 503 + numeric `Retry-After`. **Fail.**
- [ ] Per-session 21st turn → response is anything other than 429 + numeric `Retry-After`. **Fail.**
- [ ] `auth.modes.anonymous=false` AND a request presents a valid X-Intake-Session → request succeeds. **Fail** (Q9 strict dispatcher must reject).
- [ ] CAPTCHA siteverify response body is logged at Info level (unredacted). **Fail.**
- [ ] `/v1/health` or `/v1/version` returns 429 under sustained probe load. **Fail** (probe endpoints must not be rate-limited).
- [ ] Phase 1+4 anonymous-default behavior regresses: `auth.modes.anonymous=true` AND `captcha.enabled=true` AND a Phase 1 client without a `captcha_token` hitting /init → response is anything other than 400 `captcha_required`. **Fail.**

## 7. Final smoke (mandatory)

Proves the Phase 5 deliverable end-to-end. The unit layer (mocked siteverify via `httptest`, injected-clock per-IP limiter, injected-clock budget, injected-clock sessionMeta, in-process auth/middleware tests) is fully credit-free and runs in `go test ./...`. The **live** rate-limit + per-session + budget smokes are self-runnable against a local Ollama-or-fake provider (zero paid-credit consumption). The **live CAPTCHA smoke pauses for the maintainer** (needs a real Turnstile test sitekey + secret — Cloudflare publishes test keys free of charge; no real human interaction needed).

```
1. Q9 startup smoke (no LLM credit; self-runnable):
   For each of the 4 misconfig YAMLs:
     - anonymous-no-captcha.yaml: auth.modes.anonymous:true, captcha.enabled:false
     - sso-both.yaml: auth.modes.sso:true, jwks_url + hs256_secret_env both set
     - sso-neither.yaml: auth.modes.sso:true, neither set
     - bad-cidr.yaml: server.trusted_proxies: ["not-a-cidr"]
   Start the relay binary; assert exit code 1 and stdout contains the single
   structured Error log line "relay: startup config errors" with the matching
   problem text. The "combined" YAML (all four together) emits one log line
   listing all four problems.

2. Per-IP rate-limit smoke (no LLM credit; self-runnable):
   Configure ratelimit.per_ip = {requests_per_second:1, burst:5}.
   curl burst of 10 requests in 1s against /v1/intake/init →
   assert exactly 5×200 + 5×429 with Retry-After:1.
   Control: same burst against /v1/health → all 10×200 (probe endpoints exempt).

3. Per-session cap smoke (uses local Ollama-or-fake provider; no credit):
   Configure ratelimit.per_session.max_turns:20.
   Drive 20 /turn calls; assert all 200. 21st returns 429 session_turns_exhausted
   with Retry-After ≈ remaining TTL.

4. Daily budget smoke (uses local Ollama-or-fake provider; no credit):
   Configure ratelimit.daily_llm_budget = {max_input_tokens:100, max_output_tokens:100}.
   One /turn burns ~80 input tokens. Second /turn refused with 503
   daily_budget_exhausted and Retry-After ≈ secs-to-next-UTC-midnight.

5. Live CAPTCHA smoke (PAUSE for maintainer; uses Cloudflare's documented test
   sitekey 1x00000000000000000000AA + matching test secret 1x0000000000000000000000000000000AA):
   Configure captcha.enabled:true, provider:turnstile, site_key + secret_key_env.
   a. First /init with no body → 400 captcha_required + capabilities.requires_captcha:["anonymous"] + captcha.{provider,site_key}.
   b. Second /init with {captcha_token:<token from always-passes test sitekey>} → 200 + session_id.
   c. Second /init with the same token → 401 captcha_failed (reason:"duplicate" — replay protection).

6. Phase 4 regression: drive-auth-email.ts and drive-auth-sso.ts (from Phase 4)
   should pass unchanged under Phase 5's chain — proves the middleware
   ordering + dispatcher hardening + budget gate did not regress live auth.

7. Phase 1 regression: anonymous /init → /turn flow still works when
   captcha.enabled:false AND auth.anonymous.allow_without_captcha:true is set
   (the Q9 escape hatch).
```

A phase is NOT done until this smoke passes from a clean state. Steps 1-4 + 6-7 are self-runnable; step 5 (live CAPTCHA) pauses for explicit maintainer go-ahead per the credit/secret guard.

### Smoke status (2026-05-28)

- **Credit-free unit + integration layer — COMPLETE.** All four sub-plans implemented on `phase-5`, each through subagent review with fix-loops. `go build/vet/test -race ./...` green in `relay/`; `scripts/verify-contract.sh` + `scripts/check-pins.sh` green; `go mod tidy` is a no-op (`golang.org/x/time v0.9.0` promoted from indirect to direct with no other go.mod/go.sum drift). Covered credit-free: `perip.Limiter` Allow/eager-GC with injected clock; `budget.Tracker` Reserve/Commit/UTC-day-reset with injected clock + tenant isolation; `auth.Store.CheckSession`/`RecordTurn` turn-cap + token-cap + TTL with injected clock; `captcha.providerVerifier` Turnstile + hCaptcha siteverify + 5-minute single-use replay set + L005 redact-before-error; Q9 strict-anonymous dispatcher branch + consolidated startup gate covering anonymous-without-captcha + sso-both/neither + invalid CIDR + unsupported `action_on_exceeded`; clientIP middleware + trusted-proxy CIDR walking; /init two-call dance with `captcha_required` discovery shape.

- **Q9 startup-gate smoke — COMPLETE.** All 6 misconfig YAMLs (`anonymous-no-captcha`, `sso-both`, `sso-neither`, `bad-cidr`, `bad-action-on-exceeded`, `combined`) exit 1 with the structured `relay: startup config errors` log line listing every distinct problem. The combined YAML emits one log line listing all five problems — operators fix in one restart cycle.

- **Strict-anonymous dispatcher smoke — COMPLETE.** With `auth.modes.anonymous=false`, both an unknown X-Intake-Session and a valid (Store-issued) X-Intake-Session return 401 `anonymous_disabled`; timing-safety verified (constant-time path; no observable branch on session existence).

- **Per-IP rate-limit smoke — COMPLETE.** Burst of 10 requests against `/v1/intake/init` with `{rps:1, burst:5}` → exactly 5×200 + 5×429 with `Retry-After: 1`. Control: same burst against `/v1/health` → 10×200 (probe endpoints exempt from per-IP gate, as required).

- **Per-session cap smoke — COMPLETE.** Drive 3 `/turn` calls with `max_turns:3` → all 200; 4th → 429 `session_turns_exhausted` with `Retry-After` ≈ TTL remainder (within ±1s of expected).

- **Daily-budget smoke — COMPLETE.** First `/turn` burns ~80 tokens against `{max_input:100, max_output:100}`; second `/turn` → 503 `daily_budget_exhausted` with `Retry-After` ≈ secs-to-next-UTC-midnight. Tenant isolation verified: tenant A exhaustion does not affect tenant B's budget.

- **`drive-abuse.ts` — COMPLETE.** All three rate-limit gates exercised end-to-end via @intake/core-style fetch driver. First run failed because the abuse fixture's budget=(100,100) caused the budget gate to fire on turn 3 before `max_turns=3` could fire on turn 4; fix at `68382c0` raised budget to (150,150) so per-session fires first (recorded as L019). Re-run all green.

- **Live CAPTCHA smoke — COMPLETE** (2026-05-28, Cloudflare Turnstile test sitekeys `1x00000000000000000000AA` / `1x0000000000000000000000000000000AA` and `2x0000000000000000000000000000000AA` always-fails). Discovery (`/init` with no body) → 400 `captcha_required` carrying `capabilities.requires_captcha:["anonymous"]` + `captcha.{provider,site_key}`. Mint (`/init` with a passing token) → 200 + session_id. Replay (same token re-presented) → 401 `captcha_failed` with `reason:"duplicate"` from the 5-minute replay set. Fails-secret (always-fails sitekey) → 401 `captcha_failed` with `reason:<provider error-code>`. **L005 confirmed:** grep over the live-smoke log for the secret bytes returned zero matches across all four scenarios.

- **Phase 1 + Phase 4 regression smokes — COMPLETE.** With `auth.anonymous.allow_without_captcha:true` the Phase-1 anonymous /init → /turn flow passes unchanged. With `captcha.enabled:true, captcha.required_for:["anonymous"]`, Phase 4's `drive-auth-email.ts` and `drive-auth-sso.ts` pass unchanged — proves Phase 5's middleware chain (per-IP → clientIP → CAPTCHA-at-/init → auth dispatcher → per-session → budget) does not regress Phase 4. Failure injection: forcing a Phase-5 gate reject (per-session exhausted) confirms the reject reaches the consumer; passing requests still reach the provider/SMTP layer past every Phase-5 gate.

- **Phase 5 coverage: 4/4 guardrails proven** — per-IP (live), per-session (live), daily-budget (live), CAPTCHA (live). Q9 strict-anonymous + consolidated startup gate (live across 6 misconfigs). Frozen Phase-1+4 seams unmodified; auth Handler signature + SessionContext + payloadbuild unchanged.

## 8. Shared Contracts (SINGLE SOURCE OF TRUTH)

These shapes are **frozen** in the noted sub-plan; later sub-plans consume them unchanged.

### 8.1 The frozen Phase-1+4 seams (UNCHANGED)

- `auth.Middleware.Handler` signature — chi-compatible `func(http.Handler) http.Handler`. Phase 5 adds a new field (`modesAnonymous bool`) via a wider constructor `NewMiddlewareWithModes`; `NewMiddleware(store, email, sso)` is preserved as a wrapper defaulting `modesAnonymous=true`. The Handler body gains a single guard `if !m.modesAnonymous { 401 }` inside the existing anonymous fall-through branch.
- `auth.SessionContext` struct — UNCHANGED. Existing fields (SessionID, AuthMode, Verified, UserID, Email, DisplayName, Custom) stay; per-session counters live in a NEW private `sessionMeta` map on the Store, not on SessionContext.
- `auth.WithSession` / `auth.FromContext` — UNCHANGED.
- `auth.Store` — Phase 1 `NewStore()` preserved as a wrapper around the new `NewStoreWithCaps`; Phase 5 adds private `sessionMeta` map + `CheckSession` + `RecordTurn` methods.
- `adapter.Adapter` interface — UNCHANGED.
- The chi route-registration shape (`registerIntakeRoutes`) — additive only (two new middlewares wrap the existing `/v1/intake` group; the per-route registrations are unchanged).
- Generated `relay/internal/payload/types.go` — never modified.
- `payloadbuild.Builder` — UNCHANGED. Already reads `SessionContext.{AuthMode,Email,UserID,DisplayName,SessionID}` correctly.

### 8.2 Config additions (additive — `relay/internal/config/config.go`, FROZEN in 5-i)

```go
// ServerConfig gains TrustedProxies:
type ServerConfig struct {
    Addr           string   `yaml:"addr"`
    ExternalURL    string   `yaml:"external_url"`
    CORSOrigins    []string `yaml:"cors_origins"`
    TrustedProxies []string `yaml:"trusted_proxies"`  // NEW; CIDR list; empty = none
}

// AuthConfig gains Anonymous sub-struct:
type AuthConfig struct {
    Modes     AuthModes        `yaml:"modes"`
    Email     EmailConfig      `yaml:"email"`
    SSO       SSOConfig        `yaml:"sso"`
    Anonymous AnonymousConfig  `yaml:"anonymous"`  // NEW (Q9 escape hatch)
}

type AnonymousConfig struct {
    AllowWithoutCaptcha bool `yaml:"allow_without_captcha"`  // default false
}

// Top-level NEW blocks:
type Config struct {
    Server    ServerConfig    `yaml:"server"`
    LLM       LLMConfig       `yaml:"llm"`
    Auth      AuthConfig      `yaml:"auth"`
    Adapters  AdaptersConfig  `yaml:"adapters"`
    Routing   RoutingConfig   `yaml:"routing"`
    License   LicenseConfig   `yaml:"license"`
    Captcha   CaptchaConfig   `yaml:"captcha"`    // NEW
    RateLimit RateLimitConfig `yaml:"ratelimit"`  // NEW
}

type CaptchaConfig struct {
    Enabled      bool     `yaml:"enabled"`        // default false
    Provider     string   `yaml:"provider"`       // "turnstile" | "hcaptcha"
    SiteKey      string   `yaml:"site_key"`       // public; safe to commit
    SecretKeyEnv string   `yaml:"secret_key_env"` // env var name; ResolveSecret
    RequiredFor  []string `yaml:"required_for"`   // default ["anonymous"] when YAML key omitted; explicit [] honored
}

type RateLimitConfig struct {
    PerIP          PerIPConfig          `yaml:"per_ip"`
    PerSession     PerSessionConfig     `yaml:"per_session"`
    DailyLLMBudget DailyLLMBudgetConfig `yaml:"daily_llm_budget"`
}

type PerIPConfig struct {
    RequestsPerSecond float64 `yaml:"requests_per_second"` // default 1.0
    Burst             int     `yaml:"burst"`                // default 5
    IdleTTL           string  `yaml:"idle_ttl"`             // default "15m"
}

type PerSessionConfig struct {
    MaxTurns       int    `yaml:"max_turns"`         // default 20
    MaxInputTokens int    `yaml:"max_input_tokens"`  // default 8000
    SessionTTL     string `yaml:"session_ttl"`       // default "1h"
}

type DailyLLMBudgetConfig struct {
    MaxInputTokens   int    `yaml:"max_input_tokens"`    // default 5_000_000
    MaxOutputTokens  int    `yaml:"max_output_tokens"`   // default 1_000_000
    ActionOnExceeded string `yaml:"action_on_exceeded"`  // default "reject"; any other value → fatal at startup
}
```

Defaults applied in `config.applyDefaults` (5-i). Secrets resolve via `config.ResolveSecret` / `config.RequireSecret` at startup in `main.go` — never read directly from YAML, never logged.

### 8.3 Per-IP limiter (FROZEN in 5-ii — `relay/internal/ratelimit/perip/`)

```go
package perip

import (
    "sync"
    "time"
    "golang.org/x/time/rate"
)

// Limiter holds a per-client-IP token bucket. Buckets are eagerly GC'd when
// a key's last-seen exceeds the configured idle TTL.
type Limiter struct{ /* unexported */ }

// New constructs a Limiter. reqsPerSecond is the steady-state rate; burst is
// the bucket capacity. idleTTL is how long an unused key is retained before GC.
// now is injectable for tests (production: time.Now).
func New(reqsPerSecond float64, burst int, idleTTL time.Duration, now func() time.Time) *Limiter

// Allow reports whether ip may proceed now. On reject, retryAfter is the bucket
// refill interval (1/reqsPerSecond) rounded UP to seconds, floor 1.
// Both arguments must be non-empty (the caller guarantees this via clientIPMiddleware).
func (l *Limiter) Allow(ip string) (ok bool, retryAfter time.Duration)
```

### 8.4 Budget tracker (FROZEN in 5-ii — `relay/internal/budget/`)

```go
package budget

import (
    "sync"
    "time"
)

// Tracker holds daily input/output token counters keyed by tenantKey.
// Counters reset at 00:00 UTC.
type Tracker struct{ /* unexported */ }

// New constructs a Tracker. maxIn/maxOut may be 0 (= unlimited; Reserve always
// returns ok=true; the tracker still records totals for metrics).
// now is injectable for tests (production: time.Now).
func New(maxInputTokens, maxOutputTokens int, now func() time.Time) *Tracker

// Reserve checks the budget BEFORE a /turn LLM call.
// estIn / estOut are conservative caller estimates.
// On reject, retryAfter = (next-00:00-UTC - now) rounded UP to seconds.
func (t *Tracker) Reserve(tenantKey string, estIn, estOut int) (ok bool, retryAfter time.Duration)

// Commit records the actual usage AFTER SSEDone fires. Never rejects.
func (t *Tracker) Commit(tenantKey string, actualIn, actualOut int)

// Snapshot returns the current counters for tenantKey for metrics export.
func (t *Tracker) Snapshot(tenantKey string) (inputTokens, outputTokens int, dayStartUTC time.Time)
```

### 8.5 CAPTCHA verifier (FROZEN in 5-iii — `relay/internal/captcha/`)

```go
package captcha

import (
    "context"
    "net/http"
    "time"
)

// Verifier verifies a CAPTCHA token via the provider's siteverify endpoint.
// Implementations MUST scrub the secret from any returned error (L005).
type Verifier interface {
    // Verify returns (ok=true, "", nil) on a valid, single-use token.
    // remoteIP is the resolved client IP (passed through to siteverify per
    // Cloudflare/hCaptcha docs).
    // On (ok=false, reason, nil): reason carries the provider's error-codes[0]
    // — never the secret. err is reserved for transport/parse failures
    // (returned as 502 captcha_unavailable by the handler).
    Verify(ctx context.Context, token, remoteIP string) (ok bool, reason string, err error)

    // Provider returns "turnstile" or "hcaptcha" for logging.
    Provider() string
}

// New constructs the configured verifier. provider is "turnstile" or "hcaptcha".
// secret is the resolved value (caller already ran config.ResolveSecret).
// httpClient defaults to one with a 5s timeout when nil.
// now is injectable for tests (production: time.Now).
func New(provider, secret string, httpClient *http.Client, now func() time.Time) (Verifier, error)

// Stub always returns ok=true. Used when captcha is disabled or when the
// current auth mode is not in required_for.
type Stub struct{}
func (Stub) Verify(context.Context, string, string) (bool, string, error) { return true, "", nil }
func (Stub) Provider() string                                              { return "stub" }
```

### 8.6 Auth Store extension (FROZEN in 5-ii — `relay/internal/auth/store.go`)

```go
// NewStoreWithCaps is the Phase 5 constructor.
// maxTurns / maxInputTokens / sessionTTL all 0 → no cap / no TTL
// (Phase 1+4 backward-compat for tests).
// now is injectable for tests (production: time.Now).
func NewStoreWithCaps(maxTurns, maxInputTokens int, sessionTTL time.Duration, now func() time.Time) *Store

// NewStore preserved as wrapper:
// return NewStoreWithCaps(0, 0, 0, time.Now)
func NewStore() *Store

// CheckSession reports whether the session may take another turn.
// On reject: code ∈ {"session_turns_exhausted","session_tokens_exhausted","session_expired"};
// retryAfter is non-zero only for the first two (session_expired is terminal).
func (s *Store) CheckSession(id string) (ok bool, retryAfter time.Duration, code string)

// RecordTurn increments turns by 1 and adds inputTokens to cumInputTokens
// for session id. No-op if id is unknown.
func (s *Store) RecordTurn(id string, inputTokens int)
```

### 8.7 Auth Middleware extension (FROZEN in 5-i — `relay/internal/auth/middleware.go`)

```go
// NewMiddlewareWithModes is the Phase 5 constructor.
// modesAnonymous=false → the anonymous fall-through branch returns 401 even
// when a valid X-Intake-Session is presented (Q9 strict enforcement).
func NewMiddlewareWithModes(store *Store, email EmailJWTVerifier, sso SSOVerifier, modesAnonymous bool) *Middleware

// NewMiddleware preserved as Phase 1/4 wrapper:
// return NewMiddlewareWithModes(store, email, sso, true)
func NewMiddleware(store *Store, email EmailJWTVerifier, sso SSOVerifier) *Middleware

// Handler signature UNCHANGED. The anonymous fall-through branch gains a
// single guard:
//   if !m.modesAnonymous { authWriteJSON(w, 401, ...); return }
// before the existing X-Intake-Session check.
```

### 8.8 Client IP middleware (FROZEN in 5-i — `relay/internal/server/clientip.go`)

```go
// Package server (lives alongside server.go, not a new sub-package).

import "net/netip"

// clientIPMiddleware resolves the request's client IP per the trusted-proxies
// allowlist and stashes it in r.Context() under clientIPCtxKey{}.
//
// Resolution:
//   - If RemoteAddr is in any CIDR of trustedProxies, walk X-Forwarded-For
//     right-to-left and take the first hop NOT in trustedProxies. If every
//     hop is trusted, use the leftmost hop.
//   - Otherwise (or if trustedProxies is empty), use RemoteAddr verbatim.
//
// The stashed value is the IP only (no port). If RemoteAddr cannot be parsed,
// the empty string is stashed and the per-IP limiter treats all such requests
// as a single bucket (safe degraded behavior).
func clientIPMiddleware(trustedProxies []netip.Prefix) func(http.Handler) http.Handler

// ClientIPFromContext returns the IP stashed by clientIPMiddleware.
// Returns "" if not set.
func ClientIPFromContext(ctx context.Context) string
```

### 8.9 Endpoint contracts (FROZEN in 5-i for shape, fully wired in 5-iii)

```
POST /v1/intake/init
  Auth:    NONE
  Body:    {} | {"captcha_token": "<provider-token>"}
  200:     InitResponse{session_id, capabilities, auth?, captcha?}
  400:     {"error":{"code":"captcha_required","message":"call /init again with a solved captcha_token"},
            "capabilities":{...},"captcha":{"provider":"turnstile","site_key":"0x4AAA..."}}
            (only when captcha.enabled=true, "anonymous" ∈ captcha.required_for, body omits captcha_token;
             discovery fields carried so the widget can render the challenge without a separate call)
  401:     {"error":{"code":"captcha_failed","message":"captcha verification failed","reason":"<provider-error-code>"}}
            (reason from provider's error-codes[0]; never the secret)
  502:     {"error":{"code":"captcha_unavailable","message":"captcha verification provider unavailable"}}
  429:     {"error":{"code":"rate_limited","message":"too many requests; slow down"}}
            Headers: Retry-After: 1   (per-IP burst exceeded)

POST /v1/intake/turn (Phase 4 auth dispatcher; Phase 5 adds:)
  429:     {"error":{"code":"session_turns_exhausted","message":"session turn limit reached"}}
            Headers: Retry-After: <ttl-remaining-secs>
  429:     {"error":{"code":"session_tokens_exhausted","message":"session input-token limit reached"}}
            Headers: Retry-After: <ttl-remaining-secs>
  401:     {"error":{"code":"session_expired","message":"session expired; call POST /v1/intake/init again"}}
            (no Retry-After)
  503:     {"error":{"code":"daily_budget_exhausted","message":"relay daily LLM budget reached"}}
            Headers: Retry-After: <secs-to-next-utc-midnight>
  429:     {"error":{"code":"rate_limited", ...}}  (per-IP — same shape as /init)
```

`Retry-After` is always integer seconds (RFC 9110 §10.2.3). Never `0`; floor at `1`.

### 8.10 InitResponse extension (FROZEN in 5-i — `relay/internal/server/dto.go`)

```go
type Capabilities struct {
    AuthModes       []string `json:"auth_modes"`
    Streaming       bool     `json:"streaming"`
    RequiresCaptcha []string `json:"requires_captcha,omitempty"` // 5-i NEW; subset of AuthModes
}

type InitCaptcha struct {
    Provider string `json:"provider"`   // "turnstile" | "hcaptcha"
    SiteKey  string `json:"site_key"`
}

type InitResponse struct {
    SessionID    string        `json:"session_id"`
    Capabilities Capabilities  `json:"capabilities"`
    Auth         *InitAuth     `json:"auth,omitempty"`
    Captcha      *InitCaptcha  `json:"captcha,omitempty"`           // 5-i NEW; nil when disabled
}

// InitRequest is NEW in 5-i — empty in v0 except for the captcha token.
type InitRequest struct {
    CaptchaToken string `json:"captcha_token,omitempty"`
}

// CaptchaRequiredResponse is the 400 body shape — same envelope as the success
// path so the widget reuses one decoder for both cases.
type CaptchaRequiredResponse struct {
    Error        ErrorBody     `json:"error"`
    Capabilities Capabilities  `json:"capabilities"`
    Captcha      *InitCaptcha  `json:"captcha,omitempty"`
}
```

### 8.11 Deps extension (FROZEN in 5-i — `relay/internal/server/deps.go`)

```go
import (
    "net/netip"
    "intake/internal/budget"
    "intake/internal/captcha"
    "intake/internal/ratelimit/perip"
)

type Deps struct {
    // ... all existing Phase 1-4 fields unchanged ...

    // 5-i NEW:

    // CaptchaCfg is the captcha section of the loaded config; needed by initHandler.
    CaptchaCfg config.CaptchaConfig

    // CaptchaVerifier is the verifier instance. nil → "no captcha required"
    // (initHandler treats nil + cfg.Enabled=false the same way; tests can pass nil).
    CaptchaVerifier captcha.Verifier

    // Budget tracks the daily LLM spend. nil → no budget gate (unit tests of /init).
    Budget *budget.Tracker

    // PerIP is the per-IP rate limiter. nil → no per-IP gate (unit tests).
    PerIP *perip.Limiter

    // TrustedProxies is the parsed CIDR list (parsed once at startup in main.go).
    TrustedProxies []netip.Prefix
}
```

## 9. Notes

- Module path remains `intake`. Go 1.23.2.
- Per-session, per-IP, and daily-budget counters are in-memory only. Restart drops all three — acceptable for v0 (PROJECT.md §2 v0 goal #9 "stateless operation").
- The daily-budget tracker is in-memory only. Restart drops the day's counters; an operator restarting at 23:00 effectively grants the tenant a fresh budget for the last hour of the day. Documented in 5-iv's notes; v1+ may introduce a persistent counter.
- L010 (PS 5.1 BOM) applies to any new smoke YAML written via `Set-Content` — use `-Encoding ascii`.
- L014 (injectable-clock for in-memory TTL/window primitives) applies to **every** new in-memory store in Phase 5: `perip.Limiter`, `budget.Tracker`, the extended `auth.Store`. All three accept `now func() time.Time` at construction.
- L015 (derived-field test gaps) — every new dispatcher test added in 5-i MUST use the existing `requireFullSessionContext` helper from Phase 4. The Phase 5 dispatcher does NOT touch SessionID population — but the Q9 strict-anonymous branch is a new return path; the helper proves no Phase 4 field regressed.
- L005 (redact-before-truncate) applies to CAPTCHA siteverify response bodies — wrapped via a `redact(secret, body)` helper in the `captcha` package BEFORE any log line or wrapped error includes the body content.
