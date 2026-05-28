# 4-ii Email Magic-Link — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the email magic-link mode behind the **frozen** 4-i `auth.EmailJWTVerifier` seam: three new sub-packages (`auth/emailcode`, `auth/smtpsend`, `auth/emailjwt`), a small `EmailService` orchestrator that wires them together, two new UNAUTH endpoints (`POST /v1/intake/auth/email/start` and `POST /v1/intake/auth/email/verify`), and the `main.go` wiring that activates the path when `auth.modes.email: true`. After this sub-plan, a caller can POST an email, receive a 6-digit code, exchange it for a 15-min HS256 JWT, and use that JWT as `Authorization: Bearer <jwt>` to drive `/turn`/`/submit` — with `SessionContext.AuthMode=="email"` populated by the 4-i dispatcher and `payloadbuild` emitting `user.auth_mode="email", user.email=..., user.verified=true` unchanged.

**Architecture:** `emailcode.Store` is a thread-safe map keyed by email, holding `{code, issuedAt, used}` per issuance, with TTL eviction (default 10m) and per-email rate-limit (3 issuances per 10m window). `now func() time.Time` is injectable so tests advance virtual time without `time.Sleep`. Eviction is **eager on Issue/Verify** (no background goroutine — simpler, fine for v0). `smtpsend.Sender` is a small interface; `NetSMTP` wraps stdlib `net/smtp.PlainAuth` over STARTTLS for production; `FakeSender` captures `(to, code)` tuples for unit tests. `emailjwt.Mint`/`emailjwt.Verify` use `github.com/golang-jwt/jwt/v5` HS256 with `iss="intake-email"` baked into the claims; a small `*emailjwt.Verifier{Secret []byte}` adapter satisfies `auth.EmailJWTVerifier`. The two endpoints are mounted in `registerIntakeRoutes` **alongside `/init`** (NOT under the auth-gated group). `main.go` resolves `INTAKE_SMTP_PASS` and `INTAKE_EMAIL_JWT_SECRET` via `config.RequireSecret`, fatals if the JWT secret is shorter than 32 bytes (PROJECT.md §17), parses `code_ttl`/`jwt_ttl` durations, constructs the `EmailService`, hands `&emailjwt.Verifier{Secret: secret}` to `auth.NewMiddleware(store, emailVerifier, sso)`, and sets `deps.EmailService`.

**Tech Stack:** Go 1.23.2 (relay). One new external dep — `github.com/golang-jwt/jwt/v5` — **exact-pinned** at the version resolved on first task (verify via `go get github.com/golang-jwt/jwt/v5@latest`, capture, lock). `scripts/check-pins.sh` extended to fail on a caret/`@latest` for it (mirrors the existing anthropic/openai/genai gates). All other code is stdlib (`net/smtp`, `net/mail`, `crypto/rand`, `encoding/json`, `sync`, `time`).

> **Ordering note (4-ii vs 4-iii):** 4-iii (SSO) also depends on `github.com/golang-jwt/jwt/v5`. If 4-iii has already landed when this plan executes, Task 1's `go get` is idempotent (`go get` on an already-present module at the same version is a no-op; on a newer pin it's a deliberate bump — confirm the pinned version matches before proceeding). 4-iii adds `MicahParks/keyfunc/v3` separately; this plan does NOT touch that dependency.

---

## Design References

- Phase 4 README §8.2 — `EmailConfig` shape (FROZEN in 4-i; consumed here)
- Phase 4 README §8.3 — email package interfaces (`emailcode.Store`, `smtpsend.Sender`, `emailjwt.Mint/Verify`) — FROZEN here
- Phase 4 README §8.5 — endpoint contracts for `/auth/email/start` and `/auth/email/verify` — FROZEN here for wire shape
- Phase 4 README §6 — build-fail items (rate-limit 429+`Retry-After`, secret never in logs/body, JWT-secret-<32 fatal at startup)
- Phase 4 README §7 — live MailHog smoke (deferred to 4-iv; the credit-free unit layer is locked here)
- Design spec §2.2 — three new auth sub-packages
- Design spec §5 — Email JWT claims (`iss="intake-email"`, `sub=email`, `iat`, `exp`); HS256; secret ≥32 bytes per PROJECT.md §17
- Design spec §6 — endpoint contracts (generic 401 / 429 bodies — anti-enumeration)
- Design spec §8 — build-fail items
- Design spec §9 — testing strategy
- Design spec §11 — non-goals (no CAPTCHA, no per-IP rate-limit, no persistence)
- `relay/internal/config/secret.go` — `config.ResolveSecret` / `config.RequireSecret` semantics (env-or-`_FILE`; never leaks the value)
- `relay/internal/adapter/webhook/webhook.go` — reference for `defer resp.Body.Close()`, "include status in error but never the secret" pattern
- 4-i landing: `relay/internal/auth/types.go` (`EmailJWTVerifier` interface), `relay/internal/auth/middleware.go` (dispatcher), `relay/internal/server/deps.go` (`AuthCfg` field), `relay/cmd/relay/main.go` (current `auth.NewMiddleware(store, nil, nil)` call)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/go.mod` / `relay/go.sum` | Modify | Add exact pin of `github.com/golang-jwt/jwt/v5` (Task 1) |
| `scripts/check-pins.sh` | Modify | Add caret/`@latest` gate for `golang-jwt/jwt/v5` |
| `relay/internal/auth/emailcode/emailcode.go` | Create | `Store` with `Issue`/`Verify`, TTL+rate-limit+single-use, injectable `now` |
| `relay/internal/auth/emailcode/emailcode_test.go` | Create | TTL, single-use, rate-limit window, concurrency, 6-digit format |
| `relay/internal/auth/smtpsend/smtpsend.go` | Create | `Sender` interface, `NetSMTP` stdlib impl, `FakeSender` test double |
| `relay/internal/auth/smtpsend/smtpsend_test.go` | Create | `FakeSender` capture order; `NewNetSMTP` constructor stores params |
| `relay/internal/auth/emailjwt/emailjwt.go` | Create | `Mint`/`Verify` (HS256, `iss="intake-email"`, `sub=email`); `*Verifier` adapter |
| `relay/internal/auth/emailjwt/emailjwt_test.go` | Create | Round-trip, tamper, wrong secret, expired, wrong iss, len(secret)<32 |
| `relay/internal/server/email_service.go` | Create | `EmailService` orchestrator: codestore + sender + jwt mint |
| `relay/internal/server/email.go` | Create | `emailStartHandler` and `emailVerifyHandler` |
| `relay/internal/server/email_test.go` | Create | Endpoint tests with `FakeSender`; integration via `/turn` |
| `relay/internal/server/routes.go` | Modify | Mount `/auth/email/start` and `/auth/email/verify` at top of `registerIntakeRoutes` (UNAUTH) |
| `relay/internal/server/deps.go` | Modify | Add `EmailService *EmailService` field |
| `relay/cmd/relay/main.go` | Modify | Resolve secrets, build `EmailService`, pass `*emailjwt.Verifier` to `auth.NewMiddleware`, set `deps.EmailService` |

---

## Tasks

### Task 1: Pin `github.com/golang-jwt/jwt/v5` and extend `scripts/check-pins.sh`

**Files:** `relay/go.mod`, `relay/go.sum`, `scripts/check-pins.sh`

- [ ] **Step 1: Resolve and pin the latest stable**

```
cd C:/src/ai/intake/relay && go get github.com/golang-jwt/jwt/v5@latest
```

Capture the resolved version from `go.mod` (e.g. `v5.2.x`). If 4-iii has already pinned `golang-jwt/jwt/v5`, this command is a no-op (or a deliberate bump — confirm before proceeding). Verify the pin is exact (no `^`, no `@latest`):

```
cd C:/src/ai/intake/relay && grep golang-jwt/jwt go.mod
```

Expected: a single `require github.com/golang-jwt/jwt/v5 v5.X.Y` line — exact version, no `^` prefix, no `// indirect` (we will use it directly).

