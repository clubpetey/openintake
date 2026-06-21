package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	pubkglicense "github.com/clubpetey/openintake/relay/license"

	"github.com/clubpetey/openintake/relay/internal/config"
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

// --- New tests (Fix 6) ---

// 6a. Malformed state.json → trial restarts (not fatal).
func TestLoad_MalformedState_TrialRestarts(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	// Write garbage JSON to the state file.
	if err := os.WriteFile(sp, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	st, err := loadWithKey(pub, config.LicenseConfig{}, sp, now)
	if err != nil {
		t.Fatalf("malformed state must not be fatal; got error: %v", err)
	}
	if st.Mode != "trial" {
		t.Fatalf("Mode = %q; want trial (malformed state → restart)", st.Mode)
	}
}

// 6b. Expiry boundary: ExpiresAt == now → expired (free); ExpiresAt == now+1h → licensed.
func TestLoad_ExpiryBoundary(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Exactly at now → expired.
	blob := signedBlob(t, priv, []string{"zendesk"}, "pro", now)
	lf := filepath.Join(t.TempDir(), "license.json")
	_ = os.WriteFile(lf, blob, 0o600)
	st, err := loadWithKey(pub, config.LicenseConfig{File: lf}, sp, now)
	if err != nil {
		t.Fatalf("boundary-expired must not error; got: %v", err)
	}
	if st.Mode != "free" {
		t.Fatalf("ExpiresAt==now: Mode = %q; want free (exclusive expiry)", st.Mode)
	}

	// now+1h → still licensed.
	blob2 := signedBlob(t, priv, []string{"zendesk"}, "pro", now.Add(time.Hour))
	lf2 := filepath.Join(t.TempDir(), "license.json")
	_ = os.WriteFile(lf2, blob2, 0o600)
	st2, err := loadWithKey(pub, config.LicenseConfig{File: lf2}, sp, now)
	if err != nil {
		t.Fatalf("boundary-licensed must not error; got: %v", err)
	}
	if st2.Mode != "licensed" {
		t.Fatalf("ExpiresAt==now+1h: Mode = %q; want licensed", st2.Mode)
	}
}

// 6c. INTAKE_LICENSE and INTAKE_LICENSE_FILE both set → INTAKE_LICENSE wins.
func TestLoad_EnvPrecedence_IntakeLicenseWins(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// INTAKE_LICENSE → zendesk grant.
	blobZendesk := signedBlob(t, priv, []string{"zendesk"}, "pro", now.Add(365*24*time.Hour))
	b64 := base64.StdEncoding.EncodeToString(blobZendesk)

	// INTAKE_LICENSE_FILE → linear-only grant.
	blobLinear := signedBlob(t, priv, []string{"linear"}, "pro", now.Add(365*24*time.Hour))
	lf := filepath.Join(t.TempDir(), "license.json")
	_ = os.WriteFile(lf, blobLinear, 0o600)

	t.Setenv("INTAKE_LICENSE", b64)
	t.Setenv("INTAKE_LICENSE_FILE", lf)

	// cfg.File empty → env vars govern.
	st, err := loadWithKey(pub, config.LicenseConfig{}, sp, now)
	if err != nil {
		t.Fatalf("loadWithKey: %v", err)
	}
	if st.Mode != "licensed" {
		t.Fatalf("Mode = %q; want licensed", st.Mode)
	}
	if !st.Permits("zendesk") {
		t.Error("INTAKE_LICENSE (zendesk) should win; zendesk not permitted")
	}
	if st.Permits("linear") {
		t.Error("INTAKE_LICENSE_FILE (linear) must not win; linear should not be permitted")
	}
}

// 6d. Explicit cfg.File unreadable → fatal error.
func TestLoad_ExplicitFile_Unreadable_Fatal(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp := tmpStatePath(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	_, err := loadWithKey(pub, config.LicenseConfig{File: "/nonexistent/path/license.json"}, sp, now)
	if err == nil {
		t.Fatal("explicit cfg.File pointing to nonexistent path must return an error")
	}
}

// 6e. Corrupt embedded key detection via embeddedPublicKey().
// The real constant is "" so we test the empty → (nil, nil) contract directly.
// For invalid-input coverage we test the decode path via base64Decode helper.
func TestEmbeddedPublicKey_DecodesConstantState(t *testing.T) {
	// Verifies embeddedPublicKey() correctly reflects whatever state the source
	// constant is in. Pre-keygen the constant is ""; after the maintainer keygen
	// pause it carries a valid 32-byte Ed25519 public key (base64).
	//
	// Either way the function must NOT return a "(nil, nil) but the constant was
	// non-empty" combination — that was the silent-failure shape L009 hardened
	// against.
	key, err := embeddedPublicKey()
	if EmbeddedPublicKeyBase64 == "" {
		if key != nil || err != nil {
			t.Errorf("empty constant: got (key=%v, err=%v); want (nil, nil)", key, err)
		}
		return
	}
	// Non-empty constant: a valid embed must decode to a 32-byte key with no error.
	// A non-empty-but-invalid constant would (per L009) return a non-nil error.
	if err != nil {
		t.Fatalf("non-empty embedded constant did not decode: %v "+
			"(constant should be standard-base64 of a 32-byte Ed25519 public key)", err)
	}
	if len(key) != ed25519.PublicKeySize {
		t.Errorf("embedded key wrong size: got %d bytes; want %d", len(key), ed25519.PublicKeySize)
	}
}

func TestBase64Decode_InvalidInput_ReturnsError(t *testing.T) {
	// Ensure the decode helper surfaces an error for bad base64 — this is the code
	// path that embeddedPublicKey() uses to detect a corrupt embedded key.
	_, err := base64Decode("!!!not-valid-base64!!!")
	if err == nil {
		t.Error("invalid base64 must return an error from base64Decode")
	}
}
