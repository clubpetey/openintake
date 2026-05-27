package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"intake/internal/auth"
	"intake/internal/llm"
	"intake/internal/payloadbuild"
)

// submitHandler handles POST /v1/intake/submit.
// It:
//  1. Decodes the SubmitRequest body.
//  2. Extracts the SessionContext from context (placed by auth middleware).
//  3. Builds []llm.Message from request messages.
//  4. Calls Classifier.Classify to produce a classify.Result.
//  5. Calls Builder.Build to assemble + validate the canonical payload.
//  6. Calls Adapter.Create to POST to the downstream system.
//  7. Returns a SubmitResponse (200) or an ErrorEnvelope (400/502).
func submitHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Cap body size before decoding to prevent large-body DoS (1 MB limit).
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		// Decode request body.
		var req SubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: malformed JSON or body too large")
			return
		}

		// Extract session.
		sess, ok := auth.FromContext(ctx)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing session context")
			return
		}

		// Convert TurnMessages to llm.Messages for classify.
		llmMsgs := make([]llm.Message, 0, len(req.Messages))
		for _, m := range req.Messages {
			llmMsgs = append(llmMsgs, llm.Message{
				Role:    m.Role,
				Content: m.Content,
			})
		}

		// Classify.
		classifyResult, err := deps.Classifier.Classify(ctx, llmMsgs)
		if err != nil {
			// Non-nil error means safe defaults were used (graceful degradation).
			// Log but do NOT fail the request — the safe defaults produce a valid payload.
			slog.WarnContext(ctx, "classify degraded to safe defaults", "err", err)
		}

		// Assemble and validate the canonical payload.
		submissionID := payloadbuild.NewSubmissionID()
		submittedAt := time.Now().UTC()

		p, err := deps.Builder.Build(ctx, &req, classifyResult, sess, submissionID, submittedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "payload_invalid", fmt.Sprintf("payload validation failed: %v", err))
			return
		}

		// Dispatch to adapter.
		result, err := deps.Adapter.Create(ctx, p)
		if err != nil {
			// Log full detail server-side (may include URLs/responses); client gets opaque message.
			slog.ErrorContext(ctx, "adapter create failed", "adapter", deps.Adapter.Name(), "error", err)
			writeError(w, http.StatusBadGateway, "adapter_error", "downstream adapter unavailable")
			return
		}

		// Success.
		writeJSON(w, http.StatusOK, SubmitResponse{
			ExternalID:  result.ExternalID,
			ExternalURL: result.ExternalURL,
			AdapterName: result.AdapterName,
			CreatedAt:   result.CreatedAt,
		})
	}
}
