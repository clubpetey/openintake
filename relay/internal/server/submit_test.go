package server_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	webhookadapter "intake/internal/adapter/webhook"
	"intake/internal/attachvalidate"
	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/payload"
	"intake/internal/payloadbuild"
	"intake/internal/router"
	"intake/internal/server"

	"intake/internal/adapter"
)

// fakeProviderSubmit returns a canned classify JSON response.
// Named to avoid conflict with testProvider in turn_test.go.
type fakeProviderSubmit struct{ response string }

func (f *fakeProviderSubmit) Name() string { return "fake-submit" }
func (f *fakeProviderSubmit) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	ch := make(chan llm.ChatChunk, 1)
	ch <- llm.ChatChunk{Delta: f.response, Done: true}
	close(ch)
	return ch, nil
}

// fakeAdapter captures the payload POSTed to it and returns a canned result.
type fakeAdapter struct {
	received *payload.IntakePayload
}

func (a *fakeAdapter) Name() string                    { return "fake-webhook" }
func (a *fakeAdapter) RequiresLicense() bool           { return false }
func (a *fakeAdapter) Configure(map[string]any) error  { return nil }
func (a *fakeAdapter) HealthCheck(context.Context) error { return nil }
func (a *fakeAdapter) Create(_ context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	a.received = p
	return &adapter.CreateResult{
		ExternalID:  "test-ext-id-001",
		ExternalURL: "",
		AdapterName: "fake-webhook",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

const submitClassifyJSON = `{
  "summary": "User cannot log in.",
  "title_suggestion": "Login failure",
  "classification": "bug",
  "severity_guess": "high",
  "tags_suggested": ["auth"],
  "language": "en"
}`

func buildSubmitDeps(fa adapter.Adapter) server.Deps {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store, nil, nil)
	provider := &fakeProviderSubmit{response: submitClassifyJSON}
	classifier := classify.New(provider, "claude-sonnet-4-6", 512)
	builder := payloadbuild.New("0.1.0")
	rtr, err := router.New(map[string]adapter.Adapter{fa.Name(): fa}, nil, fa.Name(), nil)
	if err != nil {
		panic("buildSubmitDeps: router.New: " + err.Error())
	}
	return server.Deps{
		Auth:       mw,
		Router:     rtr,
		Classifier: classifier,
		Builder:    builder,
		// Phase 6 (6-i): explicit body cap (Phase 1 used implicit 1<<20).
		// Tests use the 14 MB cap (attachments enabled) and override
		// AttachmentsCfg.Enabled per-test as needed.
		BodyCapBytes: 14 * (1 << 20),
		AttachmentsCfg: config.AttachmentsConfig{
			Enabled:          true,
			MaxSizeBytes:     5_242_880,
			MaxTotalBytes:    10_485_760,
			AllowedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
		},
		AttachmentMIMEs: []string{"image/png", "image/jpeg", "image/webp"},
	}
}

// issueSession issues a session through the store embedded in deps.Auth and returns the session ID.
func issueSession(deps server.Deps) string {
	return deps.Auth.Store().Issue()
}

func TestSubmitHandler_HappyPath(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)
	sessionID := issueSession(deps)

	// Build a minimal SubmitRequest body.
	reqBody := server.SubmitRequest{
		Messages: []server.TurnMessage{
			{Role: "user", Content: "I cannot log in."},
		},
		Client: server.ClientInfo{
			WidgetVersion: "0.1.0",
			URL:           "http://localhost:5173/",
			UserAgent:     "Mozilla/5.0",
			Viewport:      server.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		UserClaims: map[string]any{},
		Context:    server.ContextInfo{},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)

	rr := httptest.NewRecorder()

	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", rr.Code, rr.Body.String())
	}

	var resp server.SubmitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ExternalID != "test-ext-id-001" {
		t.Errorf("expected ExternalID test-ext-id-001, got %q", resp.ExternalID)
	}
	if resp.AdapterName != "fake-webhook" {
		t.Errorf("expected AdapterName fake-webhook, got %q", resp.AdapterName)
	}

	// Assert the canonical payload was schema-valid (fakeAdapter captured it).
	if fa.received == nil {
		t.Fatal("adapter.Create was not called")
	}
	if fa.received.SchemaVersion != "1.0" {
		t.Errorf("expected schema_version=1.0, got %q", fa.received.SchemaVersion)
	}
	if fa.received.Conversation.Classification != "bug" {
		t.Errorf("expected classification=bug (from classify), got %q", fa.received.Conversation.Classification)
	}
	if fa.received.User.AuthMode != "anonymous" {
		t.Errorf("expected user.auth_mode=anonymous, got %q", fa.received.User.AuthMode)
	}
}

