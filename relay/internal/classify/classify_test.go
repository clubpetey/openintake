package classify_test

import (
	"context"
	"testing"

	"github.com/clubpetey/openintake/relay/internal/classify"
	"github.com/clubpetey/openintake/relay/internal/llm"
)

// fakeProvider returns a canned response as a single Done chunk.
type fakeProvider struct {
	response string
	err      error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan llm.ChatChunk, 1)
	ch <- llm.ChatChunk{Delta: f.response, Done: true}
	close(ch)
	return ch, nil
}

const validClassifyJSON = `{
  "summary": "User cannot log in after resetting password.",
  "title_suggestion": "Login fails after password reset",
  "classification": "bug",
  "severity_guess": "high",
  "tags_suggested": ["auth", "login"],
  "language": "en"
}`

func TestClassifier_HappyPath(t *testing.T) {
	provider := &fakeProvider{response: validClassifyJSON}
	c := classify.New(provider, "claude-sonnet-4-6", 512)

	result, err := c.Classify(context.Background(), []llm.Message{
		{Role: "user", Content: "I can't log in after resetting my password."},
		{Role: "assistant", Content: "Can you describe what happens when you try?"},
		{Role: "user", Content: "It says invalid credentials every time."},
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	if result.Classification != "bug" {
		t.Errorf("expected classification=bug, got %q", result.Classification)
	}
	if result.SeverityGuess != "high" {
		t.Errorf("expected severity_guess=high, got %q", result.SeverityGuess)
	}
	if result.TitleSuggestion != "Login fails after password reset" {
		t.Errorf("unexpected title: %q", result.TitleSuggestion)
	}
	if len(result.TagsSuggested) != 2 {
		t.Errorf("expected 2 tags, got %d", len(result.TagsSuggested))
	}
	if result.Language != "en" {
		t.Errorf("expected language=en, got %q", result.Language)
	}
}

func TestClassifier_InvalidEnumFallsBackToDefaults(t *testing.T) {
	// Both attempts return invalid classification; should degrade to safe defaults.
	provider := &fakeProvider{response: `{
		"summary": "Something",
		"title_suggestion": "X",
		"classification": "INVALID_VALUE",
		"severity_guess": "medium",
		"tags_suggested": [],
		"language": "en"
	}`}
	c := classify.New(provider, "claude-sonnet-4-6", 512)

	result, err := c.Classify(context.Background(), []llm.Message{
		{Role: "user", Content: "Test message"},
	})
	// err is non-nil (safe-defaults path) but result is still usable.
	if err == nil {
		t.Error("expected non-nil error signalling safe-defaults path")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on safe-defaults path")
	}
	if result.Classification != "other" {
		t.Errorf("expected safe default classification=other, got %q", result.Classification)
	}
	if result.SeverityGuess != "unknown" {
		t.Errorf("expected safe default severity_guess=unknown, got %q", result.SeverityGuess)
	}
}

func TestClassifier_CodeFencesStripped(t *testing.T) {
	// Model disobeys and wraps in ```json ... ```.
	wrapped := "```json\n" + validClassifyJSON + "\n```"
	provider := &fakeProvider{response: wrapped}
	c := classify.New(provider, "claude-sonnet-4-6", 512)

	result, err := c.Classify(context.Background(), []llm.Message{
		{Role: "user", Content: "test"},
	})
	if err != nil {
		t.Fatalf("Classify with fenced response: %v", err)
	}
	if result.Classification != "bug" {
		t.Errorf("expected bug, got %q", result.Classification)
	}
}

func TestClassifier_TitleTruncatedTo80(t *testing.T) {
	longTitle := `This is a very long title that definitely exceeds the eighty character limit imposed by the schema and must be truncated`
	provider := &fakeProvider{response: `{
		"summary": "s",
		"title_suggestion": "` + longTitle + `",
		"classification": "other",
		"severity_guess": "unknown",
		"tags_suggested": [],
		"language": "en"
	}`}
	c := classify.New(provider, "claude-sonnet-4-6", 512)

	result, err := c.Classify(context.Background(), []llm.Message{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(result.TitleSuggestion) > 80 {
		t.Errorf("TitleSuggestion not truncated: len=%d", len(result.TitleSuggestion))
	}
}
