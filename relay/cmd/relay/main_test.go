package main

import (
	"strings"
	"testing"

	"intake/internal/config"
)

func TestStartupProblems_ReturnsParsedPrefixes(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.0/8", "192.168.0.0/16"}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, prefixes := startupProblems(cfg)
	if len(problems) != 0 {
		t.Fatalf("problems = %v; want empty", problems)
	}
	if len(prefixes) != 2 {
		t.Fatalf("prefixes len = %d; want 2", len(prefixes))
	}
	if prefixes[0].String() != "10.0.0.0/8" {
		t.Errorf("prefixes[0] = %q; want 10.0.0.0/8", prefixes[0].String())
	}
	if prefixes[1].String() != "192.168.0.0/16" {
		t.Errorf("prefixes[1] = %q; want 192.168.0.0/16", prefixes[1].String())
	}
}

func TestStartupProblems_BadCIDR_DropsInvalidFromPrefixes(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.0/8", "not-a-cidr", "192.168.0.0/16"}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, prefixes := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1 (bad-CIDR)", problems)
	}
	// Successful parses are still returned even when others fail, so callers that
	// proceed past the gate (which fires os.Exit on non-empty problems) don't run
	// at all in this case; this assertion just pins the return-shape contract.
	if len(prefixes) != 2 {
		t.Errorf("prefixes len = %d; want 2 (the two valid CIDRs)", len(prefixes))
	}
}

func TestStartupProblems_AnonymousWithoutCaptcha(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject" // isolate the anonymous gate from Validate
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, _ := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1 problem", problems)
	}
	if !strings.Contains(problems[0], "anonymous") || !strings.Contains(problems[0], "captcha") {
		t.Errorf("problem %q does not mention both 'anonymous' and 'captcha'", problems[0])
	}
}

func TestStartupProblems_AnonymousWithCaptchaButNotRequiredForAnonymous(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = true
	cfg.Captcha.RequiredFor = []string{"email"} // missing "anonymous"
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject" // isolate the anonymous gate from Validate
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, _ := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1 problem (required_for excludes anonymous)", problems)
	}
}

func TestStartupProblems_AllowWithoutCaptchaSilencesAnonymousGate(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = true

	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject" // satisfy Validate
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, _ := startupProblems(cfg)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty (escape hatch engaged)", problems)
	}
}

func TestStartupProblems_SSOBothSet(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.SSO = true
	cfg.Auth.SSO.JWKSURL = "https://example/.well-known/jwks.json"
	cfg.Auth.SSO.HS256SecretEnv = "INTAKE_SSO_HS256"
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, _ := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "both") {
		t.Errorf("sso-both problem %q does not say 'both'", problems[0])
	}
}

func TestStartupProblems_SSONeitherSet(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.SSO = true
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, _ := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "neither") {
		t.Errorf("sso-neither problem %q does not say 'neither'", problems[0])
	}
}

func TestStartupProblems_BadCIDR(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.0/8", "not-a-cidr"}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, _ := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "trusted_proxies") || !strings.Contains(problems[0], "not-a-cidr") {
		t.Errorf("bad-CIDR problem %q does not mention the bad value", problems[0])
	}
}

func TestStartupProblems_BadActionOnExceeded(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "queue" // v0 only supports "reject"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, _ := startupProblems(cfg)
	if len(problems) == 0 {
		t.Fatal("problems is empty; want a problem for action_on_exceeded=queue")
	}
	found := false
	for _, p := range problems {
		if strings.Contains(p, "action_on_exceeded") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("problems %v does not contain an action_on_exceeded entry", problems)
	}
}

func TestStartupProblems_AllFourMisconfigsAtOnce(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	cfg.Auth.Modes.SSO = true
	cfg.Auth.SSO.JWKSURL = "https://example/jwks"
	cfg.Auth.SSO.HS256SecretEnv = "INTAKE_SSO_HS256"
	cfg.Server.TrustedProxies = []string{"not-a-cidr"}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "queue"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"

	problems, _ := startupProblems(cfg)
	if len(problems) != 4 {
		t.Errorf("problems len = %d; want 4 (anonymous-no-captcha + sso-both + bad-CIDR + bad-action)\nproblems: %v", len(problems), problems)
	}
}

func TestStartupProblems_CleanConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"
	// All other Phase 4/5 gate inputs default to safe values.
	problems, _ := startupProblems(cfg)
	if len(problems) != 0 {
		t.Errorf("clean config problems = %v; want empty", problems)
	}
}

