# 4-iii Host-App SSO Verifier — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the `relay/internal/auth/sso/` package — a `Verifier` interface with two implementations (`RS256Verifier` backed by `MicahParks/keyfunc/v3` for JWKS; `HS256Verifier` backed by a resolved shared secret) and a `New` factory that selects exactly one impl at startup. Both impls **pin the JWT algorithm** to mitigate alg-confusion attacks, validate `iss` (exact match) / `aud` (string-or-array) / `exp` / `nbf` with a 30s clock skew, and map the configured claim names into `*auth.SSOClaims`. `main.go` wires the verifier into `auth.NewMiddleware(...)` when `cfg.Auth.Modes.SSO == true`. After this sub-plan: a host-app-issued SSO JWT presented as `Authorization: Bearer ...` resolves into a verified `SessionContext{AuthMode:"sso"}`.

**Architecture:** The package exposes `Verifier interface { Verify(ctx, token) (*auth.SSOClaims, error) }` — this is the concrete type that satisfies the `auth.SSOVerifier` interface frozen by 4-i. Both impls share a private `validateAndExtract(parsedToken, cfg) (*auth.SSOClaims, error)` helper that does iss/aud/exp/nbf checks and claim mapping; they differ only in how the signing key is supplied to `jwt.ParseWithClaims`. `NewRS256Verifier` fetches the JWKS at construction time (via `keyfunc.NewDefault`) — a network failure is fatal at startup, not deferred to first request. `NewHS256Verifier` rejects secrets shorter than 32 bytes. The `New` factory enforces mutual exclusivity: exactly one of `cfg.JWKSURL` or a non-nil HS256 secret must be supplied; both-set or neither-set is a startup error. `main.go` resolves the HS256 secret (if configured) via `config.RequireSecret` BEFORE calling the factory — the verifier package never touches `os.Getenv`.

**Tech Stack:** Go 1.23.2 (relay). Two external dependencies:
- `github.com/MicahParks/keyfunc/v3` — JWKS fetch + cache + refresh-on-miss. New in Phase 4. **Exact-pinned** in `go.mod`.
- `github.com/golang-jwt/jwt/v5` — JWT parse/verify with `WithValidMethods` algorithm pinning. **May or may not already be in `go.mod`** from sub-plan 4-ii; re-running `go get` is idempotent. Either way, this plan ensures it is exact-pinned.

**Ordering note:** This plan is sequenced AFTER 4-ii in the controller's execution order (4-i → 4-ii → 4-iii → 4-iv). Plan authoring is independent of execution order, however; this plan assumes 4-i is in place (the `auth.SSOVerifier` interface and the middleware dispatcher exist) and treats `golang-jwt/jwt/v5` as **may or may not** already be in `go.mod` — Task 1 handles either case (the `go get` is idempotent; the `check-pins.sh` patch is conditional). It does NOT assume 4-ii has landed.

---

## Design References

- README §8.2 — `SSOConfig` / `SSOClaims` (frozen in 4-i)
- README §8.4 — sso package shape (frozen here)
- README §6 — build-fail items: alg-confusion accepted, JWKS-unreachable starts the relay, HS256 secret <32 bytes accepted, both/neither set accepted
- README §7 — final smoke (live SSO is deferred to 4-iv — needs a real IdP token or self-served JWKS)
- Design spec §2.3 — Verifier interface + two impls + factory
- Design spec §5 — SSO JWT shape: iss exact, aud-may-be-string-or-array, exp/nbf with 30s skew, alg pinning
- Design spec §8 — build-fail checklist
- Design spec §9 — testing strategy (in-test RSA keypair + httptest JWKS, the alg-confusion test as the load-bearing security property)
- Design spec §11 — non-goals (no OAuth code flow, no social SSO)
- Reference: 4-i's `relay/internal/auth/types.go` (`SSOClaims`, `SSOVerifier` interface)
- Reference: `relay/internal/config/secret.go` (`config.RequireSecret`)
- Reference: 4-i's call site in `main.go` (`auth.NewMiddleware(store, nil, nil)`) — this plan converts the third arg from `nil` to the configured verifier

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/go.mod` / `relay/go.sum` | Modify | Add (or re-pin) `github.com/MicahParks/keyfunc/v3` and `github.com/golang-jwt/jwt/v5` — exact versions, no carets |
| `scripts/check-pins.sh` | Modify | Gate against caret/`@latest` for `keyfunc/v3` and (if not already added by 4-ii) `golang-jwt/jwt/v5` |
| `relay/internal/auth/sso/sso.go` | Create | `Verifier` interface, shared `validateAndExtract` helper, `New` factory |
| `relay/internal/auth/sso/rs256.go` | Create | `RS256Verifier` + `NewRS256Verifier` (uses `keyfunc.NewDefault`) |
| `relay/internal/auth/sso/hs256.go` | Create | `HS256Verifier` + `NewHS256Verifier` |
| `relay/internal/auth/sso/sso_test.go` | Create | Factory mutual-exclusivity + selection tests |
| `relay/internal/auth/sso/rs256_test.go` | Create | RS256 happy path + 8 rejection tests (tampered, wrong-kid, expired, wrong-iss, wrong-aud, alg-confusion, claim mapping, nbf-future) |
| `relay/internal/auth/sso/hs256_test.go` | Create | HS256 happy path + same rejection matrix + secret-too-short + alg-confusion |
| `relay/cmd/relay/main.go` | Modify | Resolve HS256 secret (if configured), call `sso.New`, pass returned verifier to `auth.NewMiddleware` (third arg) |

---

## Tasks

### Task 1: Pin the JWT + JWKS dependencies (exact, no caret)

**Files:** Modify `relay/go.mod`, `relay/go.sum`, `scripts/check-pins.sh`

- [ ] **Step 1: Snapshot the current go.mod state**

```
cd C:/src/ai/intake/relay && type go.mod
```

Note whether `github.com/golang-jwt/jwt/v5` already appears (4-ii may have added it). Note whether `github.com/MicahParks/keyfunc/v3` already appears (should NOT — this plan introduces it).

- [ ] **Step 2: Add (or re-pin) keyfunc/v3 — exact version**

```
cd C:/src/ai/intake/relay && go get github.com/MicahParks/keyfunc/v3
```

Record the version Go resolved (e.g. `v3.3.5`). Open `relay/go.mod` and verify the line reads `github.com/MicahParks/keyfunc/v3 v3.X.Y` (NO caret, NO `@latest`). If the line carries any prefix other than the bare version, edit it to the exact version.

- [ ] **Step 3: Idempotently ensure golang-jwt/jwt/v5 is pinned**

```
cd C:/src/ai/intake/relay && go get github.com/golang-jwt/jwt/v5
```

(If 4-ii has already added it, this is a no-op or a version-bump; if not, it adds it now.) Verify `relay/go.mod` shows `github.com/golang-jwt/jwt/v5 v5.X.Y` with no caret.

- [ ] **Step 4: Tidy + confirm**

```
cd C:/src/ai/intake/relay && go mod tidy && go build ./... && echo BUILD_OK
```

Expected: `BUILD_OK`. The modules are downloaded; nothing else changed yet.

- [ ] **Step 5: Extend scripts/check-pins.sh**

Open `scripts/check-pins.sh`. Find the existing block of `# Gate: ... must be exact-pinned` checks (the ones for anthropic-sdk-go, openai-go, genai). After the `genai` gate (the one ending `fail=1; fi`) and BEFORE the line `# Gate: no go install/get ...@latest in install scripts`, insert:

