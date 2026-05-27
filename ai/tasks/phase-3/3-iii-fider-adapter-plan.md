# 3-iii Fider Adapter — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the **fider** adapter (`relay/internal/adapter/fider/`) — a free (`RequiresLicense()` → false) implementation of the frozen `adapter.Adapter` interface that creates a Fider post via `POST {base_url}/api/v1/posts` with a Bearer key. It mirrors the Phase-1 `webhook` reference adapter exactly. After this sub-plan the fider adapter is constructed in `main.go`'s `buildRegistry` (at the 3-i seam) when `adapters.fider.enabled: true`, joins the router registry, and is fully covered by credit-free `httptest` unit tests.

**Architecture:** A new `relay/internal/adapter/fider` package with one `Adapter` struct. `New()` returns an unconfigured adapter holding a default `*http.Client` (test-injectable, copied verbatim from `webhook.go`). `Configure(map[string]any)` reads `base_url` and `api_key` (both required; the `api_key` is the **resolved key value** passed in by `main.go`, NOT an env-var name) and validates them. `Create` marshals `{title, description}`, POSTs with `Authorization: Bearer <api_key>` + `Content-Type: application/json`, treats 2xx as success (parsing `{id, number}`), and returns an error with a truncated body — never the key — on non-2xx. `HealthCheck` mirrors webhook's HEAD probe with the Authorization header set. `main.go` resolves the key via `config.RequireSecret` and passes it into `Configure`; the adapter never reads the env and never logs the key.

**Tech Stack:** Go 1.23.2 (relay). Standard library only — `net/http`, `encoding/json`, `bytes`, `context`, `fmt`, `io`, `strconv`, `time`. No new dependencies (no `go.mod`/`go.sum` change). The frozen `adapter.Adapter` interface is unchanged.

---

## Design References

- README §8.1 — frozen `adapter.Adapter` interface + behavioral contract (verbatim, consumed here)
- README §8.2 — canonical payload fields available to adapters; `renderBody(p)` transcript helper recommendation
- README §8.3 — `FiderConfig{Enabled, BaseURL, APIKeyEnv}` (frozen in 3-i; read by `main.go`)
- README §8.4 — the router/registry this adapter registers into; the `buildRegistry` seam from 3-i
- Design spec §5.2 — fider mapping: `POST {base_url}/api/v1/posts` with `{title, description}`, `Authorization: Bearer <api_key>`; `title_suggestion` → `title`; `summary` + transcript → `description`
- Design spec §7 — credit-free per-adapter testing (httptest success body, non-2xx path, key-redaction assertion)
- Reference impl: `relay/internal/adapter/webhook/webhook.go`, `relay/internal/adapter/webhook/webhook_test.go` — New()/Configure/Create/HealthCheck, test-injectable `client *http.Client`, `truncate` helper, token never logged
- Payload: `relay/internal/payload/types.go` — `p.Conversation.Summary`, `.TitleSuggestion`, `.Messages []Message{Role, Content, Ts}`
- Secrets: `relay/internal/config/secret.go` — `config.RequireSecret(envName)` (used by `main.go`, not the adapter)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/adapter/fider/fider.go` | Create | `Adapter` struct, `New`, `Name`, `RequiresLicense`, `Configure`, `Create`, `HealthCheck`, `renderBody`, `truncate`; `var _ adapter.Adapter` assertion |
| `relay/internal/adapter/fider/fider_test.go` | Create | `httptest` unit tests: posts `{title,description}` with Bearer header → `{id,number}` mapping; non-2xx error; Configure validation; key-redaction |
| `relay/cmd/relay/main.go` | Modify | Add the fider branch to `buildRegistry` at the `// 3-iii fider … added here` seam; add the `intake/internal/adapter/fider` import |

---

## Tasks

### Task 1: Create the fider adapter package (TDD)

**Files:** Create `relay/internal/adapter/fider/fider_test.go`, then `relay/internal/adapter/fider/fider.go`

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/adapter/fider/fider_test.go`:

```go
package fider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/adapter/fider"
	"intake/internal/payload"
)

// minimalPayload returns a schema-satisfying IntakePayload with a title, summary,
// and a two-message transcript for body rendering.
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
			Messages: []payload.Message{
				{Role: payload.MessageRoleUser, Content: "The export button does nothing.", Ts: now},
				{Role: payload.MessageRoleAssistant, Content: "Thanks, which browser?", Ts: now},
			},
			Summary:         "Export button is unresponsive on the reports page.",
			TitleSuggestion: "Export button does nothing",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{},
			Language:        "en",
		},
	}
}

