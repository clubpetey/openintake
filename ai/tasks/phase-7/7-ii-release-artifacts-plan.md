# 7-ii Release Artifacts — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After 7-i locked the relay code seam (`accumulateStartupProblems`, the new `metrics` package, `Deps.Metrics`, lint configs, initial-fix sweep, CI lint+test jobs), 7-ii authors the **release artifact configuration**: `relay/Dockerfile` (multi-stage, distroless), `relay/.goreleaser.yaml` (5-platform matrix + dockers block + archives + checksum), `.github/workflows/release.yml` (tag-triggered; never executed in Phase 7), an exact-pin extension to `scripts/check-pins.sh` for `goreleaser` + `goreleaser-action` + the distroless base image digest, and CI jobs that prove the snapshot pipeline works end-to-end (`goreleaser check` + `goreleaser release --snapshot --clean`). Every artifact is generated **locally**; NOTHING is published in Phase 7. The `release.yml` workflow is AUTHORED but never EXECUTED in Phase 7 — its correctness is verified via `goreleaser check`, `actionlint` (optional), and the snapshot build smoke. The Phase 0–6 frozen seams stay untouched: no change to `relay/internal/adapter/adapter.go`, `relay/internal/payload/types.go`, `schema/payload.v1.json`, or any Go production code beyond extending `ci.yml`.

**Architecture:** Phase 7's deliverable is a complete release-engineering surface that a maintainer can flip on with one tag push when the public-release prerequisites are met (Q1 final product name, GitHub remote, ghcr/npm tokens). The Dockerfile and `.goreleaser.yaml` are the load-bearing artifacts; the workflow is glue. Two CI jobs (`goreleaser-check`, `snapshot-build`) prove the artifacts are correct without publishing anything. The Dockerfile is multi-stage: `golang:1.23.2-alpine` builds a static binary; `gcr.io/distroless/static-debian12:nonroot` runs it. Image total < 50 MB, binary < 10 MB, both enforced as build-fail rows. `archives.files` is an **explicit allowlist** — never an exclude list — so secrets, `local-dev/`, and `.env` files cannot slip into a tarball. The release workflow is gated to `v[0-9]+.[0-9]+.[0-9]+` tag pushes; Phase 7 never produces such a tag.

**Tech Stack:** No new Go modules. No new npm packages. Three new external tools enter the supply chain: `goreleaser` (the CLI), `goreleaser/goreleaser-action` (the GitHub Action wrapper), and the distroless base image (pinned by SHA digest). Optionally `hadolint` for Dockerfile lint and `actionlint` for workflow YAML lint. All exact-pinned per PHASE_PLANNING §5.

---

## Design References

