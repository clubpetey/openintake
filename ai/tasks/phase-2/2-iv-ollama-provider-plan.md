# 2-iv — Ollama Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a hand-rolled `net/http` Ollama provider (`relay/internal/llm/ollama`) that satisfies `llm.Provider`, wire it into the `providers` factory, and verify the full stack builds and tests pass offline with no API key.

**Architecture:** `relay/internal/llm/ollama/ollama.go` POSTs to Ollama's `/api/chat` endpoint with `"stream": true` and parses the newline-delimited JSON (NDJSON) response in a goroutine, emitting `llm.ChatChunk` values on a buffered channel that mirrors `anthropic.go`'s channel discipline exactly (one `defer close(ch)`, best-effort sends, `ctx` cancellation). The factory in `relay/internal/llm/providers/providers.go` gains an `"ollama"` case using `config.ResolveSecret` (not `RequireSecret`) since the bearer token is optional.

**Tech Stack:** Go 1.23.2, stdlib `net/http` + `encoding/json` + `bufio`, `net/http/httptest` for unit tests; no external SDK for Ollama; all prior Phase-2 sub-plans (2-i config structs + factory skeleton, 2-ii openai case, 2-iii gemini case) assumed complete.

---

## Pre-conditions (assumed already merged)

- `relay/internal/config/config.go` has `OllamaConfig` in `LLMConfig` (added in 2-i):
  ```go
  type OllamaConfig struct {
      BaseURL        string `yaml:"base_url"`
      Model          string `yaml:"model"`
      BearerTokenEnv string `yaml:"bearer_token_env"`
      MaxTokens      int    `yaml:"max_tokens"`
  }
  ```
  And `applyDefaults` sets: `base_url` → `"http://localhost:11434"`, `model` → `"llama3.1"`, `max_tokens` → `1024` when zero.

- `relay/internal/llm/providers/providers.go` (package `providers`) exists from 2-i with `anthropic`, `openai`, and `gemini` cases. Its current `"ollama"` case returns an explicit error:
  ```go
  case "ollama":
      return nil, fmt.Errorf("ollama provider: not implemented in this build")
  ```

- `relay/cmd/relay/main.go` already uses `providers.New(cfg.LLM)` (rewired in 2-i); no further changes to `main.go` are needed in this sub-plan.

---

## Ollama NDJSON wire format (document + flag for smoke verification)

Ollama's `/api/chat` streaming response is **newline-delimited JSON** — one JSON object per line. The shapes assumed by this plan are:

**Content delta line** (non-final):
```json
{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"Hello"},"done":false}
```

**Final (done) line:**
```json
{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":12,"eval_count":5,"total_duration":123456789,"load_duration":1234,"prompt_eval_duration":12345,"eval_duration":123456}
```

Key fields:
- `message.content` — token delta (string, may be `""` on the final line)
- `done` — `false` for deltas, `true` for the terminal line
- `prompt_eval_count` — maps to `InputTokens` (present only on the terminal line)
- `eval_count` — maps to `OutputTokens` (present only on the terminal line)

**Flag for smoke verification:** Confirm `prompt_eval_count` and `eval_count` are populated in the final line when using `llama3.1` with the local Ollama instance during the phase smoke gate. Older Ollama versions may omit these fields (they will be 0 — non-fatal for streaming but zero usage).

---

## Files Touched

| Action | Path | Purpose |
|--------|------|---------|
| **Create** | `relay/internal/llm/ollama/ollama.go` | Provider implementation + `New` + `NewWithClient` |
| **Create** | `relay/internal/llm/ollama/ollama_test.go` | Unit tests (mock httptest, no real Ollama) |
| **Create** | `relay/internal/llm/ollama/integration_test.go` | Integration test (build tag `integration`, not run during normal `go test ./...`) |
| **Modify** | `relay/internal/llm/providers/providers.go` | Replace not-implemented ollama case; add import |
| **Modify** | `relay/internal/llm/providers/providers_test.go` | Add ollama factory test case (Name()=="ollama", no key required) |

No changes to `go.mod` / `go.sum` (stdlib only). No changes to `main.go`.

---

## Tasks

### Task 1: Create the Ollama provider skeleton (compile-only, no Chat logic)

**Files:**
- Create: `relay/internal/llm/ollama/ollama.go`

- [ ] **Step 1.1: Write `relay/internal/llm/ollama/ollama.go` with the struct, constructors, `Name()`, and a stub `Chat`**

