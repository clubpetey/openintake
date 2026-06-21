# Phase 6 — Attachments — Design Spec

> **Status:** Approved design (brainstorming output), pre-planning
> **Date:** 2026-05-28
> **Implements:** [docs/PROJECT.md](../PROJECT.md) §11 (attachment handling), §6 (payload schema), §9 (`attachments:` config block), §17 (security — magic-byte rule)
> **Decomposes:** [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](2026-05-26-v0-decomposition-and-phasing-design.md) Phase 6 row + P3 → P6 dependency edge
> **Inherits seams from:** Phase 1 (adapter interface, /submit handler shape, payloadbuild), Phase 3 (chatwoot/linear/zendesk/fider/webhook adapters), Phase 4 (auth middleware), Phase 5 (per-IP + per-session + budget + CAPTCHA, consolidated Q9 startup gate, `Deps` shape)
> **Companion phase doc:** `ai/tasks/phase-6/README.md` (authored via writing-plans after this spec is approved)

---

## 1. Goal

Ship screenshot attachments end-to-end: html2canvas capture in the Vue widget → rectangle redaction in a modal → base64-encoded PNG/JPEG/WebP into the existing `attachments[]` field of `IntakePayload` → POSTed inline to `/v1/intake/submit` → relay validates magic-bytes + per-attachment cap + aggregate cap → router dispatches to one adapter → that adapter forwards via its native mechanism. No new endpoint, no schema change, no schema codegen rerun (the existing `attachments[]` field has been in `schema/payload.v1.json` since Phase 0).

---

## 2. Scope and non-scope

### In scope (v0)

- Widget capture of the host page via `html2canvas`, dependency-injectable for tests.
- Widget redaction modal (`ScreenshotRedactor.vue`) — rectangular solid-fill redactions on a canvas; Save/Cancel.
- Widget `AttachmentStrip.vue` showing pending attachments with remove buttons + aggregate size badge.
- Capabilities discovery via `/v1/intake/init` → `capabilities.attachments` block; widget hides the Attach button when this is `null`.
- Per-attachment cap (`max_size_bytes`, default 5 MB) and aggregate cap (`max_total_bytes`, default 10 MB), enforced server-side and pre-validated client-side as UX.
- MIME allowlist (default `["image/png","image/jpeg","image/webp"]`), enforced server-side; widget restricts capture/file-picker to that list.
- Server-side magic-byte validation via stdlib `net/http.DetectContentType` (no new Go dep).
- Per-adapter native forwarding: chatwoot inline base64 in conversation create, fider inline markdown image refs (data: URLs), linear asset-upload-before-issueCreate, zendesk uploads-then-attach, webhook pass-through via existing JSON serialization.
- Q9-style consolidated startup gate covers `storage.mode != "forward"`, `max_size_bytes > max_total_bytes`. Returns parsed `AttachmentsConfig` to consumers per L016 (no re-parse-with-discarded-error).
- Body cap raised on `/v1/intake/submit` from 1 MB to 14 MB **only when `cfg.Attachments.Enabled=true`** (10 MB aggregate × 1.37 base64 overhead + ~200 KB headroom for conversation/context); stays 1 MB otherwise.
- Phase 1+4+5 regression smokes pass unchanged.

### Out of scope (v0; deferred)

- Persistent attachment storage (v1 — `attachments.storage.mode: "s3"`).
- Console / network log capture (v1+).
- Video / screen recording (post-v1).
- Separate streaming upload endpoint `/v1/intake/upload` (v1+; revisit trigger: any single attachment must exceed 5 MB).
- File-type attachments (`type:"file"`) — schema permits the value but `attachvalidate` rejects with `400 attachment_type_unsupported` in v0. Schema stays permissive so v1 can drop the rejection without a schema bump.
- Per-tenant attachment policy (v1+ hosted relay).
- Forced "no-PII" acknowledgment dialog — decomposition spec §4 Q8 already resolved: default off; `require_redaction_ack` opt-in stays a v1+ config.
- Content-aware / AI-driven redaction (post-v1).
- Live Linear / Zendesk / Fider smokes (covered by httptest contract tests; live Linear smoke is a v1 follow-up).
- HEIC / AVIF support (stdlib `DetectContentType` does not sniff these; v1+).

---

## 3. Architectural Decision Record

Five decisions lock in here. Each carries a trigger for revisiting.

### 3.1 Inline transport in `/submit` body per PROJECT.md §11

Per-attachment cap 5 MB, aggregate cap 10 MB, base64-encoded (~1.37× raw wire overhead). `MaxBytesReader` on `/v1/intake/submit` grows from `1<<20` (1 MB) to `(1<<20)*14` (14 MB) ONLY when `cfg.Attachments.Enabled=true`; remains 1 MB otherwise to avoid widening the attack surface for operators who disabled attachments.

**Revisit trigger:** any single attachment must exceed 5 MB, OR multi-image submissions push the aggregate past 10 MB → introduce `/v1/intake/upload` with pre-signed S3 URLs (v1+).

### 3.2 No interface change to `adapter.Adapter`

Each adapter handles `p.Attachments` internally inside its existing `Create()`. The frozen `Adapter` interface (Phase 1) stays exactly unchanged. Per-adapter native sequencing varies (Linear must asset-upload BEFORE issueCreate; Chatwoot inlines DURING conversation create) and the interface should not pretend otherwise.

A NEW optional `CapableAdapter` interface advertises supported MIME types for capabilities discovery — this is metadata, not behavior, and is purely additive.

**Revisit trigger:** an adapter is added that requires post-create attachment uploads that can fail independently AND the maintainer wants centralised orphan-ticket handling → add an optional `Attacher` post-hoc interface (Approach C from the brainstorming question).

### 3.3 Per-adapter accepted-MIME-types published via `/init` Capabilities

