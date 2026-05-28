package sso

import (
	"context"
	"errors"
	"fmt"

	"intake/internal/auth"
	"intake/internal/config"
)

// HS256Verifier — full impl in Task 4.
type HS256Verifier struct{}

// NewHS256Verifier rejects secrets shorter than 32 bytes. Full Verify impl
// arrives in Task 4.
func NewHS256Verifier(cfg config.SSOConfig, secret []byte) (*HS256Verifier, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("sso: HS256 secret must be at least 32 bytes (got %d)", len(secret))
	}
	return nil, errors.New("sso: HS256Verifier not yet implemented (Task 4)")
}

func (*HS256Verifier) Verify(ctx context.Context, token string) (*auth.SSOClaims, error) {
	return nil, errors.New("sso: HS256Verifier not yet implemented (Task 4)")
}
