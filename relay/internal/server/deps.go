package server

import (
	"log/slog"

	"intake/internal/adapter"
	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/llm"
	"intake/internal/payloadbuild"
	"intake/internal/version"
)

// Deps holds the dependencies injected into the HTTP server at startup.
//
// Deps is a VALUE type — always passed by value, never as *Deps.
// Add new fields here rather than using global state.
//
// 1-i owns: Version, CORSOrigins.
// Extended by 1-iii: Logger, Auth, Provider, SystemPrompt, Model, MaxTokens.
// Extended by 1-iv: Adapter, Classifier, Builder.
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

	// from 1-iv (README §6.8):

	// Adapter is the downstream ticket-system adapter (webhook in Phase 1).
	Adapter adapter.Adapter

	// Classifier runs the server-side triage LLM call at submit time.
	Classifier *classify.Classifier

	// Builder assembles and schema-validates the canonical payload.IntakePayload.
	Builder *payloadbuild.Builder
}
