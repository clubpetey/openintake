# 2-ii — OpenAI Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the OpenAI Chat Completions provider behind the frozen `llm.Provider` interface, wire it into the `providers` factory, add a `check-pins` gate for `openai-go`, and cover it with credit-free mock-HTTP tests.

**Architecture:** A new `relay/internal/llm/openai` package mirrors `anthropic.go` exactly: `New`/`NewWithClient` constructors, one `defer close(ch)`, best-effort channel sends, ctx-cancellation, and key-redaction. Streaming uses Chat Completions with `stream_options:{include_usage:true}` so the final SSE event carries usage. The factory (`providers/providers.go`, created in 2-i) replaces its `"openai"` not-implemented stub with a real call to `openai.New`.

**Tech Stack:** Go 1.23.2, `github.com/openai/openai-go` (exact-pinned), `net/http/httptest` for credit-free unit tests.

---

## SDK surface assumptions (flag for implementer verification)

The following assumptions about the `openai-go` SDK's API surface were made while authoring this plan. **The implementer MUST verify each one with `go doc github.com/openai/openai-go.<Symbol>` against the pinned version before writing code, and adapt the code if the SDK differs.**

| # | Assumption | Symbol to verify | Risk |
|---|---|---|---|
| A1 | `openai.NewClient(option.WithAPIKey(key))` constructs the client; `option.WithHTTPClient(hc)` + `option.WithBaseURL(u)` inject test overrides | `openai.NewClient`, `option.WithAPIKey`, `option.WithHTTPClient`, `option.WithBaseURL` | Medium — option pattern may differ from anthropic-sdk-go |
| A2 | `client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{...})` returns a stream iterator | `openai.ChatService`, `openai.ChatCompletionService` | High — method path and param struct name likely differ; `go doc` required |
| A3 | The stream iterator exposes `.Next() bool`, `.Current() openai.ChatCompletionChunk`, `.Err() error`, `.Close()` — same iterator pattern as anthropic-sdk-go | The stream type returned by `NewStreaming` | High — iterator shape must be confirmed |
| A4 | `openai.ChatCompletionChunk.Choices[0].Delta.Content` carries the incremental text string | `openai.ChatCompletionChunk` | Medium |
| A5 | The final stream chunk (after all delta chunks) carries `Usage.PromptTokens` and `Usage.CompletionTokens` when `stream_options:{include_usage:true}` is set | `openai.ChatCompletionChunk.Usage`, `openai.ChatCompletionStreamOptions` | High — usage may arrive via a separate `data: [DONE]` chunk or a special chunk type; confirm exact field path |
| A6 | Message params are constructed via `openai.ChatCompletionMessageParamUnion` or similar union type; user/assistant/system roles use distinct constructor functions (e.g. `openai.UserMessage(content)`, `openai.AssistantMessage(content)`, `openai.SystemMessage(content)`) | `openai.ChatCompletionMessageParamUnion` | High — the anthropic-sdk-go constructors (`NewUserMessage`, etc.) are SDK-specific; openai-go uses a different param model |
| A7 | `MaxTokens` maps to `openai.ChatCompletionNewParams.MaxTokens` (or `MaxCompletionTokens` — names changed in the API) | `openai.ChatCompletionNewParams` | Medium — field may be `MaxCompletionTokens` in newer API versions |
| A8 | `stream_options` is set via `openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}` or equivalent | `openai.ChatCompletionStreamOptionsParam` | Medium |

**Implementer protocol for wrong assumptions:** when `go doc` shows a different type/method, use the SDK's actual shape and annotate the deviation in a code comment `// SDK: <what plan assumed> → <actual>`. Do NOT blindly copy the plan code if types don't match; the plan's code is a guide, not a guarantee.

---

## Pre-condition

Sub-plan 2-i is complete. That means:
- `relay/internal/config/config.go` has `OpenAIConfig`, `GeminiConfig`, `OllamaConfig` structs on `LLMConfig` with defaults applied in `applyDefaults`.
- `relay/internal/llm/providers/providers.go` exists with `package providers`, `func New(cfg config.LLMConfig) (llm.Provider, error)`, an `"anthropic"` case that works, and an `"openai"` stub case that returns `fmt.Errorf("openai provider: not yet implemented")` (or similar).
- `relay/cmd/relay/main.go` calls `providers.New(cfg.LLM)` instead of `anthropic.New(...)` directly.
- `go test ./...` passes clean.
- `bash scripts/check-pins.sh` passes.

If 2-i is not complete, stop here and complete 2-i first.

---

## 1. Goal

Deliver a fully tested, credit-free OpenAI provider that:
1. Satisfies `llm.Provider` exactly as the frozen interface specifies.
2. Streams Chat Completions with per-delta `ChatChunk{Delta}` and a terminal `ChatChunk{Done, InputTokens, OutputTokens}`.
3. Passes all unit tests in `go test -race ./internal/llm/openai/ ./internal/llm/providers/`.
4. Never logs or embeds the API key.
5. Is wired into the factory so `provider: openai` in `config.yaml` constructs a real provider.
6. Has an `integration_test.go` (build-tagged, not run) for later live verification.

