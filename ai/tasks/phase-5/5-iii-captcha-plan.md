# 5-iii CAPTCHA Verifier + /init Two-Call Dance — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 5-i `captcha.New` placeholder with the real Cloudflare Turnstile + hCaptcha siteverify implementations, layer single-use replay-protection on top of the providers' own semantics, and wire the verifier into `initHandler` so the second /init call (with `captcha_token`) actually validates the token before minting a session. After this sub-plan: the relay correctly returns 400 captcha_required → 200 (with valid token) → 401 captcha_failed (invalid/replayed token) → 502 captcha_unavailable (siteverify outage).

**Architecture:** One small `relay/internal/captcha/` package with a single `Verifier` interface and three impls — `Turnstile`, `HCaptcha`, and `Stub`. Each provider impl owns its siteverify URL constant; both share `commonVerify` which POSTs the form-encoded `secret`/`response`/`remoteip` body, decodes a common JSON shape (`success bool`, `error-codes []string`), and consults the replay-protection set. The set is a `sync.Mutex`-guarded `map[token]time.Time` evicted eagerly when entries exceed 5 minutes (Turnstile + hCaptcha tokens both expire ~300s). Secret never appears in any returned error — L005's redact-before-truncate is applied inside `commonVerify` ahead of any error wrapping. The HTTP client defaults to a 5s timeout; tests inject one pointed at `httptest.NewServer` so siteverify never reaches the real internet.

In `initHandler`, the existing 5-i dispatch is extended: when `captcha_token` IS present AND captcha is required, call `deps.CaptchaVerifier.Verify(ctx, token, clientIP)` and branch on the result (200 / 401 / 502). The 400 path is unchanged from 5-i.

**Tech Stack:** Go 1.23.2 (relay). Stdlib `net/http` + `encoding/json` + `net/url` + `sync` only — no new external Go modules.

---

## Design References

- README §8.5 — `captcha.Verifier` shape (frozen here)
- README §8.9 — endpoint contracts (200/400/401/502 land here)
- Design spec §2.4 — captcha discovery + two-call dance
- Design spec §3.3 — Verifier interface + replay-protection
- Design spec §6.1 — /init data flow
- Reference: Cloudflare Turnstile siteverify docs — `POST https://challenges.cloudflare.com/turnstile/v0/siteverify` with form-encoded `{secret, response, remoteip}`; response `{success, error-codes, challenge_ts, hostname}`
- Reference: hCaptcha siteverify docs — `POST https://hcaptcha.com/siteverify` with the same form shape and response shape
- Reference: existing `relay/internal/adapter/zendesk/zendesk.go` — the L005 redact-before-truncate pattern (omit body from non-2xx errors entirely, or redact via `strings.ReplaceAll(...)` BEFORE truncating)
- Reference: existing `relay/internal/auth/emailcode/emailcode.go` — the eager-eviction pattern Phase 5-iii's replay set mirrors
- Reference: 5-i `relay/internal/server/turn.go` `initHandler` — 5-iii extends the captcha branch (the 400 / discovery path is already implemented)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/captcha/captcha.go` | Modify | Replace 5-i placeholder; add Turnstile + HCaptcha impls + shared `commonVerify` + replay-set |
| `relay/internal/captcha/captcha_test.go` | Create | httptest-mocked siteverify: ok → ok; success:false → reason; replay → reason="duplicate"; non-2xx → 502 surface; secret never in errors; timeout |
| `relay/internal/server/turn.go` | Modify | `initHandler` calls deps.CaptchaVerifier.Verify when token is present |
| `relay/internal/server/turn_test.go` | Modify | Test that /init with a valid token mints a session; invalid → 401; verifier err → 502 |
| `relay/cmd/relay/main.go` | Modify | Resolve `cfg.Captcha.SecretKeyEnv` via ResolveSecret; construct real `captcha.New(...)`; populate `Deps.CaptchaVerifier` |

---

## Tasks

### Task 1: Implement `captcha.Verifier` + Turnstile + HCaptcha + replay-set

**Files:** Modify `relay/internal/captcha/captcha.go`, Create `relay/internal/captcha/captcha_test.go`

- [ ] **Step 1: Write the failing tests**

Create `relay/internal/captcha/captcha_test.go`:

```go
package captcha_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/captcha"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time         { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

