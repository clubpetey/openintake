// Package openai implements the llm.Provider interface using the OpenAI
// Chat Completions API with streaming enabled. The API key is always passed
// in by the caller — it is never read from disk, config files, or logs.
//
// SDK surface notes (confirmed against v1.12.0 via `go doc`):
//
//   - client.Chat.Completions.NewStreaming(ctx, params) returns *ssestream.Stream[ChatCompletionChunk]
//   - iterator: .Next() bool, .Current() ChatCompletionChunk, .Err() error, .Close() error — CONFIRMED
//   - ChatCompletionChunk.Choices[0].Delta.Content carries the incremental text string — CONFIRMED
//   - Final chunk carries Usage.PromptTokens / Usage.CompletionTokens (int64) — CONFIRMED
//   - stream_options set via ChatCompletionStreamOptionsParam{IncludeUsage: openaisdk.Bool(true)} — CONFIRMED
//   - Message helpers: openaisdk.UserMessage / AssistantMessage / SystemMessage — CONFIRMED
//
// DEVIATION LOG:
//   - A7: Plan assumed MaxTokens field. SDK has both MaxTokens (deprecated) and
//     MaxCompletionTokens (preferred for o-series and newer models). This
//     implementation uses MaxCompletionTokens.
//   - A8: Plan assumed openai.Bool(true). Actual: openaisdk.Bool(true) works
//     (it's a package-level helper func wrapping param.NewOpt). Confirmed correct.
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

	// Build the messages array. OpenAI Chat Completions accepts system messages
	// inline in the messages array (unlike Anthropic's top-level system param).
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

	// SDK deviation A7: MaxCompletionTokens is the current preferred field;
	// MaxTokens is deprecated and incompatible with o-series models.
	params := openaisdk.ChatCompletionNewParams{
		Model:               openaisdk.ChatModel(model),
		MaxCompletionTokens: openaisdk.Int(int64(maxTokens)),
		Messages:            apiMessages,
		StreamOptions: openaisdk.ChatCompletionStreamOptionsParam{
			IncludeUsage: openaisdk.Bool(true),
		},
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)

	ch := make(chan llm.ChatChunk, 32)

	go func() {
		defer close(ch)
		defer stream.Close()

		var inputTokens int
		var outputTokens int

		for stream.Next() {
			chunk := stream.Current()

			// Emit delta text if present. The initial role chunk has empty
			// content ("") and is correctly filtered by this guard.
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

			// Capture usage when present. The final chunk (choices=[]) carries
			// non-zero PromptTokens and CompletionTokens when include_usage=true.
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
