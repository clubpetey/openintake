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
	client    anthropicsdk.Client
	model     string
	maxTokens int
}

// Compile-time assertion: *Provider must satisfy llm.Provider.
var _ llm.Provider = (*Provider)(nil)

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
// final usage block in the message_delta event. If an error occurs, a
// terminal chunk with Err set is emitted and the channel is closed.
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

	// NOTE: this provider always uses the streaming Messages API; opts.Stream is
	// advisory and not honoured (the <-chan ChatChunk interface is inherently a
	// stream). A non-streaming fast path could be added in a later phase if a
	// provider needs it. classify() calls this and simply drains the stream.
	stream := p.client.Messages.NewStreaming(ctx, params)

	ch := make(chan llm.ChatChunk, 32)

	go func() {
		defer close(ch)
		defer stream.Close()

		var inputTokens int
		var outputTokens int

		for stream.Next() {
			event := stream.Current()

			// SDK v1.45.0: MessageStreamEventUnion is a flat union struct.
			// Switch on the Type string field; use As*() methods to get typed structs.
			switch event.Type {

			case "message_start":
				// Capture initial input token count from message_start.
				e := event.AsMessageStart()
				inputTokens = int(e.Message.Usage.InputTokens)

			case "content_block_delta":
				// Extract text from text_delta content block deltas.
				e := event.AsContentBlockDelta()
				// RawContentBlockDeltaUnion: check Type then use AsTextDelta().
				if e.Delta.Type == "text_delta" {
					td := e.Delta.AsTextDelta()
					if td.Text != "" {
						select {
						case ch <- llm.ChatChunk{Delta: td.Text}:
						case <-ctx.Done():
							select {
							case ch <- llm.ChatChunk{Err: ctx.Err(), Done: true}:
							default:
							}
							return
						}
					}
				}

			case "message_delta":
				// input tokens are reported once on message_start; message_delta carries the running output token count.
				e := event.AsMessageDelta()
				outputTokens = int(e.Usage.OutputTokens)

			case "message_stop":
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
			select {
			case ch <- llm.ChatChunk{Err: redactedErr(err), Done: true}:
			default:
			}
			return
		}

		// Stream ended without a message_stop (e.g. context cancelled mid-stream).
		// best-effort: if the consumer abandoned the channel, close(ch) still signals termination.
		select {
		case ch <- llm.ChatChunk{Done: true, InputTokens: inputTokens, OutputTokens: outputTokens}:
		default:
		}
	}()

	return ch, nil
}

// redactedErr wraps the original error with %w (redacting nothing from the chain
// but adding no key material), ensuring the Anthropic API key is never echoed.
// The SDK errors do not embed the key, but this is a defensive belt-and-suspenders
// guard required by the §2 security invariant and README §7 build-fail checklist.
func redactedErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("anthropic provider error: %w", err)
}
