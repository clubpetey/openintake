# 2-i Config + Providers Factory — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `config.LLMConfig` with OpenAI/Gemini/Ollama sub-structs and their defaults, create the `providers` factory package (anthropic case + placeholder stubs), rewire `main.go` to use the factory, and add a provider-agnostic 5-turn smoke driver.

**Architecture:** A new `relay/internal/llm/providers` package (package `providers`) acts as the selection seam: `providers.New(cfg LLMConfig) (llm.Provider, error)` switches on `cfg.Provider`, resolves secrets, and constructs the right implementation. This keeps the `llm` package import-cycle-free (impls import `llm`; the factory imports both). The factory ships the `"anthropic"` case wired to real construction and stubs `"openai"/"gemini"/"ollama"` with an explicit "not implemented in this build" error that later sub-plans replace. `main.go` loses its direct `anthropic.New(...)` call and gains a single `providers.New(cfg.LLM)`.

**Tech Stack:** Go 1.23.2 (relay), Node 24 / TypeScript 5.6.3 (smoke driver). No new Go dependencies for 2-i. `@intake/core` IntakeClient for the smoke.

---

## Design References

- README §8.3 — config sub-struct shapes (verbatim, frozen here)
- README §8.4 — factory signature + switch cases (verbatim, frozen here)
- README §6 — build-fail checklist (all items must stay green)
- README §7 — final smoke description (5-turn driver pattern)
- `docs/specs/2026-05-27-phase-2-provider-breadth-design.md` §2 (import-cycle rationale), §5 (config defaults)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/config/config.go` | Modify | Add `OpenAIConfig`, `GeminiConfig`, `OllamaConfig` structs; add fields to `LLMConfig`; extend `applyDefaults` |
| `relay/internal/config/config_test.go` | Modify | Add tests for new provider defaults |
| `relay/internal/config/testdata/sample.yaml` | Modify | Add openai/gemini/ollama blocks so the parse test can round-trip them |
| `relay/internal/llm/providers/providers.go` | Create | `providers.New` factory — anthropic case wired, openai/gemini/ollama stubs |
| `relay/internal/llm/providers/providers_test.go` | Create | Unit tests: anthropic OK, bogus error, missing key error, openai not-implemented error |
| `relay/cmd/relay/main.go` | Modify | Replace direct `anthropic.New(...)` + key resolution with `providers.New(cfg.LLM)` |
| `core/smoke/drive-multi.ts` | Create | Provider-agnostic 5-turn smoke driver using `IntakeClient` |

---

## Tasks

### Task 1: Extend `config.LLMConfig` with new provider sub-structs

**Files:**
- Modify: `relay/internal/config/config.go`

- [ ] **Step 1: Write the failing config default tests**

Open `relay/internal/config/config_test.go` and add the following test function. Append it after the existing `TestLoad_MissingFile` function (line 88, after the closing `}`):

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run from the `relay/` directory:
```
cd C:/src/ai/intake/relay && go test ./internal/config/... -run "TestLoad_AppliesOpenAIDefaults|TestLoad_AppliesGeminiDefaults|TestLoad_AppliesOllamaDefaults" -v
```

Expected output: compilation errors like `cfg.LLM.OpenAI undefined` because the struct fields don't exist yet. The tests MUST fail before proceeding.

- [ ] **Step 3: Add the three new config structs and extend `LLMConfig`**

In `relay/internal/config/config.go`, replace the `LLMConfig` struct definition (currently at lines 28–32) with:

```go
// LLMConfig selects the active provider and holds per-provider config.
type LLMConfig struct {
	Provider         string          `yaml:"provider"`
	Anthropic        AnthropicConfig `yaml:"anthropic"`
	OpenAI           OpenAIConfig    `yaml:"openai"`
	Gemini           GeminiConfig    `yaml:"gemini"`
	Ollama           OllamaConfig    `yaml:"ollama"`
	SystemPromptFile string          `yaml:"system_prompt_file"`
}
```

Then add the three new struct types immediately after the existing `AnthropicConfig` block (after line 44 in the original file — after the closing `}` of `AnthropicConfig`):

```go
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
```

- [ ] **Step 4: Extend `applyDefaults` with new provider defaults**

In `relay/internal/config/config.go`, replace the `applyDefaults` function (the full function body, currently lines 79–104) with:

```go
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
	// Anthropic defaults
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
	// Adapter defaults
	if c.Adapters.Webhook.Retry.MaxAttempts == 0 {
		c.Adapters.Webhook.Retry.MaxAttempts = 3
	}
	if c.Adapters.Webhook.Retry.Backoff == "" {
		c.Adapters.Webhook.Retry.Backoff = "exponential"
	}
}
```

- [ ] **Step 5: Run the new tests to verify they pass**

```
cd C:/src/ai/intake/relay && go test ./internal/config/... -v
```

Expected: all tests in `config` pass including the three new `TestLoad_Applies*Defaults` tests. You should see output like:
```
--- PASS: TestLoad_AppliesOpenAIDefaults (0.00s)
--- PASS: TestLoad_AppliesGeminiDefaults (0.00s)
--- PASS: TestLoad_AppliesOllamaDefaults (0.00s)
PASS
ok  	intake/internal/config
```

- [ ] **Step 6: Verify build and vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected:
```
BUILD_OK
VET_OK
```

- [ ] **Step 7: Commit**

```
cd C:/src/ai/intake/relay && git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add OpenAI/Gemini/Ollama sub-structs + defaults to LLMConfig (2-i)"
```

---

### Task 2: Update the sample testdata YAML to include the new provider blocks

**Files:**
- Modify: `relay/internal/config/testdata/sample.yaml`

The `TestLoad_ParsesSampleYAML` test round-trips `testdata/sample.yaml`. We extend the YAML so the parse test also verifies that explicit openai/gemini/ollama values are correctly loaded (not just defaulted). The test code doesn't need changing yet — we update the YAML and add assertions in step 4.

- [ ] **Step 1: Extend `testdata/sample.yaml` with provider blocks**

Replace the content of `relay/internal/config/testdata/sample.yaml` with:

```yaml
server:
  addr: ":9000"
  external_url: "http://example.com"
  cors_origins:
    - "http://localhost:5173"
    - "http://localhost:3000"
