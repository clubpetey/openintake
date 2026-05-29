package config_test

import (
	"os"
	"reflect"
	"strings"
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
	// Auth.Modes (4-i) — sample enables all three.
	if !cfg.Auth.Modes.Anonymous || !cfg.Auth.Modes.Email || !cfg.Auth.Modes.SSO {
		t.Errorf("AuthModes = %+v; want all three true", cfg.Auth.Modes)
	}
	// Auth.Email parsed (explicit values, not defaults).
	if cfg.Auth.Email.SMTPHost != "smtp.example.com" {
		t.Errorf("Email.SMTPHost = %q; want smtp.example.com", cfg.Auth.Email.SMTPHost)
	}
	if cfg.Auth.Email.SMTPPort != 587 {
		t.Errorf("Email.SMTPPort = %d; want 587", cfg.Auth.Email.SMTPPort)
	}
	if cfg.Auth.Email.SMTPPassEnv != "INTAKE_SMTP_PASS" {
		t.Errorf("Email.SMTPPassEnv = %q; want INTAKE_SMTP_PASS", cfg.Auth.Email.SMTPPassEnv)
	}
	if cfg.Auth.Email.JWTSecretEnv != "INTAKE_EMAIL_JWT_SECRET" {
		t.Errorf("Email.JWTSecretEnv = %q; want INTAKE_EMAIL_JWT_SECRET", cfg.Auth.Email.JWTSecretEnv)
	}
	// Auth.SSO parsed.
	if cfg.Auth.SSO.Issuer != "https://example.us.auth0.com/" {
		t.Errorf("SSO.Issuer = %q; mismatch", cfg.Auth.SSO.Issuer)
	}
	if cfg.Auth.SSO.JWKSURL != "https://example.us.auth0.com/.well-known/jwks.json" {
		t.Errorf("SSO.JWKSURL = %q; mismatch", cfg.Auth.SSO.JWKSURL)
	}
	if cfg.Auth.SSO.Claims.UserID != "sub" {
		t.Errorf("SSO.Claims.UserID = %q; want sub", cfg.Auth.SSO.Claims.UserID)
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

func TestLoad_AppliesDefaultRoutingAdapter(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Routing.DefaultAdapter != "chatwoot" {
		t.Errorf("default Routing.DefaultAdapter = %q; want %q", cfg.Routing.DefaultAdapter, "chatwoot")
	}
}

func TestLoad_StringList_ScalarAndSequence(t *testing.T) {
	cfg, err := config.Load("testdata/sample.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.Routing.Rules) < 2 {
		t.Fatalf("expected >=2 routing rules, got %d", len(cfg.Routing.Rules))
	}
	// Rule 0 uses a scalar classification ("bug").
	r0 := cfg.Routing.Rules[0]
	if len(r0.When.Classification) != 1 || r0.When.Classification[0] != "bug" {
		t.Errorf("rule0 classification = %v; want [bug]", r0.When.Classification)
	}
	if r0.To != "chatwoot" {
		t.Errorf("rule0.To = %q; want chatwoot", r0.To)
	}
	// Rule 1 uses a sequence classification (["question","other"]).
	r1 := cfg.Routing.Rules[1]
	if len(r1.When.Classification) != 2 {
		t.Errorf("rule1 classification = %v; want 2 entries", r1.When.Classification)
	}
}

func TestLoad_ParsesAdapterBlocks(t *testing.T) {
	cfg, err := config.Load("testdata/sample.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !cfg.Adapters.Chatwoot.Enabled || cfg.Adapters.Chatwoot.AccountID != 1 || cfg.Adapters.Chatwoot.InboxID != 3 {
		t.Errorf("chatwoot block mis-parsed: %+v", cfg.Adapters.Chatwoot)
	}
	if cfg.Adapters.Chatwoot.APITokenEnv != "CHATWOOT_TOKEN" {
		t.Errorf("chatwoot.api_token_env = %q; want CHATWOOT_TOKEN", cfg.Adapters.Chatwoot.APITokenEnv)
	}
	if cfg.Adapters.Zendesk.Subdomain != "example" || cfg.Adapters.Zendesk.Email != "agent@example.com" {
		t.Errorf("zendesk block mis-parsed: %+v", cfg.Adapters.Zendesk)
	}
	if cfg.Adapters.Linear.TeamID != "TEAM_ID" {
		t.Errorf("linear.team_id = %q; want TEAM_ID", cfg.Adapters.Linear.TeamID)
	}
	if cfg.License.File != "/etc/intake/license.json" {
		t.Errorf("license.file = %q; want /etc/intake/license.json", cfg.License.File)
	}
}

func TestLoad_AppliesAuthEmailDefaults(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.Email.CodeTTL != "10m" {
		t.Errorf("default Email.CodeTTL = %q; want 10m", cfg.Auth.Email.CodeTTL)
	}
	if cfg.Auth.Email.JWTTTL != "15m" {
		t.Errorf("default Email.JWTTTL = %q; want 15m", cfg.Auth.Email.JWTTTL)
	}
}

func TestLoad_AppliesSSOClaimDefaults(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.SSO.Claims.UserID != "sub" {
		t.Errorf("default SSO.Claims.UserID = %q; want sub", cfg.Auth.SSO.Claims.UserID)
	}
	if cfg.Auth.SSO.Claims.Email != "email" {
		t.Errorf("default SSO.Claims.Email = %q; want email", cfg.Auth.SSO.Claims.Email)
	}
	if cfg.Auth.SSO.Claims.DisplayName != "name" {
		t.Errorf("default SSO.Claims.DisplayName = %q; want name", cfg.Auth.SSO.Claims.DisplayName)
	}
}

func TestLoad_AuthModesDefaultOnlyAnonymousTrue(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Phase 1 default: anonymous only. Phase 4 adds the two flags, both default false.
	if cfg.Auth.Modes.Email {
		t.Error("default AuthModes.Email = true; want false (opt-in)")
	}
	if cfg.Auth.Modes.SSO {
		t.Error("default AuthModes.SSO = true; want false (opt-in)")
	}
}

func TestLoad_AppliesPhase5DefaultsForRateLimit(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RateLimit.PerIP.RequestsPerSecond != 1.0 {
		t.Errorf("default PerIP.RequestsPerSecond = %v; want 1.0", cfg.RateLimit.PerIP.RequestsPerSecond)
	}
	if cfg.RateLimit.PerIP.Burst != 5 {
		t.Errorf("default PerIP.Burst = %d; want 5", cfg.RateLimit.PerIP.Burst)
	}
	if cfg.RateLimit.PerIP.IdleTTL != "15m" {
		t.Errorf("default PerIP.IdleTTL = %q; want 15m", cfg.RateLimit.PerIP.IdleTTL)
	}
	if cfg.RateLimit.PerSession.MaxTurns != 20 {
		t.Errorf("default PerSession.MaxTurns = %d; want 20", cfg.RateLimit.PerSession.MaxTurns)
	}
	if cfg.RateLimit.PerSession.MaxInputTokens != 8000 {
		t.Errorf("default PerSession.MaxInputTokens = %d; want 8000", cfg.RateLimit.PerSession.MaxInputTokens)
	}
	if cfg.RateLimit.PerSession.SessionTTL != "1h" {
		t.Errorf("default PerSession.SessionTTL = %q; want 1h", cfg.RateLimit.PerSession.SessionTTL)
	}
	if cfg.RateLimit.DailyLLMBudget.MaxInputTokens != 5_000_000 {
		t.Errorf("default DailyLLMBudget.MaxInputTokens = %d; want 5_000_000", cfg.RateLimit.DailyLLMBudget.MaxInputTokens)
	}
	if cfg.RateLimit.DailyLLMBudget.MaxOutputTokens != 1_000_000 {
		t.Errorf("default DailyLLMBudget.MaxOutputTokens = %d; want 1_000_000", cfg.RateLimit.DailyLLMBudget.MaxOutputTokens)
	}
	if cfg.RateLimit.DailyLLMBudget.ActionOnExceeded != "reject" {
		t.Errorf("default DailyLLMBudget.ActionOnExceeded = %q; want reject", cfg.RateLimit.DailyLLMBudget.ActionOnExceeded)
	}
}

func TestLoad_AppliesPhase5DefaultsForCaptcha(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Captcha.Enabled {
		t.Error("default Captcha.Enabled = true; want false (opt-in)")
	}
	// When the YAML omits required_for entirely, default to ["anonymous"].
	if len(cfg.Captcha.RequiredFor) != 1 || cfg.Captcha.RequiredFor[0] != "anonymous" {
		t.Errorf("default Captcha.RequiredFor = %v; want [anonymous]", cfg.Captcha.RequiredFor)
	}
}

func TestLoad_RequiredForExplicitEmptyHonored(t *testing.T) {
	// Write a temp YAML with an explicit `required_for: []` and confirm the default
	// is NOT applied (operator opted out for all modes).
	tmp := t.TempDir() + "/captcha-empty.yaml"
	body := []byte("captcha:\n  enabled: true\n  required_for: []\n")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	cfg, err := config.Load(tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Captcha.RequiredFor == nil {
		t.Fatalf("explicit `required_for: []` should remain a non-nil empty slice; got nil (defaults applied)")
	}
	if len(cfg.Captcha.RequiredFor) != 0 {
		t.Errorf("explicit `required_for: []` should remain empty; got %v", cfg.Captcha.RequiredFor)
	}
}

func TestLoad_AppliesPhase5DefaultsForServerTrustedProxies(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// nil slice and empty slice are equivalent at the wire; len handles both.
	if len(cfg.Server.TrustedProxies) != 0 {
		t.Errorf("default TrustedProxies = %v; want empty (len 0)", cfg.Server.TrustedProxies)
	}
}

func TestLoad_AppliesPhase5DefaultsForAnonymousAllowWithoutCaptcha(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.Anonymous.AllowWithoutCaptcha {
		t.Error("default Auth.Anonymous.AllowWithoutCaptcha = true; want false (Q9 fail-closed)")
	}
}

func TestValidate_RejectsUnsupportedActionOnExceeded(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "queue"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil for action_on_exceeded=queue; want error")
	}
	if !strings.Contains(err.Error(), "action_on_exceeded") {
		t.Errorf("Validate() error %q does not mention action_on_exceeded", err.Error())
	}
}

func TestValidate_AcceptsReject(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() returned error for action_on_exceeded=reject: %v", err)
	}
}

func TestLoad_AppliesPhase6DefaultsForAttachments(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Attachments.Enabled {
		t.Error("default Attachments.Enabled = false; want true")
	}
	if cfg.Attachments.MaxSizeBytes != 5_242_880 {
		t.Errorf("default MaxSizeBytes = %d; want 5_242_880 (5 MB)", cfg.Attachments.MaxSizeBytes)
	}
	if cfg.Attachments.MaxTotalBytes != 10_485_760 {
		t.Errorf("default MaxTotalBytes = %d; want 10_485_760 (10 MB)", cfg.Attachments.MaxTotalBytes)
	}
	want := []string{"image/png", "image/jpeg", "image/webp"}
	if !reflect.DeepEqual(cfg.Attachments.AllowedMIMETypes, want) {
		t.Errorf("default AllowedMIMETypes = %v; want %v", cfg.Attachments.AllowedMIMETypes, want)
	}
	if cfg.Attachments.Storage.Mode != "" {
		t.Errorf("default Storage.Mode = %q; want \"\" (empty defaults to forward semantics)", cfg.Attachments.Storage.Mode)
	}
}

func TestLoad_ExplicitDisabledAttachmentsHonored(t *testing.T) {
	tmp := t.TempDir() + "/disabled.yaml"
	body := []byte("attachments:\n  enabled: false\n")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	cfg, err := config.Load(tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Attachments.Enabled {
		t.Error("Attachments.Enabled = true; want false (explicit disable honored)")
	}
	// Other defaults still apply on the disabled path so reading them is safe.
	if cfg.Attachments.MaxSizeBytes != 5_242_880 {
		t.Errorf("MaxSizeBytes = %d; want 5_242_880 even on disabled path", cfg.Attachments.MaxSizeBytes)
	}
}

func TestLoad_ParsesSampleYAMLPhase6AttachmentsBlock(t *testing.T) {
	cfg, err := config.Load("testdata/sample.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Attachments.Enabled {
		t.Error("sample.yaml has enabled:true; got false")
	}
	if cfg.Attachments.Storage.Mode != "forward" {
		t.Errorf("sample.yaml storage.mode = %q; want forward", cfg.Attachments.Storage.Mode)
	}
	if len(cfg.Attachments.AllowedMIMETypes) != 3 {
		t.Errorf("sample.yaml AllowedMIMETypes len = %d; want 3", len(cfg.Attachments.AllowedMIMETypes))
	}
}

func TestLoad_ParsesSampleYAMLPhase5Blocks(t *testing.T) {
	cfg, err := config.Load("testdata/sample.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Server.TrustedProxies) != 2 {
		t.Errorf("Server.TrustedProxies len = %d; want 2 (sample.yaml)", len(cfg.Server.TrustedProxies))
	}
	if cfg.Captcha.Provider != "turnstile" {
		t.Errorf("Captcha.Provider = %q; want turnstile", cfg.Captcha.Provider)
	}
	if cfg.RateLimit.PerIP.RequestsPerSecond != 2.0 {
		t.Errorf("PerIP.RequestsPerSecond = %v; want 2.0", cfg.RateLimit.PerIP.RequestsPerSecond)
	}
	if cfg.RateLimit.DailyLLMBudget.ActionOnExceeded != "reject" {
		t.Errorf("ActionOnExceeded = %q; want reject", cfg.RateLimit.DailyLLMBudget.ActionOnExceeded)
	}
	if cfg.Auth.Anonymous.AllowWithoutCaptcha {
		t.Error("sample.yaml sets AllowWithoutCaptcha=false; got true")
	}
}
