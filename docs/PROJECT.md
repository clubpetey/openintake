# OpenIntake — v0 Specification

> **Product name:** OpenIntake (repo `github.com/clubpetey/openintake`, npm scope `@openintake`)
> **Status:** Draft v0 spec, pre-implementation
> **Audience:** Project maintainers, contributors, future Claude sessions

This document is the source of truth for v0 scope, architecture, and contracts. Implementation work should reference it. Open questions are tracked at the end — resolve them before building each affected component.

---

## 1. Overview

**OpenIntake** is an open-source, AI-native feedback and support intake system. It consists of:

- An **embeddable widget** (Vue 3 in v0, React in v1) that runs in a host web application.
- A **single-binary Go relay** that orchestrates LLM conversations, classifies and routes intake, and delivers tickets to downstream support/feedback systems.

The widget runs a short conversational triage with the end-user (powered by a pluggable LLM provider), captures contextual metadata (user identity, current URL, screenshot, host-app context), and POSTs a structured payload to the relay. The relay enriches, classifies, and forwards the payload to one or more configured adapters: Chatwoot, Fider, Zendesk, Linear, or a generic webhook.

### Why it exists

Existing feedback widgets either auto-attach context with no triage, or rely on closed SaaS backends. OpenIntake is the OSS, self-hostable, AI-native alternative — built for teams that want a real intake conversation, full control over their data, and a clean integration into the support and feedback tools they already use.

### Differentiators

- **Conversational triage** front-of-funnel, not post-hoc summarization.
- **Self-hostable** end to end: single Go binary, no SaaS dependency required.
- **Pluggable LLM provider** including Ollama for fully offline operation.
- **Multi-backend routing** in a single relay instance.
- **Privacy-first:** no telemetry, no phone-home, no third-party data sharing.

---

## 2. Goals and non-goals

### v0 goals

1. Working Vue 3 AND React widget embeddable in any modern web app.
2. Single-binary Go relay deployable as native executable or Docker container.
3. Five adapters: `chatwoot`, `fider`, `zendesk`, `linear`, `webhook`.
4. Three LLM providers: `anthropic`, `openai`, `ollama`.
5. Three authentication modes: anonymous, email magic-link, host-app SSO (JWT).
6. License-key enforcement for paid adapters (Zendesk, Linear).
7. Screenshot capture and forwarding.
8. Per-IP and per-session rate limiting, daily LLM spend cap.
9. Stateless operation (no persistent storage requirement).
10. Documented JSON payload schema as cross-component contract.

### v0 non-goals

- Console/network log capture (v1).
- Persistent ticket dedup/audit storage (v1).
- License revocation list / CRL (v1).
- Go plugin loading for third-party adapters (v2).
- Hosted-relay multi-tenancy (separate project, near-term).
- Mobile SDKs (later).
- Salesforce, Jira, Intercom adapters (later).

### Success criteria for v0

- A developer can embed the widget in a React or Vue app, run the relay locally, and route a real ticket into Chatwoot in under 15 minutes following the README.
- License gating correctly blocks paid adapters without a valid license and permits them with a valid one.
- Relay handles 50 concurrent sessions on a $5 VPS without measurable degradation.
- All three LLM providers complete a 5-turn conversation end-to-end.

---

## 3. Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Host web application (any framework; React/Vue widget)     │
│                                                             │
│  ┌───────────────────────────────────────────────────┐      │
│  │  @openintake/vue widget                               │      │
│  │  ┌─────────────────────────────────────────┐      │      │
│  │  │  @openintake/core (shared TS engine)        │      │      │
│  │  │  - Conversation client                  │      │      │
│  │  │  - Context capture (URL, app context)   │      │      │
│  │  │  - Screenshot capture (html2canvas)     │      │      │
│  │  │  - SSE consumer                         │      │      │
│  │  │  - Payload serializer                   │      │      │
│  │  └─────────────────────────────────────────┘      │      │
│  └───────────────────────────────────────────────────┘      │
└──────────────────────────┬──────────────────────────────────┘
                           │ HTTPS
                           │  POST /v1/intake/turn   (SSE)
                           │  POST /v1/intake/submit (final)
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  openintake-relay (Go binary)                                   │
│                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌──────────────┐       │
│  │ HTTP server │ → │ Auth /      │ → │ Rate limit / │       │
│  │ (chi)       │   │ session     │   │ abuse guard  │       │
│  └─────────────┘   └─────────────┘   └──────┬───────┘       │
│                                             │               │
│              ┌──────────────────────────────┼────┐          │
│              ▼                              ▼    │          │
│      ┌───────────────┐              ┌─────────────────┐     │
│      │ LLM provider  │              │ Classifier +    │     │
│      │ (pluggable)   │              │ router          │     │
│      │  - anthropic  │              └────────┬────────┘     │
│      │  - openai     │                       │              │
│      │  - ollama     │              ┌────────┴───────┐      │
│      └───────────────┘              ▼                ▼      │
│                              ┌──────────────┐  ┌─────────┐  │
│                              │ Adapter pool │  │ License │  │
│                              │  - chatwoot  │  │ gate    │  │
│                              │  - fider     │  │         │  │
│                              │  - zendesk*  │  │         │  │
│                              │  - linear*   │  │         │  │
│                              │  - webhook   │  │         │  │
│                              └──────┬───────┘  └─────────┘  │
└─────────────────────────────────────┼───────────────────────┘
                                      │
                                      ▼
                          (downstream support / feedback systems)

