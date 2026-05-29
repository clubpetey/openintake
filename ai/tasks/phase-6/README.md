# Phase 6 — Attachments

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement the sub-plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Ships screenshot attachments end-to-end behind the **frozen** Phase 1+3+4+5 seams: widget html2canvas capture → rectangle redaction modal → base64-encoded PNG/JPEG/WebP into the existing `attachments[]` field of `IntakePayload` → POSTed inline to `/v1/intake/submit` → relay magic-byte validates + per-attachment cap + aggregate cap → router dispatches to one adapter → that adapter forwards via its native upload mechanism (chatwoot inline, fider markdown inline, linear asset-upload-before-issueCreate, zendesk uploads-then-attach, webhook pass-through). No schema change (the `attachments[]` field has been in `schema/payload.v1.json` since Phase 0), no codegen rerun, no change to the `adapter.Adapter` interface. One new external browser dep (`html2canvas` pinned exact at `1.4.1`); zero new Go modules.

## 1. Spec link

- Phase 6 design: [docs/specs/2026-05-28-phase-6-attachments-design.md](../../../docs/specs/2026-05-28-phase-6-attachments-design.md)
- Parent decomposition: [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](../../../docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md) (Phase 6 row + P3 → P6 dependency edge)
- Source of truth for scope/contracts: [docs/PROJECT.md](../../../docs/PROJECT.md) §11 (attachment handling), §6 (payload schema — `attachments[]` already in v1), §9 (`attachments:` config block), §17 (security — magic-byte rule)
- Secrets seam: [docs/specs/2026-05-27-configuration-and-secrets-design.md](../../../docs/specs/2026-05-27-configuration-and-secrets-design.md) (no new secrets in Phase 6; reuses adapter secret-resolution paths)
- Phase 4+5 patterns mirrored: [docs/specs/2026-05-28-phase-5-abuse-and-spend-control-design.md](../../../docs/specs/2026-05-28-phase-5-abuse-and-spend-control-design.md) (middleware composition + `Deps` shape + Q9-consolidated startup gate + L016 return-parsed-values discipline)
- Phase-1+3+4+5 frozen seams (unchanged here): `relay/internal/adapter/adapter.go` (Adapter interface), `relay/internal/payload/types.go` (generated; never edited), `relay/internal/auth/middleware.go` (Handler signature + SessionContext), `relay/internal/server/server.go` (chi route shape)

## 2. Architectural Decision Record (ADR) summary

- **Inline transport in `/submit` body per PROJECT.md §11.** Per-attachment cap 5 MB, aggregate cap 10 MB, base64-encoded (~1.37× raw overhead). `MaxBytesReader` on `/v1/intake/submit` grows from `1<<20` (1 MB) to `(1<<20)*14` (14 MB) ONLY when `cfg.Attachments.Enabled=true`; remains 1 MB otherwise (avoid widening attack surface for operators who disabled attachments). Revisit trigger: any single attachment must exceed 5 MB, OR multi-image submissions push aggregate past 10 MB → introduce `/v1/intake/upload` streaming endpoint with pre-signed S3 URLs (v1+).

- **No interface change to `adapter.Adapter`.** Each adapter handles `p.Attachments` internally inside its existing `Create()`. The frozen interface stays exactly unchanged. Per-adapter native sequencing varies (Linear must asset-upload BEFORE issueCreate; Chatwoot inlines DURING conversation create) and the interface should not pretend otherwise. A new optional `CapableAdapter` interface advertises supported MIME types for capability discovery — metadata, not behavior, purely additive. Revisit trigger: an adapter is added that requires post-create attachment uploads that can fail independently AND the maintainer wants centralised orphan-ticket handling → add an optional `Attacher` post-hoc interface.

- **Per-adapter accepted-MIME-types published via `/init` Capabilities.** `InitResponse.Capabilities` gains an optional `attachments` block (`max_size_bytes`, `max_total_bytes`, `allowed_mime_types`). Computed as the **union** across enabled adapters' `Capabilities().AcceptedMIMETypes`, **intersected** with `cfg.Attachments.AllowedMIMETypes`. If empty (or `cfg.Attachments.Enabled=false`), the block is `nil` and the widget hides the Attach button entirely. Revisit trigger: routing rules become MIME-aware (different adapters per attachment type) → compute capabilities per routing rule.

