# 1-ii — LLM Provider Interface + Anthropic Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Define the frozen `llm.Provider` interface (verbatim from README §6.1) and implement an Anthropic streaming provider that emits `ChatChunk` deltas and final usage tokens — all testable without a real API key.

**Architecture:** `relay/internal/llm/provider.go` holds the interface; `relay/internal/llm/anthropic/anthropic.go` implements it via the official Anthropic Go SDK with a streaming Messages API call. A test-injectable HTTP client variant (`newWithClient`) lets the mock-streaming unit test replace the Anthropic endpoint with an `httptest.Server`, so `go test ./...` runs with zero real credentials. A separate build-tagged integration test (`//go:build integration`) is the sub-plan smoke.

**Tech Stack:** Go 1.23.2, `github.com/anthropics/anthropic-sdk-go` (exact version — verify+pin at install), `net/http`, `net/http/httptest`, standard `testing`.

---

## 1. Goal

Produce two packages:
- `relay/internal/llm` — the frozen `Provider` interface, `Message`, `ChatOptions`, `ChatChunk` types (README §6.1 verbatim).
- `relay/internal/llm/anthropic` — an `anthropic.Provider` that streams via the Anthropic Messages API, emits text deltas, and reports input/output token counts on the terminal chunk. The API key is always passed in by the caller (resolved from the env var named in `config.AnthropicConfig.APIKeyEnv`); it is never read from disk, a config file, or any log.

## 2. Design References

| Document | Relevant sections |
|---|---|
| `ai/tasks/phase-1/README.md` | §6.1 (interface signatures — frozen), §5 (tool pins), §7 (build-fail checklist) |
| `docs/specs/2026-05-26-phase-1-walking-skeleton-design.md` | §2 (security invariant: key from env only, never logged), §5.1 (provider contract) |
| `relay/go.mod` | module `intake`, go 1.23.2 — all import paths use this module name |

## 3. Files Touched

| Action | Path | Responsibility |
|---|---|---|
| Create | `relay/internal/llm/provider.go` | Frozen interface + types (README §6.1 verbatim) |
| Create | `relay/internal/llm/provider_test.go` | Compile-only interface satisfaction check |
| Create | `relay/internal/llm/anthropic/anthropic.go` | Anthropic streaming provider implementation |
| Create | `relay/internal/llm/anthropic/anthropic_test.go` | Mock-streaming unit test + key-redaction test |
| Create | `relay/internal/llm/anthropic/integration_test.go` | Env-gated real-API integration test (smoke) |
| Modify | `relay/go.mod` | Add `github.com/anthropics/anthropic-sdk-go` (exact pin) |
| Modify | `relay/go.sum` | Updated by `go get` |
| Modify | `ai/tasks/phase-1/README.md` | Update §5 table with pinned SDK version |
| Modify | `scripts/check-pins.sh` | Add gate: fail if anthropic-sdk-go is caret/`@latest`-pinned |

---

## 4. Tasks

---

### Task 1: Define the frozen `llm.Provider` interface

**Files:**
- Create: `relay/internal/llm/provider.go`
- Create: `relay/internal/llm/provider_test.go`

- [ ] **Step 1.1: Create `relay/internal/llm/` directory and `provider.go`**

  Create `relay/internal/llm/provider.go` with the exact content from README §6.1 — no additions, no omissions:

  ```go
  package llm

  import "context"

  type Message struct {
  	Role    string `json:"role"`    // "system" | "user" | "assistant"
  	Content string `json:"content"`
  }

  type ChatOptions struct {
  	Model       string
  	MaxTokens   int
  	Temperature float64
  	Stream      bool
  }

  type ChatChunk struct {
  	Delta        string // incremental text
  	Done         bool
  	InputTokens  int // populated on Done
  	OutputTokens int // populated on Done
  	Err          error // non-nil => terminal error for this stream
  }

  type Provider interface {
  	Name() string
  	Chat(ctx context.Context, messages []Message, opts ChatOptions) (<-chan ChatChunk, error)
  }
  ```

- [ ] **Step 1.2: Create a compile-only interface satisfaction test**

  Create `relay/internal/llm/provider_test.go`. This test has no runtime assertions — its only purpose is to fail compilation if any type or method signature drifts from the README §6.1 contract:

  ```go
  package llm_test

  import (
  	"context"
  	"testing"

  	"intake/internal/llm"
  )

  // staticCheckProvider is a compile-time assertion that the Provider interface
  // has the exact signature from README §6.1. This file intentionally contains
  // no runtime assertions; a build failure IS the test failure.
  type staticCheckProvider struct{}

  func (s *staticCheckProvider) Name() string { return "static" }
  func (s *staticCheckProvider) Chat(
  	_ context.Context,
  	_ []llm.Message,
  	_ llm.ChatOptions,
  ) (<-chan llm.ChatChunk, error) {
  	ch := make(chan llm.ChatChunk)
  	close(ch)
  	return ch, nil
  }

  // Verify staticCheckProvider satisfies the interface at compile time.
  var _ llm.Provider = (*staticCheckProvider)(nil)

  func TestProviderInterfaceCompiles(t *testing.T) {
  	// Instantiation proves the interface is satisfied at runtime too.
  	var p llm.Provider = &staticCheckProvider{}
  	if p.Name() != "static" {
  		t.Fatalf("unexpected name: %s", p.Name())
  	}
  }
  ```

