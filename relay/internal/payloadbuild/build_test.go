package payloadbuild_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/dto"
	"intake/internal/payloadbuild"
)

func testSession() *auth.SessionContext {
	return &auth.SessionContext{
		SessionID: "00000000-0000-0000-0000-000000000002",
		AuthMode:  "anonymous",
		Verified:  false,
	}
}

func testClassifyResult() *classify.Result {
	return &classify.Result{
		Summary:         "User cannot log in after password reset.",
		TitleSuggestion: "Login fails after password reset",
		Classification:  "bug",
		SeverityGuess:   "high",
		TagsSuggested:   []string{"auth", "login"},
		Language:        "en",
	}
}

func testSubmitRequest() *dto.SubmitRequest {
	ref := "http://localhost:5173/"
	return &dto.SubmitRequest{
		Messages: []dto.TurnMessage{
			{Role: "user", Content: "I cannot log in."},
			{Role: "assistant", Content: "Can you describe the error?"},
			{Role: "user", Content: "It says invalid credentials."},
		},
		Client: dto.ClientInfo{
			WidgetVersion: "0.1.0",
			URL:           "http://localhost:5173/",
			Referrer:      &ref,
			UserAgent:     "Mozilla/5.0",
			Viewport:      dto.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		UserClaims: map[string]any{},
		Context: dto.ContextInfo{
			AppContext:    map[string]any{"env": "test"},
			PageMetadata: map[string]any{"title": "Home"},
		},
	}
}

// TestBuild_WellFormed_ProducesSchemaValidPayload is the main L003 mitigation test.
func TestBuild_WellFormed_ProducesSchemaValidPayload(t *testing.T) {
	b := payloadbuild.New("0.1.0")
	p, err := b.Build(
		context.Background(),
		testSubmitRequest(),
		testClassifyResult(),
		testSession(),
		payloadbuild.NewSubmissionID(),
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if p.SchemaVersion != "1.0" {
		t.Errorf("expected SchemaVersion=1.0, got %q", p.SchemaVersion)
	}
	if p.User.AuthMode != "anonymous" {
		t.Errorf("expected User.AuthMode=anonymous, got %q", p.User.AuthMode)
	}
	if p.Conversation.Classification != "bug" {
		t.Errorf("expected Classification=bug, got %q", p.Conversation.Classification)
	}
	if len(p.Conversation.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(p.Conversation.Messages))
	}
}

// TestBuild_InvalidClassification_IsRejected is the direct L003 mitigation test:
// asserts that if we set an invalid classification, the mapClassification function
// rejects it before schema validation even runs.
func TestBuild_InvalidClassification_IsRejected(t *testing.T) {
	b := payloadbuild.New("0.1.0")

	badResult := testClassifyResult()
	badResult.Classification = "INVALID_CLASSIFICATION" // not in enum

	_, err := b.Build(
		context.Background(),
		testSubmitRequest(),
		badResult,
		testSession(),
		payloadbuild.NewSubmissionID(),
		time.Now().UTC(),
	)
	if err == nil {
		t.Fatal("expected error for invalid classification, got nil")
	}
	t.Logf("correctly rejected: %v", err)
}

// TestBuild_EmbeddedSchemaIsIdenticalToCanonical guards against schema drift (L003 mitigation).
// The embedded schema.json must stay byte-identical to schema/payload.v1.json.
func TestBuild_EmbeddedSchemaIsIdenticalToCanonical(t *testing.T) {
	embedded := payloadbuild.CanonicalSchemaBytes()

	// Locate the canonical file relative to the repo root.
	// __FILE__ is relay/internal/payloadbuild/build_test.go
	// so the canonical path is ../../../schema/payload.v1.json from here.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	canonicalPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "schema", "payload.v1.json")

	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical schema: %v\npath tried: %s", err, canonicalPath)
	}

	if string(embedded) != string(canonical) {
		t.Errorf("embedded schema.json is NOT byte-identical to schema/payload.v1.json\n"+
			"Run: cd relay && go generate ./internal/payloadbuild/...\n"+
			"to regenerate the embedded copy.")
	}
}
