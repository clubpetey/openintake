# Phase 1 — Walking Skeleton: Design

> **Status:** Approved design (brainstorming output), pre-planning
> **Date:** 2026-05-26
> **Implements:** [docs/PROJECT.md](../PROJECT.md) §3, §5, §6, §7, §8, §9, §17
> **Parent:** [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 1 row)
> **Builds on:** Phase 0 contract spine (generated `payload.Payload` types)

## 1. Goal

Deliver the thinnest end-to-end path that works: a user holds a guided triage conversation in an embedded Vue widget, clicks Submit, and a structured ticket lands in a downstream system — across one provider (**Anthropic**), one adapter (**webhook**), one auth mode (**anonymous**). This phase **freezes the shared interfaces** (`llm.Provider`, `adapter.Adapter`, the auth middleware contract, and the SSE `/turn` protocol) so Phases 2–5 can extend each axis behind a stable seam.

This directly targets the v0 success criterion: "embed the widget, run the relay locally, route a real ticket … in under 15 minutes" — narrowed to the webhook adapter.

## 2. Security invariant (load-bearing)

**The relay is the sole broker for every LLM call. The browser never holds, sees, or transmits a provider API key.** There is no code path in which the widget contacts a provider directly.

- Provider keys are read **server-side from environment variables only** (`llm.anthropic.api_key_env` → `ANTHROPIC_API_KEY`), never from the config file, never in any HTTP response, never logged (PROJECT.md §17).
- The widget talks only to the relay: `POST /v1/intake/turn` returns an SSE stream the relay proxies from the provider; `POST /v1/intake/submit` triggers the server-side classify + adapter dispatch. The upstream provider call happens entirely inside the relay process.
- Browser-held credentials are **relay-scoped** (an anonymous `session_id`; later a relay-issued JWT), never provider-scoped. Compromise grants only rate-limited access to the relay, not the upstream key.

**Consequence — Phase 5 is a production prerequisite.** Because the relay is the public-facing LLM gateway, an exposed `/turn` with no limits can be abused to burn tokens. Phase 1 intentionally ships **without** rate limiting, spend caps, or CAPTCHA (acceptable for local dev and the phase smoke). A relay MUST NOT be exposed publicly until Phase 5 lands those controls. This is called out again in §9.

## 3. Architecture

```
Browser (host app)                         Relay (Go)                         Anthropic
  @openintake/vue  ── POST /v1/intake/init ───►  init: issue session_id ──┐
   (launcher+      ◄── {session_id, caps}                              │
    panel)                                                             │
  @openintake/core ── POST /v1/intake/turn ───►  auth → build messages ───┼──► Messages API (stream)
               ◄═══ SSE: {delta}… {done} ═══  proxy token stream  ◄────┘
               ── POST /v1/intake/submit ─►  classify() ─────────────────► Messages API (structured)
                                             assemble payload.Payload
                                             validate vs schema
                                             webhook.Create(payload) ──────► downstream webhook receiver
               ◄── {external_id, url…}
```

The browser is a dumb terminal: it owns conversation state (per PROJECT.md §6, the relay is stateless between turns) and renders tokens the relay streams.

## 4. Sub-plan decomposition

Phase lives in `ai/tasks/phase-1/` (README + sub-plans per [ai/PHASE_PLANNING.md](../../ai/PHASE_PLANNING.md)).

| # | Unit | Freezes | Sub-plan smoke |
|---|---|---|---|
| 1-i | **Relay server skeleton** — config loader (Phase-1 subset), chi server, middleware (request-id, panic-recover, CORS allowlist), `/v1/health`, `/v1/version` | server bootstrap + middleware chain shape | relay boots from a config file; `/v1/health` returns 200, `/v1/version` returns build info |
| 1-ii | **LLM `Provider` + Anthropic** — interface per §7; Anthropic Messages API streaming; token counts on `Done` | **`llm.Provider`** | Go test streams a real completion given `ANTHROPIC_API_KEY`; mock-based unit test for chunk assembly |
| 1-iii | **Session + anonymous auth + `/init` + `/turn`** — auth middleware contract (anonymous resolver only), session issuance/validation, bundled triage system prompt, SSE token streaming | **auth middleware contract + SSE `/turn` protocol** | `POST /init` issues a session; `POST /turn` streams assistant tokens end-to-end |
| 1-iv | **`Adapter` + webhook + `/submit`** — interface per §8; webhook adapter (URL/headers/retry); server-side `classify()`; canonical `payload.Payload` assembly + schema validation; routing to webhook | **`adapter.Adapter`** | `POST /submit` → local webhook receiver records the canonical payload with AI-derived fields |
| 1-v | **`@openintake/core`** — HTTP/SSE client, context capture (url/referrer/viewport/locale/user-agent), SSE consumer, `SubmitRequest` serializer; consumes Phase-0 generated types | client↔relay TS contract | Node script drives init→turn→submit against a running relay |
| 1-vi | **`@openintake/vue` (launcher+panel) + `examples/vue-anonymous`** — `IntakeWidget.vue`, `ConversationView.vue`, `useIntake` composable wrapping `@openintake/core` | — | **phase final smoke (§8)** |

