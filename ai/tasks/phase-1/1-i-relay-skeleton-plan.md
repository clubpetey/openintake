# Sub-plan 1-i — Relay Server Skeleton

> **Status:** Ready to implement
> **Phase:** 1 — Walking Skeleton
> **Effort:** M
> **Depends on:** Phase 0 (module `intake`, `relay/internal/payload` generated types)
> **Unblocks:** 1-ii (LLM Provider + Anthropic)

---

## 1. Goal

Deliver the relay server skeleton: dependency wiring (chi router, YAML config), HTTP middleware chain (request-ID, panic recovery, strict CORS allowlist), the `/v1/health` and `/v1/version` endpoints, all shared HTTP DTOs and error helpers used by every sub-plan, and the empty `registerIntakeRoutes` seam that 1-iii and 1-iv extend. When this sub-plan is done, the relay boots from a config file, serves health/version, and `go test ./...` is green. No provider, no auth, no adapter logic.

---

## 2. Design references

- `ai/tasks/phase-1/README.md` §6.4 (HTTP DTOs — use verbatim), §6.6 (Config structs — use verbatim), §5 (tool pins), §7 (build-fail checklist)
- `docs/specs/2026-05-26-phase-1-walking-skeleton-design.md` §3 (architecture), §7 (config YAML), §9 (error envelope, CORS)
- `relay/go.mod` — module `intake`, go 1.23, toolchain go1.23.2
- `relay/cmd/relay/main.go` — Phase 0 stub, replaced in Task 7

---

## 3. Files touched

| File | Action | Why |
|---|---|---|
| `relay/go.mod` | modified | Add chi v5, gopkg.in/yaml.v3 |
| `relay/go.sum` | modified | Updated by `go get` |
| `relay/cmd/relay/main.go` | replaced | Wire config.Load → server.New → ListenAndServe + graceful shutdown |
| `relay/internal/config/config.go` | created | Config structs (README §6.6) + Load(path) with defaults + env-override note |
| `relay/internal/config/config_test.go` | created | Tests: parse sample YAML, defaults applied, missing-file error |
| `relay/internal/config/testdata/sample.yaml` | created | Sample config used by the config test |
| `relay/internal/version/version.go` | created | BuildInfo{Version,Commit,BuildTime}; settable via -ldflags |
| `relay/internal/server/deps.go` | created | type Deps struct — 1-i fields only; comment guides 1-iii/1-iv |
| `relay/internal/server/dto.go` | created | ALL DTO structs from README §6.4, verbatim |
| `relay/internal/server/errors.go` | created | writeJSON(w, status, v) + writeError(w, status, code, msg) |
| `relay/internal/server/server.go` | created | func New(cfg, deps) http.Handler — chi mux, middleware, health/version routes |
| `relay/internal/server/routes.go` | created | registerIntakeRoutes — empty seam with comment |
| `relay/internal/server/server_test.go` | created | httptest: health 200, version JSON, CORS allow/deny |
| `ai/tasks/phase-1/README.md` | modified | Update §5 tool pin table with resolved chi + yaml versions |

---

## 4. Tasks

Each task follows the TDD loop: write failing test → confirm it fails → implement → confirm it passes → commit. Commands run from `relay/` unless stated otherwise. All code shown is complete and compilable; no placeholders.

---

### Task 1 — Add chi and yaml dependencies; record exact versions; update README §5

**Step 1.1 — Install dependencies.**

```bash
cd C:/src/ai/intake/relay
go get github.com/go-chi/chi/v5@latest
go get gopkg.in/yaml.v3@latest
```

Record the resolved versions from the output (lines like `go: added github.com/go-chi/chi/v5 v5.X.Y`). The plan uses `v5.1.0` and `v3.0.1` as placeholders; substitute the actual resolved versions everywhere they appear.

**Step 1.2 — Verify the module builds.**

```bash
cd C:/src/ai/intake/relay
go build ./...
```

Expected: exits 0. (The Phase 0 stub still compiles; no new packages exist yet.)

**Step 1.3 — Update `ai/tasks/phase-1/README.md` §5.**

