#!/usr/bin/env bash
# verify-contract.sh — Local simulation of the ci.yml contract job.
# Mirrors the GitHub Actions gate logic so the staleness gate is runnable
# without a remote or GitHub Actions. GitHub Actions arm is deferred until
# a remote is configured (see ai/tasks/phase-0/README.md §7).
#
# NOTE: this script runs `npm run codegen`. If the committed generated files are
# stale, it will regenerate them and the staleness check will exit non-zero,
# leaving a modified working tree. Review `git diff` after a non-zero exit.
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
echo "--- [1/7] Checking for caret-pinned or @latest codegen tools..."
bash scripts/check-pins.sh
echo ""

# ── 2. Validate schema fixtures ────────────────────────────────────────────
echo "--- [2/7] Validating schema fixtures (accept case)..."
npm run validate-schema
echo ""

echo "--- [3/7] Validating schema fixtures (reject case — invalid fixture must fail)..."
npm run validate-schema:reject
echo ""

# ── 4. Regenerate types from schema ────────────────────────────────────────
echo "--- [4/7] Regenerating types from schema..."
npm run codegen
echo ""

# ── 5. Staleness gate ──────────────────────────────────────────────────────
echo "--- [5/7] Checking for drift in generated files (staleness gate)..."
if ! git diff --exit-code core/src/generated/payload.ts relay/internal/payload/types.go; then
  echo ""
  echo "FAIL: generated types are stale — run 'npm run codegen' and commit the result"
  exit 1
fi
echo "OK: generated files match committed versions (no drift)"
echo ""

# ── 6. TypeScript type-check ───────────────────────────────────────────────
echo "--- [6/7] Type-checking generated TypeScript (tsc --noEmit)..."
npm run -w @openintake/core type-check
echo ""

# ── 7. Go build + vet ─────────────────────────────────────────────────────
echo "--- [7/7] Building and vetting generated Go..."
cd "${REPO_ROOT}/relay"
go build ./...
go vet ./internal/payload/...
cd "${REPO_ROOT}"
echo ""

echo "=== verify-contract.sh: ALL CHECKS PASSED ==="
