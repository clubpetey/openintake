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

// ---- Fix 2: OPTIONS without Origin must NOT be 403 (not a CORS preflight) ----

func TestCORS_OptionsWithoutOriginPassesThrough(t *testing.T) {
	h := newTestServer(t, []string{"http://localhost:5173"})

	// OPTIONS /v1/health with no Origin header — this is a plain HTTP OPTIONS
	// request, not a CORS preflight. The CORS middleware must not short-circuit
	// it with a 403; it must be handled by the router.
	req := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Errorf("OPTIONS without Origin returned 403; want router response (e.g. 405), not a CORS block")
	}
}

// ---- Fix 3: Vary: Origin is set on every response ----

func TestCORS_VaryOriginAlwaysSet(t *testing.T) {
	allowed := "http://localhost:5173"
	h := newTestServer(t, []string{allowed})

	// Verify Vary: Origin on a request with an allowed origin.
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", allowed)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	vary := w.Header().Get("Vary")
	if vary == "" {
		t.Error("Vary header is absent; want Vary: Origin on CORS responses")
	}
}