Edit the pin table to fill in the exact resolved versions for `github.com/go-chi/chi/v5` and `gopkg.in/yaml.v3`. Do this in the same commit as Task 1.

**Step 1.4 — Pin-stability note (no check-pins.sh change needed).**

chi and gopkg.in/yaml.v3 are pure libraries with strict semver and no deploy-time artifact generation. PHASE_PLANNING §5 permits caret for such libraries; no extension of `scripts/check-pins.sh` is required. Document this decision in a comment in `go.mod` is optional but not required — the pin-table entry is the authoritative record.

**Step 1.5 — Commit.**

```bash
cd C:/src/ai/intake/relay
git add go.mod go.sum ../ai/tasks/phase-1/README.md
git commit -m "1-i: add chi/v5 and yaml.v3 dependencies (Task 1)"
```

---

### Task 2 — `config` package: structs, Load(), and tests

**Step 2.1 — Create testdata sample config.**

Create `relay/internal/config/testdata/sample.yaml`:

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

**Step 2.2 — Write the failing test first.**

Create `relay/internal/config/config_test.go`:

```go
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
```

**Step 2.3 — Create `testdata/minimal.yaml`.**

Create `relay/internal/config/testdata/minimal.yaml`:

```yaml
server: {}
```

**Step 2.4 — Run the test to confirm it fails (package does not exist yet).**

```bash
cd C:/src/ai/intake/relay
go test ./internal/config/...
```

Expected: compile error — `package intake/internal/config: directory not found`.

**Step 2.5 — Implement `relay/internal/config/config.go`.**

```go
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
	MaxTokens int    `yaml:"max_tokens"`
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

// defaults applies sane default values for any field not set by the YAML file.
// Called before unmarshalling so that explicit zeros in the file override defaults
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
```

**Step 2.6 — Run the test to confirm it passes.**

```bash
cd C:/src/ai/intake/relay
go test ./internal/config/...
```

Expected output (approximately):

```
ok  	intake/internal/config	0.XXXs
```

All three test functions must pass.

**Step 2.7 — Run build-fail checklist items that apply so far.**

```bash
cd C:/src/ai/intake/relay
go build ./...
go vet ./...
```

Expected: both exit 0.

**Step 2.8 — Commit.**

```bash
cd C:/src/ai/intake/relay
git add internal/config/ cmd/
git commit -m "1-i: config package with Load(), defaults, and tests (Task 2)"
```

---

### Task 3 — `version` package and BuildInfo

**Step 3.1 — Write the failing test first.**

Create `relay/internal/version/version_test.go`:

```go
package version_test

import (
	"testing"

	"intake/internal/version"
)

func TestBuildInfo_Defaults(t *testing.T) {
	info := version.Info()
	if info.Version == "" {
		t.Error("Version is empty; want non-empty default")
	}
	if info.Commit == "" {
		t.Error("Commit is empty; want non-empty default")
	}
	if info.BuildTime == "" {
		t.Error("BuildTime is empty; want non-empty default")
	}
	// When not overridden via ldflags the defaults are "dev", "none", "unknown".
	if info.Version != "dev" {
		t.Errorf("default Version = %q; want %q", info.Version, "dev")
	}
}
```

**Step 3.2 — Run the test to confirm it fails (package does not exist).**

```bash
cd C:/src/ai/intake/relay
go test ./internal/version/...
```

Expected: compile error — directory not found.

**Step 3.3 — Implement `relay/internal/version/version.go`.**

```go
package version

// Build-time variables. Override with:
//
//	go build -ldflags "-X intake/internal/version.version=v1.2.3 \
//	  -X intake/internal/version.commit=abc1234 \
//	  -X intake/internal/version.buildTime=2026-01-01T00:00:00Z" ./cmd/relay
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

// BuildInfo carries the binary's identity, populated at link time via -ldflags.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// Info returns the build information for this binary.
func Info() BuildInfo {
	return BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildTime: buildTime,
	}
}
```

**Step 3.4 — Run the test to confirm it passes.**

