# Phase 1 — Walking Skeleton

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement the sub-plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

The thinnest end-to-end path that works: a user holds a guided triage conversation in an embedded Vue widget, clicks Submit, and a structured ticket lands at a webhook — across one provider (Anthropic), one adapter (webhook), one auth mode (anonymous). This phase **freezes the shared interfaces** so Phases 2–5 extend each axis behind a stable seam.

## 1. Spec link

- Phase 1 design: [docs/specs/2026-05-26-phase-1-walking-skeleton-design.md](../../../docs/specs/2026-05-26-phase-1-walking-skeleton-design.md)
- Parent decomposition: [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](../../../docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md)
- Product v0 spec: [docs/PROJECT.md](../../../docs/PROJECT.md) §3,§5,§6,§7,§8,§9,§17

## 2. Architectural Decision Record (ADR) summary

- **Relay is the sole LLM broker; no provider keys in the browser.** Provider keys are read server-side from env vars only; the widget talks only to the relay, which proxies the provider call. **Trigger to revisit:** never (security invariant).
- **`payload.v1.json` is the relay→adapter contract, assembled server-side from a thinner `SubmitRequest`.** The relay runs `classify()` at submit, assembles + schema-validates the canonical `payload.Payload`, then calls `Adapter.Create`. **Trigger to revisit:** a future server-to-server caller that submits a fully-formed payload (add a separate validated ingress).
- **Relay is stateless between turns; the widget owns conversation history** (PROJECT.md §6). **Trigger to revisit:** per-turn payload size becomes a problem before the Phase-5 20-turn/8000-token caps.
- **Anonymous-only auth in Phase 1; the middleware contract accommodates `Authorization: Bearer <jwt>`.** **Trigger to revisit:** Phase 4 (email/SSO).
- **chi v5 router + stdlib `net/http`; no web framework.** Matches PROJECT.md §3. **Trigger to revisit:** never planned for v0.

This phase does NOT add: rate limiting / caps / spend cap / CAPTCHA (Phase 5 — **required before any public exposure**), email/SSO (Phase 4), other providers (Phase 2), other adapters + license (Phase 3), attachments (Phase 6), metrics/release (Phase 7).

## 3. Sub-plan index

| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 1-i | [Relay server skeleton](1-i-relay-skeleton-plan.md) | config + chi server | M | Not started |
| 1-ii | [LLM Provider + Anthropic](1-ii-llm-provider-anthropic-plan.md) | provider seam | M | Not started |
| 1-iii | [Session/auth + /init + /turn SSE](1-iii-session-turn-sse-plan.md) | auth + SSE seam | L | Not started |
| 1-iv | [Adapter + webhook + /submit](1-iv-adapter-webhook-submit-plan.md) | adapter seam | L | Not started |
| 1-v | [@intake/core TS client](1-v-core-client-plan.md) | client↔relay | M | Not started |
| 1-vi | [@intake/vue widget + example](1-vi-vue-widget-example-plan.md) | widget + smoke | L | Not started |

## 4. Dependency graph

```
1-i ─► 1-ii ─► 1-iii ─► 1-iv ─► 1-v ─► 1-vi
```
Mostly serial. 1-iii consumes the 1-ii Provider; 1-iv consumes 1-i's server + the Phase-0 `payload.Payload`; the TS client (1-v) and widget (1-vi) consume the relay HTTP contract finalized by 1-iv. The frozen interfaces (§6) are the exit criterion of their introducing sub-plan and MUST NOT change afterward without re-smoking the dependents.

## 5. Tool version pin list

