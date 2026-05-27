# 2-iii — Gemini Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `relay/internal/llm/gemini` as a `llm.Provider` backed by `google.golang.org/genai`, wire it into the `providers` factory, and cover it with credit-free mock-HTTP unit tests.

**Architecture:** The `gemini.Provider` mirrors `anthropic.go` exactly: a goroutine ranges over `GenerateContentStream`'s `iter.Seq2` iterator, emits delta/terminal/error `llm.ChatChunk` values on a buffered channel with best-effort sends, and respects `ctx` cancellation. `system`-role `llm.Message` values map to `GenerateContentConfig.SystemInstruction`; all other messages map to `[]*genai.Content`. Mock injection uses `genai.ClientConfig{HTTPClient: srv.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL}}` so the unit tests hit an `httptest.Server` instead of the real Gemini API.

**Tech Stack:** Go 1.23.2 · `google.golang.org/genai v0.7.0` (exact pin, no caret) · `net/http/httptest` for mock · `iter.Seq2` (Go 1.23 range-over-func)

---

## Pre-condition

Sub-plan 2-i (Config + factory) is complete and merged. The following already exist:

- `relay/internal/config/config.go` — has `GeminiConfig{APIKeyEnv, Model, MaxTokens}` and `applyDefaults` setting `gemini.model = "gemini-2.0-flash"`, `gemini.api_key_env = "GEMINI_API_KEY"`, `gemini.max_tokens = 1024`
- `relay/internal/llm/providers/providers.go` — has a `"gemini"` case returning `fmt.Errorf("gemini provider: not implemented in this build")`
- `relay/internal/llm/providers/providers_test.go` — has a `"gemini"` sub-test asserting the not-implemented error

Verify before starting:

```bash
cd C:/src/ai/intake/relay
go build ./...
echo BUILD_OK
go test ./...
echo TEST_OK
```

Expected: `BUILD_OK` and `TEST_OK` with no failures.

---

## SDK API Assumptions (VERIFIED — flag for implementer adaptation)

The following were confirmed by `go doc google.golang.org/genai@v0.7.0` and source inspection. Flag 🚩 marks points to double-check if behavior is unexpected.

| Assumption | Confirmed |
|---|---|
| `genai.ClientConfig.HTTPClient *http.Client` injects a custom transport | Yes — `api_client.go:140` |
| `genai.ClientConfig.HTTPOptions.BaseURL string` overrides the base URL | Yes — `api_client.go:87` |
| Default `APIVersion` for Gemini API (non-Vertex) backend is `"v1beta"` | Yes — `client.go:200` |
| Stream URL pattern: `POST {BaseURL}/v1beta/{model}:streamGenerateContent?alt=sse` | Yes — `models.go:3969` |
| SSE format: lines starting with `data: ` (space after colon) followed by JSON; blank lines ignored | Yes — `api_client.go:185` |
| `GenerateContentConfig.SystemInstruction *genai.Content` holds system prompt | Yes — `types.go` |
| `GenerateContentConfig.MaxOutputTokens *int32` sets token limit | Yes — `types.go` |
| `GenerateContentResponse.Candidates[0].Content.Parts[0].Text` carries delta text | Yes — `types.go` (Part.Text field) |
| `GenerateContentResponse.UsageMetadata.PromptTokenCount *int32` — pointer, may be nil | Yes — `types.go` |
| `GenerateContentResponse.UsageMetadata.CandidatesTokenCount *int32` — pointer, may be nil | Yes — `types.go` |
| `Content.Role` uses `"user"` and `"model"` (NOT `"assistant"`) for Gemini API | Yes — Gemini API spec; `"assistant"` is NOT valid |
| 🚩 `google.golang.org/genai v0.7.0` is compatible with `go 1.23` (no `go 1.24` requirement) | Yes — `v0.7.0/go.mod` specifies `go 1.23`; the v1.x series requires 1.24 — do NOT upgrade beyond v0.x |
| 🚩 `iter.Seq2` is available in Go 1.23 | Yes — standard library `iter` package added in 1.23 |
| 🚩 Usage metadata appears on the **final** stream chunk (with `finishReason: "STOP"`) — not on every chunk | Yes — Gemini streams usage only in the terminal response; emit terminal chunk after the loop ends |
| `NewContentFromText(text, role)` builds a `*Content` with one text `Part` | Yes — confirmed by `go doc` |

