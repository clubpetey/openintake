package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/clubpetey/openintake/relay/internal/llm"
)

// Result is the structured triage output from classify(). Frozen in 1-iv (README §6.5).
type Result struct {
	Summary         string   `json:"summary"`
	TitleSuggestion string   `json:"title_suggestion"`
	Classification  string   `json:"classification"` // bug|feature_request|question|other
	SeverityGuess   string   `json:"severity_guess"` // low|medium|high|critical|unknown
	TagsSuggested   []string `json:"tags_suggested"`
	Language        string   `json:"language"`
}

// Valid enum sets for runtime validation (complements the typed consts in payload package).
var validClassifications = map[string]bool{
	"bug": true, "feature_request": true, "question": true, "other": true,
}
var validSeverities = map[string]bool{
	"low": true, "medium": true, "high": true, "critical": true, "unknown": true,
}

// safeDefaults returns a Result with safe fallback values.
func safeDefaults() *Result {
	return &Result{
		Summary:         "(classification unavailable)",
		TitleSuggestion: "Untitled",
		Classification:  "other",
		SeverityGuess:   "unknown",
		TagsSuggested:   []string{},
		Language:        "en",
	}
}

// classifySystemPrompt instructs the model to return strict JSON.
const classifySystemPrompt = `You are a triage assistant. Analyze the provided support conversation and return ONLY a JSON object with no markdown, no code fences, and no additional text.

The JSON object must have these exact keys:
- "summary": string — a one-to-three sentence plain-English summary of the issue
- "title_suggestion": string — a concise title of at most 80 characters
- "classification": one of "bug", "feature_request", "question", "other"
- "severity_guess": one of "low", "medium", "high", "critical", "unknown"
- "tags_suggested": array of strings — up to 5 relevant tags, empty array if none
- "language": ISO 639-1 two-letter language code of the user's messages

Return only the JSON object. No other text.`

// Classifier calls the LLM provider to produce a structured triage Result.
type Classifier struct {
	provider  llm.Provider
	model     string
	maxTokens int
}

// New returns a Classifier backed by the given provider.
// model and maxTokens should come from config.LLMConfig.Anthropic.
func New(provider llm.Provider, model string, maxTokens int) *Classifier {
	return &Classifier{
		provider:  provider,
		model:     model,
		maxTokens: maxTokens,
	}
}

// Classify makes one non-streaming LLM call with classifySystemPrompt and the
// provided messages, parses the JSON response into a Result, and validates
// enum fields. On parse failure it retries once; on second failure it returns
// safeDefaults (never propagates a parse error — the submit handler must still
// produce a valid payload even if classify degrades gracefully).
func (c *Classifier) Classify(ctx context.Context, messages []llm.Message) (*Result, error) {
	result, err := c.doClassify(ctx, messages)
	if err != nil {
		// Retry once.
		result, err = c.doClassify(ctx, messages)
		if err != nil {
			// Degrade gracefully: return safe defaults so the submit still completes.
			// Log the failure (caller should log the returned error for observability).
			return safeDefaults(), fmt.Errorf("classify: both attempts failed, using safe defaults: %w", err)
		}
	}
	return result, nil
}

// doClassify performs one classify LLM call and returns a parsed, validated Result.
func (c *Classifier) doClassify(ctx context.Context, messages []llm.Message) (*Result, error) {
	// Prepend the classify system message and user turn with conversation context.
	prompt := buildPrompt(messages)
	classifyMessages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	opts := llm.ChatOptions{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		Stream:    false, // classify uses accumulate-then-parse, not streaming UX
	}

	ch, err := c.provider.Chat(ctx, classifyMessages, opts)
	if err != nil {
		return nil, fmt.Errorf("provider.Chat: %w", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return nil, fmt.Errorf("provider chunk error: %w", chunk.Err)
		}
		sb.WriteString(chunk.Delta)
		if chunk.Done {
			break
		}
	}

	raw := strings.TrimSpace(sb.String())
	// Strip accidental markdown code fences if the model disobeyed.
	raw = stripCodeFences(raw)

	var result Result
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse classify JSON: %w (raw: %s)", err, truncate(raw, 300))
	}

	if err := validateResult(&result); err != nil {
		return nil, fmt.Errorf("validate classify result: %w", err)
	}

	return &result, nil
}

// maxConversationChars is the maximum total length of the conversation portion
// written into the classify prompt. Messages exceeding this are truncated from
// the oldest end so the model context limit is never blown.
const maxConversationChars = 20000

// buildPrompt combines the system prompt with the conversation for the classify call.
// Each message role is clamped to "user" or "assistant" before interpolation to
// prevent prompt injection via untrusted role values. The conversation portion is
// capped at maxConversationChars, keeping the most recent content.
func buildPrompt(messages []llm.Message) string {
	// Build the conversation lines with clamped roles.
	var conv strings.Builder
	for _, m := range messages {
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		conv.WriteString(fmt.Sprintf("[%s]: %s\n", role, m.Content))
	}
	convStr := conv.String()

	// Cap conversation length — drop from the front (oldest messages) to keep
	// the most recent context within the limit.
	if len(convStr) > maxConversationChars {
		convStr = truncateConversation(convStr, maxConversationChars)
	}

	var sb strings.Builder
	sb.WriteString(classifySystemPrompt)
	sb.WriteString("\n\n--- Conversation ---\n")
	sb.WriteString(convStr)
	return sb.String()
}

// truncateConversation trims the oldest content from a conversation string so
// that the result is at most maxChars bytes, preserving the most recent lines.
func truncateConversation(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	// Drop from the front; keep the tail so recent messages are preserved.
	return "[...truncated...]\n" + s[len(s)-maxChars:]
}

// validateResult checks that enum fields contain only valid values.
// It also normalises TagsSuggested to a non-nil slice and truncates
// TitleSuggestion to 80 characters (schema constraint).
func validateResult(r *Result) error {
	if !validClassifications[r.Classification] {
		return fmt.Errorf("invalid classification %q (must be one of: bug, feature_request, question, other)", r.Classification)
	}
	if !validSeverities[r.SeverityGuess] {
		return fmt.Errorf("invalid severity_guess %q (must be one of: low, medium, high, critical, unknown)", r.SeverityGuess)
	}
	if r.TagsSuggested == nil {
		r.TagsSuggested = []string{}
	}
	// Truncate by code points, not bytes, to avoid splitting a UTF-8 rune.
	// The schema's maxLength:80 counts code points.
	if utf8.RuneCountInString(r.TitleSuggestion) > 80 {
		runes := []rune(r.TitleSuggestion)
		r.TitleSuggestion = string(runes[:80])
	}
	if r.Language == "" {
		r.Language = "en"
	}
	return nil
}

// stripCodeFences removes ```json ... ``` wrappers if present.
// The opening fence language tag is compared case-insensitively so that
// ```JSON and ```Json are treated the same as ```json.
func stripCodeFences(s string) string {
	// Strip opening fence with optional language tag (case-insensitive).
	if strings.HasPrefix(s, "```") {
		rest := s[3:] // after the opening ```
		// Find the end of the first line (the fence + optional lang tag).
		if idx := strings.Index(rest, "\n"); idx >= 0 {
			lang := strings.ToLower(strings.TrimSpace(rest[:idx]))
			if lang == "json" || lang == "" {
				s = rest[idx+1:]
			}
		} else {
			// No newline: bare ``` with no body — just strip the prefix.
			if strings.ToLower(strings.TrimSpace(rest)) == "json" || rest == "" {
				s = ""
			}
		}
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
