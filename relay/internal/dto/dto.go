// Package dto contains the HTTP Data Transfer Objects (request/response bodies)
// shared between the server handlers and the payloadbuild package.
// Separating them avoids an import cycle between server and payloadbuild.
package dto

// TurnMessage is a single conversation turn (user or assistant).
type TurnMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

// ClientInfo captures browser context sent with each submit.
type ClientInfo struct {
	WidgetVersion string   `json:"widget_version"`
	URL           string   `json:"url"`
	Referrer      *string  `json:"referrer"`
	UserAgent     string   `json:"user_agent"`
	Viewport      Viewport `json:"viewport"`
	Locale        string   `json:"locale"`
}

// Viewport captures the browser window size in CSS pixels.
type Viewport struct {
	W int `json:"w"`
	H int `json:"h"`
}

// ContextInfo carries host-app-supplied metadata attached to each submit.
type ContextInfo struct {
	AppContext   map[string]any `json:"app_context"`
	PageMetadata map[string]any `json:"page_metadata"`
}

// SubmitRequest is the body of POST /v1/intake/submit.
type SubmitRequest struct {
	Messages    []TurnMessage      `json:"messages"`
	Client      ClientInfo         `json:"client"`
	UserClaims  map[string]any     `json:"user_claims"`
	Context     ContextInfo        `json:"context"`
	RoutingHint *string            `json:"routing_hint"`
	Attachments []SubmitAttachment `json:"attachments,omitempty"` // Phase 6
}

// SubmitAttachment is the wire shape for one inline attachment (Phase 6).
// URL is a data: URL (e.g. "data:image/png;base64,iVBORw0KGgo..."); MIMEType
// is the declared content-type and is validated against the actual bytes via
// net/http.DetectContentType in attachvalidate.ValidateAll. Type is
// "screenshot" only in v0; "file" is rejected at attachvalidate with
// 400 attachment_type_unsupported (schema permits "file" so v1 can enable it
// without a schema bump).
type SubmitAttachment struct {
	Type     string `json:"type"`
	MIMEType string `json:"mime_type"`
	URL      string `json:"url"`
	Label    string `json:"label,omitempty"`
}
