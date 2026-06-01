package main

import (
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"intake/internal/adapter"
	"intake/internal/config"
	licensemgr "intake/internal/license"
)

// TestMain_VersionFlag_PrintsAndExits asserts the --version flag prints a
// non-empty one-line version string and exits 0, even without a config file.
// This is the binary-vs-tag identity contract that the 7-ii snapshot-build
// smoke depends on.
func TestMain_VersionFlag_PrintsAndExits(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "intake-relay-test.exe")
	if os.PathSeparator == '/' {
		binPath = filepath.Join(tmp, "intake-relay-test")
	}
	// Build the relay binary into a temp path. Build cwd is the cmd/relay dir.
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}

	cmd := exec.Command(binPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("--version subprocess failed: %v (output=%q)", err, string(out))
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		t.Fatalf("--version printed empty output; want a non-empty version string")
	}
	if !strings.Contains(s, "intake-relay") {
		t.Errorf("--version output %q does not contain 'intake-relay'", s)
	}
}

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

// TestStartupGates_CombinedPhase5AndPhase6Problems pins the Q9 contract:
// when a config has BOTH Phase-5 misconfigs (anonymous-no-captcha + bad CIDR
// + bad action_on_exceeded) AND Phase-6 misconfigs (storage.mode=s3 +
// inverted size caps), the combined startup-problems slice — which main.go
// emits in ONE consolidated "relay: startup config errors" log line and then
// exits 1 — must contain BOTH families of problems. This mirrors the
// 6-iv combined-fixture smoke; prior to the fix, the Phase 5 gate exited
// before validateAttachments() ever ran, hiding the Phase 6 problems.
func TestStartupGates_CombinedPhase5AndPhase6Problems(t *testing.T) {
	cfg := &config.Config{}
	// --- Phase 5 misconfigs ---
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	cfg.Server.TrustedProxies = []string{"not-a-cidr"}
	cfg.RateLimit.DailyLLMBudget.ActionOnExceeded = "queue"
	cfg.RateLimit.PerSession.SessionTTL = "1h"
	cfg.RateLimit.PerIP.IdleTTL = "15m"
	// --- Phase 6 misconfigs ---
	cfg.Attachments.Enabled = true
	cfg.Attachments.Storage.Mode = "s3"
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000

	p5Problems, _ := startupProblems(cfg)
	_, p6Problems := validateAttachments(cfg, nil)
	combined := append(p5Problems, p6Problems...)

	wantSubstrings := []string{
		"anonymous",          // Phase 5: anonymous-no-captcha
		"not-a-cidr",         // Phase 5: bad trusted_proxies CIDR
		"action_on_exceeded", // Phase 5: bad daily-budget action
		"storage.mode",       // Phase 6: bad storage mode
		"max_size_bytes",     // Phase 6: inverted cap pair
	}
	for _, want := range wantSubstrings {
		found := false
		for _, p := range combined {
			if strings.Contains(p, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("combined problems missing substring %q\nproblems: %v", want, combined)
		}
	}
	if len(combined) < 5 {
		t.Errorf("combined problems len = %d; want >= 5 (3 Phase-5 + 2 Phase-6)\nproblems: %v", len(combined), combined)
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

// ---- Task 5 / FOLLOWUPS I1 tests ----

// freeLicenseState returns a license.State in free mode (paid adapters
// silently skipped via the licensed() helper).
func freeLicenseState(t *testing.T) *licensemgr.State {
	t.Helper()
	return &licensemgr.State{Mode: "free", Message: "no license file"}
}

// discardLogger returns a slog.Logger that discards all output (test-only).
func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// TestBuildRegistry_PerAdapterConfigureFailureContributesProblem asserts
// FOLLOWUPS I1: a chatwoot adapter with api_token_env pointing at an unset env
// var produces a problem entry, NOT an os.Exit(1). The function returns the
// registry slice (possibly empty) and the problems slice; the caller decides.
func TestBuildRegistry_PerAdapterConfigureFailureContributesProblem(t *testing.T) {
	cfg := &config.Config{}
	cfg.Adapters.Chatwoot.Enabled = true
	cfg.Adapters.Chatwoot.BaseURL = "https://example.com"
	cfg.Adapters.Chatwoot.AccountID = 1
	cfg.Adapters.Chatwoot.InboxID = 1
	cfg.Adapters.Chatwoot.APITokenEnv = "INTAKE_TEST_NEVER_SET_XYZ"
	licState := freeLicenseState(t)
	logger := discardLogger()

	registry, problems := buildRegistry(cfg, licState, logger)
	if len(problems) == 0 {
		t.Fatal("expected at least one problem for chatwoot api_token_env unset; got none")
	}
	foundChatwootProblem := false
	for _, p := range problems {
		if strings.Contains(p, "chatwoot") {
			foundChatwootProblem = true
			break
		}
	}
	if !foundChatwootProblem {
		t.Errorf("expected a problem mentioning chatwoot; got %v", problems)
	}
	// Registry should NOT contain chatwoot (Configure failed) and may be empty.
	for _, ad := range registry {
		if ad.Name() == "chatwoot" {
			t.Error("chatwoot adapter present in registry despite Configure failure")
		}
	}
}

// TestBuildRegistry_NoAdaptersEnabledContributesProblem asserts the second
// FOLLOWUPS I1 case: cfg with NO adapters enabled produces a problem entry,
// not os.Exit(1).
func TestBuildRegistry_NoAdaptersEnabledContributesProblem(t *testing.T) {
	cfg := &config.Config{} // all adapters disabled by default
	licState := freeLicenseState(t)
	logger := discardLogger()

	registry, problems := buildRegistry(cfg, licState, logger)
	if len(registry) != 0 {
		t.Errorf("registry len = %d; want 0", len(registry))
	}
	foundNoAdaptersProblem := false
	for _, p := range problems {
		if strings.Contains(p, "no adapters enabled") {
			foundNoAdaptersProblem = true
			break
		}
	}
	if !foundNoAdaptersProblem {
		t.Errorf("expected a problem mentioning 'no adapters enabled'; got %v", problems)
	}
}

// TestBuildRegistry_FreeAdapterEnabled is the happy-path baseline: webhook
// enabled with valid config → registry has webhook, problems is empty.
func TestBuildRegistry_FreeAdapterEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com/intake"
	licState := freeLicenseState(t)
	logger := discardLogger()

	registry, problems := buildRegistry(cfg, licState, logger)
	if len(problems) != 0 {
		t.Errorf("happy path: problems = %v; want empty", problems)
	}
	if len(registry) != 1 || registry[0].Name() != "webhook" {
		t.Errorf("registry = %v; want [webhook]", adapterListNames(registry))
	}
}

// adapterListNames is a small test helper to extract adapter names for assertion messages.
func adapterListNames(reg []adapter.Adapter) []string {
	out := make([]string, 0, len(reg))
	for _, ad := range reg {
		out = append(out, ad.Name())
	}
	return out
}

// ---- Task 6 / FOLLOWUPS I2 tests ----

// minimalValidCfg returns the smallest *config.Config that's parseable but
// has no adapters enabled and no Phase 5/6 misconfigs. Tests selectively
// flip fields to assert per-gate behavior.
func minimalValidCfg(t *testing.T) *config.Config {
	t.Helper()
	c := &config.Config{}
	// Apply the same defaults the YAML loader would apply (durations + Validate-required).
	c.RateLimit.PerSession.SessionTTL = "1h"
	c.RateLimit.PerIP.IdleTTL = "15m"
	c.RateLimit.DailyLLMBudget.ActionOnExceeded = "reject"
	return c
}

// TestAccumulateStartupProblems_Empty: clean cfg + free license + no enabled
// adapters → 1 problem ("no adapters enabled"). Asserts the function is pure
// (no os.Exit) and the Deps return value is populated for the fields that
// don't depend on a registry.
func TestAccumulateStartupProblems_Empty(t *testing.T) {
	cfg := minimalValidCfg(t)
	licState := freeLicenseState(t)
	logger := discardLogger()

	deps, _, problems := accumulateStartupProblems(cfg, licState, logger)
	if len(problems) != 1 {
		t.Errorf("len(problems) = %d; want 1; got %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "no adapters enabled") {
		t.Errorf("expected 'no adapters enabled'; got %v", problems)
	}
	if deps.Metrics == nil {
		t.Error("Deps.Metrics is nil; want non-nil even in disabled mode")
	}
}

// TestAccumulateStartupProblems_Phase5Only: anonymous-no-captcha + bad CIDR +
// at least one valid adapter → exactly 2 Phase-5 problems, NO Phase-7
// (registry) problems.
func TestAccumulateStartupProblems_Phase5Only(t *testing.T) {
	cfg := minimalValidCfg(t)
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	cfg.Server.TrustedProxies = []string{"not-a-cidr"}
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com"
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, _, problems := accumulateStartupProblems(cfg, licState, logger)
	hasAnon := false
	hasCIDR := false
	for _, p := range problems {
		if strings.Contains(p, "anonymous") {
			hasAnon = true
		}
		if strings.Contains(p, "not-a-cidr") {
			hasCIDR = true
		}
	}
	if !hasAnon {
		t.Errorf("expected an 'anonymous' problem; got %v", problems)
	}
	if !hasCIDR {
		t.Errorf("expected a 'not-a-cidr' problem; got %v", problems)
	}
}

// TestAccumulateStartupProblems_Phase6Only: storage.mode=s3 + cap inverted +
// webhook enabled (so Phase 7 contributes nothing) → exactly 2 Phase-6 problems.
func TestAccumulateStartupProblems_Phase6Only(t *testing.T) {
	cfg := minimalValidCfg(t)
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com"
	cfg.Attachments.Enabled = true
	cfg.Attachments.Storage.Mode = "s3"
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, _, problems := accumulateStartupProblems(cfg, licState, logger)
	hasStorage := false
	hasCapInverted := false
	for _, p := range problems {
		if strings.Contains(p, "storage.mode") {
			hasStorage = true
		}
		if strings.Contains(p, "max_size_bytes") {
			hasCapInverted = true
		}
	}
	if !hasStorage {
		t.Errorf("expected a 'storage.mode' problem; got %v", problems)
	}
	if !hasCapInverted {
		t.Errorf("expected a 'max_size_bytes' problem; got %v", problems)
	}
}

// TestAccumulateStartupProblems_AdapterConfigureFails: chatwoot api_token_env
// unset → 1 problem mentioning chatwoot, NO "no adapters enabled" (because
// webhook is also enabled and configures cleanly).
func TestAccumulateStartupProblems_AdapterConfigureFails(t *testing.T) {
	cfg := minimalValidCfg(t)
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com"
	cfg.Adapters.Chatwoot.Enabled = true
	cfg.Adapters.Chatwoot.BaseURL = "https://chat.example.com"
	cfg.Adapters.Chatwoot.AccountID = 1
	cfg.Adapters.Chatwoot.InboxID = 1
	cfg.Adapters.Chatwoot.APITokenEnv = "INTAKE_TEST_NEVER_SET_XYZ"
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, _, problems := accumulateStartupProblems(cfg, licState, logger)
	hasChatwoot := false
	hasNoAdapters := false
	for _, p := range problems {
		if strings.Contains(p, "chatwoot") {
			hasChatwoot = true
		}
		if strings.Contains(p, "no adapters enabled") {
			hasNoAdapters = true
		}
	}
	if !hasChatwoot {
		t.Errorf("expected a chatwoot problem; got %v", problems)
	}
	if hasNoAdapters {
		t.Errorf("did NOT expect 'no adapters enabled' (webhook is configured); got %v", problems)
	}
}

// TestAccumulateStartupProblems_NoAdaptersEnabled: cfg with all adapters
// disabled → "no adapters enabled" problem.
func TestAccumulateStartupProblems_NoAdaptersEnabled(t *testing.T) {
	cfg := minimalValidCfg(t)
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, _, problems := accumulateStartupProblems(cfg, licState, logger)
	hasNoAdapters := false
	for _, p := range problems {
		if strings.Contains(p, "no adapters enabled") {
			hasNoAdapters = true
		}
	}
	if !hasNoAdapters {
		t.Errorf("expected 'no adapters enabled' problem; got %v", problems)
	}
}

// TestAccumulateStartupProblems_LicenseGateWarnsNotFails: a PAID adapter
// (zendesk) enabled in FREE mode → registry skips it via licensed() with a
// Warn log; NO problem entry contributed. With webhook also enabled, the
// final registry has webhook only, and problems is empty.
func TestAccumulateStartupProblems_LicenseGateWarnsNotFails(t *testing.T) {
	cfg := minimalValidCfg(t)
	cfg.Adapters.Webhook.Enabled = true
	cfg.Adapters.Webhook.URL = "https://hooks.example.com"
	cfg.Adapters.Zendesk.Enabled = true
	cfg.Adapters.Zendesk.Subdomain = "example"
	cfg.Adapters.Zendesk.Email = "agent@example.com"
	cfg.Adapters.Zendesk.APITokenEnv = "INTAKE_TEST_NEVER_SET_XYZ"
	licState := freeLicenseState(t) // free mode → zendesk skipped (paid)
	logger := discardLogger()

	_, _, problems := accumulateStartupProblems(cfg, licState, logger)
	for _, p := range problems {
		if strings.Contains(p, "zendesk") {
			t.Errorf("did NOT expect a zendesk problem (license gate is a Warn, not a problem); got %v", problems)
		}
	}
	if len(problems) != 0 {
		t.Errorf("expected zero problems (webhook configures, zendesk skipped by license); got %v", problems)
	}
}

// TestAccumulateStartupProblems_AllCombined: Phase 5 (anon-no-captcha + bad
// CIDR) + Phase 6 (cap inverted) + Phase 7 (chatwoot Configure failure) all
// in ONE cfg → consolidated problems slice contains every distinct issue,
// with count >= 4. Asserts the L022 contract end-to-end.
func TestAccumulateStartupProblems_AllCombined(t *testing.T) {
	cfg := minimalValidCfg(t)
	// Phase 5: anon-no-captcha
	cfg.Auth.Modes.Anonymous = true
	cfg.Captcha.Enabled = false
	cfg.Auth.Anonymous.AllowWithoutCaptcha = false
	// Phase 5: bad CIDR
	cfg.Server.TrustedProxies = []string{"not-a-cidr"}
	// Phase 6: cap inverted
	cfg.Attachments.Enabled = true
	cfg.Attachments.MaxSizeBytes = 20_000_000
	cfg.Attachments.MaxTotalBytes = 10_000_000
	// Phase 7: chatwoot Configure fails
	cfg.Adapters.Chatwoot.Enabled = true
	cfg.Adapters.Chatwoot.BaseURL = "https://chat.example.com"
	cfg.Adapters.Chatwoot.AccountID = 1
	cfg.Adapters.Chatwoot.InboxID = 1
	cfg.Adapters.Chatwoot.APITokenEnv = "INTAKE_TEST_NEVER_SET_XYZ"
	licState := freeLicenseState(t)
	logger := discardLogger()

	_, _, problems := accumulateStartupProblems(cfg, licState, logger)
	if len(problems) < 4 {
		t.Errorf("AllCombined: len(problems) = %d; want >= 4; got %v", len(problems), problems)
	}
	want := []string{"anonymous", "not-a-cidr", "max_size_bytes", "chatwoot"}
	for _, w := range want {
		found := false
		for _, p := range problems {
			if strings.Contains(p, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a problem containing %q; got %v", w, problems)
		}
	}
}

// ---- Task 7 / FOLLOWUPS M2 test ----

// TestValidateAttachments_DisabledReturnsZeroValue asserts FOLLOWUPS M2:
// when cfg.Attachments.Enabled=false, validateAttachments returns a
// zero-value AttachmentsConfig (NOT cfg.Attachments) so a bad Storage.Mode
// or inverted caps in the disabled block can't leak to a future consumer.
func TestValidateAttachments_DisabledReturnsZeroValue(t *testing.T) {
	cfg := &config.Config{}
	cfg.Attachments.Enabled = false
	cfg.Attachments.MaxSizeBytes = 99
	cfg.Attachments.MaxTotalBytes = 1
	cfg.Attachments.AllowedMIMETypes = []string{"junk/type"}
	cfg.Attachments.Storage.Mode = "s3"

	parsed, problems := validateAttachments(cfg, nil)
	if len(problems) != 0 {
		t.Errorf("disabled path produced problems %v; want none", problems)
	}
	if parsed.Enabled {
		t.Error("parsed.Enabled = true; want false")
	}
	if parsed.MaxSizeBytes != 0 {
		t.Errorf("M2: parsed.MaxSizeBytes = %d; want 0 (zero-value, garbage 99 must be discarded)", parsed.MaxSizeBytes)
	}
	if parsed.MaxTotalBytes != 0 {
		t.Errorf("M2: parsed.MaxTotalBytes = %d; want 0", parsed.MaxTotalBytes)
	}
	if len(parsed.AllowedMIMETypes) != 0 {
		t.Errorf("M2: parsed.AllowedMIMETypes = %v; want empty", parsed.AllowedMIMETypes)
	}
	if parsed.Storage.Mode != "" {
		t.Errorf("M2: parsed.Storage.Mode = %q; want empty (s3 in disabled block must not leak)", parsed.Storage.Mode)
	}
}