---

## 2. Design references

- Phase 2 README §8.1 — behavioral contract (verbatim, authoritative)
- Phase 2 README §8.2 — constructor pattern
- Phase 2 README §8.4 — factory case to wire
- Phase 2 README §5 — pin table to update
- Phase 2 README §6 — build-fail checklist
- Phase 2 README §7 — final smoke (deferred, credit-guarded)
- `docs/specs/2026-05-27-phase-2-provider-breadth-design.md` §4.1 — OpenAI specifics
- Reference impl: `relay/internal/llm/anthropic/anthropic.go` + `anthropic_test.go`
- Frozen interface: `relay/internal/llm/provider.go`
- Factory: `relay/internal/llm/providers/providers.go` (created in 2-i)

---

## 3. Files touched

| File | Action | Why |
|---|---|---|
| `relay/go.mod` | Modify | Add `github.com/openai/openai-go` at exact pinned version |
| `relay/go.sum` | Modify (auto) | Updated by `go get`; committed as-is |
| `relay/internal/llm/openai/openai.go` | **Create** | The OpenAI provider implementation |
| `relay/internal/llm/openai/openai_test.go` | **Create** | Unit tests: mock streaming, ctx-cancel, key-redaction, Name |
| `relay/internal/llm/openai/integration_test.go` | **Create** | Integration test (build-tagged, DO NOT RUN during impl) |
| `relay/internal/llm/providers/providers.go` | Modify | Replace `"openai"` stub with real `openai.New(...)` call |
| `relay/internal/llm/providers/providers_test.go` | Modify | Update `"openai"` factory test case to expect `Name()=="openai"` |
| `scripts/check-pins.sh` | Modify | Add `openai-go` caret/`@latest` gate |
| `ai/tasks/phase-2/README.md` | Modify | Update §5 pin table with exact version |

---

## 4. Tasks

---

### Task 1: Add `github.com/openai/openai-go` dependency (exact pin)

**Files:**
- Modify: `relay/go.mod` (via `go get`)
- Modify: `relay/go.sum` (auto)
- Modify: `scripts/check-pins.sh`
- Modify: `ai/tasks/phase-2/README.md`

- [ ] **Step 1.1: Install the dependency and capture the exact version**

Run from `relay/`:
```
cd C:\src\ai\intake\relay
go get github.com/openai/openai-go
```

Expected output (version will differ — this is illustrative):
```
go: added github.com/openai/openai-go v0.1.0-alpha.62
```

Capture the exact version string (e.g. `v0.1.0-alpha.62`). It will appear in `go.mod`. **Use whatever version `go get` resolves — do not guess the version from this document.**

- [ ] **Step 1.2: Verify the exact version is in `go.mod` with no caret**

Open `relay/go.mod` and confirm a line like:
```
github.com/openai/openai-go v0.1.0-alpha.62
```
The version MUST have no leading `^`. If it does, the check-pins gate will fail — but `go get` without `@latest` should not add a caret.

- [ ] **Step 1.3: Confirm no indirect openai-go dep is caret-pinned**

Run:
```
cd C:\src\ai\intake\relay
go mod tidy
```

Expected: exits 0 with no errors.

- [ ] **Step 1.4: Verify the SDK surface with `go doc` before writing any code**

This is mandatory — the plan's code is written from assumptions flagged above. Run each of these and read the output:

```
cd C:\src\ai\intake\relay
go doc github.com/openai/openai-go
go doc github.com/openai/openai-go.Client
go doc github.com/openai/openai-go/option
go doc github.com/openai/openai-go.ChatCompletionNewParams
go doc github.com/openai/openai-go.ChatCompletionChunk
go doc github.com/openai/openai-go.ChatCompletionMessageParamUnion
```

Look for:
- How to call `client.Chat.Completions.NewStreaming(ctx, params)` (or whatever the actual path is)
- The iterator type returned and its `.Next()`, `.Current()`, `.Err()`, `.Close()` methods
- Where usage lives in the final chunk (`Usage.PromptTokens` / `Usage.CompletionTokens`)
- How to set `stream_options` / `include_usage`
- The `MaxTokens` / `MaxCompletionTokens` field name
- User/assistant/system message param constructors

**Note deviations from the plan assumptions in a comment block at the top of `openai.go`.** The plan code in Task 2 uses the assumed names — adapt them to the actual SDK.

- [ ] **Step 1.5: Extend `scripts/check-pins.sh` with the openai-go gate**

Add after the existing `anthropics/anthropic-sdk-go` gate (around line 21):

```bash
# Gate: openai-go must be exact-pinned (no caret, no @latest) in go.mod.
if grep -E 'openai/openai-go' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/openai/openai-go is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
```

- [ ] **Step 1.6: Verify the check-pins gate passes**

Run from `C:\src\ai\intake`:
```
bash scripts/check-pins.sh && echo PINS_OK
```