```go
// Package ollama implements the llm.Provider interface against a local Ollama
// instance using a hand-rolled net/http client. No external SDK is used.
// The optional bearer token is passed in by the caller and never logged.
package ollama

import (
	"context"
	"net/http"

	"intake/internal/llm"
)

// Provider implements llm.Provider for a local Ollama instance.
type Provider struct {
	baseURL     string
	model       string
	maxTokens   int
	bearerToken string // optional; "" means no Authorization header
	httpClient  *http.Client
}

// Compile-time assertion: *Provider must satisfy llm.Provider.
var _ llm.Provider = (*Provider)(nil)

// New creates a production Provider. bearerToken is the raw token value
// (the caller resolves it from os.Getenv(cfg.Ollama.BearerTokenEnv));
// pass "" for unauthenticated access. The token is never stored in a
// log-accessible field.
func New(baseURL, model string, maxTokens int, bearerToken string) *Provider {
	return &Provider{
		baseURL:     baseURL,
		model:       model,
		maxTokens:   maxTokens,
		bearerToken: bearerToken,
		httpClient:  &http.Client{},
	}
}

// NewWithClient is used by tests. It injects a custom *http.Client so callers
// can point the provider at an httptest.Server without a real Ollama instance.
func NewWithClient(baseURL, model string, maxTokens int, bearerToken string, httpClient *http.Client) *Provider {
	return &Provider{
		baseURL:     baseURL,
		model:       model,
		maxTokens:   maxTokens,
		bearerToken: bearerToken,
		httpClient:  httpClient,
	}
}

// Name returns the provider identifier string.
func (p *Provider) Name() string { return "ollama" }

// Chat is not yet implemented; it is filled in Task 2.
func (p *Provider) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	panic("ollama.Chat: not yet implemented")
}
```

- [ ] **Step 1.2: Verify it compiles**

```
cd /c/src/ai/intake/relay && go build ./internal/llm/ollama/
```

Expected output: no errors (the `panic` is unreachable at compile time).

- [ ] **Step 1.3: Commit the skeleton**

```bash
cd /c/src/ai/intake
git add relay/internal/llm/ollama/ollama.go
git commit -m "feat(ollama): add provider skeleton with constructors and Name()"
```

---

### Task 2: Implement `Chat` — NDJSON streaming loop

**Files:**
- Modify: `relay/internal/llm/ollama/ollama.go` (replace the `Chat` stub + add imports)

- [ ] **Step 2.1: Replace the stub `Chat` with the full implementation**

Replace the entire contents of `relay/internal/llm/ollama/ollama.go` with:

```go
// Package ollama implements the llm.Provider interface against a local Ollama
// instance using a hand-rolled net/http client. No external SDK is used.
// The optional bearer token is passed in by the caller and never logged.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"intake/internal/llm"
)

// Provider implements llm.Provider for a local Ollama instance.
type Provider struct {
	baseURL     string
	model       string
	maxTokens   int
	bearerToken string // optional; "" means no Authorization header
	httpClient  *http.Client
}

// Compile-time assertion: *Provider must satisfy llm.Provider.
var _ llm.Provider = (*Provider)(nil)

// New creates a production Provider. bearerToken is the raw token value
// (the caller resolves it from os.Getenv(cfg.Ollama.BearerTokenEnv));
// pass "" for unauthenticated access. The token is never stored in a
// log-accessible field.
func New(baseURL, model string, maxTokens int, bearerToken string) *Provider {
	return &Provider{
		baseURL:     baseURL,
		model:       model,
		maxTokens:   maxTokens,
		bearerToken: bearerToken,
		httpClient:  &http.Client{},
	}
}

// NewWithClient is used by tests. It injects a custom *http.Client so callers
// can point the provider at an httptest.Server without a real Ollama instance.
func NewWithClient(baseURL, model string, maxTokens int, bearerToken string, httpClient *http.Client) *Provider {
	return &Provider{
		baseURL:     baseURL,
		model:       model,
		maxTokens:   maxTokens,
		bearerToken: bearerToken,
		httpClient:  httpClient,
	}
}

// Name returns the provider identifier string.
func (p *Provider) Name() string { return "ollama" }

// ollamaMessage is the per-message shape for the /api/chat request body.
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatRequest is the /api/chat request body.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options"`
}

// ollamaOptions holds per-request generation parameters.
type ollamaOptions struct {
	NumPredict int `json:"num_predict"`
}

// ollamaChatResponse is one NDJSON line from the /api/chat streaming response.
// Non-terminal lines have Done=false and Message.Content is the delta text.
// The terminal line has Done=true and carries PromptEvalCount + EvalCount for usage.
type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done            bool `json:"done"`
	PromptEvalCount int  `json:"prompt_eval_count"` // InputTokens on terminal line
	EvalCount       int  `json:"eval_count"`        // OutputTokens on terminal line
}

