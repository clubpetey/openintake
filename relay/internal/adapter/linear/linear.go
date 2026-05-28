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
		// Redact the api key from GraphQL error messages before surfacing them.
		// GraphQL errors can echo request context; we must never let the key appear.
		msg := truncate(joinErrors(parsed.Errors), 200)
		msg = a.redact(msg)
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
		msg := truncate(joinErrors(parsed.Errors), 200)
		msg = a.redact(msg)
		return fmt.Errorf("linear health: graphql errors: %s", msg)
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

// redact replaces any occurrence of the api key in s with "[REDACTED]".
// This guards against servers that echo request context in error messages.
func (a *Adapter) redact(s string) string {
	if a.apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, a.apiKey, "[REDACTED]")
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
