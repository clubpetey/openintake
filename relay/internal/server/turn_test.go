package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"intake/internal/auth"
	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/server"
)

// testProvider implements llm.Provider using a fixed list of chunks.
// No network calls; safe to use in unit tests without an API key.
type testProvider struct {
	chunks []llm.ChatChunk
}

func (p *testProvider) Name() string { return "test" }

func (p *testProvider) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	ch := make(chan llm.ChatChunk, len(p.chunks))
	for _, c := range p.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func newTestDeps() (server.Deps, *auth.Store) {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store, nil, nil)
	provider := &testProvider{
		chunks: []llm.ChatChunk{
			{Delta: "Hello"},
			{Delta: " world"},
			{Done: true, InputTokens: 10, OutputTokens: 2},
		},
	}
	return server.Deps{
		Auth:         mw,
		Provider:     provider,
		SystemPrompt: "You are a test assistant.",
		Model:        "test-model",
		MaxTokens:    512,
	}, store
}

// --- /init tests ---

func TestInitHandler_Returns200AndSessionID(t *testing.T) {
	deps, _ := newTestDeps()

	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	router := server.New(cfg, deps)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/init", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("/init status = %d; want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp server.InitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode InitResponse: %v", err)
	}
	if resp.SessionID == "" {
		t.Error("session_id is empty")
	}
	if !resp.Capabilities.Streaming {
		t.Error("capabilities.streaming = false; want true")
	}
	want := []string{"anonymous"}
	if !equalStringSlice(resp.Capabilities.AuthModes, want) {
		t.Errorf("default AuthModes = %v; want %v", resp.Capabilities.AuthModes, want)
	}

	// The returned session_id must be valid in the store.
	if !deps.Auth.Store().Validate(resp.SessionID) {
		t.Error("returned session_id does not validate in the store")
	}
}

// TestInitHandler_EmitsAllEnabledModes verifies the Phase-4 init response
// includes all enabled auth modes and an email TTL hint when email mode is on.
func TestInitHandler_EmitsAllEnabledModes(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}},
		Auth: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true, Email: true, SSO: true},
			Email: config.EmailConfig{CodeTTL: "10m"},
		},
	}
	deps := server.Deps{
		Auth:    auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: cfg.Auth,
	}
	mux := server.New(cfg, deps)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/init", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}

	var resp server.InitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"anonymous", "email", "sso"}
	if got := resp.Capabilities.AuthModes; !equalStringSlice(got, want) {
		t.Errorf("AuthModes = %v; want %v", got, want)
	}
	if resp.Auth == nil || resp.Auth.Email == nil || resp.Auth.Email.CodeTTLSeconds != 600 {
		t.Errorf("Auth.Email = %+v; want CodeTTLSeconds=600", resp.Auth)
	}
}

// equalStringSlice — order-sensitive equality for the test above.
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- /turn tests ---

