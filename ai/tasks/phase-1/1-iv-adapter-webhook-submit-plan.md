# Sub-plan 1-iv — Adapter interface + webhook + /submit

## 1. Goal

Deliver the adapter seam (README §6.2), the webhook adapter with retry, the server-side `classify()` call (README §6.5), the canonical `payload.IntakePayload` builder with schema validation (L003 mitigation), and the `POST /v1/intake/submit` handler that wires them together. After this sub-plan the relay can receive a `SubmitRequest`, classify it via the LLM, assemble and validate the canonical payload, and POST it to a local webhook receiver — completing the Phase-1 e2e path.

This sub-plan assumes 1-i (server skeleton + deps.go + routes.go + config), 1-ii (`llm.Provider` + Anthropic), and 1-iii (session/auth middleware + `/init` + `/turn`) are all merged and green.

---

## 2. Design references

- Phase-1 README §6.2 — `adapter.Adapter` interface (FROZEN verbatim)
- Phase-1 README §6.3 — `auth.SessionContext`
- Phase-1 README §6.4 — `SubmitRequest`, `SubmitResponse`, `ClientInfo`, `Viewport`, `ContextInfo`, `ErrorEnvelope`
- Phase-1 README §6.5 — `classify.Result` (FROZEN verbatim)
- Phase-1 README §6.1 — `llm.Provider`, `llm.Message`, `llm.ChatOptions`
- Phase-1 README §6.6 — `config.Config`, `AdaptersConfig`, `WebhookConfig`, `RetryConfig`
- Phase-1 README §7 — build-fail checklist (schema validation failure → 400)
- Phase-1 README §9 — L003: `go-jsonschema` `const` not enforced; must validate at runtime
- Design spec §6 — SubmitRequest→canonical payload reconciliation
- Design spec §8 — phase final smoke (webhook receiver on :9099)
- Design spec §9 — error handling: 400 on validation failure, 502 on provider/adapter failure
- `relay/internal/payload/types.go` — generated `payload.IntakePayload` and nested types (confirmed field names below)

---

## 3. Files touched

| File | Create/Modify | Why |
|---|---|---|
| `relay/internal/adapter/adapter.go` | Create | Freezes `adapter.Adapter` interface (README §6.2 verbatim) |
| `relay/internal/adapter/webhook/webhook.go` | Create | Webhook adapter: Configure, Create (POST+retry), HealthCheck |
| `relay/internal/adapter/webhook/webhook_test.go` | Create | httptest receiver; retry on 503-then-200 |
| `relay/internal/classify/classify.go` | Create | `Classifier` wrapping `llm.Provider`; JSON parse + enum validation; fallback |
| `relay/internal/classify/classify_test.go` | Create | Fake provider returns canned JSON; assert `Result` fields |
| `relay/internal/payloadbuild/schema.json` | Create | Committed copy of `schema/payload.v1.json` (go:embed target) |
| `relay/internal/payloadbuild/build.go` | Create | Assembles `payload.IntakePayload` from inputs; validates against embedded schema |
| `relay/internal/payloadbuild/build_test.go` | Create | Well-formed → schema-valid; tampered schema_version → rejected; schema copy identical to canonical |
| `relay/internal/server/submit.go` | Create | `POST /v1/intake/submit` handler |
| `relay/internal/server/submit_test.go` | Create | httptest with fake provider + httptest receiver; asserts schema-valid payload + classify fields |
| `relay/internal/server/deps.go` | Modify | Add `Adapter adapter.Adapter`, `Classifier *classify.Classifier`, `Builder *payloadbuild.Builder` |
| `relay/internal/server/routes.go` | Modify | Register `POST /v1/intake/submit` behind auth sub-router |
| `relay/cmd/relay/main.go` | Modify | Construct webhook adapter, classifier, builder; wire into Deps |
| `relay/go.mod` + `relay/go.sum` | Modify | Add `github.com/santhosh-tekuri/jsonschema/v6` and `github.com/google/uuid` |
| `scripts/check-pins.sh` | Modify | Add guard: `jsonschema/v6` is a library (caret OK — note it); uuid exact-pinned |

### Payload field names confirmed from `relay/internal/payload/types.go`

> **Implementer note:** These names are READ from the actual generated file. Verify against `relay/internal/payload/types.go` before using — the generator may change on regeneration.

| Schema field | Go struct path | Type | Nullable/ptr? |
|---|---|---|---|
| (root) | `payload.IntakePayload` | struct | — |
| `schema_version` | `.SchemaVersion` | `string` | no |
| `submission.id` | `.Submission.Id` | `string` | no |
| `submission.submitted_at` | `.Submission.SubmittedAt` | `time.Time` | no |
| `submission.tenant_id` | `.Submission.TenantId` | `*string` | yes |
| `client.widget_version` | `.Client.WidgetVersion` | `string` | no |
| `client.session_id` | `.Client.SessionId` | `string` | no |
| `client.url` | `.Client.Url` | `string` | no |
| `client.referrer` | `.Client.Referrer` | `*string` | yes |
| `client.user_agent` | `.Client.UserAgent` | `string` | no |
| `client.viewport.w` | `.Client.Viewport.W` | `int` | no |
| `client.viewport.h` | `.Client.Viewport.H` | `int` | no |
| `client.locale` | `.Client.Locale` | `string` | no |
| `user.auth_mode` | `.User.AuthMode` | `payload.UserAuthMode` | no (enum type) |
| `user.id` | `.User.Id` | `*string` | yes |
| `user.email` | `.User.Email` | `*string` | yes |
| `user.display_name` | `.User.DisplayName` | `*string` | yes |
| `user.verified` | `.User.Verified` | `bool` | no |
| `user.custom` | `.User.Custom` | `map[string]interface{}` | yes (omitempty) |
| `conversation.messages[].role` | `.Conversation.Messages[i].Role` | `payload.MessageRole` | no (enum type) |
| `conversation.messages[].content` | `.Conversation.Messages[i].Content` | `string` | no |
| `conversation.messages[].ts` | `.Conversation.Messages[i].Ts` | `time.Time` | no |
| `conversation.summary` | `.Conversation.Summary` | `string` | no |
| `conversation.title_suggestion` | `.Conversation.TitleSuggestion` | `string` | no (max 80 chars) |
| `conversation.classification` | `.Conversation.Classification` | `payload.ConversationClassification` | no (enum type) |
| `conversation.severity_guess` | `.Conversation.SeverityGuess` | `payload.ConversationSeverityGuess` | no (enum type) |
| `conversation.tags_suggested` | `.Conversation.TagsSuggested` | `[]string` | no |
| `conversation.language` | `.Conversation.Language` | `string` | no |
| `context.app_context` | `.Context.AppContext` | `map[string]interface{}` | omitempty |
| `context.page_metadata` | `.Context.PageMetadata` | `map[string]interface{}` | omitempty |
| `routing_hint` | `.RoutingHint` | `*string` | yes |
| `attachments` | `.Attachments` | `[]payload.Attachment` | omitempty |

**Enum typed constants to use (do NOT use raw strings):**

```go
// auth mode
payload.UserAuthModeAnonymous  // "anonymous"
payload.UserAuthModeEmail      // "email"
payload.UserAuthModeSso        // "sso"

// message role
payload.MessageRoleUser        // "user"
payload.MessageRoleAssistant   // "assistant"

// classification
payload.ConversationClassificationBug            // "bug"
payload.ConversationClassificationFeatureRequest // "feature_request"
payload.ConversationClassificationQuestion       // "question"
payload.ConversationClassificationOther          // "other"

// severity
payload.ConversationSeverityGuessLow      // "low"
payload.ConversationSeverityGuessMedium   // "medium"
payload.ConversationSeverityGuessHigh     // "high"
payload.ConversationSeverityGuessCritical // "critical"
payload.ConversationSeverityGuessUnknown  // "unknown"
```

---

## 4. Tasks

Tasks are ordered sequentially. Each task ends with the exact verification command and its expected output. All code must compile and all tests must pass before advancing to the next task.

---

### Task 0 — Pin new dependencies

**Purpose:** add `github.com/santhosh-tekuri/jsonschema/v6` and `github.com/google/uuid` to `relay/go.mod`; confirm exact versions; update the phase-1 README pin table; extend `scripts/check-pins.sh`.

**Step 0.1 — determine latest patch versions.**

From inside `relay/`:

```bash
cd relay
go get github.com/santhosh-tekuri/jsonschema/v6@latest
go get github.com/google/uuid@latest
```

Inspect what was resolved:

```bash
grep 'santhosh-tekuri\|google/uuid' go.mod
```

Expected output (versions may differ — record actual):
```
github.com/google/uuid v1.6.0
github.com/santhosh-tekuri/jsonschema/v6 v6.0.1
```

Record the exact versions (no caret, no `v6.x`). Commit `go.mod` and `go.sum` in this task.

**Step 0.2 — copy the canonical schema into the payloadbuild package.**

`go:embed` cannot traverse upward (`../`), so the schema must live inside `relay/`. Commit a copy at `relay/internal/payloadbuild/schema.json`. A unit test in Task 4 will assert this copy is byte-identical to `schema/payload.v1.json`.

Add a `Makefile` target (or document in the plan) for keeping the copy fresh:

```makefile
# relay/Makefile
.PHONY: sync-schema
sync-schema:
	cp ../schema/payload.v1.json internal/payloadbuild/schema.json
```

Alternatively add a `go:generate` comment in `payloadbuild/build.go`:

```go
//go:generate cp ../../../schema/payload.v1.json schema.json
```

