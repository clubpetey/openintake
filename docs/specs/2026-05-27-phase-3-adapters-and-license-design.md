# Phase 3 — Adapters + License gate: Design

> **Status:** Approved design (brainstorming output), pre-planning
> **Date:** 2026-05-27
> **Implements:** [docs/PROJECT.md](../PROJECT.md) §8 (adapter interface), §12 (license model), §13 (free/paid list), §16 (multi-tenancy hooks)
> **Parent:** [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 3 row + §3.1 frozen seams + §4 Q3/Q10)
> **Builds on:** Phase 1's frozen `adapter.Adapter` interface (`relay/internal/adapter/adapter.go`) + the `webhook` reference impl; the `config.ResolveSecret` seam ([2026-05-27-configuration-and-secrets-design.md](2026-05-27-configuration-and-secrets-design.md))

## 1. Goal

Add the four remaining v0 adapters (`chatwoot`, `fider` free; `zendesk`, `linear` paid) behind the **frozen** `adapter.Adapter` interface; add a **router** that resolves a submission to one adapter (`routing_hint` → rules → default, PROJECT.md §8); add **Ed25519 license verification** (embedded public key, signed-JSON license, load order, 14-day trial, free-mode fallback, PROJECT.md §12); apply the **license gate** to the two paid adapters; and implement the maintainer-only **`intake-license` CLI** (PROJECT.md §12 key management, decomposition Q10).

Success (decomposition §2, Phase 3 row): "Route a real ticket into a **live Chatwoot**; paid adapter blocked w/o license, permitted w/ signed test license; free-mode disables paid adapters with a clear startup log."

The `webhook` adapter is the reference; each new adapter replicates its shape: a `New()` constructor returning an unconfigured `*Adapter`, `Configure(map[string]any) error`, a context-respecting `Create` over stdlib `net/http`, `HealthCheck`, and **credentials never logged**. Attachment forwarding is **out of scope for Phase 3** (P3 → P6 edge in the decomposition graph): these adapters create text tickets; Phase 6 adds native-upload forwarding.

## 2. The seams this phase introduces

### 2.1 Routing seam (generalizes the single adapter)

Today `submit.go` dispatches to a single `deps.Adapter.Create`. Phase 3 replaces that with a router:

```go
// relay/internal/router/router.go — package router
type Router struct { /* registry + rules + default */ }
func New(reg map[string]adapter.Adapter, rules []Rule, defaultName string) (*Router, error)
func (r *Router) Route(p *payload.IntakePayload) (adapter.Adapter, error)
```

- **Registry:** `map[string]adapter.Adapter` of the **enabled + license-permitted** adapters, built in `main.go` from config (the gate, §4, decides which paid adapters make it in).
- **Resolution order** (PROJECT.md §8): `routing_hint` (if it names a *registered* adapter) → first matching rule (`when:{classification|severity}` → `to`) → `default_adapter`.
- **`server.Deps`:** the `Adapter adapter.Adapter` field becomes `Router *router.Router`; `submit.go` calls `deps.Router.Route(p)` then `.Create`. This touches the server but **not** the §3.1 frozen seams — the `adapter.Adapter` interface, the chi route-registration shape (`registerIntakeRoutes`), and the middleware chain are all unchanged.

**Why a separate `router` package (not in `server`):** keeps routing logic independently unit-testable with fake adapters and no HTTP, and keeps `server` focused on transport. `server` imports `router`; `router` imports `adapter` + `payload`; no back-edge.

### 2.2 License seam (shared-package split, honoring Go's `internal/` rule)

The relay (module `intake`) **verifies** with an embedded *public* key; the `intake-license` CLI (module `intake-license-tool`) **signs** with the *private* key. Both must agree byte-for-byte on canonicalization, but `internal/` blocks the CLI from importing `intake/internal/...`. Resolution (chosen approach): a small **importable** package.

- **`relay/license/`** (package `license`, **NOT** under `internal/`): the `License` struct, `Canonicalize`, `Sign(priv, *License)`, `Verify(pub, blob) (*License, error)`. Pure `crypto/ed25519` + `encoding/json`; zero relay-internal dependencies. **Single source of truth for canonicalization.**
- **`relay/internal/license/`** (package `license`, relay-only): the **embedded public-key constant**, the **loader** (load order), the **trial/free state machine**, and the **gate** (`Permits(adapterName) bool`). Imports `intake/license`.
- **`license-tool/`**: imports `intake/license` via `replace intake => ../relay` in its `go.mod`; adds the `keygen`/`sign`/`verify` CLI on top of the shared `Sign`/`Verify`.