```bash
# Gate: github.com/MicahParks/keyfunc/v3 must be exact-pinned (no caret, no @latest) in go.mod.
if grep -E 'MicahParks/keyfunc/v3' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/MicahParks/keyfunc/v3 is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: github.com/golang-jwt/jwt/v5 must be exact-pinned (no caret, no @latest) in go.mod.
if grep -E 'golang-jwt/jwt/v5' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/golang-jwt/jwt/v5 is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
```

> If 4-ii already added the `golang-jwt/jwt/v5` gate, the second block is a no-op. The gate text matches exactly, so duplicating it is a clean no-op (`grep` runs twice; both produce no output unless a caret is present). If you prefer, scan the file first and add only the keyfunc block — both approaches are correct.

- [ ] **Step 6: Confirm the pin gate runs**

```
cd C:/src/ai/intake && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `PINS_OK`.

- [ ] **Step 7: Commit**

```
cd C:/src/ai/intake && git add relay/go.mod relay/go.sum scripts/check-pins.sh
git commit -m "build(sso): pin keyfunc/v3 + jwt/v5 exact; extend check-pins gate (4-iii)"
```

---

### Task 2: Create the package skeleton — Verifier interface + factory (TDD)

**Files:** Create `relay/internal/auth/sso/sso.go`, `relay/internal/auth/sso/sso_test.go`

- [ ] **Step 1: Write the failing factory test file**

Create `relay/internal/auth/sso/sso_test.go`:

```go
package sso_test

import (
	"log/slog"
	"strings"
	"testing"

	"intake/internal/auth/sso"
	"intake/internal/config"
)

