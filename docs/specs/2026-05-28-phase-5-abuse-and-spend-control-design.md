# Phase 5 — Abuse & Spend Control: Design

> **Status:** Approved design (brainstorming output), pre-planning
> **Date:** 2026-05-28
> **Implements:** [docs/PROJECT.md](../PROJECT.md) §10 (rate limiting and abuse protection), §17 (security considerations), §19 Q9 (anonymous without CAPTCHA)
> **Parent:** [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 5 row + §4 Q9 resolution)
> **Builds on:** Phase 1's frozen `auth.Middleware`/`SessionContext`/`Deps`/chi-route shape, Phase 4's `auth.Middleware.Handler` signature and `config.SSOConfig` mutual-exclusivity, the Phase-1+3+4 secrets seam (`config.ResolveSecret`).

## 1. Goal

Add the four guardrails from PROJECT.md §10 behind the frozen Phase 1/4 seams: **per-IP token bucket**, **per-session turn/token caps**, **daily LLM spend cap**, and **CAPTCHA** at `/v1/intake/init`. Tighten the Phase-1 anonymous-fall-through with a Q9-strict startup gate that **fail-closes** if `auth.modes.anonymous=true` and CAPTCHA is disabled, unless `auth.anonymous.allow_without_captcha: true` is explicit. Consolidate Phase 4's two SSO mutual-exclusivity checks into the same startup gate so misconfig surfaces as a single error log line, not three sequential exits.

Success (decomposition §2, Phase 5 row): "Exceed burst → 429; exceed daily budget → 503 + `Retry-After` (next UTC midnight); anonymous requires CAPTCHA; cross-origin request blocked."

The `auth.Middleware` from Phase 1+4 stays the **only** identity-resolution path; Phase 5 wraps the chi `/v1/intake` group with two additional middlewares (clientIP resolver + per-IP limiter) and adds per-session + budget checks **inside** the existing `/turn` handler. CAPTCHA verification is added **inside** the existing `/init` handler. No new endpoints. No widget-side protocol changes beyond a new optional `captcha_token` field in `/init`'s body and a new optional `captcha` capability in its response.

## 2. Seams this phase introduces

### 2.1 Middleware chain (additive on `/v1/intake` group)

```
chi.NewMux                            ← server.New (unchanged)
  middleware.RequestID                ← unchanged
  middleware.Recoverer                ← unchanged
  corsMiddleware (Phase 1)            ← unchanged
  ├─ GET /v1/health, /v1/version      ← unchanged (NOT rate-limited)
  └─ Route /v1/intake
       clientIPMiddleware  ★NEW (5-i)
       perIPLimitMiddleware ★NEW (5-i seam stub, 5-ii implementation)
       ├─ POST /init                  ← 5-iii adds: captcha verify when required
       ├─ POST /auth/email/start      ← unchanged (own rate-limit lives in emailcode)
       ├─ POST /auth/email/verify     ← unchanged
       └─ Group(auth.Handler)         ← Phase 4 dispatcher (5-i hardens internally)
            ├─ POST /turn             ← 5-ii adds: session check + budget Reserve/Commit
            └─ POST /submit           ← unchanged
```

Ordering rationale:

- `clientIPMiddleware` runs first because the per-IP limiter reads the resolved IP from the request context.
- `perIPLimitMiddleware` runs **before** `auth.Handler` so unauthenticated abusers can't exhaust the auth-store with bursts.
- Per-session and budget gates run **inside** `/turn` (not as middleware) because they need `auth.SessionContext.SessionID` (set by the auth dispatcher) and `X-Intake-Tenant` (request-scoped, optional).
- `/v1/health` and `/v1/version` are outside `/v1/intake` and are deliberately NOT rate-limited (liveness probes from load balancers / Kubernetes must always succeed).

### 2.2 Five new sub-packages

All under `relay/internal/`:

- **`ratelimit/perip`** — `*Limiter` wrapping `golang.org/x/time/rate.Limiter` per key. `Allow(ip) (ok bool, retryAfter time.Duration)`. Eager GC of buckets by last-seen idle TTL (default 15min). Injectable clock.
- **`budget`** — `*Tracker` holding daily input/output token counters keyed by tenantKey. `Reserve(tenantKey, estIn, estOut) (ok, retryAfter)`; `Commit(tenantKey, actualIn, actualOut)`. UTC-day boundary triggers reset on first access of the new day. Injectable clock.
- **`captcha`** — `Verifier` interface; `Turnstile` + `HCaptcha` implementations; `Stub` for disabled/test use. Each implementation owns a tiny replay-protection set (`map[challengeID]time.Time`, evicted after 5min). Siteverify secret never appears in errors (L005).
- **`server/clientip.go`** — small file (not a new package; lives in `internal/server`) — `clientIPMiddleware` + `ClientIPFromContext` helper. Resolves the client IP from `RemoteAddr` or the rightmost-untrusted XFF hop per `server.trusted_proxies` CIDR allowlist.
- **`auth/store` extension** — additive to the existing Phase-1 store: new `sessionMeta` map, new `NewStoreWithCaps` constructor that takes `maxTurns`, `maxInputTokens`, `sessionTTL`, and `now func() time.Time`. The Phase-1 `NewStore()` constructor is preserved as a wrapper that delegates with effectively-unlimited caps (zeros) for backward compatibility with existing tests.

