# 7-iii Docker-Compose Demo Stack — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the canonical "try intake in 60 seconds" demo stack — a single `docker-compose.yml` that boots a complete working intake instance with one command and zero external credentials. Three services share a private compose network: `relay` (built from the Phase 7-ii `relay/Dockerfile`), `fake-llm` (reusing the relay image with a `command:` override that runs the `/fake-llm` binary), and `webhook-receiver` (the existing `examples/webhook-receiver/server.mjs` containerized via a NEW `examples/webhook-receiver/Dockerfile`). After this sub-plan: a maintainer running `cd examples/docker-compose && docker-compose up -d` from a fresh clone sees three healthy containers, can POST a full `/init → /turn → /submit` flow against `http://localhost:18080`, observes the canonical payload land in `docker-compose logs webhook-receiver`, and confirms the four Prometheus series exposed on `http://localhost:19090/metrics`.

**Architecture:** docker-compose's modern `compose-spec` format (no top-level `version:` field; that field is deprecated). One named bridge network (`intake_demo`) — docker-compose auto-creates it from the `services:` map but we declare it explicitly for predictable name resolution. Service-to-service DNS uses compose's built-in resolver: relay reaches the LLM at `http://fake-llm:11434` and the adapter target at `http://webhook-receiver:9099/intake` — no `localhost`/`127.0.0.1` references between services. Host port mapping uses non-standard offsets (18080, 19090, 19099) to prevent conflicts with developer-local Prometheus / nginx / etc.; the fake-LLM keeps the standard Ollama port `11434` since it impersonates that API and there is no reason to remap it. Relay healthcheck calls its own `/v1/health` endpoint (which Phase 1 implements as a 200 JSON liveness probe); `depends_on` with `condition: service_healthy` orders boot so the relay only starts after fake-llm + webhook-receiver are reachable. The relay container runs the distroless `nonroot` user (UID 65532) from the 7-ii Dockerfile — verified by the Task 8 smoke. The compose stack ALSO drives the Phase 7-v `core/smoke/drive-docker-compose.ts` final smoke; this sub-plan ships the stack itself, 7-v drives it.

**Tech Stack:** docker-compose 2.x (`compose-spec` modern format — bundled with Docker Desktop and the `docker-compose-plugin` debian package). Container images: the Phase 7-ii `intake-relay:local` (distroless, multi-stage Go build), `node:24-alpine` for the webhook-receiver. No new dependencies in any host-side package manifest — the demo stack is pure declarative YAML + a one-file `node:24-alpine` Dockerfile. Zero new Go code; the fake-llm binary already exists at `relay/cmd/fake-llm/main.go` (Phase 5) and is incorporated into the relay image via a Dockerfile change (Task 3 below).

---

## Design References

- Phase 7 design spec §5.7 — demo stack file inventory frozen here (`examples/docker-compose/README.md`, `docker-compose.yml`, `config.yaml`, `.env.example`)
- Phase 7 design spec §7.3 — release artifact generation data flow including the docker-compose boot path: relay on host port 18080, metrics on 19090, fake-llm on 11434, webhook-receiver on 19099
- Phase 7 design spec §8.6 — docker-compose demo failure modes (mount path wrong, network alias mismatch, host port conflict) and their remediations
- Phase 7 README §7 final-smoke item 5 — `cd examples/docker-compose && docker-compose up -d` then `drive-docker-compose.ts` asserts health/init/turn/submit/webhook-log/metrics + `docker exec intake-relay id -u` returns 65532
- Phase 7 README §6 build-fail checklist — `docker-compose config` must succeed; `cfg.Observability.Metrics.Enabled=true` AND `/metrics` returns Prometheus text → REQUIRED; off-by-default invariant — metrics MUST be enabled in this demo to prove the endpoint
- Phase 7 7-ii sub-plan (READ FIRST per Task 0) — the relay `Dockerfile` is authored there. If it only builds `intake-relay`, this sub-plan extends it to also build + copy `fake-llm` into stage 2. See Task 0 + Task 3 below
- Phase 5 fake-llm source: `relay/cmd/fake-llm/main.go` — flags `--addr`, `--input-tokens`, `--output-tokens`; impersonates Ollama's `POST /api/chat` NDJSON streaming
- Phase 1 existing webhook-receiver: `examples/webhook-receiver/server.mjs` + `package.json` — listens on `:9099`, logs every `POST /intake` body as formatted JSON
- Phase 5/6 smoke configs as YAML templates: `relay/cmd/relay/smoke/clean.yaml` (minimal anonymous + webhook adapter), `relay/cmd/relay/smoke/attachments-enabled.yaml` (full attachments + ollama + ratelimit shape)
- Phase 6 6-iii — exemplar for sub-plan cadence (5-step TDD per task, exact code blocks, exact diffs, smoke + done criteria)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `examples/webhook-receiver/Dockerfile` | Create | `node:24-alpine` image; copies `server.mjs` + `package.json`; runs `node server.mjs` on `:9099` |
| `examples/docker-compose/docker-compose.yml` | Create | compose-spec; 3 services + 1 network; healthchecks; service-DNS wiring |
| `examples/docker-compose/config.yaml` | Create | Relay config: anonymous + attachments + ollama (→ fake-llm) + webhook adapter + metrics enabled |
| `examples/docker-compose/.env.example` | Create | `ANTHROPIC_API_KEY=dummy` (relay startup requires the env var; demo defaults to fake-llm) |
| `examples/docker-compose/README.md` | Create | One-paragraph "what this is" + 60-second run instructions + curl snippets + teardown |
| `relay/Dockerfile` | Modify (conditional on Task 0 outcome) | Add a second `go build` step for `fake-llm` + a second `COPY --from=builder` in stage 2 — ONLY if 7-ii's Dockerfile does not already produce the binary |

The Dockerfile modification (Task 3) is the ONE allowed exception to the "do not modify production code outside `examples/`" constraint. Every other file lives under `examples/`.

---

## Tasks

### Task 0: Read the 7-ii sub-plan; decide whether to extend `relay/Dockerfile`

**Files:** Read-only — `ai/tasks/phase-7/7-ii-release-artifacts-plan.md` and (after 7-ii lands) `relay/Dockerfile`

