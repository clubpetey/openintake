package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	pubkglicense "intake/license"

	"intake/internal/config"
)

func tmpStatePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state.json")
}

// signedBlob signs a license with priv and returns its JSON bytes.
func signedBlob(t *testing.T, priv ed25519.PrivateKey, adapters []string, tier string, expires time.Time) []byte {
	t.Helper()
	l := pubkglicense.License{
		LicenseID: "lic_x", IssuedTo: pubkglicense.IssuedTo{Org: "O", Email: "e@x.com"},
		Tier: tier, Adapters: adapters, MaxInstances: 1,
		IssuedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ExpiresAt: expires,
	}
	if err := pubkglicense.Sign(priv, &l); err != nil {
		t.Fatalf("sign: %v", err)
	}
	b, _ := json.Marshal(l)
	return b
}

func TestLoad_NoLicense_StartsTrial(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	st, err := loadWithKey(pub, config.LicenseConfig{}, sp, now)
	if err != nil {
		t.Fatalf("loadWithKey: %v", err)
	}
	if st.Mode != "trial" {
		t.Fatalf("Mode = %q; want trial", st.Mode)
	}
	if !st.Permits("zendesk") {
		t.Error("trial should permit zendesk")
	}
	// State file written.
	if _, err := os.Stat(sp); err != nil {
		t.Errorf("trial state file not written: %v", err)
	}
}

func TestLoad_TrialExpired_Free(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = writeTrialState(sp, trialState{TrialStartedAt: start})
	now := start.Add(15 * 24 * time.Hour) // 15 days later — past the 14-day trial
	st, err := loadWithKey(pub, config.LicenseConfig{}, sp, now)
	if err != nil {
		t.Fatalf("loadWithKey: %v", err)
	}
	if st.Mode != "free" {
		t.Fatalf("Mode = %q; want free", st.Mode)
	}
	if st.Permits("zendesk") {
		t.Error("expired trial must NOT permit zendesk")
	}
}

func TestLoad_ValidLicense_Licensed(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	blob := signedBlob(t, priv, []string{"zendesk"}, "pro", now.Add(365*24*time.Hour))
	lf := filepath.Join(t.TempDir(), "license.json")
	_ = os.WriteFile(lf, blob, 0o600)

	st, err := loadWithKey(pub, config.LicenseConfig{File: lf}, sp, now)
	if err != nil {
		t.Fatalf("loadWithKey: %v", err)
	}
	if st.Mode != "licensed" {
		t.Fatalf("Mode = %q; want licensed", st.Mode)
	}
	if !st.Permits("zendesk") {
		t.Error("licensed should permit zendesk (listed)")
	}
	if st.Permits("linear") {
		t.Error("licensed must NOT permit linear (not listed)")
	}
}

func TestLoad_ExpiredLicense_DowngradesToFree(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	blob := signedBlob(t, priv, []string{"zendesk"}, "pro", now.Add(-24*time.Hour)) // expired yesterday
	lf := filepath.Join(t.TempDir(), "license.json")
	_ = os.WriteFile(lf, blob, 0o600)

	st, err := loadWithKey(pub, config.LicenseConfig{File: lf}, sp, now)
	if err != nil {
		t.Fatalf("expired license must NOT be fatal; got error: %v", err)
	}
	if st.Mode != "free" {
		t.Fatalf("Mode = %q; want free (expired downgrade)", st.Mode)
	}
	if st.Permits("zendesk") {
		t.Error("expired license must not permit paid adapters")
	}
}

func TestLoad_BadSignature_Fatal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	blob := signedBlob(t, priv, []string{"zendesk"}, "pro", now.Add(time.Hour))
	// Tamper after signing.
	var m map[string]any
	_ = json.Unmarshal(blob, &m)
	m["adapters"] = []string{"zendesk", "linear", "salesforce"}
	tampered, _ := json.Marshal(m)
	lf := filepath.Join(t.TempDir(), "license.json")
	_ = os.WriteFile(lf, tampered, 0o600)

	_, err := loadWithKey(pub, config.LicenseConfig{File: lf}, sp, now)
	if err == nil {
		t.Fatal("bad signature must be fatal (error); got nil")
	}
}

func TestLoad_HostedTier_GrantsAll(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	blob := signedBlob(t, priv, nil, "hosted", now.Add(time.Hour))
	lf := filepath.Join(t.TempDir(), "license.json")
	_ = os.WriteFile(lf, blob, 0o600)

	st, _ := loadWithKey(pub, config.LicenseConfig{File: lf}, sp, now)
	if !st.Permits("zendesk") || !st.Permits("linear") {
		t.Error("hosted tier must grant all paid adapters")
	}
}
