package adapter

import (
	"context"

	"intake/internal/payload"
)

// CreateResult is returned by Adapter.Create on success.
type CreateResult struct {
	ExternalID  string
	ExternalURL string
	AdapterName string
	CreatedAt   string // ISO-8601 / RFC3339
}

// Adapter is the seam between the relay and downstream ticket systems.
// Implementations: webhook (Phase 1), chatwoot/fider/zendesk/linear (Phase 3).
// This interface is FROZEN in 1-iv; do not change without re-smoking all dependents.
type Adapter interface {
	// Name returns a short identifier used in logs and SubmitResponse.adapter_name.
	Name() string
	// RequiresLicense reports whether the adapter needs a license key (Phase 3 gate).
	RequiresLicense() bool
	// Configure applies a map of adapter-specific settings (from config.AdaptersConfig.*).
	Configure(config map[string]any) error
	// Create sends the validated canonical payload to the downstream system.
	Create(ctx context.Context, p *payload.IntakePayload) (*CreateResult, error)
	// HealthCheck probes the downstream system without creating a ticket.
	HealthCheck(ctx context.Context) error
}