```bash
cd C:/src/ai/intake/relay
go test ./internal/version/...
```

Expected: `ok  intake/internal/version  0.XXXs`.

**Step 3.5 — Commit.**

```bash
cd C:/src/ai/intake/relay
git add internal/version/
git commit -m "1-i: version package with BuildInfo and ldflags defaults (Task 3)"
```

---

### Task 4 — `server/dto.go` and `server/errors.go`

These two files contain no logic except JSON serialization helpers. They compile-only until the handlers use them; a focused test verifies the error envelope shape.

**Step 4.1 — Write the failing test first.**

Create `relay/internal/server/server_test.go` (initial skeleton — later tasks add more tests):

```go
package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"intake/internal/server"
)

// ---- helpers ----

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("JSON decode failed: %v\nbody: %s", err, body)
	}
}

// ---- Task 4: writeError shape ----

func TestWriteError_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	server.WriteErrorExported(w, http.StatusBadRequest, "bad_request", "something is wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", w.Code, http.StatusBadRequest)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q; want %q", ct, "application/json")
	}

	var env server.ErrorEnvelope
	decodeJSON(t, w.Body.Bytes(), &env)

	if env.Error.Code != "bad_request" {
		t.Errorf("error.code = %q; want %q", env.Error.Code, "bad_request")
	}
	if env.Error.Message != "something is wrong" {
		t.Errorf("error.message = %q; want %q", env.Error.Message, "something is wrong")
	}
}
```

> **Note:** The test calls `server.WriteErrorExported` — a thin exported wrapper we add to `errors.go` so the test can exercise the unexported `writeError` from outside the package. The production code (handlers) uses the unexported form.

**Step 4.2 — Run the test to confirm it fails (package does not exist).**

```bash
cd C:/src/ai/intake/relay
go test ./internal/server/...
```

Expected: compile error — directory not found.

**Step 4.3 — Implement `relay/internal/server/dto.go`.**

Verbatim from README §6.4:

```go
package server

// Session transport: the X-Intake-Session header carries the session_id on
// every /turn and /submit request (single source of truth; NOT in the body).

// InitResponse is the body returned by POST /v1/intake/init.
type InitResponse struct {
	SessionID    string       `json:"session_id"`
	Capabilities Capabilities `json:"capabilities"`
}

// Capabilities advertises relay feature flags to the widget.
type Capabilities struct {
	AuthModes []string `json:"auth_modes"` // Phase 1: ["anonymous"]
	Streaming bool     `json:"streaming"`  // true
}

// TurnMessage is a single conversation turn (user or assistant).
type TurnMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

// TurnRequest is the body of POST /v1/intake/turn.
type TurnRequest struct {
	Messages []TurnMessage `json:"messages"`
}

// SSEDelta is an SSE frame carrying a streaming token delta.
type SSEDelta struct {
	Delta string `json:"delta"`
}

// SSEDone is the terminal SSE frame for a successful turn.
type SSEDone struct {
	Done         bool `json:"done"`
	InputTokens  int  `json:"input_tokens"`
	OutputTokens int  `json:"output_tokens"`
}

// SSEError is the terminal SSE frame for a failed turn.
type SSEError struct {
	Error string `json:"error"`
}

// ClientInfo captures browser context sent with each submit.
type ClientInfo struct {
	WidgetVersion string   `json:"widget_version"`
	URL           string   `json:"url"`
	Referrer      *string  `json:"referrer"`
	UserAgent     string   `json:"user_agent"`
	Viewport      Viewport `json:"viewport"`
	Locale        string   `json:"locale"`
}

// Viewport captures the browser window size in CSS pixels.
type Viewport struct {
	W int `json:"w"`
	H int `json:"h"`
}

// ContextInfo carries host-app-supplied metadata attached to each submit.
type ContextInfo struct {
	AppContext   map[string]any `json:"app_context"`
	PageMetadata map[string]any `json:"page_metadata"`
}

// SubmitRequest is the body of POST /v1/intake/submit.
// Attachments are deferred to Phase 6.
type SubmitRequest struct {
	Messages    []TurnMessage  `json:"messages"`
	Client      ClientInfo     `json:"client"`
	UserClaims  map[string]any `json:"user_claims"`
	Context     ContextInfo    `json:"context"`
	RoutingHint *string        `json:"routing_hint"`
}

// SubmitResponse is the body returned by POST /v1/intake/submit on success.
type SubmitResponse struct {
	ExternalID  string `json:"external_id"`
	ExternalURL string `json:"external_url"`
	AdapterName string `json:"adapter_name"`
	CreatedAt   string `json:"created_at"`
}

// ErrorEnvelope is the standard error response body for all relay endpoints.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody holds the machine-readable code and human-readable message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
```

