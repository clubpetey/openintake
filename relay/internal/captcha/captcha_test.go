package captcha_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/captcha"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
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
	if verr == nil {
		t.Fatal("Verify with sub-100ms timeout: err = nil; want non-nil")
	}
	// Don't pin the specific error type — Go's http client may wrap the deadline
	// as net.Error, context.DeadlineExceeded, etc.
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
