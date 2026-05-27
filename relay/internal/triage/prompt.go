// Package triage provides the bundled system prompt for the intake triage flow.
//
// Copyright 2026 Mantichor. Licensed under Apache 2.0.
// prompt.txt is product IP — the embedded prompt drives the guided triage UX
// and is never sent to or exposed to the client.
package triage

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
)

//go:embed prompt.txt
var bundledPrompt string

// Load returns the system prompt to use for triage conversations.
//
// If systemPromptFile is non-empty, Load reads that file from disk and returns
// its contents. This lets operators override the bundled prompt without
// recompiling the relay (per config llm.system_prompt_file).
//
// If systemPromptFile is empty (the default), Load returns the compiled-in
// bundled prompt from prompt.txt.
//
// The returned string is trimmed of leading/trailing whitespace.
func Load(systemPromptFile string) (string, error) {
	if systemPromptFile != "" {
		data, err := os.ReadFile(systemPromptFile)
		if err != nil {
			return "", fmt.Errorf("triage: reading system_prompt_file %q: %w", systemPromptFile, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return strings.TrimSpace(bundledPrompt), nil
}
