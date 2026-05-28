# Phase 4 — Auth Breadth

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement the sub-plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Adds the two non-anonymous auth modes from PROJECT.md §5 behind the **frozen** `auth.Middleware` shape: **email magic-link** (SMTP → 6-digit code → 15-min HS256 JWT) and **host-app SSO** (validate inbound JWT via JWKS RS256 or shared-secret HS256). The Phase-1 middleware's `Authorization: Bearer` 501 branch is replaced with a **try-email-then-sso-then-anonymous** dispatcher. A single relay instance can enable any combination of all three modes simultaneously.

## 1. Spec link

- Phase 4 design: [docs/specs/2026-05-28-phase-4-auth-breadth-design.md](../../../docs/specs/2026-05-28-phase-4-auth-breadth-design.md)
- Parent decomposition: [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](../../../docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 4 row)
- Source of truth for scope/contracts: [docs/PROJECT.md](../../../docs/PROJECT.md) §5 (auth modes), §6 (endpoints), §9 (config), §17 (security)
- Secrets seam: [docs/specs/2026-05-27-configuration-and-secrets-design.md](../../../docs/specs/2026-05-27-configuration-and-secrets-design.md) (`config.ResolveSecret`)
- Phase-1 frozen seam: `relay/internal/auth/middleware.go` (the `Authorization: Bearer` 501 branch is Phase 4's replacement target; `auth.SessionContext`'s `UserID`/`Email`/`DisplayName`/`Custom` fields are present-but-dormant)

## 2. Architectural Decision Record (ADR) summary

- **Try-email-then-SSO middleware dispatch** for inbound `Authorization: Bearer` tokens. Cheap-fail order (HS256 verify is ~µs); no protocol changes, no widget changes. Revisit trigger: per-request load makes the wasted HS256 check measurable, or a custom IdP uses an iss/kid we'd need to peek-before-verify.
- **`golang-jwt/jwt/v5` + `MicahParks/keyfunc/v3`** as the only new external dependencies. Mint+verify HS256/RS256 + JWKS fetch/cache/refresh-on-miss. Phase 4 is the first phase to introduce non-stdlib deps to the relay since Phase 2's LLM SDKs. Caret-forbidden (both are on the security boundary). Revisit trigger: either becomes unmaintained, or `golang-jwt/v6` ships with breaking changes.
- **Stdlib `net/smtp` behind a `Sender` interface.** Sufficient for one-line auth-code emails over any modern SMTP-AUTH provider; no new send-side dep. Tests inject a `FakeSender`. Revisit trigger: email becomes a richer surface in v1 (attachments, DSN, templating).
- **In-memory code store with rate-limit + TTL**, mirrors the existing `auth.Store` pattern. No persistence; restart invalidates in-flight codes (acceptable — codes are 10-min and users just retry). Revisit trigger: multi-instance deployment behind a load balancer (needs a shared store, e.g. redis).
- **Algorithm pinning per SSO verifier.** `RS256Verifier` rejects HS256 tokens and vice versa — explicit `alg` whitelist mitigates alg-confusion attacks. Setting both `jwks_url` AND `hs256_secret_env` is a startup config error.
- **Configurable SSO claim mapping with OIDC defaults.** `sub`/`email`/`name` defaults; overridable per-deployment for custom IdPs.

This phase does NOT add: CAPTCHA (Phase 5), per-IP rate limiting (Phase 5), persistent code storage (v1+), key rotation (v1+), OAuth/OIDC code flow (we validate JWTs the host minted, not initiate login), social-only SSO (Google/GitHub login UX), or auth multi-tenancy (hosted-relay project).

## 3. Sub-plan index

| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 4-i | [Config + middleware dispatcher seam](4-i-config-middleware-dispatcher-plan.md) | the seam | M | Not started |
| 4-ii | [Email magic-link](4-ii-email-magic-link-plan.md) | Mode B | M | Not started |
| 4-iii | [Host-app SSO](4-iii-sso-verifier-plan.md) | Mode C | M | Not started |
| 4-iv | [Final smoke + docs](4-iv-smoke-docs-plan.md) | live email + SSO maintainer pause | S | Not started |

## 4. Dependency graph

```
4-i (config + middleware dispatcher seam)
      │
      ├──► 4-ii (email magic-link)   ┐
      │                              │  mutually independent;
      └──► 4-iii (SSO verifier)      ┘  parallelizable after 4-i
                  │
                  ▼
            4-iv (final smoke + docs)
```

