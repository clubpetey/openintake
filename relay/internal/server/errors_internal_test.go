package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWriteError_EnvelopeShape exercises the unexported writeError function
// directly (white-box test). It confirms the ErrorEnvelope JSON shape and
// correct Content-Type / status code.
func TestWriteError_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad_request", "something is wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", w.Code, http.StatusBadRequest)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q; want %q", ct, "application/json")
	}

	var env ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("JSON decode failed: %v\nbody: %s", err, w.Body.Bytes())
	}

	if env.Error.Code != "bad_request" {
		t.Errorf("error.code = %q; want %q", env.Error.Code, "bad_request")
	}
	if env.Error.Message != "something is wrong" {
		t.Errorf("error.message = %q; want %q", env.Error.Message, "something is wrong")
	}
}
