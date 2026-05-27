# Phase 2 — Provider Breadth: Design

> **Status:** Approved design (brainstorming output), pre-planning
> **Date:** 2026-05-27
> **Implements:** [docs/PROJECT.md](../PROJECT.md) §7 (LLM provider interface)
> **Parent:** [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 2 row)
> **Builds on:** Phase 1's frozen `llm.Provider` interface (`relay/internal/llm/provider.go`) + the Anthropic reference impl

## 1. Goal

Add `openai`, `gemini`, and `ollama` as `llm.Provider` implementations behind the **frozen** Phase-1 interface, selectable via `cfg.LLM.Provider` at startup. One provider is active per relay instance — **no runtime multi-provider routing** (PROJECT.md §7; out of v0 scope). The Anthropic provider is the reference; each new provider replicates its exact behavioral contract: per-delta `ChatChunk{Delta}`, terminal `ChatChunk{Done, InputTokens, OutputTokens}`, error `ChatChunk{Err, Done}`, `ctx` cancellation respected, best-effort channel sends (no goroutine leak — Phase-1 lessons L004 / 1-ii), and **provider keys never logged**.

Success (decomposition §2): "All [four] LLM providers complete a 5-turn conversation end-to-end" and "Ollama runs offline with no API key."

## 2. The selection seam — a `providers` factory package

A factory selects the implementation from config:

```go
// relay/internal/llm/providers/providers.go  — package providers
func New(cfg config.LLMConfig) (llm.Provider, error)
```

It switches on `cfg.Provider` (`"anthropic" | "openai" | "gemini" | "ollama"`), resolves the relevant secret via `config.ResolveSecret`, constructs the impl, and returns a clear error for an unknown provider or missing-required-secret.

**Why a separate package (not package `llm`):** `llm/anthropic` (and the other impls) import `llm` for the interface. A factory *inside* `llm` that imported `llm/anthropic` would create an import cycle (`llm → llm/anthropic → llm`). The `providers` package imports `llm` + all four impl subpackages with no back-edge, so there is no cycle. `main.go` switches from the direct `anthropic.New(...)` to `providers.New(cfg.LLM)`.

## 3. Sub-plan decomposition (`ai/tasks/phase-2/`)

| # | Unit | Adds | Sub-plan smoke |
|---|---|---|---|
| 2-i | **Config + factory** | `openai`/`gemini`/`ollama` sub-configs on `LLMConfig` (secrets via `ResolveSecret`); `providers.New` (anthropic case only); rewire `main.go` to use the factory | relay still boots & serves a turn with `provider: anthropic` *via the factory* (no behavior change); `providers.New` returns a clear error for an unknown provider (unit test) |
| 2-ii | **OpenAI** (`openai-go`) + factory case | the openai provider | 5-turn conversation completes through openai (live) |
| 2-iii | **Gemini** (`google.golang.org/genai`) + factory case | the gemini provider | 5-turn conversation completes through gemini (live) |
| 2-iv | **Ollama** (hand-rolled `net/http`) + factory case | the ollama provider + optional bearer token | 5-turn conversation completes through ollama, **offline, no API key** |

**Dependency graph:** `2-i → 2-ii → 2-iii → 2-iv` (serial; 2-i establishes the config+factory seam, each provider sub-plan adds its package + one factory `case` + its config defaults). The sub-plans are nearly independent (each adds a distinct package); serial execution avoids factory-edit conflicts.

## 4. Per-provider implementation notes

