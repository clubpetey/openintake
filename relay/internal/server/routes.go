package server

import "github.com/go-chi/chi/v5"

// registerIntakeRoutes mounts the /v1/intake/* handlers on r.
// r is already scoped to the /v1/intake prefix by the caller (server.New).
//
// 1-iii registers POST /init (no auth; issues a session) and POST /turn (behind auth).
// 1-iv will add: POST /submit (behind auth).
func registerIntakeRoutes(r chi.Router, deps Deps) {
	// POST /v1/intake/init — no auth; issues a session.
	r.Post("/init", initHandler(deps))

	// Routes that require a valid session.
	// deps.Auth.Handler is the chi-compatible middleware from auth.Middleware.
	r.Group(func(r chi.Router) {
		r.Use(deps.Auth.Handler)
		r.Post("/turn", turnHandler(deps))
		// 1-iv will add: r.Post("/submit", submitHandler(deps))
	})
}