**Critical role-mapping note:** When building `[]*genai.Content` for the `contents` parameter, map `llm.Message.Role == "assistant"` to `genai.Content.Role = "model"`. Passing `"assistant"` will cause a Gemini API error. The `system` role is extracted separately into `SystemInstruction` (not passed in `contents`).

**🚩 Mock server path:** The mock server must accept `POST` to any path ending in `:streamGenerateContent` (the model name is part of the URL). Use `strings.HasSuffix(r.URL.Path, ":streamGenerateContent")` in the handler or simply accept all `POST` requests. The query string `?alt=sse` is appended by the SDK — the handler can ignore it.

---

## Files Created or Modified

| File | Action | Purpose |
|---|---|---|
| `relay/internal/llm/gemini/gemini.go` | Create | `Provider` struct, `New`, `NewWithClient`, `Name`, `Chat` |
| `relay/internal/llm/gemini/gemini_test.go` | Create | Mock-HTTP unit tests: streaming, ctx-cancel, key-redaction |
| `relay/internal/llm/gemini/integration_test.go` | Create | Live integration test (build-tagged, never run in CI) |
| `relay/internal/llm/providers/providers.go` | Modify | Replace gemini not-implemented case with real construction |
| `relay/internal/llm/providers/providers_test.go` | Modify | Update gemini sub-test to assert `Name() == "gemini"` |
| `relay/go.mod` | Modify | Add `google.golang.org/genai v0.7.0` as direct dependency |
| `relay/go.sum` | Modify | Updated by `go mod tidy` |
| `ai/tasks/phase-2/README.md` | Modify | Update §5 pin table with `google.golang.org/genai v0.7.0` |
| `scripts/check-pins.sh` | Modify | Add caret/`@latest` gate for `google.golang.org/genai` |

---

## Task 1 — Add and Exact-Pin the `google.golang.org/genai` Dependency

**Files:**
- Modify: `relay/go.mod`
- Modify: `relay/go.sum`
- Modify: `ai/tasks/phase-2/README.md` (§5 pin table)
- Modify: `scripts/check-pins.sh`

### Step 1.1 — Install genai at the exact version compatible with Go 1.23

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go get google.golang.org/genai@v0.7.0
go mod tidy
```

Expected output (exact versions may vary for transitive deps — the genai version must be `v0.7.0`):

```
go: downloading google.golang.org/genai v0.7.0
go: added google.golang.org/genai v0.7.0
```

### Step 1.2 — Verify the pin is exact (no caret)

- [ ] Run:

```bash
grep 'google.golang.org/genai' C:/src/ai/intake/relay/go.mod
```

Expected output (must NOT contain `^`):

```
google.golang.org/genai v0.7.0
```

If the line shows `^v0.7.0` or `@latest`, stop — the pin is wrong. Re-run `go get google.golang.org/genai@v0.7.0` (exact tag, no caret).

### Step 1.3 — Verify build still passes

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go build ./...
echo BUILD_OK
go vet ./...
echo VET_OK
go test ./...
echo TEST_OK
```

Expected: all three `_OK` echoes, no failures.

### Step 1.4 — Update README §5 pin table

- [ ] Edit `C:/src/ai/intake/ai/tasks/phase-2/README.md`. Replace the `google.golang.org/genai` row in §5:

Find:
```
| google.golang.org/genai | verify+pin exact at install (2-iii) | Gemini GenerateContentStream + usageMetadata; **exact** |
```

Replace with:
```
| google.golang.org/genai | v0.7.0 | Gemini GenerateContentStream + usageMetadata; **exact** (v0.x required — v1.x needs go 1.24) |
```

### Step 1.5 — Extend `scripts/check-pins.sh` to gate genai

- [ ] Edit `C:/src/ai/intake/scripts/check-pins.sh`. After the existing anthropic-sdk-go gate block, add:

```bash
# Gate: google.golang.org/genai must be exact-pinned (no caret, no @latest) in go.mod.
if grep -E 'google.golang.org/genai' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: google.golang.org/genai is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
```

### Step 1.6 — Run the pin check

- [ ] Run:

```bash
cd C:/src/ai/intake
bash scripts/check-pins.sh
echo PINS_OK
```

