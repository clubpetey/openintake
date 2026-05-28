package server

import "github.com/go-chi/chi/v5"

// registerIntakeRoutes mounts the /v1/intake/* handlers on r.
// r is already scoped to the /v1/intake prefix by the caller (server.New).
//
// 1-iii registers POST /init (no auth; issues a session) and POST /turn (behind auth).
// 1-iv adds POST /submit (behind auth).
// 4-ii adds POST /auth/email/start + /auth/email/verify (no auth — bootstrap email JWTs).
func registerIntakeRoutes(r chi.Router, deps Deps) {
	// POST /v1/intake/init — no auth; issues a session.
	r.Post("/init", initHandler(deps))

	// POST /v1/intake/auth/email/start, /verify — no auth; email JWT bootstrap (4-ii).
	// 404s cleanly when deps.EmailService is nil (auth.modes.email disabled).
	r.Post("/auth/email/start", emailStartHandler(deps))
	r.Post("/auth/email/verify", emailVerifyHandler(deps))

	// Routes that require a valid session.
	// deps.Auth.Handler is the chi-compatible middleware from auth.Middleware.
	r.Group(func(r chi.Router) {
		r.Use(deps.Auth.Handler)
		r.Post("/turn", turnHandler(deps))
		r.Post("/submit", submitHandler(deps))
	})
}
