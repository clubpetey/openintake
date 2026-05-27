package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Middleware is a chi-compatible HTTP middleware that resolves session identity.
//
// Resolution order (per design spec §5.3):
//  1. Authorization: Bearer <token> → 501 Not Implemented.
//     Phase 4 seam: replace this block with JWT validation against the
//     configured issuer/audience/JWKS endpoint. The handler below the
//     middleware can then call FromContext to get a Verified=true session.
//  2. X-Intake-Session: <id> present AND store.Validate(id) → attach anonymous
//     SessionContext{AuthMode:"anonymous", Verified:false} via WithSession.
//  3. Else → 401 Unauthorized.
//
// The /init endpoint is NOT behind this middleware (it issues the session).
type Middleware struct {
	store *Store
}

// NewMiddleware returns a Middleware backed by the given Store.
func NewMiddleware(store *Store) *Middleware {
	return &Middleware{store: store}
}

// Store returns the underlying session store.
// Used by initHandler to issue sessions.
func (m *Middleware) Store() *Store {
	return m.store
}

// Handler wraps next with identity resolution. It is chi-compatible: use as
//
//	r.With(deps.Auth.Handler).Post("/v1/intake/turn", turnHandler)
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Phase 4 seam: JWT resolver.
		// When Phase 4 lands, replace this block with JWKS-based validation.
		// The resolver should populate SessionContext{AuthMode:"email"|"sso",
		// Verified:true, UserID, Email, DisplayName} and call WithSession before
		// invoking next.
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			authWriteJSON(w, http.StatusNotImplemented, map[string]any{
				"error": map[string]any{
					"code":    "jwt_not_implemented",
					"message": "JWT auth is not implemented until Phase 4; use anonymous session via /init",
				},
			})
			return
		}

		// Anonymous resolver.
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