Expected:

```
OK: Go module pins verified (go.sum enforces exact versions for santhosh-tekuri/jsonschema/v6 and google/uuid)
OK: all codegen tools are exact-pinned
PINS_OK
```

### Step 1.7 — Commit

- [ ] Run:

```bash
cd C:/src/ai/intake
git add relay/go.mod relay/go.sum scripts/check-pins.sh ai/tasks/phase-2/README.md
git commit -m "2-iii: pin google.golang.org/genai v0.7.0; extend check-pins gate"
```

---

## Task 2 — Implement `relay/internal/llm/gemini/gemini.go`

**Files:**
- Create: `relay/internal/llm/gemini/gemini.go`

### Step 2.1 — Write the implementation file

- [ ] Create `C:/src/ai/intake/relay/internal/llm/gemini/gemini.go` with the following content:

```go
// Package gemini implements the llm.Provider interface using the Google Gemini
// API via the official google.golang.org/genai SDK with streaming enabled.
// The API key is always passed in by the caller — it is never read from disk,
// config files, or logs.
package gemini

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/genai"

	"intake/internal/llm"
)

// Provider implements llm.Provider for the Google Gemini API.
type Provider struct {
	client    *genai.Client
	model     string
	maxTokens int
}

// Compile-time assertion: *Provider must satisfy llm.Provider.
var _ llm.Provider = (*Provider)(nil)

// New creates a production Provider. apiKey is the raw API key value
// (the caller resolves it from os.Getenv(config.LLM.Gemini.APIKeyEnv)).
// The key is passed to the SDK and never stored in a log-accessible field.
func New(apiKey, model string, maxTokens int) *Provider {
	ctx := context.Background()
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		// genai.NewClient only errors when the config is invalid (e.g. both
		// APIKey and Credentials set). For a simple APIKey config it is safe to
		// panic — the caller (providers.New) passes a validated key.
		panic(fmt.Sprintf("gemini: NewClient: %v", err))
	}
	return &Provider{client: c, model: model, maxTokens: maxTokens}
}

// NewWithClient is used by tests. It injects a custom *http.Client and base URL
// so the provider points at an httptest.Server without a real key.
// This mirrors anthropic.NewWithClient.
func NewWithClient(apiKey, model string, maxTokens int, httpClient *http.Client, baseURL string) *Provider {
	ctx := context.Background()
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: baseURL,
		},
	})
	if err != nil {
		panic(fmt.Sprintf("gemini: NewWithClient: %v", err))
	}
	return &Provider{client: c, model: model, maxTokens: maxTokens}
}

// Name returns the provider identifier string.
func (p *Provider) Name() string { return "gemini" }

// Chat opens a streaming GenerateContent request and returns a channel of
// ChatChunk values. Each non-terminal chunk carries a Delta string. The
// terminal chunk (Done=true) carries InputTokens and OutputTokens from the
// final usageMetadata block. If an error occurs, a terminal chunk with Err
// set is emitted and the channel is closed.
//
// The caller must drain the channel to completion; closing the context cancels
// the upstream HTTP request.
//
// System-role messages are separated into Gemini's top-level SystemInstruction
// field; all other messages go into the contents slice. The Gemini API uses
// "model" (not "assistant") as the role for assistant turns.
func (p *Provider) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}

	// Split system prompt from conversation messages.
	// Gemini's API keeps SystemInstruction separate from the contents slice.
	var systemText string
	var contents []*genai.Content
	for _, m := range messages {
		switch m.Role {
		case "system":
			systemText += m.Content
		case "user":
			contents = append(contents, genai.NewContentFromText(m.Content, "user"))
		case "assistant":
			// Gemini uses "model" not "assistant" for the AI role.
			contents = append(contents, genai.NewContentFromText(m.Content, "model"))
		}
	}

	maxTokensI32 := int32(maxTokens)
	cfg := &genai.GenerateContentConfig{
		MaxOutputTokens: &maxTokensI32,
	}
	if systemText != "" {
		cfg.SystemInstruction = genai.NewContentFromText(systemText, "system")
	}

	iter := p.client.Models.GenerateContentStream(ctx, model, contents, cfg)

	ch := make(chan llm.ChatChunk, 32)

	go func() {
		defer close(ch)

		var inputTokens int
		var outputTokens int

		for resp, err := range iter {
			if err != nil {
				select {
				case ch <- llm.ChatChunk{Err: redactedErr(err), Done: true}:
				default:
				}
				return
			}

			// Extract text deltas from the first candidate's parts.
			if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
				for _, part := range resp.Candidates[0].Content.Parts {
					if part.Text != "" {
						select {
						case ch <- llm.ChatChunk{Delta: part.Text}:
						case <-ctx.Done():
							select {
							case ch <- llm.ChatChunk{Err: ctx.Err(), Done: true}:
							default:
							}
							return
						}
					}
				}
			}

			// Capture usage when it arrives (present on the final chunk).
			if resp.UsageMetadata != nil {
				if resp.UsageMetadata.PromptTokenCount != nil {
					inputTokens = int(*resp.UsageMetadata.PromptTokenCount)
				}
				if resp.UsageMetadata.CandidatesTokenCount != nil {
					outputTokens = int(*resp.UsageMetadata.CandidatesTokenCount)
				}
			}
		}

		// Stream ended normally; emit the terminal Done chunk with accumulated usage.
		select {
		case ch <- llm.ChatChunk{Done: true, InputTokens: inputTokens, OutputTokens: outputTokens}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// redactedErr wraps the original error with %w, ensuring the Gemini API key is
// never echoed. The SDK errors do not embed the key, but this is a defensive
// belt-and-suspenders guard required by the §2 security invariant and README
// §7 build-fail checklist.
func redactedErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("gemini provider error: %w", err)
}
```

