package server

import "intake/internal/dto"

// Session transport: the X-Intake-Session header carries the session_id on
// every /turn and /submit request (single source of truth; NOT in the body).

// InitResponse is returned by POST /v1/intake/init.
//
// Phase 1: SessionID + Capabilities{AuthModes:["anonymous"], Streaming:true}.
// Phase 4: Capabilities.AuthModes is extended to include "email"/"sso" when the
// corresponding auth.modes.* flag is true; a new top-level Auth field carries
// per-mode hints (currently just email.code_ttl_seconds).
type InitResponse struct {
	SessionID    string       `json:"session_id"`
	Capabilities Capabilities `json:"capabilities"`
	Auth         *InitAuth    `json:"auth,omitempty"`
	Captcha      *InitCaptcha `json:"captcha,omitempty"` // 5-i: nil when captcha disabled
}

// Capabilities advertises relay feature flags to the widget.
type Capabilities struct {
	AuthModes       []string `json:"auth_modes"`
	Streaming       bool     `json:"streaming"`
	RequiresCaptcha []string `json:"requires_captcha,omitempty"` // 5-i: subset of AuthModes; only when captcha gates ≥1 mode
}

// InitAuth carries per-mode initialization hints. Only emitted when at least
// one enabled mode advertises a hint.
type InitAuth struct {
	Email *InitAuthEmail `json:"email,omitempty"`
}

// InitAuthEmail is the email-mode capability hint.
type InitAuthEmail struct {
	CodeTTLSeconds int `json:"code_ttl_seconds"`
}

// InitCaptcha carries the public CAPTCHA hint so the widget can render the
// challenge. Phase 5.
type InitCaptcha struct {
	Provider string `json:"provider"` // "turnstile" | "hcaptcha"
	SiteKey  string `json:"site_key"` // public; safe to commit
}

// InitRequest is the body of POST /v1/intake/init. Empty in v0 except for
// the optional captcha token. Phase 5.
type InitRequest struct {
	CaptchaToken string `json:"captcha_token,omitempty"`
}

// CaptchaRequiredResponse is the 400 body shape returned when captcha is
// required for the anonymous mode but the request omitted captcha_token.
// Carries the same `capabilities` + `captcha` discovery fields as the success
// path so the widget can render the challenge without a separate call. Phase 5.
type CaptchaRequiredResponse struct {
	Error        ErrorBody    `json:"error"`
	Capabilities Capabilities `json:"capabilities"`
	Captcha      *InitCaptcha `json:"captcha,omitempty"`
}

// CaptchaFailedResponse is the 401 body returned when captcha verification
// fails (Verifier.Verify returns ok=false). Extends the standard ErrorEnvelope
// with a `reason` field carrying the provider's error-codes[0]. Phase 5.
type CaptchaFailedResponse struct {
	Error CaptchaFailedError `json:"error"`
}

// CaptchaFailedError is the inner error shape for CaptchaFailedResponse —
// like ErrorBody but with a `reason` field for the provider error code.
type CaptchaFailedError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
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
