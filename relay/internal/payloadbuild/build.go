// Package payloadbuild assembles and validates the canonical payload.IntakePayload
// from a SubmitRequest, classify.Result, and auth.SessionContext.
//
// Schema validation is performed at runtime against schema/payload.v1.json
// (embedded). This is the L003 mitigation: the Go type system does NOT enforce
// the schema_version const (go-jsonschema issue); runtime validation does.
//
//go:generate cp ../../../schema/payload.v1.json schema.json
package payloadbuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	_ "embed"

	"github.com/google/uuid"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"intake/internal/auth"
	"intake/internal/classify"
	"intake/internal/dto"
	"intake/internal/payload"
)

//go:embed schema.json
var schemaBytes []byte

// compiledSchema is parsed once at package init to avoid per-request overhead.
var compiledSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		panic(fmt.Sprintf("payloadbuild: parse embedded schema JSON: %v", err))
	}
	c := jsonschema.NewCompiler()
	// AssertFormat enables format validation (uuid, date-time, uri) for draft-2020-12.
	c.AssertFormat()
	if err := c.AddResource("https://intake.dev/schema/payload.v1.json", doc); err != nil {
		panic(fmt.Sprintf("payloadbuild: add embedded schema resource: %v", err))
	}
	compiledSchema, err = c.Compile("https://intake.dev/schema/payload.v1.json")
	if err != nil {
		panic(fmt.Sprintf("payloadbuild: compile embedded schema: %v", err))
	}
}

// Builder assembles and validates canonical payloads.
type Builder struct {
	widgetVersionDefault string
}

// New returns a Builder. widgetVersionDefault is used when ClientInfo.WidgetVersion is empty.
func New(widgetVersionDefault string) *Builder {
	return &Builder{widgetVersionDefault: widgetVersionDefault}
}

// Build assembles a payload.IntakePayload from the given inputs, then validates it
// against the embedded schema/payload.v1.json. Returns the validated payload or
// a descriptive error (suitable for a 400 response to the client).
func (b *Builder) Build(
	_ context.Context,
	req *dto.SubmitRequest,
	result *classify.Result,
	sess *auth.SessionContext,
	submissionID string, // pre-generated UUID
	submittedAt time.Time,
) (*payload.IntakePayload, error) {
	wv := req.Client.WidgetVersion
	if wv == "" {
		wv = b.widgetVersionDefault
	}

	// Build conversation messages.
	msgs := make([]payload.Message, 0, len(req.Messages))
	// All messages get the same timestamp (submitted_at) because the relay is
	// stateless between turns and per-message timestamps are not in SubmitRequest.
	// Phase 5 may add per-message timestamps when the client sends them.
	for _, m := range req.Messages {
		role := payload.MessageRoleUser
		if m.Role == "assistant" {
			role = payload.MessageRoleAssistant
		}
		msgs = append(msgs, payload.Message{
			Role:    role,
			Content: m.Content,
			Ts:      submittedAt,
		})
	}

	// Map classify.Result enum strings to payload typed consts.
	classification, err := mapClassification(result.Classification)
	if err != nil {
		return nil, fmt.Errorf("payloadbuild: %w", err)
	}
	severity, err := mapSeverity(result.SeverityGuess)
	if err != nil {
		return nil, fmt.Errorf("payloadbuild: %w", err)
	}

	// Map auth mode.
	authMode, err := mapAuthMode(sess.AuthMode)
	if err != nil {
		return nil, fmt.Errorf("payloadbuild: %w", err)
	}

	p := &payload.IntakePayload{
		SchemaVersion: "1.0", // const — also enforced by schema validation below (L003 mitigation)
		Submission: payload.Submission{
			Id:          submissionID,
			SubmittedAt: submittedAt,
			TenantId:    nil, // Phase 3 (multi-tenant)
		},
		Client: payload.Client{
			WidgetVersion: wv,
			SessionId:     sess.SessionID,
			Url:           req.Client.URL,
			Referrer:      req.Client.Referrer,
			UserAgent:     req.Client.UserAgent,
			Viewport: payload.Viewport{
				W: req.Client.Viewport.W,
				H: req.Client.Viewport.H,
			},
			Locale: req.Client.Locale,
		},
		User: payload.User{
			AuthMode:    authMode,
			Id:          sess.UserID,
			Email:       sess.Email,
			DisplayName: sess.DisplayName,
			Verified:    sess.Verified,
			Custom:      req.UserClaims,
		},
		Conversation: payload.Conversation{
			Messages:        msgs,
			Summary:         result.Summary,
			TitleSuggestion: result.TitleSuggestion,
			Classification:  classification,
			SeverityGuess:   severity,
			TagsSuggested:   result.TagsSuggested,
			Language:        result.Language,
		},
		RoutingHint: req.RoutingHint,
	}

	// Copy context if non-empty.
	if req.Context.AppContext != nil || req.Context.PageMetadata != nil {
		p.Context = &payload.Context{
			AppContext:   req.Context.AppContext,
			PageMetadata: req.Context.PageMetadata,
		}
	}

	// Schema validation (runtime, L003 mitigation).
	if err := validateAgainstSchema(p); err != nil {
		return nil, fmt.Errorf("payloadbuild: schema validation failed: %w", err)
	}

	return p, nil
}

// validateAgainstSchema marshals p to JSON then validates against the compiled schema.
func validateAgainstSchema(p *payload.IntakePayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal for validation: %w", err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("unmarshal for validation: %w", err)
	}
	if err := compiledSchema.Validate(v); err != nil {
		return err
	}
	return nil
}

// mapClassification converts a classify.Result string to the typed payload const.
func mapClassification(s string) (payload.ConversationClassification, error) {
	switch s {
	case "bug":
		return payload.ConversationClassificationBug, nil
	case "feature_request":
		return payload.ConversationClassificationFeatureRequest, nil
	case "question":
		return payload.ConversationClassificationQuestion, nil
	case "other":
		return payload.ConversationClassificationOther, nil
	default:
		return "", fmt.Errorf("unknown classification %q", s)
	}
}

// mapSeverity converts a classify.Result string to the typed payload const.
func mapSeverity(s string) (payload.ConversationSeverityGuess, error) {
	switch s {
	case "low":
		return payload.ConversationSeverityGuessLow, nil
	case "medium":
		return payload.ConversationSeverityGuessMedium, nil
	case "high":
		return payload.ConversationSeverityGuessHigh, nil
	case "critical":
		return payload.ConversationSeverityGuessCritical, nil
	case "unknown":
		return payload.ConversationSeverityGuessUnknown, nil
	default:
		return "", fmt.Errorf("unknown severity_guess %q", s)
	}
}

// mapAuthMode converts an auth.SessionContext AuthMode string to the typed payload const.
func mapAuthMode(s string) (payload.UserAuthMode, error) {
	switch s {
	case "anonymous":
		return payload.UserAuthModeAnonymous, nil
	case "email":
		return payload.UserAuthModeEmail, nil
	case "sso":
		return payload.UserAuthModeSso, nil
	default:
		return "", fmt.Errorf("unknown auth_mode %q", s)
	}
}

// NewSubmissionID generates a new UUID for submission.id.
// Exposed so callers (submit handler) can pre-generate the ID.
func NewSubmissionID() string {
	return uuid.NewString()
}

// CanonicalSchemaBytes returns the embedded schema bytes.
// Used by tests to assert the embedded copy is identical to the canonical file.
func CanonicalSchemaBytes() []byte {
	return schemaBytes
}

// schemaFromOS reads a schema from the given path (for test comparison).
func schemaFromOS(path string) ([]byte, error) {
	return os.ReadFile(path)
}
