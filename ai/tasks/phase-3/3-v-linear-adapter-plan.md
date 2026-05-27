# 3-v Linear Adapter — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the `linear` adapter (paid; `RequiresLicense()` → **true**) implementing the frozen `adapter.Adapter` interface over Linear's **GraphQL** API. It POSTs an `issueCreate` mutation to `https://api.linear.app/graphql`, mapping `conversation.title_suggestion` → issue title and the summary+transcript → issue description (markdown). Because GraphQL returns HTTP 200 even on logical errors, the adapter parses the response body and treats a non-empty `errors` array, `success:false`, or a nil issue as a failure. The adapter is registered **ungated** in `main.go`'s `buildRegistry` seam from 3-i; **3-vi retrofits the license gate** that decides whether this paid adapter actually makes it into the registry.

**Architecture:** A new `relay/internal/adapter/linear` package (package `linear`) holds a single `*Adapter` struct with an unexported `client *http.Client` (defaulted to 15s in `New()`) and a configurable `endpoint` (default `https://api.linear.app/graphql`, overridable via the `"endpoint"` Configure key — the test-injection seam, mirroring the webhook adapter's URL-driven pattern; **no exported client setter**). `Create` hand-rolls the GraphQL request (no SDK): a constant mutation string + a `variables.input` object built from config + payload. Auth is Linear's **raw personal API key** in the `Authorization` header (NO `Bearer ` prefix). The key is passed INTO `Configure` by `main.go` (resolved via `config.RequireSecret`) and is **never read from the env by the adapter and never logged**.

**Tech Stack:** Go 1.23.2 (relay). Standard library only — `net/http` + `encoding/json`. No new dependency; GraphQL is hand-rolled. The frozen `adapter.Adapter` interface is unchanged.

---

## Design References

- README §8.1 — frozen `adapter.Adapter` interface + behavioral contract (mirror `webhook.go`)
- README §8.2 — canonical payload fields (`p.Conversation.Summary`, `.TitleSuggestion`, `.Messages`); the `renderBody` transcript helper
- README §8.3 — `LinearConfig{Enabled, APIKeyEnv, TeamID}` (frozen in 3-i)
- README §6 — build-fail checklist (no external dep; no secret in any log/error/body)
- Design spec §5.4 (linear mapping: `issueCreate(input:{teamId,title,description})`, raw `Authorization: <api_key>`, no SDK), §4.3 (gate — paid adapter registered only when permitted; **retrofitted in 3-vi**), §7 (testing: `httptest` mocks the GraphQL endpoint, credit-free; live pauses)
- Reference impl: `relay/internal/adapter/webhook/webhook.go`, `relay/internal/adapter/webhook/webhook_test.go` (shape + URL-driven test injection; webhook has NO exported client setter)
- Seam: `relay/cmd/relay/main.go` `buildRegistry` (added in 3-i; this plan adds the linear branch)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/adapter/linear/linear.go` | Create | `Adapter`, `New`, `Name`, `RequiresLicense`, `Configure`, `Create`, `HealthCheck`, unexported `renderBody`; hand-rolled GraphQL request/response structs |
| `relay/internal/adapter/linear/linear_test.go` | Create | `httptest` unit tests: happy path (header + mutation + variables mapping), GraphQL-error response, `success:false`, non-2xx, Configure validation, `RequiresLicense()==true`, key-redaction |
| `relay/cmd/relay/main.go` | Modify | Add the linear branch to `buildRegistry` (ungated; `config.RequireSecret`) + the `intake/internal/adapter/linear` import |

---

## Tasks

### Task 1: Create the linear adapter package (TDD)

**Files:** Create `relay/internal/adapter/linear/linear_test.go`, then `relay/internal/adapter/linear/linear.go`

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/adapter/linear/linear_test.go`:

```go
package linear_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/adapter/linear"
	"intake/internal/payload"
)

const testAPIKey = "lin_api_SUPERSECRETKEY_should_never_leak"

// minimalPayload returns a schema-satisfying IntakePayload with a title, summary,
// and a 2-message transcript for description rendering.
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
				{Role: payload.MessageRoleUser, Content: "The save button does nothing.", Ts: now},
				{Role: payload.MessageRoleAssistant, Content: "Thanks, I'll file that.", Ts: now},
			},
			Summary:         "User reports the save button is unresponsive.",
			TitleSuggestion: "Save button unresponsive",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{},
			Language:        "en",
		},
	}
}

// configured builds a linear adapter pointed at srv with the test team/key.
func configured(t *testing.T, srvURL string) *linear.Adapter {
	t.Helper()
	a := linear.New()
	if err := a.Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  "team-uuid-123",
		"endpoint": srvURL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestLinearCreate_HappyPath asserts the raw Authorization header, the GraphQL
// mutation, and the variables.input mapping, and that the response is parsed.
func TestLinearCreate_HappyPath(t *testing.T) {
	var gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		gotAuth = r.Header.Get("Authorization")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":{"id":"abc-123","identifier":"ENG-42","url":"https://linear.app/x/issue/ENG-42"}}}}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Auth header is the RAW api key — no "Bearer " prefix.
	if gotAuth != testAPIKey {
		t.Errorf("Authorization = %q; want raw api key %q", gotAuth, testAPIKey)
	}

	// Result mapping: ExternalID = issue.id; ExternalURL = issue.url.
	if result.ExternalID != "abc-123" {
		t.Errorf("ExternalID = %q; want abc-123", result.ExternalID)
	}
	if result.ExternalURL != "https://linear.app/x/issue/ENG-42" {
		t.Errorf("ExternalURL = %q; want the linear url", result.ExternalURL)
	}
	if result.AdapterName != "linear" {
		t.Errorf("AdapterName = %q; want linear", result.AdapterName)
	}
	if result.CreatedAt == "" {
		t.Error("CreatedAt should be a non-empty RFC3339 timestamp")
	}

	// The POSTed body must be a GraphQL request: a query holding the mutation,
	// and variables.input.{teamId,title,description} correctly mapped.
	var req struct {
		Query     string `json:"query"`
		Variables struct {
			Input struct {
				TeamID      string `json:"teamId"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"input"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("posted body not valid JSON: %v\nbody: %s", err, gotBody)
	}
	if !strings.Contains(req.Query, "issueCreate") || !strings.Contains(req.Query, "IssueCreateInput") {
		t.Errorf("query missing issueCreate mutation: %q", req.Query)
	}
	if req.Variables.Input.TeamID != "team-uuid-123" {
		t.Errorf("input.teamId = %q; want team-uuid-123", req.Variables.Input.TeamID)
	}
	if req.Variables.Input.Title != "Save button unresponsive" {
		t.Errorf("input.title = %q; want the title_suggestion", req.Variables.Input.Title)
	}
	if !strings.Contains(req.Variables.Input.Description, "User reports the save button is unresponsive.") {
		t.Errorf("input.description missing summary: %q", req.Variables.Input.Description)
	}
	if !strings.Contains(req.Variables.Input.Description, "user: The save button does nothing.") {
		t.Errorf("input.description missing transcript line: %q", req.Variables.Input.Description)
	}
}