*Paid adapters; require valid license to enable.
```

### Component summary

| Component | Lang | Distribution |
|---|---|---|
| `@openintake/core` | TypeScript | npm |
| `@openintake/vue` | TypeScript / Vue 3 | npm |
| `openintake-relay` | Go | GitHub Releases (multi-platform), Docker image, `go install` |
| `openintake-license` (CLI) | Go | Internal tool; not distributed publicly in v0 |
| `payload.v1.json` | JSON Schema | Repo source of truth; codegen target for both TS and Go |

---

## 4. Payload schema (v1)

The wire contract between widget and relay. All other components must conform.

### Schema source of truth

```
/schema/payload.v1.json
```

CI generates:
- `core/src/generated/payload.ts` (via `quicktype` or `json-schema-to-typescript`)
- `relay/internal/payload/types.go` (via `quicktype` or `go-jsonschema`)

CI fails if generated files are stale relative to the schema.

### Payload structure

```jsonc
{
  "schema_version": "1.0",

  "submission": {
    "id": "uuid",                           // client-generated
    "submitted_at": "ISO-8601 UTC",
    "tenant_id": "string | null"            // for multi-tenant relays (v1 hosted)
  },

  "client": {
    "widget_version": "0.4.1",
    "session_id": "uuid",                   // stable across one user session
    "url": "https://app.example.com/page",
    "referrer": "string | null",
    "user_agent": "string",
    "viewport": { "w": 1440, "h": 900 },
    "locale": "en-US"
  },

  "user": {
    "auth_mode": "anonymous | email | sso",
    "id": "string | null",                  // host-app user ID when SSO
    "email": "string | null",
    "display_name": "string | null",
    "verified": true,                       // false for unverified anon/email
    "custom": { /* host-app context */ }    // free-form
  },

  "conversation": {
    "messages": [
      { "role": "user" | "assistant", "content": "string", "ts": "ISO-8601" }
    ],
    "summary": "string",                    // AI-generated 1-2 sentence
    "title_suggestion": "string",           // AI-generated, <80 chars
    "classification": "bug | feature_request | question | other",
    "severity_guess": "low | medium | high | critical | unknown",
    "tags_suggested": ["string"],
    "language": "en"                        // detected
  },

  "context": {
    "app_context": { /* host-app provided */ },
    "page_metadata": { /* og tags, title */ }
  },

  "attachments": [
    {
      "type": "screenshot | file",
      "mime_type": "string",
      "size_bytes": 12345,
      "url": "data:..." | "https://...",    // base64 or pre-uploaded
      "label": "string"
    }
  ],

  "routing_hint": "string | null"            // optional: caller-suggested adapter
}
```

### Schema versioning

- Schema is versioned as `payload.vN.json`. Breaking changes bump `N`.
- Widget advertises its `schema_version` in every request.
- Relay supports current major version and the previous one; rejects older with 426 Upgrade Required.
- Additive (backward-compatible) changes do not bump the version; consumers must tolerate unknown fields.

---

## 5. Authentication modes

The widget supports three modes; the relay validates per its configuration.

### Mode A: Anonymous

- No identity verification.
- `user.verified = false`. `user.auth_mode = "anonymous"`.
- Optional CAPTCHA challenge before LLM access (Cloudflare Turnstile or hCaptcha).
- Use case: public bug-report widgets, OSS landing pages.

### Mode B: Email magic-link

- Widget collects email at conversation start.
- Widget POSTs to `/v1/intake/auth/email/start` → relay sends 6-digit code via configured SMTP.
- Widget POSTs `/v1/intake/auth/email/verify` with code → relay returns short-lived JWT (15 min).
- All subsequent requests carry the JWT in `Authorization: Bearer …`.
- `user.verified = true` only after code verification.

### Mode C: Host-app SSO

- Host app already authenticates the user (Auth0, Cognito, custom JWT, OAuth).
- Host app provides a JWT to the widget at init time.
- Relay validates the JWT against configured issuer/audience/JWKS endpoint.
- Supports Auth0, OIDC-generic, custom HS256/RS256.

A single relay instance can enable any combination of these modes.

---

## 6. Conversation flow

### Sequence

```
Widget                            Relay                    LLM Provider
  │                                 │                          │
  │  init (config from host app)    │                          │
  ├────────────────────────────────►│                          │
  │  ◄──── session_id, capabilities │                          │
  │                                 │                          │
  │  (optional) auth                │                          │
  ├────────────────────────────────►│                          │
  │  ◄──── JWT                      │                          │
  │                                 │                          │
  │  user types message 1           │                          │
  ├──── POST /v1/intake/turn ──────►│                          │
  │     (SSE stream opens)          │   chat(messages, ctx)    │
  │                                 ├─────────────────────────►│
  │  ◄──── assistant tokens stream  │  ◄── tokens stream       │
  │                                 │                          │
  │  user types message N           │                          │
  ├──── POST /v1/intake/turn ──────►│                          │
  │  ◄──── tokens                   ├─────────────────────────►│
  │                                 │                          │
  │  user clicks "Submit"           │                          │
  ├──── POST /v1/intake/submit ────►│                          │
  │     (full payload + attachments)│                          │
  │                                 │  classify(conversation)  │
  │                                 ├─────────────────────────►│
  │                                 │  ◄── classification      │
  │                                 │                          │
  │                                 │  route → adapter         │
  │                                 │  adapter.create(payload) │
  │                                 │  ────► downstream system │
  │  ◄──── ticket_id, external_url  │                          │
