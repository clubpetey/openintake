# 5-i Config + Middleware Chain Seam + Q9 Startup Gate + Dispatcher Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lock the Phase 5 seams so 5-ii and 5-iii can land in parallel. Add the new config blocks (`server.trusted_proxies`, `auth.anonymous`, `captcha`, `ratelimit`), introduce `clientIPMiddleware` + a stubbed `perIPLimitMiddleware` in the chi chain, wire the **consolidated Q9 startup gate** in `main.go`, and harden `auth.Middleware` so `auth.modes.anonymous=false` actually rejects anonymous sessions (Q9 strict). After this sub-plan: no actual rate-limiting / budget-tracking / CAPTCHA happens yet (the Deps fields are nil and the chain stubs always allow), but the wiring is locked and Phase 1 + Phase 4 callers see zero behavior change.

**Architecture:** Three additive surfaces: (1) `config` gains five new structs + defaults + validation (`action_on_exceeded != "reject"` is fatal); (2) `relay/internal/server/clientip.go` introduces `clientIPMiddleware` that resolves the request's client IP per `server.trusted_proxies` and stashes it in `r.Context()` under an unexported key; (3) `auth.NewMiddlewareWithModes` adds a single `modesAnonymous` field whose `false` value short-circuits the anonymous fall-through to 401. `main.go` parses CIDRs + secrets + applies the Q9 gate (a single `[]string` of problems → one structured log line → `os.Exit(1)` if non-empty) before constructing the middleware. `Deps` gains four new fields (`CaptchaCfg`, `CaptchaVerifier`, `Budget`, `PerIP`, `TrustedProxies`) that 5-i populates with nil/stub values; 5-ii and 5-iii each replace one slot.

**Tech Stack:** Go 1.23.2 (relay). No new external Go modules in 5-i; `net/netip` is stdlib. `golang.org/x/time/rate` is promoted from indirect to direct in `relay/go.mod` ahead of 5-ii's actual use (so the pin lands with the seam, not with the consumer). `gopkg.in/yaml.v3` (already present) for the new config structs.

---

## Design References

- README §8.2 — config additions (frozen here)
- README §8.7 — `auth.Middleware` extension (frozen here)
- README §8.8 — `clientIPMiddleware` shape (frozen here)
- README §8.10 — `InitResponse` extension (shape locked here for 5-iii to wire)
- README §8.11 — `Deps` extension (frozen here)
- Design spec §2.1 — middleware chain composition
- Design spec §2.3 — Q9 strict-anonymous + consolidated startup gate
- Design spec §3.5 — `clientIPMiddleware` resolution semantics
- Design spec §3.6 — `auth.Middleware` extension
- Design spec §4 — full config additions
- Reference: existing `relay/internal/server/server.go:51-96` (Phase 1 `corsMiddleware` — Phase 5 inherits the strict CORS behavior unchanged)
- Reference: existing `relay/internal/auth/middleware.go:99-117` (Phase 1 anonymous fall-through — Phase 5 adds a single `if !m.modesAnonymous` guard)
- Reference: existing `relay/cmd/relay/main.go:151-186` (Phase 4 auth wiring — Phase 5 inserts the Q9 gate before `NewMiddleware...` and changes the constructor call to `NewMiddlewareWithModes`)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/config/config.go` | Modify | Add `ServerConfig.TrustedProxies`, `AuthConfig.Anonymous`, `CaptchaConfig`, `RateLimitConfig` + sub-structs; extend `applyDefaults`; add `Validate()` for `action_on_exceeded` |
| `relay/internal/config/config_test.go` | Modify | Tests for new defaults + explicit-empty `required_for` + `Validate` rejection of bad `action_on_exceeded` |
| `relay/internal/config/testdata/sample.yaml` | Modify | Add `server.trusted_proxies`, `auth.anonymous`, `captcha`, `ratelimit` blocks |
| `relay/internal/server/clientip.go` | Create | `clientIPMiddleware` + `ClientIPFromContext` + `clientIPCtxKey` |
| `relay/internal/server/clientip_test.go` | Create | Matrix tests: empty list, single CIDR, multi-hop XFF, malformed RemoteAddr, ignored XFF when untrusted |
| `relay/internal/server/server.go` | Modify | `New` mounts `clientIPMiddleware` + stub `perIPLimitMiddleware` on `/v1/intake` group; reads `deps.TrustedProxies` and `deps.PerIP` |
| `relay/internal/server/server_test.go` | Modify | One smoke that the new middlewares are wired (request through `/v1/intake/init` carries IP in ctx) |
| `relay/internal/server/dto.go` | Modify | Add `Capabilities.RequiresCaptcha`, `InitCaptcha`, `InitRequest`, `CaptchaRequiredResponse`; extend `InitResponse` with `Captcha *InitCaptcha` |
| `relay/internal/server/deps.go` | Modify | Add `CaptchaCfg`, `CaptchaVerifier`, `Budget`, `PerIP`, `TrustedProxies` fields (5-iii / 5-ii fill them) |
| `relay/internal/server/turn.go` | Modify | `initHandler` parses optional `InitRequest`, emits `Capabilities.RequiresCaptcha` when applicable; no actual verify yet (5-iii adds that) |
| `relay/internal/auth/middleware.go` | Modify | Add `modesAnonymous bool` field + `NewMiddlewareWithModes` constructor; `NewMiddleware` preserved as wrapper; Handler anonymous-branch guard |
| `relay/internal/auth/middleware_test.go` | Modify | Add Q9 strict-anonymous regression tests (modesAnonymous=false → 401 with valid X-Intake-Session); preserve Phase 1+4 `TestDispatcher_AnonymousFallthrough_Preserved` |
| `relay/cmd/relay/main.go` | Modify | Insert Q9 consolidated gate (anonymous-no-captcha + sso-both + sso-neither + bad-CIDR + bad-action_on_exceeded) before middleware construction; switch to `NewMiddlewareWithModes`; parse `TrustedProxies` once into `Deps` |
| `relay/go.mod` | Modify | Promote `golang.org/x/time` from indirect to direct require at `v0.9.0` |
| `scripts/check-pins.sh` | Modify | Add `golang.org/x/time` caret/`@latest` check matching the existing pattern |
| `relay/internal/server/perip_stub.go` | Create | `perIPLimitMiddleware` stub that always allows (5-ii replaces with real Limiter call); kept as a file so 5-ii's diff is purely additive |

---

## Tasks

### Task 1: Promote `golang.org/x/time` to direct require + extend `check-pins.sh`

**Files:** Modify `relay/go.mod`, `scripts/check-pins.sh`

- [ ] **Step 1: Inspect current go.mod**

Run: `cat relay/go.mod`
Expected: `golang.org/x/time v0.9.0 // indirect` appears in the second `require` block.

- [ ] **Step 2: Move `golang.org/x/time v0.9.0` from the indirect to the direct require block**

In `relay/go.mod`, REMOVE this line from the indirect `require` block:

```
	golang.org/x/time v0.9.0 // indirect
```

ADD it to the primary `require` block (alongside `golang.org/x/time` would alphabetize after `gopkg.in/yaml.v3` — but the existing block uses module-path order; place it at the end of the primary block, like so):

```
require (
	github.com/MicahParks/keyfunc/v3 v3.8.0
	github.com/anthropics/anthropic-sdk-go v1.45.0
	github.com/go-chi/chi/v5 v5.3.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/openai/openai-go v1.12.0
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	golang.org/x/time v0.9.0
	google.golang.org/genai v0.7.0
	gopkg.in/yaml.v3 v3.0.1
)
```

- [ ] **Step 3: Run `go mod tidy` in `relay/` — must be a no-op**

Run: `cd relay && go mod tidy && cd ..`
Expected: no changes to go.mod or go.sum (verify with `git diff relay/go.mod relay/go.sum` — should be a clean diff with only the indirect→direct move).

- [ ] **Step 4: Add x/time check to `scripts/check-pins.sh`**

