# 6-ii Per-Adapter Native Attachment Forwarding — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After 6-i locked the wire seam (`SubmitRequest.Attachments`, `attachvalidate.ValidateAll/DecodeOne`, `adapter.CapableAdapter`, body-cap raise, Q9 startup gate), 6-ii teaches each of the five adapters to **actually forward** `p.Attachments` to its downstream system via that system's native mechanism. The `adapter.Adapter` interface stays FROZEN — each adapter consumes `p.Attachments` internally inside its existing `Create()`. Webhook passes the data: URLs through verbatim in its existing JSON body. Fider appends markdown image references to the post description (no new HTTP call). Chatwoot switches its conversation-create body from `application/json` to `multipart/form-data` only when attachments are present. Linear and Zendesk both follow the upload-before-create pattern (L011 orphan prevention): Linear POSTs each attachment to the legacy `https://uploads.linear.app/api/file/upload` multipart endpoint and threads the returned URLs into `issueCreate`'s `attachmentLinks` input; Zendesk chains POSTs to `/api/v2/uploads.json?filename=…[&token=…]` and attaches the shared token via `ticket.comment.uploads`. After this sub-plan: each adapter's `Create_WithAttachments` httptest smoke proves the documented native sequence; the existing no-attachments path is unchanged (L015 regression); the api-key / Authorization-header never leaks in any new error path (L005 + L011).

**Architecture:** Each adapter reads `p.Attachments` inside its existing `Create()`. Adapters that need raw bytes call `attachvalidate.DecodeOne(att) ([]byte, string, error)` (frozen by 6-i) to decode the data: URL back to `(raw, detectedMime)` — the validator already ran in `submitHandler` so decode failure here is the "shouldn't happen" branch but is surfaced as an error rather than panicked. Webhook + Fider do NOT decode (Webhook serializes the data: URL verbatim; Fider keeps it as a markdown `data:` URL in the description). Chatwoot decodes for the multipart body. Linear decodes per attachment, POSTs each to the uploads endpoint, then issueCreate references the returned asset URLs in `attachmentLinks`. Zendesk decodes per attachment, chains uploads.json POSTs sharing one token, then ticket-create's `comment.uploads:[<token>]` attaches them.

**Tech Stack:** Go 1.23.2 stdlib only (`net/http`, `encoding/json`, `mime/multipart`, `net/url`, `bytes`, `io`, `fmt`, `strconv`, `strings`, `time`). No new modules. `go mod tidy` must remain a no-op.

---

## Design References