### Step 2.2 — Verify the file compiles

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go build ./internal/llm/gemini/
echo BUILD_OK
go vet ./internal/llm/gemini/
echo VET_OK
```

Expected: `BUILD_OK` and `VET_OK` with no errors.

If you see `undefined: genai.NewContentFromText`, confirm the genai version is `v0.7.0` and the function exists (`go doc google.golang.org/genai NewContentFromText`). If the function has a different name in the installed version, adapt the call: build a `*genai.Content` with `Parts: []*genai.Part{{Text: m.Content}}, Role: role` directly.

---

## Task 3 — Write Unit Tests for the Gemini Provider (Credit-Free)

**Files:**
- Create: `relay/internal/llm/gemini/gemini_test.go`

The mock server emits SSE lines in the format `data: <JSON>\n` (with a leading space after `data:`). The genai SDK's SSE parser (`api_client.go`) uses `bytes.Cut(line, []byte(":"))` — so `data` is the prefix and ` <JSON>` is the data. The SDK then calls `json.Unmarshal(data, &respRaw)` on the raw bytes including the leading space, which `encoding/json` handles fine.

The SDK hits: `POST {BaseURL}/v1beta/{model}:streamGenerateContent?alt=sse`

The mock handler accepts any `POST` to any path (the model name is embedded in the path but irrelevant for testing).

### Step 3.1 — Write the test file

- [ ] Create `C:/src/ai/intake/relay/internal/llm/gemini/gemini_test.go` with the following content:

```go
package gemini_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/llm"
	geminipkg "intake/internal/llm/gemini"
)

// cannedStream is a minimal Gemini GenerateContentStream SSE response that
// emits two text deltas ("Hello", ", world") and a terminal usage block
// (promptTokenCount=10, candidatesTokenCount=7).
//
// SSE format used by google.golang.org/genai: lines starting with "data: "
// (note the space), each containing a JSON-encoded GenerateContentResponse.
// Blank lines are ignored. No "event:" lines are used.
const cannedStream = "" +
	`data: {"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"}}]}` + "\n" +
	`data: {"candidates":[{"content":{"parts":[{"text":", world"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":7,"totalTokenCount":17}}` + "\n"

// newMockServer returns an httptest.Server that serves the canned SSE stream
// for any POST request and returns its URL.
func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, cannedStream)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	return srv
}