// silentLogger returns a logger that swallows output (so test logs stay clean).
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discard{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestNew_BothSet_Errors(t *testing.T) {
	cfg := config.SSOConfig{
		Issuer:         "https://issuer.example/",
		Audience:       "https://api.example",
		JWKSURL:        "https://issuer.example/.well-known/jwks.json",
		HS256SecretEnv: "INTAKE_SSO_HS256_SECRET",
		Claims:         config.SSOClaims{UserID: "sub", Email: "email", DisplayName: "name"},
	}
	secret := []byte("0123456789abcdef0123456789abcdef")
	_, err := sso.New(cfg, secret, silentLogger())
	if err == nil {
		t.Fatal("expected error when both JWKSURL and HS256 secret are supplied")
	}
	if !strings.Contains(err.Error(), "jwks_url") || !strings.Contains(err.Error(), "hs256") {
		t.Errorf("error should name both fields; got %v", err)
	}
}

func TestNew_NeitherSet_Errors(t *testing.T) {
	cfg := config.SSOConfig{
		Issuer:   "https://issuer.example/",
		Audience: "https://api.example",
		Claims:   config.SSOClaims{UserID: "sub", Email: "email", DisplayName: "name"},
	}
	_, err := sso.New(cfg, nil, silentLogger())
	if err == nil {
		t.Fatal("expected error when neither JWKSURL nor HS256 secret are supplied")
	}
	if !strings.Contains(err.Error(), "jwks_url") || !strings.Contains(err.Error(), "hs256") {
		t.Errorf("error should mention both possible fields; got %v", err)
	}
}

func TestNew_HS256SecretTooShort_Errors(t *testing.T) {
	cfg := config.SSOConfig{
		Issuer:         "https://issuer.example/",
		Audience:       "https://api.example",
		HS256SecretEnv: "INTAKE_SSO_HS256_SECRET",
		Claims:         config.SSOClaims{UserID: "sub", Email: "email", DisplayName: "name"},
	}
	short := []byte("too-short")
	_, err := sso.New(cfg, short, silentLogger())
	if err == nil {
		t.Fatal("expected error for HS256 secret shorter than 32 bytes")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("error should mention the 32-byte minimum; got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/sso/... -v
```

Expected: `no required module provides package intake/internal/auth/sso` (or build error: package does not exist). MUST fail before proceeding.

- [ ] **Step 3: Create the package skeleton `sso.go`**

Create `relay/internal/auth/sso/sso.go`:

```go
// Package sso implements host-app SSO JWT verification for the relay's auth
// middleware. Two implementations satisfy the same Verifier interface:
//
//   - RS256Verifier — fetches+caches a JWKS via MicahParks/keyfunc/v3 and
//     validates RS256 tokens against the host's published public keys.
//   - HS256Verifier — validates HS256 tokens against a shared secret resolved
//     from the environment (≥32 bytes; main.go pre-validates and passes it in).
//
// Both impls PIN the JWT algorithm via jwt.WithValidMethods — an RS256
// verifier rejects HS256 tokens and vice versa. This mitigates alg-confusion
// attacks (an attacker cannot pass an HS256 token signed with the JWKS pubkey
// bytes as the HMAC key, nor an RS256 token to the HS256 path).
//
// Both impls validate iss (exact match — NOT prefix), aud (RFC 7519 string or
// array of strings), exp (with 30s clock skew), and nbf (with 30s clock skew,
// if present). Claim names are read from cfg.Claims (defaults sub/email/name
// applied by config.applyDefaults in 4-i).
//
// SECURITY: error strings NEVER include the HS256 secret bytes or the JWKS
// content. The underlying golang-jwt v5 errors ("signature is invalid",
// "token has invalid claims") are safe to surface; we wrap them with static
// context only.
package sso

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-jwt/jwt/v5"

	"intake/internal/auth"
	"intake/internal/config"
)

// Verifier verifies an SSO JWT and returns the mapped claims. It is the
// concrete type that satisfies auth.SSOVerifier (the interface frozen in 4-i).
type Verifier interface {
	Verify(ctx context.Context, token string) (*auth.SSOClaims, error)
}

// clockSkew is the tolerance applied to exp and nbf checks. Locked at 30s per
// design spec §5 and README §8.4.
const clockSkew = 30 // seconds

// New constructs the configured verifier. Exactly one of cfg.JWKSURL or a
// non-nil hs256Secret must be supplied; both-set or neither-set is a startup
// config error (the relay must fatal — README §6).
//
// hs256Secret is the RESOLVED secret value, passed in by main.go via
// config.RequireSecret. The verifier never reads the environment itself.
//
// For RS256, the JWKS is fetched at construction time — a network failure at
// startup is fatal (NOT deferred to the first user request).
func New(cfg config.SSOConfig, hs256Secret []byte, logger *slog.Logger) (Verifier, error) {
	hasJWKS := cfg.JWKSURL != ""
	hasHS256 := len(hs256Secret) > 0

	if hasJWKS && hasHS256 {
		return nil, errors.New("sso: cfg.jwks_url and cfg.hs256_secret_env are mutually exclusive; set exactly one")
	}
	if !hasJWKS && !hasHS256 {
		return nil, errors.New("sso: one of cfg.jwks_url (RS256) or cfg.hs256_secret_env (HS256) must be set")
	}

	if hasJWKS {
		return NewRS256Verifier(cfg, logger)
	}
	return NewHS256Verifier(cfg, hs256Secret)
}

// validateAndExtract performs the shared post-parse validation: iss exact
// match, aud contains cfg.Audience, exp/nbf within 30s skew, sub (or
// cfg.Claims.UserID) non-empty. It returns a populated *auth.SSOClaims.
//
// The parsed token MUST have already been signature-verified by the caller
// (i.e., jwt.ParseWithClaims returned token.Valid == true). This helper does
// NOT re-verify signatures.
//
// known claim names (after applyDefaults from 4-i): cfg.Claims.UserID, .Email,
// .DisplayName. All other claims (minus standard JWT claims iss/aud/exp/nbf/
// iat/jti) are copied into Custom.
func validateAndExtract(claims jwt.MapClaims, cfg config.SSOConfig) (*auth.SSOClaims, error) {
	// iss — exact match (NOT prefix). An attacker who controls a subdomain
	// must not be able to mint tokens accepted by a parent-domain iss check.
	iss, _ := claims["iss"].(string)
	if iss != cfg.Issuer {
		return nil, fmt.Errorf("sso: iss claim %q does not match configured issuer", iss)
	}

	// aud — RFC 7519 allows a string or an array of strings.
	if !audienceContains(claims["aud"], cfg.Audience) {
		return nil, errors.New("sso: aud claim does not contain configured audience")
	}

	// exp/nbf are already checked by golang-jwt with its own skew, but the
	// spec mandates a 30s skew explicitly. We re-check with our skew to ensure
	// the relay's policy holds regardless of golang-jwt's defaults.
	if err := checkExpNbf(claims); err != nil {
		return nil, err
	}

	// Pull the configured user_id claim (default "sub").
	userIDKey := cfg.Claims.UserID
	if userIDKey == "" {
		userIDKey = "sub"
	}
	userID, _ := claims[userIDKey].(string)
	if userID == "" {
		return nil, fmt.Errorf("sso: required claim %q is missing or empty", userIDKey)
	}

	out := &auth.SSOClaims{UserID: userID}

	// Optional email.
	emailKey := cfg.Claims.Email
	if emailKey == "" {
		emailKey = "email"
	}
	if v, ok := claims[emailKey].(string); ok && v != "" {
		ec := v
		out.Email = &ec
	}

	// Optional display name.
	nameKey := cfg.Claims.DisplayName
	if nameKey == "" {
		nameKey = "name"
	}
	if v, ok := claims[nameKey].(string); ok && v != "" {
		nc := v
		out.DisplayName = &nc
	}

	// Custom: any claim not consumed above and not a standard JWT claim.
	consumed := map[string]bool{
		"iss": true, "aud": true, "exp": true, "nbf": true,
		"iat": true, "jti": true,
		userIDKey: true, emailKey: true, nameKey: true,
	}
	for k, v := range claims {
		if consumed[k] {
			continue
		}
		if out.Custom == nil {
			out.Custom = make(map[string]any)
		}
		out.Custom[k] = v
	}

	return out, nil
}

// audienceContains reports whether the aud claim (string or []any of strings,
// per RFC 7519) contains want.
func audienceContains(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if s == want {
				return true
			}
		}
	}
	return false
}

// checkExpNbf applies the 30s clock-skew check explicitly. golang-jwt v5 also
// validates exp/nbf during ParseWithClaims, but we re-check here to lock the
// skew at exactly 30s regardless of the library's defaults.
func checkExpNbf(claims jwt.MapClaims) error {
	now := jwt.NewNumericDate(nowFunc()).Unix()

	// exp must be > now - skew (i.e., not too far in the past).
	if expRaw, ok := claims["exp"]; ok {
		exp, err := numericClaim(expRaw)
		if err != nil {
			return fmt.Errorf("sso: invalid exp claim: %w", err)
		}
		if exp <= now-clockSkew {
			return errors.New("sso: token is expired")
		}
	}

	// nbf (if present) must be < now + skew (i.e., not too far in the future).
	if nbfRaw, ok := claims["nbf"]; ok {
		nbf, err := numericClaim(nbfRaw)
		if err != nil {
			return fmt.Errorf("sso: invalid nbf claim: %w", err)
		}
		if nbf >= now+clockSkew {
			return errors.New("sso: token is not yet valid")
		}
	}
	return nil
}

// numericClaim coerces a JWT numeric date claim (which json.Unmarshal decodes
// as float64) to an int64 seconds-since-epoch.
func numericClaim(v any) (int64, error) {
	switch x := v.(type) {
	case float64:
		return int64(x), nil
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case jwt.NumericDate:
		return x.Unix(), nil
	case *jwt.NumericDate:
		if x == nil {
			return 0, errors.New("nil NumericDate")
		}
		return x.Unix(), nil
	}
	return 0, fmt.Errorf("unexpected numeric type %T", v)
}
```

- [ ] **Step 4: Add the `nowFunc` injection point at the package level**

Append to `relay/internal/auth/sso/sso.go`:

```go
// nowFunc is the clock used by checkExpNbf. Production: time.Now. Tests
// override this to mint deterministic expired/future tokens.
var nowFunc = defaultNow
```

And add the required imports + helper at the top (replace the import block + add `defaultNow` below):

```go
import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"intake/internal/auth"
	"intake/internal/config"
)
```

Append at the BOTTOM of `sso.go`:

```go
func defaultNow() time.Time { return time.Now() }
```

- [ ] **Step 5: Stub out the two impl constructors so the factory test compiles**

Create `relay/internal/auth/sso/rs256.go`:

```go
package sso