**Step 4.4 — Implement `relay/internal/server/errors.go`.**

```go
package server

import (
	"encoding/json"
	"net/http"
)

// writeJSON serialises v as JSON and writes it to w with the given HTTP status.
// On marshal failure it falls back to a plain-text 500.
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":{"code":"internal","message":"response encoding failed"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// writeError writes an ErrorEnvelope JSON response.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, ErrorEnvelope{
		Error: ErrorBody{Code: code, Message: msg},
	})
}

// WriteErrorExported is a thin exported wrapper so package-external tests can
// exercise writeError without needing to go through a full HTTP handler. It is
// NOT part of the production API surface; production code calls writeError.
func WriteErrorExported(w http.ResponseWriter, status int, code, msg string) {
	writeError(w, status, code, msg)
}
```

**Step 4.5 — Run the test to confirm it passes.**

```bash
cd C:/src/ai/intake/relay
go test ./internal/server/... -run TestWriteError_EnvelopeShape -v
```

Expected:

```
=== RUN   TestWriteError_EnvelopeShape
--- PASS: TestWriteError_EnvelopeShape (0.00s)
PASS
ok  	intake/internal/server	0.XXXs
```

**Step 4.6 — Commit.**

```bash
cd C:/src/ai/intake/relay
git add internal/server/
git commit -m "1-i: server dto.go, errors.go, and writeError test (Task 4)"
```

---

### Task 5 — `server/deps.go`, `server/server.go`, `server/routes.go`

**Step 5.1 — Create `relay/internal/server/deps.go`.**

```go
package server

import "intake/internal/version"

// Deps holds the dependencies injected into the HTTP server at startup.
//
// 1-i owns: Version, CORSOrigins.
// Extended by 1-iii: Auth (session middleware), Provider (llm.Provider),
//   SystemPrompt (string), Model (string), MaxTokens (int).
// Extended by 1-iv: Adapter (adapter.Adapter), Classifier, Builder.
//
// Later sub-plans assign their fields after constructing the relay server.
// The struct is defined here in full so all sub-plans share one type without
// circular imports — packages that are not yet created are NOT imported here;
// their fields will be added to this struct by those sub-plans as interface{}
// or concrete types once the packages exist.
type Deps struct {
	// Version is populated from the binary's build-time ldflags.
	Version version.BuildInfo

	// CORSOrigins is the strict allowlist of origins that may make cross-origin
	// requests. Populated from cfg.Server.CORSOrigins in main.go.
	CORSOrigins []string

	// extended by 1-iii (Auth, Provider, SystemPrompt, Model, MaxTokens)
	// and 1-iv (Adapter, Classifier, Builder)
}
```

**Step 5.2 — Create `relay/internal/server/routes.go`.**

```go
package server

import "github.com/go-chi/chi/v5"

// registerIntakeRoutes mounts the /v1/intake/* handlers on r.
//
// This body is intentionally empty in 1-i. Sub-plans extend it:
//   - 1-iii registers POST /v1/intake/init and POST /v1/intake/turn (SSE)
//   - 1-iv registers POST /v1/intake/submit
func registerIntakeRoutes(r chi.Router, deps Deps) {
	// /v1/intake/* routes registered by sub-plans 1-iii (init, turn) and 1-iv (submit)
	_ = r
	_ = deps
}
```

**Step 5.3 — Create `relay/internal/server/server.go`.**

