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
// SSE format used by google.golang.org/genai: the SDK's scanner splits on
// double-newlines (\n\n) — each SSE event must be terminated with \n\n.
// The line inside each event starts with "data:" followed by a space, then
// JSON-encoded GenerateContentResponse. The SDK splits on the first ":" so
// "data" is the prefix and " <JSON>" (with leading space) is the data value
// which json.Unmarshal handles fine.
const cannedStream = "" +
	`data: {"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"}}]}` + "\n\n" +
	`data: {"candidates":[{"content":{"parts":[{"text":", world"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":7,"totalTokenCount":17}}` + "\n\n"

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
		// The SDK scanner splits on \n\n so each event must be double-newline terminated.
		fmt.Fprint(w, `data: {"candidates":[{"content":{"parts":[{"text":"partial"}],"role":"model"}}]}`+"\n\n")
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