llm:
  provider: "anthropic"
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-sonnet-4-6"
    max_tokens: 2048
  openai:
    api_key_env: "OPENAI_API_KEY"
    model: "gpt-4o-mini"
    max_tokens: 512
  gemini:
    api_key_env: "GEMINI_API_KEY"
    model: "gemini-2.0-flash"
    max_tokens: 512
  ollama:
    base_url: "http://localhost:11434"
    model: "llama3.1"
    bearer_token_env: ""
    max_tokens: 512
  system_prompt_file: ""
auth:
  modes:
    anonymous: true
adapters:
  webhook:
    enabled: true
    url: "http://localhost:9099/intake"
    headers:
      X-Custom: "value"
    retry:
      max_attempts: 5
      backoff: "fixed"
```

- [ ] **Step 2: Add assertions for new provider fields in `TestLoad_ParsesSampleYAML`**

In `relay/internal/config/config_test.go`, inside the existing `TestLoad_ParsesSampleYAML` function (append before the closing `}`), add:

```go
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
```

- [ ] **Step 3: Run all config tests**

```
cd C:/src/ai/intake/relay && go test ./internal/config/... -v
```

Expected: all config tests pass. The `TestLoad_ParsesSampleYAML` test now also verifies the new provider fields. All tests green.

- [ ] **Step 4: Commit**

```
cd C:/src/ai/intake/relay && git add internal/config/config_test.go internal/config/testdata/sample.yaml
git commit -m "test(config): extend ParsesSampleYAML to round-trip openai/gemini/ollama fields (2-i)"
```

---

### Task 3: Create the `providers` factory package — failing test first

**Files:**
- Create: `relay/internal/llm/providers/providers_test.go`
- Create: `relay/internal/llm/providers/providers.go`

- [ ] **Step 1: Create the test file**

Create the directory and file `relay/internal/llm/providers/providers_test.go` with the following content. (On Windows: `mkdir relay\internal\llm\providers` from the relay directory if needed — the `go` toolchain creates package dirs on first compile, but the file must exist somewhere.)

```go
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
```

- [ ] **Step 2: Run to verify the tests fail (package does not exist)**

```
cd C:/src/ai/intake/relay && go test ./internal/llm/providers/... -v
```

Expected: build error like `no required module provides package intake/internal/llm/providers`. Tests MUST fail here.

- [ ] **Step 3: Create the `providers.go` implementation**

Create `relay/internal/llm/providers/providers.go` with the following content:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

```
cd C:/src/ai/intake/relay && go test ./internal/llm/providers/... -v
```

Expected output:
```
--- PASS: TestNew_Anthropic_WithKey (0.00s)
--- PASS: TestNew_Anthropic_MissingKey (0.00s)
--- PASS: TestNew_UnknownProvider (0.00s)
--- PASS: TestNew_OpenAI_NotImplemented (0.00s)
--- PASS: TestNew_Gemini_NotImplemented (0.00s)
--- PASS: TestNew_Ollama_NotImplemented (0.00s)
PASS
ok  	intake/internal/llm/providers
```

- [ ] **Step 5: Run full test suite + build**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected:
```
BUILD_OK
VET_OK
ok  	intake/internal/config
ok  	intake/internal/llm/anthropic
ok  	intake/internal/llm/providers
[... other packages ...]
TEST_OK
```

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/llm/providers/providers.go internal/llm/providers/providers_test.go
git commit -m "feat(providers): add factory package with anthropic case + provider stubs (2-i)"
```

