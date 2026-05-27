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
	"io"
	"net/http"
	"strings"

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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	ch := make(chan llm.ChatChunk, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024) // up to 1 MB per line
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
				case <-ctx.Done():
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
			case <-ctx.Done():
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