> Naming note: the relay module is `intake`, so the importable path is `intake/license` and the relay-internal path is `intake/internal/license`. Two packages both named `license` is fine — they are never imported into the same file by the same name without an alias; where both are needed, alias the internal one (e.g. `licensemgr`).

### 2.3 Canonicalization contract (locked here)

`Canonicalize(*License) []byte` = `encoding/json.Marshal` of the license value with the `Signature` field **cleared to `""`** (struct field order is deterministic in Go's encoder; nested maps are key-sorted). `Sign` sets `Signature = "ed25519:" + base64(sig over Canonicalize)`. `Verify` strips the `ed25519:` prefix, base64-decodes, re-runs `Canonicalize` on the parsed license (signature cleared), and checks `ed25519.Verify`. A **golden round-trip test** in 3-vii (CLI signs → relay `Verify` accepts; one byte flipped → rejects) locks the two modules together.

## 3. Sub-plan decomposition (`ai/tasks/phase-3/`)

Seven sub-plans. Seam first (3-i), then one adapter each (3-ii…3-v), then license+gate (3-vi), then the CLI (3-vii). The gate is **retrofitted** into the 3-i registry by 3-vi (the same incremental pattern as 2-i → 2-ii wiring a factory case).

| # | Unit | Adds | Sub-plan smoke |
|---|---|---|---|
| 3-i | **Adapter config + registry + router** | chatwoot/fider/zendesk/linear + `routing` config blocks (secrets via `ResolveSecret`); `router` pkg; `Deps.Adapter`→`Deps.Router`; rewire `submit.go`; build the registry in `main.go` (all enabled, **no gate yet**) | Unit: routing_hint/rule/default selection + `default_adapter` fatal-if-unregistered + rule-drops-with-warning, using a fake adapter. No network. Relay still boots & submits via webhook through the router. |
| 3-ii | **chatwoot** (free) | `adapter/chatwoot` — conversation in configured inbox | Unit: `httptest` mocks Chatwoot REST; maps summary/title/body; key absent from logs. Live: deferred to phase final smoke. |
| 3-iii | **fider** (free) | `adapter/fider` — post on configured board | Unit: `httptest` mocks Fider REST. Live: deferred. |
| 3-iv | **zendesk** (paid) | `adapter/zendesk`, `RequiresLicense()=true` — ticket + priority + custom fields | Unit: `httptest`; basic-auth `email/token`. Live: **pauses** (needs Zendesk token). |
| 3-v | **linear** (paid) | `adapter/linear`, `RequiresLicense()=true` — GraphQL `issueCreate` | Unit: `httptest` mocks the GraphQL endpoint. Live: **pauses** (needs Linear token). |
| 3-vi | **license verify + gate** | `relay/license` (struct/canon/sign/verify) + `relay/internal/license` (embedded key, loader, trial/free machine) + gate retrofit into the 3-i registry + `main.go` wiring + license config block | Unit: ephemeral test keypair via **injectable verifier**; trial/free/expired/bad-sig matrix; gate skips unlicensed paid. **Free-mode startup-log behavior is smokeable with NO external dep** (boot with zendesk enabled, no license → paid skipped + clear log). |
| 3-vii | **intake-license CLI** | `license-tool` `keygen` + `sign` (+ `verify`), `replace intake => ../relay`; excluded from release artifacts | Local round-trip: `keygen` → `sign` a test license → relay `Verify` accepts it; tamper one byte → relay rejects. No network, no real key. |

**Dependency graph:** `3-i → {3-ii, 3-iii, 3-iv, 3-v} → 3-vi → 3-vii`. The four adapters depend only on the 3-i registry/config seam and are mutually independent (each adds a distinct package + one config block + one registry entry); they may be built in parallel or serially. 3-vi retrofits the gate into the registry (so it follows the adapters). 3-vii depends on 3-vi's `relay/license` package (shared `Sign`/`Verify`).

## 4. License model (3-vi)

### 4.1 Load order (PROJECT.md §12)

1. CLI flag `--license-file=<path>`
2. env `INTAKE_LICENSE` (base64-encoded license JSON)
3. env `INTAKE_LICENSE_FILE` (path)
4. `/etc/intake/license.json`
5. `os.UserConfigDir()/intake/license.json`

First hit wins. The loader returns "no license found" distinctly from "license found but invalid."

### 4.2 State machine

| Condition | Result |
|---|---|
| License found, valid signature, `expires_at > now` | **Licensed** — paid adapters listed in `license.adapters` are permitted; others skipped |
| License found, **bad signature** | **Fatal** — loud startup error (signals tampering, PROJECT.md §12). *Ruling D1.* |
| License found, signature OK but **expired** | **Downgrade to free + prominent warning** — relay starts, paid adapters skipped (a lapsed paid customer's free adapters keep working). *Ruling D1.* |
| No license + no trial state | **Start 14-day trial** — write `state.json{trial_started_at:now}`, all adapters enabled, log "trial: 14 days remaining" |
| No license + trial state, ≤ 14 days old | **Trial** — all adapters enabled, log remaining days |
| No license + trial expired | **Free** — paid adapters skipped, clear startup log naming the pricing URL |

Trial-state path (decomposition Q3): `os.UserConfigDir()/intake/state.json` → `%AppData%\intake\state.json` (Windows), `~/.config/intake/state.json` (Linux), `~/Library/Application Support/intake/state.json` (macOS). In an ephemeral container the trial restarts each boot; production is expected to carry a license (noted in §6 / Phase 7 docs).

### 4.3 The gate

`internal/license.Manager.Permits(adapterName string) bool` returns true for any free adapter, and for a paid adapter only when the active state permits it (licensed-and-listed, or trial). `main.go` builds the registry by iterating enabled adapters: a free adapter is always registered; a paid adapter is registered only if `Permits` is true, else **skipped with a clear WARN log** (`adapter "zendesk" requires a license — see <pricing-url>`), never fatal. This is the §12 "refuse to enable" behavior expressed as registry omission.

### 4.4 Routing-vs-gate interaction (Ruling D2)

After the gate decides the registry, the router is constructed with validation:
- `routing.default_adapter` **must** be a registered adapter, else **fatal** at startup (a relay with a broken default is useless).
- A routing **rule** whose `to` names a non-registered adapter (e.g. free-mode but a rule targets `zendesk`) is **dropped with a warning**; submissions matching it fall through to the default. So free-mode keeps working even when the config mentions paid adapters.
- A request `routing_hint` naming a non-registered adapter is ignored (falls through to rules → default).

### 4.5 Multi-tenancy hook (PROJECT.md §16)

The `License` struct carries `tier`; a `"hosted"` tier marker is recognized by `Permits` as granting all adapters (the hosted relay uses one master license; per-tenant entitlement is enforced elsewhere). v0 self-hosted licenses use `"pro"`/`"team"`. No tenant logic in the relay beyond honoring this marker.

## 5. Per-adapter implementation notes

All four are stdlib `net/http` + `encoding/json` (mirroring `webhook.go`), each with a test-injectable `*http.Client` (or base URL) so mock unit tests hit an `httptest.Server` and run credit-free. Tokens resolve in `main.go` via `config.ResolveSecret(<adapter>.api_token_env)` and are passed into `Configure`; they are never placed in YAML or logs. Canonical-payload → downstream mapping (text only; attachments in P6):

### 5.1 chatwoot (`relay/internal/adapter/chatwoot/`) — free
- `POST {base_url}/api/v1/accounts/{account_id}/conversations` (or inbox-scoped contact+conversation create), `api_access_token` header.
- Map: `conversation.title_suggestion` → conversation subject/first line; `conversation.summary` + transcript → message body; `tags_suggested` → labels.
- Config: `base_url`, `account_id`, `inbox_id`, `api_token_env`. `Name()` → `"chatwoot"`, `RequiresLicense()` → false.

### 5.2 fider (`relay/internal/adapter/fider/`) — free
- `POST {base_url}/api/v1/posts` with `{ title, description }`, `Authorization: Bearer <api_key>`.
- Map: `title_suggestion` → `title`; `summary` + transcript → `description`.
- Config: `base_url`, `api_key_env`. `Name()` → `"fider"`, `RequiresLicense()` → false.

### 5.3 zendesk (`relay/internal/adapter/zendesk/`) — **paid**
- `POST https://{subdomain}.zendesk.com/api/v2/tickets.json` with `{ ticket: { subject, comment:{ body }, priority, tags } }`; HTTP basic auth `"{email}/token:{api_token}"`.
- Map: `title_suggestion` → `subject`; `summary` + transcript → `comment.body`; `severity_guess` → `priority` (low/normal/high/urgent); `tags_suggested` → `tags`.
- Config: `subdomain`, `email`, `api_token_env`, `default_priority`. `Name()` → `"zendesk"`, `RequiresLicense()` → **true**.

### 5.4 linear (`relay/internal/adapter/linear/`) — **paid**
- Single `POST https://api.linear.app/graphql` with the `issueCreate` mutation (`{ teamId, title, description }`), `Authorization: <api_key>` header. Minimal hand-rolled GraphQL request/response structs — no SDK.
- Map: `title_suggestion` → `title`; `summary` + transcript → `description` (markdown); `team_id` from config; optional label/priority mapping deferred.
- Config: `api_key_env`, `team_id`. `Name()` → `"linear"`, `RequiresLicense()` → **true**.

## 6. Configuration (additive)

```yaml
license:
  file: ""                       # optional explicit path; load order in §4.1 still applies

routing:
  default_adapter: "chatwoot"    # MUST be a registered adapter, else fatal (§4.4)
  rules:
    - when: { classification: "bug" }
      to: "chatwoot"
    - when: { classification: "feature_request" }
      to: "fider"

adapters:
  chatwoot: { enabled: true,  base_url: "...", account_id: 1, inbox_id: 3, api_token_env: "CHATWOOT_TOKEN" }
  fider:    { enabled: true,  base_url: "...", api_key_env: "FIDER_API_KEY" }
  zendesk:  { enabled: false, subdomain: "example", email: "agent@example.com", api_token_env: "ZENDESK_API_TOKEN", default_priority: "normal" }
  linear:   { enabled: false, api_key_env: "LINEAR_API_KEY", team_id: "TEAM_ID" }
  webhook:  { enabled: false, url: "...", headers: {...}, retry: {...} }   # unchanged from Phase 1
```

- Adapter tokens (`CHATWOOT_TOKEN`, `FIDER_API_KEY`, `ZENDESK_API_TOKEN`, `LINEAR_API_KEY`) resolve via `config.ResolveSecret` (env-or-`_FILE`); never in YAML.
- `applyDefaults` sets `routing.default_adapter` only if a sensible default exists among enabled adapters (otherwise it stays empty and §4.4 validation reports it). Config-struct shapes (the `AdaptersConfig` + `RoutingConfig` + `LicenseConfig` fields) are **frozen in 3-i**, §8.

## 7. Testing

- **Credit-free unit tests per adapter** (`go test ./...`, no real tokens): `httptest.Server` returning a canned success body; assert the request method/path/auth header/JSON body mapping; a non-2xx path; a token-redaction assertion (token never in error string or log).
- **Router unit test:** routing_hint hit; rule match; default fallback; `default_adapter` unregistered → `New` errors (fatal at startup); rule targeting unregistered adapter → dropped + warning, falls through.
- **License unit tests (3-vi):** generate an **ephemeral test keypair** in-test; `Verify` accepts a freshly-signed license, rejects a tampered one, rejects an expired one; loader load-order precedence; trial→free transition (inject a clock or a `now` arg); gate `Permits` matrix. The verifier takes an **injectable public key** so production uses the embedded constant and tests use the ephemeral key.
- **CLI round-trip (3-vii):** `keygen` → `sign` a license with the generated private key → relay `Verify` (with the matching public key) accepts; flip a byte → rejects.
- **Live smoke (phase final, pauses for the maintainer):** see §10.

## 8. Frozen shared contracts (single source of truth, locked in the noted sub-plan)

- **Config structs** (`AdaptersConfig.{Chatwoot,Fider,Zendesk,Linear}`, `RoutingConfig{DefaultAdapter, Rules}`, `LicenseConfig{File}`) — frozen in **3-i** (additive to `config.Config`; do not restructure the top-level shape, per 1-i).
- **`router.Router` + `router.Rule`** signatures — frozen in **3-i**.
- **`adapter.Adapter` interface** — UNCHANGED (Phase 1 §3.1 freeze); every adapter implements it exactly.
- **`license.License` struct + `Canonicalize`/`Sign`/`Verify`** in `relay/license` — frozen in **3-vi**; the CLI (3-vii) consumes them unchanged.
- **Embedded public-key constant** lives in `relay/internal/license`; filled by the maintainer keygen pause (§10), not invented by an implementer.

## 9. Dependencies / pins

**None added.** All adapters use stdlib `net/http` + `encoding/json`; license uses stdlib `crypto/ed25519`; the CLI uses stdlib + `intake/license` via a `replace` directive. `scripts/check-pins.sh` and `scripts/verify-contract.sh` are unchanged and must stay green. The only `go.mod` change is the `replace intake => ../relay` line in `license-tool/go.mod`.

## 10. Final smoke (phase README) — PAUSES for the maintainer

Credit-free unit tests back everything above. The live smoke needs real downstream targets and a maintainer-signed license; **pause and hand off to the maintainer** for these:

```
1. Maintainer keygen pause (one-time): run `intake-license keygen`; store the private
   key offline; commit the generated public key as the embedded constant in
   relay/internal/license; rebuild relay.
2. Maintainer sign pause: `intake-license sign` a short-lived license granting
   {zendesk, linear} → license.json.
3. Live Chatwoot: point adapters.chatwoot at a running Chatwoot instance; submit a
   conversation through the widget/driver → a conversation appears in the inbox with
   the mapped subject/summary.
4. Gate — blocked: enable zendesk with NO license → relay boots in free mode, logs
   `adapter "zendesk" requires a license …`, zendesk is absent from the registry; a
   submission routed to it falls through to default (rule dropped with warning).
5. Gate — permitted: provide the signed license.json (load order) → relay logs the
   licensed tier, zendesk/linear register; (optionally, with real tokens) a ticket is
   created in Zendesk/Linear.
6. Free-mode log (no external dep): boot with zendesk enabled + no license/expired
   trial → observe the clear "free mode, paid adapters disabled" startup log.
Teardown: re-runnable; delete state.json to reset the trial.
```

Per the credit/secret guard, steps 1–2 (keygen/sign), 3 (live Chatwoot), and any Zendesk/Linear token use are **maintainer/paid/external** and pause for explicit go-ahead; step 6 and all unit layers are credit-free and self-runnable.

## 11. ADRs locked by this phase (for the phase README)

- **Router is a distinct `internal/router` package; `Deps.Adapter` (single) becomes `Deps.Router`** (§2.1). The `adapter.Adapter` interface and chi route shape are unchanged (§3.1 freeze honored). Trigger to revisit: multi-adapter dispatch (one ticket to many systems — v1) needs the router to fan out.
- **License sign/verify lives in an importable `relay/license` package; the CLI consumes it via `replace intake => ../relay`** (§2.2) — single canonicalization source despite the two-module split. Trigger to revisit: the CLI is ever published (it is maintainer-only per Q10), or a third consumer appears (promote to a standalone module).
- **Bad signature = fatal; expired = downgrade-to-free + warn** (§4.2, ruling D1). Trigger to revisit: customer feedback that a hard fail on tamper is too strict, or that expired-downgrade masks renewal lapses.
- **default_adapter fatal-if-unregistered; rules drop-with-warning** (§4.4, ruling D2). Trigger to revisit: operators want strict config validation (promote rule-dangling to fatal via a flag).
- **All adapters hand-rolled over stdlib `net/http`; no vendor SDKs** (§5, §9) — keeps Phase 3 dependency-free and consistent with the ollama decision. Trigger to revisit: a downstream API's auth/pagination becomes too costly to hand-maintain.

## 12. Non-goals (Phase 3)

Attachment forwarding (P6); multi-adapter dispatch / fan-out (v1); license revocation / CRL (v1); online activation or phone-home (never — §12); publishing the `intake-license` CLI (maintainer-only, Q10); per-tenant adapter config overrides beyond honoring the `hosted` tier marker (hosted-relay project, §16); adapter-specific features beyond ticket/issue/post creation with basic field mapping (custom-field editors, status sync, comments — post-v0).
