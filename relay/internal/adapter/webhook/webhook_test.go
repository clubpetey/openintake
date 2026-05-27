package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"intake/internal/adapter/webhook"
	"intake/internal/payload"
)

// minimalPayload returns a schema-satisfying IntakePayload for testing.
func minimalPayload() *payload.IntakePayload {
	now := time.Now().UTC()
	return &payload.IntakePayload{
		SchemaVersion: "1.0",
		Submission: payload.Submission{
			Id:          "00000000-0000-0000-0000-000000000001",
			SubmittedAt: now,
		},
		Client: payload.Client{
			WidgetVersion: "0.1.0",
			SessionId:     "00000000-0000-0000-0000-000000000002",
			Url:           "http://localhost:5173/",
			UserAgent:     "test-agent",
			Viewport:      payload.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		User: payload.User{
			AuthMode: payload.UserAuthModeAnonymous,
			Verified: false,
		},
		Conversation: payload.Conversation{
			Messages:        []payload.Message{},
			Summary:         "test summary",
			TitleSuggestion: "Test Title",
			Classification:  payload.ConversationClassificationOther,
			SeverityGuess:   payload.ConversationSeverityGuessUnknown,
			TagsSuggested:   []string{},
			Language:        "en",
		},
	}
}

// TestWebhookCreate_ReceivesPayload asserts the adapter POSTs valid JSON to the receiver.
func TestWebhookCreate_ReceivesPayload(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		var err error
		received, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"external_id":"ext-abc-123"}`))
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{"url": srv.URL}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	p := minimalPayload()
	result, err := a.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if result.ExternalID != "ext-abc-123" {
		t.Errorf("expected ExternalID ext-abc-123, got %q", result.ExternalID)
	}
	if result.AdapterName != "webhook" {
		t.Errorf("expected AdapterName webhook, got %q", result.AdapterName)
	}

	// Assert the POSTed body is valid JSON containing schema_version.
	var parsed map[string]any
	if err := json.Unmarshal(received, &parsed); err != nil {
		t.Fatalf("receiver body not valid JSON: %v\nbody: %s", err, received)
	}
	if parsed["schema_version"] != "1.0" {
		t.Errorf("expected schema_version=1.0 in payload, got %v", parsed["schema_version"])
	}
}

// TestWebhookCreate_RetriesOn503 asserts the adapter retries once on 503 then succeeds on 200.
func TestWebhookCreate_RetriesOn503(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{
		"url": srv.URL,
		"retry": map[string]any{
			"max_attempts": 3,
			"backoff":      "fixed",
		},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create after retry: %v", err)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", callCount.Load())
	}
	if result.ExternalID == "" {
		t.Error("ExternalID should be non-empty (generated UUID on blank response)")
	}
}

// TestWebhookCreate_ExhaustsRetries asserts failure after max_attempts all return 503.
func TestWebhookCreate_ExhaustsRetries(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{
		"url": srv.URL,
		"retry": map[string]any{
			"max_attempts": 2,
			"backoff":      "fixed",
		},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if callCount.Load() != 2 {
		t.Errorf("expected exactly 2 calls, got %d", callCount.Load())
	}
}

// TestWebhookCreate_CtxCancelDuringBackoff asserts that a context cancellation
// during the backoff sleep causes Create to return promptly with an error,
// rather than waiting out all remaining retry attempts.
func TestWebhookCreate_CtxCancelDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 503 to force retries and trigger the backoff sleep.
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{
		"url": srv.URL,
		"retry": map[string]any{
			// Fixed backoff of 200ms per attempt; 5 attempts = 1s total if not cancelled.
			// The context deadline of 80ms ensures cancellation happens during backoff.
			"max_attempts": 5,
			"backoff":      "fixed",
		},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Deadline shorter than one full backoff interval (200ms) so the ctx fires mid-sleep.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := a.Create(ctx, minimalPayload())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
	// Must return well before exhausting all attempts (5 * 200ms = 1s).
	// Allow up to 600ms to account for the first attempt's network round-trip.
	if elapsed > 600*time.Millisecond {
		t.Errorf("Create took %v — expected early exit on ctx cancel, not full retry exhaustion", elapsed)
	}
	t.Logf("Create returned in %v with error: %v", elapsed, err)
}

// TestWebhookCreate_CustomHeaders asserts configured headers are sent.
func TestWebhookCreate_CustomHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Intake-Token") != "secret" {
			t.Errorf("expected X-Intake-Token: secret")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := webhook.New()
	if err := a.Configure(map[string]any{
		"url":     srv.URL,
		"headers": map[string]any{"X-Intake-Token": "secret"},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
}
