# 3-i Adapter Config + Registry + Router — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the Phase-3 config blocks (`routing`, `license`, and the four adapter sub-structs) to `config`, create the `router` package that resolves a payload to one adapter (`routing_hint` → rules → default), and migrate the server from a single `Deps.Adapter` to `Deps.Router`. After this sub-plan the relay still boots and submits through the **webhook** adapter — now via the router — with no new external dependency.

**Architecture:** A new `relay/internal/router` package owns routing logic (a `Router` holding a registry `map[string]adapter.Adapter`, ordered rules, and a default name). `router.New` validates the default names a registered adapter (else error → fatal at startup) and drops rules that name an unregistered adapter (with a warning → graceful free-mode). `server.Deps.Adapter` becomes `Deps.Router`; `submit.go` calls `deps.Router.Route(p)` then `.Create`. `main.go` builds the registry from enabled adapters (only `webhook` exists this sub-plan; 3-ii…3-v add the rest) and constructs the router.

**Tech Stack:** Go 1.23.2 (relay). No new dependencies. `gopkg.in/yaml.v3` (already present) for the `StringList` custom unmarshaler.

---

## Design References

- README §8.3 — config sub-struct shapes (verbatim, frozen here)
- README §8.4 — `router.Router` signature + Route order (verbatim, frozen here)
- Design spec §2.1 (routing seam), §4.4 (routing-vs-gate interaction — the default-fatal/rule-drop rules)
- PROJECT.md §8 (adapter resolution), §9 (routing config example)
- Reference impl: `relay/internal/adapter/webhook/webhook.go`, `relay/internal/server/submit.go`, `relay/internal/server/submit_test.go`

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/config/config.go` | Modify | Add `RoutingConfig`/`Rule`/`RuleMatch`/`StringList`, `LicenseConfig`, 4 adapter sub-structs; add `Routing`+`License` to `Config`, 4 blocks to `AdaptersConfig`; default `routing.default_adapter` |
| `relay/internal/config/config_test.go` | Modify | Tests: routing+adapter blocks parse; `StringList` scalar+sequence; default_adapter default |
| `relay/internal/config/testdata/sample.yaml` | Modify | Add `routing` + `adapters.{chatwoot,fider,zendesk,linear}` + `license` blocks |
| `relay/internal/router/router.go` | Create | `Rule`, `Router`, `New`, `Route` |
| `relay/internal/router/router_test.go` | Create | routing_hint / rule / default / dangling-default-error / dangling-rule-dropped |
| `relay/internal/server/deps.go` | Modify | Replace `Adapter adapter.Adapter` with `Router *router.Router` |
| `relay/internal/server/submit.go` | Modify | `deps.Router.Route(p)` then `.Create` |
| `relay/internal/server/submit_test.go` | Modify | Wrap fake/webhook adapter in a `router.Router` |
| `relay/cmd/relay/main.go` | Modify | `buildRegistry` (webhook only); resolve rules; `router.New`; `deps.Router` |

---

## Tasks

### Task 1: Add the Phase-3 config structs + StringList + defaults

**Files:** Modify `relay/internal/config/config.go`, `relay/internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/config/config_test.go` (after the last existing test's closing `}`):

```go
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
```

- [ ] **Step 2: Run to verify they fail**

```
cd C:/src/ai/intake/relay && go test ./internal/config/... -run "TestLoad_AppliesDefaultRoutingAdapter|TestLoad_StringList_ScalarAndSequence|TestLoad_ParsesAdapterBlocks" -v
```

Expected: compile errors (`cfg.Routing undefined`, `cfg.Adapters.Chatwoot undefined`, etc.). MUST fail before proceeding.

- [ ] **Step 3: Add the structs to `config.go`**

In `relay/internal/config/config.go`, add `"gopkg.in/yaml.v3"` to imports if not already imported for this (it is imported). Add the `Routing` and `License` fields to `Config`:

```go
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	LLM      LLMConfig      `yaml:"llm"`
	Auth     AuthConfig     `yaml:"auth"`
	Adapters AdaptersConfig `yaml:"adapters"`
	Routing  RoutingConfig  `yaml:"routing"`
	License  LicenseConfig  `yaml:"license"`
}
```

Replace the `AdaptersConfig` struct with:

```go
// AdaptersConfig holds per-adapter configuration. Webhook is Phase 1; the other
// four are added in Phase 3 (3-ii…3-v construct them; their config is read here).
type AdaptersConfig struct {
	Webhook  WebhookConfig  `yaml:"webhook"`
	Chatwoot ChatwootConfig `yaml:"chatwoot"`
	Fider    FiderConfig    `yaml:"fider"`
	Zendesk  ZendeskConfig  `yaml:"zendesk"`
	Linear   LinearConfig   `yaml:"linear"`
}
```

Add these new types (place them after `RetryConfig`):

```go
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
```

- [ ] **Step 4: Add the default in `applyDefaults`**

In `applyDefaults`, before the webhook retry defaults, add:

```go
	if c.Routing.DefaultAdapter == "" {
		c.Routing.DefaultAdapter = "chatwoot"
	}
