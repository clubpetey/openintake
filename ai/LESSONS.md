# LESSONS.md — Self-Annealing Loop

Patterns learned from corrections and mistakes. Review at session start.

---

## Project-Specific

---

## General Patterns

---

### L003: go-jsonschema does not validate JSON Schema `const` (only `enum`)

`go-jsonschema` treats a `{"type":"string","const":"1.0"}` field as a plain Go `string` with no value enforcement. The generator emits typed string consts and `UnmarshalJSON` validators for `enum` values, but `const` is silently downgraded to an unvalidated `string` field.

**Consequence:** `relay/internal/payload/types.go` will accept any value for `schema_version` — e.g. `"9.9"` — without error at unmarshal time. The TypeScript generator (`json-schema-to-typescript`) DOES emit a literal type for `const`, so the two generated targets behave differently.

**Rule:** Phase 1's relay MUST re-validate `schema_version` (and any other `const`-constrained field) at the HTTP/request boundary. Do not rely on the Go type system to enforce `const`-derived invariants. Reference: `relay/internal/payload/types.go`.

---

### L002: go-jsonschema v0.19.0 — correct module path, binary name, and flags

The plan referenced `github.com/omissis/go-jsonschema/cmd/gojsonschema@v0.19.0` but at v0.19.0 the module's own `go.mod` declares `module github.com/atombender/go-jsonschema` and the `cmd/gojsonschema` subpackage does not exist.

**Correct install command:** `go install github.com/atombender/go-jsonschema@v0.19.0`

**Binary name:** `go-jsonschema` (not `gojsonschema`)

**Flag to get `IntakePayload` as root struct name:** add `--struct-name-from-title`. Without it the generator derives the name from the filename (`PayloadV1Json`). Since the schema has `"title": "IntakePayload"`, this flag is required.

**Rule:** When using omissis/go-jsonschema redirect in plans, verify the actual `go.mod` module path matches before using it. Always run `go-jsonschema --help` to confirm flag names after installing, and check the generated root struct name matches the schema title.

---

### L001: `vue-tsc --noEmit` and `vue-tsc -b` catch different errors

The `npm run type-check` script (configured as `vue-tsc --noEmit`) does NOT catch every error that `vue-tsc -b` (project-references / build mode, used by `npm run build` and by Quinoa's `./gradlew build`) catches. Specifically, dead-code TS2367 ("This comparison appears to be unintentional because the types have no overlap") slips through `--noEmit` but trips `-b`.

**Where it hit:** Phase 3-v Task 1 F9 fix (commit `ee765f2`). A 1-line TS2367 in a `.vue` SFC's `<script setup lang="ts">` block passed local `npm run type-check` but failed Quinoa's build step on a subsequent agent's `./gradlew build`. The implementer's local pre-commit gate (`type-check` only) didn't reproduce the failure.

**Rule:** for local pre-commit verification on Vue work, run the build path that mirrors CI — `./gradlew build` (which invokes Quinoa's `vue-tsc -b`), or at minimum `cd src/main/webui && npm run build`. `npm run type-check` is a fast inner-loop check but not a complete pre-commit gate.

---
