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
//
// API quirk: keyfunc/v3's NewDefault sets NoErrorReturnFirstHTTPReq=true,
// which means a failed initial JWKS fetch is logged but NOT returned as an
// error. We override that to false via NewDefaultOverrideCtx so the relay
// fatals at startup on a misconfigured/unreachable JWKS URL, per the build-fail
// list (README §6, design spec §8).
func NewRS256Verifier(cfg config.SSOConfig, logger *slog.Logger) (*RS256Verifier, error) {
	if cfg.JWKSURL == "" {
		return nil, errors.New("sso: RS256 verifier requires cfg.jwks_url")
	}
	if logger == nil {
		logger = slog.Default()
	}
	// Force the underlying jwkset HTTP client to surface the initial fetch
	// error (default behavior swallows it). Without this override, a typo'd
	// JWKS URL would let the relay start and then 401 every SSO request.
	failFast := false
	kf, err := keyfunc.NewDefaultOverrideCtx(context.Background(), []string{cfg.JWKSURL}, keyfunc.Override{
		NoErrorReturnFirstHTTPReq: &failFast,
	})
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