- [ ] **Step 2: Run `go mod tidy`**

```
cd C:/src/ai/intake/relay && go mod tidy
```

Expected: `go.sum` updated; no other changes. If `go.mod` shows other modules being added, investigate (a stray import sneaked in).

- [ ] **Step 3: Extend `scripts/check-pins.sh` with the new gate**

In `scripts/check-pins.sh`, after the existing `google.golang.org/genai` gate block (and before the "no `go install/get @latest`" generic gate), add:

```bash
# Gate: golang-jwt/jwt/v5 must be exact-pinned (no caret, no @latest) in go.mod. Phase 4.
if grep -E 'golang-jwt/jwt/v5' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/golang-jwt/jwt/v5 is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
```

- [ ] **Step 4: Run the pin gate**

```
cd C:/src/ai/intake && bash scripts/check-pins.sh
```

Expected: `OK: all codegen tools are exact-pinned`. No errors.

- [ ] **Step 5: Build + vet (the dep is present but unused — Go tolerates that in `go.sum` only after a real import)**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

> If `go.mod` shows the new line but `go.sum` does not yet carry hashes for it, that is fine — Step 2's `go mod tidy` already did the right thing; later tasks import the package and lock it in.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add go.mod go.sum
cd C:/src/ai/intake && git add scripts/check-pins.sh
git commit -m "deps(relay): pin golang-jwt/jwt/v5 exactly + check-pins gate (4-ii)"
```

---

### Task 2: Create the `emailcode` package (TDD)

**Files:** Create `relay/internal/auth/emailcode/emailcode_test.go`, then `relay/internal/auth/emailcode/emailcode.go`

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/auth/emailcode/emailcode_test.go`:

```go
package emailcode_test

import (
	"regexp"
	"sync"
	"testing"
	"time"

	"intake/internal/auth/emailcode"
)

// virtualClock is a controllable now-source.
type virtualClock struct {
	mu sync.Mutex
	t  time.Time
}

func (v *virtualClock) Now() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.t
}

func (v *virtualClock) Advance(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.t = v.t.Add(d)
}

func newVC() *virtualClock {
	return &virtualClock{t: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)}
}

var sixDigit = regexp.MustCompile(`^[0-9]{6}$`)

func TestStore_Issue_ReturnsSixDigitCode(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	code, retry, err := s.Issue("user@example.com")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if retry != 0 {
		t.Errorf("retryAfter should be 0 on success, got %v", retry)
	}
	if !sixDigit.MatchString(code) {
		t.Errorf("code %q is not 6 digits", code)
	}
}

func TestStore_VerifyHappyPath(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	code, _, err := s.Issue("user@example.com")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !s.Verify("user@example.com", code) {
		t.Fatal("Verify should accept a fresh code")
	}
}

func TestStore_VerifyWrongCode(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	if _, _, err := s.Issue("user@example.com"); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if s.Verify("user@example.com", "000000") {
		t.Fatal("Verify should reject a wrong code")
	}
}

func TestStore_SingleUse(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	code, _, _ := s.Issue("user@example.com")
	if !s.Verify("user@example.com", code) {
		t.Fatal("first verify must succeed")
	}
	if s.Verify("user@example.com", code) {
		t.Fatal("second verify of the same code must fail (single-use)")
	}
}

func TestStore_TTLEviction(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	code, _, _ := s.Issue("user@example.com")
	vc.Advance(10*time.Minute + time.Second) // past TTL
	if s.Verify("user@example.com", code) {
		t.Fatal("Verify must reject an expired code")
	}
}

func TestStore_RateLimit_FourthIssueReturnsRetryAfter(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	for i := 0; i < 3; i++ {
		if _, _, err := s.Issue("user@example.com"); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	code, retry, err := s.Issue("user@example.com")
	if err == nil {
		t.Fatal("4th Issue must return ErrRateLimited")
	}
	if err != emailcode.ErrRateLimited {
		t.Errorf("err = %v; want ErrRateLimited", err)
	}
	if code != "" {
		t.Errorf("rate-limited Issue must return empty code, got %q", code)
	}
	if retry <= 0 {
		t.Errorf("retryAfter must be > 0 when rate-limited, got %v", retry)
	}
}

func TestStore_RateLimit_WindowResets(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	for i := 0; i < 3; i++ {
		if _, _, err := s.Issue("user@example.com"); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	if _, _, err := s.Issue("user@example.com"); err != emailcode.ErrRateLimited {
		t.Fatalf("4th issue (in-window) should rate-limit, got %v", err)
	}
	vc.Advance(10*time.Minute + time.Second) // window resets
	if _, _, err := s.Issue("user@example.com"); err != nil {
		t.Fatalf("after window reset, Issue should succeed, got %v", err)
	}
}

func TestStore_RateLimit_IndependentPerEmail(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	for i := 0; i < 3; i++ {
		if _, _, err := s.Issue("a@example.com"); err != nil {
			t.Fatalf("issue a %d: %v", i, err)
		}
	}
	// Different email — should still succeed.
	if _, _, err := s.Issue("b@example.com"); err != nil {
		t.Fatalf("Issue for different email must not be rate-limited, got %v", err)
	}
}

func TestStore_Concurrent_IssueAndVerify(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 1000, vc.Now)

	const N = 50
	codes := make(chan string, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, err := s.Issue("concurrent@example.com")
			if err != nil {
				t.Errorf("Issue: %v", err)
				return
			}
			codes <- c
		}()
	}
	wg.Wait()
	close(codes)
	// Drain all codes — they should all be unique 6-digit strings, but only the
	// most-recent matters for Verify (multiple issuances overwrite the active code).
	count := 0
	for c := range codes {
		if !sixDigit.MatchString(c) {
			t.Errorf("non-6-digit code %q", c)
		}
		count++
	}
	if count != N {
		t.Errorf("issued %d codes; want %d", count, N)
	}
}
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/emailcode/... -v
```

Expected: `no required module provides package intake/internal/auth/emailcode` or a build error. MUST fail before proceeding.

- [ ] **Step 3: Create `emailcode.go`**

Create `relay/internal/auth/emailcode/emailcode.go`:

```go
// Package emailcode is a thread-safe, in-memory store of pending email
// verification codes. Each email may hold at most one active code at a time
// (most recent overwrites prior); rate-limit caps the number of Issue calls
// per email within a rolling window.
//
// Storage is in-memory only — restart invalidates pending codes (acceptable
// for v0; design spec §11 non-goal). Eviction is eager on Issue/Verify (no
// background goroutine).
package emailcode

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ErrRateLimited is returned by Issue when the per-email rate-limit cap is hit.
var ErrRateLimited = errors.New("emailcode: rate-limited")

// entry holds a single issued code and its lifecycle metadata.
type entry struct {
	code     string
	issuedAt time.Time
	used     bool
}

// Store is the in-memory code store.
type Store struct {
	mu sync.Mutex

	codeTTL      time.Duration
	rateWindow   time.Duration
	perWindowCap int
	now          func() time.Time

	// active maps email → its single most-recent (unconsumed, unexpired) code.
	active map[string]*entry

	// history maps email → ordered timestamps of Issue calls within rateWindow.
	// Entries older than rateWindow are evicted on each Issue/Verify call.
	history map[string][]time.Time
}

// New returns a Store with the given code TTL, rate-limit window, and per-window cap.
// `now` is injectable for tests (production: time.Now).
func New(codeTTL, rateWindow time.Duration, perWindowCap int, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{
		codeTTL:      codeTTL,
		rateWindow:   rateWindow,
		perWindowCap: perWindowCap,
		now:          now,
		active:       make(map[string]*entry),
		history:      make(map[string][]time.Time),
	}
}

// Issue generates a fresh 6-digit code for email. If ≥perWindowCap codes have
// been issued for email within the last rateWindow, returns ("", retryAfter,
// ErrRateLimited) where retryAfter is the time until the oldest still-in-window
// issuance ages out. On success, stores the code (overwriting any prior active
// entry for email) and returns the code.
func (s *Store) Issue(email string) (string, time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	// Evict history entries outside the rate window.
	hist := s.history[email]
	cutoff := now.Add(-s.rateWindow)
	pruned := hist[:0]
	for _, t := range hist {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	s.history[email] = pruned

	if len(pruned) >= s.perWindowCap {
		// retryAfter: the oldest in-window issuance ages out at hist[0] + rateWindow.
		retry := pruned[0].Add(s.rateWindow).Sub(now)
		if retry < time.Second {
			retry = time.Second // caller writes Retry-After header in seconds; never advertise 0
		}
		return "", retry, ErrRateLimited
	}

	// Evict any active-entry that has expired (for any email — eager cleanup
	// keeps memory bounded). Inline since callers are infrequent.
	for k, e := range s.active {
		if now.Sub(e.issuedAt) > s.codeTTL {
			delete(s.active, k)
		}
	}

	code, err := generateSixDigitCode()
	if err != nil {
		return "", 0, fmt.Errorf("emailcode: generate: %w", err)
	}

	s.active[email] = &entry{code: code, issuedAt: now, used: false}
	s.history[email] = append(s.history[email], now)
	return code, 0, nil
}

// Verify reports whether code matches an unexpired, unused issuance for email.
// On match, marks it used (single-use); subsequent verifies of the same code fail.
func (s *Store) Verify(email, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	e, ok := s.active[email]
	if !ok {
		return false
	}
	if e.used {
		return false
	}
	if now.Sub(e.issuedAt) > s.codeTTL {
		delete(s.active, email)
		return false
	}
	if e.code != code {
		return false
	}
	e.used = true
	return true
}

// generateSixDigitCode returns a uniformly-distributed 6-digit decimal string
// using crypto/rand. Range: [000000, 999999] inclusive.
func generateSixDigitCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
```

- [ ] **Step 4: Run the emailcode tests**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/emailcode/... -v
```

Expected: all 9 tests PASS (`TestStore_Issue_ReturnsSixDigitCode`, `TestStore_VerifyHappyPath`, `TestStore_VerifyWrongCode`, `TestStore_SingleUse`, `TestStore_TTLEviction`, `TestStore_RateLimit_FourthIssueReturnsRetryAfter`, `TestStore_RateLimit_WindowResets`, `TestStore_RateLimit_IndependentPerEmail`, `TestStore_Concurrent_IssueAndVerify`).

- [ ] **Step 5: Race-detector run**

```
cd C:/src/ai/intake/relay && go test -race ./internal/auth/emailcode/... -v
```

Expected: same green; no `DATA RACE` warnings.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/auth/emailcode/
git commit -m "feat(emailcode): TTL+rate-limit code store (4-ii)"
```

---

### Task 3: Create the `smtpsend` package (TDD)

**Files:** Create `relay/internal/auth/smtpsend/smtpsend_test.go`, then `relay/internal/auth/smtpsend/smtpsend.go`

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/auth/smtpsend/smtpsend_test.go`:

```go
package smtpsend_test

import (
	"context"
	"sync"
	"testing"

	"intake/internal/auth/smtpsend"
)

func TestFakeSender_CapturesInOrder(t *testing.T) {
	f := smtpsend.NewFakeSender()
	ctx := context.Background()
	if err := f.Send(ctx, "alice@example.com", "111111"); err != nil {
		t.Fatalf("Send 1: %v", err)
	}
	if err := f.Send(ctx, "bob@example.com", "222222"); err != nil {
		t.Fatalf("Send 2: %v", err)
	}
	sent := f.Sent()
	if len(sent) != 2 {
		t.Fatalf("len(Sent) = %d; want 2", len(sent))
	}
	if sent[0].To != "alice@example.com" || sent[0].Code != "111111" {
		t.Errorf("Sent[0] = %+v; want {alice@example.com, 111111}", sent[0])
	}
	if sent[1].To != "bob@example.com" || sent[1].Code != "222222" {
		t.Errorf("Sent[1] = %+v; want {bob@example.com, 222222}", sent[1])
	}
}

func TestFakeSender_ThreadSafe(t *testing.T) {
	f := smtpsend.NewFakeSender()
	const N = 50
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.Send(context.Background(), "x@example.com", "123456")
		}()
	}
	wg.Wait()
	if got := len(f.Sent()); got != N {
		t.Errorf("len(Sent) = %d; want %d", got, N)
	}
}

func TestNewNetSMTP_StoresParams(t *testing.T) {
	// We can't dial without a real server here — but we CAN assert the
	// constructor returns a non-nil sender that satisfies the interface
	// and that subsequent Send returns an error (no SMTP server on 127.0.0.1:1).
	n := smtpsend.NewNetSMTP("127.0.0.1", 1, "user", "pass", "Intake <noreply@example.com>")
	if n == nil {
		t.Fatal("NewNetSMTP returned nil")
	}
	var _ smtpsend.Sender = n // compile-time interface check
	err := n.Send(context.Background(), "to@example.com", "123456")
	if err == nil {
		t.Fatal("Send against unreachable 127.0.0.1:1 must error")
	}
	// SECURITY: the error MUST NOT contain the password.
	if containsCaseInsensitive(err.Error(), "pass") {
		// note: literal token "pass" intentionally matches what we passed —
		// if the error embeds it, that is a leak.
		t.Errorf("Send error leaked password: %v", err)
	}
}

func containsCaseInsensitive(s, sub string) bool {
	// Avoid pulling strings — keep this test file's imports minimal.
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a := s[i+j]
			b := sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/smtpsend/... -v
```

Expected: build error — package does not exist. MUST fail before proceeding.

- [ ] **Step 3: Create `smtpsend.go`**

Create `relay/internal/auth/smtpsend/smtpsend.go`:

```go
// Package smtpsend ships one-line auth-code emails to a configured SMTP server.
// The `Sender` interface is the test seam; `NetSMTP` is the production impl
// over stdlib net/smtp (SMTP-AUTH PLAIN/LOGIN over STARTTLS); `FakeSender` is
// the test double that captures (to, code) tuples in memory.
//
// Security: implementations MUST NOT include the SMTP password in any returned
// error. The stdlib net/smtp implementation does not leak the password on its
// own; this package adds no logging that could leak it.
package smtpsend

import (
	"context"
	"fmt"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
)

// Sender ships a one-line auth-code email to `to`.
type Sender interface {
	Send(ctx context.Context, to string, code string) error
}

// NetSMTP is the production net/smtp implementation. It uses smtp.PlainAuth
// over STARTTLS-capable servers (smtp.SendMail negotiates STARTTLS when the
// server advertises it on port 587/465). For port 25 plain delivery the
// password should be empty (no auth).
type NetSMTP struct {
	host string
	port int
	user string
	pass string
	from string
}

// NewNetSMTP constructs the production sender. The password is the RESOLVED
// secret value (caller passes it in via config.ResolveSecret); this package
// never reads the environment.
func NewNetSMTP(host string, port int, user, password, from string) *NetSMTP {
	return &NetSMTP{
		host: host,
		port: port,
		user: user,
		pass: password,
		from: from,
	}
}

// Send delivers a one-line auth-code email to `to`. The body is intentionally
// minimal — no HTML, no templating — to keep v0 deliverability simple.
// The error returned never contains the SMTP password.
func (n *NetSMTP) Send(ctx context.Context, to, code string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	addr := n.host + ":" + strconv.Itoa(n.port)

	subject := "Your intake verification code"
	body := "Your intake verification code is: " + code + "\r\n\r\n" +
		"This code expires in 10 minutes. If you did not request it, you can ignore this email.\r\n"

	msg := buildMessage(n.from, to, subject, body)

	var auth smtp.Auth
	if n.user != "" || n.pass != "" {
		auth = smtp.PlainAuth("", n.user, n.pass, n.host)
	}

	if err := smtp.SendMail(addr, auth, n.from, []string{to}, msg); err != nil {
		// stdlib's net/smtp errors do not embed the password — they carry the
		// SMTP server's response text and basic transport diagnostics. We
		// nonetheless wrap defensively without referencing n.pass.
		return fmt.Errorf("smtpsend: send to %s via %s: %w", to, addr, err)
	}
	return nil
}

// buildMessage produces a minimal RFC 5322 message. CRLF line endings per spec.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(to)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(subject)
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// SentRecord is one captured (to, code) pair from FakeSender.
type SentRecord struct {
	To   string
	Code string
}

// FakeSender is the in-memory test double — captures every Send call in order.
type FakeSender struct {
	mu   sync.Mutex
	sent []SentRecord
}

// NewFakeSender returns a fresh FakeSender.
func NewFakeSender() *FakeSender { return &FakeSender{} }

// Send records the (to, code) pair and returns nil.
func (f *FakeSender) Send(_ context.Context, to, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, SentRecord{To: to, Code: code})
	return nil
}