- README §2 ADR row "No interface change to `adapter.Adapter`" — the per-adapter `Create()` consumes `p.Attachments` internally; the frozen interface is untouched.
- README §7 step 5 — the mandatory per-adapter `Create_WithAttachments` httptest smoke shape and per-adapter assertions.
- README §8.5 — `attachvalidate.DecodeOne` signature (frozen here in 6-i).
- README §8.6 — `adapter.CapableAdapter.Capabilities()` (frozen by 6-i; each adapter's `Capabilities()` method may already be in place — verify in Task 0).
- README §8.8 — endpoint contract envelope (no new error codes here; all attachment-validation errors fire in `submitHandler` BEFORE `Create()` runs).
- README §6 build-fail checklist rows — "Linear test asserts asset upload PRECEDES issueCreate" (L011) and "Zendesk uploads.json error path does NOT include the response body" (L005).
- Design spec §7.2 per-adapter handling matrix — the canonical native sequence per adapter, copied verbatim into the matrix below.
- Design spec §8.2 per-adapter `Create()` error wrapping — Linear: `linear: asset upload %d/%d returned %d: <redacted>`; Zendesk: `zendesk: upload %d/%d returned %d` (status ONLY, no body).
- LESSONS: L005 (redact-before-truncate; zendesk auth-header echo risk), L011 (linear orphan prevention; redact-before-truncate ordering), L015 (every adapter test must also exercise the no-attachments regression path).
- Reference: `relay/internal/adapter/webhook/webhook.go:102-129` (the simplest `Create()`; baseline for the webhook task).
- Reference: `relay/internal/adapter/chatwoot/chatwoot.go:228-294` (existing two-call flow; this task swaps the second call's body to multipart when attachments present).
- Reference: `relay/internal/adapter/fider/fider.go:170-181` (existing `renderBody`; this task extends it with markdown image refs).
- Reference: `relay/internal/adapter/linear/linear.go:211-274` (existing `Create`; this task inserts an upload loop BEFORE the issueCreate POST and adds `attachmentLinks` to the input).
- Reference: `relay/internal/adapter/zendesk/zendesk.go:136-187` (existing `Create`; this task inserts an upload loop BEFORE the ticket POST and adds `comment.uploads`).
- Reference: `relay/internal/adapter/linear/linear_test.go:298-339` (`KeyNeverLeaks_LongPrefix` pattern — replicated for the new asset-upload error paths).
- Reference: `relay/internal/adapter/truncate.go` — the rune-aware `adapter.Truncate(s, max)` helper.

---

## Per-adapter handling matrix (load-bearing — do not deviate)

| Adapter | Decodes raw bytes? | Native sequence | Extra roundtrips | Failure mode |
|---|---|---|---|---|
| **webhook** | No | `json.Marshal(p)` already serializes `p.Attachments` (data: URLs verbatim). No behavior change in `Create()`. | 0 | Pass-through; receiver's responsibility. |
| **fider** | No | `renderBody(p)` appends `\n\n![<label>](data:…;base64,…)` per attachment; one POST as before. | 0 additional | Markdown can't fail; only the existing post-create call can. |
| **chatwoot** | Yes | When `len(p.Attachments) > 0`, the conversation-create call switches to `multipart/form-data` with fields `inbox_id`, `source_id`, `contact_id`, `message[content]`, and one `attachments[]` part per attachment (filename + Content-Type + raw bytes). When empty, the existing JSON body is unchanged (L015 regression). | 0 additional | Single-transaction; existing 4xx/5xx error wrap covers attachment-inline failures. |
| **zendesk** | Yes | For each attachment: POST raw bytes to `{baseURL}/api/v2/uploads.json?filename=<url-escaped>` (subsequent calls add `&token=<first-token>` to share one upload token). All uploads run BEFORE the ticket POST. Then `ticket.comment.uploads:[<token>]` attaches them. | N (one per attachment) before ticket POST | Upload non-2xx → return error BEFORE ticket POST (no orphan). Error wrap is **status only** per L005 — body OMITTED (uploads endpoint may echo Authorization back). |
| **linear** | Yes | For each attachment: POST multipart to `https://uploads.linear.app/api/file/upload` with the bytes; capture `url` from `{success, uploadFile:{url}}`. All uploads run BEFORE the issueCreate mutation. Then issueCreate's `attachmentLinks:[{url,title}]` references them. | N (one per attachment) before issueCreate | Upload non-2xx, network error, or missing url in response → return error BEFORE issueCreate (no orphan). Error wrap redacts api_key BEFORE `Truncate()` per L011. |

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/adapter/webhook/webhook.go` | Verify (Task 0/1) | Confirm `Capabilities()` already present from 6-i; no behavior change in `Create()`. |
| `relay/internal/adapter/webhook/webhook_test.go` | Modify | Add `TestWebhookCreate_AttachmentsPassthrough`. |
| `relay/internal/adapter/fider/fider.go` | Modify | Extend `renderBody` with markdown image refs per attachment. |
| `relay/internal/adapter/fider/fider_test.go` | Modify | Add `TestFiderCreate_AttachmentsAppendsMarkdownImages` + no-attachments regression. |
| `relay/internal/adapter/chatwoot/chatwoot.go` | Modify | In `Create()`, swap conversation-create body to multipart when attachments present. |
| `relay/internal/adapter/chatwoot/chatwoot_test.go` | Modify | Add `TestChatwootCreate_AttachmentsMultipart` + no-attachments JSON-path regression. |
| `relay/internal/adapter/zendesk/zendesk.go` | Modify | Add upload loop before ticket POST; extend `ticketComment` with `Uploads []string`. |
| `relay/internal/adapter/zendesk/zendesk_test.go` | Modify | Add 5 new tests: happy path, first-upload-fails, mid-batch fails, body-not-in-error, token-never-leaks. |
| `relay/internal/adapter/linear/linear.go` | Modify | Add upload loop before issueCreate; pass `attachmentLinks` in the mutation input. |
| `relay/internal/adapter/linear/linear_test.go` | Modify | Add 5 new tests: happy path, first-upload-fails, mid-batch fails, missing-url, key-never-leaks-LongPrefix. |

No new files; no new packages; no changes to `adapter.Adapter` interface, payload generated types, or schema.

---

## Tasks

### Task 0: Verify 6-i prerequisites are in place

**Files:** None (verification only).

- [ ] **Step 1: Confirm `attachvalidate.DecodeOne` is exported**

Run: `cd relay && grep -n "func DecodeOne" internal/attachvalidate/attachvalidate.go && cd ..`
Expected: one line printed showing the function signature `func DecodeOne(att payload.Attachment) (raw []byte, mime string, err error)`. If missing, 6-i is incomplete — STOP and finish 6-i before continuing.

- [ ] **Step 2: Confirm `adapter.Capabilities` + `adapter.CapableAdapter` exist**

Run: `cd relay && grep -n "type Capabilities struct\|type CapableAdapter interface" internal/adapter/capabilities.go && cd ..`
Expected: both type declarations present. If missing, 6-i is incomplete — STOP.

- [ ] **Step 3: Confirm every adapter already implements `Capabilities()`**

Run: `cd relay && grep -rn "func (a \*Adapter) Capabilities()" internal/adapter/ && cd ..`
Expected: five lines, one per adapter package (webhook, chatwoot, fider, linear, zendesk). If any are missing, add the method per the trivial pattern below (this is a 6-i carry-over — log it in the commit message of the affected task):

```go
// Capabilities reports the MIME types this adapter accepts for attachments.
// Phase 6 (6-i): all five adapters advertise the same v0 image set.
func (a *Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
}
```

- [ ] **Step 4: Confirm baseline test suite is green**

Run: `cd relay && go test -race ./... && cd ..`
Expected: all packages pass under `-race`. If any failures exist BEFORE Task 1, fix them first — do not start 6-ii on a red baseline.

- [ ] **Step 5: No commit for this task** — verification only.

---

### Task 1: Webhook — assert attachments pass through the existing JSON body

**Files:** Modify `relay/internal/adapter/webhook/webhook_test.go`.

Webhook needs no production change: `json.Marshal(p)` (line 103 of `webhook.go`) already serializes `p.Attachments` with the data: URLs verbatim because `payload.IntakePayload.Attachments` uses `json:"attachments,omitempty"`. This task adds the regression test that proves it.

- [ ] **Step 1: Write the failing test**

Append to `relay/internal/adapter/webhook/webhook_test.go`:

```go
// TestWebhookCreate_AttachmentsPassthrough asserts that p.Attachments are
// serialized in the JSON body verbatim (the data: URLs survive intact).
// This is the Phase 6 6-ii contract for the webhook adapter: no native upload,
// just JSON pass-through, so receivers can decode the data: URLs themselves.
func TestWebhookCreate_AttachmentsPassthrough(t *testing.T) {
	const dataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		received, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{"url": srv.URL}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	p := minimalPayload()
	label := "screenshot 1"
	p.Attachments = []payload.Attachment{{
		Type:      payload.AttachmentTypeScreenshot,
		MimeType:  "image/png",
		Url:       dataURL,
		SizeBytes: 70,
		Label:     &label,
	}}
	if _, err := a.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var parsed struct {
		Attachments []struct {
			Type     string `json:"type"`
			MimeType string `json:"mime_type"`
			URL      string `json:"url"`
			Label    string `json:"label"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(received, &parsed); err != nil {
		t.Fatalf("receiver body not valid JSON: %v\nbody: %s", err, received)
	}
	if len(parsed.Attachments) != 1 {
		t.Fatalf("attachments len = %d; want 1", len(parsed.Attachments))
	}
	got := parsed.Attachments[0]
	if got.Type != "screenshot" {
		t.Errorf("attachments[0].type = %q; want screenshot", got.Type)
	}
	if got.MimeType != "image/png" {
		t.Errorf("attachments[0].mime_type = %q; want image/png", got.MimeType)
	}
	if got.URL != dataURL {
		t.Errorf("attachments[0].url not preserved verbatim:\n got: %q\nwant: %q", got.URL, dataURL)
	}
	if got.Label != "screenshot 1" {
		t.Errorf("attachments[0].label = %q; want screenshot 1", got.Label)
	}
}

// TestWebhookCreate_NoAttachmentsOmitsField asserts the no-attachments path
// (L015 regression) — when p.Attachments is empty, the marshaled JSON must
// omit the field entirely (omitempty on the generated type).
func TestWebhookCreate_NoAttachmentsOmitsField(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{"url": srv.URL}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(received, &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, has := parsed["attachments"]; has {
		t.Errorf("attachments key present in JSON when empty; want omitted (omitempty)")
	}
}
```

- [ ] **Step 2: Run to confirm they pass**

Run: `cd relay && go test -race ./internal/adapter/webhook/... -v && cd ..`
Expected: both new tests pass (`json.Marshal` already does the right thing). If `TestWebhookCreate_AttachmentsPassthrough` fails, investigate — the generated `payload.Attachment` field tags may have changed.

- [ ] **Step 3: Commit**

```bash
git add relay/internal/adapter/webhook/webhook_test.go
git commit -m "$(cat <<'EOF'
test(6-ii): webhook attachments pass-through assertion + L015 omit regression

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Fider — append markdown image refs per attachment

**Files:** Modify `relay/internal/adapter/fider/fider.go`, `relay/internal/adapter/fider/fider_test.go`.

Fider has no native upload endpoint usable from a server-side API; the design (spec §3 Q-B) accepts inline data: URLs in the post markdown. The post description gains one `![label](data:…;base64,…)` line per attachment, separated by a blank line. If the Fider deployment's markdown sanitizer strips data: URLs, the post still has all conversation text (graceful degradation).

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/adapter/fider/fider_test.go`:

```go
// TestFiderCreate_AttachmentsAppendsMarkdownImages asserts each attachment is
// appended as a markdown image reference using its data: URL verbatim, with
// the label as alt text. The order matches p.Attachments order, separated by
// a blank line.
func TestFiderCreate_AttachmentsAppendsMarkdownImages(t *testing.T) {
	const url1 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	const url2 = "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAA=="

	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"number":1}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	p := minimalPayload()
	label1 := "before save"
	// Second attachment has no label → renderer falls back to "screenshot 2".
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: url1, SizeBytes: 70, Label: &label1},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/jpeg", Url: url2, SizeBytes: 32},
	}
	if _, err := a.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var sent struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	want1 := "![before save](" + url1 + ")"
	want2 := "![screenshot 2](" + url2 + ")"
	if !strings.Contains(sent.Description, want1) {
		t.Errorf("description missing first attachment ref %q\ngot: %q", want1, sent.Description)
	}
	if !strings.Contains(sent.Description, want2) {
		t.Errorf("description missing second attachment ref %q\ngot: %q", want2, sent.Description)
	}
	// Order regression: first must appear before second.
	if i1, i2 := strings.Index(sent.Description, want1), strings.Index(sent.Description, want2); i1 < 0 || i2 < 0 || i1 > i2 {
		t.Errorf("attachment order not preserved (i1=%d, i2=%d)", i1, i2)
	}
	// Conversation text must still be present (graceful degradation contract).
	if !strings.Contains(sent.Description, "Export button is unresponsive on the reports page.") {
		t.Errorf("description lost summary after attachment append\ngot: %q", sent.Description)
	}
}

// TestFiderCreate_NoAttachmentsRegression asserts the no-attachments path is
// byte-identical to the existing 5-i behavior (L015 regression — no stray
// trailing newlines, no spurious markdown).
func TestFiderCreate_NoAttachmentsRegression(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"number":1}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var sent struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Contains(sent.Description, "![") {
		t.Errorf("description contains markdown image ref when no attachments present; got: %q", sent.Description)
	}
	if strings.Contains(sent.Description, "data:") {
		t.Errorf("description contains data: URL when no attachments present; got: %q", sent.Description)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/adapter/fider/ -run TestFiderCreate_Attachments -v && cd ..`
Expected: FAIL — current `renderBody` does not append attachment markdown.

- [ ] **Step 3: Extend `renderBody` in `fider.go`**

Replace the existing `renderBody` function (currently `fider.go` lines 170-181) with:

```go
// renderBody builds the post description: the summary, a blank line, then each
// message as "<Role>: <Content>". When p.Attachments is non-empty, each
// attachment is appended as a markdown image reference using the original
// data: URL verbatim — Label as alt text, falling back to "screenshot N"
// (1-indexed) when Label is nil or empty. The Fider markdown renderer
// inlines data: URLs as <img src=…>; if a deployment's sanitizer strips
// them, the post still has the full conversation text (graceful degradation
// per design spec §3 Q-B).
func renderBody(p *payload.IntakePayload) string {
	var b strings.Builder
	b.WriteString(p.Conversation.Summary)
	b.WriteString("\n\n")
	for _, m := range p.Conversation.Messages {
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	for i, att := range p.Attachments {
		label := ""
		if att.Label != nil {
			label = *att.Label
		}
		if label == "" {
			label = fmt.Sprintf("screenshot %d", i+1)
		}
		b.WriteString("\n![")
		b.WriteString(label)
		b.WriteString("](")
		b.WriteString(att.Url)
		b.WriteString(")\n")
	}
	return b.String()
}
```

No import changes required — `fmt` and `strings` are already imported in `fider.go`.

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test -race ./internal/adapter/fider/... -v && cd ..`
Expected: all existing fider tests + the 2 new tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/adapter/fider/fider.go relay/internal/adapter/fider/fider_test.go
git commit -m "$(cat <<'EOF'
feat(6-ii): fider — append markdown image refs per attachment in renderBody

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Chatwoot — multipart conversation-create when attachments present

**Files:** Modify `relay/internal/adapter/chatwoot/chatwoot.go`, `relay/internal/adapter/chatwoot/chatwoot_test.go`.

Chatwoot's `POST /api/v1/accounts/{id}/conversations` accepts either inline JSON or `multipart/form-data`. The simplest documented path for inline attachments is multipart: fields `inbox_id`, `source_id`, `contact_id`, `message[content]`, and one `attachments[]` part per attachment with the decoded raw bytes + filename + Content-Type. When `len(p.Attachments) == 0` we keep the existing JSON path (L015 regression — the existing happy-path test stays green untouched). The `createContact` call is unchanged.

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/adapter/chatwoot/chatwoot_test.go`:

```go
// goldenPNGBytes is a 1×1 transparent PNG (smallest valid PNG byte sequence)
// used by attachment tests so DecodeOne's magic-byte path agrees with the
// declared mime_type.
var goldenPNGBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

// goldenPNGDataURL is the base64-encoded data: URL form of goldenPNGBytes.
var goldenPNGDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(goldenPNGBytes)

// TestChatwootCreate_AttachmentsMultipart asserts the conversation-create call
// switches to multipart/form-data when attachments are present and the body
// contains the expected form fields + one attachments[] part per attachment
// carrying the decoded raw bytes with the correct Content-Type/filename.
func TestChatwootCreate_AttachmentsMultipart(t *testing.T) {
	var convCT string
	var convFields map[string]string
	type uploadedPart struct {
		filename string
		ctype    string
		bytes    []byte
	}
	var convAttachments []uploadedPart

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"src-uuid-mp"}}}`))

		case "/api/v1/accounts/1/conversations":
			convCT = r.Header.Get("Content-Type")
			convFields = map[string]string{}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			for k, vs := range r.MultipartForm.Value {
				if len(vs) > 0 {
					convFields[k] = vs[0]
				}
			}
			for _, fh := range r.MultipartForm.File["attachments[]"] {
				f, err := fh.Open()
				if err != nil {
					t.Fatalf("open part: %v", err)
				}
				b, err := io.ReadAll(f)
				_ = f.Close()
				if err != nil {
					t.Fatalf("read part: %v", err)
				}
				convAttachments = append(convAttachments, uploadedPart{
					filename: fh.Filename,
					ctype:    fh.Header.Get("Content-Type"),
					bytes:    b,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":999}`))

		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	label := "before-save.png"
	p.Attachments = []payload.Attachment{{
		Type:      payload.AttachmentTypeScreenshot,
		MimeType:  "image/png",
		Url:       goldenPNGDataURL,
		SizeBytes: len(goldenPNGBytes),
		Label:     &label,
	}}

	a := configure(t, srv.URL)
	result, err := a.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !strings.HasPrefix(convCT, "multipart/form-data") {
		t.Errorf("conversation Content-Type = %q; want multipart/form-data; …", convCT)
	}
	if convFields["inbox_id"] != "3" {
		t.Errorf("multipart inbox_id = %q; want 3", convFields["inbox_id"])
	}
	if convFields["source_id"] != "src-uuid-mp" {
		t.Errorf("multipart source_id = %q; want src-uuid-mp", convFields["source_id"])
	}
	if convFields["contact_id"] != "50" {
		t.Errorf("multipart contact_id = %q; want 50", convFields["contact_id"])
	}
	if got := convFields["message[content]"]; !strings.Contains(got, "Save button does nothing") {
		t.Errorf("multipart message[content] missing title; got: %q", got)
	}
	if len(convAttachments) != 1 {
		t.Fatalf("attachments[] parts = %d; want 1", len(convAttachments))
	}
	part := convAttachments[0]
	if part.filename != "before-save.png" {
		t.Errorf("part.filename = %q; want before-save.png", part.filename)
	}
	if part.ctype != "image/png" {
		t.Errorf("part Content-Type = %q; want image/png", part.ctype)
	}
	if !bytes.Equal(part.bytes, goldenPNGBytes) {
		t.Errorf("part bytes mismatch (len=%d, want=%d)", len(part.bytes), len(goldenPNGBytes))
	}
	if result.ExternalID != "999" {
		t.Errorf("ExternalID = %q; want 999", result.ExternalID)
	}
}

// TestChatwootCreate_NoAttachmentsJSONPathUnchanged asserts that when
// p.Attachments is empty the conversation-create body stays application/json
// (L015 regression — existing JSON path must not flip to multipart by accident).
func TestChatwootCreate_NoAttachmentsJSONPathUnchanged(t *testing.T) {
	var convCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"src-uuid"}}}`))
		case "/api/v1/accounts/1/conversations":
			convCT = r.Header.Get("Content-Type")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if convCT != "application/json" {
		t.Errorf("no-attachments path Content-Type = %q; want application/json", convCT)
	}
}

// TestChatwootCreate_AttachmentsLabelFallback asserts a nil/empty Label
// produces the "screenshot N" (1-indexed) filename per the design matrix.
func TestChatwootCreate_AttachmentsLabelFallback(t *testing.T) {
	var filenames []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"s"}}}`))
		case "/api/v1/accounts/1/conversations":
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			for _, fh := range r.MultipartForm.File["attachments[]"] {
				filenames = append(filenames, fh.Filename)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configure(t, srv.URL)
	if _, err := a.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(filenames) != 2 {
		t.Fatalf("filenames len = %d; want 2", len(filenames))
	}
	if filenames[0] != "screenshot 1" {
		t.Errorf("filenames[0] = %q; want screenshot 1", filenames[0])
	}
	if filenames[1] != "screenshot 2" {
		t.Errorf("filenames[1] = %q; want screenshot 2", filenames[1])
	}
}
```

Add the imports at the top of `chatwoot_test.go` if not already present: `"bytes"`, `"encoding/base64"`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/adapter/chatwoot/ -run TestChatwootCreate_Attachments -v && cd ..`
Expected: FAIL — current `Create()` always uses application/json.

- [ ] **Step 3: Modify `chatwoot.go` to support the multipart path**

Add imports to `chatwoot.go` (top of the existing `import` block): `"mime/multipart"`, `"strconv"`, `"intake/internal/attachvalidate"`.

In `chatwoot.go`, REPLACE the `Create` function body (lines 228-278) with the version below. The contract: `createContact` is unchanged; the conversation step branches on `len(p.Attachments)`.

```go
// Create executes the two-call flow: first creates a contact tied to the inbox
// to obtain a valid source_id, then creates the conversation using that
// source_id and the contact id.
//
// Phase 6 (6-ii): when p.Attachments is non-empty, the conversation-create
// body switches from application/json to multipart/form-data with one
// attachments[] part per attachment carrying the decoded raw bytes. When
// empty, the existing application/json body is used unchanged (L015
// regression — the JSON path is byte-identical to the Phase 3 behavior).
//
// Non-2xx at either step returns an error including the truncated response
// body but never the token.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	// Step 1: create contact and obtain contact_inbox source_id. Unchanged.
	contactID, sourceID, err := a.createContact(ctx, p)
	if err != nil {
		return nil, err
	}

	// Step 2: create conversation. Two body shapes: JSON (no attachments) or
	// multipart/form-data (with attachments). The endpoint path is identical.
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations", a.baseURL, a.accountID)

	var (
		req *http.Request
	)
	if len(p.Attachments) == 0 {
		reqBody := conversationRequest{
			InboxID:   a.inboxID,
			SourceID:  sourceID,
			ContactID: contactID,
			Message:   conversationMsg{Content: renderBody(p)},
		}
		body, mErr := json.Marshal(reqBody)
		if mErr != nil {
			return nil, fmt.Errorf("chatwoot: marshal conversation request: %w", mErr)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("chatwoot: build conversation request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		body, ctype, mpErr := buildConversationMultipart(p, a.inboxID, sourceID, contactID)
		if mpErr != nil {
			return nil, mpErr
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, body)
		if err != nil {
			return nil, fmt.Errorf("chatwoot: build conversation request: %w", err)
		}
		req.Header.Set("Content-Type", ctype)
	}
	req.Header.Set("api_access_token", a.apiToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: conversation http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("chatwoot: create conversation returned %d: %s",
			resp.StatusCode, adapter.Truncate(string(respBody), 200))
	}

	id, err := extractConversationID(respBody)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: parse conversation response id: %w", err)
	}

	return &adapter.CreateResult{
		ExternalID:  id,
		ExternalURL: fmt.Sprintf("%s/app/accounts/%d/conversations/%s", a.baseURL, a.accountID, id),
		AdapterName: "chatwoot",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// buildConversationMultipart constructs the multipart/form-data body for the
// conversation-create call when attachments are present. Returns the body,
// the Content-Type header value (including the multipart boundary), and any
// error encountered while decoding an attachment's data: URL or writing the
// multipart parts.
//
// Per design spec §7.2: fields inbox_id, source_id, contact_id, message[content];
// one attachments[] part per p.Attachments entry. Filename falls back to
// "screenshot N" (1-indexed) when Label is nil/empty.
func buildConversationMultipart(p *payload.IntakePayload, inboxID int, sourceID string, contactID json.Number) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("inbox_id", strconv.Itoa(inboxID)); err != nil {
		return nil, "", fmt.Errorf("chatwoot: multipart field inbox_id: %w", err)
	}
	if err := w.WriteField("source_id", sourceID); err != nil {
		return nil, "", fmt.Errorf("chatwoot: multipart field source_id: %w", err)
	}
	if err := w.WriteField("contact_id", contactID.String()); err != nil {
		return nil, "", fmt.Errorf("chatwoot: multipart field contact_id: %w", err)
	}
	if err := w.WriteField("message[content]", renderBody(p)); err != nil {
		return nil, "", fmt.Errorf("chatwoot: multipart field message[content]: %w", err)
	}

	for i, att := range p.Attachments {
		raw, _, err := attachvalidate.DecodeOne(att)
		if err != nil {
			return nil, "", fmt.Errorf("chatwoot: decode attachment %d: %w", i+1, err)
		}
		filename := ""
		if att.Label != nil {
			filename = *att.Label
		}
		if filename == "" {
			filename = fmt.Sprintf("screenshot %d", i+1)
		}
		hdr := make(map[string][]string)
		hdr["Content-Disposition"] = []string{
			fmt.Sprintf(`form-data; name="attachments[]"; filename=%q`, filename),
		}
		hdr["Content-Type"] = []string{att.MimeType}
		part, err := w.CreatePart(hdr)
		if err != nil {
			return nil, "", fmt.Errorf("chatwoot: multipart create part %d: %w", i+1, err)
		}
		if _, err := part.Write(raw); err != nil {
			return nil, "", fmt.Errorf("chatwoot: multipart write part %d: %w", i+1, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("chatwoot: multipart close: %w", err)
	}
	return &buf, w.FormDataContentType(), nil
}
```

Note: the import path `intake/internal/attachvalidate` is the 6-i seam package. `mime/multipart`'s `CreatePart` requires a `textproto.MIMEHeader` — Go's `multipart.Writer` accepts `textproto.MIMEHeader` which is `map[string][]string`; the literal map above satisfies that signature without an explicit `textproto` import.

If `go vet` complains about the missing `textproto` import, change the helper to:

```go
import "net/textproto"

hdr := textproto.MIMEHeader{
    "Content-Disposition": []string{fmt.Sprintf(`form-data; name="attachments[]"; filename=%q`, filename)},
    "Content-Type":        []string{att.MimeType},
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test -race ./internal/adapter/chatwoot/... -v && cd ..`
Expected: all existing chatwoot tests + the 3 new tests pass. The pre-existing `TestChatwootCreate_PostsConversation` and `TestChatwootConfigure_AcceptsFloatIDs` (which post NO attachments) must still pass — proves the JSON path regression.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/adapter/chatwoot/chatwoot.go relay/internal/adapter/chatwoot/chatwoot_test.go
git commit -m "$(cat <<'EOF'
feat(6-ii): chatwoot — multipart conversation-create when attachments present

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Zendesk — chained uploads.json before ticket create

**Files:** Modify `relay/internal/adapter/zendesk/zendesk.go`, `relay/internal/adapter/zendesk/zendesk_test.go`.

Zendesk's uploads endpoint: `POST {baseURL}/api/v2/uploads.json?filename=<urlencoded>` with the raw bytes as the body and `Content-Type: <mime>`. Returns `{ upload: { token, attachment: { id, content_url, ... } } }`. Subsequent uploads chain by passing `?filename=<x>&token=<first-token>` so all uploads share ONE token. The token is then attached to the ticket create body via `ticket.comment.uploads = [<token>]`. Failure of any upload returns an error BEFORE the ticket POST (L011 orphan prevention). The error wrap is **status code only** (no body) because the uploads endpoint may echo Authorization back (L005).

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/adapter/zendesk/zendesk_test.go`:

```go
// goldenPNGBytes is the smallest valid 1×1 PNG used for upload-flow tests.
var goldenPNGBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

var goldenPNGDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(goldenPNGBytes)

// TestZendeskCreate_AttachmentsChainedUploadsThenTicket asserts:
//   1. N uploads.json POSTs precede the ticket POST.
//   2. Each upload carries Content-Type=<mime> and the raw bytes as the body.
//   3. The first upload's response token is reused as ?token=<...> on subsequent uploads.
//   4. The ticket POST body's comment.uploads contains the shared token.
func TestZendeskCreate_AttachmentsChainedUploadsThenTicket(t *testing.T) {
	const sharedToken = "upl_TOKEN_abc123"
	var (
		uploadCallCount int
		uploadOrder     []string // captures ?filename + ?token per call
		uploadCTs       []string
		uploadBodies    [][]byte
		ticketBody      map[string]any
		ticketSeen      bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/uploads.json":
			if ticketSeen {
				t.Errorf("uploads called AFTER ticket POST — order regression (L011)")
			}
			uploadCallCount++
			uploadCTs = append(uploadCTs, r.Header.Get("Content-Type"))
			b, _ := io.ReadAll(r.Body)
			uploadBodies = append(uploadBodies, b)
			uploadOrder = append(uploadOrder, r.URL.RawQuery)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"upload":{"token":"` + sharedToken + `","attachment":{"id":1,"content_url":"https://example.zendesk.com/attachments/1"}}}`))

		case "/api/v2/tickets.json":
			ticketSeen = true
			b, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(b, &ticketBody); err != nil {
				t.Fatalf("ticket body not JSON: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ticket":{"id":777,"url":"https://example.zendesk.com/api/v2/tickets/777.json"}}`))

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	label1 := "before.png"
	label2 := "after.png"
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes), Label: &label1},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes), Label: &label2},
	}

	a := configured(t, srv.URL)
	result, err := a.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if uploadCallCount != 2 {
		t.Fatalf("uploadCallCount = %d; want 2", uploadCallCount)
	}
	// First call: ?filename=before.png (no token).
	if !strings.Contains(uploadOrder[0], "filename=before.png") {
		t.Errorf("first upload query missing filename=before.png: %q", uploadOrder[0])
	}
	if strings.Contains(uploadOrder[0], "token=") {
		t.Errorf("first upload query must NOT contain token=…: %q", uploadOrder[0])
	}
	// Second call: ?filename=after.png&token=<shared>.
	if !strings.Contains(uploadOrder[1], "filename=after.png") {
		t.Errorf("second upload query missing filename=after.png: %q", uploadOrder[1])
	}
	if !strings.Contains(uploadOrder[1], "token="+sharedToken) {
		t.Errorf("second upload query missing token=%s: %q", sharedToken, uploadOrder[1])
	}
	for i, ct := range uploadCTs {
		if ct != "image/png" {
			t.Errorf("upload[%d] Content-Type = %q; want image/png", i, ct)
		}
	}
	for i, body := range uploadBodies {
		if !bytes.Equal(body, goldenPNGBytes) {
			t.Errorf("upload[%d] body mismatch (len=%d, want=%d)", i, len(body), len(goldenPNGBytes))
		}
	}
	// Ticket body must include comment.uploads=[<sharedToken>].
	ticket, _ := ticketBody["ticket"].(map[string]any)
	if ticket == nil {
		t.Fatalf("ticket key missing in body: %v", ticketBody)
	}
	comment, _ := ticket["comment"].(map[string]any)
	if comment == nil {
		t.Fatalf("ticket.comment missing: %v", ticket)
	}
	uploads, _ := comment["uploads"].([]any)
	if len(uploads) != 1 {
		t.Fatalf("comment.uploads len = %d; want 1 (shared token)", len(uploads))
	}
	if uploads[0] != sharedToken {
		t.Errorf("comment.uploads[0] = %v; want %q", uploads[0], sharedToken)
	}
	if result.ExternalID != "777" {
		t.Errorf("ExternalID = %q; want 777", result.ExternalID)
	}
}