4-i locks the `AuthConfig` shape, adds the try-email-then-sso-then-anonymous dispatcher (with stub validators that fail-cleanly), and extends `Deps` with optional `EmailService` + `SSOVerifier` fields. 4-ii and 4-iii each add one of the validators (and 4-ii additionally adds the two unauth endpoints) by populating the optional `Deps` field — they touch separate packages and separate code paths in the dispatcher. 4-iv records the live evidence + lessons.

## 5. Tool version pin list

Phase 4 introduces **two** new Go modules — the relay's first non-stdlib additions since Phase 2's provider SDKs. Both are on the security boundary; caret-versioning forbidden. Verify the exact latest at install (in 4-i's first task), pin exactly, update this table + `scripts/check-pins.sh` in the same commit.

| Tool | Version | Reason |
|---|---|---|
| `github.com/golang-jwt/jwt/v5` | verify+pin exact at install | Mint HS256 (email JWTs) + verify HS256/RS256 (SSO). v5 is current; v4 unmaintained. Security-critical surface — caret forbidden. |
| `github.com/MicahParks/keyfunc/v3` | verify+pin exact at install | JWKS fetch + cache + refresh-on-miss for RS256 SSO. v3 is current API; v2 is older. Caret forbidden. |
| (smtp send) | — (stdlib `net/smtp`) | SMTP-AUTH PLAIN/LOGIN over STARTTLS — enough for any modern provider; no SDK to maintain. |

`scripts/check-pins.sh` extended to fail on a caret/`@latest` for either new module (mirrors the existing anthropic/openai/genai checks).

## 6. Build-fail checklist

- [ ] `go build ./...` / `go vet ./...` fails in `relay/`. **Fail.**
- [ ] Any Go test fails (`go test ./...`). **Fail.**
- [ ] The Phase-0 contract gate regresses (`scripts/verify-contract.sh`). **Fail.**
- [ ] An SMTP password, email JWT secret, SSO HS256 secret, or raw bearer token appears in a log line, error string, or response body. **Fail.**
- [ ] `golang-jwt` or `keyfunc` pinned with a caret or `@latest`. **Fail** (check-pins gate).
- [ ] `auth.modes.email=true` with `INTAKE_EMAIL_JWT_SECRET` unresolved or `<32 bytes` → relay starts. **Fail** (must fatal at startup).
- [ ] `auth.modes.sso=true` with both `jwks_url` AND `hs256_secret_env` set → relay starts. **Fail** (mutually exclusive; must fatal at startup).
- [ ] `auth.modes.sso=true` with neither `jwks_url` nor `hs256_secret_env` set → relay starts. **Fail** (sso must have an exactly-one mode; must fatal at startup).
- [ ] Email rate-limit returns 200 instead of 429 with `Retry-After`. **Fail.**
- [ ] An HS256 token is accepted by `RS256Verifier`, or vice versa. **Fail** (alg-confusion mitigation must be enforced).
- [ ] An expired or tampered JWT is accepted. **Fail.**
- [ ] Anonymous behavior regresses for existing Phase-1 callers — `X-Intake-Session` + no `Authorization: Bearer` must still produce `SessionContext{AuthMode:"anonymous", Verified:false}`. **Fail.**
- [ ] `payloadbuild` emits a payload with mismatched `user.auth_mode`/`user.verified`/`user.email` for a verified email or SSO request. **Fail.**

## 7. Final smoke (mandatory)

Proves the Phase 4 deliverable end-to-end. The unit layer (mock SMTP, ephemeral HS256 secret, in-test RSA keypair + `httptest.Server` JWKS) is fully credit-free and runs in `go test ./...`. The **live** email smoke uses a local MailHog/Mailpit (no maintainer pause needed — MailHog is already running on `192.168.1.102:1025/8025` from Phase 3's Fider stack, or a fresh local instance works). The **live SSO smoke pauses for the maintainer** (needs a real IdP-issued token).

```
1. Live email smoke (no pause):
   a. Configure relay with auth.modes.email=true, smtp_host/port pointing at MailHog,
      INTAKE_EMAIL_JWT_SECRET (≥32 bytes) in env.
   b. core/smoke/drive-auth-email.ts: POST /auth/email/start{email:pete@mantichor.com}
      → 200 {message_sent:true}; opens MailHog UI to read the captured code; calls
      /auth/email/verify{email,code} → receives a JWT; drives /turn with
      Authorization: Bearer <jwt> → /submit → asserts SubmitResponse, then asserts
      the canonical payload posted to a local webhook receiver carries
      user.auth_mode="email", user.email="pete@mantichor.com", user.verified=true.
   c. Re-runnable.
2. Live SSO smoke (PAUSE for maintainer): two paths, maintainer picks:
   (a) REAL Auth0 — free tenant; create an API + M2M client; hit /oauth/token to mint
       a real RS256 access token; configure relay with auth.modes.sso=true, issuer,
       audience, jwks_url; drive /turn with Authorization: Bearer <jwt>; assert
       user.auth_mode="sso", user.verified=true, user.id=<sub claim>.
   (b) SELF-SERVED JWKS — generate an RSA keypair locally; serve a static
       /.well-known/jwks.json from any local HTTP server; mint a test JWT via the
       intake-license tool (extended for SSO test-token signing) or jwt-cli;
       configure jwks_url at the local server; verify end-to-end.
3. Free-mode gate regression: anonymous /init → /turn flow (Phase-1 walking skeleton)
   still works — proves the dispatcher's fall-through to the anonymous resolver did
   not break the Phase-1 contract.
```

A phase is NOT done until this smoke passes from a clean state. Step 1 (live email) is self-runnable; step 2 (live SSO) pauses for explicit maintainer go-ahead per the credit/secret guard.

## 8. Shared Contracts (SINGLE SOURCE OF TRUTH)

These shapes are **frozen** in the noted sub-plan; later sub-plans consume them unchanged.

### 8.1 The frozen Phase-1 seams (UNCHANGED)

- `auth.Middleware.Handler` signature — chi-compatible `func(http.Handler) http.Handler`. The internals change (the 501 branch becomes a try-email-then-sso dispatcher) but the public shape is the same.
- `auth.SessionContext` struct (`relay/internal/auth/session.go`) — UNCHANGED. The `UserID`/`Email`/`DisplayName`/`Custom` fields, dormant in Phase 1, become populated by the email/sso branches.
- `auth.WithSession` / `auth.FromContext` — UNCHANGED. Handlers continue to read identity via `auth.FromContext`.
- `auth.Store` (anonymous-session store) — UNCHANGED. Continues to back `X-Intake-Session`.
- `server.Deps` field set — additive (4-i adds `EmailService` and `SSOVerifier` optional pointer fields; existing fields unchanged).
- `payloadbuild.Builder` — UNCHANGED. Already reads `SessionContext.{AuthMode,Email,UserID,DisplayName}` and emits the corresponding `IntakePayload.user.*`. No payloadbuild edits in Phase 4.

### 8.2 Config sub-structs (additive — `relay/internal/config/config.go`, FROZEN in 4-i)

```go
// AuthConfig gains Email + SSO sub-structs; AuthModes gains Email + SSO flags.
type AuthConfig struct {
	Modes AuthModes   `yaml:"modes"`
	Email EmailConfig `yaml:"email"`   // 4-i (used by 4-ii)
	SSO   SSOConfig   `yaml:"sso"`     // 4-i (used by 4-iii)
}

type AuthModes struct {
	Anonymous bool `yaml:"anonymous"`
	Email     bool `yaml:"email"`     // 4-i
	SSO       bool `yaml:"sso"`       // 4-i
}

// EmailConfig configures the email magic-link mode.
type EmailConfig struct {
	SMTPHost     string `yaml:"smtp_host"`
	SMTPPort     int    `yaml:"smtp_port"`
	SMTPUser     string `yaml:"smtp_user"`
	SMTPPassEnv  string `yaml:"smtp_pass_env"`   // env var name; never the value
	From         string `yaml:"from"`            // RFC 5322 address shown to the user
	CodeTTL      string `yaml:"code_ttl"`        // "10m" default
	JWTTTL       string `yaml:"jwt_ttl"`         // "15m" default
	JWTSecretEnv string `yaml:"jwt_secret_env"`  // env var name; resolved value must be ≥32 bytes
}

// SSOConfig configures host-app SSO.
type SSOConfig struct {
	Issuer         string     `yaml:"issuer"`            // expected `iss` claim
	Audience       string     `yaml:"audience"`          // expected `aud` claim (single)
	JWKSURL        string     `yaml:"jwks_url"`          // RS256 path; mutually exclusive with HS256SecretEnv
	HS256SecretEnv string     `yaml:"hs256_secret_env"`  // HS256 path; env var name
	Claims         SSOClaims  `yaml:"claims"`
}

// SSOClaims maps standard SessionContext fields to JWT claim names.
type SSOClaims struct {
	UserID      string `yaml:"user_id"`       // default "sub"
	Email       string `yaml:"email"`         // default "email"
	DisplayName string `yaml:"display_name"`  // default "name"
}
```

Defaults applied in `config.applyDefaults` (4-i): `CodeTTL="10m"`, `JWTTTL="15m"`, `Claims.UserID="sub"`, `Claims.Email="email"`, `Claims.DisplayName="name"`. All secrets resolve via `config.ResolveSecret` / `config.RequireSecret` (env-or-`_FILE`) at startup in `main.go` — never read directly from YAML, never logged.

### 8.3 Email packages (FROZEN in 4-ii — `relay/internal/auth/{emailcode,smtpsend,emailjwt}/`)

```go
// auth/emailcode — in-memory code store, rate-limited and TTL-evicted.
package emailcode

type Store struct{ /* unexported */ }

// New returns a Store with the given code TTL, rate-limit window, and per-window
// cap. `now` is injectable for tests (production: time.Now).
func New(codeTTL, rateWindow time.Duration, perWindowCap int, now func() time.Time) *Store

// Issue generates a 6-digit code for email. If ≥perWindowCap codes have been
// issued in the last rateWindow for this email, returns ("", retryAfter, ErrRateLimited)
// where retryAfter is seconds until the oldest still-in-window issuance ages out.
// On success, stores {code, sentAt, used:false} keyed by email and returns the code.
func (s *Store) Issue(email string) (code string, retryAfter time.Duration, err error)

// Verify reports whether code matches an unexpired, unused issuance for email.
// On match, marks it used (single-use); subsequent verify of the same code fails.
func (s *Store) Verify(email, code string) (ok bool)

var ErrRateLimited = errors.New("emailcode: rate-limited")

// auth/smtpsend — pluggable email sender.
package smtpsend

type Sender interface {
    // Send delivers a one-line auth-code email to `to`. Implementations MUST NOT
    // include the SMTP password, the email JWT secret, or any other secret in the
    // returned error.
    Send(ctx context.Context, to string, code string) error
}

// NetSMTP is the stdlib net/smtp implementation. Constructed from EmailConfig
// in main.go (passwordEnv is resolved via config.ResolveSecret before being passed in).
type NetSMTP struct{ /* unexported */ }

func NewNetSMTP(host string, port int, user, password, from string) *NetSMTP
func (n *NetSMTP) Send(ctx context.Context, to, code string) error

// FakeSender is the test double — captures (to, code) tuples in memory.
type FakeSender struct{ /* unexported; thread-safe */ }
func NewFakeSender() *FakeSender
func (f *FakeSender) Send(ctx context.Context, to, code string) error
func (f *FakeSender) Sent() []struct{ To, Code string }  // ordered, for assertions

// auth/emailjwt — HS256 mint+verify for email-mode JWTs.
package emailjwt

const Issuer = "intake-email" // baked into the iss claim; consumed by sso verifier
                              // for cheap rejection of email JWTs masquerading as SSO

// Mint returns a signed JWT for email with the given TTL. exp = now + ttl;
// iat = now; iss = Issuer; sub = email. HS256 with the supplied secret
// (caller pre-validates len(secret) >= 32 per PROJECT.md §17).
func Mint(secret []byte, email string, ttl time.Duration) (token string, expiresAt time.Time, err error)

// Verify validates the token (HS256, iss=Issuer, exp>now, sub non-empty)
// and returns the email from the sub claim. Returns an error for any defect:
// wrong signature, wrong iss, expired, malformed.
func Verify(secret []byte, token string) (email string, err error)
```

### 8.4 SSO verifier (FROZEN in 4-iii — `relay/internal/auth/sso/`)

```go
package sso

// Claims is the per-request identity surface extracted from a verified JWT.
// Maps onto auth.SessionContext.{UserID, Email, DisplayName, Custom}.
type Claims struct {
    UserID      string
    Email       *string
    DisplayName *string
    Custom      map[string]any
}

// Verifier is the single interface both impls satisfy.
type Verifier interface {
    Verify(ctx context.Context, token string) (*Claims, error)
}

// New constructs the configured verifier. Exactly one of cfg.JWKSURL or
// cfg.HS256SecretEnv must be set (the other empty); both-set or neither-set is a
// startup config error. The secret (for HS256) is the RESOLVED value, passed in
// by main.go via config.RequireSecret. cfg.Claims field names are honored
// (with applyDefaults having already populated sub/email/name).
func New(cfg config.SSOConfig, hs256Secret []byte, logger *slog.Logger) (Verifier, error)

// Internally:
//   - RS256Verifier uses keyfunc.NewDefault([]string{cfg.JWKSURL}) for JWKS fetch+cache.
//   - HS256Verifier uses the supplied secret directly.
//   - Both pin alg: RS256Verifier rejects tokens whose header.alg is not "RS256";
//     HS256Verifier rejects tokens whose header.alg is not "HS256". This mitigates
//     alg-confusion attacks (cannot pass an HS256 token signed with the JWKS pubkey).
//   - Both validate: iss == cfg.Issuer (exact match); aud contains cfg.Audience;
//     exp > now - 30s clock-skew; nbf < now + 30s clock-skew (if present); sub non-empty.
```

### 8.5 Endpoint contracts (FROZEN in 4-i for shape, fully wired in 4-ii)

```
POST /v1/intake/auth/email/start
  Auth:     NONE (unauth — bootstraps email JWTs)
  Body:     {"email":"<rfc5322 address>"}
  200:      {"message_sent": true}
  400:      {"error":{"code":"bad_request","message":"<reason>"}}
  429:      {"error":{"code":"rate_limited","message":"too many codes requested for this email; retry later"}}
            Headers: Retry-After: <seconds-until-window-resets>
  502:      {"error":{"code":"smtp_error","message":"could not send email"}}

POST /v1/intake/auth/email/verify
  Auth:     NONE (unauth)
  Body:     {"email":"<address>", "code":"<6 digits>"}
  200:      {"token":"<jwt>", "expires_at":"<iso8601>", "user":{"email":"<address>","verified":true}}
  401:      {"error":{"code":"invalid_code","message":"invalid or expired code"}}     # generic — no enumeration
  400:      {"error":{"code":"bad_request","message":"<reason>"}}
```

CORS-respected (`server.cors_origins` allowlist). Both endpoints mounted in `registerIntakeRoutes` ALONGSIDE `/init` (NOT under the auth-gated group).

### 8.6 Init capabilities (additive — `relay/internal/server/dto.go` + `turn.go`)

Existing `Capabilities.AuthModes` (Phase 1: `["anonymous"]`) is **extended** with `"email"` and/or `"sso"` when the corresponding `auth.modes.*` flag is true. Backward-compatible: existing widgets reading the same field see longer arrays.

A new top-level `auth` field carries mode-specific hints (Phase 4: just `email.code_ttl_seconds`):

```go
// Existing — extended in 4-i (additive):
type Capabilities struct {
    AuthModes []string `json:"auth_modes"` // Phase 1: ["anonymous"]; Phase 4: also "email"/"sso" when enabled
    Streaming bool     `json:"streaming"`
}

// NEW in 4-i (only present when at least one mode advertises a hint):
type InitAuth struct {
    Email *InitAuthEmail `json:"email,omitempty"`
}
type InitAuthEmail struct {
    CodeTTLSeconds int `json:"code_ttl_seconds"`
}

type InitResponse struct {
    SessionID    string       `json:"session_id"`
    Capabilities Capabilities `json:"capabilities"`
    Auth         *InitAuth    `json:"auth,omitempty"`     // 4-i; only when at least one hint applies
}
```

> Note: the design spec §2.5 sketched a richer `auth.modes` block; the canonical resolution (locked here) is to **extend the existing `capabilities.auth_modes` array** (backward-compatible) and add the NEW `auth.email.code_ttl_seconds` hint as a sibling top-level field. The spec's nested `auth.modes` would have duplicated `capabilities.auth_modes`.

## 9. Notes

- Module path remains `intake`. Go 1.23.2.
- `payloadbuild` already reads `SessionContext.{Email,DisplayName,UserID,AuthMode}` and emits the corresponding `IntakePayload.user.*`. Phase 4 only changes WHAT is in `SessionContext`; the wire format is unchanged.
- The email JWT carries the email in `sub`; the verifier puts it into `SessionContext.Email` (a `*string`). The SSO verifier puts the configured user_id-claim into `SessionContext.UserID` and the (optional) email/display-name claims into the corresponding `*string` fields.
- LESSONS L010 (PS 5.1 BOM) applies to any new smoke YAML written via `Set-Content` — use `-Encoding ascii`.
- LESSONS L011 (redact-before-truncate) applies to ANY error path that includes downstream content — SMTP responses, JWKS HTTP errors, JWT validation errors. Adapt the pattern using a redact helper per package or rely on the lib's own error texts (golang-jwt v5 errors are clean).
