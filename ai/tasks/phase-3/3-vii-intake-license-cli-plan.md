# 3-vii intake-license CLI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the maintainer-only `intake-license` CLI (`license-tool/`, module `intake-license-tool`) with `keygen`, `sign`, and `verify` subcommands, reusing the relay's importable `relay/license` package (from 3-vi) via a `replace intake => ../relay` directive so the CLI signs with exactly the same canonicalization the relay verifies. Lock the two modules together with a credit-free keygen→sign→verify round-trip test using a throwaway keypair.

**Architecture:** `license-tool/go.mod` adds `require intake v0.0.0` + `replace intake => ../relay`, giving the CLI access to `intake/license` (License struct + `Canonicalize`/`Sign`/`Verify`). `main.go` dispatches `os.Args[1]` to small testable functions (`doKeygen`/`doSign`/`doVerify`) that take explicit args + an `io.Writer`. `keygen` emits an Ed25519 keypair (private key to a 0600 file; public key base64 to stdout for embedding). `sign` reads a license template JSON, loads the private key, stamps `issued_at`/`expires_at`, and writes a signed license. `verify` checks a signed license against a base64 public key. The tool is **maintainer-only** and excluded from all release artifacts (Q10).

**Tech Stack:** Go 1.23.2 (`license-tool` module). Standard library (`crypto/ed25519`, `crypto/rand`, `encoding/base64`, `encoding/json`, `flag`, `os`, `time`) + `intake/license` (local, via replace). No external dependency.

---

## Design References

- README §8.5 — `relay/license` signatures the CLI consumes (frozen in 3-vi)
- README §5 — the `replace intake => ../relay` directive (the only go.mod change in the phase)
- README §6 — build-fail: the CLI must not land in any release artifact; CLI must build/test green
- Design spec §2.2 (package split), §2.3 (canonicalization + golden round-trip), §10 steps 1–2 (keygen/sign pauses)
- PROJECT.md §12 (key management: keypair generated once, private key offline, public key embedded; tool maintainer-only), §14 (`license-tool/` layout), decomposition Q10
- Depends on: 3-vi (`relay/license` must exist). Current stub: `license-tool/cmd/intake-license/main.go`, `license-tool/go.mod`

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `license-tool/go.mod` | Modify | Add `require intake v0.0.0` + `replace intake => ../relay` |
| `license-tool/cmd/intake-license/main.go` | Modify | Subcommand dispatch + `doKeygen`/`doSign`/`doVerify` |
| `license-tool/cmd/intake-license/main_test.go` | Create | keygen→sign→verify round-trip (throwaway key); tamper rejects; sign sets expiry |

---

## Tasks

### Task 1: Wire the module to share `relay/license` via replace

**Files:** Modify `license-tool/go.mod`

- [ ] **Step 1: Edit `license-tool/go.mod`**

Replace the contents of `license-tool/go.mod` with:

```
module intake-license-tool

go 1.23

toolchain go1.23.2

require intake v0.0.0

replace intake => ../relay
```

- [ ] **Step 2: Tidy and confirm only the local replace is added**

```
cd C:/src/ai/intake/license-tool && go mod tidy && echo TIDY_OK
```

Expected: `TIDY_OK`. `go.mod` keeps the `require intake v0.0.0` + `replace intake => ../relay`. Because `intake/license` imports only the standard library, the CLI pulls **no external compiled dependency**. `go mod tidy` may add `intake`'s module-graph entries to `go.sum` (graph completeness) — that is expected and adds nothing to the binary. If tidy tries to add an unrelated external module, stop and investigate.

- [ ] **Step 3: Confirm the shared package imports**

```
cd C:/src/ai/intake/license-tool && go build ./... && echo BUILD_OK
```

Expected: `BUILD_OK` (the current stub still builds; this just proves the module graph resolves with the replace directive).

- [ ] **Step 4: Commit**

```
cd C:/src/ai/intake/license-tool && git add go.mod go.sum
git commit -m "build(license-tool): share relay/license via replace intake => ../relay (3-vii)"
```

---

### Task 2: Implement the CLI (keygen / sign / verify) — TDD

**Files:** Create `license-tool/cmd/intake-license/main_test.go`, then rewrite `license-tool/cmd/intake-license/main.go`

