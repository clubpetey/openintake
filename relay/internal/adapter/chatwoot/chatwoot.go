// Package chatwoot is a downstream adapter that creates a Chatwoot conversation
// in a configured inbox from the canonical intake payload using a confirmed
// two-or-three-call flow (design spec §5.1, live smoke 2026-05-27; attachments
// path corrected after live smoke 2026-05-29 — see L020):
//
//  1. POST /api/v1/accounts/{id}/contacts — creates a contact tied to the inbox
//     and obtains a contact_inbox.source_id and contact.id from the response.
//  2. POST /api/v1/accounts/{id}/conversations — creates the conversation using
//     the source_id and contact_id returned by step 1. This call is ALWAYS
//     application/json. Chatwoot's ConversationsController#create silently
//     drops file parts when given multipart with attachments[] (it persists
//     only inbox_id/source_id/contact_id/message[content]), so attachments are
//     handled in a separate step.
//  3. POST /api/v1/accounts/{id}/conversations/{conv_id}/messages — ONLY when
//     p.Attachments is non-empty. Multipart/form-data carrying content,
//     message_type=outgoing, and one attachments[] part per attachment.
//
// Chatwoot Cloud's agent-side conversation endpoint returns 404 when source_id
// does not map to an existing contact_inbox association; step 1 establishes that
// association before the conversation is created.
//
// Implementation follows the webhook reference adapter conventions: stdlib
// net/http + encoding/json, a test-injectable *http.Client, and a token that is
// passed into Configure (resolved by main.go) and never read from the env or
// logged.
package chatwoot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"intake/internal/adapter"
	"intake/internal/attachvalidate"
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

