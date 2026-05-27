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
// Field names match what github.com/openai/openai-go v1.12.0 parses:
//   - chunk.Usage.PromptTokens    → JSON "prompt_tokens"
//   - chunk.Usage.CompletionTokens → JSON "completion_tokens"
//   - chunk.Choices[0].Delta.Content → JSON "content"
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

func TestName(t *testing.T) {
	p := openaipkg.NewWithClient("key", "gpt-4o-mini", 1024, http.DefaultClient, "http://unused")
	if p.Name() != "openai" {
		t.Errorf("Name(): got %q, want %q", p.Name(), "openai")
	}
}

// cannedSSENoUsage is a minimal OpenAI Chat Completions streaming response with
// two text deltas and a [DONE] sentinel but NO usage chunk. This exercises the
// path where chunk.JSON.Usage.Valid() is always false, so InputTokens and
// OutputTokens on the terminal Done chunk must be 0.
const cannedSSENoUsage = "" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}` + "\n\n" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"foo"},"finish_reason":null}]}` + "\n\n" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"bar"},"finish_reason":"stop"}]}` + "\n\n" +
	"data: [DONE]\n\n"

// TestChat_StreamWithoutUsage verifies that a stream with no usage chunk
// delivers all deltas and emits a terminal Done chunk with zero tokens (does
// not hang or skip the Done).
func TestChat_StreamWithoutUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, cannedSSENoUsage)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	p := openaipkg.NewWithClient("test-key-not-real", "gpt-4o-mini", 1024, srv.Client(), srv.URL)

	ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{Stream: true})
	if err != nil {
		t.Fatalf("Chat() returned error: %v", err)
	}

	var deltas []string
	var terminalChunk llm.ChatChunk
	var gotDone bool

	done := make(chan struct{})
	go func() {
		for chunk := range ch {
			if chunk.Err != nil {
				t.Errorf("unexpected error chunk: %v", chunk.Err)
			}
			if chunk.Done {
				terminalChunk = chunk
				gotDone = true
			} else if chunk.Delta != "" {
				deltas = append(deltas, chunk.Delta)
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed within 3s — possible hang")
	}

	if len(deltas) != 2 {
		t.Fatalf("expected 2 delta chunks, got %d: %v", len(deltas), deltas)
	}
	if deltas[0] != "foo" {
		t.Errorf("delta[0]: got %q, want %q", deltas[0], "foo")
	}
	if deltas[1] != "bar" {
		t.Errorf("delta[1]: got %q, want %q", deltas[1], "bar")
	}
	if !gotDone {
		t.Error("expected a Done=true terminal chunk")
	}
	if terminalChunk.InputTokens != 0 {
		t.Errorf("InputTokens: got %d, want 0", terminalChunk.InputTokens)
	}
	if terminalChunk.OutputTokens != 0 {
		t.Errorf("OutputTokens: got %d, want 0", terminalChunk.OutputTokens)
	}
}

// cannedSSEWithError is an SSE stream that emits one text delta followed by an
// SSE event whose data contains an "error" field. The openai-go ssestream
// package (ssestream.go:169-173) detects this and sets stream.Err() to a
// non-nil error containing the error message. This exercises the error-arm
// ctx-guard change.
const cannedSSEWithError = "" +
	`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}` + "\n\n" +
	`data: {"error":{"message":"stream interrupted","type":"server_error","code":"stream_error"}}` + "\n\n"

// TestChat_ErrorMidStream verifies that a mid-stream SSE error is surfaced as a
// terminal ChatChunk{Err != nil, Done: true} and that the API key is not leaked.
func TestChat_ErrorMidStream(t *testing.T) {
	const sensitiveKey = "sk-SUPER-SECRET-MIDSTREAM-KEY"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, cannedSSEWithError)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	p := openaipkg.NewWithClient(sensitiveKey, "gpt-4o-mini", 1024, srv.Client(), srv.URL)

	ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{Stream: true})
	if err != nil {
		t.Fatalf("Chat() returned error: %v", err)
	}

	var errChunk llm.ChatChunk
	var gotError bool

	done := make(chan struct{})
	go func() {
		for chunk := range ch {
			if chunk.Done && chunk.Err != nil {
				errChunk = chunk
				gotError = true
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed within 3s after mid-stream error — possible hang")
	}

	if !gotError {
		t.Fatal("expected a terminal ChatChunk with Err != nil and Done=true")
	}
	if errChunk.Err == nil {
		t.Error("terminal chunk Err must be non-nil")
	}
	if strings.Contains(errChunk.Err.Error(), sensitiveKey) {
		t.Errorf("API key leaked into error message: %q", errChunk.Err.Error())
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