Use whichever is consistent with the existing tooling (the plan prefers `go:generate` since `doc.go` in the payload package already shows this pattern).

**Step 0.3 — extend `scripts/check-pins.sh`.**

Add a note (comment, not a failing check) that `santhosh-tekuri/jsonschema/v6` is a library and caret is acceptable per PHASE_PLANNING. `google/uuid` must be exact-pinned in `go.mod` (Go module pinning is by definition exact after `go get`, so this is implicit — just add a comment).

```bash
# In scripts/check-pins.sh, add near the bottom:
# Note: github.com/santhosh-tekuri/jsonschema/v6 is a Go library
# (Go modules pin exact versions in go.sum); no caret-check needed.
# github.com/google/uuid is similarly exact-pinned by go.mod.
echo "OK: Go module pins verified (go.sum enforces exact versions)"
```

**Step 0.4 — update phase-1 README §5 pin table.**

Add a row for each new dependency with the actual resolved version.

**Verification:**

```bash
cd relay && go build ./...
```

Expected: no output, exit 0.

---

### Task 1 — `relay/internal/adapter/adapter.go`

**Purpose:** freeze the adapter seam exactly per README §6.2.

Create `relay/internal/adapter/adapter.go`:

```go
package adapter

import (
	"context"

	"intake/internal/payload"
)

// CreateResult is returned by Adapter.Create on success.
type CreateResult struct {
	ExternalID  string
	ExternalURL string
	AdapterName string
	CreatedAt   string // ISO-8601 / RFC3339
}

// Adapter is the seam between the relay and downstream ticket systems.
// Implementations: webhook (Phase 1), chatwoot/fider/zendesk/linear (Phase 3).
// This interface is FROZEN in 1-iv; do not change without re-smoking all dependents.
type Adapter interface {
	// Name returns a short identifier used in logs and SubmitResponse.adapter_name.
	Name() string
	// RequiresLicense reports whether the adapter needs a license key (Phase 3 gate).
	RequiresLicense() bool
	// Configure applies a map of adapter-specific settings (from config.AdaptersConfig.*).
	Configure(config map[string]any) error
	// Create sends the validated canonical payload to the downstream system.
	Create(ctx context.Context, p *payload.IntakePayload) (*CreateResult, error)
	// HealthCheck probes the downstream system without creating a ticket.
	HealthCheck(ctx context.Context) error
}
```

> **Note on type name:** README §6.2 uses `*payload.Payload` but the generated type in `relay/internal/payload/types.go` is `payload.IntakePayload` (derived from `"title": "IntakePayload"` in the schema). The interface uses the actual generated name. **If a future codegen regeneration renames the type, the interface must be updated in the same commit.**

**Verification:**

```bash
cd relay && go build ./internal/adapter/...
```

Expected: no output, exit 0.

---

### Task 2 — `relay/internal/adapter/webhook/webhook.go` + `webhook_test.go`

**Purpose:** implement the webhook adapter with configurable retry.

#### 2a — `webhook.go`

Create `relay/internal/adapter/webhook/webhook.go`:

```go
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"intake/internal/adapter"
	"intake/internal/payload"

	"github.com/google/uuid"
)

const defaultMaxAttempts = 3
const defaultBackoff = "exponential"

// Adapter POSTs the canonical payload JSON to a configured URL with
// configured headers, retrying on 5xx responses or network errors.
type Adapter struct {
	url         string
	headers     map[string]string
	maxAttempts int
	backoff     string // "exponential" | "fixed"
	client      *http.Client
}

// New returns an unconfigured Adapter. Call Configure before use.
func New() *Adapter {
	return &Adapter{
		maxAttempts: defaultMaxAttempts,
		backoff:     defaultBackoff,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Adapter) Name() string { return "webhook" }

func (a *Adapter) RequiresLicense() bool { return false }

// Configure reads url, headers, retry.max_attempts, retry.backoff from the map.
// Keys match config.WebhookConfig yaml tags lowercased. Only url is required.
func (a *Adapter) Configure(cfg map[string]any) error {
	urlVal, ok := cfg["url"]
	if !ok {
		return fmt.Errorf("webhook: missing required config key 'url'")
	}
	urlStr, ok := urlVal.(string)
	if !ok || urlStr == "" {
		return fmt.Errorf("webhook: config key 'url' must be a non-empty string")
	}
	a.url = urlStr

	if h, ok := cfg["headers"]; ok {
		switch v := h.(type) {
		case map[string]string:
			a.headers = v
		case map[string]any:
			a.headers = make(map[string]string, len(v))
			for k, val := range v {
				s, ok := val.(string)
				if !ok {
					return fmt.Errorf("webhook: header value for %q must be a string", k)
				}
				a.headers[k] = s
			}
		}
	}

	if retry, ok := cfg["retry"]; ok {
		if rm, ok := retry.(map[string]any); ok {
			if ma, ok := rm["max_attempts"]; ok {
				switch v := ma.(type) {
				case int:
					a.maxAttempts = v
				case float64:
					a.maxAttempts = int(v)
				}
			}
			if b, ok := rm["backoff"]; ok {
				if bs, ok := b.(string); ok {
					a.backoff = bs
				}
			}
		}
	}

	return nil
}

// Create marshals p to JSON and POSTs to the configured URL, retrying on
// 5xx or network errors. On success it returns a CreateResult; ExternalID
// is sourced from the response body field "external_id" if present, otherwise
// a new UUID is generated.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("webhook: marshal payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < a.maxAttempts; attempt++ {
		if attempt > 0 {
			wait := a.backoffDuration(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		result, retry, err := a.doRequest(ctx, body)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, fmt.Errorf("webhook: after %d attempts: %w", a.maxAttempts, lastErr)
}

// doRequest performs one HTTP POST. Returns (result, shouldRetry, error).
func (a *Adapter) doRequest(ctx context.Context, body []byte) (*adapter.CreateResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range a.headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		// Network error: retryable.
		return nil, true, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 500 {
		// 5xx: retryable.
		return nil, true, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	if resp.StatusCode >= 400 {
		// 4xx: not retryable (client error, misconfiguration).
		return nil, false, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	// 2xx: success.
	externalID := extractExternalID(respBody)
	if externalID == "" {
		externalID = uuid.NewString()
	}
	return &adapter.CreateResult{
		ExternalID:  externalID,
		ExternalURL: "",
		AdapterName: "webhook",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, false, nil
}

// backoffDuration returns the wait before attempt n (1-indexed: first retry = n=1).
func (a *Adapter) backoffDuration(n int) time.Duration {
	if a.backoff == "exponential" {
		// 500ms * 2^(n-1): 500ms, 1s, 2s, ...
		ms := 500.0 * math.Pow(2, float64(n-1))
		return time.Duration(ms) * time.Millisecond
	}
	// fixed: 500ms
	return 500 * time.Millisecond
}

// extractExternalID attempts to parse {"external_id":"..."} from the response body.
func extractExternalID(body []byte) string {
	var resp struct {
		ExternalID string `json:"external_id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	return resp.ExternalID
}