**Dependency graph (mostly serial):**
```
1-i ─► 1-ii ─► 1-iii ─► 1-iv ─► 1-v ─► 1-vi
```
The relay HTTP contract (init/turn/submit shapes) must stabilize (through 1-iv) before the TS client (1-v) and widget (1-vi) consume it.

## 5. The frozen seams

### 5.1 `llm.Provider` (PROJECT.md §7, verbatim)
`Name() string` and `Chat(ctx, []Message, ChatOptions) (<-chan ChatChunk, error)` with `Message{Role,Content}`, `ChatOptions{Model,MaxTokens,Temperature,Stream}`, `ChatChunk{Delta,Done,InputTokens,OutputTokens}`. The Anthropic implementation uses the Messages API with `stream: true`; `InputTokens`/`OutputTokens` populated from the final `message_delta`/`message_stop` usage. Default model `claude-sonnet-4-6`. P2 adds openai/gemini/ollama behind this interface unchanged.

### 5.2 `adapter.Adapter` (PROJECT.md §8, verbatim)
`Name()`, `RequiresLicense() bool`, `Configure(map[string]any) error`, `Create(ctx, *payload.Payload) (*CreateResult, error)`, `HealthCheck(ctx) error`. The webhook adapter returns `RequiresLicense()=false`, POSTs the canonical payload JSON to the configured URL with configured headers, and retries per the configured policy. P3 adds chatwoot/fider/zendesk/linear behind this interface unchanged.

### 5.3 Auth middleware contract
A per-request resolver yields a `SessionContext{ AuthMode, Verified bool, UserID, Email, DisplayName *string, Custom map[string]any }` attached to the request context. Phase 1 implements **only** the anonymous resolver: `/init` issues a `session_id` (server-generated UUID); `/turn` and `/submit` carry it (header `X-Intake-Session`). The contract also recognizes `Authorization: Bearer <jwt>` so Phase 4 adds the email/SSO resolvers without changing handlers. Anonymous sessions set `Verified=false, AuthMode="anonymous"`.

### 5.4 SSE `/turn` protocol
`POST /v1/intake/turn`, body `{ "messages": [{role,content}, …] }` (the session travels in the `X-Intake-Session` header per §5.3, not the body — single source of truth), response `Content-Type: text/event-stream`:
- per token chunk: `data: {"delta":"…"}`
- terminal success: `data: {"done":true,"input_tokens":N,"output_tokens":M}`
- error: `data: {"error":"<message>"}` then stream close
The widget owns the full message history and sends it each turn (relay is stateless between turns). The relay prepends the bundled system prompt; it is never sent by or exposed to the client.

## 6. The SubmitRequest ↔ canonical-payload reconciliation

PROJECT.md §4 calls `payload.v1.json` the "widget↔relay wire contract," but §6 has the relay run `classify()` **at submit** — so the AI-derived `conversation.{summary,title,classification,severity,tags,language}` (all schema-*required*) cannot be present in the body the widget sends. Resolution:

- The client→relay `/submit` body is a thinner **`SubmitRequest`**: `{ messages[], client{...}, user_claims{...}, context{...}, attachments[], routing_hint? }` (session via the `X-Intake-Session` header per §5.3). The relay sets the assembled payload's `client.session_id` from that header.
- The relay runs **one** `classify()` LLM call producing `{summary,title,classification,severity,tags,language}`, assembles the full canonical **`payload.Payload`** (Phase-0 generated type), **validates it against `schema/payload.v1.json`**, then calls `Adapter.Create(payload)`.
- Therefore **`payload.v1.json` is the relay→adapter contract**, assembled server-side — not the literal `/submit` request body.