Expected:
```
OK: Go module pins verified (go.sum enforces exact versions for santhosh-tekuri/jsonschema/v6 and google/uuid)
OK: all codegen tools are exact-pinned
PINS_OK
```

- [ ] **Step 1.7: Update the Phase 2 README §5 pin table**

In `ai/tasks/phase-2/README.md`, replace the `openai-go` row in the pin table:
```
| github.com/openai/openai-go | v0.1.0-alpha.62 | OpenAI Chat Completions streaming + usage; **exact** |
```
(substitute the actual version from Step 1.1)

- [ ] **Step 1.8: Commit the dependency addition**

```
cd C:\src\ai\intake\relay
go build ./...
```
Expected: `BUILD_OK` (exits 0, no output).

```
cd C:\src\ai\intake
git add relay/go.mod relay/go.sum scripts/check-pins.sh ai/tasks/phase-2/README.md
git commit -m "feat(2-ii): add github.com/openai/openai-go exact-pinned dep + check-pins gate"
```

---

### Task 2: Create `relay/internal/llm/openai/openai.go`

**Files:**
- Create: `relay/internal/llm/openai/openai.go`

**Read before writing:** The task 1.4 `go doc` output is authoritative. The code below uses the plan's assumed symbols — adapt every symbol that differs from what `go doc` showed. Deviations go in a comment block at the top of the file.

- [ ] **Step 2.1: Write the failing compile check (just the interface assertion)**

Create `relay/internal/llm/openai/openai.go` with only the stub that will fail to compile until the type is complete:

```go
// Package openai implements the llm.Provider interface using the OpenAI
// Chat Completions API with streaming enabled. The API key is always passed
// in by the caller — it is never read from disk, config files, or logs.
//
// SDK surface notes (confirm against pinned version via `go doc`):
// - Assumed: client.Chat.Completions.NewStreaming(ctx, params) → iterator
// - Assumed: iterator.Next() bool, iterator.Current() ChatCompletionChunk, iterator.Err() error, iterator.Close()
// - Assumed: ChatCompletionChunk.Choices[0].Delta.Content → string delta
// - Assumed: final chunk with FinishReason=="" (usage-only) carries Usage.PromptTokens/CompletionTokens
// - Assumed: stream_options set via ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}
// - Assumed: message params via openai.UserMessage/AssistantMessage/SystemMessage helpers
// - Assumed: MaxTokens field (may be MaxCompletionTokens in newer API — verify)
// DEVIATION LOG: <implementer adds deviations found in go doc here>
package openai

import (
	"context"
	"fmt"
	"net/http"

	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"intake/internal/llm"
)

// Provider implements llm.Provider for the OpenAI Chat Completions API.
type Provider struct {
	client    openaisdk.Client
	model     string
	maxTokens int
}

// Compile-time assertion: *Provider must satisfy llm.Provider.
var _ llm.Provider = (*Provider)(nil)

// New creates a production Provider. apiKey is the raw API key value
// (the caller resolves it from os.Getenv(config.LLM.OpenAI.APIKeyEnv)).
// The key is passed to the SDK and never stored in a log-accessible field.
func New(apiKey, model string, maxTokens int) *Provider {
	c := openaisdk.NewClient(
		option.WithAPIKey(apiKey),
	)
	return &Provider{client: c, model: model, maxTokens: maxTokens}
}

// NewWithClient is used by tests. It injects a custom *http.Client and base URL
// so callers can point the provider at an httptest.Server without a real key.
func NewWithClient(apiKey, model string, maxTokens int, httpClient *http.Client, baseURL string) *Provider {
	c := openaisdk.NewClient(
		option.WithAPIKey(apiKey),
		option.WithHTTPClient(httpClient),
		option.WithBaseURL(baseURL),
	)
	return &Provider{client: c, model: model, maxTokens: maxTokens}
}

// Name returns the provider identifier string.
func (p *Provider) Name() string { return "openai" }

// Chat opens a streaming Chat Completions request and returns a channel of
// ChatChunk values. Each non-terminal chunk carries a Delta string from the
// choices[0].delta.content field. The terminal chunk (Done=true) carries
// InputTokens and OutputTokens from the usage field on the final stream event
// (requires stream_options.include_usage=true).
//
// A system-role llm.Message is sent as a system message in the messages array
// (OpenAI Chat Completions accepts system messages inline, unlike Anthropic's
// top-level system param).
//
// The caller must drain the channel to completion; closing the context cancels
// the upstream HTTP request.
func (p *Provider) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}

	// Build the messages array. OpenAI accepts system messages inline.
	// ASSUMPTION A6: message constructors. Verify with go doc and adapt.
	var apiMessages []openaisdk.ChatCompletionMessageParamUnion
	for _, m := range messages {
		switch m.Role {
		case "system":
			apiMessages = append(apiMessages, openaisdk.SystemMessage(m.Content))
		case "user":
			apiMessages = append(apiMessages, openaisdk.UserMessage(m.Content))
		case "assistant":
			apiMessages = append(apiMessages, openaisdk.AssistantMessage(m.Content))
		}
	}

	// ASSUMPTION A8: stream_options for include_usage.
	// ASSUMPTION A7: MaxTokens field name (may be MaxCompletionTokens — verify).
	params := openaisdk.ChatCompletionNewParams{
		Model:     openaisdk.ChatModel(model),
		MaxTokens: openaisdk.Int(int64(maxTokens)),
		Messages:  apiMessages,
		StreamOptions: openaisdk.ChatCompletionStreamOptionsParam{
			IncludeUsage: openaisdk.Bool(true),
		},
	}

	// ASSUMPTION A2: streaming call path.
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)

	ch := make(chan llm.ChatChunk, 32)

	go func() {
		defer close(ch)
		defer stream.Close()

		var inputTokens int
		var outputTokens int

		for stream.Next() {
			chunk := stream.Current()

			// ASSUMPTION A4: delta content path.
			// ASSUMPTION A5: usage on final chunk.
			//
			// The OpenAI streaming protocol sends delta chunks where
			// choices[0].delta.content carries text. The final chunk (when
			// include_usage=true) has an empty choices array and carries Usage.
			// Some SDK versions expose usage on every chunk (zero until final).

			// Emit delta if present.
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta.Content
				if delta != "" {
					select {
					case ch <- llm.ChatChunk{Delta: delta}:
					case <-ctx.Done():
						select {
						case ch <- llm.ChatChunk{Err: ctx.Err(), Done: true}:
						default:
						}
						return
					}
				}
			}

			// Capture usage when present (the final chunk carries non-zero values).
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				inputTokens = int(chunk.Usage.PromptTokens)
				outputTokens = int(chunk.Usage.CompletionTokens)
			}
		}

		if err := stream.Err(); err != nil {
			select {
			case ch <- llm.ChatChunk{Err: redactedErr(err), Done: true}:
			default:
			}
			return
		}

		// Stream complete. Emit the terminal Done chunk with accumulated usage.
		select {
		case ch <- llm.ChatChunk{Done: true, InputTokens: inputTokens, OutputTokens: outputTokens}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// redactedErr wraps the original error, ensuring the OpenAI API key is never
// surfaced in error messages. The SDK does not embed the key in errors, but
// this is a defensive belt-and-suspenders guard required by the §2 security
// invariant and README §7 build-fail checklist.
func redactedErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("openai provider error: %w", err)
}
```

