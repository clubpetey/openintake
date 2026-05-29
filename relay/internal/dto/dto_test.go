package dto_test

import (
	"encoding/json"
	"testing"

	"intake/internal/dto"
)

func TestSubmitRequest_AttachmentsRoundTrip(t *testing.T) {
	in := dto.SubmitRequest{
		Messages: []dto.TurnMessage{{Role: "user", Content: "hi"}},
		Attachments: []dto.SubmitAttachment{
			{Type: "screenshot", MIMEType: "image/png", URL: "data:image/png;base64,iVBORw0KGgo=", Label: "shot"},
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out dto.SubmitRequest
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Attachments) != 1 {
		t.Fatalf("attachments len = %d; want 1", len(out.Attachments))
	}
	if out.Attachments[0].MIMEType != "image/png" {
		t.Errorf("MIMEType = %q; want image/png", out.Attachments[0].MIMEType)
	}
	if out.Attachments[0].URL != "data:image/png;base64,iVBORw0KGgo=" {
		t.Errorf("URL = %q; want data: URL", out.Attachments[0].URL)
	}
}

func TestSubmitRequest_AttachmentsOmitEmpty(t *testing.T) {
	in := dto.SubmitRequest{Messages: []dto.TurnMessage{{Role: "user", Content: "x"}}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(raw); contains(got, "attachments") {
		t.Errorf("marshalled JSON should omit attachments when empty; got %s", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