```

### Endpoints

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/intake/init` | Returns session_id, server capabilities, allowed auth modes |
| `POST` | `/v1/intake/auth/email/start` | Begin email magic-link |
| `POST` | `/v1/intake/auth/email/verify` | Verify code, return JWT |
| `POST` | `/v1/intake/turn` | One conversational turn; returns SSE stream of assistant tokens |
| `POST` | `/v1/intake/submit` | Final submission; returns adapter result |
| `GET`  | `/v1/health` | Health check |
| `GET`  | `/v1/version` | Build info |

All endpoints return JSON unless explicitly SSE.

### Turn semantics

- Each `POST /v1/intake/turn` includes the full conversation so far (the widget owns conversation state).
- Relay is stateless between turns; it does not store conversation history.
- Session state held only in client; relay validates session_id matches the JWT (if any) and rate-limit bucket.

This stateless turn design simplifies the relay and aligns with the v0 "no persistent storage" goal. Trade-off: per-turn payloads grow with conversation length. Accepted given the 20-turn / 8000-token caps.

---

## 7. LLM provider interface

Providers are pluggable. v0 ships three.

### Go interface

```go
// relay/internal/llm/provider.go

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
    Delta       string // incremental text
    Done        bool
    InputTokens int    // populated on Done
    OutputTokens int   // populated on Done
}

type Provider interface {
    Name() string
    Chat(ctx context.Context, messages []Message, opts ChatOptions) (<-chan ChatChunk, error)
}
```

### v0 implementations

| Provider | Package | Notes |
|---|---|---|
| `anthropic` | `relay/internal/llm/anthropic` | Default model: `claude-sonnet-4-6`. Uses native streaming. |
| `openai` | `relay/internal/llm/openai` | Default model: configurable. Uses native streaming. |
| `gemini` | `relay/internal/llm/gemini` | Default model: configurable. |
| `ollama` | `relay/internal/llm/ollama` | Default model: configurable. Local-only; no API key needed. |

### Provider selection

Relay config selects one provider at startup. Multi-provider routing is out of scope for v0.

### System prompt strategy

A built-in system prompt guides the LLM through structured triage:

1. Greet briefly, ask user to describe the issue.
2. Ask up to 3 clarifying questions to disambiguate (severity, scope, reproduction).
3. When sufficient detail is gathered, propose a summary and ask user to confirm.
4. On confirmation, produce a structured output: `{ summary, title, classification, severity, tags }`.

The system prompt is overridable via relay config for power users. Default prompt is bundled.

---

## 8. Adapter interface

Adapters translate the canonical payload into downstream system API calls.

### Go interface

