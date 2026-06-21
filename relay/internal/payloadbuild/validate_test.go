// White-box tests for validateAgainstSchema (package payloadbuild, not payloadbuild_test).
// These tests exercise the schema-validator error branch directly — i.e. cases where
// the Go type system allows a value but the JSON schema rejects it (L003 mitigation).
package payloadbuild

import (
	"strings"
	"testing"
	"time"

	"github.com/clubpetey/openintake/relay/internal/payload"
)

// validBasePayload returns a fully-valid IntakePayload that passes schema validation.
// Tests mutate exactly one field to verify the schema catches it.
func validBasePayload() *payload.IntakePayload {
	submittedAt := time.Now().UTC()
	return &payload.IntakePayload{
		SchemaVersion: "1.0",
		Submission: payload.Submission{
			Id:          "00000000-0000-0000-0000-000000000001",
			SubmittedAt: submittedAt,
		},
		Client: payload.Client{
			WidgetVersion: "0.1.0",
			SessionId:     "00000000-0000-0000-0000-000000000002",
			Url:           "http://localhost:5173/",
			UserAgent:     "Mozilla/5.0",
			Viewport:      payload.Viewport{W: 1280, H: 720},
			Locale:        "en-US",
		},
		User: payload.User{
			AuthMode: payload.UserAuthModeAnonymous,
			Verified: false,
		},
		Conversation: payload.Conversation{
			Messages: []payload.Message{
				{Role: payload.MessageRoleUser, Content: "I cannot log in.", Ts: submittedAt},
			},
			Summary:         "User cannot log in.",
			TitleSuggestion: "Login fails",
			Classification:  payload.ConversationClassificationBug,
			SeverityGuess:   payload.ConversationSeverityGuessHigh,
			TagsSuggested:   []string{"auth"},
			Language:        "en",
		},
	}
}

// TestValidateAgainstSchema_AcceptsValidPayload confirms the baseline passes.
// Without this guard, a broken validBasePayload() would make the reject tests
// vacuously pass (they'd all fail even before mutation).
func TestValidateAgainstSchema_AcceptsValidPayload(t *testing.T) {
	p := validBasePayload()
	if err := validateAgainstSchema(p); err != nil {
		t.Fatalf("expected valid payload to pass schema validation, got: %v", err)
	}
}

// TestValidateAgainstSchema_RejectsBadSchemaVersion is the required L003 coverage test.
//
// The Go type for SchemaVersion is a plain `string` (go-jsonschema does not emit
// a const-enforcing UnmarshalJSON for it). So Go happily holds "9.9" without complaint.
// The JSON schema declares `"const": "1.0"`, so validateAgainstSchema must catch it.
func TestValidateAgainstSchema_RejectsBadSchemaVersion(t *testing.T) {
	p := validBasePayload()
	p.SchemaVersion = "9.9" // Go: fine. Schema: const violation → must error.

	err := validateAgainstSchema(p)
	if err == nil {
		t.Fatal("validateAgainstSchema: expected error for schema_version=\"9.9\" (const violation), got nil — schema validator NOT guarding the const")
	}
	t.Logf("schema correctly rejected schema_version=9.9: %v", err)
}

// TestValidateAgainstSchema_RejectsTitleSuggestionTooLong verifies the schema's
// maxLength:80 constraint on title_suggestion is enforced by the validator,
// not just by the generated UnmarshalJSON (which only fires on decode, not encode).
func TestValidateAgainstSchema_RejectsTitleSuggestionTooLong(t *testing.T) {
	p := validBasePayload()
	// 81 characters — one over the maxLength:80 limit in the schema.
	p.Conversation.TitleSuggestion = strings.Repeat("x", 81)

	err := validateAgainstSchema(p)
	if err == nil {
		t.Fatal("validateAgainstSchema: expected error for title_suggestion with 81 chars (maxLength:80 violation), got nil")
	}
	t.Logf("schema correctly rejected long title_suggestion: %v", err)
}
