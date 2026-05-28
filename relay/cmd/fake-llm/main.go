// fake-llm is a tiny standalone HTTP server that mimics the Ollama /api/chat
// NDJSON streaming shape so the Phase 5 abuse-control smokes can exercise
// /turn without spending any LLM credit. Configurable input/output token
// counts via flags.
//
// Usage:
//
//	go run ./cmd/fake-llm --addr :11434 --input-tokens 50 --output-tokens 25
//
// The relay points at it via:
//
//	llm:
//	  provider: "ollama"
//	  ollama:
//	    base_url: "http://127.0.0.1:11434"
//	    model: "fake"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":11434", "listen address")
	inputTokens := flag.Int("input-tokens", 50, "input tokens to report in the final NDJSON chunk")
	outputTokens := flag.Int("output-tokens", 25, "output tokens to report in the final NDJSON chunk")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		// Drain the request body so the relay doesn't block.
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)

		// Emit a single content chunk, then a done chunk with the token counts.
		// Shape mirrors Ollama's NDJSON streaming response.
		fmt.Fprintf(w, `{"model":"fake","message":{"role":"assistant","content":"ok"},"done":false}`+"\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		fmt.Fprintf(w, `{"model":"fake","done":true,"prompt_eval_count":%d,"eval_count":%d}`+"\n", *inputTokens, *outputTokens)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	})

	log.Printf("fake-llm: listening on %s (input_tokens=%d, output_tokens=%d)", *addr, *inputTokens, *outputTokens)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("fake-llm: listen: %v", err)
	}
}
