# 3-vi License Verify + Gate — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Ed25519 license verification and the license gate. Create the importable `relay/license` package (License struct + `Canonicalize`/`Sign`/`Verify`) shared with the CLI; create the relay-only `relay/internal/license` package (embedded public key, load order, 14-day-trial / free-mode state machine, `Permits` gate); and retrofit the gate into `main.go`'s `buildRegistry` so the paid adapters (zendesk, linear) register only when the active license/trial permits them.

**Architecture:** `relay/license` is pure crypto/struct (`crypto/ed25519`, `encoding/json`, `encoding/base64`) with zero relay deps — the single canonicalization source the CLI (3-vii) reuses via a `replace` directive. `relay/internal/license` imports it, owns the embedded public key constant + the loader (CLI/env/file/default-path order) + the trial/free machine + `State.Permits`. `main.go` calls `licensemgr.Load(...)` (alias to avoid the duplicate package name), logs the mode, and passes the `*State` into `buildRegistry`, which skips a paid adapter (with a clear warning) when `state.Permits(name)` is false. Bad signature is fatal; an expired-but-valid license downgrades to free with a warning (design §4.2).

**Tech Stack:** Go 1.23.2 (relay). Standard library only (`crypto/ed25519`, `crypto/rand` in tests, `encoding/json`, `encoding/base64`, `os`). No new external dependency.

---

## Design References

- README §8.5 — the two license packages (verbatim signatures, frozen here)
- README §6 — build-fail items (bad-sig must error; paid adapter must not register without a permitting license)
- Design spec §2.2/§2.3 (package split + canonicalization contract), §4.1 (load order), §4.2 (state machine), §4.3 (gate), §4.5 (`hosted` tier)
- PROJECT.md §12 (license model, load order, trial, verification flow), §16 (hosted tier)
- Decomposition §4 Q3 (`os.UserConfigDir()` state path), Q10 (maintainer-only)
- Seam to modify: `relay/cmd/relay/main.go` `buildRegistry` (from 3-i; paid branches from 3-iv/3-v)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/license/license.go` | Create | `License`, `IssuedTo`, `Canonicalize`, `Sign`, `Verify` (importable; shared with CLI) |
| `relay/license/license_test.go` | Create | sign→verify round-trip; tamper rejects; wrong key rejects; missing-prefix errors |
| `relay/internal/license/manager.go` | Create | `State`, `Permits`, `Load`/`loadWithKey`, load order, trial/free machine, `PricingURL` |
| `relay/internal/license/embedded_key.go` | Create | `EmbeddedPublicKeyBase64` constant (placeholder, filled at keygen pause) + `embeddedPublicKey()` |
| `relay/internal/license/state_file.go` | Create | `trialState` read/write, `DefaultStatePath()` (`os.UserConfigDir()/intake/state.json`) |
| `relay/internal/license/manager_test.go` | Create | trial-start / trial-active / trial-expired / licensed / expired-downgrade / bad-sig-fatal / `Permits` matrix (ephemeral key, injected `now`, temp statePath) |
| `relay/cmd/relay/main.go` | Modify | `--license-file` flag; `licensemgr.Load`; pass `*State` into `buildRegistry`; gate paid branches |

---

## Tasks

### Task 1: Create the importable `relay/license` package (TDD)

**Files:** Create `relay/license/license_test.go`, then `relay/license/license.go`

- [ ] **Step 1: Write the failing test**

Create `relay/license/license_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify failure (package missing)**

```
cd C:/src/ai/intake/relay && go test ./license/... -v
```

Expected: `no required module provides package intake/license`. MUST fail.

- [ ] **Step 3: Create `license.go`**

Create `relay/license/license.go`:

```go
// Package license is the importable, dependency-free core of the license model:
// the License struct and the canonicalize/sign/verify primitives. It is shared
// verbatim by the relay (verifies with the embedded public key) and the
// maintainer-only intake-license CLI (signs with the private key, via a replace
// directive). Keeping ONE definition here is what guarantees the two modules agree
// byte-for-byte on what gets signed. No relay-internal imports may be added here.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const sigPrefix = "ed25519:"

// License is the signed entitlement (PROJECT.md §12). Field order is the canonical
// signing order; do not reorder without re-issuing every license.
type License struct {
	LicenseID    string    `json:"license_id"`
	IssuedTo     IssuedTo  `json:"issued_to"`
	Tier         string    `json:"tier"` // "pro" | "team" | "hosted"
	Adapters     []string  `json:"adapters"`
	MaxInstances int       `json:"max_relay_instances"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Signature    string    `json:"signature"`
}

// IssuedTo identifies the licensee.
type IssuedTo struct {
	Org   string `json:"org"`
	Email string `json:"email"`
}

// Canonicalize returns the exact bytes that are signed and verified: the JSON of
// the license with Signature cleared. Deterministic: Go marshals struct fields in
// declaration order and the License has no map fields. The argument is taken by
// value so the caller's Signature is never mutated.
func Canonicalize(l License) ([]byte, error) {
	l.Signature = ""
	b, err := json.Marshal(l)
	if err != nil {
		return nil, fmt.Errorf("license: canonicalize: %w", err)
	}
	return b, nil
}

// Sign sets l.Signature to "ed25519:" + base64(signature over Canonicalize(l)).
func Sign(priv ed25519.PrivateKey, l *License) error {
	canon, err := Canonicalize(*l)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(priv, canon)
	l.Signature = sigPrefix + base64.StdEncoding.EncodeToString(sig)
	return nil
}

// Verify parses blob, checks the "ed25519:" signature against pub over the
// re-canonicalized license, and returns the parsed License on success. Any
// tampering (the canonical bytes no longer match the signature) fails.
func Verify(pub ed25519.PublicKey, blob []byte) (*License, error) {
	var l License
	if err := json.Unmarshal(blob, &l); err != nil {
		return nil, fmt.Errorf("license: parse: %w", err)
	}
	if !strings.HasPrefix(l.Signature, sigPrefix) {
		return nil, fmt.Errorf("license: signature missing %q prefix", sigPrefix)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(l.Signature, sigPrefix))
	if err != nil {
		return nil, fmt.Errorf("license: signature not valid base64: %w", err)
	}
	canon, err := Canonicalize(l)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(pub, canon, sig) {
		return nil, fmt.Errorf("license: signature verification failed (tampered or wrong key)")
	}
	return &l, nil
}
```

- [ ] **Step 4: Run the tests**

```
cd C:/src/ai/intake/relay && go test ./license/... -v
```

Expected: all four tests PASS.

- [ ] **Step 5: Commit**

```
cd C:/src/ai/intake/relay && git add license/license.go license/license_test.go
git commit -m "feat(license): importable Ed25519 sign/verify + canonical License (3-vi)"
```

---

### Task 2: Create the relay-only `internal/license` manager (TDD)

**Files:** Create `relay/internal/license/embedded_key.go`, `state_file.go`, `manager.go`, `manager_test.go`

- [ ] **Step 1: Create the embedded-key file**

Create `relay/internal/license/embedded_key.go`:

```go
package license

import "crypto/ed25519"

// EmbeddedPublicKeyBase64 is the maintainer's Ed25519 public key (standard base64
// of the 32-byte key). It is filled during the keygen pause (phase final smoke
// step 1: `intake-license keygen` prints it). Empty in the committed source: a
// build with an empty key cannot verify a license, so Load treats a *present*
// license as an error until the maintainer embeds the real key.
const EmbeddedPublicKeyBase64 = ""

// embeddedPublicKey decodes EmbeddedPublicKeyBase64, or returns nil if unset/invalid.
func embeddedPublicKey() ed25519.PublicKey {
	if EmbeddedPublicKeyBase64 == "" {
		return nil
	}
	b, err := base64Decode(EmbeddedPublicKeyBase64)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}
```

Add a tiny base64 helper in the same file (kept local to avoid importing in multiple files):

```go
import "encoding/base64"