```go
// relay/internal/adapter/adapter.go

package adapter

import (
    "context"
    "github.com/clubpetey/openintake/relay/internal/payload"
)

type CreateResult struct {
    ExternalID  string  // ticket/issue ID in downstream system
    ExternalURL string  // user-visible URL, if available
    AdapterName string
    CreatedAt   string  // ISO-8601
}

type Adapter interface {
    Name() string
    RequiresLicense() bool                   // true for paid adapters
    Configure(config map[string]any) error
    Create(ctx context.Context, p *payload.Payload) (*CreateResult, error)
    HealthCheck(ctx context.Context) error
}
```

### v0 adapter list

| Adapter | License | Target API | Notes |
|---|---|---|---|
| `webhook` | Free | Generic HTTPS POST | Configurable URL, headers, retry policy |
| `chatwoot` | Free | Chatwoot REST API | Creates conversation in configured inbox |
| `fider` | Free | Fider API | Creates post in configured board |
| `zendesk` | **Paid** | Zendesk REST API | Creates ticket; supports custom fields |
| `linear` | **Paid** | Linear GraphQL API | Creates issue in configured team/project |

### Adapter resolution

1. If `routing_hint` in payload matches an enabled adapter, use it.
2. Else, apply configured routing rules (see configuration).
3. Else, fall back to default adapter (configurable; defaults to first enabled).

Multi-adapter dispatch (sending one ticket to multiple systems) is out of scope for v0. Reconsider in v1.

---

## 9. Configuration

Relay config is a single YAML file. Environment variables override file values. CLI flags override env vars.

### Example

```yaml
server:
  addr: ":8080"
  external_url: "https://openintake.example.com"
  cors_origins:
    - "https://app.example.com"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

license:
  file: "/etc/openintake/license.json"
  # Alternatively: license env var INTAKE_LICENSE (base64-encoded license JSON)

auth:
  modes:
    anonymous: true
    email: true
    sso: true
  email:
    smtp_host: "smtp.example.com"
    smtp_port: 587
    smtp_user: "intake@example.com"
    smtp_pass_env: "INTAKE_SMTP_PASS"
    from: "Intake <intake@example.com>"
    code_ttl: "10m"
    jwt_ttl: "15m"
    jwt_secret_env: "INTAKE_EMAIL_JWT_SECRET"
  sso:
    issuer: "https://example.us.auth0.com/"
    audience: "https://api.example.com"
    jwks_url: "https://example.us.auth0.com/.well-known/jwks.json"

captcha:
  enabled: true
  provider: "turnstile"
  site_key: "0x4AAA..."
  secret_key_env: "INTAKE_TURNSTILE_SECRET"
  required_for: ["anonymous"]

llm:
  provider: "anthropic"
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-sonnet-4-6"
    max_tokens: 1024
  # openai: { ... }
  # ollama: { base_url: "http://localhost:11434", model: "llama3.1" }
  system_prompt_file: ""   # empty = use bundled default

ratelimit:
  per_ip:
    requests_per_second: 1
    burst: 5
  per_session:
    max_turns: 20
    max_input_tokens: 8000
  daily_llm_budget:
    max_input_tokens: 5000000
    max_output_tokens: 1000000
    action_on_exceeded: "reject"   # or "queue"

routing:
  default_adapter: "chatwoot"
  rules:
    - when: { classification: "bug" }
      to: "chatwoot"
    - when: { classification: "feature_request" }
      to: "fider"
    - when: { classification: ["question", "other"] }
      to: "chatwoot"

adapters:
  chatwoot:
    enabled: true
    base_url: "https://chatwoot.example.com"
    api_token_env: "CHATWOOT_TOKEN"
    inbox_id: 3
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
  webhook:
    enabled: false
    url: "https://hooks.example.com/intake"
    headers:
      X-Auth-Token: "${WEBHOOK_TOKEN}"
    retry:
      max_attempts: 3
      backoff: "exponential"

attachments:
  max_size_bytes: 5242880    # 5 MB
  allowed_mime_types:
    - "image/png"
    - "image/jpeg"
    - "image/webp"
  storage:
    mode: "forward"            # forward to adapter only; no local storage
    # mode: "s3" — v1

observability:
  log_level: "info"
  log_format: "json"
  metrics:
    enabled: true
    addr: ":9090"              # Prometheus
```

---

## 10. Rate limiting and abuse protection

### Per-IP

Token bucket: 1 req/s, burst 5 by default. Configurable. Applied to all `/v1/intake/*` endpoints.

Implementation: `relay/internal/ratelimit/perip` — `golang.org/x/time/rate`-backed bucket per client IP, eager-GC.