// TestZendeskCreate_NoAttachmentsRegression asserts the no-attachments path
// is byte-identical to Phase 3 — no uploads.json calls, no comment.uploads
// field in the ticket body (L015).
func TestZendeskCreate_NoAttachmentsRegression(t *testing.T) {
	var (
		uploadCalled bool
		ticketBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/uploads.json":
			uploadCalled = true
			w.WriteHeader(http.StatusOK)
		case "/api/v2/tickets.json":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &ticketBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ticket":{"id":1,"url":"u"}}`))
		}
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if uploadCalled {
		t.Error("uploads.json called when no attachments present")
	}
	ticket, _ := ticketBody["ticket"].(map[string]any)
	comment, _ := ticket["comment"].(map[string]any)
	if _, has := comment["uploads"]; has {
		t.Errorf("comment.uploads present when no attachments; got: %v", comment["uploads"])
	}
}

// TestZendeskCreate_FirstUploadFails_NoTicketCreate asserts orphan prevention:
// an upload non-2xx returns an error BEFORE the ticket POST (L011).
func TestZendeskCreate_FirstUploadFails_NoTicketCreate(t *testing.T) {
	var ticketSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/uploads.json":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"upstream broken"}`))
		case "/api/v2/tickets.json":
			ticketSeen = true
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on upload 500")
	}
	if ticketSeen {
		t.Errorf("ticket POST happened despite upload failure (L011 regression)")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error must mention status 500; got: %v", err)
	}
	if !strings.Contains(err.Error(), "1/1") {
		t.Errorf("error must include upload index %q; got: %v", "1/1", err)
	}
	// L005: body MUST NOT appear in the error.
	if strings.Contains(err.Error(), "upstream broken") {
		t.Errorf("L005: error must NOT include response body (uploads endpoint may echo Authorization); got: %v", err)
	}
}