const testKey = "fdr_supersecret_key_value"

// configure builds and configures an adapter pointed at the given base URL.
func configure(t *testing.T, baseURL string) *fider.Adapter {
	t.Helper()
	a := fider.New()
	if err := a.Configure(map[string]any{
		"base_url": baseURL,
		"api_key":  testKey,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestFiderCreate_PostsPostWithBearer asserts the adapter POSTs {title,description}
// to /api/v1/posts with the Bearer header, and maps the {id,number} response.
func TestFiderCreate_PostsPostWithBearer(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
		gotAuth   string
		gotCT     string
		gotBody   []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":7,"number":42}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q; want POST", gotMethod)
	}
	if gotPath != "/api/v1/posts" {
		t.Errorf("path = %q; want /api/v1/posts", gotPath)
	}
	if gotAuth != "Bearer "+testKey {
		t.Errorf("Authorization = %q; want Bearer <key>", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", gotCT)
	}

	var sent struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("request body not valid JSON: %v\nbody: %s", err, gotBody)
	}
	if sent.Title != "Export button does nothing" {
		t.Errorf("title = %q; want title_suggestion", sent.Title)
	}
	if !strings.Contains(sent.Description, "Export button is unresponsive on the reports page.") {
		t.Errorf("description missing summary; got %q", sent.Description)
	}
	if !strings.Contains(sent.Description, "user: The export button does nothing.") {
		t.Errorf("description missing user message line; got %q", sent.Description)
	}
	if !strings.Contains(sent.Description, "assistant: Thanks, which browser?") {
		t.Errorf("description missing assistant message line; got %q", sent.Description)
	}

	if result.ExternalID != "7" {
		t.Errorf("ExternalID = %q; want 7", result.ExternalID)
	}
	if !strings.HasSuffix(result.ExternalURL, "/posts/42") {
		t.Errorf("ExternalURL = %q; want suffix /posts/42", result.ExternalURL)
	}
	if result.AdapterName != "fider" {
		t.Errorf("AdapterName = %q; want fider", result.AdapterName)
	}
	if result.CreatedAt == "" {
		t.Error("CreatedAt should be a non-empty RFC3339 timestamp")
	}
	if _, perr := time.Parse(time.RFC3339, result.CreatedAt); perr != nil {
		t.Errorf("CreatedAt = %q not RFC3339: %v", result.CreatedAt, perr)
	}
}

// TestFiderCreate_TitleFallsBackToSummary asserts an empty title_suggestion falls
// back to a truncated summary.
func TestFiderCreate_TitleFallsBackToSummary(t *testing.T) {
	var gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var sent struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(body, &sent)
		gotTitle = sent.Title
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"number":1}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	p := minimalPayload()
	p.Conversation.TitleSuggestion = ""
	p.Conversation.Summary = "A short summary used as the title."
	if _, err := a.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotTitle != "A short summary used as the title." {
		t.Errorf("title fallback = %q; want the summary", gotTitle)
	}
}

// TestFiderCreate_NonSuccessErrorRedactsKey asserts a non-2xx response yields an
// error containing the truncated body but NOT the api_key.
func TestFiderCreate_NonSuccessErrorRedactsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["forbidden"]}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected an error on 403")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error string leaked the api_key: %v", err)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention the status code; got %v", err)
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("error should include the (truncated) body; got %v", err)
	}
}

// TestFiderConfigure_MissingKeysError asserts both required keys are validated.
func TestFiderConfigure_MissingKeysError(t *testing.T) {
	t.Run("missing base_url", func(t *testing.T) {
		a := fider.New()
		err := a.Configure(map[string]any{"api_key": testKey})
		if err == nil || !strings.Contains(err.Error(), "base_url") {
			t.Errorf("expected base_url error; got %v", err)
		}
	})
	t.Run("empty base_url", func(t *testing.T) {
		a := fider.New()
		err := a.Configure(map[string]any{"base_url": "", "api_key": testKey})
		if err == nil || !strings.Contains(err.Error(), "base_url") {
			t.Errorf("expected base_url error; got %v", err)
		}
	})
	t.Run("missing api_key", func(t *testing.T) {
		a := fider.New()
		err := a.Configure(map[string]any{"base_url": "https://feedback.example.com"})
		if err == nil || !strings.Contains(err.Error(), "api_key") {
			t.Errorf("expected api_key error; got %v", err)
		}
	})
	t.Run("empty api_key", func(t *testing.T) {
		a := fider.New()
		err := a.Configure(map[string]any{"base_url": "https://feedback.example.com", "api_key": ""})
		if err == nil || !strings.Contains(err.Error(), "api_key") {
			t.Errorf("expected api_key error; got %v", err)
		}
	})
}

