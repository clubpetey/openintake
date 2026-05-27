# 3-ii Chatwoot Adapter — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the free `chatwoot` adapter (`relay/internal/adapter/chatwoot/`) behind the **frozen** `adapter.Adapter` interface, mirroring the `webhook` reference impl. It maps the canonical payload to a Chatwoot conversation-create REST call, and wires into the 3-i `buildRegistry` seam in `main.go`. After this sub-plan, when `adapters.chatwoot.enabled: true`, the relay registers chatwoot, the router can route to it, and (per 3-i) it can serve as `routing.default_adapter`.

**Architecture:** `chatwoot.Adapter` is a stdlib-`net/http` adapter shaped exactly like `webhook.Adapter`: `New()` returns an unconfigured `*Adapter` with a 15s-timeout test-injectable `*http.Client`; `Configure(map[string]any)` reads `base_url`/`account_id`/`inbox_id`/`api_token` (the token is the RESOLVED value, passed in by `main.go`, never read from the env by the adapter, never logged); `Create` POSTs a conversation-create to `{base_url}/api/v1/accounts/{account_id}/conversations` with the `api_access_token` header and parses the returned conversation `id`; `HealthCheck` probes the account endpoint. `RequiresLicense()` returns `false` (free). The frozen `adapter.Adapter` interface is UNCHANGED.

**Tech Stack:** Go 1.23.2 (relay). Standard library only — `net/http` + `encoding/json` (no new dependency). The token resolves in `main.go` via `config.RequireSecret(cfg.Adapters.Chatwoot.APITokenEnv)` and is passed into `Configure`.

---

## Design References

- README §8.1 — frozen `adapter.Adapter` interface + behavioral contract (mirror `webhook.go`)
- README §8.2 — canonical payload fields available to adapters (`p.Conversation.*`, `p.Submission.Id`, `p.User.Email`)
- README §8.3 — `ChatwootConfig` shape (frozen in 3-i: `Enabled`, `BaseURL`, `AccountID`, `InboxID`, `APITokenEnv`)
- README §7 step 3 — live Chatwoot is the phase's **primary live target** (deferred to the phase final smoke)
- Design spec §5.1 — chatwoot endpoint + mapping; §7 — credit-free unit-test contract (httptest, token-redaction)
- Reference impl: `relay/internal/adapter/webhook/webhook.go`, `relay/internal/adapter/webhook/webhook_test.go`
- Frozen interface: `relay/internal/adapter/adapter.go`; generated payload: `relay/internal/payload/types.go` (DO NOT EDIT)
- Registry seam: `relay/cmd/relay/main.go` — the `// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.` comment inside `buildRegistry` (added by 3-i)

> **Caveat — Chatwoot contact / source_id semantics:** the public Chatwoot conversation-create API may, in some deployments, require a pre-created **contact** (and a contact-scoped `source_id`). The precise endpoint/contact flow is **CONFIRMED AT THE LIVE SMOKE** against a real Chatwoot instance (README §7 step 3). For this plan the baseline is a **single conversation POST** carrying `inbox_id`, a `source_id` (= `p.Submission.Id`), and a `message`; the **unit test is what locks the wire behavior** here. Do NOT over-engineer a contact-creation pre-step — if the live smoke shows a contact is mandatory, that is a follow-up adjustment, not this plan's scope.

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/adapter/chatwoot/chatwoot.go` | Create | `Adapter`, `New`, `Name`, `RequiresLicense`, `Configure`, `Create`, `HealthCheck`, `renderBody`, `truncate`; `var _ adapter.Adapter` assertion |
| `relay/internal/adapter/chatwoot/chatwoot_test.go` | Create | httptest-mocked Create (path/header/body/id→ExternalID), non-2xx error, Configure validation, token-redaction |
| `relay/cmd/relay/main.go` | Modify | Add the chatwoot branch to `buildRegistry` at the 3-ii seam; add the `intake/internal/adapter/chatwoot` import |

---

## Tasks

### Task 1: Create the chatwoot adapter (TDD)

**Files:** Create `relay/internal/adapter/chatwoot/chatwoot_test.go`, then `relay/internal/adapter/chatwoot/chatwoot.go`

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/adapter/chatwoot/chatwoot_test.go`:

