#!/usr/bin/env bash
set -euo pipefail

# Pinned exactly — codegen output is a committed artifact (PHASE_PLANNING §5).
# The module is github.com/atombender/go-jsonschema (its go.mod declares this
# path; the omissis/go-jsonschema redirect at v0.19.0 is broken).
# The installed binary is 'go-jsonschema' (not 'gojsonschema').
GOJSONSCHEMA_VERSION="v0.19.0"
BIN="$(go env GOPATH)/bin/go-jsonschema"

if [ ! -x "$BIN" ]; then
  echo "Installing go-jsonschema ${GOJSONSCHEMA_VERSION}..."
  go install "github.com/atombender/go-jsonschema@${GOJSONSCHEMA_VERSION}"
fi

"$BIN" \
  --package payload \
  --struct-name-from-title \
  --schema-output https://intake.dev/schema/payload.v1.json=relay/internal/payload/types.go \
  schema/payload.v1.json

echo "Generated relay/internal/payload/types.go"
