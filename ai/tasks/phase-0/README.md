# Phase 0 — Contract Spine

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement the sub-plans task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

The foundational phase. Establishes the monorepo, the canonical wire contract (`schema/payload.v1.json`), the codegen pipeline that derives TypeScript and Go types from that schema, and the CI gate that fails the build if generated types drift from the schema. Nothing here is runnable end-to-end — Phase 0 produces the *contract* that every later phase consumes.

## 1. Spec link

- Phasing & decomposition: [docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md](../../../docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md)
- Product v0 spec: [docs/PROJECT.md](../../../docs/PROJECT.md) — §4 (payload schema), §14 (repo layout), §15 (build/release)

## 2. Architectural Decision Record (ADR) summary

- **Schema is the single source of truth; types are generated, never hand-written.** `schema/payload.v1.json` (JSON Schema draft 2020-12) is authored by hand; `core/src/generated/payload.ts` and `relay/internal/payload/types.go` are generated artifacts checked into git. **Triggers to revisit:** (a) a generator produces types too poor to use directly (wrap, don't hand-edit); (b) the two-major-version support window (PROJECT.md §4) forces a `payload.v2.json` — at which point codegen runs per-version.
- **Generated files are committed AND CI-verified fresh.** Committing them keeps `go build` / `tsc` working without a codegen step in every consumer; the CI staleness gate (`git diff --exit-code` after regen) prevents the committed copy from drifting. **Triggers to revisit:** (a) generators become non-deterministic across machines (then generate-in-CI-only); (b) merge-conflict pain on generated files outweighs the convenience.
- **Codegen tools are exact-pinned, not caret-pinned.** Codegen produces deploy-time-load-bearing artifacts; per PHASE_PLANNING §5 a silent generator change is a multi-hour failure class. **Triggers to revisit:** none planned; upgrades are deliberate one-PR bumps.
- **Monorepo with an npm workspace (`core`, `vue`) + a separate Go module (`relay`) + a separate Go module (`license-tool`).** Two Go modules because `license-tool` is maintainer-only and excluded from release. **Triggers to revisit:** (a) `vue` and `core` version cadences diverge enough to want separate publishes (they already publish independently; this is about workspace tooling); (b) a shared Go module between relay and license-tool is wanted.

## 3. Sub-plan index

| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 0-i | [Monorepo skeleton & tooling](0-i-monorepo-skeleton-plan.md) | scaffolding | S | Not started |
| 0-ii | [payload.v1.json schema](0-ii-payload-schema-plan.md) | schema authoring | M | Not started |
| 0-iii | [Codegen pipeline (TS + Go)](0-iii-codegen-pipeline-plan.md) | codegen | M | Not started |
| 0-iv | [CI staleness gate](0-iv-ci-staleness-gate-plan.md) | CI | S | Not started |

## 4. Dependency graph

```
0-i (skeleton) ──► 0-ii (schema) ──► 0-iii (codegen) ──► 0-iv (CI gate)
```

Strictly serial. 0-iii needs the schema (0-ii) to generate from and the package/module layout (0-i) to write into. 0-iv wires the codegen command from 0-iii into CI.

## 5. Tool version pin list

Exact versions. Confirm the latest patch at install time in sub-plan 0-i Task 1, then pin the confirmed exact version in `package.json` / `go.mod` / workflow files. Caret (`^`) is forbidden for the codegen tools.

| Tool | Version | Reason |
|---|---|---|
| Go (toolchain) | 1.23.2 (exact) | relay + license-tool toolchain; `go.mod` `toolchain go1.23.2` pins it for reproducible CI. Confirmed installed: go1.23.2 (1.23.5 unavailable on this machine). |
| Node.js | 24.12.x | CI runner + local; pinned via `.nvmrc` and `engines`. Confirmed installed: v24.12.0 (LTS 20.18 unavailable; engines lower bound kept at >=20.18). |
| npm | 11.x | ships with Node 24.12.0. Confirmed installed: 11.6.2. |
| typescript | 5.6.3 (exact) | compiles generated `payload.ts` in 0-iii smoke; exact to keep tsc diagnostics stable |
| json-schema-to-typescript | 15.0.4 (exact) | TS codegen; **exact** — generator output is a committed artifact (PHASE_PLANNING §5) |
| go-jsonschema (`github.com/atombender/go-jsonschema`) | v0.19.0 (exact) | Go codegen; installed via `go install github.com/atombender/go-jsonschema@v0.19.0`; binary installed as `go-jsonschema` (not `gojsonschema`); use `--struct-name-from-title` to emit `IntakePayload` (not `PayloadV1Json`); **exact** for the same reason. NOTE: the omissis/go-jsonschema redirect at v0.19.0 does not contain the `cmd/gojsonschema` package — use the atombender module path directly. |
| ajv-cli | 5.0.0 (exact) | validates sample payloads against the schema in 0-ii smoke |

> If a pinned version above is unavailable or a newer patch is required at install, 0-i Task 1 records the actual installed version and updates this table in the same PR. The constraint is *exact pinning*, not these specific numbers.

## 6. Build-fail checklist

The phase's CI pipeline (sub-plan 0-iv) MUST fail the build on any of these:

- [ ] `git diff --exit-code` is non-zero after running codegen → generated types are stale relative to the schema. **Fail.**
- [ ] `tsc --noEmit` reports any error compiling `core/src/generated/payload.ts`. **Fail.**
- [ ] `go build ./...` in `relay/` fails to compile `relay/internal/payload/types.go`. **Fail.**
- [ ] `ajv validate` rejects the known-good sample payload, OR accepts the known-bad sample. **Fail.**
- [ ] Any codegen tool invoked without an exact version (a `^` or unpinned spec in `package.json`, or `@latest` in a `go install`). **Fail** (grep gate in CI).

## 7. Final smoke (mandatory)

Proves the contract spine works as a round-trip on a clean checkout.

```
1. Pre-condition: fresh clone of the repo at the phase-0 merge commit; Node 20.18 + Go 1.23.5 installed; no generated-file edits.
2. Execution:
   a. From repo root: `npm ci` then `npm run codegen` (runs both TS and Go generators).
   b. Run `git status --porcelain schema core/src/generated relay/internal/payload`.
   c. Edit schema/payload.v1.json: add an optional property `"debug_note": {"type": "string"}` under client. Re-run `npm run codegen`.
   d. Run `git status --porcelain`.
3. Verification:
   - After (a)+(b): porcelain output is EMPTY — committed generated files already match the schema (no drift).
   - `cd relay && go build ./...` succeeds; `npm run -w @intake/core type-check` (tsc --noEmit) succeeds.
   - After (c)+(d): porcelain output shows BOTH core/src/generated/payload.ts AND relay/internal/payload/types.go as modified, each containing a new `debug_note`/`DebugNote` field — proving a single schema edit propagates to both targets.
   - `npx ajv validate -s schema/payload.v1.json -d schema/testdata/payload.valid.json` exits 0; `... -d schema/testdata/payload.invalid.json` exits non-zero.
4. Teardown / repeat: `git checkout -- schema core relay` reverts the step-(c) edit; smoke is idempotent and re-runnable.
```

A simulation of this smoke runs in CI (sub-plan 0-iv): CI regenerates and asserts `git diff --exit-code` is clean, so a PR that edits the schema without regenerating is rejected.

**GitHub Actions arm deferred:** This repo has no git remote (`git init` only), so the push/PR/`gh workflow run` steps in the 0-iv plan cannot execute. The workflow files (`.github/workflows/ci.yml` and `.github/workflows/codegen-negative-test.yml`) are committed as deliverables and will activate automatically once a remote is configured. In the meantime, `scripts/verify-contract.sh` provides an equivalent local gate: it runs the pin-grep, fixture validation, codegen, `git diff --exit-code` staleness check, `tsc --noEmit`, and `go build/vet` in order — exit 0 on a clean tree, non-zero on drift. The green/red/revert proof was executed locally and confirmed correct (red path: `git diff --exit-code` exits 1 after schema mutation + regen, proving drift is detected).

## 8. Done criteria

- [ ] All four sub-plans complete; each sub-plan's own smoke passes.
- [ ] The phase final smoke (§7) passes from a clean clone.
- [ ] No build-fail-checklist (§6) item is triggered on `main`.
- [ ] Codegen tool versions pinned exactly in `package.json` and recorded in §5.
- [ ] LESSONS.md updated with anything novel learned (e.g. generator determinism quirks).
- [ ] Bundled PR opened linking this README.

## 9. Notes carried from the design

- **Name placeholder:** all artifacts use `intake` / `@intake/*` / module path `intake`. The design (§6) flags this as a one-scripted-rename gate; 0-i isolates name-bearing tokens accordingly.
- **Provider/adapter breadth is NOT in this phase** — Phase 0 only defines the payload contract, not any runtime.