After the existing `MicahParks/keyfunc/v3` gate (line ~44 in `scripts/check-pins.sh`), insert this block before the final `# Note:` comment:

```bash
# Gate: golang.org/x/time must be exact-pinned (no caret, no @latest) in go.mod. Phase 5.
if grep -E 'golang.org/x/time' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: golang.org/x/time is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
```

- [ ] **Step 5: Run `scripts/check-pins.sh` — must pass**

Run: `bash scripts/check-pins.sh`
Expected: prints `OK: ...` and exits 0.

- [ ] **Step 6: Run `scripts/verify-contract.sh` — must still pass**

Run: `bash scripts/verify-contract.sh`
Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
git add relay/go.mod scripts/check-pins.sh
git commit -m "chore(5-i): promote golang.org/x/time to direct require; extend check-pins.sh"
```

---

### Task 2: Extend `config.go` with the four new struct types + defaults + `Validate()`

**Files:** Modify `relay/internal/config/config.go`, `relay/internal/config/config_test.go`, `relay/internal/config/testdata/sample.yaml`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/config/config_test.go` (after the last existing test's closing `}`):

```go
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
	if cfg.Server.TrustedProxies == nil {
		// nil and empty slice are equivalent at the wire; either is fine. The
		// invariant is that the field is loadable and ranges-over-empty safely.
		return
	}
	if len(cfg.Server.TrustedProxies) != 0 {
		t.Errorf("default TrustedProxies = %v; want empty", cfg.Server.TrustedProxies)
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
```

Add the imports `"os"` and `"strings"` at the top of the file if not already present:

```go
import (
	"os"
	"strings"
	"testing"

	"intake/internal/config"
)
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/config/... -run Phase5 -v && cd ..`
Expected: FAIL — compilation errors first (`cfg.RateLimit undefined`, `cfg.Captcha undefined`, `cfg.Auth.Anonymous undefined`, `cfg.Validate undefined`).

Also: `cd relay && go test ./internal/config/... -run TestValidate -v && cd ..` — same compilation failure.

- [ ] **Step 3: Add the new structs to `config.go`**

In `relay/internal/config/config.go`, modify `ServerConfig` to add `TrustedProxies`:

```go
// ServerConfig holds HTTP server and CORS settings.
type ServerConfig struct {
	Addr           string   `yaml:"addr"`
	ExternalURL    string   `yaml:"external_url"`
	CORSOrigins    []string `yaml:"cors_origins"`
	TrustedProxies []string `yaml:"trusted_proxies"` // Phase 5: CIDR list; empty = always use RemoteAddr
}
```

Modify `AuthConfig` to add the `Anonymous` sub-struct:

```go
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
```

Add the two new top-level config blocks to the `Config` struct:

```go
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
```

Append these new types at the bottom of `config.go` (before the `applyDefaults` function):

```go
// CaptchaConfig configures the optional CAPTCHA challenge gate at /v1/intake/init.
// All secrets reference an env var name; the value resolves via ResolveSecret in main.go.
type CaptchaConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Provider     string   `yaml:"provider"`       // "turnstile" | "hcaptcha"
	SiteKey      string   `yaml:"site_key"`       // public; safe to commit
	SecretKeyEnv string   `yaml:"secret_key_env"` // env var name; ResolveSecret
	// RequiredFor lists the auth modes that must solve a CAPTCHA before /init mints
	// a session. Default applied by applyDefaults when YAML omits the key: ["anonymous"].
	// An explicit empty list `required_for: []` is honored (operator opted out).
	RequiredFor []string `yaml:"required_for"`
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
	Burst             int     `yaml:"burst"`                // default 5
	IdleTTL           string  `yaml:"idle_ttl"`             // default "15m"
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
```

To preserve "explicit `required_for: []` is honored," `CaptchaConfig.RequiredFor` uses a custom YAML unmarshaler. Add the unmarshaler near `CaptchaConfig`:

```go
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
```

In `applyDefaults`, append at the end (before the closing `}`):

```go
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
```

Add a new `Validate` method at the bottom of the file (after `Load`):

```go
// Validate enforces invariants that defaults alone can't express. Currently:
//   - DailyLLMBudget.ActionOnExceeded must be "reject" (v0 only ships reject;
//     "queue" is documented as v1+ per PROJECT.md §10).
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
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/config/... -v && cd ..`
Expected: all existing tests + the new Phase5 tests pass.

- [ ] **Step 5: Update `testdata/sample.yaml` so it parses through the new structs**

After the existing `auth:` block in `relay/internal/config/testdata/sample.yaml`, ADD (do not replace) the Phase 5 blocks (use whatever indentation the existing file uses — typically 2-space):

```yaml
server:
  # ... existing fields preserved ...
  trusted_proxies:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
auth:
  # ... existing modes/email/sso blocks preserved ...
  anonymous:
    allow_without_captcha: false
captcha:
  enabled: false
  provider: "turnstile"
  site_key: "0x4AAA000000ExampleSiteKey"
  secret_key_env: "INTAKE_TURNSTILE_SECRET"
  required_for: ["anonymous"]
ratelimit:
  per_ip:
    requests_per_second: 2.0
    burst: 10
    idle_ttl: "5m"
  per_session:
    max_turns: 30
    max_input_tokens: 12000
    session_ttl: "30m"
  daily_llm_budget:
    max_input_tokens: 1000000
    max_output_tokens: 200000
    action_on_exceeded: "reject"
```

Read the existing `relay/internal/config/testdata/sample.yaml` to find the right insertion points (the `server:` block already exists with `addr`/`external_url`/`cors_origins`; add `trusted_proxies` underneath. Same for `auth:` — add `anonymous` underneath the existing `modes`/`email`/`sso`).

Then add a `TestLoad_ParsesSampleYAMLPhase5Blocks` test to `config_test.go`:

```go
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
```

Run: `cd relay && go test ./internal/config/... -v && cd ..`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add relay/internal/config/config.go relay/internal/config/config_test.go relay/internal/config/testdata/sample.yaml
git commit -m "feat(5-i): config blocks for trusted_proxies, captcha, ratelimit, anonymous; Validate()"
```

---

### Task 3: Create `relay/internal/server/clientip.go` (resolver + middleware + ctx helper)

**Files:** Create `relay/internal/server/clientip.go`, `relay/internal/server/clientip_test.go`

- [ ] **Step 1: Write the failing tests**

Create `relay/internal/server/clientip_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func mustPrefix(t *testing.T, s string) netip.Prefix {
	t.Helper()
	p, err := netip.ParsePrefix(s)
	if err != nil {
		t.Fatalf("ParsePrefix(%q): %v", s, err)
	}
	return p
}

func newRequestWithRemoteAndXFF(remote, xff string) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	return req
}

func TestClientIP_EmptyTrustedProxies_UsesRemoteAddrVerbatim(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware(nil)
	req := newRequestWithRemoteAndXFF("203.0.113.5:12345", "1.2.3.4")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "203.0.113.5" {
		t.Errorf("ClientIP = %q; want 203.0.113.5 (XFF must be ignored when no trusted proxies)", captured)
	}
}

func TestClientIP_TrustedProxy_RightmostUntrustedXFFHop(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{mustPrefix(t, "10.0.0.0/8")})
	// RemoteAddr is trusted (10.0.0.1); XFF chain: original=203.0.113.7,
	// then a trusted internal proxy 10.0.0.2, then the trusted edge 10.0.0.1.
	req := newRequestWithRemoteAndXFF("10.0.0.1:12345", "203.0.113.7, 10.0.0.2")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "203.0.113.7" {
		t.Errorf("ClientIP = %q; want 203.0.113.7 (rightmost untrusted XFF hop)", captured)
	}
}