### 2.3 Q9 strict-anonymous + consolidated startup gate

In `relay/cmd/relay/main.go`, after `config.Load` and all secret resolution but **before** `auth.NewMiddlewareWithModes`, a single check accumulates **all** Phase 4 + Phase 5 config errors into one `[]string` and exits with a single structured log line if non-empty:

```
problems := []string{}
anonymousProtected := cfg.Captcha.Enabled && containsString(cfg.Captcha.RequiredFor, "anonymous")
if cfg.Auth.Modes.Anonymous && !anonymousProtected && !cfg.Auth.Anonymous.AllowWithoutCaptcha {
    problems = append(problems, "auth.modes.anonymous=true requires captcha.enabled=true AND captcha.required_for to include \"anonymous\"; or set auth.anonymous.allow_without_captcha=true to acknowledge the risk (PROJECT.md §19 Q9)")
}
if cfg.Auth.Modes.SSO {
    jwks := cfg.Auth.SSO.JWKSURL != ""
    hs   := cfg.Auth.SSO.HS256SecretEnv != ""
    if jwks && hs {
        problems = append(problems, "auth.modes.sso=true: both jwks_url and hs256_secret_env are set; exactly one required")
    }
    if !jwks && !hs {
        problems = append(problems, "auth.modes.sso=true: neither jwks_url nor hs256_secret_env is set; exactly one required")
    }
}
if len(problems) > 0 {
    logger.Error("relay: startup config errors", "count", len(problems), "problems", problems)
    os.Exit(1)
}
```

This subsumes the Phase 4 sso-both/sso-neither checks that currently live inside `sso.New` — the factory still rejects (defense in depth), but operators see a single consolidated message instead of fixing the same restart cycle one error at a time.

The `auth.Middleware` itself gains a single new field, `modesAnonymous bool`, plumbed in via a new constructor `NewMiddlewareWithModes(store, email, sso, modesAnonymous)`. The existing `NewMiddleware(store, email, sso)` is kept as a thin wrapper that defaults `modesAnonymous=true` so all Phase 1/4 unit tests stay green. Inside `Handler`, the anonymous fall-through branch gains a single `if !m.modesAnonymous { 401 }` check before the existing X-Intake-Session validation — strict Q9 enforcement at the dispatcher level, not just advertisement.

### 2.4 Init capabilities extension (additive — `relay/internal/server/dto.go` + `turn.go`)

The Phase 1 `InitResponse.Capabilities.AuthModes` and the Phase 4 `InitResponse.Auth.Email.CodeTTLSeconds` stay unchanged. Two additive fields are introduced so the widget can render the CAPTCHA challenge **before** calling `/init`:

```go
type Capabilities struct {
    AuthModes       []string `json:"auth_modes"`
    Streaming       bool     `json:"streaming"`
    // 5-iii NEW:
    RequiresCaptcha []string `json:"requires_captcha,omitempty"` // subset of AuthModes
}

type InitCaptcha struct {
    Provider string `json:"provider"`           // "turnstile" | "hcaptcha"
    SiteKey  string `json:"site_key"`           // safe-to-publish public key
}

type InitResponse struct {
    SessionID    string        `json:"session_id"`
    Capabilities Capabilities  `json:"capabilities"`
    Auth         *InitAuth     `json:"auth,omitempty"`
    Captcha      *InitCaptcha  `json:"captcha,omitempty"`           // 5-iii NEW; nil when disabled
}
```

The widget reads `capabilities.requires_captcha` to decide whether to render a Turnstile/hCaptcha challenge **on the first turn**. To verify the challenge against the just-issued session, `/init` is split into a two-call dance only if CAPTCHA is required: the widget calls `/init` once with no body (or `captcha_token` omitted) to discover `captcha`; if `requires_captcha` is non-empty AND the user is on an unverified path, it solves the challenge, then re-calls `/init` with `{captcha_token: "..."}` to actually mint the session. The first (discovery) call returns `400 captcha_required` rather than minting an unrestricted session — same body shape as the existing `bad_request` error but with `code: "captcha_required"` so the widget can branch cleanly.

> Note: this is the minimal widget-side change. A future v1 simplification could move the discovery to a separate `GET /v1/intake/capabilities` endpoint to avoid the two-call dance, but that's out of scope for v0. Documented as a v1 follow-up.

### 2.5 Endpoint contracts (FROZEN in 5-i for shape, fully wired in 5-iii)