- **Magic-byte validator uses `net/http.DetectContentType` (stdlib).** Single stdlib call on the first 512 bytes; rejects when detected MIME differs from declared `mime_type`. Zero new Go modules. Tradeoff: stdlib `DetectContentType` does not sniff HEIC/AVIF — explicitly out of v0 scope. Revisit trigger: stdlib misclassifies a v0 format OR we add formats stdlib doesn't sniff → switch to a curated signature table.

- **Widget redaction is a dedicated modal component.** `ScreenshotRedactor.vue` is a full-screen modal launched on Attach-click. Capture runs immediately; modal hosts a `<canvas>` with mouse-drawn solid-fill rectangles + Clear All + Save + Cancel. Tested via `@vue/test-utils` mount-in-isolation with a stubbed `html2canvas` (via DI through `core/src/capture.ts setHtml2Canvas(fn)`) and a stubbed canvas-2d context. Revisit trigger: the v1 React widget reuses redaction logic → extract a framework-agnostic redaction primitive into `@intake/core`.

This phase does NOT add: persistent attachment storage (v1+ S3), console/network log capture (v1+), separate upload endpoint (v1+), per-tenant attachment policy (hosted-relay project), video/screen recording (post-v1), forced "no-PII" acknowledgment dialog (Q8 already resolved — opt-in via `require_redaction_ack` deferred to v1+), content-aware/AI-driven redaction (post-v1), HEIC/AVIF support (v1+), file-type attachments (`type:"file"` — schema permits, attachvalidate rejects in v0).

## 3. Sub-plan index

| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 6-i | [Config + InitResponse caps + attachvalidate + body-cap + Q9 gate + adapter Capabilities()](6-i-config-attachvalidate-seam-plan.md) | the seam | M | Not started |
| 6-ii | [Per-adapter native forwarding (chatwoot/fider/linear/zendesk/webhook)](6-ii-adapters-plan.md) | adapter implementations | M-L | Not started |
| 6-iii | [Widget capture + redaction modal + attachment strip + DTO wiring](6-iii-widget-capture-redact-plan.md) | widget UX | M | Not started |
| 6-iv | [Final live chatwoot smoke + Phase 1/4/5 regressions + docs + LESSONS](6-iv-smoke-docs-plan.md) | live evidence | S | Not started |

## 4. Dependency graph

```
6-i (config + Capabilities + attachvalidate + body-cap +
     payloadbuild surface + adapter Capabilities() helper +
     Q9-consolidated startup gate)
      │
      ├──► 6-ii  (chatwoot + fider + linear + zendesk + webhook native upload paths)   ┐
      │                                                                                 │  mutually independent;
      └──► 6-iii (widget capture + redaction modal + attachment strip + DTO wiring)     ┘  parallelizable after 6-i
                   │
                   ▼
            6-iv (live chatwoot smoke + Phase 1/4/5 regressions + docs + LESSONS)
```

6-i locks the wire contract: `Capabilities.Attachments` shape, `SubmitRequest.Attachments` shape, `attachvalidate` sentinel errors, `adapter.CapableAdapter` optional interface. 6-ii populates each adapter's `Create()` with native sequencing. 6-iii builds the widget against the (now-frozen) wire contract. 6-iv records the live evidence + LESSONS. 6-ii and 6-iii touch fully disjoint files (Go adapter code vs. TS/Vue widget code) and are parallelizable after 6-i.

## 5. Tool version pin list

Phase 6 introduces **one** new external module — `html2canvas` for the widget. Zero new Go modules. `go mod tidy` after Phase 6 must remain a no-op.

| Tool | Version | Reason |
|---|---|---|
| `html2canvas` | exact `1.4.1` (latest stable) | Load-bearing widget UX (capture step). Caret forbidden — a silent breaking change in capture semantics is a UX regression not caught by CI type-checks. Peer-dep set empty (no React/Vue requirement); installs cleanly into `core/package.json`. |
| (none new Go) | — | Validation uses stdlib `net/http.DetectContentType` + `encoding/base64`. Phase 5's `golang.org/x/time` pin unchanged. |