All four return a `*Provider` satisfying `llm.Provider`, with a test-injectable constructor variant (like Anthropic's `NewWithClient`) so mock unit tests run credit-free.

### 4.1 OpenAI (`relay/internal/llm/openai/`)
- SDK: `github.com/openai/openai-go` (official), exact-pinned + verified at install.
- Chat Completions **streaming**; set `stream_options: { include_usage: true }` so the final stream event carries usage. Map `usage.prompt_tokens` → `InputTokens`, `usage.completion_tokens` → `OutputTokens`.
- A `system`-role `llm.Message` stays a system message in the `messages` array (OpenAI has no separate system param).
- `Name()` → `"openai"`.

### 4.2 Gemini (`relay/internal/llm/gemini/`)
- SDK: `google.golang.org/genai` (official), exact-pinned + verified at install.
- `GenerateContentStream`; a `system`-role `llm.Message` maps to the request's `SystemInstruction` (Gemini separates system from contents — like Anthropic).
- Usage from `usageMetadata`: `PromptTokenCount` → `InputTokens`, `CandidatesTokenCount` → `OutputTokens`.
- `Name()` → `"gemini"`.

### 4.3 Ollama (`relay/internal/llm/ollama/`)
- Hand-rolled `net/http` against `POST {base_url}/api/chat` with `"stream": true` (newline-delimited JSON objects, each `{message:{content},done,...}`; the final object carries `prompt_eval_count` → `InputTokens`, `eval_count` → `OutputTokens`).
- A `system`-role message stays a system message in the `messages` array.
- `base_url` configurable (default `http://localhost:11434`); **no API key**. Optional bearer token via `ollama.bearer_token_env` (design Q4) — if set, sent as `Authorization: Bearer <token>` (for a relay sitting behind a hardened/proxied Ollama).
- `Name()` → `"ollama"`.

## 5. Configuration (additive to `config.LLMConfig`)

```yaml
llm:
  provider: "anthropic"   # anthropic | openai | gemini | ollama
  anthropic: { api_key_env: "ANTHROPIC_API_KEY", model: "claude-sonnet-4-6", max_tokens: 1024 }
  openai:    { api_key_env: "OPENAI_API_KEY",    model: "gpt-4o-mini",        max_tokens: 1024 }
  gemini:    { api_key_env: "GEMINI_API_KEY",    model: "gemini-2.0-flash",   max_tokens: 1024 }
  ollama:    { base_url: "http://localhost:11434", model: "llama3.1", bearer_token_env: "", max_tokens: 1024 }
  system_prompt_file: ""
```

- Secrets (`OPENAI_API_KEY`, `GEMINI_API_KEY`, optional `OLLAMA_*` bearer) resolve through `config.ResolveSecret` (env-or-`_FILE`); never in the YAML.
- **Default models** (decomposition §4 Q5) — these drift, so they are documented defaults to **confirm at the live smoke** (the implementer verifies the model id resolves, exactly as we treated the Anthropic model):
  - openai: `gpt-4o-mini` *(cost-effective default — confirm/adjust at smoke)*
  - gemini: `gemini-2.0-flash` *(flash tier — confirm/adjust at smoke)*
  - ollama: `llama3.1` *(operator must have this model pulled, or set `ollama.model` to one they have)*
- The factory validates the **selected** provider's config only (e.g. anthropic/openai/gemini require their key; ollama does not). Other providers' config may be blank.

## 6. Testing

- **Credit-free unit tests per provider** (run in `go test ./...`, no real keys):
  - openai/ollama: `httptest.Server` emitting a canned stream (SSE for openai, NDJSON for ollama); assert ordered deltas + a terminal usage chunk; ctx-cancel test; key-redaction test.
  - gemini: inject a custom HTTP client / base URL into the genai client (as Anthropic's `NewWithClient` does) pointed at an httptest server emitting a canned genai stream.
- **Factory unit test** (`providers`): selecting each provider name returns the right `Name()`; an unknown provider errors; a selected provider missing its required secret errors clearly.
- **Live smoke (Phase final):** a 5-turn conversation completes end-to-end through **each** of openai, gemini, ollama (anthropic already proven in Phase 1). Ollama runs offline with no key. Reuse the Phase-1 `core/smoke/drive.ts` pattern or a relay-level turn loop driven by config `provider: <name>`.

## 7. Dependencies (pin exactly; verify at install; update phase README §5; extend check-pins)

| Tool | Pin | Notes |
|---|---|---|
| github.com/openai/openai-go | verify+pin exact at install | response/stream shape load-bearing; caret-forbidden |
| google.golang.org/genai | verify+pin exact at install | same |
| (ollama) | — | stdlib `net/http` only |

`scripts/check-pins.sh` extended to fail on a caret/`@latest` for the two new SDKs (as it does for `anthropic-sdk-go`).

## 8. ADRs locked by this phase (for the phase README)

- **One provider per relay, selected at startup via a `providers` factory** (PROJECT.md §7). Trigger to revisit: a use case needs per-request provider routing (deferred past v0).
- **Factory lives in a separate `providers` package to avoid the `llm` import cycle** (§2). Trigger to revisit: never (structural).
- **Official SDKs for anthropic/openai/gemini; hand-rolled net/http for ollama** (local, simple). Trigger to revisit: an SDK becomes unmaintained or its streaming/usage surface regresses.
- **The frozen `llm.Provider` interface is unchanged** — Phase 2 only adds implementations. Trigger to revisit: a provider can't express the contract (none anticipated).

## 9. Non-goals

Runtime multi-provider routing / fallback; changing the `Provider` interface; embeddings or non-chat endpoints; provider-specific features beyond chat streaming + token usage (tool use, vision, etc. — not needed for triage).
