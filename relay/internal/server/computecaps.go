package server

import (
	"github.com/clubpetey/openintake/relay/internal/adapter"
	"github.com/clubpetey/openintake/relay/internal/config"
)

// ComputeAttachmentsCaps returns the CapabilitiesAttachments to advertise on
// /init. Returns nil when:
//   - cfg.Enabled is false, OR
//   - cfg.AllowedMIMETypes is empty, OR
//   - no enabled adapter implements CapableAdapter, OR
//   - the union across enabled adapters has zero overlap with cfg.AllowedMIMETypes.
//
// Otherwise returns the intersection: cfg.AllowedMIMETypes ∩ (union of
// enabled adapters' Capabilities().AcceptedMIMETypes). Order of the result
// follows cfg.AllowedMIMETypes (stable for the widget's enumeration).
//
// Called once at startup from main.go; the result lives in deps.AttachmentMIMEs
// (the published allowlist) and feeds initHandler's Capabilities emission.
func ComputeAttachmentsCaps(cfg config.AttachmentsConfig, enabled []adapter.Adapter) *CapabilitiesAttachments {
	if !cfg.Enabled || len(cfg.AllowedMIMETypes) == 0 {
		return nil
	}
	adapterUnion := make(map[string]bool)
	for _, ad := range enabled {
		c, ok := ad.(adapter.CapableAdapter)
		if !ok {
			continue
		}
		for _, m := range c.Capabilities().AcceptedMIMETypes {
			adapterUnion[m] = true
		}
	}
	if len(adapterUnion) == 0 {
		return nil
	}
	allowed := make([]string, 0, len(cfg.AllowedMIMETypes))
	for _, m := range cfg.AllowedMIMETypes {
		if adapterUnion[m] {
			allowed = append(allowed, m)
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	return &CapabilitiesAttachments{
		MaxSizeBytes:     cfg.MaxSizeBytes,
		MaxTotalBytes:    cfg.MaxTotalBytes,
		AllowedMIMETypes: allowed,
	}
}
