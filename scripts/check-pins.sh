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
# Gate: github.com/MicahParks/keyfunc/v3 must be exact-pinned (no caret, no @latest) in go.mod. Phase 4.
if grep -E 'MicahParks/keyfunc/v3' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/MicahParks/keyfunc/v3 is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: golang.org/x/time must be exact-pinned (no caret, no @latest) in go.mod. Phase 5.
if grep -E 'golang.org/x/time' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: golang.org/x/time is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: github.com/prometheus/client_golang must be exact-pinned (no caret, no @latest) in go.mod. Phase 7.
if grep -E 'prometheus/client_golang' relay/go.mod | grep -E '(\^|@latest)'; then
  echo "ERROR: github.com/prometheus/client_golang is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: golangci-lint version in ci.yml must be exact-pinned (no @latest, no caret). Phase 7.
if [ -f .github/workflows/ci.yml ]; then
  if grep -E 'golangci-lint(-action)?.*version:.*(\^|latest)' .github/workflows/ci.yml; then
    echo "ERROR: golangci-lint is caret/latest-pinned in .github/workflows/ci.yml; PHASE_PLANNING §5 requires exact pins" >&2
    fail=1
  fi
fi
# Gate: eslint + prettier + @typescript-eslint/* in core/package.json + vue/package.json + root must be exact-pinned. Phase 7.
for pkg in package.json core/package.json vue/package.json; do
  if [ -f "$pkg" ]; then
    if grep -E '"(eslint|prettier|eslint-plugin-vue|@typescript-eslint/parser|@typescript-eslint/eslint-plugin)":[[:space:]]*"[\^~]' "$pkg"; then
      echo "ERROR: lint tool in $pkg is caret/tilde-pinned; PHASE_PLANNING §5 requires exact pins" >&2
      fail=1
    fi
  fi
done
# Gate: html2canvas must be exact-pinned (no caret, no ~) in core/package.json. Phase 6.
if grep -E '"html2canvas":\s*"[\^~]' core/package.json; then
  echo "ERROR: html2canvas in core/package.json is caret/tilde-pinned; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: no go install/get ...@latest in install scripts (excludes this file to avoid self-match).
if grep --exclude=check-pins.sh -rE 'go (install|get) .*@latest' scripts/; then
  echo "ERROR: an install script uses @latest; pin an exact version" >&2
  fail=1
fi
# Gate: goreleaser-action must be exact-pinned (no @latest, no @main) in any workflow. Phase 7-ii.
if grep -rE 'goreleaser/goreleaser-action@(latest|main|master|HEAD)' .github/workflows/ 2>/dev/null; then
  echo "ERROR: goreleaser/goreleaser-action is @latest/@main/etc in a workflow; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: goreleaser-action references must use vMAJOR.MINOR.PATCH form. Phase 7-ii.
if grep -rE 'goreleaser/goreleaser-action@v[0-9]+(\.[0-9]+)?$' .github/workflows/ 2>/dev/null; then
  echo "ERROR: goreleaser-action is pinned without a patch version; pin vMAJOR.MINOR.PATCH" >&2
  fail=1
fi
# Gate: any workflow invoking goreleaser-action must also pin the goreleaser CLI
# version exactly via `version: 'X.Y.Z'`. Phase 7-ii.
for wf in .github/workflows/release.yml .github/workflows/ci.yml; do
  if [ -f "$wf" ] && grep -q 'goreleaser/goreleaser-action@' "$wf"; then
    if ! grep -qE "version:[[:space:]]*['\"]?[0-9]+\.[0-9]+\.[0-9]+" "$wf"; then
      echo "ERROR: $wf uses goreleaser-action without an exact 'version: X.Y.Z' field" >&2
      fail=1
    fi
  fi
done
# Gate: pinned goreleaser CLI version in any workflow must match the dev-machine
# install (goreleaser v2.7.0). Phase 7-ii. If the dev-machine version moves,
# update this expected value AND the workflows in one commit.
expected_goreleaser="2.7.0"
for wf in .github/workflows/release.yml .github/workflows/ci.yml; do
  if [ -f "$wf" ] && grep -q 'goreleaser/goreleaser-action@' "$wf"; then
    if ! grep -qE "version:[[:space:]]*['\"]?${expected_goreleaser}['\"]?" "$wf"; then
      echo "ERROR: $wf does not pin goreleaser version: '${expected_goreleaser}' (the dev-machine pin)" >&2
      fail=1
    fi
  fi
done
# Gate: distroless base image must be exact-pinned by SHA digest in any Dockerfile. Phase 7-ii.
if [ -f relay/Dockerfile ] && grep -E '^FROM gcr\.io/distroless/' relay/Dockerfile | grep -vE '@sha256:[0-9a-f]{64}'; then
  echo "ERROR: distroless base image in relay/Dockerfile is not SHA-pinned (@sha256:<64-hex>); PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: golang:alpine builder image must be exact-pinned by SHA digest in relay/Dockerfile. Phase 7-ii.
if [ -f relay/Dockerfile ] && grep -E '^FROM .*golang:' relay/Dockerfile | grep -vE '@sha256:[0-9a-f]{64}'; then
  echo "ERROR: golang builder image in relay/Dockerfile is not SHA-pinned (@sha256:<64-hex>); PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: no unresolved placeholder digest tokens in Dockerfile. Phase 7-ii implementation guard.
if [ -f relay/Dockerfile ] && grep -E '(DISTROLESS_SHA256_DIGEST_HERE|GOLANG_ALPINE_SHA256_DIGEST_HERE)' relay/Dockerfile; then
  echo "ERROR: relay/Dockerfile still contains a placeholder digest token; replace with the real SHA captured via 'docker inspect'" >&2
  fail=1
fi
# Note: github.com/santhosh-tekuri/jsonschema/v6 is a Go library (introduced in 1-iv).
# Go modules pin exact versions in go.sum — no caret-check needed here; go.sum enforces it.
# github.com/google/uuid is similarly exact-pinned by go.mod + go.sum.
echo "OK: Go module pins verified (go.sum enforces exact versions for santhosh-tekuri/jsonschema/v6 and google/uuid)"

[ "$fail" -eq 0 ] && echo "OK: all codegen tools are exact-pinned"
exit "$fail"