// TestFiderConfigure_ErrorNeverLeaksKey asserts a Configure validation error never
// echoes the key value.
func TestFiderConfigure_ErrorNeverLeaksKey(t *testing.T) {
	a := fider.New()
	// base_url missing but api_key present — the error must not contain the key.
	err := a.Configure(map[string]any{"api_key": testKey})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("Configure error leaked the api_key: %v", err)
	}
}

// TestFiderHealthCheck_OK asserts a non-5xx response (with the Bearer header set)
// is healthy.
func TestFiderHealthCheck_OK(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	if err := a.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if gotAuth != "Bearer "+testKey {
		t.Errorf("health Authorization = %q; want Bearer <key>", gotAuth)
	}
}
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/fider/... -v
```

Expected: `no required module provides package intake/internal/adapter/fider` (or a build error that the package does not exist). MUST fail before proceeding.

- [ ] **Step 3: Create `fider.go`**

Create `relay/internal/adapter/fider/fider.go`:

```go
// Package fider implements the adapter.Adapter for Fider (https://fider.io),
// a free feature-feedback board. It creates a post via POST {base_url}/api/v1/posts
// with an "Authorization: Bearer <api_key>" header. It mirrors the webhook
// reference adapter: stdlib net/http only, a test-injectable *http.Client, and
// the api_key is NEVER logged or placed in an error string.
package fider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"intake/internal/adapter"
	"intake/internal/payload"
)

// maxTitleLen bounds the summary-derived title fallback. The canonical
// title_suggestion is already schema-capped at 80; this only applies to the
// Summary fallback path.
const maxTitleLen = 80

// Adapter creates Fider posts. The api_key is the RESOLVED key value supplied by
// main.go (via config.RequireSecret), never an env-var name and never logged.
type Adapter struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// compile-time assertion that *Adapter satisfies the frozen interface.
var _ adapter.Adapter = (*Adapter)(nil)

// New returns an unconfigured Adapter. Call Configure before use.
func New() *Adapter {
	return &Adapter{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Adapter) Name() string { return "fider" }

func (a *Adapter) RequiresLicense() bool { return false }

// Configure reads base_url and api_key from the map. Both are required. The
// api_key value is passed in by main.go (already resolved); this adapter never
// reads the environment itself.
func (a *Adapter) Configure(cfg map[string]any) error {
	baseURL, _ := cfg["base_url"].(string)
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("fider: missing required config key 'base_url'")
	}
	apiKey, _ := cfg["api_key"].(string)
	if apiKey == "" {
		return fmt.Errorf("fider: missing required config key 'api_key'")
	}
	a.baseURL = strings.TrimRight(baseURL, "/")
	a.apiKey = apiKey
	return nil
}

// createRequest is the JSON body POSTed to /api/v1/posts.
type createRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// createResponse is the relevant subset of the Fider post-create response.
type createResponse struct {
	ID     int `json:"id"`
	Number int `json:"number"`
}

