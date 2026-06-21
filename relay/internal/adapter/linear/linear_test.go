package linear_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/clubpetey/openintake/relay/internal/adapter/linear"
	"github.com/clubpetey/openintake/relay/internal/payload"
)

// goldenPNGBytes is the smallest valid 1×1 PNG for upload-flow tests.
var goldenPNGBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

var goldenPNGDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(goldenPNGBytes)

// configuredWithUploads builds an adapter pointed at separate GraphQL and
// upload endpoints. Linear's production GraphQL endpoint and uploads endpoint
// are on different hosts; the adapter accepts upload_endpoint as a sibling of
// endpoint for test injection.
func configuredWithUploads(t *testing.T, graphqlURL, uploadsURL string) *linear.Adapter {
	t.Helper()
	a := linear.New()
	if err := a.Configure(map[string]any{
		"api_key":         testAPIKey,
		"team_id":         testTeamUUID,
		"endpoint":        graphqlURL,
		"upload_endpoint": uploadsURL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

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

// testTeamUUID is a valid UUID used by configured() so no team-key resolution
// HTTP call is made during Configure in tests that don't exercise that path.
const testTeamUUID = "00000000-0000-0000-0000-000000000123"

// configured builds a linear adapter pointed at srv with the test team/key.
// team_id is a UUID so Configure stores it verbatim without any HTTP call.
func configured(t *testing.T, srvURL string) *linear.Adapter {
	t.Helper()
	a := linear.New()
	if err := a.Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  testTeamUUID,
		"endpoint": srvURL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// configuredWithUUID builds an adapter using a full UUID team_id (passthrough path).
func configuredWithUUID(t *testing.T, srvURL, teamUUID string) *linear.Adapter {
	t.Helper()
	a := linear.New()
	if err := a.Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  teamUUID,
		"endpoint": srvURL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return a
}

// happyIssueHandler writes a successful issueCreate GraphQL response.
func happyIssueHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":{"id":"abc-123","identifier":"ENG-42","url":"https://linear.app/x/issue/ENG-42"}}}}`))
}

// teamsResponse builds a JSON teams query response with the given nodes.
func teamsResponseJSON(nodes []map[string]string) []byte {
	type node struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	type teams struct {
		Nodes []node `json:"nodes"`
	}
	type data struct {
		Teams teams `json:"teams"`
	}
	type resp struct {
		Data data `json:"data"`
	}
	ns := make([]node, 0, len(nodes))
	for _, m := range nodes {
		ns = append(ns, node{ID: m["id"], Name: m["name"], Key: m["key"]})
	}
	b, _ := json.Marshal(resp{Data: data{Teams: teams{Nodes: ns}}})
	return b
}

// ---------------------------------------------------------------------------
// Original tests (unchanged)
// ---------------------------------------------------------------------------

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
	if req.Variables.Input.TeamID != testTeamUUID {
		t.Errorf("input.teamId = %q; want %q", req.Variables.Input.TeamID, testTeamUUID)
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

// TestLinearCreate_SuccessTrueNilIssue asserts that success:true with a null issue
// is treated as a failure (the ic.Issue == nil arm).
func TestLinearCreate_SuccessTrueNilIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":null}}}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error when issueCreate returns success:true but issue is null")
	}
}

// TestLinearCreate_KeyNeverLeaks_LongPrefix locks the redact-before-truncate ordering.
// The server echoes a message where the api key is preceded by 180 chars of filler,
// so truncate-then-redact would clip the key out of range and let it survive in the error.
func TestLinearCreate_KeyNeverLeaks_LongPrefix(t *testing.T) {
	longPrefix := strings.Repeat("x", 180)
	echoMsg := longPrefix + " token " + testAPIKey + " is invalid"
	body, _ := json.Marshal(map[string]any{
		"errors": []map[string]any{{"message": echoMsg}},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on GraphQL errors response")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked after long-prefix truncation; got error: %v", err)
	}
}

// TestLinearHealthCheck_OK asserts a 200 with a valid viewer response returns nil,
// and also verifies the raw Authorization header equals the api key.
func TestLinearHealthCheck_OK(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"viewer":{"id":"u1"}}}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	if err := a.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck returned unexpected error: %v", err)
	}
	if gotAuth != testAPIKey {
		t.Errorf("Authorization = %q; want raw api key %q", gotAuth, testAPIKey)
	}
}

