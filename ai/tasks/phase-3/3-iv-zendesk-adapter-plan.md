# 3-iv Zendesk Adapter — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the `zendesk` adapter (`relay/internal/adapter/zendesk/`) — a **paid** adapter (`RequiresLicense()` → true) that creates a Zendesk ticket via the REST API using HTTP basic auth (`{email}/token:{api_token}`). It maps the canonical payload (`title_suggestion` → subject, `summary` + transcript → `comment.body`, `severity_guess` → priority, `tags_suggested` → tags), and registers in `main.go`'s `buildRegistry`. It mirrors the `webhook` reference impl shape exactly, adds no external dependency, and is registered **ungated** here (3-vi retrofits the license gate).

**Architecture:** A new `relay/internal/adapter/zendesk` package implements the **frozen** `adapter.Adapter` interface over stdlib `net/http` + `encoding/json` + `encoding/base64`. `Configure` validates `subdomain`/`email`/`api_token` and derives `baseURL = https://<subdomain>.zendesk.com` (overridable via an optional `base_url` test seam, mirroring webhook's URL-driven injection — no exported client setter). `Create` POSTs `{baseURL}/api/v2/tickets.json` with a base64 basic-auth header that embeds the token; the token and the Authorization header are **never logged** and never appear in any error. `main.go` resolves the token via `config.RequireSecret` and passes it into `Configure`; the registry entry is added **ungated** in this sub-plan (3-vi wraps paid adapters with the license gate).

**Tech Stack:** Go 1.23.2 (relay). **No new dependencies** — stdlib `net/http`, `encoding/json`, `encoding/base64` only (mirroring `webhook.go`).

---

## Design References

- README §8.1 — frozen `adapter.Adapter` interface + behavioral contract (mirror `webhook.go`)
- README §8.2 — canonical payload fields available to adapters (`Summary`, `TitleSuggestion`, `SeverityGuess`, `TagsSuggested`, `Messages`)
- README §8.3 — `ZendeskConfig` sub-struct (frozen in 3-i: `Enabled`, `Subdomain`, `Email`, `APITokenEnv`, `DefaultPriority`)
- README §5 (tool pin list) — Phase 3 is stdlib-only; `go mod tidy` must add nothing
- Design spec §5.3 — zendesk endpoint/auth/mapping; §4.3 gate (paid adapters skipped without a permitting license — retrofit in 3-vi); §7 testing (unit `httptest` + basic-auth, live pauses)
- Reference impl: `relay/internal/adapter/webhook/webhook.go`, `relay/internal/adapter/webhook/webhook_test.go` (URL-driven test injection — NO exported client setter)
- Registry seam: `relay/cmd/relay/main.go` `buildRegistry` (3-i Task 5; the `// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.` comment)
- Secrets seam: `relay/internal/config/secret.go` `config.RequireSecret`

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/adapter/zendesk/zendesk.go` | Create | `Adapter`, `New`, `Name`, `RequiresLicense`, `Configure`, `Create`, `HealthCheck`, unexported `renderBody`/`mapPriority`/`truncate` |
| `relay/internal/adapter/zendesk/zendesk_test.go` | Create | happy path (method/path/auth-decode/body mapping), `mapPriority` table, non-2xx (422) w/o token, Configure validation, `RequiresLicense`, redaction |
| `relay/cmd/relay/main.go` | Modify | Add the zendesk branch to `buildRegistry` (ungated; `config.RequireSecret` for the token) + the `intake/internal/adapter/zendesk` import |

---

## Tasks

### Task 1: Create the zendesk adapter package (TDD)

**Files:** Create `relay/internal/adapter/zendesk/zendesk_test.go`, then `relay/internal/adapter/zendesk/zendesk.go`

- [ ] **Step 1: Write the failing test file**

Create `relay/internal/adapter/zendesk/zendesk_test.go`:

```go
package zendesk_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/adapter/zendesk"
	"intake/internal/payload"
)

const (
	testEmail = "agent@example.com"
	testToken = "super-secret-zendesk-token-abc123"
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
			Messages: []payload.Message{
				{Role: payload.MessageRoleUser, Content: "It crashes on save", Ts: now},
				{Role: payload.MessageRoleAssistant, Content: "Which version?", Ts: now},
			},
			Summary:         "User reports a crash when saving.",
			TitleSuggestion: "Crash on save",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{"crash", "save"},
			Language:        "en",
		},
	}
}