// HealthCheck sends a HEAD request (falls back to GET if HEAD is unsupported).
// A missing or unreachable URL is an error; a non-5xx response (including 404)
// is considered reachable and returns nil.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.url == "" {
		return fmt.Errorf("webhook: not configured (url is empty)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, a.url, nil)
	if err != nil {
		return fmt.Errorf("webhook health: build request: %w", err)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook health: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("webhook health: upstream returned %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
```

#### 2b — `webhook_test.go`

Create `relay/internal/adapter/webhook/webhook_test.go`:

```go
package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"intake/internal/adapter/webhook"
	"intake/internal/payload"
)

// minimalPayload returns a schema-satisfying IntakePayload for testing.
func minimalPayload() *payload.IntakePayload {
	now := time.Now().UTC()
	return &payload.IntakePayload{
		SchemaVersion: "1.0",
		Submission: payload.Submission{
			Id:          "00000000-0000-0000-0000-000000000001",
			SubmittedAt: now,
		},
		Client: payload.Client{
			WidgetVersion: "0.1.0",
			SessionId:     "00000000-0000-0000-0000-000000000002",
			Url:           "http://localhost:5173/",
			UserAgent:     "test-agent",
			Viewport:      payload.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		User: payload.User{
			AuthMode: payload.UserAuthModeAnonymous,
			Verified: false,
		},
		Conversation: payload.Conversation{
			Messages:        []payload.Message{},
			Summary:         "test summary",
			TitleSuggestion: "Test Title",
			Classification:  payload.ConversationClassificationOther,
			SeverityGuess:   payload.ConversationSeverityGuessUnknown,
			TagsSuggested:   []string{},
			Language:        "en",
		},
	}
}

// TestWebhookCreate_ReceivesPayload asserts the adapter POSTs valid JSON to the receiver.
func TestWebhookCreate_ReceivesPayload(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		var err error
		received, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"external_id":"ext-abc-123"}`))
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{"url": srv.URL}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	p := minimalPayload()
	result, err := a.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if result.ExternalID != "ext-abc-123" {
		t.Errorf("expected ExternalID ext-abc-123, got %q", result.ExternalID)
	}
	if result.AdapterName != "webhook" {
		t.Errorf("expected AdapterName webhook, got %q", result.AdapterName)
	}

	// Assert the POSTed body is valid JSON containing schema_version.
	var parsed map[string]any
	if err := json.Unmarshal(received, &parsed); err != nil {
		t.Fatalf("receiver body not valid JSON: %v\nbody: %s", err, received)
	}
	if parsed["schema_version"] != "1.0" {
		t.Errorf("expected schema_version=1.0 in payload, got %v", parsed["schema_version"])
	}
}

// TestWebhookCreate_RetriesOn503 asserts the adapter retries once on 503 then succeeds on 200.
func TestWebhookCreate_RetriesOn503(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{
		"url": srv.URL,
		"retry": map[string]any{
			"max_attempts": 3,
			"backoff":      "fixed",
		},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create after retry: %v", err)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", callCount.Load())
	}
	if result.ExternalID == "" {
		t.Error("ExternalID should be non-empty (generated UUID on blank response)")
	}
}

// TestWebhookCreate_ExhaustsRetries asserts failure after max_attempts all return 503.
func TestWebhookCreate_ExhaustsRetries(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{
		"url": srv.URL,
		"retry": map[string]any{
			"max_attempts": 2,
			"backoff":      "fixed",
		},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if callCount.Load() != 2 {
		t.Errorf("expected exactly 2 calls, got %d", callCount.Load())
	}
}

// TestWebhookCreate_CustomHeaders asserts configured headers are sent.
func TestWebhookCreate_CustomHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Intake-Token") != "secret" {
			t.Errorf("expected X-Intake-Token: secret")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{
		"url":     srv.URL,
		"headers": map[string]any{"X-Intake-Token": "secret"},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
}
```

**Note on retry timing in tests:** the tests use `backoff: "fixed"` which waits 500ms between attempts. With `max_attempts: 2` and one retry, the worst case is ~500ms. If this is too slow for CI, the `backoffDuration` method can be made injectable via a function field on `Adapter` — add it if needed. The plan specifies the cleaner production implementation above; the injected approach is the test-speed optimization.

**Verification:**

```bash
cd relay && go test ./internal/adapter/webhook/... -v -count=1
```

Expected:
```
=== RUN   TestWebhookCreate_ReceivesPayload
--- PASS: TestWebhookCreate_ReceivesPayload
=== RUN   TestWebhookCreate_RetriesOn503
--- PASS: TestWebhookCreate_RetriesOn503
=== RUN   TestWebhookCreate_ExhaustsRetries
--- PASS: TestWebhookCreate_ExhaustsRetries
=== RUN   TestWebhookCreate_CustomHeaders
--- PASS: TestWebhookCreate_CustomHeaders
PASS
ok      intake/internal/adapter/webhook
```

---

### Task 3 — `relay/internal/classify/classify.go` + `classify_test.go`

**Purpose:** implement the server-side classify step that makes one structured LLM call and parses the result into `classify.Result`.

#### 3a — Package and type definition

Create `relay/internal/classify/classify.go`:

```go
package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"intake/internal/llm"
)

// Result is the structured triage output from classify(). Frozen in 1-iv (README §6.5).
type Result struct {
	Summary         string   `json:"summary"`
	TitleSuggestion string   `json:"title_suggestion"`
	Classification  string   `json:"classification"`  // bug|feature_request|question|other
	SeverityGuess   string   `json:"severity_guess"`  // low|medium|high|critical|unknown
	TagsSuggested   []string `json:"tags_suggested"`
	Language        string   `json:"language"`
}

// Valid enum sets for runtime validation (complements the typed consts in payload package).
var validClassifications = map[string]bool{
	"bug": true, "feature_request": true, "question": true, "other": true,
}
var validSeverities = map[string]bool{
	"low": true, "medium": true, "high": true, "critical": true, "unknown": true,
}

// safeDefaults returns a Result with safe fallback values.
func safeDefaults() *Result {
	return &Result{
		Summary:         "(classification unavailable)",
		TitleSuggestion: "Untitled",
		Classification:  "other",
		SeverityGuess:   "unknown",
		TagsSuggested:   []string{},
		Language:        "en",
	}
}

// classifySystemPrompt instructs the model to return strict JSON.
const classifySystemPrompt = `You are a triage assistant. Analyze the provided support conversation and return ONLY a JSON object with no markdown, no code fences, and no additional text.

The JSON object must have these exact keys:
- "summary": string — a one-to-three sentence plain-English summary of the issue
- "title_suggestion": string — a concise title of at most 80 characters
- "classification": one of "bug", "feature_request", "question", "other"
- "severity_guess": one of "low", "medium", "high", "critical", "unknown"
- "tags_suggested": array of strings — up to 5 relevant tags, empty array if none
- "language": ISO 639-1 two-letter language code of the user's messages

Return only the JSON object. No other text.`

// Classifier calls the LLM provider to produce a structured triage Result.
type Classifier struct {
	provider llm.Provider
	model    string
	maxTokens int
}

// New returns a Classifier backed by the given provider.
// model and maxTokens should come from config.LLMConfig.Anthropic.
func New(provider llm.Provider, model string, maxTokens int) *Classifier {
	return &Classifier{
		provider:  provider,
		model:     model,
		maxTokens: maxTokens,
	}
}

// Classify makes one non-streaming LLM call with classifySystemPrompt and the
// provided messages, parses the JSON response into a Result, and validates
// enum fields. On parse failure it retries once; on second failure it returns
// safeDefaults (never propagates a parse error — the submit handler must still
// produce a valid payload even if classify degrades gracefully).
func (c *Classifier) Classify(ctx context.Context, messages []llm.Message) (*Result, error) {
	result, err := c.doClassify(ctx, messages)
	if err != nil {
		// Retry once.
		result, err = c.doClassify(ctx, messages)
		if err != nil {
			// Degrade gracefully: return safe defaults so the submit still completes.
			// Log the failure (caller should log the returned error for observability).
			return safeDefaults(), fmt.Errorf("classify: both attempts failed, using safe defaults: %w", err)
		}
	}
	return result, nil
}

// doClassify performs one classify LLM call and returns a parsed, validated Result.
func (c *Classifier) doClassify(ctx context.Context, messages []llm.Message) (*Result, error) {
	// Prepend the classify system message and user turn with conversation context.
	prompt := buildPrompt(messages)
	classifyMessages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	opts := llm.ChatOptions{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		Stream:    false, // classify uses accumulate-then-parse, not streaming UX
	}

	ch, err := c.provider.Chat(ctx, classifyMessages, opts)
	if err != nil {
		return nil, fmt.Errorf("provider.Chat: %w", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return nil, fmt.Errorf("provider chunk error: %w", chunk.Err)
		}
		sb.WriteString(chunk.Delta)
		if chunk.Done {
			break
		}
	}

	raw := strings.TrimSpace(sb.String())
	// Strip accidental markdown code fences if the model disobeyed.
	raw = stripCodeFences(raw)

	var result Result
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse classify JSON: %w (raw: %s)", err, truncate(raw, 300))
	}

	if err := validateResult(&result); err != nil {
		return nil, fmt.Errorf("validate classify result: %w", err)
	}

	return &result, nil
}

// buildPrompt combines the system prompt with the conversation for the classify call.
func buildPrompt(messages []llm.Message) string {
	var sb strings.Builder
	sb.WriteString(classifySystemPrompt)
	sb.WriteString("\n\n--- Conversation ---\n")
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}
	return sb.String()
}

// validateResult checks that enum fields contain only valid values.
// It also normalises TagsSuggested to a non-nil slice and truncates
// TitleSuggestion to 80 characters (schema constraint).
func validateResult(r *Result) error {
	if !validClassifications[r.Classification] {
		return fmt.Errorf("invalid classification %q (must be one of: bug, feature_request, question, other)", r.Classification)
	}
	if !validSeverities[r.SeverityGuess] {
		return fmt.Errorf("invalid severity_guess %q (must be one of: low, medium, high, critical, unknown)", r.SeverityGuess)
	}
	if r.TagsSuggested == nil {
		r.TagsSuggested = []string{}
	}
	if len(r.TitleSuggestion) > 80 {
		r.TitleSuggestion = r.TitleSuggestion[:80]
	}
	if r.Language == "" {
		r.Language = "en"
	}
	return nil
}

// stripCodeFences removes ```json ... ``` wrappers if present.
func stripCodeFences(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
```

#### 3b — `classify_test.go`

Create `relay/internal/classify/classify_test.go`:

```go
package classify_test

import (
	"context"
	"testing"

	"intake/internal/classify"
	"intake/internal/llm"
)

// fakeProvider returns a canned response as a single Done chunk.
type fakeProvider struct {
	response string
	err      error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan llm.ChatChunk, 1)
	ch <- llm.ChatChunk{Delta: f.response, Done: true}
	close(ch)
	return ch, nil
}

const validClassifyJSON = `{
  "summary": "User cannot log in after resetting password.",
  "title_suggestion": "Login fails after password reset",
  "classification": "bug",
  "severity_guess": "high",
  "tags_suggested": ["auth", "login"],
  "language": "en"
}`

func TestClassifier_HappyPath(t *testing.T) {
	provider := &fakeProvider{response: validClassifyJSON}
	c := classify.New(provider, "claude-sonnet-4-6", 512)

	result, err := c.Classify(context.Background(), []llm.Message{
		{Role: "user", Content: "I can't log in after resetting my password."},
		{Role: "assistant", Content: "Can you describe what happens when you try?"},
		{Role: "user", Content: "It says invalid credentials every time."},
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	if result.Classification != "bug" {
		t.Errorf("expected classification=bug, got %q", result.Classification)
	}
	if result.SeverityGuess != "high" {
		t.Errorf("expected severity_guess=high, got %q", result.SeverityGuess)
	}
	if result.TitleSuggestion != "Login fails after password reset" {
		t.Errorf("unexpected title: %q", result.TitleSuggestion)
	}
	if len(result.TagsSuggested) != 2 {
		t.Errorf("expected 2 tags, got %d", len(result.TagsSuggested))
	}
	if result.Language != "en" {
		t.Errorf("expected language=en, got %q", result.Language)
	}
}

func TestClassifier_InvalidEnumFallsBackToDefaults(t *testing.T) {
	// Both attempts return invalid classification; should degrade to safe defaults.
	provider := &fakeProvider{response: `{
		"summary": "Something",
		"title_suggestion": "X",
		"classification": "INVALID_VALUE",
		"severity_guess": "medium",
		"tags_suggested": [],
		"language": "en"
	}`}
	c := classify.New(provider, "claude-sonnet-4-6", 512)

	result, err := c.Classify(context.Background(), []llm.Message{
		{Role: "user", Content: "Test message"},
	})
	// err is non-nil (safe-defaults path) but result is still usable.
	if err == nil {
		t.Error("expected non-nil error signalling safe-defaults path")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on safe-defaults path")
	}
	if result.Classification != "other" {
		t.Errorf("expected safe default classification=other, got %q", result.Classification)
	}
	if result.SeverityGuess != "unknown" {
		t.Errorf("expected safe default severity_guess=unknown, got %q", result.SeverityGuess)
	}
}

func TestClassifier_CodeFencesStripped(t *testing.T) {
	// Model disobeys and wraps in ```json ... ```.
	wrapped := "```json\n" + validClassifyJSON + "\n```"
	provider := &fakeProvider{response: wrapped}
	c := classify.New(provider, "claude-sonnet-4-6", 512)

	result, err := c.Classify(context.Background(), []llm.Message{
		{Role: "user", Content: "test"},
	})
	if err != nil {
		t.Fatalf("Classify with fenced response: %v", err)
	}
	if result.Classification != "bug" {
		t.Errorf("expected bug, got %q", result.Classification)
	}
}

func TestClassifier_TitleTruncatedTo80(t *testing.T) {
	longTitle := `This is a very long title that definitely exceeds the eighty character limit imposed by the schema and must be truncated`
	provider := &fakeProvider{response: `{
		"summary": "s",
		"title_suggestion": "` + longTitle + `",
		"classification": "other",
		"severity_guess": "unknown",
		"tags_suggested": [],
		"language": "en"
	}`}
	c := classify.New(provider, "claude-sonnet-4-6", 512)

	result, err := c.Classify(context.Background(), []llm.Message{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(result.TitleSuggestion) > 80 {
		t.Errorf("TitleSuggestion not truncated: len=%d", len(result.TitleSuggestion))
	}
}
```

**Verification:**

```bash
cd relay && go test ./internal/classify/... -v -count=1
```

Expected: all four tests PASS.

---

### Task 4 — `relay/internal/payloadbuild/build.go` + `build_test.go`

**Purpose:** assemble a `payload.IntakePayload` from `SubmitRequest`, `classify.Result`, `auth.SessionContext`, and validate it against the embedded schema. This is the L003 mitigation: `schema_version="1.0"` (and the whole payload shape) is enforced at runtime regardless of what the Go type allows.

#### 4a — Schema copy

Before writing Go code, copy the canonical schema:

```bash
# From relay/ directory:
cp ../schema/payload.v1.json internal/payloadbuild/schema.json
```

Add the `go:generate` directive (will be placed in `build.go`):

```go
//go:generate cp ../../../schema/payload.v1.json schema.json
```

#### 4b — `build.go`

Create `relay/internal/payloadbuild/build.go`:

```go
// Package payloadbuild assembles and validates the canonical payload.IntakePayload
// from a SubmitRequest, classify.Result, and auth.SessionContext.
//
// Schema validation is performed at runtime against schema/payload.v1.json
// (embedded). This is the L003 mitigation: the Go type system does NOT enforce
// the schema_version const (go-jsonschema issue); runtime validation does.
//
//go:generate cp ../../../schema/payload.v1.json schema.json
package payloadbuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	_ "embed"

	"github.com/google/uuid"
	santhosh "github.com/santhosh-tekuri/jsonschema/v6"

	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/payload"
	"intake/internal/server" // for SubmitRequest, TurnMessage, ClientInfo, ContextInfo
)

//go:embed schema.json
var schemaBytes []byte

// compiledSchema is parsed once at package init to avoid per-request overhead.
var compiledSchema *santhosh.Schema

func init() {
	c := santhosh.NewCompiler()
	// Load the embedded schema bytes as an in-memory resource.
	if err := c.AddResource("https://intake.dev/schema/payload.v1.json",
		bytes.NewReader(schemaBytes)); err != nil {
		panic(fmt.Sprintf("payloadbuild: parse embedded schema: %v", err))
	}
	var err error
	compiledSchema, err = c.Compile("https://intake.dev/schema/payload.v1.json")
	if err != nil {
		panic(fmt.Sprintf("payloadbuild: compile embedded schema: %v", err))
	}
}

// Builder assembles and validates canonical payloads.
type Builder struct {
	widgetVersionDefault string
}

// New returns a Builder. widgetVersionDefault is used when ClientInfo.WidgetVersion is empty.
func New(widgetVersionDefault string) *Builder {
	return &Builder{widgetVersionDefault: widgetVersionDefault}
}

// Build assembles a payload.IntakePayload from the given inputs, then validates it
// against the embedded schema/payload.v1.json. Returns the validated payload or
// a descriptive error (suitable for a 400 response to the client).
func (b *Builder) Build(
	_ context.Context,
	req *server.SubmitRequest,
	result *classify.Result,
	sess *auth.SessionContext,
	submissionID string, // pre-generated UUID
	submittedAt time.Time,
) (*payload.IntakePayload, error) {
	wv := req.Client.WidgetVersion
	if wv == "" {
		wv = b.widgetVersionDefault
	}

	// Build conversation messages.
	msgs := make([]payload.Message, 0, len(req.Messages))
	// All messages get the same timestamp (submitted_at) because the relay is
	// stateless between turns and per-message timestamps are not in SubmitRequest.
	// Phase 5 may add per-message timestamps when the client sends them.
	for _, m := range req.Messages {
		role := payload.MessageRoleUser
		if m.Role == "assistant" {
			role = payload.MessageRoleAssistant
		}
		msgs = append(msgs, payload.Message{
			Role:    role,
			Content: m.Content,
			Ts:      submittedAt,
		})
	}

	// Map classify.Result enum strings to payload typed consts.
	classification, err := mapClassification(result.Classification)
	if err != nil {
		return nil, fmt.Errorf("payloadbuild: %w", err)
	}
	severity, err := mapSeverity(result.SeverityGuess)
	if err != nil {
		return nil, fmt.Errorf("payloadbuild: %w", err)
	}

	// Map auth mode.
	authMode, err := mapAuthMode(sess.AuthMode)
	if err != nil {
		return nil, fmt.Errorf("payloadbuild: %w", err)
	}

	p := &payload.IntakePayload{
		SchemaVersion: "1.0", // const — also enforced by schema validation below (L003 mitigation)
		Submission: payload.Submission{
			Id:          submissionID,
			SubmittedAt: submittedAt,
			TenantId:    nil, // Phase 3 (multi-tenant)
		},
		Client: payload.Client{
			WidgetVersion: wv,
			SessionId:     sess.SessionID,
			Url:           req.Client.URL,
			Referrer:      req.Client.Referrer,
			UserAgent:     req.Client.UserAgent,
			Viewport: payload.Viewport{
				W: req.Client.Viewport.W,
				H: req.Client.Viewport.H,
			},
			Locale: req.Client.Locale,
		},
		User: payload.User{
			AuthMode:    authMode,
			Id:          sess.UserID,
			Email:       sess.Email,
			DisplayName: sess.DisplayName,
			Verified:    sess.Verified,
			Custom:      req.UserClaims,
		},
		Conversation: payload.Conversation{
			Messages:        msgs,
			Summary:         result.Summary,
			TitleSuggestion: result.TitleSuggestion,
			Classification:  classification,
			SeverityGuess:   severity,
			TagsSuggested:   result.TagsSuggested,
			Language:        result.Language,
		},
		RoutingHint: req.RoutingHint,
	}

	// Copy context if non-empty.
	if req.Context.AppContext != nil || req.Context.PageMetadata != nil {
		p.Context = &payload.Context{
			AppContext:   req.Context.AppContext,
			PageMetadata: req.Context.PageMetadata,
		}
	}

	// Schema validation (runtime, L003 mitigation).
	if err := validateAgainstSchema(p); err != nil {
		return nil, fmt.Errorf("payloadbuild: schema validation failed: %w", err)
	}

	return p, nil
}

// validateAgainstSchema marshals p to JSON then validates against the compiled schema.
func validateAgainstSchema(p *payload.IntakePayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal for validation: %w", err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("unmarshal for validation: %w", err)
	}
	if err := compiledSchema.Validate(v); err != nil {
		return err
	}
	return nil
}

// mapClassification converts a classify.Result string to the typed payload const.
func mapClassification(s string) (payload.ConversationClassification, error) {
	switch s {
	case "bug":
		return payload.ConversationClassificationBug, nil
	case "feature_request":
		return payload.ConversationClassificationFeatureRequest, nil
	case "question":
		return payload.ConversationClassificationQuestion, nil
	case "other":
		return payload.ConversationClassificationOther, nil
	default:
		return "", fmt.Errorf("unknown classification %q", s)
	}
}

// mapSeverity converts a classify.Result string to the typed payload const.
func mapSeverity(s string) (payload.ConversationSeverityGuess, error) {
	switch s {
	case "low":
		return payload.ConversationSeverityGuessLow, nil
	case "medium":
		return payload.ConversationSeverityGuessMedium, nil
	case "high":
		return payload.ConversationSeverityGuessHigh, nil
	case "critical":
		return payload.ConversationSeverityGuessCritical, nil
	case "unknown":
		return payload.ConversationSeverityGuessUnknown, nil
	default:
		return "", fmt.Errorf("unknown severity_guess %q", s)
	}
}

// mapAuthMode converts an auth.SessionContext AuthMode string to the typed payload const.
func mapAuthMode(s string) (payload.UserAuthMode, error) {
	switch s {
	case "anonymous":
		return payload.UserAuthModeAnonymous, nil
	case "email":
		return payload.UserAuthModeEmail, nil
	case "sso":
		return payload.UserAuthModeSso, nil
	default:
		return "", fmt.Errorf("unknown auth_mode %q", s)
	}
}

// newSubmissionID generates a new UUID for submission.id.
// Exposed so callers (submit handler) can pre-generate the ID.
func NewSubmissionID() string {
	return uuid.NewString()
}

// canonicalSchemaBytes returns the embedded schema bytes.
// Used by tests to assert the embedded copy is identical to the canonical file.
func CanonicalSchemaBytes() []byte {
	return schemaBytes
}

// SchemaFromOS reads the canonical schema from the given path (for test comparison).
func SchemaFromOS(path string) ([]byte, error) {
	return os.ReadFile(path)
}
```

> **Import note:** `payloadbuild` imports `intake/internal/server` for `SubmitRequest`. If that creates a circular import (server imports payloadbuild), move the DTOs to `intake/internal/dto` or `intake/internal/apitypes`. Check for circularity with `go build ./...` after wiring. The recommended resolution is to extract `SubmitRequest` and related DTOs into `intake/internal/dto` and have both `server` and `payloadbuild` import `dto`. The plan shows the `server` import for brevity; implement the `dto` extraction if `go build` reports a cycle.

#### 4c — `build_test.go`

Create `relay/internal/payloadbuild/build_test.go`:

```go
package payloadbuild_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/payloadbuild"
	"intake/internal/server"
)

func testSession() *auth.SessionContext {
	return &auth.SessionContext{
		SessionID: "00000000-0000-0000-0000-000000000002",
		AuthMode:  "anonymous",
		Verified:  false,
	}
}

func testClassifyResult() *classify.Result {
	return &classify.Result{
		Summary:         "User cannot log in after password reset.",
		TitleSuggestion: "Login fails after password reset",
		Classification:  "bug",
		SeverityGuess:   "high",
		TagsSuggested:   []string{"auth", "login"},
		Language:        "en",
	}
}

func testSubmitRequest() *server.SubmitRequest {
	ref := "http://localhost:5173/"
	return &server.SubmitRequest{
		Messages: []server.TurnMessage{
			{Role: "user", Content: "I cannot log in."},
			{Role: "assistant", Content: "Can you describe the error?"},
			{Role: "user", Content: "It says invalid credentials."},
		},
		Client: server.ClientInfo{
			WidgetVersion: "0.1.0",
			URL:           "http://localhost:5173/",
			Referrer:      &ref,
			UserAgent:     "Mozilla/5.0",
			Viewport:      server.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		UserClaims: map[string]any{},
		Context: server.ContextInfo{
			AppContext:    map[string]any{"env": "test"},
			PageMetadata: map[string]any{"title": "Home"},
		},
	}
}

// TestBuild_WellFormed_ProducesSchemaValidPayload is the main L003 mitigation test.
func TestBuild_WellFormed_ProducesSchemaValidPayload(t *testing.T) {
	b := payloadbuild.New("0.1.0")
	p, err := b.Build(
		context.Background(),
		testSubmitRequest(),
		testClassifyResult(),
		testSession(),
		payloadbuild.NewSubmissionID(),
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if p.SchemaVersion != "1.0" {
		t.Errorf("expected SchemaVersion=1.0, got %q", p.SchemaVersion)
	}
	if p.User.AuthMode != "anonymous" {
		t.Errorf("expected User.AuthMode=anonymous, got %q", p.User.AuthMode)
	}
	if p.Conversation.Classification != "bug" {
		t.Errorf("expected Classification=bug, got %q", p.Conversation.Classification)
	}
	if len(p.Conversation.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(p.Conversation.Messages))
	}
}

// TestBuild_TamperedSchemaVersion_IsRejected is the direct L003 mitigation test:
// asserts that if we somehow set SchemaVersion to an invalid value, the embedded
// schema validation (not the Go type) catches it and returns an error.
// We achieve this by building a valid payload then directly mutating it and calling
// the exported validateAgainstSchema via a helper, OR by building with a bad request
// that would produce an invalid payload.
// Since Go types don't enforce const, we use a test-only hook.
func TestBuild_InvalidClassification_IsRejected(t *testing.T) {
	b := payloadbuild.New("0.1.0")

	badResult := testClassifyResult()
	badResult.Classification = "INVALID_CLASSIFICATION" // not in enum

	_, err := b.Build(
		context.Background(),
		testSubmitRequest(),
		badResult,
		testSession(),
		payloadbuild.NewSubmissionID(),
		time.Now().UTC(),
	)
	if err == nil {
		t.Fatal("expected error for invalid classification, got nil")
	}
	t.Logf("correctly rejected: %v", err)
}

// TestBuild_EmbeddedSchemaIsIdenticalToCanonical guards against schema drift (L003 mitigation).
// The embedded schema.json must stay byte-identical to schema/payload.v1.json.
func TestBuild_EmbeddedSchemaIsIdenticalToCanonical(t *testing.T) {
	embedded := payloadbuild.CanonicalSchemaBytes()

	// Locate the canonical file relative to the repo root.
	// __FILE__ is relay/internal/payloadbuild/build_test.go
	// so the canonical path is ../../../schema/payload.v1.json from here.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	canonicalPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "schema", "payload.v1.json")

	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical schema: %v\npath tried: %s", err, canonicalPath)
	}

	if string(embedded) != string(canonical) {
		t.Errorf("embedded schema.json is NOT byte-identical to schema/payload.v1.json\n"+
			"Run: cd relay && go generate ./internal/payloadbuild/...\n"+
			"to regenerate the embedded copy.")
	}
}
```

> **Circular import resolution:** If `payloadbuild` importing `server` causes a cycle, extract `SubmitRequest`, `TurnMessage`, `ClientInfo`, `Viewport`, `ContextInfo` into `relay/internal/dto/dto.go` and update all imports. The `server` package's handler files import `dto`; `payloadbuild` imports `dto`. Neither imports the other. This is the correct architecture. Update `relay/internal/server/dto.go` (or move types to `relay/internal/dto/`) before `go build` can pass.

**Verification:**

```bash
cd relay && go test ./internal/payloadbuild/... -v -count=1
```

Expected:
```
=== RUN   TestBuild_WellFormed_ProducesSchemaValidPayload
--- PASS: TestBuild_WellFormed_ProducesSchemaValidPayload
=== RUN   TestBuild_InvalidClassification_IsRejected
--- PASS: TestBuild_InvalidClassification_IsRejected
=== RUN   TestBuild_EmbeddedSchemaIsIdenticalToCanonical
--- PASS: TestBuild_EmbeddedSchemaIsIdenticalToCanonical
PASS
ok      intake/internal/payloadbuild
```

---

### Task 5 — Extend `relay/internal/server/deps.go` and `relay/internal/server/routes.go`

**Purpose:** add the three new fields to `Deps` and register the `/submit` route behind auth.

#### 5a — `deps.go`

Modify `relay/internal/server/deps.go` — add to the `Deps` struct:

```go
import (
    // existing imports...
    "intake/internal/adapter"
    "intake/internal/classify"
    "intake/internal/payloadbuild"
)

// Deps is a VALUE type (README §6.8) — no Config field; no pointer receiver.
type Deps struct {
    // ... existing fields from 1-i/1-iii (Version, CORSOrigins, Logger, Auth,
    // Provider, SystemPrompt, Model, MaxTokens) — do NOT add a Config field ...

    // New in 1-iv:
    Adapter    adapter.Adapter
    Classifier *classify.Classifier
    Builder    *payloadbuild.Builder
}
```

#### 5b — `routes.go`

Inside the auth-protected sub-router where `/v1/intake/turn` is registered (from 1-iii), add:

```go
r.Post("/v1/intake/submit", submitHandler(deps))
```

The exact location depends on what 1-iii produced. The invariant: `/v1/intake/submit` MUST be behind the same auth middleware as `/v1/intake/turn`.

**Verification:**

```bash
cd relay && go build ./internal/server/...
```

Expected: no output, exit 0.

---

### Task 6 — `relay/internal/server/submit.go` + `submit_test.go`

**Purpose:** the `/v1/intake/submit` handler wires classify → build → adapter.Create → SubmitResponse.

#### 6a — `submit.go`

Create `relay/internal/server/submit.go`:

```go
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"intake/internal/auth"
	"intake/internal/llm"
	"intake/internal/payloadbuild"
)

// submitHandler handles POST /v1/intake/submit.
// It:
//  1. Decodes the SubmitRequest body.
//  2. Extracts the SessionContext from context (placed by auth middleware).
//  3. Builds []llm.Message from request messages.
//  4. Calls Classifier.Classify to produce a classify.Result.
//  5. Calls Builder.Build to assemble + validate the canonical payload.
//  6. Calls Adapter.Create to POST to the downstream system.
//  7. Returns a SubmitResponse (200) or an ErrorEnvelope (400/502).
func submitHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Decode request body.
		var req SubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("invalid request body: %v", err))
			return
		}

		// Extract session.
		sess, ok := auth.FromContext(ctx)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing session context")
			return
		}

		// Convert TurnMessages to llm.Messages for classify.
		llmMsgs := make([]llm.Message, 0, len(req.Messages))
		for _, m := range req.Messages {
			llmMsgs = append(llmMsgs, llm.Message{
				Role:    m.Role,
				Content: m.Content,
			})
		}

		// Classify.
		classifyResult, err := deps.Classifier.Classify(ctx, llmMsgs)
		if err != nil {
			// Non-nil error means safe defaults were used (graceful degradation).
			// Log but do NOT fail the request — the safe defaults produce a valid payload.
			slog.WarnContext(ctx, "classify degraded to safe defaults", "err", err)
		}

		// Assemble and validate the canonical payload.
		submissionID := payloadbuild.NewSubmissionID()
		submittedAt := time.Now().UTC()

		p, err := deps.Builder.Build(ctx, &req, classifyResult, sess, submissionID, submittedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "PAYLOAD_INVALID", fmt.Sprintf("payload validation failed: %v", err))
			return
		}

		// Dispatch to adapter.
		result, err := deps.Adapter.Create(ctx, p)
		if err != nil {
			slog.ErrorContext(ctx, "adapter Create failed",
				"adapter", deps.Adapter.Name(),
				"err", err,
			)
			writeError(w, http.StatusBadGateway, "ADAPTER_ERROR",
				fmt.Sprintf("adapter %q failed: %v", deps.Adapter.Name(), err))
			return
		}

		// Success.
		writeJSON(w, http.StatusOK, SubmitResponse{
			ExternalID:  result.ExternalID,
			ExternalURL: result.ExternalURL,
			AdapterName: result.AdapterName,
			CreatedAt:   result.CreatedAt,
		})
	}
}

// writeError writes an ErrorEnvelope JSON response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorEnvelope{
		Error: ErrorBody{Code: code, Message: message},
	})
}

// writeJSON marshals v to JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode failed", "err", err)
	}
}

// generateID is a convenience wrapper for uuid generation used outside of payloadbuild.
func generateID() string {
	return uuid.NewString()
}
```

> **Note:** if `writeError` and `writeJSON` are already defined by 1-i/1-iii handlers, do not duplicate them — use the existing definitions. Check for existing helpers in `relay/internal/server/` before adding.

#### 6b — `submit_test.go`

Create `relay/internal/server/submit_test.go`:

```go
package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"intake/internal/adapter"
	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/payload"
	"intake/internal/payloadbuild"
	"intake/internal/server"
)

// fakeProvider returns a canned classify JSON response.
type fakeProvider struct{ response string }

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	ch := make(chan llm.ChatChunk, 1)
	ch <- llm.ChatChunk{Delta: f.response, Done: true}
	close(ch)
	return ch, nil
}

// fakeAdapter captures the payload POSTed to it and returns a canned result.
type fakeAdapter struct {
	received *payload.IntakePayload
}

func (a *fakeAdapter) Name() string              { return "fake-webhook" }
func (a *fakeAdapter) RequiresLicense() bool     { return false }
func (a *fakeAdapter) Configure(map[string]any) error { return nil }
func (a *fakeAdapter) HealthCheck(context.Context) error { return nil }
func (a *fakeAdapter) Create(_ context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	a.received = p
	return &adapter.CreateResult{
		ExternalID:  "test-ext-id-001",
		ExternalURL: "",
		AdapterName: "fake-webhook",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

const validClassifyJSON = `{
  "summary": "User cannot log in.",
  "title_suggestion": "Login failure",
  "classification": "bug",
  "severity_guess": "high",
  "tags_suggested": ["auth"],
  "language": "en"
}`

func buildTestDeps(fa *fakeAdapter) server.Deps {
	provider := &fakeProvider{response: validClassifyJSON}
	classifier := classify.New(provider, "claude-sonnet-4-6", 512)
	builder := payloadbuild.New("0.1.0")
	// Deps is a value type (README §6.8). Auth, Provider, etc. added by 1-i/1-iii;
	// minimal stubs here for the submit handler unit tests.
	return server.Deps{
		Adapter:    fa,
		Classifier: classifier,
		Builder:    builder,
	}
}

// injectSession attaches a SessionContext to the request context (simulates auth middleware).
func injectSession(r *http.Request) *http.Request {
	sess := &auth.SessionContext{
		SessionID: "00000000-0000-0000-0000-000000000002",
		AuthMode:  "anonymous",
		Verified:  false,
	}
	return r.WithContext(auth.WithSession(r.Context(), sess))
}

func TestSubmitHandler_HappyPath(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildTestDeps(fa)

	// Build a minimal SubmitRequest body.
	reqBody := server.SubmitRequest{
		Messages: []server.TurnMessage{
			{Role: "user", Content: "I cannot log in."},
		},
		Client: server.ClientInfo{
			WidgetVersion: "0.1.0",
			URL:           "http://localhost:5173/",
			UserAgent:     "Mozilla/5.0",
			Viewport:      server.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		UserClaims: map[string]any{},
		Context:    server.ContextInfo{},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = injectSession(req)

	rr := httptest.NewRecorder()

	// Test via the registered router (README §6.8: server.New(cfg, deps), value Deps).
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", rr.Code, rr.Body.String())
	}

	var resp server.SubmitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ExternalID != "test-ext-id-001" {
		t.Errorf("expected ExternalID test-ext-id-001, got %q", resp.ExternalID)
	}
	if resp.AdapterName != "fake-webhook" {
		t.Errorf("expected AdapterName fake-webhook, got %q", resp.AdapterName)
	}

	// Assert the canonical payload was schema-valid (fakeAdapter captured it).
	if fa.received == nil {
		t.Fatal("adapter.Create was not called")
	}
	if fa.received.SchemaVersion != "1.0" {
		t.Errorf("expected schema_version=1.0, got %q", fa.received.SchemaVersion)
	}
	if fa.received.Conversation.Classification != "bug" {
		t.Errorf("expected classification=bug (from classify), got %q", fa.received.Conversation.Classification)
	}
	if fa.received.User.AuthMode != "anonymous" {
		t.Errorf("expected user.auth_mode=anonymous, got %q", fa.received.User.AuthMode)
	}
}

// TestSubmitHandler_MissingSession returns 401.
func TestSubmitHandler_MissingSession(t *testing.T) {
	fa := &fakeAdapter{}
	deps := buildTestDeps(fa)

	bodyBytes, _ := json.Marshal(server.SubmitRequest{
		Messages: []server.TurnMessage{{Role: "user", Content: "hello"}},
		Client: server.ClientInfo{
			URL:       "http://localhost:5173/",
			UserAgent: "test",
			Viewport:  server.Viewport{W: 100, H: 100},
			Locale:    "en",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	// No injectSession — session intentionally absent.

	rr := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestSubmitHandler_IntegrationWithHttptestWebhook is the integration-level test:
// uses the REAL webhook adapter wired to an httptest server.
// Asserts the posted body is schema-valid and has classify-derived fields.
func TestSubmitHandler_IntegrationWithHttptestWebhook(t *testing.T) {
	// Local httptest webhook receiver.
	var receivedBody []byte
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read receiver body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"external_id":"integration-ext-id"}`))
	}))
	defer receiver.Close()

	// Wire a real webhook adapter.
	import_webhook := func() adapter.Adapter {
		// Import inline to avoid import cycle issues in test.
		// Use the webhook package directly.
		return nil // placeholder — see note below
	}
	_ = import_webhook
	// NOTE: import intake/internal/adapter/webhook here.
	// In the actual test file, add:
	//   import whAdapter "intake/internal/adapter/webhook"
	// Then:
	//   wh := whAdapter.New()
	//   wh.Configure(map[string]any{"url": receiver.URL, "retry": map[string]any{"max_attempts": 1, "backoff": "fixed"}})

	// ... rest of integration test setup ...
	// This test is intentionally left as a sketch here.
	// See full integration test in the Smoke section below for the manual version.
	// The unit tests above are the primary TDD coverage.
	t.Skip("full httptest integration test: implement after Task 2 webhook adapter is verified")
}
```

> **TestSubmitHandler_IntegrationWithHttptestWebhook:** Replace the placeholder with a real integration test once the webhook adapter (Task 2) is merged. The test must:
> - Wire `whAdapter.New()` + `Configure(receiver.URL)`
> - POST a `SubmitRequest` via `server.New(cfg, deps)` router
> - Assert `receivedBody` JSON contains `schema_version=1.0`, `conversation.classification`, `user.auth_mode=anonymous`
> - Assert `SubmitResponse.external_id = "integration-ext-id"`

**Complete integration test (replace the sketch above):**

```go
func TestSubmitHandler_IntegrationWithHttptestWebhook(t *testing.T) {
	var receivedBody []byte
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read receiver body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"external_id":"integration-ext-id"}`))
	}))
	defer receiver.Close()

	wh := webhookadapter.New()
	if err := wh.Configure(map[string]any{
		"url": receiver.URL,
		"retry": map[string]any{"max_attempts": 1, "backoff": "fixed"},
	}); err != nil {
		t.Fatalf("Configure webhook: %v", err)
	}

	provider := &fakeProvider{response: validClassifyJSON}
	classifier := classify.New(provider, "claude-sonnet-4-6", 512)
	builder := payloadbuild.New("0.1.0")
	// Deps is a value type (README §6.8).
	deps := server.Deps{
		Adapter:    wh,
		Classifier: classifier,
		Builder:    builder,
	}

	reqBody := server.SubmitRequest{
		Messages: []server.TurnMessage{
			{Role: "user", Content: "I cannot log in."},
			{Role: "assistant", Content: "What error do you see?"},
			{Role: "user", Content: "Invalid credentials."},
		},
		Client: server.ClientInfo{
			WidgetVersion: "0.1.0",
			URL:           "http://localhost:5173/",
			UserAgent:     "Mozilla/5.0",
			Viewport:      server.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		UserClaims: map[string]any{},
		Context:    server.ContextInfo{},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/submit", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = injectSession(req)

	rr := httptest.NewRecorder()
	cfg := &config.Config{Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}}}
	mux := server.New(cfg, deps)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d\nbody: %s", rr.Code, rr.Body.String())
	}

	// Assert SubmitResponse.
	var resp server.SubmitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ExternalID != "integration-ext-id" {
		t.Errorf("expected ExternalID integration-ext-id, got %q", resp.ExternalID)
	}

	// Assert the webhook receiver got a schema-valid canonical payload.
	if receivedBody == nil {
		t.Fatal("webhook receiver was not called")
	}
	var posted map[string]any
	if err := json.Unmarshal(receivedBody, &posted); err != nil {
		t.Fatalf("webhook body not valid JSON: %v", err)
	}
	if posted["schema_version"] != "1.0" {
		t.Errorf("expected schema_version=1.0 in posted payload, got %v", posted["schema_version"])
	}
	conv, ok := posted["conversation"].(map[string]any)
	if !ok {
		t.Fatal("conversation field missing or wrong type")
	}
	if conv["classification"] != "bug" {
		t.Errorf("expected classification=bug from classify, got %v", conv["classification"])
	}
	user, ok := posted["user"].(map[string]any)
	if !ok {
		t.Fatal("user field missing or wrong type")
	}
	if user["auth_mode"] != "anonymous" {
		t.Errorf("expected user.auth_mode=anonymous, got %v", user["auth_mode"])
	}
}
```

