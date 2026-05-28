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

	now := time.Now().Truncate(time.Second)
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
