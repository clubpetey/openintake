# Phase 3 — Adapters + License gate

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement the sub-plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Adds the four remaining v0 adapters (`chatwoot`, `fider` free; `zendesk`, `linear` paid) behind the **frozen** `adapter.Adapter` interface; a **router** (`routing_hint` → rules → default); **Ed25519 license verification** (embedded public key, signed-JSON license, load order, 14-day trial, free-mode fallback); the **license gate** on the two paid adapters; and the maintainer-only **`intake-license` CLI**. The `webhook` adapter (`relay/internal/adapter/webhook/`) is the reference implementation every new adapter mirrors.

## 1. Spec link

- Phase 3 design: [docs/specs/2026-05-27-phase-3-adapters-and-license-design.md](../../../docs/specs/2026-05-27-phase-3-adapters-and-license-design.md)
- Parent decomposition: [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](../../../docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 3 row, §3.1 frozen seams, §4 Q3/Q10)
- Secrets seam: [docs/specs/2026-05-27-configuration-and-secrets-design.md](../../../docs/specs/2026-05-27-configuration-and-secrets-design.md) (`config.ResolveSecret`)
- Source of truth for scope/contracts: [docs/PROJECT.md](../../../docs/PROJECT.md) §8, §12, §13, §16

## 2. Architectural Decision Record (ADR) summary

- **Router is a distinct `internal/router` package; `server.Deps.Adapter` (single) becomes `Deps.Router`.** The `adapter.Adapter` interface, the chi route-registration shape (`registerIntakeRoutes`), and the middleware chain are UNCHANGED (Phase 1 §3.1 freeze honored). Revisit trigger: multi-adapter dispatch (one ticket → many systems, v1) needs the router to fan out.
- **License sign/verify lives in an importable `relay/license` package (NOT under `internal/`); the CLI consumes it via `replace intake => ../relay`.** Single canonicalization source despite the two-module split (`intake` relay vs `intake-license-tool`). Revisit trigger: the CLI is ever published (it is maintainer-only per Q10), or a third consumer appears (promote to a standalone module).
- **Bad signature = fatal; expired = downgrade-to-free + warn** (design §4.2). A tampered signature stops startup loudly; a merely-lapsed license lets free adapters keep working. Revisit trigger: customer feedback that hard-fail-on-tamper is too strict, or that expired-downgrade masks renewal lapses.
- **`routing.default_adapter` fatal-if-unregistered; rules drop-with-warning** (design §4.4). A broken default is useless (fatal); a rule pointing at an unlicensed/disabled adapter degrades gracefully (dropped, falls through to default). Revisit trigger: operators want strict config validation (add a flag to promote rule-dangling to fatal).
- **All adapters hand-rolled over stdlib `net/http`; no vendor SDKs** (design §5, §9). Keeps Phase 3 dependency-free, consistent with the Phase-2 ollama decision. Revisit trigger: a downstream API's auth/pagination becomes too costly to hand-maintain.

This phase does NOT add: attachment forwarding (Phase 6), multi-adapter dispatch/fan-out (v1), license revocation/CRL (v1), online activation/phone-home (never), publishing the CLI (maintainer-only), or per-tenant adapter config beyond honoring the `hosted` tier marker (hosted-relay project).

## 3. Sub-plan index

| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 3-i | [Adapter config + registry + router](3-i-adapter-config-registry-router-plan.md) | routing seam | M | Not started |
| 3-ii | [Chatwoot adapter](3-ii-chatwoot-adapter-plan.md) | free / Chatwoot REST | M | Not started |
| 3-iii | [Fider adapter](3-iii-fider-adapter-plan.md) | free / Fider REST | S | Not started |
| 3-iv | [Zendesk adapter](3-iv-zendesk-adapter-plan.md) | paid / Zendesk REST | M | Not started |
| 3-v | [Linear adapter](3-v-linear-adapter-plan.md) | paid / Linear GraphQL | M | Not started |
| 3-vi | [License verify + gate](3-vi-license-verify-gate-plan.md) | Ed25519 + trial/free + gate | L | Not started |
| 3-vii | [intake-license CLI](3-vii-intake-license-cli-plan.md) | maintainer tool | M | Not started |