// Create posts a new Fider post built from the canonical payload. 2xx is success;
// any other status returns an error with a truncated body — never the api_key.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	if a.baseURL == "" || a.apiKey == "" {
		return nil, fmt.Errorf("fider: not configured (call Configure first)")
	}

	title := p.Conversation.TitleSuggestion
	if strings.TrimSpace(title) == "" {
		title = truncate(p.Conversation.Summary, maxTitleLen)
	}

	body, err := json.Marshal(createRequest{
		Title:       title,
		Description: renderBody(p),
	})
	if err != nil {
		return nil, fmt.Errorf("fider: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/v1/posts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("fider: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fider: http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Include the truncated body for debugging; NEVER the api_key.
		return nil, fmt.Errorf("fider: create post returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var parsed createResponse
	_ = json.Unmarshal(respBody, &parsed) // tolerate an empty/odd body; fields default to 0

	externalID := ""
	if parsed.ID != 0 {
		externalID = strconv.Itoa(parsed.ID)
	} else if parsed.Number != 0 {
		externalID = strconv.Itoa(parsed.Number)
	}

	externalURL := ""
	if parsed.Number != 0 {
		externalURL = a.baseURL + "/posts/" + strconv.Itoa(parsed.Number)
	}

	return &adapter.CreateResult{
		ExternalID:  externalID,
		ExternalURL: externalURL,
		AdapterName: "fider",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// HealthCheck sends a HEAD request to base_url with the Authorization header set.
// A missing base_url or unreachable host is an error; a non-5xx response
// (including 401/404) is considered reachable and returns nil. Mirrors webhook.go.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.baseURL == "" {
		return fmt.Errorf("fider: not configured (base_url is empty)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, a.baseURL, nil)
	if err != nil {
		return fmt.Errorf("fider health: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("fider health: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("fider health: upstream returned %d", resp.StatusCode)
	}
	return nil
}

// renderBody concatenates the summary, a blank line, then each message as
// "<Role>: <Content>" (README §8.2 recommended shared approach).
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
	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
```

> Test-injectable client: tests in this package share `relay/internal/adapter/fider`'s package boundary via `httptest.NewServer` and the configured `base_url` — no exported setter is required because the default `client` already targets whatever URL `Configure` is given (the same pattern `webhook_test.go` uses). If a future test must inject a custom `*http.Client` (e.g. to force a transport error), add a `WithHTTPClient` helper mirroring the webhook pattern; not needed for this sub-plan.

- [ ] **Step 4: Run the fider tests**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/fider/... -v
```

Expected: all fider tests PASS (`TestFiderCreate_PostsPostWithBearer`, `_TitleFallsBackToSummary`, `_NonSuccessErrorRedactsKey`, `TestFiderConfigure_MissingKeysError`, `_ErrorNeverLeaksKey`, `TestFiderHealthCheck_OK`).

- [ ] **Step 5: Build + vet the package**

```
cd C:/src/ai/intake/relay && go build ./internal/adapter/fider/... && echo BUILD_OK && go vet ./internal/adapter/fider/... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/adapter/fider/fider.go internal/adapter/fider/fider_test.go
git commit -m "feat(fider): free adapter — POST /api/v1/posts with Bearer key (3-iii)"
```

---

### Task 2: Wire fider into main.go's buildRegistry

**Files:** Modify `relay/cmd/relay/main.go`

In 3-i, `buildRegistry` was created with a seam comment (`// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.`). This task adds the fider branch at that seam.

- [ ] **Step 1: Confirm current build + locate the seam**

```
cd C:/src/ai/intake/relay && go build ./cmd/relay/... && echo PRE_OK
```

```
cd C:/src/ai/intake/relay
grep -n "added here\|3-iii fider\|3-ii chatwoot\|func buildRegistry" cmd/relay/main.go
```

Expected: `PRE_OK`, and the grep shows the `buildRegistry` seam comment line. (If 3-ii has already inserted a chatwoot block, add the fider block immediately after it; if neither has been added, add fider at the seam comment. Order between sibling adapter blocks does not matter — the registry is a map.)

- [ ] **Step 2: Add the fider import**

In `relay/cmd/relay/main.go`, add to the import block (alongside `intake/internal/adapter/webhook` and the others):

```go
	"intake/internal/adapter/fider"
```

- [ ] **Step 3: Add the fider branch to `buildRegistry`**

In `buildRegistry`, at the `// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.` seam (insert this block; leave the seam comment in place for the remaining sub-plans):

```go
	// fider (3-iii) — free.
	if cfg.Adapters.Fider.Enabled {
		key, err := config.RequireSecret(cfg.Adapters.Fider.APIKeyEnv)
		if err != nil {
			return nil, fmt.Errorf("fider adapter: %w", err)
		}
		fd := fider.New()
		if err := fd.Configure(map[string]any{
			"base_url": cfg.Adapters.Fider.BaseURL,
			"api_key":  key,
		}); err != nil {
			return nil, fmt.Errorf("fider adapter: %w", err)
		}
		reg[fd.Name()] = fd
		logger.Info("relay: adapter enabled", "adapter", fd.Name())
	}
```

- [ ] **Step 4: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. If `fider` is reported as imported-and-not-used, the branch was not inserted correctly.

- [ ] **Step 5: Full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK` — every package `ok`, including the new `internal/adapter/fider`.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "feat(fider): register adapter in buildRegistry behind adapters.fider.enabled (3-iii)"
```

---

### Task 3: Final verification gate

- [ ] **Step 1: Full build + vet + test**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected: `BUILD_OK`, `VET_OK`, every package `ok`, `TEST_OK`.

- [ ] **Step 2: Contract gate + pin gate**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK`.

- [ ] **Step 3: Confirm no new external dependency**

```
cd C:/src/ai/intake/relay && go mod tidy && git diff --exit-code go.mod go.sum && echo MOD_CLEAN
```

Expected: `MOD_CLEAN` (no changes). If `go.mod`/`go.sum` changed, a non-stdlib import sneaked in — investigate and remove (Phase 3 is stdlib-only).

- [ ] **Step 4: Build-fail self-check (README §6)**

- `var _ adapter.Adapter = (*Adapter)(nil)` compiles → the frozen interface is satisfied exactly. ✓
- `RequiresLicense()` returns `false` (fider is free; never gated). ✓
- The api_key never appears in an error string or log line → `TestFiderCreate_NonSuccessErrorRedactsKey` + `TestFiderConfigure_ErrorNeverLeaksKey`. ✓
- non-2xx → error including the truncated body but not the key → `TestFiderCreate_NonSuccessErrorRedactsKey`. ✓
- no new external dep → step 3 (`MOD_CLEAN`). ✓
- `go vet ./...` / `go build ./...` clean → step 1. ✓

---

## Smoke

**Credit-free (unit):** `go test ./internal/adapter/fider/...` green — an `httptest.Server` stands in for Fider and asserts: (1) `Create` POSTs `{title, description}` to `/api/v1/posts` with `Authorization: Bearer <key>` and `Content-Type: application/json`; a `{"id":7,"number":42}` response maps to `ExternalID == "7"` and `ExternalURL` ending `/posts/42`; (2) an empty `title_suggestion` falls back to a truncated `Summary`; (3) a 403 yields an error containing the truncated body but NOT the key; (4) `Configure` errors clearly name the missing `base_url`/`api_key` and never echo the key; (5) `HealthCheck` sends the Bearer header and treats non-5xx as healthy. No network, no real key, no credits.

**Live (deferred to the phase final smoke, README §7):** point `adapters.fider` at a **running Fider instance** with `enabled: true`, `base_url: <instance>`, and a real `FIDER_API_KEY` env var (resolved by `config.RequireSecret` in `main.go`); drive a 2-turn conversation → Submit → a post appears on the Fider board with the mapped title and the summary+transcript description, and the returned `ExternalURL` opens that post. This needs a real Fider instance + a real API key (external/secret), so it **PAUSES for the maintainer** at the phase final smoke and is not runnable here.

## Done Criteria

1. `relay/internal/adapter/fider/{fider.go,fider_test.go}` exist; `var _ adapter.Adapter = (*Adapter)(nil)` compiles (frozen interface satisfied; unchanged).
2. `Name()` → `"fider"`; `RequiresLicense()` → `false`.
3. `Configure` requires non-empty `base_url` and `api_key` (the resolved value), returning a clear error naming the missing key and never echoing the key value.
4. `Create` POSTs `{title, description}` to `{base_url}/api/v1/posts` with `Authorization: Bearer <api_key>` + `Content-Type: application/json`; maps `{id, number}` → `ExternalID` (id, else number) and `ExternalURL` (`{base_url}/posts/<number>` when number present); non-2xx → error with truncated body, never the key; `CreateResult` has `AdapterName == "fider"` and an RFC3339 `CreatedAt`.
5. `HealthCheck` mirrors webhook's HEAD probe with the Authorization header set (non-5xx = healthy).
6. `main.go`'s `buildRegistry` registers fider when `adapters.fider.enabled` is true, resolving the key via `config.RequireSecret(cfg.Adapters.Fider.APIKeyEnv)` and passing it into `Configure`; the seam comment for the remaining adapters stays in place.
7. `go build ./... && go vet ./... && go test ./...` green in `relay/`; `bash scripts/verify-contract.sh` and `bash scripts/check-pins.sh` green; `go mod tidy` leaves `go.mod`/`go.sum` unchanged (stdlib-only).
