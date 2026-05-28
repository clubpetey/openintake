# Phase 4 — Auth Breadth: Design

> **Status:** Approved design (brainstorming output), pre-planning
> **Date:** 2026-05-28
> **Implements:** [docs/PROJECT.md](../PROJECT.md) §5 (auth modes), §6 (auth endpoints), §9 (config), §17 (security)
> **Parent:** [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 4 row)
> **Builds on:** Phase 1's frozen `auth.Middleware`/`SessionContext`/`Deps.Auth` seam (`relay/internal/auth/`) — the middleware already 501s on `Authorization: Bearer` with a "Phase 4 seam" comment; `SessionContext`'s `UserID`/`Email`/`DisplayName`/`Custom` fields are present-but-dormant, ready to populate.

## 1. Goal

Add the two non-anonymous auth modes from PROJECT.md §5 behind the **frozen** `auth.Middleware` shape: **email magic-link** (Mode B — SMTP → 6-digit code → 15-min HS256 JWT) and **host-app SSO** (Mode C — validate inbound JWT via JWKS RS256 or shared-secret HS256). A single relay instance may enable any combination of anonymous + email + SSO simultaneously (PROJECT.md §5 close: "A single relay instance can enable any combination of these modes").

Success (decomposition §2, Phase 4 row): "Email flow end-to-end via Mailpit (`user.verified=true`); SSO validates a real Auth0/OIDC RS256 access token."

The `auth.Middleware` from Phase 1 stays the **only** identity-resolution path into `/turn`/`/submit`; Phase 4 only replaces its `Authorization: Bearer` 501 branch with two ordered validators, and adds two new unauth endpoints to bootstrap email JWTs.

## 2. Seams this phase introduces

### 2.1 Middleware dispatcher (replaces the Phase-1 501)

```
For each request that includes `Authorization: Bearer <token>`:
  1. If auth.modes.email is enabled:
       try emailJWT.Verify(secret, token)
         → on success, populate SessionContext{AuthMode:"email", Verified:true, Email}
                       call next, return
         → on failure, fall through
  2. If auth.modes.sso is enabled:
       try ssoVerifier.Verify(token)
         → on success, populate SessionContext{AuthMode:"sso", Verified:true, UserID, Email?, DisplayName?, Custom}
                       call next, return
         → on failure, fall through
  3. 401 unauthorized

For each request WITHOUT `Authorization: Bearer`:
  4. If auth.modes.anonymous is enabled AND X-Intake-Session is present + Store.Validate:
       populate SessionContext{AuthMode:"anonymous", Verified:false}
       call next, return
  5. 401 unauthorized
```

**Try-email-first** ordering chosen as a cheap-fail dispatch: an HS256 verify is microseconds; the worst case is one wasted HS256 check per real-SSO request. No protocol changes, no widget changes, no extra header. Anonymous remains the no-`Authorization` path.

The frozen `auth.Middleware.Handler` signature is unchanged. `SessionContext`'s Phase-1 fields (`UserID`/`Email`/`DisplayName`/`Custom`) become populated; `payloadbuild` already reads them — no change there.

### 2.2 Three new auth sub-packages

All under `relay/internal/auth/` to keep the auth domain together:

- **`auth/emailcode`** — thread-safe `*Store` of pending codes. `Issue(email) (code string, retryAfter time.Duration, err error)`: rejects if ≥3 codes already issued for this email in the last 10 min; otherwise generates a 6-digit code, stores `{code, sentAt, used:false}` keyed by email, returns the code. `Verify(email, code) (ok bool)`: returns true once for a matching unexpired unused code; marks used. Background goroutine prunes entries older than 10 min. Mirrors the existing `auth.Store` patterns.
- **`auth/smtpsend`** — `Sender` interface (`Send(ctx, to, code) error`); `NetSMTP` default impl uses stdlib `net/smtp` with `PlainAuth` over STARTTLS (covers SES/Postmark/SendGrid/Mailgun/self-hosted). Constructed from `EmailConfig` (host/port/user/pass-resolved-from-env/from). Tests inject a `FakeSender` that captures `(to, code)` in memory.
- **`auth/emailjwt`** — `Mint(secret []byte, email string, ttl time.Duration) (token string, expiresAt time.Time, err error)` and `Verify(secret, token) (email string, err error)`. HS256. Claims `{sub: email, iat, exp, iss: "intake-email"}`. The `iss` lets the SSO verifier reject our own tokens trivially.