---

### Task 4: Rewire `main.go` to use `providers.New`

**Files:**
- Modify: `relay/cmd/relay/main.go`

Currently `main.go` directly resolves the Anthropic API key and calls `anthropic.New(...)`. After this task it calls `providers.New(cfg.LLM)` instead. The `deps.Model` and `deps.MaxTokens` fields (used by the classifier in `server.Deps`) also change source: they now come from the provider interface's name routing rather than being hard-coded to the Anthropic config. Since `classify.New` requires a model name and max_tokens and those values are still provider-config-derived, we use a helper that reads from whichever provider config block is active.

Note on `deps.Model` and `deps.MaxTokens`: these fields in `server.Deps` are used by the classify handler to pass model/maxTokens to `classify.New`. In Phase 2, only one provider is active per relay; we keep these populated from the active provider's config. The cleanest approach in `main.go` is a simple switch that mirrors the factory switch to extract model and maxTokens from the right sub-config.

- [ ] **Step 1: Write a compile-time guard first (no test file needed)**

Before editing `main.go`, do a dry-run build to confirm the current state compiles:

```
cd C:/src/ai/intake/relay && go build ./cmd/relay/... && echo PRE_OK
```

Expected: `PRE_OK` — confirms we have a clean baseline.

- [ ] **Step 2: Replace the direct provider construction in `main.go`**

Replace the entire content of `relay/cmd/relay/main.go` with:

```go
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/config"
	"intake/internal/llm/providers"
	"intake/internal/payloadbuild"
	"intake/internal/server"
	"intake/internal/triage"
	"intake/internal/version"
	"intake/internal/adapter/webhook"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the relay config file")
	flag.Parse()

	// --- Logger (structured JSON to stdout) ---
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// --- Config ---
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("relay: config load failed", "error", err)
		os.Exit(1)
	}

	// --- LLM Provider (via factory) ---
	// providers.New resolves the required secret internally via config.RequireSecret /
	// config.ResolveSecret. The key is NEVER logged or embedded in any error surfaced here.
	provider, err := providers.New(cfg.LLM)
	if err != nil {
		logger.Error("relay: LLM provider init failed",
			"provider", cfg.LLM.Provider,
			"error", err,
		)
		os.Exit(1)
	}
	logger.Info("relay: LLM provider ready", "provider", provider.Name())

	// --- Model / MaxTokens for classify (derived from the active provider config) ---
	// The classify.New call still needs a model name and max_tokens. We read them
	// from whichever sub-config block corresponds to the active provider.
	// This mirrors the factory switch without constructing a second provider.
	model, maxTokens := activeModelConfig(cfg.LLM)

	// --- Session Store + Auth Middleware ---
	store := auth.NewStore()
	middleware := auth.NewMiddleware(store)

	// --- Triage System Prompt ---
	// Loads from cfg.LLM.SystemPromptFile if set; else uses bundled prompt.txt.
	systemPrompt, err := triage.Load(cfg.LLM.SystemPromptFile)
	if err != nil {
		logger.Error("failed to load system prompt", "error", err)
		os.Exit(1)
	}

	// --- Webhook Adapter (1-iv) ---
	wh := webhook.New()
	whCfg := map[string]any{
		"url":     cfg.Adapters.Webhook.URL,
		"headers": cfg.Adapters.Webhook.Headers,
		"retry": map[string]any{
			"max_attempts": cfg.Adapters.Webhook.Retry.MaxAttempts,
			"backoff":      cfg.Adapters.Webhook.Retry.Backoff,
		},
	}
	if err := wh.Configure(whCfg); err != nil {
		logger.Error("webhook adapter: configure failed", "error", err)
		os.Exit(1)
	}

	// --- Classifier (1-iv) — reuses the same provider as /turn ---
	classifier := classify.New(provider, model, maxTokens)

	// --- Payload Builder (1-iv) ---
	builder := payloadbuild.New("0.1.0") // widget version default; Phase 5 may read from config

	// --- Deps ---
	// Deps is a value type (README §6.8). No Config field — config-derived values
	// are promoted to individual Deps fields. main.go populates these from cfg.
	deps := server.Deps{
		Version:      version.Info(),
		CORSOrigins:  cfg.Server.CORSOrigins,
		Logger:       logger,
		Auth:         middleware,
		Provider:     provider,
		SystemPrompt: systemPrompt,
		Model:        model,
		MaxTokens:    maxTokens,
		Adapter:      wh,
		Classifier:   classifier,
		Builder:      builder,
	}

	// --- HTTP Server ---
	handler := server.New(cfg, deps)
	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally 0: the /turn SSE handler streams for the
		// duration of an LLM response; a write deadline would truncate it.
		// Revisit per-route write deadlines when SSE lands.
	}

	// Start the server in a goroutine so the main goroutine can wait for the
	// shutdown signal.
	idleConnsClosed := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("relay: shutdown signal received; draining connections...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("relay: graceful shutdown error", "error", err)
		}
		close(idleConnsClosed)
	}()

	logger.Info("relay listening", "addr", cfg.Server.Addr, "external", cfg.Server.ExternalURL)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Error("relay: listen error", "error", err)
		os.Exit(1)
	}

	<-idleConnsClosed
	logger.Info("relay stopped")
}

// activeModelConfig returns the model name and maxTokens for the currently
// configured provider. Used to populate server.Deps.Model and .MaxTokens for
// the classifier — which mirrors the factory's provider selection without
// constructing a second provider instance.
func activeModelConfig(cfg config.LLMConfig) (model string, maxTokens int) {
	switch cfg.Provider {
	case "openai":
		return cfg.OpenAI.Model, cfg.OpenAI.MaxTokens
	case "gemini":
		return cfg.Gemini.Model, cfg.Gemini.MaxTokens
	case "ollama":
		return cfg.Ollama.Model, cfg.Ollama.MaxTokens
	default:
		// anthropic (default) and any future unknown provider: fall back to
		// the anthropic block, which is the Phase-1 default.
		return cfg.Anthropic.Model, cfg.Anthropic.MaxTokens
	}
}
```

- [ ] **Step 3: Verify the build compiles and no `anthropic` import remains**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected:
```
BUILD_OK
VET_OK
```

If you get an `anthropic imported and not used` error the old import was not removed — verify the full file matches the code above.

- [ ] **Step 4: Run the full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK`. All packages pass including `providers`, `config`, `llm/anthropic`.

- [ ] **Step 5: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "refactor(main): replace direct anthropic.New with providers.New factory (2-i)"
```

