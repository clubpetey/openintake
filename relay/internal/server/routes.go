package server

import "github.com/go-chi/chi/v5"

// registerIntakeRoutes mounts the /v1/intake/* handlers on r.
//
// This body is intentionally empty in 1-i. Sub-plans extend it:
//   - 1-iii registers POST /v1/intake/init and POST /v1/intake/turn (SSE)
//   - 1-iv registers POST /v1/intake/submit
func registerIntakeRoutes(r chi.Router, deps Deps) {
	// /v1/intake/* routes registered by sub-plans 1-iii (init, turn) and 1-iv (submit)
	_ = r
	_ = deps
}
