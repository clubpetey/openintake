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
	if a.accountID <= 0 {
		return fmt.Errorf("chatwoot: config key 'account_id' must be a positive integer")
	}

	if v, ok := cfg["inbox_id"]; ok {
		switch n := v.(type) {
		case int:
			a.inboxID = n
		case float64:
			a.inboxID = int(n)
		}
	}
	if a.inboxID <= 0 {
		return fmt.Errorf("chatwoot: config key 'inbox_id' must be a positive integer")
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
	// json.Number preserves the numeric literal exactly; no UseNumber needed.
	if err := json.Unmarshal(body, &raw); err != nil {
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
	url := fmt.Sprintf("%s/api/v1/accounts/%d", a.baseURL, a.accountID)
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