// TestZendeskCreate_MidBatchUploadFails_NoTicketCreate asserts that a 2xx
// first upload followed by a 5xx second upload returns an error BEFORE the
// ticket POST.
func TestZendeskCreate_MidBatchUploadFails_NoTicketCreate(t *testing.T) {
	var uploadCount, ticketCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/uploads.json":
			uploadCount++
			if uploadCount == 1 {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"upload":{"token":"t1","attachment":{"id":1}}}`))
				return
			}
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"transient"}`))
		case "/api/v2/tickets.json":
			ticketCount++
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on second upload 502")
	}
	if ticketCount != 0 {
		t.Errorf("ticket POST happened (count=%d) despite mid-batch upload failure", ticketCount)
	}
	if !strings.Contains(err.Error(), "2/2") {
		t.Errorf("error must reference upload index 2/2; got: %v", err)
	}
}

// TestZendeskCreate_UploadErrorOmitsBody_L005Guard asserts the Authorization
// header / token-echo guard: even when the upload response body contains the
// configured token, the error message must NOT contain it.
func TestZendeskCreate_UploadErrorOmitsBody_L005Guard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/uploads.json" {
			w.WriteHeader(http.StatusUnauthorized)
			// Echo the auth header (some Zendesk error paths echo headers in the body).
			_, _ = w.Write([]byte(`{"error":"bad auth: ` + r.Header.Get("Authorization") + `"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("L005: token leaked in error: %v", err)
	}
	if strings.Contains(err.Error(), "Basic ") {
		t.Fatalf("L005: Authorization header echoed in error: %v", err)
	}
	// Sanity: status code IS in the message.
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401; got: %v", err)
	}
}
```

Add the imports at the top of `zendesk_test.go` if not already present: `"bytes"`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/adapter/zendesk/ -run TestZendeskCreate_Attachments -v && cd ..`
Expected: FAIL — current `Create()` does not call uploads.json.

- [ ] **Step 3: Modify `zendesk.go` to add the upload loop**

Add imports to `zendesk.go`: `"net/url"`, `"intake/internal/attachvalidate"`.

REPLACE the `ticketComment` struct and `Create` function (lines 120-187) with:

```go
type ticketComment struct {
	Body    string   `json:"body"`
	Uploads []string `json:"uploads,omitempty"`
}

// Create POSTs a ticket to {baseURL}/api/v2/tickets.json. On 2xx it parses the
// ticket id and returns a CreateResult whose ExternalURL points at the agent
// UI. On non-2xx it returns an error including ONLY the status code (the
// response body is NEVER included — the Authorization header may be echoed
// back, which would leak the base64-encoded credentials, per L005).
//
// Phase 6 (6-ii): when p.Attachments is non-empty, each attachment is POSTed
// to /api/v2/uploads.json BEFORE the ticket POST. The first upload returns a
// token; subsequent uploads pass that token via ?token=<...> so all uploads
// share a single token. The shared token is attached to the ticket via
// ticket.comment.uploads. Any upload failure returns an error immediately;
// the ticket POST is NEVER reached (L011 orphan prevention).
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	// Phase 6: upload attachments before ticket create. token is "" when there
	// are no attachments — in that case the comment.uploads field is omitted
	// entirely (omitempty).
	uploadToken, err := a.uploadAttachments(ctx, p.Attachments)
	if err != nil {
		return nil, err
	}

	comment := ticketComment{Body: renderBody(p)}
	if uploadToken != "" {
		comment.Uploads = []string{uploadToken}
	}

	reqBody := ticketRequest{
		Ticket: ticketBody{
			Subject:  p.Conversation.TitleSuggestion,
			Comment:  comment,
			Priority: mapPriority(p.Conversation.SeverityGuess, a.defaultPriority),
			Tags:     []string(p.Conversation.TagsSuggested),
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("zendesk: marshal ticket: %w", err)
	}

	endpoint := a.baseURL + "/api/v2/tickets.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("zendesk: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", a.authHeader)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zendesk: http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// NEVER include the response body — a misbehaving server may echo back
		// the Authorization header, which would leak the credentials (L005).
		return nil, fmt.Errorf("zendesk: create ticket returned %d", resp.StatusCode)
	}

	var parsed ticketResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("zendesk: parse response: %w", err)
	}
	id := parsed.Ticket.ID.String()
	if id == "" {
		return nil, fmt.Errorf("zendesk: response missing ticket id: %s", adapter.Truncate(string(respBody), 200))
	}

	return &adapter.CreateResult{
		ExternalID:  id,
		ExternalURL: fmt.Sprintf("%s/agent/tickets/%s", a.baseURL, id),
		AdapterName: a.Name(),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// uploadResponse is the parsed shape of /api/v2/uploads.json. We only need
// upload.token; the server may rotate the token across chained uploads (rare
// in practice), so we always overwrite with the latest response.
type uploadResponse struct {
	Upload struct {
		Token string `json:"token"`
	} `json:"upload"`
}

// uploadAttachments POSTs each attachment to /api/v2/uploads.json, chaining
// the token across calls. Returns the FINAL token (which the ticket POST then
// references via comment.uploads). An empty atts slice returns ("", nil) and
// causes Create to skip the comment.uploads field entirely.
//
// On any upload failure (non-2xx, network error, missing token in response),
// returns an error immediately. The error includes the upload index (1-based)
// and the total count for operator diagnostics. The response BODY is NEVER
// included in the error per L005 (Authorization header echo risk).
func (a *Adapter) uploadAttachments(ctx context.Context, atts []payload.Attachment) (string, error) {
	if len(atts) == 0 {
		return "", nil
	}
	token := ""
	for i, att := range atts {
		raw, _, err := attachvalidate.DecodeOne(att)
		if err != nil {
			return "", fmt.Errorf("zendesk: decode attachment %d/%d: %w", i+1, len(atts), err)
		}
		filename := ""
		if att.Label != nil {
			filename = *att.Label
		}
		if filename == "" {
			filename = fmt.Sprintf("screenshot %d", i+1)
		}

		q := url.Values{}
		q.Set("filename", filename)
		if token != "" {
			q.Set("token", token)
		}
		endpoint := a.baseURL + "/api/v2/uploads.json?" + q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return "", fmt.Errorf("zendesk: build upload %d/%d: %w", i+1, len(atts), err)
		}
		req.Header.Set("Content-Type", att.MimeType)
		req.Header.Set("Authorization", a.authHeader)

		resp, err := a.client.Do(req)
		if err != nil {
			// Network errors typically don't echo the auth header, but match
			// the redaction discipline anyway: don't surface the raw err.
			return "", fmt.Errorf("zendesk: upload %d/%d: transport error", i+1, len(atts))
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// STATUS ONLY — body OMITTED per L005 (auth-header echo risk).
			return "", fmt.Errorf("zendesk: upload %d/%d returned %d", i+1, len(atts), resp.StatusCode)
		}

		var parsed uploadResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return "", fmt.Errorf("zendesk: upload %d/%d: decode response: %w", i+1, len(atts), err)
		}
		if parsed.Upload.Token == "" {
			return "", fmt.Errorf("zendesk: upload %d/%d response missing upload.token", i+1, len(atts))
		}
		token = parsed.Upload.Token
	}
	return token, nil
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test -race ./internal/adapter/zendesk/... -v && cd ..`
Expected: all existing zendesk tests + the 5 new tests pass. The pre-existing `TestZendeskCreate_HappyPath` (no attachments) must still pass — proves the no-attachments regression.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/adapter/zendesk/zendesk.go relay/internal/adapter/zendesk/zendesk_test.go
git commit -m "$(cat <<'EOF'
feat(6-ii): zendesk — chained uploads.json before ticket create (L005/L011)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Linear — asset uploads before issueCreate with attachmentLinks

**Files:** Modify `relay/internal/adapter/linear/linear.go`, `relay/internal/adapter/linear/linear_test.go`.

Linear's GraphQL `fileUpload` mutation returns signed S3 URLs that require a follow-up PUT — for v0 we use the simpler legacy multipart endpoint at `https://uploads.linear.app/api/file/upload` that takes the file + returns `{ success, uploadFile: { url } }` in a single call. The endpoint is overridable via the existing `endpoint` config key's sibling, but for tests we accept an additional `upload_endpoint` config override (see Step 3 for the seam). The Authorization header is the raw api_key (Linear's convention; no "Bearer "). All uploads run BEFORE `issueCreate`; the mutation input gains `attachmentLinks: [{url, title}]`. Any upload failure returns an error BEFORE issueCreate (L011 orphan prevention). The error wrap **redacts the api_key BEFORE `Truncate()`** so a long-prefix echo can't survive truncation (L011).

**Deviation note vs the prompt:** the prompt says the legacy endpoint may have changed since documentation. If during implementation the legacy endpoint returns 404 / has been retired, switch to the GraphQL `fileUpload` mutation flow (returns `uploadUrl` + `headers` + `asset.url`; PUT the bytes to `uploadUrl` with the returned headers; pass `asset.url` to issueCreate's `attachmentLinks`). The unit tests below operate against a configurable httptest server so either implementation can be tested against the same mocked response shape (`{success, uploadFile:{url}}` for the legacy path; map the GraphQL response into the same shape internally if you take the deviation).

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/adapter/linear/linear_test.go`:

```go
// goldenPNGBytes is the smallest valid 1×1 PNG for upload-flow tests.
var goldenPNGBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

var goldenPNGDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(goldenPNGBytes)

// configuredWithUploads builds an adapter pointed at separate GraphQL and
// upload endpoints. Linear's production GraphQL endpoint and uploads endpoint
// are on different hosts; the adapter accepts upload_endpoint as a sibling of
// endpoint for test injection.
func configuredWithUploads(t *testing.T, graphqlURL, uploadsURL string) *linear.Adapter {
	t.Helper()
	a := linear.New()
	if err := a.Configure(map[string]any{
		"api_key":         testAPIKey,
		"team_id":         testTeamUUID,
		"endpoint":        graphqlURL,
		"upload_endpoint": uploadsURL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestLinearCreate_AttachmentsUploadThenIssueCreate asserts:
//   1. N upload POSTs precede the issueCreate POST.
//   2. Each upload carries multipart/form-data + the raw bytes.
//   3. The issueCreate mutation's input.attachmentLinks references the
//      returned upload URLs with the attachment labels as titles.
func TestLinearCreate_AttachmentsUploadThenIssueCreate(t *testing.T) {
	var (
		uploadCount   int
		uploadBytes   [][]byte
		issueSeen     bool
		issueVariables map[string]any
	)

	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if issueSeen {
			t.Errorf("uploads called AFTER issueCreate — order regression (L011)")
		}
		uploadCount++
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("upload ParseMultipartForm: %v", err)
		}
		// Accept the first file field regardless of name (Linear's legacy
		// endpoint accepts "file"; some clients send "files[]").
		var fh *multipart.FileHeader
		for _, files := range r.MultipartForm.File {
			if len(files) > 0 {
				fh = files[0]
				break
			}
		}
		if fh == nil {
			t.Fatal("upload missing file part")
		}
		f, err := fh.Open()
		if err != nil {
			t.Fatalf("open file part: %v", err)
		}
		b, _ := io.ReadAll(f)
		_ = f.Close()
		uploadBytes = append(uploadBytes, b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"uploadFile":{"url":"https://uploads.linear.app/assets/uuid-%d.png"}}`, uploadCount)))
	}))
	defer uploadsSrv.Close()

	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issueSeen = true
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		issueVariables = body.Variables
		happyIssueHandler(w, r)
	}))
	defer graphqlSrv.Close()

	p := minimalPayload()
	label1 := "before"
	label2 := "after"
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes), Label: &label1},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes), Label: &label2},
	}

	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	if _, err := a.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if uploadCount != 2 {
		t.Fatalf("uploadCount = %d; want 2", uploadCount)
	}
	for i, b := range uploadBytes {
		if !bytes.Equal(b, goldenPNGBytes) {
			t.Errorf("upload[%d] bytes mismatch (len=%d, want=%d)", i, len(b), len(goldenPNGBytes))
		}
	}
	input, _ := issueVariables["input"].(map[string]any)
	if input == nil {
		t.Fatalf("issueCreate variables missing input: %v", issueVariables)
	}
	links, _ := input["attachmentLinks"].([]any)
	if len(links) != 2 {
		t.Fatalf("attachmentLinks len = %d; want 2", len(links))
	}
	l0, _ := links[0].(map[string]any)
	l1, _ := links[1].(map[string]any)
	if l0["url"] != "https://uploads.linear.app/assets/uuid-1.png" {
		t.Errorf("attachmentLinks[0].url = %v; want uuid-1.png", l0["url"])
	}
	if l0["title"] != "before" {
		t.Errorf("attachmentLinks[0].title = %v; want before", l0["title"])
	}
	if l1["url"] != "https://uploads.linear.app/assets/uuid-2.png" {
		t.Errorf("attachmentLinks[1].url = %v; want uuid-2.png", l1["url"])
	}
	if l1["title"] != "after" {
		t.Errorf("attachmentLinks[1].title = %v; want after", l1["title"])
	}
}

// TestLinearCreate_NoAttachmentsRegression asserts the no-attachments path
// does not call the uploads endpoint and does not pass attachmentLinks in
// the issueCreate input (L015).
func TestLinearCreate_NoAttachmentsRegression(t *testing.T) {
	var (
		uploadCalled  bool
		issueVariables map[string]any
	)
	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadsSrv.Close()
	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		issueVariables = body.Variables
		happyIssueHandler(w, r)
	}))
	defer graphqlSrv.Close()

	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if uploadCalled {
		t.Error("uploads called when no attachments present")
	}
	input, _ := issueVariables["input"].(map[string]any)
	if _, has := input["attachmentLinks"]; has {
		t.Errorf("attachmentLinks present when no attachments; got: %v", input["attachmentLinks"])
	}
}

// TestLinearCreate_FirstUploadFails_NoIssueCreate asserts orphan prevention.
func TestLinearCreate_FirstUploadFails_NoIssueCreate(t *testing.T) {
	var issueSeen bool
	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"success":false,"error":"upstream broken"}`))
	}))
	defer uploadsSrv.Close()
	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issueSeen = true
		happyIssueHandler(w, r)
	}))
	defer graphqlSrv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on upload 502")
	}
	if issueSeen {
		t.Errorf("issueCreate happened despite upload failure (L011 regression)")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error should mention status 502; got: %v", err)
	}
	if !strings.Contains(err.Error(), "1/1") {
		t.Errorf("error should mention upload index 1/1; got: %v", err)
	}
}