This task is a **planning gate**, not an implementation step. Its output decides whether Task 3 below is a no-op or an actual edit.

- [ ] **Step 1: Read `ai/tasks/phase-7/7-ii-release-artifacts-plan.md` end-to-end**

  Look specifically for: (a) the stage-1 `go build` invocation(s); (b) every `COPY --from=builder` line in stage 2; (c) the `ENTRYPOINT` / `CMD`. Confirm the binary set produced by the image.

- [ ] **Step 2: Read the actual `relay/Dockerfile` from 7-ii**

  After 7-ii has landed (this sub-plan depends on 7-ii per the Phase 7 dependency graph), read `relay/Dockerfile` directly and confirm what stage 2 contains.

- [ ] **Step 3: Verdict**

  - If 7-ii's Dockerfile **already builds `fake-llm`** and stage 2 already contains a `COPY --from=builder /build/fake-llm /fake-llm` line → Task 3 is a no-op; mark it complete with a note "no change required; 7-ii Dockerfile already ships fake-llm". Skip Task 3 entirely.
  - If 7-ii's Dockerfile **only builds `intake-relay`** → proceed with Task 3 (extend the Dockerfile). This is the **suggested option** per the sub-plan spec and is the cleanest path: one image, two binaries, no separate fake-llm image, no `go run` from source in compose.

- [ ] **Step 4: Document the verdict at the top of Task 3 below**

  Add a one-line note to Task 3 ("Verdict from Task 0: extend / no-op") so the implementer of Task 3 has unambiguous direction.

No commit for Task 0 — it is decision-making only.

---

### Task 1: Create `examples/webhook-receiver/Dockerfile`

**Files:** Create `examples/webhook-receiver/Dockerfile`

The existing `examples/webhook-receiver/server.mjs` (Phase 1) is a 35-line Node script with zero npm dependencies — it only imports `node:http` from the stdlib. The Dockerfile is therefore trivial: `node:24-alpine` base, copy two files, expose `9099`, run.

- [ ] **Step 1: Create `examples/webhook-receiver/Dockerfile`**

  Full file contents:

```dockerfile
# Phase 7-iii — webhook-receiver image for the docker-compose demo.
# Reuses the existing examples/webhook-receiver/server.mjs (Phase 1) which has
# zero npm dependencies (only node:http stdlib). Small alpine base + non-root
# user keeps the container surface minimal.

FROM node:24-alpine

# node:24-alpine ships a `node` user (UID 1000) by default. Use it.
WORKDIR /app

COPY package.json server.mjs ./

EXPOSE 9099

USER node

CMD ["node", "/app/server.mjs"]
```

  Notes:
  - `node:24-alpine` is the modern Node.js LTS line on alpine; small (~80 MB) and CVE-light vs. `node:24`.
  - The existing `package.json` declares `"type": "module"` and a `"start": "node server.mjs"` script; we use the explicit `CMD ["node", "/app/server.mjs"]` form rather than `npm start` to avoid the npm CLI wrapping the process (signals/exit codes pass through `node` directly to PID 1).
  - `USER node` runs the receiver as a non-root user — mirroring the relay's distroless nonroot invariant. Confirmed by Task 8 smoke (`docker exec intake-webhook-receiver id -u` → `1000`).
  - No `npm install` — the script has zero dependencies. Skipping that step keeps the image lean and removes a class of supply-chain risk for the demo.

- [ ] **Step 2: Verify the Dockerfile parses + builds locally (informational)**

  ```
  docker build -t intake-webhook-receiver:local examples/webhook-receiver/
  ```

  Expected: exits 0; image size < 100 MB; `docker inspect` shows `Config.User = "node"`.

  (This is informational; the compose smoke in Task 8 re-runs the build via `docker-compose build`.)

- [ ] **Step 3: Commit**

  ```
  git add examples/webhook-receiver/Dockerfile
  git commit -m "$(cat <<'EOF'
  feat(7-iii): examples/webhook-receiver/Dockerfile — node:24-alpine + non-root + zero deps

  Containerizes the Phase 1 webhook-receiver script for the docker-compose demo.
  No npm install needed (server.mjs uses only node:http stdlib). Runs as the
  built-in `node` user (UID 1000) on EXPOSE 9099.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

### Task 2: Verify `examples/webhook-receiver/package.json` works in the container

**Files:** No modification expected; verify only

The existing `package.json` is already minimal and correct (`"type": "module"`, `"start": "node server.mjs"`, `"private": true`, no dependencies). The Task 1 Dockerfile bypasses `npm start` and invokes `node /app/server.mjs` directly, so `package.json` is only consulted for the `"type": "module"` flag (which makes `.mjs` superfluous but harmless).

- [ ] **Step 1: Read `examples/webhook-receiver/package.json`**

  Confirm it contains:
  - `"type": "module"` — required so `node:24` treats `.mjs` and any future `.js` as ESM
  - `"private": true` — required so future `npm publish` accidents are rejected (defense in depth)
  - No `"dependencies"` block — required so the Dockerfile can skip `npm install`

  If any of the above is missing, file a one-line edit. The current file (verified during plan authoring) has all three; this step is typically a no-op.

- [ ] **Step 2: No commit**

  Nothing changes. This task is verification only.

---

### Task 3: (Conditional) Extend `relay/Dockerfile` to also build + ship `fake-llm`

**Files:** Modify `relay/Dockerfile`

**Verdict from Task 0:** _[fill in: "extend" or "no-op"]_

If the verdict is "no-op", skip this task. If the verdict is "extend", proceed.

The 7-iii spec recommends the cleanest option: **add `fake-llm` to the existing relay image** rather than building a separate image or running from source via `golang:1.23.2-alpine`. This keeps the demo stack to a single Go image and means the `fake-llm` service in `docker-compose.yml` is a trivial `command:` override on the same image — no second build context, no second go-build cache, no `golang.org/x/...` toolchain pulled at compose-up time.

- [ ] **Step 1: Read the existing `relay/Dockerfile`**

  Locate (a) the stage-1 `go build` line(s) and (b) the stage-2 `COPY --from=builder` line(s).

- [ ] **Step 2: Apply the extension hunk**

  Assuming the 7-ii Dockerfile builds `intake-relay` from `./cmd/relay`, the extension is:

```diff
@@ stage 1 — builder ──────────────────────────────────────────────
 FROM golang:1.23.2-alpine AS builder
 WORKDIR /build
 COPY go.mod go.sum ./
 RUN go mod download
 COPY . .
 RUN CGO_ENABLED=0 GOOS=linux go build \
       -ldflags '-s -w' -trimpath \
       -o /build/intake-relay ./cmd/relay
