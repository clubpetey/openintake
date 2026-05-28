package license_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"intake/license"
)

func newLicense() license.License {
	return license.License{
		LicenseID:    "lic_test_001",
		IssuedTo:     license.IssuedTo{Org: "Example Org", Email: "billing@example.com"},
		Tier:         "pro",
		Adapters:     []string{"zendesk", "linear"},
		MaxInstances: 3,
		IssuedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt:    time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestSignVerify_RoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	l := newLicense()
	if err := license.Sign(priv, &l); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if l.Signature == "" {
		t.Fatal("Sign did not set Signature")
	}
	blob, _ := json.Marshal(l)
	got, err := license.Verify(pub, blob)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.LicenseID != l.LicenseID || got.Tier != "pro" || len(got.Adapters) != 2 {
		t.Errorf("verified license mismatch: %+v", got)
	}
}

func TestVerify_TamperRejected(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	l := newLicense()
	_ = license.Sign(priv, &l)
	// Tamper: grant an extra adapter after signing.
	l.Adapters = append(l.Adapters, "salesforce")
	blob, _ := json.Marshal(l)
	if _, err := license.Verify(pub, blob); err == nil {
		t.Fatal("Verify accepted a tampered license; want error")
	}
}

func TestVerify_WrongKeyRejected(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	l := newLicense()
	_ = license.Sign(priv, &l)
	blob, _ := json.Marshal(l)
	if _, err := license.Verify(otherPub, blob); err == nil {
		t.Fatal("Verify accepted a wrong-key signature; want error")
	}
}

func TestVerify_MissingPrefix(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	l := newLicense()
	l.Signature = "not-prefixed"
	blob, _ := json.Marshal(l)
	if _, err := license.Verify(pub, blob); err == nil {
		t.Fatal("Verify accepted a signature without the ed25519: prefix; want error")
	}
}