`InitResponse.Capabilities` gains an optional `attachments` block (`max_size_bytes`, `max_total_bytes`, `allowed_mime_types`). The relay computes it as the **union** across enabled adapters' `Capabilities().AcceptedMIMETypes`, **intersected** with `cfg.Attachments.AllowedMIMETypes`. If the result is empty (or `cfg.Attachments.Enabled=false`), the block is `nil` and the widget hides the Attach button entirely.

If routing later picks an adapter that doesn't accept a given type, that adapter's `Create()` silently drops the attachment with a `slog.Warn` (the graceful per-adapter pattern). Operators wanting strict guarantees should configure `allowed_mime_types` to the intersection by hand.

**Revisit trigger:** routing rules become MIME-aware (different adapters per attachment type) → compute capabilities per routing rule.

### 3.4 Magic-byte validator uses `net/http.DetectContentType` (stdlib)

Single stdlib call on the first 512 bytes; rejects when detected MIME ≠ declared `mime_type`. Zero new Go modules; stdlib-version drift is the failure surface tests must cover. Tradeoff accepted: stdlib `DetectContentType` does not sniff HEIC/AVIF — explicitly out of v0 scope.

**Revisit trigger:** stdlib detection misclassifies a v0 format (WebP edge case observed in the wild) OR we add formats stdlib doesn't sniff → switch to a curated signature table.

### 3.5 Widget redaction is a dedicated modal component

`ScreenshotRedactor.vue` is a full-screen modal launched on Attach-click. Capture runs immediately; modal hosts a `<canvas>` with mouse-drawn solid-fill rectangles + Clear All + Save + Cancel. Tested via `@vue/test-utils` mount-in-isolation with a stubbed `html2canvas` and a stubbed canvas-2d context. `html2canvas` is dependency-injected via `core/src/capture.ts` `setHtml2Canvas(fn)` so tests don't load the real library.

**Revisit trigger:** the v1 React widget reuses redaction logic → extract a framework-agnostic redaction primitive into `@openintake/core`.

---

## 4. Open-question resolutions

All design-time open questions raised during brainstorming were resolved; none are deferred.

| # | Question | Resolution |
|---|---|---|
| Q-A | Adapter Attach() seam shape | **Approach A** — `adapter.Adapter` UNCHANGED; each `Create()` handles `p.Attachments` internally. New optional `CapableAdapter` interface for capability discovery only; no behavior interface change. |
| Q-B | Fider attachment behavior | **Inline as markdown image references** in the post description (data: URLs). Fider advertises `image/png`+`image/jpeg`+`image/webp` in its `Capabilities()`. Graceful degradation if a Fider deployment's markdown sanitizer strips data: URLs — the post still has all conversation text. |
| Q-C | Magic-byte validator strictness | **`net/http.DetectContentType`** (stdlib, first 512 bytes). No new Go module. |
| Q-D | Widget redaction UX | **Full-screen modal overlay** (`ScreenshotRedactor.vue`) on Attach-click. Capture runs immediately; canvas with mouse-drawn solid-fill rectangles; Save commits canvas→base64 PNG into pending list. |
| Q-E | Decomposition approach | **Approach A — 4 sub-plans** (seam / adapters / widget / smoke), 6-ii + 6-iii parallelizable after 6-i. |
| Q-F | Per-attachment vs aggregate cap | **Both** — `max_size_bytes` (5 MB default, per-attachment) AND `max_total_bytes` (10 MB default, aggregate). Both enforced server-side; widget enforces them client-side as UX. |
| Q-G | Body cap for over-cap submissions | **413 `request_body_too_large`** (new), refining the existing 400 path. Pure over-cap → 413; malformed JSON stays 400. |
| Q-H | `type:"file"` in v0 | **Rejected at attachvalidate with 400 `attachment_type_unsupported`**, NOT at the schema layer. Schema stays permissive so v1 drops the rejection without a schema bump. |
| Q-I | Capabilities intersection model | **Union across enabled adapters, intersected with operator `allowed_mime_types`**. Widget hides Attach when result is empty. An adapter that receives a type it doesn't accept silently drops with `slog.Warn`. |
| Q-J | Live smoke scope | **Live chatwoot only** in Phase 6. Linear/Zendesk covered by httptest contract tests against documented API shapes. Fider not live-smoked (deployment-variant markdown sanitizer behavior). |

---

## 5. Components

### 5.1 New Go packages

```
relay/internal/attachvalidate/
    attachvalidate.go              ValidateAll(atts, cfg) → []Decoded or sentinel error
    attachvalidate_test.go         magic-byte mismatch, oversized, wrong-mime, base64 corruption,
                                   empty-allowlist, aggregate-cap, per-attachment-cap,
                                   type:"file" rejection, label sanitization
    fixtures/                      golden PNG/JPEG/WebP byte fixtures (smallest valid headers)
```

Exports:

```go
package attachvalidate

// Decoded is the validator's output: a single attachment whose bytes have been
// base64-decoded out of the data: URL, magic-byte-matched against its declared
// mime_type, and confirmed against per-attachment + aggregate caps.
type Decoded struct {
    Raw       []byte
    MIMEType  string
    SizeBytes int
    Label     string
    Type      payload.AttachmentType  // "screenshot" only in v0
}

// Config carries the validator's enforcement knobs. Passed in by submitHandler;
// the caller composes it from cfg.Attachments + the resolved
// capabilities-intersection list (so attachvalidate doesn't need to know about
// adapter capabilities directly).
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

var (
    ErrAttachmentTooLarge       = errors.New("attachment exceeds max_size_bytes")        // 413
    ErrAggregateTooLarge        = errors.New("attachments exceed total cap")              // 413
    ErrMIMENotAllowed           = errors.New("attachment mime_type not in allowlist")     // 415
    ErrMIMEMismatch             = errors.New("attachment bytes do not match declared mime_type") // 415
    ErrBadDataURL               = errors.New("attachment url is not a valid data: URL")   // 400
    ErrAttachmentTypeUnsupported = errors.New("attachment type unsupported in v0")        // 400
)
```

