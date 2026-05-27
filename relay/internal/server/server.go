package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"intake/internal/config"
)

// New constructs the relay HTTP handler (a chi Mux) with all middleware and
// built-in routes wired. Routes specific to intake sessions are registered via
// registerIntakeRoutes, which 1-iii and 1-iv extend.
func New(cfg *config.Config, deps Deps) http.Handler {
	r := chi.NewMux()

	// Global middleware — order matters: request-ID first, then recovery.
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(deps.CORSOrigins))

	// Built-in relay endpoints.
	r.Get("/v1/health", handleHealth)
	r.Get("/v1/version", handleVersion(deps))

	// Intake session endpoints — seam for 1-iii and 1-iv.
	r.Route("/v1/intake", func(r chi.Router) {
		registerIntakeRoutes(r, deps)
	})

	return r
}

// handleHealth returns a minimal liveness probe response.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleVersion returns build info as JSON.
func handleVersion(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, deps.Version)
	}
}

// corsMiddleware returns a middleware that enforces a strict CORS allowlist.
// It sets Access-Control-Allow-Origin ONLY for origins that appear in the list.
// No wildcard is ever set. Preflight OPTIONS requests are handled directly.
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.ToLower(o)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Always vary on Origin so caches key on it correctly.
			w.Header().Add("Vary", "Origin")

			// Compute the allow decision once.
			_, originAllowed := allowed[strings.ToLower(origin)]

			// Set CORS response headers only when the origin is known and allowed.
			if origin != "" && originAllowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers",
					"Content-Type, X-Intake-Session, Authorization, X-Request-Id")
				w.Header().Set("Access-Control-Allow-Methods",
					"GET, POST, OPTIONS")
			}

			// Handle preflight (OPTIONS).
			// - allowed origin → 204 No Content (CORS preflight complete).
			// - disallowed origin → 403 Forbidden.
			// - no Origin header → not a CORS preflight; pass to router unchanged.
			if r.Method == http.MethodOptions {
				if origin != "" {
					if originAllowed {
						w.WriteHeader(http.StatusNoContent)
					} else {
						w.WriteHeader(http.StatusForbidden)
					}
					return
				}
				// origin == "": fall through to router.
			}

			next.ServeHTTP(w, r)
		})
	}
}