```

- [ ] **Step 5: Run the new tests (they should pass once sample.yaml is updated in Task 2)**

For now run only the default test, which uses `minimal.yaml` and needs no sample change:

```
cd C:/src/ai/intake/relay && go test ./internal/config/... -run "TestLoad_AppliesDefaultRoutingAdapter" -v
```

Expected: PASS. (The other two depend on Task 2's sample.yaml; they will still fail to find rules/blocks until then — that is expected.)

- [ ] **Step 6: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 7: Commit**

```
cd C:/src/ai/intake/relay && git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add routing/license/adapter config blocks + StringList (3-i)"
```

---

### Task 2: Extend testdata/sample.yaml with routing + adapter + license blocks

**Files:** Modify `relay/internal/config/testdata/sample.yaml`

- [ ] **Step 1: Append the new blocks to `testdata/sample.yaml`**

Append (do not remove existing content) to `relay/internal/config/testdata/sample.yaml`:

```yaml
routing:
  default_adapter: "chatwoot"
  rules:
    - when:
        classification: "bug"
      to: "chatwoot"
    - when:
        classification: ["question", "other"]
      to: "chatwoot"
license:
  file: "/etc/intake/license.json"
```

Then, under the existing `adapters:` mapping (which currently has only `webhook:`), add the four blocks as siblings of `webhook:` (same indentation as `webhook:`):

```yaml
  chatwoot:
    enabled: true
    base_url: "https://chatwoot.example.com"
    account_id: 1
    inbox_id: 3
    api_token_env: "CHATWOOT_TOKEN"
  fider:
    enabled: true
    base_url: "https://feedback.example.com"
    api_key_env: "FIDER_API_KEY"
  zendesk:
    enabled: false
    subdomain: "example"
    email: "agent@example.com"
    api_token_env: "ZENDESK_API_TOKEN"
    default_priority: "normal"
  linear:
    enabled: false
    api_key_env: "LINEAR_API_KEY"
    team_id: "TEAM_ID"
```

> If `sample.yaml` has no `adapters:` key yet (it may only contain `webhook` under a top-level `adapters:`), confirm by reading the file first; the four blocks must be nested under the existing `adapters:` mapping at the same level as `webhook:`.

- [ ] **Step 2: Run the config tests**

```
cd C:/src/ai/intake/relay && go test ./internal/config/... -v
```

Expected: ALL config tests pass, including `TestLoad_StringList_ScalarAndSequence` and `TestLoad_ParsesAdapterBlocks`.

- [ ] **Step 3: Commit**

```
cd C:/src/ai/intake/relay && git add internal/config/testdata/sample.yaml
git commit -m "test(config): sample.yaml routing+adapter+license blocks (3-i)"
```

---

### Task 3: Create the `router` package (TDD)

**Files:** Create `relay/internal/router/router_test.go`, then `relay/internal/router/router.go`

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/router/router_test.go`:

```go
package router_test

import (
	"strings"
	"testing"
	"time"

	"intake/internal/payload"
	"intake/internal/router"

	"intake/internal/adapter"
	"context"
)

// stubAdapter is a no-op adapter with a fixed name.
type stubAdapter struct{ name string }

func (s *stubAdapter) Name() string                       { return s.name }
func (s *stubAdapter) RequiresLicense() bool              { return false }
func (s *stubAdapter) Configure(map[string]any) error     { return nil }
func (s *stubAdapter) HealthCheck(context.Context) error  { return nil }
func (s *stubAdapter) Create(context.Context, *payload.IntakePayload) (*adapter.CreateResult, error) {
	return &adapter.CreateResult{AdapterName: s.name}, nil
}

func reg(names ...string) map[string]adapter.Adapter {
	m := make(map[string]adapter.Adapter, len(names))
	for _, n := range names {
		m[n] = &stubAdapter{name: n}
	}
	return m
}

// mkPayload builds a minimal payload with the given classification/severity/hint.
func mkPayload(class, sev string, hint *string) *payload.IntakePayload {
	return &payload.IntakePayload{
		RoutingHint: hint,
		Conversation: payload.Conversation{
			Classification: payload.ConversationClassification(class),
			SeverityGuess:  payload.ConversationSeverityGuess(sev),
			Messages:       []payload.Message{{Role: "user", Content: "x", Ts: time.Now()}},
		},
	}
}

func ptr(s string) *string { return &s }

func TestNew_DefaultNotRegistered_Errors(t *testing.T) {
	_, err := router.New(reg("chatwoot"), nil, "zendesk", nil)
	if err == nil {
		t.Fatal("expected error when default_adapter names an unregistered adapter")
	}
	if !strings.Contains(err.Error(), "zendesk") {
		t.Errorf("error should name the bad default; got %v", err)
	}
}

func TestNew_DanglingRuleDropped(t *testing.T) {
	// Rule points at zendesk (not registered) — must be dropped, New still succeeds.
	rules := []router.Rule{{Classification: []string{"bug"}, To: "zendesk"}}
	r, err := router.New(reg("chatwoot"), rules, "chatwoot", nil)
	if err != nil {
		t.Fatalf("New errored on a dangling rule (should drop+warn): %v", err)
	}
	// A bug payload should now fall through to the default (chatwoot), not zendesk.
	ad, err := r.Route(mkPayload("bug", "high", nil))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if ad.Name() != "chatwoot" {
		t.Errorf("dangling rule not dropped: routed to %q; want chatwoot", ad.Name())
	}
}

func TestRoute_RoutingHintWins(t *testing.T) {
	r, _ := router.New(reg("chatwoot", "fider"), nil, "chatwoot", nil)
	ad, err := r.Route(mkPayload("bug", "high", ptr("fider")))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if ad.Name() != "fider" {
		t.Errorf("routing_hint ignored: got %q; want fider", ad.Name())
	}
}

func TestRoute_UnknownHintFallsThrough(t *testing.T) {
	r, _ := router.New(reg("chatwoot"), nil, "chatwoot", nil)
	ad, err := r.Route(mkPayload("bug", "high", ptr("nonexistent")))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if ad.Name() != "chatwoot" {
		t.Errorf("unknown hint should fall through to default; got %q", ad.Name())
	}
}

func TestRoute_RuleMatch(t *testing.T) {
	rules := []router.Rule{
		{Classification: []string{"feature_request"}, To: "fider"},
		{Classification: []string{"bug"}, To: "chatwoot"},
	}
	r, _ := router.New(reg("chatwoot", "fider"), rules, "chatwoot", nil)
	ad, _ := r.Route(mkPayload("feature_request", "low", nil))
	if ad.Name() != "fider" {
		t.Errorf("rule match failed: got %q; want fider", ad.Name())
	}
}

func TestRoute_SeverityAndWildcard(t *testing.T) {
	// Rule matches any classification but only critical severity.
	rules := []router.Rule{{Severity: []string{"critical"}, To: "fider"}}
	r, _ := router.New(reg("chatwoot", "fider"), rules, "chatwoot", nil)
	if ad, _ := r.Route(mkPayload("bug", "critical", nil)); ad.Name() != "fider" {
		t.Errorf("severity wildcard-classification rule failed: got %q; want fider", ad.Name())
	}
	if ad, _ := r.Route(mkPayload("bug", "low", nil)); ad.Name() != "chatwoot" {
		t.Errorf("non-critical should fall to default; got %q", ad.Name())
	}
}

func TestRoute_DefaultFallback(t *testing.T) {
	r, _ := router.New(reg("chatwoot"), nil, "chatwoot", nil)
	ad, _ := r.Route(mkPayload("other", "unknown", nil))
	if ad.Name() != "chatwoot" {
		t.Errorf("default fallback failed: got %q", ad.Name())
	}
}
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/router/... -v
```