func TestChat_MockStreaming(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	p := geminipkg.NewWithClient(
		"test-key-not-real",
		"gemini-2.0-flash",
		1024,
		srv.Client(),
		srv.URL,
	)

	ctx := context.Background()
	messages := []llm.Message{
		{Role: "user", Content: "say hello"},
	}
	opts := llm.ChatOptions{
		Model:     "gemini-2.0-flash",
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

	// Assert the terminal Done chunk carries usage from the canned response.
	if !terminalChunk.Done {
		t.Error("expected a Done=true terminal chunk")
	}
	if terminalChunk.InputTokens != 10 {
		t.Errorf("InputTokens: got %d, want 10", terminalChunk.InputTokens)
	}
	if terminalChunk.OutputTokens != 7 {
		t.Errorf("OutputTokens: got %d, want 7", terminalChunk.OutputTokens)
	}
}

func TestChat_SystemMessage(t *testing.T) {
	// Verifies that a system-role message is accepted without error
	// (it maps to SystemInstruction, not to the contents slice).
	srv := newMockServer(t)
	defer srv.Close()

	p := geminipkg.NewWithClient(
		"test-key-not-real",
		"gemini-2.0-flash",
		1024,
		srv.Client(),
		srv.URL,
	)

	ctx := context.Background()
	messages := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "hi"},
	}
	opts := llm.ChatOptions{Stream: true}

	ch, err := p.Chat(ctx, messages, opts)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	var gotDone bool
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected error chunk: %v", chunk.Err)
		}
		if chunk.Done {
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("expected Done=true terminal chunk")
	}
}

func TestChat_ContextCancellation(t *testing.T) {
	// The mock server blocks until the client disconnects.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Write one delta then block; the test cancels before we get further.
		fmt.Fprint(w, `data: {"candidates":[{"content":{"parts":[{"text":"partial"}],"role":"model"}}]}`+"\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := geminipkg.NewWithClient(
		"test-key-not-real",
		"gemini-2.0-flash",
		1024,
		srv.Client(),
		srv.URL,
	)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Chat(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{Stream: true})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	// Read the first (partial) chunk, then cancel.
	first := <-ch
	if first.Delta != "partial" {
		t.Errorf("expected 'partial', got %q", first.Delta)
	}
	cancel()

	// Drain the channel with a timeout; a hang here means a goroutine leaked.
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

func TestName(t *testing.T) {
	p := geminipkg.NewWithClient("key", "gemini-2.0-flash", 1024, http.DefaultClient, "http://unused")
	if p.Name() != "gemini" {
		t.Errorf("Name(): got %q, want %q", p.Name(), "gemini")
	}
}

func TestKeyNotInError(t *testing.T) {
	// Verifies that if the provider encounters an HTTP error, the API key
	// does NOT appear in the returned error text.
	const sensitiveKey = "AIzaSy-SUPER-SECRET-GEMINI-KEY-ABCDEF"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":401,"message":"API key not valid","status":"UNAUTHENTICATED"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := geminipkg.NewWithClient(
		sensitiveKey,
		"gemini-2.0-flash",
		1024,
		srv.Client(),
		srv.URL,
	)

	ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{Stream: true})

	// The error may surface either synchronously (err != nil) or as a terminal
	// chunk with Err set — handle both.
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

	if strings.Contains(errText, sensitiveKey) {
		t.Errorf("API key leaked into error message: %q", errText)
	}
}
```

### Step 3.2 — Run the unit tests to verify they pass

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go test -race -v ./internal/llm/gemini/
```

Expected output (order may vary):

```
=== RUN   TestChat_MockStreaming
--- PASS: TestChat_MockStreaming (0.00s)
=== RUN   TestChat_SystemMessage
--- PASS: TestChat_SystemMessage (0.00s)
=== RUN   TestChat_ContextCancellation
--- PASS: TestChat_ContextCancellation (0.00s)
=== RUN   TestName
--- PASS: TestName (0.00s)
=== RUN   TestKeyNotInError
--- PASS: TestKeyNotInError (0.00s)
PASS
ok      intake/internal/llm/gemini      0.XXXs
```

No test should be skipped or failed. If `TestChat_MockStreaming` fails with "expected 2 delta chunks, got 0", the SSE format is likely wrong — check `cannedStream` matches the SDK's parser (lines must start with `data:` followed by a space, then JSON; the SDK splits on the first `:` so `data :<JSON>` would break it). See Task 3, Step 3.1 comment for the format details.

If `TestChat_ContextCancellation` hangs, the goroutine is leaking — the `select { case <-ctx.Done(): }` guard in `gemini.go` is missing or unreachable.

