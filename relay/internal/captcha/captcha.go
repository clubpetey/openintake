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
	name   string // "turnstile" | "hcaptcha"
	url    string // siteverify endpoint
	secret string
	client *http.Client
	now    func() time.Time
	mu     sync.Mutex
	replay map[string]time.Time
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
		// Don't leak secret in the error wrapping.
		return false, "", fmt.Errorf("captcha siteverify: build request: %w", v.redactErr(err))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("captcha siteverify: transport: %w", v.redactErr(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// L005: drain but do NOT include body in error.
		_, _ = io.Copy(io.Discard, resp.Body)
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
	redacted := strings.ReplaceAll(msg, v.secret, "[REDACTED]")
	return errors.New(redacted)
}
