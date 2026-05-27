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
# Gate: no go get @latest for anthropic-sdk-go anywhere in scripts.
if grep -rE 'go get.*anthropics/anthropic-sdk-go@latest' scripts/; then
  echo "ERROR: a script installs anthropic-sdk-go @latest; pin an exact version" >&2
  fail=1
fi
[ "$fail" -eq 0 ] && echo "OK: all codegen tools are exact-pinned"
exit "$fail"