### Step 3.3 — Commit the provider and unit tests

- [ ] Run:

```bash
cd C:/src/ai/intake
git add relay/internal/llm/gemini/gemini.go relay/internal/llm/gemini/gemini_test.go
git commit -m "2-iii: add gemini provider with mock-HTTP unit tests"
```

---

## Task 4 — Write the Integration Test (Credit-Free Gate — Do NOT Run)

**Files:**
- Create: `relay/internal/llm/gemini/integration_test.go`

This test is build-tagged `integration` and excluded from `go test ./...`. It performs a real Gemini API call and costs credits. **Do NOT run it during implementation.** It is written here for completeness and deferred to the live smoke gate.

### Step 4.1 — Write the integration test file

- [ ] Create `C:/src/ai/intake/relay/internal/llm/gemini/integration_test.go` with the following content:

```go
//go:build integration

package gemini_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"intake/internal/llm"
	geminipkg "intake/internal/llm/gemini"
)

// TestIntegration_RealStream performs a live Gemini API call.
// It is excluded from the default test run (go test ./...) and CI.
//
// To run:
//
//	export GEMINI_API_KEY=AIzaSy...
//	cd relay
//	go test -tags integration -v ./internal/llm/gemini/
//
// The test asserts:
//  1. At least one delta chunk arrives with non-empty text.
//  2. The terminal Done chunk has non-zero InputTokens and OutputTokens.
//  3. The joined deltas contain a recognisable word (non-empty assistant reply).
//  4. The API key does not appear in any error message.
func TestIntegration_RealStream(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set — skipping integration test")
	}

	p := geminipkg.New(apiKey, "gemini-2.0-flash", 64)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messages := []llm.Message{
		{Role: "user", Content: "Reply with exactly three words."},
	}
	opts := llm.ChatOptions{
		Model:     "gemini-2.0-flash",
		MaxTokens: 64,
		Stream:    true,
	}

	ch, err := p.Chat(ctx, messages, opts)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	var allDeltas []string
	var terminal llm.ChatChunk
	for chunk := range ch {
		if chunk.Err != nil {
			// Paranoia: even on error, the key must not appear in the message.
			if strings.Contains(chunk.Err.Error(), apiKey) {
				t.Fatalf("API key leaked in error: %v", chunk.Err)
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
	if terminal.InputTokens == 0 {
		t.Errorf("InputTokens is 0 — expected non-zero for a real call")
	}
	if terminal.OutputTokens == 0 {
		t.Errorf("OutputTokens is 0 — expected non-zero for a real call")
	}

	t.Logf("Token usage — input: %d, output: %d", terminal.InputTokens, terminal.OutputTokens)
}
```

### Step 4.2 — Verify the integration test is excluded from `go test ./...`

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go test ./internal/llm/gemini/
echo TEST_OK
```

Expected: `PASS` and `TEST_OK`. The integration test must NOT appear in the output (it is build-tagged out).

### Step 4.3 — Commit the integration test

- [ ] Run:

```bash
cd C:/src/ai/intake
git add relay/internal/llm/gemini/integration_test.go
git commit -m "2-iii: add gemini integration test (build-tagged, credit-gated)"
```

---

## Task 5 — Wire the Factory: Replace the Not-Implemented Gemini Case

**Files:**
- Modify: `relay/internal/llm/providers/providers.go`
- Modify: `relay/internal/llm/providers/providers_test.go`

At this point, `providers.go` was created in sub-plan 2-i. It has a `"gemini"` case that returns an error. This task replaces that case with a real construction.

### Step 5.1 — Read the current providers.go

- [ ] Read `C:/src/ai/intake/relay/internal/llm/providers/providers.go` and locate the `"gemini"` case. It should look like:

```go
case "gemini":
    return nil, fmt.Errorf("gemini provider: not implemented in this build")
```

### Step 5.2 — Add the gemini import and replace the case

- [ ] Edit `C:/src/ai/intake/relay/internal/llm/providers/providers.go`:

Add the import for the gemini package. In the import block, alongside the existing provider imports, add:

```go
geminipkg "intake/internal/llm/gemini"
```

Replace the `"gemini"` case:

Find:
```go
case "gemini":
    return nil, fmt.Errorf("gemini provider: not implemented in this build")