`scripts/check-pins.sh` extended with one new check that `html2canvas` has no caret in `core/package.json` — same style as the existing `golang-jwt`/`keyfunc/v3`/`golang.org/x/time` checks.

## 6. Build-fail checklist

- [ ] `go build ./... && go vet ./...` fails in `relay/`. **Fail.**
- [ ] `go test -race ./...` fails. **Fail.**
- [ ] `npm run type-check && npm run build` fails in `core/` or `vue/`. **Fail.**
- [ ] `scripts/verify-contract.sh` regresses. **Fail** (the schema hash + codegen — Phase 6 makes zero schema changes).
- [ ] `scripts/check-pins.sh` regresses. **Fail** (must extend it for `html2canvas`).
- [ ] `go mod tidy` produces any change to `relay/go.mod` or `relay/go.sum`. **Fail** (Phase 6 introduces zero Go modules).
- [ ] `cfg.Attachments.Storage.Mode` set to anything other than `""` or `"forward"` → relay starts. **Fail** (consolidated Q9 gate).
- [ ] `cfg.Attachments.MaxSizeBytes > cfg.Attachments.MaxTotalBytes` → relay starts. **Fail** (same gate).
- [ ] `html2canvas` pinned with a caret in `core/package.json`. **Fail** (check-pins gate).
- [ ] Request body up to 14 MB with `cfg.Attachments.Enabled=true` returns anything other than 200/4xx (no transport-layer failure). **Fail.**
- [ ] Request body > 14 MB returns anything other than 413 `request_body_too_large`. **Fail.**
- [ ] Request body > 1 MB with `cfg.Attachments.Enabled=false` returns anything other than 413 `request_body_too_large`. **Fail** (Phase 1 cap preserved when attachments are off).
- [ ] One 6 MB attachment under `MaxSizeBytes=5MB` → response is anything other than 413 `attachment_too_large`. **Fail.**
- [ ] Submit with declared `image/png` but JPEG bytes → response is anything other than 415 `attachment_mime_mismatch`. **Fail.**
- [ ] Submit with `mime_type` not in published `allowed_mime_types` → response is anything other than 415 `attachment_mime_not_allowed`. **Fail.**
- [ ] Submit with `attachments[]` non-empty when `cfg.Attachments.Enabled=false` → response is anything other than 400 `attachments_disabled`. **Fail.**
- [ ] Submit with `type:"file"` → response is anything other than 400 `attachment_type_unsupported`. **Fail.**
- [ ] Submit with `url` not a `data:` URL → response is anything other than 400 `attachment_malformed`. **Fail.**
- [ ] An adapter's `Create()` causes a downstream call after `attachvalidate` rejected the attachments. **Fail** (order of operations: validate before route).
- [ ] Linear test: an asset-upload non-2xx leads to issueCreate being called anyway. **Fail** (orphan-prevention regression).
- [ ] Zendesk test: an uploads.json non-2xx error message includes the response body. **Fail** (token-echo guard, L005).
- [ ] Phase 1+4+5 regression: anonymous + email + SSO smokes pass unchanged with `cfg.Attachments.Enabled=true` and empty `attachments[]`. **Fail otherwise.**

## 7. Final smoke (mandatory)

