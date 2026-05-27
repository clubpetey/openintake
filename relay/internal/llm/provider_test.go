package llm_test

import (
	"context"
	"testing"

	"intake/internal/llm"
)

// staticCheckProvider is a compile-time assertion that the Provider interface
// has the exact signature from README §6.1. This file intentionally contains
// no runtime assertions; a build failure IS the test failure.
type staticCheckProvider struct{}

func (s *staticCheckProvider) Name() string { return "static" }
func (s *staticCheckProvider) Chat(
	_ context.Context,
	_ []llm.Message,
	_ llm.ChatOptions,
) (<-chan llm.ChatChunk, error) {
	ch := make(chan llm.ChatChunk)
	close(ch)
	return ch, nil
}

// Verify staticCheckProvider satisfies the interface at compile time.
var _ llm.Provider = (*staticCheckProvider)(nil)

func TestProviderInterfaceCompiles(t *testing.T) {
	// Instantiation proves the interface is satisfied at runtime too.
	var p llm.Provider = &staticCheckProvider{}
	if p.Name() != "static" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}