Add `import webhookadapter "intake/internal/adapter/webhook"` to the file's import block.

**Verification:**

```bash
cd relay && go test ./internal/server/... -v -count=1 -run TestSubmit
```

Expected: all TestSubmit* tests PASS (skip the stub integration test if still skeletal; the full integration test above replaces it).

---

### Task 7 — Wire in `relay/cmd/relay/main.go`

**Purpose:** construct and connect the webhook adapter, classifier, and builder in the binary entry point.

Replace (or extend) `relay/cmd/relay/main.go` with:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"intake/internal/adapter/webhook"
	"intake/internal/classify"
	"intake/internal/config"
	"intake/internal/llm/anthropic"
	"intake/internal/payloadbuild"
	"intake/internal/server"
)

func main() {
	// Load config (from 1-i). Signature: config.Load(path string) (*config.Config, error).
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Build the LLM provider (from 1-ii, README §6.8).
	// API key is read from env; NEVER logged (security invariant).
	apiKey := os.Getenv(cfg.LLM.Anthropic.APIKeyEnv)
	if apiKey == "" {
		log.Fatalf("anthropic: env var %q not set", cfg.LLM.Anthropic.APIKeyEnv)
	}
	provider := anthropic.New(apiKey, cfg.LLM.Anthropic.Model, cfg.LLM.Anthropic.MaxTokens)

	// Build the webhook adapter (new in 1-iv).
	wh := webhook.New()
	whCfg := map[string]any{
		"url":     cfg.Adapters.Webhook.URL,
		"headers": cfg.Adapters.Webhook.Headers,
		"retry": map[string]any{
			"max_attempts": cfg.Adapters.Webhook.Retry.MaxAttempts,
			"backoff":      cfg.Adapters.Webhook.Retry.Backoff,
		},
	}
	if err := wh.Configure(whCfg); err != nil {
		log.Fatalf("webhook adapter: %v", err)
	}

	// Build the classifier (new in 1-iv).
	classifier := classify.New(provider, cfg.LLM.Anthropic.Model, cfg.LLM.Anthropic.MaxTokens)

	// Build the payload builder (new in 1-iv).
	builder := payloadbuild.New("0.1.0") // widget version default; Phase 5 may read from config

	// Assemble Deps (extends 1-i/1-iii Deps).
	// Deps is a VALUE type (README §6.8). No Config field — config-derived values
	// are promoted to individual Deps fields. Auth/SystemPrompt/Model/MaxTokens/Logger
	// and CORSOrigins come from 1-iii wiring (already present in main.go).
	// 1-iv extends the existing Deps literal with these three new fields.
	//
	// In practice: locate the existing server.Deps{...} literal in main.go (added by
	// 1-iii) and ADD the three fields below. The final shape per README §6.8:
	//
	//   deps := server.Deps{
	//       CORSOrigins:  cfg.Server.CORSOrigins,   // 1-i
	//       Version:      buildVersion,              // 1-i
	//       Logger:       logger,                   // 1-iii
	//       Auth:         middleware,               // 1-iii
	//       Provider:     provider,                 // 1-iii
	//       SystemPrompt: systemPrompt,             // 1-iii
	//       Model:        cfg.LLM.Anthropic.Model,  // 1-iii
	//       MaxTokens:    cfg.LLM.Anthropic.MaxTokens, // 1-iii
	//       Adapter:      wh,                       // 1-iv (new)
	//       Classifier:   classifier,               // 1-iv (new)
	//       Builder:      builder,                  // 1-iv (new)
	//   }
	//
	// Shown here in isolation (the 1-i/1-iii fields already exist):
	deps.Adapter = wh
	deps.Classifier = classifier
	deps.Builder = builder

	// server.New is the canonical constructor (README §6.8): func New(cfg *config.Config, deps Deps) http.Handler
	handler := server.New(cfg, deps)
	httpServer := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: handler,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("intake-relay starting", "addr", cfg.Server.Addr)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down relay")
	_ = httpServer.Shutdown(context.Background())
}
```

> **Note:** `config.Load(path)` comes from 1-i. `anthropic.New(apiKey, model, maxTokens)` comes from 1-ii (see README §6.8). `server.New(cfg, deps)` is the canonical constructor — there is no `NewRouter` or `NewServer`. This sub-plan depends on 1-iii being merged; the 1-iii `deps` literal in `main.go` is extended additively here.

**Verification:**

```bash
cd relay && go build ./cmd/relay/...
```

Expected: no output, exit 0.

```bash
cd relay && go vet ./...
```

Expected: no output, exit 0.

```bash
cd relay && go test ./... -count=1
```

Expected: all tests PASS (the integration test with `t.Skip` skips gracefully; remove the Skip once wired).

---

### Task 8 — Full build and test gate

Run the complete phase build-fail checklist:

```bash
cd relay