- README §2 ADR row "`goreleaser` builds 5 platforms; `release.yml` authored but never executed in Phase 7" — the canonical 5-platform matrix + the tag-gate pattern.
- README §5 Tool version pin list — `goreleaser` is exact-pinned in both `scripts/check-pins.sh` AND the workflow YAMLs; the SHA-pinned distroless base image is a `check-pins.sh` row.
- README §6 build-fail checklist rows — `docker build` exit 0, `nonroot` user invariant, image-size < 50 MB, `goreleaser check` clean, snapshot produces 5 archives + SHA256SUMS.txt + image, `actionlint` clean (when adopted), `tar -tf … | grep secrets|.env|local-dev` returns nothing.
- README §7 final smoke items 3 + 4 — the snapshot release smoke + npm pack dry-run smoke (the npm side stays in 7-v's driver; 7-ii authors the workflow steps that will run them on a future tag).
- README §8.1 frozen seams — `relay/internal/adapter/adapter.go`, `relay/internal/payload/types.go`, `schema/payload.v1.json`, the chi route shape: **none touched by 7-ii**.
- Phase 7 design spec §3.4 — Dockerfile target choice (distroless/static-debian12:nonroot), default exposed ports (8080 + 9090), `nonroot` UID 65532, size budget.
- Phase 7 design spec §3.5 — goreleaser platform matrix, `dockers:` block, `archives.files` explicit-allowlist rationale.
- Phase 7 design spec §5.6 — release-artifact deliverable list (Dockerfile, .goreleaser.yaml, release.yml, check-pins.sh extension).
- Phase 7 design spec §7.3 — release artifact generation flow (the snapshot path is the 7-ii smoke target).
- Phase 7 design spec §8.5 — snapshot-build failure modes (unsupported GOOS/GOARCH, missing archive file, version mismatch).
- LESSONS to mirror: L022 (consolidated startup problems — not directly applicable here but reflects the "fail loudly with all errors" discipline that goreleaser also follows by default). The cumulative L005/L011 (no secret leak) extends to release artifacts: NO `.env`, NO `local-dev/`, NO `secrets` in any tarball.
- Reference: existing `scripts/check-pins.sh` (see Files Touched) — the existing pin-pattern style is mirrored exactly.
- Reference: existing `.github/workflows/ci.yml` (3-job structure: checkout → setup-node → setup-go → npm ci → checks) — 7-ii adds 2-3 new jobs in the same shape.

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/Dockerfile` | Create (NEW) | Multi-stage build: golang:1.23.2-alpine builder + distroless/static-debian12:nonroot runner. Binary < 10 MB, image < 50 MB, runs as UID 65532. |
| `relay/.goreleaser.yaml` | Create (NEW) | goreleaser v2 config: 5-platform builds, archives with explicit allowlist, dockers block, checksum, release config, snapshot template, changelog filter. |
| `.github/workflows/release.yml` | Create (NEW) | Tag-triggered workflow (`v[0-9]+.[0-9]+.[0-9]+`); authored but NEVER executed in Phase 7. Two jobs: `release-relay` (goreleaser + ghcr image) and `release-npm` (publish `@intake/core` + `@intake/vue`). |
| `scripts/check-pins.sh` | Modify | Add 3 new pin gates: `goreleaser` version in `.github/workflows/release.yml` + `.github/workflows/ci.yml`, `goreleaser/goreleaser-action@vX` version, distroless base image SHA digest in `relay/Dockerfile`. |
| `.github/workflows/ci.yml` | Modify | Add `goreleaser-check` job (runs `goreleaser check` on `relay/.goreleaser.yaml`) and `snapshot-build` job (runs `goreleaser release --snapshot --clean` on PRs and uploads `./dist/` as an artifact). Optionally `dockerfile-lint` and `actionlint` jobs. |

No new files outside the four listed; no changes to relay Go source code, no schema change, no `go.mod` change, no npm dependency change. No `Dockerfile` outside `relay/`.

---

## Pin discipline (load-bearing — do not deviate)

| Tool | Where pinned | Pattern in `check-pins.sh` |
|---|---|---|
| `goreleaser` CLI | `.github/workflows/release.yml` + `.github/workflows/ci.yml` (both `goreleaser/goreleaser-action@vN` + `version: X.Y.Z` field) | grep for `goreleaser-action@v` without an exact `version:` neighbor; grep for `:latest` |
| `goreleaser/goreleaser-action` | `@v6.1.0` (latest stable as of 2026-06-01; verify against https://github.com/goreleaser/goreleaser-action/releases) | grep for `goreleaser/goreleaser-action@` without `@v6.1.0` |
| Distroless base image | `relay/Dockerfile` `FROM` line — `gcr.io/distroless/static-debian12:nonroot@sha256:<digest>` | grep for `gcr.io/distroless/` without `@sha256:` |
| `golang:1.23.2-alpine` (builder) | `relay/Dockerfile` `FROM` line — `golang:1.23.2-alpine@sha256:<digest>` | grep for `FROM golang:` without `@sha256:` |
| `goreleaser` CLI version | inside `goreleaser-action` `version:` field — `version: '2.7.0'` (latest stable as of 2026-06-01) | grep for `goreleaser-action@` followed by absent `version:` line within 5 lines |
| (optional) `hadolint/hadolint-action` | `.github/workflows/ci.yml` `@v3.x.y` | optional |
| (optional) `rhysd/actionlint` Docker image | `.github/workflows/ci.yml` `:vX.Y.Z` | optional |

**Versions to verify at Task 1 implementation time** (commands listed in Task 1 Step 1):

- `goreleaser` latest stable: `goreleaser --version` after `brew install goreleaser` or `go install github.com/goreleaser/goreleaser/v2@latest` then check; OR visit `https://github.com/goreleaser/goreleaser/releases/latest`. The plan assumes **v2.7.0** as of 2026-06-01; if the latest is different, use the latest and update every reference in this plan accordingly.
- `goreleaser-action`: visit `https://github.com/goreleaser/goreleaser-action/releases/latest`. Plan assumes **v6.1.0**.
- Distroless `static-debian12:nonroot` digest: `docker pull gcr.io/distroless/static-debian12:nonroot && docker inspect gcr.io/distroless/static-debian12:nonroot --format '{{index .RepoDigests 0}}'`. Plan uses a placeholder digest `sha256:abc...` in the file contents below — REPLACE with the real digest captured at Task 2 Step 0.
- `golang:1.23.2-alpine` digest: `docker pull golang:1.23.2-alpine && docker inspect golang:1.23.2-alpine --format '{{index .RepoDigests 0}}'`. Same digest-replacement pattern.

The plan's file contents below use the symbolic placeholder strings `DISTROLESS_SHA256_DIGEST_HERE` and `GOLANG_ALPINE_SHA256_DIGEST_HERE` where the maintainer-captured digests must be substituted at implementation time. The pin gate in `check-pins.sh` will catch any commit that leaves those placeholders unresolved (rejection grep on the literal placeholder strings).

---

## Tasks

### Task 0: Verify 7-i prerequisites are in place

**Files:** None (verification only).

- [ ] **Step 1: Confirm 7-i landed on `phase-7`**

Run: `git log --oneline phase-7 | head -20`
Expected: the most recent commits include `feat(7-i): metrics package`, `feat(7-i): accumulateStartupProblems`, `chore(7-i): initial lint sweep`, and CI jobs for `lint-go` + `lint-ts` + `test-go` + `test-ts`. If any are missing, STOP — 7-i is incomplete.

- [ ] **Step 2: Confirm `Deps.Metrics` field exists**

Run: `grep -n "Metrics \*metrics.Registry" relay/internal/server/deps.go`
Expected: one line printed. If missing, 7-i is incomplete — STOP.

- [ ] **Step 3: Confirm baseline tests are green**

Run: `cd relay && go test -race ./... && cd ..`
Expected: full suite green. Do not start 7-ii on a red baseline.

- [ ] **Step 4: Confirm `scripts/check-pins.sh` baseline is green**

Run: `bash scripts/check-pins.sh`
Expected: exit 0. 7-ii extends this script; the baseline must already pass.

- [ ] **Step 5: Confirm the current branch is `phase-7`**

Run: `git rev-parse --abbrev-ref HEAD`
Expected: `phase-7`. If not, switch: `git checkout phase-7`. Do NOT push.

- [ ] **Step 6: No commit for this task** — verification only.

---

### Task 1: Pin goreleaser version + extend check-pins.sh

**Files:** Modify `scripts/check-pins.sh`.

This task locks the tool versions BEFORE any artifact references them. The gate fails fast if a future change introduces a caret-pinned or `@latest` reference.

- [ ] **Step 1: Capture the exact versions to pin**

Run (in this order, recording each output verbatim into a temp note):

```bash
# goreleaser CLI version (download the binary for sanity-check; do not commit it)
curl -sSL https://api.github.com/repos/goreleaser/goreleaser/releases/latest | grep '"tag_name"' | head -1
# expected output: "tag_name": "v2.7.0"  (or newer; use whatever is current as of implementation date)

# goreleaser-action version
curl -sSL https://api.github.com/repos/goreleaser/goreleaser-action/releases/latest | grep '"tag_name"' | head -1
# expected output: "tag_name": "v6.1.0"

# Distroless base image digest
docker pull gcr.io/distroless/static-debian12:nonroot
docker inspect gcr.io/distroless/static-debian12:nonroot --format '{{index .RepoDigests 0}}'
# expected output: gcr.io/distroless/static-debian12@sha256:abc123...

# Golang alpine builder digest
docker pull golang:1.23.2-alpine
docker inspect golang:1.23.2-alpine --format '{{index .RepoDigests 0}}'
# expected output: golang@sha256:def456...
```

If your environment lacks Docker, fall back to:

```bash
curl -sSL "https://gcr.io/v2/distroless/static-debian12/manifests/nonroot" \
  -H 'Accept: application/vnd.docker.distribution.manifest.v2+json' \
  | grep '"digest"' | head -1
# (the index manifest is fine for SHA pinning; record whichever digest you use)
```

Record the 4 captured values:
- `GORELEASER_VERSION` (e.g. `2.7.0` — no `v` prefix for the `version:` field)
- `GORELEASER_ACTION_VERSION` (e.g. `v6.1.0` — keep the `v` prefix for the `uses:` field)
- `DISTROLESS_DIGEST` (e.g. `sha256:abc123...`)
- `GOLANG_ALPINE_DIGEST` (e.g. `sha256:def456...`)

These values get substituted into:
- The `relay/Dockerfile` (Task 2) — `DISTROLESS_DIGEST`, `GOLANG_ALPINE_DIGEST`
- The `.github/workflows/release.yml` (Task 5) — `GORELEASER_VERSION`, `GORELEASER_ACTION_VERSION`
- The `.github/workflows/ci.yml` (Task 7) — `GORELEASER_VERSION`, `GORELEASER_ACTION_VERSION`

- [ ] **Step 2: Extend `scripts/check-pins.sh` with 5 new gates**

Open `scripts/check-pins.sh`. After the last `fi` (currently around line 54) and BEFORE the trailing `echo "OK: Go module pins verified..."` line, INSERT the following block:

```bash
# Gate: goreleaser-action must be exact-pinned (no @latest, no @main) in any workflow. Phase 7.
if grep -rE 'goreleaser/goreleaser-action@(latest|main|master|HEAD)' .github/workflows/ 2>/dev/null; then
  echo "ERROR: goreleaser/goreleaser-action is @latest/@main/etc in a workflow; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: goreleaser-action references must use vN.M.P form (semver with patch). Phase 7.
if grep -rE 'goreleaser/goreleaser-action@v[0-9]+(\.[0-9]+)?$' .github/workflows/ 2>/dev/null; then
  echo "ERROR: goreleaser-action is pinned without a patch version; pin vMAJOR.MINOR.PATCH" >&2
  fail=1
fi
# Gate: goreleaser CLI version inside goreleaser-action must be exact. Phase 7.
# (We require that any workflow file invoking goreleaser-action also sets `version: 'X.Y.Z'`
# within 10 lines below the uses: line. A missing version: implies the action defaults to
# `latest`, which violates PHASE_PLANNING §5.)
for wf in .github/workflows/release.yml .github/workflows/ci.yml; do
  if [ -f "$wf" ] && grep -q 'goreleaser/goreleaser-action@' "$wf"; then
    # Find every line index of a goreleaser-action use; assert each has a matching version: line within 10 lines.
    awk '
      /goreleaser\/goreleaser-action@/ { use_line = NR; in_block = 1; found = 0; next }
      in_block && NR <= use_line + 10 && /version:[[:space:]]*['\''"]?[0-9]+\.[0-9]+\.[0-9]+/ { found = 1 }
      in_block && NR == use_line + 10 {
        if (!found) { print "MISSING_VERSION_NEAR_LINE:" use_line; exit 2 }
        in_block = 0
      }
      END {
        if (in_block && !found) { print "MISSING_VERSION_NEAR_LINE:" use_line; exit 2 }
      }
    ' "$wf" || {
      echo "ERROR: $wf uses goreleaser-action without an adjacent exact-pin 'version:' field" >&2
      fail=1
    }
  fi
done
# Gate: distroless base image must be exact-pinned by SHA digest in any Dockerfile. Phase 7.
if grep -rE '^FROM gcr\.io/distroless/' relay/Dockerfile 2>/dev/null | grep -vE '@sha256:[0-9a-f]{64}'; then
  echo "ERROR: distroless base image in relay/Dockerfile is not SHA-pinned (@sha256:<64-hex-digest>); PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: golang:alpine builder image must be exact-pinned by SHA digest in relay/Dockerfile. Phase 7.
if grep -rE '^FROM golang:' relay/Dockerfile 2>/dev/null | grep -vE '@sha256:[0-9a-f]{64}'; then
  echo "ERROR: golang builder image in relay/Dockerfile is not SHA-pinned (@sha256:<64-hex-digest>); PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
# Gate: no unresolved placeholder digest tokens left in Dockerfile. Phase 7 implementation guard.
if grep -rE '(DISTROLESS_SHA256_DIGEST_HERE|GOLANG_ALPINE_SHA256_DIGEST_HERE)' relay/Dockerfile 2>/dev/null; then
  echo "ERROR: relay/Dockerfile still contains a placeholder digest token; replace with the real SHA captured via 'docker inspect'" >&2
  fail=1
fi
```

Note: the awk block above is one option. If it proves too fragile across awk variants, replace with a simpler grep-based gate:

```bash
# Simpler alternative for the goreleaser-action version pin gate:
for wf in .github/workflows/release.yml .github/workflows/ci.yml; do
  if [ -f "$wf" ] && grep -q 'goreleaser/goreleaser-action@' "$wf"; then
    if ! grep -qE "version:[[:space:]]*['\"]?[0-9]+\.[0-9]+\.[0-9]+" "$wf"; then
      echo "ERROR: $wf uses goreleaser-action without an exact 'version: X.Y.Z' field" >&2
      fail=1
    fi
  fi
done
```

Choose the simpler form unless multiple goreleaser-action invocations live in one workflow (in which case the awk form is needed). Either way, the gate FAILS the CI run if a `version:` field is missing.

- [ ] **Step 3: Run the gate against the still-empty workflow set**

Run: `bash scripts/check-pins.sh`
Expected: exit 0 — the new gates pass because `.github/workflows/release.yml` does not exist yet (the `if [ -f "$wf" ]` guard handles that) and `relay/Dockerfile` does not exist yet (`grep -rE … relay/Dockerfile 2>/dev/null` returns nothing, which is acceptable).

If the script errors out, fix the syntax. The new gates MUST be silent on the current tree (no Dockerfile, no release.yml) AND active once Tasks 2 and 5 land.

- [ ] **Step 4: Commit**

```bash
git add scripts/check-pins.sh
git commit -m "$(cat <<'EOF'
chore(7-ii): extend check-pins.sh — goreleaser-action exact-pin, distroless+golang digest pins

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Author `relay/Dockerfile`

**Files:** Create `relay/Dockerfile`.

Multi-stage build per design spec §3.4. Stage 1 builds a CGO-disabled static binary with `-ldflags='-s -w' -trimpath`. Stage 2 is distroless/static-debian12:nonroot. The image gets OCI labels (`source`, `description`, `licenses`, `version`) for traceability. BuildKit cache mounts (`--mount=type=cache`) speed CI repeats.

- [ ] **Step 0: Capture the two SHA digests recorded in Task 1 Step 1**

Substitute the values for `DISTROLESS_SHA256_DIGEST_HERE` and `GOLANG_ALPINE_SHA256_DIGEST_HERE` into the file contents in Step 1 below. Do NOT commit a file that still contains either placeholder string — `check-pins.sh` will reject it.

- [ ] **Step 1: Write `relay/Dockerfile`**

Create `relay/Dockerfile` with the following exact contents (after substituting the two digests):

```dockerfile
# syntax=docker/dockerfile:1.7

# ============================================================================
# Stage 1: builder
#   - Pinned to golang:1.23.2-alpine by SHA digest (scripts/check-pins.sh
#     gate rejects any non-SHA reference).
#   - BuildKit cache mounts speed up `go mod download` + `go build` on CI repeats.
#   - CGO disabled + -trimpath + -ldflags='-s -w' produces a static, stripped binary
#     suitable for distroless/static (~10MB).
# ============================================================================
FROM --platform=$BUILDPLATFORM golang:1.23.2-alpine@sha256:GOLANG_ALPINE_SHA256_DIGEST_HERE AS builder

# ARGs for cross-compilation: BuildKit sets these per --platform target.
ARG TARGETOS
ARG TARGETARCH
# Version label for OCI metadata + the binary's --version flag (set by goreleaser).
ARG VERSION=dev

WORKDIR /src

# Copy module manifests first so `go mod download` is cacheable across source-only changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the rest of the relay source tree (cmd/, internal/, etc.).
COPY . .

# Build a static binary. The build cache mount is intentional: distinct source
# changes still benefit from cached compiled std-lib + unchanged-package artifacts.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
      -trimpath \
      -ldflags="-s -w -X intake/internal/version.Version=${VERSION}" \
      -o /out/intake-relay \
      ./cmd/relay

# ============================================================================
# Stage 2: runner (distroless/static-debian12:nonroot)
#   - SHA-pinned; gate in scripts/check-pins.sh rejects any non-SHA reference.
#   - distroless/static contains no shell, no package manager, no apt CVEs.
#   - The nonroot variant defaults USER to 65532:65532.
#   - Final image total: < 50 MB (enforced as build-fail row).
# ============================================================================
FROM gcr.io/distroless/static-debian12:nonroot@sha256:DISTROLESS_SHA256_DIGEST_HERE

# OCI image annotations (operator/registry visible metadata).
ARG VERSION=dev
LABEL org.opencontainers.image.source="https://github.com/intake/intake" \
      org.opencontainers.image.description="Intake relay — Go HTTP server for the intake widget" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}"

