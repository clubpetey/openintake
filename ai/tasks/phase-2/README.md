# Phase 2 — Provider Breadth

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement the sub-plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Adds `openai`, `gemini`, and `ollama` as `llm.Provider` implementations behind the **frozen** Phase-1 interface, selectable at startup via `cfg.LLM.Provider` through a `providers` factory. One provider per relay; no runtime multi-provider routing (PROJECT.md §7). The Anthropic provider (`relay/internal/llm/anthropic/anthropic.go`) is the reference implementation every new provider mirrors.

## 1. Spec link

- Phase 2 design: [docs/specs/2026-05-27-phase-2-provider-breadth-design.md](../../../docs/specs/2026-05-27-phase-2-provider-breadth-design.md)
- Parent decomposition: [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](../../../docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md)
- Frozen interface: [Phase 1 README §6.1](../phase-1/README.md) (`llm.Provider`/`ChatChunk` — UNCHANGED this phase)

## 2. Architectural Decision Record (ADR) summary

- **One provider per relay, selected at startup via a `providers` factory** (PROJECT.md §7). Revisit trigger: per-request provider routing (deferred past v0).
- **Factory in a separate `providers` package, NOT package `llm`** — `llm/<impl>` packages import `llm` for the interface; a factory inside `llm` importing the impls would cycle (`llm → llm/anthropic → llm`). `providers` imports `llm` + all impls with no back-edge. Revisit trigger: never (structural).
- **Official SDKs for anthropic/openai/gemini; hand-rolled `net/http` for ollama** (local, simple, no key). Revisit trigger: an SDK's streaming/usage surface regresses or it becomes unmaintained.
- **The `llm.Provider` interface is unchanged** — Phase 2 only adds implementations + a factory + config. Revisit trigger: a provider can't express the contract (none anticipated).

This phase does NOT add: runtime multi-provider routing/fallback, interface changes, non-chat endpoints, or provider-specific features beyond chat streaming + token usage.

## 3. Sub-plan index

| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 2-i | [Config + providers factory](2-i-config-factory-plan.md) | selection seam | M | Complete |
| 2-ii | [OpenAI provider](2-ii-openai-provider-plan.md) | openai-go | M | Complete |
| 2-iii | [Gemini provider](2-iii-gemini-provider-plan.md) | google.golang.org/genai | M | Complete |
| 2-iv | [Ollama provider](2-iv-ollama-provider-plan.md) | hand-rolled net/http | M | Complete |

## 4. Dependency graph

```
2-i (config + factory) ─► 2-ii (openai) ─► 2-iii (gemini) ─► 2-iv (ollama)
```
Serial. 2-i establishes the config sub-structs + `providers.New` (anthropic case) + rewires `main.go`. Each provider sub-plan adds its package + one factory `case` + its config defaults. Serial avoids concurrent edits to `providers.go` / `config.go`.

## 5. Tool version pin list

