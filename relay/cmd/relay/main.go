package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"intake/internal/adapter/webhook"
	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/config"
	"intake/internal/llm/providers"
	"intake/internal/payloadbuild"
	"intake/internal/server"
	"intake/internal/triage"
	"intake/internal/version"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the relay config file")
	flag.Parse()

	// --- Logger (structured JSON to stdout) ---
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// --- Config ---
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("relay: config load failed", "error", err)
		os.Exit(1)
	}

	// --- LLM Provider (via factory) ---
	// providers.New resolves the required secret internally via config.RequireSecret /
	// config.ResolveSecret. The key is NEVER logged or embedded in any error surfaced here.
	provider, err := providers.New(cfg.LLM)
	if err != nil {
		logger.Error("relay: LLM provider init failed",
			"provider", cfg.LLM.Provider,
			"error", err,
		)
		os.Exit(1)
	}
	logger.Info("relay: LLM provider ready", "provider", provider.Name())

	// --- Model / MaxTokens for classify (derived from the active provider config) ---
	// The classify.New call still needs a model name and max_tokens. We read them
	// from whichever sub-config block corresponds to the active provider.
	// This mirrors the factory switch without constructing a second provider.
	model, maxTokens := activeModelConfig(cfg.LLM)

	// --- Session Store + Auth Middleware ---
	store := auth.NewStore()
	middleware := auth.NewMiddleware(store)

	// --- Triage System Prompt ---
	// Loads from cfg.LLM.SystemPromptFile if set; else uses bundled prompt.txt.
	systemPrompt, err := triage.Load(cfg.LLM.SystemPromptFile)
	if err != nil {
		logger.Error("failed to load system prompt", "error", err)
		os.Exit(1)
	}

	// --- Webhook Adapter (1-iv) ---
	wh := webhook.New()
	whCfg := map[string]any{
		"url":     cfg.Adapters.Webhook.URL,
		"headers": cfg.Adapters.Webhook.Headers,
		"retry": map[string]any{
			"max_attempts": cfg.Adapters.Webhook.Retry.MaxAttempts,
			"backoff":      cfg.Adapters.Webhook.Retry.Backoff,
		},
	}
	if err := wh.Configure(whCfg); err != nil {
		logger.Error("webhook adapter: configure failed", "error", err)
		os.Exit(1)
	}

	// --- Classifier (1-iv) — reuses the same provider as /turn ---
	classifier := classify.New(provider, model, maxTokens)

	// --- Payload Builder (1-iv) ---
	builder := payloadbuild.New("0.1.0") // widget version default; Phase 5 may read from config

	// --- Deps ---
	// Deps is a value type (README §6.8). No Config field — config-derived values
	// are promoted to individual Deps fields. main.go populates these from cfg.
	deps := server.Deps{
		Version:      version.Info(),
		CORSOrigins:  cfg.Server.CORSOrigins,
		Logger:       logger,
		Auth:         middleware,
		Provider:     provider,
		SystemPrompt: systemPrompt,
		Model:        model,
		MaxTokens:    maxTokens,
		Adapter:      wh,
		Classifier:   classifier,
		Builder:      builder,
	}

	// --- HTTP Server ---
	handler := server.New(cfg, deps)
	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally 0: the /turn SSE handler streams for the
		// duration of an LLM response; a write deadline would truncate it.
		// Revisit per-route write deadlines when SSE lands.
	}

	// Start the server in a goroutine so the main goroutine can wait for the
	// shutdown signal.
	idleConnsClosed := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("relay: shutdown signal received; draining connections...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("relay: graceful shutdown error", "error", err)
		}
		close(idleConnsClosed)
	}()

	logger.Info("relay listening", "addr", cfg.Server.Addr, "external", cfg.Server.ExternalURL)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Error("relay: listen error", "error", err)
		os.Exit(1)
	}

	<-idleConnsClosed
	logger.Info("relay stopped")
}

// activeModelConfig returns the model name and maxTokens for the currently
// configured provider. Used to populate server.Deps.Model and .MaxTokens for
// the classifier — which mirrors the factory's provider selection without
// constructing a second provider instance.
func activeModelConfig(cfg config.LLMConfig) (model string, maxTokens int) {
	switch cfg.Provider {
	case "openai":
		return cfg.OpenAI.Model, cfg.OpenAI.MaxTokens
	case "gemini":
		return cfg.Gemini.Model, cfg.Gemini.MaxTokens
	case "ollama":
		return cfg.Ollama.Model, cfg.Ollama.MaxTokens
	default:
		// anthropic (default) and any future unknown provider: fall back to
		// the anthropic block, which is the Phase-1 default.
		return cfg.Anthropic.Model, cfg.Anthropic.MaxTokens
	}
}