## 4. Dependency graph

```
3-i (config + registry + router)
      │
      ├──► 3-ii  (chatwoot)   ┐
      ├──► 3-iii (fider)      │  mutually independent;
      ├──► 3-iv  (zendesk)    │  parallelizable after 3-i
      └──► 3-v   (linear)     ┘
                  │
                  ▼
            3-vi (license verify + gate retrofit into the 3-i registry)
                  │
                  ▼
            3-vii (intake-license CLI — consumes relay/license from 3-vi)
```

3-i establishes the config sub-structs + `router` package + the registry build in `main.go` (ungated). The four adapter sub-plans each add one package + one config block + one registry entry and depend only on 3-i. 3-vi retrofits the license gate into the registry (so it follows the adapters, which give it paid adapters to gate). 3-vii depends on the `relay/license` package created in 3-vi.

## 5. Tool version pin list

**No external tools or libraries are introduced in Phase 3.** All adapters use the Go standard library (`net/http`, `encoding/json`); the license uses `crypto/ed25519` (stdlib). The only `go.mod`-level change is a **local `replace` directive** (no version):

| Tool | Version | Reason |
|---|---|---|
| (chatwoot/fider/zendesk/linear) | — (stdlib `net/http`) | mirror the webhook adapter; no SDK, nothing to pin |
| (license / CLI crypto) | — (stdlib `crypto/ed25519`) | Go stdlib; no external dependency |
| `replace intake => ../relay` (in `license-tool/go.mod`) | — (local path) | shares the `relay/license` canonicalization package with the CLI; not a versioned dependency |

`scripts/check-pins.sh` and `scripts/verify-contract.sh` are unchanged and MUST stay green. `go mod tidy` in `relay/` must add nothing (any new external dep is a red flag — investigate).

## 6. Build-fail checklist