// TestLinearHealthCheck_GraphQLError asserts that a 200 response containing a
// non-empty GraphQL errors array causes HealthCheck to return an error.
func TestLinearHealthCheck_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"nope"}]}`))
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	if err := a.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error when HealthCheck response contains GraphQL errors")
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

// ---------------------------------------------------------------------------
// New tests: UUID passthrough and team-key resolution
// ---------------------------------------------------------------------------

// TestLinearConfigure_UUIDPassthrough verifies that a UUID team_id is stored
// verbatim and no HTTP call is made during Configure. A Create call afterwards
// proves the teamId field carries the exact UUID supplied.
func TestLinearConfigure_UUIDPassthrough(t *testing.T) {
	const inputUUID = "9ddb7234-31d1-4dd3-b9b0-32ad948b6104"

	var gotBody []byte
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		b, _ := io.ReadAll(r.Body)
		gotBody = b
		// If this is a Configure-phase resolution call, fail the test immediately.
		if strings.Contains(string(b), "teams") {
			t.Errorf("Configure should not call the API when team_id is a UUID; got body: %s", b)
		}
		// For the subsequent Create call, return a happy response.
		happyIssueHandler(w, r)
	}))
	defer srv.Close()

	a := configuredWithUUID(t, srv.URL, inputUUID)

	// Configure must not have triggered any HTTP call.
	if callCount != 0 {
		t.Errorf("Configure made %d HTTP call(s); want 0 for a UUID team_id", callCount)
	}

	// Drive a Create and verify teamId in the request body equals the input UUID.
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var req struct {
		Variables struct {
			Input struct {
				TeamID string `json:"teamId"`
			} `json:"input"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("parse Create body: %v", err)
	}
	if req.Variables.Input.TeamID != inputUUID {
		t.Errorf("teamId = %q; want %q (the UUID passed as team_id)", req.Variables.Input.TeamID, inputUUID)
	}
}

