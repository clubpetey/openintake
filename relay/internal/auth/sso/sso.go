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

	"github.com/clubpetey/openintake/relay/internal/auth"
	"github.com/clubpetey/openintake/relay/internal/config"
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
func validateAndExtract(claims jwt.MapClaims, cfg config.SSOConfig) (*auth.SSOClaims, error) {
	// iss — exact match (NOT prefix). An attacker who controls a subdomain
	// must not be able to mint tokens accepted by a parent-domain iss check.
	iss, _ := claims["iss"].(string)

	// Defense in depth (L013-adjacent): explicitly reject email-mode JWTs even if
	// they somehow passed alg-pinning, so a misconfigured operator setting
	// auth.sso.issuer == "intake-email" cannot accidentally accept relay-minted
	// email JWTs as SSO bearers.
	if iss == "intake-email" { // matches emailjwt.Issuer constant
		return nil, fmt.Errorf("sso: rejecting email-mode iss")
	}

	// The %q here echoes an attacker-controllable string into the error,
	// but the dispatcher surfaces only a static "invalid bearer token" message
	// to the client — this detail is for operator logs only.
	if iss != cfg.Issuer {
		return nil, fmt.Errorf("sso: iss claim %q does not match configured issuer", iss)
	}

	// aud — RFC 7519 allows a string or an array of strings.
	if !audienceContains(claims["aud"], cfg.Audience) {
		return nil, errors.New("sso: aud claim does not contain configured audience")
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