### 2.3 SSO verifier (one verifier interface, two implementations)

`relay/internal/auth/sso/`:

```go
package sso

type Claims struct {
    UserID      string
    Email       *string
    DisplayName *string
    Custom      map[string]any
}

type Verifier interface {
    Verify(ctx context.Context, token string) (*Claims, error)
}
```

Two impls selected at config-time:

- **`RS256Verifier`** — backed by `MicahParks/keyfunc/v3` (handles JWKS fetch+cache+refresh-on-miss against `jwks_url`). Validates iss/aud/exp/nbf + 30s clock-skew. Maps the configured claim names (defaults `sub`/`email`/`name`) into `Claims`.
- **`HS256Verifier`** — shared secret (resolved from env). Same iss/aud/exp/nbf checks. Same claim mapping.

`sso.New(cfg config.SSOConfig) (Verifier, error)` is a factory that selects the impl based on which secret/JWKS the operator provided. If both are set, that's a config error (fail at startup).

### 2.4 Two new endpoints (additive, UNAUTH)

`POST /v1/intake/auth/email/start`:
- Body: `{"email":"<rfc5322 address>"}`
- 200 `{"message_sent": true}` on success.
- 400 on malformed body or invalid email format.
- 429 + `Retry-After: <seconds>` + generic body on rate-limit (≥3 codes in 10 min for this address). The body is generic — `{error:{code:"rate_limited", message:"too many codes requested for this email; retry later"}}` — and does NOT expose count or window-reset timestamp (anti-enumeration; the operator-facing log carries the detail).
- 502 on SMTP failure (with the underlying error in the server-side log, never in the response body).

`POST /v1/intake/auth/email/verify`:
- Body: `{"email":"...", "code":"123456"}`
- Success: `200 {"token":"<jwt>", "expires_at":"<iso8601>", "user":{"email":"...", "verified":true}}`
- 401 on missing/expired/used/wrong code. Generic body, no detail on which (anti-enumeration).
- 400 on malformed body.

Both endpoints are NOT behind `auth.Middleware.Handler` — they're the bootstrap. Mounted at the top of `registerIntakeRoutes` alongside `/init`.

### 2.5 `/v1/intake/init` capability response (additive)

`InitResponse` gains an `auth` block so the widget knows which flows to expose:

```json
{
  "session_id": "<uuid>",
  "capabilities": { ... existing ... },
  "auth": {
    "modes": ["anonymous", "email", "sso"],
    "email": { "code_ttl_seconds": 600 }
  }
}
```

`modes` lists the enabled modes (subset of all three); `email` is omitted when email is disabled. Backward-compatible — existing widgets that ignore the field still work.

## 3. Sub-plan decomposition (`ai/tasks/phase-4/`)

Four sub-plans. Seam first (4-i), then email (4-ii) and SSO (4-iii) — mutually independent after 4-i — then the final live smoke (4-iv). Same shape as Phase 2's seam → providers → smoke pattern.