# Copy the static binary from the builder stage. Distroless provides /etc/passwd
# with the `nonroot` user mapped to UID 65532 — keep the same ownership.
COPY --from=builder --chown=nonroot:nonroot /out/intake-relay /intake-relay

# Distroless/static ships USER=nonroot (UID 65532) — restate it explicitly to
# satisfy auditors and the docker inspect Config.User invariant.
USER nonroot:nonroot

# 8080 — main relay HTTP server (config.server.listen)
# 9090 — Prometheus metrics endpoint (config.observability.metrics.addr; off-by-default)
EXPOSE 8080 9090

ENTRYPOINT ["/intake-relay"]
CMD ["--config", "/etc/intake/config.yaml"]
```

Notes on each line cluster:
- **`# syntax=docker/dockerfile:1.7`** — opts into BuildKit features (cache mounts, here-doc, COPY --link). Without it, the `--mount=type=cache` lines are silently ignored.
- **`FROM --platform=$BUILDPLATFORM golang:1.23.2-alpine@sha256:…`** — the builder runs on the host's native platform regardless of the target. Cross-compilation happens via `GOOS`/`GOARCH` env vars set from `TARGETOS`/`TARGETARCH`, which is faster than emulated build.
- **`COPY go.mod go.sum ./`** — module manifests separately so `go mod download` re-runs only when deps change. This is the canonical pattern for fast Go Docker builds.
- **`-ldflags="-s -w -X intake/internal/version.Version=${VERSION}"`** — `-s -w` strips the symbol + debug tables (saves several MB); `-X` injects the version string into the existing `version.Version` package variable, which the relay's `--version` flag prints. This is the binary-vs-tag identity invariant for §11.1 item 5.
- **`-trimpath`** — removes absolute build paths from the binary, making it reproducible.
- **`-o /out/intake-relay`** — the binary name is identical to the goreleaser output name, which makes the docker-image-from-goreleaser path (`dockers:` block) trivial.
- **`gcr.io/distroless/static-debian12:nonroot@sha256:…`** — distroless/static is for fully-static Go binaries (which is what we produce with `CGO_ENABLED=0`). The `:nonroot` variant ships `USER 65532:65532` baked in.
- **`USER nonroot:nonroot`** — explicit restatement so `docker inspect --format '{{.Config.User}}'` returns `nonroot:nonroot` (the build-fail row).
- **`EXPOSE 8080 9090`** — documentation only; doesn't open ports. Operators wire ports via compose / Kubernetes.

- [ ] **Step 2: Verify the build with `docker build`**

Run: `docker build -t intake-relay:test relay/`

Expected: exit 0. First build will pull the digests (slow); subsequent builds use the cache mounts and complete in seconds.

If `docker build` fails:
- "no such image" → the digest captured in Task 1 Step 1 is stale; re-pull and re-capture.
- "go: cannot find main module" → the `WORKDIR /src` + `COPY . .` pair didn't pick up the source; verify you ran from the repo root with `relay/` as the build context.

- [ ] **Step 3: Verify the image runs and reports nonroot**

Run:

```bash
docker inspect intake-relay:test --format '{{.Config.User}}'
# expected: nonroot:nonroot

docker images intake-relay:test --format '{{.Size}}'
# expected: a value smaller than "50MB" (e.g. "31.2MB"). If > 50MB, investigate
#   - ldflags=-s -w missing? → binary contains debug tables
#   - CGO_ENABLED accidentally enabled? → binary statically linked against libc
#   - extraneous COPY pulled in test fixtures? → tighten .dockerignore

docker run --rm intake-relay:test --version
# expected: prints the version string (Phase 7 default: "dev" — the goreleaser
#   build sets it to the tag string via the --build-arg VERSION=<tag>).
```

If `docker run --rm intake-relay:test --version` errors with "exec /intake-relay: exec format error", the binary was cross-compiled for the wrong arch — re-build with `--platform linux/amd64` (or your host arch) and re-test.

- [ ] **Step 4: Author `.dockerignore` if not present**