+RUN CGO_ENABLED=0 GOOS=linux go build \
+      -ldflags '-s -w' -trimpath \
+      -o /build/fake-llm ./cmd/fake-llm
@@ stage 2 — distroless runtime ──────────────────────────────────
 FROM gcr.io/distroless/static-debian12:nonroot
 COPY --from=builder /build/intake-relay /intake-relay
+COPY --from=builder /build/fake-llm     /fake-llm
 USER nonroot:nonroot
 EXPOSE 8080 9090
 ENTRYPOINT ["/intake-relay"]
```

  Notes:
  - The exact lines in the existing Dockerfile may differ; the hunk shows the **shape** of the change. The implementer applies it to the file as 7-ii actually wrote it. Preserve any flags 7-ii used (e.g., if 7-ii pinned a `-buildvcs=false` flag or a different `-ldflags` string, mirror them in the new `fake-llm` build line).
  - `fake-llm` does NOT get its own `EXPOSE` line because the relay image is the same image — the compose `command:` override redirects to fake-llm and the compose `expose:` / `ports:` block governs port publishing per service.
  - `ENTRYPOINT ["/intake-relay"]` stays. Compose overrides it for the `fake-llm` service via `command: ["/fake-llm", ...]` — but compose's `command:` only overrides `CMD`, not `ENTRYPOINT`. To make the override work, the compose service entry uses BOTH `entrypoint: []` (cleared) and `command:` — see Task 4. Alternative: change the Dockerfile's `ENTRYPOINT` to `CMD ["/intake-relay"]` so `command:` overrides cleanly. **Recommended:** keep `ENTRYPOINT` in the Dockerfile (production semantics — image should run the relay by default) and use `entrypoint: []` in the compose `fake-llm` service.

- [ ] **Step 3: Local build verification**

  ```
  docker build -t intake-relay:local relay/
  docker run --rm intake-relay:local --version  # relay --version
  docker run --rm --entrypoint /fake-llm intake-relay:local --help  # fake-llm flag list
  ```

  Expected: both commands print expected output. Image size < 50 MB (build-fail invariant from Phase 7 README §6).

- [ ] **Step 4: Verify Phase 1-6 regression is not broken**

  ```
  cd relay && go test -race ./... && cd ..
  ```

  Expected: green. The Dockerfile change is build-system-only; no Go source code moved.

- [ ] **Step 5: Commit**

  ```
  git add relay/Dockerfile
  git commit -m "$(cat <<'EOF'
  feat(7-iii): relay/Dockerfile — also build + copy fake-llm into stage 2

  The docker-compose demo (examples/docker-compose/) reuses the relay image
  for the fake-llm service with a `command: [/fake-llm, ...]` override.
  Adding the binary to the same multi-stage image keeps the demo to a single
  Go build context — no separate fake-llm image, no `go run` from source.

  ENTRYPOINT remains /intake-relay (production semantics); compose clears it
  via `entrypoint: []` on the fake-llm service.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

### Task 4: Create `examples/docker-compose/docker-compose.yml`

**Files:** Create `examples/docker-compose/docker-compose.yml`

The compose file uses the modern `compose-spec` format — **no `version:` field at the top** (deprecated in compose v2.x; warns on every up). One bridge network (`intake_demo`) made explicit so the network name is predictable across `docker-compose down -v` cycles.

- [ ] **Step 1: Create `examples/docker-compose/docker-compose.yml`**

  Full file contents:

```yaml
# Phase 7-iii — Intake demo stack.
#
# Three services + one bridge network. Boots a complete working intake instance
# with one command and zero external credentials:
#
#   cd examples/docker-compose
#   docker-compose up -d
#
# See README.md for the full walkthrough and verification curl snippets.
#
# Service-to-service DNS uses docker-compose's built-in resolver:
#   relay → fake-llm           via http://fake-llm:11434
#   relay → webhook-receiver   via http://webhook-receiver:9099
# Host port mapping uses non-standard offsets (18080, 19090, 19099) to avoid
# conflicts with developer-local processes; 11434 is the standard Ollama port
# and is kept as-is so the fake-llm impersonates it transparently.

name: intake-demo

networks:
  intake_demo:
    driver: bridge

services:
  fake-llm:
    image: intake-relay:local
    build:
      context: ../../relay
    container_name: intake-fake-llm
    # The relay image's default ENTRYPOINT is /intake-relay; clear it so the
    # `command:` below runs /fake-llm instead. See Task 3 in the 7-iii plan.
    entrypoint: []
    command:
      - /fake-llm
      - --addr
      - ":11434"
      - --input-tokens
      - "50"
      - --output-tokens
      - "25"
    networks:
      - intake_demo
    ports:
      - "11434:11434"
    healthcheck:
      # fake-llm has no /health endpoint; an `nc -z` style check is unavailable
      # in distroless. Use a TCP-reachability check via the Go binary itself:
      # the binary listens immediately, so any successful TCP connect signals
      # ready. Compose can use a `tcp://` test in modern versions, but to keep
      # the file portable we just rely on the relay's depends_on retry loop:
      # the relay healthcheck retries until /v1/health is 200, which only
      # happens after the LLM is reachable for any startup probe. Hence
      # fake-llm declares only a static `service_started` condition — see
      # the relay's depends_on block below.
      disable: true

  webhook-receiver:
    image: intake-webhook-receiver:local
    build:
      context: ../webhook-receiver
    container_name: intake-webhook-receiver
    networks:
      - intake_demo
    ports:
      - "19099:9099"
    healthcheck:
      # The receiver exposes no /health endpoint; a 404 on / signals "process
      # is up and serving". Use wget --spider with an accepted-status hack:
      # node:24-alpine ships busybox wget which exits 0 on any 2xx/3xx and
      # exits 1 on 4xx/5xx. We POST a minimal probe to /intake which returns
      # 200, so this works cleanly.
      test: ["CMD-SHELL", "wget -q --post-data='{}' -O - http://127.0.0.1:9099/intake || exit 1"]
      interval: 5s
      timeout: 3s
      retries: 5
      start_period: 5s

  relay:
    image: intake-relay:local
    build:
      context: ../../relay
    container_name: intake-relay
    depends_on:
      fake-llm:
        condition: service_started
      webhook-receiver:
        condition: service_healthy
    environment:
      # Relay startup gate requires ANTHROPIC_API_KEY to be SET (not necessarily
      # valid) when the anthropic adapter is in the registry. Default to "dummy"
      # so the demo runs without any real credentials. The demo uses fake-llm
      # for LLM calls, so the value is never sent over the wire.
      ANTHROPIC_API_KEY: "${ANTHROPIC_API_KEY:-dummy}"
    volumes:
      - ./config.yaml:/etc/intake/config.yaml:ro
    command:
      - --config
      - /etc/intake/config.yaml
    networks:
      - intake_demo
    ports:
      - "18080:8080"
      - "19090:9090"
    healthcheck:
      # Distroless has no shell + no wget. The relay image ships the
      # intake-relay binary which has a built-in /v1/health endpoint we curl
      # from a tiny one-shot exec. Distroless also has no curl. We use the
      # one tool guaranteed to exist: the relay binary itself with a hidden
      # `--health-check` flag would be ideal but Phase 7-i does NOT add one.
      #
      # Instead: rely on docker-compose's start-period + the receiver gating
      # via service_healthy; the relay's HTTP listener binds < 1s after
      # process start. Declare no healthcheck (disable: true) so dependent
      # smokes use `until curl ... /v1/health` from the host (port 18080) —
      # this is what drive-docker-compose.ts (Phase 7-v) does.
      disable: true
