package license

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// trialState is persisted at DefaultStatePath() to remember when the trial began.
type trialState struct {
	TrialStartedAt time.Time `json:"trial_started_at"`
}

// DefaultStatePath returns os.UserConfigDir()/openintake/state.json (Q3): %AppData%\openintake
// on Windows, ~/.config/openintake on Linux, ~/Library/Application Support/openintake on macOS.
func DefaultStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "openintake", "state.json"), nil
}

// readTrialState returns (state, true, nil) if a state file exists and parses,
// (zero, false, nil) if it is absent or contains malformed JSON (a malformed file
// is treated as absent — trial restarts — because anyone who can corrupt the file
// can delete it anyway; see PROJECT.md §12), or an error for low-level I/O
// failures (e.g. permission denied) where the file genuinely exists.
func readTrialState(path string) (trialState, bool, error) {
	if path == "" {
		return trialState{}, false, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return trialState{}, false, nil
	}
	if err != nil {
		// Genuine I/O error (permission denied, etc.) — propagate.
		return trialState{}, false, err
	}
	var st trialState
	if err := json.Unmarshal(data, &st); err != nil {
		// Malformed JSON → treat as absent, restart trial.
		log.Printf("license: state file %s is malformed (%v); restarting trial", path, err)
		return trialState{}, false, nil
	}
	return st, true, nil
}

// writeTrialState atomically writes the state file by writing to a temp file in
// the same directory and then renaming over the target, so a crash or concurrent
// start mid-write cannot leave a truncated file. Creating the parent directory and
// the write itself are best-effort: a failure (e.g. read-only container fs) is
// returned so the caller can log it but does not stop startup.
func writeTrialState(path string, st trialState) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	// Write to a temp file in the same directory, then rename atomically.
	tmp, err := os.CreateTemp(dir, "state-*.json")
	if err != nil {
		return fmt.Errorf("creating temp state file: %w", err)
	}
	tmpName := tmp.Name()
	// Clean up temp file on any error path.
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting temp state file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp state file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming temp state file to %s: %w", path, err)
	}
	// Rename succeeded — don't try to remove the temp (it's now the target).
	tmpName = ""
	return nil
}
