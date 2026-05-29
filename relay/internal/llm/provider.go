package llm

import "context"

type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
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
	InputTokens  int   // populated on Done
	OutputTokens int   // populated on Done
	Err          error // non-nil => terminal error for this stream
}

type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []Message, opts ChatOptions) (<-chan ChatChunk, error)
}