```
POST /v1/intake/init
  Auth:    NONE (unauth — issues a session)
  Body:    {} | {"captcha_token": "<provider-token>"}
  200:     InitResponse (see §2.4)
  400:     {"error":{"code":"captcha_required","message":"call /init again with a solved captcha_token"},
            "capabilities":{...},"captcha":{"provider":"turnstile","site_key":"0x4AAA..."}}
            Only returned when (a) captcha.enabled=true, (b) "anonymous" ∈ captcha.required_for, and (c) the body omits captcha_token. The 400 body carries the same `capabilities` + `captcha` fields as the success path so the widget can render the challenge without a separate discovery call. The widget MUST handle this by re-calling /init with a solved token; it must NOT treat the absence of a session_id as a fatal error.
  401:     {"error":{"code":"captcha_failed","message":"captcha verification failed","reason":"<provider-error-code>"}}
            The `reason` field carries the provider's `error-codes[0]` (Turnstile) or `error-codes[0]` (hCaptcha) — never the secret. Examples: "invalid-input-response", "timeout-or-duplicate".
  502:     {"error":{"code":"captcha_unavailable","message":"captcha verification provider unavailable"}}
            siteverify returned non-2xx, network error, or response body unparseable. The actual error text is logged at Debug level only.
  429:     {"error":{"code":"rate_limited","message":"too many requests; slow down"}}
            Headers: Retry-After: 1   (per-IP burst exceeded)
```

```
POST /v1/intake/turn
  Auth:    bearer JWT (email/sso) or X-Intake-Session (anonymous)
  200:     SSE stream (unchanged)
  429:     {"error":{"code":"session_turns_exhausted","message":"session turn limit reached"}}
            Headers: Retry-After: <ttl-remaining-secs>  (rounded up; floor 1)
  429:     {"error":{"code":"session_tokens_exhausted","message":"session input-token limit reached"}}
            Headers: Retry-After: <ttl-remaining-secs>
  401:     {"error":{"code":"session_expired","message":"session expired; call POST /v1/intake/init again"}}
            (no Retry-After — caller must mint a new session)
  503:     {"error":{"code":"daily_budget_exhausted","message":"relay daily LLM budget reached"}}
            Headers: Retry-After: <secs-to-next-utc-midnight>
  429:     {"error":{"code":"rate_limited", ...}}  (per-IP — same shape as /init)
```

`Retry-After` is always seconds in numeric form (RFC 9110 §10.2.3). Never `0`; floor at `1`.

## 3. Component shapes (FROZEN in their respective sub-plans)

### 3.1 `ratelimit/perip` (FROZEN in 5-ii)

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

// Allow reports whether ip may proceed now.
// On reject, retryAfter = (1/reqsPerSecond) rounded UP to seconds, floor 1.
// Both arguments must be non-empty (the caller guarantees this via clientIPMiddleware).
func (l *Limiter) Allow(ip string) (ok bool, retryAfter time.Duration)
```

Behavior:

- Internally `map[string]*entry` where `entry = {bucket *rate.Limiter, lastSeen time.Time}`.
- Each `Allow` call updates `lastSeen` to `now()`. Buckets older than `idleTTL` are evicted by a single inline pass at the start of every `Allow` (eager GC, matches the `emailcode` pattern — no background goroutine).
- Thread-safe via a single `sync.Mutex`. Per-key contention is negligible for the expected scale.

### 3.2 `budget` (FROZEN in 5-ii)

```go
package budget

import (
    "sync"
    "time"
)

// Tracker holds daily input/output token counters keyed by tenantKey.
// Counters reset at 00:00 UTC.
type Tracker struct{ /* unexported */ }

// New constructs a Tracker. maxIn/maxOut may be 0 (= unlimited; the tracker
// still records totals for metrics but Reserve always returns ok=true).
// now is injectable for tests (production: time.Now).
func New(maxInputTokens, maxOutputTokens int, now func() time.Time) *Tracker

// Reserve checks the budget BEFORE a /turn LLM call.
// estInputTokens / estOutputTokens are conservative caller estimates.
// On reject, retryAfter = (next-00:00-UTC - now) rounded UP to seconds.
func (t *Tracker) Reserve(tenantKey string, estInputTokens, estOutputTokens int) (ok bool, retryAfter time.Duration)

// Commit records the actual usage AFTER SSEDone fires. Always increments;
// never rejects (the Reserve gate already authorized the work).
func (t *Tracker) Commit(tenantKey string, actualInputTokens, actualOutputTokens int)