```go
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"intake/internal/config"
)

// New constructs the relay HTTP handler (a chi Mux) with all middleware and
// built-in routes wired. Routes specific to intake sessions are registered via
// registerIntakeRoutes, which 1-iii and 1-iv extend.
func New(cfg *config.Config, deps Deps) http.Handler {
	r := chi.NewMux()

	// Global middleware — order matters: request-ID first, then recovery.
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(deps.CORSOrigins))

	// Built-in relay endpoints.
	r.Get("/v1/health", handleHealth)
	r.Get("/v1/version", handleVersion(deps))

	// Intake session endpoints — seam for 1-iii and 1-iv.
	r.Route("/v1/intake", func(r chi.Router) {
		registerIntakeRoutes(r, deps)
	})

	return r
}

// handleHealth returns a minimal liveness probe response.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleVersion returns build info as JSON.
func handleVersion(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, deps.Version)
	}
}

// corsMiddleware returns a middleware that enforces a strict CORS allowlist.
// It sets Access-Control-Allow-Origin ONLY for origins that appear in the list.
// No wildcard is ever set. Preflight OPTIONS requests are handled directly.
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.ToLower(o)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := allowed[strings.ToLower(origin)]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Headers",
						"Content-Type, X-Intake-Session, Authorization, X-Request-Id")
					w.Header().Set("Access-Control-Allow-Methods",
						"GET, POST, OPTIONS")
				}
			}

			// Handle preflight.
			if r.Method == http.MethodOptions {
				if origin != "" {
					if _, ok := allowed[strings.ToLower(origin)]; ok {
						w.WriteHeader(http.StatusNoContent)
						return
					}
				}
				w.WriteHeader(http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Ensure json is imported (used in writeJSON via errors.go; this comment keeps
// the import live if the compiler can't see it used directly in this file).
var _ = json.Marshal
```

**Step 5.4 — Run `go build ./...` and `go vet ./...` to confirm compilation.**

```bash
cd C:/src/ai/intake/relay
go build ./...
go vet ./...
```

Expected: both exit 0. The `var _ = json.Marshal` import guard in server.go is not needed once writeJSON (in errors.go) uses it — remove it if the compiler complains about redundancy, but it should compile cleanly.

> **Implementation note for the implementer:** If the compiler reports `imported and not used: "encoding/json"` for `server.go`, remove the `import "encoding/json"` line and the `var _ = json.Marshal` sentinel — `writeJSON` is in `errors.go` (same package) and covers the import there.

**Step 5.5 — Commit.**

```bash
cd C:/src/ai/intake/relay
git add internal/server/
git commit -m "1-i: server deps, mux, CORS middleware, health/version handlers (Task 5)"
```

---

### Task 6 — `server_test.go`: health, version, and CORS tests

**Step 6.1 — Extend `relay/internal/server/server_test.go` with full handler tests.**

Replace the file entirely (it currently only has the Task 4 writeError test). The final content is:

```go
package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"intake/internal/config"
	"intake/internal/server"
	"intake/internal/version"
)

// ---- helpers ----

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("JSON decode failed: %v\nbody: %s", err, body)
	}
}

func newTestServer(t *testing.T, corsOrigins []string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr:        ":8080",
			ExternalURL: "http://localhost:8080",
			CORSOrigins: corsOrigins,
		},
	}
	deps := server.Deps{
		Version:     version.Info(),
		CORSOrigins: corsOrigins,
	}
	return server.New(cfg, deps)
}

// ---- Task 4: writeError shape ----

func TestWriteError_EnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	server.WriteErrorExported(w, http.StatusBadRequest, "bad_request", "something is wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", w.Code, http.StatusBadRequest)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q; want %q", ct, "application/json")
	}

	var env server.ErrorEnvelope
	decodeJSON(t, w.Body.Bytes(), &env)

	if env.Error.Code != "bad_request" {
		t.Errorf("error.code = %q; want %q", env.Error.Code, "bad_request")
	}
	if env.Error.Message != "something is wrong" {
		t.Errorf("error.message = %q; want %q", env.Error.Message, "something is wrong")
	}
}

// ---- Task 6: /v1/health ----

func TestHealth_Returns200(t *testing.T) {
	h := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
	}
	var body map[string]string
	decodeJSON(t, w.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("body.status = %q; want %q", body["status"], "ok")
	}
}

// ---- Task 6: /v1/version ----

func TestVersion_ReturnsBuildInfo(t *testing.T) {
	h := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
	}
	var info version.BuildInfo
	decodeJSON(t, w.Body.Bytes(), &info)
	if info.Version == "" {
		t.Error("version.version is empty")
	}
}

// ---- Task 6: CORS — allowed origin gets ACAO header ----

func TestCORS_AllowedOriginGetsHeader(t *testing.T) {
	allowed := "http://localhost:5173"
	h := newTestServer(t, []string{allowed})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", allowed)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != allowed {
		t.Errorf("ACAO = %q; want %q", acao, allowed)
	}
}

// ---- Task 6: CORS — disallowed origin does NOT get ACAO header ----

func TestCORS_DisallowedOriginNoHeader(t *testing.T) {
	h := newTestServer(t, []string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "" {
		t.Errorf("ACAO = %q; want empty for disallowed origin", acao)
	}
}

// ---- Task 6: CORS — preflight for allowed origin returns 204 ----

func TestCORS_PreflightAllowedOrigin(t *testing.T) {
	allowed := "http://localhost:5173"
	h := newTestServer(t, []string{allowed})

	req := httptest.NewRequest(http.MethodOptions, "/v1/intake/init", nil)
	req.Header.Set("Origin", allowed)
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d; want %d", w.Code, http.StatusNoContent)
	}
}

// ---- Task 6: CORS — preflight for disallowed origin returns 403 ----

func TestCORS_PreflightDisallowedOrigin(t *testing.T) {
	h := newTestServer(t, []string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodOptions, "/v1/intake/init", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("preflight status = %d; want %d", w.Code, http.StatusForbidden)
	}
}
```

**Step 6.2 — Run all server tests.**

```bash
cd C:/src/ai/intake/relay
go test ./internal/server/... -v
```

Expected: all 7 tests pass (TestWriteError_EnvelopeShape, TestHealth_Returns200, TestVersion_ReturnsBuildInfo, TestCORS_AllowedOriginGetsHeader, TestCORS_DisallowedOriginNoHeader, TestCORS_PreflightAllowedOrigin, TestCORS_PreflightDisallowedOrigin).

**Step 6.3 — Run the full test suite.**

```bash
cd C:/src/ai/intake/relay
go test ./...
```

Expected: all packages pass (config, version, server).

**Step 6.4 — Commit.**

```bash
cd C:/src/ai/intake/relay
git add internal/server/server_test.go
git commit -m "1-i: health/version/CORS handler tests (Task 6)"
```

---

### Task 7 — `cmd/relay/main.go`: wire config → server → ListenAndServe + graceful shutdown

**Step 7.1 — Replace the Phase 0 stub.**

Replace `relay/cmd/relay/main.go` entirely with:

```go
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"intake/internal/config"
	"intake/internal/server"
	"intake/internal/version"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the relay config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("relay: config: %v", err)
	}

	deps := server.Deps{
		Version:     version.Info(),
		CORSOrigins: cfg.Server.CORSOrigins,
	}

	handler := server.New(cfg, deps)

	srv := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: handler,
	}

	// Start the server in a goroutine so the main goroutine can wait for the
	// shutdown signal.
	idleConnsClosed := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("relay: shutdown signal received; draining connections...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("relay: graceful shutdown error: %v", err)
		}
		close(idleConnsClosed)
	}()

	log.Printf("relay: listening on %s (external: %s)", cfg.Server.Addr, cfg.Server.ExternalURL)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("relay: listen: %v", err)
	}

	<-idleConnsClosed
	log.Println("relay: stopped")
}
```

**Step 7.2 — Build the binary to confirm wiring compiles.**

```bash
cd C:/src/ai/intake/relay
go build ./cmd/relay
```

