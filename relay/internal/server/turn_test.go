package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	mw := auth.NewMiddleware(store)
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
	if len(resp.Capabilities.AuthModes) == 0 || resp.Capabilities.AuthModes[0] != "anonymous" {
		t.Errorf("auth_modes = %v; want [\"anonymous\"]", resp.Capabilities.AuthModes)
	}

	// The returned session_id must be valid in the store.
	if !deps.Auth.Store().Validate(resp.SessionID) {
		t.Error("returned session_id does not validate in the store")
	}
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

func TestTurnHandler_BearerToken_Returns501(t *testing.T) {
	deps, _ := newTestDeps()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	router := server.New(cfg, deps)

	body := `{"messages":[{"role":"user","content":"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/turn", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake.jwt.token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("/turn with Bearer: status = %d; want 501", rr.Code)
	}
}
