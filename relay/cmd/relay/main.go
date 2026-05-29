package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
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
	"intake/internal/auth/emailcode"
	"intake/internal/auth/emailjwt"
	"intake/internal/auth/smtpsend"
	"intake/internal/auth/sso"
	"intake/internal/budget"
	"intake/internal/captcha"
	"intake/internal/classify"
	"intake/internal/config"
	licensemgr "intake/internal/license"
	"intake/internal/llm/providers"
	"intake/internal/payloadbuild"
	"intake/internal/ratelimit/perip"
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

	// --- Q9 consolidated startup gate (Phase 5) ---
	// All Phase 4 + Phase 5 misconfigs collected into one structured Error line.
	// Operators fix every problem in one restart cycle, not three.
	problems, trustedProxies := startupProblems(cfg)
	if len(problems) > 0 {
		logger.Error("relay: startup config errors", "count", len(problems), "problems", problems)
		os.Exit(1)
	}
	// trustedProxies is the parsed []netip.Prefix from cfg.Server.TrustedProxies
	// — parsed once inside startupProblems; used by clientIPMiddleware via Deps.

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
	// Phase 5 (5-ii): Store gains per-session caps + TTL from cfg.RateLimit.PerSession.
	sessionTTL, err := time.ParseDuration(cfg.RateLimit.PerSession.SessionTTL)
	if err != nil {
		logger.Error("relay: invalid ratelimit.per_session.session_ttl", "value", cfg.RateLimit.PerSession.SessionTTL, "err", err)
		os.Exit(1)
	}
	store := auth.NewStoreWithCaps(
		cfg.RateLimit.PerSession.MaxTurns,
		cfg.RateLimit.PerSession.MaxInputTokens,
		sessionTTL,
		time.Now,
	)

	// 4-ii: email magic-link wiring.
	var emailVerifier auth.EmailJWTVerifier // nil unless cfg.Auth.Modes.Email is true
	var emailSvc *server.EmailService
	if cfg.Auth.Modes.Email {
		smtpPass, err := config.ResolveSecret(cfg.Auth.Email.SMTPPassEnv)
		if err != nil {
			logger.Error("email auth: resolve SMTP password", "env", cfg.Auth.Email.SMTPPassEnv, "err", err)
			os.Exit(1)
		}
		// smtpPass may legitimately be empty (e.g. local MailHog with no auth);
		// the SMTPUser presence is the operator's choice.

		jwtSecret, err := config.RequireSecret(cfg.Auth.Email.JWTSecretEnv)
		if err != nil {
			logger.Error("email auth: resolve JWT secret", "env", cfg.Auth.Email.JWTSecretEnv, "err", err)
			os.Exit(1)
		}
		if len(jwtSecret) < 32 {
			logger.Error("email auth: jwt_secret must be at least 32 bytes (PROJECT.md §17)", "env", cfg.Auth.Email.JWTSecretEnv, "len", len(jwtSecret))
			os.Exit(1)
		}

		codeTTL, err := time.ParseDuration(cfg.Auth.Email.CodeTTL)
		if err != nil {
			logger.Error("email auth: invalid code_ttl", "value", cfg.Auth.Email.CodeTTL, "err", err)
			os.Exit(1)
		}
		jwtTTL, err := time.ParseDuration(cfg.Auth.Email.JWTTTL)
		if err != nil {
			logger.Error("email auth: invalid jwt_ttl", "value", cfg.Auth.Email.JWTTTL, "err", err)
			os.Exit(1)
		}

		// Rate-limit: 3 codes per code_ttl window (matches the design — "≥3 codes
		// in 10 min for this address" per spec §2.4).
		const perWindowCap = 3
		codeStore := emailcode.New(codeTTL, codeTTL, perWindowCap, time.Now)

		sender := smtpsend.NewNetSMTP(
			cfg.Auth.Email.SMTPHost,
			cfg.Auth.Email.SMTPPort,
			cfg.Auth.Email.SMTPUser,
			smtpPass,
			cfg.Auth.Email.From,
		)

		secretBytes := []byte(jwtSecret)
		emailSvc = server.NewEmailService(codeStore, sender, secretBytes, jwtTTL)
		emailVerifier = &emailjwt.Verifier{Secret: secretBytes}

		logger.Info("relay: email auth enabled",
			"smtp_host", cfg.Auth.Email.SMTPHost,
			"smtp_port", cfg.Auth.Email.SMTPPort,
			"from", cfg.Auth.Email.From,
			"code_ttl", codeTTL.String(),
			"jwt_ttl", jwtTTL.String(),
		)
	}

	// 4-iii: construct the SSO verifier when sso mode is enabled.
	var ssoVerifier auth.SSOVerifier
	if cfg.Auth.Modes.SSO {
		var hs256Secret []byte
		if cfg.Auth.SSO.HS256SecretEnv != "" {
			s, err := config.RequireSecret(cfg.Auth.SSO.HS256SecretEnv)
			if err != nil {
				logger.Error("sso: resolve HS256 secret", "env", cfg.Auth.SSO.HS256SecretEnv, "err", err)
				os.Exit(1)
			}
			hs256Secret = []byte(s)
		}
		v, err := sso.New(cfg.Auth.SSO, hs256Secret, logger)
		if err != nil {
			// Catches: both jwks_url and hs256_secret_env set, neither set,
			// JWKS unreachable at startup, HS256 secret <32 bytes.
			logger.Error("sso: construct verifier", "err", err)
			os.Exit(1)
		}
		ssoVerifier = v
		logger.Info("relay: sso auth enabled",
			"issuer", cfg.Auth.SSO.Issuer,
			"audience", cfg.Auth.SSO.Audience,
			"mode", func() string {
				if cfg.Auth.SSO.JWKSURL != "" {
					return "rs256-jwks"
				}
				return "hs256-shared-secret"
			}(),
		)
	}

	// 4-i: middleware accepts optional email + sso verifiers.
	// 4-ii: emailVerifier is nil-OK when email mode disabled.
	// 4-iii: ssoVerifier is nil-OK when sso mode disabled.
	// Phase 5: switch to the modes-aware constructor; modesAnonymous is read directly
	// from cfg so the dispatcher rejects anonymous when the operator disabled the mode.
	middleware := auth.NewMiddlewareWithModes(store, emailVerifier, ssoVerifier, cfg.Auth.Modes.Anonymous)

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

	// --- Phase 6 attachments startup gate ---
	// Runs after buildRegistry because the warn-on-unknown-MIME side-channel
	// needs the enabled-adapter list. Returns the PARSED AttachmentsConfig per
	// L016 — consumers below must use `attachmentsCfg`, not `cfg.Attachments`.
	enabledList := make([]adapter.Adapter, 0, len(registry))
	for _, ad := range registry {
		enabledList = append(enabledList, ad)
	}
	attachmentsCfg, attProblems := validateAttachments(cfg, enabledList)
	if len(attProblems) > 0 {
		logger.Error("relay: startup config errors", "count", len(attProblems), "problems", attProblems)
		os.Exit(1)
	}
	// Compute the published allowlist (cfg ∩ adapter union) — empty list means
	// /init will omit capabilities.attachments and submitHandler will refuse
	// non-empty attachments[] with 400 attachments_disabled.
	attCaps := server.ComputeAttachmentsCaps(attachmentsCfg, enabledList)
	var attachmentMIMEs []string
	if attCaps != nil {
		attachmentMIMEs = attCaps.AllowedMIMETypes
	}
	// Body cap: 14 MB when enabled, 1 MB otherwise. Computed once at startup.
	bodyCapBytes := int64(1 << 20)
	if attachmentsCfg.Enabled {
		bodyCapBytes = 14 * (1 << 20)
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

	// Phase 5 (5-ii): per-IP rate limiter + daily-budget tracker.
	idleTTL, err := time.ParseDuration(cfg.RateLimit.PerIP.IdleTTL)
	if err != nil {
		logger.Error("relay: invalid ratelimit.per_ip.idle_ttl", "value", cfg.RateLimit.PerIP.IdleTTL, "err", err)
		os.Exit(1)
	}
	perIPLimiter := perip.New(
		cfg.RateLimit.PerIP.RequestsPerSecond,
		cfg.RateLimit.PerIP.Burst,
		idleTTL,
		time.Now,
	)
	budgetTracker := budget.New(
		cfg.RateLimit.DailyLLMBudget.MaxInputTokens,
		cfg.RateLimit.DailyLLMBudget.MaxOutputTokens,
		time.Now,
	)
	logger.Info("relay: rate limits configured",
		"per_ip_rps", cfg.RateLimit.PerIP.RequestsPerSecond,
		"per_ip_burst", cfg.RateLimit.PerIP.Burst,
		"per_session_max_turns", cfg.RateLimit.PerSession.MaxTurns,
		"per_session_max_input_tokens", cfg.RateLimit.PerSession.MaxInputTokens,
		"daily_budget_max_input_tokens", cfg.RateLimit.DailyLLMBudget.MaxInputTokens,
		"daily_budget_max_output_tokens", cfg.RateLimit.DailyLLMBudget.MaxOutputTokens,
	)

	// Phase 5 (5-iii): construct the captcha verifier when enabled.
	var captchaVerifier captcha.Verifier
	if cfg.Captcha.Enabled {
		if cfg.Captcha.SecretKeyEnv == "" {
			logger.Error("relay: captcha.enabled=true requires captcha.secret_key_env")
			os.Exit(1)
		}
		secret, err := config.RequireSecret(cfg.Captcha.SecretKeyEnv)
		if err != nil {
			logger.Error("captcha: resolve secret", "env", cfg.Captcha.SecretKeyEnv, "err", err)
			os.Exit(1)
		}
		v, err := captcha.New(cfg.Captcha.Provider, secret, nil, time.Now)
		if err != nil {
			logger.Error("captcha: construct verifier", "provider", cfg.Captcha.Provider, "err", err)
			os.Exit(1)
		}
		captchaVerifier = v
		// Log NEVER includes the secret; provider + site_key + required_for are safe.
		logger.Info("relay: captcha enabled",
			"provider", cfg.Captcha.Provider,
			"required_for", cfg.Captcha.RequiredFor,
		)
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
		Model:        model,
		MaxTokens:    maxTokens,
		Router:       rtr,
		Classifier:   classifier,
		Builder:      builder,
		AuthCfg:      cfg.Auth,
		EmailService: emailSvc,

		// Phase 5 (5-i): config + parsed prefixes wired in; the actual
		// Limiter, Budget, and CaptchaVerifier are nil here — 5-ii and 5-iii
		// replace nil with the real instances.
		CaptchaCfg:      cfg.Captcha,
		CaptchaVerifier: captchaVerifier, // 5-iii: nil when cfg.Captcha.Enabled=false; real verifier otherwise
		Budget:          budgetTracker,   // 5-ii
		PerIP:           perIPLimiter,    // 5-ii
		TrustedProxies:  trustedProxies,

		// Phase 6 (6-i):
		AttachmentsCfg:  attachmentsCfg,
		AttachmentMIMEs: attachmentMIMEs,
		BodyCapBytes:    bodyCapBytes,
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

// startupProblems enumerates every Phase 4 + Phase 5 misconfig in cfg as a flat
// []string. main() logs the slice in one structured Error line and exits 1 when
// non-empty (PROJECT.md §19 Q9 fail-closed; PHASE_PLANNING §4 build-fail discipline).
//
// Also returns the parsed []netip.Prefix from cfg.Server.TrustedProxies as a side
// product of validation. Callers that proceed past the gate (problems is empty)
// can use the parsed slice directly, avoiding a re-parse with discarded errors.
// Each problem string is self-describing — names the offending key, the value
// it found, and the fix. Order: anonymous gate, SSO mutual-exclusivity,
// trusted-proxy CIDR parse, config.Validate (currently action_on_exceeded).
func startupProblems(cfg *config.Config) (problems []string, trustedProxies []netip.Prefix) {
	// Q9: anonymous-without-captcha-gating.
	if cfg.Auth.Modes.Anonymous {
		anonymousProtected := cfg.Captcha.Enabled && containsString(cfg.Captcha.RequiredFor, "anonymous")
		if !anonymousProtected && !cfg.Auth.Anonymous.AllowWithoutCaptcha {
			problems = append(problems, `auth.modes.anonymous=true requires captcha.enabled=true AND captcha.required_for to include "anonymous"; or set auth.anonymous.allow_without_captcha=true to acknowledge the risk (PROJECT.md §19 Q9)`)
		}
	}

	// SSO mutual-exclusivity (Phase 4; consolidated here so all gates fire in one pass).
	if cfg.Auth.Modes.SSO {
		jwks := cfg.Auth.SSO.JWKSURL != ""
		hs := cfg.Auth.SSO.HS256SecretEnv != ""
		if jwks && hs {
			problems = append(problems, "auth.modes.sso=true: both jwks_url and hs256_secret_env are set; exactly one required")
		}
		if !jwks && !hs {
			problems = append(problems, "auth.modes.sso=true: neither jwks_url nor hs256_secret_env is set; exactly one required")
		}
	}

	// Trusted-proxy CIDRs — fatal at startup, not at first request.
	// Successfully parsed prefixes are returned alongside problems so the caller
	// can use them directly without a re-parse.
	trustedProxies = make([]netip.Prefix, 0, len(cfg.Server.TrustedProxies))
	for _, raw := range cfg.Server.TrustedProxies {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			problems = append(problems, fmt.Sprintf("server.trusted_proxies contains an invalid CIDR %q: %v", raw, err))
			continue
		}
		trustedProxies = append(trustedProxies, p)
	}

	// Phase 5: validate rate-limit duration fields parse cleanly. main.go's
	// inline ParseDuration calls are defense-in-depth, but having the check
	// here means an operator with multiple bad durations sees one consolidated
	// error log line, not one per restart cycle.
	if _, err := time.ParseDuration(cfg.RateLimit.PerSession.SessionTTL); err != nil {
		problems = append(problems, fmt.Sprintf("ratelimit.per_session.session_ttl=%q is not a valid Go duration (e.g. \"1h\", \"30m\"): %v", cfg.RateLimit.PerSession.SessionTTL, err))
	}
	if _, err := time.ParseDuration(cfg.RateLimit.PerIP.IdleTTL); err != nil {
		problems = append(problems, fmt.Sprintf("ratelimit.per_ip.idle_ttl=%q is not a valid Go duration (e.g. \"15m\", \"5m\"): %v", cfg.RateLimit.PerIP.IdleTTL, err))
	}

	// Config-level validation (action_on_exceeded etc.).
	if err := cfg.Validate(); err != nil {
		problems = append(problems, err.Error())
	}

	return problems, trustedProxies
}

// containsString reports whether haystack contains needle (case-sensitive).
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// validateAttachments validates the Phase 6 attachments block. Returns the
// parsed/defaulted AttachmentsConfig per L016 — consumers (ComputeAttachmentsCaps,
// attachvalidate.Config) MUST use the returned value rather than re-reading cfg.
//
// Gates (fatal — append to problems):
//   - Storage.Mode set to anything other than "" or "forward".
//   - MaxTotalBytes > 0 AND MaxSizeBytes > MaxTotalBytes (an inverted cap pair
//     is always operator error; the per-attachment cap is unreachable).
//
// Side-channel (warn-not-fatal):
//   - AllowedMIMETypes contains a MIME that no enabled adapter advertises.
//     This is legitimate (operator may want a stricter allowlist than any
//     single adapter), so it emits slog.Warn rather than blocking startup.
//
// When Enabled=false the function is a no-op (returns the cfg unchanged with
// zero problems) — a disabled feature shouldn't fail startup.
func validateAttachments(cfg *config.Config, enabled []adapter.Adapter) (config.AttachmentsConfig, []string) {
	parsed := cfg.Attachments
	if !parsed.Enabled {
		return parsed, nil
	}

	var problems []string

	// Gate 1: storage.mode.
	switch parsed.Storage.Mode {
	case "", "forward":
		// OK.
	default:
		problems = append(problems, fmt.Sprintf("attachments.storage.mode=%q is not supported in v0; only \"\" or \"forward\" is supported (S3 storage is v1+)", parsed.Storage.Mode))
	}

	// Gate 2: cap pair sanity.
	if parsed.MaxTotalBytes > 0 && parsed.MaxSizeBytes > parsed.MaxTotalBytes {
		problems = append(problems, fmt.Sprintf("attachments.max_size_bytes=%d exceeds attachments.max_total_bytes=%d; per-attachment cap must be <= aggregate cap", parsed.MaxSizeBytes, parsed.MaxTotalBytes))
	}

	// Warn (not fatal): MIMEs in the allowlist that no adapter advertises.
	if len(parsed.AllowedMIMETypes) > 0 {
		adapterUnion := make(map[string]bool)
		for _, ad := range enabled {
			if c, ok := ad.(adapter.CapableAdapter); ok {
				for _, m := range c.Capabilities().AcceptedMIMETypes {
					adapterUnion[m] = true
				}
			}
		}
		var unknown []string
		for _, m := range parsed.AllowedMIMETypes {
			if !adapterUnion[m] {
				unknown = append(unknown, m)
			}
		}
		if len(unknown) > 0 {
			slog.Warn("relay: attachments.allowed_mime_types contains types no enabled adapter advertises; widget will hide these",
				"unknown", unknown,
				"enabled_adapters", adapterNames(adapterRegistryFromSlice(enabled)),
			)
		}
	}

	return parsed, problems
}

// adapterRegistryFromSlice is a tiny shim so we can reuse adapterNames (which
// already exists in this file and takes map[string]adapter.Adapter) from a
// []adapter.Adapter input.
func adapterRegistryFromSlice(enabled []adapter.Adapter) map[string]adapter.Adapter {
	out := make(map[string]adapter.Adapter, len(enabled))
	for _, ad := range enabled {
		out[ad.Name()] = ad
	}
	return out
}