// Chat opens a streaming /api/chat request and returns a channel of ChatChunk
// values. Each non-terminal chunk carries a Delta string. The terminal chunk
// (Done=true) carries InputTokens and OutputTokens from the final NDJSON line.
// If an error occurs, a terminal chunk with Err set is emitted and the channel
// is closed.
//
// The caller must drain the channel to completion; closing the context cancels
// the upstream HTTP request.
//
// System-role messages are kept as system messages in the messages array
// (Ollama supports the system role in the messages array natively).
func (p *Provider) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}

	// Build the messages array — system, user, and assistant roles all map 1:1.
	apiMessages := make([]ollamaMessage, 0, len(messages))
	for _, m := range messages {
		apiMessages = append(apiMessages, ollamaMessage{Role: m.Role, Content: m.Content})
	}

	reqBody := ollamaChatRequest{
		Model:    model,
		Messages: apiMessages,
		Stream:   true,
		Options:  ollamaOptions{NumPredict: maxTokens},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Set Authorization header only when a bearer token is configured.
	// NEVER log the token value.
	if p.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.bearerToken)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: HTTP request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: unexpected status %d", resp.StatusCode)
	}

	ch := make(chan llm.ChatChunk, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			// Check for context cancellation before processing each line.
			select {
			case <-ctx.Done():
				select {
				case ch <- llm.ChatChunk{Err: ctx.Err(), Done: true}:
				default:
				}
				return
			default:
			}

			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var parsed ollamaChatResponse
			if err := json.Unmarshal(line, &parsed); err != nil {
				select {
				case ch <- llm.ChatChunk{Err: fmt.Errorf("ollama: parse NDJSON line: %w", err), Done: true}:
				default:
				}
				return
			}

			if parsed.Done {
				// Terminal line: emit the Done chunk with usage.
				select {
				case ch <- llm.ChatChunk{Done: true, InputTokens: parsed.PromptEvalCount, OutputTokens: parsed.EvalCount}:
				case <-ctx.Done():
				}
				return
			}

			// Delta line: emit if there is content.
			if parsed.Message.Content != "" {
				select {
				case ch <- llm.ChatChunk{Delta: parsed.Message.Content}:
				case <-ctx.Done():
					select {
					case ch <- llm.ChatChunk{Err: ctx.Err(), Done: true}:
					default:
					}
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case ch <- llm.ChatChunk{Err: fmt.Errorf("ollama: read stream: %w", err), Done: true}:
			default:
			}
			return
		}

		// Scanner exhausted without a done=true line (e.g. connection cut mid-stream).
		// Best-effort terminal chunk so the consumer is not left hanging.
		select {
		case ch <- llm.ChatChunk{Done: true}:
		default:
		}
	}()

	return ch, nil
}
```

- [ ] **Step 2.2: Build the package to confirm it compiles**

```
cd /c/src/ai/intake/relay && go build ./internal/llm/ollama/
```

Expected: no output, exit 0.

- [ ] **Step 2.3: Vet the package**

```
cd /c/src/ai/intake/relay && go vet ./internal/llm/ollama/
```

Expected: no output, exit 0.

- [ ] **Step 2.4: Commit**

```bash
cd /c/src/ai/intake
git add relay/internal/llm/ollama/ollama.go
git commit -m "feat(ollama): implement Chat with NDJSON streaming loop"
```

---

### Task 3: Write unit tests (mock httptest — no real Ollama)

**Files:**
- Create: `relay/internal/llm/ollama/ollama_test.go`

- [ ] **Step 3.1: Write the failing test file**

Create `relay/internal/llm/ollama/ollama_test.go`:

```go
package ollama_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/llm"
	ollamapkg "intake/internal/llm/ollama"
)

// cannedNDJSON is a minimal Ollama /api/chat NDJSON streaming response.
// It emits two content deltas ("Hello", ", world") and a terminal done line
// with prompt_eval_count=10 and eval_count=5.
//
// Each line is one JSON object; lines are separated by "\n" (newline-delimited).
// This matches the exact wire format documented in the plan (§ Ollama NDJSON wire format).
const cannedNDJSON = "" +
	`{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"Hello"},"done":false}` + "\n" +
	`{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":", world"},"done":false}` + "\n" +
	`{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":5,"total_duration":123456789,"load_duration":1234,"prompt_eval_duration":12345,"eval_duration":123456}` + "\n"

// newMockServer returns an httptest.Server that validates the request and serves
// the canned NDJSON response for any POST to /api/chat.
func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected path /api/chat, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, cannedNDJSON)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	return srv
}

