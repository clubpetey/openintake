package chatwoot_test

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

// happyPathHandler returns an http.HandlerFunc that serves the standard two-call
// flow: /contacts returns a canned contact+contact_inbox payload, /conversations
// returns a canned conversation id. Unexpected paths are reported as test errors.
func happyPathHandler(t *testing.T, accountID int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"src-uuid-abc"}}}`))
		case "/api/v1/accounts/1/conversations":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":123}`))
		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// TestChatwootCreate_PostsConversation asserts both POSTs in the two-call flow:
//   - /contacts receives the correct inbox_id, name, identifier, and (absent) email
//   - /conversations receives source_id and contact_id from the contact response
//   - the response id becomes ExternalID / ExternalURL
func TestChatwootCreate_PostsConversation(t *testing.T) {
	var gotContactMethod, gotContactAuth, gotContactCT string
	var gotContactBody map[string]any
	var gotConvMethod, gotConvAuth, gotConvCT string
	var gotConvBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			gotContactMethod = r.Method
			gotContactAuth = r.Header.Get("api_access_token")
			gotContactCT = r.Header.Get("Content-Type")
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read contact body: %v", err)
			}
			if err := json.Unmarshal(raw, &gotContactBody); err != nil {
				t.Fatalf("contact body not valid JSON: %v\nbody: %s", err, raw)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"src-uuid-abc"}}}`))

		case "/api/v1/accounts/1/conversations":
			gotConvMethod = r.Method
			gotConvAuth = r.Header.Get("api_access_token")
			gotConvCT = r.Header.Get("Content-Type")
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read conv body: %v", err)
			}
			if err := json.Unmarshal(raw, &gotConvBody); err != nil {
				t.Fatalf("conv body not valid JSON: %v\nbody: %s", err, raw)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":123}`))

		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	a := configure(t, srv.URL)
	result, err := a.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// --- Contact call assertions ---
	if gotContactMethod != http.MethodPost {
		t.Errorf("contact: expected POST, got %s", gotContactMethod)
	}
	if gotContactAuth != testToken {
		t.Errorf("contact: expected api_access_token header = token, got %q", gotContactAuth)
	}
	if gotContactCT != "application/json" {
		t.Errorf("contact: expected Content-Type application/json, got %q", gotContactCT)
	}
	if iv, ok := gotContactBody["inbox_id"].(float64); !ok || int(iv) != 3 {
		t.Errorf("contact: expected inbox_id=3, got %v", gotContactBody["inbox_id"])
	}
	if id, _ := gotContactBody["identifier"].(string); id != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("contact: expected identifier=submission id, got %v", gotContactBody["identifier"])
	}
	// minimalPayload has no email; the field must be absent entirely (not an empty string).
	if _, hasEmail := gotContactBody["email"]; hasEmail {
		t.Errorf("contact: email key must be absent when User.Email is nil, got %v", gotContactBody["email"])
	}
	// name should be the fallback label derived from the submission id
	if name, _ := gotContactBody["name"].(string); name == "" {
		t.Error("contact: expected non-empty name")
	}

	// --- Conversation call assertions ---
	if gotConvMethod != http.MethodPost {
		t.Errorf("conv: expected POST, got %s", gotConvMethod)
	}
	if gotConvAuth != testToken {
		t.Errorf("conv: expected api_access_token header = token, got %q", gotConvAuth)
	}
	if gotConvCT != "application/json" {
		t.Errorf("conv: expected Content-Type application/json, got %q", gotConvCT)
	}
	if sid, _ := gotConvBody["source_id"].(string); sid != "src-uuid-abc" {
		t.Errorf("conv: expected source_id=src-uuid-abc (from contact response), got %v", gotConvBody["source_id"])
	}
	if iv, ok := gotConvBody["inbox_id"].(float64); !ok || int(iv) != 3 {
		t.Errorf("conv: expected inbox_id=3, got %v", gotConvBody["inbox_id"])
	}
	// contact_id should be 50 (from the canned contact response).
	if cid, ok := gotConvBody["contact_id"].(float64); !ok || int(cid) != 50 {
		t.Errorf("conv: expected contact_id=50, got %v", gotConvBody["contact_id"])
	}
	msg, ok := gotConvBody["message"].(map[string]any)
	if !ok {
		t.Fatalf("conv: expected message object in body, got %v", gotConvBody["message"])
	}
	content, _ := msg["content"].(string)
	for _, want := range []string{
		"Save button does nothing",
		"Save button is unresponsive",
		"user: The save button does nothing",
		"assistant: Thanks, can you share",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("conv: message.content missing %q\ncontent: %s", want, content)
		}
	}

	// --- Result assertions ---
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