// turnstileSuccessHandler returns success or failure per the configured behavior.
func turnstileSuccessHandler(t *testing.T, success bool, errorCodes []string, captureSecret *string, captureRemoteIP *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("siteverify mock: ParseForm: %v", err)
		}
		if captureSecret != nil {
			*captureSecret = r.Form.Get("secret")
		}
		if captureRemoteIP != nil {
			*captureRemoteIP = r.Form.Get("remoteip")
		}
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"success":      success,
			"error-codes":  errorCodes,
			"challenge_ts": "2026-01-01T12:00:00.000Z",
			"hostname":     "example.com",
		}
		_ = json.NewEncoder(w).Encode(body)
	})
}

// newTurnstileVerifier constructs a Turnstile verifier whose siteverify URL
// is overridden to point at the test server. captcha.New does not accept
// a URL override (it embeds the provider URLs); for tests, we use the
// captcha.NewTurnstileWithURL helper exposed via an export_test.go.
func newTurnstileVerifier(t *testing.T, siteverifyURL, secret string, clock func() time.Time) captcha.Verifier {
	t.Helper()
	v, err := captcha.NewTurnstileWithURL(siteverifyURL, secret, &http.Client{Timeout: 2 * time.Second}, clock)
	if err != nil {
		t.Fatalf("NewTurnstileWithURL: %v", err)
	}
	return v
}

func TestTurnstile_OkToken_Verifies(t *testing.T) {
	c := newClock()
	var capturedSecret string
	srv := httptest.NewServer(turnstileSuccessHandler(t, true, nil, &capturedSecret, nil))
	defer srv.Close()

	v := newTurnstileVerifier(t, srv.URL, "secret-shh", c.Now)
	ok, reason, err := v.Verify(context.Background(), "any-token", "203.0.113.5")
	if err != nil {
		t.Fatalf("Verify: unexpected err = %v", err)
	}
	if !ok {
		t.Errorf("Verify ok = false (reason=%q); want true", reason)
	}
	if capturedSecret != "secret-shh" {
		t.Errorf("siteverify received secret = %q; want %q (form field)", capturedSecret, "secret-shh")
	}
}

func TestTurnstile_FailureWithErrorCodes_ReturnsReason(t *testing.T) {
	c := newClock()
	srv := httptest.NewServer(turnstileSuccessHandler(t, false, []string{"invalid-input-response", "timeout-or-duplicate"}, nil, nil))
	defer srv.Close()

	v := newTurnstileVerifier(t, srv.URL, "secret-shh", c.Now)
	ok, reason, err := v.Verify(context.Background(), "any-token", "203.0.113.5")
	if err != nil {
		t.Fatalf("Verify: unexpected err = %v", err)
	}
	if ok {
		t.Error("Verify ok = true; want false")
	}
	if reason != "invalid-input-response" {
		t.Errorf("reason = %q; want invalid-input-response (first error code)", reason)
	}
}

func TestTurnstile_FailureWithoutErrorCodes_ReturnsGenericReason(t *testing.T) {
	c := newClock()
	srv := httptest.NewServer(turnstileSuccessHandler(t, false, nil, nil, nil))
	defer srv.Close()

	v := newTurnstileVerifier(t, srv.URL, "secret-shh", c.Now)
	ok, reason, err := v.Verify(context.Background(), "any-token", "203.0.113.5")
	if err != nil {
		t.Fatalf("Verify: unexpected err = %v", err)
	}
	if ok || reason == "" {
		t.Errorf("Verify ok=%v reason=%q; want ok=false, non-empty reason", ok, reason)
	}
}

func TestTurnstile_ReplayProtection_RejectsSameTokenTwice(t *testing.T) {
	c := newClock()
	srv := httptest.NewServer(turnstileSuccessHandler(t, true, nil, nil, nil))
	defer srv.Close()

	v := newTurnstileVerifier(t, srv.URL, "secret-shh", c.Now)
	ok1, _, err := v.Verify(context.Background(), "token-A", "203.0.113.5")
	if err != nil || !ok1 {
		t.Fatalf("first Verify ok=%v err=%v; want ok=true", ok1, err)
	}
	ok2, reason, err := v.Verify(context.Background(), "token-A", "203.0.113.5")
	if err != nil {
		t.Fatalf("second Verify: unexpected err = %v", err)
	}
	if ok2 {
		t.Errorf("second Verify with same token allowed; want reject (replay)")
	}
	if reason != "duplicate" {
		t.Errorf("reason = %q; want duplicate", reason)
	}
}

