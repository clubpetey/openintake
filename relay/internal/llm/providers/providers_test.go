package providers_test

import (
	"strings"
	"testing"

	"intake/internal/config"
	"intake/internal/llm/providers"
)

// TestNew_Anthropic_WithKey verifies that a valid anthropic config + key env
// returns a provider whose Name() is "anthropic". Uses t.Setenv to avoid
// touching real environment state. No real API call is made.
func TestNew_Anthropic_WithKey(t *testing.T) {
	t.Setenv("TEST_ANTHROPIC_KEY", "sk-ant-fake-key-for-unit-test")

	cfg := config.LLMConfig{
		Provider: "anthropic",
		Anthropic: config.AnthropicConfig{
			APIKeyEnv: "TEST_ANTHROPIC_KEY",
			Model:     "claude-sonnet-4-6",
			MaxTokens: 1024,
		},
	}

	p, err := providers.New(cfg)
	if err != nil {
		t.Fatalf("providers.New() returned error: %v", err)
	}
	if p == nil {
		t.Fatal("providers.New() returned nil provider with nil error")
	}
	if p.Name() != "anthropic" {
		t.Errorf("provider.Name() = %q; want %q", p.Name(), "anthropic")
	}
}

// TestNew_Anthropic_MissingKey verifies that a missing required API key env
// returns a non-nil error and a nil provider.
func TestNew_Anthropic_MissingKey(t *testing.T) {
	// Ensure the env var is NOT set.
	t.Setenv("TEST_ANTHROPIC_MISSING", "")

	cfg := config.LLMConfig{
		Provider: "anthropic",
		Anthropic: config.AnthropicConfig{
			APIKeyEnv: "TEST_ANTHROPIC_MISSING",
			Model:     "claude-sonnet-4-6",
			MaxTokens: 1024,
		},
	}

	p, err := providers.New(cfg)
	if err == nil {
		t.Fatal("providers.New() returned nil error for missing required key; want non-nil")
	}
	if p != nil {
		t.Errorf("providers.New() returned non-nil provider with an error; want nil provider")
	}
	// Error must mention the env var name to guide the operator — not the key value.
	if !strings.Contains(err.Error(), "TEST_ANTHROPIC_MISSING") {
		t.Errorf("error should mention the env var name; got: %v", err)
	}
}

// TestNew_UnknownProvider verifies that an unrecognised provider name returns
// a clear error and a nil provider.
func TestNew_UnknownProvider(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "bogus",
	}

	p, err := providers.New(cfg)
	if err == nil {
		t.Fatal("providers.New() returned nil error for unknown provider; want non-nil")
	}
	if p != nil {
		t.Errorf("providers.New() returned non-nil provider for unknown name; want nil")
	}
	// Error must mention the provider name.
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should mention the unknown provider name; got: %v", err)
	}
}

// TestNew_OpenAI_NotImplemented verifies that "openai" returns the expected
// "not implemented in this build" placeholder error (until sub-plan 2-ii).
func TestNew_OpenAI_NotImplemented(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "openai",
		OpenAI: config.OpenAIConfig{
			APIKeyEnv: "OPENAI_API_KEY",
			Model:     "gpt-4o-mini",
			MaxTokens: 1024,
		},
	}

	p, err := providers.New(cfg)
	if err == nil {
		t.Fatal("providers.New() for openai returned nil error; want not-implemented error")
	}
	if p != nil {
		t.Errorf("providers.New() for openai returned non-nil provider; want nil")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error should contain 'not implemented'; got: %v", err)
	}
}

// TestNew_Gemini_NotImplemented verifies that "gemini" returns the not-implemented placeholder.
func TestNew_Gemini_NotImplemented(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "gemini",
		Gemini: config.GeminiConfig{
			APIKeyEnv: "GEMINI_API_KEY",
			Model:     "gemini-2.0-flash",
			MaxTokens: 1024,
		},
	}

	p, err := providers.New(cfg)
	if err == nil {
		t.Fatal("providers.New() for gemini returned nil error; want not-implemented error")
	}
	if p != nil {
		t.Errorf("providers.New() for gemini returned non-nil provider; want nil")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error should contain 'not implemented'; got: %v", err)
	}
}

// TestNew_Ollama_NotImplemented verifies that "ollama" returns the not-implemented placeholder.
func TestNew_Ollama_NotImplemented(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "ollama",
		Ollama: config.OllamaConfig{
			BaseURL:   "http://localhost:11434",
			Model:     "llama3.1",
			MaxTokens: 1024,
		},
	}

	p, err := providers.New(cfg)
	if err == nil {
		t.Fatal("providers.New() for ollama returned nil error; want not-implemented error")
	}
	if p != nil {
		t.Errorf("providers.New() for ollama returned non-nil provider; want nil")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error should contain 'not implemented'; got: %v", err)
	}
}