func base64Decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
```

> Merge the two `import` blocks into one (`crypto/ed25519` + `encoding/base64`); the split above is for readability.

- [ ] **Step 2: Create the state-file helpers**

Create `relay/internal/license/state_file.go`:

```go
package license

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// trialState is persisted at DefaultStatePath() to remember when the trial began.
type trialState struct {
	TrialStartedAt time.Time `json:"trial_started_at"`
}

// DefaultStatePath returns os.UserConfigDir()/intake/state.json (Q3): %AppData%\intake
// on Windows, ~/.config/intake on Linux, ~/Library/Application Support/intake on macOS.
func DefaultStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "intake", "state.json"), nil
}

// readTrialState returns (state, true, nil) if a state file exists and parses,
// (zero, false, nil) if it is absent, or an error if it exists but is unreadable.
func readTrialState(path string) (trialState, bool, error) {
	if path == "" {
		return trialState{}, false, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return trialState{}, false, nil
	}
	if err != nil {
		return trialState{}, false, err
	}
	var st trialState
	if err := json.Unmarshal(data, &st); err != nil {
		return trialState{}, false, err
	}
	return st, true, nil
}

// writeTrialState writes the state file, creating the parent directory. A best-effort
// operation: a write failure (e.g. read-only container fs) is returned so the caller
// can log it, but it does not stop startup.
func writeTrialState(path string, st trialState) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
```

- [ ] **Step 3: Write the failing manager test**

Create `relay/internal/license/manager_test.go` (white-box — `package license` — so it can call the unexported `loadWithKey` and inject an ephemeral key):

```go
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
```

- [ ] **Step 4: Run to verify failure**

```
cd C:/src/ai/intake/relay && go test ./internal/license/... -v
```

Expected: compile error (`loadWithKey` / `State` undefined). MUST fail.

- [ ] **Step 5: Create `manager.go`**

Create `relay/internal/license/manager.go`:

```go
// Package license (internal) is the relay-only license manager: it embeds the
// maintainer public key, runs the load order, applies the trial/free state machine,
// and answers the adapter gate via State.Permits. It imports the importable
// intake/license package for the struct + Verify. Where both packages are needed
// (main.go), import THIS one with the alias `licensemgr`.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	pubkglicense "intake/license"

	"intake/internal/config"
)

// PricingURL is shown in the free/expired startup logs (PROJECT.md §12 wording).
const PricingURL = "https://intake.example.com/pricing"

const trialDuration = 14 * 24 * time.Hour

// State is the resolved entitlement after load order + trial/free rules.
type State struct {
	Mode      string    // "licensed" | "trial" | "free"
	Adapters  []string  // paid adapters granted when Mode == "licensed" (non-hosted)
	ExpiresAt time.Time // trial or license expiry
	Message   string    // human-readable startup log line
	grantAll  bool      // hosted tier
}

// Permits reports whether a PAID adapter name is allowed. Callers gate only paid
// adapters (free adapters bypass the gate entirely).
func (s *State) Permits(name string) bool {
	switch s.Mode {
	case "trial":
		return true
	case "licensed":
		if s.grantAll {
			return true
		}
		for _, a := range s.Adapters {
			if a == name {
				return true
			}
		}
		return false
	default: // "free"
		return false
	}
}

// Load resolves the license using the embedded public key.
func Load(cfg config.LicenseConfig, statePath string, now time.Time) (*State, error) {
	return loadWithKey(embeddedPublicKey(), cfg, statePath, now)
}

