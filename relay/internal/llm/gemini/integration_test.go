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