| # | Unit | Adds | Sub-plan smoke |
|---|---|---|---|
| **4-i** | **Config + middleware dispatcher seam** | `AuthConfig` gains `Email`, `SSO`, `Modes{Email,SSO}` sub-structs (frozen here); `auth.Middleware` updated to try-email-then-sso-then-anonymous dispatcher with stub validators that fail-cleanly until 4-ii/4-iii; `Deps` gains `EmailService` + `SSOVerifier` optional fields; init capabilities response extended | Unit: dispatcher correctly routes (a) email-JWT to email validator, (b) SSO-JWT to SSO validator, (c) anonymous to existing Store path, (d) all-disabled → 401 cleanly; existing Phase 1 anonymous tests stay green |
| **4-ii** | **Email magic-link** | `auth/emailcode` + `auth/smtpsend` + `auth/emailjwt` packages; `/auth/email/start` + `/auth/email/verify` handlers; `main.go` wires `EmailService` when `auth.modes.email=true` | Unit: codestore TTL/rate-limit/single-use; SMTP send via `FakeSender`; emailjwt mint+verify round-trip + tamper rejection. Live: `start → verify → /turn` against local MailHog (we already have one running from the Fider stack) |
| **4-iii** | **Host-app SSO** | `auth/sso` package with `Verifier` interface, `RS256Verifier` (keyfunc-backed), `HS256Verifier`; configurable `auth.sso.claims` mapping with defaults; `sso.New` factory | Unit: RS256 with in-test RSA keypair + `httptest.Server` JWKS endpoint (happy path + expired/wrong-iss/wrong-aud/tampered/wrong-kid); HS256 with shared secret (same matrix); claim-mapping happy path + override; both-secret-and-jwks-set → factory error. Live: deferred to phase final smoke. |
| **4-iv** | **Final smoke + docs** | Phase 4 README + live smoke evidence + LESSONS additions | Live email smoke against MailHog (no maintainer pause needed — already configured); live SSO smoke **pauses for maintainer** (either real Auth0 tenant + minted access token, or a self-served local JWKS with maintainer-signed RS256 token). |

**Dependency graph:** `4-i → {4-ii, 4-iii} → 4-iv`. 4-ii and 4-iii are mutually independent after 4-i and may be implemented in parallel (different packages, different endpoints, only the middleware dispatcher in 4-i is shared) — or serially. Pick at execution time.

## 4. Config schema (additive)

Frozen in 4-i:

```yaml
auth:
  modes:
    anonymous: true
    email: false      # 4-ii flips on
    sso: false        # 4-iii flips on
  email:
    smtp_host: "smtp.example.com"
    smtp_port: 587
    smtp_user: "intake@example.com"
    smtp_pass_env: "INTAKE_SMTP_PASS"        # secret env var name; never the value
    from: "Intake <intake@example.com>"
    code_ttl: "10m"                          # how long a code is valid
    jwt_ttl: "15m"                           # how long an issued JWT is valid
    jwt_secret_env: "INTAKE_EMAIL_JWT_SECRET" # secret env var name; ≥32 bytes (§17)
  sso:
    issuer: "https://example.us.auth0.com/"
    audience: "https://api.example.com"
    jwks_url: "https://example.us.auth0.com/.well-known/jwks.json"   # RS256 path
    hs256_secret_env: ""                     # HS256 path (alternative to jwks_url); secret env name
    claims:
      user_id: "sub"
      email: "email"
      display_name: "name"
```

- All adapter-style secrets (smtp_pass, email JWT secret, HS256 SSO secret) flow through `config.ResolveSecret` (env-or-`_FILE`, never the value in YAML).
- Setting both `jwks_url` and `hs256_secret_env` is a startup config error.
- `applyDefaults`: `code_ttl=10m`, `jwt_ttl=15m`, claims default to `sub`/`email`/`name`.

## 5. JWT shapes (locked)

**Email JWT** (minted by `/auth/email/verify`, consumed by `auth/emailjwt.Verify`):
- Algorithm: HS256
- Header: standard (`{alg:"HS256", typ:"JWT"}`)
- Claims: `{sub:<verified email>, iat:<unix>, exp:<iat + jwt_ttl>, iss:"intake-email"}`
- Secret: `INTAKE_EMAIL_JWT_SECRET` (≥32 bytes per §17; verified at startup)
- TTL: 15 minutes (configurable `jwt_ttl`)

**SSO JWT** (issued by the host app, consumed by `auth/sso.Verify`):
- Algorithm: RS256 (JWKS path) or HS256 (shared-secret path) — alg validated against configured path (mitigates alg-confusion attacks)
- Issuer: must equal `auth.sso.issuer`
- Audience: must contain `auth.sso.audience`
- Expiry/not-before: with 30s clock skew
- Claims read via `auth.sso.claims`: defaults `sub`→`UserID`, `email`→`Email`, `name`→`DisplayName`; any unmapped claims go into `Custom map[string]any`.