Check: `ls -la relay/.dockerignore`

If absent, create `relay/.dockerignore` with:

```
# Test artifacts + IDE files that have no business in the build context.
*_test.go
testdata/
.git/
.github/
.idea/
.vscode/
*.md
ai/
docs/
local-dev/
examples/
node_modules/
dist/
**/.DS_Store
```

The `*_test.go` exclusion is safe because `go build ./cmd/relay` doesn't compile `_test.go` files anyway, but excluding them shrinks the build context (faster docker builds). The `local-dev/` exclusion is the safety belt that complements `archives.files` allowlist in `.goreleaser.yaml`.

- [ ] **Step 5: Re-run check-pins.sh to confirm the new gates trigger correctly**

Run: `bash scripts/check-pins.sh`
Expected: exit 0. The distroless digest + golang digest gates now have a Dockerfile to match against; they pass because the digests are real (not the `_HERE` placeholders).

If the gate fails with "Dockerfile still contains a placeholder digest token", you skipped Step 0's substitution. Replace `DISTROLESS_SHA256_DIGEST_HERE` and `GOLANG_ALPINE_SHA256_DIGEST_HERE` with the real captured digests and re-run.

- [ ] **Step 6: Commit**

```bash
git add relay/Dockerfile relay/.dockerignore
git commit -m "$(cat <<'EOF'
feat(7-ii): relay Dockerfile — multi-stage builder + distroless/static-debian12:nonroot

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Author `relay/.goreleaser.yaml`

**Files:** Create `relay/.goreleaser.yaml`.

goreleaser v2 config covering all 5 platforms, archives with explicit allowlist, dockers block, checksum config, release config, snapshot template, changelog filter. The file lives under `relay/` so `goreleaser` is invoked from the `relay/` directory (matching `cmd/relay`'s path expectations). The repo's other directories (`core/`, `vue/`, etc.) are out of goreleaser's scope.

- [ ] **Step 1: Write `relay/.goreleaser.yaml`**

Create `relay/.goreleaser.yaml` with:

```yaml
# .goreleaser.yaml — Phase 7-ii
#
# Verified against goreleaser v2.7.0 (`goreleaser check` exit 0).
# Snapshot path (`goreleaser release --snapshot --clean`) is the load-bearing
# CI smoke; the tag-triggered publish path is wired in
# .github/workflows/release.yml but never executes in Phase 7.

version: 2

# `project_name` controls the archive name template and the default Docker
# image name. Stable across pre-release / final-release.
project_name: intake-relay

before:
  hooks:
    - go mod tidy
    - go vet ./...

# ============================================================================
# Builds — 5 platforms, one binary, ldflags inject the version string.
# Identical ldflags to relay/Dockerfile so the binary's --version matches.
# ============================================================================
builds:
  - id: relay
    main: ./cmd/relay
    binary: intake-relay
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w -X intake/internal/version.Version={{.Version}}
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      # Windows ARM64 builds are not produced; skip the matrix cell.
      - goos: windows
        goarch: arm64

# ============================================================================
# Archives — explicit allowlist (never an exclude list).
# Prevents accidental inclusion of .env, local-dev/, secrets, etc.
# tar.gz on linux/darwin; zip on windows.
# ============================================================================
archives:
  - id: relay-archive
    ids:
      - relay
    name_template: "intake-relay_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    formats: ['tar.gz']
    format_overrides:
      - goos: windows
        formats: ['zip']
    files:
      # Explicit allowlist — every file here must exist at the repo root
      # (not under relay/) because goreleaser resolves files relative to
      # the repo root, NOT relative to its working directory.
      - LICENSE
      - README.md
      - CHANGELOG.md
      - src: docs/**/*
        dst: docs

# ============================================================================
# Checksum — single SHA256SUMS.txt covering all archives.
# ============================================================================
checksum:
  name_template: "SHA256SUMS.txt"
  algorithm: sha256

# ============================================================================
# Snapshot — name template used when --snapshot flag is set.
# Distinct from release version so the snapshot can't be mistaken for a
# real release in any registry.
# ============================================================================
snapshot:
  version_template: "{{ incpatch .Version }}-snapshot+{{ .ShortCommit }}"

# ============================================================================
# Changelog — github-style commit grouping + filtered to feat/fix only.
# chore/docs/test commits stay in git history but don't pollute the
# public CHANGELOG.md.
# ============================================================================
changelog:
  use: github
  sort: asc
  filters:
    exclude:
      - '^chore(\(.*\))?:'
      - '^docs(\(.*\))?:'
      - '^test(\(.*\))?:'
      - '^ci(\(.*\))?:'

# ============================================================================
# Dockers — multi-arch image built locally; pushed only on the tagged-release
# path (release.yml sets DOCKER_PUSH=true). Snapshot mode NEVER pushes.
# Two images: amd64 + arm64, then a manifest stitches them together below.
# ============================================================================
dockers:
  - id: amd64-image
    ids:
      - relay
    image_templates:
      - "ghcr.io/intake/intake-relay:{{ .Version }}-amd64"
    dockerfile: Dockerfile
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--build-arg=VERSION={{ .Version }}"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"
  - id: arm64-image
    ids:
      - relay
    goarch: arm64
    image_templates:
      - "ghcr.io/intake/intake-relay:{{ .Version }}-arm64"
    dockerfile: Dockerfile
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--build-arg=VERSION={{ .Version }}"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

# Multi-arch manifest — stitches amd64 + arm64 into a single tag.
docker_manifests:
  - name_template: "ghcr.io/intake/intake-relay:{{ .Version }}"
    image_templates:
      - "ghcr.io/intake/intake-relay:{{ .Version }}-amd64"
      - "ghcr.io/intake/intake-relay:{{ .Version }}-arm64"
  - name_template: "ghcr.io/intake/intake-relay:latest"
    image_templates:
      - "ghcr.io/intake/intake-relay:{{ .Version }}-amd64"
      - "ghcr.io/intake/intake-relay:{{ .Version }}-arm64"
    # `latest` updates only on full releases (not pre-releases).
    skip_push: auto

