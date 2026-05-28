// Package captcha verifies CAPTCHA tokens at /v1/intake/init.
// Phase 5-i exports the Verifier interface + Stub so the server chain compiles;
// 5-iii fills in the Turnstile + hCaptcha implementations.
package captcha

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Verifier verifies a CAPTCHA token via the provider's siteverify endpoint.
// Implementations MUST scrub the secret from any returned error (L005).
type Verifier interface {
	// Verify returns (ok=true, "", nil) on a valid, single-use token.
	// On (ok=false, reason, nil): reason carries the provider's error-codes[0] —
	// never the secret. err is reserved for transport/parse failures.
	Verify(ctx context.Context, token, remoteIP string) (ok bool, reason string, err error)

	// Provider returns "turnstile" | "hcaptcha" | "stub" for logging.
	Provider() string
}

// Stub always returns ok=true. Used when captcha is disabled or when the
// current auth mode is not in required_for.
type Stub struct{}

func (Stub) Verify(context.Context, string, string) (bool, string, error) {
	return true, "", nil
}
func (Stub) Provider() string { return "stub" }

// New: 5-iii implements Turnstile + hCaptcha. 5-i: this returns an error so
// any 5-i caller is forced into the nil-CaptchaVerifier path (which initHandler
// handles as "no captcha required").
func New(provider, secret string, httpClient *http.Client, now func() time.Time) (Verifier, error) {
	_ = provider
	_ = secret
	_ = httpClient
	_ = now
	return nil, errors.New("captcha.New: not implemented in 5-i; sub-plan 5-iii implements Turnstile + hCaptcha")
}