func TestClientIP_UntrustedRemoteAddr_IgnoresSpoofedXFF(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{mustPrefix(t, "10.0.0.0/8")})
	// RemoteAddr is NOT in any trusted CIDR → XFF is ignored even if present.
	req := newRequestWithRemoteAndXFF("203.0.113.5:12345", "1.2.3.4")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "203.0.113.5" {
		t.Errorf("ClientIP = %q; want 203.0.113.5 (untrusted RemoteAddr must use RemoteAddr verbatim)", captured)
	}
}

func TestClientIP_AllHopsTrusted_FallsBackToLeftmost(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware([]netip.Prefix{mustPrefix(t, "10.0.0.0/8")})
	// Every hop is trusted — fall back to the leftmost (original client per RFC 7239).
	req := newRequestWithRemoteAndXFF("10.0.0.1:12345", "10.1.1.1, 10.2.2.2")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "10.1.1.1" {
		t.Errorf("ClientIP = %q; want 10.1.1.1 (all-trusted falls back to leftmost)", captured)
	}
}

func TestClientIP_MalformedRemoteAddr_StashesEmpty(t *testing.T) {
	var captured string
	hit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware(nil)
	req := newRequestWithRemoteAndXFF("not-an-address", "")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if !hit {
		t.Fatal("next handler not invoked")
	}
	if captured != "" {
		t.Errorf("ClientIP = %q; want \"\" (malformed RemoteAddr → empty)", captured)
	}
}

func TestClientIP_NoMiddleware_ReturnsEmpty(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if ip := ClientIPFromContext(req.Context()); ip != "" {
		t.Errorf("ClientIPFromContext on bare ctx = %q; want \"\"", ip)
	}
}

func TestClientIP_IPv6RemoteAddr(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClientIPFromContext(r.Context())
	})
	mw := clientIPMiddleware(nil)
	req := newRequestWithRemoteAndXFF("[2001:db8::1]:9999", "")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if captured != "2001:db8::1" {
		t.Errorf("ClientIP = %q; want 2001:db8::1 (IPv6 from bracketed RemoteAddr)", captured)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/server/ -run TestClientIP -v && cd ..`
Expected: FAIL — `clientIPMiddleware undefined`, `ClientIPFromContext undefined`.

- [ ] **Step 3: Create `clientip.go` with the implementation**

Create `relay/internal/server/clientip.go`:

```go
package server

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// clientIPCtxKey is the unexported context key for the resolved client IP.
type clientIPCtxKey struct{}

// clientIPMiddleware resolves the request's client IP per the trusted-proxies
// allowlist and stashes it in r.Context() under clientIPCtxKey{}.
//
// Resolution:
//   - If RemoteAddr is in any CIDR of trustedProxies, walk X-Forwarded-For
//     right-to-left and take the first hop NOT in trustedProxies. If every
//     hop is trusted, use the leftmost hop (the original client per
//     RFC 7239 standard).
//   - Otherwise (or if trustedProxies is empty), use RemoteAddr verbatim.
//
// The stashed value is the IP only (no port). If RemoteAddr cannot be parsed,
// the empty string is stashed — the per-IP limiter will treat all such
// requests as a single bucket (safe degraded behavior).
func clientIPMiddleware(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trustedProxies)
			ctx := context.WithValue(r.Context(), clientIPCtxKey{}, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientIPFromContext returns the IP stashed by clientIPMiddleware.
// Returns "" if not set (or if the middleware stashed "" for an
// unparseable RemoteAddr).
func ClientIPFromContext(ctx context.Context) string {
	v, _ := ctx.Value(clientIPCtxKey{}).(string)
	return v
}

// resolveClientIP applies the rules in the clientIPMiddleware doc.
func resolveClientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	remoteIP := parseHostFromRemoteAddr(r.RemoteAddr)
	if remoteIP == "" {
		return ""
	}
	if !ipInAnyPrefix(remoteIP, trustedProxies) {
		return remoteIP
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remoteIP
	}
	// Right-to-left scan for the first untrusted hop.
	hops := strings.Split(xff, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(hops[i])
		if hop == "" {
			continue
		}
		if !ipInAnyPrefix(hop, trustedProxies) {
			return hop
		}
	}
	// All hops trusted → return the leftmost (original client per RFC 7239).
	leftmost := strings.TrimSpace(hops[0])
	if leftmost == "" {
		return remoteIP
	}
	return leftmost
}

// parseHostFromRemoteAddr extracts just the host portion of an "ip:port" pair,
// handling IPv6 bracketed form. Returns "" if r.RemoteAddr is unparseable.
func parseHostFromRemoteAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// Try as a bare IP (some test paths set RemoteAddr without a port).
		if ip, perr := netip.ParseAddr(remoteAddr); perr == nil {
			return ip.String()
		}
		return ""
	}
	return host
}