Proves the Phase 6 deliverable end-to-end. The unit + integration layer (httptest mocks for every adapter's native upload sequence; injected stub `html2canvas`; injected stub canvas-2d context; golden PNG/JPEG/WebP byte fixtures; consolidated startup gate fixtures) is fully credit-free and runs in `go test ./...` + `npm run test`. The **live chatwoot attachment smoke pauses for the maintainer** (reuses Phase 3 chatwoot.cloud credentials). No live Linear/Zendesk/Fider smoke in Phase 6 — covered by httptest contract tests.

```
1. Q9 startup smoke (no LLM credit; self-runnable):
   For each new misconfig YAML:
     - attachments-bad-storage-mode.yaml:    storage.mode: "s3"
     - attachments-cap-inverted.yaml:        max_size_bytes:20000000, max_total_bytes:10000000
   Start the relay binary; assert exit 1 and stdout contains the structured
   "relay: startup config errors" log line listing the matching problem text.
   Combined YAML (all Phase-5 + Phase-6 misconfigs) emits ONE log line listing
   every problem (operator fixes all in one restart cycle).

2. Caps-discovery smoke (no LLM credit; self-runnable):
   - cfg.Attachments.Enabled=true, all five adapters registered:
       /init returns capabilities.attachments with the intersection list
       (in v0: ["image/png","image/jpeg","image/webp"]).
   - cfg.Attachments.Enabled=false:
       /init omits capabilities.attachments entirely.
   - cfg.Attachments.AllowedMIMETypes=[]:
       /init omits capabilities.attachments.

3. Validation smokes (no LLM credit; self-runnable via drive-attachments.ts):
   - 6 MB attachment with MaxSizeBytes=5MB → 413 attachment_too_large.
   - Two 6 MB attachments (each over cap)  → 413 attachment_too_large (first encountered).
   - Three 4 MB attachments (12 MB aggregate, MaxTotalBytes=10MB) → 413 attachments_exceed_total.
   - Declared image/png with JPEG magic bytes → 415 attachment_mime_mismatch.
   - mime_type "image/heic" with cfg.AllowedMIMETypes=[png,jpeg,webp] → 415 attachment_mime_not_allowed.
   - url "not-a-data-url" → 400 attachment_malformed.
   - type:"file" with valid PNG → 400 attachment_type_unsupported.
   - cfg.Attachments.Enabled=false + non-empty attachments[] → 400 attachments_disabled.

4. Forward smoke via webhook adapter (no LLM credit; self-runnable):
   Submit with one valid 1×1 PNG, route to webhook adapter on a local httptest
   server. Webhook receiver logs the POST body; assert attachments[0].url is the
   original data: URL verbatim AND attachments[0].mime_type == "image/png".

5. Adapter native-sequence smokes (no LLM credit; httptest):
   For each of chatwoot/fider/linear/zendesk, drive Create() via httptest and
   assert the documented native sequence:
     - chatwoot:   assert attachments[] present in conversation-create body.
     - fider:      assert ![alt](data:image/png;base64,...) substring in description.
     - linear:     assert N asset-upload POSTs PRECEDE issueCreate AND
                   issueCreate body references the returned asset URLs.
     - zendesk:    assert N /uploads.json POSTs share one token AND
                   ticket body includes uploads:[<token>].
   Failure-injection: each adapter's failure path is exercised; orphan paths
   verified (linear+zendesk return error BEFORE create call, no orphan ticket).

6. Live chatwoot attachment smoke (PAUSE for maintainer; uses Phase 3
   chatwoot.cloud creds: CHATWOOT_TOKEN, CHATWOOT_INBOX_ID, CHATWOOT_ACCOUNT_ID):
   Spin up examples/vue-anonymous. Click Attach → modal opens with captured
   page → draw one redaction rectangle → click Save → click Submit.
   EXPECT: Chatwoot conversation appears in the configured inbox WITH the
   screenshot visible inline as an attachment; the redaction rectangle is
   visible in the saved image. Maintainer greps over the relay's live-smoke
   log for the chatwoot api token → zero matches (L005 confirmation).

7. Phase 1+4+5 regression (no LLM credit; self-runnable):
   drive-auth-email.ts (Phase 4) + drive-auth-sso.ts (Phase 4) + drive-abuse.ts
   (Phase 5) pass unchanged under Phase 6's chain (cfg.Attachments.Enabled=true,
   no attachments in the smoke payloads). Confirms no regression in middleware
   chain, dispatcher, abuse gates, or payloadbuild for non-attachment submissions.

8. Body-cap regression (no LLM credit; self-runnable):
   With cfg.Attachments.Enabled=false, a 2 MB submission (no attachments)
   returns 413 request_body_too_large (the 1 MB cap is preserved when
   attachments are off).
```

A phase is NOT done until this smoke passes from a clean state. Steps 1–5 + 7–8 are self-runnable; step 6 (live chatwoot) pauses for explicit maintainer go-ahead per the credit/secret guard.

## 8. Shared Contracts (SINGLE SOURCE OF TRUTH)

These shapes are **frozen** in the noted sub-plan; later sub-plans consume them unchanged.

### 8.1 The frozen Phase-1+3+4+5 seams (UNCHANGED)

- `adapter.Adapter` interface (`relay/internal/adapter/adapter.go`) — UNCHANGED. Phase 6 adds a SEPARATE optional `CapableAdapter` interface for capability discovery (metadata only).
- `payload.IntakePayload` / `payload.Attachment` generated types (`relay/internal/payload/types.go`) — UNCHANGED. The `attachments[]` field + `Attachment` struct have been present since Phase 0.
- `schema/payload.v1.json` — UNCHANGED. No codegen rerun. Schema's `Attachment.type` enum permits `"file"`; Phase 6 rejects `"file"` at the attachvalidate layer (schema stays permissive so v1+ enables file attachments without a schema bump).
- `auth.Middleware.Handler` signature, `auth.SessionContext`, `auth.Store`, `auth.NewMiddleware/NewMiddlewareWithModes` — UNCHANGED.
- Phase 5 abuse gates (per-IP, per-session, daily budget, CAPTCHA) — UNCHANGED. `/submit` still flows through all of them; Phase 6 only widens the body cap.
- The chi route-registration shape (`registerIntakeRoutes`) — additive only (no new routes; `/init` and `/submit` body shapes grow additively).
- `intake/license` canonicalization — Phase 3 frozen.
- `payloadbuild.Builder` constructor — additive only.
- `setRetryAfter` helper (`relay/internal/server/errors.go`) — unused by Phase 6 (no time-based remediation for 413/415).

### 8.2 Config additions (additive — `relay/internal/config/config.go`, FROZEN in 6-i)

```go
type Config struct {
    Server      ServerConfig      `yaml:"server"`
    LLM         LLMConfig         `yaml:"llm"`
    Auth        AuthConfig        `yaml:"auth"`
    Adapters    AdaptersConfig    `yaml:"adapters"`
    Routing     RoutingConfig     `yaml:"routing"`
    License     LicenseConfig     `yaml:"license"`
    Captcha     CaptchaConfig     `yaml:"captcha"`     // Phase 5
    RateLimit   RateLimitConfig   `yaml:"ratelimit"`   // Phase 5
    Attachments AttachmentsConfig `yaml:"attachments"` // 6-i NEW
}

type AttachmentsConfig struct {
    Enabled          bool               `yaml:"enabled"`            // default true
    MaxSizeBytes     int                `yaml:"max_size_bytes"`     // default 5_242_880  (5 MB)
    MaxTotalBytes    int                `yaml:"max_total_bytes"`    // default 10_485_760 (10 MB)
    AllowedMIMETypes []string           `yaml:"allowed_mime_types"` // default ["image/png","image/jpeg","image/webp"]
    Storage          AttachmentsStorage `yaml:"storage"`
}

type AttachmentsStorage struct {
    Mode string `yaml:"mode"` // "" or "forward" only in v0; any other value → fatal startup
}
```

Defaults applied in `config.applyDefaults` (6-i). Validated by the Q9-style consolidated startup gate in `main.go startupProblems` (per L016, the gate RETURNS the parsed/defaulted `AttachmentsConfig` for consumers to use — no re-parse-with-discarded-error).

### 8.3 SubmitRequest extension (additive — `relay/internal/dto/dto.go`, FROZEN in 6-i)

```go
type SubmitRequest struct {
    Messages    []TurnMessage       `json:"messages"`
    Client      ClientInfo          `json:"client"`
    UserClaims  map[string]any      `json:"user_claims"`
    Context     ContextInfo         `json:"context"`
    RoutingHint *string             `json:"routing_hint"`
    Attachments []SubmitAttachment  `json:"attachments,omitempty"` // 6-i NEW
}

type SubmitAttachment struct {
    Type     string `json:"type"`            // "screenshot" only in v0; "file" → 400 attachment_type_unsupported
    MIMEType string `json:"mime_type"`       // declared; magic-byte validated against bytes
    URL      string `json:"url"`             // data:image/png;base64,...
    Label    string `json:"label,omitempty"`
}
```

The corresponding TS types in `core/src/types.ts` mirror these additively in 6-iii.

### 8.4 Capabilities + InitResponse extension (additive — `relay/internal/server/dto.go`, FROZEN in 6-i)

```go
type Capabilities struct {
    AuthModes       []string                  `json:"auth_modes"`
    Streaming       bool                      `json:"streaming"`
    RequiresCaptcha []string                  `json:"requires_captcha,omitempty"` // Phase 5
    Attachments     *CapabilitiesAttachments  `json:"attachments,omitempty"`      // 6-i NEW
}

// nil → attachments disabled OR no enabled adapter accepts any allowed type.
// Widget hides Attach UI when this is nil.
type CapabilitiesAttachments struct {
    MaxSizeBytes     int      `json:"max_size_bytes"`
    MaxTotalBytes    int      `json:"max_total_bytes"`
    AllowedMIMETypes []string `json:"allowed_mime_types"`
}
```

### 8.5 attachvalidate package (FROZEN in 6-i — `relay/internal/attachvalidate/`)

```go
package attachvalidate

import (
    "errors"

    "intake/internal/payload"
)

// Decoded is the validator's output: a single attachment whose bytes have been
// base64-decoded out of the data: URL, magic-byte-matched against its declared
// mime_type, and confirmed against per-attachment + aggregate caps.
type Decoded struct {
    Raw       []byte
    MIMEType  string
    SizeBytes int
    Label     string
    Type      payload.AttachmentType // "screenshot" only in v0
}

// Config carries the validator's enforcement knobs.
type Config struct {
    MaxSizeBytes     int      // per-attachment cap
    MaxTotalBytes    int      // aggregate cap (sum of Raw lengths)
    AllowedMIMETypes []string // intersection of cfg + adapter capabilities
}

// ValidateAll decodes and validates every attachment in atts. Returns the
// decoded slice (1:1 with atts) on success, or the FIRST encountered sentinel
// error. Errors are NOT wrapped — callers (submitHandler) inspect them with
// errors.Is to map to specific HTTP status codes.
func ValidateAll(atts []payload.Attachment, cfg Config) ([]Decoded, error)

// DecodeOne is the per-adapter helper: decodes one payload.Attachment's data:
// URL to raw bytes + detected MIME. Used inside each adapter's Create() to
// obtain the bytes for native upload (the validation already passed in submit).
func DecodeOne(att payload.Attachment) (raw []byte, mime string, err error)

var (
    ErrAttachmentTooLarge        = errors.New("attachment exceeds max_size_bytes")               // 413
    ErrAggregateTooLarge         = errors.New("attachments exceed total cap")                    // 413
    ErrMIMENotAllowed            = errors.New("attachment mime_type not in allowlist")           // 415
    ErrMIMEMismatch              = errors.New("attachment bytes do not match declared mime_type")// 415
    ErrBadDataURL                = errors.New("attachment url is not a valid data: URL")         // 400
    ErrAttachmentTypeUnsupported = errors.New("attachment type unsupported in v0")               // 400
)
```

### 8.6 Adapter Capabilities interface (FROZEN in 6-i — `relay/internal/adapter/capabilities.go`)

```go
package adapter

// Capabilities reports what an adapter supports.
type Capabilities struct {
    AcceptedMIMETypes []string // empty = no attachments supported
}

// CapableAdapter is an OPTIONAL interface adapters implement to advertise
// attachment capabilities. Adapters that don't implement it advertise no
// capabilities (effectively []string{}). The frozen Adapter interface is
// UNCHANGED.
type CapableAdapter interface {
    Capabilities() Capabilities
}
```

Each adapter (`webhook`, `chatwoot`, `fider`, `linear`, `zendesk`) implements `Capabilities()` returning `["image/png","image/jpeg","image/webp"]` in v0. The struct exists so v1+ can specialise per-adapter without touching every call site.

### 8.7 Deps extension (FROZEN in 6-i — `relay/internal/server/deps.go`)

```go
type Deps struct {
    // ... all existing Phase 1-5 fields unchanged ...

    // 6-i NEW:

    // AttachmentsCfg is the attachments section of the loaded config.
    // initHandler reads it to compute capabilities.attachments; submitHandler
    // reads it to construct the attachvalidate.Config; the chi handler reads
    // it to size MaxBytesReader.
    AttachmentsCfg config.AttachmentsConfig

    // AttachmentMIMEs is the published allowlist (cfg ∩ adapter union),
    // computed once at startup. Empty → /init omits capabilities.attachments
    // and submitHandler refuses any non-empty attachments[] with 400
    // attachments_disabled.
    AttachmentMIMEs []string

    // BodyCapBytes is the per-request MaxBytesReader cap on /submit
    // (14 MB when cfg.Attachments.Enabled=true, 1 MB otherwise). Used by
    // submitHandler.
    BodyCapBytes int64
}
```

### 8.8 Endpoint contract shapes (FROZEN in 6-i, fully exercised in 6-ii)

```
POST /v1/intake/init
  200 InitResponse{
        session_id, capabilities:{
          auth_modes, streaming,
          requires_captcha?,             // Phase 5
          attachments?:{                 // 6-i NEW; nil when cfg disabled or no caps
            max_size_bytes, max_total_bytes, allowed_mime_types
          }
        },
        auth?, captcha?                  // Phase 4 + 5
      }

POST /v1/intake/submit (Phase 1; Phase 6 ADDS:)
  Body: SubmitRequest now optionally carries attachments[].
  413 {"error":{"code":"request_body_too_large", "message":"submission body exceeds limit"}}
        (replaces existing 400 path for over-cap bodies; malformed JSON stays 400)
  413 {"error":{"code":"attachment_too_large", "message":"attachment exceeds max_size_bytes"}}
  413 {"error":{"code":"attachments_exceed_total", "message":"attachments exceed total cap"}}
  415 {"error":{"code":"attachment_mime_not_allowed", "message":"attachment mime_type not allowed"}}
  415 {"error":{"code":"attachment_mime_mismatch", "message":"attachment bytes do not match declared mime_type"}}
  400 {"error":{"code":"attachment_malformed", "message":"attachment url is not a valid data: URL"}}
  400 {"error":{"code":"attachment_type_unsupported", "message":"attachment type unsupported in v0"}}
  400 {"error":{"code":"attachments_disabled", "message":"attachments are disabled on this relay"}}
```

No `Retry-After` on any of these (per Phase 5's RFC 9110 stance — `setRetryAfter` is for rate-limit / time-based remediation, not user-action errors).

## 9. Notes

- Module path remains `intake`. Go 1.23.2.
- L010 (PS 5.1 BOM) applies to any new smoke YAML written via `Set-Content` — use `-Encoding ascii`.
- L016 (return parsed values from startup gate) applies to `validateAttachments` in `main.go`: it MUST return the parsed/defaulted `AttachmentsConfig` for the `attachvalidate.Config` constructor + `computeAttachmentsCaps` to use. No re-parse, no discarded error.
- L005 (redact-before-truncate) applies to new linear asset-upload error paths and zendesk uploads endpoint paths.
- L011 (linear's asset-upload-before-create) — Phase 6 follows the same pattern: asset upload MUST precede issueCreate so a failure doesn't leave an orphan issue. Regression-tested in 6-ii.
- L015 (derived-field test gaps) — every new adapter's `Create_WithAttachments` test MUST also exercise the no-attachments path (regression for the Phase 1+4 baseline).
- L019 (smoke-fixture math) — when 6-iv's `drive-attachments.ts` exercises multiple validators (per-attachment cap + aggregate cap), the fixture math must make per-attachment fire on the smallest test case AND aggregate fire on a separate larger test case. Don't shadow gates.
- Schema is unchanged in Phase 6. `scripts/verify-contract.sh` should pass without re-running codegen.
- `go mod tidy` after Phase 6 must be a no-op.
