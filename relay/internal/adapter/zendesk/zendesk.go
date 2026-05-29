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
	"net/url"
	"strings"
	"time"

	"intake/internal/adapter"
	"intake/internal/attachvalidate"
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

// Capabilities advertises the accepted attachment MIME types for /init
// capability discovery (Phase 6, 6-i).
func (a *Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
}

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
	Subject  string        `json:"subject"`
	Comment  ticketComment `json:"comment"`
	Priority string        `json:"priority"`
	Tags     []string      `json:"tags"`
}

type ticketComment struct {
	Body    string   `json:"body"`
	Uploads []string `json:"uploads,omitempty"`
}

// ticketResponse parses the 2xx body: {"ticket":{"id":<num>,"url":"<api url>"}}.
type ticketResponse struct {
	Ticket struct {
		ID  json.Number `json:"id"`
		URL string      `json:"url"`
	} `json:"ticket"`
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
			// http.Client.Do transport errors do not contain the Authorization
			// header (it lives on req, not err). Wrap with %w to match the
			// ticket-POST path's wrapping behavior for consistency.
			return "", fmt.Errorf("zendesk: upload %d/%d: %w", i+1, len(atts), err)
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
