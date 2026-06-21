package sso

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/clubpetey/openintake/relay/internal/auth"
	"github.com/clubpetey/openintake/relay/internal/config"
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

// Verify parses the token (with HS256 pinning and 30s clock-skew leeway), then
// runs the shared iss/aud and claim-mapping checks.
func (v *HS256Verifier) Verify(ctx context.Context, token string) (*auth.SSOClaims, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(
		token,
		claims,
		func(t *jwt.Token) (any, error) {
			return v.secret, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithLeeway(clockSkew*time.Second),
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