### Per-session

- Max 20 turns per session.
- Max 8000 input tokens cumulative per session.
- Session bucket TTL: 1 hour from creation.

Implementation: `relay/internal/auth` (`NewStoreWithCaps` + `CheckSession` + `RecordTurn`).

### Daily LLM spend cap

- Relay tracks total input/output tokens against configured daily budget.
- On exceeded: reject new LLM calls with 503 and a `Retry-After` for next UTC midnight.
- Counters reset at 00:00 UTC.
- Per-tenant counters supported when `tenant_id` present (hosted relay).

Implementation: `relay/internal/budget` — Reserve/Commit + UTC-day reset + tenant isolation.

### CAPTCHA

- Optional; configurable per auth mode (default: required for anonymous, off for verified).
- v0 supports Cloudflare Turnstile (recommended; free) and hCaptcha.
- Challenge solved at `/v1/intake/init`; token bound to session_id.

Implementation: `relay/internal/captcha` — Turnstile + hCaptcha siteverify + 5-minute single-use replay set.

### Origin enforcement

- `cors_origins` allowlist enforced strictly on browser requests.
- Server-to-server callers (e.g., webhook adapter receivers verifying signatures) use a separate auth path with signed requests.

Implementation: `relay/internal/server/server.go` `corsMiddleware` (Phase 1; unchanged in Phase 5).

---

## 11. Attachment handling

v0 scope: **screenshots only**.

### Capture

- Widget uses `html2canvas` to capture the current page or a user-selected element.
- User can blur/redact regions before submission.
- Encoded as base64 PNG in payload.

Implementation: `core/src/capture.ts` (DI-injectable `html2canvas` wrapper — see L021 for the SSR-safety pattern), `core/src/attachments.ts` (pending-attachment list + aggregate-size accounting), `vue/src/components/ScreenshotRedactor.vue` (full-screen modal + rectangle redaction), `vue/src/components/AttachmentStrip.vue` (pending strip + aggregate-size badge). Operator + UX reference: `docs/attachments.md`.

### Transport

- Sent inline in the `attachments[]` array on `POST /v1/intake/submit`.
- Max 5 MB per attachment (configurable). Multiple attachments allowed up to total payload size cap (10 MB default).
- Relay validates MIME type magic-byte against declared `mime_type`.

Implementation: `relay/internal/attachvalidate/` (magic-byte + size-cap validation; sentinel errors mapped 1:1 to HTTP codes), `relay/internal/server/submit.go` (orchestration: body-cap raise to 14 MB when `cfg.Attachments.Enabled=true`, `attachvalidate.ValidateAll` after `payloadbuild.Build` and before `Router.Route`), `relay/internal/server/init.go` (capabilities intersection emitted under `capabilities.attachments`). Validation error matrix: `docs/attachments.md` "Validation errors".

### Forwarding

- Each adapter is responsible for forwarding attachments to its downstream system using that system's native upload mechanism (Chatwoot media upload, Zendesk attachments endpoint, Linear file upload, etc.).
- Relay does not store attachments locally in v0.

Implementation: `relay/internal/adapter/capabilities.go` (optional `CapableAdapter` interface advertising accepted MIME types), per-adapter `Create()` native sequences in `relay/internal/adapter/{webhook,chatwoot,fider,linear,zendesk}/`. Chatwoot uses a three-call flow (contacts → conversations(JSON) → messages(multipart)) because the conversation-create endpoint silently drops `attachments[]` — see L020. Operator-facing per-adapter matrix: `docs/attachments.md` "Per-adapter behavior".

### v1 enhancements (out of scope)

- Console log capture
- Network request log capture
- Video/screen recording
- S3-backed staging storage

---

## 12. License model

### Overview

v0 uses **runtime license-key verification**:

- License = signed JSON blob.
- Signature: Ed25519 (`crypto/ed25519`, Go stdlib).
- Public key embedded as a Go constant in the relay binary.
- No phone-home, no online activation, no telemetry.

### License JSON structure

```json
{
  "license_id": "lic_2026_abc123def",
  "issued_to": {
    "org": "Example Org",
    "email": "billing@example.com"
  },
  "tier": "pro",
  "adapters": ["zendesk", "linear"],
  "max_relay_instances": 3,
  "issued_at": "2026-05-26T00:00:00Z",
  "expires_at": "2027-05-26T00:00:00Z",
  "signature": "ed25519:base64(...)"
}
```

The `signature` covers the canonical JSON of all other fields. Verification fails on any tampering.

### Load order

Relay searches for the license in this order:

1. CLI flag: `--license-file=/path/to/license.json`
2. Env var: `INTAKE_LICENSE` (base64-encoded JSON)
3. Env var: `INTAKE_LICENSE_FILE` (path)
4. Default path: `/etc/openintake/license.json`
5. Default path: `~/.config/openintake/license.json`

If no license found, relay starts in **free mode** — all free adapters available, all paid adapters disabled (with clear startup log).

### Trial mode

- A new relay install (no license, no prior trial state) enters a **14-day trial** automatically.
- During trial, all adapters including paid ones are enabled.
- Trial state stored in `~/.config/openintake/state.json` (or configured state path).
- Startup log clearly displays remaining trial days.
- On trial expiry: paid adapters disabled, free adapters continue working.

### Verification flow

1. Load license JSON (from any of the locations above).
2. Verify Ed25519 signature with embedded public key. Fail loudly on bad signature.
3. Check `expires_at` > now. Fail with clear error on expired license.
4. Cache verified license in memory.
5. When loading adapter from config: if `RequiresLicense()` is true, check `adapter.Name()` is in license.adapters. Otherwise refuse to enable with: `adapter "X" requires a license — see https://[product].com/pricing`.

### Key management (maintainer side)

- Ed25519 keypair generated once. Private key stored offline (1Password, hardware token, paper backup).
- Public key committed to the repo as a Go constant.
- License generation tool: separate Go CLI (`openintake-license`) not distributed publicly. Used by maintainer to sign new licenses.

### Revocation

Not in v0. License duration is 1 year; revocation handled at renewal. v1 may add an optional CRL fetched from a maintainer-controlled URL.

### Pirate resistance

License-key verification is friction, not DRM. A determined attacker can patch out the verification call. This is acceptable:

- Companies running Zendesk have legal exposure for license violation.
- The customers worth winning pay because compliance is cheaper than rebuilding the gate.
- OSS goodwill from a transparent model outweighs marginal piracy.

---

## 13. Free / paid adapter list

### v0

| Adapter | Tier | Reasoning |
|---|---|---|
| `webhook` | Free | Generic; no upstream cost; table-stakes |
| `chatwoot` | Free | OSS upstream; aligns with OSS positioning |
| `fider` | Free | OSS upstream; same |
| `zendesk` | Paid | Enterprise/SMB segment with budget; clear willingness to pay |
| `linear` | Paid | Modern dev-team segment with budget |

### Anticipated v1+

| Adapter | Tier | Notes |
|---|---|---|
| `github-issues` | Free | GitHub is free for users; charging here feels off |
| `jira` | Paid | High API maintenance cost; enterprise customers |
| `intercom` | Paid | Same segment as Zendesk |
| `slack` (notification) | Free | Read-only fan-out |
| `slack` (interactive thread) | Paid | Full bidirectional conversation |
| `salesforce` | Paid | Enterprise-only |

---

## 14. Repository layout

Single monorepo. v0 layout:

```
.
├── README.md
├── LICENSE                     # Apache 2.0 for core; commercial terms for paid adapters
├── CONTRIBUTING.md
├── SECURITY.md
├── .github/
│   └── workflows/
│       ├── ci.yml              # lint, test, codegen-check
│       └── release.yml         # goreleaser, npm publish
├── schema/
│   ├── payload.v1.json
│   └── codegen.config.json
├── core/                       # @openintake/core (TypeScript)
│   ├── package.json
│   ├── src/
│   │   ├── index.ts
│   │   ├── client.ts           # conversation HTTP/SSE client
│   │   ├── context.ts          # URL, viewport, etc capture
│   │   ├── capture.ts          # html2canvas DI wrapper (Phase 6 — see L021)
│   │   ├── attachments.ts      # pending-attachment state + size accounting
│   │   ├── sse.ts              # SSE parser/decoder
│   │   ├── client-types.ts     # client-internal types
│   │   ├── types.ts            # shared TS types (includes SubmitAttachment from Phase 6)
│   │   └── generated/
│   │       └── payload.ts      # generated from schema
│   └── tsconfig.json
├── vue/                        # @openintake/vue (TypeScript)
│   ├── package.json
│   ├── src/
│   │   ├── index.ts
│   │   ├── components/
│   │   │   ├── IntakeWidget.vue
│   │   │   ├── ConversationView.vue
│   │   │   ├── ScreenshotRedactor.vue    # Phase 6 — rectangle redaction modal
│   │   │   └── AttachmentStrip.vue       # Phase 6 — pending thumbnail strip
│   │   └── composables/
│   │       └── useIntake.ts
│   └── tsconfig.json
├── relay/                      # openintake-relay (Go)
│   ├── go.mod
│   ├── cmd/
│   │   └── relay/
│   │       └── main.go
│   ├── internal/
│   │   ├── config/
│   │   ├── server/
│   │   ├── auth/
│   │   ├── ratelimit/
│   │   ├── license/
│   │   ├── llm/
│   │   │   ├── provider.go
│   │   │   ├── anthropic/
│   │   │   ├── openai/
│   │   │   └── ollama/
│   │   ├── adapter/
│   │   │   ├── adapter.go
│   │   │   ├── webhook/
│   │   │   ├── chatwoot/
│   │   │   ├── fider/
│   │   │   ├── zendesk/
│   │   │   └── linear/
│   │   ├── router/
│   │   └── payload/
│   │       └── types.go         # generated from schema
│   ├── Dockerfile
│   └── .goreleaser.yaml
├── license-tool/               # Maintainer-only CLI; not published
│   ├── go.mod
│   └── cmd/openintake-license/
│       └── main.go
├── docs/
│   ├── quickstart.md
│   ├── self-hosting.md
│   ├── adapters/
│   │   ├── chatwoot.md
│   │   ├── fider.md
│   │   ├── zendesk.md
│   │   ├── linear.md
│   │   └── webhook.md
│   ├── llm-providers.md
│   ├── auth-modes.md
│   ├── license.md
│   └── widget-integration.md
└── examples/
    ├── vue-anonymous/          # minimal Vue + anonymous mode
    ├── vue-sso/                # Vue + JWT SSO + multi-adapter
    └── docker-compose/         # one-command demo stack
```

### License terms

- Apache 2.0 for the widget, core, relay framework, free adapters, LLM providers, docs, and examples.
- A separate `COMMERCIAL.md` file describes the license required to use paid adapters in production. The paid adapter source code lives in the same repo under Apache 2.0 — the *use* of paid adapter functionality is gated by license key at runtime.

This approach (open source code, license-key-gated runtime use) follows the pattern used by Sentry, Cal.com, and Mattermost.

---

## 15. Build and release

### CI

GitHub Actions on every PR:

- Lint: `golangci-lint`, `eslint`, `prettier`.
- Codegen check: regenerate types from schema; fail if diff.
- Tests: Go unit + integration, TS unit.
- Build artifacts: relay binary, npm packages (dry-run).

### Release

Triggered by tagging `vX.Y.Z`:

- `goreleaser` builds relay binaries for: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64.
- Docker image published to GitHub Container Registry: `ghcr.io/[org]/openintake-relay:vX.Y.Z`.
- npm packages published: `@openintake/core@X.Y.Z`, `@openintake/vue@X.Y.Z`.
- GitHub Release with changelog and checksums.

The `openintake-license` maintainer CLI in `license-tool/` is excluded from goreleaser per the v0 decomposition decision Q10 (it is not distributed publicly in v0; the maintainer runs it locally to sign new licenses). It is built ad-hoc via `go build -o openintake-license ./license-tool/cmd/openintake-license`.

### Versioning

- Semantic versioning across all packages.
- npm packages and relay binary share the same version number to reduce confusion.
- Payload schema version is independent (`payload.v1.json`, `payload.v2.json`, …); both packages must reference compatible schema versions.

---

## 16. Multi-tenancy hooks (for near-term hosted relay)

The hosted relay is a separate near-term project. v0 self-hosted relay must not preclude it. Required hooks:

- `tenant_id` field present in payload schema (`null` for self-hosted).
- Rate limits and LLM budget counters keyed by `tenant_id` when present.
- Adapter config supports per-tenant overrides (deferred to hosted-relay codebase; relay framework just exposes the interface).
- License model supports a "hosted" tier marker that grants all adapters (the hosted relay uses one master license; tenant entitlement is enforced separately at the hosted-relay layer).

The hosted relay will be a separate binary (or build mode) that wraps the OSS relay with multi-tenant orchestration, billing, and tenant management. Architecture for that is out of scope here.

---

## 17. Security considerations

- All endpoints require TLS in production. Relay can do TLS termination directly or sit behind a reverse proxy.
- Email codes: 6 digits, 10-minute TTL, single-use, rate-limited per email (max 3 codes / 10 min / address).
- JWT secrets: minimum 32 bytes; rejected if shorter.
- All SMTP and adapter credentials sourced from env vars, never from config file directly.
- Payload validation: strict JSON Schema validation; reject unknown top-level fields in v1.0.
- Attachment MIME validation: magic-byte check, not just declared type.
- CORS: strict allowlist; no wildcards in production guidance.
- LLM provider API keys: never logged; redacted in error messages.
- License private key: never in the repo; maintainer-only.
- CAPTCHA siteverify secret: never logged; never in any returned error (`relay/internal/captcha` — redact-before-error per LESSONS L005).

