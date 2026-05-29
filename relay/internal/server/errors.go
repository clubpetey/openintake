package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

// writeJSON serialises v as JSON and writes it to w with the given HTTP status.
// On marshal failure it falls back to a JSON 500 (Content-Type: application/json).
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"code":"internal","message":"response encoding failed"}}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// writeError writes an ErrorEnvelope JSON response.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, ErrorEnvelope{
		Error: ErrorBody{Code: code, Message: msg},
	})
}

// setRetryAfter writes the Retry-After header in HTTP numeric form (seconds),
// flooring sub-second durations at 1 (RFC 9110 §10.2.3 — Retry-After in
// integer seconds; never advertise 0). Used by every 429 / 503 path in the
// server.
func setRetryAfter(w http.ResponseWriter, d time.Duration) {
	secs := int(d.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
}
