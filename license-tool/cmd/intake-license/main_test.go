package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"intake/license"
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
	pubB64, _ := os.ReadFile(pubPath)

	tmpl := license.License{LicenseID: "x", Tier: "pro", Adapters: []string{"zendesk"}}
	tb, _ := json.Marshal(tmpl)
	inPath := filepath.Join(dir, "t.json")
	_ = os.WriteFile(inPath, tb, 0o600)
	outPath := filepath.Join(dir, "lic.json")
	if err := doSign(inPath, keyPath, outPath, 10, &out); err != nil {
		t.Fatal(err)
	}

	// Tamper: add an adapter after signing.
	var m map[string]any
	signed, _ := os.ReadFile(outPath)
	_ = json.Unmarshal(signed, &m)
	m["adapters"] = []string{"zendesk", "linear"}
	tampered, _ := json.Marshal(m)
	_ = os.WriteFile(outPath, tampered, 0o600)

	if err := doVerify(outPath, string(bytes.TrimSpace(pubB64)), &out); err == nil {
		t.Fatal("doVerify accepted a tampered license; want error")
	}
}
