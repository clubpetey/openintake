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

	problems, _ := startupProblems(cfg)
	if len(problems) != 4 {
		t.Errorf("problems len = %d; want 4 (anonymous-no-captcha + sso-both + bad-CIDR + bad-action)\nproblems: %v", len(problems), problems)
	}
}

func TestStartupProblems_CleanConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
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
