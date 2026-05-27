package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level relay configuration. It is frozen in 1-i and extended
// additively by later sub-plans (add fields inside the nested structs; do not
// restructure the top-level shape).
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	LLM      LLMConfig      `yaml:"llm"`
	Auth     AuthConfig     `yaml:"auth"`
	Adapters AdaptersConfig `yaml:"adapters"`
}

// ServerConfig holds HTTP server and CORS settings.
type ServerConfig struct {
	Addr        string   `yaml:"addr"`
	ExternalURL string   `yaml:"external_url"`
	CORSOrigins []string `yaml:"cors_origins"`
}

// LLMConfig selects the active provider and holds per-provider config.
type LLMConfig struct {
	Provider         string          `yaml:"provider"`
	Anthropic        AnthropicConfig `yaml:"anthropic"`
	SystemPromptFile string          `yaml:"system_prompt_file"`
}

// AnthropicConfig holds Anthropic-specific settings.
// APIKeyEnv is the NAME of the environment variable that contains the API key.
// The key itself is never stored in config; it is resolved by 1-ii at startup.
type AnthropicConfig struct {
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`
	// MaxTokens is the maximum number of tokens the model may generate per turn.
	// 0 is invalid for the Anthropic API; applyDefaults sets 1024 for both missing
	// and zero values.
	MaxTokens int `yaml:"max_tokens"`
}

// AuthConfig selects which auth modes are enabled.
type AuthConfig struct {
	Modes AuthModes `yaml:"modes"`
}

// AuthModes enables or disables specific auth strategies.
type AuthModes struct {
	Anonymous bool `yaml:"anonymous"`
}

// AdaptersConfig holds per-adapter configuration.
type AdaptersConfig struct {
	Webhook WebhookConfig `yaml:"webhook"`
}

// WebhookConfig configures the webhook adapter.
type WebhookConfig struct {
	Enabled bool              `yaml:"enabled"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Retry   RetryConfig       `yaml:"retry"`
}

// RetryConfig controls retry behaviour for outbound adapter calls.
type RetryConfig struct {
	MaxAttempts int    `yaml:"max_attempts"`
	Backoff     string `yaml:"backoff"` // "exponential" | "fixed"
}

// applyDefaults applies sane default values for any field not set by the YAML file.
// Called after unmarshalling so that explicit zeros in the file override defaults
// only for non-zero types; for structs we apply defaults after unmarshal and check
// for zero values.
func applyDefaults(c *Config) {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.ExternalURL == "" {
		c.Server.ExternalURL = "http://localhost:8080"
	}
	if c.LLM.Provider == "" {
		c.LLM.Provider = "anthropic"
	}
	if c.LLM.Anthropic.APIKeyEnv == "" {
		c.LLM.Anthropic.APIKeyEnv = "ANTHROPIC_API_KEY"
	}
	if c.LLM.Anthropic.Model == "" {
		c.LLM.Anthropic.Model = "claude-sonnet-4-6"
	}
	if c.LLM.Anthropic.MaxTokens == 0 {
		c.LLM.Anthropic.MaxTokens = 1024
	}
	if c.Adapters.Webhook.Retry.MaxAttempts == 0 {
		c.Adapters.Webhook.Retry.MaxAttempts = 3
	}
	if c.Adapters.Webhook.Retry.Backoff == "" {
		c.Adapters.Webhook.Retry.Backoff = "exponential"
	}
}

// Load reads the YAML config file at path, applies defaults for missing fields,
// and returns a fully-populated Config.
//
// Secret resolution (e.g. reading os.Getenv(cfg.LLM.Anthropic.APIKeyEnv)) is
// intentionally NOT done here. The config holds only the env var NAME; the actual
// key is resolved by the provider constructor in 1-ii.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	applyDefaults(&cfg)
	return &cfg, nil
}