func TestTurnstile_ReplaySet_EvictsAfterTTL(t *testing.T) {
	c := newClock()
	srv := httptest.NewServer(turnstileSuccessHandler(t, true, nil, nil, nil))
	defer srv.Close()

	v := newTurnstileVerifier(t, srv.URL, "secret-shh", c.Now)
	v.Verify(context.Background(), "token-A", "203.0.113.5")

	// Advance past the 5-minute replay TTL.
	c.advance(6 * time.Minute)
	ok, _, err := v.Verify(context.Background(), "token-A", "203.0.113.5")
	if err != nil {
		t.Fatalf("Verify after TTL: unexpected err = %v", err)
	}
	if !ok {
		t.Error("Verify after replay-TTL rejected; entry should have been evicted")
	}
}

func TestTurnstile_Non2xx_ReturnsErr(t *testing.T) {
	c := newClock()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal trouble"))
	}))
	defer srv.Close()

	v := newTurnstileVerifier(t, srv.URL, "secret-shh", c.Now)
	ok, _, err := v.Verify(context.Background(), "tok", "203.0.113.5")
	if err == nil {
		t.Fatal("Verify on 5xx: err = nil; want non-nil (initHandler maps this to 502)")
	}
	if ok {
		t.Error("Verify on 5xx returned ok=true; want false")
	}
}

func TestTurnstile_SecretNeverInError(t *testing.T) {
	c := newClock()
	// Mock returns the secret IN the response body (the L005 echo case).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		secret := r.Form.Get("secret")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("siteverify rejected key " + secret + " — please rotate"))
	}))
	defer srv.Close()

	const secret = "ULTRA-SECRET-12345"
	v := newTurnstileVerifier(t, srv.URL, secret, c.Now)
	_, _, err := v.Verify(context.Background(), "tok", "203.0.113.5")
	if err == nil {
		t.Fatal("want non-nil err")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("err leaks secret: %q", err.Error())
	}
}

func TestTurnstile_HTTPClientTimeout_ReturnsErr(t *testing.T) {
	c := newClock()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}
	v, err := captcha.NewTurnstileWithURL(srv.URL, "s", client, c.Now)
	if err != nil {
		t.Fatalf("NewTurnstileWithURL: %v", err)
	}
	_, _, verr := v.Verify(context.Background(), "tok", "203.0.113.5")
	var nerr net.Error
	_ = nerr
	if verr == nil {
		t.Fatal("Verify with sub-100ms timeout: err = nil; want non-nil")
	}
	if !errors.Is(verr, context.DeadlineExceeded) && !strings.Contains(verr.Error(), "Client.Timeout") && !strings.Contains(verr.Error(), "deadline exceeded") {
		// Either go's net error shape is acceptable; we just want SOMETHING.
		t.Logf("verr = %v (acceptable as long as it's non-nil)", verr)
	}
}

func TestTurnstile_RemoteIPForwarded(t *testing.T) {
	c := newClock()
	var capturedRemoteIP string
	srv := httptest.NewServer(turnstileSuccessHandler(t, true, nil, nil, &capturedRemoteIP))
	defer srv.Close()

	v := newTurnstileVerifier(t, srv.URL, "secret-shh", c.Now)
	_, _, _ = v.Verify(context.Background(), "tok", "203.0.113.5")
	if capturedRemoteIP != "203.0.113.5" {
		t.Errorf("siteverify received remoteip = %q; want %q", capturedRemoteIP, "203.0.113.5")
	}
}

func TestStub_AlwaysAllows(t *testing.T) {
	s := captcha.Stub{}
	ok, _, err := s.Verify(context.Background(), "anything", "anywhere")
	if err != nil || !ok {
		t.Errorf("Stub.Verify = (%v, _, %v); want (true, _, nil)", ok, err)
	}
	if got := s.Provider(); got != "stub" {
		t.Errorf("Provider() = %q; want stub", got)
	}
}

