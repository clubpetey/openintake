# 6-i Config + attachvalidate Package + InitResponse Caps + Body-Cap + Q9 Gate Extension + Adapter Capabilities() — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lock the Phase 6 wire contract so 6-ii (per-adapter native upload paths) and 6-iii (widget capture + redaction modal) can land in parallel. Introduce a `config.AttachmentsConfig` block + defaults, a new `relay/internal/attachvalidate/` package (`ValidateAll`, `DecodeOne`, `Decoded`, `Config`, six sentinel errors), an optional `adapter.CapableAdapter` interface with one `Capabilities()` method on each of the five existing adapters (no behavior change — metadata only), `Capabilities.Attachments` on the `/init` response, `SubmitRequest.Attachments` + `SubmitAttachment`, three new `Deps` fields (`AttachmentsCfg`, `AttachmentMIMEs`, `BodyCapBytes`), an extension to the Q9 consolidated startup gate in `main.go` (`validateAttachments` — returns parsed values per L016; warns when allowlist references unknown MIMEs), and `submitHandler` orchestration that (a) replaces the hard-coded `1<<20` body cap with `deps.BodyCapBytes`, (b) distinguishes `*http.MaxBytesError` (413 `request_body_too_large`) from other decode errors (400 `bad_request`), (c) refuses non-empty `attachments[]` with 400 `attachments_disabled` when `cfg.Attachments.Enabled=false`, and (d) calls `attachvalidate.ValidateAll` AFTER `Builder.Build` and BEFORE `Router.Route`, mapping sentinel errors to 413/415/400 with the documented codes. `initHandler` learns to emit `capabilities.attachments` from `deps.AttachmentMIMEs`. After this sub-plan: no widget-side capture happens yet (6-iii adds it), no adapter actually decodes bytes from a `data:` URL during `Create()` (6-ii adds that via the `DecodeOne` helper), but the entire wire contract is frozen and Phase 1+4+5 callers see zero behavior change on the non-attachment path.

**Architecture:** Three additive surfaces. (1) `config` gains `AttachmentsConfig` + `AttachmentsStorage` plus defaults in `applyDefaults`; the existing Q9 gate (`startupProblems` in `main.go`) gains a sibling `validateAttachments` helper that returns `(config.AttachmentsConfig, []string)` — its parsed value is the L016 single-source-of-truth for downstream consumers. (2) A new `relay/internal/attachvalidate/` package owns base64 decode + `net/http.DetectContentType` magic-byte match + per-attachment + aggregate cap enforcement; six sentinel errors (no wrapping) drive the HTTP status mapping in `submitHandler`. `payloadbuild.Build` is extended additively (one `for` loop) to populate `p.Attachments` 1:1 from `req.Attachments`. (3) Each existing adapter gets a one-method `Capabilities()` returning the v0 list `["image/png","image/jpeg","image/webp"]` (frozen `Adapter` interface untouched; the new `CapableAdapter` is optional and discovered via a type assertion). `main.go` parses the config, runs the gate, computes `computeAttachmentsCaps` once at startup, sets `BodyCapBytes` to `14*1<<20` when enabled and `1<<20` otherwise, and writes all three values into `Deps`.

**Tech Stack:** Go 1.23.2 (relay). Zero new external Go modules — validation uses `encoding/base64` (stdlib) + `net/http.DetectContentType` (stdlib). `go mod tidy` must remain a no-op after this sub-plan lands. No schema codegen rerun (the `Attachment` struct + `attachments[]` field have been generated since Phase 0). No new TS / npm modules in 6-i (`html2canvas` is a 6-iii concern).

---

## Design References

