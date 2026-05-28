package auth_test

import (
	"context"
	"errors"
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
	mw := auth.NewMiddleware(store, nil, nil)

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
	mw := auth.NewMiddleware(store, nil, nil)

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
	mw := auth.NewMiddleware(store, nil, nil)

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

// --- stubs for the new dispatcher tests (4-i) ---

type stubEmailVerifier struct {
	wantToken string
	email     string
	err       error
}

func (s *stubEmailVerifier) Verify(token string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.wantToken != "" && token != s.wantToken {
		return "", errors.New("stub: token mismatch")
	}
	return s.email, nil
}

type stubSSOVerifier struct {
	wantToken string
	claims    *auth.SSOClaims
	err       error
}

func (s *stubSSOVerifier) Verify(_ context.Context, token string) (*auth.SSOClaims, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.wantToken != "" && token != s.wantToken {
		return nil, errors.New("stub: token mismatch")
	}
	return s.claims, nil
}

// alwaysFail satisfies the EmailJWTVerifier interface and always returns an error.
type alwaysFail struct{}

func (alwaysFail) Verify(string) (string, error) { return "", errors.New("stub: always fails") }

type alwaysFailSSO struct{}

func (alwaysFailSSO) Verify(context.Context, string) (*auth.SSOClaims, error) {
	return nil, errors.New("stub: always fails")
}

// runRequest is a small helper that runs an http.Request through a middleware
// and a no-op `next` handler that captures the resolved SessionContext.
func runRequest(t *testing.T, mw *auth.Middleware, r *http.Request) (status int, sess *auth.SessionContext) {
	t.Helper()
	rr := httptest.NewRecorder()
	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _ = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, r)
	return rr.Code, sess
}

// --- New dispatcher tests (4-i) ---

func TestDispatcher_EmailModeOnly_ValidToken(t *testing.T) {
	store := auth.NewStore()
	email := &stubEmailVerifier{wantToken: "valid-token", email: "user@example.com"}
	mw := auth.NewMiddleware(store, email, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer valid-token")

	status, sess := runRequest(t, mw, r)

	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	if sess == nil {
		t.Fatal("session not attached")
	}
	if sess.AuthMode != "email" || !sess.Verified {
		t.Errorf("session = %+v; want AuthMode=email Verified=true", sess)
	}
	if sess.Email == nil || *sess.Email != "user@example.com" {
		t.Errorf("session.Email = %v; want user@example.com", sess.Email)
	}
}

func TestDispatcher_EmailModeOnly_InvalidToken_401(t *testing.T) {
	store := auth.NewStore()
	email := alwaysFail{}
	mw := auth.NewMiddleware(store, email, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer bogus")

	status, sess := runRequest(t, mw, r)

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", status)
	}
	if sess != nil {
		t.Errorf("session should not be attached on failure; got %+v", sess)
	}
}

func TestDispatcher_SSOModeOnly_ValidToken(t *testing.T) {
	store := auth.NewStore()
	email := "user@sso.example"
	name := "Alice User"
	sso := &stubSSOVerifier{
		wantToken: "valid-sso",
		claims: &auth.SSOClaims{
			UserID:      "auth0|abc123",
			Email:       &email,
			DisplayName: &name,
		},
	}
	mw := auth.NewMiddleware(store, nil, sso)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer valid-sso")

	status, sess := runRequest(t, mw, r)

	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	if sess.AuthMode != "sso" || !sess.Verified {
		t.Errorf("session = %+v; want AuthMode=sso Verified=true", sess)
	}
	if sess.UserID == nil || *sess.UserID != "auth0|abc123" {
		t.Errorf("session.UserID = %v; want auth0|abc123", sess.UserID)
	}
	if sess.Email == nil || *sess.Email != "user@sso.example" {
		t.Errorf("session.Email = %v", sess.Email)
	}
}

func TestDispatcher_BothModes_EmailWinsWhenValid(t *testing.T) {
	store := auth.NewStore()
	email := &stubEmailVerifier{email: "user@example.com"}
	sso := alwaysFailSSO{}
	mw := auth.NewMiddleware(store, email, sso)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer any")

	_, sess := runRequest(t, mw, r)

	if sess == nil || sess.AuthMode != "email" {
		t.Errorf("session.AuthMode = %v; want email (email tried first)", sess)
	}
}

func TestDispatcher_BothModes_SSOReachedWhenEmailFails(t *testing.T) {
	store := auth.NewStore()
	email := alwaysFail{}
	sub := "user-from-sso"
	sso := &stubSSOVerifier{claims: &auth.SSOClaims{UserID: sub}}
	mw := auth.NewMiddleware(store, email, sso)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer any")

	_, sess := runRequest(t, mw, r)

	if sess == nil || sess.AuthMode != "sso" {
		t.Errorf("session.AuthMode = %v; want sso (fall-through)", sess)
	}
	if sess.UserID == nil || *sess.UserID != sub {
		t.Errorf("session.UserID = %v; want %s", sess.UserID, sub)
	}
}

func TestDispatcher_BothModes_BothFail_401(t *testing.T) {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store, alwaysFail{}, alwaysFailSSO{})

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer bogus")

	status, _ := runRequest(t, mw, r)

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", status)
	}
}

