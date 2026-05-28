package chatwoot_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/adapter/chatwoot"
	"intake/internal/payload"
)

const testToken = "super-secret-chatwoot-token-xyz"

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
			Messages: []payload.Message{
				{Role: payload.MessageRoleUser, Content: "The save button does nothing", Ts: now},
				{Role: payload.MessageRoleAssistant, Content: "Thanks, can you share the page URL?", Ts: now},
			},
			Summary:         "Save button is unresponsive on the settings page.",
			TitleSuggestion: "Save button does nothing",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{"ui", "settings"},
			Language:        "en",
		},
	}
}

// configure builds an adapter pointed at the given base URL with the test token.
func configure(t *testing.T, baseURL string) *chatwoot.Adapter {
	t.Helper()
	a := chatwoot.New()
	if err := a.Configure(map[string]any{
		"base_url":   baseURL,
		"account_id": 1,
		"inbox_id":   3,
		"api_token":  testToken,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestChatwootCreate_PostsConversation asserts the adapter POSTs to the right
// path with the api_access_token header and a body carrying the mapped content
// and inbox_id, and that the response id becomes ExternalID/ExternalURL.
func TestChatwootCreate_PostsConversation(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotCT string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("api_access_token")
		gotCT = r.Header.Get("Content-Type")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("request body not valid JSON: %v\nbody: %s", err, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":123,"inbox_id":3}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/v1/accounts/1/conversations" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if gotAuth != testToken {
		t.Errorf("expected api_access_token header = token, got %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", gotCT)
	}

	// inbox_id is mapped (JSON numbers decode to float64).
	if iv, ok := gotBody["inbox_id"].(float64); !ok || int(iv) != 3 {
		t.Errorf("expected inbox_id=3 in body, got %v", gotBody["inbox_id"])
	}
	// source_id is the submission id.
	if sid, _ := gotBody["source_id"].(string); sid != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected source_id=submission id, got %v", gotBody["source_id"])
	}
	// message.content carries the rendered transcript (title + summary + messages).
	msg, ok := gotBody["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected message object in body, got %v", gotBody["message"])
	}
	content, _ := msg["content"].(string)
	for _, want := range []string{"Save button does nothing", "Save button is unresponsive", "user: The save button does nothing", "assistant: Thanks, can you share"} {
		if !strings.Contains(content, want) {
			t.Errorf("message.content missing %q\ncontent: %s", want, content)
		}
	}

	if result.ExternalID != "123" {
		t.Errorf("expected ExternalID 123, got %q", result.ExternalID)
	}
	wantURL := srv.URL + "/app/accounts/1/conversations/123"
	if result.ExternalURL != wantURL {
		t.Errorf("expected ExternalURL %q, got %q", wantURL, result.ExternalURL)
	}
	if result.AdapterName != "chatwoot" {
		t.Errorf("expected AdapterName chatwoot, got %q", result.AdapterName)
	}
	if result.CreatedAt == "" {
		t.Error("CreatedAt should be non-empty")
	}
}

// TestChatwootCreate_NonOKErrorNoToken asserts a non-2xx response returns an
// error that includes the (truncated) body but NEVER the token.
func TestChatwootCreate_NonOKErrorNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Access denied"}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention the status code, got %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("SECURITY: token leaked into error: %v", err)
	}
}

// TestChatwootConfigure_MissingKeys asserts required-key validation.
func TestChatwootConfigure_MissingKeys(t *testing.T) {
	t.Run("missing base_url", func(t *testing.T) {
		a := chatwoot.New()
		err := a.Configure(map[string]any{
			"account_id": 1,
			"inbox_id":   3,
			"api_token":  testToken,
		})
		if err == nil || !strings.Contains(err.Error(), "base_url") {
			t.Fatalf("expected error naming base_url, got %v", err)
		}
	})
	t.Run("missing api_token", func(t *testing.T) {
		a := chatwoot.New()
		err := a.Configure(map[string]any{
			"base_url":   "https://chatwoot.example.com",
			"account_id": 1,
			"inbox_id":   3,
		})
		if err == nil || !strings.Contains(err.Error(), "api_token") {
			t.Fatalf("expected error naming api_token, got %v", err)
		}
	})
}

// TestChatwootConfigure_AcceptsFloatIDs asserts account_id/inbox_id accept the
// float64 form that a JSON/YAML decode may produce (mirrors webhook's retry ints).
func TestChatwootConfigure_AcceptsFloatIDs(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":7}`))
	}))
	defer srv.Close()

	a := chatwoot.New()
	if err := a.Configure(map[string]any{
		"base_url":   srv.URL,
		"account_id": float64(9), // as a JSON/YAML decode might supply it
		"inbox_id":   float64(4),
		"api_token":  testToken,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotPath != "/api/v1/accounts/9/conversations" {
		t.Errorf("float account_id not applied: path %q", gotPath)
	}
	if iv, ok := gotBody["inbox_id"].(float64); !ok || int(iv) != 4 {
		t.Errorf("float inbox_id not applied: %v", gotBody["inbox_id"])
	}
}
