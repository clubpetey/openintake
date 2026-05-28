package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"intake/internal/adapter"
	"intake/internal/adapter/chatwoot"
	"intake/internal/adapter/fider"
	"intake/internal/adapter/linear"
	"intake/internal/adapter/webhook"
	"intake/internal/adapter/zendesk"
	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/config"
	licensemgr "intake/internal/license"
	"intake/internal/llm/providers"
	"intake/internal/payloadbuild"
	"intake/internal/router"
	"intake/internal/server"
	"intake/internal/triage"
	"intake/internal/version"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the relay config file")
	licenseFile := flag.String("license-file", "", "path to license.json (overrides config license.file)")
	flag.Parse()

	// --- Logger (structured JSON to stdout) ---
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// --- Config ---
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("relay: config load failed", "error", err)
		os.Exit(1)
	}

	// --- License (3-vi): flag overrides config.license.file, then env/default paths ---
	if *licenseFile != "" {
		cfg.License.File = *licenseFile
	}
	statePath, sperr := licensemgr.DefaultStatePath()
	if sperr != nil {
		logger.Warn("relay: cannot resolve trial-state path; trial will be ephemeral", "error", sperr)
		statePath = ""
	}
	licState, err := licensemgr.Load(cfg.License, statePath, time.Now().UTC())
	if err != nil {
		// Bad signature / unreadable explicit license / present-license-but-no-embedded-key → fatal.
		logger.Error("relay: license verification failed", "error", err)
		os.Exit(1)
	}
	logger.Info("relay: license", "mode", licState.Mode, "detail", licState.Message)

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
	// 4-i: middleware accepts optional email + sso verifiers (both nil here; 4-ii
	// and 4-iii wire them when the corresponding auth.modes.* flag is set).
	middleware := auth.NewMiddleware(store, nil, nil)

	// --- Triage System Prompt ---
	// Loads from cfg.LLM.SystemPromptFile if set; else uses bundled prompt.txt.
	systemPrompt, err := triage.Load(cfg.LLM.SystemPromptFile)
	if err != nil {
		logger.Error("failed to load system prompt", "error", err)
		os.Exit(1)
	}

	// --- Adapter registry (3-i; 3-ii…3-v add adapters; 3-vi adds the license gate) ---
	registry, err := buildRegistry(cfg, licState, logger)
	if err != nil {
		logger.Error("relay: adapter registry build failed", "error", err)
		os.Exit(1)
	}
	if len(registry) == 0 {
		logger.Error("relay: no adapters enabled — enable at least one in config.adapters")
		os.Exit(1)
	}

	// --- Router (3-i) ---
	rules := make([]router.Rule, 0, len(cfg.Routing.Rules))
	for _, rc := range cfg.Routing.Rules {
		rules = append(rules, router.Rule{
			Classification: []string(rc.When.Classification),
			Severity:       []string(rc.When.Severity),
			To:             rc.To,
		})
	}
	rtr, err := router.New(registry, rules, cfg.Routing.DefaultAdapter, logger)
	if err != nil {
		logger.Error("relay: router init failed", "default_adapter", cfg.Routing.DefaultAdapter, "error", err)
		os.Exit(1)
	}
	logger.Info("relay: router ready", "default_adapter", cfg.Routing.DefaultAdapter, "adapters", adapterNames(registry))

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
		Router:       rtr,
		Classifier:   classifier,
		Builder:      builder,
		AuthCfg:      cfg.Auth,
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
		// anthropic: the configured default provider. Unknown providers are
		// already rejected by providers.New before this function is called,
		// so this branch handles anthropic only — not future unknowns.
		return cfg.Anthropic.Model, cfg.Anthropic.MaxTokens
	}
}

// licensed reports whether an adapter may be registered under the current license
// state. Free adapters (RequiresLicense()==false) always pass. A paid adapter passes
// only when the state permits its name; otherwise it is skipped with a clear warning.
// This makes "paid ⇒ gated" structural rather than a per-adapter convention.
func licensed(ad adapter.Adapter, st *licensemgr.State, logger *slog.Logger) bool {
	if !ad.RequiresLicense() {
		return true
	}
	if st.Permits(ad.Name()) {
		return true
	}
	logger.Warn(`relay: adapter requires a license — disabled`,
		"adapter", ad.Name(), "mode", st.Mode, "see", licensemgr.PricingURL)
	return false
}