// TestLinearCreate_UploadMissingURL_NoIssueCreate asserts that a 200 upload
// response without a uploadFile.url returns an error before issueCreate.
func TestLinearCreate_UploadMissingURL_NoIssueCreate(t *testing.T) {
	var issueSeen bool
	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"uploadFile":{}}`))
	}))
	defer uploadsSrv.Close()
	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issueSeen = true
		happyIssueHandler(w, r)
	}))
	defer graphqlSrv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on missing uploadFile.url")
	}
	if issueSeen {
		t.Errorf("issueCreate happened despite missing upload url (L011 regression)")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error should mention missing url; got: %v", err)
	}
}

// TestLinearCreate_UploadKeyNeverLeaks_LongPrefix replicates the existing
// KeyNeverLeaks_LongPrefix pattern for the new asset-upload error path: the
// server's error body contains the api key after 180 chars of filler. The
// adapter must redact BEFORE truncate so the key cannot survive in the error.
func TestLinearCreate_UploadKeyNeverLeaks_LongPrefix(t *testing.T) {
	longPrefix := strings.Repeat("x", 180)
	echoMsg := longPrefix + " token " + testAPIKey + " is invalid"

	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(echoMsg))
	}))
	defer uploadsSrv.Close()
	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("issueCreate must not be called when upload fails")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer graphqlSrv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on upload 401")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Fatalf("L011: api key leaked in upload error (redact-before-truncate ordering broken): %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401; got: %v", err)
	}
}
```