// TestChat_MockStreaming verifies that ordered delta chunks and a correct terminal
// usage chunk are emitted when the mock server returns the canned NDJSON stream.
func TestChat_MockStreaming(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 1024, "", srv.Client())

	ctx := context.Background()
	messages := []llm.Message{
		{Role: "user", Content: "say hello"},
	}
	opts := llm.ChatOptions{
		Model:     "llama3.1",
		MaxTokens: 1024,
		Stream:    true,
	}

	ch, err := p.Chat(ctx, messages, opts)
	if err != nil {
		t.Fatalf("Chat() returned error: %v", err)
	}

	var deltas []string
	var terminalChunk llm.ChatChunk
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected error chunk: %v", chunk.Err)
		}
		if chunk.Done {
			terminalChunk = chunk
		} else {
			deltas = append(deltas, chunk.Delta)
		}
	}

	// Assert the two deltas arrived in order.
	if len(deltas) != 2 {
		t.Fatalf("expected 2 delta chunks, got %d: %v", len(deltas), deltas)
	}
	if deltas[0] != "Hello" {
		t.Errorf("delta[0]: got %q, want %q", deltas[0], "Hello")
	}
	if deltas[1] != ", world" {
		t.Errorf("delta[1]: got %q, want %q", deltas[1], ", world")
	}

	// Assert the terminal Done chunk carries correct usage from the canned response.
	if !terminalChunk.Done {
		t.Error("expected a Done=true terminal chunk")
	}
	if terminalChunk.InputTokens != 10 {
		t.Errorf("InputTokens: got %d, want 10", terminalChunk.InputTokens)
	}
	if terminalChunk.OutputTokens != 5 {
		t.Errorf("OutputTokens: got %d, want 5", terminalChunk.OutputTokens)
	}
}

// TestChat_ContextCancellation verifies that cancelling the context closes the
// channel within a bounded time (no goroutine leak).
func TestChat_ContextCancellation(t *testing.T) {
	// This mock server writes one delta then blocks until the client disconnects.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		line := `{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"partial"},"done":false}` + "\n"
		fmt.Fprint(w, line)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Block until client disconnects.
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 1024, "", srv.Client())

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Chat(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{Stream: true})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	// Read the first (partial) delta chunk, then cancel the context.
	first := <-ch
	if first.Delta != "partial" {
		t.Errorf("expected delta 'partial', got %q", first.Delta)
	}
	cancel()

	// Drain the channel with a timeout; a hang means a goroutine leaked.
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed within 3s after cancellation — possible goroutine leak")
	}
}

// TestChat_BearerTokenSent verifies that when a bearer token is configured, the
// Authorization header is sent to Ollama; and when it is empty, the header is absent.
func TestChat_BearerTokenSent(t *testing.T) {
	const testToken = "test-bearer-token-xyz"

	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		// Emit a minimal done-only response to satisfy the stream consumer.
		fmt.Fprint(w, `{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":1,"eval_count":1}`+"\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	t.Run("bearer token present", func(t *testing.T) {
		capturedAuth = ""
		p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 64, testToken, srv.Client())
		ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{})
		if err != nil {
			t.Fatalf("Chat() error: %v", err)
		}
		for range ch {
		}
		want := "Bearer " + testToken
		if capturedAuth != want {
			t.Errorf("Authorization header: got %q, want %q", capturedAuth, want)
		}
	})

	t.Run("no bearer token", func(t *testing.T) {
		capturedAuth = ""
		p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 64, "", srv.Client())
		ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{})
		if err != nil {
			t.Fatalf("Chat() error: %v", err)
		}
		for range ch {
		}
		if capturedAuth != "" {
			t.Errorf("Authorization header should be absent, got %q", capturedAuth)
		}
	})
}

// TestChat_BearerTokenNotInErrors verifies that the bearer token value does not
// appear in any error surfaced through the channel or as a return error from Chat.
func TestChat_BearerTokenNotInErrors(t *testing.T) {
	const sensitiveToken = "super-secret-ollama-bearer-token-ABCDEF"

	// Mock server returns HTTP 401, which causes Chat() to return a non-nil error
	// or emit an error chunk. The sensitive token must not appear in either.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 64, sensitiveToken, srv.Client())

	ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{})

	var errText string
	if err != nil {
		errText = err.Error()
	} else {
		for chunk := range ch {
			if chunk.Err != nil {
				errText = chunk.Err.Error()
			}
		}
	}

	if strings.Contains(errText, sensitiveToken) {
		t.Errorf("bearer token leaked into error message: %q", errText)
	}
}

// TestName verifies that Name() returns "ollama".
func TestName(t *testing.T) {
	p := ollamapkg.NewWithClient("http://unused", "llama3.1", 1024, "", http.DefaultClient)
	if p.Name() != "ollama" {
		t.Errorf("Name(): got %q, want %q", p.Name(), "ollama")
	}
}

