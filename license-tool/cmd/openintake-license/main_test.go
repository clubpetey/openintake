package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/clubpetey/openintake/relay/license"
)

// TestRoundTrip_KeygenSignVerify proves the CLI signs a license that the shared
// intake/license.Verify accepts — the cross-module canonicalization lock. Uses a
// throwaway keypair written by doKeygen; no real/embedded key involved.
func TestRoundTrip_KeygenSignVerify(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "priv.key")
	pubPath := filepath.Join(dir, "pub.txt")

	// keygen → writes private key file, prints public key base64 to out.
	var out bytes.Buffer
	if err := doKeygen(keyPath, pubPath, &out); err != nil {
		t.Fatalf("doKeygen: %v", err)
	}
	pubB64, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read pub: %v", err)
	}

	// Write a license template.
	tmpl := license.License{
		LicenseID: "lic_rt_001",
		IssuedTo:  license.IssuedTo{Org: "RoundTrip Inc", Email: "ops@rt.example"},
		Tier:      "pro",
		Adapters:  []string{"zendesk", "linear"},
	}
	tmplBytes, _ := json.Marshal(tmpl)
	inPath := filepath.Join(dir, "template.json")
	if err := os.WriteFile(inPath, tmplBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "license.json")

	// sign --days 30
	if err := doSign(inPath, keyPath, outPath, 30, &out); err != nil {
		t.Fatalf("doSign: %v", err)
	}

	// The signed license verifies with the public key via the SHARED package.
	pub, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(pubB64)))
	if err != nil {
		t.Fatalf("decode pub: %v", err)
	}
	signed, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	lic, err := license.Verify(pub, signed)
	if err != nil {
		t.Fatalf("shared Verify rejected a CLI-signed license: %v", err)
	}
	if lic.Tier != "pro" || len(lic.Adapters) != 2 {
		t.Errorf("verified license mismatch: %+v", lic)
	}
	// expires_at ~ now + 30 days.
	wantMin := time.Now().UTC().Add(29 * 24 * time.Hour)
	if !lic.ExpiresAt.After(wantMin) {
		t.Errorf("expires_at = %v; want ~30 days out", lic.ExpiresAt)
	}

	// doVerify on the good file returns nil.
	if err := doVerify(outPath, string(bytes.TrimSpace(pubB64)), &out); err != nil {
		t.Errorf("doVerify rejected a valid license: %v", err)
	}
}

func TestVerify_TamperRejected(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "priv.key")
	pubPath := filepath.Join(dir, "pub.txt")
	var out bytes.Buffer
	if err := doKeygen(keyPath, pubPath, &out); err != nil {
		t.Fatal(err)
	}
	pubB64, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read pubPath: %v", err)
	}

	tmpl := license.License{LicenseID: "x", Tier: "pro", Adapters: []string{"zendesk"}}
	tb, err := json.Marshal(tmpl)
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}
	inPath := filepath.Join(dir, "t.json")
	if err := os.WriteFile(inPath, tb, 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	outPath := filepath.Join(dir, "lic.json")
	if err := doSign(inPath, keyPath, outPath, 10, &out); err != nil {
		t.Fatal(err)
	}

	// Tamper: add an adapter after signing.
	var m map[string]any
	signed, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read signed license: %v", err)
	}
	if err := json.Unmarshal(signed, &m); err != nil {
		t.Fatalf("unmarshal signed license: %v", err)
	}
	m["adapters"] = []string{"zendesk", "linear"}
	tampered, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal tampered license: %v", err)
	}
	if err := os.WriteFile(outPath, tampered, 0o600); err != nil {
		t.Fatalf("write tampered license: %v", err)
	}

	if err := doVerify(outPath, string(bytes.TrimSpace(pubB64)), &out); err == nil {
		t.Fatal("doVerify accepted a tampered license; want error")
	}
}

// TestVerify_WrongKeyRejected signs a license with keypair A and verifies with
// keypair B's public key — must be rejected.
func TestVerify_WrongKeyRejected(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	// Keypair A.
	keyA := filepath.Join(dir, "priv_a.key")
	pubA := filepath.Join(dir, "pub_a.txt")
	if err := doKeygen(keyA, pubA, &out); err != nil {
		t.Fatalf("doKeygen A: %v", err)
	}

	// Sign a template with A's private key.
	tmpl := license.License{LicenseID: "wkr_001", Tier: "pro", Adapters: []string{"zendesk"}}
	tb, err := json.Marshal(tmpl)
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}
	inPath := filepath.Join(dir, "template.json")
	if err := os.WriteFile(inPath, tb, 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	outPath := filepath.Join(dir, "license.json")
	if err := doSign(inPath, keyA, outPath, 30, &out); err != nil {
		t.Fatalf("doSign with A: %v", err)
	}

	// Keypair B.
	keyB := filepath.Join(dir, "priv_b.key")
	pubB := filepath.Join(dir, "pub_b.txt")
	if err := doKeygen(keyB, pubB, &out); err != nil {
		t.Fatalf("doKeygen B: %v", err)
	}
	pubBBytes, err := os.ReadFile(pubB)
	if err != nil {
		t.Fatalf("read pub B: %v", err)
	}

	// Verifying A-signed license with B's public key must fail.
	if err := doVerify(outPath, string(bytes.TrimSpace(pubBBytes)), &out); err == nil {
		t.Fatal("doVerify accepted license signed by key A when verified with key B; want error")
	}
}

// TestSign_MalformedKeyFile exercises the base64/size validation path in doSign.
func TestSign_MalformedKeyFile(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	// Write a non-base64 string as the key file.
	keyPath := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(keyPath, []byte("not-a-key"), 0o600); err != nil {
		t.Fatalf("write bad key: %v", err)
	}

	// A minimal template is needed so doSign gets past the template-read step.
	tmpl := license.License{LicenseID: "malf_001", Tier: "pro", Adapters: []string{"zendesk"}}
	tb, err := json.Marshal(tmpl)
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}
	inPath := filepath.Join(dir, "template.json")
	if err := os.WriteFile(inPath, tb, 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	outPath := filepath.Join(dir, "license.json")

	if err := doSign(inPath, keyPath, outPath, 30, &out); err == nil {
		t.Fatal("doSign with malformed key file succeeded; want error")
	}
}

// TestKeygen_PrivateKeyPerms asserts that the private key file is created with
// mode 0600. Skipped on Windows where file permission bits are not enforced.
func TestKeygen_PrivateKeyPerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file perms not enforced on Windows")
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "priv.key")
	pubPath := filepath.Join(dir, "pub.txt")
	var out bytes.Buffer
	if err := doKeygen(keyPath, pubPath, &out); err != nil {
		t.Fatalf("doKeygen: %v", err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("private key file mode = %04o; want 0600", got)
	}
}
