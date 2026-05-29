# Attachments

Phase 6 ships screenshot attachments end-to-end through the relay's existing `/v1/intake/submit` endpoint. This document describes the operator configuration, the per-adapter behavior, and the widget UI flow.

## Operator configuration

Add an `attachments:` block to your `relay.yaml`:

```yaml
attachments:
  enabled: true                                              # default true
  max_size_bytes: 5242880                                    # default 5 MB (per attachment)
  max_total_bytes: 10485760                                  # default 10 MB (aggregate per request)
  allowed_mime_types: ["image/png", "image/jpeg", "image/webp"]  # default
  storage:
    mode: "forward"                                          # only "" or "forward" supported in v0
```

All fields are optional; defaults apply when omitted. The defaults match the values published in `/v1/intake/init`'s `capabilities.attachments` block, which the widget reads to decide whether to show the Attach button.

### Startup-fatal misconfigurations

The relay's Q9-style consolidated startup gate (`main.go startupProblems`) refuses to start when:

- `attachments.storage.mode` is set to any value other than `""` or `"forward"`. v0 stores nothing locally — every attachment is forwarded to the chosen adapter via that adapter's native upload mechanism. The hook exists so v1's S3-backed mode can be added without a schema bump.
- `attachments.max_size_bytes > attachments.max_total_bytes`. A per-attachment cap larger than the aggregate cap is unreachable — a single attachment would always trip the aggregate cap first. The check forces operators to fix the inconsistency before startup.

Both problems are surfaced in the same `relay: startup config errors` log line as Phase 5's CIDR/duration/CAPTCHA misconfigs — one consolidated line, fix everything in one restart cycle.

### Body-cap behavior

When `attachments.enabled: true`, the `/v1/intake/submit` `MaxBytesReader` cap rises from 1 MB to 14 MB (10 MB aggregate × 1.37 base64 overhead + ~200 KB headroom for the JSON envelope). When `enabled: false`, the cap stays at 1 MB — operators who disabled attachments are not exposed to the wider attack surface.

A body that exceeds the cap returns:

```json
{"error": {"code": "request_body_too_large", "message": "submission body exceeds limit"}}
```

with HTTP 413.

## Validation errors

The relay validates every attachment in two layers. First, JSON-schema rules (existing since Phase 0) confirm the field shape. Second, the new `attachvalidate` package decodes the `data:` URL, magic-byte-matches the bytes against the declared `mime_type`, and enforces the per-attachment + aggregate caps:

| HTTP | Code | When |
|---|---|---|
| 413 | `request_body_too_large` | Request body exceeds `MaxBytesReader` (14 MB / 1 MB). |
| 413 | `attachment_too_large` | One attachment's raw bytes exceed `max_size_bytes`. |
| 413 | `attachments_exceed_total` | Sum of all attachment raw bytes exceeds `max_total_bytes`. |
| 415 | `attachment_mime_not_allowed` | Declared `mime_type` not in the published allowlist. |
| 415 | `attachment_mime_mismatch` | `net/http.DetectContentType` on the bytes returns a MIME different from the declared one. |
| 400 | `attachment_malformed` | `url` is not a `data:` URL, or base64 decode fails. |
| 400 | `attachment_type_unsupported` | `type` is not `"screenshot"` (v0 scope). Schema permits `"file"` for v1+; the validator rejects it in v0. |
| 400 | `attachments_disabled` | Non-empty `attachments[]` but `attachments.enabled: false`. |