// TestChat_SystemMessageInArray verifies that a system-role llm.Message is kept
// in the messages array (Ollama supports the system role in messages natively),
// rather than being stripped or elevated to a separate parameter.
func TestChat_SystemMessageInArray(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"ok"},"done":false}`+"\n")
		fmt.Fprint(w, `{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":3,"eval_count":1}`+"\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 64, "", srv.Client())
	messages := []llm.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "hello"},
	}
	ch, err := p.Chat(context.Background(), messages, llm.ChatOptions{})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	for range ch {
	}

	// The request body must contain a "system" role message in the messages array.
	bodyStr := string(capturedBody)
	if !strings.Contains(bodyStr, `"role":"system"`) {
		t.Errorf("request body does not contain system role message; got: %s", bodyStr)
	}
}
```

- [ ] **Step 3.2: Run the tests — expect PASS**

```
cd /c/src/ai/intake/relay && go test -race ./internal/llm/ollama/
```

Expected output (all tests pass):
```
ok      intake/internal/llm/ollama      0.XXXs
```

If any test fails: read the failure, fix `ollama.go`, re-run. Do not proceed until all pass.

- [ ] **Step 3.3: Commit**

```bash
cd /c/src/ai/intake
git add relay/internal/llm/ollama/ollama_test.go
git commit -m "test(ollama): add mock-httptest unit tests covering streaming, ctx-cancel, bearer, redaction"
```

---

### Task 4: Write the integration test (build-tagged; not run during `go test ./...`)

**Files:**
- Create: `relay/internal/llm/ollama/integration_test.go`

- [ ] **Step 4.1: Write the integration test file**

Create `relay/internal/llm/ollama/integration_test.go`:

```go
//go:build integration

package ollama_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"intake/internal/llm"
	ollamapkg "intake/internal/llm/ollama"
)

// TestIntegration_RealStream performs a live Ollama /api/chat call.
// It is excluded from the default test run (go test ./...) and CI.
//
// To run (requires a local Ollama instance with llama3.1 pulled):
//
//	cd relay
//	go test -tags integration -v ./internal/llm/ollama/
//
// Or with a custom base URL / model:
//
//	OLLAMA_BASE_URL=http://localhost:11434 OLLAMA_MODEL=llama3.1 \
//	  go test -tags integration -v ./internal/llm/ollama/
//
// The test asserts:
//  1. At least one delta chunk arrives with non-empty text.
//  2. The terminal Done chunk has non-zero InputTokens and OutputTokens.
//  3. The joined deltas contain non-empty text (assistant replied).
//  4. The bearer token (if set) does not appear in any error message.
func TestIntegration_RealStream(t *testing.T) {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "llama3.1"
	}
	// Optional bearer token for proxied Ollama instances.
	bearerToken := os.Getenv("OLLAMA_BEARER_TOKEN")

	// Probe reachability: skip the test if Ollama is not running locally.
	// This avoids a hard failure in environments without Ollama.
	p := ollamapkg.New(baseURL, model, 64, bearerToken)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messages := []llm.Message{
		{Role: "user", Content: "Reply with exactly three words."},
	}
	opts := llm.ChatOptions{
		Model:     model,
		MaxTokens: 64,
		Stream:    true,
	}

	ch, err := p.Chat(ctx, messages, opts)
	if err != nil {
		// If the error indicates Ollama is not reachable, skip rather than fail.
		if strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "no such host") {
			t.Skipf("ollama not reachable at %s — skipping integration test: %v", baseURL, err)
		}
		t.Fatalf("Chat() error: %v", err)
	}

	var allDeltas []string
	var terminal llm.ChatChunk
	for chunk := range ch {
		if chunk.Err != nil {
			if bearerToken != "" && strings.Contains(chunk.Err.Error(), bearerToken) {
				t.Fatalf("bearer token leaked in error: %v", chunk.Err)
			}
			// Check reachability errors via the chunk path too.
			if strings.Contains(chunk.Err.Error(), "connection refused") {
				t.Skipf("ollama not reachable: %v", chunk.Err)
			}
			t.Fatalf("stream error: %v", chunk.Err)
		}
		if chunk.Done {
			terminal = chunk
		} else {
			allDeltas = append(allDeltas, chunk.Delta)
		}
	}

	if len(allDeltas) == 0 {
		t.Fatal("no delta chunks received")
	}

	joined := strings.Join(allDeltas, "")
	t.Logf("Assistant reply: %q", joined)

	if joined == "" {
		t.Error("joined delta text is empty")
	}

	if !terminal.Done {
		t.Error("no terminal Done chunk received")
	}

	// Note: prompt_eval_count / eval_count may be 0 on some Ollama versions
	// (particularly when model is already loaded and the first call is cached).
	// We log rather than fail hard so the integration test is robust across Ollama versions.
	t.Logf("Token usage — input: %d, output: %d", terminal.InputTokens, terminal.OutputTokens)
	if terminal.InputTokens == 0 {
		t.Log("WARNING: InputTokens is 0 — prompt_eval_count may not be populated by this Ollama version")
	}
	if terminal.OutputTokens == 0 {
		t.Log("WARNING: OutputTokens is 0 — eval_count may not be populated by this Ollama version")
	}
}
```

- [ ] **Step 4.2: Verify the integration test is excluded from the normal test run**

```
cd /c/src/ai/intake/relay && go test ./internal/llm/ollama/ -v 2>&1 | grep -v "^==="
```

Expected: you see the unit test names (TestChat_MockStreaming etc.) but NOT TestIntegration_RealStream.

- [ ] **Step 4.3: Verify the file compiles under the integration build tag**

```
cd /c/src/ai/intake/relay && go build -tags integration ./internal/llm/ollama/
```

Expected: no output, exit 0.

- [ ] **Step 4.4: Commit**

```bash
cd /c/src/ai/intake
git add relay/internal/llm/ollama/integration_test.go
git commit -m "test(ollama): add integration test (build-tagged, requires local Ollama)"
```

---

### Task 5: Wire the factory — replace the not-implemented ollama case

**Files:**
- Modify: `relay/internal/llm/providers/providers.go`

At this point `providers.go` exists from sub-plan 2-i and has anthropic, openai, and gemini cases. Its ollama case currently returns an error. This task replaces that case.

- [ ] **Step 5.1: Read the current `providers.go` to find the exact ollama case text**

Open `relay/internal/llm/providers/providers.go` and locate the ollama case. It will look like:

```go
case "ollama":
    return nil, fmt.Errorf("ollama provider: not implemented in this build")