```

Replace with:
```go
case "gemini":
    key, err := config.RequireSecret(cfg.Gemini.APIKeyEnv)
    if err != nil {
        return nil, fmt.Errorf("providers: gemini: %w", err)
    }
    return geminipkg.New(key, cfg.Gemini.Model, cfg.Gemini.MaxTokens), nil
```

### Step 5.3 — Verify the factory compiles

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go build ./internal/llm/providers/
echo BUILD_OK
go vet ./internal/llm/providers/
echo VET_OK
```

Expected: `BUILD_OK` and `VET_OK`.

### Step 5.4 — Update the factory test for the gemini case

- [ ] Read `C:/src/ai/intake/relay/internal/llm/providers/providers_test.go` and find the gemini sub-test. It currently asserts that `provider: "gemini"` returns an error. Replace that sub-test so it asserts `Name() == "gemini"` when the key is set.

Locate the gemini sub-test block. It will look similar to:

```go
t.Run("gemini not-implemented", func(t *testing.T) {
    cfg := config.LLMConfig{Provider: "gemini"}
    _, err := providers.New(cfg)
    if err == nil {
        t.Fatal("expected error for gemini not-implemented, got nil")
    }
})
```

Replace it with:

```go
t.Run("gemini", func(t *testing.T) {
    t.Setenv("GEMINI_API_KEY", "test-key-not-real")
    cfg := config.LLMConfig{
        Provider: "gemini",
        Gemini: config.GeminiConfig{
            APIKeyEnv: "GEMINI_API_KEY",
            Model:     "gemini-2.0-flash",
            MaxTokens: 1024,
        },
    }
    p, err := providers.New(cfg)
    if err != nil {
        t.Fatalf("providers.New() error: %v", err)
    }
    if p.Name() != "gemini" {
        t.Errorf("Name(): got %q, want %q", p.Name(), "gemini")
    }
})
```

Also add a sub-test for the missing-key error case:

```go
t.Run("gemini missing key", func(t *testing.T) {
    cfg := config.LLMConfig{
        Provider: "gemini",
        Gemini: config.GeminiConfig{
            APIKeyEnv: "GEMINI_API_KEY_MISSING_IN_TEST",
            Model:     "gemini-2.0-flash",
            MaxTokens: 1024,
        },
    }
    _, err := providers.New(cfg)
    if err == nil {
        t.Fatal("expected error for missing GEMINI_API_KEY, got nil")
    }
})
```

### Step 5.5 — Run the factory tests

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go test -race -v ./internal/llm/providers/
```

Expected: all sub-tests pass including `gemini`, `gemini missing key`. No test should fail.

### Step 5.6 — Run the full test suite

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go test -race ./...
echo TEST_OK
```

Expected: `TEST_OK` with no failures. No real API calls are made.

### Step 5.7 — Commit the factory wiring

- [ ] Run:

```bash
cd C:/src/ai/intake
git add relay/internal/llm/providers/providers.go relay/internal/llm/providers/providers_test.go
git commit -m "2-iii: wire gemini provider into factory; update factory test"
```

---

## Task 6 — Final Verification

Run all verification commands and paste the actual output into a comment on the task file (or check the boxes below after confirming output).

### Step 6.1 — Full build + vet + test

- [ ] Run:

```bash
cd C:/src/ai/intake/relay
go build ./...
echo BUILD_OK
go vet ./...
echo VET_OK
go test -race ./internal/llm/gemini/ ./internal/llm/providers/
echo PACKAGE_TESTS_OK
go test -race ./...
echo ALL_TESTS_OK
```

Expected: `BUILD_OK`, `VET_OK`, `PACKAGE_TESTS_OK`, `ALL_TESTS_OK` with no failures.

### Step 6.2 — Pin check and contract check

- [ ] Run:

```bash
cd C:/src/ai/intake
bash scripts/check-pins.sh
echo PINS_OK
bash scripts/verify-contract.sh
echo CONTRACT_OK
```

Expected: `PINS_OK` and `CONTRACT_OK`.

### Step 6.3 — Confirm no real API calls were made

The `GEMINI_API_KEY` environment variable must NOT be set during any of the above test runs. Confirm:

```bash
echo "GEMINI_API_KEY is set: ${GEMINI_API_KEY:-<not set>}"
```

