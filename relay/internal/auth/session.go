package auth

import "context"

// SessionContext carries per-request identity. Attached to the request context
// by the auth middleware via WithSession; retrieved by handlers via FromContext.
//
// Phase 1 populates only SessionID, AuthMode ("anonymous"), and Verified (false).
// UserID, Email, DisplayName, and Custom are reserved for Phase 4 (email/SSO).
type SessionContext struct {
	SessionID   string
	AuthMode    string // "anonymous" | "email" | "sso"
	Verified    bool
	UserID      *string
	Email       *string
	DisplayName *string
	Custom      map[string]any
}

type ctxKey struct{}

// FromContext returns the SessionContext attached by the auth middleware.
// Returns (nil, false) if no session has been attached.
func FromContext(ctx context.Context) (*SessionContext, bool) {
	s, ok := ctx.Value(ctxKey{}).(*SessionContext)
	return s, ok
}

// WithSession attaches a SessionContext to ctx. Used by the auth middleware.
func WithSession(ctx context.Context, s *SessionContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}