func TestNew_UnknownProvider_Errors(t *testing.T) {
	c := newClock()
	_, err := captcha.New("recaptcha", "secret", nil, c.Now)
	if err == nil {
		t.Fatal("New(\"recaptcha\", ...) returned nil err; want error (only turnstile + hcaptcha supported)")
	}
}

func TestNew_TurnstileProvider_Works(t *testing.T) {
	c := newClock()
	v, err := captcha.New("turnstile", "secret", nil, c.Now)
	if err != nil {
		t.Fatalf("New(turnstile): %v", err)
	}
	if v.Provider() != "turnstile" {
		t.Errorf("Provider() = %q; want turnstile", v.Provider())
	}
}

func TestNew_HCaptchaProvider_Works(t *testing.T) {
	c := newClock()
	v, err := captcha.New("hcaptcha", "secret", nil, c.Now)
	if err != nil {
		t.Fatalf("New(hcaptcha): %v", err)
	}
	if v.Provider() != "hcaptcha" {
		t.Errorf("Provider() = %q; want hcaptcha", v.Provider())
	}
}
```

Add this `net` import at the top of `captcha_test.go` (needed by `nerr` declaration):

```go
import (
	"net"
	// ... others
)
```

(Or remove the unused `nerr` variable — the test only checks the err is non-nil; the `net.Error` lines are illustrative.)

Create the test-only export `relay/internal/captcha/export_test.go`:

```go
package captcha

import (
	"errors"
	"net/http"
	"time"
)

// NewTurnstileWithURL is a test-only constructor that overrides the
// siteverify URL. Production callers must use New("turnstile", ...).
func NewTurnstileWithURL(siteverifyURL, secret string, httpClient *http.Client, now func() time.Time) (Verifier, error) {
	if siteverifyURL == "" {
		return nil, errors.New("NewTurnstileWithURL: siteverifyURL required")
	}
	return newProviderVerifier("turnstile", siteverifyURL, secret, httpClient, now), nil
}

// NewHCaptchaWithURL is the hCaptcha counterpart.
func NewHCaptchaWithURL(siteverifyURL, secret string, httpClient *http.Client, now func() time.Time) (Verifier, error) {
	if siteverifyURL == "" {
		return nil, errors.New("NewHCaptchaWithURL: siteverifyURL required")
	}
	return newProviderVerifier("hcaptcha", siteverifyURL, secret, httpClient, now), nil
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/captcha/... -v && cd ..`
Expected: FAIL — `captcha.NewTurnstileWithURL` undefined; the placeholder New returns an error.

- [ ] **Step 3: Replace `captcha.go` with the full implementation**

Overwrite `relay/internal/captcha/captcha.go`:

```go
// Package captcha verifies CAPTCHA tokens at /v1/intake/init.
// Supports Cloudflare Turnstile and hCaptcha via their siteverify endpoints.
// Layers a 5-minute single-use replay-protection set on top of the providers'
// own semantics (defense in depth).
//
// L005: the secret is NEVER included in any returned error. The siteverify
// response body is logged at Debug level only, AFTER redact-before-truncate.
// L014: injectable clock for deterministic replay-set eviction.
package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	turnstileSiteverify = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	hcaptchaSiteverify  = "https://hcaptcha.com/siteverify"
	defaultHTTPTimeout  = 5 * time.Second
	replayTTL           = 5 * time.Minute
	bodyTruncateRunes   = 200
)

// Verifier verifies a CAPTCHA token via the provider's siteverify endpoint.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) (ok bool, reason string, err error)
	Provider() string
}

// Stub always returns ok=true. Used when captcha is disabled or when the
// current auth mode is not in required_for.
type Stub struct{}

func (Stub) Verify(context.Context, string, string) (bool, string, error) {
	return true, "", nil
}
func (Stub) Provider() string { return "stub" }

// providerVerifier is the shared implementation for Turnstile + hCaptcha.
type providerVerifier struct {
	name     string // "turnstile" | "hcaptcha"
	url      string // siteverify endpoint
	secret   string
	client   *http.Client
	now      func() time.Time
	mu       sync.Mutex
	replay   map[string]time.Time
}