// Sent returns a copy of the captured records (ordered).
func (f *FakeSender) Sent() []SentRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SentRecord, len(f.sent))
	copy(out, f.sent)
	return out
}
```

> Note the test's `Sent()` method returns `[]SentRecord` (not `[]struct{To, Code string}` as the README §8.3 sketches). That sketch's anonymous-struct form is awkward at call sites; the equivalent named type `SentRecord` with the same two fields satisfies the same need and reads better. The README §8.3 sketch is a shape contract, not a syntactic mandate.

- [ ] **Step 4: Run the smtpsend tests**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/smtpsend/... -v
```

Expected: 3 tests PASS (`TestFakeSender_CapturesInOrder`, `TestFakeSender_ThreadSafe`, `TestNewNetSMTP_StoresParams`).

- [ ] **Step 5: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/auth/smtpsend/
git commit -m "feat(smtpsend): Sender interface + NetSMTP + FakeSender (4-ii)"
```

---

### Task 4: Create the `emailjwt` package (TDD)

**Files:** Create `relay/internal/auth/emailjwt/emailjwt_test.go`, then `relay/internal/auth/emailjwt/emailjwt.go`

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/auth/emailjwt/emailjwt_test.go`:

```go
package emailjwt_test

import (
	"strings"
	"testing"
	"time"

	"intake/internal/auth/emailjwt"

	"github.com/golang-jwt/jwt/v5"
)

// thirtyTwoByteSecret returns a deterministic 32-byte secret for tests.
var thirtyTwoByteSecret = []byte("0123456789abcdef0123456789abcdef")

func TestMint_RejectsShortSecret(t *testing.T) {
	short := []byte("too-short")
	_, _, err := emailjwt.Mint(short, "user@example.com", 15*time.Minute)
	if err == nil {
		t.Fatal("Mint must reject a secret shorter than 32 bytes")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("error should mention the 32-byte minimum, got %v", err)
	}
}

func TestMintVerify_RoundTrip(t *testing.T) {
	token, expiresAt, err := emailjwt.Mint(thirtyTwoByteSecret, "user@example.com", 15*time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if token == "" {
		t.Fatal("Mint returned empty token")
	}
	if !expiresAt.After(time.Now()) {
		t.Errorf("expiresAt %v must be in the future", expiresAt)
	}

	email, err := emailjwt.Verify(thirtyTwoByteSecret, token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "user@example.com" {
		t.Errorf("email = %q; want user@example.com", email)
	}
}

func TestVerify_RejectsTamperedToken(t *testing.T) {
	token, _, err := emailjwt.Mint(thirtyTwoByteSecret, "user@example.com", 15*time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	// Flip a character in the signature segment (last dot-separated piece).
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape: %d parts", len(parts))
	}
	last := []byte(parts[2])
	if last[0] == 'A' {
		last[0] = 'B'
	} else {
		last[0] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(last)
	if _, err := emailjwt.Verify(thirtyTwoByteSecret, tampered); err == nil {
		t.Fatal("Verify must reject a tampered token")
	}
}

func TestVerify_RejectsWrongSecret(t *testing.T) {
	token, _, err := emailjwt.Mint(thirtyTwoByteSecret, "user@example.com", 15*time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	other := []byte("ffffffffffffffffffffffffffffffff") // also 32 bytes, but different
	if _, err := emailjwt.Verify(other, token); err == nil {
		t.Fatal("Verify must reject a token signed with a different secret")
	}
}

func TestVerify_RejectsExpiredToken(t *testing.T) {
	// Mint with a tiny negative TTL via direct claims (the public Mint takes a
	// positive ttl); craft an already-expired token using jwt directly.
	claims := jwt.MapClaims{
		"sub": "user@example.com",
		"iat": time.Now().Add(-time.Hour).Unix(),
		"exp": time.Now().Add(-time.Minute).Unix(), // expired
		"iss": emailjwt.Issuer,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(thirtyTwoByteSecret)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	if _, err := emailjwt.Verify(thirtyTwoByteSecret, signed); err == nil {
		t.Fatal("Verify must reject an expired token")
	}
}

func TestVerify_RejectsWrongIssuer(t *testing.T) {
	// Mint a token with iss="other-system" — Verify must reject so an SSO token
	// minted with the same secret can never sneak through the email branch.
	claims := jwt.MapClaims{
		"sub": "user@example.com",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		"iss": "other-system",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(thirtyTwoByteSecret)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	if _, err := emailjwt.Verify(thirtyTwoByteSecret, signed); err == nil {
		t.Fatal("Verify must reject a token whose iss is not 'intake-email'")
	}
}

func TestVerify_RejectsEmptySub(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		"iss": emailjwt.Issuer,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString(thirtyTwoByteSecret)
	if _, err := emailjwt.Verify(thirtyTwoByteSecret, signed); err == nil {
		t.Fatal("Verify must reject a token with empty sub")
	}
}

func TestVerifier_AdapterSatisfiesInterface(t *testing.T) {
	v := &emailjwt.Verifier{Secret: thirtyTwoByteSecret}
	token, _, err := emailjwt.Mint(thirtyTwoByteSecret, "user@example.com", time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	email, err := v.Verify(token)
	if err != nil {
		t.Fatalf("Verifier.Verify: %v", err)
	}
	if email != "user@example.com" {
		t.Errorf("email = %q; want user@example.com", email)
	}
}
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/emailjwt/... -v
```

Expected: build error — package does not exist; or `intake/internal/auth/emailjwt` import not found. MUST fail.

- [ ] **Step 3: Create `emailjwt.go`**

Create `relay/internal/auth/emailjwt/emailjwt.go`:

```go
// Package emailjwt mints and verifies HS256 JWTs for the email magic-link auth
// mode. Claims are `{sub:<email>, iat, exp, iss:"intake-email"}`. The iss
// constant is consumed by the sso verifier to trivially reject email-mode
// tokens that masquerade as SSO.
package emailjwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer is the value baked into the `iss` claim of every email-mode JWT.
// It is also exported so callers may add it to their reject-list (e.g. the
// SSO verifier rejecting iss="intake-email" outright).
const Issuer = "intake-email"

// minSecretLen is the minimum HS256 secret length (PROJECT.md §17).
const minSecretLen = 32

// Mint returns a signed JWT for the given email with the given TTL.
// Claims: {sub:email, iat:now, exp:now+ttl, iss:Issuer}. HS256.
// Returns an error if len(secret) < 32 bytes.
func Mint(secret []byte, email string, ttl time.Duration) (string, time.Time, error) {
	if len(secret) < minSecretLen {
		return "", time.Time{}, fmt.Errorf("emailjwt: secret must be at least %d bytes (got %d)", minSecretLen, len(secret))
	}
	if email == "" {
		return "", time.Time{}, errors.New("emailjwt: email must not be empty")
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	claims := jwt.MapClaims{
		"sub": email,
		"iat": now.Unix(),
		"exp": expiresAt.Unix(),
		"iss": Issuer,
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("emailjwt: sign: %w", err)
	}
	return signed, expiresAt, nil
}

// Verify validates the token (HS256, iss=Issuer, exp>now, sub non-empty) and
// returns the email from the sub claim. Returns an error for any defect.
// The returned error MUST NOT contain the secret (golang-jwt v5 errors are
// clean — we wrap defensively without referencing the secret bytes).
func Verify(secret []byte, token string) (string, error) {
	if len(secret) < minSecretLen {
		return "", fmt.Errorf("emailjwt: secret must be at least %d bytes", minSecretLen)
	}

	parsed, err := jwt.Parse(
		token,
		func(t *jwt.Token) (any, error) {
			// Algorithm pinning: only HS256 accepted (mitigates alg-confusion).
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return secret, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", fmt.Errorf("emailjwt: verify: %w", err)
	}
	if !parsed.Valid {
		return "", errors.New("emailjwt: token invalid")
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("emailjwt: claims wrong shape")
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("emailjwt: sub claim missing or empty")
	}
	return sub, nil
}

// Verifier adapts Verify to the auth.EmailJWTVerifier interface (single-method
// `Verify(token) (string, error)`). main.go constructs &Verifier{Secret: resolved}
// and passes it to auth.NewMiddleware.
type Verifier struct {
	Secret []byte
}

// Verify satisfies auth.EmailJWTVerifier.
func (v *Verifier) Verify(token string) (string, error) {
	return Verify(v.Secret, token)
}
```

- [ ] **Step 4: Run the emailjwt tests**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/emailjwt/... -v
```

Expected: 8 tests PASS (`TestMint_RejectsShortSecret`, `TestMintVerify_RoundTrip`, `TestVerify_RejectsTamperedToken`, `TestVerify_RejectsWrongSecret`, `TestVerify_RejectsExpiredToken`, `TestVerify_RejectsWrongIssuer`, `TestVerify_RejectsEmptySub`, `TestVerifier_AdapterSatisfiesInterface`).

- [ ] **Step 5: Compile-time check that `*emailjwt.Verifier` satisfies `auth.EmailJWTVerifier`**

The middleware dispatcher (4-i) accepts an `EmailJWTVerifier` interface. Confirm shape compatibility:

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK
```

If `*emailjwt.Verifier`'s Verify signature does not match (e.g. wrong return types), Task 7's main.go wiring will fail to compile — flag early.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/auth/emailjwt/
git commit -m "feat(emailjwt): HS256 mint+verify with iss=intake-email (4-ii)"
```

---

### Task 5: Create the `EmailService` orchestrator

**Files:** Create `relay/internal/server/email_service.go`

> Placement decision: `EmailService` lives in `internal/server/` rather than as a new `internal/auth/email/` package. Rationale: it is a one-screen orchestrator with no domain logic of its own (it just composes the three sub-packages + a config snapshot); the handlers that consume it live in the server package; a separate auth/email package would add an import edge without adding behavior. README §8.3 freezes the three sub-packages — `EmailService` is plumbing.

- [ ] **Step 1: Create `email_service.go`**

Create `relay/internal/server/email_service.go`:

```go
package server

import (
	"context"
	"errors"
	"time"

	"intake/internal/auth/emailcode"
	"intake/internal/auth/emailjwt"
	"intake/internal/auth/smtpsend"
)

// EmailService is the small orchestrator that the /auth/email/start and
// /auth/email/verify handlers consume. It owns the codestore + sender + the
// JWT secret/TTL, and exposes two methods that wrap the underlying flows.
//
// Constructed once in main.go when auth.modes.email is true.
type EmailService struct {
	codes  *emailcode.Store
	sender smtpsend.Sender
	secret []byte
	jwtTTL time.Duration
}

// ErrSMTP wraps an underlying SMTP send error; handlers translate this to 502.
// The error never contains the SMTP password.
var ErrSMTP = errors.New("email: smtp send failed")

// NewEmailService wires the components. secret MUST be ≥32 bytes (caller
// validates at startup — emailjwt.Mint will additionally guard).
func NewEmailService(codes *emailcode.Store, sender smtpsend.Sender, secret []byte, jwtTTL time.Duration) *EmailService {
	return &EmailService{
		codes:  codes,
		sender: sender,
		secret: secret,
		jwtTTL: jwtTTL,
	}
}

// IssueAndSend issues a code for email and sends it via the sender. Returns
// (retryAfter, ErrRateLimited) when rate-limited, or (0, ErrSMTP-wrapped) when
// the sender fails. The handler maps these to 429 / 502.
//
// Caller is responsible for log-scrubbing — this method never logs the code.
func (e *EmailService) IssueAndSend(ctx context.Context, email string) (retryAfter time.Duration, err error) {
	code, retryAfter, err := e.codes.Issue(email)
	if err != nil {
		return retryAfter, err
	}
	if err := e.sender.Send(ctx, email, code); err != nil {
		return 0, errors.Join(ErrSMTP, err)
	}
	return 0, nil
}

// VerifyAndMint verifies code against email and mints a JWT on success.
// Returns (token, expiresAt, nil) on success; (_, _, error) on
// invalid/expired/used code (handler maps to a generic 401).
func (e *EmailService) VerifyAndMint(email, code string) (string, time.Time, error) {
	if !e.codes.Verify(email, code) {
		return "", time.Time{}, errors.New("email: invalid or expired code")
	}
	return emailjwt.Mint(e.secret, email, e.jwtTTL)
}
```

- [ ] **Step 2: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. The new file is referenced by no test yet — that's fine.

- [ ] **Step 3: Add `EmailService` to `Deps`**

In `relay/internal/server/deps.go`, add after the existing `Builder` field (at the bottom of the struct):

```go
	// from 4-ii:

	// EmailService is the orchestrator for /auth/email/start and /auth/email/verify.
	// nil when auth.modes.email is false (handlers respond 404 in that case).
	EmailService *EmailService
```

- [ ] **Step 4: Build**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK
```

Expected: `BUILD_OK`.

- [ ] **Step 5: Commit**

```
cd C:/src/ai/intake/relay && git add internal/server/email_service.go internal/server/deps.go
git commit -m "feat(server): EmailService orchestrator + Deps.EmailService (4-ii)"
```

---

### Task 6: Create the `/auth/email/start` and `/auth/email/verify` handlers (TDD)

**Files:** Create `relay/internal/server/email_test.go`, then `relay/internal/server/email.go`; modify `relay/internal/server/routes.go`.

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/server/email_test.go`:

```go
package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"intake/internal/auth"
	"intake/internal/auth/emailcode"
	"intake/internal/auth/emailjwt"
	"intake/internal/auth/smtpsend"
	"intake/internal/config"
	"intake/internal/server"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef") // 32 bytes

func buildEmailServer(t *testing.T, fake smtpsend.Sender) (*server.Deps, http.Handler) {
	t.Helper()
	codes := emailcode.New(10*time.Minute, 10*time.Minute, 3, time.Now)
	emailSvc := server.NewEmailService(codes, fake, testSecret, 15*time.Minute)

	cfg := &config.Config{
		Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}},
		Auth: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true, Email: true},
			Email: config.EmailConfig{CodeTTL: "10m", JWTTTL: "15m"},
		},
	}
	deps := server.Deps{
		Auth:         auth.NewMiddleware(auth.NewStore(), &emailjwt.Verifier{Secret: testSecret}, nil),
		AuthCfg:      cfg.Auth,
		EmailService: emailSvc,
	}
	return &deps, server.New(cfg, deps)
}