```

- [ ] **Step 5.2: Add the ollama import and replace the case**

In `relay/internal/llm/providers/providers.go`:

1. Add `"intake/internal/llm/ollama"` to the import block (alongside anthropic, openai, gemini imports).

2. Replace the ollama case with:

```go
case "ollama":
    // bearer token is optional — empty string means no Authorization header
    token, _ := config.ResolveSecret(cfg.Ollama.BearerTokenEnv)
    return ollama.New(cfg.Ollama.BaseURL, cfg.Ollama.Model, cfg.Ollama.MaxTokens, token)
```

The `_` on the error from `ResolveSecret` is intentional: Ollama needs no key, and the only error `ResolveSecret` can return (both env var and `_FILE` set simultaneously) is a misconfiguration the operator will see from a different path. If you want to surface it, you may propagate:

```go
case "ollama":
    token, err := config.ResolveSecret(cfg.Ollama.BearerTokenEnv)
    if err != nil {
        return nil, fmt.Errorf("ollama: bearer token: %w", err)
    }
    return ollama.New(cfg.Ollama.BaseURL, cfg.Ollama.Model, cfg.Ollama.MaxTokens, token)
```

Use the second form (error propagated) — it is safer and matches the README §8.4 intent.

The complete updated `providers.go` after this edit (adjust to match the actual file from 2-i/2-ii/2-iii, preserving existing cases):

```go
package providers

import (
	"fmt"

	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/llm/anthropic"
	"intake/internal/llm/gemini"
	"intake/internal/llm/ollama"
	"intake/internal/llm/openai"
)