A `SECURITY.md` file documents vulnerability disclosure process.

---

## 18. Out of scope for v0

Documented to prevent scope creep:

- React widget (v1)
- Angular / Svelte widgets (later)
- Mobile SDKs (later)
- Persistent ticket storage on relay (v1)
- Adapter retry queue / DLQ (v1)
- Console log / network log capture (v1)
- Video recording (later)
- License revocation list (v1)
- Hosted relay multi-tenancy (separate project)
- Go plugin loading for third-party adapters (v2)
- Multi-adapter dispatch (v1)
- Conversational UI customization beyond theming (v1)
- Whitelabeling / OEM (later)
- AI-driven auto-resolution / Fin-style agent (post-v1)
- Workflow automation / rules engine (post-v1)

---

## 19. Resolved decisions

These were the v0 open questions. All are now resolved; recorded here for provenance.

1. **Product name** — **OpenIntake**. Repo `github.com/clubpetey/openintake`; npm scope `@openintake`; Go module path, GitHub org, and binary names (`openintake-relay`, `openintake-license`) follow.
2. **Pricing** — Finalized in `COMMERCIAL.md`: a single commercial license at **$1,500/yr** (both paid adapters + up to 3 production relay instances), **+$400/yr** per additional instance, plus an **Enterprise/custom** tier (>3 instances, air-gapped, or bundled support). Supersedes the earlier Free/Pro/Team sketch.
3. **Trial-state file location** — `license.DefaultStatePath()` uses `os.UserConfigDir()`: `%AppData%\openintake\state.json` (Windows), `~/.config/openintake/state.json` (Linux), `~/Library/Application Support/openintake/state.json` (macOS). Cross-platform, no per-OS branching.
4. **Ollama auth** — Optional bearer token via `llm.ollama.bearer_token_env`. The `Authorization: Bearer` header is sent only when configured; the token is redacted from logs and error messages.
5. **Default models** — Applied in `config.go` when the model is unset: `anthropic: claude-sonnet-4-6`, `openai: gpt-4o-mini`, `gemini: gemini-2.0-flash`, `ollama: llama3.1`. All overridable via `llm.<provider>.model`.
6. **System prompt licensing** — No special treatment: the bundled prompt (`relay/internal/triage/prompt.txt`) is Apache 2.0 like all source, and operator-overridable at runtime via `llm.system_prompt_file` (§7). No "prompt-IP" carve-out.
7. **Schema codegen tool** — `json-schema-to-typescript` (TS) + `go-jsonschema` (Go), driven by `npm run codegen` / `scripts/codegen-go.sh`. CI fails on stale generated files.
8. **Attachment redaction UX** — No forced "no-PII" confirmation by default; the widget ships redaction tooling, and a host app may opt into a forced acknowledgment via `require_redaction_ack` (default `false`). Redaction responsibility otherwise sits with the host app.
9. **Anonymous mode without CAPTCHA** — Fail-closed: anonymous requires a CAPTCHA by default. An operator may explicitly opt out via `auth.anonymous.allow_without_captcha` (default `false`); the relay never silently allows it.
10. **License-generation tool** — Maintainer-only. The `license-tool/` source stays in the repo (transparency; it shares `relay/license` with the runtime gate) but is excluded from all release artifacts and ships no public binary. Issuing a valid license requires the maintainer's offline Ed25519 private key, which is never committed — so the tool's presence does not enable license forgery.

---

## 20. Glossary

| Term | Meaning |
|---|---|
| **Widget** | The embeddable Vue (or future React) component that runs in a host app. |
| **Relay** | The Go binary that orchestrates LLM calls and routes payloads to adapters. |
| **Host app** | The web application embedding the widget. |
| **Adapter** | A relay plugin that translates the canonical payload to a downstream system's API. |
| **Provider** | An LLM backend (Anthropic, OpenAI, Ollama). |
| **Session** | One end-user's intake conversation; bounded by turn/token caps. |
| **Tenant** | A logical isolation boundary (only meaningful in the hosted relay). |
| **License** | A signed JSON blob granting use of paid adapters. |
| **Payload** | The canonical JSON document sent from widget to relay, defined by `payload.v1.json`. |

---

*End of v0 spec.*