// Snapshot returns the current counters for tenantKey for metrics export.
func (t *Tracker) Snapshot(tenantKey string) (inputTokens, outputTokens int, dayStartUTC time.Time)
```

Behavior:

- Internally `map[string]*dailyCounters` where `dailyCounters = {in, out int, dayStartUTC time.Time}`.
- On every `Reserve` or `Commit`, the tracker checks `now().UTC().Truncate(24h)` against `dayStartUTC`; if newer, counters are reset to 0 and `dayStartUTC` is advanced. This is per-tenant — different tenants roll over independently.
- Reserve does **not** mutate counters; Commit does. So an aborted /turn (client disconnect mid-stream) never inflates the counter.
- Estimates are not "held" — Reserve is purely a yes/no gate against current totals + estimate. This means two concurrent Reserves can both pass when one of them would push over the cap. Acceptable for v0: the cap is a soft daily budget, not a hard transactional limit; the race window is bounded by the LLM call duration (~seconds).

### 3.3 `captcha` (FROZEN in 5-iii)

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
    // Verify returns (ok=true, "") on a valid, single-use token.
    // remoteIP is the resolved client IP (passed through to siteverify per
    // Cloudflare/hCaptcha docs).
    // On (ok=false, reason!=""), reason carries the provider's error-codes[0] —
    // never the secret.
    Verify(ctx context.Context, token, remoteIP string) (ok bool, reason string, err error)

    // Provider returns "turnstile" or "hcaptcha" for logging.
    Provider() string
}

// New constructs the configured verifier. provider is "turnstile" or "hcaptcha".
// secret is the resolved value (caller already ran config.ResolveSecret).
// httpClient defaults to one with a 5s timeout when nil.
// now is injectable for tests (production: time.Now).
func New(provider, secret string, httpClient *http.Client, now func() time.Time) (Verifier, error)

// Stub always returns ok=true. Used by handlers when captcha is disabled OR
// when the auth mode is not in required_for.
type Stub struct{}
func (Stub) Verify(context.Context, string, string) (bool, string, error) { return true, "", nil }
func (Stub) Provider() string                                              { return "stub" }
```

Behavior:

- Turnstile siteverify: `POST https://challenges.cloudflare.com/turnstile/v0/siteverify` with form-encoded `secret`, `response`, `remoteip`. Response: `{"success": bool, "error-codes": [...], "challenge_ts": "...", "hostname": "...", ...}`.
- hCaptcha siteverify: `POST https://hcaptcha.com/siteverify` with the same form-encoded shape. Response: `{"success": bool, "error-codes": [...], "challenge_ts": "...", "hostname": "...", ...}`.
- Both providers' tokens are valid for ~5 minutes — replay protection lives inside the Verifier as a small `sync.Mutex`-guarded `map[token]time.Time` set; entries evicted on every Verify call where `now() - entry > 5*time.Minute`. A token seen twice within 5 minutes is rejected with `reason="duplicate"` even if siteverify would still accept it (defense in depth — provider may not enforce single-use under all conditions).
- All non-2xx responses from siteverify, any JSON-parse failure, any network error → wrapped error returned. The error text is `"captcha siteverify: <kind>"` — no body content, no secret. Body is logged at `Debug` level after `redact(secret, body)`.

### 3.4 `auth/store` extension (FROZEN in 5-ii)

```go
package auth

// sessionMeta holds per-session counters and TTL state.
// Phase 5 additive — Phase 1 anonymous sessions get a default-populated entry.
type sessionMeta struct {
    createdAt      time.Time
    turns          int
    cumInputTokens int
}

// Store gains two private fields:
//   sessionMeta    map[string]*sessionMeta
//   maxTurns       int
//   maxInputTokens int
//   sessionTTL     time.Duration
//   now            func() time.Time

// NewStoreWithCaps is the Phase 5 constructor. maxTurns and maxInputTokens of 0
// mean "no cap" (Phase 1 backward-compat for tests that don't care about Phase 5).
// sessionTTL of 0 means "no TTL" (sessions never expire — Phase 1 default).
func NewStoreWithCaps(maxTurns, maxInputTokens int, sessionTTL time.Duration, now func() time.Time) *Store

// NewStore is preserved as a wrapper that delegates with caps=0, TTL=0, now=time.Now.
// Phase 1+4 callers see no behavior change.
func NewStore() *Store { return NewStoreWithCaps(0, 0, 0, time.Now) }

// CheckSession reports whether the session may take another turn.
// On reject, code is one of "session_turns_exhausted" | "session_tokens_exhausted"
// | "session_expired". retryAfter is non-zero only for the first two
// (session_expired is terminal — caller must re-init).
func (s *Store) CheckSession(id string) (ok bool, retryAfter time.Duration, code string)

// RecordTurn increments turns by 1 and adds inputTokens to cumInputTokens
// for session id. No-op if id is unknown (session_expired during the call).
func (s *Store) RecordTurn(id string, inputTokens int)
```

Behavior:

- Issue() additionally creates a `sessionMeta{createdAt: now(), turns: 0, cumInputTokens: 0}` entry.
- Validate() additionally checks `now() - createdAt < sessionTTL` (when TTL > 0). Expired sessions are evicted on read.
- Eager-eviction inside Issue+Validate of any sessionMeta older than `sessionTTL` — matches the emailcode pattern. No background goroutine.

### 3.5 `server/clientip.go` (FROZEN in 5-i)

```go
// Package server

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
// the empty string is stashed and the per-IP limiter will treat all such
// requests as a single bucket — safe degraded behavior.
func clientIPMiddleware(trustedProxies []netip.Prefix) func(http.Handler) http.Handler

// ClientIPFromContext returns the IP stashed by clientIPMiddleware.
// Returns "" if not set.
func ClientIPFromContext(ctx context.Context) string
```

