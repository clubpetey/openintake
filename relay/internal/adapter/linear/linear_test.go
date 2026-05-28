package linear_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intake/internal/adapter/linear"
	"intake/internal/payload"
)

const testAPIKey = "lin_api_SUPERSECRETKEY_should_never_leak"

// minimalPayload returns a schema-satisfying IntakePayload with a title, summary,
// and a 2-message transcript for description rendering.
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
				{Role: payload.MessageRoleUser, Content: "The save button does nothing.", Ts: now},
				{Role: payload.MessageRoleAssistant, Content: "Thanks, I'll file that.", Ts: now},
			},
			Summary:         "User reports the save button is unresponsive.",
			TitleSuggestion: "Save button unresponsive",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{},
			Language:        "en",
		},
	}
}

// configured builds a linear adapter pointed at srv with the test team/key.
func configured(t *testing.T, srvURL string) *linear.Adapter {
	t.Helper()
	a := linear.New()
	if err := a.Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  "team-uuid-123",
		"endpoint": srvURL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// TestLinearCreate_HappyPath asserts the raw Authorization header, the GraphQL
// mutation, and the variables.input mapping, and that the response is parsed.
func TestLinearCreate_HappyPath(t *testing.T) {
	var gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		gotAuth = r.Header.Get("Authorization")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":{"id":"abc-123","identifier":"ENG-42","url":"https://linear.app/x/issue/ENG-42"}}}}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	result, err := a.Create(context.Background(), minimalPayload())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Auth header is the RAW api key — no "Bearer " prefix.
	if gotAuth != testAPIKey {
		t.Errorf("Authorization = %q; want raw api key %q", gotAuth, testAPIKey)
	}

	// Result mapping: ExternalID = issue.id; ExternalURL = issue.url.
	if result.ExternalID != "abc-123" {
		t.Errorf("ExternalID = %q; want abc-123", result.ExternalID)
	}
	if result.ExternalURL != "https://linear.app/x/issue/ENG-42" {
		t.Errorf("ExternalURL = %q; want the linear url", result.ExternalURL)
	}
	if result.AdapterName != "linear" {
		t.Errorf("AdapterName = %q; want linear", result.AdapterName)
	}
	if result.CreatedAt == "" {
		t.Error("CreatedAt should be a non-empty RFC3339 timestamp")
	}

	// The POSTed body must be a GraphQL request: a query holding the mutation,
	// and variables.input.{teamId,title,description} correctly mapped.
	var req struct {
		Query     string `json:"query"`
		Variables struct {
			Input struct {
				TeamID      string `json:"teamId"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"input"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("posted body not valid JSON: %v\nbody: %s", err, gotBody)
	}
	if !strings.Contains(req.Query, "issueCreate") || !strings.Contains(req.Query, "IssueCreateInput") {
		t.Errorf("query missing issueCreate mutation: %q", req.Query)
	}
	if req.Variables.Input.TeamID != "team-uuid-123" {
		t.Errorf("input.teamId = %q; want team-uuid-123", req.Variables.Input.TeamID)
	}
	if req.Variables.Input.Title != "Save button unresponsive" {
		t.Errorf("input.title = %q; want the title_suggestion", req.Variables.Input.Title)
	}
	if !strings.Contains(req.Variables.Input.Description, "User reports the save button is unresponsive.") {
		t.Errorf("input.description missing summary: %q", req.Variables.Input.Description)
	}
	if !strings.Contains(req.Variables.Input.Description, "user: The save button does nothing.") {
		t.Errorf("input.description missing transcript line: %q", req.Variables.Input.Description)
	}
}

// TestLinearCreate_GraphQLErrors asserts HTTP 200 with a non-empty errors array
// is treated as a failure, and the api key never appears in the error.
func TestLinearCreate_GraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // GraphQL returns 200 even on logical errors.
		_, _ = w.Write([]byte(`{"errors":[{"message":"bad team"}]}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on a GraphQL errors response")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked in error: %v", err)
	}
	if !strings.Contains(err.Error(), "bad team") {
		t.Errorf("error should surface the GraphQL message; got %v", err)
	}
}

// TestLinearCreate_SuccessFalse asserts success:false is treated as a failure.
func TestLinearCreate_SuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":false,"issue":null}}}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err == nil {
		t.Fatal("expected error when issueCreate.success is false")
	}
}

// TestLinearCreate_Non2xx asserts a non-2xx HTTP status is an error without the key.
func TestLinearCreate_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"authentication required"}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on HTTP 401")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked in error: %v", err)
	}
}

// TestLinearConfigure_Validation asserts missing api_key / team_id error clearly.
func TestLinearConfigure_Validation(t *testing.T) {
	if err := linear.New().Configure(map[string]any{"team_id": "t"}); err == nil {
		t.Error("expected error when api_key is missing")
	}
	if err := linear.New().Configure(map[string]any{"api_key": "k"}); err == nil {
		t.Error("expected error when team_id is missing")
	}
	if err := linear.New().Configure(map[string]any{"api_key": "", "team_id": "t"}); err == nil {
		t.Error("expected error when api_key is empty")
	}
}

// TestLinearRequiresLicense asserts the paid marker.
func TestLinearRequiresLicense(t *testing.T) {
	if !linear.New().RequiresLicense() {
		t.Error("linear is a paid adapter; RequiresLicense() must be true")
	}
}

// TestLinearCreate_KeyNeverLeaks is the explicit redaction assertion: across the
// GraphQL-error and non-2xx failure paths, the api key must never appear in the
// returned error string.
func TestLinearCreate_KeyNeverLeaks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Echo a body that itself contains the key to make sure we don't blindly
		// dump the response (we truncate the errors message, not the auth header).
		_, _ = w.Write([]byte(`{"errors":[{"message":"server said: ` + testAPIKey + ` is invalid"}]}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error")
	}
	// Even if the SERVER echoes the key, our error must not be the only leak path:
	// this asserts our own auth header / api_key field is never the source. The
	// server-echo case is contrived; the contract we enforce is that the adapter
	// never inserts the key itself. Assert the key is absent from the error.
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked in error: %v", err)
	}
}