Expected: exits 0, produces `relay.exe` (Windows) or `relay` (Linux/macOS) in `relay/`.

**Step 7.3 — Quick boot smoke (no real config needed; use the testdata sample config).**

Create a temporary `config.yaml` in `relay/` for the smoke (copy from testdata, adjust addr):

```yaml
server:
  addr: ":18080"
  external_url: "http://localhost:18080"
  cors_origins:
    - "http://localhost:5173"
llm:
  provider: "anthropic"
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-sonnet-4-6"
    max_tokens: 1024
auth:
  modes:
    anonymous: true
adapters:
  webhook:
    enabled: false
    url: ""
    retry:
      max_attempts: 3
      backoff: "exponential"
```

Then in a terminal:

```bash
cd C:/src/ai/intake/relay
./relay.exe --config config.yaml &
curl -s http://localhost:18080/v1/health
# Expected: {"status":"ok"}
curl -s http://localhost:18080/v1/version
# Expected: {"version":"dev","commit":"none","build_time":"unknown"}
kill %1
```

On bash-on-Windows or PowerShell adapt the background process management appropriately:

```powershell
# PowerShell alternative
$proc = Start-Process -FilePath ".\relay.exe" -ArgumentList "--config", "config.yaml" -PassThru
Start-Sleep -Seconds 1
Invoke-RestMethod http://localhost:18080/v1/health
Invoke-RestMethod http://localhost:18080/v1/version
$proc | Stop-Process
```

**Step 7.4 — Run full test suite one final time.**

```bash
cd C:/src/ai/intake/relay
go test ./...
go vet ./...
```

Expected: all pass.

**Step 7.5 — Commit.**

```bash
cd C:/src/ai/intake/relay
git add cmd/relay/main.go
git commit -m "1-i: main.go with config→server wiring and graceful shutdown (Task 7)"
```

---

## 5. Smoke

**Scope:** this sub-plan alone (does not require 1-ii–1-iv). If the local binary boots and the two curl checks pass, the smoke passes.

### Pre-conditions

- Go 1.23.2 installed; `go` on PATH.
- Working directory: `C:/src/ai/intake/relay`.
- A `config.yaml` exists (use the one created in Task 7.3 or adapt as needed).
- Port 18080 is free on the dev machine.

### Execution

```bash
# Build a fresh binary
cd C:/src/ai/intake/relay
go build -o relay-smoke ./cmd/relay

# Start in background
./relay-smoke --config config.yaml &
RELAY_PID=$!
sleep 1

# Check health
HEALTH=$(curl -s http://localhost:18080/v1/health)
echo "health: $HEALTH"

# Check version
VERSION=$(curl -s http://localhost:18080/v1/version)
echo "version: $VERSION"

# Check CORS — allowed origin
ACAO=$(curl -s -o /dev/null -D - \
  -H "Origin: http://localhost:5173" \
  http://localhost:18080/v1/health | grep -i "access-control-allow-origin")
echo "ACAO (should be set): $ACAO"

# Check CORS — disallowed origin
ACAO_DENIED=$(curl -s -o /dev/null -D - \
  -H "Origin: http://evil.example.com" \
  http://localhost:18080/v1/health | grep -i "access-control-allow-origin")
echo "ACAO (should be empty): $ACAO_DENIED"

kill $RELAY_PID
rm relay-smoke
```

**PowerShell alternative** (Windows without bash):

```powershell
cd C:/src/ai/intake/relay
go build -o relay-smoke.exe ./cmd/relay
$proc = Start-Process -FilePath ".\relay-smoke.exe" -ArgumentList "--config","config.yaml" -PassThru
Start-Sleep -Seconds 1

# Health
Invoke-RestMethod http://localhost:18080/v1/health

# Version
Invoke-RestMethod http://localhost:18080/v1/version

# CORS — allowed
$r = Invoke-WebRequest -Uri http://localhost:18080/v1/health -Headers @{ Origin = "http://localhost:5173" }
$r.Headers["Access-Control-Allow-Origin"]   # expect: http://localhost:5173

# CORS — disallowed
$r2 = Invoke-WebRequest -Uri http://localhost:18080/v1/health -Headers @{ Origin = "http://evil.example.com" }
$r2.Headers["Access-Control-Allow-Origin"]  # expect: empty / not present

$proc | Stop-Process
Remove-Item relay-smoke.exe
```

