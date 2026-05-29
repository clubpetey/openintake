// Package linear implements the Linear adapter (paid). It creates an issue via
// Linear's GraphQL API (issueCreate mutation) over the standard library — no SDK.
// Auth is a Linear personal API key sent RAW in the Authorization header (no
// "Bearer " prefix). The key is passed into Configure by main.go (resolved via
// config.RequireSecret) and is never read from the env here and never logged.
//
// The team_id config key accepts either a UUID
// (e.g. "9ddb7234-31d1-4dd3-b9b0-32ad948b6104") or a short team key
// (e.g. "REF"). When a key is supplied, Configure resolves it to a UUID via a
// single GraphQL teams query at startup; subsequent issueCreate calls use the
// resolved UUID. Supply a UUID directly to skip the startup network call.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"regexp"
	"sort"
	"strings"
	"time"

	"intake/internal/adapter"
	"intake/internal/attachvalidate"
	"intake/internal/payload"
)

const defaultEndpoint = "https://api.linear.app/graphql"

// defaultUploadEndpoint is Linear's legacy file-upload endpoint (separate host
// from the GraphQL endpoint). Overridable via the `upload_endpoint` config key
// for test injection (see Configure).
const defaultUploadEndpoint = "https://uploads.linear.app/api/file/upload"

// uuidRE matches the canonical 8-4-4-4-12 UUID form, case-insensitive.
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// issueCreateMutation is the GraphQL mutation creating one issue. Constant so the
// request body is fully deterministic (no string building from untrusted input).
const issueCreateMutation = `mutation IssueCreate($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { id identifier url } } }`

// teamsQuery fetches all teams so a short key can be resolved to a UUID once at startup.
const teamsQuery = `{ teams(first: 250) { nodes { id name key } } }`

// Adapter creates Linear issues via GraphQL.
type Adapter struct {
	apiKey         string
	teamID         string
	endpoint       string
	uploadEndpoint string // 6-ii: Linear's legacy file-upload endpoint (separate host).
	client         *http.Client
}

// New returns an unconfigured Adapter. Call Configure before use.
func New() *Adapter {
	return &Adapter{
		endpoint:       defaultEndpoint,
		uploadEndpoint: defaultUploadEndpoint,
		client:         &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Adapter) Name() string { return "linear" }

// RequiresLicense reports that linear is a paid adapter.
func (a *Adapter) RequiresLicense() bool { return true }

// Capabilities advertises the accepted attachment MIME types for /init
// capability discovery (Phase 6, 6-i).
func (a *Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
}

// Configure reads api_key (required), team_id (required), and an optional endpoint
// override (the test-injection seam; defaults to the live GraphQL endpoint). The
// api_key value is the RESOLVED secret passed in by main.go — never the env name.
//
// team_id may be a UUID (stored verbatim, no network call) or a short team key
// (resolved to a UUID via a single GraphQL teams query; see resolveTeamKey).
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

	if ep, ok := cfg["endpoint"].(string); ok && ep != "" {
		a.endpoint = ep
	}
	// 6-ii: optional upload_endpoint override mirrors the GraphQL endpoint
	// seam for test injection. Production deployments use defaultUploadEndpoint.
	if up, ok := cfg["upload_endpoint"].(string); ok && up != "" {
		a.uploadEndpoint = up
	}

	if uuidRE.MatchString(team) {
		// Already a UUID — store directly, no network call needed.
		a.teamID = team
		return nil
	}

	// Treat team as a short key; resolve it to a UUID at startup.
	resolved, err := a.resolveTeamKey(team)
	if err != nil {
		return err
	}
	a.teamID = resolved
	return nil
}