// TestChatwootCreate_NonOKErrorNoToken asserts a non-2xx from the /contacts
// endpoint returns an error that includes the status code and the truncated body
// but NEVER the token. The /conversations endpoint is never reached.
func TestChatwootCreate_NonOKErrorNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Access denied"}`))
		default:
			// /conversations should never be reached
			t.Errorf("unexpected request path after contact failure: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
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

// TestChatwootCreate_ContactCreateFails asserts that a 422 from /contacts
// returns an error containing the status code and truncated body, and never the
// token. The /conversations endpoint is never reached.
func TestChatwootCreate_ContactCreateFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"Email has already been taken"}`))
		default:
			t.Errorf("unexpected request path after contact failure: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	_, err := a.Create(context.Background(), minimalPayload())
	if err == nil {
		t.Fatal("expected error on 422, got nil")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("error should mention status code 422, got %v", err)
	}
	if !strings.Contains(err.Error(), "Email has already been taken") {
		t.Errorf("error should contain truncated response body, got %v", err)
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
	t.Run("zero account_id", func(t *testing.T) {
		a := chatwoot.New()
		err := a.Configure(map[string]any{
			"base_url":   "https://chatwoot.example.com",
			"account_id": 0,
			"inbox_id":   3,
			"api_token":  testToken,
		})
		if err == nil || !strings.Contains(err.Error(), "account_id") {
			t.Fatalf("expected error naming account_id, got %v", err)
		}
	})
	t.Run("missing account_id", func(t *testing.T) {
		a := chatwoot.New()
		err := a.Configure(map[string]any{
			"base_url":  "https://chatwoot.example.com",
			"inbox_id":  3,
			"api_token": testToken,
		})
		if err == nil || !strings.Contains(err.Error(), "account_id") {
			t.Fatalf("expected error naming account_id, got %v", err)
		}
	})
	t.Run("zero inbox_id", func(t *testing.T) {
		a := chatwoot.New()
		err := a.Configure(map[string]any{
			"base_url":   "https://chatwoot.example.com",
			"account_id": 1,
			"inbox_id":   0,
			"api_token":  testToken,
		})
		if err == nil || !strings.Contains(err.Error(), "inbox_id") {
			t.Fatalf("expected error naming inbox_id, got %v", err)
		}
	})
	t.Run("missing inbox_id", func(t *testing.T) {
		a := chatwoot.New()
		err := a.Configure(map[string]any{
			"base_url":   "https://chatwoot.example.com",
			"account_id": 1,
			"api_token":  testToken,
		})
		if err == nil || !strings.Contains(err.Error(), "inbox_id") {
			t.Fatalf("expected error naming inbox_id, got %v", err)
		}
	})
}

// goldenPNGBytes is a 1×1 transparent PNG (smallest valid PNG byte sequence)
// used by attachment tests so DecodeOne's magic-byte path agrees with the
// declared mime_type.
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

// goldenPNGDataURL is the base64-encoded data: URL form of goldenPNGBytes.
var goldenPNGDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(goldenPNGBytes)

// TestChatwootCreate_AttachmentsMultipart asserts the conversation-create call
// switches to multipart/form-data when attachments are present and the body
// contains the expected form fields + one attachments[] part per attachment
// carrying the decoded raw bytes with the correct Content-Type/filename.
func TestChatwootCreate_AttachmentsMultipart(t *testing.T) {
	var convCT string
	var convFields map[string]string
	type uploadedPart struct {
		filename string
		ctype    string
		bytes    []byte
	}
	var convAttachments []uploadedPart

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"src-uuid-mp"}}}`))

		case "/api/v1/accounts/1/conversations":
			convCT = r.Header.Get("Content-Type")
			convFields = map[string]string{}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			for k, vs := range r.MultipartForm.Value {
				if len(vs) > 0 {
					convFields[k] = vs[0]
				}
			}
			for _, fh := range r.MultipartForm.File["attachments[]"] {
				f, err := fh.Open()
				if err != nil {
					t.Fatalf("open part: %v", err)
				}
				b, err := io.ReadAll(f)
				_ = f.Close()
				if err != nil {
					t.Fatalf("read part: %v", err)
				}
				convAttachments = append(convAttachments, uploadedPart{
					filename: fh.Filename,
					ctype:    fh.Header.Get("Content-Type"),
					bytes:    b,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":999}`))

		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	label := "before-save.png"
	p.Attachments = []payload.Attachment{{
		Type:      payload.AttachmentTypeScreenshot,
		MimeType:  "image/png",
		Url:       goldenPNGDataURL,
		SizeBytes: len(goldenPNGBytes),
		Label:     &label,
	}}

	a := configure(t, srv.URL)
	result, err := a.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !strings.HasPrefix(convCT, "multipart/form-data") {
		t.Errorf("conversation Content-Type = %q; want multipart/form-data", convCT)
	}
	if convFields["inbox_id"] != "3" {
		t.Errorf("multipart inbox_id = %q; want 3", convFields["inbox_id"])
	}
	if convFields["source_id"] != "src-uuid-mp" {
		t.Errorf("multipart source_id = %q; want src-uuid-mp", convFields["source_id"])
	}
	if convFields["contact_id"] != "50" {
		t.Errorf("multipart contact_id = %q; want 50", convFields["contact_id"])
	}
	if got := convFields["message[content]"]; !strings.Contains(got, "Save button does nothing") {
		t.Errorf("multipart message[content] missing title; got: %q", got)
	}
	if len(convAttachments) != 1 {
		t.Fatalf("attachments[] parts = %d; want 1", len(convAttachments))
	}
	part := convAttachments[0]
	if part.filename != "before-save.png" {
		t.Errorf("part.filename = %q; want before-save.png", part.filename)
	}
	if part.ctype != "image/png" {
		t.Errorf("part Content-Type = %q; want image/png", part.ctype)
	}
	if !bytes.Equal(part.bytes, goldenPNGBytes) {
		t.Errorf("part bytes mismatch (len=%d, want=%d)", len(part.bytes), len(goldenPNGBytes))
	}
	if result.ExternalID != "999" {
		t.Errorf("ExternalID = %q; want 999", result.ExternalID)
	}
}

// TestChatwootCreate_NoAttachmentsJSONPathUnchanged asserts that when
// p.Attachments is empty the conversation-create body stays application/json
// (L015 regression — existing JSON path must not flip to multipart by accident).
func TestChatwootCreate_NoAttachmentsJSONPathUnchanged(t *testing.T) {
	var convCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"src-uuid"}}}`))
		case "/api/v1/accounts/1/conversations":
			convCT = r.Header.Get("Content-Type")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := configure(t, srv.URL)
	if _, err := a.Create(context.Background(), minimalPayload()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if convCT != "application/json" {
		t.Errorf("no-attachments path Content-Type = %q; want application/json", convCT)
	}
}

// TestChatwootCreate_AttachmentsLabelFallback asserts a nil/empty Label
// produces the "screenshot N" (1-indexed) filename per the design matrix.
func TestChatwootCreate_AttachmentsLabelFallback(t *testing.T) {
	var filenames []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/1/contacts":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"s"}}}`))
		case "/api/v1/accounts/1/conversations":
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			for _, fh := range r.MultipartForm.File["attachments[]"] {
				filenames = append(filenames, fh.Filename)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		}
	}))
	defer srv.Close()

	p := minimalPayload()
	p.Attachments = []payload.Attachment{
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
		{Type: payload.AttachmentTypeScreenshot, MimeType: "image/png", Url: goldenPNGDataURL, SizeBytes: len(goldenPNGBytes)},
	}
	a := configure(t, srv.URL)
	if _, err := a.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(filenames) != 2 {
		t.Fatalf("filenames len = %d; want 2", len(filenames))
	}
	if filenames[0] != "screenshot 1" {
		t.Errorf("filenames[0] = %q; want screenshot 1", filenames[0])
	}
	if filenames[1] != "screenshot 2" {
		t.Errorf("filenames[1] = %q; want screenshot 2", filenames[1])
	}
}

// TestChatwootConfigure_AcceptsFloatIDs asserts account_id/inbox_id accept the
// float64 form that a JSON/YAML decode may produce (mirrors webhook's retry ints).
// The httptest server handles both calls in the two-call flow.
func TestChatwootConfigure_AcceptsFloatIDs(t *testing.T) {
	var gotConvPath string
	var gotConvBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/9/contacts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"payload":{"contact":{"id":50},"contact_inbox":{"id":42,"source_id":"src-float-test"}}}`))
		case "/api/v1/accounts/9/conversations":
			gotConvPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotConvBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":7}`))
		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
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
	if gotConvPath != "/api/v1/accounts/9/conversations" {
		t.Errorf("float account_id not applied: path %q", gotConvPath)
	}
	if iv, ok := gotConvBody["inbox_id"].(float64); !ok || int(iv) != 4 {
		t.Errorf("float inbox_id not applied: %v", gotConvBody["inbox_id"])
	}
}