// loadWithKey is the testable core: pub is injected (production passes the embedded
// key; tests pass an ephemeral key).
//
//   - bad signature → error (FATAL; design §4.2)
//   - valid but expired → free mode + warning message (NOT an error)
//   - no license → 14-day trial, then free (state persisted at statePath)
func loadWithKey(pub ed25519.PublicKey, cfg config.LicenseConfig, statePath string, now time.Time) (*State, error) {
	blob, found, err := findLicense(cfg)
	if err != nil {
		return nil, err
	}

	if found {
		if pub == nil {
			return nil, fmt.Errorf("license: a license is present but this build has no embedded public key (maintainer must run `intake-license keygen` and embed it)")
		}
		lic, err := pubkglicense.Verify(pub, blob)
		if err != nil {
			return nil, fmt.Errorf("license: %w", err) // bad signature → fatal
		}
		if !lic.ExpiresAt.After(now) {
			return &State{
				Mode:      "free",
				ExpiresAt: lic.ExpiresAt,
				Message:   fmt.Sprintf("license expired %s — running in FREE mode (paid adapters disabled); renew at %s", lic.ExpiresAt.Format("2006-01-02"), PricingURL),
			}, nil
		}
		days := int(lic.ExpiresAt.Sub(now).Hours() / 24)
		return &State{
			Mode:      "licensed",
			Adapters:  lic.Adapters,
			ExpiresAt: lic.ExpiresAt,
			grantAll:  lic.Tier == "hosted",
			Message:   fmt.Sprintf("licensed: tier=%s, %d day(s) remaining (expires %s)", lic.Tier, days, lic.ExpiresAt.Format("2006-01-02")),
		}, nil
	}

	// No license → trial / free machine.
	st, exists, err := readTrialState(statePath)
	if err != nil {
		return nil, fmt.Errorf("license: reading trial state %s: %w", statePath, err)
	}
	if !exists {
		// Start a fresh trial.
		if werr := writeTrialState(statePath, trialState{TrialStartedAt: now}); werr != nil {
			// Non-fatal: a read-only fs (container) just means the trial is ephemeral.
			return &State{
				Mode:      "trial",
				ExpiresAt: now.Add(trialDuration),
				Message:   fmt.Sprintf("trial started (could not persist state: %v) — 14 days remaining; all adapters enabled", werr),
			}, nil
		}
		return &State{
			Mode:      "trial",
			ExpiresAt: now.Add(trialDuration),
			Message:   "trial started — 14 days remaining; all adapters enabled",
		}, nil
	}

	expiry := st.TrialStartedAt.Add(trialDuration)
	if now.Before(expiry) {
		days := int(expiry.Sub(now).Hours()/24) + 1
		return &State{
			Mode:      "trial",
			ExpiresAt: expiry,
			Message:   fmt.Sprintf("trial — %d day(s) remaining; all adapters enabled", days),
		}, nil
	}
	return &State{
		Mode:      "free",
		ExpiresAt: expiry,
		Message:   fmt.Sprintf("trial expired %s — FREE mode (paid adapters disabled); see %s", expiry.Format("2006-01-02"), PricingURL),
	}, nil
}

// findLicense applies the load order (design §4.1). Returns (blob, found, error).
// An explicit source (cfg.File, INTAKE_LICENSE, INTAKE_LICENSE_FILE) that is set
// but unreadable/undecodable is an error; the default paths are skipped if absent.
func findLicense(cfg config.LicenseConfig) ([]byte, bool, error) {
	// 1. explicit path (CLI flag is folded into cfg.File by main.go) / YAML license.file
	if cfg.File != "" {
		data, err := os.ReadFile(cfg.File)
		if err != nil {
			return nil, false, fmt.Errorf("license: reading license.file %s: %w", cfg.File, err)
		}
		return data, true, nil
	}
	// 2. INTAKE_LICENSE (base64-encoded JSON)
	if b64 := os.Getenv("INTAKE_LICENSE"); b64 != "" {
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, false, fmt.Errorf("license: INTAKE_LICENSE is not valid base64: %w", err)
		}
		return data, true, nil
	}
	// 3. INTAKE_LICENSE_FILE (path)
	if fp := os.Getenv("INTAKE_LICENSE_FILE"); fp != "" {
		data, err := os.ReadFile(fp)
		if err != nil {
			return nil, false, fmt.Errorf("license: reading INTAKE_LICENSE_FILE %s: %w", fp, err)
		}
		return data, true, nil
	}
	// 4 & 5. default paths — absent is normal.
	for _, p := range defaultLicensePaths() {
		if p == "" {
			continue
		}
		if data, err := os.ReadFile(p); err == nil {
			return data, true, nil
		}
	}
	return nil, false, nil
}

