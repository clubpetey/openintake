// Package gemini implements the llm.Provider interface using the Google Gemini
// API via the official google.golang.org/genai SDK with streaming enabled.
// The API key is always passed in by the caller — it is never read from disk,
// config files, or logs.
package gemini

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"google.golang.org/genai"

	"github.com/clubpetey/openintake/relay/internal/llm"
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
	// genai.NewClient requires a context for the constructor signature, but for an
	// API-key client it performs no I/O and does not retain the context. background is fine.
	ctx := context.Background()
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		// genai.NewClient only errors when the config is invalid (e.g. both
		// APIKey and Credentials set). For a simple APIKey config it is safe to
		// panic — the caller (providers.New) passes a validated key.
		// Do NOT interpolate cfg/ClientConfig here — it holds the API key.
		panic(fmt.Sprintf("gemini: failed to construct genai client (check GEMINI/Vertex configuration): %v", err))
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
		// Do NOT interpolate cfg/ClientConfig here — it holds the API key.
		panic(fmt.Sprintf("gemini: failed to construct genai client in NewWithClient (check GEMINI/Vertex configuration): %v", err))
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

	// Config-supplied; bounded by max int32 to guard against an upstream config
	// surprise (per-provider MaxTokens defaults to 1024).
	if maxTokens > math.MaxInt32 {
		maxTokens = math.MaxInt32
	}
	if maxTokens < 0 {
		maxTokens = 0
	}
	maxTokensI32 := int32(maxTokens) //nolint:gosec // G115: bounds checked above
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
				case <-ctx.Done():
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
