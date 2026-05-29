package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"intake/internal/auth"
	"intake/internal/llm"
)

// initHandler handles POST /v1/intake/init.
// Response: InitResponse{SessionID, Capabilities, Auth?, Captcha?}.
//
// Phase 4: AuthModes includes "anonymous"/"email"/"sso" based on cfg.Auth.Modes;
// InitResponse.Auth carries hints (currently just email.code_ttl_seconds).
//
// Phase 5 (5-i): when captcha gates one of the enabled auth modes, the response
// also carries Capabilities.RequiresCaptcha + Captcha. The body may carry
// captcha_token. If captcha is required but the token is missing, returns
// 400 captcha_required with the same discovery fields so the widget can render
// the challenge. (5-iii adds the actual verifier call.)
func initHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil {
			writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
			return
		}

		// Decode optional InitRequest. Empty body is allowed (Phase 1 behavior);
		// a malformed body returns 400. io.EOF on empty body is normal — not an error.
		var initReq InitRequest
		if err := json.NewDecoder(r.Body).Decode(&initReq); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: "+err.Error())
			return
		}

		// Compute the auth modes list (4-ii).
		modes := make([]string, 0, 3)
		if deps.AuthCfg.Modes.Anonymous {
			modes = append(modes, "anonymous")
		}
		if deps.AuthCfg.Modes.Email {
			modes = append(modes, "email")
		}
		if deps.AuthCfg.Modes.SSO {
			modes = append(modes, "sso")
		}
		if len(modes) == 0 {
			modes = []string{"anonymous"}
		}

		// Compute the captcha discovery hint (5-i).
		var captchaHint *InitCaptcha
		var requiresCaptcha []string
		if deps.CaptchaCfg.Enabled {
			// requires_captcha = intersection(modes, deps.CaptchaCfg.RequiredFor)
			rfSet := make(map[string]struct{}, len(deps.CaptchaCfg.RequiredFor))
			for _, m := range deps.CaptchaCfg.RequiredFor {
				rfSet[m] = struct{}{}
			}
			for _, m := range modes {
				if _, ok := rfSet[m]; ok {
					requiresCaptcha = append(requiresCaptcha, m)
				}
			}
			if len(requiresCaptcha) > 0 {
				captchaHint = &InitCaptcha{
					Provider: deps.CaptchaCfg.Provider,
					SiteKey:  deps.CaptchaCfg.SiteKey,
				}
			}
		}

		// If captcha is required and the body omits captcha_token, return 400
		// captcha_required with the discovery fields (5-i shape; 5-iii adds
		// the actual Verify call when the token IS present).
		if len(requiresCaptcha) > 0 && initReq.CaptchaToken == "" {
			writeJSON(w, http.StatusBadRequest, CaptchaRequiredResponse{
				Error: ErrorBody{
					Code:    "captcha_required",
					Message: "call /init again with a solved captcha_token",
				},
				Capabilities: Capabilities{
					AuthModes:       modes,
					Streaming:       true,
					RequiresCaptcha: requiresCaptcha,
				},
				Captcha: captchaHint,
			})
			return
		}

		// 5-iii: verify the captcha token now that we know it's present and required.
		if len(requiresCaptcha) > 0 && initReq.CaptchaToken != "" && deps.CaptchaVerifier != nil {
			clientIP := ClientIPFromContext(r.Context())
			ok, reason, err := deps.CaptchaVerifier.Verify(r.Context(), initReq.CaptchaToken, clientIP)
			if err != nil {
				slog.WarnContext(r.Context(), "init: captcha siteverify unavailable", "provider", deps.CaptchaVerifier.Provider(), "err", err)
				writeError(w, http.StatusBadGateway, "captcha_unavailable", "captcha verification provider unavailable")
				return
			}
			if !ok {
				writeJSON(w, http.StatusUnauthorized, CaptchaFailedResponse{
					Error: CaptchaFailedError{
						Code:    "captcha_failed",
						Message: "captcha verification failed",
						Reason:  reason,
					},
				})
				return
			}
		}

		sessionID := deps.Auth.Store().Issue()

		caps := Capabilities{
			AuthModes:       modes,
			Streaming:       true,
			RequiresCaptcha: requiresCaptcha,
		}
		// Phase 6 (6-i): emit capabilities.attachments when the published
		// allowlist (cfg.AllowedMIMETypes ∩ enabled adapter union, computed
		// once at startup by computeAttachmentsCaps) is non-empty.
		if deps.AttachmentsCfg.Enabled && len(deps.AttachmentMIMEs) > 0 {
			caps.Attachments = &CapabilitiesAttachments{
				MaxSizeBytes:     deps.AttachmentsCfg.MaxSizeBytes,
				MaxTotalBytes:    deps.AttachmentsCfg.MaxTotalBytes,
				AllowedMIMETypes: deps.AttachmentMIMEs,
			}
		}

		resp := InitResponse{
			SessionID:    sessionID,
			Capabilities: caps,
			Captcha:      captchaHint,
		}

		if deps.AuthCfg.Modes.Email {
			d, err := time.ParseDuration(deps.AuthCfg.Email.CodeTTL)
			if err != nil {
				slog.WarnContext(r.Context(), "init: auth.email.code_ttl unparseable; omitting Auth.Email hint",
					"code_ttl", deps.AuthCfg.Email.CodeTTL, "error", err)
			} else {
				resp.Auth = &InitAuth{Email: &InitAuthEmail{CodeTTLSeconds: int(d.Seconds())}}
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// turnHandler handles POST /v1/intake/turn (behind the auth middleware).
//
// Phase 5 gating, applied in order:
//  1. Resolve SessionContext (already in ctx from auth middleware).
//  2. deps.Auth.Store().CheckSession — 429 session_turns/_tokens_exhausted
//     or 401 session_expired on reject.
//  3. deps.Budget.Reserve — 503 daily_budget_exhausted on reject.
//  4. provider.Chat (Phase 1+4 unchanged).
//  5. On SSEDone: deps.Budget.Commit + deps.Auth.Store().RecordTurn.
//
// Reserve uses conservative estimates (4-chars/token input; deps.MaxTokens out)
// so the gate trips BEFORE the LLM call. An aborted stream means no Commit
// and no RecordTurn — failed turns don't count against the user.
func turnHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req TurnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: "+err.Error())
			return
		}

		sess, _ := auth.FromContext(r.Context())
		if sess == nil {
			// 500 here means the auth middleware wasn't wired — a server misconfig.
			// Log so ops can find this in their dashboards.
			slog.ErrorContext(r.Context(), "turn: session context missing from request — auth middleware not wired?")
			writeError(w, http.StatusInternalServerError, "internal", "session context missing")
			return
		}

		// Phase 5 gate 1: per-session caps.
		if deps.Auth != nil {
			ok, retryAfter, code := deps.Auth.Store().CheckSession(sess.SessionID)
			if !ok {
				if code == "session_expired" {
					writeError(w, http.StatusUnauthorized, "session_expired", "session expired; call POST /v1/intake/init again")
					return
				}
				// Note: when retryAfter is close to the TTL boundary, the floored 1s
				// advertised here can race into a 401 session_expired on the next call.
				// That is the intended behavior — the client should re-init when the
				// TTL has elapsed. Observability dashboards plotting "429→success"
				// funnels will see expected drops at this race window.
				setRetryAfter(w, retryAfter)
				var msg string
				if code == "session_turns_exhausted" {
					msg = "session turn limit reached"
				} else {
					msg = "session input-token limit reached"
				}
				writeError(w, http.StatusTooManyRequests, code, msg)
				return
			}
		}

		// Phase 5 gate 2: daily LLM budget.
		tenantKey := r.Header.Get("X-Intake-Tenant")
		estIn := approximateInputTokens(req.Messages)
		estOut := deps.MaxTokens
		if deps.Budget != nil {
			ok, retryAfter := deps.Budget.Reserve(tenantKey, estIn, estOut)
			if !ok {
				setRetryAfter(w, retryAfter)
				writeError(w, http.StatusServiceUnavailable, "daily_budget_exhausted", "relay daily LLM budget reached")
				return
			}
		}

		// Build the message list for the provider.
		// System prompt is always prepended; never sent by or exposed to the client.
		msgs := make([]llm.Message, 0, len(req.Messages)+1)
		if deps.SystemPrompt != "" {
			msgs = append(msgs, llm.Message{Role: "system", Content: deps.SystemPrompt})
		}
		for _, m := range req.Messages {
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}

		if deps.Provider == nil {
			writeError(w, http.StatusInternalServerError, "internal", "provider not configured")
			return
		}

		opts := llm.ChatOptions{
			Model:     deps.Model,
			MaxTokens: deps.MaxTokens,
			Stream:    true,
		}

		ch, err := deps.Provider.Chat(r.Context(), msgs, opts)
		if err != nil {
			slog.ErrorContext(r.Context(), "provider chat failed", "error", err)
			writeError(w, http.StatusBadGateway, "provider_error", "upstream provider unavailable")
			return
		}

		// Set SSE headers before writing any body.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, canFlush := w.(http.Flusher)
		ctx := r.Context()

		for {
			select {
			case <-ctx.Done():
				// Client disconnected. Drain so the provider goroutine never blocks on a send;
				// ctx cancellation terminates the provider, which then closes ch.
				go func() {
					for range ch {
					}
				}()
				return
			case chunk, ok := <-ch:
				if !ok {
					// Channel closed without a Done chunk — treat as done.
					return
				}
				if chunk.Err != nil {
					writeSSEFrame(w, SSEError{Error: chunk.Err.Error()})
					if canFlush {
						flusher.Flush()
					}
					return
				}
				if chunk.Delta != "" {
					writeSSEFrame(w, SSEDelta{Delta: chunk.Delta})
					if canFlush {
						flusher.Flush()
					}
				}
				if chunk.Done {
					// Phase 5: Commit budget + RecordTurn on successful SSEDone.
					// Order matters: Budget first (global spend), then per-session record.
					// Both fire ONLY on the success path (chunk.Done with chunk.Err == nil);
					// failed/aborted turns produce neither Commit nor RecordTurn.
					if deps.Budget != nil {
						deps.Budget.Commit(tenantKey, chunk.InputTokens, chunk.OutputTokens)
					}
					if deps.Auth != nil {
						deps.Auth.Store().RecordTurn(sess.SessionID, chunk.InputTokens)
					}
					writeSSEFrame(w, SSEDone{
						Done:         true,
						InputTokens:  chunk.InputTokens,
						OutputTokens: chunk.OutputTokens,
					})
					if canFlush {
						flusher.Flush()
					}
					return
				}
			}
		}
	}
}

// approximateInputTokens returns a conservative estimate of the input-token
// count of the messages, used by budget.Reserve as the pre-flight estimate.
// Uses a simple 4-chars-per-token heuristic (ceiling division so a 1-char
// message still costs 1 token); the actual count comes back in SSEDone and
// replaces this estimate at Commit time.
func approximateInputTokens(msgs []TurnMessage) int {
	const charsPerToken = 4
	total := 0
	for _, m := range msgs {
		total += (len(m.Content) + charsPerToken - 1) / charsPerToken
	}
	return total
}

// writeSSEFrame marshals v to JSON and writes it as a single SSE data frame.
// Format: "data: <json>\n\n"
func writeSSEFrame(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		// Should not happen for our known types; log defensively.
		_, _ = fmt.Fprintf(w, "data: {\"error\":\"internal marshal error\"}\n\n")
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