// configured returns a zendesk adapter pointed at the given test server URL.
func configured(t *testing.T, baseURL string) *zendesk.Adapter {
	t.Helper()
	a := zendesk.New()
	if err := a.Configure(map[string]any{
		"subdomain":        "example",
		"email":            testEmail,
		"api_token":        testToken,
		"default_priority": "normal",
		"base_url":         baseURL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestZendeskCreate_HappyPath asserts the POST target, method, basic-auth header,
// and the mapped ticket body, then verifies the parsed CreateResult.
func TestZendeskCreate_HappyPath(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ticket":{"id":555,"url":"https://example.zendesk.com/api/v2/tickets/555.json"}}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s; want POST", gotMethod)
	}
	if gotPath != "/api/v2/tickets.json" {
		t.Errorf("path = %s; want /api/v2/tickets.json", gotPath)
	}

	// Authorization must be "Basic <base64(email/token:token)>".
	const prefix = "Basic "
	if !strings.HasPrefix(gotAuth, prefix) {
		t.Fatalf("Authorization = %q; want %q prefix", gotAuth, prefix)
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(gotAuth, prefix))
	if err != nil {
		t.Fatalf("decode auth: %v", err)
	}
	wantCreds := testEmail + "/token:" + testToken
	if string(dec) != wantCreds {
		t.Errorf("decoded auth = %q; want %q", dec, wantCreds)
	}

	// Body shape: {"ticket":{"subject","comment":{"body"},"priority","tags"}}.
	var parsed struct {
		Ticket struct {
			Subject string `json:"subject"`
			Comment struct {
				Body string `json:"body"`
			} `json:"comment"`
			Priority string   `json:"priority"`
			Tags     []string `json:"tags"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("body not valid JSON: %v\nbody: %s", err, gotBody)
	}
	if parsed.Ticket.Subject != "Crash on save" {
		t.Errorf("subject = %q; want %q", parsed.Ticket.Subject, "Crash on save")
	}
	if !strings.Contains(parsed.Ticket.Comment.Body, "User reports a crash when saving.") {
		t.Errorf("comment.body missing summary; got %q", parsed.Ticket.Comment.Body)
	}
	if !strings.Contains(parsed.Ticket.Comment.Body, "user: It crashes on save") {
		t.Errorf("comment.body missing transcript line; got %q", parsed.Ticket.Comment.Body)
	}
	if parsed.Ticket.Priority != "high" { // high severity → "high"
		t.Errorf("priority = %q; want high", parsed.Ticket.Priority)
	}
	if len(parsed.Ticket.Tags) != 2 || parsed.Ticket.Tags[0] != "crash" || parsed.Ticket.Tags[1] != "save" {
		t.Errorf("tags = %v; want [crash save]", parsed.Ticket.Tags)
	}

	// CreateResult: ExternalID stringified id; ExternalURL is the agent UI url.
	if result.ExternalID != "555" {
		t.Errorf("ExternalID = %q; want 555", result.ExternalID)
	}
	if !strings.HasSuffix(result.ExternalURL, "/agent/tickets/555") {
		t.Errorf("ExternalURL = %q; want suffix /agent/tickets/555", result.ExternalURL)
	}
	if result.AdapterName != "zendesk" {
		t.Errorf("AdapterName = %q; want zendesk", result.AdapterName)
	}
	if result.CreatedAt == "" {
		t.Error("CreatedAt should be non-empty (RFC3339)")
	}
	if _, err := time.Parse(time.RFC3339, result.CreatedAt); err != nil {
		t.Errorf("CreatedAt %q not RFC3339: %v", result.CreatedAt, err)
	}
}

// TestZendeskMapPriority covers the severity → Zendesk priority mapping table.
func TestZendeskMapPriority(t *testing.T) {
	cases := []struct {
		name     string
		severity payload.ConversationSeverityGuess
		want     string
	}{
		{"low", payload.ConversationSeverityGuessLow, "low"},
		{"medium", payload.ConversationSeverityGuessMedium, "normal"},
		{"high", payload.ConversationSeverityGuessHigh, "high"},
		{"critical", payload.ConversationSeverityGuessCritical, "urgent"},
		{"unknown_uses_default", payload.ConversationSeverityGuessUnknown, "normal"},
		{"empty_uses_default", payload.ConversationSeverityGuess(""), "normal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPriority string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var parsed struct {
					Ticket struct {
						Priority string `json:"priority"`
					} `json:"ticket"`
				}
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &parsed)
				gotPriority = parsed.Ticket.Priority
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"ticket":{"id":1,"url":"x"}}`))
			}))
			defer srv.Close()

			a := configured(t, srv.URL) // default_priority "normal"
			p := minimalPayload()
			p.Conversation.SeverityGuess = tc.severity
			if _, err := a.Create(context.Background(), p); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if gotPriority != tc.want {
				t.Errorf("severity %q → priority %q; want %q", tc.severity, gotPriority, tc.want)
			}
		})
	}
}

