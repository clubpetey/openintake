package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Middleware is a chi-compatible HTTP middleware that resolves session identity.
//
// Resolution order (4-i):
//  1. Authorization: Bearer <token>:
//       a. If email mode is enabled (m.email != nil), try emailjwt.Verify; on
//          success, attach SessionContext{AuthMode:"email", Verified:true, Email}.
//       b. Else fall through to SSO; if sso mode is enabled (m.sso != nil), try
//          ssoVerifier.Verify; on success, attach SessionContext{AuthMode:"sso",
//          Verified:true, UserID, Email?, DisplayName?, Custom}.
//       c. Bearer present but no verifier accepted → 401 unauthorized.
//          (A present-but-invalid bearer is NEVER silently downgraded to anonymous.)
//  2. No Authorization header:
//       a. X-Intake-Session present + store.Validate → SessionContext{AuthMode:"anonymous"}.
//       b. Else → 401.
//
// The /init endpoint is NOT behind this middleware (it issues anonymous sessions).
// The /auth/email/start and /auth/email/verify endpoints are ALSO not behind this
// middleware (they bootstrap email JWTs — see sub-plan 4-ii).
type Middleware struct {
	store *Store
	email EmailJWTVerifier // nil → email mode off
	sso   SSOVerifier      // nil → sso mode off
}

// NewMiddleware returns a Middleware backed by the given Store. email and sso
// are optional; pass nil to disable the corresponding bearer-token validator.
func NewMiddleware(store *Store, email EmailJWTVerifier, sso SSOVerifier) *Middleware {
	return &Middleware{store: store, email: email, sso: sso}
}

// Store returns the underlying session store. Used by initHandler to issue
// anonymous sessions.
func (m *Middleware) Store() *Store { return m.store }

// Handler wraps next with identity resolution. chi-compatible.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authHeader := r.Header.Get("Authorization"); len(authHeader) >= 7 && strings.EqualFold(authHeader[:7], "bearer ") {
			token := strings.TrimSpace(authHeader[7:])

			// Try email-mode JWT first (cheap-fail HS256).
			if m.email != nil {
				if email, err := m.email.Verify(token); err == nil {
					emailCopy := email // avoid taking the address of the named return variable (style)
					ctx := WithSession(r.Context(), &SessionContext{
						AuthMode: "email",
						Verified: true,
						Email:    &emailCopy,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Fall through to SSO.
			if m.sso != nil {
				if claims, err := m.sso.Verify(r.Context(), token); err == nil {
					userID := claims.UserID
					sc := &SessionContext{
						AuthMode:    "sso",
						Verified:    true,
						UserID:      &userID,
						Email:       claims.Email,
						DisplayName: claims.DisplayName,
						Custom:      claims.Custom,
					}
					next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sc)))
					return
				}
			}

			// Bearer present but neither verifier accepted it.
			authWriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "invalid bearer token",
				},
			})
			return
		}

		// Anonymous fallback.
		sessionID := r.Header.Get("X-Intake-Session")
		if sessionID == "" || !m.store.Validate(sessionID) {
			authWriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "missing or invalid X-Intake-Session header; call POST /v1/intake/init first",
				},
			})
			return
		}

		ctx := WithSession(r.Context(), &SessionContext{
			SessionID: sessionID,
			AuthMode:  "anonymous",
			Verified:  false,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authWriteJSON writes a JSON-encoded body with the given status code.
// Named authWriteJSON to avoid conflict with server.writeJSON in the server package.
func authWriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
