package server

import (
	"encoding/json"
	"net/http"
)

// writeJSON serialises v as JSON and writes it to w with the given HTTP status.
// On marshal failure it falls back to a plain-text 500.
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":{"code":"internal","message":"response encoding failed"}}`, http.StatusInternalServerError)
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

// WriteErrorExported is a thin exported wrapper so package-external tests can
// exercise writeError without needing to go through a full HTTP handler. It is
// NOT part of the production API surface; production code calls writeError.
func WriteErrorExported(w http.ResponseWriter, status int, code, msg string) {
	writeError(w, status, code, msg)
}
