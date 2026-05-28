// Package router resolves a canonical payload to exactly one downstream adapter.
// Resolution order (PROJECT.md §8): routing_hint → first matching rule → default.
// The router holds a registry of the enabled+permitted adapters built in main.go;
// the license gate (3-vi) decides which paid adapters make it into that registry.
package router

import (
	"fmt"
	"log/slog"

	"intake/internal/adapter"
	"intake/internal/payload"
)

// Rule is the resolved form of config.Rule. Empty Classification/Severity = wildcard.
type Rule struct {
	Classification []string
	Severity       []string
	To             string
}

// Router selects an adapter for a payload.
type Router struct {
	registry map[string]adapter.Adapter
	rules    []Rule
	def      string
	logger   *slog.Logger
}

// New builds a Router. It errors if defaultName does not name a registered adapter
// (a relay with a broken default is useless — fatal at startup). Rules whose To
// names an unregistered adapter are dropped with a warning (graceful free-mode).
func New(registry map[string]adapter.Adapter, rules []Rule, defaultName string, logger *slog.Logger) (*Router, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if _, ok := registry[defaultName]; !ok {
		return nil, fmt.Errorf("router: default_adapter %q is not a registered/enabled adapter", defaultName)
	}
	kept := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if _, ok := registry[r.To]; !ok {
			logger.Warn("router: dropping rule targeting unregistered adapter",
				"to", r.To, "reason", "adapter not enabled or not licensed")
			continue
		}
		kept = append(kept, r)
	}
	return &Router{registry: registry, rules: kept, def: defaultName, logger: logger}, nil
}

// Route resolves p to one registered adapter. Never returns (nil, nil).
func (r *Router) Route(p *payload.IntakePayload) (adapter.Adapter, error) {
	// 1. routing_hint, if it names a registered adapter.
	if p.RoutingHint != nil && *p.RoutingHint != "" {
		if ad, ok := r.registry[*p.RoutingHint]; ok {
			return ad, nil
		}
	}
	// 2. first matching rule.
	class := string(p.Conversation.Classification)
	sev := string(p.Conversation.SeverityGuess)
	for _, rule := range r.rules {
		if matches(rule.Classification, class) && matches(rule.Severity, sev) {
			return r.registry[rule.To], nil // guaranteed registered (dangling dropped in New)
		}
	}
	// 3. default. New guarantees r.def is registered; the guard below is defensive
	// in case that invariant is ever violated.
	if ad, ok := r.registry[r.def]; ok {
		return ad, nil
	}
	return nil, fmt.Errorf("router: no adapter resolved and default %q missing", r.def)
}

// matches reports whether want is empty (wildcard) or contains got.
func matches(want []string, got string) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		if w == got {
			return true
		}
	}
	return false
}