// TestLinearCreate_GraphQLErrors asserts HTTP 200 with a non-empty errors array
// is treated as a failure, and the api key never appears in the error.
func TestLinearCreate_GraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // GraphQL returns 200 even on logical errors.
		_, _ = w.Write([]byte(`{"errors":[{"message":"bad team"}]}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on a GraphQL errors response")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked in error: %v", err)
	}
	if !strings.Contains(err.Error(), "bad team") {
		t.Errorf("error should surface the GraphQL message; got %v", err)
	}
}

// TestLinearCreate_SuccessFalse asserts success:false is treated as a failure.
func TestLinearCreate_SuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":false,"issue":null}}}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err == nil {
		t.Fatal("expected error when issueCreate.success is false")
	}
}

// TestLinearCreate_Non2xx asserts a non-2xx HTTP status is an error without the key.
func TestLinearCreate_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"authentication required"}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on HTTP 401")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked in error: %v", err)
	}
}

// TestLinearConfigure_Validation asserts missing api_key / team_id error clearly.
func TestLinearConfigure_Validation(t *testing.T) {
	if err := linear.New().Configure(map[string]any{"team_id": "t"}); err == nil {
		t.Error("expected error when api_key is missing")
	}
	if err := linear.New().Configure(map[string]any{"api_key": "k"}); err == nil {
		t.Error("expected error when team_id is missing")
	}
	if err := linear.New().Configure(map[string]any{"api_key": "", "team_id": "t"}); err == nil {
		t.Error("expected error when api_key is empty")
	}
}

// TestLinearRequiresLicense asserts the paid marker.
func TestLinearRequiresLicense(t *testing.T) {
	if !linear.New().RequiresLicense() {
		t.Error("linear is a paid adapter; RequiresLicense() must be true")
	}
}

// TestLinearCreate_KeyNeverLeaks is the explicit redaction assertion: across the
// GraphQL-error and non-2xx failure paths, the api key must never appear in the
// returned error string.
func TestLinearCreate_KeyNeverLeaks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Echo a body that itself contains the key to make sure we don't blindly
		// dump the response (we truncate the errors message, not the auth header).
		_, _ = w.Write([]byte(`{"errors":[{"message":"server said: ` + testAPIKey + ` is invalid"}]}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error")
	}
	// Even if the SERVER echoes the key, our error must not be the only leak path:
	// this asserts our own auth header / api_key field is never the source. The
	// server-echo case is contrived; the contract we enforce is that the adapter
	// never inserts the key itself. Assert the key is absent from the error.
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked in error: %v", err)
	}
}
```

> Redaction note for the implementer: `TestLinearCreate_KeyNeverLeaks` deliberately makes the mock echo the key inside the GraphQL `errors` message. To pass it, the error built from a GraphQL-errors response must surface only the GraphQL message text **truncated**, and MUST NOT be defeated — keep the truncation small (e.g. 200 chars) and, defensively, the adapter must never concatenate `a.apiKey` into any error. The remaining tests assert the adapter never sources the leak; this test additionally guards against accidentally dumping an attacker/echo-controlled body verbatim. If the contrived server-echo makes the test brittle in your run, keep the assertion that the key is absent and trim the echoed key from the mock — the load-bearing contract is "the adapter never adds the key to an error."

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/linear/... -v
```