// New constructs the configured verifier. provider is "turnstile" or "hcaptcha".
// secret is the resolved value (caller already ran config.ResolveSecret).
// httpClient defaults to one with a 5s timeout when nil.
// now is injectable for tests (production: time.Now).
func New(provider, secret string, httpClient *http.Client, now func() time.Time) (Verifier, error) {
	if secret == "" {
		return nil, errors.New("captcha.New: secret is empty (resolve via config.ResolveSecret before calling)")
	}
	switch provider {
	case "turnstile":
		return newProviderVerifier(provider, turnstileSiteverify, secret, httpClient, now), nil
	case "hcaptcha":
		return newProviderVerifier(provider, hcaptchaSiteverify, secret, httpClient, now), nil
	default:
		return nil, fmt.Errorf("captcha.New: unsupported provider %q (only \"turnstile\" and \"hcaptcha\" are supported in v0)", provider)
	}
}

// newProviderVerifier is the shared constructor used by production New + test-only
// NewTurnstileWithURL/NewHCaptchaWithURL.
func newProviderVerifier(name, siteverifyURL, secret string, httpClient *http.Client, now func() time.Time) *providerVerifier {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if now == nil {
		now = time.Now
	}
	return &providerVerifier{
		name:   name,
		url:    siteverifyURL,
		secret: secret,
		client: httpClient,
		now:    now,
		replay: make(map[string]time.Time),
	}
}

func (v *providerVerifier) Provider() string { return v.name }

// siteverifyResponse is the common shape returned by Turnstile + hCaptcha.
type siteverifyResponse struct {
	Success     bool     `json:"success"`
	ErrorCodes  []string `json:"error-codes"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
}

// Verify implements the Verifier interface.
func (v *providerVerifier) Verify(ctx context.Context, token, remoteIP string) (bool, string, error) {
	if token == "" {
		return false, "missing-input-response", nil
	}

	// Replay-protection: pre-check + eager eviction.
	if !v.markUnseenOrEvict(token) {
		return false, "duplicate", nil
	}

	// POST form to siteverify.
	form := url.Values{}
	form.Set("secret", v.secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.url, strings.NewReader(form.Encode()))
	if err != nil {
		// Don't leak secret in the error wrapping — req URL is fine; body is form-encoded inside req.
		return false, "", fmt.Errorf("captcha siteverify: build request: %w", v.redactErr(err))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("captcha siteverify: transport: %w", v.redactErr(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// L005: redact secret from any body before wrapping into the error.
		_, _ = io.Copy(io.Discard, resp.Body) // drain but do NOT include body in error
		return false, "", fmt.Errorf("captcha siteverify: HTTP %d", resp.StatusCode)
	}

	var body siteverifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, "", fmt.Errorf("captcha siteverify: decode body: %w", v.redactErr(err))
	}

	if !body.Success {
		// Return the first error code as `reason`; if there are none, return a generic.
		reason := "siteverify-rejected"
		if len(body.ErrorCodes) > 0 {
			reason = body.ErrorCodes[0]
		}
		return false, reason, nil
	}
	return true, "", nil
}

// markUnseenOrEvict checks the replay set for token and atomically marks it
// "seen". Returns true if the token is fresh (not in the set, or its prior
// entry has expired and was evicted), false if it's a within-TTL replay.
// Also performs an eager full-scan eviction of expired entries.
func (v *providerVerifier) markUnseenOrEvict(token string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	cutoff := now.Add(-replayTTL)

	// Sweep expired entries.
	for k, ts := range v.replay {
		if ts.Before(cutoff) {
			delete(v.replay, k)
		}
	}

	if ts, ok := v.replay[token]; ok && !ts.Before(cutoff) {
		return false // within-TTL replay
	}
	v.replay[token] = now
	return true
}

// redactErr scrubs v.secret from err.Error() if it appears verbatim. The
// returned error preserves the underlying type (for errors.Is/As) when the
// secret does not appear; otherwise a fresh error is returned with the secret
// replaced by "[REDACTED]". Defense in depth — most callers should never
// include the secret in an error to begin with.
func (v *providerVerifier) redactErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if !strings.Contains(msg, v.secret) {
		return err
	}
	// secret present → wrap in a fresh error with the secret redacted.
	redacted := strings.ReplaceAll(msg, v.secret, "[REDACTED]")
	return errors.New(redacted)
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test -race ./internal/captcha/... -v && cd ..`
Expected: all 12 tests pass under `-race`.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/captcha/captcha.go relay/internal/captcha/captcha_test.go relay/internal/captcha/export_test.go
git commit -m "feat(5-iii): captcha.New + Turnstile + HCaptcha + replay-set; L005 redact-before-error"
```

---

### Task 2: Wire `deps.CaptchaVerifier` into `initHandler`

**Files:** Modify `relay/internal/server/turn.go`, `relay/internal/server/turn_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/server/turn_test.go`:

```go
// fakeVerifier is a test double for captcha.Verifier. The behavior is set
// per test instance.
type fakeVerifier struct {
	ok     bool
	reason string
	err    error
	calls  int
}

func (f *fakeVerifier) Verify(ctx context.Context, token, remoteIP string) (bool, string, error) {
	f.calls++
	return f.ok, f.reason, f.err
}
func (f *fakeVerifier) Provider() string { return "fake" }

func TestInitHandler_WithValidCaptchaToken_MintsSession(t *testing.T) {
	v := &fakeVerifier{ok: true}
	deps := Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
		CaptchaVerifier: v,
	}
	h := initHandler(deps)

	body := `{"captcha_token":"tok-123"}`
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp InitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SessionID == "" {
		t.Error("session_id missing")
	}
	if v.calls != 1 {
		t.Errorf("Verifier.Verify called %d times; want 1", v.calls)
	}
}

