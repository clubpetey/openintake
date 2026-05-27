package server

// Session transport: the X-Intake-Session header carries the session_id on
// every /turn and /submit request (single source of truth; NOT in the body).

// InitResponse is the body returned by POST /v1/intake/init.
type InitResponse struct {
	SessionID    string       `json:"session_id"`
	Capabilities Capabilities `json:"capabilities"`
}

// Capabilities advertises relay feature flags to the widget.
type Capabilities struct {
	AuthModes []string `json:"auth_modes"` // Phase 1: ["anonymous"]
	Streaming bool     `json:"streaming"`  // true
}

// TurnMessage is a single conversation turn (user or assistant).
type TurnMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

// TurnRequest is the body of POST /v1/intake/turn.
type TurnRequest struct {
	Messages []TurnMessage `json:"messages"`
}

// SSEDelta is an SSE frame carrying a streaming token delta.
type SSEDelta struct {
	Delta string `json:"delta"`
}

// SSEDone is the terminal SSE frame for a successful turn.
type SSEDone struct {
	Done         bool `json:"done"`
	InputTokens  int  `json:"input_tokens"`
	OutputTokens int  `json:"output_tokens"`
}

// SSEError is the terminal SSE frame for a failed turn.
type SSEError struct {
	Error string `json:"error"`
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
// Attachments are deferred to Phase 6.
type SubmitRequest struct {
	Messages    []TurnMessage  `json:"messages"`
	Client      ClientInfo     `json:"client"`
	UserClaims  map[string]any `json:"user_claims"`
	Context     ContextInfo    `json:"context"`
	RoutingHint *string        `json:"routing_hint"`
}

// SubmitResponse is the body returned by POST /v1/intake/submit on success.
type SubmitResponse struct {
	ExternalID  string `json:"external_id"`
	ExternalURL string `json:"external_url"`
	AdapterName string `json:"adapter_name"`
	CreatedAt   string `json:"created_at"`
}

// ErrorEnvelope is the standard error response body for all relay endpoints.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody holds the machine-readable code and human-readable message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