// ipInAnyPrefix reports whether ip is contained in any of the given prefixes.
// Returns false for an unparseable ip.
func ipInAnyPrefix(ip string, prefixes []netip.Prefix) bool {
	if len(prefixes) == 0 {
		return false
	}
	parsed, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, p := range prefixes {
		if p.Contains(parsed) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/server/ -run TestClientIP -v && cd ..`
Expected: all 7 ClientIP tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/server/clientip.go relay/internal/server/clientip_test.go
git commit -m "feat(5-i): clientIPMiddleware + ClientIPFromContext (trusted-proxies + XFF)"
```

---

### Task 4: Add `perIPLimitMiddleware` stub (5-ii replaces; chain wiring is here)

**Files:** Create `relay/internal/server/perip_stub.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/internal/server/clientip_test.go`:

```go
func TestPerIPLimit_NilLimiter_AllowsAll(t *testing.T) {
	hit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true })
	mw := perIPLimitMiddleware(nil) // nil → "no limiter wired" → always allow
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)
	if !hit {
		t.Fatal("next handler not invoked when limiter is nil")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want 200 (default OK when next does not write)", rec.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./internal/server/ -run TestPerIPLimit -v && cd ..`
Expected: FAIL — `perIPLimitMiddleware undefined`.

- [ ] **Step 3: Create the stub**

Create `relay/internal/server/perip_stub.go`:

```go
package server

import (
	"net/http"

	"intake/internal/ratelimit/perip"
)

// perIPLimitMiddleware enforces the per-IP token bucket via the supplied
// Limiter. When limiter is nil, all requests pass through (the chain is wired
// but the gate is inert — used in 5-i before 5-ii lands the real Limiter,
// and in unit tests that don't exercise rate-limiting).
//
// On reject: writes 429 + Retry-After: <secs> + the standard ErrorEnvelope
// with code "rate_limited". The client IP is read from r.Context() via
// ClientIPFromContext; an empty IP shares one bucket (degraded safe).
func perIPLimitMiddleware(limiter *perip.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}
			ip := ClientIPFromContext(r.Context())
			ok, retryAfter := limiter.Allow(ip)
			if ok {
				next.ServeHTTP(w, r)
				return
			}
			secs := int(retryAfter.Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", itoa(secs))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests; slow down")
		})
	}
}

// itoa is strconv.Itoa inlined to avoid an import — keeps the stub minimal
// and matches the style of other tiny server-internal helpers.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
```

- [ ] **Step 4: Run the test — must pass**

Run: `cd relay && go test ./internal/server/ -run TestPerIPLimit -v && cd ..`
Expected: PASS.

Run: `cd relay && go build ./... && cd ..`
Expected: build fails — `intake/internal/ratelimit/perip` does not exist yet. This is expected; 5-ii creates the package. For 5-i we need a placeholder.

- [ ] **Step 5: Create a placeholder `perip` package so the chain compiles**

Create `relay/internal/ratelimit/perip/perip.go`:

```go
// Package perip is the per-IP token-bucket rate limiter for the relay's
// /v1/intake/* endpoints. Phase 5-i exports the empty type and the New
// constructor as placeholders so the server chain compiles; 5-ii fills in
// the actual implementation backed by golang.org/x/time/rate.
package perip

import "time"

// Limiter holds a per-client-IP token bucket. 5-i placeholder; 5-ii implements.
type Limiter struct {
	// Phase 5-i: no fields. Phase 5-ii populates with rps/burst/idleTTL/now
	// and a sync.Mutex-guarded map[string]*entry.
}

// New constructs a Limiter. 5-i placeholder; 5-ii implements.
// Until 5-ii lands, the only safe constructor is one that returns nil (so
// perIPLimitMiddleware short-circuits in its nil branch). main.go in 5-i
// passes nil to Deps.PerIP; do NOT call this constructor in 5-i.
func New(reqsPerSecond float64, burst int, idleTTL time.Duration, now func() time.Time) *Limiter {
	// 5-ii implements. Returning the zero value here is acceptable only
	// because no 5-i code path calls New (main.go passes nil to Deps.PerIP).
	return &Limiter{}
}

// Allow is the gate the middleware calls. 5-i placeholder; 5-ii implements.
func (l *Limiter) Allow(ip string) (ok bool, retryAfter time.Duration) {
	// 5-i: an all-allow stub keeps the chain compiling. 5-ii replaces with
	// the real rate.Limiter call.
	return true, 0
}
```

- [ ] **Step 6: Run full build + tests**

Run: `cd relay && go build ./... && go vet ./... && cd ..`
Expected: build + vet pass.

Run: `cd relay && go test ./internal/server/ -v && cd ..`
Expected: existing server tests + the new ClientIP + PerIPLimit stub tests all pass.

- [ ] **Step 7: Commit**

```bash
git add relay/internal/server/perip_stub.go relay/internal/server/clientip_test.go relay/internal/ratelimit/perip/perip.go
git commit -m "feat(5-i): perIPLimitMiddleware + placeholder perip.Limiter (5-ii implements)"
```

---

### Task 5: Harden `auth.Middleware` — add `modesAnonymous` + `NewMiddlewareWithModes`

**Files:** Modify `relay/internal/auth/middleware.go`, `relay/internal/auth/middleware_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/auth/middleware_test.go`:

```go
func TestDispatcher_StrictAnonymous_RejectsValidSessionWhenModesAnonymousFalse(t *testing.T) {
	store := auth.NewStore()
	sessionID := store.Issue()

	m := auth.NewMiddlewareWithModes(store, nil, nil, false) // modesAnonymous=false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be invoked when modesAnonymous=false")
	})

	req := httptest.NewRequest("POST", "/v1/intake/turn", nil)
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	m.Handler(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401 (Q9 strict anonymous reject)", rec.Code)
	}
}

func TestDispatcher_StrictAnonymous_PreservedPhase1DefaultBehavior(t *testing.T) {
	// NewMiddleware (the Phase 1+4 constructor) must default modesAnonymous=true.
	store := auth.NewStore()
	sessionID := store.Issue()

	m := auth.NewMiddleware(store, nil, nil)
	hit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		sess, ok := auth.FromContext(r.Context())
		if !ok {
			t.Fatal("SessionContext not attached")
		}
		if sess.AuthMode != "anonymous" {
			t.Errorf("AuthMode = %q; want anonymous", sess.AuthMode)
		}
	})
	req := httptest.NewRequest("POST", "/v1/intake/turn", nil)
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	m.Handler(next).ServeHTTP(rec, req)

	if !hit {
		t.Fatal("next handler not invoked (Phase 1 regression!)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want 200 (default OK when next does not write)", rec.Code)
	}
}
```

Add the required imports at the top of `middleware_test.go` if not present: `"net/http"`, `"net/http/httptest"`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/auth/ -run TestDispatcher_StrictAnonymous -v && cd ..`
Expected: FAIL — `auth.NewMiddlewareWithModes undefined`.

- [ ] **Step 3: Modify `middleware.go` — add the field + constructor + guard**

In `relay/internal/auth/middleware.go`, modify the `Middleware` struct to add `modesAnonymous`:

```go
type Middleware struct {
	store          *Store
	email          EmailJWTVerifier // nil → email mode off
	sso            SSOVerifier      // nil → sso mode off
	modesAnonymous bool             // Phase 5: false → reject anonymous even with valid X-Intake-Session
}
```

Update the doc comment on the type to mention Phase 5:

```go
// Middleware is a chi-compatible HTTP middleware that resolves session identity.
//
// Resolution order (4-i):
//  1. Authorization: Bearer <token>: try email mode, then SSO; on either success
//     attach SessionContext{Verified:true,...}.
//  2. No Authorization header:
//       a. modesAnonymous=true AND X-Intake-Session present + store.Validate →
//          SessionContext{AuthMode:"anonymous"}.
//       b. modesAnonymous=false → 401 (Phase 5 Q9 strict enforcement).
//       c. otherwise → 401.
```

Replace the existing `NewMiddleware`:

```go
// NewMiddleware returns a Middleware backed by the given Store. email and sso
// are optional; pass nil to disable the corresponding bearer-token validator.
// Phase 1+4 wrapper around NewMiddlewareWithModes — defaults modesAnonymous=true
// so callers that haven't migrated see no behavior change.
func NewMiddleware(store *Store, email EmailJWTVerifier, sso SSOVerifier) *Middleware {
	return NewMiddlewareWithModes(store, email, sso, true)
}

// NewMiddlewareWithModes is the Phase 5 constructor. modesAnonymous=false →
// the anonymous fall-through branch returns 401 even when a valid
// X-Intake-Session is presented (Q9 strict enforcement; PROJECT.md §19 Q9).
func NewMiddlewareWithModes(store *Store, email EmailJWTVerifier, sso SSOVerifier, modesAnonymous bool) *Middleware {
	return &Middleware{store: store, email: email, sso: sso, modesAnonymous: modesAnonymous}
}
```

In `Handler`, insert the strict guard ABOVE the existing X-Intake-Session check. Find this block (currently lines ~99-117):

```go
		// Anonymous fallback.
		sessionID := r.Header.Get("X-Intake-Session")
		if sessionID == "" || !m.store.Validate(sessionID) {
			authWriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "missing or invalid X-Intake-Session header; call POST /v1/intake/init first",
				},
			})
			return
		}

		ctx := WithSession(r.Context(), &SessionContext{
			SessionID: sessionID,
			AuthMode:  "anonymous",
			Verified:  false,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
```

Replace with (inserts the new guard at the top):

```go
		// Anonymous fallback.
		// Phase 5 Q9 strict: modesAnonymous=false → never serve anonymous,
		// even when a valid X-Intake-Session is presented.
		if !m.modesAnonymous {
			authWriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "anonymous mode is disabled on this relay",
				},
			})
			return
		}
		sessionID := r.Header.Get("X-Intake-Session")
		if sessionID == "" || !m.store.Validate(sessionID) {
			authWriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"code":    "unauthorized",
					"message": "missing or invalid X-Intake-Session header; call POST /v1/intake/init first",
				},
			})
			return
		}

		ctx := WithSession(r.Context(), &SessionContext{
			SessionID: sessionID,
			AuthMode:  "anonymous",
			Verified:  false,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
```

- [ ] **Step 4: Run all auth tests — Phase 1+4 must pass + the new Q9 tests must pass**

Run: `cd relay && go test ./internal/auth/ -v && cd ..`
Expected: all existing dispatcher tests pass; the two new Q9 tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/auth/middleware.go relay/internal/auth/middleware_test.go
git commit -m "feat(5-i): NewMiddlewareWithModes + Q9 strict-anonymous dispatcher guard"
```

---

### Task 6: Extend `Deps` + `InitResponse` DTO + `initHandler` for the captcha shape

**Files:** Modify `relay/internal/server/deps.go`, `relay/internal/server/dto.go`, `relay/internal/server/turn.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/internal/server/turn_test.go` (find the file; if it doesn't have a test for /init's capabilities, create one — but it does have InitResponse tests from Phase 4):

```go
func TestInitHandler_EmitsRequiresCaptchaWhenAnonymousAndEnabled(t *testing.T) {
	// Build a Deps with captcha enabled and required_for ["anonymous"];
	// no CaptchaVerifier wired (Phase 5-i pre-5-iii), so the handler should
	// still return 400 captcha_required + the discovery hint fields.
	deps := Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		CaptchaCfg: config.CaptchaConfig{
			Enabled:     true,
			Provider:    "turnstile",
			SiteKey:     "0x4AAA000000Test",
			RequiredFor: []string{"anonymous"},
		},
	}
	h := initHandler(deps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 (captcha_required)", rec.Code)
	}
	var body CaptchaRequiredResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (raw: %s)", err, rec.Body.String())
	}
	if body.Error.Code != "captcha_required" {
		t.Errorf("error.code = %q; want captcha_required", body.Error.Code)
	}
	if body.Captcha == nil || body.Captcha.Provider != "turnstile" || body.Captcha.SiteKey != "0x4AAA000000Test" {
		t.Errorf("body.captcha = %+v; want {turnstile, 0x4AAA000000Test}", body.Captcha)
	}
	if len(body.Capabilities.RequiresCaptcha) != 1 || body.Capabilities.RequiresCaptcha[0] != "anonymous" {
		t.Errorf("capabilities.requires_captcha = %v; want [anonymous]", body.Capabilities.RequiresCaptcha)
	}
}

func TestInitHandler_NoCaptchaConfig_MintsSessionAsBefore(t *testing.T) {
	deps := Deps{
		Auth: auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true},
		},
		// CaptchaCfg.Enabled defaults to false.
	}
	h := initHandler(deps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	var body InitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SessionID == "" {
		t.Error("session_id is empty; want a UUID")
	}
	if body.Captcha != nil {
		t.Errorf("body.captcha = %+v; want nil (captcha disabled)", body.Captcha)
	}
	if body.Capabilities.RequiresCaptcha != nil {
		t.Errorf("capabilities.requires_captcha = %v; want nil/omitted", body.Capabilities.RequiresCaptcha)
	}
}
```

Add imports if missing at the top of `turn_test.go`: `"strings"`, `"intake/internal/config"`, `"encoding/json"`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/server/ -run TestInitHandler -v && cd ..`
Expected: FAIL — `CaptchaRequiredResponse undefined`, `CaptchaCfg undefined`, etc.

- [ ] **Step 3: Extend `Deps` with the four new fields**

In `relay/internal/server/deps.go`, ADD the imports (if not present):

```go
import (
	"log/slog"
	"net/netip"

	"intake/internal/auth"
	"intake/internal/budget"
	"intake/internal/captcha"
	"intake/internal/classify"
	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/payloadbuild"
	"intake/internal/ratelimit/perip"
	"intake/internal/router"
	"intake/internal/version"
)
```

ADD the new fields to the `Deps` struct (at the bottom, before the closing brace):

```go
	// from 5-i (Phase 5):

	// CaptchaCfg is the captcha section of the loaded config. initHandler reads
	// it to decide whether to emit RequiresCaptcha + InitCaptcha hints and (with
	// CaptchaVerifier) whether to demand a captcha_token in the request body.
	CaptchaCfg config.CaptchaConfig

	// CaptchaVerifier is the verifier instance. nil → "no captcha required"
	// (initHandler treats nil + CaptchaCfg.Enabled=false the same way).
	CaptchaVerifier captcha.Verifier

	// Budget tracks the daily LLM spend. nil → no budget gate (unit tests).
	Budget *budget.Tracker

	// PerIP is the per-IP rate limiter (used by perIPLimitMiddleware in server.New).
	// nil → no per-IP gate.
	PerIP *perip.Limiter

	// TrustedProxies is the parsed CIDR list (parsed once at startup in main.go;
	// consumed by clientIPMiddleware in server.New).
	TrustedProxies []netip.Prefix
```

5-i creates stub packages `budget` and `captcha` ONLY if needed to compile. The cleanest path: import the perip package (already created above) plus two more placeholder packages so `Deps` compiles. Add these minimal placeholders:

Create `relay/internal/budget/budget.go`:

```go
// Package budget tracks daily LLM token spend for the relay. Phase 5-i
// exports the empty type as a placeholder so the server chain compiles;
// 5-ii fills in the actual Reserve/Commit implementation.
package budget

import "time"

// Tracker is the daily-budget tracker. 5-i placeholder; 5-ii implements.
type Tracker struct {
	// 5-ii populates with {in, out, max, dayStartUTC, now, mu}.
}

// New constructs a Tracker. 5-i placeholder; 5-ii implements.
// 5-i callers in main.go pass nil to Deps.Budget; do NOT call this in 5-i.
func New(maxInputTokens, maxOutputTokens int, now func() time.Time) *Tracker {
	return &Tracker{}
}

// Reserve / Commit / Snapshot: 5-ii implements. 5-i: stub methods that
// always allow (so a downstream caller's nil-check is the gate, not these).
func (t *Tracker) Reserve(tenantKey string, estIn, estOut int) (ok bool, retryAfter time.Duration) {
	return true, 0
}
func (t *Tracker) Commit(tenantKey string, actualIn, actualOut int) {}
func (t *Tracker) Snapshot(tenantKey string) (in, out int, dayStartUTC time.Time) {
	return 0, 0, time.Time{}
}
```

Create `relay/internal/captcha/captcha.go`:

```go
// Package captcha verifies CAPTCHA tokens at /v1/intake/init.
// Phase 5-i exports the Verifier interface + Stub so the server chain compiles;
// 5-iii fills in the Turnstile + hCaptcha implementations.
package captcha

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Verifier verifies a CAPTCHA token via the provider's siteverify endpoint.
// Implementations MUST scrub the secret from any returned error (L005).
type Verifier interface {
	// Verify returns (ok=true, "", nil) on a valid, single-use token.
	// On (ok=false, reason, nil): reason carries the provider's error-codes[0] —
	// never the secret. err is reserved for transport/parse failures.
	Verify(ctx context.Context, token, remoteIP string) (ok bool, reason string, err error)

	// Provider returns "turnstile" | "hcaptcha" | "stub" for logging.
	Provider() string
}

// Stub always returns ok=true. Used when captcha is disabled or when the
// current auth mode is not in required_for.
type Stub struct{}

func (Stub) Verify(context.Context, string, string) (bool, string, error) {
	return true, "", nil
}
func (Stub) Provider() string { return "stub" }

// New: 5-iii implements Turnstile + hCaptcha. 5-i: this returns an error so
// any 5-i caller is forced into the nil-CaptchaVerifier path (which initHandler
// handles as "no captcha required").
func New(provider, secret string, httpClient *http.Client, now func() time.Time) (Verifier, error) {
	return nil, errors.New("captcha.New: not implemented in 5-i; sub-plan 5-iii implements Turnstile + hCaptcha")
}
```

- [ ] **Step 4: Extend `dto.go` with the new types**

In `relay/internal/server/dto.go`, modify `Capabilities`:

```go
// Capabilities advertises relay feature flags to the widget.
type Capabilities struct {
	AuthModes       []string `json:"auth_modes"`
	Streaming       bool     `json:"streaming"`
	RequiresCaptcha []string `json:"requires_captcha,omitempty"` // 5-i: subset of AuthModes; only when captcha gates ≥1 mode
}
```

Extend `InitResponse` and ADD the new types (after `InitAuthEmail`):

```go
// InitResponse is returned by POST /v1/intake/init.
type InitResponse struct {
	SessionID    string       `json:"session_id"`
	Capabilities Capabilities `json:"capabilities"`
	Auth         *InitAuth    `json:"auth,omitempty"`
	Captcha      *InitCaptcha `json:"captcha,omitempty"` // 5-i: nil when captcha disabled
}

// InitCaptcha carries the public CAPTCHA hint so the widget can render the
// challenge. Phase 5.
type InitCaptcha struct {
	Provider string `json:"provider"`  // "turnstile" | "hcaptcha"
	SiteKey  string `json:"site_key"`  // public; safe to commit
}

// InitRequest is the body of POST /v1/intake/init. Empty in v0 except for
// the optional captcha token. Phase 5.
type InitRequest struct {
	CaptchaToken string `json:"captcha_token,omitempty"`
}

// CaptchaRequiredResponse is the 400 body shape returned when captcha is
// required for the anonymous mode but the request omitted captcha_token.
// Carries the same `capabilities` + `captcha` discovery fields as the success
// path so the widget can render the challenge without a separate call. Phase 5.
type CaptchaRequiredResponse struct {
	Error        ErrorBody    `json:"error"`
	Capabilities Capabilities `json:"capabilities"`
	Captcha      *InitCaptcha `json:"captcha,omitempty"`
}
```

- [ ] **Step 5: Update `initHandler` in `turn.go`**

In `relay/internal/server/turn.go`, replace the entire `initHandler` function with:

```go
// initHandler handles POST /v1/intake/init.
// Response: InitResponse{SessionID, Capabilities, Auth?, Captcha?}.
//
// Phase 4: AuthModes includes "anonymous"/"email"/"sso" based on cfg.Auth.Modes;
// InitResponse.Auth carries hints (currently just email.code_ttl_seconds).
//
// Phase 5 (5-i): when captcha gates one of the enabled auth modes, the response
// also carries Capabilities.RequiresCaptcha + Captcha. The body may carry
// captcha_token. If captcha is required but the token is missing, returns
// 400 captcha_required with the same discovery fields so the widget can render
// the challenge. (5-iii adds the actual verifier call.)
func initHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil {
			writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
			return
		}

		// Decode optional InitRequest. Empty body is allowed (and is the Phase 1
		// behavior); a malformed body (non-empty + bad JSON) returns 400.
		var initReq InitRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&initReq); err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: "+err.Error())
				return
			}
		}

		// Compute the auth modes list (4-ii).
		modes := make([]string, 0, 3)
		if deps.AuthCfg.Modes.Anonymous {
			modes = append(modes, "anonymous")
		}
		if deps.AuthCfg.Modes.Email {
			modes = append(modes, "email")
		}
		if deps.AuthCfg.Modes.SSO {
			modes = append(modes, "sso")
		}
		if len(modes) == 0 {
			modes = []string{"anonymous"}
		}

		// Compute the captcha discovery hint (5-i).
		var captchaHint *InitCaptcha
		var requiresCaptcha []string
		if deps.CaptchaCfg.Enabled {
			// requires_captcha = intersection(modes, deps.CaptchaCfg.RequiredFor)
			rfSet := make(map[string]struct{}, len(deps.CaptchaCfg.RequiredFor))
			for _, m := range deps.CaptchaCfg.RequiredFor {
				rfSet[m] = struct{}{}
			}
			for _, m := range modes {
				if _, ok := rfSet[m]; ok {
					requiresCaptcha = append(requiresCaptcha, m)
				}
			}
			if len(requiresCaptcha) > 0 {
				captchaHint = &InitCaptcha{
					Provider: deps.CaptchaCfg.Provider,
					SiteKey:  deps.CaptchaCfg.SiteKey,
				}
			}
		}

		// If captcha is required and the body omits captcha_token, return 400
		// captcha_required with the discovery fields (5-i shape; 5-iii adds
		// the actual Verify call when the token IS present).
		if len(requiresCaptcha) > 0 && initReq.CaptchaToken == "" {
			writeJSON(w, http.StatusBadRequest, CaptchaRequiredResponse{
				Error: ErrorBody{
					Code:    "captcha_required",
					Message: "call /init again with a solved captcha_token",
				},
				Capabilities: Capabilities{
					AuthModes:       modes,
					Streaming:       true,
					RequiresCaptcha: requiresCaptcha,
				},
				Captcha: captchaHint,
			})
			return
		}

		// 5-iii: verify the captcha token now that we know it's present and required.
		// (5-i leaves this as a one-line marker comment that 5-iii extends with the
		// actual deps.CaptchaVerifier.Verify call; the missing-token branch above
		// is what 5-i covers.)

		sessionID := deps.Auth.Store().Issue()

		resp := InitResponse{
			SessionID: sessionID,
			Capabilities: Capabilities{
				AuthModes:       modes,
				Streaming:       true,
				RequiresCaptcha: requiresCaptcha,
			},
			Captcha: captchaHint,
		}

		if deps.AuthCfg.Modes.Email {
			d, err := time.ParseDuration(deps.AuthCfg.Email.CodeTTL)
			if err != nil {
				slog.WarnContext(r.Context(), "init: auth.email.code_ttl unparseable; omitting Auth.Email hint",
					"code_ttl", deps.AuthCfg.Email.CodeTTL, "error", err)
			} else {
				resp.Auth = &InitAuth{Email: &InitAuthEmail{CodeTTLSeconds: int(d.Seconds())}}
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
```

- [ ] **Step 6: Run the failing tests — must pass now**

Run: `cd relay && go build ./... && go vet ./... && cd ..`
Expected: build + vet pass.

Run: `cd relay && go test ./internal/server/ -run TestInitHandler -v && cd ..`
Expected: both new TestInitHandler tests pass.

Run: `cd relay && go test ./... && cd ..`
Expected: ALL relay tests pass (no Phase 1+4 regressions).

- [ ] **Step 7: Commit**

```bash
git add relay/internal/server/deps.go relay/internal/server/dto.go relay/internal/server/turn.go relay/internal/server/turn_test.go relay/internal/budget/budget.go relay/internal/captcha/captcha.go
git commit -m "feat(5-i): Deps + InitResponse + initHandler — captcha discovery + 400 captcha_required"
```

---

### Task 7: Wire the new middlewares into `server.New`

**Files:** Modify `relay/internal/server/server.go`, `relay/internal/server/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/internal/server/server_test.go`:

```go
func TestServerNew_MountsClientIPMiddlewareOnIntakeGroup(t *testing.T) {
	// Build a Deps with TrustedProxies set; hit /v1/intake/init; the resolved
	// IP must be in the request context (we observe via the response body when
	// we wire a debug header through initHandler — but the simpler proof is
	// that /v1/intake/init returns 200 (no 5xx from a panicking middleware).
	// /v1/health (outside /v1/intake) should still respond 200.
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{}}}
	deps := Deps{
		Auth:    auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{Modes: config.AuthModes{Anonymous: true}},
		TrustedProxies: nil, // empty list — default behavior
		PerIP:   nil,        // nil limiter → always-allow
		Version: version.BuildInfo{Version: "test"},
	}
	srv := New(cfg, deps)

	// /v1/health is OUTSIDE /v1/intake — no rate limit, no client-IP middleware.
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/health", nil)
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("/v1/health status = %d; want 200", rec.Code)
		}
	}

	// /v1/intake/init flows through clientIPMiddleware + perIPLimitMiddleware.
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
		req.RemoteAddr = "203.0.113.10:12345"
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("/v1/intake/init status = %d; want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	}
}
```

Add imports if missing: `"strings"`, `"intake/internal/version"`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./internal/server/ -run TestServerNew_Mounts -v && cd ..`
Expected: FAIL — `TrustedProxies` field on Deps not consumed; OR the middlewares aren't actually wired; OR the test compiles but doesn't prove the wiring (depending on whether the existing server.New already routes through). Read the failure to confirm.