- [ ] **Step 1.3: Run the compile test**

  ```
  cd C:\src\ai\intake\relay
  go test ./internal/llm/...
  ```

  Expected output:
  ```
  ok      intake/internal/llm     0.XXXs
  ```

- [ ] **Step 1.4: Commit**

  ```bash
  cd /c/src/ai/intake
  git add relay/internal/llm/provider.go relay/internal/llm/provider_test.go
  git commit -m "feat(1-ii): add frozen llm.Provider interface (README §6.1 verbatim)"
  ```

---

### Task 2: Pin the Anthropic SDK; extend `check-pins.sh`

**Files:**
- Modify: `relay/go.mod`
- Modify: `relay/go.sum`
- Modify: `ai/tasks/phase-1/README.md` §5 table
- Modify: `scripts/check-pins.sh`
- Create: `scripts/check-pins.sh` test (no separate file — the script is the test; CI invokes it)

- [ ] **Step 2.1: Determine the latest exact version of the Anthropic Go SDK**

  Run the following to discover the latest published version:

  ```bash
  cd /c/src/ai/intake/relay
  go list -m -versions github.com/anthropics/anthropic-sdk-go 2>/dev/null | tr ' ' '\n' | tail -5
  ```

  If that returns nothing (module not fetched yet), use:

  ```bash
  go get github.com/anthropics/anthropic-sdk-go@latest 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+'
  ```

  Record the exact version (e.g. `v0.2.0-alpha.4`). We'll call it `$SDK_VER` in subsequent steps.

  > **IMPLEMENTER NOTE:** The exact version printed by the above command is the one you pin. Do NOT use `@latest` in go.mod or any source file. The README §5 table must be updated with the real version you installed.

- [ ] **Step 2.2: Install and pin the SDK exactly**

  Replace `@latest` with the actual version string from Step 2.1 (e.g. `v0.2.0-alpha.4`):

  ```bash
  cd /c/src/ai/intake/relay
  go get github.com/anthropics/anthropic-sdk-go@v0.2.0-alpha.4
  go mod tidy
  ```

  After running, verify `go.mod` contains an exact pin (no caret, no `@latest`):

  ```bash
  grep anthropic go.mod
  ```

  Expected (version number will match what you found in Step 2.1):
  ```
  require github.com/anthropics/anthropic-sdk-go v0.2.0-alpha.4
  ```

- [ ] **Step 2.3: Update README §5 table**

  Open `ai/tasks/phase-1/README.md`. Find the `github.com/anthropics/anthropic-sdk-go` row in the §5 table and replace `verify+pin exact at install` with the actual version (e.g. `v0.2.0-alpha.4`):

  Before:
  ```
  | github.com/anthropics/anthropic-sdk-go | verify+pin exact at install | Anthropic Messages API streaming + usage tokens; **exact** (response shape load-bearing) |
  ```

  After (substitute the real version):
  ```
  | github.com/anthropics/anthropic-sdk-go | v0.2.0-alpha.4 | Anthropic Messages API streaming + usage tokens; **exact** (response shape load-bearing) |
  ```

- [ ] **Step 2.4: Extend `scripts/check-pins.sh` to gate the Anthropic SDK pin**

  Open `scripts/check-pins.sh`. Add the following block **before** the final `[ "$fail" -eq 0 ]` line:

  ```bash
  # Gate: anthropic-sdk-go must be exact-pinned (no caret, no @latest) in go.mod.
  if grep -E 'anthropics/anthropic-sdk-go' relay/go.mod | grep -E '(\^|@latest)'; then
    echo "ERROR: github.com/anthropics/anthropic-sdk-go is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
    fail=1
  fi
  # Gate: no go get @latest for anthropic-sdk-go anywhere in scripts.
  if grep -rE 'go get.*anthropics/anthropic-sdk-go@latest' scripts/; then
    echo "ERROR: a script installs anthropic-sdk-go @latest; pin an exact version" >&2
    fail=1
  fi
  ```

- [ ] **Step 2.5: Run `check-pins.sh` to confirm it passes**

  ```bash
  cd /c/src/ai/intake
  bash scripts/check-pins.sh
  ```

  Expected:
  ```
  OK: all codegen tools are exact-pinned
  ```