// teamsResponse is the parsed shape of the teams query result.
type teamsResponse struct {
	Data struct {
		Teams struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Key  string `json:"key"`
			} `json:"nodes"`
		} `json:"teams"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// resolveTeamKey POSTs the teams query, finds the node whose key matches the
// supplied team key (exact case), and returns its UUID. A bounded background
// context (10 s) is used because Configure has no context parameter.
func (a *Adapter) resolveTeamKey(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	body, err := json.Marshal(graphQLRequest{Query: teamsQuery})
	if err != nil {
		return "", fmt.Errorf("linear: resolve team key %q: marshal request: %w", key, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("linear: resolve team key %q: build request: %w", key, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		// Redact defensively; http.Client.Do errors typically don't contain auth
		// headers but apply the helper uniformly. Wrap with %w to preserve the
		// underlying error for callers that inspect error chains.
		return "", fmt.Errorf("linear: resolve team key %q: %s", key, a.redact(err.Error()))
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Redact BEFORE truncate per L011.
		snippet := adapter.Truncate(a.redact(string(respBody)), 200)
		return "", fmt.Errorf("linear: resolve team key %q: graphql endpoint returned %d: %s", key, resp.StatusCode, snippet)
	}

	var parsed teamsResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("linear: resolve team key %q: decode response: %w", key, err)
	}

	if len(parsed.Errors) > 0 {
		// Redact BEFORE truncate per L011.
		msg := adapter.Truncate(a.redact(joinErrors(parsed.Errors)), 200)
		return "", fmt.Errorf("linear: resolve team key %q: graphql errors: %s", key, msg)
	}

	// Search for the matching team key (exact match; Linear keys are typically uppercase).
	var available []string
	for _, node := range parsed.Data.Teams.Nodes {
		if node.Key == key {
			return node.ID, nil
		}
		available = append(available, node.Key)
	}

	// Build a helpful "available keys" list, sorted, capped at 10 with "+N more".
	sort.Strings(available)
	if len(available) == 0 {
		return "", fmt.Errorf("linear: no team found with key %q; set team_id to a UUID or a valid team key", key)
	}
	var keyList string
	if len(available) <= 10 {
		keyList = strings.Join(available, ", ")
	} else {
		keyList = strings.Join(available[:10], ", ") + fmt.Sprintf(" +%d more", len(available)-10)
	}
	return "", fmt.Errorf("linear: no team found with key %q (available keys: %s); set team_id to a UUID or a valid team key", key, keyList)
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

// Create runs the Phase 6 (6-ii) sequence: upload each attachment to Linear's
// file-upload endpoint, then POST the issueCreate mutation referencing the
// returned asset URLs in attachmentLinks. Any upload failure returns an error
// BEFORE issueCreate is called (L011 orphan prevention). The api key is never
// included in any error message — redaction runs BEFORE truncate so a
// long-prefix echo cannot slip the key past the 200-rune cap.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	// Phase 6: upload every attachment first; returns a slice of {url, title}
	// values ready for the issueCreate input. Empty p.Attachments returns nil
	// and the issueCreate input omits attachmentLinks entirely.
	links, err := a.uploadAttachments(ctx, p.Attachments)
	if err != nil {
		return nil, err
	}

	input := map[string]any{
		"teamId":      a.teamID,
		"title":       p.Conversation.TitleSuggestion,
		"description": renderBody(p),
	}
	if len(links) > 0 {
		input["attachmentLinks"] = links
	}

	reqBody := graphQLRequest{
		Query:     issueCreateMutation,
		Variables: map[string]any{"input": input},
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
		// Non-2xx: redact first so a key in the body can't survive, then truncate.
		return nil, fmt.Errorf("linear: graphql endpoint returned %d: %s", resp.StatusCode, adapter.Truncate(a.redact(string(respBody)), 200))
	}

	var parsed issueCreateResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("linear: decode response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		// Redact BEFORE truncate: truncation could split the key and defeat redaction.
		msg := adapter.Truncate(a.redact(joinErrors(parsed.Errors)), 200)
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
	if externalID == "" {
		return nil, fmt.Errorf("linear: issueCreate returned an issue with no id or identifier")
	}
	return &adapter.CreateResult{
		ExternalID:  externalID,
		ExternalURL: ic.Issue.URL,
		AdapterName: a.Name(),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// uploadResponse is the legacy upload endpoint's response shape.
type uploadResponse struct {
	Success    bool `json:"success"`
	UploadFile struct {
		URL string `json:"url"`
	} `json:"uploadFile"`
}

// uploadAttachments POSTs each attachment as multipart/form-data to Linear's
// legacy file-upload endpoint and returns one {url,title} map per upload for
// the issueCreate input.attachmentLinks. Failure short-circuits and returns
// an error BEFORE issueCreate (L011 orphan prevention).
//
// Per-upload error wrapping redacts the api_key BEFORE Truncate per L011 so a
// long-prefix server echo cannot survive truncation.
func (a *Adapter) uploadAttachments(ctx context.Context, atts []payload.Attachment) ([]map[string]any, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	links := make([]map[string]any, 0, len(atts))
	for i, att := range atts {
		raw, _, err := attachvalidate.DecodeOne(att)
		if err != nil {
			return nil, fmt.Errorf("linear: decode attachment %d/%d: %w", i+1, len(atts), err)
		}
		title := ""
		if att.Label != nil {
			title = *att.Label
		}
		if title == "" {
			title = fmt.Sprintf("screenshot %d", i+1)
		}

		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		hdr := textproto.MIMEHeader{
			"Content-Disposition": []string{fmt.Sprintf(`form-data; name="file"; filename=%q`, title)},
			"Content-Type":        []string{att.MimeType},
		}
		part, err := mw.CreatePart(hdr)
		if err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d build part: %w", i+1, len(atts), err)
		}
		if _, err := part.Write(raw); err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d write bytes: %w", i+1, len(atts), err)
		}
		if err := mw.Close(); err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d close multipart: %w", i+1, len(atts), err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.uploadEndpoint, &buf)
		if err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d build request: %w", i+1, len(atts), err)
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", a.apiKey)

		resp, err := a.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d: %s", i+1, len(atts), a.redact(err.Error()))
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// L011: redact BEFORE truncate.
			snippet := adapter.Truncate(a.redact(string(body)), 200)
			return nil, fmt.Errorf("linear: upload %d/%d returned %d: %s", i+1, len(atts), resp.StatusCode, snippet)
		}

		var parsed uploadResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("linear: upload %d/%d decode response: %w", i+1, len(atts), err)
		}
		if !parsed.Success {
			return nil, fmt.Errorf("linear: upload %d/%d response success=false", i+1, len(atts))
		}
		if parsed.UploadFile.URL == "" {
			return nil, fmt.Errorf("linear: upload %d/%d response missing uploadFile.url", i+1, len(atts))
		}
		links = append(links, map[string]any{
			"url":   parsed.UploadFile.URL,
			"title": title,
		})
	}
	return links, nil
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
		// Redact BEFORE truncate: truncation could split the key and defeat redaction.
		msg := adapter.Truncate(a.redact(joinErrors(parsed.Errors)), 200)
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

// Compile-time assertion that *Adapter satisfies the frozen interface.
var _ adapter.Adapter = (*Adapter)(nil)