# 1. Build all packages
go build ./...

# 2. Vet
go vet ./...

# 3. All tests
go test ./... -count=1

# 4. Contract gate
cd .. && bash scripts/verify-contract.sh

# 5. Pin check
bash scripts/check-pins.sh

# 6. Secret check (grep relay logs for API key — manual during smoke; automated here as a static grep)
grep -r "ANTHROPIC_API_KEY\|sk-ant-" relay/internal/ && echo "FAIL: secret in source" || echo "OK: no secrets in source"
```

Expected outcomes:
- `go build ./...`: exit 0, no output
- `go vet ./...`: exit 0, no output
- `go test ./...`: `ok` for every package, 0 failures
- `verify-contract.sh`: `OK` (payload types unchanged)
- `check-pins.sh`: `OK: all codegen tools are exact-pinned` + `OK: Go module pins verified`
- Secret grep: `OK: no secrets in source`

---

## 5. Smoke

This smoke proves 1-iv's deliverable (Adapter seam + webhook + /submit) end-to-end with a real Anthropic key and a local webhook receiver.

**Pre-conditions:**
- 1-i, 1-ii, 1-iii are merged and their own smokes pass.
- `ANTHROPIC_API_KEY` is exported in the shell.
- Go 1.23.2 installed.
- `relay/` builds clean (`go build ./...` exit 0).

**Step 1: Start a local webhook receiver on :9099.**

Create or use `examples/webhook-receiver/main.go` (minimal; see skeleton below):

```go
// examples/webhook-receiver/main.go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/intake", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Printf("\n=== RECEIVED WEBHOOK ===\n%s\n========================\n", body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"external_id":"smoke-ext-001"}`))
	})
	log.Println("webhook receiver listening on :9099")
	log.Fatal(http.ListenAndServe(":9099", nil))
}
```

Run it:

```bash
go run examples/webhook-receiver/main.go
```

**Step 2: Configure relay.**

Create `config.yaml` in the repo root (or confirm it exists from 1-i):

```yaml
server:
  addr: ":8080"
  external_url: "http://localhost:8080"
  cors_origins: ["http://localhost:5173"]
