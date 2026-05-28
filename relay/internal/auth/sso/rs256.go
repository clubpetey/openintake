package sso

import (
	"context"
	"errors"
	"log/slog"

	"intake/internal/auth"
	"intake/internal/config"
)

// RS256Verifier — full impl in Task 3.
type RS256Verifier struct{}

// NewRS256Verifier — full impl in Task 3.
func NewRS256Verifier(cfg config.SSOConfig, logger *slog.Logger) (*RS256Verifier, error) {
	return nil, errors.New("sso: RS256Verifier not yet implemented (Task 3)")
}

func (*RS256Verifier) Verify(ctx context.Context, token string) (*auth.SSOClaims, error) {
	return nil, errors.New("sso: RS256Verifier not yet implemented (Task 3)")
}