### 5.2 Adapter capabilities helper

```
relay/internal/adapter/capabilities.go     NEW (6-i)
```

```go
package adapter

// Capabilities reports what an adapter supports.
type Capabilities struct {
    AcceptedMIMETypes []string  // empty = no attachments supported
}

// CapableAdapter is an OPTIONAL interface adapters implement to advertise
// attachment capabilities. Adapters that don't implement it advertise no
// capabilities (effectively []string{}). The frozen Adapter interface is
// untouched.
type CapableAdapter interface {
    Capabilities() Capabilities
}
```

Each existing adapter file adds one tiny `Capabilities()` method:

| Adapter | Returns |
|---|---|
| `webhook.Adapter.Capabilities()` | `{AcceptedMIMETypes: ["image/png","image/jpeg","image/webp"]}` |
| `chatwoot.Adapter.Capabilities()` | same |
| `fider.Adapter.Capabilities()` | same |
| `linear.Adapter.Capabilities()` | same |
| `zendesk.Adapter.Capabilities()` | same |

In v0 all five advertise identically, so the published intersection equals `cfg.Attachments.allowed_mime_types`. The struct exists so v1+ can specialise without touching every call site.

### 5.3 Config addition

```go
// relay/internal/config/config.go — NEW top-level block (6-i)

type Config struct {
    // ... existing fields ...
    Attachments AttachmentsConfig `yaml:"attachments"`  // NEW
}

type AttachmentsConfig struct {
    Enabled          bool               `yaml:"enabled"`            // default true
    MaxSizeBytes     int                `yaml:"max_size_bytes"`     // default 5_242_880
    MaxTotalBytes    int                `yaml:"max_total_bytes"`    // default 10_485_760
    AllowedMIMETypes []string           `yaml:"allowed_mime_types"` // default ["image/png","image/jpeg","image/webp"]
    Storage          AttachmentsStorage `yaml:"storage"`
}

type AttachmentsStorage struct {
    Mode string `yaml:"mode"`  // "forward" only in v0; any other value → fatal startup
}
```

Defaults applied in `config.applyDefaults`. Validated by the Q9-style consolidated startup gate in `main.go startupProblems`:

- `Storage.Mode != "" && Storage.Mode != "forward"` → fatal
- `MaxTotalBytes > 0 && MaxSizeBytes > MaxTotalBytes` → fatal
- `AllowedMIMETypes` contains a type no enabled adapter advertises → `slog.Warn`, not fatal (operator may want a stricter allowlist than any adapter)

The validator function **returns the parsed/defaulted `AttachmentsConfig`** per L016 — consumers (the `attachvalidate.Config` constructor, the `computeAttachmentsCaps` helper) MUST use the returned value; no re-parse-with-discarded-error path.

### 5.4 DTO + InitResponse extensions (additive, omitempty)

```go
// relay/internal/server/dto.go — Capabilities gains an attachments block

type Capabilities struct {
    AuthModes       []string                  `json:"auth_modes"`
    Streaming       bool                      `json:"streaming"`
    RequiresCaptcha []string                  `json:"requires_captcha,omitempty"`
    Attachments     *CapabilitiesAttachments  `json:"attachments,omitempty"` // 6-i NEW
}

// nil → attachments disabled OR no enabled adapter accepts any allowed type;
// widget hides Attach UI.
type CapabilitiesAttachments struct {
    MaxSizeBytes     int      `json:"max_size_bytes"`
    MaxTotalBytes    int      `json:"max_total_bytes"`
    AllowedMIMETypes []string `json:"allowed_mime_types"`
}
```

```go
// relay/internal/dto/dto.go — SubmitRequest gains attachments

type SubmitRequest struct {
    // ... existing fields ...
    Attachments []SubmitAttachment `json:"attachments,omitempty"` // 6-i NEW
}

type SubmitAttachment struct {
    Type     string `json:"type"`              // "screenshot" only in v0
    MIMEType string `json:"mime_type"`         // declared; validated against bytes
    URL      string `json:"url"`               // data:image/png;base64,...
    Label    string `json:"label,omitempty"`
}
```

The corresponding TS types in `core/src/types.ts` mirror these additively.

### 5.5 submit.go orchestration changes (6-i)

1. **Body-size cap** — `MaxBytesReader` raised from `1<<20` (1 MB) to `(1<<20)*14` (14 MB) **only when `cfg.Attachments.Enabled=true`**, via a value passed through `Deps`. The bytes-too-large path distinguishes `*http.MaxBytesError` (413 `request_body_too_large`) from other decode errors (400 `bad_request`).
2. **Order of operations** (new step inserted after Build, before Route):
   1. Body decode → `SubmitRequest` (existing)
   2. Session extraction (existing)
   3. Classify (existing)
   4. `Builder.Build` → `payload.IntakePayload` (existing; now populates `p.Attachments` additively)
   5. **`attachvalidate.ValidateAll(p.Attachments, cfg)` → `[]Decoded` or sentinel error (NEW)**
   6. `Router.Route` (existing)
   7. `adapter.Create` (existing; the adapter reads `p.Attachments` internally and its native sequence handles the bytes)