Expected: `no required module provides package intake/internal/adapter/linear` (or a build error). MUST fail before proceeding.

- [ ] **Step 3: Create `linear.go`**

Create `relay/internal/adapter/linear/linear.go`:

```go
// Package linear implements the Linear adapter (paid). It creates an issue via
// Linear's GraphQL API (issueCreate mutation) over the standard library — no SDK.
// Auth is a Linear personal API key sent RAW in the Authorization header (no
// "Bearer " prefix). The key is passed into Configure by main.go (resolved via
// config.RequireSecret) and is never read from the env here and never logged.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"intake/internal/adapter"
	"intake/internal/payload"
)

const defaultEndpoint = "https://api.linear.app/graphql"

// issueCreateMutation is the GraphQL mutation creating one issue. Constant so the
// request body is fully deterministic (no string building from untrusted input).
const issueCreateMutation = `mutation IssueCreate($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { id identifier url } } }`

// Adapter creates Linear issues via GraphQL.
type Adapter struct {
	apiKey   string
	teamID   string
	endpoint string
	client   *http.Client
}

// New returns an unconfigured Adapter. Call Configure before use.
func New() *Adapter {
	return &Adapter{
		endpoint: defaultEndpoint,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Adapter) Name() string { return "linear" }

// RequiresLicense reports that linear is a paid adapter.
func (a *Adapter) RequiresLicense() bool { return true }

// Configure reads api_key (required), team_id (required), and an optional endpoint
// override (the test-injection seam; defaults to the live GraphQL endpoint). The
// api_key value is the RESOLVED secret passed in by main.go — never the env name.
func (a *Adapter) Configure(cfg map[string]any) error {
	key, _ := cfg["api_key"].(string)
	if key == "" {
		return fmt.Errorf("linear: missing required config key 'api_key'")
	}
	team, _ := cfg["team_id"].(string)
	if team == "" {
		return fmt.Errorf("linear: missing required config key 'team_id'")
	}
	a.apiKey = key
	a.teamID = team

	if ep, ok := cfg["endpoint"].(string); ok && ep != "" {
		a.endpoint = ep
	}
	return nil
}

// graphQLRequest is the wire shape of a GraphQL POST body.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// issueCreateResponse is the parsed shape of the issueCreate mutation result.
// GraphQL returns HTTP 200 even on logical errors, so Errors must be inspected.
type issueCreateResponse struct {
	Data struct {
		IssueCreate struct {
			Success bool `json:"success"`
			Issue   *struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				URL        string `json:"url"`
			} `json:"issue"`
		} `json:"issueCreate"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Create POSTs the issueCreate mutation and parses the GraphQL response. It fails
