package config_test

import (
	"testing"

	"intake/internal/config"
)

func TestLoad_ParsesSampleYAML(t *testing.T) {
	cfg, err := config.Load("testdata/sample.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.Addr != ":9000" {
		t.Errorf("Server.Addr = %q; want %q", cfg.Server.Addr, ":9000")
	}
	if cfg.Server.ExternalURL != "http://example.com" {
		t.Errorf("Server.ExternalURL = %q; want %q", cfg.Server.ExternalURL, "http://example.com")
	}
	if len(cfg.Server.CORSOrigins) != 2 {
		t.Errorf("Server.CORSOrigins len = %d; want 2", len(cfg.Server.CORSOrigins))
	}
	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("LLM.Provider = %q; want %q", cfg.LLM.Provider, "anthropic")
	}
	if cfg.LLM.Anthropic.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("LLM.Anthropic.APIKeyEnv = %q; want %q", cfg.LLM.Anthropic.APIKeyEnv, "ANTHROPIC_API_KEY")
	}
	if cfg.LLM.Anthropic.Model != "claude-sonnet-4-6" {
		t.Errorf("LLM.Anthropic.Model = %q; want %q", cfg.LLM.Anthropic.Model, "claude-sonnet-4-6")
	}
	if cfg.LLM.Anthropic.MaxTokens != 2048 {
		t.Errorf("LLM.Anthropic.MaxTokens = %d; want 2048", cfg.LLM.Anthropic.MaxTokens)
	}
	if !cfg.Auth.Modes.Anonymous {
		t.Error("Auth.Modes.Anonymous = false; want true")
	}
	if !cfg.Adapters.Webhook.Enabled {
		t.Error("Adapters.Webhook.Enabled = false; want true")
	}
	if cfg.Adapters.Webhook.URL != "http://localhost:9099/intake" {
		t.Errorf("Adapters.Webhook.URL = %q; want %q", cfg.Adapters.Webhook.URL, "http://localhost:9099/intake")
	}
	if cfg.Adapters.Webhook.Retry.MaxAttempts != 5 {
		t.Errorf("Adapters.Webhook.Retry.MaxAttempts = %d; want 5", cfg.Adapters.Webhook.Retry.MaxAttempts)
	}
	if cfg.Adapters.Webhook.Retry.Backoff != "fixed" {
		t.Errorf("Adapters.Webhook.Retry.Backoff = %q; want %q", cfg.Adapters.Webhook.Retry.Backoff, "fixed")
	}
	if v, ok := cfg.Adapters.Webhook.Headers["X-Custom"]; !ok || v != "value" {
		t.Errorf("Adapters.Webhook.Headers[X-Custom] = %q, ok=%v; want %q, true", v, ok, "value")
	}
	// OpenAI parsed from sample.yaml (explicit values, not defaults)
	if cfg.LLM.OpenAI.APIKeyEnv != "OPENAI_API_KEY" {
		t.Errorf("LLM.OpenAI.APIKeyEnv = %q; want %q", cfg.LLM.OpenAI.APIKeyEnv, "OPENAI_API_KEY")
	}
	if cfg.LLM.OpenAI.Model != "gpt-4o-mini" {
		t.Errorf("LLM.OpenAI.Model = %q; want %q", cfg.LLM.OpenAI.Model, "gpt-4o-mini")
	}
	if cfg.LLM.OpenAI.MaxTokens != 512 {
		t.Errorf("LLM.OpenAI.MaxTokens = %d; want 512", cfg.LLM.OpenAI.MaxTokens)
	}
	// Gemini parsed from sample.yaml
	if cfg.LLM.Gemini.APIKeyEnv != "GEMINI_API_KEY" {
		t.Errorf("LLM.Gemini.APIKeyEnv = %q; want %q", cfg.LLM.Gemini.APIKeyEnv, "GEMINI_API_KEY")
	}
	if cfg.LLM.Gemini.Model != "gemini-2.0-flash" {
		t.Errorf("LLM.Gemini.Model = %q; want %q", cfg.LLM.Gemini.Model, "gemini-2.0-flash")
	}
	if cfg.LLM.Gemini.MaxTokens != 512 {
		t.Errorf("LLM.Gemini.MaxTokens = %d; want 512", cfg.LLM.Gemini.MaxTokens)
	}
	// Ollama parsed from sample.yaml
	if cfg.LLM.Ollama.BaseURL != "http://localhost:11434" {
		t.Errorf("LLM.Ollama.BaseURL = %q; want %q", cfg.LLM.Ollama.BaseURL, "http://localhost:11434")
	}
	if cfg.LLM.Ollama.Model != "llama3.1" {
		t.Errorf("LLM.Ollama.Model = %q; want %q", cfg.LLM.Ollama.Model, "llama3.1")
	}
	if cfg.LLM.Ollama.MaxTokens != 512 {
		t.Errorf("LLM.Ollama.MaxTokens = %d; want 512", cfg.LLM.Ollama.MaxTokens)
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	// Write a minimal YAML into a temp file (only mandatory server section, no llm/adapters).
	// Load() must fill in sane defaults for missing fields.
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.Addr != ":8080" {
		t.Errorf("default Server.Addr = %q; want %q", cfg.Server.Addr, ":8080")
	}
	if cfg.Server.ExternalURL != "http://localhost:8080" {
		t.Errorf("default Server.ExternalURL = %q; want %q", cfg.Server.ExternalURL, "http://localhost:8080")
	}
	if cfg.LLM.Anthropic.Model != "claude-sonnet-4-6" {
		t.Errorf("default LLM.Anthropic.Model = %q; want %q", cfg.LLM.Anthropic.Model, "claude-sonnet-4-6")
	}
	if cfg.LLM.Anthropic.MaxTokens != 1024 {
		t.Errorf("default LLM.Anthropic.MaxTokens = %d; want 1024", cfg.LLM.Anthropic.MaxTokens)
	}
	if cfg.Adapters.Webhook.Retry.MaxAttempts != 3 {
		t.Errorf("default Retry.MaxAttempts = %d; want 3", cfg.Adapters.Webhook.Retry.MaxAttempts)
	}
	if cfg.Adapters.Webhook.Retry.Backoff != "exponential" {
		t.Errorf("default Retry.Backoff = %q; want %q", cfg.Adapters.Webhook.Retry.Backoff, "exponential")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("testdata/does-not-exist.yaml")
	if err == nil {
		t.Fatal("Load returned nil error for missing file; want non-nil")
	}
}

func TestLoad_AppliesOpenAIDefaults(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LLM.OpenAI.APIKeyEnv != "OPENAI_API_KEY" {
		t.Errorf("default OpenAI.APIKeyEnv = %q; want %q", cfg.LLM.OpenAI.APIKeyEnv, "OPENAI_API_KEY")
	}
	if cfg.LLM.OpenAI.Model != "gpt-4o-mini" {
		t.Errorf("default OpenAI.Model = %q; want %q", cfg.LLM.OpenAI.Model, "gpt-4o-mini")
	}
	if cfg.LLM.OpenAI.MaxTokens != 1024 {
		t.Errorf("default OpenAI.MaxTokens = %d; want 1024", cfg.LLM.OpenAI.MaxTokens)
	}
}

func TestLoad_AppliesGeminiDefaults(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LLM.Gemini.APIKeyEnv != "GEMINI_API_KEY" {
		t.Errorf("default Gemini.APIKeyEnv = %q; want %q", cfg.LLM.Gemini.APIKeyEnv, "GEMINI_API_KEY")
	}
	if cfg.LLM.Gemini.Model != "gemini-2.0-flash" {
		t.Errorf("default Gemini.Model = %q; want %q", cfg.LLM.Gemini.Model, "gemini-2.0-flash")
	}
	if cfg.LLM.Gemini.MaxTokens != 1024 {
		t.Errorf("default Gemini.MaxTokens = %d; want 1024", cfg.LLM.Gemini.MaxTokens)
	}
}

func TestLoad_AppliesOllamaDefaults(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LLM.Ollama.BaseURL != "http://localhost:11434" {
		t.Errorf("default Ollama.BaseURL = %q; want %q", cfg.LLM.Ollama.BaseURL, "http://localhost:11434")
	}
	if cfg.LLM.Ollama.Model != "llama3.1" {
		t.Errorf("default Ollama.Model = %q; want %q", cfg.LLM.Ollama.Model, "llama3.1")
	}
	if cfg.LLM.Ollama.MaxTokens != 1024 {
		t.Errorf("default Ollama.MaxTokens = %d; want 1024", cfg.LLM.Ollama.MaxTokens)
	}
	if cfg.LLM.Ollama.BearerTokenEnv != "" {
		t.Errorf("default Ollama.BearerTokenEnv = %q; want empty string", cfg.LLM.Ollama.BearerTokenEnv)
	}
}