// defaultLicensePaths returns /etc/intake/license.json and
// os.UserConfigDir()/intake/license.json (the latter only if resolvable).
func defaultLicensePaths() []string {
	paths := []string{"/etc/intake/license.json"}
	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, dir+string(os.PathSeparator)+"intake"+string(os.PathSeparator)+"license.json")
	}
	return paths
}
```

- [ ] **Step 6: Run the manager tests**

```
cd C:/src/ai/intake/relay && go test ./internal/license/... -v
```

Expected: all manager tests PASS (trial-start, trial-expired, licensed, expired-downgrade, bad-sig-fatal, hosted-grants-all).

- [ ] **Step 7: Commit**

```
cd C:/src/ai/intake/relay && git add internal/license/
git commit -m "feat(license): relay manager — load order, trial/free machine, Permits gate (3-vi)"
```

---

### Task 3: Wire the license load + gate into main.go

**Files:** Modify `relay/cmd/relay/main.go`

This adds the `--license-file` flag, loads the license state, logs the mode, and gates the paid adapters in `buildRegistry`. `buildRegistry`'s signature gains a `*licensemgr.State` parameter, and the zendesk/linear branches (added by 3-iv/3-v) are wrapped with the gate.

- [ ] **Step 1: Add the flag, the alias import, and the license load**

In `relay/cmd/relay/main.go`:

1. Add the import with alias: `licensemgr "intake/internal/license"`. (Alias avoids the clash with the importable `intake/license`, which `main.go` does not import directly.)
2. Add the flag next to `configPath`:

```go
	licenseFile := flag.String("license-file", "", "path to license.json (overrides config license.file)")
```

3. After `cfg, err := config.Load(...)` (and its error check), insert:

```go
	// --- License (3-vi): flag overrides config.license.file, then env/default paths ---
	if *licenseFile != "" {
		cfg.License.File = *licenseFile
	}
	statePath, sperr := licensemgr.DefaultStatePath()
	if sperr != nil {
		logger.Warn("relay: cannot resolve trial-state path; trial will be ephemeral", "error", sperr)
		statePath = ""
	}
	licState, err := licensemgr.Load(cfg.License, statePath, time.Now().UTC())
	if err != nil {
		// Bad signature / unreadable explicit license / present-license-but-no-embedded-key → fatal.
		logger.Error("relay: license verification failed", "error", err)
		os.Exit(1)
	}
	logger.Info("relay: license", "mode", licState.Mode, "detail", licState.Message)
```

- [ ] **Step 2: Pass the state into buildRegistry and update its signature**

Change the call site:

```go
	registry, err := buildRegistry(cfg, licState, logger)