// TestLinearConfigure_KeyResolved_HappyPath verifies that a short team key is
// resolved to a UUID during Configure and that subsequent Create calls send the
// resolved UUID (not the key) as teamId.
func TestLinearConfigure_KeyResolved_HappyPath(t *testing.T) {
	const teamKey = "REF"
	const resolvedUUID = "resolved-uuid-abc"

	teamsBody := teamsResponseJSON([]map[string]string{
		{"id": resolvedUUID, "name": "RefSquare", "key": "REF"},
		{"id": "other-uuid", "name": "Other", "key": "OTHER"},
	})

	callCount := 0
	var issueBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "teams") {
			// Teams resolution call — assert the request and return team data.
			if !strings.Contains(string(b), "teams") {
				t.Errorf("expected teams query, got body: %s", b)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(teamsBody)
			return
		}
		// Subsequent Create call.
		issueBody = b
		happyIssueHandler(w, r)
	}))
	defer srv.Close()

	a := linear.New()
	if err := a.Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  teamKey,
		"endpoint": srv.URL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var req struct {
		Variables struct {
			Input struct {
				TeamID string `json:"teamId"`
			} `json:"input"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(issueBody, &req); err != nil {
		t.Fatalf("parse Create body: %v", err)
	}
	if req.Variables.Input.TeamID != resolvedUUID {
		t.Errorf("teamId = %q; want resolved UUID %q", req.Variables.Input.TeamID, resolvedUUID)
	}
}

// TestLinearConfigure_KeyNotFound verifies that supplying an unknown team key
// returns an error that names the key, lists available keys, and never leaks the api_key.
func TestLinearConfigure_KeyNotFound(t *testing.T) {
	const teamKey = "NOPE"

	teamsBody := teamsResponseJSON([]map[string]string{
		{"id": "id1", "name": "One", "key": "REF"},
		{"id": "id2", "name": "Two", "key": "OTHER"},
		{"id": "id3", "name": "Three", "key": "THIRD"},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(teamsBody)
	}))
	defer srv.Close()

	err := linear.New().Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  teamKey,
		"endpoint": srv.URL,
	})
	if err == nil {
		t.Fatal("expected error for unknown team key")
	}
	msg := err.Error()
	if !strings.Contains(msg, "NOPE") {
		t.Errorf("error should mention the unknown key %q; got: %v", teamKey, err)
	}
	for _, k := range []string{"REF", "OTHER", "THIRD"} {
		if !strings.Contains(msg, k) {
			t.Errorf("error should list available key %q; got: %v", k, err)
		}
	}
	if strings.Contains(msg, testAPIKey) {
		t.Errorf("api key leaked in error: %v", err)
	}
}

// TestLinearConfigure_ResolveGraphQLErrors verifies that a 200 response with a
// non-empty errors array during key resolution surfaces the error message and
// never leaks the api_key (redact-before-truncate ordering).
func TestLinearConfigure_ResolveGraphQLErrors(t *testing.T) {
	// Build a body where the api key appears after 180 chars of filler, mirroring
	// the long-prefix scenario in TestLinearCreate_KeyNeverLeaks_LongPrefix.
	longPrefix := strings.Repeat("x", 180)
	echoMsg := longPrefix + " token " + testAPIKey + " Authentication required"
	body, _ := json.Marshal(map[string]any{
		"errors": []map[string]any{{"message": echoMsg}},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	err := linear.New().Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  "MYTEAM",
		"endpoint": srv.URL,
	})
	if err == nil {
		t.Fatal("expected error on GraphQL errors response during key resolution")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked after long-prefix truncation in resolution error; got: %v", err)
	}
}

// TestLinearConfigure_ResolveNon2xx verifies that a non-2xx HTTP response during
// key resolution produces an error containing the status code but not the api_key.
func TestLinearConfigure_ResolveNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	err := linear.New().Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  "MYTEAM",
		"endpoint": srv.URL,
	})
	if err == nil {
		t.Fatal("expected error on 401 during key resolution")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401; got: %v", err)
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked in resolution error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Phase 6 (6-ii) tests: attachment uploads before issueCreate
// ---------------------------------------------------------------------------

// TestLinearCreate_AttachmentsUploadThenIssueCreate asserts:
//  1. N upload POSTs precede the issueCreate POST.
//  2. Each upload carries multipart/form-data + the raw bytes.
//  3. The issueCreate mutation's input.attachmentLinks references the
//     returned upload URLs with the attachment labels as titles.
func TestLinearCreate_AttachmentsUploadThenIssueCreate(t *testing.T) {
	var (
		uploadCount    int
		uploadBytes    [][]byte
		issueSeen      bool
		issueVariables map[string]any
	)

	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if issueSeen {
			t.Errorf("uploads called AFTER issueCreate — order regression (L011)")
		}
		uploadCount++
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("upload ParseMultipartForm: %v", err)
		}
		var fh *multipart.FileHeader
		for _, files := range r.MultipartForm.File {
			if len(files) > 0 {
				fh = files[0]
				break
			}
		}
		if fh == nil {
			t.Fatal("upload missing file part")
		}
		f, err := fh.Open()
		if err != nil {
			t.Fatalf("open file part: %v", err)
		}
		b, _ := io.ReadAll(f)
		_ = f.Close()
		uploadBytes = append(uploadBytes, b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"uploadFile":{"url":"https://uploads.linear.app/assets/uuid-%d.png"}}`, uploadCount)))
	}))
	defer uploadsSrv.Close()

	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issueSeen = true
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		issueVariables = body.Variables
		happyIssueHandler(w, r)
	}))
	defer graphqlSrv.Close()

	p := minimalPayload()
	label1 := "before"
	label2 := "after"
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes), Label: &label1},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes), Label: &label2},
	}

	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	if _, err := a.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if uploadCount != 2 {
		t.Fatalf("uploadCount = %d; want 2", uploadCount)
	}
	for i, b := range uploadBytes {
		if !bytes.Equal(b, goldenPNGBytes) {
			t.Errorf("upload[%d] bytes mismatch (len=%d, want=%d)", i, len(b), len(goldenPNGBytes))
		}
	}
	input, _ := issueVariables["input"].(map[string]any)
	if input == nil {
		t.Fatalf("issueCreate variables missing input: %v", issueVariables)
	}
	links, _ := input["attachmentLinks"].([]any)
	if len(links) != 2 {
		t.Fatalf("attachmentLinks len = %d; want 2", len(links))
	}
	l0, _ := links[0].(map[string]any)
	l1, _ := links[1].(map[string]any)
	if l0["url"] != "https://uploads.linear.app/assets/uuid-1.png" {
		t.Errorf("attachmentLinks[0].url = %v; want uuid-1.png", l0["url"])
	}
	if l0["title"] != "before" {
		t.Errorf("attachmentLinks[0].title = %v; want before", l0["title"])
	}
	if l1["url"] != "https://uploads.linear.app/assets/uuid-2.png" {
		t.Errorf("attachmentLinks[1].url = %v; want uuid-2.png", l1["url"])
	}
	if l1["title"] != "after" {
		t.Errorf("attachmentLinks[1].title = %v; want after", l1["title"])
	}
}

// TestLinearCreate_NoAttachmentsRegression asserts the no-attachments path
// does not call the uploads endpoint and does not pass attachmentLinks in
// the issueCreate input (L015).
func TestLinearCreate_NoAttachmentsRegression(t *testing.T) {
	var (
		uploadCalled   bool
		issueVariables map[string]any
	)
	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadsSrv.Close()
	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		issueVariables = body.Variables
		happyIssueHandler(w, r)
	}))
	defer graphqlSrv.Close()

	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if uploadCalled {
		t.Error("uploads called when no attachments present")
	}
	input, _ := issueVariables["input"].(map[string]any)
	if _, has := input["attachmentLinks"]; has {
		t.Errorf("attachmentLinks present when no attachments; got: %v", input["attachmentLinks"])
	}
}