Trusted-proxy parsing happens once at startup (in `main.go`) using `netip.ParsePrefix`; invalid CIDRs are a fatal startup error joined into the same Q9 consolidated error list.

### 3.6 `auth.Middleware` extension (FROZEN in 5-i)

```go
// Phase 5 additive:
type Middleware struct {
    store           *Store
    email           EmailJWTVerifier
    sso             SSOVerifier
    modesAnonymous  bool             // NEW
}

// NewMiddlewareWithModes is the Phase 5 constructor.
// modesAnonymous=false → the anonymous fall-through branch returns 401 even
// when a valid X-Intake-Session is presented (Q9 strict enforcement).
func NewMiddlewareWithModes(store *Store, email EmailJWTVerifier, sso SSOVerifier, modesAnonymous bool) *Middleware

// NewMiddleware preserved as Phase 1/4 wrapper.
func NewMiddleware(store *Store, email EmailJWTVerifier, sso SSOVerifier) *Middleware {
    return NewMiddlewareWithModes(store, email, sso, true)  // Phase 1/4 default
}

// Handler signature UNCHANGED. The anonymous fall-through branch gains a
// single guard:
//   if !m.modesAnonymous { 401 }
// before the existing X-Intake-Session check.
```

## 4. Config additions (FROZEN in 5-i — `relay/internal/config/config.go`)

```go
// ServerConfig gains:
type ServerConfig struct {
    Addr           string   `yaml:"addr"`
    ExternalURL    string   `yaml:"external_url"`
    CORSOrigins    []string `yaml:"cors_origins"`
    TrustedProxies []string `yaml:"trusted_proxies"`  // NEW; CIDR list; empty = none
}

// AuthConfig gains:
type AuthConfig struct {
    Modes     AuthModes        `yaml:"modes"`
    Email     EmailConfig      `yaml:"email"`
    SSO       SSOConfig        `yaml:"sso"`
    Anonymous AnonymousConfig  `yaml:"anonymous"`  // NEW
}

type AnonymousConfig struct {
    AllowWithoutCaptcha bool `yaml:"allow_without_captcha"`  // Q9 escape hatch
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
    RequiredFor  []string `yaml:"required_for"`   // default ["anonymous"]
}

type RateLimitConfig struct {
    PerIP           PerIPConfig           `yaml:"per_ip"`
    PerSession      PerSessionConfig      `yaml:"per_session"`
    DailyLLMBudget  DailyLLMBudgetConfig  `yaml:"daily_llm_budget"`
}

type PerIPConfig struct {
    RequestsPerSecond float64 `yaml:"requests_per_second"` // default 1
    Burst             int     `yaml:"burst"`                // default 5
    IdleTTL           string  `yaml:"idle_ttl"`             // default "15m"
}

type PerSessionConfig struct {
    MaxTurns       int    `yaml:"max_turns"`         // default 20
    MaxInputTokens int    `yaml:"max_input_tokens"`  // default 8000
    SessionTTL     string `yaml:"session_ttl"`       // default "1h"
}

type DailyLLMBudgetConfig struct {
    MaxInputTokens  int `yaml:"max_input_tokens"`   // default 5_000_000
    MaxOutputTokens int `yaml:"max_output_tokens"`  // default 1_000_000
    // ActionOnExceeded is "reject" in v0. "queue" is documented as v1+ only.
    // The field is parsed for forward-compat; unknown values are a fatal startup error.
    ActionOnExceeded string `yaml:"action_on_exceeded"` // default "reject"
}
```

`applyDefaults` populates each missing value. `Captcha.RequiredFor` defaults to `["anonymous"]` only when the YAML omits the key entirely; an explicit empty list `required_for: []` is honored (operator opted out of CAPTCHA for all modes — equivalent to `enabled: false`). `DailyLLMBudget.ActionOnExceeded != "reject"` is a fatal startup error (Phase 5 only ships "reject"; documenting "queue" without implementing it is the silent-failure trap PHASE_PLANNING §4 forbids).

## 5. `Deps` extension (FROZEN in 5-i — `relay/internal/server/deps.go`)

```go
type Deps struct {
    // ... all existing fields unchanged ...

    // 5-i NEW:

    // CaptchaCfg is the captcha section of the loaded config; needed by initHandler
    // to populate the InitCaptcha hint and decide whether to verify on /init.
    CaptchaCfg config.CaptchaConfig

    // CaptchaVerifier is the verifier instance. nil when cfg.Captcha.Enabled=false
    // OR when the Phase 5 wiring is not yet present (e.g. unit tests of /turn).
    // initHandler treats nil as "no captcha required".
    CaptchaVerifier captcha.Verifier

    // Budget tracks the daily LLM spend. nil → no budget gate (unit tests of /init).
    // turnHandler treats nil as "no budget gate".
    Budget *budget.Tracker

    // TrustedProxies is the parsed CIDR list (parsed once at startup).
    TrustedProxies []netip.Prefix
}
```

## 6. Data flow