import (
	"context"
	"errors"
	"log/slog"

	"intake/internal/auth"
	"intake/internal/config"
)

// RS256Verifier — full impl in Task 3.
type RS256Verifier struct{}

// NewRS256Verifier — full impl in Task 3.
func NewRS256Verifier(cfg config.SSOConfig, logger *slog.Logger) (*RS256Verifier, error) {
	return nil, errors.New("sso: RS256Verifier not yet implemented (Task 3)")
}

func (*RS256Verifier) Verify(ctx context.Context, token string) (*auth.SSOClaims, error) {
	return nil, errors.New("sso: RS256Verifier not yet implemented (Task 3)")
}
```

Create `relay/internal/auth/sso/hs256.go`:

```go
package sso

import (
	"context"
	"errors"
	"fmt"

	"intake/internal/auth"
	"intake/internal/config"
)

// HS256Verifier — full impl in Task 4.
type HS256Verifier struct{}

// NewHS256Verifier rejects secrets shorter than 32 bytes. Full Verify impl
// arrives in Task 4.
func NewHS256Verifier(cfg config.SSOConfig, secret []byte) (*HS256Verifier, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("sso: HS256 secret must be at least 32 bytes (got %d)", len(secret))
	}
	return nil, errors.New("sso: HS256Verifier not yet implemented (Task 4)")
}

func (*HS256Verifier) Verify(ctx context.Context, token string) (*auth.SSOClaims, error) {
	return nil, errors.New("sso: HS256Verifier not yet implemented (Task 4)")
}
```

- [ ] **Step 6: Run the factory tests**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/sso/... -v -run "TestNew_"
```

Expected: `TestNew_BothSet_Errors`, `TestNew_NeitherSet_Errors`, `TestNew_HS256SecretTooShort_Errors` all PASS. (The two impls aren't implemented yet but the factory's pre-checks fire before the constructors are called for the both/neither tests, and the HS256-too-short test trips the size check before the stub error.)

- [ ] **Step 7: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 8: Commit**

```
cd C:/src/ai/intake/relay && git add internal/auth/sso/sso.go internal/auth/sso/rs256.go internal/auth/sso/hs256.go internal/auth/sso/sso_test.go
git commit -m "feat(sso): Verifier interface + factory with mutual-exclusivity (4-iii)"
```

---

### Task 3: Implement RS256Verifier with JWKS fetch + alg pinning (TDD)

**Files:** Modify `relay/internal/auth/sso/rs256.go`, create `relay/internal/auth/sso/rs256_test.go`

- [ ] **Step 1: Write the failing RS256 test file**

Create `relay/internal/auth/sso/rs256_test.go`:

```go
package sso_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"intake/internal/auth/sso"
	"intake/internal/config"
)

// rsaKid is a deterministic kid used in all RS256 tests.
const rsaKid = "test-kid-001"

// rsaFixture bundles an in-test RSA keypair, a JWKS server that serves the
// matching public key, and a helper to mint signed tokens.
type rsaFixture struct {
	priv    *rsa.PrivateKey
	jwksURL string
	server  *httptest.Server
}

func newRSAFixture(t *testing.T) *rsaFixture {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	jwks := buildJWKS(t, priv, rsaKid)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	return &rsaFixture{priv: priv, jwksURL: srv.URL + "/.well-known/jwks.json", server: srv}
}

func (f *rsaFixture) close() { f.server.Close() }

// buildJWKS returns a JWKS JSON containing the RSA public key as the sole entry.
func buildJWKS(t *testing.T, priv *rsa.PrivateKey, kid string) []byte {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes())
	doc := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": kid,
				"n":   n,
				"e":   e,
			},
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return b
}

// mintRS256 signs claims with priv using the given kid in the header.
func mintRS256(t *testing.T, priv *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return s
}

// validClaims returns a fresh, valid claim set targeting the standard test
// issuer/audience.
func validClaims() jwt.MapClaims {
	now := time.Now().Unix()
	return jwt.MapClaims{
		"iss":   "https://issuer.test/",
		"aud":   "https://api.test",
		"exp":   now + 3600,
		"iat":   now,
		"nbf":   now - 5,
		"sub":   "user-abc-123",
		"email": "user@example.com",
		"name":  "Test User",
	}
}

// cfgFor returns an SSOConfig pointed at the fixture's JWKS URL.
func cfgFor(jwksURL string) config.SSOConfig {
	return config.SSOConfig{
		Issuer:   "https://issuer.test/",
		Audience: "https://api.test",
		JWKSURL:  jwksURL,
		Claims:   config.SSOClaims{UserID: "sub", Email: "email", DisplayName: "name"},
	}
}

func TestRS256_HappyPath(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()

	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}
	tok := mintRS256(t, f.priv, rsaKid, validClaims())

	claims, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "user-abc-123" {
		t.Errorf("UserID = %q; want user-abc-123", claims.UserID)
	}
	if claims.Email == nil || *claims.Email != "user@example.com" {
		t.Errorf("Email = %v; want user@example.com", claims.Email)
	}
	if claims.DisplayName == nil || *claims.DisplayName != "Test User" {
		t.Errorf("DisplayName = %v; want Test User", claims.DisplayName)
	}
}

func TestRS256_TamperedToken_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	tok := mintRS256(t, f.priv, rsaKid, validClaims())
	// Flip a byte in the payload section (between the two dots).
	tampered := []byte(tok)
	for i := range tampered {
		if tampered[i] == '.' {
			// Mutate the next byte if available.
			if i+1 < len(tampered) {
				if tampered[i+1] == 'A' {
					tampered[i+1] = 'B'
				} else {
					tampered[i+1] = 'A'
				}
				break
			}
		}
	}
	_, err = v.Verify(context.Background(), string(tampered))
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestRS256_WrongKid_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	// Mint a token signed by a DIFFERENT keypair with the same kid — JWKS won't
	// have a key whose public bytes match.
	otherPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := mintRS256(t, otherPriv, rsaKid, validClaims())
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for token signed with non-JWKS key")
	}
}

func TestRS256_Expired_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	// 1 hour ago — well past the 30s clock skew.
	claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	tok := mintRS256(t, f.priv, rsaKid, claims)
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestRS256_WrongIssuer_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	claims["iss"] = "https://attacker.test/"
	tok := mintRS256(t, f.priv, rsaKid, claims)
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestRS256_WrongAudience_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	claims["aud"] = "https://other-api.test"
	tok := mintRS256(t, f.priv, rsaKid, claims)
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for wrong audience")
	}
}

// TestRS256_AudienceAsArray exercises the RFC 7519 array-aud form. The token's
// aud is ["other", configured]; verification must succeed.
func TestRS256_AudienceAsArray(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	claims["aud"] = []string{"https://other-api.test", "https://api.test"}
	tok := mintRS256(t, f.priv, rsaKid, claims)
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Errorf("expected success for array-aud containing configured audience; got %v", err)
	}
}

// TestRS256_AlgConfusion_Rejected is the load-bearing security test. An
// attacker mints an HS256 token using the RSA public key bytes as the HMAC
// secret. Without algorithm pinning, a naive verifier would accept this.
// With jwt.WithValidMethods([]string{"RS256"}), the RS256 verifier rejects it.
func TestRS256_AlgConfusion_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	// Mint an HS256 token where the HMAC secret is the RSA public modulus
	// bytes — the classic alg-confusion payload.
	hsClaims := validClaims()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, hsClaims)
	tok.Header["kid"] = rsaKid
	pubBytes := f.priv.N.Bytes()
	signed, err := tok.SignedString(pubBytes)
	if err != nil {
		t.Fatalf("HS256 mint: %v", err)
	}

	_, err = v.Verify(context.Background(), signed)
	if err == nil {
		t.Fatal("SECURITY: alg-confusion HS256 token was accepted by RS256 verifier")
	}
}

func TestRS256_ClaimMappingOverride(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()

	cfg := cfgFor(f.jwksURL)
	cfg.Claims = config.SSOClaims{UserID: "user_id", Email: "email_addr", DisplayName: "full_name"}

	v, err := sso.NewRS256Verifier(cfg, silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	delete(claims, "sub")
	delete(claims, "email")
	delete(claims, "name")
	claims["user_id"] = "custom-user-42"
	claims["email_addr"] = "u42@example.com"
	claims["full_name"] = "User Forty-Two"
	claims["extra"] = "carry-into-custom"

	tok := mintRS256(t, f.priv, rsaKid, claims)
	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != "custom-user-42" {
		t.Errorf("UserID = %q; want custom-user-42", got.UserID)
	}
	if got.Email == nil || *got.Email != "u42@example.com" {
		t.Errorf("Email = %v; want u42@example.com", got.Email)
	}
	if got.DisplayName == nil || *got.DisplayName != "User Forty-Two" {
		t.Errorf("DisplayName = %v; want User Forty-Two", got.DisplayName)
	}
	if v, ok := got.Custom["extra"]; !ok || fmt.Sprint(v) != "carry-into-custom" {
		t.Errorf("Custom[extra] = %v; want carry-into-custom", got.Custom["extra"])
	}
}

func TestRS256_NotBeforeFuture_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	// 5 minutes in the future — well past the 30s clock skew.
	claims["nbf"] = time.Now().Add(5 * time.Minute).Unix()
	tok := mintRS256(t, f.priv, rsaKid, claims)
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for nbf in the future")
	}
}

// TestRS256_JWKSUnreachable_Errors asserts that a JWKS URL that returns 500 at
// construction fails fast — not deferred to first user request.
func TestRS256_JWKSUnreachable_Errors(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	cfg := cfgFor(bad.URL + "/.well-known/jwks.json")
	_, err := sso.NewRS256Verifier(cfg, silentLogger())
	if err == nil {
		t.Fatal("expected NewRS256Verifier to fail when the JWKS URL is unreachable")
	}
}
```

- [ ] **Step 2: Run to verify failure (stub returns error)**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/sso/... -v -run TestRS256_
```

Expected: every `TestRS256_*` FAILS — the stub `NewRS256Verifier` returns `"not yet implemented"`. MUST fail before proceeding.

- [ ] **Step 3: Implement RS256Verifier**

Replace the entire contents of `relay/internal/auth/sso/rs256.go` with:

```go
package sso

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"intake/internal/auth"
	"intake/internal/config"
)

// RS256Verifier validates RS256-signed SSO tokens against a JWKS fetched
// (and cached + refreshed-on-miss) from cfg.JWKSURL.
//
// Algorithm pinning: jwt.WithValidMethods([]string{"RS256"}) ensures an HS256
// token presented to this verifier is rejected (alg-confusion mitigation).
type RS256Verifier struct {
	cfg     config.SSOConfig
	keyfunc jwt.Keyfunc
	logger  *slog.Logger
}