func TestInitHandler_InvalidCaptchaToken_Returns401(t *testing.T) {
	v := &fakeVerifier{ok: false, reason: "invalid-input-response"}
	deps := Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
		CaptchaVerifier: v,
	}
	h := initHandler(deps)

	body := `{"captcha_token":"bad"}`
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	// Body shape: standard ErrorEnvelope plus "reason" field.
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errBody, ok := raw["error"].(map[string]any)
	if !ok {
		t.Fatalf("error key missing; body: %s", rec.Body.String())
	}
	if errBody["code"] != "captcha_failed" {
		t.Errorf("code = %v; want captcha_failed", errBody["code"])
	}
	if errBody["reason"] != "invalid-input-response" {
		t.Errorf("reason = %v; want invalid-input-response", errBody["reason"])
	}
}

func TestInitHandler_CaptchaVerifierErr_Returns502(t *testing.T) {
	v := &fakeVerifier{err: errors.New("upstream-flaky")}
	deps := Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
		CaptchaVerifier: v,
	}
	h := initHandler(deps)

	body := `{"captcha_token":"tok"}`
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d; want 502", rec.Code)
	}
	var body2 ErrorEnvelope
	json.Unmarshal(rec.Body.Bytes(), &body2)
	if body2.Error.Code != "captcha_unavailable" {
		t.Errorf("code = %q; want captcha_unavailable", body2.Error.Code)
	}
}

func TestInitHandler_CaptchaTokenIgnoredWhenNotRequired(t *testing.T) {
	// captcha.enabled=false → token in body is ignored; verifier never called.
	v := &fakeVerifier{ok: false, err: errors.New("would never be called")}
	deps := Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg:      config.CaptchaConfig{Enabled: false},
		CaptchaVerifier: v,
	}
	h := initHandler(deps)

	body := `{"captcha_token":"tok"}`
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (captcha disabled)", rec.Code)
	}
	if v.calls != 0 {
		t.Errorf("Verifier called %d times; want 0 (captcha disabled)", v.calls)
	}
}
```

Add to imports: `"errors"` (if not already imported).

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/server/ -run TestInitHandler -v && cd ..`
Expected: FAIL — the 5-i initHandler does not actually call CaptchaVerifier when a token IS present.

- [ ] **Step 3: Extend `initHandler` with the verify branch**

In `relay/internal/server/turn.go`, find the marker comment that begins `// 5-iii: verify the captcha token now that we know it's present and required.` inside `initHandler` (added in 5-i; sits between the 400-captcha_required branch and `sessionID := deps.Auth.Store().Issue()`). Replace the full multi-line marker comment with this block:

```go
		// 5-iii: verify the captcha token now that we know it's present and required.
		if len(requiresCaptcha) > 0 && initReq.CaptchaToken != "" && deps.CaptchaVerifier != nil {
			clientIP := ClientIPFromContext(r.Context())
			ok, reason, err := deps.CaptchaVerifier.Verify(r.Context(), initReq.CaptchaToken, clientIP)
			if err != nil {
				slog.WarnContext(r.Context(), "captcha siteverify unavailable", "provider", deps.CaptchaVerifier.Provider(), "err", err)
				writeError(w, http.StatusBadGateway, "captcha_unavailable", "captcha verification provider unavailable")
				return
			}
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"error": map[string]any{
						"code":    "captcha_failed",
						"message": "captcha verification failed",
						"reason":  reason,
					},
				})
				return
			}
		}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/server/ -v && cd ..`
Expected: all existing tests + the 4 new TestInitHandler captcha tests pass.

Run: `cd relay && go test ./... && cd ..`
Expected: ALL relay tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/server/turn.go relay/internal/server/turn_test.go
git commit -m "feat(5-iii): initHandler verifies captcha_token via deps.CaptchaVerifier (200/401/502)"
```

---

### Task 3: Wire real `captcha.Verifier` in `main.go`

**Files:** Modify `relay/cmd/relay/main.go`

- [ ] **Step 1: Add the CaptchaVerifier construction block**

In `relay/cmd/relay/main.go`, find the existing 5-ii block where `perIPLimiter` and `budgetTracker` are constructed. AFTER `budgetTracker`, ADD:

```go
	// Phase 5 (5-iii): construct the captcha verifier when enabled.
	var captchaVerifier captcha.Verifier
	if cfg.Captcha.Enabled {
		if cfg.Captcha.SecretKeyEnv == "" {
			logger.Error("relay: captcha.enabled=true requires captcha.secret_key_env")
			os.Exit(1)
		}
		secret, err := config.RequireSecret(cfg.Captcha.SecretKeyEnv)
		if err != nil {
			logger.Error("captcha: resolve secret", "env", cfg.Captcha.SecretKeyEnv, "err", err)
			os.Exit(1)
		}
		v, err := captcha.New(cfg.Captcha.Provider, secret, nil, time.Now)
		if err != nil {
			logger.Error("captcha: construct verifier", "provider", cfg.Captcha.Provider, "err", err)
			os.Exit(1)
		}
		captchaVerifier = v
		// Log NEVER includes the secret; provider + site_key + required_for are safe.
		logger.Info("relay: captcha enabled",
			"provider", cfg.Captcha.Provider,
			"required_for", cfg.Captcha.RequiredFor,
		)
	}
```

In the `deps := server.Deps{...}` block, REPLACE this line:

```go
		CaptchaVerifier: nil,             // 5-iii lands the real verifier
```

With:

```go
		CaptchaVerifier: captchaVerifier, // 5-iii: nil when cfg.Captcha.Enabled=false; real verifier otherwise