- [ ] **Step 3: Update `server.New` to mount the two new middlewares on the `/v1/intake` group**

In `relay/internal/server/server.go`, replace `New` with:

```go
// New constructs the relay HTTP handler (a chi Mux) with all middleware and
// built-in routes wired. Routes specific to intake sessions are registered via
// registerIntakeRoutes, which 1-iii and 1-iv extend.
//
// Phase 5 (5-i): the /v1/intake group gains two new middlewares —
// clientIPMiddleware (resolves the client IP per server.trusted_proxies) and
// perIPLimitMiddleware (per-IP token bucket; 5-i passes nil so the gate is
// inert, 5-ii lands the real Limiter). /v1/health and /v1/version stay OUTSIDE
// the /v1/intake group so liveness probes are not rate-limited.
func New(cfg *config.Config, deps Deps) http.Handler {
	r := chi.NewMux()

	// Global middleware — order matters: request-ID first, then recovery.
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(deps.CORSOrigins))

	// Built-in relay endpoints (NOT rate-limited — load-balancer liveness probes).
	r.Get("/v1/health", handleHealth)
	r.Get("/v1/version", handleVersion(deps))

	// Intake session endpoints — seam for 1-iii and 1-iv.
	// Phase 5: wrap the group with clientIP + per-IP limit middlewares.
	r.Route("/v1/intake", func(r chi.Router) {
		r.Use(clientIPMiddleware(deps.TrustedProxies))
		r.Use(perIPLimitMiddleware(deps.PerIP))
		registerIntakeRoutes(r, deps)
	})

	return r
}
```

