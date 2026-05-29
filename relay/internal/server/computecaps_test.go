package server

import (
	"reflect"
	"testing"

	"intake/internal/adapter"
	"intake/internal/adapter/webhook"
	"intake/internal/config"
)

func TestComputeAttachmentsCaps_DisabledReturnsNil(t *testing.T) {
	cfg := config.AttachmentsConfig{Enabled: false, AllowedMIMETypes: []string{"image/png"}}
	caps := ComputeAttachmentsCaps(cfg, nil)
	if caps != nil {
		t.Errorf("caps = %v; want nil when disabled", caps)
	}
}

func TestComputeAttachmentsCaps_EmptyAllowlistReturnsNil(t *testing.T) {
	cfg := config.AttachmentsConfig{Enabled: true, AllowedMIMETypes: []string{}}
	caps := ComputeAttachmentsCaps(cfg, nil)
	if caps != nil {
		t.Errorf("caps = %v; want nil when allowlist empty", caps)
	}
}

func TestComputeAttachmentsCaps_NoCapableAdaptersReturnsNil(t *testing.T) {
	cfg := config.AttachmentsConfig{
		Enabled:          true,
		AllowedMIMETypes: []string{"image/png"},
	}
	caps := ComputeAttachmentsCaps(cfg, nil) // no adapters
	if caps != nil {
		t.Errorf("caps = %v; want nil when no adapter advertises", caps)
	}
}

func TestComputeAttachmentsCaps_Intersection(t *testing.T) {
	// Real webhook adapter advertises all three; cfg permits two — output is the cfg ∩ adapter.
	cfg := config.AttachmentsConfig{
		Enabled:          true,
		MaxSizeBytes:     5_242_880,
		MaxTotalBytes:    10_485_760,
		AllowedMIMETypes: []string{"image/png", "image/webp"},
	}
	wh := webhook.New()
	caps := ComputeAttachmentsCaps(cfg, []adapter.Adapter{wh})
	if caps == nil {
		t.Fatal("caps = nil; want non-nil intersection")
	}
	want := []string{"image/png", "image/webp"}
	if !reflect.DeepEqual(caps.AllowedMIMETypes, want) {
		t.Errorf("AllowedMIMETypes = %v; want %v", caps.AllowedMIMETypes, want)
	}
	if caps.MaxSizeBytes != 5_242_880 || caps.MaxTotalBytes != 10_485_760 {
		t.Errorf("size caps mismatch: %+v", caps)
	}
}