// NewRS256Verifier fetches the JWKS at construction time and returns an error
// if the URL is unreachable. The relay must NOT start with a broken SSO
// config that fails on the first user request.
func NewRS256Verifier(cfg config.SSOConfig, logger *slog.Logger) (*RS256Verifier, error) {
	if cfg.JWKSURL == "" {
		return nil, errors.New("sso: RS256 verifier requires cfg.jwks_url")
	}
	if logger == nil {
		logger = slog.Default()
	}
	// keyfunc.NewDefault performs an initial JWKS fetch; a network/parse error
	// surfaces here, at startup.
	kf, err := keyfunc.NewDefault([]string{cfg.JWKSURL})
	if err != nil {
		// The error from keyfunc may include the URL but not any secrets.
		return nil, fmt.Errorf("sso: fetch JWKS at startup: %w", err)
	}
	return &RS256Verifier{
		cfg:     cfg,
		keyfunc: kf.Keyfunc,
		logger:  logger,
	}, nil
}

// Verify parses the token (with RS256 pinning), then runs the shared
// iss/aud/exp/nbf checks and claim mapping.
func (v *RS256Verifier) Verify(ctx context.Context, token string) (*auth.SSOClaims, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(
		token,
		claims,
		v.keyfunc,
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		// golang-jwt v5 error strings are safe ("signature is invalid",
		// "token has invalid claims", "token signing method <X> is invalid")
		// and never include key material.
		return nil, fmt.Errorf("sso: rs256 parse: %w", err)
	}
	if !parsed.Valid {
		return nil, errors.New("sso: token reported invalid")
	}
	return validateAndExtract(claims, v.cfg)
}

// Compile-time assertion: RS256Verifier satisfies the Verifier interface (and
// transitively auth.SSOVerifier).
var _ Verifier = (*RS256Verifier)(nil)
var _ auth.SSOVerifier = (*RS256Verifier)(nil)
```

- [ ] **Step 4: Run the RS256 tests**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/sso/... -v -run TestRS256_
```

Expected: every `TestRS256_*` PASSES — happy path, tampered, wrong-kid, expired, wrong-iss, wrong-aud, audience-as-array, alg-confusion, claim-mapping-override, nbf-future, JWKS-unreachable.

- [ ] **Step 5: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/auth/sso/rs256.go internal/auth/sso/rs256_test.go
git commit -m "feat(sso): RS256 verifier via JWKS + alg pinning (4-iii)"
```

---

### Task 4: Implement HS256Verifier with alg pinning (TDD)

**Files:** Modify `relay/internal/auth/sso/hs256.go`, create `relay/internal/auth/sso/hs256_test.go`

- [ ] **Step 1: Write the failing HS256 test file**

Create `relay/internal/auth/sso/hs256_test.go`:

```go
package sso_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"intake/internal/auth/sso"
	"intake/internal/config"
)

// hs256Secret is a deterministic 32-byte secret used in all HS256 tests.
var hs256Secret = []byte("0123456789abcdef0123456789abcdef")

// cfgForHS256 returns an SSOConfig that selects the HS256 path (JWKSURL empty).
func cfgForHS256() config.SSOConfig {
	return config.SSOConfig{
		Issuer:         "https://issuer.test/",
		Audience:       "https://api.test",
		HS256SecretEnv: "INTAKE_SSO_HS256_SECRET",
		Claims:         config.SSOClaims{UserID: "sub", Email: "email", DisplayName: "name"},
	}
}

// mintHS256 signs claims with the given secret.
func mintHS256(t *testing.T, secret []byte, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("HS256 SignedString: %v", err)
	}
	return s
}

func TestHS256_HappyPath(t *testing.T) {
	v, err := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	if err != nil {
		t.Fatalf("NewHS256Verifier: %v", err)
	}
	tok := mintHS256(t, hs256Secret, validClaims())

	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != "user-abc-123" {
		t.Errorf("UserID = %q; want user-abc-123", got.UserID)
	}
	if got.Email == nil || *got.Email != "user@example.com" {
		t.Errorf("Email = %v", got.Email)
	}
}

func TestHS256_TamperedToken_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	tok := mintHS256(t, hs256Secret, validClaims())
	tampered := []byte(tok)
	for i := range tampered {
		if tampered[i] == '.' && i+1 < len(tampered) {
			if tampered[i+1] == 'A' {
				tampered[i+1] = 'B'
			} else {
				tampered[i+1] = 'A'
			}
			break
		}
	}
	if _, err := v.Verify(context.Background(), string(tampered)); err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestHS256_WrongSecret_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	other := []byte("ffffffffffffffffffffffffffffffff")
	tok := mintHS256(t, other, validClaims())
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for token signed with a different secret")
	}
}

func TestHS256_Expired_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	claims := validClaims()
	claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	tok := mintHS256(t, hs256Secret, claims)
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestHS256_WrongIssuer_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	claims := validClaims()
	claims["iss"] = "https://attacker.test/"
	tok := mintHS256(t, hs256Secret, claims)
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestHS256_WrongAudience_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	claims := validClaims()
	claims["aud"] = "https://other-api.test"
	tok := mintHS256(t, hs256Secret, claims)
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for wrong audience")
	}
}

// TestHS256_AlgConfusion_Rejected: an RS256 token must NOT be accepted by the
// HS256 verifier even if some attacker presents one. WithValidMethods enforces
// the alg whitelist.
func TestHS256_AlgConfusion_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims())
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("RS256 SignedString: %v", err)
	}

	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("SECURITY: RS256 token accepted by HS256 verifier")
	}
}

func TestHS256_ClaimMappingOverride(t *testing.T) {
	cfg := cfgForHS256()
	cfg.Claims = config.SSOClaims{UserID: "user_id", Email: "email_addr", DisplayName: "full_name"}
	v, _ := sso.NewHS256Verifier(cfg, hs256Secret)

	claims := validClaims()
	delete(claims, "sub")
	delete(claims, "email")
	delete(claims, "name")
	claims["user_id"] = "hs-user-7"
	claims["email_addr"] = "h7@example.com"
	claims["full_name"] = "HS User Seven"

	tok := mintHS256(t, hs256Secret, claims)
	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != "hs-user-7" {
		t.Errorf("UserID = %q; want hs-user-7", got.UserID)
	}
	if got.Email == nil || *got.Email != "h7@example.com" {
		t.Errorf("Email = %v", got.Email)
	}
	if got.DisplayName == nil || *got.DisplayName != "HS User Seven" {
		t.Errorf("DisplayName = %v", got.DisplayName)
	}
}