```

Change the `buildRegistry` signature and gate the paid branches. Replace the `buildRegistry` function header line with:

```go
func buildRegistry(cfg *config.Config, licState *licensemgr.State, logger *slog.Logger) (map[string]adapter.Adapter, error) {
```

Then, inside `buildRegistry`, wrap the **zendesk** branch (added in 3-iv) and the **linear** branch (added in 3-v) with the gate. The gate check comes BEFORE `RequireSecret`, so a free-mode operator with the adapter enabled but no token does not hit a fatal "missing token" — they get a clear "requires a license" skip. Zendesk branch becomes:

```go
	// zendesk (3-iv) — PAID; gated (3-vi).
	if cfg.Adapters.Zendesk.Enabled {
		if !licState.Permits("zendesk") {
			logger.Warn(`relay: adapter "zendesk" requires a license — disabled`, "mode", licState.Mode, "see", licensemgr.PricingURL)
		} else {
			token, err := config.RequireSecret(cfg.Adapters.Zendesk.APITokenEnv)
			if err != nil {
				return nil, fmt.Errorf("zendesk adapter: %w", err)
			}
			zd := zendesk.New()
			if err := zd.Configure(map[string]any{
				"subdomain":        cfg.Adapters.Zendesk.Subdomain,
				"email":            cfg.Adapters.Zendesk.Email,
				"api_token":        token,
				"default_priority": cfg.Adapters.Zendesk.DefaultPriority,
			}); err != nil {
				return nil, fmt.Errorf("zendesk adapter: %w", err)
			}
			reg[zd.Name()] = zd
			logger.Info("relay: adapter enabled", "adapter", zd.Name())
		}
	}
```

Linear branch becomes:

```go
	// linear (3-v) — PAID; gated (3-vi).
	if cfg.Adapters.Linear.Enabled {
		if !licState.Permits("linear") {
			logger.Warn(`relay: adapter "linear" requires a license — disabled`, "mode", licState.Mode, "see", licensemgr.PricingURL)
		} else {
			key, err := config.RequireSecret(cfg.Adapters.Linear.APIKeyEnv)
			if err != nil {
				return nil, fmt.Errorf("linear adapter: %w", err)
			}
			ln := linear.New()
			if err := ln.Configure(map[string]any{
				"api_key": key,
				"team_id": cfg.Adapters.Linear.TeamID,
			}); err != nil {
				return nil, fmt.Errorf("linear adapter: %w", err)
			}
			reg[ln.Name()] = ln
			logger.Info("relay: adapter enabled", "adapter", ln.Name())
		}
	}
```

(The webhook/chatwoot/fider free branches are unchanged — no gate.)

> If 3-iv/3-v have not been implemented yet when this sub-plan runs, only the branches that exist need wrapping; the gate helper pattern is the same. Per the dependency graph, 3-vi runs after the adapters.

- [ ] **Step 3: Build + vet**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK
```

Expected: `BUILD_OK` / `VET_OK`. Confirm `time` is imported (it already is in main.go).

- [ ] **Step 4: Full test suite**

```
cd C:/src/ai/intake/relay && go test ./... && echo TEST_OK
```

Expected: `TEST_OK` — including `intake/license` and `intake/internal/license`.

- [ ] **Step 5: Commit**

```
cd C:/src/ai/intake/relay && git add cmd/relay/main.go
git commit -m "feat(main): load license state + gate paid adapters in buildRegistry (3-vi)"
```

---

### Task 4: Free-mode startup-log smoke (credit-free, self-runnable)

This is the one phase-smoke step that needs NO external dependency. It proves the gate disables a paid adapter and logs clearly when there is no license.

**Files:** none (manual run; optionally captured in a scratch config).

- [ ] **Step 1: Create a scratch config enabling zendesk with no license**

Write a temporary `relay/smoke-freemode.yaml` (do NOT commit) :

```yaml
server: { addr: ":8099" }
llm: { provider: "anthropic" }   # not started end-to-end; we only observe startup logs
routing:
  default_adapter: "webhook"
adapters:
  webhook: { enabled: true, url: "http://localhost:9099/intake" }
  zendesk: { enabled: true, subdomain: "example", email: "a@example.com", api_token_env: "ZENDESK_API_TOKEN" }
```

- [ ] **Step 2: Run with a guaranteed-fresh (free) state and observe logs**

Point the trial state at a non-existent dir so the trial would start; to force FREE instead, simulate an expired trial OR simply observe the gate when no license + fresh trial grants it. To specifically observe FREE mode, pre-write an expired trial state, or temporarily set `trialDuration` expectations aside and assert the gate via the unit tests (Task 2) which already cover free mode deterministically.

The deterministic, credit-free assertion of the FREE-mode gate is the unit test `TestLoad_TrialExpired_Free` + the `buildRegistry` gate code path. For a live log observation, run:

```
cd C:/src/ai/intake/relay && ANTHROPIC_API_KEY=dummy go run ./cmd/relay -config smoke-freemode.yaml
```

Observe in the startup logs (before any request):
- `relay: license  mode=trial detail=trial started — 14 days remaining` (fresh trial) OR, with an expired trial state present, `mode=free`.
- With `mode=free`: `relay: adapter "zendesk" requires a license — disabled  mode=free see=…` and zendesk ABSENT from `relay: router ready … adapters=[webhook]`.

Stop the relay (Ctrl+C). Delete `smoke-freemode.yaml`.

> The authoritative, repeatable proof of the gate is the Task-2 unit suite (free vs trial vs licensed, deterministic with injected `now`). This step is a human-visible confirmation of the same logic in the real startup path.

- [ ] **Step 2b: Confirm trial reset**

```
# Show the trial state path, then delete it to reset the 14-day trial:
cd C:/src/ai/intake/relay && go run ./cmd/relay -config smoke-freemode.yaml  # logs the trial; Ctrl+C
```

Delete `os.UserConfigDir()/intake/state.json` (e.g. on Windows `%AppData%\intake\state.json`) to reset; re-running starts a fresh trial.

---

### Task 5: Final verification gate

- [ ] **Step 1: Full build + vet + test**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected: `BUILD_OK`, `VET_OK`, every package `ok` (including `intake/license`, `intake/internal/license`), `TEST_OK`.

- [ ] **Step 2: Contract + pins + mod-clean**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
cd C:/src/ai/intake/relay && go mod tidy && git diff --exit-code go.mod go.sum && echo MOD_CLEAN
```

Expected: `CONTRACT_OK`, `PINS_OK`, `MOD_CLEAN` (stdlib-only; no new dependency).

- [ ] **Step 3: Build-fail self-check (README §6)**

- `license.Verify` rejects tampered/wrong-key → `TestVerify_TamperRejected`, `TestVerify_WrongKeyRejected`. ✓
- bad signature is fatal in Load → `TestLoad_BadSignature_Fatal`. ✓
- paid adapter NOT registered without a permitting license → gate in `buildRegistry` + `TestLoad_TrialExpired_Free` (`Permits` false). ✓
- no secret/private key logged → `Load`/`buildRegistry` log only mode/name/PricingURL, never a key. ✓
- no new external dep → step 2 `MOD_CLEAN`. ✓

---

## Smoke

**Credit-free (unit):** `go test ./license/... ./internal/license/...` proves sign/verify (round-trip, tamper, wrong-key, prefix) and the full state machine (trial-start, trial-active, trial-expired→free, licensed, expired→free-downgrade, bad-sig→fatal, hosted→grant-all, `Permits` matrix) — all with an ephemeral test keypair, injected `now`, and a temp state path. **No real key, no embedded key needed.**

**Free-mode log (Task 4):** self-runnable, no external dependency — the relay boots, logs the trial/free mode, and (in free/expired mode) disables a paid adapter with a clear message; zendesk is absent from the router's adapter list.

**Licensed path (PAUSES for the maintainer — phase final smoke README §7 steps 1–2, 4–5):** requires the keygen pause (the maintainer runs `intake-license keygen`, embeds the printed public key in `embedded_key.go`, rebuilds) and a maintainer-signed test license. With the signed license present, the relay logs `mode=licensed` and registers zendesk/linear; without it, free mode disables them. This is deferred because it needs the real Ed25519 keypair (3-vii produces the keygen tool) and a signed license.

## Done Criteria

1. `relay/license` exists and is dependency-free; `Sign`/`Verify` round-trip and reject tampering/wrong-key/missing-prefix (unit green).
2. `relay/internal/license` implements the load order + 14-day-trial / free machine + `Permits`; bad signature is fatal, expired license downgrades to free (unit green, deterministic with injected `now`).
3. `main.go` loads the license state, logs the mode, and `buildRegistry` skips a paid adapter (with a clear warning naming `PricingURL`) when `Permits` is false — the gate check precedes the token requirement.
4. The embedded-key constant is a documented placeholder (empty) with a clear comment that the maintainer fills it at the keygen pause; a present license with an empty embedded key is a fatal error.
5. `go test ./...` green with NO real or embedded key; `verify-contract.sh`, `check-pins.sh` green; `go mod tidy` leaves go.mod/go.sum unchanged.
6. The free-mode startup-log smoke (Task 4) shows the paid adapter disabled and absent from the router.