- [ ] **Step 4: Run the test — must pass**

Run: `cd relay && go test ./internal/server/ -run TestServerNew -v && cd ..`
Expected: PASS.

Run: `cd relay && go test ./... && cd ..`
Expected: ALL relay tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/server/server.go relay/internal/server/server_test.go
git commit -m "feat(5-i): wire clientIPMiddleware + perIPLimitMiddleware on /v1/intake group"
```

---

### Task 8: Q9 consolidated startup gate in `main.go`

**Files:** Modify `relay/cmd/relay/main.go`

The Q9 gate is a self-contained block of pure code (no new packages). Test it via a thin helper that returns the `problems` slice so unit tests can assert content; main.go calls the helper then logs+exits.

- [ ] **Step 1: Write the failing test**

Create `relay/cmd/relay/main_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"intake/internal/config"
)

func TestStartupProblems_AnonymousWithoutCaptcha(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false

	problems := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1 problem", problems)
	}
	if !strings.Contains(problems[0], "anonymous") || !strings.Contains(problems[0], "captcha") {
		t.Errorf("problem %q does not mention both 'anonymous' and 'captcha'", problems[0])
	}
}

func TestStartupProblems_AnonymousWithCaptchaButNotRequiredForAnonymous(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = true
	cfg.Captcha.RequiredFor = []string{"email"} // missing "anonymous"
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false

	problems := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1 problem (required_for excludes anonymous)", problems)
	}
}