## 6. Endpoint contracts (locked)

```
POST /v1/intake/auth/email/start
  Body:     {"email":"<address>"}
  200:      {"message_sent": true}
  400:      {"error":{"code":"bad_request","message":"<reason>"}}     # bad JSON or invalid email format
  429:      {"error":{"code":"rate_limited","message":"too many codes requested for this email; retry later"}}
            Headers: Retry-After: <seconds-until-window-resets>
  502:      {"error":{"code":"smtp_error","message":"could not send email"}}   # SMTP failure; details server-side only

POST /v1/intake/auth/email/verify
  Body:     {"email":"<address>", "code":"<6 digits>"}
  200:      {"token":"<jwt>", "expires_at":"<iso8601>", "user":{"email":"<address>","verified":true}}
  401:      {"error":{"code":"invalid_code","message":"invalid or expired code"}}   # generic; no enumeration
  400:      {"error":{"code":"bad_request","message":"<reason>"}}
```

Both endpoints CORS-allowlisted (respects `server.cors_origins`). Both UNAUTH (not behind `auth.Middleware.Handler`).

For `/turn` and `/submit` (already auth-gated): when the bearer is a valid email JWT or SSO JWT, `SessionContext` is populated with verified identity and `payloadbuild.Build` emits `IntakePayload.user.auth_mode = "email"|"sso"`, `user.email`, `user.display_name`, `user.id`, `user.verified = true`. The widget needn't change its submit shape — the relay does the mapping.

## 7. Tool version pins (additive — Phase 4 introduces two)

| Tool | Pin | Reason |
|---|---|---|
| `github.com/golang-jwt/jwt/v5` | exact (verify+pin at install) | Mint+verify HS256 (email JWTs) and verify RS256 (SSO). Security-critical surface — caret forbidden. v5 is current; v4 is older and unmaintained. |
| `github.com/MicahParks/keyfunc/v3` | exact (verify+pin at install) | JWKS fetch + cache + refresh-on-miss for RS256 SSO. v3 is current; v2 is older API. Caret forbidden. |
| (smtp send) | — (stdlib `net/smtp`) | Stdlib covers SMTP-AUTH PLAIN/LOGIN over STARTTLS — enough for any modern provider; no SDK to maintain. |

`scripts/check-pins.sh` extended to fail on a caret/`@latest` for either new module.

## 8. Build-fail checklist (for the phase README)

- [ ] `go build ./...` / `go vet ./...` fails in `relay/`. **Fail.**
- [ ] Any Go test fails (`go test ./...`). **Fail.**
- [ ] Phase-0 contract gate regresses (`scripts/verify-contract.sh`). **Fail.**
- [ ] An SMTP password, email JWT secret, SSO HS256 secret, or raw bearer token appears in a log line, error string, or response body. **Fail.**
- [ ] `golang-jwt` or `keyfunc` pinned with a caret or `@latest`. **Fail** (check-pins gate).
- [ ] `auth.Modes.Email=true` with empty `jwt_secret_env` resolution, or `jwt_secret` <32 bytes → relay starts. **Fail** (must error fatally).
- [ ] `auth.Modes.SSO=true` with both `jwks_url` AND `hs256_secret_env` set → relay starts. **Fail** (must error fatally — mutually exclusive).
- [ ] Email rate-limit returns 200 instead of 429. **Fail** (test must assert 429 + Retry-After).
- [ ] SSO token signed with HS256 accepted when configured for RS256, or vice versa. **Fail** (alg-confusion mitigation).
- [ ] Anonymous behavior changes for existing Phase-1 callers (regression). **Fail.**

## 9. Testing strategy

**Credit-free unit tests per sub-plan:**

