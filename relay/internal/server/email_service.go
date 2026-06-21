package server

import (
	"context"
	"errors"
	"time"

	"github.com/clubpetey/openintake/relay/internal/auth/emailcode"
	"github.com/clubpetey/openintake/relay/internal/auth/emailjwt"
	"github.com/clubpetey/openintake/relay/internal/auth/smtpsend"
)

// EmailService is the small orchestrator that the /auth/email/start and
// /auth/email/verify handlers consume. It owns the codestore + sender + the
// JWT secret/TTL, and exposes two methods that wrap the underlying flows.
//
// Constructed once in main.go when auth.modes.email is true.
type EmailService struct {
	codes  *emailcode.Store
	sender smtpsend.Sender
	secret []byte
	jwtTTL time.Duration
}

// ErrSMTP wraps an underlying SMTP send error; handlers translate this to 502.
// The error never contains the SMTP password.
var ErrSMTP = errors.New("email: smtp send failed")

// NewEmailService wires the components. secret MUST be ≥32 bytes (caller
// validates at startup — emailjwt.Mint will additionally guard).
func NewEmailService(codes *emailcode.Store, sender smtpsend.Sender, secret []byte, jwtTTL time.Duration) *EmailService {
	return &EmailService{
		codes:  codes,
		sender: sender,
		secret: secret,
		jwtTTL: jwtTTL,
	}
}

// IssueAndSend issues a code for email and sends it via the sender. Returns
// (retryAfter, ErrRateLimited) when rate-limited, or (0, ErrSMTP-wrapped) when
// the sender fails. The handler maps these to 429 / 502.
//
// Caller is responsible for log-scrubbing — this method never logs the code.
func (e *EmailService) IssueAndSend(ctx context.Context, email string) (retryAfter time.Duration, err error) {
	code, retryAfter, err := e.codes.Issue(email)
	if err != nil {
		return retryAfter, err
	}
	if err := e.sender.Send(ctx, email, code); err != nil {
		return 0, errors.Join(ErrSMTP, err)
	}
	return 0, nil
}

// VerifyAndMint verifies code against email and mints a JWT on success.
// Returns (token, expiresAt, nil) on success; (_, _, error) on
// invalid/expired/used code (handler maps to a generic 401).
func (e *EmailService) VerifyAndMint(email, code string) (string, time.Time, error) {
	if !e.codes.Verify(email, code) {
		return "", time.Time{}, errors.New("email: invalid or expired code")
	}
	return emailjwt.Mint(e.secret, email, e.jwtTTL)
}