func TestStartupProblems_AllowWithoutCaptchaSilencesAnonymousGate(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = true

	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject" // satisfy Validate

	problems := startupProblems(cfg)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty (escape hatch engaged)", problems)
	}
}

func TestStartupProblems_SSOBothSet(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.SSO = true
	cfg.Auth.SSO.JWKSURL = "https://example/.well-known/jwks.json"
	cfg.Auth.SSO.HS256SecretEnv = "INTAKE_SSO_HS256"
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"

	problems := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "both") {
		t.Errorf("sso-both problem %q does not say 'both'", problems[0])
	}
}

func TestStartupProblems_SSONeitherSet(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.SSO = true
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"

	problems := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "neither") {
		t.Errorf("sso-neither problem %q does not say 'neither'", problems[0])
	}
}

func TestStartupProblems_BadCIDR(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.0/8", "not-a-cidr"}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"

	problems := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "trusted_proxies") || !strings.Contains(problems[0], "not-a-cidr") {
		t.Errorf("bad-CIDR problem %q does not mention the bad value", problems[0])
	}
}

func TestStartupProblems_BadActionOnExceeded(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "queue" // v0 only supports "reject"

	problems := startupProblems(cfg)
	if len(problems) == 0 {
		t.Fatal("problems is empty; want a problem for action_on_exceeded=queue")
	}
	found := false
	for _, p := range problems {
		if strings.Contains(p, "action_on_exceeded") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("problems %v does not contain an action_on_exceeded entry", problems)
	}
}

func TestStartupProblems_AllFourMisconfigsAtOnce(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	cfg.Auth.Modes.SSO = true
	cfg.Auth.SSO.JWKSURL = "https://example/jwks"
	cfg.Auth.SSO.HS256SecretEnv = "INTAKE_SSO_HS256"
	cfg.Server.TrustedProxies = []string{"not-a-cidr"}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "queue"

	problems := startupProblems(cfg)
	if len(problems) != 4 {
		t.Errorf("problems len = %d; want 4 (anonymous-no-captcha + sso-both + bad-CIDR + bad-action)\nproblems: %v", len(problems), problems)
	}
}