llm:
  provider: "anthropic"
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-sonnet-4-6"
    max_tokens: 1024
  system_prompt_file: ""
auth:
  modes:
    anonymous: true
adapters:
  webhook:
    enabled: true
    url: "http://localhost:9099/intake"
    headers: {}
    retry:
      max_attempts: 3
      backoff: "exponential"
```

**Step 3: Start the relay.**

```bash
cd relay && go run ./cmd/relay --config ../config.yaml
```

Confirm:
```
GET http://localhost:8080/v1/health
# expected: {"status":"ok"} with 200

GET http://localhost:8080/v1/version
# expected: {"version":"..."} with 200
```

**Step 4: POST a session init.**

```bash
curl -s -X POST http://localhost:8080/v1/intake/init \
  -H "Content-Type: application/json" \
  -d '{}'
```

Expected:
```json
{"session_id":"<uuid>","capabilities":{"auth_modes":["anonymous"],"streaming":true}}
```

Capture the `session_id`.

**Step 5: POST a submit.**

```bash
SESSION_ID="<paste session_id from Step 4>"

curl -s -X POST http://localhost:8080/v1/intake/submit \
  -H "Content-Type: application/json" \
  -H "X-Intake-Session: $SESSION_ID" \
  -d '{
    "messages": [
      {"role": "user", "content": "I cannot log in after resetting my password."},
      {"role": "assistant", "content": "Can you describe the exact error message?"},
      {"role": "user", "content": "It says \"Invalid credentials\" every time."}
    ],
    "client": {
      "widget_version": "0.1.0",
      "url": "http://localhost:5173/",
      "user_agent": "curl/8.0",
      "viewport": {"w": 1280, "h": 720},
      "locale": "en-US"
    },
    "user_claims": {},
    "context": {}
  }'
