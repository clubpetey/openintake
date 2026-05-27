package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"intake/internal/llm"
)

// initHandler handles POST /v1/intake/init.
// No auth middleware: this endpoint ISSUES the session.
//
// Response: InitResponse{SessionID, Capabilities{AuthModes:["anonymous"], Streaming:true}}
func initHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil {
			writeError(w, http.StatusInternalServerError, "internal", "auth not configured")
			return
		}
		sessionID := deps.Auth.Store().Issue()

		resp := InitResponse{
			SessionID: sessionID,
			Capabilities: Capabilities{
				AuthModes: []string{"anonymous"},
				Streaming: true,
			},
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// turnHandler handles POST /v1/intake/turn (behind the auth middleware).
//
// The handler:
//  1. Decodes TurnRequest from the body.
//  2. Prepends the triage system prompt as a system message.
//  3. Calls deps.Provider.Chat with Stream:true.
//  4. Writes SSE frames: SSEDelta per chunk, SSEDone on completion, SSEError on failure.
//  5. Respects ctx.Done() (client disconnect).
func turnHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req TurnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: "+err.Error())
			return
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