func TestStartupProblems_EmptyActionOnExceeded_TreatedAsProblem(t *testing.T) {
	// A bare &config.Config{} has ActionOnExceeded == "". applyDefaults would
	// have set it to "reject" via Load, but tests that build Config directly
	// bypass that path. startupProblems must surface this as a Validate problem
	// so it's caught at startup rather than at first /turn.
	cfg := &config.Config{}
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"
	// No other gate inputs set → only the Validate empty-string case fires.

	problems, _ := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1 (empty ActionOnExceeded from Validate)", problems)
	}
	if !strings.Contains(problems[0], "action_on_exceeded") {
		t.Errorf("problem %q does not mention action_on_exceeded", problems[0])
	}
	if !strings.Contains(problems[0], "empty") {
		t.Errorf("problem %q does not mention 'empty'", problems[0])
	}
}

func TestStartupProblems_BadSessionTTL_TreatedAsProblem(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.PerSession.SessionTTL = "1 hour" // invalid — should be "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"            // valid
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"

	problems, _ := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1 (bad session_ttl)", problems)
	}
	if !strings.Contains(problems[0], "session_ttl") {
		t.Errorf("problem %q does not mention session_ttl", problems[0])
	}
}

func TestStartupProblems_BadIdleTTL_TreatedAsProblem(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.PerSession.SessionTTL = "1h" // valid
	cfg.RateLimit.PerIP.IdleTTL = "15min"      // invalid — should be "15m"
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"

	problems, _ := startupProblems(cfg)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1 (bad idle_ttl)", problems)
	}
	if !strings.Contains(problems[0], "idle_ttl") {
		t.Errorf("problem %q does not mention idle_ttl", problems[0])
	}
}

func TestStartupProblems_BothBadDurations_ReportsBoth(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.PerSession.SessionTTL = "1 hour" // invalid
	cfg.RateLimit.PerIP.IdleTTL = "15min"          // invalid
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"

	problems, _ := startupProblems(cfg)
	if len(problems) != 2 {
		t.Errorf("problems len = %d; want 2 (both durations bad)\nproblems: %v", len(problems), problems)
	}
}

func TestValidateAttachments_CleanConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 5_242_880
	cfg.Attachments.MaxTotalBytes = 10_485_760
	cfg.Attachments.AllowedMIMETypes = []string{"image/png", "image/jpeg", "image/webp"}
	cfg.Attachments.Storage.Mode = "forward"

	parsed, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty", problems)
	}
	if parsed.MaxSizeBytes != 5_242_880 {
		t.Errorf("parsed.MaxSizeBytes = %d; want 5_242_880", parsed.MaxSizeBytes)
	}
}

func TestValidateAttachments_BadStorageMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.Storage.Mode = "s3"
	cfg.Attachments.MaxSizeBytes = 1
	cfg.Attachments.MaxTotalBytes = 1
	cfg.Attachments.AllowedMIMETypes = []string{"image/png"}

	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "storage.mode") || !strings.Contains(problems[0], "s3") {
		t.Errorf("problem %q does not mention storage.mode + s3", problems[0])
	}
}

func TestValidateAttachments_CapInverted(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000
	cfg.Attachments.AllowedMIMETypes = []string{"image/png"}

	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 1 {
		t.Fatalf("problems = %v; want exactly 1", problems)
	}
	if !strings.Contains(problems[0], "max_size_bytes") || !strings.Contains(problems[0], "max_total_bytes") {
		t.Errorf("problem %q does not name both caps", problems[0])
	}
}

func TestValidateAttachments_CapInvertedSkippedWhenTotalZero(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 0 // disabled aggregate cap; per-attachment is the only gate
	cfg.Attachments.AllowedMIMETypes = []string{"image/png"}

	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty when total cap is 0", problems)
	}
}

func TestValidateAttachments_UnknownMIMETypeIsWarnNotFatal(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 1
	cfg.Attachments.MaxTotalBytes = 1
	cfg.Attachments.AllowedMIMETypes = []string{"image/heic"} // no adapter advertises this
	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty (unknown-MIME is warn-not-fatal)", problems)
	}
}

func TestValidateAttachments_DisabledShortCircuit(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = false
	cfg.Attachments.Storage.Mode = "s3" // would normally be fatal — but disabled trumps
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000

	_, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("problems = %v; want empty when Enabled=false", problems)
	}
}