# ============================================================================
# Release — GitHub Release config for the tagged-release path.
# `draft: false` + `prerelease: auto` — goreleaser infers prerelease from
# a SemVer pre-release suffix (e.g. v1.0.0-rc.1 → prerelease=true).
# Phase 7 NEVER pushes a tag, so this block is dormant.
# ============================================================================
release:
  draft: false
  prerelease: auto
  github:
    owner: intake
    name: intake
  header: |
    ## intake-relay {{ .Tag }}

    See the changelog below for what changed.
  footer: |
    ---
    **Container image:** `docker pull ghcr.io/intake/intake-relay:{{ .Tag }}`
    **Source:** [intake/intake@{{ .ShortCommit }}](https://github.com/intake/intake/tree/{{ .FullCommit }})
```

A few subtle points the implementer should note before running `goreleaser check`:
- The `archives.files[].src` glob `docs/**/*` resolves relative to the repo root, not relative to `relay/`. If `goreleaser` is invoked from inside `relay/`, the working directory is `relay/` and `docs/` would resolve to `relay/docs/` (which doesn't exist). To handle this, the workflows in Tasks 5 and 7 invoke goreleaser from the **repo root** with `--config relay/.goreleaser.yaml`. The corresponding `dockerfile: Dockerfile` reference inside `dockers[]` resolves against the build context, which we set to `relay/` in the workflows.
- Alternatively, if simpler: leave goreleaser invoked from `relay/`, change `archives.files` to relative paths like `../LICENSE`, `../README.md`, etc. The first approach (repo-root cwd) is cleaner.
- `formats: ['tar.gz']` is the v2 syntax (v1 used singular `format: tar.gz`). The v2 format-array also accepts `format_overrides` per-platform.
- `version_template` (under `snapshot:`) is the v2 name (v1 used `name_template`). Goreleaser v2.7+ accepts the v2 name; v1 spelling produces a deprecation warning that `goreleaser check` flags.
- `docker_manifests.skip_push: auto` only applies to the `:latest` tag — pre-release tags should not move `:latest`.

- [ ] **Step 2: Verify the config**

Run from the repo root: `goreleaser check --config relay/.goreleaser.yaml`

Expected: exit 0. Any deprecation warning, typo, or unsupported field causes a non-zero exit and is a build-fail.

Common first-time errors:
- "field 'version_template' not found" → you're running goreleaser v1.x; install v2.7.0+ per Task 1.
- "no archive file found: docs/**/*" → the `archives.files[].src` resolves relative to goreleaser's cwd; run from the repo root.
- "image_templates: must reference a docker registry path" → make sure each `image_templates:` line is a full `ghcr.io/...` path (not a bare `intake-relay:...`).

- [ ] **Step 3: Run the snapshot build locally**

Run from the repo root:

```bash
goreleaser release --snapshot --clean --config relay/.goreleaser.yaml
```

Expected: produces `./dist/` containing:
```
dist/
├── artifacts.json
├── config.yaml
├── intake-relay_<snapshot-version>_linux_amd64/
│   └── intake-relay
├── intake-relay_<snapshot-version>_linux_arm64/
│   └── intake-relay
├── intake-relay_<snapshot-version>_darwin_amd64/
│   └── intake-relay
├── intake-relay_<snapshot-version>_darwin_arm64/
│   └── intake-relay
├── intake-relay_<snapshot-version>_windows_amd64/
│   └── intake-relay.exe
├── intake-relay_<snapshot-version>_linux_amd64.tar.gz
├── intake-relay_<snapshot-version>_linux_arm64.tar.gz
├── intake-relay_<snapshot-version>_darwin_amd64.tar.gz
├── intake-relay_<snapshot-version>_darwin_arm64.tar.gz
├── intake-relay_<snapshot-version>_windows_amd64.zip
├── SHA256SUMS.txt
└── CHANGELOG.md
```

Plus local Docker tags `ghcr.io/intake/intake-relay:<snapshot-version>-amd64` and `…-arm64` (only the matching-arch one if Docker buildx isn't multi-arch ready; the manifest step succeeds anyway because `--snapshot` skips push).

If any of the 5 archives is missing, the matrix is wrong — re-read Step 1's `builds[].goos` + `goarch` block + `ignore:` list.

- [ ] **Step 4: Verify each archive's binary reports a version**

Run for each archive (linux/amd64 example):

```bash
mkdir -p /tmp/intake-check && cd /tmp/intake-check
tar xzf <repo-root>/dist/intake-relay_*_linux_amd64.tar.gz
./intake-relay --version
# expected: a string containing the snapshot version (e.g.
#   "intake-relay 0.0.1-snapshot+abc1234")
cd <repo-root>
```

Repeat for `darwin_*` (skip on non-mac hosts — file command should confirm the Mach-O magic), and `windows_amd64.zip` (extract with `unzip`, run via `wine` or inspect with `file`).

If `intake-relay --version` doesn't include the snapshot version, the ldflags `-X intake/internal/version.Version=…` injection isn't taking effect. Verify:
- `intake/internal/version/version.go` (existing) exports a `Version` variable.
- The package path `intake/internal/version` in the ldflags matches the module path in `relay/go.mod` (`module intake`).

This binary-vs-tag identity assertion is §11.1 item 5 in the README — a build-fail otherwise.

- [ ] **Step 5: Verify the archives contain only the allowlisted files**

Run:

```bash
tar -tzf dist/intake-relay_*_linux_amd64.tar.gz | sort
```

Expected output (order may vary slightly):
```
LICENSE
README.md
CHANGELOG.md
docs/...    (every docs/ file present at the repo root)
intake-relay
```

Critically: **no `.env`, no `local-dev/`, no `secrets/`** should appear. If anything beyond the allowlist appears, edit `archives.files` and re-run.

- [ ] **Step 6: Verify SHA256SUMS.txt covers all 5 archives**

Run: `cat dist/SHA256SUMS.txt`

Expected: 5 lines, one per archive. Each line `<hex-digest>  intake-relay_..._<os>_<arch>.<tar.gz|zip>`. The `algorithm: sha256` field in the config produces 64-hex-char digests.

- [ ] **Step 7: Commit**

```bash
git add relay/.goreleaser.yaml
git commit -m "$(cat <<'EOF'
feat(7-ii): goreleaser v2 config — 5 platforms + dockers block + archives allowlist

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 8: Clean up the snapshot dist tree** (optional)

Run: `rm -rf dist/ && docker image rm $(docker images 'ghcr.io/intake/intake-relay' -q) 2>/dev/null || true`

Avoids confusion in later tasks. (`dist/` is gitignored by goreleaser convention but the local Docker tags can confuse later builds.)

---

### Task 4: Verify the Dockerfile path goreleaser uses also passes from the repo root

**Files:** None (verification — possibly minor `.goreleaser.yaml` edit).

The `dockers[].dockerfile: Dockerfile` field is interpreted relative to goreleaser's working directory. We invoke goreleaser from the repo root with `--config relay/.goreleaser.yaml`. The actual Dockerfile is at `relay/Dockerfile`. Two options:

- **Option A**: change `dockers[].dockerfile: Dockerfile` to `dockers[].dockerfile: relay/Dockerfile`.
- **Option B**: tell goreleaser to use `relay/` as the build context with `extra_files:` or `goreleaser`'s `--working-dir` flag.

Option A is simpler. If Step 3 in Task 3 succeeded, the Dockerfile reference is already working — confirm and move on. If it failed with "no such file or directory: Dockerfile", apply Option A.

- [ ] **Step 1: Re-read Task 3 Step 3's output**

If `goreleaser release --snapshot --clean` succeeded with `dockers[].dockerfile: Dockerfile`, no fix is needed; mark this task complete with no edits.

- [ ] **Step 2: If goreleaser failed on the Dockerfile path, apply Option A**

Edit `relay/.goreleaser.yaml`, change both occurrences of:

```yaml
    dockerfile: Dockerfile
```

to:

```yaml
    dockerfile: relay/Dockerfile
```

Re-run: `goreleaser release --snapshot --clean --config relay/.goreleaser.yaml`. Expect success.

- [ ] **Step 3: If a fix was applied, commit**

```bash
git add relay/.goreleaser.yaml
git commit -m "$(cat <<'EOF'
fix(7-ii): goreleaser dockers.dockerfile path — resolve from repo root

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If no fix was applied, no commit for this task.

---

### Task 5: Author `.github/workflows/release.yml`

**Files:** Create `.github/workflows/release.yml`.

This workflow is the public-release trigger. It's authored in Phase 7 but never EXECUTED in Phase 7 — no tag is pushed. Correctness is verified by:
1. `goreleaser check` confirming the YAML the workflow invokes is valid (Task 3).
2. `actionlint` (Task 8, optional) confirming the workflow YAML itself parses cleanly.
3. Manual inspection for secret-reference syntax correctness.

The workflow has two jobs: `release-relay` (goreleaser + ghcr image) and `release-npm` (publish `@intake/core` + `@intake/vue` tarballs). Both jobs gate on the same tag trigger.

- [ ] **Step 1: Verify the `.github/workflows/` directory exists**

Run: `ls .github/workflows/`
Expected: `ci.yml` present. If `.github/workflows/` does not exist, create it: `mkdir -p .github/workflows`.

- [ ] **Step 2: Write `.github/workflows/release.yml`**

Create `.github/workflows/release.yml` with the following exact contents. Substitute `GORELEASER_VERSION` (e.g. `2.7.0`) and `GORELEASER_ACTION_VERSION` (e.g. `v6.1.0`) with the values captured in Task 1 Step 1.

```yaml
# .github/workflows/release.yml
#
# Phase 7-ii: authored but NEVER executed in Phase 7.
# Triggered only on tag push matching v[0-9]+.[0-9]+.[0-9]+ — Phase 7 produces
# no such tag. A future maintainer-driven Phase 7.5 publish action pushes the
# first tag.
#
# Correctness verified via:
#   - goreleaser check (Phase 7-ii Task 3, also wired as a CI job)
#   - actionlint (Phase 7-ii Task 8, optional)
#   - manual inspection of secret-reference syntax

name: release

on:
  push:
    tags:
      - 'v[0-9]+.[0-9]+.[0-9]+'
      - 'v[0-9]+.[0-9]+.[0-9]+-*'  # pre-releases (rc, beta, alpha)

permissions:
  contents: read

concurrency:
  # One release at a time per tag.
  group: release-${{ github.ref }}
  cancel-in-progress: false

jobs:
  # ==========================================================================
  # release-relay — goreleaser produces 5-platform binaries + multi-arch
  # docker image + GitHub Release. Requires write access to contents + packages.
  # ==========================================================================
  release-relay:
    runs-on: ubuntu-latest
    permissions:
      contents: write   # gh release create
      packages: write   # ghcr.io push
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0   # goreleaser needs the full history for the changelog.

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23.2'

      - name: Setup Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Setup QEMU (for multi-arch builds)
        uses: docker/setup-qemu-action@v3
        with:
          platforms: linux/amd64,linux/arm64

      - name: Login to ghcr.io
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Run goreleaser
        uses: goreleaser/goreleaser-action@GORELEASER_ACTION_VERSION
        with:
          distribution: goreleaser
          version: 'GORELEASER_VERSION'
          args: release --clean --config relay/.goreleaser.yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  # ==========================================================================
  # release-npm — publishes @intake/core and @intake/vue to the public npm
  # registry. Requires NPM_TOKEN with publish scope; configured per-repo by
  # the maintainer before the first Phase 7.5+ release.
  # ==========================================================================
  release-npm:
    runs-on: ubuntu-latest
    needs: release-relay   # only publish to npm if the binary release succeeded
    permissions:
      contents: read
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
          registry-url: 'https://registry.npmjs.org'
          cache: npm

      - name: Install workspace dependencies
        run: npm ci

      - name: Build @intake/core
        run: npm run -w @intake/core build || true   # @intake/core is source-only TS today; build is no-op

      - name: Build @intake/vue
        run: npm run -w @intake/vue build

      - name: Publish @intake/core
        run: npm publish -w @intake/core --access public
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}

      - name: Publish @intake/vue
        run: npm publish -w @intake/vue --access public
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

