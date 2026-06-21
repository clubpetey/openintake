package zendesk_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/clubpetey/openintake/relay/internal/adapter/zendesk"
	"github.com/clubpetey/openintake/relay/internal/payload"
)

// goldenPNGBytes is the smallest valid 1×1 PNG used for upload-flow tests.
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

// TestZendeskCreate_AttachmentsChainedUploadsThenTicket asserts:
//  1. N uploads.json POSTs precede the ticket POST.
//  2. Each upload carries Content-Type=<mime> and the raw bytes as the body.
//  3. The first upload's response token is reused as ?token=<...> on subsequent uploads.
//  4. The ticket POST body's comment.uploads contains the shared token.
func TestZendeskCreate_AttachmentsChainedUploadsThenTicket(t *testing.T) {
	const sharedToken = "upl_TOKEN_abc123"
	var (
		uploadCallCount int
		uploadOrder     []string
		uploadCTs       []string
		uploadBodies    [][]byte
		ticketBody      map[string]any
		ticketSeen      bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/uploads.json":
			if ticketSeen {
				t.Errorf("uploads called AFTER ticket POST — order regression (L011)")
			}
			uploadCallCount++
			uploadCTs = append(uploadCTs, r.Header.Get("Content-Type"))
			b, _ := io.ReadAll(r.Body)
			uploadBodies = append(uploadBodies, b)
			uploadOrder = append(uploadOrder, r.URL.RawQuery)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"upload":{"token":"` + sharedToken + `","attachment":{"id":1,"content_url":"https://example.zendesk.com/attachments/1"}}}`))

		case "/api/v2/tickets.json":
			ticketSeen = true
			b, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(b, &ticketBody); err != nil {
				t.Fatalf("ticket body not JSON: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ticket":{"id":777,"url":"https://example.zendesk.com/api/v2/tickets/777.json"}}`))

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	label1 := "before.png"
	label2 := "after.png"
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes), Label: &label1},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes), Label: &label2},
	}

	a := configured(t, srv.URL)
	result, err := a.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if uploadCallCount != 2 {
		t.Fatalf("uploadCallCount = %d; want 2", uploadCallCount)
	}
	if !strings.Contains(uploadOrder[0], "filename=before.png") {
		t.Errorf("first upload query missing filename=before.png: %q", uploadOrder[0])
	}
	if strings.Contains(uploadOrder[0], "token=") {
		t.Errorf("first upload query must NOT contain token=…: %q", uploadOrder[0])
	}
	if !strings.Contains(uploadOrder[1], "filename=after.png") {
		t.Errorf("second upload query missing filename=after.png: %q", uploadOrder[1])
	}
	if !strings.Contains(uploadOrder[1], "token="+sharedToken) {
		t.Errorf("second upload query missing token=%s: %q", sharedToken, uploadOrder[1])
	}
	for i, ct := range uploadCTs {
		if ct != "image/png" {
			t.Errorf("upload[%d] Content-Type = %q; want image/png", i, ct)
		}
	}
	for i, body := range uploadBodies {
		if !bytes.Equal(body, goldenPNGBytes) {
			t.Errorf("upload[%d] body mismatch (len=%d, want=%d)", i, len(body), len(goldenPNGBytes))
		}
	}
	ticket, _ := ticketBody["ticket"].(map[string]any)
	if ticket == nil {
		t.Fatalf("ticket key missing in body: %v", ticketBody)
	}
	comment, _ := ticket["comment"].(map[string]any)
	if comment == nil {
		t.Fatalf("ticket.comment missing: %v", ticket)
	}
	uploads, _ := comment["uploads"].([]any)
	if len(uploads) != 1 {
		t.Fatalf("comment.uploads len = %d; want 1 (shared token)", len(uploads))
	}
	if uploads[0] != sharedToken {
		t.Errorf("comment.uploads[0] = %v; want %q", uploads[0], sharedToken)
	}
	if result.ExternalID != "777" {
		t.Errorf("ExternalID = %q; want 777", result.ExternalID)
	}
}

// TestZendeskCreate_NoAttachmentsRegression asserts the no-attachments path
// is byte-identical to Phase 3 — no uploads.json calls, no comment.uploads
// field in the ticket body (L015).
func TestZendeskCreate_NoAttachmentsRegression(t *testing.T) {
	var (
		uploadCalled bool
		ticketBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/uploads.json":
			uploadCalled = true
			w.WriteHeader(http.StatusOK)
		case "/api/v2/tickets.json":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &ticketBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ticket":{"id":1,"url":"u"}}`))
		}
	}))
	defer srv.Close()

	a := configured(t, srv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if uploadCalled {
		t.Error("uploads.json called when no attachments present")
	}
	ticket, _ := ticketBody["ticket"].(map[string]any)
	comment, _ := ticket["comment"].(map[string]any)
	if _, has := comment["uploads"]; has {
		t.Errorf("comment.uploads present when no attachments; got: %v", comment["uploads"])
	}
}

// TestZendeskCreate_FirstUploadFails_NoTicketCreate asserts orphan prevention:
// an upload non-2xx returns an error BEFORE the ticket POST (L011).
func TestZendeskCreate_FirstUploadFails_NoTicketCreate(t *testing.T) {
	var ticketSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/uploads.json":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"upstream broken"}`))
		case "/api/v2/tickets.json":
			ticketSeen = true
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on upload 500")
	}
	if ticketSeen {
		t.Errorf("ticket POST happened despite upload failure (L011 regression)")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error must mention status 500; got: %v", err)
	}
	if !strings.Contains(err.Error(), "1/1") {
		t.Errorf("error must include upload index %q; got: %v", "1/1", err)
	}
	if strings.Contains(err.Error(), "upstream broken") {
		t.Errorf("L005: error must NOT include response body (uploads endpoint may echo Authorization); got: %v", err)
	}
}

// TestZendeskCreate_MidBatchUploadFails_NoTicketCreate asserts that a 2xx
// first upload followed by a 5xx second upload returns an error BEFORE the
// ticket POST.
func TestZendeskCreate_MidBatchUploadFails_NoTicketCreate(t *testing.T) {
	var uploadCount, ticketCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/uploads.json":
			uploadCount++
			if uploadCount == 1 {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"upload":{"token":"t1","attachment":{"id":1}}}`))
				return
			}
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"transient"}`))
		case "/api/v2/tickets.json":
			ticketCount++
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on second upload 502")
	}
	if ticketCount != 0 {
		t.Errorf("ticket POST happened (count=%d) despite mid-batch upload failure", ticketCount)
	}
	if !strings.Contains(err.Error(), "2/2") {
		t.Errorf("error must reference upload index 2/2; got: %v", err)
	}
}

// TestZendeskCreate_UploadErrorOmitsBody_L005Guard asserts the Authorization
// header / token-echo guard: even when the upload response body contains the
// configured token, the error message must NOT contain it.
func TestZendeskCreate_UploadErrorOmitsBody_L005Guard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/uploads.json" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"bad auth: ` + r.Header.Get("Authorization") + `"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configured(t, srv.URL)
	_, err := a.Create(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("L005: token leaked in error: %v", err)
	}
	if strings.Contains(err.Error(), "Basic ") {
		t.Fatalf("L005: Authorization header echoed in error: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401; got: %v", err)
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
