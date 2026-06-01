# Phase 7 — Deferred follow-ups (Phase 8 candidates)

The Phase 7 final cross-phase code review (APPROVED FOR MERGE) flagged items
that do NOT block the phase-7 → main merge but should be addressed in a
follow-up phase. Tracked here per the same audit-trail pattern Phase 6 used
(see `ai/tasks/phase-6/FOLLOWUPS-resolved.md`).

## I1 (Important) — drive-docker-compose.ts submit payload doesn't match the canonical SubmitRequest shape

`core/smoke/drive-docker-compose.ts:220-233` sends a sloppy `client` block
(`viewport_width`/`viewport_height`, `language`, `page_title`, `dom_snippet`)
that bears almost no resemblance to the wire `ClientInfo` contract. It passes
today only because `submit.go:41` uses `json.NewDecoder().Decode()` without
`DisallowUnknownFields`, so unknown keys are dropped and missing required
fields silently default to zero values (the schema's minimum is 0 for
viewport, no length-min for locale).

**Consequence:** the smoke driver is the load-bearing cross-phase integration
assertion (boots demo, exercises metrics endpoint, asserts UID, confirms
canonical payload lands). As written, it would NOT catch a wire-contract
regression — if someone renamed `widget_version` → `widgetVersion` on the
wire, this smoke would still pass.

**Suggested fix (Phase 8):** make the smoke driver's submit body match the
canonical SubmitRequest shape verbatim (the same payload the README documents
at `examples/docker-compose/README.md:71-87`). One file change.

**Why deferred:** the per-sub-plan smoke evidence is honest about what was
tested; the assertion still proves "HTTP 200 round-trip + demo stack boots +
metrics work + nonroot UID". The contract-regression-detection gap is real
but not load-bearing for the phase-7 merge.

## I2 (Important) — Supply-chain pin discipline is uneven across new infra

`scripts/check-pins.sh` enforces SHA pins for distroless/golang base images
and version pins for goreleaser-action + golangci-lint-action. But the
following references in the new Phase 7 workflows are NOT SHA-pinned and NOT
gated by check-pins.sh:

- `actions/checkout@v4` (3 sites in release.yml, 6 in ci.yml)
- `actions/setup-go@v5`, `actions/setup-node@v4`
- `docker/setup-buildx-action@v3`, `docker/setup-qemu-action@v3`, `docker/login-action@v3`
- `actions/upload-artifact@v4`
- `node:24-alpine` in `examples/webhook-receiver/Dockerfile:6` (no SHA, no minor tag pin)

**Suggested fix (Phase 8):** pick a policy — either SHA-pin everything in the
published-artifact path (release.yml + ci.yml + relay/Dockerfile{,.goreleaser})
and document the policy at the top of check-pins.sh, OR explicitly document
which actions are exempt and why (typical pragmatic choice: SHA-pin first-party
actions when we have CVE-watch tooling, tag-pin otherwise).

**Why deferred:** consistent with industry practice; the inconsistency surfaces
a policy gap, not a vulnerability. Worth a brief PR comment + check-pins.sh
header before any external announcement.

## I3 (Important) — Demo image vs published image surface area diverge

`relay/Dockerfile` (used by `examples/docker-compose/`'s `build:` directive)
builds BOTH `/intake-relay` AND `/fake-llm` into stage 2 — required for the
demo's fake-llm service to reuse the same image with `entrypoint: []`.

`relay/Dockerfile.goreleaser` (used by `release.yml` for publishing to
`ghcr.io/intake/intake-relay:VERSION`) contains ONLY `/intake-relay`. The
fake-llm binary doesn't ship in the published image.

This is correct — `fake-llm` is a test utility and shouldn't be in a
production image — but it's an undocumented integration constraint. A user
who reads `examples/docker-compose/README.md` and decides to skip `build:` in
favor of `image: ghcr.io/intake/intake-relay:vX.Y.Z` will get a runtime
failure when the fake-llm service tries to exec `/fake-llm`.

**Suggested fix (Phase 8):** add one sentence to
`examples/docker-compose/README.md` (or as a comment in `docker-compose.yml`
near the fake-llm service) noting that fake-llm is only available when
building from `relay/Dockerfile` locally, not from the published image.

**Why deferred:** the demo today uses `build:` (not `image:`), so the gotcha
only fires if a future maintainer edits the compose to use the published
image. Doc-only follow-up.

## M1 (Minor) — `.goreleaser.yaml` hardcodes the `intake/intake` ghcr namespace

`relay/.goreleaser.yaml:118, 134, 148, 152, 167-169` hardcode
`ghcr.io/intake/intake-relay` and `owner: intake, name: intake`.
SECURITY.md + COMMERCIAL.md correctly mark email domains as Q1-pending;
the goreleaser config doesn't carry the same TBD marker.

**Suggested fix:** add a short comment at the top of `.goreleaser.yaml`
("ghcr namespace pending Q1 final-name lock") for symmetry with the
governance docs. Can roll into the Q1 final-name commit.

## M2 (Minor) — Two-Dockerfile drift risk

`relay/Dockerfile` and `relay/Dockerfile.goreleaser` share invariants (same
distroless SHA, same EXPOSE 8080+9090, same USER nonroot:nonroot, same
ENTRYPOINT `/intake-relay`) but have no automated consistency check. If
someone bumps the distroless SHA in one, check-pins.sh would NOT catch the
drift.

**Suggested fix:** add a 3-line shell check to scripts/check-pins.sh that
asserts the `FROM` lines match between the two files.

## M3 (Minor) — Metrics path-label cardinality contains both leaf-route + group-prefix forms

Under sustained 429 attack, the metrics middleware will emit
`path="/v1/intake/init"` AND `path="/v1/intake/*"` (per chi's incremental
routing). Cardinality is still bounded by chi's route table (safe), but
Grafana dashboards built off these series will need to handle both shapes.

**Suggested fix:** add a one-line comment in `relay/internal/metrics/metrics.go`
documenting that operators may see both leaf-route and group-prefix paths
in the same series. Optionally: collapse group-prefix to a fixed bucket
("group") in the middleware.

## C1 (Cross-phase concern, not in I/M classification) — Smoke driver's cleanup chain has no timeout

`core/smoke/drive-docker-compose.ts:320-337` uses try/finally + outer
`composeDown().finally()` for cleanup. Good pattern. But if `composeDown`
itself hangs (rare but possible), the process could orphan containers.

**Suggested fix:** wrap the cleanup invocation in a 30s timeout. Edge case;
not load-bearing.

---

These follow-ups are tracked here rather than in an issue tracker because
the project's working pattern is task-files under `ai/tasks/`. A Phase 8
README can simply reference this file as a backlog source. Once Phase 8
closes any of these items, rename per the Phase 6 → Phase 7-i precedent
(`FOLLOWUPS-resolved.md` with per-item closure summaries).
