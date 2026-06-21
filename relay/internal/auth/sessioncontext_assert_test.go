package auth_test

import (
	"testing"

	"github.com/clubpetey/openintake/relay/internal/auth"
)

// requireFullSessionContext asserts that every field of sess matches the
// corresponding field of want, with per-field failure messages so a test
// failure points at the specific missing/wrong field (LESSONS L015).
//
// Usage in dispatcher tests:
//
//	requireFullSessionContext(t, sess, auth.SessionContext{
//	    SessionID: id,
//	    AuthMode:  "anonymous",
//	    Verified:  false,
//	    // pointer fields left nil if not expected to be populated
//	})
//
// String pointer fields (Email, UserID, DisplayName) are compared by value
// when both are non-nil; nil-vs-non-nil mismatch is reported. Custom is
// compared shallowly (length + per-key equality).
//
// L015: derived-field test gaps surface only at live smoke. Every dispatcher
// path produces a SessionContext whose fields are read downstream by
// payloadbuild.Build; a test that asserts only the fields its path uniquely
// owns will pass while shipping a payload that fails schema validation.
// Calling this helper from every per-path dispatcher test makes "every path
// populates every required field" structural, not a per-test convention.
func requireFullSessionContext(t *testing.T, sess *auth.SessionContext, want auth.SessionContext) {
	t.Helper()
	if sess == nil {
		t.Fatalf("SessionContext is nil; want %+v", want)
	}
	if sess.SessionID != want.SessionID {
		t.Errorf("SessionID = %q; want %q", sess.SessionID, want.SessionID)
	}
	if sess.AuthMode != want.AuthMode {
		t.Errorf("AuthMode = %q; want %q", sess.AuthMode, want.AuthMode)
	}
	if sess.Verified != want.Verified {
		t.Errorf("Verified = %v; want %v", sess.Verified, want.Verified)
	}
	requireStringPtrEq(t, "UserID", sess.UserID, want.UserID)
	requireStringPtrEq(t, "Email", sess.Email, want.Email)
	requireStringPtrEq(t, "DisplayName", sess.DisplayName, want.DisplayName)
	if len(sess.Custom) != len(want.Custom) {
		t.Errorf("Custom len = %d; want %d (got=%+v want=%+v)", len(sess.Custom), len(want.Custom), sess.Custom, want.Custom)
	} else {
		for k, v := range want.Custom {
			if got, ok := sess.Custom[k]; !ok {
				t.Errorf("Custom[%q] missing; want %v", k, v)
			} else if got != v {
				t.Errorf("Custom[%q] = %v; want %v", k, got, v)
			}
		}
	}
}

func requireStringPtrEq(t *testing.T, field string, got, want *string) {
	t.Helper()
	switch {
	case got == nil && want == nil:
		return
	case got == nil && want != nil:
		t.Errorf("%s = nil; want %q", field, *want)
	case got != nil && want == nil:
		t.Errorf("%s = %q; want nil", field, *got)
	case *got != *want:
		t.Errorf("%s = %q; want %q", field, *got, *want)
	}
}