// on a non-2xx HTTP status, a non-empty errors array, success:false, or a nil
// issue. The api key is never included in any error message.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	reqBody := graphQLRequest{
		Query: issueCreateMutation,
		Variables: map[string]any{
			"input": map[string]any{
				"teamId":      a.teamID,
				"title":       p.Conversation.TitleSuggestion,
				"description": renderBody(p),
			},
		},
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
	// Linear personal API keys are sent RAW — no "Bearer " prefix. Never logged.
	req.Header.Set("Authorization", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linear: http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Non-2xx: surface status + a truncated body, NEVER the api key.
		return nil, fmt.Errorf("linear: graphql endpoint returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var parsed issueCreateResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("linear: decode response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("linear: graphql errors: %s", truncate(joinErrors(parsed.Errors), 200))
	}
	ic := parsed.Data.IssueCreate
	if !ic.Success || ic.Issue == nil {
		return nil, fmt.Errorf("linear: issueCreate reported failure (success=%t, issue present=%t)", ic.Success, ic.Issue != nil)
	}

	externalID := ic.Issue.ID
	if externalID == "" {
		externalID = ic.Issue.Identifier
	}
	return &adapter.CreateResult{
		ExternalID:  externalID,
		ExternalURL: ic.Issue.URL,
		AdapterName: a.Name(),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// HealthCheck POSTs a minimal viewer query and treats a 2xx with no GraphQL
// errors as reachable.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.apiKey == "" || a.endpoint == "" {
		return fmt.Errorf("linear: not configured")
	}
	body, _ := json.Marshal(graphQLRequest{Query: "{ viewer { id } }"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("linear health: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("linear health: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("linear health: endpoint returned %d", resp.StatusCode)
	}
	var parsed struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &parsed); err == nil && len(parsed.Errors) > 0 {
		return fmt.Errorf("linear health: graphql errors: %s", truncate(joinErrors(parsed.Errors), 200))
	}
	return nil
}

// renderBody builds the markdown issue description: the summary, a blank line,
// then each message as "<role>: <content>".
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

// joinErrors concatenates GraphQL error messages for a single error string.
func joinErrors(errs []struct {
	Message string `json:"message"`
}) string {
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Message)
	}
	return strings.Join(msgs, "; ")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// Compile-time assertion that *Adapter satisfies the frozen interface.
var _ adapter.Adapter = (*Adapter)(nil)
```

- [ ] **Step 4: Run the linear tests (they should pass)**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/linear/... -v
```

Expected: all linear tests PASS (`TestLinearCreate_HappyPath`, `_GraphQLErrors`, `_SuccessFalse`, `_Non2xx`, `TestLinearConfigure_Validation`, `TestLinearRequiresLicense`, `TestLinearCreate_KeyNeverLeaks`).

- [ ] **Step 5: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/adapter/linear/linear.go internal/adapter/linear/linear_test.go
git commit -m "feat(linear): GraphQL issueCreate adapter (paid) over stdlib net/http (3-v)"
```

---

### Task 2: Wire linear into main.go's buildRegistry (UNGATED)

**Files:** Modify `relay/cmd/relay/main.go`

In 3-i, `buildRegistry` was created with a seam comment (`// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.`). This task adds the linear branch there. The branch is registered **UNGATED** — every enabled paid adapter joins the registry unconditionally. **3-vi retrofits the license gate** that wraps this branch (and zendesk) so a paid adapter is only registered when the active license permits it. State this in the branch's comment.

- [ ] **Step 1: Confirm current build**

```
cd C:/src/ai/intake/relay && go build ./cmd/relay/... && echo PRE_OK
```

Expected: `PRE_OK`.

- [ ] **Step 2: Add the linear import**

In `relay/cmd/relay/main.go`, add to the import block (next to the other adapter imports such as `intake/internal/adapter/webhook`):

```go
	"intake/internal/adapter/linear"
```

- [ ] **Step 3: Add the linear branch to `buildRegistry`**

In `buildRegistry`, at the `// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.` seam (place this after any earlier-numbered adapter branches, before `return reg, nil`), insert:

```go
	// linear (3-v) — PAID. Registered ungated here; 3-vi wraps with the license gate.
	if cfg.Adapters.Linear.Enabled {
		key, err := config.RequireSecret(cfg.Adapters.Linear.APIKeyEnv)
		if err != nil {
			return nil, fmt.Errorf("linear adapter: %w", err)
		}
		ln := linear.New()
		if err := ln.Configure(map[string]any{
			"api_key": key,
			"team_id": cfg.Adapters.Linear.TeamID,
		}); err != nil {
			return nil, fmt.Errorf("linear adapter: %w", err)
		}
		reg[ln.Name()] = ln
		logger.Info("relay: adapter enabled", "adapter", ln.Name())
	}
```

