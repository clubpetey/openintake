package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"intake/internal/config"
	"intake/internal/server"
	"intake/internal/version"
)

// ---- helpers ----

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("JSON decode failed: %v\nbody: %s", err, body)
	}
}

func newTestServer(t *testing.T, corsOrigins []string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr:        ":8080",
			ExternalURL: "http://localhost:8080",
			CORSOrigins: corsOrigins,
		},
	}
	deps := server.Deps{
		Version:     version.Info(),
		CORSOrigins: corsOrigins,
	}
	return server.New(cfg, deps)
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

// ---- Task 6: /v1/health ----

func TestHealth_Returns200(t *testing.T) {
	h := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
	}
	var body map[string]string
	decodeJSON(t, w.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("body.status = %q; want %q", body["status"], "ok")
	}
}

// ---- Task 6: /v1/version ----

func TestVersion_ReturnsBuildInfo(t *testing.T) {
	h := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
	}
	var info version.BuildInfo
	decodeJSON(t, w.Body.Bytes(), &info)
	if info.Version == "" {
		t.Error("version.version is empty")
	}
}

// ---- Task 6: CORS — allowed origin gets ACAO header ----

func TestCORS_AllowedOriginGetsHeader(t *testing.T) {
	allowed := "http://localhost:5173"
	h := newTestServer(t, []string{allowed})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", allowed)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != allowed {
		t.Errorf("ACAO = %q; want %q", acao, allowed)
	}
}

// ---- Task 6: CORS — disallowed origin does NOT get ACAO header ----

func TestCORS_DisallowedOriginNoHeader(t *testing.T) {
	h := newTestServer(t, []string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "" {
		t.Errorf("ACAO = %q; want empty for disallowed origin", acao)
	}
}

// ---- Task 6: CORS — preflight for allowed origin returns 204 ----

func TestCORS_PreflightAllowedOrigin(t *testing.T) {
	allowed := "http://localhost:5173"
	h := newTestServer(t, []string{allowed})

	req := httptest.NewRequest(http.MethodOptions, "/v1/intake/init", nil)
	req.Header.Set("Origin", allowed)
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d; want %d", w.Code, http.StatusNoContent)
	}
}

// ---- Task 6: CORS — preflight for disallowed origin returns 403 ----

func TestCORS_PreflightDisallowedOrigin(t *testing.T) {
	h := newTestServer(t, []string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodOptions, "/v1/intake/init", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("preflight status = %d; want %d", w.Code, http.StatusForbidden)
	}
}
