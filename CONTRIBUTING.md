# Contributing to intake

Thanks for your interest in contributing. This document covers how the project is laid out, how changes flow from idea to merge, and the local commands you should run before opening a pull request.

## Code of conduct

This project follows the Contributor Covenant 2.1 (TBD — pending maintainer adoption; until then, treat all contributors with professional respect). If you encounter unacceptable behavior, email the maintainer privately (see `SECURITY.md` for the contact placeholder).

## Project layout

```
intake/
├── core/                # @intake/core — shared TypeScript engine
├── vue/                 # @intake/vue — Vue 3 widget package
├── relay/               # intake-relay Go binary + internal packages
├── license-tool/        # maintainer-only license signer (not published)
├── schema/              # payload.v1.json — wire contract (source of truth)
├── examples/            # vue-anonymous, webhook-receiver, docker-compose
├── scripts/             # codegen-go.sh, verify-contract.sh, check-pins.sh
├── docs/                # operator-facing documentation + design specs
└── ai/                  # task plans, lessons, phase READMEs (developer notes)
```

See `docs/PROJECT.md` §14 for the canonical repo layout description, and `docs/PROJECT.md` §15 for the build & release pipeline overview.

## Branch and merge model

intake develops in long-lived **phase branches** that batch related changes into a single bundled merge to `main`:

- `main` — the integration branch. Always green: every Phase N merge passes the Phase N final smoke before merging.
- `phase-N` — the active development branch for phase N. Sub-plans are implemented as a sequence of commits on this branch (typically one or more commits per sub-plan).
- Smaller fixes that don't belong to an active phase go directly to `main` via a PR.

When a phase completes, the maintainer merges the phase branch into `main` with `git merge --no-ff phase-N` so the merge commit preserves the phase boundary for history.

## Phase-driven development model

Multi-step changes follow the phase model documented in `ai/PHASE_PLANNING.md`:

1. **Spec** — author a design doc under `docs/specs/YYYY-MM-DD-<title>.md` that captures goals, ADRs, scope, and the testing strategy. Use the existing specs (e.g. `2026-05-29-phase-7-release-ops-design.md`) as the template.
2. **Phase README** — author `ai/tasks/phase-N/README.md` with the spec link, ADR summary, sub-plan index, dependency graph, build-fail checklist, and final smoke definition.
3. **Sub-plans** — break the phase into 3–6 sub-plans under `ai/tasks/phase-N/<N-letter>-<title>-plan.md`. Each sub-plan has a Goal, Architecture, Tech Stack, Files Touched table, ordered Tasks with checkboxes, and a mandatory Smoke section.
4. **Smoke** — the final sub-plan (typically `N-v`) runs every smoke and records the evidence inline in the README.
5. **LESSONS** — append any novel patterns or surprises to `ai/LESSONS.md` as numbered L0XX entries.

This model exists because Phase 0f shipped silently-broken Auth0 IaC and Phase 0g spent ~24 hours debugging downstream symptoms. The phase READMEs and build-fail checklists exist to make every silent-failure mode loud.

## Commit conventions

intake uses Conventional Commits with a phase-scoped scope:

```
<type>(<scope>): <short subject>

<optional body>

Co-Authored-By: <coauthor-line>
```

| Type | Use |
|---|---|
| `feat` | New feature or capability. |
| `fix` | Bug fix. |
| `docs` | Documentation only. |
| `test` | Tests only. |
| `chore` | Build, tooling, dependencies, governance files. |
| `refactor` | Refactor with no behavior change. |

The `<scope>` is the active sub-plan when one applies (e.g. `feat(6-iii):`, `chore(7-iv):`). For standalone main-branch fixes, use a directory scope (e.g. `fix(relay):`, `docs(README):`).

Examples from the actual history:

```
feat(6-iii): chatwoot multipart attachments — JSON conv-create + multipart msg-create
chore(7-iv): LICENSE — verbatim Apache 2.0 + project copyright
fix(5-iv): raise abuse-driver budget to (150,150) so per-session fires before budget
docs(7-iv): COMMERCIAL.md DRAFT — paid-adapter open-core terms
```

## Local pre-commit commands

Before opening a PR, run each of these locally and confirm they pass. The `phase-N` branch will not merge until they all pass; CI runs the same set on every push.

### Go

```bash
cd relay
go build ./...                  # compile every package
go vet ./...                    # static analysis
go test -race ./...             # full unit suite with race detector
golangci-lint run ./...         # curated lint ruleset (Phase 7+)
```

### TypeScript (core + vue)

```bash
cd core
npm ci                          # clean install from package-lock.json
npm run type-check              # tsc --noEmit
npm run build                   # production bundle
npm test                        # vitest
npm run lint                    # eslint . (Phase 7+)

cd ../vue
npm ci
npm run type-check
npm run build
npm test
npm run lint
```

### Repo-wide

