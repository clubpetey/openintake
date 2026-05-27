package server

import "intake/internal/dto"

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
// Alias to dto.TurnMessage to avoid import cycle with payloadbuild.
type TurnMessage = dto.TurnMessage

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
// Alias to dto.ClientInfo to avoid import cycle with payloadbuild.
type ClientInfo = dto.ClientInfo

// Viewport captures the browser window size in CSS pixels.
// Alias to dto.Viewport to avoid import cycle with payloadbuild.
type Viewport = dto.Viewport

// ContextInfo carries host-app-supplied metadata attached to each submit.
// Alias to dto.ContextInfo to avoid import cycle with payloadbuild.
type ContextInfo = dto.ContextInfo

// SubmitRequest is the body of POST /v1/intake/submit.
// Alias to dto.SubmitRequest to avoid import cycle with payloadbuild.
// Attachments are deferred to Phase 6.
type SubmitRequest = dto.SubmitRequest

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
