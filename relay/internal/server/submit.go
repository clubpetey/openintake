package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/clubpetey/openintake/relay/internal/attachvalidate"
	"github.com/clubpetey/openintake/relay/internal/auth"
	"github.com/clubpetey/openintake/relay/internal/llm"
	"github.com/clubpetey/openintake/relay/internal/payloadbuild"
)

// submitHandler handles POST /v1/intake/submit.
// It:
//  1. Caps body size to deps.BodyCapBytes (14 MB when attachments enabled, 1 MB otherwise).
//  2. Decodes the SubmitRequest body. *http.MaxBytesError → 413 request_body_too_large;
//     other decode errors → 400 bad_request.
//  3. Refuses non-empty attachments[] with 400 attachments_disabled when
//     cfg.Attachments.Enabled=false.
//  4. Extracts the SessionContext from context (placed by auth middleware).
//  5. Builds []llm.Message and calls Classifier.Classify.
//  6. Calls Builder.Build to assemble + validate the canonical payload (which now
//     additively carries Attachments).
//  7. NEW: calls attachvalidate.ValidateAll on p.Attachments. Sentinel errors
//     map to 413/415/400 per the documented error codes (README §8.8).
//  8. Calls Router.Route + adapter.Create only after attachvalidate passes.
//  9. Returns a SubmitResponse (200) or an ErrorEnvelope.
func submitHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 1. Body cap (Phase 6: from Deps; Phase 1 used a literal 1<<20).
		r.Body = http.MaxBytesReader(w, r.Body, deps.BodyCapBytes)

		// 2. Decode request body.
		var req SubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "request_body_too_large", "submission body exceeds limit")
				return
			}
			writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: malformed JSON")
			return
		}

		// 3. Attachments-disabled short-circuit. Runs BEFORE attachvalidate so the
		//    operator's intent (Enabled=false OR no adapter accepts anything)
		//    surfaces a clear error even if the bytes would have otherwise passed.
		if len(req.Attachments) > 0 && (!deps.AttachmentsCfg.Enabled || len(deps.AttachmentMIMEs) == 0) {
			writeError(w, http.StatusBadRequest, "attachments_disabled", "attachments are disabled on this relay")
			return
		}

		// 4. Extract session.
		sess, ok := auth.FromContext(ctx)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing session context")
			return
		}

		// 5. Classify (Phase 1+4+5 unchanged).
		llmMsgs := make([]llm.Message, 0, len(req.Messages))
		for _, m := range req.Messages {
			llmMsgs = append(llmMsgs, llm.Message{Role: m.Role, Content: m.Content})
		}
		classifyResult, err := deps.Classifier.Classify(ctx, llmMsgs)
		if err != nil {
			slog.WarnContext(ctx, "classify degraded to safe defaults", "err", err)
		}

		// 6. Build canonical payload (now additively carries Attachments).
		submissionID := payloadbuild.NewSubmissionID()
		submittedAt := time.Now().UTC()
		p, err := deps.Builder.Build(ctx, &req, classifyResult, sess, submissionID, submittedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "payload_invalid", fmt.Sprintf("payload validation failed: %v", err))
			return
		}

		// 7. NEW (Phase 6): validate attachments after Build, before Route. Sentinel
		//    errors map to specific HTTP status codes per README §8.8.
		if len(p.Attachments) > 0 {
			validatorCfg := attachvalidate.Config{
				MaxSizeBytes:     deps.AttachmentsCfg.MaxSizeBytes,
				MaxTotalBytes:    deps.AttachmentsCfg.MaxTotalBytes,
				AllowedMIMETypes: deps.AttachmentMIMEs,
			}
			if _, vErr := attachvalidate.ValidateAll(p.Attachments, validatorCfg); vErr != nil {
				status, code, msg := mapAttachvalidateError(vErr)
				writeError(w, status, code, msg)
				return
			}
		}

		// 8. Route + create.
		ad, err := deps.Router.Route(p)
		if err != nil {
			slog.ErrorContext(ctx, "router: no adapter resolved", "error", err)
			writeError(w, http.StatusBadGateway, "adapter_error", "no adapter available")
			return
		}
		result, err := ad.Create(ctx, p)
		if err != nil {
			slog.ErrorContext(ctx, "adapter create failed", "adapter", ad.Name(), "error", err)
			// Phase 7 (7-i): record the adapter failure. Nil-safe.
			deps.Metrics.RecordAdapterCall(ad.Name(), "error")
			writeError(w, http.StatusBadGateway, "adapter_error", "downstream adapter unavailable")
			return
		}
		// Phase 7 (7-i): record the adapter success. Nil-safe.
		deps.Metrics.RecordAdapterCall(ad.Name(), "success")

		// 9. Success.
		writeJSON(w, http.StatusOK, SubmitResponse{
			ExternalID:  result.ExternalID,
			ExternalURL: result.ExternalURL,
			AdapterName: result.AdapterName,
			CreatedAt:   result.CreatedAt,
		})
	}
}

// mapAttachvalidateError maps the six sentinel errors to (status, code, message).
// The message is the client-facing string; the server log has already captured
// the underlying sentinel via slog at the call site if needed (none in 6-i).
func mapAttachvalidateError(err error) (status int, code string, msg string) {
	switch {
	case errors.Is(err, attachvalidate.ErrAttachmentTooLarge):
		return http.StatusRequestEntityTooLarge, "attachment_too_large", "attachment exceeds max_size_bytes"
	case errors.Is(err, attachvalidate.ErrAggregateTooLarge):
		return http.StatusRequestEntityTooLarge, "attachments_exceed_total", "attachments exceed total cap"
	case errors.Is(err, attachvalidate.ErrMIMENotAllowed):
		return http.StatusUnsupportedMediaType, "attachment_mime_not_allowed", "attachment mime_type not allowed"
	case errors.Is(err, attachvalidate.ErrMIMEMismatch):
		return http.StatusUnsupportedMediaType, "attachment_mime_mismatch", "attachment bytes do not match declared mime_type"
	case errors.Is(err, attachvalidate.ErrBadDataURL):
		return http.StatusBadRequest, "attachment_malformed", "attachment url is not a valid data: URL"
	case errors.Is(err, attachvalidate.ErrAttachmentTypeUnsupported):
		return http.StatusBadRequest, "attachment_type_unsupported", "attachment type unsupported in v0"
	default:
		// Should not happen — attachvalidate only returns the six sentinels.
		// Defensive: surface as 400 so we don't accidentally 500.
		return http.StatusBadRequest, "bad_request", "attachment validation failed"
	}
}