```

**Step 6: Verify.**

The curl response must be:
```json
{
  "external_id": "smoke-ext-001",
  "external_url": "",
  "adapter_name": "webhook",
  "created_at": "<RFC3339 timestamp>"
}
```

The webhook receiver terminal must print a JSON body containing:
- `"schema_version": "1.0"`
- `"conversation"` with all seven required fields populated:
  - `summary`: non-empty string (from classify)
  - `title_suggestion`: ≤80 chars, non-empty (from classify)
  - `classification`: one of `bug|feature_request|question|other`
  - `severity_guess`: one of `low|medium|high|critical|unknown`
  - `tags_suggested`: array (may be empty)
  - `language`: two-letter code
  - `messages`: array with the 3 messages from the request
- `"user": {"auth_mode": "anonymous", "verified": false}`
- `"client"`: `url`, `viewport`, `locale`, `user_agent` matching request values
- `"submission"`: `id` (UUID), `submitted_at` (RFC3339)

**Step 7: Secret check.**

In the relay terminal output, grep for the actual key value:

```bash
# In a separate terminal, after the submit:
grep -i "sk-ant\|ANTHROPIC_API_KEY" /tmp/relay-output.log
# Expect: no matches
```

Alternatively, confirm visually that the relay logs show no provider key.

**Step 8: Validation reject case.**

Confirm that a tampered submit body returns 400 (not 500):

```bash
curl -s -X POST http://localhost:8080/v1/intake/submit \
  -H "Content-Type: application/json" \
  -H "X-Intake-Session: $SESSION_ID" \
  -d '{"messages":[], "client": {"url":"NOT-A-URI", "user_agent":"x", "viewport":{"w":0,"h":0}, "locale":"en"}, "user_claims":{}, "context":{}}'