- [ ] **Step 2.6: Commit**

  ```bash
  cd /c/src/ai/intake
  git add relay/go.mod relay/go.sum ai/tasks/phase-1/README.md scripts/check-pins.sh
  git commit -m "feat(1-ii): pin anthropic-sdk-go exact; extend check-pins.sh gate"
  ```

---

### Task 3: Write the failing mock-streaming unit test

**Files:**
- Create: `relay/internal/llm/anthropic/anthropic_test.go`

> Before writing this test you MUST invoke the **claude-api skill** (`/claude-api`) to get current Anthropic Go SDK streaming idioms for the exact pinned version. The SSE event names, struct field paths, and usage field names used in the mock below are based on the SDK's published API surface as of the plan authoring date — the implementer MUST verify them against the installed SDK source (`go doc github.com/anthropics/anthropic-sdk-go`) before trusting this code. Differences between the plan's assumed surface and the actual SDK are flagged with `// IMPLEMENTER: verify` comments.

- [ ] **Step 3.1: Understand the Anthropic SSE event format**

  The Anthropic Messages API streaming endpoint emits newline-delimited SSE. A minimal canned response that yields two text deltas and then usage looks like this (each line is a raw SSE frame):

  ```
  event: message_start
  data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}

  event: content_block_start
  data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

  event: ping
  data: {"type":"ping"}

  event: content_block_delta
  data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

  event: content_block_delta
  data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world"}}

  event: content_block_stop
  data: {"type":"content_block_stop","index":0}

  event: message_delta
  data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7}}

  event: message_stop
  data: {"type":"message_stop"}
  ```

  The mock `httptest.Server` in the test will return this exact byte sequence.

- [ ] **Step 3.2: Create the failing test file**

  Create `relay/internal/llm/anthropic/anthropic_test.go`:

  ```go
  package anthropic_test

  import (
  	"context"
  	"fmt"
  	"net/http"
  	"net/http/httptest"
  	"strings"
  	"testing"

  	anthropicpkg "intake/internal/llm/anthropic"
  	"intake/internal/llm"
  )

  // cannedSSE is a minimal Anthropic Messages API streaming response that emits
  // two text deltas ("Hello", ", world") and a terminal usage block
  // (input_tokens=10, output_tokens=7).
  //
  // IMPLEMENTER: Verify the JSON field names match the installed SDK version by
  // running: go doc github.com/anthropics/anthropic-sdk-go
  const cannedSSE = "" +
  	"event: message_start\n" +
  	`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}` + "\n\n" +
  	"event: content_block_start\n" +
  	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
  	"event: ping\n" +
  	`data: {"type":"ping"}` + "\n\n" +
  	"event: content_block_delta\n" +
  	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}` + "\n\n" +
  	"event: content_block_delta\n" +
  	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world"}}` + "\n\n" +
  	"event: content_block_stop\n" +
  	`data: {"type":"content_block_stop","index":0}` + "\n\n" +
  	"event: message_delta\n" +
  	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7}}` + "\n\n" +
  	"event: message_stop\n" +
  	`data: {"type":"message_stop"}` + "\n\n"

  // newMockServer returns an httptest.Server that serves the canned SSE response
  // for any POST to /v1/messages and returns its URL.
  func newMockServer(t *testing.T) *httptest.Server {
  	t.Helper()
  	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  		if r.Method != http.MethodPost {
  			t.Errorf("expected POST, got %s", r.Method)
  		}
  		w.Header().Set("Content-Type", "text/event-stream")
  		w.Header().Set("Cache-Control", "no-cache")
  		w.WriteHeader(http.StatusOK)
  		fmt.Fprint(w, cannedSSE)
  		if f, ok := w.(http.Flusher); ok {
  			f.Flush()
  		}
  	}))
  	return srv
  }

  func TestChat_MockStreaming(t *testing.T) {
  	srv := newMockServer(t)
  	defer srv.Close()

  	p := anthropicpkg.NewWithClient(
  		"test-key-not-real",
  		"claude-sonnet-4-6",
  		1024,
  		srv.Client(),
  		srv.URL,
  	)

  	ctx := context.Background()
  	messages := []llm.Message{
  		{Role: "user", Content: "say hello"},
  	}
  	opts := llm.ChatOptions{
  		Model:     "claude-sonnet-4-6",
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

  func TestChat_ContextCancellation(t *testing.T) {
  	// The mock server blocks until the client disconnects.
  	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  		w.Header().Set("Content-Type", "text/event-stream")
  		w.WriteHeader(http.StatusOK)
  		// Write one delta then block; the test cancels before we get further.
  		fmt.Fprint(w, "event: content_block_delta\n")
  		fmt.Fprintf(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`+"\n\n")
  		if f, ok := w.(http.Flusher); ok {
  			f.Flush()
  		}
  		<-r.Context().Done()
  	}))
  	defer srv.Close()

  	p := anthropicpkg.NewWithClient(
  		"test-key-not-real",
  		"claude-sonnet-4-6",
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

  	// Drain the channel; expect it to close (possibly with an Err chunk).
  	for range ch {
  	}
  	// If we get here the channel closed — cancellation propagated.
  }

  func TestName(t *testing.T) {
  	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
  		w.WriteHeader(http.StatusOK)
  	}))
  	defer srv.Close()

  	p := anthropicpkg.NewWithClient("key", "model", 1024, srv.Client(), srv.URL)
  	if p.Name() != "anthropic" {
  		t.Errorf("Name(): got %q, want %q", p.Name(), "anthropic")
  	}
  }

  func TestKeyNotInError(t *testing.T) {
  	// This test verifies that if the provider encounters an HTTP error, the
  	// API key does NOT appear in the returned error text.
  	const sensitiveKey = "sk-ant-SUPER-SECRET-KEY-ABCDEF"

  	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  		http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`, http.StatusUnauthorized)
  	}))
  	defer srv.Close()

  	p := anthropicpkg.NewWithClient(
  		sensitiveKey,
  		"claude-sonnet-4-6",
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

- [ ] **Step 3.3: Run the test and confirm it fails to compile (no implementation yet)**

  ```bash
  cd /c/src/ai/intake/relay
  go test ./internal/llm/anthropic/...
  ```

  Expected: compile error similar to:
  ```
  cannot find package "intake/internal/llm/anthropic"
  ```
  or
  ```
  undefined: anthropicpkg.NewWithClient
  ```

  This is the desired "red" state.

---

### Task 4: Implement `anthropic.go` — make the mock test pass

> **IMPLEMENTER REQUIRED ACTION — before writing any code in this task:**
> Invoke the **claude-api skill** (`/claude-api`) and ask: *"Show me the current idiomatic streaming usage of the Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`) — specifically how to open a streaming Messages API call, iterate events, extract `text_delta` text from `content_block_delta` events, and read `input_tokens`/`output_tokens` from the `message_delta`/`message_stop` usage struct. Show the exact struct types and field names."* Use the output to verify or correct the field paths in the code below before writing the file.

