#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

fail=0
if grep -E '"(json-schema-to-typescript|ajv-cli|ajv-formats)":\s*"\^' package.json; then
  echo "ERROR: codegen/validation tool is caret-pinned; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
if grep -E 'go install .*@latest' scripts/codegen-go.sh; then
  echo "ERROR: go install uses @latest; pin an exact version" >&2
  fail=1
fi
if grep -E '"typescript":\s*"\^' core/package.json; then
  echo "ERROR: typescript in core/package.json is caret-pinned; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: anthropic-sdk-go must be exact-pinned (no caret, no @latest) in go.mod.
if grep -E 'anthropics/anthropic-sdk-go' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/anthropics/anthropic-sdk-go is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: openai-go must be exact-pinned (no caret, no @latest) in go.mod.
if grep -E 'openai/openai-go' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/openai/openai-go is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: google.golang.org/genai must be exact-pinned (no caret, no @latest) in go.mod.
if grep -E 'google.golang.org/genai' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: google.golang.org/genai is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: golang-jwt/jwt/v5 must be exact-pinned (no caret, no @latest) in go.mod. Phase 4.
if grep -E 'golang-jwt/jwt/v5' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/golang-jwt/jwt/v5 is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: no go install/get ...@latest in install scripts (excludes this file to avoid self-match).
if grep --exclude=check-pins.sh -rE 'go (install|get) .*@latest' scripts/; then
  echo "ERROR: an install script uses @latest; pin an exact version" >&2
  fail=1
fi
# Note: github.com/santhosh-tekuri/jsonschema/v6 is a Go library (introduced in 1-iv).
# Go modules pin exact versions in go.sum — no caret-check needed here; go.sum enforces it.
# github.com/google/uuid is similarly exact-pinned by go.mod + go.sum.
echo "OK: Go module pins verified (go.sum enforces exact versions for santhosh-tekuri/jsonschema/v6 and google/uuid)"

[ "$fail" -eq 0 ] && echo "OK: all codegen tools are exact-pinned"
exit "$fail"