- [ ] **Step 2.2: Verify it compiles**

```
cd C:\src\ai\intake\relay
go build ./internal/llm/openai/
```

**If it does not compile**, read the compiler error. The most likely causes:
- `openaisdk.ChatCompletionMessageParamUnion` is a different type name → fix the import alias and type reference
- `openaisdk.SystemMessage` / `openaisdk.UserMessage` / `openaisdk.AssistantMessage` don't exist → check `go doc github.com/openai/openai-go` for the actual message constructor names (may be `openaisdk.ChatCompletionMessageParam{Role: "user", Content: ...}` or similar union)
- `openaisdk.ChatCompletionNewParams.MaxTokens` is `MaxCompletionTokens` → rename the field
- `stream.Current()` returns a different type → check the return type with `go doc`
- `chunk.Usage.PromptTokens` / `chunk.Usage.CompletionTokens` → field may be `InputTokens`/`OutputTokens` or `PromptTokenCount`/`CompletionTokenCount` → check `go doc`
- `p.client.Chat.Completions.NewStreaming` → path may be `p.client.Chat.Completions.NewStreaming` or a different path; check `go doc github.com/openai/openai-go.Client`

Fix until `go build ./internal/llm/openai/` exits 0. Document each deviation from the plan's assumed symbols in the `DEVIATION LOG` comment block at the top of `openai.go`.

- [ ] **Step 2.3: Run go vet**

```
cd C:\src\ai\intake\relay
go vet ./internal/llm/openai/
```

Expected: exits 0, no output.

---

### Task 3: Write mock-HTTP unit tests (`openai_test.go`)

**Files:**
- Create: `relay/internal/llm/openai/openai_test.go`

**Context:** The test serves a canned SSE stream from an `httptest.Server` and uses `NewWithClient` to point the provider at it. This is identical in structure to `anthropic_test.go`. The SSE stream format must match what the openai-go SDK parses — the SDK handles `data:` lines itself, so the test server just needs to emit valid OpenAI SSE lines.

**OpenAI SSE stream format reference:**
- Each event: `data: <JSON>\n\n`
- Delta events: `{"id":"...","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"text"},"finish_reason":null}]}`
- Final usage event (with `include_usage:true`): `{"id":"...","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":7,"total_tokens":17}}`
- Stream end: `data: [DONE]\n\n`

**IMPLEMENTER NOTE:** Verify that the `openai-go` SDK's streaming iterator can be driven by a raw SSE httptest server. The SDK must be able to parse the `data:` lines it receives. Check `go doc` for how the SDK's streaming client is initialized — if it requires specific headers (e.g. `Content-Type: text/event-stream`) ensure the mock server sets them.

