package sso_test

import (
	"log/slog"
	"strings"
	"testing"

	"intake/internal/auth/sso"
	"intake/internal/config"
)

// silentLogger returns a logger that swallows output (so test logs stay clean).
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discard{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestNew_BothSet_Errors(t *testing.T) {
	cfg := config.SSOConfig{
		Issuer:         "https://issuer.example/",
		Audience:       "https://api.example",
		JWKSURL:        "https://issuer.example/.well-known/jwks.json",
		HS256SecretEnv: "INTAKE_SSO_HS256_SECRET",
		Claims:         config.SSOClaimNames{UserID: "sub", Email: "email", DisplayName: "name"},
	}
	secret := []byte("0123456789abcdef0123456789abcdef")
	_, err := sso.New(cfg, secret, silentLogger())
	if err == nil {
		t.Fatal("expected error when both JWKSURL and HS256 secret are supplied")
	}
	if !strings.Contains(err.Error(), "jwks_url") || !strings.Contains(err.Error(), "hs256") {
		t.Errorf("error should name both fields; got %v", err)
	}
}

func TestNew_NeitherSet_Errors(t *testing.T) {
	cfg := config.SSOConfig{
		Issuer:   "https://issuer.example/",
		Audience: "https://api.example",
		Claims:   config.SSOClaimNames{UserID: "sub", Email: "email", DisplayName: "name"},
	}
	_, err := sso.New(cfg, nil, silentLogger())
	if err == nil {
		t.Fatal("expected error when neither JWKSURL nor HS256 secret are supplied")
	}
	if !strings.Contains(err.Error(), "jwks_url") || !strings.Contains(err.Error(), "hs256") {
		t.Errorf("error should mention both possible fields; got %v", err)
	}
}

func TestNew_HS256SecretTooShort_Errors(t *testing.T) {
	cfg := config.SSOConfig{
		Issuer:         "https://issuer.example/",
		Audience:       "https://api.example",
		HS256SecretEnv: "INTAKE_SSO_HS256_SECRET",
		Claims:         config.SSOClaimNames{UserID: "sub", Email: "email", DisplayName: "name"},
	}
	short := []byte("too-short")
	_, err := sso.New(cfg, short, silentLogger())
	if err == nil {
		t.Fatal("expected error for HS256 secret shorter than 32 bytes")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("error should mention the 32-byte minimum; got %v", err)
	}
}