### 6.1 `POST /v1/intake/init` (anonymous, captcha required)

```
1. clientIPMiddleware: resolves IP, stashes in ctx.
2. perIPLimitMiddleware: Limiter.Allow(ip)
     reject → 429 {error:"rate_limited"} + Retry-After: 1
3. initHandler:
   a. If cfg.Captcha.Enabled AND "anonymous" ∈ cfg.Captcha.RequiredFor:
        decode body: {captcha_token?: string}
        if captcha_token == "":
          → 400 {error:"captcha_required"} + response body includes
            capabilities.requires_captcha + captcha.{provider,site_key}
            (no session_id minted)
        v.Verify(ctx, token, ip)
          err != nil  → 502 {error:"captcha_unavailable"}
          ok == false → 401 {error:"captcha_failed", reason: <provider-code>}
          ok == true  → continue
   b. store.Issue() → session_id
   c. response: InitResponse{session_id, capabilities, auth?, captcha?}
```

The widget MUST handle the 400 captcha_required path by rendering the challenge using `captcha.{provider,site_key}` and re-calling `/init` with the solved token. This is the only branch where /init returns 400 without a session_id.

### 6.2 `POST /v1/intake/turn` (any auth mode)

```
1. clientIPMiddleware + perIPLimitMiddleware (same as /init).
2. auth.Middleware.Handler (Phase 4 dispatcher, Q9-hardened):
   - anonymous: if !cfg.Auth.Modes.Anonymous → 401 unauthorized (NEW)
   - email/sso: bearer dispatch unchanged
   - SessionContext attached.
3. turnHandler:
   a. sess := auth.FromContext(ctx)
   b. ok, retryAfter, code := deps.Auth.Store().CheckSession(sess.SessionID)
        !ok && code == "session_expired" → 401 {error:"session_expired"}
        !ok                              → 429 {error:code} + Retry-After
   c. tenantKey := r.Header.Get("X-Intake-Tenant")  // "" if absent
   d. estIn := approxTokens(req.messages)  // simple 4-chars/token heuristic
      estOut := deps.MaxTokens
      ok, retryAfter := deps.Budget.Reserve(tenantKey, estIn, estOut)
        !ok → 503 {error:"daily_budget_exhausted"} + Retry-After
   e. provider.Chat(...) streams; on SSEDone:
        deps.Budget.Commit(tenantKey, chunk.InputTokens, chunk.OutputTokens)
        deps.Auth.Store().RecordTurn(sess.SessionID, chunk.InputTokens)
```

Reserve-then-Commit semantics: Reserve uses estimates so the gate trips **before** an LLM credit is spent; Commit replaces the reservation with the actuals from `SSEDone.{InputTokens, OutputTokens}` so counters reflect real spend. A failed-mid-stream turn (provider error or client disconnect) results in no Commit and no RecordTurn — the turn does not count.

### 6.3 Startup — Q9 consolidated gate

Pseudocode in §2.3. The check runs in `main.go` after `config.Load` returns and after `config.ResolveSecret` calls for any auth secrets, but **before** `auth.NewMiddlewareWithModes`, `captcha.New`, `budget.New`, or `perip.New`. Trusted-proxy CIDR parse errors join the same `problems` slice.

## 7. Testing strategy

### 7.1 Credit-free unit layer

| Package | Coverage |
|---|---|
| `ratelimit/perip` | injected clock; burst ok→reject→1s-recharge→ok; idle TTL eviction shrinks map; empty-key behavior; concurrent Allow safe under `-race` |
| `budget` | injected clock; Reserve under cap → ok with no mutation; Reserve at cap → reject with correct seconds-to-midnight; Commit increments; UTC midnight crossover resets; tenants isolated; maxIn=0 → unlimited |
| `captcha` | `httptest.Server` mocks Turnstile + hCaptcha; ok-token accepted; replay rejected ("duplicate"); siteverify non-2xx → captcha_unavailable error; secret never appears in returned error text; 5s default httpClient timeout exercised |
| `auth/store` | clock injection; 20 turns ok / 21st rejected; cumInputTokens crosses 8000 → reject; createdAt+1h → session_expired; eager prune verified via `len(sessionMeta)` |
| `auth/middleware` | NEW dispatcher tests pin Q9 strict-anonymous: modes.anonymous=false + valid X-Intake-Session → 401; Phase 1/4 default (true) regression-pinned via `TestDispatcher_AnonymousFallthrough_Preserved` (already exists) |
| `server/clientip` | matrix: empty TrustedProxies → RemoteAddr verbatim; one CIDR + XFF chain → rightmost-untrusted; spoofed XFF when RemoteAddr ∉ CIDR → ignored; malformed RemoteAddr → empty string |
| Q9 startup gate (in main_test.go or a dedicated helper) | each of 4 misconfigs (anon-no-captcha, sso-both, sso-neither, bad-CIDR) individually triggers exit 1 with the right `problems` entry; all-at-once triggers a single Error log line listing all 4 |

