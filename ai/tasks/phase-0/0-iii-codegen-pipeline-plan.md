# Sub-plan 0-iii — Codegen Pipeline (TS + Go)

## 1. Goal

Wire the schema → types generators: `json-schema-to-typescript` produces `core/src/generated/payload.ts`, and `go-jsonschema` produces `relay/internal/payload/types.go`. Expose both behind a single root `npm run codegen`. Commit the generated outputs. Prove that a schema edit, re-run, regenerates both targets and that both compile.

## 2. Design references

- Codegen targets: [docs/PROJECT.md](../../../docs/PROJECT.md) §4 ("CI generates…"), §14
- Tool choice + exact-pin rationale: design doc §4 (Q7); phase README §2, §5
- Generators: `json-schema-to-typescript` 15.0.4, `go-jsonschema` v0.19.0

## 3. Files touched

| File | Create/Modify | Why |
|---|---|---|
| `package.json` (root) | Modify | add `json-schema-to-typescript` devDep; real `codegen` + `codegen:ts` + `codegen:go` scripts |
| `schema/codegen.config.json` | Create | documents the generator invocations + output paths (single reference) |
| `core/src/generated/payload.ts` | Create (generated) | TS types; committed |
| `relay/internal/payload/types.go` | Create (generated) | Go types; committed |
| `relay/internal/payload/doc.go` | Create | package doc + `go:generate` directive for the Go generator |
| `scripts/codegen-go.sh` | Create | wraps `go-jsonschema` install-check + invocation (cross-platform via bash) |

## 4. Tasks

- [ ] **Step 1: Install the TS generator (exact)**

Modify root `package.json` `devDependencies`:

```json
{
  "devDependencies": {
    "json-schema-to-typescript": "15.0.4"
  }
}
```

Run:

```bash
npm install
```

Expected: `json-schema-to-typescript@15.0.4` installed (exact, no caret).

- [ ] **Step 2: Add the TS codegen script and generate**

Modify root `package.json` `scripts`, replacing the failing stub:

```json
{
  "scripts": {
    "codegen": "npm run codegen:ts && npm run codegen:go",
    "codegen:ts": "json2ts -i schema/payload.v1.json -o core/src/generated/payload.ts --no-additionalProperties --bannerComment \"// GENERATED from schema/payload.v1.json — DO NOT EDIT. Run: npm run codegen\"",
    "codegen:go": "bash scripts/codegen-go.sh"
  }
}
```

Run only the TS half now:

```bash
npm run codegen:ts
```

Expected: creates `core/src/generated/payload.ts` containing an exported `interface IntakePayload` with nested interfaces (e.g. `Submission`, `Client`, `Viewport`, `Conversation`, etc.) matching the schema.

- [ ] **Step 3: Verify the generated TS compiles**

Replace `core/src/index.ts` so it re-exports the generated types (gives `tsc` a reason to type-check them and gives consumers a stable import):

```typescript
// @intake/core — shared TS engine. Populated in Phase 1.
export * from "./generated/payload.js";
```

Run:

```bash
npm run -w @intake/core type-check
```

Expected: `tsc --noEmit` exits 0. The generated `payload.ts` compiles under `strict`.

- [ ] **Step 4: Write the Go codegen wrapper script**

Create `scripts/codegen-go.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Pinned exactly — codegen output is a committed artifact (PHASE_PLANNING §5).
# The omissis/go-jsonschema CLI installs as the binary 'gojsonschema'.
GOJSONSCHEMA_VERSION="v0.19.0"
BIN="$(go env GOPATH)/bin/gojsonschema"

if [ ! -x "$BIN" ]; then
  echo "Installing gojsonschema ${GOJSONSCHEMA_VERSION}..."
  go install "github.com/omissis/go-jsonschema/cmd/gojsonschema@${GOJSONSCHEMA_VERSION}"
fi

"$BIN" \
  --package payload \
  --schema-output https://intake.dev/schema/payload.v1.json=relay/internal/payload/types.go \
  schema/payload.v1.json

echo "Generated relay/internal/payload/types.go"
```

> CI installs the pinned version fresh each run (no `$GOPATH/bin` cache), so the simple `-x` existence check is fine there. For repeated local runs after a version bump, delete `$(go env GOPATH)/bin/gojsonschema` first so the new version installs.

