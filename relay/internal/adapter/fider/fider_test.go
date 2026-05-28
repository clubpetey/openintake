package fider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/adapter/fider"
	"intake/internal/payload"
)

// minimalPayload returns a schema-satisfying IntakePayload with a title, summary,
// and a two-message transcript for body rendering.
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
				{Role: payload.MessageRoleUser, Content: "The export button does nothing.", Ts: now},
				{Role: payload.MessageRoleAssistant, Content: "Thanks, which browser?", Ts: now},
			},
			Summary:         "Export button is unresponsive on the reports page.",
			TitleSuggestion: "Export button does nothing",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{},
			Language:        "en",
		},
	}
}

const testKey = "fdr_supersecret_key_value"

// configure builds and configures an adapter pointed at the given base URL.
func configure(t *testing.T, baseURL string) *fider.Adapter {
	t.Helper()
	a := fider.New()
	if err := a.Configure(map[string]any{
		"base_url": baseURL,
		"api_key":  testKey,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestFiderCreate_PostsPostWithBearer asserts the adapter POSTs {title,description}
// to /api/v1/posts with the Bearer header, and maps the {id,number} response.
func TestFiderCreate_PostsPostWithBearer(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
		gotAuth   string
		gotCT     string
		gotBody   []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":7,"number":42}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q; want POST", gotMethod)
	}
	if gotPath != "/api/v1/posts" {
		t.Errorf("path = %q; want /api/v1/posts", gotPath)
	}
	if gotAuth != "Bearer "+testKey {
		t.Errorf("Authorization = %q; want Bearer <key>", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", gotCT)
	}

	var sent struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("request body not valid JSON: %v\nbody: %s", err, gotBody)
	}
	if sent.Title != "Export button does nothing" {
		t.Errorf("title = %q; want title_suggestion", sent.Title)
	}
	if !strings.Contains(sent.Description, "Export button is unresponsive on the reports page.") {
		t.Errorf("description missing summary; got %q", sent.Description)
	}
	if !strings.Contains(sent.Description, "user: The export button does nothing.") {
		t.Errorf("description missing user message line; got %q", sent.Description)
	}
	if !strings.Contains(sent.Description, "assistant: Thanks, which browser?") {
		t.Errorf("description missing assistant message line; got %q", sent.Description)
	}

	if result.ExternalID != "7" {
		t.Errorf("ExternalID = %q; want 7", result.ExternalID)
	}
	if !strings.HasSuffix(result.ExternalURL, "/posts/42") {
		t.Errorf("ExternalURL = %q; want suffix /posts/42", result.ExternalURL)
	}
	if result.AdapterName != "fider" {
		t.Errorf("AdapterName = %q; want fider", result.AdapterName)
	}
	if result.CreatedAt == "" {
		t.Error("CreatedAt should be a non-empty RFC3339 timestamp")
	}
	if _, perr := time.Parse(time.RFC3339, result.CreatedAt); perr != nil {
		t.Errorf("CreatedAt = %q not RFC3339: %v", result.CreatedAt, perr)
	}
}

// TestFiderCreate_TitleFallsBackToSummary asserts an empty title_suggestion falls
// back to a truncated summary.
func TestFiderCreate_TitleFallsBackToSummary(t *testing.T) {
	var gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var sent struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(body, &sent)
		gotTitle = sent.Title
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"number":1}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	p := minimalPayload()
	p.Conversation.TitleSuggestion = ""
	p.Conversation.Summary = "A short summary used as the title."
	if _, err := a.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotTitle != "A short summary used as the title." {
		t.Errorf("title fallback = %q; want the summary", gotTitle)
	}
}

// TestFiderCreate_NonSuccessErrorRedactsKey asserts a non-2xx response yields an
// error containing the truncated body but NOT the api_key.
func TestFiderCreate_NonSuccessErrorRedactsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["forbidden"]}`))
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected an error on 403")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error string leaked the api_key: %v", err)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention the status code; got %v", err)
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("error should include the (truncated) body; got %v", err)
	}
}

// TestFiderConfigure_MissingKeysError asserts both required keys are validated.
func TestFiderConfigure_MissingKeysError(t *testing.T) {
	t.Run("missing base_url", func(t *testing.T) {
		a := fider.New()
		err := a.Configure(map[string]any{"api_key": testKey})
		if err == nil || !strings.Contains(err.Error(), "base_url") {
			t.Errorf("expected base_url error; got %v", err)
		}
	})
	t.Run("empty base_url", func(t *testing.T) {
		a := fider.New()
		err := a.Configure(map[string]any{"base_url": "", "api_key": testKey})
		if err == nil || !strings.Contains(err.Error(), "base_url") {
			t.Errorf("expected base_url error; got %v", err)
		}
	})
	t.Run("missing api_key", func(t *testing.T) {
		a := fider.New()
		err := a.Configure(map[string]any{"base_url": "https://feedback.example.com"})
		if err == nil || !strings.Contains(err.Error(), "api_key") {
			t.Errorf("expected api_key error; got %v", err)
		}
	})
	t.Run("empty api_key", func(t *testing.T) {
		a := fider.New()
		err := a.Configure(map[string]any{"base_url": "https://feedback.example.com", "api_key": ""})
		if err == nil || !strings.Contains(err.Error(), "api_key") {
			t.Errorf("expected api_key error; got %v", err)
		}
	})
}

// TestFiderConfigure_ErrorNeverLeaksKey asserts a Configure validation error never
// echoes the key value.
func TestFiderConfigure_ErrorNeverLeaksKey(t *testing.T) {
	a := fider.New()
	// base_url missing but api_key present — the error must not contain the key.
	err := a.Configure(map[string]any{"api_key": testKey})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("Configure error leaked the api_key: %v", err)
	}
}

// TestFiderHealthCheck_OK asserts a non-5xx response (with the Bearer header set)
// is healthy.
func TestFiderHealthCheck_OK(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	if err := a.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if gotAuth != "Bearer "+testKey {
		t.Errorf("health Authorization = %q; want Bearer <key>", gotAuth)
	}
}