func TestEmailStart_HappyPath(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	body := bytes.NewBufferString(`{"email":"user@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		MessageSent bool `json:"message_sent"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.MessageSent {
		t.Errorf("message_sent = false; want true")
	}
	sent := fake.Sent()
	if len(sent) != 1 {
		t.Fatalf("FakeSender captured %d; want 1", len(sent))
	}
	if sent[0].To != "user@example.com" {
		t.Errorf("captured To = %q; want user@example.com", sent[0].To)
	}
	if len(sent[0].Code) != 6 {
		t.Errorf("captured code %q is not 6 chars", sent[0].Code)
	}
}

func TestEmailStart_BadJSON_400(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rr.Code)
	}
}

func TestEmailStart_InvalidEmail_400(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"not-an-email"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rr.Code)
	}
}

func TestEmailStart_RateLimited_429_WithRetryAfter(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// Three within cap.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("issue %d: status = %d; body = %s", i, rr.Code, rr.Body.String())
		}
	}
	// Fourth exceeds cap.
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; want 429", rr.Code)
	}
	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("Retry-After header is empty; must be set on 429")
	}
	if n, err := strconv.Atoi(retryAfter); err != nil || n <= 0 {
		t.Errorf("Retry-After = %q; want positive integer seconds", retryAfter)
	}
	// Anti-enumeration: body must be the generic shape; must NOT include count or window-reset timestamp.
	body := rr.Body.String()
	if !strings.Contains(body, "rate_limited") {
		t.Errorf("429 body missing error.code rate_limited: %s", body)
	}
	if strings.Contains(body, "4") || strings.Contains(body, "count") || strings.Contains(body, "window") {
		t.Errorf("429 body must not expose count/window detail: %s", body)
	}
}

// stubFailingSender returns a fixed error on every Send.
type stubFailingSender struct{}

func (stubFailingSender) Send(_ interface{ Done() <-chan struct{} }, _ string, _ string) error { panic("unused") }

// Implement smtpsend.Sender directly to avoid signature confusion.
type failingSender struct{}

func (failingSender) Send(_ ctxAlias, _ string, _ string) error {
	return errSMTPFailure
}

// We use the real context interface here via a tiny alias to keep the test file
// import-tidy without dragging context in explicitly.
type ctxAlias = interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(any) any
}

var errSMTPFailure = newSMTPErr("upstream rejected: 535 Authentication credentials invalid")

type smtpErr struct{ s string }

func (e *smtpErr) Error() string { return e.s }
func newSMTPErr(s string) error  { return &smtpErr{s: s} }

func TestEmailStart_SMTPFailure_502_NoDetailLeak(t *testing.T) {
	_, mux := buildEmailServer(t, failingSender{})

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d; want 502", rr.Code)
	}
	body := rr.Body.String()
	// Body must NOT echo the SMTP detail.
	if strings.Contains(body, "535") || strings.Contains(body, "credentials") {
		t.Errorf("502 body leaked SMTP detail: %s", body)
	}
	if !strings.Contains(body, "smtp_error") {
		t.Errorf("502 body missing error.code smtp_error: %s", body)
	}
}

func TestEmailVerify_HappyPath_RoundTrips(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// 1. Start
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start status %d; body %s", rr.Code, rr.Body.String())
	}
	code := fake.Sent()[0].Code

	// 2. Verify
	body := `{"email":"user@example.com","code":"` + code + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify status %d; body %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
		User      struct {
			Email    string `json:"email"`
			Verified bool   `json:"verified"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("token is empty")
	}
	if resp.User.Email != "user@example.com" || !resp.User.Verified {
		t.Errorf("user = %+v; want {email:user@example.com, verified:true}", resp.User)
	}
	if !resp.ExpiresAt.After(time.Now()) {
		t.Errorf("expires_at %v must be in the future", resp.ExpiresAt)
	}

	// The token round-trips through emailjwt.Verify.
	email, err := emailjwt.Verify(testSecret, resp.Token)
	if err != nil {
		t.Fatalf("emailjwt.Verify(returned token): %v", err)
	}
	if email != "user@example.com" {
		t.Errorf("verified email = %q; want user@example.com", email)
	}
}

func TestEmailVerify_WrongCode_401_Generic(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// Start to ensure a code exists.
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start: %d", rr.Code)
	}

	// Verify with the wrong code.
	body := `{"email":"user@example.com","code":"000000"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rr.Code)
	}
	// Generic body.
	respBody := rr.Body.String()
	if !strings.Contains(respBody, "invalid_code") {
		t.Errorf("body missing error.code invalid_code: %s", respBody)
	}
	// Must not say "not found" vs "expired" vs "used" — anti-enumeration.
	for _, leak := range []string{"not found", "expired", "already used", "consumed"} {
		if strings.Contains(strings.ToLower(respBody), leak) {
			t.Errorf("401 body leaks enumeration detail %q: %s", leak, respBody)
		}
	}
}

func TestEmailVerify_AlreadyUsed_401(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// Start.
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	code := fake.Sent()[0].Code

	// First verify succeeds.
	body := `{"email":"user@example.com","code":"` + code + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first verify: %d", rr.Code)
	}

	// Second verify of same code fails 401.
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("second verify status = %d; want 401", rr.Code)
	}
}

func TestEmailVerify_BadJSON_400(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rr.Code)
	}
}

// Integration: full flow end-to-end — start → verify → /turn with the returned
// bearer reaches a handler that sees SessionContext.AuthMode=="email".
func TestEmailFlow_DrivesTurnWithBearer(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// 1. start
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start: %d", rr.Code)
	}
	code := fake.Sent()[0].Code

	// 2. verify
	body := `{"email":"user@example.com","code":"` + code + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify: %d body=%s", rr.Code, rr.Body.String())
	}
	var verifyResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &verifyResp); err != nil {
		t.Fatalf("decode verify: %v", err)
	}

	// 3. drive /turn with the bearer — turn handler may fail downstream (no LLM),
	//    but the middleware must accept the bearer and not 401.
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/turn", bytes.NewBufferString(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+verifyResp.Token)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("turn returned 401 despite valid email JWT; body = %s", rr.Body.String())
	}
	// Status 500 (no Provider wired in the test Deps) is acceptable here — the
	// point is that the middleware accepted the bearer and dispatched to the
	// turn handler.
}
```

> Note on the `failingSender` / `ctxAlias` stubs in the test file: they avoid importing `context` solely so the SMTP-failure test stays compact. If during implementation it reads cleaner to import `context` and define `func (failingSender) Send(context.Context, string, string) error`, that is equivalent — pick whichever is cleaner in the final implementation. The behavioral assertions are the contract.

- [ ] **Step 2: Run to verify failure (handlers + routes missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/server/... -run TestEmail -v
```

Expected: routes not registered, all `TestEmail*` tests fail with 404 (chi default) or similar. MUST fail.

- [ ] **Step 3: Create the handlers file**

Create `relay/internal/server/email.go`:

```go
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"

	"intake/internal/auth/emailcode"
)

// emailStartRequest is the body of POST /v1/intake/auth/email/start.
type emailStartRequest struct {
	Email string `json:"email"`
}

// emailVerifyRequest is the body of POST /v1/intake/auth/email/verify.
type emailVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// emailVerifyResponse is the success body of POST /v1/intake/auth/email/verify.
type emailVerifyResponse struct {
	Token     string             `json:"token"`
	ExpiresAt string             `json:"expires_at"`
	User      emailVerifyUser    `json:"user"`
}

type emailVerifyUser struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}

// emailStartHandler handles POST /v1/intake/auth/email/start.
// Mounted UNAUTH at the top of registerIntakeRoutes.
func emailStartHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.EmailService == nil {
			writeError(w, http.StatusNotFound, "not_found", "email auth not enabled")
			return
		}

		var req emailStartRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
		if _, err := mail.ParseAddress(req.Email); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid email address")
			return
		}

		retry, err := deps.EmailService.IssueAndSend(r.Context(), req.Email)
		switch {
		case errors.Is(err, emailcode.ErrRateLimited):
			// Anti-enumeration: generic body, detail only via Retry-After header.
			seconds := int(retry.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many codes requested for this email; retry later")
			return
		case errors.Is(err, ErrSMTP):
			// Log the underlying detail server-side; the body stays generic.
			if deps.Logger != nil {
				deps.Logger.Error("email start: smtp send failed", "email_redacted", redactEmail(req.Email), "err", err.Error())
			}
			writeError(w, http.StatusBadGateway, "smtp_error", "could not send email")
			return
		case err != nil:
			// Defensive — should not happen.
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"message_sent": true})
	}
}

// emailVerifyHandler handles POST /v1/intake/auth/email/verify.
func emailVerifyHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.EmailService == nil {
			writeError(w, http.StatusNotFound, "not_found", "email auth not enabled")
			return
		}

		var req emailVerifyRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
		if req.Email == "" || req.Code == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "email and code are required")
			return
		}

		token, expiresAt, err := deps.EmailService.VerifyAndMint(req.Email, req.Code)
		if err != nil {
			// Anti-enumeration: generic 401 regardless of "not found"/"expired"/"used".
			writeError(w, http.StatusUnauthorized, "invalid_code", "invalid or expired code")
			return
		}

		writeJSON(w, http.StatusOK, emailVerifyResponse{
			Token:     token,
			ExpiresAt: expiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			User: emailVerifyUser{
				Email:    req.Email,
				Verified: true,
			},
		})
	}
}

// redactEmail returns the email with the local-part redacted, suitable for
// log lines. "user@example.com" → "u***@example.com".
func redactEmail(email string) string {
	at := -1
	for i := 0; i < len(email); i++ {
		if email[i] == '@' {
			at = i
			break
		}
	}
	if at < 1 {
		return "***"
	}
	return string(email[0]) + "***" + email[at:]
}
```

- [ ] **Step 4: Mount the two endpoints at the top of `registerIntakeRoutes` (UNAUTH)**

In `relay/internal/server/routes.go`, replace the function body with:

```go
func registerIntakeRoutes(r chi.Router, deps Deps) {
	// POST /v1/intake/init — no auth; issues a session.
	r.Post("/init", initHandler(deps))

	// POST /v1/intake/auth/email/start, /verify — no auth; email JWT bootstrap (4-ii).
	// 404s cleanly when deps.EmailService is nil (auth.modes.email disabled).
	r.Post("/auth/email/start", emailStartHandler(deps))
	r.Post("/auth/email/verify", emailVerifyHandler(deps))

	// Routes that require a valid session.
	r.Group(func(r chi.Router) {
		r.Use(deps.Auth.Handler)
		r.Post("/turn", turnHandler(deps))
		r.Post("/submit", submitHandler(deps))
	})
}
```

- [ ] **Step 5: Run the email endpoint tests**

```
cd C:/src/ai/intake/relay && go test ./internal/server/... -run TestEmail -v
```

Expected: all 9 `TestEmail*` tests PASS (`TestEmailStart_HappyPath`, `TestEmailStart_BadJSON_400`, `TestEmailStart_InvalidEmail_400`, `TestEmailStart_RateLimited_429_WithRetryAfter`, `TestEmailStart_SMTPFailure_502_NoDetailLeak`, `TestEmailVerify_HappyPath_RoundTrips`, `TestEmailVerify_WrongCode_401_Generic`, `TestEmailVerify_AlreadyUsed_401`, `TestEmailVerify_BadJSON_400`, `TestEmailFlow_DrivesTurnWithBearer`).

- [ ] **Step 6: Full server suite + race**

```
cd C:/src/ai/intake/relay && go test ./internal/server/... -v && echo SERVER_OK
cd C:/src/ai/intake/relay && go test -race ./internal/server/... && echo RACE_OK
```

Expected: `SERVER_OK`, `RACE_OK` — existing 4-i init-handler tests still pass; new email tests + integration test green; no races.

- [ ] **Step 7: Commit**

```
cd C:/src/ai/intake/relay && git add internal/server/email.go internal/server/email_test.go internal/server/routes.go
git commit -m "feat(server): /auth/email/start + /verify endpoints, UNAUTH-mounted (4-ii)"
```

---

### Task 7: Wire `main.go` for `auth.modes.email: true`

**Files:** Modify `relay/cmd/relay/main.go`

- [ ] **Step 1: Add imports**

In `relay/cmd/relay/main.go`, ensure the import block contains:

```go
	"intake/internal/auth/emailcode"
	"intake/internal/auth/emailjwt"
	"intake/internal/auth/smtpsend"
```

- [ ] **Step 2: Build the email wiring block**

Insert before the `auth.NewMiddleware(...)` call (the 4-i version: `middleware := auth.NewMiddleware(store, nil, nil)`):

```go
	// 4-ii: email magic-link wiring.
	var emailVerifier auth.EmailJWTVerifier // nil unless cfg.Auth.Modes.Email is true
	var emailSvc *server.EmailService
	if cfg.Auth.Modes.Email {
		smtpPass, err := config.ResolveSecret(cfg.Auth.Email.SMTPPassEnv)
		if err != nil {
			logger.Error("email auth: resolve SMTP password", "env", cfg.Auth.Email.SMTPPassEnv, "err", err)
			os.Exit(1)
		}
		// smtpPass may legitimately be empty (e.g. local MailHog with no auth);
		// the SMTPUser presence is the operator's choice.

		jwtSecret, err := config.RequireSecret(cfg.Auth.Email.JWTSecretEnv)
		if err != nil {
			logger.Error("email auth: resolve JWT secret", "env", cfg.Auth.Email.JWTSecretEnv, "err", err)
			os.Exit(1)
		}
		if len(jwtSecret) < 32 {
			logger.Error("email auth: jwt_secret must be at least 32 bytes (PROJECT.md §17)", "env", cfg.Auth.Email.JWTSecretEnv, "len", len(jwtSecret))
			os.Exit(1)
		}

		codeTTL, err := time.ParseDuration(cfg.Auth.Email.CodeTTL)
		if err != nil {
			logger.Error("email auth: invalid code_ttl", "value", cfg.Auth.Email.CodeTTL, "err", err)
			os.Exit(1)
		}
		jwtTTL, err := time.ParseDuration(cfg.Auth.Email.JWTTTL)
		if err != nil {
			logger.Error("email auth: invalid jwt_ttl", "value", cfg.Auth.Email.JWTTTL, "err", err)
			os.Exit(1)
		}

		// Rate-limit: 3 codes per code_ttl window (matches the design — "≥3 codes
		// in 10 min for this address" per spec §2.4).
		const perWindowCap = 3
		codeStore := emailcode.New(codeTTL, codeTTL, perWindowCap, time.Now)

		sender := smtpsend.NewNetSMTP(
			cfg.Auth.Email.SMTPHost,
			cfg.Auth.Email.SMTPPort,
			cfg.Auth.Email.SMTPUser,
			smtpPass,
			cfg.Auth.Email.From,
		)

		secretBytes := []byte(jwtSecret)
		emailSvc = server.NewEmailService(codeStore, sender, secretBytes, jwtTTL)
		emailVerifier = &emailjwt.Verifier{Secret: secretBytes}

		logger.Info("relay: email auth enabled",
			"smtp_host", cfg.Auth.Email.SMTPHost,
			"smtp_port", cfg.Auth.Email.SMTPPort,
			"from", cfg.Auth.Email.From,
			"code_ttl", codeTTL.String(),
			"jwt_ttl", jwtTTL.String(),
		)
	}
```