New dependencies this phase introduces. Confirm exact latest patch at install (in the introducing sub-plan's first task), pin exactly, and update this table in the same commit (per Phase-0 precedent and PHASE_PLANNING §5). Caret forbidden for the Anthropic SDK (its output shape is load-bearing for the provider) and the vue build toolchain (produces the shipped widget bundle).

| Tool | Version | Reason |
|---|---|---|
| github.com/go-chi/chi/v5 | v5.1.0 (verify+pin at install) | HTTP router/middleware; PROJECT.md §3 |
| github.com/google/uuid | v1.6.0 (verify+pin at install) | session_id / submission.id generation |
| github.com/anthropics/anthropic-sdk-go | verify+pin exact at install | Anthropic Messages API streaming + usage tokens; **exact** (response shape load-bearing) |
| github.com/santhosh-tekuri/jsonschema/v6 | verify+pin at install | server-side validation of the assembled payload against `schema/payload.v1.json` (Go-native draft 2020-12) |
| vue | 3.5.x (verify+pin at install) | widget framework |
| vite | 5.4.x (verify+pin exact at install) | widget + example build/dev; **exact** (bundle output) |
| @vitejs/plugin-vue | verify+pin at install | vite Vue SFC support |
| tsx | verify+pin at install | run the @intake/core Node smoke script (1-v) |

> If a pinned version is unavailable or a newer patch is needed at install, the introducing sub-plan's first task records the actual version and updates this table in the same commit. The constraint is *exact pinning of load-bearing tools*, not these specific numbers.

## 6. Shared Contracts (SINGLE SOURCE OF TRUTH)

Every sub-plan MUST use these exact signatures for cross-cutting types. Do NOT invent alternative signatures; reference this section.

### 6.1 Go — `relay/internal/llm/provider.go` (frozen in 1-ii)
```go
package llm

import "context"

type Message struct {
	Role    string `json:"role"`    // "system" | "user" | "assistant"
	Content string `json:"content"`
}

type ChatOptions struct {
	Model       string
	MaxTokens   int
	Temperature float64
	Stream      bool
}

type ChatChunk struct {
	Delta        string // incremental text
	Done         bool
	InputTokens  int // populated on Done
	OutputTokens int // populated on Done
	Err          error // non-nil => terminal error for this stream
}

type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []Message, opts ChatOptions) (<-chan ChatChunk, error)
}
```

### 6.2 Go — `relay/internal/adapter/adapter.go` (frozen in 1-iv)
```go
package adapter

import (
	"context"
	"intake/internal/payload"
)

type CreateResult struct {
	ExternalID  string
	ExternalURL string
	AdapterName string
	CreatedAt   string // ISO-8601
}

type Adapter interface {
	Name() string
	RequiresLicense() bool
	Configure(config map[string]any) error
	Create(ctx context.Context, p *payload.Payload) (*CreateResult, error)
	HealthCheck(ctx context.Context) error
}
```

### 6.3 Go — `relay/internal/auth/session.go` (frozen in 1-iii)
```go
package auth

import "context"

type SessionContext struct {
	SessionID   string
	AuthMode    string // "anonymous" | "email" | "sso"
	Verified    bool
	UserID      *string
	Email       *string
	DisplayName *string
	Custom      map[string]any
}

type ctxKey struct{}

// FromContext returns the SessionContext attached by the auth middleware.
func FromContext(ctx context.Context) (*SessionContext, bool) {
	s, ok := ctx.Value(ctxKey{}).(*SessionContext)
	return s, ok
}

// WithSession attaches a SessionContext (used by the middleware).
func WithSession(ctx context.Context, s *SessionContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}
```

### 6.4 Go — HTTP DTOs (frozen in 1-iii/1-iv), package `server`
```go
// Session transport: the X-Intake-Session header carries the session_id on
// every /turn and /submit request (single source of truth; NOT in the body).

type InitResponse struct {
	SessionID    string       `json:"session_id"`
	Capabilities Capabilities `json:"capabilities"`
}

type Capabilities struct {
	AuthModes []string `json:"auth_modes"` // Phase 1: ["anonymous"]
	Streaming bool     `json:"streaming"`  // true
}

type TurnMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

type TurnRequest struct {
	Messages []TurnMessage `json:"messages"`
}

// SSE frames on /turn (each written as `data: <json>\n\n`):
type SSEDelta struct{ Delta string `json:"delta"` }
type SSEDone struct {
	Done         bool `json:"done"`
	InputTokens  int  `json:"input_tokens"`
	OutputTokens int  `json:"output_tokens"`
}
type SSEError struct{ Error string `json:"error"` }

type ClientInfo struct {
	WidgetVersion string   `json:"widget_version"`
	URL           string   `json:"url"`
	Referrer      *string  `json:"referrer"`
	UserAgent     string   `json:"user_agent"`
	Viewport      Viewport `json:"viewport"`
	Locale        string   `json:"locale"`
}
type Viewport struct {
	W int `json:"w"`
	H int `json:"h"`
}
type ContextInfo struct {
	AppContext   map[string]any `json:"app_context"`
	PageMetadata map[string]any `json:"page_metadata"`
}

type SubmitRequest struct {
	Messages    []TurnMessage  `json:"messages"`
	Client      ClientInfo     `json:"client"`
	UserClaims  map[string]any `json:"user_claims"`
	Context     ContextInfo    `json:"context"`
	RoutingHint *string        `json:"routing_hint"`
	// attachments deferred to Phase 6
}

type SubmitResponse struct {
	ExternalID  string `json:"external_id"`
	ExternalURL string `json:"external_url"`
	AdapterName string `json:"adapter_name"`
	CreatedAt   string `json:"created_at"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
```

### 6.5 Go — classifier output (frozen in 1-iv), package `classify`
```go
package classify

// Result is the structured triage output the relay derives at submit time and
// writes into the canonical payload's conversation.* fields.
type Result struct {
	Summary        string   `json:"summary"`
	TitleSuggestion string  `json:"title_suggestion"`
	Classification string   `json:"classification"`  // bug|feature_request|question|other
	SeverityGuess  string   `json:"severity_guess"`  // low|medium|high|critical|unknown
	TagsSuggested  []string `json:"tags_suggested"`
	Language       string   `json:"language"`
}
```

### 6.6 Go — config (frozen in 1-i, extended additively by later sub-plans)
```go
package config

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	LLM      LLMConfig      `yaml:"llm"`
	Auth     AuthConfig     `yaml:"auth"`
	Adapters AdaptersConfig `yaml:"adapters"`
}
type ServerConfig struct {
	Addr        string   `yaml:"addr"`
	ExternalURL string   `yaml:"external_url"`
	CORSOrigins []string `yaml:"cors_origins"`
}
type LLMConfig struct {
	Provider         string          `yaml:"provider"`
	Anthropic        AnthropicConfig `yaml:"anthropic"`
	SystemPromptFile string          `yaml:"system_prompt_file"`
}
type AnthropicConfig struct {
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}
type AuthConfig struct {
	Modes AuthModes `yaml:"modes"`
}
type AuthModes struct {
	Anonymous bool `yaml:"anonymous"`
}
type AdaptersConfig struct {
	Webhook WebhookConfig `yaml:"webhook"`
}
type WebhookConfig struct {
	Enabled bool              `yaml:"enabled"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Retry   RetryConfig       `yaml:"retry"`
}
type RetryConfig struct {
	MaxAttempts int    `yaml:"max_attempts"`
	Backoff     string `yaml:"backoff"` // "exponential" | "fixed"
}
```

### 6.7 TS — `@intake/core` public API (frozen in 1-v)
```ts
import type { IntakePayload } from "./generated/payload.js";

export interface IntakeConfig {
  relayUrl: string;          // e.g. "http://localhost:8080"
  widgetVersion: string;
  appContext?: Record<string, unknown>;
}

export interface ChatMessage { role: "user" | "assistant"; content: string; }

export interface SubmitResult {
  external_id: string;
  external_url: string;
  adapter_name: string;
  created_at: string;
}

export class IntakeClient {
  constructor(config: IntakeConfig);
  init(): Promise<{ session_id: string; capabilities: { auth_modes: string[]; streaming: boolean } }>;
  // turn() streams assistant text deltas via onDelta, resolves on done.
  turn(messages: ChatMessage[], onDelta: (delta: string) => void): Promise<{ input_tokens: number; output_tokens: number }>;
  submit(messages: ChatMessage[], routingHint?: string): Promise<SubmitResult>;
}
```
The client sends the `X-Intake-Session` header (from `init()`) on `turn()`/`submit()`. It captures `client.*` (url, referrer, user_agent, viewport, locale) and `context.page_metadata` from the browser when assembling the `SubmitRequest`.

## 7. Build-fail checklist

The phase's verification (and CI, once 0-iv's workflows have a remote) MUST fail on any of these:

- [ ] `go build ./...` or `go vet ./...` fails in `relay/`. **Fail.**
- [ ] Any Go test fails (`go test ./...`). **Fail.**
- [ ] `tsc --noEmit` fails in `core/` or `vue/`. **Fail.**
- [ ] The Phase-0 contract gate regresses (`scripts/verify-contract.sh` non-zero). **Fail.**
- [ ] A provider API key, or any secret, appears in a log line or HTTP response body. **Fail** (grep gate over relay logs in the smoke).
- [ ] The assembled canonical payload fails `schema/payload.v1.json` validation at submit. **Fail** (this is a runtime 400, and a unit test asserts a well-formed submit produces a schema-valid payload).
- [ ] Any new load-bearing tool (Anthropic SDK, vite) is caret/unpinned. **Fail** (extend `scripts/check-pins.sh`).

## 8. Final smoke (mandatory)

Proves the walking skeleton end-to-end against real Anthropic + a real local webhook receiver.

```
1. Pre-condition: Phase-1 merged; Node 24.12 + Go 1.23.2; ANTHROPIC_API_KEY exported;
   a local webhook receiver logging POST bodies on http://localhost:9099/intake
   (a tiny helper is provided at examples/webhook-receiver/); relay config (examples/
   docker-compose or a local config.yaml) points adapters.webhook.url at it.
2. Execution:
   a. Start the relay: `cd relay && go run ./cmd/relay --config ../config.yaml`.
      Confirm GET /v1/health -> 200 and GET /v1/version -> build info.
   b. Start the example: `npm run -w examples/vue-anonymous dev`; open the printed URL.
   c. Click the launcher button; hold a triage conversation of >=2 user turns; the
      assistant asks clarifying questions and proposes a summary; confirm.
   d. Click Submit.
3. Verification:
   - Assistant tokens stream into the panel incrementally (SSE working end-to-end).
   - The webhook receiver logs exactly ONE POST whose body is a schema-valid canonical
     payload: conversation.{summary,title_suggestion,classification,severity_guess,
     tags_suggested,language} populated by the server-side classify(); client.{url,
     viewport,locale,user_agent} captured; user.auth_mode="anonymous", user.verified=false;
     schema_version="1.0".
   - The widget displays the returned ticket result (external_id).
   - Grep the relay's stdout/stderr for the ANTHROPIC_API_KEY value -> NOT present.
4. Teardown / repeat: stop relay + example + receiver; re-runnable (relay is stateless).
```

A credential-free unit layer backs this: provider chunk-assembly (mock HTTP), webhook adapter (httptest receiver), handlers (httptest), classify() (mock provider), and the core serializer (TS unit) — all run without real keys in `go test ./...` and the TS test.

## 9. Notes carried from the design

- Bundled triage system prompt is authored in 1-iii; overridable via `llm.system_prompt_file`. It is never sent by or exposed to the client.
- Module path remains `intake` (placeholder name; mechanical rename gate per the decomposition design §6).
- `go-jsonschema`-generated `const` is NOT enforced at the Go type level (LESSONS L003) — 1-iv's submit handler MUST validate `schema_version` (and the whole payload) against the schema at runtime, exactly the mitigation that ADR notes.