Add the imports at the top of `linear_test.go` if not already present: `"bytes"`, `"encoding/base64"`, `"fmt"`, `"mime/multipart"`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/adapter/linear/ -run TestLinearCreate_Attachments -v && cd ..`
Expected: FAIL — Configure rejects `upload_endpoint`, Create doesn't call uploads, etc.

- [ ] **Step 3: Modify `linear.go` to add the upload loop + Configure key**

Add imports to `linear.go`: `"mime/multipart"`, `"net/textproto"`, `"intake/internal/attachvalidate"`.

Add a constant near `defaultEndpoint`:

```go
const defaultUploadEndpoint = "https://uploads.linear.app/api/file/upload"
```

Add a field to `Adapter`:

```go
type Adapter struct {
	apiKey         string
	teamID         string
	endpoint       string
	uploadEndpoint string // 6-ii: Linear's legacy file-upload endpoint (separate host).
	client         *http.Client
}
```

Initialize the new field in `New()`:

```go
func New() *Adapter {
	return &Adapter{
		endpoint:       defaultEndpoint,
		uploadEndpoint: defaultUploadEndpoint,
		client:         &http.Client{Timeout: 15 * time.Second},
	}
}
```

In `Configure`, after the existing `endpoint` override block (around line 80-82), ADD:

```go
	if up, ok := cfg["upload_endpoint"].(string); ok && up != "" {
		a.uploadEndpoint = up
	}