Expected: `no required module provides package intake/internal/router`. MUST fail.

- [ ] **Step 3: Create `router.go`**

Create `relay/internal/router/router.go`:

```go
// Package router resolves a canonical payload to exactly one downstream adapter.
// Resolution order (PROJECT.md §8): routing_hint → first matching rule → default.
// The router holds a registry of the enabled+permitted adapters built in main.go;
// the license gate (3-vi) decides which paid adapters make it into that registry.
package router

import (
	"fmt"
	"log/slog"

	"intake/internal/adapter"
	"intake/internal/payload"
)

// Rule is the resolved form of config.Rule. Empty Classification/Severity = wildcard.
type Rule struct {
	Classification []string
	Severity       []string
	To             string
}

// Router selects an adapter for a payload.
type Router struct {
	registry map[string]adapter.Adapter
	rules    []Rule
	def      string
	logger   *slog.Logger
}

// New builds a Router. It errors if defaultName does not name a registered adapter
// (a relay with a broken default is useless — fatal at startup). Rules whose To
// names an unregistered adapter are dropped with a warning (graceful free-mode).
func New(registry map[string]adapter.Adapter, rules []Rule, defaultName string, logger *slog.Logger) (*Router, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if _, ok := registry[defaultName]; !ok {
		return nil, fmt.Errorf("router: default_adapter %q is not a registered/enabled adapter", defaultName)
	}
	kept := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if _, ok := registry[r.To]; !ok {
			logger.Warn("router: dropping rule targeting unregistered adapter",
				"to", r.To, "reason", "adapter not enabled or not licensed")
			continue
		}
		kept = append(kept, r)
	}
	return &Router{registry: registry, rules: kept, def: defaultName, logger: logger}, nil
}

// Route resolves p to one registered adapter. Never returns (nil, nil).
func (r *Router) Route(p *payload.IntakePayload) (adapter.Adapter, error) {
	// 1. routing_hint, if it names a registered adapter.
	if p.RoutingHint != nil && *p.RoutingHint != "" {
		if ad, ok := r.registry[*p.RoutingHint]; ok {
			return ad, nil
		}
	}
	// 2. first matching rule.
	class := string(p.Conversation.Classification)
	sev := string(p.Conversation.SeverityGuess)
	for _, rule := range r.rules {
		if matches(rule.Classification, class) && matches(rule.Severity, sev) {
			return r.registry[rule.To], nil // guaranteed registered (dangling dropped in New)
		}
	}
	// 3. default (guaranteed registered by New).
	if ad, ok := r.registry[r.def]; ok {
		return ad, nil
	}
	return nil, fmt.Errorf("router: no adapter resolved and default %q missing", r.def)
}

// matches reports whether want is empty (wildcard) or contains got.
func matches(want []string, got string) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		if w == got {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the router tests**

```
cd C:/src/ai/intake/relay && go test ./internal/router/... -v
```

Expected: all router tests PASS.

- [ ] **Step 5: Commit**

```
cd C:/src/ai/intake/relay && git add internal/router/router.go internal/router/router_test.go
git commit -m "feat(router): payload→adapter resolution (hint→rules→default) (3-i)"
```

---

### Task 4: Migrate the server from Deps.Adapter to Deps.Router

**Files:** Modify `relay/internal/server/deps.go`, `relay/internal/server/submit.go`, `relay/internal/server/submit_test.go`

- [ ] **Step 1: Confirm the only references to `deps.Adapter`**

```
cd C:/src/ai/intake/relay && grep -rn "\.Adapter\b\|Adapter:" internal/server cmd
```

Expected references: `deps.go` (field), `submit.go` (`deps.Adapter.Create`, `deps.Adapter.Name()`), `submit_test.go` (`Adapter:` in two Deps literals + the integration test), `main.go` (`Adapter: wh`). No others. If `turn.go`/`turn_test.go` reference it, stop and reassess.

- [ ] **Step 2: Update `deps.go`**

In `relay/internal/server/deps.go`, change the import block to add `router` and drop nothing (keep `adapter` import — `Deps` no longer needs it; remove the `adapter` import if it becomes unused). Replace the `Adapter` field:

```go
	// from 1-iv, generalized in 3-i:

	// Router resolves a submission to one downstream adapter (routing_hint→rules→default).
	Router *router.Router
