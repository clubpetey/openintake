package server

import (
	"log/slog"
	"net/netip"

	"intake/internal/auth"
	"intake/internal/budget"
	"intake/internal/captcha"
	"intake/internal/classify"
	"intake/internal/config"
	"intake/internal/llm"
	"intake/internal/payloadbuild"
	"intake/internal/ratelimit/perip"
	"intake/internal/router"
	"intake/internal/version"
)

// Deps holds the dependencies injected into the HTTP server at startup.
//
// Deps is a VALUE type — always passed by value, never as *Deps.
// Add new fields here rather than using global state.
//
// 1-i owns: Version, CORSOrigins.
// Extended by 1-iii: Logger, Auth, Provider, SystemPrompt, Model, MaxTokens.
// Extended by 1-iv: Router, Classifier, Builder.
type Deps struct {
	// from 1-i (README §6.8):

	// Version is populated from the binary's build-time ldflags.
	Version version.BuildInfo

	// CORSOrigins is the strict allowlist of origins that may make cross-origin
	// requests. Populated from cfg.Server.CORSOrigins in main.go.
	CORSOrigins []string

	// from 1-iii (README §6.8):

	// Logger is the structured logger for the server. Uses slog.Default() if nil.
	Logger *slog.Logger

	// Auth resolves per-request identity. nil = auth not wired (unit tests may omit).
	Auth *auth.Middleware

	// Provider is the LLM backend. nil = not wired (unit tests may stub).
	Provider llm.Provider

	// SystemPrompt is the triage system prompt text (loaded via triage.Load).
	SystemPrompt string

	// Model is the LLM model name, e.g. "claude-sonnet-4-6".
	Model string

	// MaxTokens is the maximum output tokens per turn.
	MaxTokens int

	// from 1-iv, generalized in 3-i:

	// Router resolves a submission to one downstream adapter (routing_hint→rules→default).
	Router *router.Router

	// Classifier runs the server-side triage LLM call at submit time.
	Classifier *classify.Classifier

	// Builder assembles and schema-validates the canonical payload.IntakePayload.
	Builder *payloadbuild.Builder

	// AuthCfg is the auth section of the loaded config — needed by initHandler
	// to emit the correct capabilities + auth hints. Set by main.go.
	AuthCfg config.AuthConfig

	// from 4-ii:

	// EmailService is the orchestrator for /auth/email/start and /auth/email/verify.
	// nil when auth.modes.email is false (handlers respond 404 in that case).
	EmailService *EmailService

	// from 5-i (Phase 5):

	// CaptchaCfg is the captcha section of the loaded config. initHandler reads
	// it to decide whether to emit RequiresCaptcha + InitCaptcha hints and (with
	// CaptchaVerifier) whether to demand a captcha_token in the request body.
	CaptchaCfg config.CaptchaConfig

	// CaptchaVerifier is the verifier instance. nil → "no captcha required"
	// (initHandler treats nil + CaptchaCfg.Enabled=false the same way).
	CaptchaVerifier captcha.Verifier

	// Budget tracks the daily LLM spend. nil → no budget gate (unit tests of /init).
	Budget *budget.Tracker

	// PerIP is the per-IP rate limiter (used by perIPLimitMiddleware in server.New).
	// nil → no per-IP gate.
	PerIP *perip.Limiter

	// TrustedProxies is the parsed CIDR list (parsed once at startup in main.go;
	// consumed by clientIPMiddleware in server.New).
	TrustedProxies []netip.Prefix

	// from 6-i (Phase 6):

	// AttachmentsCfg is the attachments section of the loaded config. submitHandler
	// reads it to construct the attachvalidate.Config and to enforce the
	// attachments_disabled short-circuit when Enabled=false; initHandler reads it
	// to populate Capabilities.Attachments (in conjunction with AttachmentMIMEs).
	AttachmentsCfg config.AttachmentsConfig

	// AttachmentMIMEs is the published allowlist (cfg.AllowedMIMETypes ∩ enabled
	// adapter union), computed once at startup via ComputeAttachmentsCaps. Empty
	// → /init omits capabilities.attachments AND submitHandler refuses any
	// non-empty attachments[] with 400 attachments_disabled.
	AttachmentMIMEs []string

	// BodyCapBytes is the per-request MaxBytesReader cap on /submit. 14*1<<20 (14 MB)
	// when cfg.Attachments.Enabled=true; 1<<20 (1 MB) otherwise. main.go sets it
	// once at startup based on cfg.Attachments.Enabled.
	BodyCapBytes int64
}