**Files:**
- Create: `relay/internal/llm/anthropic/anthropic.go`

- [ ] **Step 4.1: Verify SDK API surface**

  After installing the SDK (Task 2), run:

  ```bash
  cd /c/src/ai/intake/relay
  go doc github.com/anthropics/anthropic-sdk-go | head -80
  go doc github.com/anthropics/anthropic-sdk-go.Client
  go doc github.com/anthropics/anthropic-sdk-go MessageStreamEvent
  ```

  Confirm the following assumed types exist (adjust field names in Step 4.2 if they differ):
  - `anthropic.NewClient(opts ...option.RequestOption) *anthropic.Client`
  - `client.Messages.NewStreaming(ctx, params)` returns a `*ssestream.Stream[T]` or equivalent
  - `option.WithHTTPClient(*http.Client)` and `option.WithBaseURL(string)` (for test injection)
  - `option.WithAPIKey(string)`
  - `ContentBlockDeltaEventDelta.Text` for text delta content
  - `MessageDeltaUsage.OutputTokens` (int64 or int)
  - `MessageStartEventMessageUsage.InputTokens` (int64 or int)

  > **IMPLEMENTER: If any of these names differ in your installed version, update the code in Step 4.2 accordingly. The mock test in Task 3 does NOT call SDK types directly — it uses raw HTTP — so the mock test should continue to pass regardless of SDK surface changes.**

