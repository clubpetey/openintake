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
