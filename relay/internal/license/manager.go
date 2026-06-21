// Package license (internal) is the relay-only license manager: it embeds the
// maintainer public key, runs the load order, applies the trial/free state machine,
// and answers the adapter gate via State.Permits. It imports the importable
// relay/license package for the struct + Verify. Where both packages are needed
// (main.go), import THIS one with the alias `licensemgr`.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	pubkglicense "github.com/clubpetey/openintake/relay/license"

	"github.com/clubpetey/openintake/relay/internal/config"
)

// PricingURL is shown in the free/expired startup logs (PROJECT.md §12 wording).
// Points at the committed commercial terms (no standalone pricing site by design).
const PricingURL = "https://github.com/clubpetey/openintake/blob/main/COMMERCIAL.md"

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

// Load resolves the license using the embedded public key. A corrupt (non-empty
// but invalid) embedded key is fatal; an absent key (empty constant) is normal
// pre-keygen state.
func Load(cfg config.LicenseConfig, statePath string, now time.Time) (*State, error) {
	pub, err := embeddedPublicKey()
	if err != nil {
		// Non-empty but invalid embedded key → fatal; never masquerade as "no key".
		return nil, fmt.Errorf("license: %w", err)
	}
	return loadWithKey(pub, cfg, statePath, now)
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
			return nil, fmt.Errorf("license: a license is present but this build has no embedded public key (maintainer must run `openintake-license keygen` and embed it)")
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
		days := int(math.Ceil(lic.ExpiresAt.Sub(now).Hours() / 24))
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
		days := int(math.Ceil(expiry.Sub(now).Hours() / 24))
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

// defaultLicensePaths returns /etc/openintake/license.json and
// os.UserConfigDir()/openintake/license.json (the latter only if resolvable).
func defaultLicensePaths() []string {
	paths := []string{"/etc/openintake/license.json"}
	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(dir, "openintake", "license.json"))
	}
	return paths
}