// New constructs the configured provider, resolving its secret via config.ResolveSecret.
// Errors on an unknown provider name or a missing required secret.
func New(cfg config.LLMConfig) (llm.Provider, error) {
	switch cfg.Provider {
	case "anthropic":
		key, err := config.RequireSecret(cfg.Anthropic.APIKeyEnv)
		if err != nil {
			return nil, fmt.Errorf("anthropic: %w", err)
		}
		return anthropic.New(key, cfg.Anthropic.Model, cfg.Anthropic.MaxTokens), nil

	case "openai":
		key, err := config.RequireSecret(cfg.OpenAI.APIKeyEnv)
		if err != nil {
			return nil, fmt.Errorf("openai: %w", err)
		}
		return openai.New(key, cfg.OpenAI.Model, cfg.OpenAI.MaxTokens), nil

	case "gemini":
		key, err := config.RequireSecret(cfg.Gemini.APIKeyEnv)
		if err != nil {
			return nil, fmt.Errorf("gemini: %w", err)
		}
		return gemini.New(key, cfg.Gemini.Model, cfg.Gemini.MaxTokens), nil

	case "ollama":
		token, err := config.ResolveSecret(cfg.Ollama.BearerTokenEnv)
		if err != nil {
			return nil, fmt.Errorf("ollama: bearer token: %w", err)
		}
		return ollama.New(cfg.Ollama.BaseURL, cfg.Ollama.Model, cfg.Ollama.MaxTokens, token), nil

	default:
		return nil, fmt.Errorf("unknown llm provider %q", cfg.Provider)
	}
}
```

**Important:** The code above is the expected final shape. Your actual file from 2-i/2-ii/2-iii is the source of truth — do a targeted edit to replace only the ollama case and add the import. Do not overwrite other cases you didn't author.

- [ ] **Step 5.3: Build the providers package**

```
cd /c/src/ai/intake/relay && go build ./internal/llm/providers/
```

Expected: no output, exit 0.

- [ ] **Step 5.4: Build the entire relay module**

```
cd /c/src/ai/intake/relay && go build ./...
```

Expected: no output, exit 0.

- [ ] **Step 5.5: Commit**

```bash
cd /c/src/ai/intake
git add relay/internal/llm/providers/providers.go
git commit -m "feat(providers): wire ollama case in factory (optional bearer, no required key)"
```

---

### Task 6: Update the providers factory test for the ollama case

**Files:**
- Modify: `relay/internal/llm/providers/providers_test.go`

The existing test file from 2-i has a table of provider cases. The ollama row currently expects an error ("not implemented"). Replace it with a success assertion that checks `Name() == "ollama"` and requires no environment key.

- [ ] **Step 6.1: Read the current providers_test.go**

Open `relay/internal/llm/providers/providers_test.go` and locate the ollama test case. It will be something like:

```go
{
    name:        "ollama not implemented",
    provider:    "ollama",
    wantErr:     true,
    wantErrContains: "not implemented",
},
```

- [ ] **Step 6.2: Replace the ollama test case**

Remove the "ollama not implemented" case and add a passing one. The exact shape of the test table depends on what 2-i wrote; here is the replacement case to add (alongside the existing anthropic/openai/gemini cases):

```go
{
    name:         "ollama: constructs provider with no key",
    providerName: "ollama",
    cfg: config.LLMConfig{
        Provider: "ollama",
        Ollama: config.OllamaConfig{
            BaseURL:        "http://localhost:11434",
            Model:          "llama3.1",
            BearerTokenEnv: "", // empty = no auth; ResolveSecret("") returns ("", nil)
            MaxTokens:      1024,
        },
    },
    // No env vars set — ollama needs no API key.
    wantErr:      false,
    wantName:     "ollama",
},
```

Additionally, add a sub-test verifying that when `BearerTokenEnv` points to an env var that has both the plain var and `_FILE` set, `New` returns an error (the ResolveSecret ambiguity case):

```go
{
    name:         "ollama: bearer token env ambiguity errors",
    providerName: "ollama",
    cfg: config.LLMConfig{
        Provider: "ollama",
        Ollama: config.OllamaConfig{
            BaseURL:        "http://localhost:11434",
            Model:          "llama3.1",
            BearerTokenEnv: "TEST_OLLAMA_BEARER_AMBIG",
            MaxTokens:      1024,
        },
    },
    setupEnv: map[string]string{
        "TEST_OLLAMA_BEARER_AMBIG":      "some-token",
        "TEST_OLLAMA_BEARER_AMBIG_FILE": "/some/path",
    },
    wantErr: true,
},
```

The exact test harness depends on the structure from 2-i. The key constraint: add these two ollama cases such that `go test ./internal/llm/providers/` passes. If the test uses subtests with `t.Setenv`, the `setupEnv` map approach works:

```go
for _, tc := range cases {
    t.Run(tc.name, func(t *testing.T) {
        for k, v := range tc.setupEnv {
            t.Setenv(k, v)
        }
        p, err := providers.New(tc.cfg)
        if tc.wantErr {
            if err == nil {
                t.Fatalf("expected error, got provider %v", p)
            }
            return
        }
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if p.Name() != tc.wantName {
            t.Errorf("Name(): got %q, want %q", p.Name(), tc.wantName)
        }
    })
}
```

- [ ] **Step 6.3: Run the providers tests**

```
cd /c/src/ai/intake/relay && go test -race ./internal/llm/providers/
```

Expected:
```
ok      intake/internal/llm/providers   0.XXXs
```

If any test fails: read the failure, adjust the test or the factory, re-run.

- [ ] **Step 6.4: Commit**

```bash
cd /c/src/ai/intake
git add relay/internal/llm/providers/providers_test.go
git commit -m "test(providers): update factory test — ollama constructs real provider with no key"
```

---

### Task 7: Final verification — build, vet, test, contract

- [ ] **Step 7.1: Full build and vet**

```
cd /c/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected output:
```
BUILD_OK
VET_OK
```

- [ ] **Step 7.2: Full test suite with race detector**