func TestTurnHandler_StreamsSSEFrames(t *testing.T) {
	deps, store := newTestDeps()
	sessionID := store.Issue()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	router := server.New(cfg, deps)

	body := `{"messages":[{"role":"user","content":"the export button is broken"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("/turn status = %d; want 200; body: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q; want text/event-stream", ct)
	}

	// Parse SSE frames from the body.
	var deltas []string
	var doneFrame *server.SSEDone
	scanner := bufio.NewScanner(bytes.NewReader(rr.Body.Bytes()))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		// Try to decode as SSEDone.
		var done server.SSEDone
		if err := json.Unmarshal([]byte(payload), &done); err == nil && done.Done {
			doneFrame = &done
			continue
		}

		// Try to decode as SSEDelta.
		var delta server.SSEDelta
		if err := json.Unmarshal([]byte(payload), &delta); err == nil && delta.Delta != "" {
			deltas = append(deltas, delta.Delta)
		}
	}

	if len(deltas) != 2 {
		t.Errorf("got %d delta frames; want 2; deltas: %v", len(deltas), deltas)
	}
	if len(deltas) > 0 && deltas[0] != "Hello" {
		t.Errorf("deltas[0] = %q; want \"Hello\"", deltas[0])
	}
	if len(deltas) > 1 && deltas[1] != " world" {
		t.Errorf("deltas[1] = %q; want \" world\"", deltas[1])
	}
	if doneFrame == nil {
		t.Fatal("no done frame received")
	}
	if doneFrame.InputTokens != 10 {
		t.Errorf("done.input_tokens = %d; want 10", doneFrame.InputTokens)
	}
	if doneFrame.OutputTokens != 2 {
		t.Errorf("done.output_tokens = %d; want 2", doneFrame.OutputTokens)
	}
}

func TestTurnHandler_MissingSession_Returns401(t *testing.T) {
	deps, _ := newTestDeps()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	router := server.New(cfg, deps)

	body := `{"messages":[{"role":"user","content":"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-Intake-Session header.
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("/turn without session: status = %d; want 401", rr.Code)
	}
}

// 4-i: with no verifiers configured, a bearer token must NOT silently downgrade
// to anonymous. It is rejected with 401 (Phase-1 returned 501; the dispatcher
// now consistently returns 401 for any unaccepted bearer).
func TestTurnHandler_BearerToken_Returns401(t *testing.T) {
	deps, _ := newTestDeps()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	router := server.New(cfg, deps)

	body := `{"messages":[{"role":"user","content":"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake.jwt.token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("/turn with Bearer: status = %d; want 401", rr.Code)
	}
}

// TestTurnHandler_SSEErrorFrame verifies that when the provider yields a chunk
// with Err != nil, the handler emits a data: {"error":"..."} SSE frame and
// then terminates (no subsequent frames).
func TestTurnHandler_SSEErrorFrame(t *testing.T) {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store, nil, nil)
	provider := &testProvider{
		chunks: []llm.ChatChunk{
			{Delta: "partial"},
			{Err: errors.New("boom"), Done: true},
		},
	}
	deps := server.Deps{
		Auth:         mw,
		Provider:     provider,
		SystemPrompt: "You are a test assistant.",
		Model:        "test-model",
		MaxTokens:    512,
	}
	sessionID := store.Issue()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	router := server.New(cfg, deps)

	body := `{"messages":[{"role":"user","content":"something broke"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("/turn SSEError: status = %d; want 200 (SSE headers already sent)", rr.Code)
	}

	// Scan SSE frames; expect exactly one error frame and no done frame after it.
	var errorPayload string
	var afterError []string
	foundError := false
	scanner := bufio.NewScanner(bytes.NewReader(rr.Body.Bytes()))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if foundError {
			afterError = append(afterError, payload)
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			continue
		}
		if _, hasError := m["error"]; hasError {
			errorPayload = payload
			foundError = true
		}
	}

	if !foundError {
		t.Fatalf("no SSE error frame found in body:\n%s", rr.Body.String())
	}
	// Verify the error field is non-empty.
	var errFrame map[string]any
	if err := json.Unmarshal([]byte(errorPayload), &errFrame); err != nil {
		t.Fatalf("unmarshal error frame: %v", err)
	}
	if errFrame["error"] == "" {
		t.Errorf("error frame has empty error field: %s", errorPayload)
	}
	// Handler must stop after the error frame — no further frames expected.
	if len(afterError) > 0 {
		t.Errorf("got %d frames after error frame; want 0: %v", len(afterError), afterError)
	}
}

func TestInitHandler_EmitsRequiresCaptchaWhenAnonymousAndEnabled(t *testing.T) {
	// Build a Deps with captcha enabled and required_for ["anonymous"];
	// no CaptchaVerifier wired (Phase 5-i pre-5-iii), so the handler should
	// still return 400 captcha_required + the discovery hint fields.
	cfg := &config.Config{
		Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}},
		Auth: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		Captcha: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
	}
	deps := server.Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
	}
	mux := server.New(cfg, deps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 (captcha_required)", rec.Code)
	}
	var body server.CaptchaRequiredResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (raw: %s)", err, rec.Body.String())
	}
	if body.Error.Code != "captcha_required" {
		t.Errorf("error.code = %q; want captcha_required", body.Error.Code)
	}
	if body.Captcha == nil || body.Captcha.Provider != "turnstile" || body.Captcha.SiteKey != "0x4AAA000000Test" {
		t.Errorf("body.captcha = %+v; want {turnstile, 0x4AAA000000Test}", body.Captcha)
	}
	if len(body.Capabilities.RequiresCaptcha) != 1 || body.Capabilities.RequiresCaptcha[0] != "anonymous" {
		t.Errorf("capabilities.requires_captcha = %v; want [anonymous]", body.Capabilities.RequiresCaptcha)
	}
}

