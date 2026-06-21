#!/usr/bin/env bash
set -euo pipefail

# Pinned exactly — codegen output is a committed artifact (PHASE_PLANNING §5).
# Module is github.com/atombender/go-jsonschema; the installed binary is 'go-jsonschema'.
GOJSONSCHEMA_VERSION="v0.19.0"

# Anchor to repo root so this works whether invoked via `npm run codegen:go`
# (cwd = repo root) or via `go generate ./...` from inside the relay tree.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

# `go install` is idempotent and fast when the version is already in the module
# cache; always running it guarantees the PINNED version is the one used.
go install "github.com/atombender/go-jsonschema@${GOJSONSCHEMA_VERSION}"
BIN="$(go env GOPATH)/bin/go-jsonschema"

"$BIN" \
  --package payload \
  --struct-name-from-title \
  --schema-output https://openintake.dev/schema/payload.v1.json=relay/internal/payload/types.go \
  schema/payload.v1.json

echo "Generated relay/internal/payload/types.go"