> The `--schema-output <$id>=<path>` form maps the schema's `$id` to the output file; `--package payload` sets the Go package. Confirm the exact flag names against `gojsonschema --help` at install (0-i recorded the version); adjust flag spelling if the pinned version differs, and note any change in the phase README §5.

Make it executable:

```bash
chmod +x scripts/codegen-go.sh
```

- [ ] **Step 5: Write the Go package doc + go:generate directive**

Create `relay/internal/payload/doc.go`:

```go
// Package payload defines the canonical widget→relay wire contract.
//
// types.go is GENERATED from schema/payload.v1.json — DO NOT EDIT by hand.
// Regenerate from the repo root with: npm run codegen
//
//go:generate bash ../../../scripts/codegen-go.sh
package payload
```

- [ ] **Step 6: Generate the Go types**

Run from repo root:

```bash
npm run codegen:go
```

Expected: installs `gojsonschema@v0.19.0` if absent, then writes `relay/internal/payload/types.go` with a `package payload` and an exported `IntakePayload` struct plus nested structs, each field carrying `json:"..."` tags matching the schema property names.

- [ ] **Step 7: Verify the generated Go compiles**

Run:

```bash
cd relay && go build ./... && go vet ./internal/payload/... && cd ..
```

Expected: builds and vets clean. (`go vet` catches malformed struct tags from the generator.)

- [ ] **Step 8: Document the pipeline in codegen.config.json**

Create `schema/codegen.config.json` (reference/manifest — the scripts are authoritative, this documents them):

```json
{
  "source": "schema/payload.v1.json",
  "targets": [
    {
      "lang": "typescript",
      "tool": "json-schema-to-typescript@15.0.4",
      "output": "core/src/generated/payload.ts",
      "command": "npm run codegen:ts"
    },
    {
      "lang": "go",
      "tool": "github.com/omissis/go-jsonschema@v0.19.0",
      "output": "relay/internal/payload/types.go",
      "package": "payload",
      "command": "npm run codegen:go"
    }
  ],
  "note": "Generated files are committed AND verified fresh in CI (sub-plan 0-iv). DO NOT hand-edit generated outputs."
}
```

- [ ] **Step 9: Run the full pipeline end-to-end**

Run:

```bash
npm run codegen
```

Expected: both `codegen:ts` and `codegen:go` run; both generated files (re)written identically (no diff vs. what Steps 2 & 6 produced — determinism check).

- [ ] **Step 10: Verify determinism (re-run produces no diff)**

Run:

```bash
npm run codegen && git diff --exit-code core/src/generated/payload.ts relay/internal/payload/types.go
```

Expected: exits 0 — regenerating produces byte-identical output. If non-zero, the generator is non-deterministic; record in `ai/LESSONS.md` and pin/normalize (e.g. sort keys) before proceeding.

- [ ] **Step 11: Commit**

```bash
git add package.json package-lock.json schema/codegen.config.json scripts/codegen-go.sh \
  core/src/generated/payload.ts core/src/index.ts relay/internal/payload/doc.go relay/internal/payload/types.go
git commit -m "feat(phase-0): codegen pipeline (json-schema-to-typescript + go-jsonschema)"
```

## 5. Smoke

```
1. Pre-condition: 0-ii merged; npm install run; Go 1.23.5 available.
2. Execution:
   a. npm run codegen
   b. git diff --exit-code on the two generated paths
   c. Add optional property "debug_note": {"type": "string"} to client.properties in schema/payload.v1.json
   d. npm run codegen
   e. git status --porcelain core/src/generated relay/internal/payload
   f. git checkout -- schema core relay  (revert)
3. Verification:
   - (b) exits 0 — committed outputs already match the schema.
   - cd relay && go build ./... succeeds; npm run -w @intake/core type-check succeeds.
   - (e) shows BOTH payload.ts and types.go modified, each now containing a debug_note / DebugNote field — single edit propagated to both targets.
4. Teardown / repeat: the Step (f) checkout reverts; idempotent and re-runnable.
```

## 6. Done criteria

- [ ] `npm run codegen` regenerates both targets; both are committed.
- [ ] Generated TS compiles (`tsc --noEmit`); generated Go builds + vets clean.
- [ ] Codegen is deterministic (Step 10 diff is empty on re-run).
- [ ] A schema edit propagates to both targets (smoke §5).
- [ ] Generators pinned exactly (no caret in `package.json`; `@v0.19.0` in the Go install script).