func TestHS256_NotBeforeFuture_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	claims := validClaims()
	claims["nbf"] = time.Now().Add(5 * time.Minute).Unix()
	tok := mintHS256(t, hs256Secret, claims)
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for nbf in the future")
	}
}

// TestHS256_SecretTooShort_Rejected asserts the constructor enforces the
// 32-byte minimum on the resolved secret.
func TestHS256_SecretTooShort_Rejected(t *testing.T) {
	_, err := sso.NewHS256Verifier(cfgForHS256(), []byte("short"))
	if err == nil {
		t.Fatal("expected error for HS256 secret shorter than 32 bytes")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("error should mention the 32-byte minimum; got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure (stub returns error)**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/sso/... -v -run TestHS256_
```

Expected: every `TestHS256_*` either FAILS (the stub returns "not yet implemented") OR — for `TestHS256_SecretTooShort_Rejected` — already passes because the size check is in the stub. The bulk of them MUST fail before proceeding.

- [ ] **Step 3: Implement HS256Verifier**

Replace the entire contents of `relay/internal/auth/sso/hs256.go` with:

```go
package sso

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"intake/internal/auth"
	"intake/internal/config"
)

// HS256Verifier validates HS256-signed SSO tokens against a shared secret.
//
// Algorithm pinning: jwt.WithValidMethods([]string{"HS256"}) ensures an RS256
// token presented to this verifier is rejected (alg-confusion mitigation).
//
// The secret bytes are held in the struct but NEVER appear in any returned
// error.
type HS256Verifier struct {
	cfg    config.SSOConfig
	secret []byte
}

// NewHS256Verifier validates that the resolved secret is at least 32 bytes
// (PROJECT.md §17). The secret is held by reference; the caller (main.go)
// resolves it via config.RequireSecret before passing it in.
func NewHS256Verifier(cfg config.SSOConfig, secret []byte) (*HS256Verifier, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("sso: HS256 secret must be at least 32 bytes (got %d)", len(secret))
	}
	return &HS256Verifier{cfg: cfg, secret: secret}, nil
}

// Verify parses the token (with HS256 pinning), then runs the shared
// iss/aud/exp/nbf checks and claim mapping.
func (v *HS256Verifier) Verify(ctx context.Context, token string) (*auth.SSOClaims, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(
		token,
		claims,
		func(t *jwt.Token) (any, error) {
			return v.secret, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		// golang-jwt v5 error strings are safe (e.g. "signature is invalid")
		// and never include the secret bytes.
		return nil, fmt.Errorf("sso: hs256 parse: %w", err)
	}
	if !parsed.Valid {
		return nil, errors.New("sso: token reported invalid")
	}
	return validateAndExtract(claims, v.cfg)
}

// Compile-time assertions.
var _ Verifier = (*HS256Verifier)(nil)
var _ auth.SSOVerifier = (*HS256Verifier)(nil)
```

- [ ] **Step 4: Run the HS256 tests**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/sso/... -v -run TestHS256_
```

Expected: every `TestHS256_*` PASSES — happy path, tampered, wrong-secret, expired, wrong-iss, wrong-aud, alg-confusion, claim-mapping-override, nbf-future, secret-too-short.

- [ ] **Step 5: Run the entire sso package suite**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/sso/... -v
```

Expected: every test passes — both `TestNew_*`, every `TestRS256_*`, every `TestHS256_*`.

- [ ] **Step 6: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 7: Commit**

```
cd C:/src/ai/intake/relay && git add internal/auth/sso/hs256.go internal/auth/sso/hs256_test.go
git commit -m "feat(sso): HS256 verifier + alg pinning + 32-byte secret check (4-iii)"
```

---

### Task 5: Wire `sso.New` into main.go's middleware construction

**Files:** Modify `relay/cmd/relay/main.go`

- [ ] **Step 1: Locate the call site**

Open `relay/cmd/relay/main.go` and find the line introduced by 4-i:

```go
middleware := auth.NewMiddleware(store, nil, nil)
```

(If 4-ii has already replaced the second arg with an email verifier, only the third arg is `nil` — that's the one this plan replaces.)

- [ ] **Step 2: Add the imports**

Add to the import block at the top of `main.go` (alongside the other `intake/internal/...` imports):

```go
	"intake/internal/auth/sso"
```

(`intake/internal/config` is already imported.)

- [ ] **Step 3: Resolve the HS256 secret (if configured) and call the factory**

In `main.go`, IMMEDIATELY BEFORE the `middleware := auth.NewMiddleware(...)` line, add:

```go
	// 4-iii: construct the SSO verifier when sso mode is enabled.
	var ssoVerifier auth.SSOVerifier
	if cfg.Auth.Modes.SSO {
		var hs256Secret []byte
		if cfg.Auth.SSO.HS256SecretEnv != "" {
			s, err := config.RequireSecret(cfg.Auth.SSO.HS256SecretEnv)
			if err != nil {
				logger.Error("sso: resolve HS256 secret", "err", err)
				os.Exit(1)
			}
			hs256Secret = []byte(s)
		}
		v, err := sso.New(cfg.Auth.SSO, hs256Secret, logger)
		if err != nil {
			// Catches: both jwks_url and hs256_secret_env set, neither set,
			// JWKS unreachable at startup, HS256 secret <32 bytes.
			logger.Error("sso: construct verifier", "err", err)
			os.Exit(1)
		}
		ssoVerifier = v
	}
```

> Match the existing fatal-error idiom in `main.go` (e.g. `logger.Error(...); os.Exit(1)` if that's what the file uses; if it uses `log.Fatalf`, use that instead — keep the file consistent). The `os` package is already imported by `main.go`; if not, add it.

- [ ] **Step 4: Pass the verifier into `auth.NewMiddleware`**

Replace:

```go
middleware := auth.NewMiddleware(store, nil, nil)
```

with (preserving the second arg — which is `nil` here unless 4-ii has wired the email verifier):

```go
middleware := auth.NewMiddleware(store, nil, ssoVerifier)
```

> If 4-ii has already populated the second arg (e.g. `emailVerifier`), preserve that and only change the third arg.

- [ ] **Step 5: Build**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK
```

Expected: `BUILD_OK`.

- [ ] **Step 6: Vet**

```
cd C:/src/ai/intake/relay && go vet ./... && echo VET_OK
```

Expected: `VET_OK`.

- [ ] **Step 7: Full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK`. Every package — including the existing dispatcher tests from 4-i which still pass `nil` for both verifiers in their constructor calls — stays green.

- [ ] **Step 8: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "refactor(main): wire sso.New into auth.NewMiddleware when sso mode enabled (4-iii)"
```

---

### Task 6: Final verification gate

- [ ] **Step 1: Full build + vet + test**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected: `BUILD_OK`, `VET_OK`, every package `ok` (including `internal/auth/sso`), `TEST_OK`.

- [ ] **Step 2: Contract + pins**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK`.

- [ ] **Step 3: Confirm dependencies are exact-pinned**

```
cd C:/src/ai/intake/relay && go mod tidy && git diff --exit-code go.mod go.sum && echo MOD_CLEAN
```

Expected: `MOD_CLEAN` (no changes after tidy). The two new deps — `MicahParks/keyfunc/v3` and `golang-jwt/jwt/v5` — appear in `go.mod` with exact versions, no caret.

- [ ] **Step 4: Build-fail self-check (README §6)**

- HS256 token rejected by RS256 verifier → `TestRS256_AlgConfusion_Rejected`. ✓
- RS256 token rejected by HS256 verifier → `TestHS256_AlgConfusion_Rejected`. ✓
- Expired token rejected → `TestRS256_Expired_Rejected`, `TestHS256_Expired_Rejected`. ✓
- Tampered token rejected → `TestRS256_TamperedToken_Rejected`, `TestHS256_TamperedToken_Rejected`. ✓
- `auth.modes.sso=true` with both `jwks_url` AND `hs256_secret_env` set → factory error → `TestNew_BothSet_Errors`. ✓
- `auth.modes.sso=true` with neither set → factory error → `TestNew_NeitherSet_Errors`. ✓
- HS256 secret <32 bytes → constructor error → `TestHS256_SecretTooShort_Rejected`, `TestNew_HS256SecretTooShort_Errors`. ✓
- JWKS unreachable at startup → constructor error → `TestRS256_JWKSUnreachable_Errors`. ✓
- `keyfunc/v3` / `golang-jwt/jwt/v5` exact-pinned, gated by `check-pins.sh` → step 2 PASSES. ✓
- HS256 secret bytes / JWKS content never in any error → guaranteed by static error strings + golang-jwt v5's known-safe error texts; reviewers can grep for the literal secret bytes in test logs (none present). ✓
- `auth.SessionContext` shape unchanged. ✓
- `adapter.Adapter` interface unchanged. ✓

---

## Smoke

**Credit-free (unit):** `go test ./internal/auth/sso/... -v` is fully green — 23 tests across three files:

- `sso_test.go` (3): both-set, neither-set, HS256-secret-too-short factory errors.
- `rs256_test.go` (11): happy path, tampered, wrong-kid, expired, wrong-iss, wrong-aud, audience-as-array, alg-confusion (the load-bearing security test), claim-mapping-override, nbf-future, JWKS-unreachable.
- `hs256_test.go` (10): happy path, tampered, wrong-secret, expired, wrong-iss, wrong-aud, alg-confusion (the load-bearing security test), claim-mapping-override, nbf-future, secret-too-short.

The RS256 tests use an in-test RSA-2048 keypair (`rsa.GenerateKey(rand.Reader, 2048)`) and an `httptest.Server` JWKS endpoint — no network, no real IdP, no credits.

**Live SSO smoke (DEFERRED to 4-iv — pauses for maintainer):** requires either a real Auth0 (or other OIDC IdP) tenant + minted RS256 access token, OR a self-served local JWKS with a maintainer-minted RS256 test token. Configure `auth.modes.sso=true` + `auth.sso.issuer`/`audience`/`jwks_url`, drive `/init` → `/turn` with `Authorization: Bearer <real-jwt>`, assert `SubmitResponse.payload.user.auth_mode == "sso"`, `user.verified == true`, `user.id == <sub claim from the token>`. This sub-plan's smoke is the credit-free unit suite only.

## Done Criteria

1. `go build ./... && go vet ./...` clean in `relay/`.
2. `go test ./...` green, including all 23 tests in `internal/auth/sso/`, with NO real JWT and NO real JWKS server (all `httptest`).
3. `relay/internal/auth/sso/` exposes `Verifier` interface, `RS256Verifier`, `HS256Verifier`, and `New` factory. Both impl types satisfy both the local `sso.Verifier` interface AND the 4-i `auth.SSOVerifier` interface (compile-time `var _` assertions).
4. `New` rejects both-set and neither-set configurations at startup; `NewHS256Verifier` rejects secrets <32 bytes; `NewRS256Verifier` fetches the JWKS at construction time and fails if unreachable.
5. Both verifiers pin the JWT algorithm via `jwt.WithValidMethods` — an HS256 token presented to `RS256Verifier` is rejected, and an RS256 token presented to `HS256Verifier` is rejected. The two alg-confusion tests are the load-bearing security property and MUST be present and passing.
6. `iss` is exact-matched (NOT prefix); `aud` accepts both string and array-of-strings forms per RFC 7519; `exp`/`nbf` are checked with a 30s clock skew.
7. Claim names honor `cfg.Claims.{UserID,Email,DisplayName}` (with defaults `sub`/`email`/`name` from 4-i's `applyDefaults`); unmapped non-standard claims land in `Custom`.
8. HS256 secret bytes and JWKS content NEVER appear in any returned error (static error strings + golang-jwt v5's known-safe error texts).
9. `bash scripts/verify-contract.sh` and `bash scripts/check-pins.sh` green; `go mod tidy` leaves `go.mod`/`go.sum` clean. `github.com/MicahParks/keyfunc/v3` and `github.com/golang-jwt/jwt/v5` are exact-pinned with the corresponding entries in `scripts/check-pins.sh`.
10. `main.go` resolves the HS256 secret (if configured) via `config.RequireSecret`, calls `sso.New(cfg.Auth.SSO, hs256Secret, logger)`, fatals on error, and passes the returned verifier as the third arg of `auth.NewMiddleware(store, ..., ssoVerifier)`. The Phase-1 anonymous flow and the 4-i dispatcher tests remain unaffected.