```

Add the import `"intake/internal/router"`. Remove `"intake/internal/adapter"` from `deps.go` imports if no longer referenced there (it is only referenced by the old `Adapter` field). Run `go build` in step 5 to confirm.

- [ ] **Step 3: Update `submit.go`**

In `relay/internal/server/submit.go`, replace the "Dispatch to adapter" block (the `result, err := deps.Adapter.Create(ctx, p)` section) with:

```go
		// Resolve the adapter for this submission, then dispatch.
		ad, err := deps.Router.Route(p)
		if err != nil {
			slog.ErrorContext(ctx, "router: no adapter resolved", "error", err)
			writeError(w, http.StatusBadGateway, "adapter_error", "no adapter available")
			return
		}

		result, err := ad.Create(ctx, p)
		if err != nil {
			// Log full detail server-side (may include URLs/responses); client gets opaque message.
			slog.ErrorContext(ctx, "adapter create failed", "adapter", ad.Name(), "error", err)
			writeError(w, http.StatusBadGateway, "adapter_error", "downstream adapter unavailable")
			return
		}
```

- [ ] **Step 4: Update `submit_test.go`**

In `relay/internal/server/submit_test.go`:

1. Add imports: `"intake/internal/router"` (and keep `"intake/internal/adapter"`).
2. In `buildSubmitDeps`, replace `Adapter: fa,` with a router wrapping `fa`:

```go
func buildSubmitDeps(fa adapter.Adapter) server.Deps {
	store := auth.NewStore()
	mw := auth.NewMiddleware(store)
	provider := &fakeProviderSubmit{response: submitClassifyJSON}
	classifier := classify.New(provider, "claude-sonnet-4-6", 512)
	builder := payloadbuild.New("0.1.0")
	rtr, err := router.New(map[string]adapter.Adapter{fa.Name(): fa}, nil, fa.Name(), nil)
	if err != nil {
		panic("buildSubmitDeps: router.New: " + err.Error())
	}
	return server.Deps{
		Auth:       mw,
		Router:     rtr,
		Classifier: classifier,
		Builder:    builder,
	}
}
```

3. In `TestSubmitHandler_IntegrationWithHttptestWebhook`, replace the `deps := server.Deps{ ... Adapter: wh ... }` literal with a router wrapping `wh`:

```go
	rtr, err := router.New(map[string]adapter.Adapter{wh.Name(): wh}, nil, wh.Name(), nil)
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	deps := server.Deps{
		Auth:       mw,
		Router:     rtr,
		Classifier: classifier,
		Builder:    builder,
	}