```

  Notes:
  - `name: intake-demo` sets the compose project name, which prefixes container/network/volume names. Without it, the project name derives from the parent directory (`docker-compose`), which is ambiguous.
  - **Healthcheck honesty.** The compose file's healthchecks for `fake-llm` and `relay` are `disable: true` because the distroless base image has neither a shell nor a probe tool. The Phase 7-v smoke driver (`drive-docker-compose.ts`) polls `http://localhost:18080/v1/health` from the host instead, which is the load-bearing reachability proof. Documenting `disable: true` explicitly (vs. omitting `healthcheck:`) is intentional so future readers see the design choice. If Phase 7-i adds a `intake-relay healthcheck` subcommand, this block becomes a real `CMD ["/intake-relay", "healthcheck"]` test.
  - **Webhook-receiver healthcheck.** Uses busybox `wget` (shipped in `node:24-alpine`) to POST `{}` to `/intake`. The receiver returns 200 to any POST regardless of body shape, so this is a reliable readiness probe.
  - **`depends_on`.** Relay waits for `webhook-receiver: service_healthy` (real readiness) and `fake-llm: service_started` (port-bind only — adequate because the relay only talks to fake-llm during `/turn`, not at startup). This keeps the boot ordering deterministic without inventing healthchecks for the distroless containers.
  - **Volumes.** `./config.yaml` is mounted **read-only** (`:ro`) to prevent the relay container from accidentally mutating it.
  - **Ports.** Host-side ports are offset by 10000 (18080, 19090, 19099) to dodge developer-local collisions per the spec. Port 11434 is the Ollama-standard port and stays as-is.

- [ ] **Step 2: Validate the compose file**

  ```
  cd examples/docker-compose && docker-compose config && cd ../..
  ```

  Expected: exits 0; prints the rendered config to stdout with no warnings.

  Specifically, no `WARN: the "version" field is deprecated` line — we omit it on purpose.

- [ ] **Step 3: Commit**

  ```
  git add examples/docker-compose/docker-compose.yml
  git commit -m "$(cat <<'EOF'
  feat(7-iii): examples/docker-compose/docker-compose.yml — 3-service demo stack

  compose-spec modern format (no `version:` field). One bridge network
  intake_demo; service DNS for relay→fake-llm and relay→webhook-receiver.
  Host ports 18080 (relay HTTP), 19090 (metrics), 19099 (receiver), 11434
  (fake-llm, Ollama-standard). fake-llm reuses the relay image with
  `entrypoint: []` + `command: [/fake-llm, ...]`.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

### Task 5: Create `examples/docker-compose/config.yaml`

**Files:** Create `examples/docker-compose/config.yaml`

Relay config tuned for the demo: anonymous auth without captcha (so curl works without a Turnstile token), attachments enabled (so the demo proves the full Phase 6 path), metrics enabled (so the `/metrics` endpoint is reachable and the smoke can grep the 4 series), `ollama` provider pointing at the in-network `fake-llm:11434`, webhook adapter pointing at the in-network `webhook-receiver:9099/intake`.

- [ ] **Step 1: Create `examples/docker-compose/config.yaml`**

  Full file contents:

```yaml
# Phase 7-iii — Intake relay config for the docker-compose demo.
#
# Mounted into the relay container at /etc/intake/config.yaml (read-only).
# Drives a complete working intake instance with zero external credentials:
#
#   - Anonymous auth, captcha NOT required → curl-friendly
#   - Attachments enabled → the demo proves the full Phase 6 path
#   - LLM = fake-llm (in-network) → no LLM credits consumed
#   - Adapter = webhook → POSTs to the in-network receiver, which logs the payload
#   - Metrics enabled → /metrics on :9090 (host port 19090) exposes the 4 series

server:
  addr: ":8080"
  # external_url MUST be the URL clients reach the relay at. For the demo,
  # that is the host-side mapped port (http://localhost:18080), not the
  # in-container :8080 address.
  external_url: "http://localhost:18080"
  cors_origins:
    - "http://localhost:5173"
    - "http://localhost:18080"

llm:
  provider: "ollama"
  ollama:
    # docker-compose DNS resolves `fake-llm` to the in-network IP.
    base_url: "http://fake-llm:11434"
    model: "fake"
    max_tokens: 50

auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true

ratelimit:
  per_ip:
    requests_per_second: 100.0
    burst: 200
    idle_ttl: "15m"
  per_session:
    max_turns: 100
    max_input_tokens: 1000000
    session_ttl: "1h"
  daily_llm_budget:
    max_input_tokens: 1000000
    max_output_tokens: 1000000
    action_on_exceeded: "reject"