```go
package chatwoot_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/adapter/chatwoot"
	"intake/internal/payload"
)

const testToken = "super-secret-chatwoot-token-xyz"

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
			Messages: []payload.Message{
				{Role: payload.MessageRoleUser, Content: "The save button does nothing", Ts: now},
				{Role: payload.MessageRoleAssistant, Content: "Thanks, can you share the page URL?", Ts: now},
			},
			Summary:         "Save button is unresponsive on the settings page.",
			TitleSuggestion: "Save button does nothing",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{"ui", "settings"},
			Language:        "en",
		},
	}
}

// configure builds an adapter pointed at the given base URL with the test token.
func configure(t *testing.T, baseURL string) *chatwoot.Adapter {
	t.Helper()
	a := chatwoot.New()
	if err := a.Configure(map[string]any{
		"base_url":   baseURL,
		"account_id": 1,
		"inbox_id":   3,
		"api_token":  testToken,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestChatwootCreate_PostsConversation asserts the adapter POSTs to the right
// path with the api_access_token header and a body carrying the mapped content
// and inbox_id, and that the response id becomes ExternalID/ExternalURL.
func TestChatwootCreate_PostsConversation(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotCT string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("api_access_token")
		gotCT = r.Header.Get("Content-Type")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("request body not valid JSON: %v\nbody: %s", err, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":123,"inbox_id":3}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/v1/accounts/1/conversations" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if gotAuth != testToken {
		t.Errorf("expected api_access_token header = token, got %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", gotCT)
	}

	// inbox_id is mapped (JSON numbers decode to float64).
	if iv, ok := gotBody["inbox_id"].(float64); !ok || int(iv) != 3 {
		t.Errorf("expected inbox_id=3 in body, got %v", gotBody["inbox_id"])
	}
	// source_id is the submission id.
	if sid, _ := gotBody["source_id"].(string); sid != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected source_id=submission id, got %v", gotBody["source_id"])
	}
	// message.content carries the rendered transcript (title + summary + messages).
	msg, ok := gotBody["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected message object in body, got %v", gotBody["message"])
	}
	content, _ := msg["content"].(string)
	for _, want := range []string{"Save button does nothing", "Save button is unresponsive", "user: The save button does nothing", "assistant: Thanks, can you share"} {
		if !strings.Contains(content, want) {
			t.Errorf("message.content missing %q\ncontent: %s", want, content)
		}
	}

	if result.ExternalID != "123" {
		t.Errorf("expected ExternalID 123, got %q", result.ExternalID)
	}
	wantURL := srv.URL + "/app/accounts/1/conversations/123"
	if result.ExternalURL != wantURL {
		t.Errorf("expected ExternalURL %q, got %q", wantURL, result.ExternalURL)
	}
	if result.AdapterName != "chatwoot" {
		t.Errorf("expected AdapterName chatwoot, got %q", result.AdapterName)
	}
	if result.CreatedAt == "" {
		t.Error("CreatedAt should be non-empty")
	}
}

// TestChatwootCreate_NonOKErrorNoToken asserts a non-2xx response returns an
// error that includes the (truncated) body but NEVER the token.
func TestChatwootCreate_NonOKErrorNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Access denied"}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention the status code, got %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("SECURITY: token leaked into error: %v", err)
	}
}

// TestChatwootConfigure_MissingKeys asserts required-key validation.
func TestChatwootConfigure_MissingKeys(t *testing.T) {
	t.Run("missing base_url", func(t *testing.T) {
		a := chatwoot.New()
		err := a.Configure(map[string]any{
			"account_id": 1,
			"inbox_id":   3,
			"api_token":  testToken,
		})
		if err == nil || !strings.Contains(err.Error(), "base_url") {
			t.Fatalf("expected error naming base_url, got %v", err)
		}
	})
	t.Run("missing api_token", func(t *testing.T) {
		a := chatwoot.New()
		err := a.Configure(map[string]any{
			"base_url":   "https://chatwoot.example.com",
			"account_id": 1,
			"inbox_id":   3,
		})
		if err == nil || !strings.Contains(err.Error(), "api_token") {
			t.Fatalf("expected error naming api_token, got %v", err)
		}
	})
}

// TestChatwootConfigure_AcceptsFloatIDs asserts account_id/inbox_id accept the
// float64 form that a JSON/YAML decode may produce (mirrors webhook's retry ints).
func TestChatwootConfigure_AcceptsFloatIDs(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":7}`))
	}))
	defer srv.Close()

	a := chatwoot.New()
	if err := a.Configure(map[string]any{
		"base_url":   srv.URL,
		"account_id": float64(9), // as a JSON/YAML decode might supply it
		"inbox_id":   float64(4),
		"api_token":  testToken,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotPath != "/api/v1/accounts/9/conversations" {
		t.Errorf("float account_id not applied: path %q", gotPath)
	}
	if iv, ok := gotBody["inbox_id"].(float64); !ok || int(iv) != 4 {
		t.Errorf("float inbox_id not applied: %v", gotBody["inbox_id"])
	}
}
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/chatwoot/... -v
```

Expected: `no required module provides package intake/internal/adapter/chatwoot` (or a build error that the package does not exist). MUST fail before proceeding.

- [ ] **Step 3: Create `chatwoot.go`**

Create `relay/internal/adapter/chatwoot/chatwoot.go`:

```go
// Package chatwoot is a free downstream adapter that creates a Chatwoot
// conversation in a configured inbox from the canonical intake payload.
// It mirrors the webhook reference adapter: stdlib net/http + encoding/json,
// a test-injectable *http.Client, and a token that is passed into Configure
// (resolved by main.go) and never read from the env or logged.
package chatwoot

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

// Adapter creates a Chatwoot conversation via the Chatwoot REST API.
type Adapter struct {
	baseURL   string
	accountID int
	inboxID   int
	apiToken  string // resolved value, passed into Configure; never logged
	client    *http.Client
}

// compile-time assertion that Adapter satisfies the frozen interface.
var _ adapter.Adapter = (*Adapter)(nil)

// New returns an unconfigured Adapter. Call Configure before use.
func New() *Adapter {
	return &Adapter{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Adapter) Name() string { return "chatwoot" }

func (a *Adapter) RequiresLicense() bool { return false }

// WithHTTPClient overrides the HTTP client (used by tests to point at httptest).
func (a *Adapter) WithHTTPClient(c *http.Client) *Adapter {
	if c != nil {
		a.client = c
	}
	return a
}

// Configure reads base_url, account_id, inbox_id, api_token from the map.
// base_url and api_token are required; api_token is the RESOLVED token value
// (main.go resolves it via config.RequireSecret) — the adapter never reads the
// environment and never logs the token. account_id/inbox_id accept int or
// float64 (a JSON/YAML decode may supply either).
func (a *Adapter) Configure(cfg map[string]any) error {
	baseVal, ok := cfg["base_url"]
	if !ok {
		return fmt.Errorf("chatwoot: missing required config key 'base_url'")
	}
	baseStr, ok := baseVal.(string)
	if !ok || baseStr == "" {
		return fmt.Errorf("chatwoot: config key 'base_url' must be a non-empty string")
	}
	a.baseURL = strings.TrimRight(baseStr, "/")

	tokVal, ok := cfg["api_token"]
	if !ok {
		return fmt.Errorf("chatwoot: missing required config key 'api_token'")
	}
	tokStr, ok := tokVal.(string)
	if !ok || tokStr == "" {
		return fmt.Errorf("chatwoot: config key 'api_token' must be a non-empty string")
	}
	a.apiToken = tokStr

	if v, ok := cfg["account_id"]; ok {
		switch n := v.(type) {
		case int:
			a.accountID = n
		case float64:
			a.accountID = int(n)
		}
	}
	if v, ok := cfg["inbox_id"]; ok {
		switch n := v.(type) {
		case int:
			a.inboxID = n
		case float64:
			a.inboxID = int(n)
		}
	}

	return nil
}

// conversationRequest is the Chatwoot conversation-create body (subset).
type conversationRequest struct {
	InboxID  int             `json:"inbox_id"`
	SourceID string          `json:"source_id"`
	Message  conversationMsg `json:"message"`
}

type conversationMsg struct {
	Content string `json:"content"`
}

// Create POSTs a conversation-create to Chatwoot and maps the response id to a
// CreateResult. Non-2xx returns an error including the truncated body but never
// the token.
//
// NOTE: some Chatwoot deployments require a pre-created contact and a
// contact-scoped source_id; the exact flow is confirmed at the live smoke
// (phase README §7 step 3). The baseline here is a single conversation POST.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	reqBody := conversationRequest{
		InboxID:  a.inboxID,
		SourceID: p.Submission.Id,
		Message:  conversationMsg{Content: renderBody(p)},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations", a.baseURL, a.accountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("chatwoot: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", a.apiToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Body may echo our request but never the token (it is only in the header).
		return nil, fmt.Errorf("chatwoot: create returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	id, err := extractConversationID(respBody)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: parse response id: %w", err)
	}

	return &adapter.CreateResult{
		ExternalID:  id,
		ExternalURL: fmt.Sprintf("%s/app/accounts/%d/conversations/%s", a.baseURL, a.accountID, id),
		AdapterName: "chatwoot",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// extractConversationID parses {"id": <number>} from the response. Chatwoot
// returns the conversation id as a JSON number; handle it via json.Number.
func extractConversationID(body []byte) (string, error) {
	var raw struct {
		ID json.Number `json:"id"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return "", fmt.Errorf("decode body: %w", err)
	}
	if raw.ID.String() == "" {
		return "", fmt.Errorf("response missing 'id' field")
	}
	return raw.ID.String(), nil
}

// renderBody concatenates the title suggestion (first line), the summary, a
// blank line, then each message as "<Role>: <Content>".
func renderBody(p *payload.IntakePayload) string {
	var b strings.Builder
	if p.Conversation.TitleSuggestion != "" {
		b.WriteString(p.Conversation.TitleSuggestion)
		b.WriteString("\n")
	}
	if p.Conversation.Summary != "" {
		b.WriteString(p.Conversation.Summary)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	for _, m := range p.Conversation.Messages {
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// HealthCheck GETs the account endpoint with the token header. A non-5xx
// response (including 401/404) means Chatwoot is reachable.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.baseURL == "" {
		return fmt.Errorf("chatwoot: not configured (base_url is empty)")
	}
	url := fmt.Sprintf("%s/api/v1/accounts/%s", a.baseURL, strconv.Itoa(a.accountID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("chatwoot health: build request: %w", err)
	}
	req.Header.Set("api_access_token", a.apiToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("chatwoot health: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("chatwoot health: upstream returned %d", resp.StatusCode)
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

- [ ] **Step 4: Run the chatwoot tests**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/chatwoot/... -v
```

Expected: all four tests PASS (`TestChatwootCreate_PostsConversation`, `TestChatwootCreate_NonOKErrorNoToken`, `TestChatwootConfigure_MissingKeys`, `TestChatwootConfigure_AcceptsFloatIDs`).

- [ ] **Step 5: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/adapter/chatwoot/chatwoot.go internal/adapter/chatwoot/chatwoot_test.go
git commit -m "feat(chatwoot): conversation-create adapter over stdlib net/http (3-ii)"
```

---

### Task 2: Wire chatwoot into the main.go registry

**Files:** Modify `relay/cmd/relay/main.go`

The 3-i `buildRegistry` left a seam comment: `// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.` This task inserts the chatwoot branch there.

- [ ] **Step 1: Confirm the seam exists**

```
cd C:/src/ai/intake/relay && grep -n "3-ii chatwoot" cmd/relay/main.go
```

Expected: one match (the seam comment inside `buildRegistry`). If it is missing, 3-i has not landed — stop and complete 3-i first.

- [ ] **Step 2: Add the chatwoot import**

In `relay/cmd/relay/main.go`, add to the import block (alongside `"intake/internal/adapter/webhook"`):

```go
	"intake/internal/adapter/chatwoot"
```

- [ ] **Step 3: Insert the chatwoot branch at the seam**

In `buildRegistry`, replace the seam comment line:

```go
	// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.
```

with the chatwoot branch followed by the (now-narrowed) seam comment:

```go
	// chatwoot (3-ii) — free.
	if cfg.Adapters.Chatwoot.Enabled {
		token, err := config.RequireSecret(cfg.Adapters.Chatwoot.APITokenEnv)
		if err != nil {
			return nil, fmt.Errorf("chatwoot adapter: %w", err)
		}
		cw := chatwoot.New()
		if err := cw.Configure(map[string]any{
			"base_url":   cfg.Adapters.Chatwoot.BaseURL,
			"account_id": cfg.Adapters.Chatwoot.AccountID,
			"inbox_id":   cfg.Adapters.Chatwoot.InboxID,
			"api_token":  token,
		}); err != nil {
			return nil, fmt.Errorf("chatwoot adapter: %w", err)
		}
		reg[cw.Name()] = cw
		logger.Info("relay: adapter enabled", "adapter", cw.Name())
	}

	// 3-iii fider, 3-iv zendesk, 3-v linear are added here.
```

> The token is resolved here via `config.RequireSecret` (fail-fast if an *enabled* chatwoot's token env is unset) and passed into `Configure` as the resolved value. The adapter never reads the env and never logs the token.

- [ ] **Step 4: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. If `chatwoot` import is reported unused, the branch wasn't inserted correctly.

- [ ] **Step 5: Full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK` (config, router, server, adapter/webhook, adapter/chatwoot, providers, anthropic, etc. all `ok`).

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "feat(chatwoot): register in buildRegistry behind enabled flag (3-ii)"
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

- [ ] **Step 4: Token-redaction self-check (README §6)**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/chatwoot/... -run TestChatwootCreate_NonOKErrorNoToken -v
```

Expected: PASS — proves a non-2xx error never contains the token string.

- [ ] **Step 5: Build-fail self-check (README §6)**

- chatwoot satisfies the frozen interface → `var _ adapter.Adapter = (*Adapter)(nil)` compiles. ✓
- token never logged / never in error → `TestChatwootCreate_NonOKErrorNoToken`. ✓
- `RequiresLicense()` → false (free; no gate needed) — chatwoot is registered whenever enabled. ✓
- no new external dep → step 3. ✓
- frozen `adapter.Adapter` interface UNCHANGED — only a new package + a `buildRegistry` branch were added. ✓

---

## Smoke

**Credit-free (unit):** `go test ./internal/adapter/chatwoot/...` green — the httptest mock proves Create POSTs to `/api/v1/accounts/{account_id}/conversations` with the `api_access_token` header and a body carrying `inbox_id`, `source_id`, and the rendered `message.content`; a `{"id":123}` response yields `ExternalID "123"` and the `/app/accounts/{account_id}/conversations/123` URL; a 401 returns an error that excludes the token; Configure validates `base_url`/`api_token`. No network, no real token.

**Live (deferred to the phase final smoke — README §7 step 3; this is the phase's PRIMARY live target):** point `adapters.chatwoot` at a **running Chatwoot instance** with a real `CHATWOOT_TOKEN`, set `routing.default_adapter: "chatwoot"`, drive a conversation via `core/smoke/drive.ts` → Submit → a conversation appears in the configured inbox with the mapped title/summary/transcript. This needs a live Chatwoot + a real token (external/secret) and an `ANTHROPIC_API_KEY` for the classify step, so it **PAUSES for the maintainer** at the phase smoke — it is NOT run here.

> Live caveat (restate): if the live Chatwoot deployment rejects the bare conversation POST and requires a pre-created **contact** / contact-scoped `source_id`, that is a follow-up adjustment confirmed at the smoke — the unit test locks the baseline wire shape this plan delivers.

## Done Criteria

1. `go build ./... && go vet ./...` clean in `relay/`.
2. `go test ./...` green, including the new `adapter/chatwoot` tests, with NO real token.
3. `chatwoot.Adapter` satisfies the frozen `adapter.Adapter` interface (compile-time `var _` assertion); `Name()` = `"chatwoot"`, `RequiresLicense()` = `false`.
4. `Create` POSTs to `{base_url}/api/v1/accounts/{account_id}/conversations` with the `api_access_token` header; maps `inbox_id` + `source_id` (= `p.Submission.Id`) + `message.content` (= `renderBody(p)`); parses the response `id` into `ExternalID` and builds `ExternalURL`; a non-2xx returns an error with the truncated body but NEVER the token.
5. The token is passed into `Configure` (resolved in `main.go` via `config.RequireSecret`); the adapter never reads the env and never logs the token.
6. `buildRegistry` in `main.go` registers chatwoot when `adapters.chatwoot.enabled` is true; the 3-iii…3-v seam comment remains for later sub-plans.
7. `bash scripts/verify-contract.sh` and `bash scripts/check-pins.sh` green; `go mod tidy` leaves `go.mod`/`go.sum` unchanged (stdlib-only).
</content>
</invoke>
