package ollama_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clubpetey/openintake/relay/internal/llm"
	ollamapkg "github.com/clubpetey/openintake/relay/internal/llm/ollama"
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
// Each sub-test gets its own httptest.Server and local capturedAuth guarded by a
// sync.Mutex so the test is safe under -race.
func TestChat_BearerTokenSent(t *testing.T) {
	const testToken = "test-bearer-token-xyz"

	// newBearerServer creates a fresh server that records the Authorization header.
	newBearerServer := func(t *testing.T) (*httptest.Server, *string, *sync.Mutex) {
		t.Helper()
		var mu sync.Mutex
		var capturedAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			mu.Lock()
			capturedAuth = auth
			mu.Unlock()
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":1,"eval_count":1}`+"\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}))
		return srv, &capturedAuth, &mu
	}

	t.Run("bearer token present", func(t *testing.T) {
		srv, capturedAuthPtr, mu := newBearerServer(t)
		defer srv.Close()
		p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 64, testToken, srv.Client())
		ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{})
		if err != nil {
			t.Fatalf("Chat() error: %v", err)
		}
		for range ch {
		}
		want := "Bearer " + testToken
		mu.Lock()
		got := *capturedAuthPtr
		mu.Unlock()
		if got != want {
			t.Errorf("Authorization header: got %q, want %q", got, want)
		}
	})

	t.Run("no bearer token", func(t *testing.T) {
		srv, capturedAuthPtr, mu := newBearerServer(t)
		defer srv.Close()
		p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 64, "", srv.Client())
		ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{})
		if err != nil {
			t.Fatalf("Chat() error: %v", err)
		}
		for range ch {
		}
		mu.Lock()
		got := *capturedAuthPtr
		mu.Unlock()
		if got != "" {
			t.Errorf("Authorization header should be absent, got %q", got)
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

// TestChat_Non2xxWithBody verifies that when the server returns a non-2xx status,
// Chat returns a synchronous error (not a channel) whose message contains both the
// status code and text from the response body. The bearer token must not appear.
func TestChat_Non2xxWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"model 'nope' not found, try pulling it first"}`)
	}))
	defer srv.Close()

	p := ollamapkg.NewWithClient(srv.URL, "nope", 64, "", srv.Client())
	ch, err := p.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{})

	// The implementation surfaces non-2xx as a synchronous error from Chat().
	// If it instead returns a channel with an error chunk, we accept that too.
	var errText string
	if err != nil {
		errText = err.Error()
	} else {
		for chunk := range ch {
			if chunk.Err != nil {
				errText = chunk.Err.Error()
				break
			}
		}
		// drain remaining
		if ch != nil {
			for range ch {
			}
		}
	}

	if errText == "" {
		t.Fatal("expected an error for non-2xx response, got none")
	}
	if !strings.Contains(errText, "404") {
		t.Errorf("error should contain status code 404; got: %q", errText)
	}
	if !strings.Contains(errText, "not found") {
		t.Errorf("error should contain body text 'not found'; got: %q", errText)
	}
}

// TestChat_StreamWithoutDone verifies that when the server closes the connection
// after emitting delta lines without a done:true terminal line, both deltas arrive
// and a terminal ChatChunk{Done:true} with zero token counts is emitted (no hang).
func TestChat_StreamWithoutDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"alpha"},"done":false}`+"\n")
		fmt.Fprint(w, `{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"beta"},"done":false}`+"\n")
		// Close without sending a done:true line.
	}))
	defer srv.Close()

	p := ollamapkg.NewWithClient(srv.URL, "llama3.1", 64, "", srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := p.Chat(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatOptions{})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	var deltas []string
	var terminalChunk llm.ChatChunk
	var gotTerminal bool
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected error chunk: %v", chunk.Err)
		}
		if chunk.Done {
			terminalChunk = chunk
			gotTerminal = true
		} else if chunk.Delta != "" {
			deltas = append(deltas, chunk.Delta)
		}
	}

	if len(deltas) != 2 {
		t.Fatalf("expected 2 delta chunks, got %d: %v", len(deltas), deltas)
	}
	if deltas[0] != "alpha" {
		t.Errorf("delta[0]: got %q, want %q", deltas[0], "alpha")
	}
	if deltas[1] != "beta" {
		t.Errorf("delta[1]: got %q, want %q", deltas[1], "beta")
	}
	if !gotTerminal {
		t.Fatal("expected a terminal Done=true chunk, got none")
	}
	if terminalChunk.InputTokens != 0 {
		t.Errorf("InputTokens: got %d, want 0", terminalChunk.InputTokens)
	}
	if terminalChunk.OutputTokens != 0 {
		t.Errorf("OutputTokens: got %d, want 0", terminalChunk.OutputTokens)
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
