# 4-i Config + Middleware Dispatcher Seam — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the Phase-4 config blocks (`Email`, `SSO`, `Modes{Email,SSO}`), introduce two small verifier interfaces in `relay/internal/auth/`, replace the Phase-1 `Authorization: Bearer` → 501 branch with a **try-email-then-sso-then-anonymous** dispatcher, and extend `InitResponse` with the new auth advertisement fields. After this sub-plan: no actual JWT validation happens yet (verifiers are nil unless 4-ii/4-iii are in place), but the seam is wired and the existing Phase-1 anonymous flow is fully preserved.

**Architecture:** Two new tiny interfaces in `relay/internal/auth/types.go` — `EmailJWTVerifier` (Verify a token → email) and `SSOVerifier` (Verify a token → `*SSOClaims`). `auth.Middleware` gains optional `email`/`sso` fields via a wider `NewMiddleware` constructor; the `Handler` dispatcher tries email first, then sso, then falls through to the existing anonymous path. `main.go` passes `nil` for both verifiers in 4-i (sub-plans 4-ii and 4-iii each wire their respective verifier when their mode is enabled). `InitResponse.Capabilities.AuthModes` is extended to include `"email"`/`"sso"` when the corresponding mode is enabled, and a new top-level `auth.email.code_ttl_seconds` hint is emitted when email mode is on.

**Tech Stack:** Go 1.23.2 (relay). No new dependencies in 4-i; both the JWT and JWKS libraries land in 4-ii and 4-iii respectively. `gopkg.in/yaml.v3` (already present) for the new config structs.

---

## Design References

- README §8.1 — frozen Phase-1 seams (unchanged here)
- README §8.2 — config sub-struct shapes (frozen in this plan)
- README §8.5 — endpoint contracts (shape locked here for 4-ii to wire)
- README §8.6 — init capabilities resolution (extended here)
- Design spec §2.1 — middleware dispatcher
- Design spec §4 — config schema
- Reference: existing `relay/internal/auth/middleware.go` (Phase 1; we replace the 501 branch)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/config/config.go` | Modify | Extend `AuthModes` with `Email`/`SSO`; add `EmailConfig`, `SSOConfig`, `SSOClaims`; add `Email`+`SSO` to `AuthConfig`; defaults in `applyDefaults` |
| `relay/internal/config/config_test.go` | Modify | Tests for new config defaults + parse |
| `relay/internal/config/testdata/sample.yaml` | Modify | Add `auth.email` + `auth.sso` blocks |
| `relay/internal/auth/types.go` | Create | `SSOClaims`, `EmailJWTVerifier`, `SSOVerifier` interfaces |
| `relay/internal/auth/middleware.go` | Modify | Wider `NewMiddleware` constructor; try-email-then-sso-then-anonymous dispatcher |
| `relay/internal/auth/middleware_test.go` | Modify | Update existing Phase-1 tests; add 8+ new dispatcher tests with stub verifiers |
| `relay/internal/server/dto.go` | Modify | Add `Auth *InitAuth` (with `Email *InitAuthEmail`) to `InitResponse` |
| `relay/internal/server/turn.go` | Modify | `initHandler` emits the new `auth_modes`/`auth` fields based on cfg |
| `relay/internal/server/server.go` | Read-only check | `New(cfg, deps)` already accepts `*config.Config` — initHandler will need cfg too |
| `relay/internal/server/deps.go` | Modify | Add `AuthCfg config.AuthConfig` field so initHandler can read modes (or pass cfg via server.New — pick the consistent style) |
| `relay/cmd/relay/main.go` | Modify | Update `auth.NewMiddleware(...)` call to the 3-arg form (pass nil for email + sso verifiers); populate `deps.AuthCfg = cfg.Auth` |

---

## Tasks

### Task 1: Extend `config.AuthConfig` with Email + SSO sub-structs and defaults

**Files:** Modify `relay/internal/config/config.go`, `relay/internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/config/config_test.go` (after the last existing test's closing `}`):

```go
func TestLoad_AppliesAuthEmailDefaults(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.Email.CodeTTL != "10m" {
		t.Errorf("default Email.CodeTTL = %q; want 10m", cfg.Auth.Email.CodeTTL)
	}
	if cfg.Auth.Email.JWTTTL != "15m" {
		t.Errorf("default Email.JWTTTL = %q; want 15m", cfg.Auth.Email.JWTTTL)
	}
}