**Cross-cutting fixture (L015):** the existing `requireFullSessionContext(t, sess, ...)` helper from Phase 4 is reused by every dispatcher test added in 5-i to confirm SessionID + new modesAnonymous behavior do not leave a derived field unset.

### 7.2 Live smokes (in 5-iv)

1. **Q9 startup smoke (self-runnable, no LLM credit):** spin the relay binary with each of the 4 misconfigs (env-controlled YAML); assert `exit code 1` and the consolidated error log line is present.
2. **Per-IP rate-limit smoke (self-runnable, no LLM credit):** scripted curl burst of 10 requests in 1s against `/v1/health` (NOT rate-limited — controls for the chain) + same burst against `/v1/intake/init` → assert 5×200 + 5×429 with `Retry-After: 1`.
3. **Per-session cap smoke (uses local Ollama stub OR Phase 1 fake provider — no credit):** drive 20 /turn calls; 21st returns 429 `session_turns_exhausted`; verify `Retry-After` ≈ remaining TTL.
4. **Daily budget smoke (uses local Ollama stub OR fake provider — no credit):** configure `max_input_tokens: 100`; one /turn burns ~80; second /turn refused with 503 `daily_budget_exhausted` + `Retry-After` ≈ secs-to-midnight.
5. **CAPTCHA smoke (PAUSES for maintainer):** uses Cloudflare's documented Turnstile test sitekey `1x00000000000000000000AA` (always-passes) and the matching test secret `1x0000000000000000000000000000000AA` — confirms wiring without a real challenge UI. Documented; pauses for explicit go-ahead before running.

Smokes 2–4 hit a real provider only for the actual /turn LLM call, which uses the local Ollama-or-fake path Phase 1 already supports — zero paid-credit consumption.

## 8. Architectural Decision Records (for the phase README)

- **`golang.org/x/time/rate` as the per-IP limiter primitive.** Already an indirect dep via `keyfunc/v3 v3.8.0`. Phase 5 promotes it to a direct require. No new external dependency. Trigger to revisit: a multi-instance deployment requires a shared limiter (then: Redis + an external token-bucket impl).
- **Eager-GC + injectable-clock pattern for all in-memory stores (perip, budget, sessionMeta).** Matches L014; no background goroutines; deterministic tests. Trigger to revisit: per-op cost becomes measurable under load (then: amortize the GC pass).
- **Reserve-then-Commit budget semantics with estimates.** Gates spend on conservative estimates; reconciles with actuals on SSEDone. Tolerates a small race window for concurrent Reserves (acceptable for v0 soft cap). Trigger to revisit: hosted-relay multi-tenant billing requires transactional accuracy (then: per-tenant mutex + held reservations).
- **CAPTCHA secret never appears in any error or log line.** L005 redact-before-truncate pattern applied to siteverify response bodies. Replay protection layered inside the verifier independent of the provider's own single-use semantics (defense in depth).
- **Two-call `/init` dance when CAPTCHA is required.** Widget calls /init once to discover capabilities, solves the challenge, re-calls /init with the token. Cleanest separation; one new error code (`captcha_required`). Trigger to revisit: v1 may introduce `GET /v1/intake/capabilities` for cleaner discovery.
- **Q9 strict-anonymous + consolidated startup gate.** Dispatcher rejects anonymous when `auth.modes.anonymous=false`; startup gate emits a single consolidated error covering all of (anonymous-without-captcha, sso both-set, sso neither-set, invalid trusted-proxy CIDR). Operators fix all misconfigs in one restart cycle. Trigger to revisit: an additional auth mode or guardrail requires a new misconfig class (extend the same `problems` slice).
- **Per-IP middleware sits BEFORE auth.** Prevents unauthenticated abusers from exhausting the auth-store or the email rate-limiter with bursts. `/v1/health` and `/v1/version` stay outside `/v1/intake` and are NOT rate-limited (load-balancer liveness probes must always succeed).

## 9. Build-fail checklist (for the phase README)

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
- [ ] Phase 1+4 anonymous-default behavior regresses (`auth.modes.anonymous=true` AND `captcha.enabled=true` AND a Phase 1 client without a captcha_token hitting /init → response is anything other than 400 captcha_required). **Fail.**

## 10. Tool version pin list (for the phase README)

| Tool | Version | Reason |
|---|---|---|
| `golang.org/x/time` | promote from indirect (currently v0.9.0 via keyfunc/v3) to direct require | Per-IP token bucket via `rate.Limiter`. Already in go.sum from Phase 4; the promotion is a one-line go.mod edit, not a new download. Caret forbidden because the rate-limiter is on the abuse-control boundary. |

No other new modules. `scripts/check-pins.sh` gains a one-line check for `golang.org/x/time` matching the existing pattern.

## 11. Sub-plan decomposition

