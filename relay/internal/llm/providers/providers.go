// Package providers is the LLM provider selection seam. It imports all provider
// implementations and constructs the one specified by cfg.Provider. It lives in
// a separate package (not package llm) to avoid the import cycle:
//
//	llm/anthropic → llm (for the interface)
//	providers → llm + llm/anthropic (no back-edge into llm/*)
//
// main.go calls providers.New(cfg.LLM) and never constructs a provider directly.
package providers

import (
	"fmt"

	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/llm/anthropic"
)

// New constructs the provider selected by cfg.Provider, resolving the required
// secret via config.RequireSecret / config.ResolveSecret. It returns an error
// if the provider name is unknown, the required secret is absent, or
// construction fails. It returns a nil provider on any error.
//
// Switch cases:
//   - "anthropic" — wired: resolves ANTHROPIC_API_KEY and calls anthropic.New.
//   - "openai"    — placeholder until sub-plan 2-ii.
//   - "gemini"    — placeholder until sub-plan 2-iii.
//   - "ollama"    — placeholder until sub-plan 2-iv.
//   - default     — returns a clear "unknown provider" error.
func New(cfg config.LLMConfig) (llm.Provider, error) {
	switch cfg.Provider {
	case "anthropic":
		key, err := config.RequireSecret(cfg.Anthropic.APIKeyEnv)
		if err != nil {
			return nil, fmt.Errorf("providers: anthropic: %w", err)
		}
		return anthropic.New(key, cfg.Anthropic.Model, cfg.Anthropic.MaxTokens), nil

	case "openai":
		return nil, fmt.Errorf("llm provider %q not implemented in this build", cfg.Provider)

	case "gemini":
		return nil, fmt.Errorf("llm provider %q not implemented in this build", cfg.Provider)

	case "ollama":
		return nil, fmt.Errorf("llm provider %q not implemented in this build", cfg.Provider)

	default:
		return nil, fmt.Errorf("unknown llm provider %q", cfg.Provider)
	}
}