func TestLoad_AppliesSSOClaimDefaults(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.SSO.Claims.UserID != "sub" {
		t.Errorf("default SSO.Claims.UserID = %q; want sub", cfg.Auth.SSO.Claims.UserID)
	}
	if cfg.Auth.SSO.Claims.Email != "email" {
		t.Errorf("default SSO.Claims.Email = %q; want email", cfg.Auth.SSO.Claims.Email)
	}
	if cfg.Auth.SSO.Claims.DisplayName != "name" {
		t.Errorf("default SSO.Claims.DisplayName = %q; want name", cfg.Auth.SSO.Claims.DisplayName)
	}
}

func TestLoad_AuthModesDefaultOnlyAnonymousTrue(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Phase 1 default: anonymous only. Phase 4 adds the two flags, both default false.
	if cfg.Auth.Modes.Email {
		t.Error("default AuthModes.Email = true; want false (opt-in)")
	}
	if cfg.Auth.Modes.SSO {
		t.Error("default AuthModes.SSO = true; want false (opt-in)")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```
cd C:/src/ai/intake/relay && go test ./internal/config/... -run "TestLoad_AppliesAuthEmailDefaults|TestLoad_AppliesSSOClaimDefaults|TestLoad_AuthModesDefaultOnlyAnonymousTrue" -v
```

Expected: compile errors (`cfg.Auth.Email`, `cfg.Auth.SSO`, `cfg.Auth.Modes.Email`, etc. undefined). MUST fail before proceeding.

- [ ] **Step 3: Add the new structs and fields in `config.go`**

In `relay/internal/config/config.go`, replace `AuthConfig` and `AuthModes` with:

```go
// AuthConfig selects which auth modes are enabled and configures email/SSO.
type AuthConfig struct {
	Modes AuthModes   `yaml:"modes"`
	Email EmailConfig `yaml:"email"`
	SSO   SSOConfig   `yaml:"sso"`
}

// AuthModes enables or disables specific auth strategies.
type AuthModes struct {
	Anonymous bool `yaml:"anonymous"`
	Email     bool `yaml:"email"`
	SSO       bool `yaml:"sso"`
}

// EmailConfig configures the email magic-link mode.
// All secrets reference an env var name; the value resolves via ResolveSecret in main.go.
type EmailConfig struct {
	SMTPHost     string `yaml:"smtp_host"`
	SMTPPort     int    `yaml:"smtp_port"`
	SMTPUser     string `yaml:"smtp_user"`
	SMTPPassEnv  string `yaml:"smtp_pass_env"`   // env var holding the SMTP password
	From         string `yaml:"from"`            // RFC 5322 From address
	CodeTTL      string `yaml:"code_ttl"`        // default "10m"
	JWTTTL       string `yaml:"jwt_ttl"`         // default "15m"
	JWTSecretEnv string `yaml:"jwt_secret_env"`  // env var; resolved value must be ≥32 bytes
}

// SSOConfig configures host-app SSO. Exactly one of JWKSURL (RS256) or
// HS256SecretEnv (HS256) must be set; both-set or neither-set is a startup error.
type SSOConfig struct {
	Issuer         string    `yaml:"issuer"`            // expected `iss` claim
	Audience       string    `yaml:"audience"`          // expected `aud` claim
	JWKSURL        string    `yaml:"jwks_url"`          // RS256 path
	HS256SecretEnv string    `yaml:"hs256_secret_env"`  // HS256 path; env var name
	Claims         SSOClaims `yaml:"claims"`
}

// SSOClaims maps SessionContext fields to JWT claim names.
// Defaults: sub/email/name (standard OIDC).
type SSOClaims struct {
	UserID      string `yaml:"user_id"`
	Email       string `yaml:"email"`
	DisplayName string `yaml:"display_name"`
}
```

- [ ] **Step 4: Extend `applyDefaults` with the new defaults**

In `relay/internal/config/config.go`, inside the `applyDefaults` function, before the Webhook retry defaults, add:

```go
	// Auth (4-i): email/SSO sub-structs gain sensible defaults.
	if c.Auth.Email.CodeTTL == "" {
		c.Auth.Email.CodeTTL = "10m"
	}
	if c.Auth.Email.JWTTTL == "" {
		c.Auth.Email.JWTTTL = "15m"
	}
	if c.Auth.SSO.Claims.UserID == "" {
		c.Auth.SSO.Claims.UserID = "sub"
	}
	if c.Auth.SSO.Claims.Email == "" {
		c.Auth.SSO.Claims.Email = "email"
	}
	if c.Auth.SSO.Claims.DisplayName == "" {
		c.Auth.SSO.Claims.DisplayName = "name"
	}
```

- [ ] **Step 5: Run the new tests + the full config suite**

```
cd C:/src/ai/intake/relay && go test ./internal/config/... -v
```

Expected: all config tests pass, including the three new tests.

- [ ] **Step 6: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. (Existing `main.go`/`middleware.go` will not need changes yet for the build — the new struct fields just exist unused.)

- [ ] **Step 7: Commit**

```
cd C:/src/ai/intake/relay && git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add auth.email/sso config blocks + AuthModes.Email/SSO flags (4-i)"
```

---

### Task 2: Update sample.yaml to round-trip the new blocks

**Files:** Modify `relay/internal/config/testdata/sample.yaml`

- [ ] **Step 1: Append the new auth.email + auth.sso blocks**

Read the current sample.yaml first:

```
cd C:/src/ai/intake/relay && type internal\config\testdata\sample.yaml
```

Find the existing `auth:` block and REPLACE just that block (keep everything else unchanged):

```yaml
auth:
  modes:
    anonymous: true
    email: true
    sso: true
  email:
    smtp_host: "smtp.example.com"
    smtp_port: 587
    smtp_user: "intake@example.com"
    smtp_pass_env: "INTAKE_SMTP_PASS"
    from: "Intake <intake@example.com>"
    code_ttl: "10m"
    jwt_ttl: "15m"
    jwt_secret_env: "INTAKE_EMAIL_JWT_SECRET"
  sso:
    issuer: "https://example.us.auth0.com/"
    audience: "https://api.example.com"
    jwks_url: "https://example.us.auth0.com/.well-known/jwks.json"
    hs256_secret_env: ""
    claims:
      user_id: "sub"
      email: "email"
      display_name: "name"
```

- [ ] **Step 2: Add round-trip assertions to `TestLoad_ParsesSampleYAML`**

In `relay/internal/config/config_test.go`, find `TestLoad_ParsesSampleYAML` and append before its closing `}`:

```go
	// Auth.Modes (4-i) — sample enables all three.
	if !cfg.Auth.Modes.Anonymous || !cfg.Auth.Modes.Email || !cfg.Auth.Modes.SSO {
		t.Errorf("AuthModes = %+v; want all three true", cfg.Auth.Modes)
	}
	// Auth.Email parsed (explicit values, not defaults).
	if cfg.Auth.Email.SMTPHost != "smtp.example.com" {
		t.Errorf("Email.SMTPHost = %q; want smtp.example.com", cfg.Auth.Email.SMTPHost)
	}
	if cfg.Auth.Email.SMTPPort != 587 {
		t.Errorf("Email.SMTPPort = %d; want 587", cfg.Auth.Email.SMTPPort)
	}
	if cfg.Auth.Email.SMTPPassEnv != "INTAKE_SMTP_PASS" {
		t.Errorf("Email.SMTPPassEnv = %q; want INTAKE_SMTP_PASS", cfg.Auth.Email.SMTPPassEnv)
	}
	if cfg.Auth.Email.JWTSecretEnv != "INTAKE_EMAIL_JWT_SECRET" {
		t.Errorf("Email.JWTSecretEnv = %q; want INTAKE_EMAIL_JWT_SECRET", cfg.Auth.Email.JWTSecretEnv)
	}
	// Auth.SSO parsed.
	if cfg.Auth.SSO.Issuer != "https://example.us.auth0.com/" {
		t.Errorf("SSO.Issuer = %q; mismatch", cfg.Auth.SSO.Issuer)
	}
	if cfg.Auth.SSO.JWKSURL != "https://example.us.auth0.com/.well-known/jwks.json" {
		t.Errorf("SSO.JWKSURL = %q; mismatch", cfg.Auth.SSO.JWKSURL)
	}
	if cfg.Auth.SSO.Claims.UserID != "sub" {
		t.Errorf("SSO.Claims.UserID = %q; want sub", cfg.Auth.SSO.Claims.UserID)
	}
```

- [ ] **Step 3: Run the config tests**

```
cd C:/src/ai/intake/relay && go test ./internal/config/... -v
```

Expected: all config tests pass.

- [ ] **Step 4: Commit**

```
cd C:/src/ai/intake/relay && git add internal/config/config_test.go internal/config/testdata/sample.yaml
git commit -m "test(config): sample.yaml round-trips auth.email/sso blocks (4-i)"
```

---

### Task 3: Add the verifier interfaces in `auth/types.go`

**Files:** Create `relay/internal/auth/types.go`

- [ ] **Step 1: Create `types.go`**

Create `relay/internal/auth/types.go`:

```go
package auth

import "context"

// SSOClaims is the per-request identity surface produced by an SSOVerifier.
// Maps onto SessionContext.{UserID, Email, DisplayName, Custom} when the SSO
// branch of the middleware dispatcher succeeds.
//
// UserID is always populated (claim name configurable via auth.sso.claims.user_id,
// default "sub"); Email and DisplayName are optional pointers (the configured
// claim may be absent in the token); Custom carries any additional claims the
// caller passed through (not used by the relay today; reserved for v1+).
type SSOClaims struct {
	UserID      string
	Email       *string
	DisplayName *string
	Custom      map[string]any
}

// EmailJWTVerifier verifies an email-mode JWT and returns the verified email
// from its sub claim. Implementations MUST reject tokens whose iss is not
// "intake-email" (so an SSO token can't sneak through the email branch), MUST
// reject expired tokens, and MUST NOT include the secret in any returned error.
//
// Implemented by *emailjwt.Verifier (sub-plan 4-ii).
type EmailJWTVerifier interface {
	Verify(token string) (email string, err error)
}

// SSOVerifier verifies a host-app-issued SSO JWT and returns the mapped claims.
// Implementations MUST pin the JWT algorithm (RS256 implementations reject HS256
// tokens and vice versa — mitigates alg-confusion attacks), MUST validate iss /
// aud / exp / nbf (with 30s clock-skew), and MUST NOT include the secret/JWKS
// content in any returned error.
//
// Implemented by *sso.RS256Verifier and *sso.HS256Verifier (sub-plan 4-iii).
type SSOVerifier interface {
	Verify(ctx context.Context, token string) (*SSOClaims, error)
}
```

- [ ] **Step 2: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. The interfaces are defined but not yet used; that's fine.

- [ ] **Step 3: Commit**

```
cd C:/src/ai/intake/relay && git add internal/auth/types.go
git commit -m "feat(auth): EmailJWTVerifier + SSOVerifier interfaces + SSOClaims (4-i)"
```

---

### Task 4: Update `auth.Middleware` to the try-email-then-sso-then-anonymous dispatcher

**Files:** Modify `relay/internal/auth/middleware.go`

- [ ] **Step 1: Replace the entire middleware.go file**

Replace `relay/internal/auth/middleware.go` with:

```go
package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Middleware is a chi-compatible HTTP middleware that resolves session identity.
//
// Resolution order (4-i):
//  1. Authorization: Bearer <token>:
//       a. If email mode is enabled (m.email != nil), try emailjwt.Verify; on
//          success, attach SessionContext{AuthMode:"email", Verified:true, Email}.
//       b. Else fall through to SSO; if sso mode is enabled (m.sso != nil), try
//          ssoVerifier.Verify; on success, attach SessionContext{AuthMode:"sso",
//          Verified:true, UserID, Email?, DisplayName?, Custom}.
//       c. Bearer present but no verifier accepted → 401 unauthorized.
//          (A present-but-invalid bearer is NEVER silently downgraded to anonymous.)
//  2. No Authorization header:
//       a. X-Intake-Session present + store.Validate → SessionContext{AuthMode:"anonymous"}.
//       b. Else → 401.
//
// The /init endpoint is NOT behind this middleware (it issues anonymous sessions).
// The /auth/email/start and /auth/email/verify endpoints are ALSO not behind this
// middleware (they bootstrap email JWTs — see sub-plan 4-ii).
type Middleware struct {
	store *Store
	email EmailJWTVerifier // nil → email mode off
	sso   SSOVerifier      // nil → sso mode off
}

// NewMiddleware returns a Middleware backed by the given Store. email and sso
// are optional; pass nil to disable the corresponding bearer-token validator.
func NewMiddleware(store *Store, email EmailJWTVerifier, sso SSOVerifier) *Middleware {
	return &Middleware{store: store, email: email, sso: sso}
}

// Store returns the underlying session store. Used by initHandler to issue
// anonymous sessions.
func (m *Middleware) Store() *Store { return m.store }

// Handler wraps next with identity resolution. chi-compatible.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authHeader := r.Header.Get("Authorization"); len(authHeader) >= 7 && strings.EqualFold(authHeader[:7], "bearer ") {
			token := strings.TrimSpace(authHeader[7:])

			// Try email-mode JWT first (cheap-fail HS256).
			if m.email != nil {
				if email, err := m.email.Verify(token); err == nil {
					emailCopy := email // distinct pointer per request
					ctx := WithSession(r.Context(), &SessionContext{
						AuthMode: "email",
						Verified: true,
						Email:    &emailCopy,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Fall through to SSO.
			if m.sso != nil {
				if claims, err := m.sso.Verify(r.Context(), token); err == nil {
					userID := claims.UserID
					sc := &SessionContext{
						AuthMode:    "sso",
						Verified:    true,
						UserID:      &userID,
						Email:       claims.Email,
						DisplayName: claims.DisplayName,
						Custom:      claims.Custom,
					}
					next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sc)))
					return
				}
			}

			// Bearer present but neither verifier accepted it.
			authWriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "invalid bearer token",
				},
			})
			return
		}

		// Anonymous fallback.
		sessionID := r.Header.Get("X-Intake-Session")
		if sessionID == "" || !m.store.Validate(sessionID) {
			authWriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "missing or invalid X-Intake-Session header; call POST /v1/intake/init first",
				},
			})
			return
		}

		ctx := WithSession(r.Context(), &SessionContext{
			SessionID: sessionID,
			AuthMode:  "anonymous",
			Verified:  false,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authWriteJSON writes a JSON-encoded body with the given status code.
// Named authWriteJSON to avoid conflict with server.writeJSON in the server package.
func authWriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
```

- [ ] **Step 2: Build (existing Phase-1 callers will break — fix in steps 3 + Task 6)**

```
cd C:/src/ai/intake/relay && go build ./... 2>&1 | head -20
```

Expected output includes errors like:
```
internal/auth/middleware_test.go:NN:NN: not enough arguments in call to NewMiddleware
cmd/relay/main.go:NN:NN: not enough arguments in call to auth.NewMiddleware
```

That's the intended state. We fix the test in step 3 + 4 and `main.go` in Task 6.

- [ ] **Step 3: Update existing Phase-1 tests to the new constructor signature**

In `relay/internal/auth/middleware_test.go`, replace every existing `auth.NewMiddleware(store)` call with `auth.NewMiddleware(store, nil, nil)`. Quick way: open the file, do a find-replace.

Then run only the existing tests to confirm Phase-1 behavior is preserved with both verifiers nil:

```
cd C:/src/ai/intake/relay && go test ./internal/auth/... -v
```

Expected: all existing tests pass (Phase 1 behavior unchanged when email/sso are nil).

- [ ] **Step 4: Add the new dispatcher tests**

In `relay/internal/auth/middleware_test.go`, ADD these helper types + tests (append after existing tests):

```go
// --- stubs for the new dispatcher tests (4-i) ---

type stubEmailVerifier struct {
	wantToken string
	email     string
	err       error
}

func (s *stubEmailVerifier) Verify(token string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.wantToken != "" && token != s.wantToken {
		return "", errors.New("stub: token mismatch")
	}
	return s.email, nil
}

type stubSSOVerifier struct {
	wantToken string
	claims    *auth.SSOClaims
	err       error
}

func (s *stubSSOVerifier) Verify(_ context.Context, token string) (*auth.SSOClaims, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.wantToken != "" && token != s.wantToken {
		return nil, errors.New("stub: token mismatch")
	}
	return s.claims, nil
}

// alwaysFail satisfies both interfaces and always returns an error.
type alwaysFail struct{}

func (alwaysFail) Verify(string) (string, error)                                  { return "", errors.New("stub: always fails") }
func (alwaysFail) VerifyCtx(context.Context, string) (*auth.SSOClaims, error)     { return nil, errors.New("stub: always fails") }

type alwaysFailSSO struct{}

func (alwaysFailSSO) Verify(context.Context, string) (*auth.SSOClaims, error) {
	return nil, errors.New("stub: always fails")
}

// runRequest is a small helper that runs an http.Request through a middleware
// and a no-op `next` handler that captures the resolved SessionContext.
func runRequest(t *testing.T, mw *auth.Middleware, r *http.Request) (status int, sess *auth.SessionContext) {
	t.Helper()
	rr := httptest.NewRecorder()
	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _ = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, r)
	return rr.Code, sess
}

// --- New dispatcher tests (4-i) ---

func TestDispatcher_EmailModeOnly_ValidToken(t *testing.T) {
	store := auth.NewStore()
	email := &stubEmailVerifier{wantToken: "valid-token", email: "user@example.com"}
	mw := auth.NewMiddleware(store, email, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer valid-token")

	status, sess := runRequest(t, mw, r)

	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	if sess == nil {
		t.Fatal("session not attached")
	}
	if sess.AuthMode != "email" || !sess.Verified {
		t.Errorf("session = %+v; want AuthMode=email Verified=true", sess)
	}
	if sess.Email == nil || *sess.Email != "user@example.com" {
		t.Errorf("session.Email = %v; want user@example.com", sess.Email)
	}
}

func TestDispatcher_EmailModeOnly_InvalidToken_401(t *testing.T) {
	store := auth.NewStore()
	email := alwaysFail{}
	mw := auth.NewMiddleware(store, email, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer bogus")

	status, sess := runRequest(t, mw, r)

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", status)
	}
	if sess != nil {
		t.Errorf("session should not be attached on failure; got %+v", sess)
	}
}

func TestDispatcher_SSOModeOnly_ValidToken(t *testing.T) {
	store := auth.NewStore()
	email := "user@sso.example"
	name := "Alice User"
	sso := &stubSSOVerifier{
		wantToken: "valid-sso",
		claims: &auth.SSOClaims{
			UserID:      "auth0|abc123",
			Email:       &email,
			DisplayName: &name,
		},
	}
	mw := auth.NewMiddleware(store, nil, sso)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer valid-sso")

	status, sess := runRequest(t, mw, r)

	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	if sess.AuthMode != "sso" || !sess.Verified {
		t.Errorf("session = %+v; want AuthMode=sso Verified=true", sess)
	}
	if sess.UserID == nil || *sess.UserID != "auth0|abc123" {
		t.Errorf("session.UserID = %v; want auth0|abc123", sess.UserID)
	}
	if sess.Email == nil || *sess.Email != "user@sso.example" {
		t.Errorf("session.Email = %v", sess.Email)
	}
}

func TestDispatcher_BothModes_EmailWinsWhenValid(t *testing.T) {
	store := auth.NewStore()
	email := &stubEmailVerifier{email: "user@example.com"}
	sso := alwaysFailSSO{}
	mw := auth.NewMiddleware(store, email, sso)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer any")

	_, sess := runRequest(t, mw, r)

	if sess == nil || sess.AuthMode != "email" {
		t.Errorf("session.AuthMode = %v; want email (email tried first)", sess)
	}
}

func TestDispatcher_BothModes_SSOReachedWhenEmailFails(t *testing.T) {
	store := auth.NewStore()
	email := alwaysFail{}
	sub := "user-from-sso"
	sso := &stubSSOVerifier{claims: &auth.SSOClaims{UserID: sub}}
	mw := auth.NewMiddleware(store, email, sso)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer any")

	_, sess := runRequest(t, mw, r)

	if sess == nil || sess.AuthMode != "sso" {
		t.Errorf("session.AuthMode = %v; want sso (fall-through)", sess)
	}
	if sess.UserID == nil || *sess.UserID != sub {
		t.Errorf("session.UserID = %v; want %s", sess.UserID, sub)
	}
}

func TestDispatcher_BothModes_BothFail_401(t *testing.T) {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store, alwaysFail{}, alwaysFailSSO{})

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer bogus")

	status, _ := runRequest(t, mw, r)

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", status)
	}
}

func TestDispatcher_NoModes_BearerPresent_401(t *testing.T) {
	// Regression: even if both modes are off, a bearer must NOT silently downgrade
	// to anonymous. Phase 1 returned 501 here; 4-i changes that to 401.
	store := auth.NewStore()
	store.Issue() // session exists but bearer should still 401
	mw := auth.NewMiddleware(store, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer something")

	status, _ := runRequest(t, mw, r)

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401 (bearer must not silently downgrade)", status)
	}
}

func TestDispatcher_AnonymousFallthrough_Preserved(t *testing.T) {
	// Phase 1 behavior: no Authorization header + valid X-Intake-Session = anonymous.
	store := auth.NewStore()
	sid := store.Issue()
	mw := auth.NewMiddleware(store, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("X-Intake-Session", sid)

	status, sess := runRequest(t, mw, r)

	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	if sess.AuthMode != "anonymous" || sess.Verified {
		t.Errorf("session = %+v; want AuthMode=anonymous Verified=false", sess)
	}
	if sess.SessionID != sid {
		t.Errorf("session.SessionID = %q; want %q", sess.SessionID, sid)
	}
}
```

Make sure the imports at the top of `middleware_test.go` include:

```go
import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"intake/internal/auth"
)
```

- [ ] **Step 5: Run the full auth suite**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/... -v
```

Expected: every test passes — the original Phase 1 tests AND the 8 new dispatcher tests.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "feat(auth): middleware dispatcher — try email, then sso, then anonymous (4-i)"
```

---

### Task 5: Extend `InitResponse` with the new auth advertisement

**Files:** Modify `relay/internal/server/dto.go`, `relay/internal/server/turn.go`, `relay/internal/server/deps.go`, `relay/internal/server/server.go`

- [ ] **Step 1: Add the new DTO types**

In `relay/internal/server/dto.go`, replace the existing `InitResponse` + `Capabilities` block with:

```go
// InitResponse is returned by POST /v1/intake/init.
//
// Phase 1: SessionID + Capabilities{AuthModes:["anonymous"], Streaming:true}.
// Phase 4: Capabilities.AuthModes is extended to include "email"/"sso" when the
// corresponding auth.modes.* flag is true; a new top-level Auth field carries
// per-mode hints (currently just email.code_ttl_seconds).
type InitResponse struct {
	SessionID    string       `json:"session_id"`
	Capabilities Capabilities `json:"capabilities"`
	Auth         *InitAuth    `json:"auth,omitempty"`
}

// Capabilities advertises relay feature flags to the widget.
type Capabilities struct {
	AuthModes []string `json:"auth_modes"`
	Streaming bool     `json:"streaming"`
}

// InitAuth carries per-mode initialization hints. Only emitted when at least
// one enabled mode advertises a hint.
type InitAuth struct {
	Email *InitAuthEmail `json:"email,omitempty"`
}

// InitAuthEmail is the email-mode capability hint.
type InitAuthEmail struct {
	CodeTTLSeconds int `json:"code_ttl_seconds"`
}
```

- [ ] **Step 2: Add `AuthCfg` to `Deps` so `initHandler` can read modes**

In `relay/internal/server/deps.go`, add the import `"intake/internal/config"` if not already present, and add a field on `Deps`:

```go
	// AuthCfg is the auth section of the loaded config — needed by initHandler
	// to emit the correct capabilities + auth hints. Set by main.go.
	AuthCfg config.AuthConfig
```

- [ ] **Step 3: Update `initHandler` to emit the new fields**

In `relay/internal/server/turn.go`, replace the `initHandler` function with:

```go
// initHandler handles POST /v1/intake/init.
// Response: InitResponse{SessionID, Capabilities, Auth?}.
//
// Phase 4: AuthModes includes "anonymous"/"email"/"sso" based on cfg.Auth.Modes;
// InitResponse.Auth carries hints (currently just email.code_ttl_seconds).
func initHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil {
			writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
			return
		}
		sessionID := deps.Auth.Store().Issue()

		modes := make([]string, 0, 3)
		if deps.AuthCfg.Modes.Anonymous {
			modes = append(modes, "anonymous")
		}
		if deps.AuthCfg.Modes.Email {
			modes = append(modes, "email")
		}
		if deps.AuthCfg.Modes.SSO {
			modes = append(modes, "sso")
		}
		// Backward compat: if no flag was set (somehow), preserve Phase-1 default.
		if len(modes) == 0 {
			modes = []string{"anonymous"}
		}

		resp := InitResponse{
			SessionID: sessionID,
			Capabilities: Capabilities{
				AuthModes: modes,
				Streaming: true,
			},
		}

		// Per-mode hints: email's code_ttl_seconds (parsed from cfg).
		if deps.AuthCfg.Modes.Email {
			if d, err := time.ParseDuration(deps.AuthCfg.Email.CodeTTL); err == nil {
				resp.Auth = &InitAuth{Email: &InitAuthEmail{CodeTTLSeconds: int(d.Seconds())}}
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
```

Add `"time"` to the imports at the top of `turn.go` if not already present.

- [ ] **Step 4: Update or add an init handler test**

In `relay/internal/server/turn_test.go` (or create a new `init_test.go` if turn_test.go is unwieldy), add a test that asserts:

```go
func TestInitHandler_EmitsAllEnabledModes(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}},
		Auth: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true, Email: true, SSO: true},
			Email: config.EmailConfig{CodeTTL: "10m"},
		},
	}
	deps := server.Deps{
		Auth:    auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: cfg.Auth,
	}
	mux := server.New(cfg, deps)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/init", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}

	var resp server.InitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"anonymous", "email", "sso"}
	if got := resp.Capabilities.AuthModes; !equalStringSlice(got, want) {
		t.Errorf("AuthModes = %v; want %v", got, want)
	}
	if resp.Auth == nil || resp.Auth.Email == nil || resp.Auth.Email.CodeTTLSeconds != 600 {
		t.Errorf("Auth.Email = %+v; want CodeTTLSeconds=600", resp.Auth)
	}
}

// equalStringSlice — order-sensitive equality for the test above.
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Make sure the test file imports `"encoding/json"`, `"net/http"`, `"net/http/httptest"`, `"testing"`, `"intake/internal/auth"`, `"intake/internal/config"`, `"intake/internal/server"`.

- [ ] **Step 5: Run the server tests**

```
cd C:/src/ai/intake/relay && go test ./internal/server/... -v
```

Expected: all server tests pass, including the new `TestInitHandler_EmitsAllEnabledModes`. NOTE: the existing `TestSubmitHandler_*` tests construct a Deps without `AuthCfg` — if those tests touch `/init` they'll also need `AuthCfg` set, but the submit tests issue sessions via `deps.Auth.Store().Issue()` directly so they're unaffected.

If any existing test breaks because it now calls /init through `server.New(cfg, deps)` without an `AuthCfg`, fix it: add `AuthCfg: config.AuthConfig{Modes: config.AuthModes{Anonymous: true}}` to its Deps literal.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/server/dto.go internal/server/turn.go internal/server/deps.go internal/server/turn_test.go
git commit -m "feat(server): init response advertises auth modes + email code_ttl hint (4-i)"
```

---

### Task 6: Update `main.go` to the new `auth.NewMiddleware` constructor

**Files:** Modify `relay/cmd/relay/main.go`

- [ ] **Step 1: Confirm baseline build state (currently failing per Task 4 step 2)**

```
cd C:/src/ai/intake/relay && go build ./cmd/relay/... 2>&1 | head -5
```

Expected: an error about `not enough arguments in call to auth.NewMiddleware`. That's what we're fixing.

- [ ] **Step 2: Update the `auth.NewMiddleware` call site**

In `relay/cmd/relay/main.go`, find the `middleware := auth.NewMiddleware(store)` line and replace it with:

```go
	// 4-i: middleware accepts optional email + sso verifiers (both nil here; 4-ii
	// and 4-iii wire them when the corresponding auth.modes.* flag is set).
	middleware := auth.NewMiddleware(store, nil, nil)
```

- [ ] **Step 3: Populate `Deps.AuthCfg`**

In the `deps := server.Deps{...}` literal in main.go, add the new field:

```go
		AuthCfg: cfg.Auth,
```

- [ ] **Step 4: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 5: Full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK` — every package green (config, auth, server, providers, anthropic, ollama, openai, gemini, license, internal/license, router, all four adapters, payloadbuild).

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "refactor(main): wire 3-arg auth.NewMiddleware + Deps.AuthCfg (4-i)"
```

---

### Task 7: Final verification gate

- [ ] **Step 1: Full build + vet + test**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected: `BUILD_OK`, `VET_OK`, every package `ok`, `TEST_OK`.

- [ ] **Step 2: Contract + pins**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK`.

- [ ] **Step 3: Confirm no new dependency**

```
cd C:/src/ai/intake/relay && go mod tidy && git diff --exit-code go.mod go.sum && echo MOD_CLEAN
```

Expected: `MOD_CLEAN` (4-i adds no Go dependency — that's 4-ii and 4-iii). If `go.mod`/`go.sum` change, remove the offending import.

- [ ] **Step 4: Build-fail self-check (README §6)**

- Dispatcher rejects an invalid bearer with 401 (does NOT downgrade to anonymous) → `TestDispatcher_NoModes_BearerPresent_401`. ✓
- Anonymous flow unchanged when both verifiers are nil → `TestDispatcher_AnonymousFallthrough_Preserved`. ✓
- Email-mode-first ordering → `TestDispatcher_BothModes_EmailWinsWhenValid`. ✓
- No new external dep → step 3. ✓
- `auth.SessionContext` shape unchanged (Phase 1 frozen seam). ✓

---

## Smoke

**Credit-free (unit):** `go test ./internal/auth/... ./internal/config/... ./internal/server/...` all green — proves the dispatcher's branching, the config defaults + sample.yaml round-trip, and the init response carries the new fields.

**Live re-confirmation (deferred to phase final smoke 4-iv):** anonymous /init → /turn flow still works against a real relay boot. Email and SSO live behaviors are deferred to 4-ii/4-iii implementations + 4-iv's live smoke.

## Done Criteria

1. `go build ./... && go vet ./...` clean in `relay/`.
2. `go test ./...` green, including the new dispatcher tests and the init-response test, with no real keys or tokens.
3. `auth.Middleware` accepts both verifiers as optional (nil-OK) constructor args; dispatcher tries email→sso→anonymous; a present-but-invalid bearer returns 401, NEVER silently downgrades to anonymous.
4. `AuthConfig` carries `Email EmailConfig` + `SSO SSOConfig` + `AuthModes.{Email,SSO}` with defaults `CodeTTL=10m`, `JWTTTL=15m`, `Claims.{UserID="sub",Email="email",DisplayName="name"}`.
5. `InitResponse` extends `Capabilities.AuthModes` based on enabled modes AND emits `Auth.Email.CodeTTLSeconds` when email mode is enabled.
6. `bash scripts/verify-contract.sh` and `bash scripts/check-pins.sh` green; `go mod tidy` leaves go.mod/go.sum unchanged (4-i is stdlib-only).
7. `Deps.AuthCfg` exists; `main.go` populates it from `cfg.Auth`.