// TestLinearCreate_FirstUploadFails_NoIssueCreate asserts orphan prevention.
func TestLinearCreate_FirstUploadFails_NoIssueCreate(t *testing.T) {
	var issueSeen bool
	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"success":false,"error":"upstream broken"}`))
	}))
	defer uploadsSrv.Close()
	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issueSeen = true
		happyIssueHandler(w, r)
	}))
	defer graphqlSrv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on upload 502")
	}
	if issueSeen {
		t.Errorf("issueCreate happened despite upload failure (L011 regression)")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error should mention status 502; got: %v", err)
	}
	if !strings.Contains(err.Error(), "1/1") {
		t.Errorf("error should mention upload index 1/1; got: %v", err)
	}
}

// TestLinearCreate_UploadMissingURL_NoIssueCreate asserts that a 200 upload
// response without a uploadFile.url returns an error before issueCreate.
func TestLinearCreate_UploadMissingURL_NoIssueCreate(t *testing.T) {
	var issueSeen bool
	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"uploadFile":{}}`))
	}))
	defer uploadsSrv.Close()
	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issueSeen = true
		happyIssueHandler(w, r)
	}))
	defer graphqlSrv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on missing uploadFile.url")
	}
	if issueSeen {
		t.Errorf("issueCreate happened despite missing upload url (L011 regression)")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error should mention missing url; got: %v", err)
	}
}

// TestLinearCreate_UploadKeyNeverLeaks_LongPrefix replicates the existing
// KeyNeverLeaks_LongPrefix pattern for the new asset-upload error path: the
// server's error body contains the api key after 180 chars of filler. The
// adapter must redact BEFORE truncate so the key cannot survive in the error.
func TestLinearCreate_UploadKeyNeverLeaks_LongPrefix(t *testing.T) {
	longPrefix := strings.Repeat("x", 180)
	echoMsg := longPrefix + " token " + testAPIKey + " is invalid"

	uploadsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(echoMsg))
	}))
	defer uploadsSrv.Close()
	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("issueCreate must not be called when upload fails")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer graphqlSrv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configuredWithUploads(t, graphqlSrv.URL, uploadsSrv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on upload 401")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Fatalf("L011: api key leaked in upload error (redact-before-truncate ordering broken): %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401; got: %v", err)
	}
}

// TestLinearConfigure_ResolveNetworkError verifies that a transport-level failure
// during key resolution returns a non-nil error without leaking the api_key.
func TestLinearConfigure_ResolveNetworkError(t *testing.T) {
	// Create a server and close it immediately so any connection attempt fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srvURL := srv.URL
	srv.Close()

	err := linear.New().Configure(map[string]any{
		"api_key":  testAPIKey,
		"team_id":  "MYTEAM",
		"endpoint": srvURL,
	})
	if err == nil {
		t.Fatal("expected error when endpoint is unreachable")
	}
	// The underlying network error must be present (non-nil, surfaced as a string).
	if err.Error() == "" {
		t.Error("error string should not be empty")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("api key leaked in network error: %v", err)
	}
}

func TestLinearCreate_UploadSuccessFalse_NoIssueCreate(t *testing.T) {
	var issueSeen bool
	uploadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// success=false even with a populated url: orphan-prevention requires we reject this.
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"success":false,"uploadFile":{"url":"https://example.invalid/asset.png"}}`))
	}))
	defer uploadSrv.Close()
	issueSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issueSeen = true
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":{"id":"abc","identifier":"REF-1","url":"https://linear/REF-1"}}}}`))
	}))
	defer issueSrv.Close()

	a := linear.New()
	if err := a.Configure(map[string]any{
		"api_key":         "lin_api_test_key",
		"team_id":         "00000000-0000-0000-0000-000000000001",
		"endpoint":        issueSrv.URL,
		"upload_endpoint": uploadSrv.URL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	label := "shot"
	p := &payload.IntakePayload{
		Submission:   payload.Submission{Id: "00000000-0000-0000-0000-000000000001", SubmittedAt: time.Now()},
		Client:       payload.Client{},
		User:         payload.User{AuthMode: payload.UserAuthModeAnonymous},
		Conversation: payload.Conversation{TitleSuggestion: "T", Summary: "S", Messages: nil},
		Attachments: []payload.Attachment{{
			Type:      payload.AttachmentTypeScreenshot,
			MimeType:  "image/png",
			SizeBytes: 8,
			Url:       "data:image/png;base64,iVBORw0KGgo=",
			Label:     &label,
		}},
	}
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatalf("Create succeeded; expected error from success=false upload")
	}
	if !strings.Contains(err.Error(), "success=false") {
		t.Errorf("error %q does not mention success=false", err.Error())
	}
	if issueSeen {
		t.Errorf("issueCreate called even though upload reported success=false")
	}
}