### Expected assertions

| Check | Expected |
|---|---|
| `/v1/health` body | `{"status":"ok"}` |
| `/v1/version` body | JSON with `version`, `commit`, `build_time` fields; values `"dev"`, `"none"`, `"unknown"` |
| ACAO for `http://localhost:5173` | `http://localhost:5173` |
| ACAO for `http://evil.example.com` | absent / empty |
| `go test ./...` | all pass (0 failures) |

---

## 6. Done criteria

- [ ] `go get github.com/go-chi/chi/v5` and `go get gopkg.in/yaml.v3` resolved and recorded with exact versions in `relay/go.mod` and `relay/go.sum`.
- [ ] `ai/tasks/phase-1/README.md` §5 pin table updated with exact resolved chi and yaml versions in the same commit as Task 1.
- [ ] `relay/internal/config/config.go` implements all structs from README §6.6 verbatim; `Load()` applies sane defaults; missing-file returns a clear error; secret resolution is NOT done (only env-var name stored).
- [ ] Config tests pass: sample YAML parses correctly; defaults applied when fields omitted; missing file yields non-nil error.
- [ ] `relay/internal/version/version.go` defines `BuildInfo` and `Info()`; ldflags vars default to `"dev"`, `"none"`, `"unknown"`.
- [ ] `relay/internal/server/dto.go` contains all DTOs from README §6.4 verbatim (no omissions, no additions).
- [ ] `relay/internal/server/errors.go` provides `writeJSON` and `writeError`; `WriteErrorExported` wrapper present for tests.
- [ ] `relay/internal/server/deps.go` defines `Deps` with `Version` and `CORSOrigins` fields plus the extension comment for 1-iii/1-iv.
- [ ] `relay/internal/server/routes.go` defines `registerIntakeRoutes` with an empty body and the guiding comment.
- [ ] `relay/internal/server/server.go` builds the chi mux with RequestID, Recoverer, CORS middleware, `/v1/health`, `/v1/version`, and the `/v1/intake` route group wired to `registerIntakeRoutes`.
- [ ] CORS middleware rejects unlisted origins (no wildcard ever); handles OPTIONS preflight.
- [ ] `relay/cmd/relay/main.go` parses `--config`, calls `config.Load`, builds `server.Deps`, calls `server.New`, starts `http.Server`, and handles SIGINT/SIGTERM with a 15-second drain timeout.
- [ ] `go test ./...` in `relay/` passes with zero failures.
- [ ] `go build ./...` and `go vet ./...` in `relay/` exit 0.
- [ ] Smoke passes: relay boots from config; `/v1/health` → 200 `{"status":"ok"}`; `/v1/version` → BuildInfo JSON; allowed CORS origin gets ACAO header; disallowed origin does not.
- [ ] No secret value (API key, webhook auth header value) appears anywhere in test output, log output, or HTTP response bodies.

---

## Environment notes for the implementer

- **OS:** Windows 10, dev machine.
- **Go:** 1.23.2 (`go version` should print `go1.23.2`).
- **Shell:** PowerShell is default; bash is available via Git Bash or WSL. Most `go` commands are shell-agnostic; the smoke can be run in either.
- **Git remote:** none (local commits only; no `git push`).
- **Test command:** always run from `C:/src/ai/intake/relay/` as `go test ./...`.
- **Paths in code:** use forward slashes in import paths (Go standard); Windows absolute paths with backslashes only in shell commands.
- **Binary output:** `go build ./cmd/relay` produces `relay.exe` on Windows; invoke as `.\relay.exe`.
- **Background processes in PowerShell:** use `Start-Process -PassThru` to get a handle for later `Stop-Process`; `Start-Sleep -Seconds 1` after starting to let the listener bind.
