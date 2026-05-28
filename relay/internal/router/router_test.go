package router_test

import (
	"strings"
	"testing"
	"time"

	"intake/internal/payload"
	"intake/internal/router"

	"context"
	"intake/internal/adapter"
)

// stubAdapter is a no-op adapter with a fixed name.
type stubAdapter struct{ name string }

func (s *stubAdapter) Name() string                      { return s.name }
func (s *stubAdapter) RequiresLicense() bool             { return false }
func (s *stubAdapter) Configure(map[string]any) error    { return nil }
func (s *stubAdapter) HealthCheck(context.Context) error { return nil }
func (s *stubAdapter) Create(context.Context, *payload.IntakePayload) (*adapter.CreateResult, error) {
	return &adapter.CreateResult{AdapterName: s.name}, nil
}

func reg(names ...string) map[string]adapter.Adapter {
	m := make(map[string]adapter.Adapter, len(names))
	for _, n := range names {
		m[n] = &stubAdapter{name: n}
	}
	return m
}

// mkPayload builds a minimal payload with the given classification/severity/hint.
func mkPayload(class, sev string, hint *string) *payload.IntakePayload {
	return &payload.IntakePayload{
		RoutingHint: hint,
		Conversation: payload.Conversation{
			Classification: payload.ConversationClassification(class),
			SeverityGuess:  payload.ConversationSeverityGuess(sev),
			Messages:       []payload.Message{{Role: "user", Content: "x", Ts: time.Now()}},
		},
	}
}

func ptr(s string) *string { return &s }

func TestNew_DefaultNotRegistered_Errors(t *testing.T) {
	_, err := router.New(reg("chatwoot"), nil, "zendesk", nil)
	if err == nil {
		t.Fatal("expected error when default_adapter names an unregistered adapter")
	}
	if !strings.Contains(err.Error(), "zendesk") {
		t.Errorf("error should name the bad default; got %v", err)
	}
}

func TestNew_DanglingRuleDropped(t *testing.T) {
	// Rule points at zendesk (not registered) — must be dropped, New still succeeds.
	rules := []router.Rule{{Classification: []string{"bug"}, To: "zendesk"}}
	r, err := router.New(reg("chatwoot"), rules, "chatwoot", nil)
	if err != nil {
		t.Fatalf("New errored on a dangling rule (should drop+warn): %v", err)
	}
	// A bug payload should now fall through to the default (chatwoot), not zendesk.
	ad, err := r.Route(mkPayload("bug", "high", nil))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if ad.Name() != "chatwoot" {
		t.Errorf("dangling rule not dropped: routed to %q; want chatwoot", ad.Name())
	}
}

func TestRoute_RoutingHintWins(t *testing.T) {
	r, _ := router.New(reg("chatwoot", "fider"), nil, "chatwoot", nil)
	ad, err := r.Route(mkPayload("bug", "high", ptr("fider")))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if ad.Name() != "fider" {
		t.Errorf("routing_hint ignored: got %q; want fider", ad.Name())
	}
}

func TestRoute_UnknownHintFallsThrough(t *testing.T) {
	r, _ := router.New(reg("chatwoot"), nil, "chatwoot", nil)
	ad, err := r.Route(mkPayload("bug", "high", ptr("nonexistent")))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if ad.Name() != "chatwoot" {
		t.Errorf("unknown hint should fall through to default; got %q", ad.Name())
	}
}

func TestRoute_RuleMatch(t *testing.T) {
	rules := []router.Rule{
		{Classification: []string{"feature_request"}, To: "fider"},
		{Classification: []string{"bug"}, To: "chatwoot"},
	}
	r, _ := router.New(reg("chatwoot", "fider"), rules, "chatwoot", nil)
	ad, _ := r.Route(mkPayload("feature_request", "low", nil))
	if ad.Name() != "fider" {
		t.Errorf("rule match failed: got %q; want fider", ad.Name())
	}
}

func TestRoute_SeverityAndWildcard(t *testing.T) {
	// Rule matches any classification but only critical severity.
	rules := []router.Rule{{Severity: []string{"critical"}, To: "fider"}}
	r, _ := router.New(reg("chatwoot", "fider"), rules, "chatwoot", nil)
	if ad, _ := r.Route(mkPayload("bug", "critical", nil)); ad.Name() != "fider" {
		t.Errorf("severity wildcard-classification rule failed: got %q; want fider", ad.Name())
	}
	if ad, _ := r.Route(mkPayload("bug", "low", nil)); ad.Name() != "chatwoot" {
		t.Errorf("non-critical should fall to default; got %q", ad.Name())
	}
}

func TestRoute_DefaultFallback(t *testing.T) {
	r, _ := router.New(reg("chatwoot"), nil, "chatwoot", nil)
	ad, _ := r.Route(mkPayload("other", "unknown", nil))
	if ad.Name() != "chatwoot" {
		t.Errorf("default fallback failed: got %q", ad.Name())
	}
}
