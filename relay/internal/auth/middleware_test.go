package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"intake/internal/auth"
)

// sentinelHandler is a handler that records whether it was called and captures
// the SessionContext from the request context.
func sentinelHandler(called *bool, captured **auth.SessionContext) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		sess, _ := auth.FromContext(r.Context())
		*captured = sess
		w.WriteHeader(http.StatusOK)
	})
}

func TestMiddleware_ValidSession_AttachesContext(t *testing.T) {
	store := auth.NewStore()
	id := store.Issue()
	mw := auth.NewMiddleware(store)

	var called bool
	var captured *auth.SessionContext
	next := sentinelHandler(&called, &captured)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	req.Header.Set("X-Intake-Session", id)
	rr := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
	if captured == nil {
		t.Fatal("SessionContext not attached to context")
	}
	if captured.SessionID != id {
		t.Errorf("SessionID = %q; want %q", captured.SessionID, id)
	}
	if captured.AuthMode != "anonymous" {
		t.Errorf("AuthMode = %q; want \"anonymous\"", captured.AuthMode)
	}
	if captured.Verified {
		t.Error("Verified = true; want false")
	}
}

func TestMiddleware_MissingSession_Returns401(t *testing.T) {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store)

	var called bool
	var captured *auth.SessionContext
	next := sentinelHandler(&called, &captured)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	rr := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rr.Code)
	}
	if called {
		t.Error("next handler was called; should not have been")
	}
}

func TestMiddleware_InvalidSession_Returns401(t *testing.T) {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store)

	var called bool
	var captured *auth.SessionContext
	next := sentinelHandler(&called, &captured)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	req.Header.Set("X-Intake-Session", "not-a-real-session-id")
	rr := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rr.Code)
	}
	if called {
		t.Error("next handler was called; should not have been")
	}
}

func TestMiddleware_BearerToken_Returns501(t *testing.T) {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store)

	var called bool
	var captured *auth.SessionContext
	next := sentinelHandler(&called, &captured)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJSUzI1NiJ9.fake.token")
	rr := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d; want 501", rr.Code)
	}
	if called {
		t.Error("next handler was called; should not have been for Bearer 501 seam")
	}
}