- [ ] `go build ./...` / `go vet ./...` fails in `relay/` **or** `license-tool/`. **Fail.**
- [ ] Any Go test fails (`go test ./...` in both modules). **Fail.**
- [ ] The Phase-0 contract gate regresses (`scripts/verify-contract.sh`). **Fail.**
- [ ] An adapter token, license private key, or any secret appears in a log line, error string, or response body. **Fail.**
- [ ] `relay/go.mod` gains any **external** dependency (a non-stdlib import), or `go mod tidy` is dirty. **Fail** (Phase 3 is stdlib-only).
- [ ] `router.New` returns a non-nil router for an unregistered `default_adapter`, instead of erroring (→ would let `main.go` start with a broken default). **Fail.**
- [ ] `router.Route` returns a nil adapter with a nil error. **Fail** (must always resolve or error).
- [ ] `license.Verify` accepts a tampered or wrong-key signature. **Fail** (the gate's whole point).
- [ ] A paid adapter is registered in free/expired-trial mode (without a permitting license). **Fail** (gate bypass).
- [ ] The `intake-license` binary is wired into any release artifact (goreleaser/npm). **Fail** (maintainer-only, Q10) — note: release pipeline is Phase 7; this item guards against accidentally adding it.

## 7. Final smoke (mandatory)

Proves the Phase 3 deliverable end-to-end. The unit layer (mock HTTP via `httptest`, mock license verification with a test keypair) is fully **credit-free** and runs in `go test ./...`. The **live** smoke needs real downstream targets and a maintainer-signed license, and **pauses for the maintainer** (paid/external/secret per the credit-secret guard).

```
1. Maintainer keygen pause (one-time): run `intake-license keygen`; store the private
   key offline; commit the printed public key as the embedded constant in
   relay/internal/license/embedded_key.go; rebuild the relay.
2. Maintainer sign pause: `intake-license sign --in test-license.json --key <privkey>`
   producing a signed license granting adapters [zendesk, linear], short expiry.
3. Live Chatwoot (free): point adapters.chatwoot at a running Chatwoot instance with a
   real CHATWOOT_TOKEN; drive a conversation via core/smoke/drive.ts → Submit → a
   conversation appears in the configured inbox with the mapped subject/summary/transcript.
4. Gate — blocked: enable adapters.zendesk, provide NO license, delete trial state →
   relay boots in FREE mode, logs `adapter "zendesk" requires a license — see <url>`,
   zendesk is ABSENT from the registry; a rule routing bug→zendesk is dropped with a
   warning and the submission falls through to the default (chatwoot).
5. Gate — permitted: place the signed license (load order) → relay logs the licensed
   tier and remaining days; zendesk + linear register. (Optionally, with real tokens, a
   ticket/issue is created in Zendesk/Linear.)
6. Free-mode log (NO external dep): boot with zendesk enabled + no license + expired/no
   trial → observe the clear "free mode — paid adapters disabled" startup log and the
   per-adapter "requires a license" line. This step alone is self-runnable.
7. Teardown: delete os.UserConfigDir()/intake/state.json to reset the 14-day trial;
   re-runnable.
```

A phase is NOT done until this smoke passes from a clean state. Steps 1–2 (keygen/sign), 3 (live Chatwoot), and any Zendesk/Linear token use are maintainer/paid/external and require explicit go-ahead.

## 8. Shared Contracts (SINGLE SOURCE OF TRUTH)

These shapes are **frozen** in the noted sub-plan; later sub-plans consume them unchanged.

### 8.1 The frozen adapter interface (UNCHANGED — `relay/internal/adapter/adapter.go`)

Every adapter implements `adapter.Adapter` exactly as frozen in Phase 1:

```go
type Adapter interface {
	Name() string
	RequiresLicense() bool
	Configure(config map[string]any) error
	Create(ctx context.Context, p *payload.IntakePayload) (*CreateResult, error)
	HealthCheck(ctx context.Context) error
}
type CreateResult struct {
	ExternalID  string
	ExternalURL string
	AdapterName string
	CreatedAt   string // ISO-8601 / RFC3339
}
```

Behavioral contract each impl MUST honor (mirror `webhook.go`):
- `New()` returns an unconfigured `*Adapter`; `Configure` reads the per-adapter config map (keys match the config sub-struct yaml tags) and validates required keys, returning a clear error naming the missing key.
- A test-injectable HTTP client (an unexported `client *http.Client` field, defaulted in `New()`, overridable in tests via a `WithHTTPClient`-style helper or an exported field setter) so mock unit tests hit an `httptest.Server` and run **credit-free**.
- `Create` marshals the relevant fields, POSTs over `net/http` with a `ctx`-bound request, treats 2xx as success and non-2xx as an error that **includes the response body (truncated) but never the token**; returns a populated `CreateResult` (`AdapterName` = `Name()`, `CreatedAt` = `time.Now().UTC().Format(time.RFC3339)`).
- `RequiresLicense()` → false for chatwoot/fider; **true** for zendesk/linear.
- The downstream **secret** (api token / key) is passed INTO `Configure` (resolved by `main.go` via `config.ResolveSecret`); the adapter never reads the env itself and never logs the token.

### 8.2 Canonical payload fields available to adapters (generated — `relay/internal/payload/types.go`, DO NOT EDIT)

```go
p.Conversation.Summary          // string
p.Conversation.TitleSuggestion  // string (≤80 chars, schema-enforced)
p.Conversation.Classification   // ConversationClassification ("bug"|"feature_request"|"question"|"other")
p.Conversation.SeverityGuess    // ConversationSeverityGuess ("low"|"medium"|"high"|"critical"|"unknown")
p.Conversation.TagsSuggested    // []string
p.Conversation.Messages         // []Message{ Role MessageRole, Content string, Ts time.Time }
p.User.Email                    // *string  (may be nil)
p.User.DisplayName              // *string  (may be nil)
p.RoutingHint                   // *string  (may be nil)
```

Helper each adapter needs: render the transcript. Recommended shared approach — a small unexported `renderBody(p)` per adapter that concatenates `Summary`, a blank line, then each message as `"<Role>: <Content>"`. (Kept per-adapter to avoid premature shared-package coupling; the logic is ~8 lines.)

### 8.3 Config sub-structs (additive — `relay/internal/config/config.go`, FROZEN in 3-i)

```go
// Config gains two top-level fields (additive; do not restructure):
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	LLM      LLMConfig      `yaml:"llm"`
	Auth     AuthConfig     `yaml:"auth"`
	Adapters AdaptersConfig `yaml:"adapters"`
	Routing  RoutingConfig  `yaml:"routing"`   // 3-i
	License  LicenseConfig  `yaml:"license"`   // 3-i
}

// AdaptersConfig gains four adapter blocks (Webhook unchanged from Phase 1):
type AdaptersConfig struct {
	Webhook  WebhookConfig  `yaml:"webhook"`
	Chatwoot ChatwootConfig `yaml:"chatwoot"`
	Fider    FiderConfig    `yaml:"fider"`
	Zendesk  ZendeskConfig  `yaml:"zendesk"`
	Linear   LinearConfig   `yaml:"linear"`
}

type ChatwootConfig struct {
	Enabled     bool   `yaml:"enabled"`
	BaseURL     string `yaml:"base_url"`
	AccountID   int    `yaml:"account_id"`
	InboxID     int    `yaml:"inbox_id"`
	APITokenEnv string `yaml:"api_token_env"`
}
type FiderConfig struct {
	Enabled   bool   `yaml:"enabled"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
}
type ZendeskConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Subdomain       string `yaml:"subdomain"`
	Email           string `yaml:"email"`
	APITokenEnv     string `yaml:"api_token_env"`
	DefaultPriority string `yaml:"default_priority"`
}
type LinearConfig struct {
	Enabled   bool   `yaml:"enabled"`
	APIKeyEnv string `yaml:"api_key_env"`
	TeamID    string `yaml:"team_id"`
}