```bash
npx prettier --check .          # formatting (Phase 7+)
bash scripts/verify-contract.sh # schema and codegen drift check
bash scripts/check-pins.sh      # tool/module pin verification
```

`scripts/verify-contract.sh` regenerates the types from `schema/payload.v1.json` and diffs them against the checked-in `relay/internal/payload/types.go` and `core/src/types.ts`. Any drift fails the check.

`scripts/check-pins.sh` verifies every pinned tool (`goreleaser`, `golangci-lint`, `prettier`, `eslint`, etc.) and Go module (`prometheus/client_golang`, `golang-jwt`, `keyfunc/v3`, `golang.org/x/time`) is exact-pinned. Caret-versioning is forbidden per `ai/PHASE_PLANNING.md` §5.

## Running the demo locally

The fastest way to see the full stack in action:

```bash
cd examples/docker-compose
docker-compose up -d
# In a browser: open http://localhost:5173 (the vue widget)
# In a terminal: docker-compose logs -f webhook-receiver to watch tickets arrive
docker-compose down -v          # tear down + remove volumes when done
```

See `docs/quickstart.md` for a full walkthrough.

## Adding a new adapter

1. **Read an existing adapter** — `relay/internal/adapter/webhook/webhook.go` for the simplest shape; `relay/internal/adapter/chatwoot/chatwoot.go` for a two-call upload-then-create pattern; `relay/internal/adapter/zendesk/zendesk.go` for an upload-token pattern.
2. **Mirror the interface** — implement the frozen `adapter.Adapter` interface from `relay/internal/adapter/adapter.go`: `Name()`, `Configure(map[string]any) error`, `Capabilities() Capabilities`, `Create(ctx, payload) (*Result, error)`.
3. **Implement attachments** — if the downstream system accepts attachments, advertise the MIME types via `Capabilities().AcceptedMIMETypes` and forward them via the downstream's native upload mechanism. See `docs/attachments.md` for the per-adapter behavior contract.
4. **Add tests** — copy the test layout from an existing adapter package. Include `TestConfigure_*`, `TestCreate_*`, `TestAttachments_*` (when applicable), and an error-path test for each downstream failure mode.
5. **Register in main.go** — add the adapter to `buildRegistry` in `relay/cmd/relay/main.go`. After Phase 7-i, registration failures flow into the consolidated startup-problems slice (see `docs/self-hosting.md`).
6. **Document** — add a row to `docs/adapters.md` matrix and a config example to `docs/self-hosting.md`.
7. **Tier decision** — Free or Paid? See `docs/PROJECT.md` §13 for the tiering rationale. Paid adapters require a runtime license check via `intake/license`; free adapters do not.

## Pull request expectations

- **One sub-plan per PR (when in a phase).** PRs against `phase-N` typically implement exactly one sub-plan from `ai/tasks/phase-N/`. Cross-phase PRs are rare and need an explicit rationale.
- **Link the issue or task plan.** Every PR description should link the `ai/tasks/...` plan or GitHub Issue it implements.
- **Smoke evidence in the PR body.** Paste the command + output for the smoke that proves the PR works. Phase smokes include "what command was run" and "what output was observed" — replicate that shape in the PR body.
- **Tests added or modified for every behavior change.** A behavior change with no test diff is a red flag in review.
- **No schema or frozen-seam changes in non-seam phases.** The frozen seams (`adapter.Adapter` interface, `payload.IntakePayload` types, `schema/payload.v1.json`, `auth.Middleware.Handler` signature, the chi route shape, etc.) only change in seam sub-plans (the `-i` plan of each phase) and only with an explicit ADR in the phase design spec.
- **Conventional commits with HEREDOC.** Multi-line commit messages use a single-quoted heredoc for cross-platform compatibility (see existing history).
- **No `--no-verify`, no `--amend`** unless the maintainer asks. Pre-commit hooks exist for a reason; if a hook fails, fix the underlying issue and create a new commit.

## Reviewer expectations

- Read the linked plan or spec before the diff.
- Verify the smoke evidence is real (commands match what would actually run; outputs are not hand-edited).
- Check the build-fail checklist in the phase README — every item should still hold after the PR.
- Run the relevant smoke locally for high-risk changes.

## Getting help

- **Design questions** — open a GitHub Discussion (or file an Issue tagged `design`) before authoring a spec.
- **Implementation questions** — comment on the sub-plan file in `ai/tasks/` with a checkbox-level question, or open an Issue.
- **Security issues** — see `SECURITY.md`. Do NOT file a public Issue.

## License

By contributing to intake, you agree that your contributions are licensed under Apache 2.0 (the project's primary license — see `LICENSE`). You retain copyright to your contributions; the Apache 2.0 license grants the project (and downstream users) the rights needed to use, modify, and distribute them.

Contributions to the paid adapters (`zendesk`, `linear`) are also under Apache 2.0 — the commercial license model (see `COMMERCIAL.md`) gates *runtime use* in production, not contribution or modification of the source.