No `Retry-After` is set on any of these — they are user-action errors with no time-based remediation (per Phase 5's RFC 9110 stance).

## Per-adapter behavior

Each enabled adapter handles `p.Attachments` inside its existing `Create()` method using that downstream system's native upload mechanism. The frozen `adapter.Adapter` interface is unchanged.

| Adapter | Sequence | Notes |
|---|---|---|
| **webhook** | JSON pass-through. `p.Attachments` is serialized verbatim into the POST body via `json.Marshal(p)`. | Receiver is responsible for handling the `data:` URL. Zero behavior change in `webhook.go` beyond the new `Capabilities()` method. |
| **chatwoot** | Three-call flow when attachments are present: (1) `POST /contacts` (existing Phase 3 contact create), (2) `POST /conversations` with JSON body (existing Phase 3 conversation create — byte-identical to no-attachment path), (3) `POST /conversations/{id}/messages` with `multipart/form-data` carrying `content` + `message_type=outgoing` + `attachments[]` parts. When no attachments are present, the third call is skipped. | Conversation-create MUST stay JSON: Chatwoot's `ConversationsController#create` silently drops `attachments[]` multipart parts (see L020). The image upload happens on the SEPARATE `MessagesController#create` endpoint. Upload failures surface as 502 from `submitHandler`; no orphan-prevention attempt since the conversation already exists. |
| **fider** | `renderBody(p)` appends `\n\n![<label or "screenshot N">](data:image/png;base64,...)` per attachment to the post description. Attachment labels are markdown-escaped (defense-in-depth against label-injection). Markdown rendering can't fail; if a Fider deployment's sanitizer strips data: URLs, the post still carries all conversation text (graceful degradation). | No additional roundtrips. |
| **linear** | For each attachment: `POST /upload/file` (GraphQL `fileUpload` mutation via REST) → upload raw bytes to the returned signed URL → receive asset URL. Then `issueCreate` references the asset URLs via `attachmentLinks`. Upload BEFORE create — failure returns error before `issueCreate`, so no orphan issue. The upload response's `success` field is checked explicitly (L020 contract); a `success:false` response rejects before issueCreate. | N additional roundtrips per request (one per attachment). |
| **zendesk** | For each attachment: `POST /api/v2/uploads.json` (subsequent uploads pass `?token=<first-token>` to share one token). Then `ticket-create` includes `comment.uploads: [<token>]`. Upload BEFORE create — same orphan-prevention as Linear. Upload transport errors are wrapped with `%w` (matches the ticket-POST error path; L005 redact-before-truncate applied). | N additional roundtrips per request. Zendesk garbage-collects unattached uploads after 3 days. |

If routing picks an adapter that doesn't accept a given MIME type, the adapter silently drops the attachment with a `slog.Warn` (graceful per-adapter pattern). Operators wanting strict guarantees should configure `allowed_mime_types` to the intersection by hand.

## Capabilities discovery

`/v1/intake/init` returns the relay's published attachment capabilities:

```json
{
  "session_id": "...",
  "capabilities": {
    "auth_modes": ["anonymous"],
    "streaming": true,
    "attachments": {
      "max_size_bytes": 5242880,
      "max_total_bytes": 10485760,
      "allowed_mime_types": ["image/png", "image/jpeg", "image/webp"]
    }
  }
}
```

The `attachments` block is the **union** of every enabled adapter's `Capabilities().AcceptedMIMETypes`, **intersected** with `cfg.attachments.allowed_mime_types`. When the intersection is empty (or `cfg.attachments.enabled: false`), the block is omitted entirely (`omitempty` on the pointer). The widget reads this and hides the Attach button when the block is absent.

## Widget UI flow

The Vue 3 widget (`vue/src/components/IntakeWidget.vue`) ships these new components:

- **`ScreenshotRedactor.vue`** — Full-screen modal overlay opened by clicking Attach. The current page is captured via `html2canvas` (DI-injected through `core/src/capture.ts setHtml2Canvas(fn)` for SSR-safety + testability — see L021), and rendered to a `<canvas>` element. The user draws solid-fill rectangles with the mouse to redact sensitive regions; Save commits the canvas → base64 PNG into the pending attachments list, Cancel discards.
- **`AttachmentStrip.vue`** — Thumbnail strip showing the pending attachments with per-thumb remove buttons and an aggregate-size badge. The Submit button stays enabled when the aggregate is under cap.

The user flow:

1. Widget mounts → calls `/init` → reads `capabilities.attachments`.
2. If the block is null, the Attach button is hidden entirely. Otherwise it is visible.
3. User clicks Attach → `html2canvas(document.body)` captures the host page → `ScreenshotRedactor` modal opens with the canvas.
4. User draws zero or more rectangles → clicks Save → canvas is converted to a base64 PNG and added to the pending strip.
5. User clicks Submit → `IntakeClient.submit()` includes the `attachments[]` array in the POST body; the relay validates, dispatches to one adapter, and the adapter forwards via its native sequence.

Errors mapped to user-readable strings (see `useIntake.ts`):

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

The widget never shows raw relay error messages for attachment paths — only the curated user-readable strings above.

## Cross-references

- Validator: `relay/internal/attachvalidate/` (6-i)
- Adapter `Capabilities()` helpers: `relay/internal/adapter/capabilities.go` + each adapter's `<name>.go` (6-i + 6-ii)
- Widget capture: `core/src/capture.ts` (6-iii)
- Widget pending-state + size accounting: `core/src/attachments.ts` (6-iii)
- Redaction modal: `vue/src/components/ScreenshotRedactor.vue` (6-iii)
- Attachment strip: `vue/src/components/AttachmentStrip.vue` (6-iii)
- Design spec: `docs/specs/2026-05-28-phase-6-attachments-design.md`
- Phase README: `ai/tasks/phase-6/README.md`
- Lessons: `ai/LESSONS.md` L020 (chatwoot multipart drop), L021 (html2canvas SSR-safety DI), L022 (consolidate startup-gate problems before exit)
