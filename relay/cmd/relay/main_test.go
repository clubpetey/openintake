package main

import (
	"strings"
	"testing"

	"intake/internal/config"
)

func TestStartupProblems_AnonymousWithoutCaptcha(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject" // isolate the anonymous gate from Validate

	problems := startupProblems(cfg)
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

	problems := startupProblems(cfg)
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

	problems := startupProblems(cfg)
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

	problems := startupProblems(cfg)
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

	problems := startupProblems(cfg)
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

	problems := startupProblems(cfg)
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

	problems := startupProblems(cfg)
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

	problems := startupProblems(cfg)
	if len(problems) != 4 {
		t.Errorf("problems len = %d; want 4 (anonymous-no-captcha + sso-both + bad-CIDR + bad-action)\nproblems: %v", len(problems), problems)
	}
}

func TestStartupProblems_CleanConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	// All other Phase 4/5 gate inputs default to safe values.
	problems := startupProblems(cfg)
	if len(problems) != 0 {
		t.Errorf("clean config problems = %v; want empty", problems)
	}
}