---

### Task 5: Create `core/smoke/drive-multi.ts` — provider-agnostic 5-turn driver

**Files:**
- Create: `core/smoke/drive-multi.ts`

This smoke script drives 5 sequential turns through the relay using `IntakeClient`. It is provider-agnostic: it hits only `init` + `turn` (NOT `submit`), so it needs no browser-global stubs (those are only required because `submit` calls `captureClient`/`capturePageMetadata` which read `window`/`navigator`/`document`). Each turn accumulates the conversation history by appending the user message and a synthesized assistant message built from the streamed deltas. The script exits non-zero on any error.

**Assumption about assistant history synthesis:** After each `turn()` call, all streamed delta strings are collected via the `onDelta` callback. The concatenated deltas form the assistant's response text. This is appended to the `messages` array as `{ role: 'assistant', content: <accumulated text> }` before the next turn. This matches how a real widget accumulates streaming text — the relay's `/turn` endpoint expects the full conversation history on each call. See `client.ts` `turn()` method: the `onDelta` callback is called per delta; we collect all of them.

- [ ] **Step 1: Write the smoke script**

Create `core/smoke/drive-multi.ts` with the following content:

```typescript
/**
 * Provider-agnostic 5-turn smoke driver for @intake/core.
 *
 * Drives init() then 5 sequential turn() calls, accumulating the conversation
 * history across turns (user message + synthesized assistant response from
 * streamed deltas). Prints streamed deltas and per-turn token counts. Exits
 * non-zero on any failure.
 *
 * Works against whatever provider the relay is configured with — the Phase-2
 * final smoke points the relay at each provider in turn.
 *
 * Does NOT call submit(), so no browser-global stubs (window/navigator/document)
 * are required. init() and turn() are SSR-safe in @intake/core.
 *
 * Usage:
 *   RELAY_URL=http://localhost:8080 npx tsx core/smoke/drive-multi.ts
 *
 * Prerequisites:
 *   - Relay running with a configured provider (any of anthropic/openai/gemini/ollama)
 *   - Provider's secret exported in the relay's environment
 */

import { IntakeClient } from '../src/index.js';
import type { ChatMessage } from '../src/index.js';

const RELAY_URL = process.env['RELAY_URL'] ?? 'http://localhost:8080';
const WIDGET_VERSION = '0.1.0-multi-smoke';

// The 5 user turns to send. Each builds naturally on the previous so the
// conversation is coherent and the provider must attend to history.
const USER_TURNS: string[] = [
  'I found a bug: clicking the Save button twice submits the form twice. ' +
    'This is reproducible on Chrome 124 on macOS.',
  'It also happens on Firefox 125 on Windows. ' +
    'The button does not disable itself after the first click.',
  'Looking at the network tab, I see two identical POST /api/items requests ' +
    'with the same payload within milliseconds of each other.',
  'Could this be a debounce issue? The Save button has no debounce logic ' +
    'in our current implementation.',
  'Please summarise the issue and suggest a fix in plain English.',
];

async function main(): Promise<void> {
  console.log(`[drive-multi] connecting to relay at ${RELAY_URL}`);

  const client = new IntakeClient({
    relayUrl: RELAY_URL,
    widgetVersion: WIDGET_VERSION,
    appContext: { smoke: true, driver: 'drive-multi' },
  });

  // 1. Init — establishes the session.
  console.log('[drive-multi] POST /v1/intake/init ...');
  const initResult = await client.init();
  console.log(`[drive-multi] session_id: ${initResult.session_id}`);

  // Conversation history accumulates across all turns.
  // Format: alternating user/assistant ChatMessage entries.
  // The relay expects the FULL history on each /turn call.
  const history: ChatMessage[] = [];

  // 2. Drive 5 turns.
  for (let i = 0; i < USER_TURNS.length; i++) {
    const userContent = USER_TURNS[i];
    console.log(`\n[drive-multi] --- Turn ${i + 1} / ${USER_TURNS.length} ---`);
    console.log(`[user] ${userContent}`);

    // Append the user message before calling turn().
    history.push({ role: 'user', content: userContent });

    // Collect all delta strings so we can synthesize the assistant message.
    const deltaChunks: string[] = [];

    process.stdout.write('[assistant] ');
    const tokenCounts = await client.turn(history, (delta: string) => {
      process.stdout.write(delta);
      deltaChunks.push(delta);
    });
    process.stdout.write('\n');

    // Validate that we received meaningful usage data.
    if (tokenCounts.input_tokens <= 0) {
      throw new Error(
        `Turn ${i + 1}: expected input_tokens > 0, got ${tokenCounts.input_tokens}`
      );
    }
    if (tokenCounts.output_tokens <= 0) {
      throw new Error(
        `Turn ${i + 1}: expected output_tokens > 0, got ${tokenCounts.output_tokens}`
      );
    }

    console.log(
      `[drive-multi] turn ${i + 1} complete: ` +
        `input_tokens=${tokenCounts.input_tokens} ` +
        `output_tokens=${tokenCounts.output_tokens}`
    );

    // Synthesize the assistant message from accumulated deltas and append to history.
    // This is how a real widget reconstructs the assistant's full response from a stream.
    const assistantContent = deltaChunks.join('');
    if (assistantContent.length === 0) {
      throw new Error(`Turn ${i + 1}: received zero delta text from assistant`);
    }
    history.push({ role: 'assistant', content: assistantContent });
  }

  console.log('\n[drive-multi] PASS — 5 turns completed successfully');
}

main().catch((err: unknown) => {
  console.error('[drive-multi] FAIL:', err);
  process.exit(1);
});
```