attachments:
  enabled: true
  max_size_bytes: 5242880    # 5 MiB per attachment
  max_total_bytes: 10485760  # 10 MiB aggregate
  allowed_mime_types:
    - "image/png"
    - "image/jpeg"
    - "image/webp"
  storage:
    mode: "forward"

adapters:
  webhook:
    enabled: true
    # docker-compose DNS resolves `webhook-receiver` to the in-network IP.
    url: "http://webhook-receiver:9099/intake"

routing:
  default_adapter: "webhook"

observability:
  log_level: "info"
  log_format: "json"
  metrics:
    # Phase 7-i invariant: off-by-default. The demo explicitly enables it so
    # the smoke proves the /metrics endpoint and the four series. Operators
    # deploying for real should leave this off until they wire a scrape target.
    enabled: true
    addr: ":9090"
```

  Notes:
  - `server.addr: ":8080"` matches the Dockerfile's `EXPOSE 8080` and the compose port-mapping `"18080:8080"`.
  - `server.external_url: "http://localhost:18080"` is the **host-side** URL clients reach the relay at — important for any code path that echoes the external URL back (e.g., absolute redirect URLs in auth flows).
  - `llm.ollama.base_url: "http://fake-llm:11434"` is the in-network DNS name. NOT `localhost` — the relay container would resolve `localhost` to itself, not the LLM container.
  - `adapters.webhook.url: "http://webhook-receiver:9099/intake"` is also in-network DNS, same reasoning.
  - `observability.metrics.enabled: true` is the **demo-only** opt-in. Phase 7-i ships metrics off-by-default; this config explicitly turns them on so the smoke + curl-from-host verifications work. README.md (Task 7) documents that operators deploying for real should leave it off until a scrape target is configured.
  - `attachments.storage.mode: "forward"` matches Phase 6's only supported v0 mode (`s3` is rejected by the Q9 startup gate).
  - `ratelimit.daily_llm_budget.action_on_exceeded: "reject"` matches Phase 5's only supported v0 value (`queue` is rejected by the Q9 startup gate).

- [ ] **Step 2: Validate against the relay's config loader (informational)**

  After the container builds, the relay's startup gate will reject any misconfig in this file with a single consolidated log line. Run a one-off smoke:

  ```
  cd examples/docker-compose && docker-compose up -d relay && docker-compose logs relay && cd ../..
  ```

  Expected: relay starts, logs `level=INFO msg="server: listening"` on `:8080`. If any field is wrong, the Phase 7-i consolidated startup gate emits one log line with the problem list — fix and retry.

  (Final verification happens in Task 8 with all 3 services up.)

- [ ] **Step 3: Commit**

  ```
  git add examples/docker-compose/config.yaml
  git commit -m "$(cat <<'EOF'
  feat(7-iii): examples/docker-compose/config.yaml — demo relay config

  Anonymous + no captcha, attachments enabled, metrics enabled (explicit
  opt-in for the demo), ollama provider → fake-llm:11434, webhook adapter →
  webhook-receiver:9099/intake. Uses in-network DNS names exclusively (no
  localhost/127.0.0.1 between services).

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

### Task 6: Create `examples/docker-compose/.env.example`

**Files:** Create `examples/docker-compose/.env.example`

The relay's anthropic adapter requires `ANTHROPIC_API_KEY` to be set in the environment at startup if the adapter is in the registry. The demo uses fake-llm + webhook adapter so the key is never sent over the wire — but the env var must still be SET (any non-empty value) for the relay to boot.

- [ ] **Step 1: Create `examples/docker-compose/.env.example`**

  Full file contents:

```
# Phase 7-iii — environment overrides for the docker-compose demo.
#
# Copy this file to .env (it will be picked up automatically by docker-compose
# when you run `docker-compose up`):
#
#   cp .env.example .env
#
# The demo works out of the box with no further changes — the value below is
# a placeholder. The demo's LLM calls go to the fake-llm service (no credit
# consumed) and the adapter target is the local webhook-receiver, so no real
# secrets are ever sent over the wire.
#
# To switch the demo to a real Anthropic-backed flow:
#   1. Replace the value below with a real key (sk-ant-...).
#   2. Edit config.yaml: change llm.provider from "ollama" to "anthropic"
#      and add an `llm.anthropic` block with model + max_tokens.
#   3. docker-compose up -d --force-recreate relay
#
# .env is in the project's gitignore — never commit a real key.

ANTHROPIC_API_KEY=dummy
```

  Notes:
  - The repo's root `.gitignore` already lists `.env` + `.env.*` with a `!.env.example` allowlist, so this file IS tracked but a developer-created `.env` IS NOT. Confirmed by Task 9.
  - The value `dummy` is intentional: any non-empty string passes the relay's "env var must be set" startup gate. The relay never USES the value during the demo flow (because the config uses `provider: ollama`, not `provider: anthropic`).
  - The README (Task 7) repeats the `cp .env.example .env` instruction so users encountering the project for the first time have a single command.

- [ ] **Step 2: Confirm gitignore treatment**

  ```
  git status --short examples/docker-compose/.env.example
  ```

  Expected: `?? examples/docker-compose/.env.example` (untracked, ready to add). NOT `!! .env.example` (ignored).

  Additional sanity check — temporarily create a `.env` file in the same directory:

  ```
  echo "test" > examples/docker-compose/.env
  git status --short examples/docker-compose/
  rm examples/docker-compose/.env
  ```

  Expected: `git status` shows `examples/docker-compose/.env.example` as untracked, but the `examples/docker-compose/.env` file is hidden by `.gitignore`. This proves the allowlist is wired right.

- [ ] **Step 3: Commit**

  ```
  git add examples/docker-compose/.env.example
  git commit -m "$(cat <<'EOF'
  feat(7-iii): examples/docker-compose/.env.example — ANTHROPIC_API_KEY default

  Defaults to `dummy` so the demo runs without any real credentials (the demo
  uses fake-llm for LLM calls). README walks operators through the
  cp .env.example .env step and how to swap in a real key.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

### Task 7: Create `examples/docker-compose/README.md`

**Files:** Create `examples/docker-compose/README.md`

One-paragraph overview + a minimal walkthrough. The README is meant to be read in ~3 minutes and run in ~60 seconds.

- [ ] **Step 1: Create `examples/docker-compose/README.md`**

  Full file contents:

```markdown
# Intake — Docker-Compose Demo