// Capabilities advertises the accepted attachment MIME types for /init
// capability discovery (Phase 6, 6-i).
func (a *Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
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

// contactRequest is the Chatwoot contact-create body (subset).
// Email is omitted when empty to avoid Chatwoot uniqueness constraint errors on
// repeated empty-string submissions; use a pointer so omitempty works correctly.
type contactRequest struct {
	InboxID    int    `json:"inbox_id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Email      string `json:"email,omitempty"`
}

// contactCreateResponse mirrors the relevant subset of the Chatwoot contact
// create response: payload.contact.id and payload.contact_inbox.source_id.
type contactCreateResponse struct {
	Payload struct {
		Contact struct {
			ID json.Number `json:"id"`
		} `json:"contact"`
		ContactInbox struct {
			SourceID string `json:"source_id"`
		} `json:"contact_inbox"`
	} `json:"payload"`
}

// conversationRequest is the Chatwoot conversation-create body (subset).
type conversationRequest struct {
	InboxID   int             `json:"inbox_id"`
	SourceID  string          `json:"source_id"`
	ContactID json.Number     `json:"contact_id"`
	Message   conversationMsg `json:"message"`
}

type conversationMsg struct {
	Content string `json:"content"`
}

// contactName returns the best-effort display name for the contact. It prefers
// DisplayName, falls back to Email, and finally synthesises a readable label
// from the short submission ID.
func contactName(p *payload.IntakePayload) string {
	if p.User.DisplayName != nil && *p.User.DisplayName != "" {
		return *p.User.DisplayName
	}
	if p.User.Email != nil && *p.User.Email != "" {
		return *p.User.Email
	}
	return "Intake user (submission " + shortID(p.Submission.Id) + ")"
}

// shortID returns the first 8 chars of a UUID for log/display readability.
func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

// createContact POSTs to /contacts and returns the contact id and the
// contact_inbox source_id that must be passed to the conversation create call.
func (a *Adapter) createContact(ctx context.Context, p *payload.IntakePayload) (contactID json.Number, sourceID string, err error) {
	reqData := contactRequest{
		InboxID:    a.inboxID,
		Name:       contactName(p),
		Identifier: p.Submission.Id,
	}
	// Only set Email when present; empty string would trigger Chatwoot uniqueness
	// constraints across submissions.
	if p.User.Email != nil && *p.User.Email != "" {
		reqData.Email = *p.User.Email
	}

	body, err := json.Marshal(reqData)
	if err != nil {
		return "", "", fmt.Errorf("chatwoot: marshal contact request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/contacts", a.baseURL, a.accountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("chatwoot: build contact request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", a.apiToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("chatwoot: contact http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("chatwoot: create contact returned %d: %s",
			resp.StatusCode, adapter.Truncate(string(respBody), 200))
	}

	var parsed contactCreateResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", fmt.Errorf("chatwoot: decode contact response: %w (body: %s)",
			err, adapter.Truncate(string(respBody), 200))
	}

	cid := parsed.Payload.Contact.ID
	sid := parsed.Payload.ContactInbox.SourceID
	if cid.String() == "" {
		return "", "", fmt.Errorf("chatwoot: contact response missing payload.contact.id (body: %s)",
			adapter.Truncate(string(respBody), 200))
	}
	if sid == "" {
		return "", "", fmt.Errorf("chatwoot: contact response missing payload.contact_inbox.source_id (body: %s)",
			adapter.Truncate(string(respBody), 200))
	}
	return cid, sid, nil
}

// Create executes the two-or-three-call flow: first creates a contact tied to
// the inbox to obtain a valid source_id, then creates the conversation using
// that source_id and the contact id. When p.Attachments is non-empty, a third
// call POSTs a multipart message to the conversation's /messages endpoint
// carrying the decoded raw bytes.
//
// Phase 6 (6-ii, corrected after live smoke 2026-05-29; see L020):
// conversation-create is ALWAYS application/json — byte-identical to Phase 3.
// Chatwoot's ConversationsController#create silently drops attachments[] file
// parts when given a multipart body, so attachments must be uploaded via a
// SEPARATE POST /messages call. When that upload fails the conversation
// already exists in chatwoot with text but no image; we return the upload
// error from Create() (mapped to 502 adapter_error by submitHandler) rather
// than silently succeeding.
//
// Non-2xx at any step returns an error including the truncated response body
// but never the token.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	// Step 1: create contact and obtain contact_inbox source_id. Unchanged.
	contactID, sourceID, err := a.createContact(ctx, p)
	if err != nil {
		return nil, err
	}

	// Step 2: create conversation. Always application/json (Phase 3
	// byte-identical). See package doc + L020 for why attachments do not
	// belong here.
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations", a.baseURL, a.accountID)

	reqBody := conversationRequest{
		InboxID:   a.inboxID,
		SourceID:  sourceID,
		ContactID: contactID,
		Message:   conversationMsg{Content: renderBody(p)},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: marshal conversation request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("chatwoot: build conversation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", a.apiToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: conversation http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("chatwoot: create conversation returned %d: %s",
			resp.StatusCode, adapter.Truncate(string(respBody), 200))
	}

	id, err := extractConversationID(respBody)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: parse conversation response id: %w", err)
	}

	// Step 3 (only when attachments are present): upload via /messages.
	// If this fails, the conversation already exists in chatwoot with the
	// transcript but no image; we return the upload error unwrapped so the
	// operator sees what actually failed. Orphan prevention is accepted as a
	// non-goal for v0 — see L020.
	if len(p.Attachments) > 0 {
		if err := a.uploadAttachments(ctx, id, p); err != nil {
			return nil, err
		}
	}

	return &adapter.CreateResult{
		ExternalID:  id,
		ExternalURL: fmt.Sprintf("%s/app/accounts/%d/conversations/%s", a.baseURL, a.accountID, id),
		AdapterName: "chatwoot",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// uploadAttachments POSTs a multipart/form-data message to the conversation's
// /messages endpoint, carrying one attachments[] part per p.Attachments entry.
// Returns nil on 2xx; on non-2xx returns an error with the truncated body. The
// api_access_token is sent as a header and never appears in the body or error.
func (a *Adapter) uploadAttachments(ctx context.Context, conversationID string, p *payload.IntakePayload) error {
	body, ctype, err := buildMessageMultipart(p)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%s/messages",
		a.baseURL, a.accountID, conversationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("chatwoot: build message request: %w", err)
	}
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("api_access_token", a.apiToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("chatwoot: message http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("chatwoot: upload attachments to conversation %s returned %d (conversation exists with transcript; image upload failed): %s",
			conversationID, resp.StatusCode, adapter.Truncate(string(respBody), 200))
	}
	return nil
}

// buildMessageMultipart constructs the multipart/form-data body for the
// POST /conversations/{id}/messages call when attachments are present.
// Returns the body, the Content-Type header value (including the multipart
// boundary), and any error encountered while decoding an attachment's data:
// URL or writing the multipart parts.
//
// Per Chatwoot API (messages_controller#create): fields content (empty when
// the transcript already lives in the conversation-create message),
// message_type=outgoing (pin agent-side behavior), and one attachments[] part
// per p.Attachments entry. Filename falls back to "screenshot N" (1-indexed)
// when Label is nil/empty. See L020 for why this call exists separately from
// conversation-create.
func buildMessageMultipart(p *payload.IntakePayload) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("content", ""); err != nil {
		return nil, "", fmt.Errorf("chatwoot: multipart field content: %w", err)
	}
	if err := w.WriteField("message_type", "outgoing"); err != nil {
		return nil, "", fmt.Errorf("chatwoot: multipart field message_type: %w", err)
	}

	for i, att := range p.Attachments {
		raw, _, err := attachvalidate.DecodeOne(att)
		if err != nil {
			return nil, "", fmt.Errorf("chatwoot: decode attachment %d: %w", i+1, err)
		}
		filename := ""
		if att.Label != nil {
			filename = *att.Label
		}
		if filename == "" {
			filename = fmt.Sprintf("screenshot %d", i+1)
		}
		hdr := textproto.MIMEHeader{
			"Content-Disposition": []string{fmt.Sprintf(`form-data; name="attachments[]"; filename=%q`, filename)},
			"Content-Type":        []string{att.MimeType},
		}
		part, err := w.CreatePart(hdr)
		if err != nil {
			return nil, "", fmt.Errorf("chatwoot: multipart create part %d: %w", i+1, err)
		}
		if _, err := part.Write(raw); err != nil {
			return nil, "", fmt.Errorf("chatwoot: multipart write part %d: %w", i+1, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("chatwoot: multipart close: %w", err)
	}
	return &buf, w.FormDataContentType(), nil
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