```
5-i   Config + middleware chain seam + Q9 startup gate + dispatcher hardening
        ─ new config blocks (TrustedProxies, RateLimitConfig, CaptchaConfig,
          AnonymousConfig); applyDefaults; ActionOnExceeded validation
        ─ relay/internal/server/clientip.go (clientIPMiddleware + helper)
        ─ chi /v1/intake group gains clientIP + perIPLimit middlewares (stubs)
        ─ Q9 consolidated startup gate in main.go
        ─ auth.NewMiddlewareWithModes + Q9 strict-anonymous dispatcher branch
        ─ Deps gains CaptchaCfg, CaptchaVerifier, Budget, TrustedProxies
        ─ STUB perIP/Budget/CaptchaVerifier (Stub) so the chain dispatches
        ─ scripts/check-pins.sh extension
        ─ smoke (self-runnable): Q9 startup fatal for each of the 4 misconfigs
          + consolidated-error variant

5-ii  Per-IP token bucket + per-session counters + daily budget tracker
        ─ relay/internal/ratelimit/perip (rate.Limiter wrapper, eager GC)
        ─ relay/internal/auth/store NewStoreWithCaps + CheckSession + RecordTurn
        ─ relay/internal/budget Tracker (Reserve/Commit + tenant key + UTC reset)
        ─ turnHandler check-then-stream wiring + per-session increment after SSEDone
        ─ perIPLimitMiddleware replaces 5-i's stub
        ─ smoke (self-runnable): 429 on burst exceed; 429 on session-exhaust;
          503 on budget exceed (uses local fake provider — no credit)

5-iii CAPTCHA verifier + /init integration
        ─ relay/internal/captcha {Verifier, Turnstile, HCaptcha, Stub}
        ─ siteverify HTTP client (httptest-mocked in tests; redact secret per L005)
        ─ /init: parse captcha_token; verify when required; emit captcha_required
          on missing token; new Capabilities.RequiresCaptcha + InitCaptcha hint
        ─ Replay-protection in-memory set
        ─ smoke (PAUSED for maintainer): Cloudflare Turnstile test sitekey
          → token → /init

5-iv  Smoke + docs + LESSONS
        ─ Live end-to-end smoke driver (drive-abuse.ts: rate-limit + session-cap + budget)
        ─ Live CAPTCHA smoke (paused for maintainer; Turnstile test sitekey)
        ─ Q9 startup-gate smoke runner
        ─ Doc updates (docs/PROJECT.md §10 cross-refs; README §Auth & abuse)
        ─ LESSONS L016+ as needed
```

Dependency graph:

```
5-i (config + middleware chain seam + Q9 + dispatcher hardening)
      │
      ├──► 5-ii (per-IP + per-session + budget)   ┐
      │                                           │  mutually independent;
      └──► 5-iii (captcha)                        ┘  parallelizable after 5-i
                  │
                  ▼
            5-iv (final smoke + docs)
```

## 11.1 Strict CORS — inherited from Phase 1, no change

The decomposition §2 Phase 5 row lists "strict CORS/origin enforcement" as a deliverable. Phase 1's `corsMiddleware` (`relay/internal/server/server.go:51-96`) already enforces this: no wildcard `Access-Control-Allow-Origin`, header set only for origins explicitly listed in `server.cors_origins`, disallowed-origin OPTIONS preflight returns 403, and `Vary: Origin` is always set. Phase 5 introduces **no changes** to CORS — the existing behavior already meets PROJECT.md §10 "Origin enforcement" and §17 "CORS: strict allowlist; no wildcards in production guidance." A regression test for the Phase-1 CORS shape is run as part of the 5-iv smoke (curl with a disallowed Origin against /v1/intake/init → expect no `Access-Control-Allow-Origin` header in the response).

## 12. Out of scope

- **`action_on_exceeded: "queue"`** — documented as v1+. v0 only ships `"reject"`.
- **Persistent rate-limit state** (multi-instance shared limiter) — v1+ (decomposition §3.1 trigger).
- **Per-tenant CAPTCHA thresholds** — v0 uses one global CAPTCHA config.
- **CAPTCHA on /turn or /submit** — v0 only gates /init (one-shot, bound to session).
- **Cookie-based long-lived CAPTCHA bypass** — v1+; v0 always requires a fresh token per session.
- **Sliding-window per-IP limiter** (e.g. 100 req/min) — v0 ships a token bucket only.
- **CIDR allowlist for /v1/health and /v1/version** — these endpoints stay open by design.
- **An additional `GET /v1/intake/capabilities` endpoint to avoid the two-call /init dance** — v1+.

## 13. Notes

- Module path remains `intake`. Go 1.23.2.
- Per-session and per-IP counters are in-memory only. Restart drops both — acceptable for v0 (PROJECT.md §10 implicit; §2 v0 goal #9 "stateless operation").
- The daily-budget tracker is in-memory only. Restart drops the day's counters; an operator restarting at 23:00 effectively grants the tenant a fresh budget for the last hour of the day. Documented in 5-iv's notes.
- L010 (PS 5.1 BOM) applies to any new smoke YAML written via `Set-Content` — use `-Encoding ascii`.
- L015 (derived-field test gaps) — every new dispatcher test added in 5-i MUST use the existing `requireFullSessionContext` helper.

*End of Phase 5 design.*