- [ ] **Step 2: Run the TypeScript type-check for `@intake/core`**

The `core/tsconfig.json` already includes `"smoke/**/*.ts"` in its `include` array, so `drive-multi.ts` is covered without any tsconfig changes.

```
cd C:/src/ai/intake && npm run -w @intake/core type-check
```

Expected: exits 0 with no errors. If you see type errors:
- `ChatMessage` from `'../src/index.js'` must match what `client.turn()` accepts — confirmed from `client-types.ts`: `role: 'user' | 'assistant'`, which the `history` array uses correctly.
- `IntakeClient` constructor and `init()`/`turn()` signatures match `client.ts` exactly.

- [ ] **Step 3: Verify the contract gate still passes**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK
```

Expected: `CONTRACT_OK`. The smoke driver does not touch any schema or contract files.

- [ ] **Step 4: Commit**

```
cd C:/src/ai/intake && git add core/smoke/drive-multi.ts
git commit -m "feat(smoke): add provider-agnostic 5-turn drive-multi.ts smoke driver (2-i)"
```

---

### Task 6: Final verification gate

This task runs the complete credit-free verification suite described in the sub-plan scope and confirms all done criteria are met.

- [ ] **Step 1: Run the full Go build + vet + test suite**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected — paste actual output, it must contain:
```
BUILD_OK
VET_OK
ok  	intake/internal/config
ok  	intake/internal/llm/anthropic
ok  	intake/internal/llm/providers
TEST_OK
```

Every `go test` line must be `ok` — no `FAIL`.

- [ ] **Step 2: Run the TypeScript type-check**

```
cd C:/src/ai/intake && npm run -w @intake/core type-check && echo TYPECHECK_OK
```

Expected: `TYPECHECK_OK`.

- [ ] **Step 3: Run the contract gate**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK
```

Expected: `CONTRACT_OK`.

- [ ] **Step 4: Confirm the factory test passes with NO real keys**

Verify `providers_test.go` tests run without any `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` / `GEMINI_API_KEY` set in the environment:

```
cd C:/src/ai/intake/relay && go test ./internal/llm/providers/... -v -count=1
```

Expected: all 6 provider tests PASS. The `TestNew_Anthropic_WithKey` test uses `t.Setenv("TEST_ANTHROPIC_KEY", "sk-ant-fake-key-for-unit-test")` — a fake key that never reaches any API because `anthropic.New()` doesn't make a network call during construction.

- [ ] **Step 5: Build-fail checklist self-check**

