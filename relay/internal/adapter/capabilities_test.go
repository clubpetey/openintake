package adapter_test

import (
	"reflect"
	"testing"

	"intake/internal/adapter"
	"intake/internal/adapter/chatwoot"
	"intake/internal/adapter/fider"
	"intake/internal/adapter/linear"
	"intake/internal/adapter/webhook"
	"intake/internal/adapter/zendesk"
)

// TestCapableAdapter_AllFiveAdaptersAdvertiseV0List asserts each of the five
// Phase 1+3 adapters implements the optional CapableAdapter interface and
// returns the v0 MIME list. This is the per-adapter row of design spec §5.2.
func TestCapableAdapter_AllFiveAdaptersAdvertiseV0List(t *testing.T) {
	want := []string{"image/png", "image/jpeg", "image/webp"}
	cases := []struct {
		name string
		ad   adapter.Adapter
	}{
		{"webhook", webhook.New()},
		{"chatwoot", chatwoot.New()},
		{"fider", fider.New()},
		{"linear", linear.New()},
		{"zendesk", zendesk.New()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cap, ok := c.ad.(adapter.CapableAdapter)
			if !ok {
				t.Fatalf("%s does not implement adapter.CapableAdapter", c.name)
			}
			got := cap.Capabilities().AcceptedMIMETypes
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s Capabilities().AcceptedMIMETypes = %v; want %v", c.name, got, want)
			}
		})
	}
}