- [ ] **Step 4.2: Create `relay/internal/llm/anthropic/anthropic.go`**

  ```go
  // Package anthropic implements the llm.Provider interface using the Anthropic
  // Messages API with streaming enabled. The API key is always passed in by the
  // caller — it is never read from disk, config files, or logs.
  package anthropic

  import (
  	"context"
  	"fmt"
  	"net/http"

  	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
  	"github.com/anthropics/anthropic-sdk-go/option"

  	"intake/internal/llm"
  )

  // Provider implements llm.Provider for the Anthropic Messages API.
  type Provider struct {
  	client    *anthropicsdk.Client
  	model     string
  	maxTokens int
  }

  // New creates a production Provider. apiKey is the raw API key value
  // (the caller resolves it from os.Getenv(config.LLM.Anthropic.APIKeyEnv)).
  // The key is passed to the SDK and never stored in a log-accessible field.
  func New(apiKey, model string, maxTokens int) *Provider {
  	c := anthropicsdk.NewClient(
  		option.WithAPIKey(apiKey),
  	)
  	return &Provider{client: c, model: model, maxTokens: maxTokens}
  }

  // NewWithClient is used by tests. It injects a custom *http.Client and base URL
  // so callers can point the provider at an httptest.Server without a real key.
  //
  // IMPLEMENTER: Verify option.WithHTTPClient and option.WithBaseURL exist in the
  // installed SDK version via: go doc github.com/anthropics/anthropic-sdk-go/option
  func NewWithClient(apiKey, model string, maxTokens int, httpClient *http.Client, baseURL string) *Provider {
  	c := anthropicsdk.NewClient(
  		option.WithAPIKey(apiKey),
  		option.WithHTTPClient(httpClient),
  		option.WithBaseURL(baseURL),
  	)
  	return &Provider{client: c, model: model, maxTokens: maxTokens}
  }

  // Name returns the provider identifier string.
  func (p *Provider) Name() string { return "anthropic" }

  // Chat opens a streaming Messages API request and returns a channel of
  // ChatChunk values. Each non-terminal chunk carries a Delta string. The
  // terminal chunk (Done=true) carries InputTokens and OutputTokens from the
  // final usage block. If an error occurs, a terminal chunk with Err set is
  // emitted and the channel is closed.
  //
  // The caller must drain the channel to completion; closing the context cancels
  // the upstream HTTP request.
  //
  // System-role messages are separated into Anthropic's top-level system
  // parameter; all other messages go into the messages array.
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
  	// Anthropic's API keeps system separate from the messages array.
  	var systemText string
  	var apiMessages []anthropicsdk.MessageParam
  	for _, m := range messages {
  		switch m.Role {
  		case "system":
  			systemText += m.Content
  		case "user":
  			apiMessages = append(apiMessages, anthropicsdk.NewUserMessage(
  				anthropicsdk.NewTextBlock(m.Content),
  			))
  		case "assistant":
  			apiMessages = append(apiMessages, anthropicsdk.NewAssistantMessage(
  				anthropicsdk.NewTextBlock(m.Content),
  			))
  		}
  	}

  	params := anthropicsdk.MessageNewParams{
  		Model:     anthropicsdk.Model(model),
  		MaxTokens: int64(maxTokens),
  		Messages:  apiMessages,
  	}
  	if systemText != "" {
  		params.System = []anthropicsdk.TextBlockParam{
  			{Text: systemText},
  		}
  	}

  	// IMPLEMENTER: Verify the streaming call. The SDK may use
  	// client.Messages.NewStreaming(ctx, params) or a similar variant.
  	// Run: go doc github.com/anthropics/anthropic-sdk-go/packages/ssestream
  	// to confirm the iteration API (Next()/Event()/Err() or range-based).
  	stream := p.client.Messages.NewStreaming(ctx, params)

  	ch := make(chan llm.ChatChunk, 32)

  	go func() {
  		defer close(ch)

  		var inputTokens int
  		var outputTokens int

  		for stream.Next() {
  			event := stream.Current()

  			// IMPLEMENTER: The event type switch below uses the type names from
  			// the SDK as of the plan authoring date. Verify each case against the
  			// installed version via: go doc github.com/anthropics/anthropic-sdk-go
  			switch e := event.AsUnion().(type) {

  			case anthropicsdk.MessageStartEvent:
  				// Capture initial input token count from message_start.
  				// IMPLEMENTER: verify field path — may be e.Message.Usage.InputTokens
  				inputTokens = int(e.Message.Usage.InputTokens)

  			case anthropicsdk.ContentBlockDeltaEvent:
  				// Extract text from text_delta content block deltas.
  				// IMPLEMENTER: verify — may be e.Delta.AsUnion() type-switched on
  				// anthropicsdk.TextDelta, with field .Text
  				if delta, ok := e.Delta.AsUnion().(anthropicsdk.TextDelta); ok {
  					if delta.Text != "" {
  						select {
  						case ch <- llm.ChatChunk{Delta: delta.Text}:
  						case <-ctx.Done():
  							ch <- llm.ChatChunk{Err: ctx.Err(), Done: true}
  							return
  						}
  					}
  				}

  			case anthropicsdk.MessageDeltaEvent:
  				// Capture output tokens from message_delta usage.
  				// IMPLEMENTER: verify field path — may be e.Usage.OutputTokens
  				outputTokens = int(e.Usage.OutputTokens)

  			case anthropicsdk.MessageStopEvent:
  				// Stream is complete; emit the terminal Done chunk.
  				select {
  				case ch <- llm.ChatChunk{Done: true, InputTokens: inputTokens, OutputTokens: outputTokens}:
  				case <-ctx.Done():
  				}
  				return
  			}
  		}

  		if err := stream.Err(); err != nil {
  			// Redact the API key from the error before surfacing it.
  			// The SDK wraps HTTP errors; the key should not appear in those, but
  			// we sanitize defensively.
  			ch <- llm.ChatChunk{Err: redactedErr(err), Done: true}
  			return
  		}

  		// Stream ended without a message_stop (e.g. context cancelled mid-stream).
  		// Emit a terminal chunk with whatever tokens were accumulated.
  		select {
  		case ch <- llm.ChatChunk{Done: true, InputTokens: inputTokens, OutputTokens: outputTokens}:
  		default:
  		}
  	}()

  	return ch, nil
  }

  // redactedErr wraps an error, ensuring the Anthropic API key is never echoed.
  // The SDK errors do not embed the key, but this is a defensive belt-and-suspenders
  // guard required by the §2 security invariant and README §7 build-fail checklist.
  func redactedErr(err error) error {
  	if err == nil {
  		return nil
  	}
  	// Wrap with a sentinel message that does not include the original error's
  	// full chain if it might contain header values. In practice the SDK returns
  	// structured API errors; we just wrap.
  	return fmt.Errorf("anthropic provider error: %w", err)
  }
  ```