```

(`wh.Name()` is `"webhook"`.)

- [ ] **Step 5: Build + test the server package**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go test ./internal/server/... -v
```

Expected: `BUILD_OK` and all server tests PASS (`TestSubmitHandler_HappyPath`, `_MissingSession`, `_IntegrationWithHttptestWebhook`). If `go build` complains `adapter imported and not used` in `deps.go`, remove that import.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/server/deps.go internal/server/submit.go internal/server/submit_test.go
git commit -m "refactor(server): Deps.Adapter -> Deps.Router; submit dispatches via router (3-i)"
```

---

### Task 5: Rewire main.go to build the registry + router

**Files:** Modify `relay/cmd/relay/main.go`

In 3-i only the **webhook** adapter exists, so `buildRegistry` registers webhook. Sub-plans 3-ii…3-v each add their adapter's branch to `buildRegistry`; 3-vi adds the license gate to it.

- [ ] **Step 1: Confirm current build**

```
cd C:/src/ai/intake/relay && go build ./cmd/relay/... && echo PRE_OK
```

Expected: `PRE_OK`.

- [ ] **Step 2: Replace the webhook+Deps section of `main.go`**

In `relay/cmd/relay/main.go`:

1. Add imports: `"intake/internal/adapter"` and `"intake/internal/router"` (keep `"intake/internal/adapter/webhook"`).
2. Replace the `// --- Webhook Adapter (1-iv) ---` block (the `wh := webhook.New()` … `wh.Configure` block) **and** the `Adapter: wh,` line in the `deps` literal with the registry+router wiring below. Insert the registry/router construction before the `deps :=` literal:

```go
	// --- Adapter registry (3-i; 3-ii…3-v add adapters; 3-vi adds the license gate) ---
	registry, err := buildRegistry(cfg, logger)
	if err != nil {
		logger.Error("relay: adapter registry build failed", "error", err)
		os.Exit(1)
	}
	if len(registry) == 0 {
		logger.Error("relay: no adapters enabled — enable at least one in config.adapters")
		os.Exit(1)
	}

	// --- Router (3-i) ---
	rules := make([]router.Rule, 0, len(cfg.Routing.Rules))
	for _, rc := range cfg.Routing.Rules {
		rules = append(rules, router.Rule{
			Classification: []string(rc.When.Classification),
			Severity:       []string(rc.When.Severity),
			To:             rc.To,
		})
	}
	rtr, err := router.New(registry, rules, cfg.Routing.DefaultAdapter, logger)
	if err != nil {
		logger.Error("relay: router init failed", "default_adapter", cfg.Routing.DefaultAdapter, "error", err)
		os.Exit(1)
	}
	logger.Info("relay: router ready", "default_adapter", cfg.Routing.DefaultAdapter, "adapters", adapterNames(registry))
```

Change the `deps` literal field from `Adapter: wh,` to `Router: rtr,`.

3. Add these helpers at the end of the file:

```go
// buildRegistry constructs the set of enabled adapters. Each Phase-3 adapter
// sub-plan (3-ii…3-v) adds its block here; 3-vi wraps paid adapters with the
// license gate. Tokens resolve via config.ResolveSecret and are passed into
// Configure — never read from the environment by the adapter, never logged.
func buildRegistry(cfg *config.Config, logger *slog.Logger) (map[string]adapter.Adapter, error) {
	reg := make(map[string]adapter.Adapter)

	// webhook (1-iv) — free.
	if cfg.Adapters.Webhook.Enabled {
		wh := webhook.New()
		if err := wh.Configure(map[string]any{
			"url":     cfg.Adapters.Webhook.URL,
			"headers": cfg.Adapters.Webhook.Headers,
			"retry": map[string]any{
				"max_attempts": cfg.Adapters.Webhook.Retry.MaxAttempts,
				"backoff":      cfg.Adapters.Webhook.Retry.Backoff,
			},
		}); err != nil {
			return nil, fmt.Errorf("webhook adapter: %w", err)
		}
		reg[wh.Name()] = wh
		logger.Info("relay: adapter enabled", "adapter", wh.Name())
	}

	// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.

	return reg, nil
}

// adapterNames returns the sorted-insertion-order keys of the registry for logging.
func adapterNames(reg map[string]adapter.Adapter) []string {
	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}
	return names
}
```

4. Add `"fmt"` to the imports (used by `buildRegistry`).

- [ ] **Step 3: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. If `webhook` import is reported unused, it means buildRegistry wasn't added correctly.

- [ ] **Step 4: Full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK` (config, router, server, providers, anthropic, etc. all `ok`).

- [ ] **Step 5: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "refactor(main): build adapter registry + router; drop direct webhook wiring (3-i)"
```

---

### Task 6: Final verification gate

- [ ] **Step 1: Full build + vet + test**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected: `BUILD_OK`, `VET_OK`, every package `ok`, `TEST_OK`.

- [ ] **Step 2: Contract gate + pin gate**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK`.

- [ ] **Step 3: Confirm no new external dependency**

```
cd C:/src/ai/intake/relay && go mod tidy && git diff --exit-code go.mod go.sum && echo MOD_CLEAN
```

Expected: `MOD_CLEAN` (no changes). If `go.mod`/`go.sum` changed, a non-stdlib import sneaked in — investigate and remove.

- [ ] **Step 4: Build-fail self-check (README §6)**

- `router.New` errors on an unregistered default → `TestNew_DefaultNotRegistered_Errors`. ✓
- dangling rule dropped (not fatal) → `TestNew_DanglingRuleDropped`. ✓
- `Route` never returns (nil,nil) → default branch always resolves or errors. ✓
- no new external dep → step 3. ✓

---

## Smoke

**Credit-free (unit):** `go test ./internal/router/... ./internal/config/... ./internal/server/...` all green — proves routing resolution, config parsing, and the server submit path through the router with a fake/webhook adapter (no network).

**Live re-confirmation (deferred to the phase final smoke, README §7):** relay boots with `adapters.webhook.enabled: true` and `routing.default_adapter: "webhook"`, a 2-turn conversation + Submit routes the payload to a local webhook receiver — the Phase-1 walking-skeleton smoke, now exercised through the router. This needs an `ANTHROPIC_API_KEY` (paid) so it runs at the phase smoke, not here.

> Note: with the **default** config (`routing.default_adapter` defaults to `"chatwoot"`), the relay will fatally refuse to start until chatwoot is registered (3-ii) — this is the correct §4.4 behavior (default names an unregistered adapter). The 3-i functional config must set `routing.default_adapter: "webhook"` and enable webhook.

## Done Criteria

1. `go build ./... && go vet ./...` clean in `relay/`.
2. `go test ./...` green, including the new `router` and `config` tests, with NO real keys.
3. `Deps.Adapter` is gone; `Deps.Router *router.Router` is the only dispatch path in `submit.go`.
4. `router.New` returns an error for an unregistered `default_adapter`; drops (with a warning) rules targeting unregistered adapters; `Route` never returns `(nil, nil)`.
5. `bash scripts/verify-contract.sh` and `bash scripts/check-pins.sh` green.
6. `go mod tidy` leaves `go.mod`/`go.sum` unchanged (stdlib-only).
7. `buildRegistry` exists in `main.go` with a clear seam comment for 3-ii…3-v to add adapters and 3-vi to add the gate.