- [ ] **Step 1: Write the failing test**

Create `license-tool/cmd/intake-license/main_test.go` (`package main` — white-box, exercises the helper functions directly):

```go
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
```

- [ ] **Step 2: Run to verify failure**

```
cd C:/src/ai/intake/license-tool && go test ./... -v
```

Expected: compile errors (`doKeygen`/`doSign`/`doVerify` undefined). MUST fail.

- [ ] **Step 3: Rewrite `main.go`**

Replace `license-tool/cmd/intake-license/main.go` with:

```go
// Command intake-license is the MAINTAINER-ONLY license tool. It is NOT distributed
// (Q10): excluded from all release artifacts (goreleaser ignore / not published).
// It signs licenses with the maintainer's offline Ed25519 private key, using the
// SAME relay/license canonicalization the relay verifies (shared via replace).
//
//	intake-license keygen [--key priv.key] [--pub pub.txt]
//	intake-license sign   --in template.json --key priv.key [--out license.json] [--days 365]
//	intake-license verify --in license.json --pubkey <base64>
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"intake/license"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		keyPath, pubPath := "intake-license-private.key", "intake-license-public.txt"
		parseFlags(os.Args[2:], map[string]*string{"key": &keyPath, "pub": &pubPath})
		err = doKeygen(keyPath, pubPath, os.Stdout)
	case "sign":
		in, key, out, days := "", "", "license.json", "365"
		parseFlags(os.Args[2:], map[string]*string{"in": &in, "key": &key, "out": &out, "days": &days})
		if in == "" || key == "" {
			err = fmt.Errorf("sign: --in and --key are required")
			break
		}
		var d int
		if _, e := fmt.Sscanf(days, "%d", &d); e != nil {
			err = fmt.Errorf("sign: --days must be an integer: %v", e)
			break
		}
		err = doSign(in, key, out, d, os.Stdout)
	case "verify":
		in, pubkey := "", ""
		parseFlags(os.Args[2:], map[string]*string{"in": &in, "pubkey": &pubkey})
		if in == "" || pubkey == "" {
			err = fmt.Errorf("verify: --in and --pubkey are required")
			break
		}
		err = doVerify(in, pubkey, os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "intake-license:", err)
		os.Exit(1)
	}
}

const usage = `intake-license (maintainer-only)
  keygen [--key priv.key] [--pub pub.txt]
  sign   --in template.json --key priv.key [--out license.json] [--days 365]
  verify --in license.json --pubkey <base64>`

// parseFlags is a tiny --name value parser (avoids a flag.FlagSet per subcommand).
func parseFlags(args []string, dst map[string]*string) {
	for i := 0; i+1 < len(args); i += 2 {
		name := args[i]
		if len(name) > 2 && name[:2] == "--" {
			if p, ok := dst[name[2:]]; ok {
				*p = args[i+1]
			}
		}
	}
}

// doKeygen generates an Ed25519 keypair, writes the private key (base64 of the
// 64-byte seed+pub) to keyPath (0600), writes the public key base64 to pubPath,
// and prints the public key base64 to out (for embedding in the relay).
func doKeygen(keyPath, pubPath string, out io.Writer) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("keygen: %w", err)
	}
	privB64 := base64.StdEncoding.EncodeToString(priv)
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	if err := os.WriteFile(keyPath, []byte(privB64), 0o600); err != nil {
		return fmt.Errorf("keygen: writing private key: %w", err)
	}
	if err := os.WriteFile(pubPath, []byte(pubB64), 0o644); err != nil {
		return fmt.Errorf("keygen: writing public key: %w", err)
	}
	fmt.Fprintf(out, "private key written to %s (KEEP OFFLINE — never commit)\n", keyPath)
	fmt.Fprintf(out, "public key written to %s\n", pubPath)
	fmt.Fprintf(out, "\nEmbed this public key in relay/internal/license/embedded_key.go:\n")
	fmt.Fprintf(out, "  const EmbeddedPublicKeyBase64 = %q\n", pubB64)
	return nil
}

// doSign reads a license template JSON, loads the base64 private key from keyPath,
// stamps issued_at=now and expires_at=now+days (UTC, second precision), signs with
// the shared relay/license.Sign, and writes the signed license to outPath.
func doSign(inPath, keyPath, outPath string, days int, out io.Writer) error {
	tmpl, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("sign: reading template %s: %w", inPath, err)
	}
	var lic license.License
	if err := json.Unmarshal(tmpl, &lic); err != nil {
		return fmt.Errorf("sign: parsing template: %w", err)
	}
	privB64, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("sign: reading private key %s: %w", keyPath, err)
	}
	privBytes, err := base64.StdEncoding.DecodeString(string(trimSpace(privB64)))
	if err != nil {
		return fmt.Errorf("sign: private key not valid base64: %w", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return fmt.Errorf("sign: private key wrong size (%d; want %d)", len(privBytes), ed25519.PrivateKeySize)
	}
	now := time.Now().UTC().Truncate(time.Second)
	lic.IssuedAt = now
	lic.ExpiresAt = now.Add(time.Duration(days) * 24 * time.Hour)
	if err := license.Sign(ed25519.PrivateKey(privBytes), &lic); err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	signed, err := json.MarshalIndent(lic, "", "  ")
	if err != nil {
		return fmt.Errorf("sign: marshal: %w", err)
	}
	if err := os.WriteFile(outPath, signed, 0o644); err != nil {
		return fmt.Errorf("sign: writing %s: %w", outPath, err)
	}
	fmt.Fprintf(out, "signed license written to %s (tier=%s, adapters=%v, expires=%s)\n",
		outPath, lic.Tier, lic.Adapters, lic.ExpiresAt.Format("2006-01-02"))
	return nil
}

// doVerify checks a signed license file against a base64 public key.
func doVerify(inPath, pubB64 string, out io.Writer) error {
	pubBytes, err := base64.StdEncoding.DecodeString(string(trimSpace([]byte(pubB64))))
	if err != nil {
		return fmt.Errorf("verify: public key not valid base64: %w", err)
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("verify: public key wrong size (%d; want %d)", len(pubBytes), ed25519.PublicKeySize)
	}
	blob, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("verify: reading %s: %w", inPath, err)
	}
	lic, err := license.Verify(ed25519.PublicKey(pubBytes), blob)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "OK — license %s, tier=%s, adapters=%v, expires=%s\n",
		lic.LicenseID, lic.Tier, lic.Adapters, lic.ExpiresAt.Format("2006-01-02"))
	return nil
}

// trimSpace trims leading/trailing ASCII whitespace from a byte slice (avoids a
// strings import for one call).
func trimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && isSpace(b[start]) {
		start++
	}
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool { return c == ' ' || c == '\n' || c == '\r' || c == '\t' }
```

