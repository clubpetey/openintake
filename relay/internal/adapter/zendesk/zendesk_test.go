package zendesk_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/adapter/zendesk"
	"intake/internal/payload"
)

const (
	testEmail = "agent@example.com"
	testToken = "super-secret-zendesk-token-abc123"
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
			Messages: []payload.Message{
				{Role: payload.MessageRoleUser, Content: "It crashes on save", Ts: now},
				{Role: payload.MessageRoleAssistant, Content: "Which version?", Ts: now},
			},
			Summary:         "User reports a crash when saving.",
			TitleSuggestion: "Crash on save",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{"crash", "save"},
			Language:        "en",
		},
	}
}

// configured returns a zendesk adapter pointed at the given test server URL.
func configured(t *testing.T, baseURL string) *zendesk.Adapter {
	t.Helper()
	a := zendesk.New()
	if err := a.Configure(map[string]any{
		"subdomain":        "example",
		"email":            testEmail,
		"api_token":        testToken,
		"default_priority": "normal",
		"base_url":         baseURL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestZendeskCreate_HappyPath asserts the POST target, method, basic-auth header,
// and the mapped ticket body, then verifies the parsed CreateResult.
func TestZendeskCreate_HappyPath(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ticket":{"id":555,"url":"https://example.zendesk.com/api/v2/tickets/555.json"}}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s; want POST", gotMethod)
	}
	if gotPath != "/api/v2/tickets.json" {
		t.Errorf("path = %s; want /api/v2/tickets.json", gotPath)
	}

	// Authorization must be "Basic <base64(email/token:token)>".
	const prefix = "Basic "
	if !strings.HasPrefix(gotAuth, prefix) {
		t.Fatalf("Authorization = %q; want %q prefix", gotAuth, prefix)
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(gotAuth, prefix))
	if err != nil {
		t.Fatalf("decode auth: %v", err)
	}
	wantCreds := testEmail + "/token:" + testToken
	if string(dec) != wantCreds {
		t.Errorf("decoded auth = %q; want %q", dec, wantCreds)
	}

	// Body shape: {"ticket":{"subject","comment":{"body"},"priority","tags"}}.
	var parsed struct {
		Ticket struct {
			Subject string `json:"subject"`
			Comment struct {
				Body string `json:"body"`
			} `json:"comment"`
			Priority string   `json:"priority"`
			Tags     []string `json:"tags"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("body not valid JSON: %v\nbody: %s", err, gotBody)
	}
	if parsed.Ticket.Subject != "Crash on save" {
		t.Errorf("subject = %q; want %q", parsed.Ticket.Subject, "Crash on save")
	}
	if !strings.Contains(parsed.Ticket.Comment.Body, "User reports a crash when saving.") {
		t.Errorf("comment.body missing summary; got %q", parsed.Ticket.Comment.Body)
	}
	if !strings.Contains(parsed.Ticket.Comment.Body, "user: It crashes on save") {
		t.Errorf("comment.body missing transcript line; got %q", parsed.Ticket.Comment.Body)
	}
	if parsed.Ticket.Priority != "high" { // high severity → "high"
		t.Errorf("priority = %q; want high", parsed.Ticket.Priority)
	}
	if len(parsed.Ticket.Tags) != 2 || parsed.Ticket.Tags[0] != "crash" || parsed.Ticket.Tags[1] != "save" {
		t.Errorf("tags = %v; want [crash save]", parsed.Ticket.Tags)
	}

	// CreateResult: ExternalID stringified id; ExternalURL is the agent UI url.
	if result.ExternalID != "555" {
		t.Errorf("ExternalID = %q; want 555", result.ExternalID)
	}
	if !strings.HasSuffix(result.ExternalURL, "/agent/tickets/555") {
		t.Errorf("ExternalURL = %q; want suffix /agent/tickets/555", result.ExternalURL)
	}
	if result.AdapterName != "zendesk" {
		t.Errorf("AdapterName = %q; want zendesk", result.AdapterName)
	}
	if result.CreatedAt == "" {
		t.Error("CreatedAt should be non-empty (RFC3339)")
	}
	if _, err := time.Parse(time.RFC3339, result.CreatedAt); err != nil {
		t.Errorf("CreatedAt %q not RFC3339: %v", result.CreatedAt, err)
	}
}

// TestZendeskMapPriority covers the severity → Zendesk priority mapping table.
func TestZendeskMapPriority(t *testing.T) {
	cases := []struct {
		name     string
		severity payload.ConversationSeverityGuess
		want     string
	}{
		{"low", payload.ConversationSeverityGuessLow, "low"},
		{"medium", payload.ConversationSeverityGuessMedium, "normal"},
		{"high", payload.ConversationSeverityGuessHigh, "high"},
		{"critical", payload.ConversationSeverityGuessCritical, "urgent"},
		{"unknown_uses_default", payload.ConversationSeverityGuessUnknown, "normal"},
		{"empty_uses_default", payload.ConversationSeverityGuess(""), "normal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPriority string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var parsed struct {
					Ticket struct {
						Priority string `json:"priority"`
					} `json:"ticket"`
				}
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &parsed)
				gotPriority = parsed.Ticket.Priority
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"ticket":{"id":1,"url":"x"}}`))
			}))
			defer srv.Close()

			a := configured(t, srv.URL) // default_priority "normal"
			p := minimalPayload()
			p.Conversation.SeverityGuess = tc.severity
			if _, err := a.Create(context.Background(), p); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if gotPriority != tc.want {
				t.Errorf("severity %q → priority %q; want %q", tc.severity, gotPriority, tc.want)
			}
		})
	}
}

// TestZendeskCreate_Non2xx asserts a 422 produces an error WITHOUT leaking the token.
func TestZendeskCreate_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"RecordInvalid","description":"Subject: cannot be blank"}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on 422, got nil")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("error should mention status 422; got %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("SECURITY: error leaks the api token: %v", err)
	}
}

// TestZendeskConfigure_MissingKeys asserts each required key is validated.
func TestZendeskConfigure_MissingKeys(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{
			"subdomain": "example",
			"email":     testEmail,
			"api_token": testToken,
		}
	}
	cases := []struct {
		name string
		drop string
	}{
		{"missing subdomain", "subdomain"},
		{"missing email", "email"},
		{"missing api_token", "api_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			delete(cfg, tc.drop)
			a := zendesk.New()
			err := a.Configure(cfg)
			if err == nil {
				t.Fatalf("expected error when %q is missing", tc.drop)
			}
			if !strings.Contains(err.Error(), tc.drop) {
				t.Errorf("error should name the missing key %q; got %v", tc.drop, err)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Fatalf("SECURITY: Configure error leaks the api token: %v", err)
			}
		})
	}
}

// TestZendeskRequiresLicense asserts zendesk is a paid adapter.
func TestZendeskRequiresLicense(t *testing.T) {
	if !zendesk.New().RequiresLicense() {
		t.Error("zendesk.RequiresLicense() = false; want true (paid adapter)")
	}
}

// TestZendeskCreate_TokenNeverLeaks drives Create against a server that always
// 500s and asserts the token never appears in the returned error string.
func TestZendeskCreate_TokenNeverLeaks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo the auth header back in the body — the adapter must NOT surface it.
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error: " + r.Header.Get("Authorization")))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("SECURITY: error leaks the api token: %v", err)
	}
	// The base64-encoded credentials must not leak either.
	encoded := base64.StdEncoding.EncodeToString([]byte(testEmail + "/token:" + testToken))
	if strings.Contains(err.Error(), encoded) {
		t.Fatalf("SECURITY: error leaks the base64 basic-auth credentials: %v", err)
	}
}