// Routing: default_adapter + ordered rules.
type RoutingConfig struct {
	DefaultAdapter string `yaml:"default_adapter"`
	Rules          []Rule `yaml:"rules"`
}
type Rule struct {
	When RuleMatch `yaml:"when"`
	To   string    `yaml:"to"`
}
// RuleMatch matches on classification and/or severity. Each field accepts a YAML
// scalar ("bug") OR a sequence (["question","other"]) via StringList. An empty
// field is a wildcard (matches any).
type RuleMatch struct {
	Classification StringList `yaml:"classification"`
	Severity       StringList `yaml:"severity"`
}

// LicenseConfig: optional explicit path (load order in design §4.1 still applies).
type LicenseConfig struct {
	File string `yaml:"file"`
}

// StringList unmarshals a YAML scalar OR sequence into []string.
type StringList []string
```

Defaults (`applyDefaults`, 3-i): `routing.default_adapter` defaults to `"chatwoot"` only when empty (validation in 3-i's router catches a default that names no registered adapter). The adapter blocks have no enable-by-default (all `enabled:false` unless set). The webhook retry defaults stay as Phase 1.

### 8.4 The router (FROZEN in 3-i — `relay/internal/router/router.go`, package `router`)

```go
package router

// Rule is the resolved form of config.Rule (classification/severity → adapter name).
type Rule struct {
	Classification []string // empty = any
	Severity       []string // empty = any
	To             string
}

// New builds a Router. It validates that defaultName names a registered adapter
// (else returns an error → fatal at startup). Rules whose To names an
// unregistered adapter are DROPPED with a warning on logger (graceful free-mode).
func New(registry map[string]adapter.Adapter, rules []Rule, defaultName string, logger *slog.Logger) (*Router, error)