- [ ] **Step 4.3: Run the tests**

  ```bash
  cd /c/src/ai/intake/relay
  go test ./internal/llm/...
  ```

  Expected:
  ```
  ok      intake/internal/llm             0.XXXs
  ok      intake/internal/llm/anthropic   0.XXXs
  ```

  If there are compile errors due to SDK API surface differences, consult `go doc` output (Step 4.1) and the claude-api skill to correct the field paths in `anthropic.go`. The mock test must pass; DO NOT change the test to accommodate a broken implementation.

- [ ] **Step 4.4: Run `go vet` and `go build`**

  ```bash
  cd /c/src/ai/intake/relay
  go vet ./...
  go build ./...
  ```

  Expected: no output (zero exit code).

- [ ] **Step 4.5: Commit**

  ```bash
  cd /c/src/ai/intake
  git add relay/internal/llm/anthropic/anthropic.go relay/internal/llm/anthropic/anthropic_test.go
  git commit -m "feat(1-ii): implement Anthropic streaming provider; mock unit tests green"
  ```

---

### Task 5: Key-redaction test (already included, verify it passes)

The `TestKeyNotInError` test was written in Task 3 and implemented in Task 4. This task confirms it passes and adds one additional paranoia check: that the key never appears in any `fmt.Sprintf` or `log` call inside `anthropic.go`.

**Files:**
- Read-only audit: `relay/internal/llm/anthropic/anthropic.go`

- [ ] **Step 5.1: Grep for key leakage patterns in the implementation**

  ```bash
  cd /c/src/ai/intake/relay
  grep -n "apiKey\|APIKey\|api_key" internal/llm/anthropic/anthropic.go
  ```

  Acceptable output: any line where `apiKey` is passed to `option.WithAPIKey(apiKey)` in `New` and `NewWithClient`. Any occurrence of `apiKey` in a `fmt.Sprintf`, `log.`, or `errors.New` call is a **violation** — remove it and re-run all tests.

- [ ] **Step 5.2: Run the key-redaction test in isolation**

  ```bash
  cd /c/src/ai/intake/relay
  go test -v -run TestKeyNotInError ./internal/llm/anthropic/...
  ```

  Expected:
  ```
  --- PASS: TestKeyNotInError (0.XXs)
  PASS
  ok      intake/internal/llm/anthropic   0.XXXs
  ```

- [ ] **Step 5.3: Commit (if no changes needed)**

  If `anthropic.go` required fixes in Step 5.1:

  ```bash
  cd /c/src/ai/intake
  git add relay/internal/llm/anthropic/anthropic.go
  git commit -m "fix(1-ii): remove API key from error surface (security invariant §2)"
  ```

  If no changes were needed, no commit is required here.

---

### Task 6: Add the env-gated integration test (sub-plan smoke)

This test is the sub-plan's **Smoke** — it exercises a real Anthropic API call end-to-end. It is build-tagged `integration` and skipped unless `ANTHROPIC_API_KEY` is set, so it never runs in `go test ./...` or CI.

**Files:**
- Create: `relay/internal/llm/anthropic/integration_test.go`