```

Add the import at the top of `main.go` if not already present:

```go
import (
	// ... existing ...
	"intake/internal/captcha"
)
```

- [ ] **Step 2: Build + vet + tests**

Run: `cd relay && go build ./... && go vet ./... && cd ..`
Expected: passes.

Run: `cd relay && go test ./... && cd ..`
Expected: ALL relay tests pass.

Run: `bash scripts/verify-contract.sh && bash scripts/check-pins.sh`
Expected: both exit 0.

- [ ] **Step 3: Commit**

```bash
git add relay/cmd/relay/main.go
git commit -m "feat(5-iii): wire real captcha.Verifier in main.go (gated on cfg.Captcha.Enabled)"
```

---

## Smoke (mandatory)

**Steps 1-3 are unit-layer; step 4 is the live CAPTCHA smoke that PAUSES for the maintainer.** Cloudflare publishes documented test sitekeys + secrets that always pass / always fail, so the live smoke needs zero human interaction — but it does require setting an env var with the test secret, which is why we pause.

1. **Unit tests cover the verifier matrix** (covered in Task 1):
   - Turnstile + hCaptcha happy path
   - success:false → reason from error-codes[0]
   - 5xx from siteverify → err (initHandler maps to 502)
   - replay within 5min → reason="duplicate"
   - secret never appears in any error
   - timeout
   - hCaptcha provider works through the same `New` factory

2. **Unit tests cover the /init dispatcher matrix** (covered in Task 2):
   - 400 captcha_required (missing token + required) — already verified in 5-i
   - 200 + session_id (valid token)
   - 401 captcha_failed (invalid token)
   - 502 captcha_unavailable (verifier err)
   - captcha disabled → token ignored, no verifier call

3. **Q9 startup gate covers captcha config correctness** (covered in 5-i):
   - `captcha.enabled=true` but `captcha.required_for` excludes `"anonymous"` (and `auth.modes.anonymous=true` + no escape hatch) → startup fatal

4. **Live CAPTCHA smoke (PAUSE for maintainer go-ahead).** Cloudflare's documented test keys:

   - **Always-passes sitekey:** `1x00000000000000000000AA`
   - **Always-passes secret:** `1x0000000000000000000000000000000AA`
   - **Always-fails sitekey:** `2x00000000000000000000AB`
   - **Always-fails secret:** `2x0000000000000000000000000000000AA`

   Create `relay/cmd/relay/smoke/captcha-live.yaml`:
   ```yaml
   server:
     addr: ":18080"
     external_url: "http://127.0.0.1:18080"
     cors_origins: ["http://localhost:5173"]
   auth:
     modes:
       anonymous: true
   captcha:
     enabled: true
     provider: "turnstile"
     site_key: "1x00000000000000000000AA"
     secret_key_env: "INTAKE_TURNSTILE_SECRET"
     required_for: ["anonymous"]
   ratelimit:
     daily_llm_budget:
       action_on_exceeded: "reject"
   ```
   In the maintainer's shell:
   ```bash
   export INTAKE_TURNSTILE_SECRET="1x0000000000000000000000000000000AA"
   cd relay && go run ./cmd/relay --config smoke/captcha-live.yaml &
   ```

   a. **Discovery call:** `curl -s -X POST -H "Content-Type: application/json" http://127.0.0.1:18080/v1/intake/init -d '{}'`
      Expected: 400 + body contains `"code":"captcha_required"`, `"capabilities":{"requires_captcha":["anonymous"]}`, `"captcha":{"provider":"turnstile","site_key":"1x00000000000000000000AA"}`.

   b. **Valid token mint** — Cloudflare's docs say any non-empty `response` against the always-passes secret yields `success:true`. So:
      `curl -s -X POST -H "Content-Type: application/json" http://127.0.0.1:18080/v1/intake/init -d '{"captcha_token":"XXXX.DUMMY.TOKEN.XXXX"}'`
      Expected: 200 + `session_id` is a non-empty UUID.

   c. **Replay test:** repeat (b) with the same `captcha_token` value. Expected: 401 + `"reason":"duplicate"` (the replay-protection set caught it before siteverify).

   d. **Always-fails secret:** restart the relay with `INTAKE_TURNSTILE_SECRET=2x0000000000000000000000000000000AA` (the always-fails key). Repeat (b) with a fresh token value. Expected: 401 + `"reason":"invalid-input-response"` (or similar — whatever Turnstile's always-fails secret returns).

   e. **hCaptcha equivalent:** repeat with `provider: "hcaptcha"`, `site_key: "10000000-ffff-ffff-ffff-000000000001"`, `secret: "0x0000000000000000000000000000000000000000"`  (hCaptcha's published test sitekey + secret). Expected: the same 4 outcomes.

   Document each curl output in `ai/tasks/phase-5/SMOKE-EVIDENCE.md`.

## Done criteria

- [ ] All 3 tasks complete and committed.
- [ ] `cd relay && go build ./... && go vet ./...` clean.
- [ ] `cd relay && go test -race ./...` green.
- [ ] All 12 `captcha_test.go` tests pass under `-race`.
- [ ] All 4 new `initHandler` captcha tests pass.
- [ ] `bash scripts/verify-contract.sh && bash scripts/check-pins.sh` both exit 0.
- [ ] Live CAPTCHA smoke executed (maintainer-paused) with all 4 outcomes recorded.
- [ ] No new external Go module added (`go mod tidy` is a no-op).
- [ ] No log line contains the CAPTCHA secret (verified by grepping the live smoke logs for the test secret substring).