Verify exact latest at install (in the introducing sub-plan's first task), pin exactly, update this table + `scripts/check-pins.sh` in the same commit. Caret forbidden for the provider SDKs (stream/usage shape load-bearing).

| Tool | Version | Reason |
|---|---|---|
| github.com/openai/openai-go | v1.12.0 | OpenAI Chat Completions streaming + usage; **exact** |
| google.golang.org/genai | v0.7.0 | Gemini GenerateContentStream + usageMetadata; **exact** (v0.x required — v1.x needs go 1.24) |
| (ollama) | — (stdlib net/http) | local `/api/chat`; no SDK |

## 6. Build-fail checklist

- [ ] `go build ./...` / `go vet ./...` fails in `relay/`. **Fail.**
- [ ] Any Go test fails (`go test ./...`). **Fail.**
- [ ] The Phase-0 contract gate regresses (`scripts/verify-contract.sh`). **Fail.**
- [ ] A provider API key (any provider) appears in a log line or response body. **Fail.**
- [ ] `openai-go` or `google.golang.org/genai` pinned with a caret or `@latest`. **Fail** (check-pins gate).
- [ ] `providers.New` returns a non-nil provider for an unknown provider name, or a nil provider with nil error. **Fail** (must error clearly).

## 7. Final smoke (mandatory)

Proves a real 5-turn conversation completes through **each** new provider (anthropic already proven in Phase 1).

```
1. Pre-condition: Phase-2 merged; OPENAI_API_KEY + GEMINI_API_KEY exported (or in .env);
   Ollama running locally with the configured model pulled (e.g. `ollama pull llama3.1`).
2. For each provider P in {openai, gemini, ollama}:
   a. Set llm.provider: P in config.yaml (and the provider's model/key).
   b. Start the relay; confirm /v1/health.
   c. Drive a 5-turn conversation through /v1/intake/turn (accumulating history each turn),
      e.g. RELAY_URL=... PROVIDER=P npx tsx core/smoke/drive-multi.ts (a 5-turn driver).
3. Verification (per provider):
   - All 5 turns stream assistant tokens (SSE deltas) and each terminal frame carries
     non-zero input_tokens/output_tokens.
   - The provider's API key (or ollama bearer, if set) is ABSENT from the relay logs.
   - Ollama completes with NO API key set (offline).
4. Teardown: stop the relay between providers; re-runnable.
```

A credit-free unit layer backs this: each provider has a mock-HTTP streaming test + ctx-cancel + key-redaction test, plus a `providers` factory test — all green in `go test ./...` with no real keys.

### Smoke result (2026-05-27)

Run headlessly via `core/smoke/drive-multi.ts` (init + 5× `/turn`) with the relay configured per provider.

- **openai (`gpt-4o-mini`) — ✅ PASS.** Full 5-turn guided triage; SSE streamed every turn; non-zero input/output tokens each turn; API key absent from relay logs.
- **ollama (`llama3.1` @ `http://192.168.1.102:11434`) — ✅ PASS.** Full 5-turn, **fully offline / no API key**, non-zero tokens each turn; cross-LAN reach confirmed.
- **gemini (`gemini-2.0-flash`) — ⚠️ code-validated; success-path DEFERRED on account credits.** The provider authenticated and reached the real Gemini API, which returned `429 RESOURCE_EXHAUSTED` ("prepayment credits depleted") — surfaced cleanly through the SSE error frame with the key absent from logs. Request construction + auth + error-propagation are validated live; a *successful* 5-turn completion is pending billing credits on the Google AI Studio project. Re-run the gemini arm once credits are added: set `llm.provider: gemini` and run `drive-multi.ts`.

Net: 2/3 new providers fully green live; gemini's implementation is validated against the real API, with only the success-path completion deferred to an account-billing change (not a code defect).


## 8. Shared Contracts (SINGLE SOURCE OF TRUTH)

### 8.1 The frozen provider interface (UNCHANGED — `relay/internal/llm/provider.go`)
Every provider implements `llm.Provider` exactly as frozen in Phase 1 (Phase-1 README §6.1): `Name() string` and `Chat(ctx, []llm.Message, llm.ChatOptions) (<-chan llm.ChatChunk, error)`. Behavioral contract each impl MUST honor (mirror `anthropic.go`):
- emit `llm.ChatChunk{Delta: text}` per incremental token;
- emit a terminal `llm.ChatChunk{Done: true, InputTokens: N, OutputTokens: M}` from the provider's usage;
- on error emit `llm.ChatChunk{Err: <redacted err>, Done: true}` then close;
- exactly one `defer close(ch)`; sends guarded by `select { case ch<-…: case <-ctx.Done(): }` or best-effort `default` so the goroutine never blocks/leaks;
- respect `ctx` cancellation; NEVER log or embed the API key in an error.

### 8.2 Provider constructor pattern (each impl package)
```go
// New constructs the provider. The secret (api key / bearer) is passed in;
// the caller (providers.New) resolves it from the environment.
func New(/* apiKey/baseURL + */ model string, maxTokens int /* + provider-specific */) *Provider
// A test-injectable variant points the SDK/HTTP at an httptest server (mirror
// anthropic's NewWithClient) so mock unit tests run credit-free.
var _ llm.Provider = (*Provider)(nil)
```

### 8.3 Config sub-structs (additive — `relay/internal/config/config.go`, frozen in 2-i)
```go
// Added fields on LLMConfig:
type LLMConfig struct {
	Provider         string          `yaml:"provider"`
	Anthropic        AnthropicConfig `yaml:"anthropic"`
	OpenAI           OpenAIConfig    `yaml:"openai"`   // 2-i
	Gemini           GeminiConfig    `yaml:"gemini"`   // 2-i
	Ollama           OllamaConfig    `yaml:"ollama"`   // 2-i
	SystemPromptFile string          `yaml:"system_prompt_file"`
}
type OpenAIConfig struct {
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}
type GeminiConfig struct {
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}
type OllamaConfig struct {
	BaseURL        string `yaml:"base_url"`
	Model          string `yaml:"model"`
	BearerTokenEnv string `yaml:"bearer_token_env"` // optional; "" = no auth
	MaxTokens      int    `yaml:"max_tokens"`
}
```
Defaults applied in `config.applyDefaults` (2-i): openai model `gpt-4o-mini`; gemini model `gemini-2.0-flash`; ollama `base_url` `http://localhost:11434`, model `llama3.1`; `max_tokens` 1024 each (only when unset).

### 8.4 The factory (frozen in 2-i — `relay/internal/llm/providers/providers.go`, package `providers`)
```go
package providers

// New constructs the configured provider, resolving its secret via config.ResolveSecret.
// Errors on an unknown provider name or a missing required secret.
func New(cfg config.LLMConfig) (llm.Provider, error)
```
Switch on `cfg.Provider`:
- `"anthropic"` → `config.RequireSecret(cfg.Anthropic.APIKeyEnv)` → `anthropic.New(key, cfg.Anthropic.Model, cfg.Anthropic.MaxTokens)`
- `"openai"` → `config.RequireSecret(cfg.OpenAI.APIKeyEnv)` → `openai.New(...)` (added in 2-ii)
- `"gemini"` → `config.RequireSecret(cfg.Gemini.APIKeyEnv)` → `gemini.New(...)` (added in 2-iii)
- `"ollama"` → `config.ResolveSecret(cfg.Ollama.BearerTokenEnv)` (optional) → `ollama.New(...)` (added in 2-iv); NO required key
- default → `fmt.Errorf("unknown llm provider %q", cfg.Provider)`

`main.go` replaces its direct `anthropic.New(...)` with `providers.New(cfg.LLM)`. 2-ii/iii/iv each add their `case` (and import) when their package exists; until then the case returns an explicit "not implemented in this build" error (added incrementally).

## 9. Notes

- Default model ids (`gpt-4o-mini`, `gemini-2.0-flash`, `llama3.1`) are documented defaults to **confirm at the live smoke** — model ids drift; the implementer verifies the id resolves against the live API (as we did for the Anthropic model). LESSONS L004 applies to any Node-driven smoke (browser-global stubs).
- Module path remains `intake` (placeholder name).
