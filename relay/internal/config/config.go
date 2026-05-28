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
	Server    ServerConfig    `yaml:"server"`
	LLM       LLMConfig       `yaml:"llm"`
	Auth      AuthConfig      `yaml:"auth"`
	Adapters  AdaptersConfig  `yaml:"adapters"`
	Routing   RoutingConfig   `yaml:"routing"`
	License   LicenseConfig   `yaml:"license"`
	Captcha   CaptchaConfig   `yaml:"captcha"`   // Phase 5
	RateLimit RateLimitConfig `yaml:"ratelimit"` // Phase 5
}

// ServerConfig holds HTTP server and CORS settings.
type ServerConfig struct {
	Addr           string   `yaml:"addr"`
	ExternalURL    string   `yaml:"external_url"`
	CORSOrigins    []string `yaml:"cors_origins"`
	TrustedProxies []string `yaml:"trusted_proxies"` // Phase 5: CIDR list; empty = always use RemoteAddr
}

// LLMConfig selects the active provider and holds per-provider config.
type LLMConfig struct {
	Provider         string          `yaml:"provider"`
	Anthropic        AnthropicConfig `yaml:"anthropic"`
	OpenAI           OpenAIConfig    `yaml:"openai"`
	Gemini           GeminiConfig    `yaml:"gemini"`
	Ollama           OllamaConfig    `yaml:"ollama"`
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

// OpenAIConfig holds OpenAI-specific settings.
// APIKeyEnv is the NAME of the environment variable containing the API key.
type OpenAIConfig struct {
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

// GeminiConfig holds Gemini-specific settings.
// APIKeyEnv is the NAME of the environment variable containing the API key.
type GeminiConfig struct {
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

// OllamaConfig holds Ollama-specific settings.
// BearerTokenEnv is optional: "" means no auth header is sent.
type OllamaConfig struct {
	BaseURL        string `yaml:"base_url"`
	Model          string `yaml:"model"`
	BearerTokenEnv string `yaml:"bearer_token_env"`
	MaxTokens      int    `yaml:"max_tokens"`
}

// AuthConfig selects which auth modes are enabled and configures email/SSO.
type AuthConfig struct {
	Modes     AuthModes       `yaml:"modes"`
	Email     EmailConfig     `yaml:"email"`
	SSO       SSOConfig       `yaml:"sso"`
	Anonymous AnonymousConfig `yaml:"anonymous"` // Phase 5 Q9 escape hatch
}

// AnonymousConfig configures anonymous mode (Phase 5).
type AnonymousConfig struct {
	// AllowWithoutCaptcha is the Q9 escape hatch (PROJECT.md §19 Q9):
	// when true, the relay starts even though auth.modes.anonymous=true and
	// captcha is not gating anonymous. Default false (fail-closed).
	AllowWithoutCaptcha bool `yaml:"allow_without_captcha"`
}

// AuthModes enables or disables specific auth strategies.
type AuthModes struct {
	// Anonymous advertises that the relay accepts X-Intake-Session headers.
	// Note: setting this to false does NOT disable anonymous access — the
	// dispatcher accepts anonymous sessions whenever a valid X-Intake-Session
	// is presented. This flag is advertisement-only (read by initHandler to
	// populate Capabilities.AuthModes).
	Anonymous bool `yaml:"anonymous"`
	Email     bool `yaml:"email"`
	SSO       bool `yaml:"sso"`
}

// EmailConfig configures the email magic-link mode.
// All secrets reference an env var name; the value resolves via ResolveSecret in main.go.
type EmailConfig struct {
	SMTPHost     string `yaml:"smtp_host"`
	SMTPPort     int    `yaml:"smtp_port"`
	SMTPUser     string `yaml:"smtp_user"`
	SMTPPassEnv  string `yaml:"smtp_pass_env"`  // env var holding the SMTP password
	From         string `yaml:"from"`           // RFC 5322 From address
	CodeTTL      string `yaml:"code_ttl"`       // default "10m"
	JWTTTL       string `yaml:"jwt_ttl"`        // default "15m"
	JWTSecretEnv string `yaml:"jwt_secret_env"` // env var; resolved value must be ≥32 bytes
}

// SSOConfig configures host-app SSO. Exactly one of JWKSURL (RS256) or
// HS256SecretEnv (HS256) must be set; both-set or neither-set is a startup error.
type SSOConfig struct {
	Issuer         string    `yaml:"issuer"`           // expected `iss` claim
	Audience       string    `yaml:"audience"`         // expected `aud` claim
	JWKSURL        string    `yaml:"jwks_url"`         // RS256 path
	HS256SecretEnv string    `yaml:"hs256_secret_env"` // HS256 path; env var name
	Claims         SSOClaimNames `yaml:"claims"`
}

// SSOClaimNames maps SessionContext fields to JWT claim names. Defaults: sub/email/name (standard OIDC).
type SSOClaimNames struct {
	UserID      string `yaml:"user_id"`
	Email       string `yaml:"email"`
	DisplayName string `yaml:"display_name"`
}

// AdaptersConfig holds per-adapter configuration. Webhook is Phase 1; the other
// four are added in Phase 3 (3-ii…3-v construct them; their config is read here).
type AdaptersConfig struct {
	Webhook  WebhookConfig  `yaml:"webhook"`
	Chatwoot ChatwootConfig `yaml:"chatwoot"`
	Fider    FiderConfig    `yaml:"fider"`
	Zendesk  ZendeskConfig  `yaml:"zendesk"`
	Linear   LinearConfig   `yaml:"linear"`
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

// ChatwootConfig configures the Chatwoot adapter (free). APITokenEnv is the NAME
// of the env var holding the api_access_token; the value resolves via ResolveSecret.
type ChatwootConfig struct {
	Enabled     bool   `yaml:"enabled"`
	BaseURL     string `yaml:"base_url"`
	AccountID   int    `yaml:"account_id"`
	InboxID     int    `yaml:"inbox_id"`
	APITokenEnv string `yaml:"api_token_env"`
}

// FiderConfig configures the Fider adapter (free).
type FiderConfig struct {
	Enabled   bool   `yaml:"enabled"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// ZendeskConfig configures the Zendesk adapter (paid).
type ZendeskConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Subdomain       string `yaml:"subdomain"`
	Email           string `yaml:"email"`
	APITokenEnv     string `yaml:"api_token_env"`
	DefaultPriority string `yaml:"default_priority"`
}

// LinearConfig configures the Linear adapter (paid).
type LinearConfig struct {
	Enabled   bool   `yaml:"enabled"`
	APIKeyEnv string `yaml:"api_key_env"`
	TeamID    string `yaml:"team_id"`
}

// RoutingConfig selects which adapter receives a submission.
type RoutingConfig struct {
	DefaultAdapter string `yaml:"default_adapter"`
	Rules          []Rule `yaml:"rules"`
}

// Rule maps a classification/severity match to an adapter name.
type Rule struct {
	When RuleMatch `yaml:"when"`
	To   string    `yaml:"to"`
}

// RuleMatch matches a submission's classification and/or severity. Each field
// accepts a YAML scalar ("bug") OR a sequence (["question","other"]). An empty
// field is a wildcard (matches anything).
type RuleMatch struct {
	Classification StringList `yaml:"classification"`
	Severity       StringList `yaml:"severity"`
}

// LicenseConfig holds the optional explicit license path. The full load order
// (CLI flag, INTAKE_LICENSE, INTAKE_LICENSE_FILE, default paths) is applied by
// internal/license.Load in 3-vi; this field is one input to it.
type LicenseConfig struct {
	File string `yaml:"file"`
}

// StringList unmarshals either a YAML scalar or a sequence of strings into []string.
type StringList []string

// UnmarshalYAML accepts a scalar ("bug") or a sequence (["a","b"]).
func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var single string
		if err := value.Decode(&single); err != nil {
			return err
		}
		*s = StringList{single}
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*s = StringList(list)
	default:
		return fmt.Errorf("config: classification/severity must be a string or a list of strings")
	}
	return nil
}

// CaptchaConfig configures the optional CAPTCHA challenge gate at /v1/intake/init.
// All secrets reference an env var name; the value resolves via ResolveSecret in main.go.
type CaptchaConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Provider     string `yaml:"provider"`       // "turnstile" | "hcaptcha"
	SiteKey      string `yaml:"site_key"`       // public; safe to commit
	SecretKeyEnv string `yaml:"secret_key_env"` // env var name; ResolveSecret
	// RequiredFor lists the auth modes that must solve a CAPTCHA before /init mints
	// a session. Default applied by applyDefaults when YAML omits the key: ["anonymous"].
	// An explicit empty list `required_for: []` is honored (operator opted out).
	RequiredFor []string `yaml:"required_for"`
}

// captchaConfigRaw lets us detect "key omitted entirely" vs "explicit empty list".
type captchaConfigRaw struct {
	Enabled      bool      `yaml:"enabled"`
	Provider     string    `yaml:"provider"`
	SiteKey      string    `yaml:"site_key"`
	SecretKeyEnv string    `yaml:"secret_key_env"`
	RequiredFor  *[]string `yaml:"required_for"` // pointer distinguishes nil vs []
}

// UnmarshalYAML implements yaml.Unmarshaler so we can distinguish a missing
// required_for key (apply default) from an explicit empty list (honor as-is).
func (c *CaptchaConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw captchaConfigRaw
	if err := value.Decode(&raw); err != nil {
		return err
	}
	c.Enabled = raw.Enabled
	c.Provider = raw.Provider
	c.SiteKey = raw.SiteKey
	c.SecretKeyEnv = raw.SecretKeyEnv
	if raw.RequiredFor != nil {
		c.RequiredFor = *raw.RequiredFor
		if c.RequiredFor == nil {
			c.RequiredFor = []string{} // normalise: explicit empty is non-nil
		}
	}
	// raw.RequiredFor == nil → leave c.RequiredFor as nil; applyDefaults will populate
	return nil
}

// RateLimitConfig holds the Phase-5 abuse-control sub-configurations.
type RateLimitConfig struct {
	PerIP          PerIPConfig          `yaml:"per_ip"`
	PerSession     PerSessionConfig     `yaml:"per_session"`
	DailyLLMBudget DailyLLMBudgetConfig `yaml:"daily_llm_budget"`
}

// PerIPConfig configures the per-IP token bucket.
type PerIPConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"` // default 1.0
	Burst             int     `yaml:"burst"`               // default 5
	IdleTTL           string  `yaml:"idle_ttl"`            // default "15m"
}

// PerSessionConfig configures per-session counters and TTL.
type PerSessionConfig struct {
	MaxTurns       int    `yaml:"max_turns"`        // default 20
	MaxInputTokens int    `yaml:"max_input_tokens"` // default 8000
	SessionTTL     string `yaml:"session_ttl"`      // default "1h"
}

// DailyLLMBudgetConfig configures the daily LLM spend cap.
type DailyLLMBudgetConfig struct {
	MaxInputTokens   int    `yaml:"max_input_tokens"`   // default 5_000_000
	MaxOutputTokens  int    `yaml:"max_output_tokens"`  // default 1_000_000
	ActionOnExceeded string `yaml:"action_on_exceeded"` // "reject" only in v0; "queue" is v1+
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
	// OpenAI defaults
	if c.LLM.OpenAI.APIKeyEnv == "" {
		c.LLM.OpenAI.APIKeyEnv = "OPENAI_API_KEY"
	}
	if c.LLM.OpenAI.Model == "" {
		c.LLM.OpenAI.Model = "gpt-4o-mini"
	}
	if c.LLM.OpenAI.MaxTokens == 0 {
		c.LLM.OpenAI.MaxTokens = 1024
	}
	// Gemini defaults
	if c.LLM.Gemini.APIKeyEnv == "" {
		c.LLM.Gemini.APIKeyEnv = "GEMINI_API_KEY"
	}
	if c.LLM.Gemini.Model == "" {
		c.LLM.Gemini.Model = "gemini-2.0-flash"
	}
	if c.LLM.Gemini.MaxTokens == 0 {
		c.LLM.Gemini.MaxTokens = 1024
	}
	// Ollama defaults
	if c.LLM.Ollama.BaseURL == "" {
		c.LLM.Ollama.BaseURL = "http://localhost:11434"
	}
	if c.LLM.Ollama.Model == "" {
		c.LLM.Ollama.Model = "llama3.1"
	}
	if c.LLM.Ollama.MaxTokens == 0 {
		c.LLM.Ollama.MaxTokens = 1024
	}
	// BearerTokenEnv intentionally left as "" (no default — absence means no auth)
	// Default to chatwoot (the documented primary free adapter, PROJECT.md §9).
	// NOTE: chatwoot must be enabled (3-ii+) or routing.default_adapter overridden,
	// else router.New fails fast at startup — that is the intended §4.4 guard.
	if c.Routing.DefaultAdapter == "" {
		c.Routing.DefaultAdapter = "chatwoot"
	}
	// Auth (4-i): email/SSO sub-structs gain sensible defaults.
	if c.Auth.Email.CodeTTL == "" {
		c.Auth.Email.CodeTTL = "10m"
	}
	if c.Auth.Email.JWTTTL == "" {
		c.Auth.Email.JWTTTL = "15m"
	}
	if c.Auth.SSO.Claims.UserID == "" {
		c.Auth.SSO.Claims.UserID = "sub"
	}
	if c.Auth.SSO.Claims.Email == "" {
		c.Auth.SSO.Claims.Email = "email"
	}
	if c.Auth.SSO.Claims.DisplayName == "" {
		c.Auth.SSO.Claims.DisplayName = "name"
	}
	if c.Adapters.Webhook.Retry.MaxAttempts == 0 {
		c.Adapters.Webhook.Retry.MaxAttempts = 3
	}
	if c.Adapters.Webhook.Retry.Backoff == "" {
		c.Adapters.Webhook.Retry.Backoff = "exponential"
	}
	// Phase 5 defaults
	if c.RateLimit.PerIP.RequestsPerSecond == 0 {
		c.RateLimit.PerIP.RequestsPerSecond = 1.0
	}
	if c.RateLimit.PerIP.Burst == 0 {
		c.RateLimit.PerIP.Burst = 5
	}
	if c.RateLimit.PerIP.IdleTTL == "" {
		c.RateLimit.PerIP.IdleTTL = "15m"
	}
	if c.RateLimit.PerSession.MaxTurns == 0 {
		c.RateLimit.PerSession.MaxTurns = 20
	}
	if c.RateLimit.PerSession.MaxInputTokens == 0 {
		c.RateLimit.PerSession.MaxInputTokens = 8000
	}
	if c.RateLimit.PerSession.SessionTTL == "" {
		c.RateLimit.PerSession.SessionTTL = "1h"
	}
	if c.RateLimit.DailyLLMBudget.MaxInputTokens == 0 {
		c.RateLimit.DailyLLMBudget.MaxInputTokens = 5_000_000
	}
	if c.RateLimit.DailyLLMBudget.MaxOutputTokens == 0 {
		c.RateLimit.DailyLLMBudget.MaxOutputTokens = 1_000_000
	}
	if c.RateLimit.DailyLLMBudget.ActionOnExceeded == "" {
		c.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	}
	// Captcha: only populate RequiredFor when nil (UnmarshalYAML leaves it nil
	// for "key omitted" and [] for "explicit empty"). Default: ["anonymous"].
	if c.Captcha.RequiredFor == nil {
		c.Captcha.RequiredFor = []string{"anonymous"}
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

// Validate enforces invariants that defaults alone can't express. Currently:
//   - DailyLLMBudget.ActionOnExceeded must be "reject" (v0 only ships reject;
//     "queue" is documented as v1+ per PROJECT.md §10).
//
// Returns an error wrapping every problem; callers (main.go) typically append
// this to the Q9 consolidated startup-gate slice.
func (c *Config) Validate() error {
	switch c.RateLimit.DailyLLMBudget.ActionOnExceeded {
	case "reject":
		return nil
	case "":
		// applyDefaults should have populated this; treat as a programmer error.
		return fmt.Errorf("ratelimit.daily_llm_budget.action_on_exceeded is empty; Load must have applied default \"reject\"")
	default:
		return fmt.Errorf("ratelimit.daily_llm_budget.action_on_exceeded=%q is not supported in v0; only \"reject\" is supported (\"queue\" is documented as v1+)", c.RateLimit.DailyLLMBudget.ActionOnExceeded)
	}
}