func TestDispatcher_NoModes_BearerPresent_401(t *testing.T) {
	// Regression: even if both modes are off, a bearer must NOT silently downgrade
	// to anonymous. Phase 1 returned 501 here; 4-i changes that to 401.
	store := auth.NewStore()
	store.Issue() // session exists but bearer should still 401
	mw := auth.NewMiddleware(store, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("Authorization", "Bearer something")

	status, _ := runRequest(t, mw, r)

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401 (bearer must not silently downgrade)", status)
	}
}

func TestDispatcher_EmailMode_SessionIDFromHeader(t *testing.T) {
	store := auth.NewStore()
	email := &stubEmailVerifier{email: "user@example.com"}
	mw := auth.NewMiddleware(store, email, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", nil)
	r.Header.Set("Authorization", "Bearer valid")
	r.Header.Set("X-Intake-Session", "00000000-0000-0000-0000-000000000abc")

	status, sess := runRequest(t, mw, r)

	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	if sess.SessionID != "00000000-0000-0000-0000-000000000abc" {
		t.Errorf("SessionID = %q; want it to come from X-Intake-Session header", sess.SessionID)
	}
	if sess.AuthMode != "email" {
		t.Errorf("AuthMode = %q; want email", sess.AuthMode)
	}
}

func TestDispatcher_SSOMode_SessionIDFromHeader(t *testing.T) {
	store := auth.NewStore()
	sub := "auth0|user-001"
	sso := &stubSSOVerifier{claims: &auth.SSOClaims{UserID: sub}}
	mw := auth.NewMiddleware(store, nil, sso)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", nil)
	r.Header.Set("Authorization", "Bearer valid")
	r.Header.Set("X-Intake-Session", "11111111-1111-1111-1111-111111111111")

	status, sess := runRequest(t, mw, r)

	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	if sess.SessionID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("SessionID = %q; want it to come from X-Intake-Session header", sess.SessionID)
	}
	if sess.AuthMode != "sso" {
		t.Errorf("AuthMode = %q; want sso", sess.AuthMode)
	}
}

func TestDispatcher_AnonymousFallthrough_Preserved(t *testing.T) {
	// Phase 1 behavior: no Authorization header + valid X-Intake-Session = anonymous.
	store := auth.NewStore()
	sid := store.Issue()
	mw := auth.NewMiddleware(store, nil, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", nil)
	r.Header.Set("X-Intake-Session", sid)

	status, sess := runRequest(t, mw, r)

	if status != http.StatusOK {
		t.Fatalf("status = %d; want 200", status)
	}
	requireFullSessionContext(t, sess, auth.SessionContext{
		SessionID: sid,
		AuthMode:  "anonymous",
		Verified:  false,
	})
}

func TestDispatcher_StrictAnonymous_RejectsValidSessionWhenModesAnonymousFalse(t *testing.T) {
	store := auth.NewStore()
	sessionID := store.Issue()

	m := auth.NewMiddlewareWithModes(store, nil, nil, false) // modesAnonymous=false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be invoked when modesAnonymous=false")
	})

	req := httptest.NewRequest("POST", "/v1/intake/turn", nil)
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	m.Handler(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401 (Q9 strict anonymous reject)", rec.Code)
	}
}

func TestDispatcher_StrictAnonymous_PreservedPhase1DefaultBehavior(t *testing.T) {
	// NewMiddleware (the Phase 1+4 constructor) must default modesAnonymous=true.
	store := auth.NewStore()
	sessionID := store.Issue()

	m := auth.NewMiddleware(store, nil, nil)
	hit := false
	var captured *auth.SessionContext
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		sess, ok := auth.FromContext(r.Context())
		if !ok {
			t.Fatal("SessionContext not attached")
		}
		captured = sess
	})
	req := httptest.NewRequest("POST", "/v1/intake/turn", nil)
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	m.Handler(next).ServeHTTP(rec, req)

	if !hit {
		t.Fatal("next handler not invoked (Phase 1 regression!)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want 200 (default OK when next does not write)", rec.Code)
	}
	requireFullSessionContext(t, captured, auth.SessionContext{
		SessionID: sessionID,
		AuthMode:  "anonymous",
		Verified:  false,
	})
}