// TestSubmitHandler_MissingSession returns 401.
func TestSubmitHandler_MissingSession(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)

	bodyBytes, _ := json.Marshal(server.SubmitRequest{
		Messages: []server.TurnMessage{{Role: "user", Content: "hello"}},
		Client: server.ClientInfo{
			URL:       "http://localhost:5173/",
			UserAgent: "test",
			Viewport:  server.Viewport{W: 100, H: 100},
			Locale:    "en",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	// No X-Intake-Session header — session intentionally absent.

	rr := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d\nbody: %s", rr.Code, rr.Body.String())
	}
}

// TestSubmitHandler_IntegrationWithHttptestWebhook is the integration-level test:
// uses the REAL webhook adapter wired to an httptest server.
// Asserts the posted body is schema-valid and has classify-derived fields.
func TestSubmitHandler_IntegrationWithHttptestWebhook(t *testing.T) {
	var receivedBody []byte
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read receiver body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"external_id":"integration-ext-id"}`))
	}))
	defer receiver.Close()

	wh := webhookadapter.New()
	if err := wh.Configure(map[string]any{
		"url": receiver.URL,
		"retry": map[string]any{"max_attempts": 1, "backoff": "fixed"},
	}); err != nil {
		t.Fatalf("Configure webhook: %v", err)
	}

	store := auth.NewStore()
	mw := auth.NewMiddleware(store, nil, nil)
	provider := &fakeProviderSubmit{response: submitClassifyJSON}
	classifier := classify.New(provider, "claude-sonnet-4-6", 512)
	builder := payloadbuild.New("0.1.0")
	rtr, err := router.New(map[string]adapter.Adapter{wh.Name(): wh}, nil, wh.Name(), nil)
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	deps := server.Deps{
		Auth:         mw,
		Router:       rtr,
		Classifier:   classifier,
		Builder:      builder,
		BodyCapBytes: 14 * (1 << 20),
		AttachmentsCfg: config.AttachmentsConfig{
			Enabled:          true,
			MaxSizeBytes:     5_242_880,
			MaxTotalBytes:    10_485_760,
			AllowedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
		},
		AttachmentMIMEs: []string{"image/png", "image/jpeg", "image/webp"},
	}
	sessionID := store.Issue()

	reqBody := server.SubmitRequest{
		Messages: []server.TurnMessage{
			{Role: "user", Content: "I cannot log in."},
			{Role: "assistant", Content: "What error do you see?"},
			{Role: "user", Content: "Invalid credentials."},
		},
		Client: server.ClientInfo{
			WidgetVersion: "0.1.0",
			URL:           "http://localhost:5173/",
			UserAgent:     "Mozilla/5.0",
			Viewport:      server.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		UserClaims: map[string]any{},
		Context:    server.ContextInfo{},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)

	rr := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", rr.Code, rr.Body.String())
	}

	// Assert SubmitResponse.
	var resp server.SubmitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ExternalID != "integration-ext-id" {
		t.Errorf("expected ExternalID integration-ext-id, got %q", resp.ExternalID)
	}

	// Assert the webhook receiver got a schema-valid canonical payload.
	if receivedBody == nil {
		t.Fatal("webhook receiver was not called")
	}
	var posted map[string]any
	if err := json.Unmarshal(receivedBody, &posted); err != nil {
		t.Fatalf("webhook body not valid JSON: %v", err)
	}
	if posted["schema_version"] != "1.0" {
		t.Errorf("expected schema_version=1.0 in posted payload, got %v", posted["schema_version"])
	}
	conv, ok := posted["conversation"].(map[string]any)
	if !ok {
		t.Fatal("conversation field missing or wrong type")
	}
	if conv["classification"] != "bug" {
		t.Errorf("expected classification=bug from classify, got %v", conv["classification"])
	}
	user, ok := posted["user"].(map[string]any)
	if !ok {
		t.Fatal("user field missing or wrong type")
	}
	if user["auth_mode"] != "anonymous" {
		t.Errorf("expected user.auth_mode=anonymous, got %v", user["auth_mode"])
	}
}

// --- Phase 6 (6-i) tests for submitHandler ---

func newPNGAttachment(t *testing.T) server.SubmitAttachment {
	t.Helper()
	return server.SubmitAttachment{
		Type:     "screenshot",
		MIMEType: "image/png",
		URL:      "data:image/png;base64," + base64.StdEncoding.EncodeToString(attachvalidate.GoldenPNG()),
	}
}

func validClient() server.ClientInfo {
	return server.ClientInfo{
		WidgetVersion: "test", URL: "https://example.com", UserAgent: "ua",
		Viewport: server.Viewport{W: 100, H: 100}, Locale: "en",
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestSubmit_OverBodyCap_413_RequestBodyTooLarge(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)
	sessionID := issueSession(deps)
	// Build valid JSON that is > 14 MB. The decoder reads incrementally;
	// MaxBytesReader returns *http.MaxBytesError as soon as the limit is
	// exceeded, regardless of whether the JSON would have been valid.
	// We pad the user-content string so the body is ~15 MB.
	big := make([]byte, 15*1024*1024)
	for i := range big {
		big[i] = 'a'
	}
	body := []byte(`{"messages":[{"role":"user","content":"` + string(big) + `"}],"client":{"url":"x","user_agent":"u","widget_version":"v","viewport":{"w":1,"h":1},"locale":"en"}}`)
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d; want 413; body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("request_body_too_large")) {
		t.Errorf("body does not contain request_body_too_large: %s", rec.Body.String())
	}
}

func TestSubmit_MalformedJSON_400_BadRequest(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)
	sessionID := issueSession(deps)
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("bad_request")) {
		t.Errorf("body does not contain bad_request: %s", rec.Body.String())
	}
}

func TestSubmit_AttachmentsDisabled_400_AttachmentsDisabled(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)
	// Disable attachments and reduce body cap to mirror disabled-mode startup.
	deps.AttachmentsCfg.Enabled = false
	deps.AttachmentMIMEs = nil
	body := mustMarshal(t, server.SubmitRequest{
		Messages:    []server.TurnMessage{{Role: "user", Content: "hi"}},
		Client:      validClient(),
		Attachments: []server.SubmitAttachment{newPNGAttachment(t)},
	})
	sessionID := issueSession(deps)
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("attachments_disabled")) {
		t.Errorf("want attachments_disabled; got %s", rec.Body.String())
	}
}

func TestSubmit_AttachmentMIMEMismatch_415(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)
	bad := newPNGAttachment(t)
	bad.MIMEType = "image/jpeg" // declared JPEG, bytes are PNG
	body := mustMarshal(t, server.SubmitRequest{
		Messages:    []server.TurnMessage{{Role: "user", Content: "hi"}},
		Client:      validClient(),
		Attachments: []server.SubmitAttachment{bad},
	})
	sessionID := issueSession(deps)
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d; want 415; body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("attachment_mime_mismatch")) {
		t.Errorf("want attachment_mime_mismatch; got %s", rec.Body.String())
	}
}

func TestSubmit_AttachmentTooLarge_413(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)
	deps.AttachmentsCfg.MaxSizeBytes = 10 // tiny
	body := mustMarshal(t, server.SubmitRequest{
		Messages:    []server.TurnMessage{{Role: "user", Content: "hi"}},
		Client:      validClient(),
		Attachments: []server.SubmitAttachment{newPNGAttachment(t)},
	})
	sessionID := issueSession(deps)
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d; want 413; body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("attachment_too_large")) {
		t.Errorf("want attachment_too_large; got %s", rec.Body.String())
	}
}

// Order-of-operations regression: when attachvalidate rejects, Router.Route
// MUST NOT be called (the underlying adapter.Create must not run).
func TestSubmit_AttachvalidateFails_AdapterNotCalled(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)
	bad := newPNGAttachment(t)
	bad.MIMEType = "image/jpeg" // forces ErrMIMEMismatch
	body := mustMarshal(t, server.SubmitRequest{
		Messages:    []server.TurnMessage{{Role: "user", Content: "hi"}},
		Client:      validClient(),
		Attachments: []server.SubmitAttachment{bad},
	})
	sessionID := issueSession(deps)
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d; want 415; body=%s", rec.Code, rec.Body.String())
	}
	if fa.received != nil {
		t.Errorf("fakeAdapter.Create was called; want NOT called when attachvalidate fails")
	}
}

func TestSubmit_PhaseOneRegression_NoAttachments_200(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildSubmitDeps(fa)
	body := mustMarshal(t, server.SubmitRequest{
		Messages: []server.TurnMessage{{Role: "user", Content: "hi"}},
		Client:   validClient(),
	})
	sessionID := issueSession(deps)
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want 200 (no attachments path); body=%s", rec.Code, rec.Body.String())
	}
}
