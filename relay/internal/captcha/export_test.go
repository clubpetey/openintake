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