- `emailcode`: TTL eviction (write→`time.Sleep`-equivalent via injectable `now`→verify expired); single-use (verify-twice → second fails); rate-limit (3 issues OK, 4th returns retry-after; 11 minutes later resets); concurrent issue/verify race-free.
- `smtpsend`: `FakeSender` captures `(to, code)` tuples; production `NetSMTP` impl is exercised in the live smoke (no SMTP fixture in unit tests).
- `emailjwt`: mint→verify round-trip; tampered token rejected; wrong-secret rejected; expired token rejected; non-email JWT issuer rejected.
- `sso` (RS256): ephemeral RSA keypair generated in-test; `httptest.Server` serves a JWKS containing the matching pubkey; happy-path verify + tamper/wrong-iss/wrong-aud/expired/wrong-kid all rejected with distinct errors; alg-confusion test (HS256 token rejected when verifier configured for RS256).
- `sso` (HS256): shared-secret round-trip + the same rejection matrix.
- Dispatcher (in `auth.Middleware`): table-driven — for each combination of (email-mode, sso-mode, anonymous-mode) × (request style), assert the correct validator is reached and the correct `SessionContext.AuthMode` is set.

**Live smokes:**

- **Email (no pause):** local MailHog instance (existing — running on `192.168.1.102:1025/8025`, or spin up a fresh local one). Configure relay's email mode pointing at MailHog. Driver hits `/auth/email/start` (real email lands in MailHog UI), reads the captured code, calls `/auth/email/verify`, gets a JWT, calls `/turn` with the bearer, asserts `user.email/verified/auth_mode` in the SubmitResponse's payload.
- **SSO (pause for maintainer):** two paths, maintainer picks:
  - **(a) Real Auth0 (or other IdP):** create a free Auth0 tenant, configure an API + an M2M client, hit `/oauth/token` to mint a real RS256 access token, pass it to the smoke driver.
  - **(b) Self-served JWKS:** generate an RSA keypair locally, serve a small static `jwks.json` from a local HTTP server, mint a test JWT with `jwt-cli` or a few lines of Go, point the relay at that JWKS URL.

## 10. ADRs locked by this phase

- **Try-email-then-SSO middleware ordering** (§2.1). Cheap-fail dispatch via HS256 verify (microseconds); no protocol/widget changes. Revisit if `iss`-claim-routing or an explicit mode header proves cleaner under load.
- **`golang-jwt/jwt/v5` + `MicahParks/keyfunc/v3` as the only new external deps for Phase 4** (§3, §7). Two well-maintained, widely-vetted libs on a security-critical surface; hand-rolled JWT/JWKS is the wrong trade-off. Revisit if either becomes unmaintained.
- **Stdlib `net/smtp` behind a `Sender` interface** (§2.2). Sufficient for one-line auth-code emails; no new send-side dep. Revisit if email becomes a richer surface in v1.
- **In-memory code store** (§2.2). No persistence dependency; restart invalidates in-flight codes (acceptable — codes are 10-min and users just retry). Revisit if running multi-instance behind a load balancer (would need a shared store, redis-style).
- **HS256 alg-pinning per verifier** (§5). The HS256 verifier rejects RS256 tokens and vice versa — explicit `alg` whitelist mitigates alg-confusion attacks. Locked in `sso` factory.
- **Claim mapping is configurable** (§4). Defaults `sub`/`email`/`name`; overridable for custom IdPs. Locked in `auth.sso.claims`.

## 11. Non-goals (Phase 4)

- **No CAPTCHA on `/auth/email/start`.** Per-email rate-limit is the only rate gate Phase 4 adds. Per-IP rate-limiting and CAPTCHA are Phase 5 (abuse & spend control). For now, anonymous mode with no CAPTCHA is still allowed (§9 Q9 will tighten that in P5).
- **No persistent code store / no multi-instance support.** v0 is single-instance.
- **No JWT key rotation / overlapping keys.** Restart relay = invalidate JWTs. v1+.
- **No OAuth2/OIDC code flow** (initiating login). The relay validates JWTs minted *by the host app* — it does not redirect to an IdP or handle authorization codes.
- **No social-only SSO (Google/GitHub/etc.).** The host app authenticates the user however it wants and hands the relay a JWT; the relay doesn't care how the host got it.
- **No multi-tenancy in auth.** Tenant scoping is the hosted-relay project's concern (PROJECT.md §16).
