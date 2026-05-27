package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intake/internal/config"
)

// writeFile writes content to a new file in dir and returns the path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return p
}

// ── ResolveSecret ─────────────────────────────────────────────────────────────

func TestResolveSecret_EnvVarSet(t *testing.T) {
	t.Setenv("TEST_RS_KEY", "my-secret-value")

	got, err := config.ResolveSecret("TEST_RS_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-secret-value" {
		t.Errorf("got %q; want %q", got, "my-secret-value")
	}
}

func TestResolveSecret_FileSet(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "secret.txt", "file-secret-value")
	t.Setenv("TEST_RS_KEY2_FILE", p)

	got, err := config.ResolveSecret("TEST_RS_KEY2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "file-secret-value" {
		t.Errorf("got %q; want %q", got, "file-secret-value")
	}
}

func TestResolveSecret_FileContentsTrimed(t *testing.T) {
	dir := t.TempDir()
	// File with trailing newline and surrounding spaces — common in mounted secrets.
	p := writeFile(t, dir, "secret.txt", "  trimmed-value  \n")
	t.Setenv("TEST_RS_TRIM_FILE", p)

	got, err := config.ResolveSecret("TEST_RS_TRIM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "trimmed-value" {
		t.Errorf("got %q; want %q", got, "trimmed-value")
	}
}

func TestResolveSecret_BothSet_Error(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "secret.txt", "file-secret")
	t.Setenv("TEST_RS_BOTH", "direct-value")
	t.Setenv("TEST_RS_BOTH_FILE", p)

	_, err := config.ResolveSecret("TEST_RS_BOTH")
	if err == nil {
		t.Fatal("expected error when both env and _FILE are set; got nil")
	}
	// Error must mention the env var NAME.
	if !strings.Contains(err.Error(), "TEST_RS_BOTH") {
		t.Errorf("error should mention env var name; got: %v", err)
	}
	// Error must NOT contain the secret value.
	if strings.Contains(err.Error(), "direct-value") || strings.Contains(err.Error(), "file-secret") {
		t.Errorf("error must not contain secret value; got: %v", err)
	}
}

func TestResolveSecret_NeitherSet_ReturnsEmpty(t *testing.T) {
	// Ensure neither var is set (t.Setenv with "" clears via t.Cleanup/os.Unsetenv).
	t.Setenv("TEST_RS_NONE", "")
	t.Setenv("TEST_RS_NONE_FILE", "")

	got, err := config.ResolveSecret("TEST_RS_NONE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q; want empty string", got)
	}
}

func TestResolveSecret_FileNotExist_Error(t *testing.T) {
	t.Setenv("TEST_RS_NOFILE_FILE", "/nonexistent/path/to/secret.txt")

	_, err := config.ResolveSecret("TEST_RS_NOFILE")
	if err == nil {
		t.Fatal("expected error for nonexistent file; got nil")
	}
	// Error must mention the path so the operator can diagnose.
	if !strings.Contains(err.Error(), "/nonexistent/path/to/secret.txt") {
		t.Errorf("error should mention the file path; got: %v", err)
	}
	// There is no secret value to leak, but assert the pattern holds.
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("error must not contain secret value; got: %v", err)
	}
}

func TestResolveSecret_EmptyFile_Error(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "empty.txt", "")
	t.Setenv("TEST_RS_EMPTY_FILE", p)

	_, err := config.ResolveSecret("TEST_RS_EMPTY")
	if err == nil {
		t.Fatal("expected error for empty secret file; got nil")
	}
	// Error must mention the file path.
	if !strings.Contains(err.Error(), p) {
		t.Errorf("error should mention the file path; got: %v", err)
	}
}

func TestResolveSecret_WhitespaceOnlyFile_Error(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "ws.txt", "   \n\t  ")
	t.Setenv("TEST_RS_WS_FILE", p)

	_, err := config.ResolveSecret("TEST_RS_WS")
	if err == nil {
		t.Fatal("expected error for whitespace-only secret file; got nil")
	}
}

// ── RequireSecret ─────────────────────────────────────────────────────────────

func TestRequireSecret_NeitherSet_Error(t *testing.T) {
	t.Setenv("TEST_REQ_NONE", "")
	t.Setenv("TEST_REQ_NONE_FILE", "")

	_, err := config.RequireSecret("TEST_REQ_NONE")
	if err == nil {
		t.Fatal("RequireSecret: expected error when neither is set; got nil")
	}
	// Error must mention the env var name to guide the operator.
	if !strings.Contains(err.Error(), "TEST_REQ_NONE") {
		t.Errorf("error should mention env var name; got: %v", err)
	}
}

func TestRequireSecret_EnvVarSet_OK(t *testing.T) {
	t.Setenv("TEST_REQ_OK", "required-value")

	got, err := config.RequireSecret("TEST_REQ_OK")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "required-value" {
		t.Errorf("got %q; want %q", got, "required-value")
	}
}

func TestRequireSecret_BothSet_PropagatesError(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "s.txt", "v")
	t.Setenv("TEST_REQ_BOTH", "direct")
	t.Setenv("TEST_REQ_BOTH_FILE", p)

	_, err := config.RequireSecret("TEST_REQ_BOTH")
	if err == nil {
		t.Fatal("RequireSecret: expected error when both are set; got nil")
	}
}
