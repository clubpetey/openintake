package auth

import "context"

// SSOClaims is the per-request identity surface produced by an SSOVerifier.
// Maps onto SessionContext.{UserID, Email, DisplayName, Custom} when the SSO
// branch of the middleware dispatcher succeeds.
//
// UserID is always populated (claim name configurable via auth.sso.claims.user_id,
// default "sub"); Email and DisplayName are optional pointers (the configured
// claim may be absent in the token); Custom carries any additional claims the
// caller passed through (not used by the relay today; reserved for v1+).
type SSOClaims struct {
	UserID      string
	Email       *string
	DisplayName *string
	Custom      map[string]any
}

// EmailJWTVerifier verifies an email-mode JWT and returns the verified email
// from its sub claim. Implementations MUST reject tokens whose iss is not
// "intake-email" (so an SSO token can't sneak through the email branch), MUST
// reject expired tokens, and MUST NOT include the secret in any returned error.
//
// Implemented by *emailjwt.Verifier (sub-plan 4-ii).
type EmailJWTVerifier interface {
	Verify(token string) (email string, err error)
}

// SSOVerifier verifies a host-app-issued SSO JWT and returns the mapped claims.
// Implementations MUST pin the JWT algorithm (RS256 implementations reject HS256
// tokens and vice versa — mitigates alg-confusion attacks), MUST validate iss /
// aud / exp / nbf (with 30s clock-skew), and MUST NOT include the secret/JWKS
// content in any returned error.
//
// Implemented by *sso.RS256Verifier and *sso.HS256Verifier (sub-plan 4-iii).
type SSOVerifier interface {
	Verify(ctx context.Context, token string) (*SSOClaims, error)
}