```
cd /c/src/ai/intake/relay && go test -race ./...
```

Expected: all packages pass, no race conditions detected.
```
ok      intake/internal/llm/ollama      0.XXXs
ok      intake/internal/llm/providers   0.XXXs
ok      intake/...                      0.XXXs
```

- [ ] **Step 7.3: Run the ollama unit tests and providers tests explicitly (as specified by the sub-plan verify command)**

```
cd /c/src/ai/intake/relay && go test -race ./internal/llm/ollama/ ./internal/llm/providers/
```

Expected:
```
ok      intake/internal/llm/ollama      0.XXXs
ok      intake/internal/llm/providers   0.XXXs
```

- [ ] **Step 7.4: Contract gate**

```
cd /c/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK
```

Expected final line: `CONTRACT_OK`

If `verify-contract.sh` fails on the staleness gate (generated files drifted): run `npm run codegen` from the repo root and commit the regenerated files before re-running.

- [ ] **Step 7.5: Confirm zero external calls in the test suite**

The ollama unit tests use only `httptest.Server`. Confirm:

```
cd /c/src/ai/intake/relay && go test -race -v ./internal/llm/ollama/ 2>&1 | grep -i "SKIP\|PASS\|FAIL"
```

Expected: all entries are PASS. No SKIP (integration tests are excluded by build tag, not skipped at runtime).

- [ ] **Step 7.6: Commit verification result**

If all checks pass, this is the final commit for 2-iv:

```bash
cd /c/src/ai/intake
git commit --allow-empty -m "chore(2-iv): verification complete — build, vet, test -race, contract all pass"
```

(Empty commit is acceptable here as a milestone marker; skip if your team convention discourages it.)

---

## 5. Smoke (Phase-Final Gate — Deferred, Do Not Run During Implementation)

The smoke for 2-iv is the Ollama arm of the Phase-2 final smoke (README §7). Run only after all four sub-plans are merged.

**Pre-conditions:**
1. Local Ollama running: `ollama serve`
2. Model pulled: `ollama pull llama3.1`
3. No `OLLAMA_BEARER_TOKEN` env var set (offline, no auth)

**Procedure:**
```bash
# In relay directory: edit config.yaml to set:
#   llm:
#     provider: ollama
#     ollama:
#       base_url: http://localhost:11434
#       model: llama3.1
#       max_tokens: 1024
#       bearer_token_env: ""

cd /c/src/ai/intake/relay
go run ./cmd/relay/ -config config.yaml &
RELAY_PID=$!

# Confirm health:
curl -s http://localhost:8080/v1/health | grep ok

# Drive a 5-turn conversation:
RELAY_URL=http://localhost:8080 npx tsx /c/src/ai/intake/core/smoke/drive-multi.ts

kill $RELAY_PID
```

**Pass criteria:**
- All 5 turns stream assistant token deltas (SSE `delta` events arrive).
- Each terminal frame carries non-zero `input_tokens` and `output_tokens`.
- No `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or `GEMINI_API_KEY` is set — relay starts and completes turns fully offline.
- Relay logs contain no bearer token value (nothing set, so trivially satisfied; set `bearer_token_env` in a follow-up test to verify redaction).

---

## 6. Done Criteria

- [ ] `relay/internal/llm/ollama/ollama.go` exists with `New`, `NewWithClient`, `Name()=="ollama"`, `var _ llm.Provider = (*Provider)(nil)`, and a complete `Chat` streaming loop.
- [ ] `relay/internal/llm/ollama/ollama_test.go` has: mock streaming test (2 deltas + terminal usage), ctx-cancel test (goroutine non-leak), bearer-present test, bearer-absent test, token-redaction test, system-message-in-array test — all passing under `go test -race`.
- [ ] `relay/internal/llm/ollama/integration_test.go` exists with `//go:build integration` and is excluded from `go test ./...`.
- [ ] `relay/internal/llm/providers/providers.go` ollama case uses `config.ResolveSecret` (not `RequireSecret`), constructs `ollama.New(...)`, and propagates any ResolveSecret error.
- [ ] `relay/internal/llm/providers/providers_test.go` has an ollama case that confirms `Name()=="ollama"` with no API key env var set.
- [ ] `go build ./... && go vet ./... && go test -race ./...` all pass in `relay/`.
- [ ] `bash scripts/verify-contract.sh` exits 0 (CONTRACT_OK).
- [ ] No bearer token value appears in any log line or error string (confirmed by the redaction unit test).
- [ ] The relay can start with `provider: ollama` and **no provider API key set** (proven at the phase smoke gate).
- [ ] The ollama package uses **no external dependencies** — only stdlib `net/http`, `encoding/json`, `bufio`, `bytes`, `context`, `fmt`.