func TestStartupProblems_CleanConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	// All other Phase 4/5 gate inputs default to safe values.
	problems := startupProblems(cfg)
	if len(problems) != 0 {
		t.Errorf("clean config problems = %v; want empty", problems)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./cmd/relay/... -v && cd ..`
Expected: FAIL — `startupProblems undefined`.

- [ ] **Step 3: Add `startupProblems` to `main.go`**

In `relay/cmd/relay/main.go`, ADD a helper function (place it AFTER `licensed` near the bottom of the file):

```go
// startupProblems enumerates every Phase 4 + Phase 5 misconfig in cfg as a flat
// []string. main() logs the slice in one structured Error line and exits 1 when
// non-empty (PROJECT.md §19 Q9 fail-closed; PHASE_PLANNING §4 build-fail discipline).
//
// Each problem string is self-describing — names the offending key, the value
// it found, and the fix. Order: anonymous gate, SSO mutual-exclusivity,
// trusted-proxy CIDR parse, config.Validate (currently action_on_exceeded).
func startupProblems(cfg *config.Config) []string {
	var problems []string

	// Q9: anonymous-without-captcha-gating.
	if cfg.Auth.Modes.Anonymous {
		anonymousProtected := cfg.Captcha.Enabled && containsString(cfg.Captcha.RequiredFor, "anonymous")
		if !anonymousProtected && !cfg.Auth.Anonymous.AllowWithoutCaptcha {
			problems = append(problems, `auth.modes.anonymous=true requires captcha.enabled=true AND captcha.required_for to include "anonymous"; or set auth.anonymous.allow_without_captcha=true to acknowledge the risk (PROJECT.md §19 Q9)`)
		}
	}

	// SSO mutual-exclusivity (Phase 4; consolidated here so all gates fire in one pass).
	if cfg.Auth.Modes.SSO {
		jwks := cfg.Auth.SSO.JWKSURL != ""
		hs := cfg.Auth.SSO.HS256SecretEnv != ""
		if jwks && hs {
			problems = append(problems, "auth.modes.sso=true: both jwks_url and hs256_secret_env are set; exactly one required")
		}
		if !jwks && !hs {
			problems = append(problems, "auth.modes.sso=true: neither jwks_url nor hs256_secret_env is set; exactly one required")
		}
	}

	// Trusted-proxy CIDRs — fatal at startup, not at first request.
	for _, raw := range cfg.Server.TrustedProxies {
		if _, err := netip.ParsePrefix(raw); err != nil {
			problems = append(problems, fmt.Sprintf("server.trusted_proxies contains an invalid CIDR %q: %v", raw, err))
		}
	}

	// Config-level validation (action_on_exceeded etc.).
	if err := cfg.Validate(); err != nil {
		problems = append(problems, err.Error())
	}

	return problems
}

// containsString reports whether haystack contains needle (case-sensitive).
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
```

Add the imports `"fmt"` (already present) and `"net/netip"` at the top of `main.go`:

```go
import (
	// ... existing imports ...
	"net/netip"
)
```

- [ ] **Step 4: Wire the gate into `main()` itself**

In `main.go`, find the existing block right after `licState, err := licensemgr.Load(...)` (before `--- LLM Provider ---`). INSERT this block:

```go
	// --- Q9 consolidated startup gate (Phase 5) ---
	// All Phase 4 + Phase 5 misconfigs collected into one structured Error line.
	// Operators fix every problem in one restart cycle, not three.
	if problems := startupProblems(cfg); len(problems) > 0 {
		logger.Error("relay: startup config errors", "count", len(problems), "problems", problems)
		os.Exit(1)
	}
```

- [ ] **Step 5: Run all main_test.go tests — must pass**

Run: `cd relay && go test ./cmd/relay/... -v && cd ..`
Expected: all 9 startupProblems tests pass.

Run: `cd relay && go build ./... && cd ..`
Expected: build succeeds (the new `netip` import is the only addition).

- [ ] **Step 6: Commit**

```bash
git add relay/cmd/relay/main.go relay/cmd/relay/main_test.go
git commit -m "feat(5-i): Q9 consolidated startup gate (anon-no-captcha + sso-both/neither + bad-CIDR + bad-action_on_exceeded)"
```

---

### Task 9: Wire the new Deps fields in `main.go` + switch to `NewMiddlewareWithModes`

**Files:** Modify `relay/cmd/relay/main.go`

- [ ] **Step 1: Update the `auth.NewMiddleware` call to use the wider constructor**

In `main.go`, find this line (~line 186):

```go
	middleware := auth.NewMiddleware(store, emailVerifier, ssoVerifier)
```

Replace with:

```go
	// Phase 5: switch to the modes-aware constructor; modesAnonymous is read directly
	// from cfg so the dispatcher rejects anonymous when the operator disabled the mode.
	middleware := auth.NewMiddlewareWithModes(store, emailVerifier, ssoVerifier, cfg.Auth.Modes.Anonymous)
```

- [ ] **Step 2: Parse `cfg.Server.TrustedProxies` into `[]netip.Prefix` once at startup**

After the Q9 gate (which already validated the CIDRs), parse them. ADD this block right after the Q9 gate:

```go
	// Parse trusted-proxy CIDRs once (Q9 gate already validated them; this just rebuilds the prefix list for the middleware).
	trustedProxies := make([]netip.Prefix, 0, len(cfg.Server.TrustedProxies))
	for _, raw := range cfg.Server.TrustedProxies {
		p, _ := netip.ParsePrefix(raw) // already validated; ignore err
		trustedProxies = append(trustedProxies, p)
	}
```

- [ ] **Step 3: Populate the new Deps fields**

Find the existing `deps := server.Deps{...}` block (around line 232) and ADD the new fields at the end (before the closing brace):

```go
	deps := server.Deps{
		Version:      version.Info(),
		CORSOrigins:  cfg.Server.CORSOrigins,
		Logger:       logger,
		Auth:         middleware,
		Provider:     provider,
		SystemPrompt: systemPrompt,
		Model:        model,
		MaxTokens:    maxTokens,
		Router:       rtr,
		Classifier:   classifier,
		Builder:      builder,
		AuthCfg:      cfg.Auth,
		EmailService: emailSvc,

		// Phase 5 (5-i): config + parsed prefixes wired in; the actual
		// Limiter, Budget, and CaptchaVerifier are nil here — 5-ii and 5-iii
		// replace nil with the real instances.
		CaptchaCfg:      cfg.Captcha,
		CaptchaVerifier: nil,
		Budget:          nil,
		PerIP:           nil,
		TrustedProxies:  trustedProxies,
	}
```

- [ ] **Step 4: Build + vet + full test suite**

Run: `cd relay && go build ./... && go vet ./... && cd ..`
Expected: build + vet pass.

Run: `cd relay && go test ./... && cd ..`
Expected: ALL relay tests pass.

Run: `bash scripts/verify-contract.sh`
Expected: exits 0 (Phase 0 contract gate unchanged).

Run: `bash scripts/check-pins.sh`
Expected: exits 0.

- [ ] **Step 5: Commit**

```bash
git add relay/cmd/relay/main.go
git commit -m "feat(5-i): wire Deps.{CaptchaCfg,TrustedProxies}; switch to NewMiddlewareWithModes"
```

---

## Smoke (mandatory)

**Self-runnable; no LLM credit; no maintainer pause.** Proves the 5-i seam works end-to-end.

1. **Q9 gate fires on each misconfig and the combined case.**

   Author four tiny YAML fixtures plus one combined fixture under `relay/cmd/relay/smoke/` (gitignored by `.gitignore` at the repo root if not already):

   - `anonymous-no-captcha.yaml`: `auth.modes.anonymous: true`, `captcha.enabled: false`, no `allow_without_captcha`.
   - `sso-both.yaml`: `auth.modes.sso: true`, `auth.sso.jwks_url: "x"`, `auth.sso.hs256_secret_env: "y"`.
   - `sso-neither.yaml`: `auth.modes.sso: true`, no `jwks_url` or `hs256_secret_env`.
   - `bad-cidr.yaml`: `server.trusted_proxies: ["not-a-cidr"]`.
   - `combined.yaml`: all four misconfigs at once.

   For each: `cd relay && go run ./cmd/relay --config smoke/<file>.yaml ; echo $?`
   Expected (Linux/macOS): exit code 1. On Windows PowerShell: `cd relay; go run ./cmd/relay --config smoke/<file>.yaml; $LASTEXITCODE` → 1.

   For the combined case: assert the single Error log line `"relay: startup config errors"` lists all 4 problems (substring match: each problem text appears once).

   Use `-Encoding ascii` (PowerShell) per L010 when authoring the YAMLs.

2. **Clean config starts and responds 200 on /v1/health.**

   Create `relay/cmd/relay/smoke/clean.yaml`:
   ```yaml
   server:
     addr: ":18080"
     external_url: "http://127.0.0.1:18080"
     cors_origins: ["http://localhost:5173"]
   auth:
     modes:
       anonymous: true
     anonymous:
       allow_without_captcha: true   # acknowledge risk for smoke
   ratelimit:
     daily_llm_budget:
       action_on_exceeded: "reject"
   ```
   Run: `cd relay && go run ./cmd/relay --config smoke/clean.yaml &`
   In another shell: `curl -s http://127.0.0.1:18080/v1/health` → `{"status":"ok"}`.
   Stop with Ctrl-C / `taskkill /F`.

3. **`auth.modes.anonymous=false` + valid X-Intake-Session returns 401.**

   Create `relay/cmd/relay/smoke/strict-anonymous.yaml`: same as `clean.yaml` but `auth.modes.anonymous: false` and `auth.anonymous.allow_without_captcha: true`. Start the relay; `curl -s -X POST -H "X-Intake-Session: any-uuid" http://127.0.0.1:18080/v1/intake/turn -d '{}'` → 401 with `unauthorized` code. The Q9 gate does NOT fire (anonymous is false, so the gate is vacuously satisfied).

4. **`/v1/health` is NOT rate-limited even when the per-IP limiter is wired.** Phase 5-ii will verify the full rate-limit smoke; 5-i pre-smokes this by confirming the chain order — see Task 7's TestServerNew_MountsClientIPMiddlewareOnIntakeGroup unit test.

## Done criteria

- [ ] All 9 tasks complete and committed.
- [ ] `cd relay && go build ./... && go vet ./...` clean.
- [ ] `cd relay && go test ./...` green (all Phase 1+4 tests + the new Phase 5-i tests pass; no skipped tests).
- [ ] `bash scripts/verify-contract.sh` green.
- [ ] `bash scripts/check-pins.sh` green (includes the new `golang.org/x/time` check).
- [ ] `cd relay && go mod tidy` is a no-op (no spurious dep changes).
- [ ] All five smoke steps pass.
- [ ] L015 helper (`requireFullSessionContext`) called from at least one new dispatcher test added in Task 5.
- [ ] The Phase 4 frozen seams are byte-equivalent in shape: `auth.Middleware.Handler`, `auth.SessionContext`, `auth.WithSession`/`FromContext`, `adapter.Adapter`. (Only field additions on `Middleware` + a new constructor.)
- [ ] No new external Go module in `go.mod` (only an indirect→direct promotion).
