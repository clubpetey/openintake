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

	"intake/internal/auth"
	"intake/internal/config"
	"intake/internal/llm/anthropic"
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

	// --- LLM Provider ---
	// API key is read from env; NEVER from config file or logs (security invariant §2).
	apiKey := os.Getenv(cfg.LLM.Anthropic.APIKeyEnv)
	if apiKey == "" {
		logger.Error("LLM API key not set", "env_var", cfg.LLM.Anthropic.APIKeyEnv)
		os.Exit(1)
	}
	provider := anthropic.New(apiKey, cfg.LLM.Anthropic.Model, cfg.LLM.Anthropic.MaxTokens)

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
		Model:        cfg.LLM.Anthropic.Model,
		MaxTokens:    cfg.LLM.Anthropic.MaxTokens,
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