// TestZendeskCreate_Non2xx asserts a 422 produces an error WITHOUT leaking the token.
func TestZendeskCreate_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"RecordInvalid","description":"Subject: cannot be blank"}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on 422, got nil")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("error should mention status 422; got %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("SECURITY: error leaks the api token: %v", err)
	}
}

// TestZendeskConfigure_MissingKeys asserts each required key is validated.
func TestZendeskConfigure_MissingKeys(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{
			"subdomain": "example",
			"email":     testEmail,
			"api_token": testToken,
		}
	}
	cases := []struct {
		name string
		drop string
	}{
		{"missing subdomain", "subdomain"},
		{"missing email", "email"},
		{"missing api_token", "api_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			delete(cfg, tc.drop)
			a := zendesk.New()
			err := a.Configure(cfg)
			if err == nil {
				t.Fatalf("expected error when %q is missing", tc.drop)
			}
			if !strings.Contains(err.Error(), tc.drop) {
				t.Errorf("error should name the missing key %q; got %v", tc.drop, err)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Fatalf("SECURITY: Configure error leaks the api token: %v", err)
			}
		})
	}
}

// TestZendeskRequiresLicense asserts zendesk is a paid adapter.
func TestZendeskRequiresLicense(t *testing.T) {
	if !zendesk.New().RequiresLicense() {
		t.Error("zendesk.RequiresLicense() = false; want true (paid adapter)")
	}
}

