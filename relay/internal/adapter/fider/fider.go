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

// Capabilities advertises the accepted attachment MIME types for /init
// capability discovery (Phase 6, 6-i).
func (a *Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
}

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
		title = adapter.Truncate(p.Conversation.Summary, maxTitleLen)
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
		return nil, fmt.Errorf("fider: create post returned %d: %s", resp.StatusCode, adapter.Truncate(string(respBody), 200))
	}

	var parsed createResponse
	// A non-JSON or empty 2xx body is tolerated: id/number stay 0, yielding a
	// CreateResult with empty ExternalID/ExternalURL. The relay stores it as-is;
	// there is no upstream assertion that ExternalID is non-empty.
	_ = json.Unmarshal(respBody, &parsed)

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

// renderBody builds the post description: the summary, a blank line, then each
// message as "<Role>: <Content>". Unlike chatwoot, the title is a separate Fider
// API field (the `title` POST field) and is deliberately NOT repeated here.
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