- [ ] **Step 4: Run the tests**

```
cd C:/src/ai/intake/license-tool && go test ./... -v
```

Expected: `TestRoundTrip_KeygenSignVerify` and `TestVerify_TamperRejected` PASS.

- [ ] **Step 5: Build + vet**

```
cd C:/src/ai/intake/license-tool && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`.

- [ ] **Step 6: Commit**

```
cd C:/src/ai/intake/license-tool && git add cmd/intake-license/main.go cmd/intake-license/main_test.go
git commit -m "feat(intake-license): keygen/sign/verify CLI sharing relay/license (3-vii)"
```

---

### Task 3: Self-runnable round-trip smoke (throwaway key, credit-free)

This proves the full keygen → sign → relay-verifiable pipeline without any real/embedded key.

- [ ] **Step 1: keygen a throwaway key**

```
cd C:/src/ai/intake/license-tool
go run ./cmd/intake-license keygen --key /tmp/intake-rt.key --pub /tmp/intake-rt.pub
```

Expected: prints the private/public key paths and a `const EmbeddedPublicKeyBase64 = "…"` line. (Throwaway — do NOT embed or commit.)

- [ ] **Step 2: sign a template**

Write `/tmp/intake-tmpl.json`:

```json
{ "license_id": "lic_rt", "issued_to": { "org": "RT", "email": "ops@rt.example" }, "tier": "pro", "adapters": ["zendesk","linear"], "max_relay_instances": 1 }
```