Verify each item from README §6 is green:
- `go build ./...` passes — confirmed in step 1.
- `go test ./...` passes — confirmed in step 1.
- `verify-contract.sh` passes — confirmed in step 3.
- No API key appears in logs — `providers.New` logs only `provider.Name()`, not the key value; `main.go` logs `"provider"` name and the error string (which `config.RequireSecret` guarantees never contains the key value).
- No caret/`@latest` pins introduced — 2-i adds no new Go dependencies; no `go.mod` changes.
- `providers.New` returns nil provider + non-nil error for unknown provider — confirmed by `TestNew_UnknownProvider`.
- `providers.New` never returns non-nil provider + non-nil error — all factory paths either return `(provider, nil)` or `(nil, error)`.

---

## Smoke

The sub-plan smoke is **credit-free** at the unit level (the factory test + clean build). The live relay re-confirmation (relay boots + serves a turn with `provider: anthropic` via the factory) is deferred to the Phase-2 final smoke, which requires a real `ANTHROPIC_API_KEY`.

When you do run the live smoke (after all tasks above complete), the command is:

```bash
# Terminal 1: start relay (requires ANTHROPIC_API_KEY in env)
cd C:/src/ai/intake/relay
ANTHROPIC_API_KEY=<real-key> go run ./cmd/relay -config config.yaml

# Terminal 2: run the 5-turn multi-turn driver
cd C:/src/ai/intake
RELAY_URL=http://localhost:8080 npx tsx core/smoke/drive-multi.ts
```

Expected live output (abbreviated):
```
[drive-multi] connecting to relay at http://localhost:8080
[drive-multi] POST /v1/intake/init ...
[drive-multi] session_id: <uuid>

[drive-multi] --- Turn 1 / 5 ---
[user] I found a bug: clicking the Save button twice ...
[assistant] <streaming tokens ...>
[drive-multi] turn 1 complete: input_tokens=NNN output_tokens=MMM
...
[drive-multi] --- Turn 5 / 5 ---
...
[drive-multi] PASS — 5 turns completed successfully
```

---

## Done Criteria

Sub-plan 2-i is complete when ALL of the following are true:

1. **Build gate:** `cd C:/src/ai/intake/relay && go build ./... && go vet ./...` exits 0.
2. **Unit gate:** `go test ./...` exits 0 with no FAIL lines; this includes:
   - `intake/internal/config` — new defaults tests pass.
   - `intake/internal/llm/providers` — all 6 factory tests pass with NO real API keys.
   - All pre-existing tests continue to pass.
3. **TypeScript gate:** `npm run -w @intake/core type-check` exits 0 (covers `drive-multi.ts`).
4. **Contract gate:** `bash scripts/verify-contract.sh` exits 0.
5. **No API key in logs:** The `providers.New` error path and the `main.go` startup log never include a secret value — verified by reading the code (`RequireSecret` error messages contain only env var names).
6. **Factory invariant:** `providers.New` returns `(nil, non-nil error)` for unknown providers and missing required secrets; returns `(non-nil, nil)` for a valid anthropic config + fake key — confirmed by tests.
7. **Smoke driver exists and type-checks:** `core/smoke/drive-multi.ts` is present and covered by `core/tsconfig.json`'s `include: ["smoke/**/*.ts"]`.

---

## Environment Notes

- **OS:** Windows 10, but all shell commands in this plan use bash syntax (available via Git Bash / the Bash tool). PowerShell equivalents: `cd C:\...` with backslashes, but the commands shown use forward slashes which work in Git Bash.
- **Go:** 1.23.2 (toolchain set in `relay/go.mod`). Run `go version` to confirm.
- **Node:** 24.12. Run `node --version` to confirm.
- **No git remote:** commits are local only — `git push` is not required.
- **Module path:** `intake` (set in `relay/go.mod` — do NOT change).
- **Workspace root:** `C:/src/ai/intake`. The relay module lives at `C:/src/ai/intake/relay` — all `go` commands run from there.
- **No new Go dependencies in 2-i:** `go.mod` and `go.sum` should be unchanged after all tasks. If `go mod tidy` adds anything, investigate — it should not.