A complete working intake instance you can boot in 60 seconds with one
command. Three services share a private docker-compose network: the **relay**
(intake-relay binary, distroless), a **fake-llm** that impersonates the
Ollama API (no LLM credit consumed), and a **webhook-receiver** that logs
every submitted ticket to its stdout. Use this stack to evaluate intake,
exercise the `/init → /turn → /submit` flow against an adapter, or as a
template for your own self-hosted deployment.

## Prerequisites

- Docker Desktop (Windows/macOS) **OR** the Linux Docker engine
  (docker-engine + docker-compose-plugin)
- Free host ports: **18080** (relay HTTP), **19090** (Prometheus metrics),
  **19099** (webhook receiver), **11434** (fake-llm, Ollama-standard port).
  All four are non-standard offsets to avoid conflicts with developer-local
  processes; if any of them collide with your machine, edit
  `docker-compose.yml` and remap.

## Quickstart

```bash
cd examples/docker-compose

# Optional: copy .env.example to .env (the demo defaults to a fake LLM and
# does NOT require a real ANTHROPIC_API_KEY; the placeholder value is enough).
cp .env.example .env

# Build and start all three services in the background.
docker-compose up -d

# Verify all three containers are running.
docker-compose ps
```

You should see three rows: `intake-relay`, `intake-fake-llm`, and
`intake-webhook-receiver`, all with `State: running`. The relay takes ~1
second to start after the receiver is healthy.

## Submit a ticket

The full intake flow is three HTTP calls: `/init → /turn → /submit`. Open
the receiver's log in one terminal:

```bash
docker-compose logs -f webhook-receiver
```

Then in another terminal:

```bash
# 1. /init — obtain a session_id + capabilities snapshot.
INIT=$(curl -s -X POST http://localhost:18080/v1/intake/init \
  -H "Content-Type: application/json" \
  -d '{}')
echo "$INIT"
SESSION=$(echo "$INIT" | jq -r .session_id)

# 2. /turn — stream one assistant turn (SSE). The fake-llm returns a single
#    "ok" content chunk and a done frame with token counts.
curl -N -X POST "http://localhost:18080/v1/intake/turn" \
  -H "Content-Type: application/json" \
  -H "X-Intake-Session: ${SESSION}" \
  -d '{"messages":[{"role":"user","content":"Hello, intake!"}]}'

# 3. /submit — POST the final message list. The webhook adapter forwards
#    the canonical payload to webhook-receiver:9099/intake.
curl -s -X POST "http://localhost:18080/v1/intake/submit" \
  -H "Content-Type: application/json" \
  -H "X-Intake-Session: ${SESSION}" \
  -d '{
    "messages": [
      {"role": "user", "content": "Hello, intake!"},
      {"role": "assistant", "content": "ok"}
    ],
    "client": {"widget_version": "demo"},
    "user_claims": {},
    "context": {"app_context": {}, "page_metadata": {}},
    "routing_hint": null
  }'
```

Switch back to the receiver's log terminal — you should see the canonical
payload printed as formatted JSON, including the `messages`, `client`,
`user_claims`, and `context` blocks.

## Verify the Prometheus metrics endpoint

The demo enables the off-by-default metrics endpoint so you can see the four
v0 series:

```bash
curl http://localhost:19090/metrics | head -40
```

Look for `# HELP intake_http_requests_total`, `# HELP
intake_http_request_duration_seconds`, `# HELP intake_llm_tokens_total`,
`# HELP intake_adapter_calls_total`. After driving the curl flow above,
the request counter for `/v1/intake/init`, `/v1/intake/turn`, and
`/v1/intake/submit` will each have incremented by one.

## Teardown

```bash
docker-compose down -v
```