// Route resolves a payload to exactly one registered adapter:
//   1. p.RoutingHint (if non-nil/non-empty AND names a registered adapter)
//   2. first rule whose (non-empty) Classification AND Severity both match
//   3. the default adapter
// Always returns a non-nil adapter or a non-nil error; never (nil, nil).
func (r *Router) Route(p *payload.IntakePayload) (adapter.Adapter, error)
```

`server.Deps.Adapter adapter.Adapter` is **replaced** by `Deps.Router *router.Router`; `submit.go` calls `deps.Router.Route(p)` then `.Create`. `main.go` builds the registry from enabled+permitted adapters and passes resolved `router.Rule`s.

### 8.5 The license packages (FROZEN in 3-vi)

```go
// relay/license — IMPORTABLE (not internal); shared with the CLI via replace.
package license

type License struct {
	LicenseID   string    `json:"license_id"`
	IssuedTo    IssuedTo  `json:"issued_to"`
	Tier        string    `json:"tier"`               // "pro" | "team" | "hosted"
	Adapters    []string  `json:"adapters"`           // permitted paid adapter names
	MaxInstances int      `json:"max_relay_instances"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Signature   string    `json:"signature"`          // "ed25519:" + base64(sig)
}
type IssuedTo struct {
	Org   string `json:"org"`
	Email string `json:"email"`
}

// Canonicalize returns the bytes that are signed/verified: JSON of the license
// with Signature cleared to "". Deterministic (Go struct field order; sorted map keys).
func Canonicalize(l License) ([]byte, error)

// Sign sets l.Signature = "ed25519:" + base64(Sign(priv, Canonicalize(l))). (CLI side.)
func Sign(priv ed25519.PrivateKey, l *License) error

// Verify parses blob, strips the "ed25519:" prefix, re-canonicalizes, and checks
// the signature against pub. Returns the parsed License on success.
func Verify(pub ed25519.PublicKey, blob []byte) (*License, error)
```

```go
// relay/internal/license — relay-only: embedded key + loader + trial/free + gate.
package license // import alias `licensemgr` where relay/license is also imported

// EmbeddedPublicKey is the maintainer's Ed25519 public key (filled by the keygen pause).
// State is the resolved entitlement after applying load order + trial/free rules.
type State struct {
	Mode      string   // "licensed" | "trial" | "free"
	Adapters  []string // permitted paid adapters ("" tier "hosted" => all)
	ExpiresAt time.Time
	Message   string   // human-readable startup log line
}

// Load runs the load order (design §4.1), verifies with EmbeddedPublicKey, applies
// the trial/free state machine (design §4.2) using statePath for trial state and
// now for the clock (injectable for tests). Bad signature => error (fatal). Expired
// or absent => trial/free State (not an error).
func Load(cfg config.LicenseConfig, statePath string, now time.Time) (*State, error)

// Permits reports whether a paid adapter name is allowed under this State.
func (s *State) Permits(adapterName string) bool
```

## 9. Notes

- **Module path** remains `intake` (placeholder name); `license-tool` module is `intake-license-tool`.
- **Attachments** are deferred to Phase 6 — adapters create text tickets only.
- **Two packages named `license`**: the importable one is `intake/license`; the relay-only one is `intake/internal/license`. In `main.go` (the only place both could meet), import the internal one with alias `licensemgr`.
- **Credit/secret guard:** all unit tests mock downstream HTTP (`httptest`) and license verification (ephemeral test keypair). Live smokes pause for the maintainer (real Chatwoot/Zendesk/Linear targets + a maintainer-signed license).
- **LESSONS:** L003 (re-validate `const` fields at the boundary — already handled by 1-iv's builder; adapters consume the already-validated payload). L004 (Node smoke browser-global stubs) applies if any Node driver is used in the live smoke.