// TestZendeskCreate_TokenNeverLeaks drives Create against a server that always
// 500s and asserts the token never appears in the returned error string.
func TestZendeskCreate_TokenNeverLeaks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo the auth header back in the body — the adapter must NOT surface it.
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error: " + r.Header.Get("Authorization")))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("SECURITY: error leaks the api token: %v", err)
	}
	// The base64-encoded credentials must not leak either.
	encoded := base64.StdEncoding.EncodeToString([]byte(testEmail + "/token:" + testToken))
	if strings.Contains(err.Error(), encoded) {
		t.Fatalf("SECURITY: error leaks the base64 basic-auth credentials: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/zendesk/... -v
```

Expected: `no required module provides package intake/internal/adapter/zendesk` (or a build error: `undefined: zendesk`). MUST fail before proceeding.

- [ ] **Step 3: Create `zendesk.go`**

Create `relay/internal/adapter/zendesk/zendesk.go`:

```go
// Package zendesk implements the adapter.Adapter interface for Zendesk Support,
// creating a ticket via the REST API. It is a PAID adapter (RequiresLicense → true);
// the license gate that decides whether it is registered lives in main.go's
// buildRegistry (added in 3-vi). All HTTP is hand-rolled over net/http + encoding/json
// + encoding/base64 (no SDK), mirroring the webhook reference adapter.
//
// SECURITY: the basic-auth header embeds the API token (base64). The token and the
// Authorization header are NEVER logged and NEVER included in any returned error.
package zendesk

import (
	"bytes"
	"context"
	"encoding/base64"
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

const defaultPriorityFallback = "normal"

// Adapter creates Zendesk tickets. Construct via New, then Configure before use.
type Adapter struct {
	baseURL         string // e.g. https://example.zendesk.com (no trailing slash)
	email           string
	apiToken        string
	defaultPriority string
	authHeader      string // "Basic <base64(email/token:token)>" — never logged
	client          *http.Client
}

// Compile-time assertion that Adapter satisfies the frozen interface.
var _ adapter.Adapter = (*Adapter)(nil)

// New returns an unconfigured Adapter with a default 15s HTTP client.
func New() *Adapter {
	return &Adapter{
		defaultPriority: defaultPriorityFallback,
		client:          &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Adapter) Name() string { return "zendesk" }

func (a *Adapter) RequiresLicense() bool { return true }

// Configure reads subdomain, email, api_token (required), default_priority (optional,
// defaults to "normal"), and an optional base_url override (a test seam; otherwise
// baseURL is derived as https://<subdomain>.zendesk.com). The api_token is the RESOLVED
// secret value passed in by main.go (never read from the env here, never logged).
func (a *Adapter) Configure(cfg map[string]any) error {
	subdomain, err := requiredString(cfg, "subdomain")
	if err != nil {
		return err
	}
	email, err := requiredString(cfg, "email")
	if err != nil {
		return err
	}
	apiToken, err := requiredString(cfg, "api_token")
	if err != nil {
		return err
	}

	// Optional base_url override (test seam); otherwise derive from subdomain.
	baseURL := fmt.Sprintf("https://%s.zendesk.com", subdomain)
	if bv, ok := cfg["base_url"]; ok {
		if bs, ok := bv.(string); ok && bs != "" {
			baseURL = bs
		}
	}
	a.baseURL = strings.TrimRight(baseURL, "/")

	// Optional default_priority.
	if pv, ok := cfg["default_priority"]; ok {
		if ps, ok := pv.(string); ok && ps != "" {
			a.defaultPriority = ps
		}
	}

	a.email = email
	a.apiToken = apiToken
	creds := email + "/token:" + apiToken
	a.authHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
	return nil
}

// requiredString returns a non-empty string config value or a clear error naming
// the key. The error NEVER includes the value (so a missing/blank token cannot leak).
func requiredString(cfg map[string]any, key string) (string, error) {
	v, ok := cfg[key]
	if !ok {
		return "", fmt.Errorf("zendesk: missing required config key %q", key)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("zendesk: config key %q must be a non-empty string", key)
	}
	return s, nil
}

// ticketRequest is the POST body: {"ticket":{...}}.
type ticketRequest struct {
	Ticket ticketBody `json:"ticket"`
}

type ticketBody struct {
	Subject  string         `json:"subject"`
	Comment  ticketComment  `json:"comment"`
	Priority string         `json:"priority"`
	Tags     []string       `json:"tags"`
}

type ticketComment struct {
	Body string `json:"body"`
}

// ticketResponse parses the 2xx body: {"ticket":{"id":<num>,"url":"<api url>"}}.
type ticketResponse struct {
	Ticket struct {
		ID  json.Number `json:"id"`
		URL string      `json:"url"`
	} `json:"ticket"`
}

// Create POSTs a ticket to {baseURL}/api/v2/tickets.json. On 2xx it parses the ticket
// id and returns a CreateResult whose ExternalURL points at the agent UI (the API
// `url` field is an api endpoint, not the user-facing ticket page). On non-2xx it
// returns an error including the truncated body but NEVER the token or auth header.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	reqBody := ticketRequest{
		Ticket: ticketBody{
			Subject:  p.Conversation.TitleSuggestion,
			Comment:  ticketComment{Body: renderBody(p)},
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
		// NEVER include the auth header or token — only the status + truncated body.
		return nil, fmt.Errorf("zendesk: create ticket returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var parsed ticketResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("zendesk: parse response: %w", err)
	}
	id := parsed.Ticket.ID.String()
	if id == "" {
		return nil, fmt.Errorf("zendesk: response missing ticket id: %s", truncate(string(respBody), 200))
	}

	return &adapter.CreateResult{
		ExternalID:  id,
		ExternalURL: fmt.Sprintf("%s/agent/tickets/%s", a.baseURL, id),
		AdapterName: a.Name(),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// renderBody concatenates the summary, a blank line, then each message as
// "<Role>: <Content>" (one per line). Matches the README §8.2 recommended shape.
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
	return strings.TrimRight(b.String(), "\n")
}

// mapPriority maps the canonical severity to a Zendesk priority. unknown/"" falls
// back to the configured default_priority.
func mapPriority(sev payload.ConversationSeverityGuess, def string) string {
	switch sev {
	case payload.ConversationSeverityGuessLow:
		return "low"
	case payload.ConversationSeverityGuessMedium:
		return "normal"
	case payload.ConversationSeverityGuessHigh:
		return "high"
	case payload.ConversationSeverityGuessCritical:
		return "urgent"
	default: // "unknown", "" or any unexpected value
		return def
	}
}

// HealthCheck GETs {baseURL}/api/v2/tickets/count.json with the auth header. A non-5xx
// response (including 401/403/404) is considered reachable; only a 5xx or transport
// error is a failure. Mirrors the webhook adapter's error shape.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.baseURL == "" {
		return fmt.Errorf("zendesk: not configured (base url is empty)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/api/v2/tickets/count.json", nil)
	if err != nil {
		return fmt.Errorf("zendesk health: build request: %w", err)
	}
	req.Header.Set("Authorization", a.authHeader)
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("zendesk health: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("zendesk health: upstream returned %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// strconv is imported for potential id formatting; keep the import live.
var _ = strconv.Itoa
```

> Implementation note: `json.Number` stringifies the ticket id without float rounding (a large id stays exact). The unused `strconv` guard above can be removed by the implementer — it is only present so the import list compiles if they prefer `strconv` over `json.Number`; if `json.Number` is used (as written), DELETE both the `"strconv"` import and the `var _ = strconv.Itoa` line and run `go vet`.

- [ ] **Step 4: Run the zendesk tests (all should pass)**

```
cd C:/src/ai/intake/relay && go test ./internal/adapter/zendesk/... -v
```

Expected: every test PASSES — `TestZendeskCreate_HappyPath`, `TestZendeskMapPriority` (all 6 sub-cases), `TestZendeskCreate_Non2xx`, `TestZendeskConfigure_MissingKeys` (all 3 sub-cases), `TestZendeskRequiresLicense`, `TestZendeskCreate_TokenNeverLeaks`.

- [ ] **Step 5: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. (If you kept `json.Number` and removed the `strconv` guard, confirm there is no `imported and not used` error.)

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/relay && git add internal/adapter/zendesk/zendesk.go internal/adapter/zendesk/zendesk_test.go
git commit -m "feat(zendesk): paid adapter — ticket create over Zendesk REST + basic auth (3-iv)"
```

---

### Task 2: Register zendesk in main.go's buildRegistry (UNGATED)

**Files:** Modify `relay/cmd/relay/main.go`

> The zendesk branch is registered **ungated** in this sub-plan — exactly like the webhook/chatwoot/fider branches. **Sub-plan 3-vi retrofits the license gate** that wraps the two paid adapters (zendesk, linear), skipping them when no permitting license is present (design §4.3). Do NOT add any gate logic here; add only the enabled-check + token resolution + Configure + registry entry below.

- [ ] **Step 1: Confirm the current registry build**

```
cd C:/src/ai/intake/relay && go build ./cmd/relay/... && echo PRE_OK
```

Expected: `PRE_OK`. (3-i's `buildRegistry` with the `// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.` seam comment must exist; if it does not, 3-i has not landed — stop and land 3-i first.)

- [ ] **Step 2: Add the zendesk branch to `buildRegistry`**

In `relay/cmd/relay/main.go`, add the import `"intake/internal/adapter/zendesk"` alongside the other adapter imports. Then, inside `buildRegistry`, immediately after the seam comment (`// 3-ii chatwoot, 3-iii fider, 3-iv zendesk, 3-v linear are added here.`) — or after the chatwoot/fider branches if 3-ii/3-iii already inserted theirs — add:

```go
	// zendesk (3-iv) — PAID. Registered ungated here; 3-vi wraps with the license gate.
	if cfg.Adapters.Zendesk.Enabled {
		token, err := config.RequireSecret(cfg.Adapters.Zendesk.APITokenEnv)
		if err != nil {
			return nil, fmt.Errorf("zendesk adapter: %w", err)
		}
		zd := zendesk.New()
		if err := zd.Configure(map[string]any{
			"subdomain":        cfg.Adapters.Zendesk.Subdomain,
			"email":            cfg.Adapters.Zendesk.Email,
			"api_token":        token,
			"default_priority": cfg.Adapters.Zendesk.DefaultPriority,
		}); err != nil {
			return nil, fmt.Errorf("zendesk adapter: %w", err)
		}
		reg[zd.Name()] = zd
		logger.Info("relay: adapter enabled", "adapter", zd.Name())
	}
```

> The resolved `token` is passed INTO `Configure` and is never logged. `config.RequireSecret` returns a clear error (env name only, never the value) if `ZENDESK_API_TOKEN` / `ZENDESK_API_TOKEN_FILE` is unset.

- [ ] **Step 3: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. If `zendesk` is reported `imported and not used`, the branch was not inserted into `buildRegistry`.

- [ ] **Step 4: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "feat(zendesk): register ungated in buildRegistry; token via RequireSecret (3-iv)"
```

---

### Task 3: Final verification gate

- [ ] **Step 1: Full build + vet + test**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected: `BUILD_OK`, `VET_OK`, every package `ok` (including `internal/adapter/zendesk`), `TEST_OK`.

- [ ] **Step 2: Contract gate + pin gate**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK`.

- [ ] **Step 3: Confirm no new external dependency (stdlib-only)**

```
cd C:/src/ai/intake/relay && go mod tidy && git diff --exit-code go.mod go.sum && echo MOD_CLEAN
```

Expected: `MOD_CLEAN` (no changes). If `go.mod`/`go.sum` changed, a non-stdlib import sneaked in — the adapter must use only `net/http`, `encoding/json`, `encoding/base64` (+ existing stdlib). Investigate and remove.

- [ ] **Step 4: Build-fail self-check (README §6)**

- `var _ adapter.Adapter = (*Adapter)(nil)` compiles → zendesk satisfies the frozen interface. ✓
- `RequiresLicense()` → true → `TestZendeskRequiresLicense`. ✓
- token never in an error/log → `TestZendeskCreate_Non2xx` + `TestZendeskCreate_TokenNeverLeaks` (asserts neither the raw token nor the base64 credentials leak). ✓
- non-2xx is an error including the truncated body but never the token → `TestZendeskCreate_Non2xx`. ✓
- no new external dependency → step 3. ✓
- zendesk registered **ungated** (gate is 3-vi) → confirmed by the explicit seam comment in `buildRegistry`. ✓

---

## Smoke

**Credit-free (unit):** `go test ./internal/adapter/zendesk/... -v` is fully green against an `httptest` mock — proves the POST target/method, the base64-decoded basic-auth credentials (`{email}/token:{token}`), the mapped ticket body (subject/comment.body/priority/tags), the `mapPriority` table, non-2xx error handling, Configure validation, `RequiresLicense()`, and that the token never leaks into an error. No network, no credits.

**Live (deferred to the phase final smoke, README §7 — PAUSES for the maintainer):** Requires a real Zendesk subdomain, a real agent `email`, a real `ZENDESK_API_TOKEN` (set via env or `ZENDESK_API_TOKEN_FILE`), **and** a maintainer-signed license whose `adapters` include `zendesk` (the gate from 3-vi must permit it — without it, zendesk is correctly absent from the registry in free mode). With those: set `adapters.zendesk.enabled: true` + `subdomain`/`email`/`api_token_env`/`default_priority`, boot the relay, drive a 2-turn conversation → Submit → a ticket appears in the Zendesk inbox with the mapped subject/body/priority/tags, and `ExternalURL` opens the agent ticket page. This is maintainer/paid/external and runs only at the Phase 3 final smoke with explicit go-ahead (per the credit/secret guard).

## Done Criteria

1. `go build ./... && go vet ./...` clean in `relay/`.
2. `go test ./...` green, including the new `internal/adapter/zendesk` tests, with NO real Zendesk token (all `httptest`).
3. `relay/internal/adapter/zendesk/zendesk.go` implements the **frozen** `adapter.Adapter` interface (`var _ adapter.Adapter = (*Adapter)(nil)`), `Name()` → `"zendesk"`, `RequiresLicense()` → **true**.
4. `Create` POSTs `{baseURL}/api/v2/tickets.json` with the base64 basic-auth header `{email}/token:{api_token}`, the mapped ticket body, parses the ticket id into `ExternalID`, and sets `ExternalURL` to the agent UI page; non-2xx errors include the truncated body but NEVER the token or Authorization header.
5. The token is passed INTO `Configure` (resolved by `main.go` via `config.RequireSecret`); the adapter never reads the env itself and never logs the token (proved by the redaction tests).
6. `buildRegistry` in `main.go` has an **ungated** zendesk branch with the explicit `3-vi wraps with the license gate` comment; the `intake/internal/adapter/zendesk` import is added.
7. `bash scripts/verify-contract.sh` and `bash scripts/check-pins.sh` green; `go mod tidy` leaves `go.mod`/`go.sum` unchanged (stdlib-only — `net/http`, `encoding/json`, `encoding/base64`).
8. Attachments remain out of scope (Phase 6); the adapter creates a text ticket only.