The `-v` flag also removes the named volumes (there are none in this demo,
but it's good hygiene). The network and containers are deleted in one step;
re-running `docker-compose up -d` rebuilds from scratch.

## What's next

- See the [self-hosting guide](../../docs/self-hosting.md) for production
  deployment patterns (reverse proxy, real LLM credentials, persistent
  attachment storage).
- See the [adapters overview](../../docs/adapters.md) for the matrix of
  built-in adapters (chatwoot, fider, linear, zendesk, webhook).
- The demo's metrics endpoint is opt-in **for the demo only**. Production
  deployments should keep `observability.metrics.enabled: false` until a
  Prometheus scrape target is wired up — there is no built-in authentication
  on `/metrics`.
```

  Notes:
  - The quickstart uses `jq` for one line — most developers have it; if not, the SESSION extraction can be done with `sed`/`awk`. We leave `jq` because the README is "evaluating intake," and the audience for that step is also the audience that has `jq`.
  - The `curl -N` flag on `/turn` disables curl's buffering so the SSE stream is visible. Without `-N`, curl waits for the full body before printing, which makes the demo confusing.
  - The "What's next" section links to docs that **7-iv writes**. If 7-iv has not landed when this README is authored, leave the links — they will resolve once 7-iv merges. The Phase 7-v smoke checks for broken links.
  - The note about metrics being demo-only is load-bearing — Phase 7's off-by-default invariant must not be undermined by a "just keep enabled: true in production" misreading.

- [ ] **Step 2: Verify the README renders cleanly**

  Inspect the file in a Markdown viewer (GitHub preview, `glow`, VS Code preview). Expected: no broken code-fence delimiters, no missed list-marker indentation, all relative links resolve.

- [ ] **Step 3: Commit**

  ```
  git add examples/docker-compose/README.md
  git commit -m "$(cat <<'EOF'
  feat(7-iii): examples/docker-compose/README.md — 60-second demo walkthrough

  One-paragraph overview, prereqs, quickstart, /init → /turn → /submit curl
  flow, webhook-receiver log verification, /metrics endpoint verification,
  teardown, and pointers to self-hosting + adapters docs.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

### Task 8: Smoke — bring the stack up, drive a ticket, tear down

**Files:** No file changes. This task is the **mandatory smoke** for 7-iii.

The smoke exercises the entire stack from a clean state and confirms every assertion the 7-iii spec calls for.

- [ ] **Step 1: Clean preflight**

  ```
  cd examples/docker-compose
  docker-compose down -v 2>/dev/null || true
  docker rmi intake-relay:local intake-webhook-receiver:local 2>/dev/null || true
  ```

  Ensures the smoke starts from a known clean state (no cached image, no leftover containers).

- [ ] **Step 2: Bring the stack up**

  ```
  docker-compose up -d --build
  ```

  Expected: build phase completes for both images; three containers start;
  `docker-compose ps` shows all three with `State: running`.

  ```
  docker-compose ps
  ```

- [ ] **Step 3: Wait for the relay to be reachable**

  Distroless has no shell so the relay container has no in-container healthcheck. Poll from the host:

  ```
  for i in $(seq 1 20); do
    code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:18080/v1/health || echo "000")
    if [ "$code" = "200" ]; then
      echo "relay is healthy"
      break
    fi
    sleep 1
  done
  ```

  Expected: prints `relay is healthy` within ~3 iterations.

- [ ] **Step 4: Drive the /init → /turn → /submit flow**

  Use the README's curl snippets (Task 7, Step 1, "Submit a ticket" section). Capture the responses:

  ```
  curl -s -X POST http://localhost:18080/v1/intake/init -H "Content-Type: application/json" -d '{}' | tee /tmp/intake-init.json
  SESSION=$(jq -r .session_id < /tmp/intake-init.json)

  curl -N -s -X POST http://localhost:18080/v1/intake/turn \
    -H "Content-Type: application/json" \
    -H "X-Intake-Session: ${SESSION}" \
    -d '{"messages":[{"role":"user","content":"Hello, intake!"}]}'

  curl -s -X POST http://localhost:18080/v1/intake/submit \
    -H "Content-Type: application/json" \
    -H "X-Intake-Session: ${SESSION}" \
    -d '{"messages":[{"role":"user","content":"Hello"}],"client":{"widget_version":"demo"},"user_claims":{},"context":{"app_context":{},"page_metadata":{}},"routing_hint":null}' \
    | tee /tmp/intake-submit.json
  ```

  Expected:
  - `/init` returns 200 with `session_id` + `capabilities.attachments` (the demo enables attachments)
  - `/turn` streams SSE events ending with `event: done`
  - `/submit` returns 200 with `external_id` + `adapter_name: "webhook"`

- [ ] **Step 5: Verify the webhook-receiver captured the payload**

  ```
  docker-compose logs webhook-receiver | tail -40
  ```

  Expected: a `POST /intake` log line followed by a formatted JSON dump of the canonical payload including the `messages`, `client`, `user_claims`, and `context` blocks. The body's `messages[0].content` should equal `"Hello"`.

- [ ] **Step 6: Verify the metrics endpoint exposes the 4 series**

  ```
  curl -s http://localhost:19090/metrics | grep -E '^# HELP intake_'
  ```

  Expected: exactly 4 lines, one per series:

  ```
  # HELP intake_http_requests_total ...
  # HELP intake_http_request_duration_seconds ...
  # HELP intake_llm_tokens_total ...
  # HELP intake_adapter_calls_total ...
  ```

- [ ] **Step 7: Verify the relay runs as nonroot (UID 65532)**

  Distroless does not include `id`, but `docker inspect` reads the configured user from image metadata:

  ```
  docker inspect intake-relay --format '{{.Config.User}}'
  ```

  Expected: `nonroot:nonroot` OR `65532:65532` (depending on how 7-ii configured the USER directive).

- [ ] **Step 8: Verify webhook-receiver runs as non-root (UID 1000)**

  node:24-alpine ships a real shell, so `id -u` works:

  ```
  docker exec intake-webhook-receiver id -u
  ```

  Expected: `1000`.

- [ ] **Step 9: Verify `docker-compose config` is clean**

  ```
  docker-compose config > /dev/null
  ```

  Expected: exits 0 with no warnings (especially: NO `WARN: the "version" field is deprecated` line).

- [ ] **Step 10: Tear down**

  ```
  docker-compose down -v
  docker-compose ps
  cd ../..
  ```

  Expected: `docker-compose ps` shows zero rows under the project name `intake-demo`.

- [ ] **Step 11: Smoke verdict**

  If every step above passed, the 7-iii smoke is GREEN. If any step failed, do NOT mark 7-iii done — diagnose, fix, re-run from Step 1.

  No commit for Task 8; the smoke is a verification gate.

---

### Task 9: Verify `examples/docker-compose/` is NOT gitignored

**Files:** No modification expected; verify only

The Phase 7 demo stack IS part of the repo. The repo's existing `.gitignore` (verified during plan authoring) is benign for this directory — it ignores `.env`, `.env.*` (with a `!.env.example` allowlist), `node_modules/`, `dist/`, and `local-dev/`. None of those patterns match `examples/docker-compose/{docker-compose.yml,config.yaml,README.md,.env.example}`.

- [ ] **Step 1: Confirm gitignore treatment**

  ```
  git status --short examples/docker-compose/
  ```

  Expected output (before final commit run; after all prior task commits this directory is clean):

  ```
  ?? examples/docker-compose/README.md
  ?? examples/docker-compose/.env.example
  ?? examples/docker-compose/config.yaml
  ?? examples/docker-compose/docker-compose.yml
  ```

  OR (after all prior task commits land): empty output (everything committed).

  NEVER expected: any `!! examples/docker-compose/...` line (would mean the path is ignored).

- [ ] **Step 2: Confirm the negative case — a real `.env` IS ignored**

  ```
  echo "ANTHROPIC_API_KEY=sk-real-key" > examples/docker-compose/.env
  git status --short examples/docker-compose/
  rm examples/docker-compose/.env
  ```

  Expected: `examples/docker-compose/.env` does NOT appear in the output (it is matched by the root `.gitignore`'s `.env` rule).

  This is a defense-in-depth check: even if a developer accidentally creates a `.env` with real credentials, it cannot be committed by mistake.

- [ ] **Step 3: No commit**

  Verification only. No file changes.

---

## Smoke (mandatory)

Proves the 7-iii deliverable end-to-end. Fully self-runnable from a clean clone; no external credentials, no LLM credit.

```
1. Pin/config gate (no LLM credit; self-runnable):
   cd examples/docker-compose && docker-compose config && cd ../..
   Expected: exit 0; rendered config printed to stdout; NO
   "the version field is deprecated" warning (compose-spec modern format).

2. Cold-boot smoke (no LLM credit; self-runnable; the load-bearing 7-iii smoke):
   cd examples/docker-compose
   docker-compose down -v 2>/dev/null || true
   docker rmi intake-relay:local intake-webhook-receiver:local 2>/dev/null || true
   docker-compose up -d --build
   docker-compose ps           → 3 rows, all State: running
   for i in $(seq 1 20); do
     code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:18080/v1/health || echo "000")
     [ "$code" = "200" ] && break
     sleep 1
   done                        → relay reachable within ~3 attempts
   Expected: 3 healthy containers; /v1/health returns 200 from the host.

3. End-to-end flow smoke (no LLM credit; self-runnable):
   POST /v1/intake/init       → 200; session_id + capabilities.attachments present
   POST /v1/intake/turn (SSE) → stream completes with event: done from fake-llm
   POST /v1/intake/submit     → 200; external_id present; adapter_name = "webhook"
   docker-compose logs webhook-receiver | tail
                              → canonical payload printed as formatted JSON

4. Metrics endpoint smoke (no LLM credit; self-runnable):
   curl http://localhost:19090/metrics | grep -E '^# HELP intake_'
   Expected: exactly 4 lines (intake_http_requests_total,
   intake_http_request_duration_seconds, intake_llm_tokens_total,
   intake_adapter_calls_total).

5. Non-root invariant smoke (no LLM credit; self-runnable):
   docker inspect intake-relay --format '{{.Config.User}}'
                              → "nonroot:nonroot" or "65532:65532"
   docker exec intake-webhook-receiver id -u
                              → 1000

6. Teardown smoke (no LLM credit; self-runnable):
   docker-compose down -v
   docker-compose ps          → zero rows
   cd ../..

7. Gitignore allowlist smoke (no LLM credit; self-runnable):
   git status --short examples/docker-compose/
   Expected: lists the four committed files OR is empty (after commits).
   NEVER: any !! prefix line for paths in this directory.
   Negative regression: temporarily create examples/docker-compose/.env;
   git status hides it (matches root .gitignore .env rule); remove.

8. Phase 7-ii regression (no LLM credit; self-runnable; only if Task 3 ran):
   docker build -t intake-relay:local relay/
                              → exits 0
   docker run --rm intake-relay:local --version
                              → relay --version printout
   docker run --rm --entrypoint /fake-llm intake-relay:local --help
                              → fake-llm flag list
   docker images intake-relay --format '{{.Size}}'
                              → < 50 MB (Phase 7 distroless invariant)
```

Smokes 1-7 are mandatory; smoke 8 is conditional on Task 3 having modified the relay Dockerfile (per Task 0's verdict). All smokes are fully self-runnable; no live API credentials consumed; `html2canvas`/Anthropic/Turnstile are never touched.

---

## Done criteria

- [ ] Task 0 verdict recorded: 7-ii Dockerfile either already builds `fake-llm` (Task 3 is a no-op) OR does not (Task 3 extends it). The verdict is noted in the Task 3 preamble.
- [ ] `examples/webhook-receiver/Dockerfile` exists, uses `node:24-alpine`, `EXPOSE 9099`, `USER node`, and `CMD ["node", "/app/server.mjs"]`; `docker build examples/webhook-receiver/` exits 0.
- [ ] `examples/webhook-receiver/package.json` confirmed to have `"type": "module"`, `"private": true`, and no `dependencies` block.
- [ ] (If Task 3 ran) `relay/Dockerfile` builds + ships BOTH `intake-relay` and `fake-llm` binaries in stage 2; `docker run --rm --entrypoint /fake-llm intake-relay:local --help` prints the flag list; `docker images intake-relay --format '{{.Size}}'` is < 50 MB.
- [ ] `examples/docker-compose/docker-compose.yml` uses compose-spec modern format (no `version:` field), declares 3 services + 1 named bridge network, uses `entrypoint: []` + `command:` on the `fake-llm` service, uses `depends_on` with `condition: service_healthy` on `webhook-receiver` and `condition: service_started` on `fake-llm`, and `docker-compose config` exits 0 with no warnings.
- [ ] `examples/docker-compose/config.yaml` declares `server.addr: ":8080"`, `auth.modes.anonymous: true`, `auth.anonymous.allow_without_captcha: true`, `llm.provider: "ollama"` with `ollama.base_url: "http://fake-llm:11434"`, `attachments.enabled: true`, `adapters.webhook.url: "http://webhook-receiver:9099/intake"`, `routing.default_adapter: "webhook"`, `ratelimit.daily_llm_budget.action_on_exceeded: "reject"`, AND `observability.metrics.enabled: true` + `observability.metrics.addr: ":9090"`.
- [ ] `examples/docker-compose/.env.example` defines `ANTHROPIC_API_KEY=dummy` and is tracked in git (NOT matched by the `.gitignore` rule for `.env*` thanks to the `!.env.example` allowlist).
- [ ] `examples/docker-compose/README.md` covers: one-paragraph overview, prereqs, quickstart, `/init → /turn → /submit` curl flow, `docker-compose logs webhook-receiver` verification, `/metrics` endpoint verification, and `docker-compose down -v` teardown.
- [ ] Smoke section above passes from a clean state (no cached images, no leftover containers, no `.env` file).
- [ ] `git status --short examples/docker-compose/` is empty after all commits land; no `!!` (ignored) lines for paths in this directory.
- [ ] All commits use `feat(7-iii): ...` (or `chore(7-iii): ...` if Task 3's Dockerfile change is judged purely build-system); commit messages include the HEREDOC `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` footer.
- [ ] No production code outside `examples/` modified, with the sole exception of `relay/Dockerfile` (Task 3, conditional on Task 0).
- [ ] No frozen Phase 0-6 seam touched: `adapter.Adapter`, `payload/types.go`, `schema/payload.v1.json`, `auth.Middleware`, the chi route-registration shape — all unchanged.

*End of 7-iii plan.*
