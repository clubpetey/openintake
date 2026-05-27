package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"intake/internal/server"
)

// ---- helpers ----

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("JSON decode failed: %v\nbody: %s", err, body)
	}
}

// ---- Task 4: writeError shape ----

func TestWriteError_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	server.WriteErrorExported(w, http.StatusBadRequest, "bad_request", "something is wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", w.Code, http.StatusBadRequest)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q; want %q", ct, "application/json")
	}

	var env server.ErrorEnvelope
	decodeJSON(t, w.Body.Bytes(), &env)

	if env.Error.Code != "bad_request" {
		t.Errorf("error.code = %q; want %q", env.Error.Code, "bad_request")
	}
	if env.Error.Message != "something is wrong" {
		t.Errorf("error.message = %q; want %q", env.Error.Message, "something is wrong")
	}
}