func TestInitHandler_NoCaptchaConfig_MintsSessionAsBefore(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}},
		Auth: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
	}
	deps := server.Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		// CaptchaCfg.Enabled defaults to false.
	}
	mux := server.New(cfg, deps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	var body server.InitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SessionID == "" {
		t.Error("session_id is empty; want a UUID")
	}
	if body.Captcha != nil {
		t.Errorf("body.captcha = %+v; want nil (captcha disabled)", body.Captcha)
	}
	if body.Capabilities.RequiresCaptcha != nil {
		t.Errorf("capabilities.requires_captcha = %v; want nil/omitted", body.Capabilities.RequiresCaptcha)
	}
}

func TestInitHandler_CaptchaEnabledButNoModeIntersection_MintsSession(t *testing.T) {
	// captcha is enabled and configured for "email", but only anonymous mode
	// is on. The intersection is empty → no captcha gate, no captcha hint,
	// session minted normally.
	deps := server.Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true}, // only anonymous
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"email"}, // excludes anonymous
		},
	}

	// Wire through server.New so the routing is honored (same pattern as
	// the existing init handler tests).
	cfg := &config.Config{
		Server: config.ServerConfig{CORSOrigins: []string{}},
	}
	srv := server.New(cfg, deps)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (intersection is empty → no captcha gate)\nbody: %s", rec.Code, rec.Body.String())
	}
	var body server.InitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SessionID == "" {
		t.Error("session_id is empty; want a UUID")
	}
	if body.Captcha != nil {
		t.Errorf("body.captcha = %+v; want nil (RequiredFor excludes anonymous → no hint emitted)", body.Captcha)
	}
	if body.Capabilities.RequiresCaptcha != nil {
		t.Errorf("capabilities.requires_captcha = %v; want nil (empty intersection)", body.Capabilities.RequiresCaptcha)
	}
}

// --- 5-iii: deps.CaptchaVerifier integration tests ---

// fakeVerifier is a test double for captcha.Verifier. Behavior is set per test instance.
type fakeVerifier struct {
	ok     bool
	reason string
	err    error
	calls  int
}

func (f *fakeVerifier) Verify(ctx context.Context, token, remoteIP string) (bool, string, error) {
	f.calls++
	return f.ok, f.reason, f.err
}
func (f *fakeVerifier) Provider() string { return "fake" }

func TestInitHandler_WithValidCaptchaToken_MintsSession(t *testing.T) {
	v := &fakeVerifier{ok: true}
	deps := server.Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
		CaptchaVerifier: v,
	}
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{}}}
	srv := server.New(cfg, deps)

	body := `{"captcha_token":"tok-123"}`
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp server.InitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SessionID == "" {
		t.Error("session_id missing")
	}
	if v.calls != 1 {
		t.Errorf("Verifier.Verify called %d times; want 1", v.calls)
	}
}

func TestInitHandler_InvalidCaptchaToken_Returns401(t *testing.T) {
	v := &fakeVerifier{ok: false, reason: "invalid-input-response"}
	deps := server.Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
		CaptchaVerifier: v,
	}
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{}}}
	srv := server.New(cfg, deps)

	body := `{"captcha_token":"bad"}`
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	var respBody server.CaptchaFailedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if respBody.Error.Code != "captcha_failed" {
		t.Errorf("error.code = %q; want captcha_failed", respBody.Error.Code)
	}
	if respBody.Error.Reason != "invalid-input-response" {
		t.Errorf("error.reason = %q; want invalid-input-response", respBody.Error.Reason)
	}
}

func TestInitHandler_CaptchaVerifierErr_Returns502(t *testing.T) {
	v := &fakeVerifier{err: errors.New("upstream-flaky")}
	deps := server.Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
		CaptchaVerifier: v,
	}
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{}}}
	srv := server.New(cfg, deps)

	body := `{"captcha_token":"tok"}`
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d; want 502", rec.Code)
	}
	var body2 server.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body2.Error.Code != "captcha_unavailable" {
		t.Errorf("code = %q; want captcha_unavailable", body2.Error.Code)
	}
}

func TestInitHandler_CaptchaTokenIgnoredWhenNotRequired(t *testing.T) {
	// captcha.enabled=false → token in body is ignored; verifier never called.
	v := &fakeVerifier{ok: false, err: errors.New("would never be called")}
	deps := server.Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg:      config.CaptchaConfig{Enabled: false},
		CaptchaVerifier: v,
	}
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{}}}
	srv := server.New(cfg, deps)

	body := `{"captcha_token":"tok"}`
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (captcha disabled)", rec.Code)
	}
	if v.calls != 0 {
		t.Errorf("Verifier called %d times; want 0 (captcha disabled)", v.calls)
	}
}