> The block above NEVER logs `smtpPass` or `jwtSecret`. The `logger.Error` lines on resolve-failure carry the env-var NAME (safe) and `err.Error()` from `ResolveSecret` (which is guaranteed by `config/secret.go` to never include the value).

- [ ] **Step 3: Update the `auth.NewMiddleware` call**

Replace the 4-i line:

```go
	middleware := auth.NewMiddleware(store, nil, nil)
```

with:

```go
	// 4-ii: pass emailVerifier (nil-OK when email mode disabled); SSO stays nil until 4-iii.
	middleware := auth.NewMiddleware(store, emailVerifier, nil)
```

- [ ] **Step 4: Populate `deps.EmailService`**

In the `deps := server.Deps{...}` literal, add the new field (next to the existing `AuthCfg: cfg.Auth`):

```go
		EmailService: emailSvc,
```

- [ ] **Step 5: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. If there are stray unused imports (e.g. `emailcode` not used after a typo), fix.

- [ ] **Step 6: Full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK` — every package (config, auth, auth/emailcode, auth/smtpsend, auth/emailjwt, server, providers, anthropic, ollama, openai, gemini, router, payloadbuild, adapter/*, license, internal/license) `ok`. No regression in Phase-1/2/3 tests.

- [ ] **Step 7: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "feat(main): wire EmailService + emailjwt.Verifier when auth.modes.email (4-ii)"
```

---

### Task 8: Final verification gate

- [ ] **Step 1: Full build + vet + test (with race)**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK && go test -race ./internal/auth/... ./internal/server/... && echo RACE_OK
```

Expected: `BUILD_OK`, `VET_OK`, `TEST_OK`, `RACE_OK`.

- [ ] **Step 2: Contract + pins gates**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK` (`PINS_OK` proves the new `golang-jwt/jwt/v5` gate is in place and the pin is exact).

- [ ] **Step 3: Confirm `go.mod` carries `golang-jwt/jwt/v5` exactly once, not as `// indirect`, no caret**

```
cd C:/src/ai/intake/relay && grep -n golang-jwt go.mod
```

Expected: exactly one `require` line, exact version (e.g. `v5.2.1`), no `^`, no `// indirect`.

- [ ] **Step 4: Confirm no NEW external dependency beyond `golang-jwt/jwt/v5`**

```
cd C:/src/ai/intake/relay && go mod tidy && git diff --exit-code go.mod go.sum && echo MOD_CLEAN
```

Expected: `MOD_CLEAN`. If `go.mod`/`go.sum` change, a stray transitive sneaked in — investigate.

- [ ] **Step 5: Secret-redaction self-check**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/smtpsend/... -run TestNewNetSMTP_StoresParams -v
cd C:/src/ai/intake/relay && go test ./internal/server/... -run TestEmailStart_SMTPFailure_502_NoDetailLeak -v
cd C:/src/ai/intake/relay && go test ./internal/server/... -run TestEmailStart_RateLimited_429_WithRetryAfter -v
cd C:/src/ai/intake/relay && go test ./internal/server/... -run TestEmailVerify_WrongCode_401_Generic -v
```

Expected: all four PASS — proves (a) no SMTP password in NetSMTP error; (b) no SMTP detail in 502 body; (c) no count/window in 429 body; (d) no enumeration detail ("not found"/"expired"/"used") in 401 body.

- [ ] **Step 6: Build-fail self-check (README §6)**

- `golang-jwt/jwt/v5` exact-pinned, check-pins gate active → Step 2. ✓
- SMTP password / JWT secret never logged or in body → Step 5 (a), (b). ✓
- `len(jwt_secret) >= 32` fatal at startup → `TestMint_RejectsShortSecret` proves emailjwt enforces; main.go has an additional `len(jwtSecret) < 32` guard. ✓
- Email rate-limit returns 429 + `Retry-After` (not 200) → `TestEmailStart_RateLimited_429_WithRetryAfter`. ✓
- Anonymous flow unchanged → 4-i `TestDispatcher_AnonymousFallthrough_Preserved` still green in Step 1. ✓
- `auth.SessionContext` shape UNCHANGED — middleware populates `AuthMode/Verified/Email` only on the email branch. ✓
- Frozen seams: `adapter.Adapter`, `auth.Middleware.Handler`, generated `payload/types.go` — none modified by this plan. ✓

---

## Smoke

**Credit-free (unit):** the full `go test ./...` run is the proof — `internal/auth/emailcode` (9 tests, race-clean), `internal/auth/smtpsend` (3 tests), `internal/auth/emailjwt` (8 tests), `internal/server` (10 new email tests including the end-to-end `TestEmailFlow_DrivesTurnWithBearer` that drives `/turn` with a returned JWT). The `FakeSender` is the SMTP seam; no real SMTP server is touched here. Total new tests: **30** across the four packages, all stdlib + `golang-jwt/jwt/v5` deterministic.

**Live (deferred to 4-iv — README §7 step 1):** real MailHog smoke. 4-iv's `core/smoke/drive-auth-email.ts` will configure relay with `auth.modes.email: true`, `smtp_host=192.168.1.102`, `smtp_port=1025`, a real `INTAKE_EMAIL_JWT_SECRET` (≥32 bytes), drive `/auth/email/start → fetch code from MailHog UI → /auth/email/verify → /turn → /submit`, and assert `user.auth_mode=="email"`, `user.email=="pete@mantichor.com"`, `user.verified=true` in the canonical payload posted to a local webhook receiver. The unit layer here proves the contract; 4-iv proves the live wire.

## Done Criteria

1. `go build ./... && go vet ./...` clean in `relay/`.
2. `go test ./...` green; `go test -race ./internal/auth/... ./internal/server/...` green.
3. `github.com/golang-jwt/jwt/v5` exact-pinned in `relay/go.mod`; `scripts/check-pins.sh` carries the matching caret/`@latest` gate; both gates pass.
4. Three new packages exist under `relay/internal/auth/`:
   - `emailcode` — `Store{Issue(email)→(code, retryAfter, err), Verify(email, code)→ok}`, 10-min TTL, 3-per-10-min rate-limit, single-use, injectable `now`, race-free.
   - `smtpsend` — `Sender` interface, `NetSMTP` stdlib impl (PLAIN over STARTTLS), `FakeSender` test double.
   - `emailjwt` — `Mint`/`Verify` HS256 with `iss="intake-email"`, `*Verifier{Secret}` adapter satisfying `auth.EmailJWTVerifier`.
5. Two new endpoints exist:
   - `POST /v1/intake/auth/email/start` — 200 on success, 400 on bad JSON / invalid email, 429 + `Retry-After` on rate-limit (generic body), 502 on SMTP failure (no detail in body).
   - `POST /v1/intake/auth/email/verify` — 200 with JWT + `expires_at` + `{user:{email,verified:true}}` on success, 401 (generic body) on invalid/expired/used code, 400 on bad JSON.
   - Both mounted UNAUTH at the top of `registerIntakeRoutes`.
6. `Deps` carries `EmailService *EmailService`; `main.go` resolves `INTAKE_SMTP_PASS` (optional) + `INTAKE_EMAIL_JWT_SECRET` (required), fatals on `len(secret)<32`, parses durations, constructs `EmailService`, hands `&emailjwt.Verifier{Secret}` to `auth.NewMiddleware(store, emailVerifier, nil)`.
7. Anti-enumeration confirmed: 429 body has no count/window detail, 401 body has no "not found"/"expired"/"used" distinction.
8. Secrets never leak: NetSMTP error has no password, 502 body has no SMTP detail, log lines log env-var NAMES (not values) and use `redactEmail` for the user address.
9. Frozen seams unchanged: `adapter.Adapter`, `auth.Middleware.Handler`, `auth.SessionContext`, generated `payload/types.go`.
10. `bash scripts/verify-contract.sh` and `bash scripts/check-pins.sh` green; `go mod tidy` is clean except for the one new direct dep.
