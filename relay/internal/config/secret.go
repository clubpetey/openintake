package config

import (
	"fmt"
	"os"
	"strings"
)

// ResolveSecret resolves a secret value from the environment.
// Resolution order: $<envName> OR contents of the file at $<envName>_FILE (trimmed).
// It is an error for BOTH $<envName> and $<envName>_FILE to be set (non-empty) — no
// silent ambiguity about which wins.
// Returns ("", nil) when neither is set; caller decides if the secret is required.
//
// Security invariant: the resolved secret VALUE is never included in any error message.
// The env var NAME and the file PATH are safe to include.
func ResolveSecret(envName string) (string, error) {
	v := os.Getenv(envName)
	fp := os.Getenv(envName + "_FILE")

	if v != "" && fp != "" {
		return "", fmt.Errorf("config: both %s and %s_FILE are set; set only one", envName, envName)
	}

	if v != "" {
		return v, nil
	}

	if fp != "" {
		data, err := os.ReadFile(fp)
		if err != nil {
			return "", fmt.Errorf("config: reading secret file %s (from %s_FILE): %w", fp, envName, err)
		}
		val := strings.TrimSpace(string(data))
		if val == "" {
			return "", fmt.Errorf("config: secret file %s is empty (from %s_FILE)", fp, envName)
		}
		return val, nil
	}

	return "", nil
}

// RequireSecret is like ResolveSecret but returns an error if the resolved value is empty.
// Use this for secrets that must be present for the service to function.
func RequireSecret(envName string) (string, error) {
	val, err := ResolveSecret(envName)
	if err != nil {
		return "", err
	}
	if val == "" {
		return "", fmt.Errorf("config: required secret %s is not set (set %s or %s_FILE)", envName, envName, envName)
	}
	return val, nil
}