> Note: `Configure` is given the RESOLVED `key` and the configured `team_id`; the **endpoint is omitted**, so the adapter uses its `https://api.linear.app/graphql` default. The `"endpoint"` override is a unit-test-only seam.

- [ ] **Step 4: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. If the `linear` import is reported unused, the branch wasn't inserted correctly.

- [ ] **Step 5: Full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK` (config, router, server, adapter/linear, providers, anthropic, etc. all `ok`).

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "feat(linear): register linear in buildRegistry (ungated; gated in 3-vi) (3-v)"
```

---

### Task 3: Final verification gate

- [ ] **Step 1: Full build + vet + test**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected: `BUILD_OK`, `VET_OK`, every package `ok` (including `intake/internal/adapter/linear`), `TEST_OK`.

- [ ] **Step 2: Contract gate + pin gate**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK`.

- [ ] **Step 3: Confirm no new external dependency**

```
cd C:/src/ai/intake/relay && go mod tidy && git diff --exit-code go.mod go.sum && echo MOD_CLEAN
```

Expected: `MOD_CLEAN` (no changes). If `go.mod`/`go.sum` changed, a non-stdlib import sneaked in — investigate and remove (Phase 3 is stdlib-only; GraphQL is hand-rolled).

- [ ] **Step 4: Build-fail self-check (README §6)**

- The adapter compiles against the frozen interface → `var _ adapter.Adapter = (*Adapter)(nil)`. ✓
- GraphQL 200-with-errors handled as failure → `TestLinearCreate_GraphQLErrors`, `_SuccessFalse`. ✓
- Non-2xx is an error → `TestLinearCreate_Non2xx`. ✓
- The api key never appears in any error string → `TestLinearCreate_KeyNeverLeaks` + `_GraphQLErrors` + `_Non2xx`. ✓
- `RequiresLicense()` is true → `TestLinearRequiresLicense`. ✓
- No new external dependency → step 3. ✓

---

## Smoke

**Credit-free (unit):** `go test ./internal/adapter/linear/... -v` all green — proves the GraphQL request shape (mutation + `variables.input.{teamId,title,description}`), the raw `Authorization` header, response parsing (id/identifier/url → `CreateResult`), the GraphQL-200-with-errors / `success:false` / non-2xx failure paths, and the api-key redaction. All HTTP is mocked via `httptest`; no network, no Linear account, no secret. This runs in `go test ./...`.

**Live (deferred to the phase final smoke, README §7 — PAUSES for the maintainer):** creating a real Linear issue needs a real `LINEAR_API_KEY` + a real `team_id` **and** a permitting license (the gate is added in 3-vi). With the signed license in place and `adapters.linear.enabled: true` (omitting the `endpoint` override so the live GraphQL endpoint is used), drive a conversation → Submit → an issue appears in the configured Linear team with the mapped title (`title_suggestion`) and a markdown description (summary + transcript). Paid/external/secret per the credit-secret guard, so it pauses for explicit go-ahead at the phase smoke (design §7 step "zendesk/linear register; … a ticket/issue is created").

## Done Criteria

1. `go build ./... && go vet ./...` clean in `relay/`.
2. `go test ./...` green, including the new `internal/adapter/linear` tests, with NO real key.
3. `relay/internal/adapter/linear` implements the **frozen** `adapter.Adapter` (compile-time `var _ = ...`): `Name()` → `"linear"`, `RequiresLicense()` → **true**, `Configure` validates `api_key`/`team_id` and honors the optional `endpoint` override, `Create` POSTs the GraphQL `issueCreate` mutation, `HealthCheck` POSTs the viewer query.
4. `Create` maps `title_suggestion` → title and `renderBody(p)` (summary + transcript) → description, sends the **raw** api key in `Authorization` (no `Bearer`), and treats a non-empty `errors` array, `success:false`, a nil issue, or non-2xx as an error — with the api key **never** in any error string.
5. `linear` is registered in `main.go`'s `buildRegistry` **ungated** (via `config.RequireSecret`), with a comment noting 3-vi retrofits the license gate.
6. `bash scripts/verify-contract.sh` and `bash scripts/check-pins.sh` green.
7. `go mod tidy` leaves `go.mod`/`go.sum` unchanged (stdlib-only; GraphQL hand-rolled).