```

REPLACE the `Create` function body (lines 211-274) with:

```go
// Create runs the Phase 6 (6-ii) sequence: upload each attachment to Linear's
// file-upload endpoint, then POST the issueCreate mutation referencing the
// returned asset URLs in attachmentLinks. Any upload failure returns an error
// BEFORE issueCreate is called (L011 orphan prevention). The api key is never
// included in any error message — redaction runs BEFORE truncate so a
// long-prefix echo cannot slip the key past the 200-rune cap.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	// Phase 6: upload every attachment first; returns a slice of {url, title}
	// values ready for the issueCreate input. Empty p.Attachments returns nil
	// and the issueCreate input omits attachmentLinks entirely.
	links, err := a.uploadAttachments(ctx, p.Attachments)
	if err != nil {
		return nil, err
	}

	input := map[string]any{
		"teamId":      a.teamID,
		"title":       p.Conversation.TitleSuggestion,
		"description": renderBody(p),
	}
	if len(links) > 0 {
		input["attachmentLinks"] = links
	}

	reqBody := graphQLRequest{
		Query:     issueCreateMutation,
		Variables: map[string]any{"input": input},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("linear: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("linear: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linear: http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("linear: graphql endpoint returned %d: %s", resp.StatusCode, adapter.Truncate(a.redact(string(respBody)), 200))
	}

	var parsed issueCreateResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("linear: decode response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		msg := adapter.Truncate(a.redact(joinErrors(parsed.Errors)), 200)
		return nil, fmt.Errorf("linear: graphql errors: %s", msg)
	}
	ic := parsed.Data.IssueCreate
	if !ic.Success || ic.Issue == nil {
		return nil, fmt.Errorf("linear: issueCreate reported failure (success=%t, issue present=%t)", ic.Success, ic.Issue != nil)
	}

	externalID := ic.Issue.ID
	if externalID == "" {
		externalID = ic.Issue.Identifier
	}
	if externalID == "" {
		return nil, fmt.Errorf("linear: issueCreate returned an issue with no id or identifier")
	}
	return &adapter.CreateResult{
		ExternalID:  externalID,
		ExternalURL: ic.Issue.URL,
		AdapterName: a.Name(),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// uploadResponse is the legacy upload endpoint's response shape.
type uploadResponse struct {
	Success    bool `json:"success"`
	UploadFile struct {
		URL string `json:"url"`
	} `json:"uploadFile"`
}

// uploadAttachments POSTs each attachment as multipart/form-data to Linear's
// legacy file-upload endpoint and returns one {url,title} map per upload for
// the issueCreate input.attachmentLinks. Failure short-circuits and returns
// an error BEFORE issueCreate (L011 orphan prevention).
//
// Per-upload error wrapping redacts the api_key BEFORE Truncate per L011 so a
// long-prefix server echo cannot survive truncation.
func (a *Adapter) uploadAttachments(ctx context.Context, atts []payload.Attachment) ([]map[string]any, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	links := make([]map[string]any, 0, len(atts))
	for i, att := range atts {
		raw, _, err := attachvalidate.DecodeOne(att)
		if err != nil {
			return nil, fmt.Errorf("linear: decode attachment %d/%d: %w", i+1, len(atts), err)
		}
		title := ""
		if att.Label != nil {
			title = *att.Label
		}
		if title == "" {
			title = fmt.Sprintf("screenshot %d", i+1)
		}

		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		hdr := textproto.MIMEHeader{
			"Content-Disposition": []string{fmt.Sprintf(`form-data; name="file"; filename=%q`, title)},
			"Content-Type":        []string{att.MimeType},
		}
		part, err := mw.CreatePart(hdr)
		if err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d build part: %w", i+1, len(atts), err)
		}
		if _, err := part.Write(raw); err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d write bytes: %w", i+1, len(atts), err)
		}
		if err := mw.Close(); err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d close multipart: %w", i+1, len(atts), err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.uploadEndpoint, &buf)
		if err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d build request: %w", i+1, len(atts), err)
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", a.apiKey)

		resp, err := a.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d: %s", i+1, len(atts), a.redact(err.Error()))
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// L011: redact BEFORE truncate.
			snippet := adapter.Truncate(a.redact(string(body)), 200)
			return nil, fmt.Errorf("linear: upload %d/%d returned %d: %s", i+1, len(atts), resp.StatusCode, snippet)
		}

		var parsed uploadResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d decode response: %w", i+1, len(atts), err)
		}
		if parsed.UploadFile.URL == "" {
			return nil, fmt.Errorf("linear: upload %d/%d response missing uploadFile.url", i+1, len(atts))
		}
		links = append(links, map[string]any{
			"url":   parsed.UploadFile.URL,
			"title": title,
		})
	}
	return links, nil
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test -race ./internal/adapter/linear/... -v && cd ..`
Expected: all existing linear tests + the 5 new tests pass. The pre-existing happy-path tests (no attachments) prove the L015 regression: `Configure` still accepts no `upload_endpoint`; `Create` with no attachments still issues a single issueCreate POST.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/adapter/linear/linear.go relay/internal/adapter/linear/linear_test.go
git commit -m "$(cat <<'EOF'
feat(6-ii): linear — file uploads before issueCreate with attachmentLinks (L011)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Cross-adapter sweep — full test pass + verify-contract + no-mod-tidy

**Files:** None (verification only).

- [ ] **Step 1: Full relay test suite under race**

Run: `cd relay && go build ./... && go vet ./... && cd ..`
Expected: clean.

Run: `cd relay && go test -race ./... && cd ..`
Expected: ALL relay tests pass (Phase 1+3+4+5+6-i unaffected; 6-ii tests green).

- [ ] **Step 2: Schema contract regression (no codegen changes)**

Run: `bash scripts/verify-contract.sh`
Expected: exits 0. Phase 6 makes ZERO schema changes; if this script regresses, a 6-i carry-over edited a generated file.

- [ ] **Step 3: Pin discipline**

Run: `bash scripts/check-pins.sh`
Expected: exits 0 (only checks pinned Go + npm modules; 6-ii adds none).

- [ ] **Step 4: `go mod tidy` must be a no-op**

Run: `cd relay && go mod tidy && cd .. && git diff --exit-code relay/go.mod relay/go.sum`
Expected: clean diff (no change to either file). If a diff appears, an accidental new import slipped in — investigate. The only NEW import paths in this task set are stdlib (`mime/multipart`, `net/textproto`, `net/url`) plus the existing `intake/internal/attachvalidate` (added by 6-i, not 6-ii).

- [ ] **Step 5: No commit for this task** — verification gate.

---

## Smoke (mandatory)

**Self-runnable; no LLM credit; no maintainer pause.** Each adapter's `Create_WithAttachments` httptest unit smoke runs as part of `go test ./...`. The README §7 step 5 expectation: each adapter's smoke asserts its documented native sequence — covered by the tests in Tasks 1-5. To run only the new smokes:

```bash
cd relay && go test -race -run "TestWebhookCreate_AttachmentsPassthrough|TestFiderCreate_AttachmentsAppendsMarkdownImages|TestChatwootCreate_AttachmentsMultipart|TestZendeskCreate_AttachmentsChainedUploadsThenTicket|TestLinearCreate_AttachmentsUploadThenIssueCreate" ./internal/adapter/... -v && cd ..
```

Expected: all 5 happy-path smokes pass.

Then run the orphan-prevention regressions (L011):

```bash
cd relay && go test -race -run "TestZendeskCreate_FirstUploadFails_NoTicketCreate|TestZendeskCreate_MidBatchUploadFails_NoTicketCreate|TestLinearCreate_FirstUploadFails_NoIssueCreate|TestLinearCreate_UploadMissingURL_NoIssueCreate" ./internal/adapter/... -v && cd ..
```

Expected: all 4 orphan-prevention smokes pass (each asserts the downstream create call is NEVER reached on upload failure).

Then run the secret-redaction regressions (L005 + L011):

```bash
cd relay && go test -race -run "TestZendeskCreate_UploadErrorOmitsBody_L005Guard|TestLinearCreate_UploadKeyNeverLeaks_LongPrefix" ./internal/adapter/... -v && cd ..
```

Expected: both pass (no token / no api_key in any error message; redact-before-truncate ordering holds against a 180-char prefix).

Finally, the L015 no-attachments regressions:

```bash
cd relay && go test -race -run "TestWebhookCreate_NoAttachmentsOmitsField|TestFiderCreate_NoAttachmentsRegression|TestChatwootCreate_NoAttachmentsJSONPathUnchanged|TestZendeskCreate_NoAttachmentsRegression|TestLinearCreate_NoAttachmentsRegression" ./internal/adapter/... -v && cd ..
```

Expected: all 5 pass (each adapter's no-attachments path is byte-identical to its Phase 1/3 behavior).

---

## Done criteria

- [ ] All 6 tasks complete and committed.
- [ ] `cd relay && go build ./... && go vet ./...` clean.
- [ ] `cd relay && go test -race ./...` green (all Phase 1/3/4/5/6-i tests still pass; 6-ii adds 18 new tests across the 5 adapter packages — 2 webhook, 2 fider, 3 chatwoot, 6 zendesk, 5 linear).
- [ ] `bash scripts/verify-contract.sh` green (no schema changes).
- [ ] `bash scripts/check-pins.sh` green.
- [ ] `cd relay && go mod tidy` produces no diff to `go.mod` / `go.sum`.
- [ ] Each adapter's `Create_WithAttachments` httptest smoke pass (covered by the smoke section above).
- [ ] Linear + Zendesk orphan-prevention smokes pass (upload failure → no downstream create call).
- [ ] Linear `KeyNeverLeaks_LongPrefix` and Zendesk `UploadErrorOmitsBody_L005Guard` pass (token/api_key never leaks in any new error path; redact-before-truncate ordering holds).
- [ ] Each adapter's no-attachments regression test passes (L015 — byte-identical to its Phase 1/3 baseline).
- [ ] `adapter.Adapter` interface is **unchanged** (`relay/internal/adapter/adapter.go` byte-identical to its pre-6-ii state).
- [ ] `relay/internal/payload/types.go` is **unchanged** (generated file; never edited).
- [ ] `schema/payload.v1.json` is **unchanged**.
