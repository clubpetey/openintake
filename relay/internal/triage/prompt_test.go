package triage

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_EmptyPath returns the embedded bundled prompt (non-empty, no error).
func TestLoad_EmptyPath(t *testing.T) {
	t.Parallel()
	got, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") returned error: %v", err)
	}
	if got == "" {
		t.Error("Load(\"\") returned empty string; want non-empty bundled prompt")
	}
}

// TestLoad_NonexistentPath returns a non-nil error for a bad override path.
func TestLoad_NonexistentPath(t *testing.T) {
	t.Parallel()
	_, err := Load("/nonexistent/path/that/does/not/exist.txt")
	if err == nil {
		t.Error("Load(nonexistent) returned nil error; want non-nil")
	}
}

// TestLoad_TempFile returns the content of a temp file written with known content.
func TestLoad_TempFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "custom_prompt.txt")
	want := "custom triage system prompt for testing"
	if err := os.WriteFile(path, []byte(want), 0600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load(tempfile) returned error: %v", err)
	}
	if got != want {
		t.Errorf("Load(tempfile) = %q; want %q", got, want)
	}
}