This is more robust than parsing structured output out of chat text and guarantees the schema's required AI fields are always present. The guided triage conversation still shapes the UX; the authoritative structured fields come from the server-side `classify()` at submit.

`/submit` response: `{ "external_id", "external_url", "adapter_name", "created_at" }` (mirrors `adapter.CreateResult`).

## 7. Configuration (Phase 1 subset of §9)

```yaml
server:
  addr: ":8080"
  external_url: "http://localhost:8080"
  cors_origins: ["http://localhost:5173"]   # the vue example dev server
llm:
  provider: "anthropic"
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-sonnet-4-6"
    max_tokens: 1024
  system_prompt_file: ""        # empty = bundled default triage prompt
auth:
  modes:
    anonymous: true
adapters:
  webhook:
    enabled: true
    url: "http://localhost:9099/intake"   # local receiver for the smoke
    headers: {}
    retry:
      max_attempts: 3
      backoff: "exponential"
```
Precedence: CLI flags > env vars > file. Secrets (`ANTHROPIC_API_KEY`, any webhook auth header value) come from env only. Bundled defaults cover everything except `adapters.webhook.url`.

## 8. Phase final smoke (mandatory)

```
1. Pre-condition: Phase-1 merged; Node 24.12 + Go 1.23.2; ANTHROPIC_API_KEY exported;
   a local webhook receiver running (e.g. a tiny Go/node server logging POST bodies on :9099);
   relay config points the webhook adapter at it.
2. Execution:
   a. Start the relay: `go run ./cmd/relay --config config.yaml`. /v1/health returns 200.
   b. Start the vue example: `npm run -w examples/vue-anonymous dev`; open it in a browser.
   c. Click the launcher, hold a multi-turn triage conversation (≥2 user turns); the assistant
      asks clarifying questions and proposes a summary.
   d. Click Submit.
3. Verification:
   - Assistant tokens stream visibly (SSE working).
   - The local webhook receiver logs ONE POST whose body is a schema-valid canonical payload:
     conversation.summary/title_suggestion/classification/severity_guess/tags_suggested populated
     by the server-side classify(); client.url/viewport/locale captured; user.auth_mode="anonymous",
     user.verified=false.
   - The widget shows the returned ticket result (external_id).
4. Teardown / repeat: stop relay + example; re-runnable. No persistent state (relay is stateless).
```

A non-API-key unit layer backs this: provider chunk-assembly (mock HTTP), webhook adapter (httptest receiver), handlers (httptest), and the core serializer (TS unit) are all tested without real credentials.

## 9. Error handling & non-goals

- **REST errors:** JSON envelope `{ "error": { "code", "message" } }`; schema-validation failure → `400`; provider failure → `502`; adapter failure after retries → `502` with adapter name.
- **SSE errors:** terminal `data: {"error":"…"}` frame, then close; the widget surfaces it in the panel.
- **CORS:** strict `cors_origins` allowlist; no wildcard.
- **Key redaction:** provider keys and webhook auth header values never logged; redacted in error messages.

**Explicitly NOT in Phase 1** (deferred exactly as the parent decomposition specifies): rate limiting / per-session caps / daily spend cap / CAPTCHA (**Phase 5 — required before any public exposure, see §2**); email + SSO auth (Phase 4); openai/gemini/ollama providers (Phase 2); chatwoot/fider/zendesk/linear adapters + license gate (Phase 3); attachments/screenshots (Phase 6); observability/metrics + release packaging (Phase 7).

## 10. ADRs locked by this phase (for the phase README)

- **Relay is the sole LLM broker; no provider keys in the browser** (§2). Trigger to revisit: never (this is a security invariant).
- **`payload.v1.json` is the relay→adapter contract, assembled server-side from a thinner `SubmitRequest`** (§6). Trigger to revisit: if a future client needs to submit a fully-formed payload (e.g. a server-to-server caller), add a separate ingress that still validates against the schema.
- **Relay is stateless between turns; widget owns conversation history** (PROJECT.md §6). Trigger to revisit: if per-turn payload size becomes a problem before the 20-turn/8000-token caps (Phase 5).
- **Anonymous-only auth in Phase 1, but the middleware contract accommodates Bearer JWT** (§5.3). Trigger to revisit: Phase 4 (email/SSO).