- [ ] **Step 3: Verify the YAML parses**

Run: `cat .github/workflows/release.yml | python -c 'import sys, yaml; yaml.safe_load(sys.stdin)'` (or any YAML parser handy — `yq` etc.).

Expected: no parse error. If parse fails, fix indentation / syntax.

If `actionlint` is available locally (Task 8 makes it optional), run:

```bash
actionlint .github/workflows/release.yml
```

Expected: no findings. Common issues actionlint catches:
- Misspelled action references (`goreleser/goreleaser-action` → typo).
- Unquoted YAML values that look like booleans (`on: push` → fine, but `version: 2.7.0` without quotes is parsed as a float).
- Missing `permissions:` blocks when secrets are referenced.

- [ ] **Step 4: Verify check-pins.sh accepts the new workflow**

Run: `bash scripts/check-pins.sh`
Expected: exit 0. The new gates:
- `goreleaser/goreleaser-action@v6.1.0` matches the pattern (exact patch).
- `version: '2.7.0'` is within 10 lines below the `uses:` line.
- No `@latest`, no `@main`.

If the script fails, double-check that you substituted both `GORELEASER_ACTION_VERSION` (in the `uses:` line) AND `GORELEASER_VERSION` (in the `version:` field).

- [ ] **Step 5: Confirm the workflow trigger is correct**

Manually inspect:
- `on.push.tags` includes `v[0-9]+.[0-9]+.[0-9]+` AND `v[0-9]+.[0-9]+.[0-9]+-*` for pre-releases.
- There is NO `pull_request:` trigger (the release workflow must never run on PRs).
- There is NO `workflow_dispatch:` trigger in v0 (we deliberately make manual runs require a maintainer to amend this file; defense-in-depth against accidental publishes).
- `concurrency.group: release-${{ github.ref }}` + `cancel-in-progress: false` so two simultaneous tag pushes serialize instead of one canceling the other mid-release.

- [ ] **Step 6: Confirm the `release-npm` job's `--access public` flag**

The `@intake/` scope is public-by-default; the `--access public` flag is belt-and-suspenders so the first publish under a fresh `@intake` scope doesn't accidentally land as private.

- [ ] **Step 7: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "$(cat <<'EOF'
feat(7-ii): release.yml — tag-gated goreleaser + npm publish workflow (authored, never executed in Phase 7)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Extend `ci.yml` with `goreleaser-check` job

**Files:** Modify `.github/workflows/ci.yml`.

The first new CI job validates `relay/.goreleaser.yaml` against the pinned goreleaser version on every PR + push to main. Catches config typos before they reach the snapshot build.

- [ ] **Step 1: Read the existing `ci.yml`**

Run: `cat .github/workflows/ci.yml` and confirm the existing `contract` job structure (steps: checkout, setup-node, setup-go, install deps, check-pins, validate-schema, codegen, fail-if-stale, type-check, build+vet).

- [ ] **Step 2: Append the `goreleaser-check` job to `ci.yml`**

After the closing of the `contract` job (end of file, currently line 59), APPEND:

```yaml

  goreleaser-check:
    runs-on: ubuntu-latest
    needs: contract   # only validate goreleaser after schema/codegen sanity passes
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.23.2"

      - name: Run goreleaser check
        uses: goreleaser/goreleaser-action@GORELEASER_ACTION_VERSION
        with:
          distribution: goreleaser
          version: 'GORELEASER_VERSION'
          args: check --config relay/.goreleaser.yaml
```

Substitute `GORELEASER_ACTION_VERSION` (e.g. `v6.1.0`) and `GORELEASER_VERSION` (e.g. `2.7.0`) with the values from Task 1 Step 1.

- [ ] **Step 3: Verify the workflow parses**

Run: `cat .github/workflows/ci.yml | python -c 'import sys, yaml; yaml.safe_load(sys.stdin)'`

Expected: no parse error.

- [ ] **Step 4: Verify check-pins.sh accepts the modified ci.yml**

Run: `bash scripts/check-pins.sh`
Expected: exit 0. Both workflows (`release.yml` + `ci.yml`) now reference goreleaser-action; both pass the exact-pin gate.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "$(cat <<'EOF'
feat(7-ii): ci.yml — goreleaser-check job validates relay/.goreleaser.yaml

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Extend `ci.yml` with `snapshot-build` job

**Files:** Modify `.github/workflows/ci.yml`.

The second new CI job runs the full snapshot pipeline on PRs and uploads `./dist/` as an artifact. Proves the snapshot build works end-to-end without publishing anything. The artifact upload lets a reviewer download and inspect any PR's would-be release artifacts.

- [ ] **Step 1: Append the `snapshot-build` job to `ci.yml`**

After the `goreleaser-check` job (added in Task 6), APPEND:

```yaml

  snapshot-build:
    # Run only on PRs — on main pushes, the snapshot is redundant with
    # goreleaser-check (faster) and the dist/ artifact would pile up.
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    needs: goreleaser-check
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # goreleaser needs the full history for the changelog template

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.23.2"

      - name: Setup Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Setup QEMU
        uses: docker/setup-qemu-action@v3
        with:
          platforms: linux/amd64,linux/arm64

      - name: Run goreleaser snapshot
        uses: goreleaser/goreleaser-action@GORELEASER_ACTION_VERSION
        with:
          distribution: goreleaser
          version: 'GORELEASER_VERSION'
          args: release --snapshot --clean --config relay/.goreleaser.yaml

      - name: Verify all 5 archives present
        run: |
          set -euo pipefail
          missing=0
          for f in \
            dist/intake-relay_*_linux_amd64.tar.gz \
            dist/intake-relay_*_linux_arm64.tar.gz \
            dist/intake-relay_*_darwin_amd64.tar.gz \
            dist/intake-relay_*_darwin_arm64.tar.gz \
            dist/intake-relay_*_windows_amd64.zip; do
            if ! ls $f >/dev/null 2>&1; then
              echo "::error::missing archive matching pattern: $f"
              missing=1
            fi
          done
          if [ -f dist/SHA256SUMS.txt ]; then
            wc -l < dist/SHA256SUMS.txt
          else
            echo "::error::dist/SHA256SUMS.txt missing"
            missing=1
          fi
          exit $missing

      - name: Verify no secrets in archives
        run: |
          set -euo pipefail
          leak=0
          for f in dist/intake-relay_*.tar.gz; do
            if tar -tzf "$f" | grep -qE '(\.env|secrets|local-dev|credentials)'; then
              echo "::error::archive contains secret-like file: $f"
              tar -tzf "$f" | grep -E '(\.env|secrets|local-dev|credentials)'
              leak=1
            fi
          done
          for f in dist/intake-relay_*.zip; do
            if unzip -l "$f" | grep -qE '(\.env|secrets|local-dev|credentials)'; then
              echo "::error::archive contains secret-like file: $f"
              unzip -l "$f" | grep -E '(\.env|secrets|local-dev|credentials)'
              leak=1
            fi
          done
          exit $leak

      - name: Verify binary version string
        run: |
          set -euo pipefail
          tar xzf dist/intake-relay_*_linux_amd64.tar.gz -C /tmp/
          /tmp/intake-relay --version
          # The version string MUST contain the snapshot template's output.
          # We don't assert a specific value (it depends on tag + commit), just
          # that --version exits 0 and prints something.

      - name: Upload dist/ artifact for PR review
        uses: actions/upload-artifact@v4
        with:
          name: relay-snapshot-dist-${{ github.run_id }}
          path: dist/
          retention-days: 7
```