Expected: `GEMINI_API_KEY is set: <not set>`. If it is set, unset it and re-run the tests to confirm they still pass (the unit tests must not require it).

---

## 5. Sub-plan Smoke (Deferred — Live, Requires Real Key + Credits)

**DO NOT RUN during implementation.** This is the live 5-turn conversation gate deferred to the phase credit-smoke.

```
Pre-conditions:
  - GEMINI_API_KEY is exported and valid
  - relay built with 2-iii merged
  - config.yaml has:
      llm:
        provider: "gemini"
        gemini:
          api_key_env: "GEMINI_API_KEY"
          model: "gemini-2.0-flash"
          max_tokens: 1024

Steps:
  1. cd C:/src/ai/intake/relay && go build -o relay . && ./relay -config config.yaml
  2. Confirm: curl http://localhost:8080/v1/health returns {"status":"ok"}
  3. RELAY_URL=http://localhost:8080 npx tsx core/smoke/drive-multi.ts

Verification:
  - All 5 turns stream assistant tokens (SSE deltas)
  - Each terminal frame carries non-zero input_tokens/output_tokens
  - GEMINI_API_KEY is ABSENT from relay logs
  - Model id "gemini-2.0-flash" resolves (if not, update config to a valid model id)
```

---

## 6. Done Criteria

All of the following must be true before 2-iii is considered complete:

- [ ] `google.golang.org/genai v0.7.0` is in `relay/go.mod` as a direct dep, exact-pinned (no caret)
- [ ] `scripts/check-pins.sh` gates caret/`@latest` for `google.golang.org/genai` and passes
- [ ] `ai/tasks/phase-2/README.md` §5 pin table shows `google.golang.org/genai v0.7.0`
- [ ] `relay/internal/llm/gemini/gemini.go` exists with `New`, `NewWithClient`, `Name() == "gemini"`, `Chat` (streaming), `var _ llm.Provider = (*Provider)(nil)`
- [ ] `relay/internal/llm/gemini/gemini_test.go` has: mock-streaming test (ordered deltas + usage), system-message test, ctx-cancel test, Name test, key-redaction test — all credit-free
- [ ] `relay/internal/llm/gemini/integration_test.go` exists with `//go:build integration` and is NOT run by `go test ./...`
- [ ] `relay/internal/llm/providers/providers.go` gemini case calls `RequireSecret` → `gemini.New` (not the not-implemented error)
- [ ] `relay/internal/llm/providers/providers_test.go` gemini sub-test asserts `Name() == "gemini"`; missing-key sub-test asserts error
- [ ] `go build ./... && go vet ./... && go test -race ./...` all pass from `relay/`
- [ ] `bash scripts/check-pins.sh && bash scripts/verify-contract.sh` both pass from repo root
- [ ] No real Gemini API calls are made during `go test ./...` (GEMINI_API_KEY unset, all unit tests pass)
- [ ] Live smoke deferred to phase credit gate (5-turn conversation through gemini with non-zero usage tokens, key absent from logs)

---

## Appendix: Mock HTTP Injection — How It Works

The `google.golang.org/genai` SDK supports clean mock injection via `ClientConfig`:

```go
c, _ := genai.NewClient(ctx, &genai.ClientConfig{
    APIKey:     "test-key",
    HTTPClient: srv.Client(),      // httptest server's transport
    HTTPOptions: genai.HTTPOptions{
        BaseURL: srv.URL,          // e.g. "http://127.0.0.1:12345"
    },
})
```

The SDK builds the request URL as `{BaseURL}/{APIVersion}/{model}:streamGenerateContent?alt=sse`, where `APIVersion` defaults to `"v1beta"` for the Gemini API backend. So the mock server sees:

```
POST /v1beta/gemini-2.0-flash:streamGenerateContent?alt=sse
```

The mock handler accepts any POST (path irrelevant for unit tests) and responds with SSE lines in the format `data: <JSON>\n`. Blank lines are ignored by the SDK's scanner. The JSON must be a valid `GenerateContentResponse` shape — specifically the SDK deserializes it through `generateContentResponseFromMldev`, which extracts `candidates`, `usageMetadata`, etc. from the raw JSON map directly (no special envelope).

This approach is identical to `anthropic.NewWithClient` and requires no test-only interfaces or build tags.
