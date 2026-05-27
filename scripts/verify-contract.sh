#!/usr/bin/env bash
# verify-contract.sh — Local simulation of the ci.yml contract job.
# Mirrors the GitHub Actions gate logic so the staleness gate is runnable
# without a remote or GitHub Actions. GitHub Actions arm is deferred until
# a remote is configured (see ai/tasks/phase-0/README.md §7).
#
# Usage: bash scripts/verify-contract.sh
# Run from the repository root.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

echo "=== verify-contract.sh: local CI gate simulation ==="
echo ""

# ── 1. Pin gate: forbid caret or @latest on codegen tools ──────────────────
echo "--- [1/6] Checking for caret-pinned or @latest codegen tools..."
if grep -E '"(json-schema-to-typescript|ajv-cli|ajv-formats)":\s*"\^' package.json; then
  echo "FAIL: codegen tool is caret-pinned; PHASE_PLANNING §5 requires exact pins"
  exit 1
fi
if grep -E 'go install .*@latest' scripts/codegen-go.sh; then
  echo "FAIL: go-jsonschema installed with @latest; pin an exact version"
  exit 1
fi
echo "OK: all codegen tools are exact-pinned"
echo ""

# ── 2. Validate schema fixtures ────────────────────────────────────────────
echo "--- [2/6] Validating schema fixtures (accept case)..."
npm run validate-schema
echo ""

echo "--- [2/6] Validating schema fixtures (reject case — invalid fixture must fail)..."
npm run validate-schema:reject
echo ""

# ── 3. Regenerate types from schema ────────────────────────────────────────
echo "--- [3/6] Regenerating types from schema..."
npm run codegen
echo ""

# ── 4. Staleness gate ──────────────────────────────────────────────────────
echo "--- [4/6] Checking for drift in generated files (staleness gate)..."
if ! git diff --exit-code core/src/generated/payload.ts relay/internal/payload/types.go; then
  echo ""
  echo "FAIL: generated types are stale — run 'npm run codegen' and commit the result"
  exit 1
fi
echo "OK: generated files match committed versions (no drift)"
echo ""

# ── 5. TypeScript type-check ───────────────────────────────────────────────
echo "--- [5/6] Type-checking generated TypeScript (tsc --noEmit)..."
npm run -w @intake/core type-check
echo ""

# ── 6. Go build + vet ─────────────────────────────────────────────────────
echo "--- [6/6] Building and vetting generated Go..."
cd "${REPO_ROOT}/relay"
go build ./...
go vet ./internal/payload/...
cd "${REPO_ROOT}"
echo ""

echo "=== verify-contract.sh: ALL CHECKS PASSED ==="