```
go run ./cmd/intake-license sign --in /tmp/intake-tmpl.json --key /tmp/intake-rt.key --out /tmp/intake-license.json --days 7
```

Expected: `signed license written to /tmp/intake-license.json (tier=pro, adapters=[zendesk linear], expires=…)`.

- [ ] **Step 3: verify with the CLI and confirm the shape**

```
go run ./cmd/intake-license verify --in /tmp/intake-license.json --pubkey "$(cat /tmp/intake-rt.pub)"
```

Expected: `OK — license lic_rt, tier=pro, adapters=[zendesk linear], expires=…`.

- [ ] **Step 4: (optional) prove the RELAY accepts it**

Temporarily set `relay/internal/license/embedded_key.go`'s `EmbeddedPublicKeyBase64` to the throwaway public key, point `license.file` at `/tmp/intake-license.json`, boot the relay, and observe `mode=licensed`. **Revert the embedded key afterward** (the real key is embedded only during the maintainer pause). This optional step bridges to the phase final smoke; the credit-free unit proof is the Task-2 round-trip + 3-vi's `loadWithKey` tests.

> Clean up `/tmp/intake-*` afterward.

---

### Task 4: Final verification gate

- [ ] **Step 1: Build + vet + test both modules**

```
cd C:/src/ai/intake/relay && go build ./... && go vet ./... && go test ./... && echo RELAY_OK
cd C:/src/ai/intake/license-tool && go build ./... && go vet ./... && go test ./... && echo CLI_OK
```

Expected: `RELAY_OK` and `CLI_OK`, all packages `ok`.

- [ ] **Step 2: Contract + pins**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK`.

- [ ] **Step 3: Confirm no external dependency crept into the CLI**

```
cd C:/src/ai/intake/license-tool && go mod tidy && echo TIDY_OK
```

Inspect `go.mod`: it must contain only `require intake v0.0.0` + `replace intake => ../relay` (no external module requires). If an external module appears, investigate.

- [ ] **Step 4: Build-fail self-check (README §6)**

- CLI builds + tests green (both modules) — step 1. ✓
- A CLI-signed license verifies via the shared `relay/license` package — `TestRoundTrip_KeygenSignVerify`. ✓
- Tampered license rejected — `TestVerify_TamperRejected`. ✓
- No private key is committed — `doKeygen` writes the private key to a 0600 file the maintainer keeps offline; only the PUBLIC key is meant for embedding. The plan commits no key material.
- CLI excluded from release — note for Phase 7's goreleaser config (no release pipeline exists yet; this is a guard).

---

## Smoke

**Credit-free (unit + local):** `go test ./...` in `license-tool` runs the keygen→sign→verify round-trip with a throwaway in-test keypair, proving the CLI signs licenses the shared `relay/license.Verify` accepts and that tampering is rejected — the cross-module canonicalization lock (design §2.3). Task 3 repeats this at the command line with a throwaway key. **No real key, no credits.**

**Maintainer keygen/sign pause (phase final smoke README §7 steps 1–2):** the real production keypair is generated once by the maintainer (`intake-license keygen`), the private key stored offline, and the printed public key embedded in `relay/internal/license/embedded_key.go`; then a real short-lived test license is signed for the gate smoke. This pauses for the maintainer because it creates and handles the real signing key — a secret/maintainer action, not a code step.

## Done Criteria

1. `license-tool/go.mod` shares `relay/license` via `require intake v0.0.0` + `replace intake => ../relay`; `go mod tidy` adds no external dependency.
2. `intake-license keygen|sign|verify` work via testable `doKeygen`/`doSign`/`doVerify` functions; `keygen` prints the `EmbeddedPublicKeyBase64` line for embedding and writes the private key to a 0600 file.
3. `sign` stamps `issued_at`/`expires_at` (UTC, `--days` offset) and produces a license that the shared `relay/license.Verify` accepts; tampering is rejected (round-trip + tamper tests green).
4. `go test ./...` green in BOTH `relay/` and `license-tool/`; `verify-contract.sh` and `check-pins.sh` green.
5. No key material is committed; the CLI is documented maintainer-only and flagged for exclusion from Phase 7 release artifacts.
6. The Task-3 throwaway-key round trip runs end-to-end at the command line, credit-free.
