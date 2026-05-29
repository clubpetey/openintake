#!/usr/bin/env bash
#
# Q9 startup-gate smoke: each misconfig YAML must cause the relay to exit 1
# with the matching consolidated Error log line. The combined fixture must
# emit ONE log line listing all four problems.
#
# Requires INTAKE_SSO_HS256 to be set as a dummy value so secret resolution
# doesn't error before Q9 fires. Any non-empty value works.
set -euo pipefail

cd "$(dirname "$0")/../../../.."

export INTAKE_SSO_HS256="${INTAKE_SSO_HS256:-dummy-secret-32-bytes-padded----}"

run_misconfig() {
  local name="$1"
  local fixture="$2"
  local expected_substring="$3"

  echo "=== Q9 smoke: $name ==="
  local output
  output=$(cd relay && go run ./cmd/relay --config "../$fixture" 2>&1 || true)

  if echo "$output" | grep -q "relay: startup config errors"; then
    echo "OK: consolidated error log line present"
  else
    echo "FAIL: missing 'relay: startup config errors' line"
    echo "Output:"
    echo "$output"
    exit 1
  fi

  if echo "$output" | grep -q "$expected_substring"; then
    echo "OK: matched expected problem '$expected_substring'"
  else
    echo "FAIL: expected problem substring '$expected_substring' not found"
    echo "Output:"
    echo "$output"
    exit 1
  fi
  echo
}

run_misconfig "anonymous-no-captcha"  "relay/cmd/relay/smoke/anonymous-no-captcha.yaml"  "anonymous"
run_misconfig "sso-both"              "relay/cmd/relay/smoke/sso-both.yaml"              "both"
run_misconfig "sso-neither"           "relay/cmd/relay/smoke/sso-neither.yaml"           "neither"
run_misconfig "bad-cidr"              "relay/cmd/relay/smoke/bad-cidr.yaml"              "not-a-cidr"
run_misconfig "bad-action"            "relay/cmd/relay/smoke/bad-action.yaml"            "action_on_exceeded"

echo "=== Q9 smoke: combined ==="
combined_output=$(cd relay && go run ./cmd/relay --config "../relay/cmd/relay/smoke/combined.yaml" 2>&1 || true)
for substr in "anonymous" "both" "not-a-cidr" "action_on_exceeded"; do
  if echo "$combined_output" | grep -q "$substr"; then
    echo "OK: combined fixture matched '$substr'"
  else
    echo "FAIL: combined fixture missing '$substr'"
    echo "Output:"
    echo "$combined_output"
    exit 1
  fi
done
log_count=$(echo "$combined_output" | grep -c "relay: startup config errors" || true)
if [ "$log_count" -ne 1 ]; then
  echo "FAIL: expected exactly 1 'startup config errors' line; got $log_count"
  echo "Output:"
  echo "$combined_output"
  exit 1
fi
echo

echo "=== Q9 smoke: attachments-combined (Phase 5 + Phase 6 in ONE log line) ==="
# Asserts the fix for the 6-iv ordering bug: when a config has BOTH Phase-5
# AND Phase-6 misconfigs, the SINGLE consolidated "startup config errors"
# log line must list problems from BOTH gates. Prior to the fix, the Phase 5
# gate called os.Exit(1) before validateAttachments() ran.
ac_output=$(cd relay && go run ./cmd/relay --config "../relay/cmd/relay/smoke/attachments-combined.yaml" 2>&1 || true)
for substr in "anonymous" "not-a-cidr" "action_on_exceeded" "storage.mode" "max_size_bytes"; do
  if echo "$ac_output" | grep -q "$substr"; then
    echo "OK: attachments-combined matched '$substr'"
  else
    echo "FAIL: attachments-combined missing '$substr'"
    echo "Output:"
    echo "$ac_output"
    exit 1
  fi
done
ac_log_count=$(echo "$ac_output" | grep -c "relay: startup config errors" || true)
if [ "$ac_log_count" -ne 1 ]; then
  echo "FAIL: attachments-combined expected exactly 1 'startup config errors' line; got $ac_log_count"
  echo "Output:"
  echo "$ac_output"
  exit 1
fi
echo "OK: attachments-combined emitted exactly 1 consolidated log line"
echo
echo "All Q9 smokes passed."