Substitute `GORELEASER_ACTION_VERSION` and `GORELEASER_VERSION` per Task 1 Step 1.

- [ ] **Step 2: Verify the workflow parses**

Run: `cat .github/workflows/ci.yml | python -c 'import sys, yaml; yaml.safe_load(sys.stdin)'`

Expected: no parse error.

- [ ] **Step 3: Verify check-pins.sh still accepts the workflow**

Run: `bash scripts/check-pins.sh`
Expected: exit 0.

- [ ] **Step 4: Dry-run the snapshot-build steps locally**

To simulate the new job (since CI hasn't run yet), run from the repo root:

```bash
goreleaser release --snapshot --clean --config relay/.goreleaser.yaml
# expected: ./dist/ contains all 5 archives + SHA256SUMS.txt (already verified
# in Task 3 Step 3 — redundant but confirms nothing regressed)

# Verify no secret files leaked:
for f in dist/intake-relay_*.tar.gz; do tar -tzf "$f" | grep -E '\.env|secrets|local-dev|credentials' && echo "LEAK in $f"; done
# expected: silence (no LEAK lines)

# Verify the binary --version works:
mkdir -p /tmp/check
tar xzf dist/intake-relay_*_linux_amd64.tar.gz -C /tmp/check
/tmp/check/intake-relay --version
# expected: prints version string including the snapshot suffix
```

If any step fails, the corresponding step in the CI job will also fail. Fix locally, re-run, confirm.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "$(cat <<'EOF'
feat(7-ii): ci.yml — snapshot-build job runs goreleaser release --snapshot on PRs

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8 (OPTIONAL): Add `dockerfile-lint` + `actionlint` jobs

**Files:** Modify `.github/workflows/ci.yml`, modify `scripts/check-pins.sh`.

The Phase 7 design spec leaves both linters optional:
- `hadolint` for the Dockerfile: catches style + best-practice issues (e.g. `apt-get update` without `apt-get install` in the same RUN). For our distroless multi-stage build with minimal RUN lines, hadolint findings will be rare; the dep cost may not be worth it.
- `actionlint` for workflow YAMLs: catches typos in action references, missing secret declarations, shellcheck issues in `run:` blocks. More likely to catch real issues than hadolint here.

**Recommendation:** Adopt `actionlint`. Defer `hadolint` to v1+.

If skipping this task entirely, mark it complete with no commits and move on.

- [ ] **Step 1 (skip-decision): Decide whether to adopt one, both, or neither**

If skipping both: no commits; mark task complete.

If adopting `actionlint` only: proceed to Step 2 + 3 + 4 (skip the hadolint steps).

If adopting both: proceed to all steps.

- [ ] **Step 2: Capture the actionlint Docker image tag**

Run: `curl -sSL https://api.github.com/repos/rhysd/actionlint/releases/latest | grep '"tag_name"' | head -1`

Expected output: something like `"tag_name": "v1.7.7"`. Use the captured tag in place of `ACTIONLINT_VERSION` below (the leading `v` IS part of the docker tag).

- [ ] **Step 3: Append the `actionlint` job to `ci.yml`**

After the `snapshot-build` job, APPEND:

```yaml

  actionlint:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4

      - name: Run actionlint
        run: |
          docker run --rm \
            -v "${{ github.workspace }}:/repo" \
            -w /repo \
            rhysd/actionlint:ACTIONLINT_VERSION \
            -color
```

Substitute `ACTIONLINT_VERSION` (e.g. `1.7.7` — Docker image tags omit the leading `v`).

Add an additional pin gate to `scripts/check-pins.sh` after the goreleaser gates:

```bash
# Gate: actionlint Docker image must be exact-pinned (no :latest, no :master). Phase 7 (optional).
if grep -rE 'rhysd/actionlint:(latest|master|main|edge)' .github/workflows/ 2>/dev/null; then
  echo "ERROR: rhysd/actionlint is :latest/:master in a workflow; PHASE_PLANNING §5 requires exact pins" >&2
  fail=1
fi
```

- [ ] **Step 4 (optional sub-step): Add the `dockerfile-lint` job**

If adopting hadolint, append after the `actionlint` job:

```yaml

  dockerfile-lint:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4

      - name: Run hadolint
        uses: hadolint/hadolint-action@v3.1.0
        with:
          dockerfile: relay/Dockerfile
          failure-threshold: warning
```

(Pin `hadolint/hadolint-action@v3.1.0` exactly; capture the actual latest tag per Task 1 Step 1's pattern. Add a check-pins gate mirroring the goreleaser-action gate.)

- [ ] **Step 5: Verify everything still parses and passes the gates**

```bash
cat .github/workflows/ci.yml | python -c 'import sys, yaml; yaml.safe_load(sys.stdin)'
bash scripts/check-pins.sh
```

Expected: both exit 0.

If `actionlint` is installed locally, run it against the workflow:

```bash
actionlint .github/workflows/ci.yml .github/workflows/release.yml
```

Expected: no findings.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/ci.yml scripts/check-pins.sh
git commit -m "$(cat <<'EOF'
feat(7-ii): ci.yml — actionlint job (optional; pinned by SHA)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If hadolint was also added, a second commit:

```bash
git add .github/workflows/ci.yml scripts/check-pins.sh
git commit -m "$(cat <<'EOF'
feat(7-ii): ci.yml — hadolint job for relay/Dockerfile (optional; pinned)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Cross-task sweep — full smoke + regression

**Files:** None (verification only).

Final verification that the 7-ii deliverable is complete and nothing in 7-i regressed.

- [ ] **Step 1: Re-run the full snapshot pipeline**

From the repo root:

```bash
rm -rf dist/
goreleaser release --snapshot --clean --config relay/.goreleaser.yaml
```

Expected:
- exit 0
- 5 archives present (linux/{amd64,arm64}, darwin/{amd64,arm64}, windows/amd64)
- `dist/SHA256SUMS.txt` present
- local Docker tags `ghcr.io/intake/intake-relay:<snapshot-version>-amd64` (and `-arm64` if buildx is configured)

- [ ] **Step 2: Re-verify the Dockerfile invariants**

```bash
docker build -t intake-relay:smoke relay/
docker inspect intake-relay:smoke --format '{{.Config.User}}'    # → nonroot:nonroot
docker inspect intake-relay:smoke --format '{{.Config.Cmd}}'     # → [--config /etc/intake/config.yaml]
docker inspect intake-relay:smoke --format '{{.Config.Entrypoint}}'  # → [/intake-relay]
docker inspect intake-relay:smoke --format '{{.Config.ExposedPorts}}'  # → {8080/tcp:{}, 9090/tcp:{}}
docker images intake-relay:smoke --format '{{.Size}}'             # → < 50MB
docker run --rm intake-relay:smoke --version                       # → prints version
```

All must match expectations. Any mismatch → fix in the corresponding earlier task.

- [ ] **Step 3: Re-run check-pins.sh**

Run: `bash scripts/check-pins.sh`
Expected: exit 0. All five 7-ii gates active and passing.

- [ ] **Step 4: Run `goreleaser check` standalone**

Run: `goreleaser check --config relay/.goreleaser.yaml`
Expected: exit 0; no deprecation warnings.

- [ ] **Step 5: Parse all workflow YAMLs**

```bash
for wf in .github/workflows/*.yml; do
  echo "=== $wf ==="
  python -c "import sys, yaml; yaml.safe_load(open('$wf'))" && echo OK
done
```

Expected: every file prints `OK`. If `actionlint` is locally available, also run it against all three workflows; expect no findings.

- [ ] **Step 6: Phase 1+4+5+6 regression — full relay test suite**

```bash
cd relay && go build ./... && go vet ./... && go test -race ./... && cd ..
```

Expected: all green. Phase 7-ii touches no relay Go code, so this is a pure sanity check that nothing in the working tree drifted.

- [ ] **Step 7: Schema contract regression**

```bash
bash scripts/verify-contract.sh
```

Expected: exit 0. No schema change in 7-ii.

- [ ] **Step 8: `go mod tidy` is a no-op**

```bash
cd relay && go mod tidy && cd .. && git diff --exit-code relay/go.mod relay/go.sum
```

Expected: clean diff (no change). 7-ii adds zero Go modules.

- [ ] **Step 9: No commit for this task** — verification gate only.

---

## Smoke (mandatory)

**Self-runnable; no LLM credit; no maintainer pause.** All five smoke commands below run locally without registry credentials.

### S1. goreleaser config validates clean

```bash
goreleaser check --config relay/.goreleaser.yaml
```

Expected: exit 0, no deprecation warnings, no unrecognized fields.

### S2. Dockerfile builds, runs as nonroot, < 50 MB

```bash
docker build -t intake-relay:smoke relay/
test "$(docker inspect intake-relay:smoke --format '{{.Config.User}}')" = "nonroot:nonroot" || { echo "FAIL: not nonroot"; exit 1; }
size_bytes=$(docker images intake-relay:smoke --format '{{.Size}}' | tr -d 'MB.kB ' | head -c 5)
# crude check; in practice use `docker inspect intake-relay:smoke --format '{{.Size}}'` for bytes
docker run --rm intake-relay:smoke --version
```

Expected: each command exits 0; the version string prints; the user check passes.

### S3. Full snapshot pipeline produces all 5 archives + image

```bash
rm -rf dist/
goreleaser release --snapshot --clean --config relay/.goreleaser.yaml
ls dist/intake-relay_*_linux_amd64.tar.gz \
   dist/intake-relay_*_linux_arm64.tar.gz \
   dist/intake-relay_*_darwin_amd64.tar.gz \
   dist/intake-relay_*_darwin_arm64.tar.gz \
   dist/intake-relay_*_windows_amd64.zip \
   dist/SHA256SUMS.txt
docker images 'ghcr.io/intake/intake-relay' --format '{{.Repository}}:{{.Tag}}'
```

Expected: `ls` finds all 6 files (5 archives + SHA256SUMS.txt); `docker images` lists at least one `ghcr.io/intake/intake-relay:*` tag.

### S4. Archives contain no secret files

```bash
for f in dist/intake-relay_*.tar.gz; do
  if tar -tzf "$f" | grep -qE '(\.env|secrets|local-dev|credentials)'; then
    echo "LEAK in $f"
    exit 1
  fi
done
for f in dist/intake-relay_*.zip; do
  if unzip -l "$f" | grep -qE '(\.env|secrets|local-dev|credentials)'; then
    echo "LEAK in $f"
    exit 1
  fi
done
echo "OK: no secret-like files in any archive"
```

Expected: prints `OK: …`; exit 0.

### S5. Binary --version matches the snapshot tag

```bash
mkdir -p /tmp/intake-smoke && cd /tmp/intake-smoke
tar xzf <repo-root>/dist/intake-relay_*_linux_amd64.tar.gz
ver=$(./intake-relay --version)
echo "version=$ver"
# Expected: $ver contains "snapshot" (the snapshot template injects the snapshot suffix).
case "$ver" in
  *snapshot*) echo "OK" ;;
  *)          echo "FAIL: version string missing snapshot suffix"; exit 1 ;;
esac
cd <repo-root>
```

Expected: prints `OK`; exit 0.

### S6. check-pins.sh passes with all 5 new gates active

```bash
bash scripts/check-pins.sh
```

Expected: exit 0; final line `OK: all codegen tools are exact-pinned`. The new gates (goreleaser-action, distroless digest, golang-alpine digest, version-near-action, placeholder-rejection) all silently pass when the artifact files are correct.

---

## Done criteria

- [ ] All 9 tasks complete and committed (Task 0 + Task 4 commit-free; Task 8 optional).
- [ ] `relay/Dockerfile` exists, multi-stage, distroless target, both base images SHA-pinned, no placeholder tokens.
- [ ] `relay/.dockerignore` excludes `local-dev/`, `*_test.go`, `.git/`, `dist/`, `node_modules/`, etc.
- [ ] `relay/.goreleaser.yaml` exists, validates clean against `goreleaser check`, produces all 5 archives + SHA256SUMS.txt + docker image via `goreleaser release --snapshot --clean`.
- [ ] `.github/workflows/release.yml` exists, tag-gated to `v[0-9]+.[0-9]+.[0-9]+`, two jobs (relay + npm), exact-pinned action references.
- [ ] `.github/workflows/ci.yml` has new jobs `goreleaser-check` (always) and `snapshot-build` (PRs only). Optionally `actionlint` and `dockerfile-lint`.
- [ ] `scripts/check-pins.sh` extended with 5 new gates: goreleaser-action exact-pin, version-near-action exact-pin, distroless SHA-pin, golang-alpine SHA-pin, placeholder-token rejection.
- [ ] All 6 smoke items (S1-S6) pass locally.
- [ ] `go mod tidy` produces no diff to `relay/go.mod` / `relay/go.sum`.
- [ ] `scripts/verify-contract.sh` green (no schema change in 7-ii).
- [ ] `cd relay && go test -race ./...` green (no relay Go code change).
- [ ] No file outside the 5 listed in "Files Touched" is modified.
- [ ] `relay/internal/adapter/adapter.go` byte-identical to its pre-7-ii state.
- [ ] `relay/internal/payload/types.go` byte-identical (generated; never edited).
- [ ] `schema/payload.v1.json` byte-identical.
- [ ] `.github/workflows/release.yml` has NO `pull_request:` trigger, NO `workflow_dispatch:` trigger, NO `push.branches: [main]` trigger — ONLY `push.tags: ['v[0-9]+.[0-9]+.[0-9]+', 'v[0-9]+.[0-9]+.[0-9]+-*']`.
- [ ] No `docker push` command is invoked by any Phase-7-executable workflow (goreleaser's `--snapshot` flag disables image push; the production push lives only in `release.yml`, which Phase 7 never executes).
- [ ] Branch is still `phase-7`; not pushed.

---

## Notes for the implementer

- **Substitution discipline:** The plan uses six placeholder strings — `DISTROLESS_SHA256_DIGEST_HERE`, `GOLANG_ALPINE_SHA256_DIGEST_HERE`, `GORELEASER_VERSION`, `GORELEASER_ACTION_VERSION`, `ACTIONLINT_VERSION`, `HADOLINT_VERSION`. The check-pins.sh placeholder-rejection gate catches digests; the workflow YAML's exact-pin gates catch versions. Do NOT commit a file with any placeholder string still in it.
- **Working-directory invariant:** All goreleaser invocations run from the **repo root** with `--config relay/.goreleaser.yaml`. Never invoke goreleaser from inside `relay/` — `archives.files` paths would break.
- **Frozen-seam invariant:** This plan touches zero Go source files. If `git diff --stat` after Task 9 shows any change under `relay/internal/` or `relay/cmd/relay/`, something went wrong — revert and investigate.
- **`go.mod` invariant:** This plan adds zero Go modules. `go mod tidy` MUST be a no-op after Task 9. If it isn't, an accidental `import "_"` slipped into one of the workflow's `run:` blocks or similar — investigate.
- **`schema/payload.v1.json` invariant:** This plan changes zero schemas. `scripts/verify-contract.sh` MUST pass.
- **Workflow execution invariant:** `release.yml` MUST NOT execute in Phase 7. Phase 7 does not push any tag. The only way to validate the workflow is `actionlint` + `goreleaser check` + manual inspection — all of which Task 5 and Task 8 cover.
- **Pin freshness:** The placeholder versions (`v6.1.0`, `2.7.0`, `v1.7.7`, etc.) reflect the latest stable releases as of 2026-06-01. If implementation happens significantly later, re-run Task 1 Step 1's capture commands and substitute the current latest.
- **Docker buildx + QEMU on CI:** The `snapshot-build` job in `ci.yml` uses `docker/setup-buildx-action@v3` + `docker/setup-qemu-action@v3` to enable cross-arch builds (linux/arm64 on the linux/amd64 GHA runner). Same for `release.yml`. If buildx isn't set up, goreleaser fails the arm64 docker build with "exec format error".
- **L022 echo:** The plan inherits the "fail loudly with all problems" discipline from Phase 6. `goreleaser check` reports every config issue in one run, not one-at-a-time. `actionlint` does the same for workflow YAMLs.
- **L023 echo:** Every step that runs an external command (`docker build`, `goreleaser check`, `goreleaser release --snapshot`, `actionlint`) inspects the exit code AND the output. A zero exit code with a deprecation warning in stderr is NOT a pass — the build-fail checklist treats deprecation warnings as failures.
- **Phase 7.5 outlook:** When the maintainer eventually flips the public-release switch, they:
  1. Set the `@intake` npm scope to public-by-default + create an NPM_TOKEN secret in the GitHub repo.
  2. Confirm the ghcr.io/intake org exists and `GITHUB_TOKEN` has packages:write.
  3. Push a tag `v0.1.0`.
  4. Watch `release.yml` execute end-to-end.
  Nothing in 7-ii's authored artifacts needs to change for this transition — the snapshot path and the publish path use the same `goreleaser` config.