// buildRegistry constructs the set of enabled adapters. Each Phase-3 adapter
// sub-plan (3-ii…3-v) adds its block here. The license gate is applied uniformly
// via licensed() (3-vi hardening): construct, gate, then resolve token + configure.
// This ordering ensures a paid adapter in free mode is silently skipped — never a
// fatal missing-token error. Tokens resolve via config.RequireSecret and are passed
// into Configure — never read from the environment by the adapter, never logged.
func buildRegistry(cfg *config.Config, licState *licensemgr.State, logger *slog.Logger) (map[string]adapter.Adapter, error) {
	reg := make(map[string]adapter.Adapter)

	// webhook (1-iv) — free.
	if cfg.Adapters.Webhook.Enabled {
		wh := webhook.New()
		if licensed(wh, licState, logger) {
			if err := wh.Configure(map[string]any{
				"url":     cfg.Adapters.Webhook.URL,
				"headers": cfg.Adapters.Webhook.Headers,
				"retry": map[string]any{
					"max_attempts": cfg.Adapters.Webhook.Retry.MaxAttempts,
					"backoff":      cfg.Adapters.Webhook.Retry.Backoff,
				},
			}); err != nil {
				return nil, fmt.Errorf("webhook adapter: %w", err)
			}
			reg[wh.Name()] = wh
			logger.Info("relay: adapter enabled", "adapter", wh.Name())
		}
	}

	// chatwoot (3-ii) — free.
	if cfg.Adapters.Chatwoot.Enabled {
		cw := chatwoot.New()
		if licensed(cw, licState, logger) {
			token, err := config.RequireSecret(cfg.Adapters.Chatwoot.APITokenEnv)
			if err != nil {
				return nil, fmt.Errorf("chatwoot adapter: %w", err)
			}
			if err := cw.Configure(map[string]any{
				"base_url":   cfg.Adapters.Chatwoot.BaseURL,
				"account_id": cfg.Adapters.Chatwoot.AccountID,
				"inbox_id":   cfg.Adapters.Chatwoot.InboxID,
				"api_token":  token,
			}); err != nil {
				return nil, fmt.Errorf("chatwoot adapter: %w", err)
			}
			reg[cw.Name()] = cw
			logger.Info("relay: adapter enabled", "adapter", cw.Name())
		}
	}

	// fider (3-iii) — free.
	if cfg.Adapters.Fider.Enabled {
		fd := fider.New()
		if licensed(fd, licState, logger) {
			key, err := config.RequireSecret(cfg.Adapters.Fider.APIKeyEnv)
			if err != nil {
				return nil, fmt.Errorf("fider adapter: %w", err)
			}
			if err := fd.Configure(map[string]any{
				"base_url": cfg.Adapters.Fider.BaseURL,
				"api_key":  key,
			}); err != nil {
				return nil, fmt.Errorf("fider adapter: %w", err)
			}
			reg[fd.Name()] = fd
			logger.Info("relay: adapter enabled", "adapter", fd.Name())
		}
	}

	// zendesk (3-iv) — PAID; gated generically via RequiresLicense() (3-vi hardening).
	if cfg.Adapters.Zendesk.Enabled {
		zd := zendesk.New()
		if licensed(zd, licState, logger) {
			token, err := config.RequireSecret(cfg.Adapters.Zendesk.APITokenEnv)
			if err != nil {
				return nil, fmt.Errorf("zendesk adapter: %w", err)
			}
			if err := zd.Configure(map[string]any{
				"subdomain":        cfg.Adapters.Zendesk.Subdomain,
				"email":            cfg.Adapters.Zendesk.Email,
				"api_token":        token,
				"default_priority": cfg.Adapters.Zendesk.DefaultPriority,
			}); err != nil {
				return nil, fmt.Errorf("zendesk adapter: %w", err)
			}
			reg[zd.Name()] = zd
			logger.Info("relay: adapter enabled", "adapter", zd.Name())
		}
	}

	// linear (3-v) — PAID; gated generically via RequiresLicense() (3-vi hardening).
	if cfg.Adapters.Linear.Enabled {
		ln := linear.New()
		if licensed(ln, licState, logger) {
			key, err := config.RequireSecret(cfg.Adapters.Linear.APIKeyEnv)
			if err != nil {
				return nil, fmt.Errorf("linear adapter: %w", err)
			}
			if err := ln.Configure(map[string]any{
				"api_key": key,
				"team_id": cfg.Adapters.Linear.TeamID,
			}); err != nil {
				return nil, fmt.Errorf("linear adapter: %w", err)
			}
			reg[ln.Name()] = ln
			logger.Info("relay: adapter enabled", "adapter", ln.Name())
		}
	}

	return reg, nil
}

// adapterNames returns the registry keys sorted alphabetically, for stable logging.
func adapterNames(reg map[string]adapter.Adapter) []string {
	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