- [ ] **Step 3.1: Write the canned SSE constant and mock server helper**

Create `relay/internal/llm/openai/openai_test.go`:

```go
package openai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/llm"
	openaipkg "intake/internal/llm/openai"
)

// cannedSSE is a minimal OpenAI Chat Completions streaming response that emits
// two text deltas ("Hello", ", world") and a terminal usage chunk
// (prompt_tokens=10, completion_tokens=7).
//
// The format follows the OpenAI SSE wire protocol:
//   - Each event: data: <JSON>\n\n
//   - Delta: choices[0].delta.content carries the text
//   - Final usage chunk: choices=[] + usage object (requires include_usage:true)
//   - Stream terminated by: data: [DONE]\n\n
//
// IMPLEMENTER: if the openai-go SDK parses the chunks differently (e.g. usage
// field name differs), adapt the JSON to match what the SDK expects. Use
// `go doc github.com/openai/openai-go.ChatCompletionChunk` to confirm field names.
const cannedSSE = "" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}` + "\n\n" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}` + "\n\n" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":", world"},"finish_reason":null}]}` + "\n\n" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":7,"total_tokens":17}}` + "\n\n" +
	"data: [DONE]\n\n"

// newMockServer returns an httptest.Server that serves the canned SSE response
// for any POST to the chat completions endpoint and returns its URL.
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
```

- [ ] **Step 3.2: Write `TestChat_MockStreaming`**

Append to `openai_test.go`:

```go
func TestChat_MockStreaming(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	p := openaipkg.NewWithClient(
		"test-key-not-real",
		"gpt-4o-mini",
		1024,
		srv.Client(),
		srv.URL,
	)

	ctx := context.Background()
	messages := []llm.Message{
		{Role: "user", Content: "say hello"},
	}
	opts := llm.ChatOptions{
		Model:     "gpt-4o-mini",
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
			if chunk.Delta != "" {
				deltas = append(deltas, chunk.Delta)
			}
		}
	}

	// Assert the two non-empty deltas arrived in order.
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
```

- [ ] **Step 3.3: Write `TestChat_ContextCancellation`**

Append to `openai_test.go`:

```go
func TestChat_ContextCancellation(t *testing.T) {
	// The mock server blocks after the first delta until the client disconnects.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Write one delta then block; the test cancels before we get further.
		fmt.Fprint(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`+"\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := openaipkg.NewWithClient(
		"test-key-not-real",
		"gpt-4o-mini",
		1024,
		srv.Client(),
		srv.URL,
	)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Chat(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{Stream: true})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	// Read the first chunk, then cancel.
	first := <-ch
	if first.Delta != "partial" {
		t.Errorf("expected 'partial', got %q", first.Delta)
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
```

- [ ] **Step 3.4: Write `TestName` and `TestKeyNotInError`**

Append to `openai_test.go`:

```go
func TestName(t *testing.T) {
	p := openaipkg.NewWithClient("key", "gpt-4o-mini", 1024, http.DefaultClient, "http://unused")
	if p.Name() != "openai" {
		t.Errorf("Name(): got %q, want %q", p.Name(), "openai")
	}
}

func TestKeyNotInError(t *testing.T) {
	// Verifies that if the provider encounters an HTTP error, the API key does
	// NOT appear in the returned error text.
	const sensitiveKey = "sk-SUPER-SECRET-KEY-OPENAI-ABCDEF"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid api key","type":"invalid_request_error","code":"invalid_api_key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := openaipkg.NewWithClient(
		sensitiveKey,
		"gpt-4o-mini",
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

- [ ] **Step 3.5: Run the unit tests (first run — expect failures if SDK surface differs)**

```
cd C:\src\ai\intake\relay
go test -race -v ./internal/llm/openai/
```

If tests fail because the SDK's streaming format differs from the canned SSE, adapt `cannedSSE` to match the SDK's parser expectations. The canned SSE must produce the chunks the SDK delivers to `.Current()`. Use `go doc` to understand the SDK's parser — it may expect slightly different JSON shapes.

**If `TestChat_MockStreaming` fails with "expected 2 delta chunks, got 0":** the SDK may emit an initial empty-content role chunk that the provider's delta guard (`if delta != ""`) correctly skips — but the test also skips it. Check whether the provider correctly captures "Hello" and ", world". If not, the `Choices[0].Delta.Content` field name is wrong in `openai.go` — fix it.

**If `TestChat_MockStreaming` fails with wrong usage values:** the usage JSON field names in `cannedSSE` may differ from what `chunk.Usage.PromptTokens/CompletionTokens` parse. Adapt `cannedSSE` to match the SDK's expected field names.

Expected final output:
```
--- PASS: TestChat_MockStreaming (0.00s)
--- PASS: TestChat_ContextCancellation (0.00s)
--- PASS: TestName (0.00s)
--- PASS: TestKeyNotInError (0.00s)
PASS
ok  	intake/internal/llm/openai	0.XXXs
```

- [ ] **Step 3.6: Commit unit tests**

```
cd C:\src\ai\intake
git add relay/internal/llm/openai/openai.go relay/internal/llm/openai/openai_test.go
git commit -m "feat(2-ii): add openai provider with mock-HTTP unit tests"
```

---

### Task 4: Write the integration test (`integration_test.go`)

**Files:**
- Create: `relay/internal/llm/openai/integration_test.go`

**CREDIT GUARD: DO NOT RUN THIS TEST during implementation. It makes real OpenAI API calls. Write it; leave it gated behind the `integration` build tag.**

- [ ] **Step 4.1: Create the integration test file**

Create `relay/internal/llm/openai/integration_test.go`:

```go
//go:build integration

package openai_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"intake/internal/llm"
	openaipkg "intake/internal/llm/openai"
)

// TestIntegration_RealStream performs a live OpenAI API call.
// It is excluded from the default test run (go test ./...) and CI.
//
// To run:
//
//	export OPENAI_API_KEY=sk-...
//	cd relay
//	go test -tags integration -v ./internal/llm/openai/
//
// The test asserts:
//  1. At least one delta chunk arrives with non-empty text.
//  2. The terminal Done chunk has non-zero InputTokens and OutputTokens.
//  3. The joined deltas contain a recognisable word (non-empty assistant reply).
//  4. The API key does not appear in any error message.
func TestIntegration_RealStream(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set — skipping integration test")
	}

	// Use a small, cheap model for the integration test.
	// Confirm this model ID is still valid at the live smoke.
	p := openaipkg.New(apiKey, "gpt-4o-mini", 64)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messages := []llm.Message{
		{Role: "user", Content: "Reply with exactly three words."},
	}
	opts := llm.ChatOptions{
		Model:     "gpt-4o-mini",
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

- [ ] **Step 4.2: Verify the integration test is excluded from the default test run**

```
cd C:\src\ai\intake\relay
go test -v ./internal/llm/openai/
```

Expected: the integration test does NOT appear — only the four unit tests run.

```
--- PASS: TestChat_MockStreaming (0.00s)
--- PASS: TestChat_ContextCancellation (0.00s)
--- PASS: TestName (0.00s)
--- PASS: TestKeyNotInError (0.00s)
PASS
ok  	intake/internal/llm/openai	0.XXXs
```

- [ ] **Step 4.3: Commit the integration test**

```
cd C:\src\ai\intake
git add relay/internal/llm/openai/integration_test.go
git commit -m "feat(2-ii): add openai integration test (build-tagged, not run during impl)"
```

---

### Task 5: Wire the factory — replace the `"openai"` stub in `providers.go`

**Files:**
- Modify: `relay/internal/llm/providers/providers.go`
- Modify: `relay/internal/llm/providers/providers_test.go`

**Pre-condition:** `relay/internal/llm/providers/providers.go` exists (created in 2-i) with an `"openai"` case returning a "not yet implemented" error.

- [ ] **Step 5.1: Add the openai import and replace the stub case**

Open `relay/internal/llm/providers/providers.go`. It currently looks like (from 2-i):

```go
package providers

import (
	"fmt"

	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/llm/anthropic"
)

// New constructs the configured provider, resolving its secret via config.RequireSecret.
// Errors on an unknown provider name or a missing required secret.
func New(cfg config.LLMConfig) (llm.Provider, error) {
	switch cfg.Provider {
	case "anthropic":
		key, err := config.RequireSecret(cfg.Anthropic.APIKeyEnv)
		if err != nil {
			return nil, err
		}
		return anthropic.New(key, cfg.Anthropic.Model, cfg.Anthropic.MaxTokens), nil
	case "openai":
		return nil, fmt.Errorf("openai provider: not yet implemented")
	case "gemini":
		return nil, fmt.Errorf("gemini provider: not yet implemented")
	case "ollama":
		return nil, fmt.Errorf("ollama provider: not yet implemented")
	default:
		return nil, fmt.Errorf("unknown llm provider %q", cfg.Provider)
	}
}
```

Add the `openai` import and replace the `"openai"` case:

```go
package providers

import (
	"fmt"

	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/llm/anthropic"
	"intake/internal/llm/openai"
)

// New constructs the configured provider, resolving its secret via config.RequireSecret.
// Errors on an unknown provider name or a missing required secret.
func New(cfg config.LLMConfig) (llm.Provider, error) {
	switch cfg.Provider {
	case "anthropic":
		key, err := config.RequireSecret(cfg.Anthropic.APIKeyEnv)
		if err != nil {
			return nil, err
		}
		return anthropic.New(key, cfg.Anthropic.Model, cfg.Anthropic.MaxTokens), nil
	case "openai":
		key, err := config.RequireSecret(cfg.OpenAI.APIKeyEnv)
		if err != nil {
			return nil, err
		}
		return openai.New(key, cfg.OpenAI.Model, cfg.OpenAI.MaxTokens), nil
	case "gemini":
		return nil, fmt.Errorf("gemini provider: not yet implemented")
	case "ollama":
		return nil, fmt.Errorf("ollama provider: not yet implemented")
	default:
		return nil, fmt.Errorf("unknown llm provider %q", cfg.Provider)
	}
}
```

**NOTE:** The package import alias `openai` will conflict with the package name if the package declares `package openai` (which it does). Go resolves this by using the last path segment as the import name, so `"intake/internal/llm/openai"` is accessible as `openai.New(...)`. No alias needed unless there is a collision.

- [ ] **Step 5.2: Verify providers.go compiles**

```
cd C:\src\ai\intake\relay
go build ./internal/llm/providers/
```

Expected: exits 0, no output.

- [ ] **Step 5.3: Update the factory test to expect a real openai provider**

Open `relay/internal/llm/providers/providers_test.go`. It currently has a test for the `"openai"` case that expects a "not yet implemented" error (from 2-i). Update that test so it expects `Name() == "openai"`.

The factory test uses a fake API key — the test must NOT set a real `OPENAI_API_KEY` env var. The key only needs to be non-empty so `RequireSecret` doesn't error. Use a sentinel env var name that won't be set in CI.

The existing test structure from 2-i looks like:

```go
package providers_test

import (
	"os"
	"testing"

	"intake/internal/config"
	"intake/internal/llm/providers"
)

func TestNew_Anthropic(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-real")
	cfg := config.LLMConfig{
		Provider: "anthropic",
		Anthropic: config.AnthropicConfig{
			APIKeyEnv: "ANTHROPIC_API_KEY",
			Model:     "claude-sonnet-4-6",
			MaxTokens: 1024,
		},
	}
	p, err := providers.New(cfg)
	if err != nil {
		t.Fatalf("providers.New() error: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("Name() = %q; want %q", p.Name(), "anthropic")
	}
}

func TestNew_OpenAI_NotImplemented(t *testing.T) {
	cfg := config.LLMConfig{Provider: "openai"}
	_, err := providers.New(cfg)
	if err == nil {
		t.Fatal("expected error for openai not-implemented, got nil")
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	cfg := config.LLMConfig{Provider: "does-not-exist"}
	_, err := providers.New(cfg)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestNew_MissingSecret(t *testing.T) {
	// Ensure the env var is NOT set.
	os.Unsetenv("ANTHROPIC_API_KEY")
	cfg := config.LLMConfig{
		Provider: "anthropic",
		Anthropic: config.AnthropicConfig{
			APIKeyEnv: "ANTHROPIC_API_KEY",
			Model:     "claude-sonnet-4-6",
			MaxTokens: 1024,
		},
	}
	_, err := providers.New(cfg)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}
}
```

Replace `TestNew_OpenAI_NotImplemented` with `TestNew_OpenAI`:

```go
func TestNew_OpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key-not-real")
	cfg := config.LLMConfig{
		Provider: "openai",
		OpenAI: config.OpenAIConfig{
			APIKeyEnv: "OPENAI_API_KEY",
			Model:     "gpt-4o-mini",
			MaxTokens: 1024,
		},
	}
	p, err := providers.New(cfg)
	if err != nil {
		t.Fatalf("providers.New() error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("Name() = %q; want %q", p.Name(), "openai")
	}
}
```

Also add a test for missing OpenAI secret:

```go
func TestNew_OpenAI_MissingSecret(t *testing.T) {
	os.Unsetenv("OPENAI_API_KEY")
	cfg := config.LLMConfig{
		Provider: "openai",
		OpenAI: config.OpenAIConfig{
			APIKeyEnv: "OPENAI_API_KEY",
			Model:     "gpt-4o-mini",
			MaxTokens: 1024,
		},
	}
	_, err := providers.New(cfg)
	if err == nil {
		t.Fatal("expected error for missing OPENAI_API_KEY, got nil")
	}
}
```

- [ ] **Step 5.4: Run the providers tests**

```
cd C:\src\ai\intake\relay
go test -race -v ./internal/llm/providers/
```

Expected:
```
--- PASS: TestNew_Anthropic (0.00s)
--- PASS: TestNew_OpenAI (0.00s)
--- PASS: TestNew_OpenAI_MissingSecret (0.00s)
--- PASS: TestNew_UnknownProvider (0.00s)
--- PASS: TestNew_MissingSecret (0.00s)
PASS
ok  	intake/internal/llm/providers	0.XXXs
```

- [ ] **Step 5.5: Run the full test suite**

```
cd C:\src\ai\intake\relay
go test -race ./...
```

Expected: all packages pass. If `go test ./...` runs integration tests (it should not, since the `integration` build tag is required), verify the build tag is correct in `integration_test.go`.

- [ ] **Step 5.6: Commit the factory wiring**

```
cd C:\src\ai\intake
git add relay/internal/llm/providers/providers.go relay/internal/llm/providers/providers_test.go
git commit -m "feat(2-ii): wire openai provider into providers factory; update factory tests"
```

---

### Task 6: Full verification pass

**Files:** none new

- [ ] **Step 6.1: Build and vet**

```
cd C:\src\ai\intake\relay
go build ./... && echo BUILD_OK
go vet ./... && echo VET_OK
```

Expected:
```
BUILD_OK
VET_OK
```

- [ ] **Step 6.2: Race-safe test run (all packages, no integration tag)**

```
cd C:\src\ai\intake\relay
go test -race ./internal/llm/openai/ ./internal/llm/providers/ && echo OPENAI_TESTS_OK
go test -race ./... && echo ALL_TESTS_OK
```

Expected:
```
ok  	intake/internal/llm/openai	0.XXXs
ok  	intake/internal/llm/providers	0.XXXs
OPENAI_TESTS_OK
... (all other packages) ...
ALL_TESTS_OK
```

**If any test fails:** do not proceed. Debug the failure. The most likely remaining failures at this point:
- A package that imports `providers` (e.g. `main`) fails to compile because the `openai` import in `providers.go` indirectly brings in a package that the `relay` module doesn't yet have in `go.sum` — run `go mod tidy` and retry.
- `TestNew_OpenAI_MissingSecret` fails because the `OPENAI_API_KEY` env var is set in your shell — `t.Setenv` in another test leaked it (unlikely with `t.Setenv` which auto-restores, but check).

- [ ] **Step 6.3: Contract + pin gates**

```
cd C:\src\ai\intake
bash scripts/check-pins.sh && echo PINS_OK
bash scripts/verify-contract.sh && echo CONTRACT_OK
```

Expected:
```
OK: Go module pins verified (go.sum enforces exact versions for santhosh-tekuri/jsonschema/v6 and google/uuid)
OK: all codegen tools are exact-pinned
PINS_OK
=== verify-contract.sh: ALL CHECKS PASSED ===
CONTRACT_OK
```

- [ ] **Step 6.4: Paste the actual command outputs into this task checklist as evidence**

Replace the "Expected:" blocks above with the actual output. This is the verification record required by the PHASE_PLANNING build-fail checklist and §4 (Verification Before Done).

---

## 5. Smoke (5-turn live conversation — DEFERRED, credit-guarded)

**DO NOT RUN during implementation.** This smoke spends real OpenAI API credits. It runs only at the Phase 2 final credit-smoke gate after all four providers are implemented.

Pre-condition:
1. Phase 2 merged (all of 2-i through 2-iv complete).
2. `OPENAI_API_KEY` exported in the shell.
3. `config.yaml` updated:
   ```yaml
   llm:
     provider: openai
     openai:
       api_key_env: OPENAI_API_KEY
       model: gpt-4o-mini
       max_tokens: 1024
   ```
4. Relay started: `cd relay && go run ./cmd/relay/`; confirm `GET /v1/health` returns 200.

Execution:
```
RELAY_URL=http://localhost:8080 npx tsx core/smoke/drive-multi.ts
```
(The `drive-multi.ts` 5-turn driver was created in sub-plan 2-i.)

Verification (all must be true):
- All 5 turns complete; each SSE stream emits at least one `ChatChunk{Delta}` and a terminal `ChatChunk{Done: true, InputTokens: N, OutputTokens: M}` with N > 0 and M > 0.
- The string `sk-` does NOT appear anywhere in the relay's stdout log.
- The model ID `gpt-4o-mini` resolves successfully (if it has been retired, update to the current default and note the change in `ai/tasks/phase-2/README.md` §5).

Teardown: stop the relay (`Ctrl-C`); re-runnable.

---

## 6. Done criteria

- [ ] `go build ./...` and `go vet ./...` pass in `relay/` with no errors.
- [ ] `go test -race ./internal/llm/openai/ ./internal/llm/providers/` passes with zero failures, no real key required.
- [ ] `go test -race ./...` passes (all relay packages, no integration tag).
- [ ] `bash scripts/check-pins.sh` passes, including the new `openai-go` caret gate.
- [ ] `bash scripts/verify-contract.sh` passes (staleness + contract gates unchanged).
- [ ] `providers.New` with `provider: "openai"` and a non-empty `OPENAI_API_KEY` returns a provider where `Name() == "openai"` (proven by `TestNew_OpenAI`).
- [ ] The string `sk-` (or any API key value) does NOT appear in any relay log line (proven by `TestKeyNotInError` and by visual inspection of the relay's stdout during the deferred smoke).
- [ ] `relay/internal/llm/openai/integration_test.go` exists and is gated by `//go:build integration` — it does NOT run in `go test ./...`.
- [ ] All SDK surface deviations from the plan's assumptions are documented in the `DEVIATION LOG` comment at the top of `openai.go`.
- [ ] The Phase 2 README §5 pin table is updated with the exact `openai-go` version.
- [ ] The deferred 5-turn live smoke is listed in the Phase 2 final smoke gate (not run yet).