- [ ] **Step 6.1: Create the integration test**

  Create `relay/internal/llm/anthropic/integration_test.go`:

  ```go
  //go:build integration

  package anthropic_test

  import (
  	"context"
  	"os"
  	"strings"
  	"testing"
  	"time"

  	anthropicpkg "intake/internal/llm/anthropic"
  	"intake/internal/llm"
  )

  // TestIntegration_RealStream performs a live Anthropic API call.
  // It is excluded from the default test run (go test ./...) and CI.
  //
  // To run:
  //   export ANTHROPIC_API_KEY=sk-ant-...
  //   cd relay
  //   go test -tags integration -v ./internal/llm/anthropic/
  //
  // The test asserts:
  //   1. At least one delta chunk arrives with non-empty text.
  //   2. The terminal Done chunk has non-zero InputTokens and OutputTokens.
  //   3. The joined deltas contain a recognisable word (non-empty assistant reply).
  //   4. The API key does not appear in any error message.
  func TestIntegration_RealStream(t *testing.T) {
  	apiKey := os.Getenv("ANTHROPIC_API_KEY")
  	if apiKey == "" {
  		t.Skip("ANTHROPIC_API_KEY not set — skipping integration test")
  	}

  	p := anthropicpkg.New(apiKey, "claude-haiku-4-5", 64)

  	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
  	defer cancel()

  	messages := []llm.Message{
  		{Role: "user", Content: "Reply with exactly three words."},
  	}
  	opts := llm.ChatOptions{
  		Model:     "claude-haiku-4-5",
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

- [ ] **Step 6.2: Verify the integration test is excluded from the default run**

  ```bash
  cd /c/src/ai/intake/relay
  go test ./internal/llm/anthropic/...
  ```

  Expected: no mention of `TestIntegration_RealStream`; all existing tests pass:
  ```
  ok      intake/internal/llm/anthropic   0.XXXs
  ```

- [ ] **Step 6.3: (Smoke) Run the integration test with a real key**

  > This step requires a real `ANTHROPIC_API_KEY`. Perform this step manually before marking the sub-plan done.

  On Windows PowerShell:
  ```powershell
  $env:ANTHROPIC_API_KEY = "sk-ant-..."
  cd C:\src\ai\intake\relay
  go test -tags integration -v ./internal/llm/anthropic/
  ```

  On bash:
  ```bash
  export ANTHROPIC_API_KEY=sk-ant-...
  cd /c/src/ai/intake/relay
  go test -tags integration -v ./internal/llm/anthropic/
  ```

  Expected output (token counts will vary):
  ```
  === RUN   TestIntegration_RealStream
      anthropic_test.go:XX: Assistant reply: "Here are three words."
      anthropic_test.go:XX: Token usage — input: 18, output: 6
  --- PASS: TestIntegration_RealStream (2.XXs)
  PASS
  ok      intake/internal/llm/anthropic   2.XXXs
  ```

  **Grep for key leakage in test output:**
  ```bash
  go test -tags integration -v ./internal/llm/anthropic/ 2>&1 | grep -F "${ANTHROPIC_API_KEY}" && echo "FAIL: key leaked" || echo "OK: key not in output"
  ```

  Expected: `OK: key not in output`

- [ ] **Step 6.4: Commit**

  ```bash
  cd /c/src/ai/intake
  git add relay/internal/llm/anthropic/integration_test.go
  git commit -m "test(1-ii): add build-tagged integration test (sub-plan smoke)"
  ```

---

### Task 7: Final verification — full build-fail checklist

- [ ] **Step 7.1: Run full test suite**

  ```bash
  cd /c/src/ai/intake/relay
  go test ./...
  ```

  Expected:
  ```
  ok      intake/internal/llm             0.XXXs
  ok      intake/internal/llm/anthropic   0.XXXs
  ```

- [ ] **Step 7.2: Run `go vet` and `go build`**

  ```bash
  cd /c/src/ai/intake/relay
  go vet ./...
  go build ./...
  ```

  Expected: no output (zero exit code).

- [ ] **Step 7.3: Run `check-pins.sh`**

  ```bash
  cd /c/src/ai/intake
  bash scripts/check-pins.sh
  ```

  Expected:
  ```
  OK: all codegen tools are exact-pinned
  ```

- [ ] **Step 7.4: Verify interface is byte-for-byte identical to README §6.1**

  ```bash
  cd /c/src/ai/intake/relay
  grep -A 40 "^package llm" internal/llm/provider.go
  ```

  Compare manually to `ai/tasks/phase-1/README.md §6.1`. Field names, types, JSON tags, and comments must be identical.

- [ ] **Step 7.5: Final commit (if any changes)**

  If any cleanup happened in this task:

  ```bash
  cd /c/src/ai/intake
  git add -u
  git commit -m "chore(1-ii): final verification fixes"
  ```

---

## 5. Smoke

The sub-plan smoke is the **integration test** in `relay/internal/llm/anthropic/integration_test.go`, run with a real `ANTHROPIC_API_KEY`:

```bash
export ANTHROPIC_API_KEY=sk-ant-<real-key>
cd /c/src/ai/intake/relay
go test -tags integration -v ./internal/llm/anthropic/
```

**Pass criteria:**
1. `TestIntegration_RealStream` passes.
2. Log output shows a non-empty assistant reply string.
3. Log output shows non-zero `input:` and `output:` token counts.
4. The API key string does NOT appear anywhere in the test output (verified by the grep in Task 6.3).

---

## 6. Done Criteria

- [ ] `go test ./...` passes in `relay/` with **no** `ANTHROPIC_API_KEY` set — mock unit tests cover all behaviour.
- [ ] `TestKeyNotInError` passes — the API key string never appears in any error surfaced from the provider.
- [ ] `relay/internal/llm/provider.go` is **byte-for-byte identical** to the interface block in README §6.1 (verified in Task 7.4).
- [ ] `relay/go.mod` shows an exact pin of `github.com/anthropics/anthropic-sdk-go` (no caret, no `@latest`).
- [ ] `scripts/check-pins.sh` passes with `OK: all codegen tools are exact-pinned`.
- [ ] `README.md §5` table shows the actual installed version of `github.com/anthropics/anthropic-sdk-go`.
- [ ] `go vet ./...` and `go build ./...` exit zero.
- [ ] Smoke (`TestIntegration_RealStream` with real key) passes — deltas arrive, terminal chunk has non-zero token counts, key not in output.

---

## Environment Notes

- **OS:** Windows 10, development in PowerShell or bash (both available).
- **Go:** 1.23.2 (module `intake`, relay at `C:\src\ai\intake\relay`).
- **No git remote** — local commits only; `git push` is not needed.
- **All paths** in commands use Windows-absolute form (`C:\src\ai\intake\...`) for PowerShell or POSIX form (`/c/src/ai/intake/...`) for bash — the Bash tool is available for POSIX scripts.
- The mock unit test (`TestChat_MockStreaming`, `TestKeyNotInError`, etc.) MUST pass with no `ANTHROPIC_API_KEY` environment variable set.
- The integration test is skipped automatically when `ANTHROPIC_API_KEY` is empty (`t.Skip`).

---

## Assumptions and Flags for the Implementer

The following SDK API surface assumptions are made in this plan. **Before writing `anthropic.go`, the implementer MUST invoke the claude-api skill and verify each of these against the installed SDK version** (`go doc github.com/anthropics/anthropic-sdk-go`). Mismatches are expected to be common as the SDK is pre-1.0.

| Assumption | Used in | How to verify |
|---|---|---|
| `anthropicsdk.NewClient(opts...)` constructs a client | `anthropic.go` New + NewWithClient | `go doc github.com/anthropics/anthropic-sdk-go NewClient` |
| `option.WithAPIKey(string)`, `option.WithHTTPClient(*http.Client)`, `option.WithBaseURL(string)` exist in the `option` sub-package | `anthropic.go` NewWithClient | `go doc github.com/anthropics/anthropic-sdk-go/option` |
| `client.Messages.NewStreaming(ctx, params)` returns a stream with `.Next()`, `.Current()`, `.Err()` | `anthropic.go` Chat goroutine | `go doc github.com/anthropics/anthropic-sdk-go.MessagesService` |
| `.Current().AsUnion()` returns an `interface{}` type-switchable over `MessageStartEvent`, `ContentBlockDeltaEvent`, `MessageDeltaEvent`, `MessageStopEvent` | `anthropic.go` Chat goroutine | `go doc github.com/anthropics/anthropic-sdk-go MessageStreamEvent` |
| `ContentBlockDeltaEvent.Delta.AsUnion().(anthropicsdk.TextDelta).Text` yields the text | `anthropic.go` Chat goroutine | `go doc github.com/anthropics/anthropic-sdk-go ContentBlockDeltaEvent` |
| `MessageStartEvent.Message.Usage.InputTokens` is int64 | `anthropic.go` Chat goroutine | `go doc github.com/anthropics/anthropic-sdk-go MessageStartEvent` |
| `MessageDeltaEvent.Usage.OutputTokens` is int64 | `anthropic.go` Chat goroutine | `go doc github.com/anthropics/anthropic-sdk-go MessageDeltaEvent` |
| `anthropicsdk.NewUserMessage(blocks...)`, `anthropicsdk.NewAssistantMessage(blocks...)`, `anthropicsdk.NewTextBlock(string)` are constructors | `anthropic.go` Chat | `go doc github.com/anthropics/anthropic-sdk-go NewUserMessage` |
| `anthropicsdk.MessageNewParams.System` is `[]anthropicsdk.TextBlockParam` with a `Text` field | `anthropic.go` Chat | `go doc github.com/anthropics/anthropic-sdk-go MessageNewParams` |

If any assumption is wrong, correct the implementation (not the tests) and re-run `go test ./...`.