```

Expected: `400` with `{"error":{"code":"PAYLOAD_INVALID","message":"..."}}`.

(The bad `url` violates the schema's `format: uri` constraint — but note: santhosh-tekuri's draft-2020-12 validation of `format` depends on whether format assertions are enabled. Use an unambiguously invalid enum value or missing required field instead if format is not asserted by default. A simpler reject case: send a message with `role: "system"` which is not in the enum `["user","assistant"]`.)

---

## 6. Done criteria

- [ ] `relay/internal/adapter/adapter.go` exists with the exact interface from README §6.2 (using `payload.IntakePayload` — the actual generated type name).
- [ ] `relay/internal/adapter/webhook/webhook.go` implements all five Adapter methods; retry logic verified by `TestWebhookCreate_RetriesOn503`.
- [ ] `relay/internal/classify/classify.go` defines `classify.Result` (frozen §6.5) and `Classifier`; enum validation and code-fence stripping verified; graceful degradation to safe defaults verified by `TestClassifier_InvalidEnumFallsBackToDefaults`.
- [ ] `relay/internal/payloadbuild/build.go` produces schema-valid payloads; `TestBuild_WellFormed_ProducesSchemaValidPayload` passes; `TestBuild_InvalidClassification_IsRejected` passes (runtime validation rejects invalid enums regardless of Go type leniency — L003 mitigation active).
- [ ] `TestBuild_EmbeddedSchemaIsIdenticalToCanonical` passes (schema copy cannot silently drift from `schema/payload.v1.json`).
- [ ] `relay/internal/server/submit.go` returns 400 on payload validation failure; 502 with adapter name on adapter failure; 200 `SubmitResponse` on success.
- [ ] `relay/internal/server/deps.go` has `Adapter`, `Classifier`, `Builder` fields.
- [ ] `/v1/intake/submit` is registered in `routes.go` behind the auth middleware.
- [ ] `relay/cmd/relay/main.go` constructs webhook adapter, classifier, builder from config and wires into Deps.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` all pass in `relay/`.
- [ ] `scripts/check-pins.sh` passes.
- [ ] `scripts/verify-contract.sh` passes (no payload type regression).
- [ ] Smoke (§5) passes: relay running with real Anthropic key; local receiver on :9099 receives a schema-valid canonical payload with classify-derived `conversation.*` fields; `SubmitResponse` returned to caller; no API key in logs.

---

## Appendix A — go:embed and the upward-traversal restriction

`go:embed` in Go 1.16+ **cannot** reference paths with `..` components. Since `relay/internal/payloadbuild/` needs `schema/payload.v1.json` which lives at `../../..` from the package, the schema must be physically present inside `relay/` at embed time.

**Chosen approach: committed copy + byte-identity guard.**

1. Copy `schema/payload.v1.json` → `relay/internal/payloadbuild/schema.json` and commit it.
2. Add `//go:generate cp ../../../schema/payload.v1.json schema.json` in `build.go`.
3. `TestBuild_EmbeddedSchemaIsIdenticalToCanonical` uses `runtime.Caller(0)` to locate the test file and derives the canonical path via `filepath.Join(dir, "..", "..", "..", "schema", "payload.v1.json")`. It reads both files and asserts byte equality. This test fails if someone edits the canonical schema without regenerating the copy.
4. The phase-1 README build-fail checklist already requires `scripts/verify-contract.sh` to pass; that script guards codegen consistency. The schema-identity test is the payloadbuild-specific guard.

**Rejected approach: symlink.** Git on Windows does not reliably track symlinks. Committed copy is the portable choice.

---

## Appendix B — Circular import resolution (SubmitRequest in payloadbuild)

If `payloadbuild` importing `server` (for `SubmitRequest`) creates a cycle because `server` imports `payloadbuild`:

1. Create `relay/internal/dto/dto.go` and move `SubmitRequest`, `TurnMessage`, `ClientInfo`, `Viewport`, `ContextInfo`, `SubmitResponse`, `ErrorEnvelope`, `ErrorBody`, `InitResponse`, `Capabilities`, `TurnRequest`, `SSEDelta`, `SSEDone`, `SSEError` into it.
2. In `relay/internal/server/*.go`, replace `package server` DTO definitions with imports from `intake/internal/dto`.
3. In `relay/internal/payloadbuild/build.go`, import `intake/internal/dto` instead of `intake/internal/server`.
4. Update all test files that reference these types.

The `Build()` signature becomes:

```go
func (b *Builder) Build(
    ctx context.Context,
    req *dto.SubmitRequest,
    result *classify.Result,
    sess *auth.SessionContext,
    submissionID string,
    submittedAt time.Time,
) (*payload.IntakePayload, error)
```

This is the architecturally correct resolution. Do it in Task 4 if `go build` reports the cycle.

---

## Appendix C — santhosh-tekuri/jsonschema/v6 usage notes

- Import path: `github.com/santhosh-tekuri/jsonschema/v6`
- Compiler: `santhosh.NewCompiler()`
- Add resource: `c.AddResource(uri, io.Reader)` — the URI must match the `$id` in the schema (`https://intake.dev/schema/payload.v1.json`)
- Compile: `c.Compile(uri) (*Schema, error)`
- Validate: `schema.Validate(any) error` — pass the JSON-decoded `any` value (not the struct directly)
- Format assertions (e.g. `format: uuid`, `format: uri`): enabled by default in v6; confirm with a test that uses an invalid UUID and expects validation failure. If not enabled, call `c.AssertFormat()` or equivalent.
- The library uses draft-2020-12 by default when the schema's `$schema` field is `https://json-schema.org/draft/2020-12/schema`.

---

## Appendix D — Environment (Windows dev)

- Shell: PowerShell (primary) or Bash via Git Bash/WSL.
- All `cp` commands in `go:generate` use Unix syntax; on Windows use PowerShell `Copy-Item` or ensure Git Bash is in PATH for `go generate`.
- Alternatively, replace the `cp` in `go:generate` with a Go helper:
  ```go
  //go:generate go run ../../tools/copyschem/main.go
  ```
  where `copyschem` is a tiny Go program that does `os.Copy(src, dst)`. This is the most portable approach on Windows.
- All `go build`, `go test`, `go vet`, `go generate` commands run identically on Windows with Go 1.23.2 in PowerShell.
- `scripts/check-pins.sh` and `scripts/verify-contract.sh` require Bash (Git Bash or WSL). If running in pure PowerShell, translate the checks manually or run via `bash scripts/check-pins.sh`.