3. **Wiring `[]Decoded` to adapters.** The `payload.Attachment.Url` field continues to carry the original `data:` URL (so `payloadbuild`'s schema validation passes against the existing schema). Adapters that need raw bytes call a tiny helper `attachvalidate.DecodeOne(att) ([]byte, string, error)` inside their `Create()`. This avoids threading a separate `[]Decoded` slice alongside `p.Attachments` (which would force every adapter signature to change). The decode is cheap (base64) and runs twice — once in `ValidateAll` for validation, once per adapter — accepted because the alternative (mutating `p.Attachments[i].Url` or carrying parallel state via context) is messier.

### 5.6 payloadbuild additions

`payloadbuild.Build` is extended additively: when `req.Attachments` is non-empty, populate `p.Attachments` with one `payload.Attachment` per entry. The runtime schema validation (L003 mitigation) covers existing schema rules. **Magic-byte + size-cap validation does NOT live in payloadbuild** — it's a separate post-build step in submitHandler. payloadbuild stays focused on shape; attachvalidate stays focused on content.

### 5.7 Widget components (Vue 3 + @openintake/core)

```
core/src/
    capture.ts                    NEW (6-iii) — html2canvas wrapper + canvas→base64 PNG encoder;
                                  exports setHtml2Canvas(fn) for DI; production registers once on first call
    capture.test.ts               NEW (6-iii) — stub html2canvas, FileReader; assert blob→data URL conversion
    attachments.ts                NEW (6-iii) — pending-attachments client state + per-/aggregate-size accounting
    attachments.test.ts           NEW (6-iii)
    client.ts                     MODIFY — submit() threads attachments; init() parses caps.attachments
    types.ts                      MODIFY — InitResponse.capabilities.attachments + SubmitRequest.attachments

vue/src/components/
    ScreenshotRedactor.vue        NEW (6-iii) — full-screen modal; canvas + rectangle draw + Save/Cancel
    ScreenshotRedactor.spec.ts    NEW (6-iii)
    AttachmentStrip.vue           NEW (6-iii) — thumbnail strip + remove buttons + aggregate-size badge
    AttachmentStrip.spec.ts       NEW (6-iii)
    IntakeWidget.vue              MODIFY — Attach button (visible iff caps non-null), strip, hooks

vue/src/composables/
    useIntake.ts                  MODIFY — pendingAttachments ref, attachAndRedact(), removeAttachment(i),
                                  clearAttachments(), canAttach computed; submit() threads attachments
    useIntake.spec.ts             MODIFY
```

`html2canvas` is the only new browser dep — pinned exactly per §6.

---

## 6. Tool version pins (per PHASE_PLANNING §5)

| Tool | Version | Reason |
|---|---|---|
| `html2canvas` | exact `1.4.1` | Load-bearing widget UX. Caret forbidden — a silent breaking change in capture semantics is a UX regression not caught by CI type-checks. Peer-dep set empty (no React/Vue requirement). |
| (none new Go) | — | Validation uses stdlib `net/http.DetectContentType` + `encoding/base64`. `go mod tidy` must remain a no-op after Phase 6. |

`scripts/check-pins.sh` gets one new line checking `html2canvas` has no caret in `core/package.json` — same style as the existing `golang-jwt`/`keyfunc/v3`/`golang.org/x/time` checks.

---

## 7. Data flow

### 7.1 End-to-end

```
Widget (browser)                          Relay (Go)                              Downstream
─────────────────                         ──────────                              ──────────

1. start()  ── POST /v1/intake/init ────►
                                          initHandler:
                                            caps.Attachments = compute(cfg, enabled-adapters)
                                          ◄── InitResponse{capabilities:{
                                                 attachments: {max_size_bytes,
                                                               max_total_bytes,
                                                               allowed_mime_types}
                                              }}

   If caps.attachments is null → widget hides Attach button entirely.
   If non-null → widget reveals Attach; restricts capture/file picker to allowed_mime_types.

2. user clicks Attach
   capture.ts → html2canvas(document.body) → HTMLCanvasElement
   → mount ScreenshotRedactor modal with canvas
   → user draws redaction rectangles (solid black fill on overlay layer)
   → user clicks Save
   → canvas.toBlob('image/png') → FileReader.readAsDataURL → push into pending[]

3. user clicks Submit
   submit() builds SubmitRequest with
   attachments: [{type, mime_type, url:"data:...;base64,...", label}]

   ── POST /v1/intake/submit ─────────────►
                                          submitHandler:
                                            1. MaxBytesReader at 14 MB (cfg.Attachments.Enabled=true)
                                            2. json.Decode → SubmitRequest
                                            3. auth.FromContext → SessionContext
                                            4. classify.Classify → Result (existing)
                                            5. Builder.Build → payload.IntakePayload
                                               (existing; populates p.Attachments additively)
                                            6. attachvalidate.ValidateAll(p.Attachments, attCfg)
                                               ↳ 413 attachment_too_large
                                               ↳ 413 attachments_exceed_total
                                               ↳ 415 attachment_mime_not_allowed
                                               ↳ 415 attachment_mime_mismatch
                                               ↳ 400 attachment_malformed
                                               ↳ 400 attachment_type_unsupported
                                            7. Router.Route → one adapter (existing)
                                            8. adapter.Create(ctx, p)
                                               ↳ adapter reads p.Attachments and runs native sequence
                                                            ── (native flow) ──►
                                                                 contact + inline base64 (chatwoot)
                                                                 asset upload then issueCreate (linear)
                                                                 uploads.json then ticket (zendesk)
                                                                 markdown inline (fider)
                                                                 JSON pass-through (webhook)
                                                            ◄── ticket / issue / post id
                                          ◄── SubmitResponse{external_id, external_url, ...}
```

### 7.2 Per-adapter handling matrix

| Adapter | Sequence | Extra roundtrips |
|---|---|---|
| **webhook** | None — `p.Attachments` already serializes in `json.Marshal(p)`. No code change in `webhook.go` beyond the `Capabilities()` method. | 0 |
| **chatwoot** | Existing 2-call flow (createContact → createConversation) — the conversation-create body gains an inline `attachments[]` field with `[{file_type, data_url}]`. Single transaction with the conversation create. | 0 additional |
| **fider** | `renderBody(p)` extension appends `\n\n![<label or "screenshot N">](data:image/png;base64,...)` per attachment. If the Fider deployment's markdown sanitizer strips data: URLs, the post still has all conversation text (graceful degradation). | 0 additional |
| **linear** | For each attachment: POST raw bytes to the Linear file-upload endpoint → returns asset URL. Then issueCreate references those URLs in `attachmentLinks` (or the equivalent input field per Linear's IssueCreateInput schema). Upload-before-create ensures no orphan: if upload fails, issueCreate is never called. | N (one per attachment) before issueCreate |
| **zendesk** | For each attachment: POST raw bytes to `/api/v2/uploads.json` (subsequent uploads pass `?token=<first-token>` to share one token). Then the ticket-create includes `uploads: [<token>]`. | N (one per attachment) before ticket create |

**Failure modes** (per adapter, all internal to `Create()`):

- **chatwoot** — single-transaction; either succeeds entirely or fails entirely. No orphan.
- **fider** — markdown rendering can't fail; only the existing post-create call can.
- **linear** — asset-upload failure returns error before issueCreate. No orphan.
- **zendesk** — uploads.json failure returns error before ticket create. If upload N succeeds but N+1 fails, prior uploads' tokens are abandoned; Zendesk garbage-collects unattached uploads after 3 days per its docs.
- **webhook** — pass-through; receiver's responsibility.

### 7.3 Capabilities intersection (relay-side)

```go
func computeAttachmentsCaps(cfg config.AttachmentsConfig, enabled []adapter.Adapter) *CapabilitiesAttachments {
    if !cfg.Enabled || len(cfg.AllowedMIMETypes) == 0 {
        return nil  // widget hides UI
    }
    // Union across enabled adapters' Capabilities() — any adapter that supports
    // the type means routing MAY route to it.
    adapterUnion := map[string]bool{}
    for _, ad := range enabled {
        if c, ok := ad.(adapter.CapableAdapter); ok {
            for _, m := range c.Capabilities().AcceptedMIMETypes {
                adapterUnion[m] = true
            }
        }
    }
    if len(adapterUnion) == 0 {
        return nil
    }
    allowed := []string{}
    for _, m := range cfg.AllowedMIMETypes {
        if adapterUnion[m] {
            allowed = append(allowed, m)
        }
    }
    if len(allowed) == 0 {
        return nil
    }
    return &CapabilitiesAttachments{
        MaxSizeBytes:     cfg.MaxSizeBytes,
        MaxTotalBytes:    cfg.MaxTotalBytes,
        AllowedMIMETypes: allowed,
    }
}
```

---

## 8. Error handling

### 8.1 New error envelope codes (all reuse the existing `ErrorEnvelope` shape)

| HTTP | Code | When | Retryable? |
|---|---|---|---|
| **413** | `request_body_too_large` | Body exceeds `MaxBytesReader` (14 MB / 1 MB). Surfaced via `*http.MaxBytesError`; existing 400 path for over-cap bodies is REPLACED by 413 + this code. | No |
| **413** | `attachment_too_large` | One attachment's raw bytes exceed `cfg.Attachments.MaxSizeBytes` | No |
| **413** | `attachments_exceed_total` | Sum of all attachment raw bytes exceeds `cfg.Attachments.MaxTotalBytes` | No |
| **415** | `attachment_mime_not_allowed` | Declared `mime_type` not in published allowlist | No |
| **415** | `attachment_mime_mismatch` | `net/http.DetectContentType` on the bytes returns a MIME different from the declared one | No |
| **400** | `attachment_malformed` | `url` is not a `data:` URL, or base64 decode fails | No |
| **400** | `attachment_type_unsupported` | `type` ≠ `"screenshot"` in v0 (schema permits `"file"` for v1+) | No |
| **400** | `attachments_disabled` | Non-empty `attachments[]` but `cfg.Attachments.Enabled=false` | No |
| **502** | `adapter_error` | EXISTING. Adapter's native upload step failed (chatwoot inline rejected, linear asset upload 5xx, zendesk upload network error). Opaque client message; server log carries the adapter name + attachment count. | Maybe |

`writeError` is reused unchanged. `setRetryAfter` is NOT used for these — all 413/415/400 are user-action errors with no time-based remediation (RFC 9110 §10.2.3 stance Phase 5 codified).

### 8.2 Per-adapter Create() error wrapping

Each adapter follows existing conventions (L005 redact-before-truncate, L007 rune-aware Truncate):

- **chatwoot** — existing `chatwoot: create conversation returned %d: ...` continues to cover attachment-inline failures.
- **fider** — NO new error surface (markdown rendering can't fail).
- **linear** — NEW `linear: asset upload %d/%d returned %d: <redacted>` per L011 (asset uploads carry the same `Authorization` header).
- **zendesk** — NEW `zendesk: upload %d/%d returned %d` — body OMITTED per L005 (basic-auth echo risk applies to uploads endpoint too).
- **webhook** — no change.

### 8.3 Widget error surface

The existing red banner (`error.value` in `IntakeWidget.vue`) is reused unchanged. Phase 6 introduces a **code → friendly-string mapping in `useIntake.ts`** for attachment errors (replacing the current raw `submit failed: ${res.status} ${body}` message for these codes only — non-attachment errors keep their existing behavior). The mapping:

| Code | Banner text |
|---|---|
| `attachment_too_large` | "Screenshot too large — try a smaller region." |
| `attachments_exceed_total` | "Too many attachments — remove one." |
| `attachment_mime_not_allowed` | "This attachment type isn't supported." |
| `attachment_mime_mismatch` | "This attachment couldn't be verified — try recapturing." |
| `attachment_malformed` | "This attachment couldn't be verified — try recapturing." |
| `attachment_type_unsupported` | "This attachment type isn't supported." |
| `attachments_disabled` | "Attachments are disabled on this server." |
| `request_body_too_large` | "Your submission is too large to send." |

The widget never shows raw relay messages; it maps codes → user-readable strings.

### 8.4 Schema-validation interaction

The schema's `Attachment.type` enum includes `"file"` but PROJECT.md §11 says "v0 scope: screenshots only". Phase 6 keeps the schema permissive (so v1 can drop the rejection cleanly) and `attachvalidate` rejects `type != "screenshot"` with `400 attachment_type_unsupported`. Inline doc comment on the rejection notes the v1+ migration path.

---

## 9. Frozen seams Phase 6 must NOT modify

- `adapter.Adapter` interface — the whole point of decision §3.2.
- `payload.IntakePayload` / `payload.Attachment` generated types — never modified directly; schema is the source of truth and schema is unchanged.
- `schema/payload.v1.json` — unchanged in Phase 6 (`attachments[]` and `Attachment` have been present since Phase 0).
- `auth.Middleware.Handler` signature, `auth.SessionContext`, `auth.Store`, `auth.NewMiddleware/NewMiddlewareWithModes` constructors — frozen by Phase 4+5.
- Phase 5 abuse gates: per-IP, per-session, daily budget, CAPTCHA. `/submit` still flows through all of them; Phase 6 only widens the body cap.
- `chi` route-registration shape — no new routes; `/init` and `/submit` body shapes grow additively.
- `intake/license` canonicalization — Phase 3 frozen.
- `payloadbuild.Builder` constructor — additive only; magic-byte validation does NOT live here.
- `setRetryAfter` helper — unused by Phase 6 (no time-based remediation for 413/415).
- Phase 5 `Deps` shape — additive only; Phase 6 adds `AttachmentsCfg config.AttachmentsConfig` and `BodyCapBytes int64`.

---

## 10. Sub-plan decomposition (mirrors Phase 4+5 structure)

| # | Plan | Driver | Effort |
|---|---|---|---|
| **6-i** | Config (`AttachmentsConfig`) + `attachvalidate` package + InitResponse.Capabilities extension + SubmitRequest extension + body-cap raise + payloadbuild additive + Q9-consolidated startup gate extension + adapter `Capabilities()` (no behavior change) | the seam | M |
| **6-ii** | Per-adapter Create() handles attachments — chatwoot inline base64, fider markdown inline, linear asset-upload-before-issueCreate, zendesk uploads-then-attach, webhook pass-through verified | adapter implementations | M-L |
| **6-iii** | Widget: `core/src/capture.ts` (html2canvas wrapper, DI), `core/src/attachments.ts` (pending state + size accounting), `vue/src/components/ScreenshotRedactor.vue` (modal), `vue/src/components/AttachmentStrip.vue`, IntakeWidget + useIntake wiring, types.ts + client.ts extensions | widget UX | M |
| **6-iv** | Live chatwoot attachment smoke + Phase 1/4/5 regression smokes + `drive-attachments.ts` self-runnable + docs + LESSONS | live evidence | S |

### Dependency graph

```
6-i (config + Capabilities + attachvalidate + body-cap + payloadbuild surface
     + adapter Capabilities() helper + Q9-consolidated startup gate)
      │
      ├──► 6-ii  (chatwoot + fider + linear + zendesk + webhook native upload paths)   ┐
      │                                                                                 │  mutually independent;
      └──► 6-iii (widget capture + redaction modal + attachment strip + DTO wiring)     ┘  parallelizable after 6-i
                   │
                   ▼
            6-iv (live chatwoot smoke + Phase 1/4/5 regressions + docs + LESSONS)
```

---

## 11. Testing strategy

### 11.1 Credit-free unit + integration coverage

Every line of new code has either a Go unit test (httptest mocks) or a Vue unit test (`@vue/test-utils` + Vitest), with zero paid-credit consumption.

#### Go unit tests (6-i + 6-ii)

| Package | Coverage |
|---|---|
| `relay/internal/attachvalidate` | golden PNG/JPEG/WebP header fixtures pass; deliberate mismatch (PNG bytes labeled `image/jpeg`) → `ErrMIMEMismatch`; over-cap single → `ErrAttachmentTooLarge`; aggregate-cap boundary (sum = cap OK, sum > cap fail); `type:"file"` → `ErrAttachmentTypeUnsupported`; declared MIME not in allowlist → `ErrMIMENotAllowed`; bad data: URL header → `ErrBadDataURL`; corrupt base64 → `ErrBadDataURL`; empty data → `ErrBadDataURL`; max-size-bytes boundary per L017 (`<=` vs `<` discipline); empty allowlist → all rejected |
| `relay/internal/config` | YAML defaults applied; `storage.mode` non-"forward" captured by `validateAttachments`; `max_size_bytes > max_total_bytes` captured; consolidated startup gate emits attachment + existing Q9/CIDR/duration problems in ONE log line (combined-misconfig fixture) |
| `relay/internal/server` (submit_test.go) | body cap raises 1MB→14MB when `cfg.Attachments.Enabled=true`, stays 1MB otherwise; over-cap → 413 `request_body_too_large` (NOT 400); `attachvalidate` error mapping → correct HTTP code per sentinel; orchestration order: schema-validate fails → 400 (no attachvalidate call), attachvalidate fails → 413/415 (no Router.Route call), all pass → adapter.Create called once; `initHandler` capabilities intersection: empty → `caps.Attachments=nil`, non-empty → correct list |
| Each adapter | `Capabilities()` returns the expected list (deterministic); `Create_WithAttachments` asserts the native sequence via httptest: chatwoot asserts `attachments[]` in conversation-create body; fider asserts `![alt](data:image/png;base64,...)` substring in description; linear asserts N asset-upload POSTs PRECEDE issueCreate AND issueCreate body references returned asset URLs; zendesk asserts N `/uploads.json` POSTs share one token AND ticket body includes `uploads:[<token>]`; webhook asserts JSON body includes `attachments[]` verbatim |
| `relay/internal/adapter/linear` (new failure paths per L011) | asset upload non-2xx → no issueCreate call, error wraps redacted body BEFORE truncate; asset upload network error → no issueCreate call; asset upload returns success but no URL → error before issueCreate; `KeyNeverLeaks_LongPrefix` pattern replicated for new asset-upload paths |
| `relay/internal/adapter/zendesk` (new failure paths) | uploads.json non-2xx → no ticket create, error wraps **status only** (L005 token-echo risk); uploads.json mid-batch failure → no ticket create; second upload reuses first call's token |
| `relay/internal/payloadbuild` | `req.Attachments` non-empty → `p.Attachments` populated 1:1; runtime schema validation passes on canonical form |

#### TS / Vue unit tests (6-iii)

| File | Coverage |
|---|---|
| `core/src/capture.test.ts` | `setHtml2Canvas(stubFn)` overrides production; `capturePage()` invokes the stub with `document.body`; canvas → blob → data: URL conversion (mocked `FileReader`); rejects on 0×0 canvas |
| `core/src/attachments.test.ts` | add/remove/clear; aggregate-size accounting matches `MaxTotalBytes`; rejects when adding would push aggregate over cap; rejects when single exceeds `MaxSizeBytes`; rejects when MIME not in allowed list |
| `core/src/client.test.ts` | `submit()` includes `attachments[]` when non-empty, omits when empty; `init()` parses `capabilities.attachments` when present and when null |
| `vue/src/components/ScreenshotRedactor.spec.ts` | mount in isolation with stubbed canvas-context; `mousedown→mousemove→mouseup` draws rect; Clear resets; Save emits `save(dataUrl)`; Cancel emits `cancel` and does NOT call `toDataURL`; ESC cancels; focus-trap; rectangles outside bounds clamp |
| `vue/src/components/AttachmentStrip.spec.ts` | one thumbnail per pending; remove emits `remove(index)`; aggregate-size badge correct human-readable; empty state hidden |
| `vue/src/components/IntakeWidget.spec.ts` | Attach button hidden when `caps.attachments=null`; visible when non-null; click Attach → modal opens with captured blob; Save → strip updates; Submit threads attachments through |
| `vue/src/composables/useIntake.spec.ts` | new `pendingAttachments` ref + `attachAndRedact()` + `removeAttachment(i)` + `clearAttachments()`; capabilities-driven `canAttach` computed; submit threads attachments |

#### Self-runnable smokes (no credit)

- `relay/cmd/relay/smoke/drive-attachments.ts` — full /init → /turn → /submit with a golden 1×1 PNG; local-Ollama-or-fake provider + httptest webhook receiver. Asserts: caps.attachments emitted; submit accepts; webhook receiver logs attachment in canonical payload; over-cap → 413; mismatched MIME → 415; type:"file" → 400.
- `relay/cmd/relay/smoke/fixtures/attachments-*.yaml` — enabled, disabled, oversized, mismatched-mime, fider-only fixtures.
- Phase 4+5 smoke drivers re-run unchanged under Phase 6 chain (regression catch).

### 11.2 Maintainer-paused live smoke (6-iv)

ONE pause, mirroring Phase 3:

**Live chatwoot attachment smoke** — uses Phase 3 maintainer creds (`CHATWOOT_TOKEN`, `CHATWOOT_INBOX_ID`, `CHATWOOT_ACCOUNT_ID`) and `examples/vue-anonymous`. Procedure: spin up widget, click Attach, accept default capture, draw one redaction rectangle, Save, Submit. **Expected:** Chatwoot conversation appears in inbox **with the screenshot visible inline as an attachment**.

**No live Linear / Zendesk / Fider smokes in Phase 6.** Linear + Zendesk are covered by full httptest harnesses against documented API shapes; Fider is deployment-variant. Live Linear smoke is a v1 follow-up once a stable workspace is available.

### 11.3 Credit/secret guard

- The chatwoot live smoke is the only API-credential-touching test. PAUSE inserted.
- No new credit-spending LLM calls (smokes run on local-Ollama-or-fake provider Phase 5 introduced).
- `html2canvas` is npm — no API cost, but pin exactly per §6.

---

## 12. Build-fail discipline (extends Phase 5 checklist)

Every silent-failure shape gets a CI gate. Phase 6 additions to `ai/tasks/phase-6/README.md` §6:

- [ ] `go build ./... && go vet ./...` fails in `relay/`. **Fail.**
- [ ] `go test -race ./...` fails. **Fail.**
- [ ] `npm run type-check && npm run build` fails in `core/` or `vue/`. **Fail.**
- [ ] `scripts/verify-contract.sh` regresses. **Fail.**
- [ ] `scripts/check-pins.sh` regresses (must extend for `html2canvas`). **Fail.**
- [ ] `cfg.Attachments.Storage.Mode` set to anything other than `"forward"` → relay starts. **Fail** (consolidated Q9 gate).
- [ ] `cfg.Attachments.MaxSizeBytes > cfg.Attachments.MaxTotalBytes` → relay starts. **Fail** (same gate).
- [ ] `html2canvas` pinned with caret in `core/package.json`. **Fail** (check-pins gate).
- [ ] Request body up to 14 MB with `cfg.Attachments.Enabled=true` returns anything other than 200/4xx (no transport failure). **Fail.**
- [ ] Request body > 14 MB returns anything other than 413 `request_body_too_large`. **Fail.**
- [ ] One 6 MB attachment under `MaxSizeBytes=5MB` returns anything other than 413 `attachment_too_large`. **Fail.**
- [ ] Submit with declared `image/png` but JPEG bytes returns anything other than 415 `attachment_mime_mismatch`. **Fail.**
- [ ] Submit with `attachments[]` non-empty when `cfg.Attachments.Enabled=false` returns anything other than 400 `attachments_disabled`. **Fail.**
- [ ] Submit with `type:"file"` returns anything other than 400 `attachment_type_unsupported`. **Fail.**
- [ ] Phase 1+4+5 regression: anonymous + email + SSO smokes pass unchanged with `cfg.Attachments.Enabled=true` and empty `attachments[]`. **Fail otherwise.**
- [ ] Linear test asserts asset upload PRECEDES issueCreate (order regression catches a refactor that flips them and orphans assets). **Fail otherwise.**
- [ ] Zendesk test asserts uploads endpoint error path does NOT include the response body (token-echo guard, L005). **Fail otherwise.**

---

## 13. Final smoke (mandatory per PHASE_PLANNING §7)

```
1. Q9 startup smoke (no LLM credit; self-runnable):
   For each new misconfig YAML:
     - attachments-bad-storage-mode.yaml: storage.mode: "s3"
     - attachments-cap-inverted.yaml:     max_size_bytes:20000000, max_total_bytes:10000000
   Start the relay binary; assert exit 1 and stdout contains the "relay: startup config errors"
   log line listing the matching problem text. Combined YAML (all attachment + Phase 5 misconfigs)
   emits one log line listing every problem (operator fixes all in one restart cycle).

2. Caps-discovery smoke (no LLM credit; self-runnable):
   - cfg.Attachments.Enabled=true, all five adapters registered: /init returns
     capabilities.attachments with the intersection list.
   - cfg.Attachments.Enabled=false: /init omits capabilities.attachments entirely.
   - cfg.Attachments.AllowedMIMETypes:[] → /init omits capabilities.attachments.

3. Validation smokes (no LLM credit; self-runnable via drive-attachments.ts):
   - 6 MB attachment with MaxSizeBytes=5MB → 413 attachment_too_large.
   - Two 6 MB attachments → 413 attachments_exceed_total.
   - Declared image/png with JPEG magic bytes → 415 attachment_mime_mismatch.
   - mime_type "image/heic" with cfg.AllowedMIMETypes=[png,jpeg,webp] → 415 attachment_mime_not_allowed.
   - url "not-a-data-url" → 400 attachment_malformed.
   - type:"file" with valid PNG → 400 attachment_type_unsupported.
   - cfg.Attachments.Enabled=false + non-empty attachments[] → 400 attachments_disabled.

4. Forward smoke (no LLM credit; self-runnable):
   Submit with one valid 1×1 PNG, route to webhook adapter on local httptest server.
   Webhook receiver logs the POST body; assert attachments[0].url is the original
   data: URL verbatim AND attachments[0].mime_type == "image/png".

5. Adapter native-sequence smokes (no LLM credit; httptest):
   For each of chatwoot/fider/linear/zendesk, drive Create() via httptest and assert
   the documented native sequence (per §11.1 adapter row). Failure-injection: each
   adapter's failure path is exercised; orphan paths verified (linear+zendesk return
   error BEFORE create call, no orphan ticket).

6. Live chatwoot smoke (PAUSE for maintainer; uses Phase 3 chatwoot.cloud creds):
   examples/vue-anonymous spun up. Click Attach → modal opens with captured page →
   draw one redaction rectangle → Save → click Submit. EXPECT: Chatwoot conversation
   appears in the configured inbox WITH the screenshot visible inline as an attachment;
   the redaction rectangle is visible in the saved image. Maintainer grep over the
   relay's live-smoke log for the chatwoot api token returns zero matches (L005 check).

7. Phase 1+4+5 regression:
   drive-auth-email.ts (Phase 4) + drive-auth-sso.ts (Phase 4) + drive-abuse.ts (Phase 5)
   pass unchanged under Phase 6's chain (cfg.Attachments.Enabled=true, no attachments
   in the smoke payloads). Confirms no regression in middleware chain, dispatcher,
   gates, or payloadbuild for non-attachment submissions.

8. Body-cap regression:
   With cfg.Attachments.Enabled=false, a 2 MB submission (no attachments) returns
   413 request_body_too_large (the 1 MB cap is preserved when attachments are off).
```

Smokes 1–5 + 7–8 are self-runnable; smoke 6 (live chatwoot) pauses for explicit maintainer go-ahead per the credit/secret guard.

---

## 14. Inconsistencies to fix in PROJECT.md (deferred, not blocking)

One contradiction noticed during Phase 6 design — fix when convenient, does not block:

- §11 reads "**v0 scope: screenshots only.**" but §9 lists `image/png`, `image/jpeg`, `image/webp` in `allowed_mime_types`. These are consistent if "screenshot" is the `type` and the MIME is the file format — the spec is just ambiguous on whether "screenshot" implies PNG only. Phase 6 interpretation: any of PNG/JPEG/WebP captured by `html2canvas.toBlob('image/png')` (production captures PNG; the allowlist permits JPEG/WebP for any future host-app injection path). Recommend a one-sentence clarification in §11 noting that `type:"screenshot"` accepts any MIME in `cfg.Attachments.allowed_mime_types`.

---

## 15. Next step

Per the brainstorming → writing-plans workflow, the next planning action is to author Phase 6 under `ai/tasks/phase-6/` per [ai/PHASE_PLANNING.md](../../ai/PHASE_PLANNING.md): the README (this spec's index) plus four sub-plans (6-i through 6-iv). Each sub-plan has its own mandatory smoke; the phase README carries the final §13 smoke.

*End of Phase 6 design spec.*