- README §8.2 — `AttachmentsConfig` + `AttachmentsStorage` (frozen here)
- README §8.3 — `SubmitRequest.Attachments` + `SubmitAttachment` (frozen here)
- README §8.4 — `Capabilities.Attachments` + `CapabilitiesAttachments` (frozen here)
- README §8.5 — `attachvalidate` package surface (frozen here)
- README §8.6 — `adapter.CapableAdapter` interface (frozen here)
- README §8.7 — `Deps` extension (frozen here)
- README §8.8 — endpoint contract shapes (frozen here; 413/415/400 codes)
- Design spec §3.1 — inline transport via `/submit` body; body-cap 1 MB → 14 MB only when enabled
- Design spec §3.2 — no change to `adapter.Adapter`; optional `CapableAdapter`
- Design spec §3.3 — capabilities intersection (union of adapter caps ∩ operator allowlist)
- Design spec §3.4 — magic-byte validator uses `net/http.DetectContentType`
- Design spec §5.1 — full `attachvalidate` package layout + exports
- Design spec §5.2 — adapter capabilities helper
- Design spec §5.5 — `submit.go` orchestration changes (order of operations)
- Design spec §5.6 — `payloadbuild` additive change
- Design spec §7.3 — `computeAttachmentsCaps` exact code
- Design spec §8.1 — error envelope code table
- README §9 Notes — L010 PowerShell ascii encoding, L016 return-parsed-values, L005 redact-before-truncate
- Reference: existing `relay/internal/config/config.go:288-314` (Phase 5's `RateLimitConfig` is the exact template for `AttachmentsConfig` placement + defaulting)
- Reference: existing `relay/internal/server/turn.go:27-143` (Phase 5 `initHandler` — Phase 6 inserts the `caps.Attachments` emission alongside the captcha block)
- Reference: existing `relay/internal/server/submit.go:24-96` (Phase 1 `submitHandler` — Phase 6 replaces the literal `1<<20`, distinguishes `*http.MaxBytesError`, and inserts the attachvalidate step before `Router.Route`)
- Reference: existing `relay/cmd/relay/main.go:74-83` (the Phase 5 Q9 gate call site — Phase 6 inserts `validateAttachments(cfg, enabledAdapters)` immediately after it, BEFORE the LLM provider construction so all consolidated misconfigs print in one log line)
- Reference: existing `relay/cmd/relay/main.go:534-595` (`startupProblems` + parsed-values return — Phase 6 mirrors this style exactly for `validateAttachments`)
- Reference: existing `relay/internal/adapter/webhook/webhook.go:41-43` (the `Name()` / `RequiresLicense()` shape — `Capabilities()` slots in alongside them)
- Reference: existing `relay/internal/payloadbuild/build.go:65-164` (Phase 1+4+5 `Build` — Phase 6 inserts ~5 lines populating `p.Attachments` between the `Context` block and the `validateAgainstSchema` call)
- Reference: existing `relay/internal/payload/types.go:10-25` (generated `Attachment` struct — confirms `Type`, `MimeType`, `Url`, `SizeBytes`, `Label` fields are present and 6-i never edits this file)
- Reference: existing `schema/payload.v1.json:15` (`attachments` field already present since Phase 0)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/config/config.go` | Modify | Add `Config.Attachments`, `AttachmentsConfig`, `AttachmentsStorage`; extend `applyDefaults` with the five attachment defaults |
| `relay/internal/config/config_test.go` | Modify | Tests for new defaults + explicit-empty `allowed_mime_types` honored + Phase 5 regression preserved |
| `relay/internal/config/testdata/sample.yaml` | Modify | Add `attachments:` block to sample fixture |
| `relay/internal/dto/dto.go` | Modify | Add `SubmitRequest.Attachments` + `SubmitAttachment` struct |
| `relay/internal/server/dto.go` | Modify | Add `Capabilities.Attachments *CapabilitiesAttachments`; add the `CapabilitiesAttachments` struct |
| `relay/internal/server/deps.go` | Modify | Add `AttachmentsCfg config.AttachmentsConfig`, `AttachmentMIMEs []string`, `BodyCapBytes int64` |
| `relay/internal/attachvalidate/attachvalidate.go` | Create | `Decoded`, `Config`, `ValidateAll`, `DecodeOne`, six sentinel errors |
| `relay/internal/attachvalidate/attachvalidate_test.go` | Create | Magic-byte mismatch, oversized, aggregate-cap, type:"file", bad data URL, base64 corruption, empty allowlist, boundary cases |
| `relay/internal/attachvalidate/fixtures.go` | Create | Smallest-valid PNG/JPEG/WebP byte fixtures (compile-time constants, no testdata dir) |
| `relay/internal/adapter/capabilities.go` | Create | `Capabilities` struct + optional `CapableAdapter` interface |
| `relay/internal/adapter/webhook/webhook.go` | Modify | Add `Capabilities()` method returning the v0 MIME list |
| `relay/internal/adapter/chatwoot/chatwoot.go` | Modify | Same |
| `relay/internal/adapter/fider/fider.go` | Modify | Same |
| `relay/internal/adapter/linear/linear.go` | Modify | Same |
| `relay/internal/adapter/zendesk/zendesk.go` | Modify | Same |
| `relay/internal/adapter/capabilities_test.go` | Create | Asserts every adapter `Capabilities().AcceptedMIMETypes` equals the v0 list |
| `relay/internal/payloadbuild/build.go` | Modify | Populate `p.Attachments` 1:1 from `req.Attachments` (before `validateAgainstSchema`) |
| `relay/internal/payloadbuild/build_test.go` | Modify | Add `TestBuild_PopulatesAttachmentsFromRequest` |
| `relay/internal/server/submit.go` | Modify | Body cap from `deps.BodyCapBytes`; `*http.MaxBytesError` → 413 `request_body_too_large`; `attachments_disabled` short-circuit; `attachvalidate.ValidateAll` step between `Builder.Build` and `Router.Route`; sentinel-error → HTTP-code mapping |
| `relay/internal/server/submit_test.go` | Modify | New tests for body cap, attachments_disabled, each sentinel error mapping, order-of-operations (Route not called when validate fails) |
| `relay/internal/server/turn.go` | Modify | `initHandler` populates `Capabilities.Attachments` from `deps.AttachmentMIMEs` + `deps.AttachmentsCfg` |
| `relay/internal/server/turn_test.go` | Modify | Add tests for `capabilities.attachments` emitted, omitted-when-disabled, omitted-when-empty-allowlist |
| `relay/internal/server/computecaps.go` | Create | `computeAttachmentsCaps(cfg, enabled) *CapabilitiesAttachments` per design spec §7.3 |
| `relay/internal/server/computecaps_test.go` | Create | Empty-adapter-union → nil; cfg ∩ adapter union; disabled → nil |
| `relay/cmd/relay/main.go` | Modify | Add `validateAttachments(cfg, enabled) (config.AttachmentsConfig, []string)` (L016 return parsed value); call it after the Phase 5 Q9 gate; compute `attachmentMIMEs := computeAttachmentsCaps(...)`; set `bodyCapBytes`; populate `Deps.{AttachmentsCfg, AttachmentMIMEs, BodyCapBytes}` |
| `relay/cmd/relay/main_test.go` | Modify | Add `TestValidateAttachments_*` covering bad-storage-mode, cap-inverted, unknown-MIME-warn (returns parsed cfg, problems list, warn log), and clean config |

---

## Tasks

### Task 1: Extend `config.go` with `AttachmentsConfig` + defaults

**Files:** Modify `relay/internal/config/config.go`, `relay/internal/config/config_test.go`, `relay/internal/config/testdata/sample.yaml`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/config/config_test.go` (after the last existing test):

```go
func TestLoad_AppliesPhase6DefaultsForAttachments(t *testing.T) {
	cfg, err := config.Load("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Attachments.Enabled {
		t.Error("default Attachments.Enabled = false; want true")
	}
	if cfg.Attachments.MaxSizeBytes != 5_242_880 {
		t.Errorf("default MaxSizeBytes = %d; want 5_242_880 (5 MB)", cfg.Attachments.MaxSizeBytes)
	}
	if cfg.Attachments.MaxTotalBytes != 10_485_760 {
		t.Errorf("default MaxTotalBytes = %d; want 10_485_760 (10 MB)", cfg.Attachments.MaxTotalBytes)
	}
	want := []string{"image/png", "image/jpeg", "image/webp"}
	if !reflect.DeepEqual(cfg.Attachments.AllowedMIMETypes, want) {
		t.Errorf("default AllowedMIMETypes = %v; want %v", cfg.Attachments.AllowedMIMETypes, want)
	}
	if cfg.Attachments.Storage.Mode != "" {
		t.Errorf("default Storage.Mode = %q; want \"\" (empty defaults to forward semantics)", cfg.Attachments.Storage.Mode)
	}
}

func TestLoad_ExplicitDisabledAttachmentsHonored(t *testing.T) {
	tmp := t.TempDir() + "/disabled.yaml"
	body := []byte("attachments:\n  enabled: false\n")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	cfg, err := config.Load(tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Attachments.Enabled {
		t.Error("Attachments.Enabled = true; want false (explicit disable honored)")
	}
	// Other defaults still apply on the disabled path so reading them is safe.
	if cfg.Attachments.MaxSizeBytes != 5_242_880 {
		t.Errorf("MaxSizeBytes = %d; want 5_242_880 even on disabled path", cfg.Attachments.MaxSizeBytes)
	}
}
```

Ensure imports include `"reflect"` and `"os"` at the top of the file.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/config/... -run TestLoad_AppliesPhase6 -v && cd ..`
Expected: FAIL — `cfg.Attachments undefined`.

- [ ] **Step 3: Add the new structs to `config.go`**

In `relay/internal/config/config.go`, modify the `Config` struct (lines ~13-22) to ADD the new field:

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
	Attachments AttachmentsConfig `yaml:"attachments"` // Phase 6
}
```

Append the two new types at the bottom of `config.go`, AFTER `DailyLLMBudgetConfig` (line ~314) and BEFORE `applyDefaults`:

```go
// AttachmentsConfig configures inline screenshot attachments on /v1/intake/submit (Phase 6).
type AttachmentsConfig struct {
	Enabled          bool               `yaml:"enabled"`            // default true
	MaxSizeBytes     int                `yaml:"max_size_bytes"`     // default 5_242_880   (5 MB per attachment)
	MaxTotalBytes    int                `yaml:"max_total_bytes"`    // default 10_485_760  (10 MB aggregate)
	AllowedMIMETypes []string           `yaml:"allowed_mime_types"` // default ["image/png","image/jpeg","image/webp"]
	Storage          AttachmentsStorage `yaml:"storage"`
}

// AttachmentsStorage selects the attachment storage mode. v0 ships only "forward"
// semantics (inline base64 forwarded to the adapter); persistent S3 storage is v1+.
// Empty string is treated as "forward" at the gate; any other non-empty value is
// fatal at startup via validateAttachments in main.go.
type AttachmentsStorage struct {
	Mode string `yaml:"mode"` // "" | "forward"
}
```

In `applyDefaults`, append at the END (after the Phase 5 captcha default, line ~430):

```go
	// Phase 6 attachment defaults
	if !attachmentsExplicitlyDisabled(c) && c.Attachments.MaxSizeBytes == 0 {
		c.Attachments.MaxSizeBytes = 5_242_880
	}
	if c.Attachments.MaxTotalBytes == 0 {
		c.Attachments.MaxTotalBytes = 10_485_760
	}
	if c.Attachments.AllowedMIMETypes == nil {
		c.Attachments.AllowedMIMETypes = []string{"image/png", "image/jpeg", "image/webp"}
	}
	// Enabled defaults to true. yaml.v3 unmarshalling leaves the field at the
	// Go zero (false) when omitted; the custom unmarshaler below distinguishes
	// "omitted" from "explicitly false" via a pointer.
```

The bool-default-true is a known YAML idiom; mirror the Phase 5 `CaptchaConfig.RequiredFor` custom-unmarshal pattern. Append AFTER the `AttachmentsStorage` type:

```go
// attachmentsConfigRaw lets us distinguish "key omitted entirely" from
// "explicit enabled: false" so the default-true behavior is honorable.
type attachmentsConfigRaw struct {
	Enabled          *bool              `yaml:"enabled"`
	MaxSizeBytes     int                `yaml:"max_size_bytes"`
	MaxTotalBytes    int                `yaml:"max_total_bytes"`
	AllowedMIMETypes *[]string          `yaml:"allowed_mime_types"`
	Storage          AttachmentsStorage `yaml:"storage"`
}

// UnmarshalYAML implements yaml.Unmarshaler so the default for Enabled can be
// true (Phase 6 ships attachments on by default; an operator with the key
// omitted gets the feature, but `enabled: false` is honored).
func (c *AttachmentsConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw attachmentsConfigRaw
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if raw.Enabled != nil {
		c.Enabled = *raw.Enabled
	} else {
		c.Enabled = true
	}
	c.MaxSizeBytes = raw.MaxSizeBytes
	c.MaxTotalBytes = raw.MaxTotalBytes
	if raw.AllowedMIMETypes != nil {
		c.AllowedMIMETypes = *raw.AllowedMIMETypes
		if c.AllowedMIMETypes == nil {
			c.AllowedMIMETypes = []string{} // normalise: explicit empty is non-nil
		}
	}
	c.Storage = raw.Storage
	return nil
}

// attachmentsExplicitlyDisabled is a defensive helper for applyDefaults — when
// the operator set enabled: false we still populate sane numeric defaults so
// reading them on the disabled path doesn't panic, but the caps validation
// gate becomes vacuous.
func attachmentsExplicitlyDisabled(c *Config) bool {
	return !c.Attachments.Enabled
}
```

NOTE: The `attachmentsExplicitlyDisabled` helper currently short-circuits the `MaxSizeBytes` default only — that's intentional. `MaxTotalBytes` and `AllowedMIMETypes` always default so all consumers (`computeAttachmentsCaps`, the disabled-path `attachments_disabled` log line) read consistent values. If the test in Step 1 fails because `MaxSizeBytes` is 0 on the disabled path, the helper is doing its job — fix the test, not the helper. (The Step 1 test asserts on the ENABLED path so this should not occur.)

Actually, re-check the Step 1 `TestLoad_ExplicitDisabledAttachmentsHonored` test: it asserts `MaxSizeBytes == 5_242_880` even on the disabled path. Remove the `attachmentsExplicitlyDisabled` guard — defaults must apply uniformly. The simpler form:

```go
	// Phase 6 attachment defaults — apply uniformly so reads from a disabled
	// attachments block still see consistent values.
	if c.Attachments.MaxSizeBytes == 0 {
		c.Attachments.MaxSizeBytes = 5_242_880
	}
	if c.Attachments.MaxTotalBytes == 0 {
		c.Attachments.MaxTotalBytes = 10_485_760
	}
	if c.Attachments.AllowedMIMETypes == nil {
		c.Attachments.AllowedMIMETypes = []string{"image/png", "image/jpeg", "image/webp"}
	}
```

Drop the helper. The custom `UnmarshalYAML` handles the only Phase 6 quirk (Enabled defaults to true).

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/config/... -v && cd ..`
Expected: all Phase 1+4+5 tests + the two new Phase 6 tests pass.

- [ ] **Step 5: Extend `testdata/sample.yaml` with an `attachments:` block**

Read the existing `relay/internal/config/testdata/sample.yaml` to find the end. APPEND (do not replace) the new block (use 2-space YAML indent matching the rest of the file):

```yaml
attachments:
  enabled: true
  max_size_bytes: 5242880
  max_total_bytes: 10485760
  allowed_mime_types: ["image/png", "image/jpeg", "image/webp"]
  storage:
    mode: "forward"
```

Add a sample-parsing test to `config_test.go`:

```go
func TestLoad_ParsesSampleYAMLPhase6AttachmentsBlock(t *testing.T) {
	cfg, err := config.Load("testdata/sample.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Attachments.Enabled {
		t.Error("sample.yaml has enabled:true; got false")
	}
	if cfg.Attachments.Storage.Mode != "forward" {
		t.Errorf("sample.yaml storage.mode = %q; want forward", cfg.Attachments.Storage.Mode)
	}
	if len(cfg.Attachments.AllowedMIMETypes) != 3 {
		t.Errorf("sample.yaml AllowedMIMETypes len = %d; want 3", len(cfg.Attachments.AllowedMIMETypes))
	}
}
```

Run: `cd relay && go test ./internal/config/... -v && cd ..`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add relay/internal/config/config.go relay/internal/config/config_test.go relay/internal/config/testdata/sample.yaml
git commit -m "$(cat <<'EOF'
feat(6-i): AttachmentsConfig + defaults + sample.yaml block

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Extend `dto.SubmitRequest` with `Attachments` + `SubmitAttachment`

**Files:** Modify `relay/internal/dto/dto.go`

- [ ] **Step 1: Write the failing test**

Create (or append to) `relay/internal/dto/dto_test.go`:

```go
package dto_test

import (
	"encoding/json"
	"testing"

	"intake/internal/dto"
)

func TestSubmitRequest_AttachmentsRoundTrip(t *testing.T) {
	in := dto.SubmitRequest{
		Messages: []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Attachments: []dto.SubmitAttachment{
			{Type: "screenshot", MIMEType: "image/png", URL: "data:image/png;base64,iVBORw0KGgo=", Label: "shot"},
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out dto.SubmitRequest
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Attachments) != 1 {
		t.Fatalf("attachments len = %d; want 1", len(out.Attachments))
	}
	if out.Attachments[0].MIMEType != "image/png" {
		t.Errorf("MIMEType = %q; want image/png", out.Attachments[0].MIMEType)
	}
	if out.Attachments[0].URL != "data:image/png;base64,iVBORw0KGgo=" {
		t.Errorf("URL = %q; want data: URL", out.Attachments[0].URL)
	}
}

func TestSubmitRequest_AttachmentsOmitEmpty(t *testing.T) {
	in := dto.SubmitRequest{Messages: []dto.TurnMessage{{Role: "user", Content: "x"}}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(raw); contains(got, "attachments") {
		t.Errorf("marshalled JSON should omit attachments when empty; got %s", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./internal/dto/... -v && cd ..`
Expected: FAIL — `dto.SubmitAttachment undefined`.

- [ ] **Step 3: Extend `dto.go`**

In `relay/internal/dto/dto.go`, modify `SubmitRequest` (lines ~36-42) to ADD `Attachments`:

```go
// SubmitRequest is the body of POST /v1/intake/submit.
type SubmitRequest struct {
	Messages    []TurnMessage      `json:"messages"`
	Client      ClientInfo         `json:"client"`
	UserClaims  map[string]any     `json:"user_claims"`
	Context     ContextInfo        `json:"context"`
	RoutingHint *string            `json:"routing_hint"`
	Attachments []SubmitAttachment `json:"attachments,omitempty"` // Phase 6
}

// SubmitAttachment is the wire shape for one inline attachment (Phase 6).
// URL is a data: URL (e.g. "data:image/png;base64,iVBORw0KGgo..."); MIMEType
// is the declared content-type and is validated against the actual bytes via
// net/http.DetectContentType in attachvalidate.ValidateAll. Type is
// "screenshot" only in v0; "file" is rejected at attachvalidate with
// 400 attachment_type_unsupported (schema permits "file" so v1 can enable it
// without a schema bump).
type SubmitAttachment struct {
	Type     string `json:"type"`
	MIMEType string `json:"mime_type"`
	URL      string `json:"url"`
	Label    string `json:"label,omitempty"`
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/dto/... -v && cd ..`
Expected: PASS.

Run: `cd relay && go build ./... && cd ..`
Expected: build passes (`server.SubmitRequest` is a type alias to `dto.SubmitRequest`, so the server side automatically sees the new field).

- [ ] **Step 5: Commit**

```bash
git add relay/internal/dto/dto.go relay/internal/dto/dto_test.go
git commit -m "$(cat <<'EOF'
feat(6-i): SubmitRequest.Attachments + SubmitAttachment wire type

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Extend `server.Capabilities` + `InitResponse` with the attachments block

**Files:** Modify `relay/internal/server/dto.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/internal/server/turn_test.go` (the file already exists with Phase 4+5 tests):

```go
func TestCapabilities_AttachmentsRoundTrip(t *testing.T) {
	caps := Capabilities{
		AuthModes: []string{"anonymous"},
		Streaming: true,
		Attachments: &CapabilitiesAttachments{
			MaxSizeBytes:     5_242_880,
			MaxTotalBytes:    10_485_760,
			AllowedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
		},
	}
	raw, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Capabilities
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Attachments == nil {
		t.Fatal("Attachments round-tripped to nil")
	}
	if out.Attachments.MaxSizeBytes != 5_242_880 {
		t.Errorf("MaxSizeBytes = %d; want 5_242_880", out.Attachments.MaxSizeBytes)
	}
}

func TestCapabilities_AttachmentsNilOmitsKey(t *testing.T) {
	caps := Capabilities{AuthModes: []string{"anonymous"}, Streaming: true}
	raw, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "attachments") {
		t.Errorf("nil Attachments should omit the key; got %s", string(raw))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./internal/server/ -run TestCapabilities_Attachments -v && cd ..`
Expected: FAIL — `CapabilitiesAttachments undefined`, `Capabilities.Attachments` missing.

- [ ] **Step 3: Extend `dto.go`**

In `relay/internal/server/dto.go`, modify `Capabilities` (lines ~22-26) to ADD `Attachments`:

```go
// Capabilities advertises relay feature flags to the widget.
type Capabilities struct {
	AuthModes       []string                 `json:"auth_modes"`
	Streaming       bool                     `json:"streaming"`
	RequiresCaptcha []string                 `json:"requires_captcha,omitempty"` // 5-i
	Attachments     *CapabilitiesAttachments `json:"attachments,omitempty"`      // 6-i
}

// CapabilitiesAttachments advertises the inline-attachment policy when the
// relay accepts attachments. nil → attachments disabled OR no enabled adapter
// accepts any allowed type; widget hides the Attach button. The list is the
// intersection of cfg.Attachments.AllowedMIMETypes and the union across
// enabled adapters' Capabilities().AcceptedMIMETypes (computed once at
// startup in main.go via computeAttachmentsCaps).
type CapabilitiesAttachments struct {
	MaxSizeBytes     int      `json:"max_size_bytes"`
	MaxTotalBytes    int      `json:"max_total_bytes"`
	AllowedMIMETypes []string `json:"allowed_mime_types"`
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/server/ -run TestCapabilities_Attachments -v && cd ..`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/server/dto.go relay/internal/server/turn_test.go
git commit -m "$(cat <<'EOF'
feat(6-i): Capabilities.Attachments + CapabilitiesAttachments DTO

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Extend `server.Deps` with `AttachmentsCfg`, `AttachmentMIMEs`, `BodyCapBytes`

**Files:** Modify `relay/internal/server/deps.go`

- [ ] **Step 1: Add the three new fields**

In `relay/internal/server/deps.go`, append at the END of the `Deps` struct (after `TrustedProxies`, line ~98):

```go
	// from 6-i (Phase 6):

	// AttachmentsCfg is the attachments section of the loaded config. submitHandler
	// reads it to construct the attachvalidate.Config and to enforce the
	// attachments_disabled short-circuit when Enabled=false; initHandler reads it
	// to populate Capabilities.Attachments (in conjunction with AttachmentMIMEs).
	AttachmentsCfg config.AttachmentsConfig

	// AttachmentMIMEs is the published allowlist (cfg.AllowedMIMETypes ∩ enabled
	// adapter union), computed once at startup via computeAttachmentsCaps. Empty
	// → /init omits capabilities.attachments AND submitHandler refuses any
	// non-empty attachments[] with 400 attachments_disabled.
	AttachmentMIMEs []string

	// BodyCapBytes is the per-request MaxBytesReader cap on /submit. 14*1<<20 (14 MB)
	// when cfg.Attachments.Enabled=true; 1<<20 (1 MB) otherwise. main.go sets it
	// once at startup based on cfg.Attachments.Enabled.
	BodyCapBytes int64
```

- [ ] **Step 2: Build — must pass**

Run: `cd relay && go build ./... && cd ..`
Expected: build passes (Deps is a value type; existing callers populate without these new fields and they default to zero values, which is safe — the submitHandler change in Task 9 reads them).

NOTE: A `BodyCapBytes` of 0 means `http.MaxBytesReader(w, body, 0)` which rejects ALL bodies. This is intentional defense-in-depth — main.go MUST set this field, and Task 9's submit_test.go explicitly tests the >0 path. Unit tests for `submitHandler` that don't set Deps must opt in to a sane value (Task 9 documents the required value).

- [ ] **Step 3: Commit**

```bash
git add relay/internal/server/deps.go
git commit -m "$(cat <<'EOF'
feat(6-i): Deps gains AttachmentsCfg + AttachmentMIMEs + BodyCapBytes

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Create the `adapter.Capabilities` struct + `CapableAdapter` interface

**Files:** Create `relay/internal/adapter/capabilities.go`, `relay/internal/adapter/capabilities_test.go`

- [ ] **Step 1: Write the failing test**

Create `relay/internal/adapter/capabilities_test.go`:

```go
package adapter_test

import (
	"reflect"
	"testing"

	"intake/internal/adapter"
	"intake/internal/adapter/chatwoot"
	"intake/internal/adapter/fider"
	"intake/internal/adapter/linear"
	"intake/internal/adapter/webhook"
	"intake/internal/adapter/zendesk"
)

// TestCapableAdapter_AllFiveAdaptersAdvertiseV0List asserts each of the five
// Phase 1+3 adapters implements the optional CapableAdapter interface and
// returns the v0 MIME list. This is the per-adapter row of design spec §5.2.
func TestCapableAdapter_AllFiveAdaptersAdvertiseV0List(t *testing.T) {
	want := []string{"image/png", "image/jpeg", "image/webp"}
	cases := []struct {
		name string
		ad   adapter.Adapter
	}{
		{"webhook", webhook.New()},
		{"chatwoot", chatwoot.New()},
		{"fider", fider.New()},
		{"linear", linear.New()},
		{"zendesk", zendesk.New()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cap, ok := c.ad.(adapter.CapableAdapter)
			if !ok {
				t.Fatalf("%s does not implement adapter.CapableAdapter", c.name)
			}
			got := cap.Capabilities().AcceptedMIMETypes
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s Capabilities().AcceptedMIMETypes = %v; want %v", c.name, got, want)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./internal/adapter/... -run TestCapableAdapter -v && cd ..`
Expected: FAIL — `adapter.CapableAdapter undefined`.

- [ ] **Step 3: Create `capabilities.go`**

Create `relay/internal/adapter/capabilities.go`:

```go
// Phase 6 (6-i): optional Capabilities() seam for adapters that advertise
// what attachment MIME types they accept. The frozen Adapter interface is
// UNCHANGED — Capabilities is a separate, optional interface discovered via
// a type assertion (see server.computeAttachmentsCaps).
//
// In v0 all five built-in adapters return the same list
// ["image/png","image/jpeg","image/webp"]. The struct exists so v1+ can
// specialise per-adapter (e.g. a chat-only adapter that accepts only PNG)
// without touching every call site.
package adapter

// Capabilities reports what an adapter supports. v0 carries a single field;
// future versions may add MaxBytes, attachment-count caps, etc.
type Capabilities struct {
	AcceptedMIMETypes []string // empty = no attachments supported
}

// CapableAdapter is the OPTIONAL interface adapters implement to advertise
// attachment capabilities. Adapters that don't implement it advertise no
// capabilities (effectively []string{}). The frozen Adapter interface in
// adapter.go is UNCHANGED — Phase 6 callers use a type assertion to discover
// the optional method.
type CapableAdapter interface {
	Capabilities() Capabilities
}
```

- [ ] **Step 4: Run the test — must FAIL with a different error**

Run: `cd relay && go test ./internal/adapter/... -run TestCapableAdapter -v && cd ..`
Expected: FAIL — five sub-tests fail because no adapter implements `Capabilities()` yet. The error message names each adapter that lacks the method.

- [ ] **Step 5: Commit the interface (the adapters land in Task 6)**

```bash
git add relay/internal/adapter/capabilities.go relay/internal/adapter/capabilities_test.go
git commit -m "$(cat <<'EOF'
feat(6-i): adapter.Capabilities + optional CapableAdapter interface

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Implement `Capabilities()` on each of the five adapters

**Files:** Modify `relay/internal/adapter/{webhook,chatwoot,fider,linear,zendesk}/*.go`

The pattern is identical for every adapter — a single method returning the v0 list. Slot the method directly after `RequiresLicense()` in each file.

- [ ] **Step 1: Add `Capabilities()` to webhook**

In `relay/internal/adapter/webhook/webhook.go`, AFTER line 43 (`func (a *Adapter) RequiresLicense() bool { return false }`), ADD:

```go
// Capabilities advertises the accepted attachment MIME types for /init
// capability discovery (Phase 6, 6-i). In v0 webhook is a pass-through, so
// it accepts every type the relay-wide allowlist permits.
func (a *Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
}
```

The `adapter` import is already present in `webhook.go` (line 13). No new import needed.

- [ ] **Step 2: Add `Capabilities()` to chatwoot**

In `relay/internal/adapter/chatwoot/chatwoot.go`, AFTER line 55 (`func (a *Adapter) RequiresLicense() bool { return false }`), ADD the same method (text identical to the webhook version). Confirm the `intake/internal/adapter` import is present; if not, add it.

- [ ] **Step 3: Add `Capabilities()` to fider**

In `relay/internal/adapter/fider/fider.go`, AFTER line 48, ADD the same method. Confirm/add the `adapter` import.

- [ ] **Step 4: Add `Capabilities()` to linear**

In `relay/internal/adapter/linear/linear.go`, AFTER line 61, ADD the same method. Confirm/add the `adapter` import.

- [ ] **Step 5: Add `Capabilities()` to zendesk**

In `relay/internal/adapter/zendesk/zendesk.go`, AFTER line 51, ADD the same method. Confirm/add the `adapter` import.

- [ ] **Step 6: Run the Task 5 capability test — all five sub-tests pass**

Run: `cd relay && go test ./internal/adapter/... -v && cd ..`
Expected: `TestCapableAdapter_AllFiveAdaptersAdvertiseV0List` passes for every adapter; the existing Phase 1+3 adapter unit tests continue to pass (the new method is purely additive).

- [ ] **Step 7: Commit**

```bash
git add relay/internal/adapter/webhook/webhook.go relay/internal/adapter/chatwoot/chatwoot.go relay/internal/adapter/fider/fider.go relay/internal/adapter/linear/linear.go relay/internal/adapter/zendesk/zendesk.go
git commit -m "$(cat <<'EOF'
feat(6-i): Capabilities() returning v0 MIME list on all five adapters

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Create the `attachvalidate` package (fixtures, errors, Decoded, Config, ValidateAll, DecodeOne)

**Files:** Create `relay/internal/attachvalidate/attachvalidate.go`, `relay/internal/attachvalidate/fixtures.go`, `relay/internal/attachvalidate/attachvalidate_test.go`

- [ ] **Step 1: Write the failing tests**

Create `relay/internal/attachvalidate/attachvalidate_test.go`:

```go
package attachvalidate_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"intake/internal/attachvalidate"
	"intake/internal/payload"
)

func defaultCfg() attachvalidate.Config {
	return attachvalidate.Config{
		MaxSizeBytes:     5_242_880,
		MaxTotalBytes:    10_485_760,
		AllowedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
}

func att(mime, dataURL string) payload.Attachment {
	return payload.Attachment{
		Type:      payload.AttachmentTypeScreenshot,
		MimeType:  mime,
		Url:       dataURL,
		SizeBytes: 0, // declared size_bytes is not the validation source of truth
	}
}

func dataURL(mime string, raw []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

func TestValidateAll_GoldenPNG_OK(t *testing.T) {
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	decoded, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if err != nil {
		t.Fatalf("ValidateAll: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded len = %d; want 1", len(decoded))
	}
	if decoded[0].MIMEType != "image/png" {
		t.Errorf("MIMEType = %q; want image/png", decoded[0].MIMEType)
	}
	if decoded[0].SizeBytes != len(attachvalidate.GoldenPNG()) {
		t.Errorf("SizeBytes = %d; want %d", decoded[0].SizeBytes, len(attachvalidate.GoldenPNG()))
	}
}

func TestValidateAll_GoldenJPEG_OK(t *testing.T) {
	a := att("image/jpeg", dataURL("image/jpeg", attachvalidate.GoldenJPEG()))
	if _, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg()); err != nil {
		t.Fatalf("ValidateAll JPEG: %v", err)
	}
}

func TestValidateAll_GoldenWebP_OK(t *testing.T) {
	a := att("image/webp", dataURL("image/webp", attachvalidate.GoldenWebP()))
	if _, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg()); err != nil {
		t.Fatalf("ValidateAll WebP: %v", err)
	}
}

func TestValidateAll_MIMEMismatch_PNGBytesLabeledJPEG(t *testing.T) {
	a := att("image/jpeg", dataURL("image/jpeg", attachvalidate.GoldenPNG())) // declared JPEG, actually PNG
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrMIMEMismatch) {
		t.Errorf("err = %v; want ErrMIMEMismatch", err)
	}
}

func TestValidateAll_MIMENotAllowed(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowedMIMETypes = []string{"image/jpeg"} // PNG not in this allowlist
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, cfg)
	if !errors.Is(err, attachvalidate.ErrMIMENotAllowed) {
		t.Errorf("err = %v; want ErrMIMENotAllowed", err)
	}
}

func TestValidateAll_EmptyAllowlist_RejectsEverything(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowedMIMETypes = []string{}
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, cfg)
	if !errors.Is(err, attachvalidate.ErrMIMENotAllowed) {
		t.Errorf("err = %v; want ErrMIMENotAllowed (empty allowlist case)", err)
	}
}

func TestValidateAll_AttachmentTooLarge(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxSizeBytes = 10 // tiny
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, cfg)
	if !errors.Is(err, attachvalidate.ErrAttachmentTooLarge) {
		t.Errorf("err = %v; want ErrAttachmentTooLarge", err)
	}
}

// L017 boundary: <= must allow exactly equal sizes.
func TestValidateAll_AttachmentSizeBoundaryEqual_OK(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxSizeBytes = len(attachvalidate.GoldenPNG())
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	if _, err := attachvalidate.ValidateAll([]payload.Attachment{a}, cfg); err != nil {
		t.Errorf("ValidateAll at size boundary equal: %v; want OK", err)
	}
}

func TestValidateAll_AggregateTooLarge(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxTotalBytes = len(attachvalidate.GoldenPNG()) // one fits, two does not
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a, a}, cfg)
	if !errors.Is(err, attachvalidate.ErrAggregateTooLarge) {
		t.Errorf("err = %v; want ErrAggregateTooLarge", err)
	}
}

func TestValidateAll_AggregateBoundaryEqual_OK(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxTotalBytes = 2 * len(attachvalidate.GoldenPNG())
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	if _, err := attachvalidate.ValidateAll([]payload.Attachment{a, a}, cfg); err != nil {
		t.Errorf("ValidateAll at aggregate boundary equal: %v; want OK", err)
	}
}

func TestValidateAll_BadDataURL_NotDataURL(t *testing.T) {
	a := att("image/png", "http://example.com/foo.png")
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

func TestValidateAll_BadDataURL_MissingBase64Marker(t *testing.T) {
	a := att("image/png", "data:image/png,iVBORw0KGgo")
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

func TestValidateAll_BadDataURL_CorruptBase64(t *testing.T) {
	a := att("image/png", "data:image/png;base64,!!!not-base64!!!")
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

func TestValidateAll_BadDataURL_EmptyData(t *testing.T) {
	a := att("image/png", "data:image/png;base64,")
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

func TestValidateAll_TypeFileRejected(t *testing.T) {
	a := payload.Attachment{
		Type:     payload.AttachmentTypeFile,
		MimeType: "image/png",
		Url:      dataURL("image/png", attachvalidate.GoldenPNG()),
	}
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrAttachmentTypeUnsupported) {
		t.Errorf("err = %v; want ErrAttachmentTypeUnsupported", err)
	}
}

func TestValidateAll_EmptySliceOK(t *testing.T) {
	decoded, err := attachvalidate.ValidateAll(nil, defaultCfg())
	if err != nil {
		t.Errorf("ValidateAll on nil: %v; want nil", err)
	}
	if len(decoded) != 0 {
		t.Errorf("decoded len = %d; want 0", len(decoded))
	}
}

func TestDecodeOne_OK(t *testing.T) {
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	raw, mime, err := attachvalidate.DecodeOne(a)
	if err != nil {
		t.Fatalf("DecodeOne: %v", err)
	}
	if mime != "image/png" {
		t.Errorf("mime = %q; want image/png", mime)
	}
	if string(raw) != string(attachvalidate.GoldenPNG()) {
		t.Errorf("raw bytes mismatch")
	}
}

func TestDecodeOne_BadDataURL(t *testing.T) {
	a := att("image/png", "not-a-data-url")
	_, _, err := attachvalidate.DecodeOne(a)
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

// Sentinel-error sanity: each exported error has a stable, descriptive message.
func TestSentinelErrorMessages(t *testing.T) {
	cases := map[error]string{
		attachvalidate.ErrAttachmentTooLarge:        "max_size_bytes",
		attachvalidate.ErrAggregateTooLarge:         "total cap",
		attachvalidate.ErrMIMENotAllowed:            "allowlist",
		attachvalidate.ErrMIMEMismatch:              "match",
		attachvalidate.ErrBadDataURL:                "data:",
		attachvalidate.ErrAttachmentTypeUnsupported: "v0",
	}
	for err, sub := range cases {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("error %v: message does not contain %q", err, sub)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/attachvalidate/... -v && cd ..`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Create the fixtures file**

Create `relay/internal/attachvalidate/fixtures.go`:

```go
package attachvalidate

// Minimal-header byte fixtures for the three v0 MIME types. These are the
// smallest byte sequences that net/http.DetectContentType recognises as the
// claimed format — sufficient for the magic-byte path. Hexdumps below.
//
// They are exported via the GoldenXxx helpers so the test file can build
// data: URLs from them without colocating testdata files (CI keeps the
// package self-contained).

// pngMagic is the 8-byte PNG signature followed by a minimal IHDR-less stub.
// net/http.DetectContentType requires the 8 magic bytes and an IHDR-like
// pattern (\x89PNG\r\n\x1a\n) to return "image/png".
var pngMagic = []byte{
	0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A,
	// padding so DetectContentType has at least the 8 magic bytes; the rest
	// is irrelevant to detection.
	0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R',
}

// jpegMagic is the SOI marker (\xFF\xD8\xFF) plus enough header to satisfy
// stdlib sniffing.
var jpegMagic = []byte{
	0xFF, 0xD8, 0xFF, 0xE0,
	0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01,
}

// webpMagic is the "RIFF...WEBP" container header.
var webpMagic = []byte{
	'R', 'I', 'F', 'F',
	0x00, 0x00, 0x00, 0x00, // file length (don't care for sniff)
	'W', 'E', 'B', 'P',
	'V', 'P', '8', ' ',
}

// GoldenPNG returns the minimal valid PNG header (sniffable by
// net/http.DetectContentType as image/png). Exported for tests.
func GoldenPNG() []byte { return append([]byte(nil), pngMagic...) }

// GoldenJPEG returns the minimal valid JPEG header.
func GoldenJPEG() []byte { return append([]byte(nil), jpegMagic...) }

// GoldenWebP returns the minimal valid WebP header.
func GoldenWebP() []byte { return append([]byte(nil), webpMagic...) }
```

- [ ] **Step 4: Create the main package file**

Create `relay/internal/attachvalidate/attachvalidate.go`:

```go
// Package attachvalidate decodes and validates inline attachments carried in
// a SubmitRequest. It is the single source of truth for Phase 6 magic-byte +
// size-cap enforcement; submitHandler calls ValidateAll between Builder.Build
// and Router.Route. Each adapter that needs raw bytes calls DecodeOne inside
// its own Create() — the validation already passed by then; this is the
// cheap (base64) decode helper.
//
// Errors are SENTINELS — never wrapped — so submitHandler can map them to
// specific HTTP status codes via errors.Is. The six exported errors cover
// every misuse path. The first-encountered error wins on a multi-attachment
// request; the rest are not inspected.
package attachvalidate

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"intake/internal/payload"
)

// Decoded is the validator's per-attachment output: bytes pulled out of the
// data: URL, with the magic-byte-detected MIME and the byte length. Not used
// in 6-i beyond returning a value to the caller; 6-ii's adapters will use
// the equivalent of this via DecodeOne inside their own Create().
type Decoded struct {
	Raw       []byte
	MIMEType  string
	SizeBytes int
	Label     string
	Type      payload.AttachmentType
}

// Config carries the validator's enforcement knobs. submitHandler composes
// this from deps.AttachmentsCfg + deps.AttachmentMIMEs (the
// capabilities-intersection list — so attachvalidate doesn't need to know
// about adapter capabilities directly).
type Config struct {
	MaxSizeBytes     int
	MaxTotalBytes    int
	AllowedMIMETypes []string
}

// Sentinel errors. submitHandler uses errors.Is to map each to a specific
// HTTP status (413/415/400). The package never wraps these; callers can rely
// on identity comparison.
var (
	ErrAttachmentTooLarge        = errors.New("attachment exceeds max_size_bytes")
	ErrAggregateTooLarge         = errors.New("attachments exceed total cap")
	ErrMIMENotAllowed            = errors.New("attachment mime_type not in allowlist")
	ErrMIMEMismatch              = errors.New("attachment bytes do not match declared mime_type")
	ErrBadDataURL                = errors.New("attachment url is not a valid data: URL")
	ErrAttachmentTypeUnsupported = errors.New("attachment type unsupported in v0")
)

// ValidateAll decodes and validates every attachment in atts. Returns the
// decoded slice (1:1 with atts) on success, or the FIRST encountered sentinel
// error on any failure. A nil/empty atts is a successful no-op.
//
// Validation order per attachment:
//  1. Type must be "screenshot" (v0 only — "file" rejected).
//  2. MIME must be in cfg.AllowedMIMETypes (an empty allowlist rejects all).
//  3. URL must be a "data:<mime>;base64,<payload>" with non-empty payload.
//  4. base64 must decode cleanly.
//  5. Decoded length must be <= cfg.MaxSizeBytes.
//  6. Sum of decoded lengths must be <= cfg.MaxTotalBytes.
//  7. net/http.DetectContentType on the first 512 bytes must match the
//     declared MIME (case-insensitive; ignores params like "; charset=").
func ValidateAll(atts []payload.Attachment, cfg Config) ([]Decoded, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	out := make([]Decoded, 0, len(atts))
	allowed := buildAllowlistSet(cfg.AllowedMIMETypes)
	total := 0
	for _, a := range atts {
		// 1. type guard
		if a.Type != payload.AttachmentTypeScreenshot {
			return nil, ErrAttachmentTypeUnsupported
		}
		// 2. mime allowlist
		if _, ok := allowed[a.MimeType]; !ok {
			return nil, ErrMIMENotAllowed
		}
		// 3-4. parse + decode the data: URL
		raw, err := decodeDataURL(a.Url)
		if err != nil {
			return nil, err
		}
		// 5. per-attachment cap
		if cfg.MaxSizeBytes > 0 && len(raw) > cfg.MaxSizeBytes {
			return nil, ErrAttachmentTooLarge
		}
		// 6. aggregate cap
		total += len(raw)
		if cfg.MaxTotalBytes > 0 && total > cfg.MaxTotalBytes {
			return nil, ErrAggregateTooLarge
		}
		// 7. magic-byte match
		sniffed := sniffMIME(raw)
		if !mimeBaseEqual(sniffed, a.MimeType) {
			return nil, ErrMIMEMismatch
		}
		label := ""
		if a.Label != nil {
			label = *a.Label
		}
		out = append(out, Decoded{
			Raw:       raw,
			MIMEType:  a.MimeType,
			SizeBytes: len(raw),
			Label:     label,
			Type:      a.Type,
		})
	}
	return out, nil
}

// DecodeOne is the per-adapter helper. Decodes one Attachment's data: URL to
// raw bytes + the magic-byte-detected MIME. submitHandler has already passed
// the same attachment through ValidateAll by the time any adapter sees it;
// DecodeOne is for adapters that need the raw bytes for native upload
// (chatwoot inline base64, linear asset upload, zendesk uploads.json).
// Returns ErrBadDataURL on malformed input. Does NOT enforce caps or
// allowlist — those are submitHandler's job.
func DecodeOne(att payload.Attachment) (raw []byte, mime string, err error) {
	raw, err = decodeDataURL(att.Url)
	if err != nil {
		return nil, "", err
	}
	return raw, sniffMIME(raw), nil
}

// decodeDataURL parses "data:<mime>;base64,<payload>" and returns the raw
// bytes. Returns ErrBadDataURL on any malformed input.
func decodeDataURL(s string) ([]byte, error) {
	const prefix = "data:"
	const marker = ";base64,"
	if !strings.HasPrefix(s, prefix) {
		return nil, ErrBadDataURL
	}
	idx := strings.Index(s, marker)
	if idx < 0 {
		return nil, ErrBadDataURL
	}
	encoded := s[idx+len(marker):]
	if encoded == "" {
		return nil, ErrBadDataURL
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrBadDataURL
	}
	if len(raw) == 0 {
		return nil, ErrBadDataURL
	}
	return raw, nil
}

// sniffMIME returns net/http.DetectContentType's verdict on the first 512
// bytes. Returns the lower-cased MIME (no params) so mimeBaseEqual can
// match it against the declared value.
func sniffMIME(raw []byte) string {
	limit := len(raw)
	if limit > 512 {
		limit = 512
	}
	full := http.DetectContentType(raw[:limit])
	// DetectContentType may append "; charset=...", "; boundary=..." etc.
	if i := strings.IndexByte(full, ';'); i >= 0 {
		full = full[:i]
	}
	return strings.ToLower(strings.TrimSpace(full))
}

// mimeBaseEqual compares two MIME values case-insensitively, ignoring
// params and whitespace.
func mimeBaseEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func buildAllowlistSet(list []string) map[string]struct{} {
	out := make(map[string]struct{}, len(list))
	for _, m := range list {
		out[m] = struct{}{}
	}
	return out
}
```

- [ ] **Step 5: Run the tests — must pass**

Run: `cd relay && go test ./internal/attachvalidate/... -v && cd ..`
Expected: every test in `attachvalidate_test.go` passes. If any DetectContentType edge case (especially WebP `VP8 ` vs `VP8L`) fails, expand `webpMagic` to match the variant the stdlib expects — the fix is local to `fixtures.go`.

- [ ] **Step 6: Commit**

```bash
git add relay/internal/attachvalidate/attachvalidate.go relay/internal/attachvalidate/attachvalidate_test.go relay/internal/attachvalidate/fixtures.go
git commit -m "$(cat <<'EOF'
feat(6-i): attachvalidate package (ValidateAll, DecodeOne, 6 sentinel errors)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Extend `payloadbuild.Build` to populate `p.Attachments` from `req.Attachments`

**Files:** Modify `relay/internal/payloadbuild/build.go`, `relay/internal/payloadbuild/build_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/internal/payloadbuild/build_test.go`:

```go
func TestBuild_PopulatesAttachmentsFromRequest(t *testing.T) {
	req := &dto.SubmitRequest{
		Messages: []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Client: dto.ClientInfo{
			WidgetVersion: "test",
			URL:           "https://example.com",
			UserAgent:     "ua",
			Viewport:      dto.Viewport{W: 100, H: 100},
			Locale:        "en",
		},
		Attachments: []dto.SubmitAttachment{
			{Type: "screenshot", MIMEType: "image/png", URL: "data:image/png;base64,iVBORw0KGgo=", Label: "shot-1"},
			{Type: "screenshot", MIMEType: "image/jpeg", URL: "data:image/jpeg;base64,/9j/4AAQ="},
		},
	}
	cls := &classify.Result{
		Classification:  "bug",
		SeverityGuess:   "low",
		Summary:         "s",
		TitleSuggestion: "t",
		TagsSuggested:   []string{},
		Language:        "en",
	}
	sess := &auth.SessionContext{SessionID: "00000000-0000-0000-0000-000000000001", AuthMode: "anonymous"}

	b := payloadbuild.New("0.1.0")
	p, err := b.Build(context.Background(), req, cls, sess, payloadbuild.NewSubmissionID(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Attachments) != 2 {
		t.Fatalf("p.Attachments len = %d; want 2", len(p.Attachments))
	}
	if p.Attachments[0].MimeType != "image/png" || p.Attachments[1].MimeType != "image/jpeg" {
		t.Errorf("p.Attachments order/mime wrong: %+v", p.Attachments)
	}
	if p.Attachments[0].Url != "data:image/png;base64,iVBORw0KGgo=" {
		t.Errorf("Url not carried verbatim: %q", p.Attachments[0].Url)
	}
	if p.Attachments[0].Label == nil || *p.Attachments[0].Label != "shot-1" {
		t.Errorf("Label not carried: %+v", p.Attachments[0].Label)
	}
	if p.Attachments[1].Label != nil {
		t.Errorf("Empty label should be nil-pointer; got %v", *p.Attachments[1].Label)
	}
}

func TestBuild_NoAttachments_NilOrEmpty(t *testing.T) {
	req := &dto.SubmitRequest{
		Messages: []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Client: dto.ClientInfo{
			WidgetVersion: "test", URL: "https://example.com", UserAgent: "ua",
			Viewport: dto.Viewport{W: 100, H: 100}, Locale: "en",
		},
	}
	cls := &classify.Result{
		Classification: "bug", SeverityGuess: "low",
		Summary: "s", TitleSuggestion: "t", TagsSuggested: []string{}, Language: "en",
	}
	sess := &auth.SessionContext{SessionID: "00000000-0000-0000-0000-000000000001", AuthMode: "anonymous"}

	b := payloadbuild.New("0.1.0")
	p, err := b.Build(context.Background(), req, cls, sess, payloadbuild.NewSubmissionID(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Attachments) != 0 {
		t.Errorf("len(p.Attachments) = %d; want 0", len(p.Attachments))
	}
}
```

Add imports as needed at the top of the existing `build_test.go` (`context`, `time`, `intake/internal/auth`, `intake/internal/classify`, `intake/internal/dto`, `intake/internal/payloadbuild`).

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/payloadbuild/... -run TestBuild_(PopulatesAttachments|NoAttachments) -v && cd ..`
Expected: FAIL — `p.Attachments` is empty even though `req.Attachments` has entries.

- [ ] **Step 3: Modify `Build` to populate `p.Attachments`**

In `relay/internal/payloadbuild/build.go`, AFTER the `// Copy context if non-empty.` block (lines ~150-156) and BEFORE the `// Schema validation (runtime, L003 mitigation).` block, INSERT:

```go
	// Phase 6 (6-i): copy attachments 1:1 from the request. Magic-byte +
	// size-cap validation happens AFTER Build returns, in submitHandler's
	// attachvalidate.ValidateAll call — payloadbuild stays focused on shape
	// while attachvalidate owns content validation.
	if len(req.Attachments) > 0 {
		p.Attachments = make([]payload.Attachment, 0, len(req.Attachments))
		for _, a := range req.Attachments {
			var labelPtr *string
			if a.Label != "" {
				lbl := a.Label
				labelPtr = &lbl
			}
			p.Attachments = append(p.Attachments, payload.Attachment{
				Type:      payload.AttachmentType(a.Type),
				MimeType:  a.MIMEType,
				Url:       a.URL,
				Label:     labelPtr,
				SizeBytes: 0, // declared size; attachvalidate substitutes the real value if needed downstream
			})
		}
	}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/payloadbuild/... -v && cd ..`
Expected: all existing Phase 1+4+5 tests pass + the two new Phase 6 tests pass.

NOTE on schema validation: the generated `payload.Attachment` requires `size_bytes` per schema, BUT the field is `int` with no minimum > 0 (just `>= 0` — see `payload/types.go:80`), so a literal `0` passes. The schema also permits `type: "file"`, so a request carrying `type:"file"` reaches `attachvalidate` (which rejects it). If a future schema bump tightens `size_bytes` to `> 0`, this Task will need to set `SizeBytes` from the base64-decoded byte length at build time — flagged here so future maintainers don't miss it.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/payloadbuild/build.go relay/internal/payloadbuild/build_test.go
git commit -m "$(cat <<'EOF'
feat(6-i): payloadbuild.Build populates p.Attachments from req.Attachments

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Extend `submitHandler` — body cap from Deps, 413 split, attachments_disabled, attachvalidate step

**Files:** Modify `relay/internal/server/submit.go`, `relay/internal/server/submit_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/server/submit_test.go` (create the file if it doesn't exist — but the package has existing tests; locate or create):

```go
func newPNGAttachment() dto.SubmitAttachment {
	return dto.SubmitAttachment{
		Type:     "screenshot",
		MIMEType: "image/png",
		URL:      "data:image/png;base64," + base64.StdEncoding.EncodeToString(attachvalidate.GoldenPNG()),
	}
}

func newSubmitDeps(t *testing.T, attachmentsEnabled bool) Deps {
	t.Helper()
	cfg := config.AttachmentsConfig{
		Enabled:          attachmentsEnabled,
		MaxSizeBytes:     5_242_880,
		MaxTotalBytes:    10_485_760,
		AllowedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
	bodyCap := int64(1 << 20)
	if attachmentsEnabled {
		bodyCap = 14 * (1 << 20)
	}
	return Deps{
		Auth:           auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg:        config.AuthConfig{Modes: config.AuthModes{Anonymous: true}},
		Classifier:     newStubClassifier(),     // a Phase-1 helper already in the test file
		Builder:        payloadbuild.New("test"),
		Router:         newStubRouter(t),         // returns a webhook-style adapter
		AttachmentsCfg: cfg,
		AttachmentMIMEs: []string{"image/png", "image/jpeg", "image/webp"},
		BodyCapBytes:   bodyCap,
	}
}

// withSession is a small helper that mounts the auth middleware so the
// downstream submitHandler sees a SessionContext in r.Context().
func withSession(t *testing.T, deps Deps, h http.Handler) http.Handler {
	return deps.Auth.Handler(h)
}

func TestSubmit_OverBodyCap_413_RequestBodyTooLarge(t *testing.T) {
	deps := newSubmitDeps(t, true)
	// 15 MB body (above the 14 MB cap) — content doesn't need to be valid JSON
	// because MaxBytesReader rejects BEFORE the decoder sees it.
	big := make([]byte, 15*1024*1024)
	for i := range big {
		big[i] = 'a'
	}
	sessionID := deps.Auth.Store().Issue()
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(big))
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	withSession(t, deps, submitHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d; want 413", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "request_body_too_large") {
		t.Errorf("body does not contain request_body_too_large: %s", rec.Body.String())
	}
}

func TestSubmit_MalformedJSON_400_BadRequest(t *testing.T) {
	deps := newSubmitDeps(t, true)
	sessionID := deps.Auth.Store().Issue()
	req := httptest.NewRequest("POST", "/v1/intake/submit", strings.NewReader("{not json"))
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	withSession(t, deps, submitHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "bad_request") {
		t.Errorf("body does not contain bad_request: %s", rec.Body.String())
	}
}

func TestSubmit_AttachmentsDisabled_400_AttachmentsDisabled(t *testing.T) {
	deps := newSubmitDeps(t, false) // attachments off; body cap stays 1 MB
	deps.AttachmentMIMEs = nil
	body := mustMarshal(t, dto.SubmitRequest{
		Messages:    []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Client:      validClient(),
		Attachments: []dto.SubmitAttachment{newPNGAttachment()},
	})
	sessionID := deps.Auth.Store().Issue()
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	withSession(t, deps, submitHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "attachments_disabled") {
		t.Errorf("want attachments_disabled; got %s", rec.Body.String())
	}
}

func TestSubmit_AttachmentMIMEMismatch_415(t *testing.T) {
	deps := newSubmitDeps(t, true)
	bad := newPNGAttachment()
	bad.MIMEType = "image/jpeg" // declared JPEG, bytes are PNG
	body := mustMarshal(t, dto.SubmitRequest{
		Messages:    []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Client:      validClient(),
		Attachments: []dto.SubmitAttachment{bad},
	})
	sessionID := deps.Auth.Store().Issue()
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	withSession(t, deps, submitHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d; want 415", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "attachment_mime_mismatch") {
		t.Errorf("want attachment_mime_mismatch; got %s", rec.Body.String())
	}
}

func TestSubmit_AttachmentTooLarge_413(t *testing.T) {
	deps := newSubmitDeps(t, true)
	deps.AttachmentsCfg.MaxSizeBytes = 10 // tiny
	body := mustMarshal(t, dto.SubmitRequest{
		Messages:    []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Client:      validClient(),
		Attachments: []dto.SubmitAttachment{newPNGAttachment()},
	})
	sessionID := deps.Auth.Store().Issue()
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	withSession(t, deps, submitHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d; want 413", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "attachment_too_large") {
		t.Errorf("want attachment_too_large; got %s", rec.Body.String())
	}
}

// Order-of-operations regression: when attachvalidate rejects, Router.Route
// MUST NOT be called (no orphan adapter create).
func TestSubmit_AttachvalidateFails_RouterNotCalled(t *testing.T) {
	deps := newSubmitDeps(t, true)
	routeCalls := 0
	deps.Router = newStubRouterCounter(t, &routeCalls)
	bad := newPNGAttachment()
	bad.MIMEType = "image/jpeg" // forces ErrMIMEMismatch
	body := mustMarshal(t, dto.SubmitRequest{
		Messages:    []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Client:      validClient(),
		Attachments: []dto.SubmitAttachment{bad},
	})
	sessionID := deps.Auth.Store().Issue()
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	withSession(t, deps, submitHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d; want 415", rec.Code)
	}
	if routeCalls != 0 {
		t.Errorf("Router.Route called %d times; want 0 when attachvalidate fails", routeCalls)
	}
}

func TestSubmit_PhaseOneRegression_NoAttachments_200(t *testing.T) {
	deps := newSubmitDeps(t, true)
	body := mustMarshal(t, dto.SubmitRequest{
		Messages: []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Client:   validClient(),
	})
	sessionID := deps.Auth.Store().Issue()
	req := httptest.NewRequest("POST", "/v1/intake/submit", bytes.NewReader(body))
	req.Header.Set("X-Intake-Session", sessionID)
	rec := httptest.NewRecorder()
	withSession(t, deps, submitHandler(deps)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want 200 (no attachments path)", rec.Code)
	}
}
```

The test file references helpers — `validClient()`, `mustMarshal`, `newStubClassifier`, `newStubRouter`, `newStubRouterCounter`. The first two are tiny utilities to define in the same file. The router/classifier stubs may already exist in the package's existing test files (Phase 1's `submit_test.go`); reuse them. If they don't exist with these names, mirror the Phase 1 test scaffolding:

```go
func validClient() dto.ClientInfo {
	return dto.ClientInfo{
		WidgetVersion: "test", URL: "https://example.com", UserAgent: "ua",
		Viewport: dto.Viewport{W: 100, H: 100}, Locale: "en",
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
```

For `newStubClassifier`, `newStubRouter`, `newStubRouterCounter` — Phase 1's `submit_test.go` (`relay/internal/server/submit_test.go` if it exists, else inferable from `turn_test.go`'s helpers) is the source. If absent, create them as thin in-memory stubs that satisfy the `*classify.Classifier` and `*router.Router` types' minimal interface used by `submitHandler`. (Both packages expose constructor functions; the simplest path is to use the real types with a fake LLM + a single-adapter router.)

Add the imports `bytes`, `encoding/base64`, `intake/internal/attachvalidate`, `intake/internal/dto` at the top of `submit_test.go`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/server/ -run TestSubmit -v && cd ..`
Expected: every new test fails because submit.go still uses the literal `1<<20`, doesn't distinguish `*http.MaxBytesError`, and doesn't call attachvalidate.

- [ ] **Step 3: Rewrite `submitHandler`**

Replace `relay/internal/server/submit.go` in its entirety with:

```go
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"intake/internal/attachvalidate"
	"intake/internal/auth"
	"intake/internal/llm"
	"intake/internal/payloadbuild"
)

// submitHandler handles POST /v1/intake/submit.
// It:
//  1. Caps body size to deps.BodyCapBytes (14 MB when attachments enabled, 1 MB otherwise).
//  2. Decodes the SubmitRequest body. *http.MaxBytesError → 413 request_body_too_large;
//     other decode errors → 400 bad_request.
//  3. Refuses non-empty attachments[] with 400 attachments_disabled when
//     cfg.Attachments.Enabled=false.
//  4. Extracts the SessionContext from context (placed by auth middleware).
//  5. Builds []llm.Message and calls Classifier.Classify.
//  6. Calls Builder.Build to assemble + validate the canonical payload (which now
//     additively carries Attachments).
//  7. NEW: calls attachvalidate.ValidateAll on p.Attachments. Sentinel errors
//     map to 413/415/400 per the documented error codes (README §8.8).
//  8. Calls Router.Route + adapter.Create only after attachvalidate passes.
//  9. Returns a SubmitResponse (200) or an ErrorEnvelope.
func submitHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 1. Body cap (Phase 6: from Deps; Phase 1 used a literal 1<<20).
		r.Body = http.MaxBytesReader(w, r.Body, deps.BodyCapBytes)

		// 2. Decode request body.
		var req SubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "request_body_too_large", "submission body exceeds limit")
				return
			}
			writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: malformed JSON")
			return
		}

		// 3. Attachments-disabled short-circuit. Runs BEFORE attachvalidate so the
		//    operator's intent (Enabled=false OR no adapter accepts anything)
		//    surfaces a clear error even if the bytes would have otherwise passed.
		if len(req.Attachments) > 0 && (!deps.AttachmentsCfg.Enabled || len(deps.AttachmentMIMEs) == 0) {
			writeError(w, http.StatusBadRequest, "attachments_disabled", "attachments are disabled on this relay")
			return
		}

		// 4. Extract session.
		sess, ok := auth.FromContext(ctx)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing session context")
			return
		}

		// 5. Classify (Phase 1+4+5 unchanged).
		llmMsgs := make([]llm.Message, 0, len(req.Messages))
		for _, m := range req.Messages {
			llmMsgs = append(llmMsgs, llm.Message{Role: m.Role, Content: m.Content})
		}
		classifyResult, err := deps.Classifier.Classify(ctx, llmMsgs)
		if err != nil {
			slog.WarnContext(ctx, "classify degraded to safe defaults", "err", err)
		}

		// 6. Build canonical payload (now additively carries Attachments).
		submissionID := payloadbuild.NewSubmissionID()
		submittedAt := time.Now().UTC()
		p, err := deps.Builder.Build(ctx, &req, classifyResult, sess, submissionID, submittedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "payload_invalid", fmt.Sprintf("payload validation failed: %v", err))
			return
		}

		// 7. NEW (Phase 6): validate attachments after Build, before Route. Sentinel
		//    errors map to specific HTTP status codes per README §8.8.
		if len(p.Attachments) > 0 {
			validatorCfg := attachvalidate.Config{
				MaxSizeBytes:     deps.AttachmentsCfg.MaxSizeBytes,
				MaxTotalBytes:    deps.AttachmentsCfg.MaxTotalBytes,
				AllowedMIMETypes: deps.AttachmentMIMEs,
			}
			if _, vErr := attachvalidate.ValidateAll(p.Attachments, validatorCfg); vErr != nil {
				status, code, msg := mapAttachvalidateError(vErr)
				writeError(w, status, code, msg)
				return
			}
		}

		// 8. Route + create.
		ad, err := deps.Router.Route(p)
		if err != nil {
			slog.ErrorContext(ctx, "router: no adapter resolved", "error", err)
			writeError(w, http.StatusBadGateway, "adapter_error", "no adapter available")
			return
		}
		result, err := ad.Create(ctx, p)
		if err != nil {
			slog.ErrorContext(ctx, "adapter create failed", "adapter", ad.Name(), "error", err)
			writeError(w, http.StatusBadGateway, "adapter_error", "downstream adapter unavailable")
			return
		}

		// 9. Success.
		writeJSON(w, http.StatusOK, SubmitResponse{
			ExternalID:  result.ExternalID,
			ExternalURL: result.ExternalURL,
			AdapterName: result.AdapterName,
			CreatedAt:   result.CreatedAt,
		})
	}
}

// mapAttachvalidateError maps the six sentinel errors to (status, code, message).
// The message is the client-facing string; the server log has already captured
// the underlying sentinel via slog at the call site if needed (none in 6-i).
func mapAttachvalidateError(err error) (status int, code string, msg string) {
	switch {
	case errors.Is(err, attachvalidate.ErrAttachmentTooLarge):
		return http.StatusRequestEntityTooLarge, "attachment_too_large", "attachment exceeds max_size_bytes"
	case errors.Is(err, attachvalidate.ErrAggregateTooLarge):
		return http.StatusRequestEntityTooLarge, "attachments_exceed_total", "attachments exceed total cap"
	case errors.Is(err, attachvalidate.ErrMIMENotAllowed):
		return http.StatusUnsupportedMediaType, "attachment_mime_not_allowed", "attachment mime_type not allowed"
	case errors.Is(err, attachvalidate.ErrMIMEMismatch):
		return http.StatusUnsupportedMediaType, "attachment_mime_mismatch", "attachment bytes do not match declared mime_type"
	case errors.Is(err, attachvalidate.ErrBadDataURL):
		return http.StatusBadRequest, "attachment_malformed", "attachment url is not a valid data: URL"
	case errors.Is(err, attachvalidate.ErrAttachmentTypeUnsupported):
		return http.StatusBadRequest, "attachment_type_unsupported", "attachment type unsupported in v0"
	default:
		// Should not happen — attachvalidate only returns the six sentinels.
		// Defensive: surface as 400 so we don't accidentally 500.
		return http.StatusBadRequest, "bad_request", "attachment validation failed"
	}
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/server/ -run TestSubmit -v && cd ..`
Expected: every Task 9 test passes; existing Phase 1 `TestSubmitHandler_*` tests pass IF they construct Deps with `BodyCapBytes` set. If they fail with `MaxBytesReader limit=0` rejecting the body, update the existing test helpers to set `BodyCapBytes: 1 << 20` on the Deps they construct. This is a one-line change per test helper and is the correct fix — the Phase 1 cap was implicit; Phase 6 makes it explicit.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/server/submit.go relay/internal/server/submit_test.go
git commit -m "$(cat <<'EOF'
feat(6-i): submitHandler — body cap from Deps; 413 split; attachvalidate step

Phase 6 6-i: replaces the literal 1<<20 with deps.BodyCapBytes; distinguishes
*http.MaxBytesError (413 request_body_too_large) from other decode errors
(400 bad_request); short-circuits non-empty attachments[] with 400
attachments_disabled when disabled; calls attachvalidate.ValidateAll between
Builder.Build and Router.Route; maps sentinel errors to 413/415/400.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Create `computeAttachmentsCaps` + extend `initHandler`

**Files:** Create `relay/internal/server/computecaps.go`, `relay/internal/server/computecaps_test.go`; modify `relay/internal/server/turn.go`, `relay/internal/server/turn_test.go`

- [ ] **Step 1: Write the failing test for the compute helper**

Create `relay/internal/server/computecaps_test.go`:

```go
package server

import (
	"reflect"
	"testing"

	"intake/internal/adapter"
	"intake/internal/config"
)

type fakeCapableAdapter struct {
	name string
	caps []string
}

func (f fakeCapableAdapter) Name() string                                           { return f.name }
func (f fakeCapableAdapter) RequiresLicense() bool                                  { return false }
func (f fakeCapableAdapter) Configure(map[string]any) error                         { return nil }
func (f fakeCapableAdapter) Create(any, any) (any, error)                           { return nil, nil }
func (f fakeCapableAdapter) HealthCheck(any) error                                  { return nil }
func (f fakeCapableAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AcceptedMIMETypes: f.caps}
}

// satisfy adapter.Adapter — the test uses ad-hoc adapters that don't need to
// match the full signature; we cast via interface{ Capabilities() adapter.Capabilities }
// in the helper, so a partial method set is fine.
// However computeAttachmentsCaps takes []adapter.Adapter — for the unit test
// here we use real adapter implementations (webhook etc.) to keep the test
// honest. Inline-stub above is sketched for reference; the test below uses
// the real ones.

func TestComputeAttachmentsCaps_DisabledReturnsNil(t *testing.T) {
	cfg := config.AttachmentsConfig{Enabled: false, AllowedMIMETypes: []string{"image/png"}}
	caps := computeAttachmentsCaps(cfg, nil)
	if caps != nil {
		t.Errorf("caps = %v; want nil when disabled", caps)
	}
}

func TestComputeAttachmentsCaps_EmptyAllowlistReturnsNil(t *testing.T) {
	cfg := config.AttachmentsConfig{Enabled: true, AllowedMIMETypes: []string{}}
	caps := computeAttachmentsCaps(cfg, nil)
	if caps != nil {
		t.Errorf("caps = %v; want nil when allowlist empty", caps)
	}
}

func TestComputeAttachmentsCaps_NoCapableAdaptersReturnsNil(t *testing.T) {
	cfg := config.AttachmentsConfig{
		Enabled:          true,
		AllowedMIMETypes: []string{"image/png"},
	}
	caps := computeAttachmentsCaps(cfg, nil) // no adapters
	if caps != nil {
		t.Errorf("caps = %v; want nil when no adapter advertises", caps)
	}
}

func TestComputeAttachmentsCaps_Intersection(t *testing.T) {
	// Real webhook adapter advertises all three; cfg permits two — output is the cfg ∩ adapter.
	cfg := config.AttachmentsConfig{
		Enabled:          true,
		MaxSizeBytes:     5_242_880,
		MaxTotalBytes:    10_485_760,
		AllowedMIMETypes: []string{"image/png", "image/webp"},
	}
	// Construct one real webhook adapter so the test doesn't depend on the
	// fake-adapter shape above.
	wh := newCapableWebhookForTest(t)
	caps := computeAttachmentsCaps(cfg, []adapter.Adapter{wh})
	if caps == nil {
		t.Fatal("caps = nil; want non-nil intersection")
	}
	want := []string{"image/png", "image/webp"}
	if !reflect.DeepEqual(caps.AllowedMIMETypes, want) {
		t.Errorf("AllowedMIMETypes = %v; want %v", caps.AllowedMIMETypes, want)
	}
	if caps.MaxSizeBytes != 5_242_880 || caps.MaxTotalBytes != 10_485_760 {
		t.Errorf("size caps mismatch: %+v", caps)
	}
}

// newCapableWebhookForTest returns a fresh webhook.Adapter (which implements
// adapter.CapableAdapter per Task 6). Kept tiny so callers don't need to import
// webhook directly.
func newCapableWebhookForTest(t *testing.T) adapter.Adapter {
	t.Helper()
	return webhookForTest{}
}

type webhookForTest struct{}

func (webhookForTest) Name() string                          { return "webhook" }
func (webhookForTest) RequiresLicense() bool                 { return false }
func (webhookForTest) Configure(map[string]any) error        { return nil }
func (webhookForTest) Create(ctx any, p any) (any, error)    { return nil, nil } // unused here
func (webhookForTest) HealthCheck(ctx any) error             { return nil }
func (webhookForTest) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"}}
}
```

NOTE: `webhookForTest` does NOT actually satisfy `adapter.Adapter` because `Create` and `HealthCheck` have generic `any` signatures rather than the real types. The cleanest path is to either:
(a) Import the real `webhook` package and use `webhook.New()` directly, OR
(b) Update `webhookForTest` to match the real signatures (`context.Context`, `*payload.IntakePayload`, etc.).

Choose (a) for honesty — the test exercises the real type's `Capabilities()`:

```go
// Replace newCapableWebhookForTest body with:
import "intake/internal/adapter/webhook"
func newCapableWebhookForTest(t *testing.T) adapter.Adapter {
	t.Helper()
	return webhook.New()
}
// And delete webhookForTest + fakeCapableAdapter entirely.
```

Refactor the test file to use the real webhook (matches the production code path the test is meant to cover).

- [ ] **Step 2: Run to verify it fails**

Run: `cd relay && go test ./internal/server/ -run TestComputeAttachmentsCaps -v && cd ..`
Expected: FAIL — `computeAttachmentsCaps undefined`.

- [ ] **Step 3: Create `computecaps.go`**

Create `relay/internal/server/computecaps.go`:

```go
package server

import (
	"intake/internal/adapter"
	"intake/internal/config"
)

// computeAttachmentsCaps returns the CapabilitiesAttachments to advertise on
// /init. Returns nil when:
//   - cfg.Enabled is false, OR
//   - cfg.AllowedMIMETypes is empty, OR
//   - no enabled adapter implements CapableAdapter, OR
//   - the union across enabled adapters has zero overlap with cfg.AllowedMIMETypes.
//
// Otherwise returns the intersection: cfg.AllowedMIMETypes ∩ (union of
// enabled adapters' Capabilities().AcceptedMIMETypes). Order of the result
// follows cfg.AllowedMIMETypes (stable for the widget's enumeration).
//
// Called once at startup from main.go; the result lives in deps.AttachmentMIMEs
// (the published allowlist) and feeds initHandler's Capabilities emission.
func computeAttachmentsCaps(cfg config.AttachmentsConfig, enabled []adapter.Adapter) *CapabilitiesAttachments {
	if !cfg.Enabled || len(cfg.AllowedMIMETypes) == 0 {
		return nil
	}
	adapterUnion := make(map[string]bool)
	for _, ad := range enabled {
		c, ok := ad.(adapter.CapableAdapter)
		if !ok {
			continue
		}
		for _, m := range c.Capabilities().AcceptedMIMETypes {
			adapterUnion[m] = true
		}
	}
	if len(adapterUnion) == 0 {
		return nil
	}
	allowed := make([]string, 0, len(cfg.AllowedMIMETypes))
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

- [ ] **Step 4: Write the failing test for `initHandler` emitting `capabilities.attachments`**

Append to `relay/internal/server/turn_test.go`:

```go
func TestInitHandler_EmitsCapabilitiesAttachmentsWhenEnabled(t *testing.T) {
	deps := Deps{
		Auth:    auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{Modes: config.AuthModes{Anonymous: true}},
		AttachmentsCfg: config.AttachmentsConfig{
			Enabled:          true,
			MaxSizeBytes:     5_242_880,
			MaxTotalBytes:    10_485_760,
			AllowedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
		},
		AttachmentMIMEs: []string{"image/png", "image/jpeg", "image/webp"},
	}
	h := initHandler(deps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body InitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Capabilities.Attachments == nil {
		t.Fatal("Capabilities.Attachments is nil; want populated")
	}
	if body.Capabilities.Attachments.MaxSizeBytes != 5_242_880 {
		t.Errorf("MaxSizeBytes = %d; want 5_242_880", body.Capabilities.Attachments.MaxSizeBytes)
	}
	if len(body.Capabilities.Attachments.AllowedMIMETypes) != 3 {
		t.Errorf("AllowedMIMETypes len = %d; want 3", len(body.Capabilities.Attachments.AllowedMIMETypes))
	}
}

func TestInitHandler_OmitsAttachmentsWhenDisabled(t *testing.T) {
	deps := Deps{
		Auth:    auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{Modes: config.AuthModes{Anonymous: true}},
		AttachmentsCfg: config.AttachmentsConfig{Enabled: false},
		AttachmentMIMEs: nil,
	}
	h := initHandler(deps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"attachments"`) {
		t.Errorf("body contains attachments key; want omitted: %s", rec.Body.String())
	}
}

func TestInitHandler_OmitsAttachmentsWhenIntersectionEmpty(t *testing.T) {
	deps := Deps{
		Auth:    auth.NewMiddleware(auth.NewStore(), nil, nil),
		AuthCfg: config.AuthConfig{Modes: config.AuthModes{Anonymous: true}},
		AttachmentsCfg: config.AttachmentsConfig{Enabled: true, MaxSizeBytes: 1, MaxTotalBytes: 1},
		AttachmentMIMEs: []string{}, // empty intersection
	}
	h := initHandler(deps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/intake/init", strings.NewReader(`{}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"attachments"`) {
		t.Errorf("body contains attachments key on empty intersection; want omitted: %s", rec.Body.String())
	}
}
```

- [ ] **Step 5: Run to verify they fail**

Run: `cd relay && go test ./internal/server/ -run TestInitHandler_(Emits|Omits)CapabilitiesAttachments -v && cd ..`
Expected: FAIL — `initHandler` does not emit `Capabilities.Attachments`.

- [ ] **Step 6: Update `initHandler` in `turn.go`**

In `relay/internal/server/turn.go`, inside `initHandler`, AFTER the captcha block (around line 117, after the `if !ok` Verify branch returns) and BEFORE `sessionID := deps.Auth.Store().Issue()`, the capability struct construction will need the attachments block. The simplest delta is to populate `resp.Capabilities.Attachments` after `resp` is built. REPLACE the existing `resp := InitResponse{...}` block (lines ~121-129) with:

```go
		sessionID := deps.Auth.Store().Issue()

		caps := Capabilities{
			AuthModes:       modes,
			Streaming:       true,
			RequiresCaptcha: requiresCaptcha,
		}
		// Phase 6 (6-i): emit capabilities.attachments when the published
		// allowlist (cfg.AllowedMIMETypes ∩ enabled adapter union, computed
		// once at startup by computeAttachmentsCaps) is non-empty.
		if deps.AttachmentsCfg.Enabled && len(deps.AttachmentMIMEs) > 0 {
			caps.Attachments = &CapabilitiesAttachments{
				MaxSizeBytes:     deps.AttachmentsCfg.MaxSizeBytes,
				MaxTotalBytes:    deps.AttachmentsCfg.MaxTotalBytes,
				AllowedMIMETypes: deps.AttachmentMIMEs,
			}
		}

		resp := InitResponse{
			SessionID:    sessionID,
			Capabilities: caps,
			Captcha:      captchaHint,
		}
```

The remaining Phase 4 email-hint block (`if deps.AuthCfg.Modes.Email { ... }`) stays unchanged below.

- [ ] **Step 7: Run the tests — must pass**

Run: `cd relay && go test ./internal/server/ -v && cd ..`
Expected: every existing Phase 1+4+5 server test passes + the new Task 10 tests pass.

- [ ] **Step 8: Commit**

```bash
git add relay/internal/server/computecaps.go relay/internal/server/computecaps_test.go relay/internal/server/turn.go relay/internal/server/turn_test.go
git commit -m "$(cat <<'EOF'
feat(6-i): computeAttachmentsCaps + initHandler emits capabilities.attachments

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: Extend the Q9 startup gate — `validateAttachments` (L016 returns parsed cfg)

**Files:** Modify `relay/cmd/relay/main.go`, `relay/cmd/relay/main_test.go`

The pattern mirrors the existing `startupProblems(cfg) ([]string, []netip.Prefix)` (`main.go:534`). Add a sibling `validateAttachments(cfg, enabled) (config.AttachmentsConfig, []string)` that returns the PARSED config and any problems. main() appends those problems to the existing slice so all misconfigs fire in one log line; uses the returned parsed value for the rest of startup.

- [ ] **Step 1: Write the failing tests**

Append to `relay/cmd/relay/main_test.go`:

```go
func TestValidateAttachments_CleanConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 5_242_880
	cfg.Attachments.MaxTotalBytes = 10_485_760
	cfg.Attachments.AllowedMIMETypes = []string{"image/png", "image/jpeg", "image/webp"}
	cfg.Attachments.Storage.Mode = "forward"

	parsed, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty", problems)
	}
	if parsed.MaxSizeBytes != 5_242_880 {
		t.Errorf("parsed.MaxSizeBytes = %d; want 5_242_880", parsed.MaxSizeBytes)
	}
}

func TestValidateAttachments_BadStorageMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.Storage.Mode = "s3"
	cfg.Attachments.MaxSizeBytes = 1
	cfg.Attachments.MaxTotalBytes = 1
	cfg.Attachments.AllowedMIMETypes = []string{"image/png"}

	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "storage.mode") || !strings.Contains(problems[0], "s3") {
		t.Errorf("problem %q does not mention storage.mode + s3", problems[0])
	}
}

func TestValidateAttachments_CapInverted(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000
	cfg.Attachments.AllowedMIMETypes = []string{"image/png"}

	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "max_size_bytes") || !strings.Contains(problems[0], "max_total_bytes") {
		t.Errorf("problem %q does not name both caps", problems[0])
	}
}

func TestValidateAttachments_CapInvertedSkippedWhenTotalZero(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 0 // disabled aggregate cap; per-attachment is the only gate
	cfg.Attachments.AllowedMIMETypes = []string{"image/png"}

	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty when total cap is 0", problems)
	}
}

func TestValidateAttachments_UnknownMIMETypeIsWarnNotFatal(t *testing.T) {
	// The unknown-MIME case logs a slog.Warn but does NOT add to problems.
	// We can't easily inspect slog output here without a custom handler;
	// the contract is: problems remains empty when only unknown-MIME is
	// present. The warning's side-channel is exercised in main_test_warn.go
	// (or omitted — the test above covers the gate's silence; a Smoke test
	// confirms the log line shape).
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 1
	cfg.Attachments.MaxTotalBytes = 1
	cfg.Attachments.AllowedMIMETypes = []string{"image/heic"} // no adapter advertises this
	// Empty enabled adapter list → empty union → every MIME is "unknown".
	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty (unknown-MIME is warn-not-fatal)", problems)
	}
}

func TestValidateAttachments_DisabledShortCircuit(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = false
	cfg.Attachments.Storage.Mode = "s3" // would normally be fatal — but disabled trumps
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000

	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty when Enabled=false", problems)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./cmd/relay/... -run TestValidateAttachments -v && cd ..`
Expected: FAIL — `validateAttachments undefined`.

- [ ] **Step 3: Add `validateAttachments` to `main.go`**

In `relay/cmd/relay/main.go`, AFTER `containsString` (end of file, ~line 605), ADD:

```go
// validateAttachments validates the Phase 6 attachments block. Returns the
// parsed/defaulted AttachmentsConfig per L016 — consumers (computeAttachmentsCaps,
// attachvalidate.Config) MUST use the returned value rather than re-reading cfg.
//
// Gates (fatal — append to problems):
//   - Storage.Mode set to anything other than "" or "forward".
//   - MaxTotalBytes > 0 AND MaxSizeBytes > MaxTotalBytes (an inverted cap pair
//     is always operator error; the per-attachment cap is unreachable).
//
// Side-channel (warn-not-fatal):
//   - AllowedMIMETypes contains a MIME that no enabled adapter advertises.
//     This is legitimate (operator may want a stricter allowlist than any
//     single adapter), so it emits slog.Warn rather than blocking startup.
//
// When Enabled=false the function is a no-op (returns the cfg unchanged with
// zero problems) — a disabled feature shouldn't fail startup.
func validateAttachments(cfg *config.Config, enabled []adapter.Adapter) (config.AttachmentsConfig, []string) {
	parsed := cfg.Attachments
	if !parsed.Enabled {
		return parsed, nil
	}

	var problems []string

	// Gate 1: storage.mode.
	switch parsed.Storage.Mode {
	case "", "forward":
		// OK.
	default:
		problems = append(problems, fmt.Sprintf("attachments.storage.mode=%q is not supported in v0; only \"\" or \"forward\" is supported (S3 storage is v1+)", parsed.Storage.Mode))
	}

	// Gate 2: cap pair sanity.
	if parsed.MaxTotalBytes > 0 && parsed.MaxSizeBytes > parsed.MaxTotalBytes {
		problems = append(problems, fmt.Sprintf("attachments.max_size_bytes=%d exceeds attachments.max_total_bytes=%d; per-attachment cap must be <= aggregate cap", parsed.MaxSizeBytes, parsed.MaxTotalBytes))
	}

	// Warn (not fatal): MIMEs in the allowlist that no adapter advertises.
	if len(parsed.AllowedMIMETypes) > 0 {
		adapterUnion := make(map[string]bool)
		for _, ad := range enabled {
			if c, ok := ad.(adapter.CapableAdapter); ok {
				for _, m := range c.Capabilities().AcceptedMIMETypes {
					adapterUnion[m] = true
				}
			}
		}
		var unknown []string
		for _, m := range parsed.AllowedMIMETypes {
			if !adapterUnion[m] {
				unknown = append(unknown, m)
			}
		}
		if len(unknown) > 0 {
			slog.Warn("relay: attachments.allowed_mime_types contains types no enabled adapter advertises; widget will hide these",
				"unknown", unknown,
				"enabled_adapters", adapterNames(adapterRegistryFromSlice(enabled)),
			)
		}
	}

	return parsed, problems
}

// adapterRegistryFromSlice is a tiny shim so we can reuse adapterNames (which
// already exists in this file and takes map[string]adapter.Adapter) from a
// []adapter.Adapter input.
func adapterRegistryFromSlice(enabled []adapter.Adapter) map[string]adapter.Adapter {
	out := make(map[string]adapter.Adapter, len(enabled))
	for _, ad := range enabled {
		out[ad.Name()] = ad
	}
	return out
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./cmd/relay/... -run TestValidateAttachments -v && cd ..`
Expected: all 6 tests pass.

- [ ] **Step 5: Wire the gate into `main()`**

In `relay/cmd/relay/main.go`, find the Phase 5 Q9 block (lines 74-83). Currently it reads:

```go
	problems, trustedProxies := startupProblems(cfg)
	if len(problems) > 0 {
		logger.Error("relay: startup config errors", "count", len(problems), "problems", problems)
		os.Exit(1)
	}
```

The attachments gate must run AFTER the registry has been built (so it knows which adapters are enabled — needed for the warn-on-unknown-MIME path). But the existing Q9 gate runs BEFORE registry construction. The solution is to ADD a second gate call right after `buildRegistry` (around line 225), collecting its problems and merging.

Find this existing block (lines ~224-233):

```go
	// --- Adapter registry (3-i; 3-ii…3-v add adapters; 3-vi adds the license gate) ---
	registry, err := buildRegistry(cfg, licState, logger)
	if err != nil {
		logger.Error("relay: adapter registry build failed", "error", err)
		os.Exit(1)
	}
	if len(registry) == 0 {
		logger.Error("relay: no adapters enabled — enable at least one in config.adapters")
		os.Exit(1)
	}
```

INSERT immediately AFTER it:

```go
	// --- Phase 6 attachments startup gate ---
	// Runs after buildRegistry because the warn-on-unknown-MIME side-channel
	// needs the enabled-adapter list. Returns the PARSED AttachmentsConfig per
	// L016 — consumers below must use `attachmentsCfg`, not `cfg.Attachments`.
	enabledList := make([]adapter.Adapter, 0, len(registry))
	for _, ad := range registry {
		enabledList = append(enabledList, ad)
	}
	attachmentsCfg, attProblems := validateAttachments(cfg, enabledList)
	if len(attProblems) > 0 {
		logger.Error("relay: startup config errors", "count", len(attProblems), "problems", attProblems)
		os.Exit(1)
	}
	// Compute the published allowlist (cfg ∩ adapter union) — empty list means
	// /init will omit capabilities.attachments and submitHandler will refuse
	// non-empty attachments[] with 400 attachments_disabled.
	caps := server.ComputeAttachmentsCaps(attachmentsCfg, enabledList) // exported wrapper
	var attachmentMIMEs []string
	if caps != nil {
		attachmentMIMEs = caps.AllowedMIMETypes
	}
	// Body cap: 14 MB when enabled, 1 MB otherwise. Computed once at startup.
	bodyCapBytes := int64(1 << 20)
	if attachmentsCfg.Enabled {
		bodyCapBytes = 14 * (1 << 20)
	}
```

NOTE on `server.ComputeAttachmentsCaps`: Task 10 created the helper as unexported `computeAttachmentsCaps` (used by `initHandler` in the same package). main.go is in package `main`, not `server`, so it cannot call the unexported form. The cleanest fix is to add an exported wrapper at the bottom of `relay/internal/server/computecaps.go`:

```go
// ComputeAttachmentsCaps is the exported wrapper for main.go. The unexported
// computeAttachmentsCaps is used by initHandler in the same package; this
// wrapper lets cmd/relay/main.go reuse the same intersection logic so the
// published allowlist on /init exactly matches the allowlist submitHandler
// uses to gate incoming attachments.
func ComputeAttachmentsCaps(cfg config.AttachmentsConfig, enabled []adapter.Adapter) *CapabilitiesAttachments {
	return computeAttachmentsCaps(cfg, enabled)
}
```

Make this exported-wrapper change as part of Task 11 (one-line edit to `computecaps.go`).

- [ ] **Step 6: Populate the new Deps fields**

In `main.go`, find the existing `deps := server.Deps{...}` block (around line 311). ADD the three new fields at the END (before the closing brace), replacing the existing close pattern:

```go
	deps := server.Deps{
		// ... all existing Phase 1-5 fields unchanged ...

		// Phase 6 (6-i):
		AttachmentsCfg:  attachmentsCfg,
		AttachmentMIMEs: attachmentMIMEs,
		BodyCapBytes:    bodyCapBytes,
	}
```

- [ ] **Step 7: Build, vet, and run the full suite**

Run: `cd relay && go build ./... && go vet ./... && cd ..`
Expected: build + vet pass.

Run: `cd relay && go test ./... && cd ..`
Expected: every existing test + every new Phase 6 test passes.

Run: `cd relay && go mod tidy && cd ..`
Then: `git diff relay/go.mod relay/go.sum`
Expected: no changes (Phase 6 is stdlib-only).

Run: `bash scripts/verify-contract.sh`
Expected: exits 0.

- [ ] **Step 8: Commit**

```bash
git add relay/cmd/relay/main.go relay/cmd/relay/main_test.go relay/internal/server/computecaps.go
git commit -m "$(cat <<'EOF'
feat(6-i): validateAttachments gate + main.go wires Deps.{AttachmentsCfg,AttachmentMIMEs,BodyCapBytes}

Phase 6 6-i: extends the Q9 consolidated startup gate with attachments-block
validation (storage.mode, cap-pair sanity, unknown-MIME warn-not-fatal). Returns
the parsed AttachmentsConfig per L016. main.go runs the gate AFTER buildRegistry
so the warn-on-unknown-MIME side-channel knows the enabled-adapter set, computes
the published allowlist via ComputeAttachmentsCaps once at startup, sizes
BodyCapBytes per Enabled, and populates Deps.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Smoke (mandatory)

**Self-runnable; no LLM credit; no maintainer pause.** Proves the 6-i seam works end-to-end at the wire level.

1. **Caps-discovery smoke — `/init` emits the new `capabilities.attachments` field.**

   Author `relay/cmd/relay/smoke/attachments-clean.yaml`:

   ```yaml
   server:
     addr: ":18086"
     external_url: "http://127.0.0.1:18086"
     cors_origins: ["http://localhost:5173"]
   auth:
     modes:
       anonymous: true
     anonymous:
       allow_without_captcha: true
   adapters:
     webhook:
       enabled: true
       url: "http://127.0.0.1:1/discard"
   routing:
     default_adapter: "webhook"
   ratelimit:
     daily_llm_budget:
       action_on_exceeded: "reject"
   attachments:
     enabled: true
     max_size_bytes: 5242880
     max_total_bytes: 10485760
     allowed_mime_types: ["image/png", "image/jpeg", "image/webp"]
     storage:
       mode: "forward"
   ```

   (Author with PowerShell `Set-Content -Encoding ascii` per L010.)

   Run (PowerShell):
   ```
   cd relay
   $proc = Start-Process -PassThru -FilePath "go" -ArgumentList "run ./cmd/relay --config smoke/attachments-clean.yaml"
   Start-Sleep -Seconds 2
   $body = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18086/v1/intake/init" -ContentType "application/json" -Body "{}"
   $body.capabilities.attachments | ConvertTo-Json
   Stop-Process -Id $proc.Id
   ```
   Expected output: a JSON object with `max_size_bytes: 5242880`, `max_total_bytes: 10485760`, and `allowed_mime_types: ["image/png","image/jpeg","image/webp"]`.

2. **Body-cap smoke — > 14 MB returns 413 `request_body_too_large`.**

   With the same `attachments-clean.yaml` relay running, POST a 15 MB body to `/v1/intake/submit`:

   ```
   $session = (Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18086/v1/intake/init" -ContentType "application/json" -Body "{}").session_id
   $big = "a" * (15 * 1024 * 1024)
   try {
     Invoke-WebRequest -Method Post -Uri "http://127.0.0.1:18086/v1/intake/submit" `
       -ContentType "application/json" `
       -Headers @{"X-Intake-Session"=$session} `
       -Body $big
   } catch {
     $_.Exception.Response.StatusCode.value__
     $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
     $reader.ReadToEnd()
   }
   ```
   Expected: HTTP 413 + body containing `"code":"request_body_too_large"`.

3. **Body-cap regression smoke — with `enabled: false`, 2 MB body returns 413 `request_body_too_large`.**

   Author `relay/cmd/relay/smoke/attachments-disabled.yaml` (same as `attachments-clean.yaml` but `attachments.enabled: false`). Spin up the relay; POST a 2 MB body to `/v1/intake/submit`. Expected: HTTP 413 — the 1 MB Phase 1 cap is preserved when attachments are off.

4. **Q9 startup gate smoke — `storage.mode: "s3"` and inverted caps each exit 1 with the expected problem string in one log line.**

   Author `relay/cmd/relay/smoke/attachments-bad-storage-mode.yaml` (sets `storage.mode: "s3"`) and `relay/cmd/relay/smoke/attachments-cap-inverted.yaml` (sets `max_size_bytes: 20000000, max_total_bytes: 10000000`). For each:

   ```
   cd relay; go run ./cmd/relay --config smoke/attachments-bad-storage-mode.yaml; $LASTEXITCODE
   ```
   Expected: exit code 1; stdout contains a line `"msg":"relay: startup config errors"` listing the matching problem text.

   Author `relay/cmd/relay/smoke/attachments-combined.yaml` setting BOTH misconfigs. Expected: exit code 1; ONE log line listing BOTH problem texts (operator fixes both in one restart cycle).

5. **Adapter `Capabilities()` smoke — every enabled adapter advertises the v0 list.**

   Already covered by Task 5's `TestCapableAdapter_AllFiveAdaptersAdvertiseV0List`. Re-run `cd relay && go test ./internal/adapter/... -v` to confirm.

6. **Phase 1+4+5 regression — anonymous + email + SSO smokes pass unchanged.**

   With `attachments.enabled: true` and empty `attachments[]` in the request body, re-run the Phase 4 drivers (`drive-auth-email.ts`, `drive-auth-sso.ts`) and the Phase 5 driver (`drive-abuse.ts`). Expected: every existing assertion passes. Confirms 6-i changes are additive on the non-attachment path.

---

## Done criteria

- [ ] All 11 tasks complete and committed.
- [ ] `cd relay && go build ./... && go vet ./...` is clean.
- [ ] `cd relay && go test ./...` is green (all Phase 1+4+5 tests + all new Phase 6-i tests pass; no skipped tests).
- [ ] `cd relay && go mod tidy` is a no-op (no spurious dep changes; 6-i is stdlib-only).
- [ ] `bash scripts/verify-contract.sh` is green (schema unchanged in Phase 6).
- [ ] `bash scripts/check-pins.sh` is green (`html2canvas` pin lands in 6-iii, not here).
- [ ] All 6 smoke steps pass.
- [ ] The frozen Phase 1+3+4+5 seams are byte-equivalent in shape: `adapter.Adapter` (interface unchanged — `Capabilities()` is on a SEPARATE optional `CapableAdapter` interface), `payload.IntakePayload` / `payload.Attachment` generated types (untouched), `schema/payload.v1.json` (untouched), `auth.Middleware.Handler` / `auth.SessionContext` (untouched), Phase 5 `Deps` Phase-5 fields (additive only — 6-i adds three new fields at the bottom).
- [ ] `validateAttachments` returns the parsed `AttachmentsConfig` per L016 — consumers (`computeAttachmentsCaps`, `BodyCapBytes` sizing, `Deps.AttachmentsCfg`) use the returned value; no re-parse-with-discarded-error.
- [ ] The 413 `request_body_too_large` path REPLACES the prior 400 path for over-cap bodies (malformed JSON stays 400) per the Q-G design resolution.
- [ ] The `attachments_disabled` short-circuit runs BEFORE `attachvalidate.ValidateAll` so an operator's intent surfaces a clear error even if the bytes would have otherwise passed.
- [ ] `attachvalidate.ValidateAll` is called AFTER `Builder.Build` and BEFORE `Router.Route` (order-of-operations regression locked in by `TestSubmit_AttachvalidateFails_RouterNotCalled`).
- [ ] L010 (PowerShell ascii encoding) applied to every new YAML written via `Set-Content`/`Out-File`.
- [ ] No new external Go module in `relay/go.mod` and no new TS/npm module in `core/package.json` (the `html2canvas` pin is owned by 6-iii).
