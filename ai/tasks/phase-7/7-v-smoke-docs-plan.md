# 7-v Final Smoke + Docs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## Goal

After 7-i (relay code: FOLLOWUPS I1/I2/M2/M4 + Prometheus metrics + lint configs), 7-ii (release artifacts: Dockerfile + .goreleaser.yaml + release.yml), 7-iii (docker-compose demo stack), and 7-iv (docs + governance) all land, 7-v is the final-evidence sub-plan. It (1) authors `core/smoke/drive-docker-compose.ts` — a self-runnable smoke driver that boots the demo stack, drives the full intake flow, asserts the metrics surface, and tears down with `down -v`; (2) executes ALL 8 self-runnable final smoke items from Phase 7 README §7 and captures evidence; (3) appends L024-L028 to `ai/LESSONS.md`; (4) populates the Phase 7 README §7 evidence subsection and flips §3 status column to "Live + smoked"; (5) renames `ai/tasks/phase-6/FOLLOWUPS.md` → `FOLLOWUPS-resolved.md` with a "resolved by Phase 7-i" header banner; (6) (optional) fixes the PROJECT.md §14 + §15 inconsistencies the design spec flagged; (7) records final green-bar exit codes in a `chore(7-v):` commit.

**No maintainer-paused live smokes in Phase 7** — per the scope-boundary decision (Phase 7 generates artifacts locally and never publishes), every smoke item is self-runnable. The Phase 6 PAUSE-for-chatwoot pattern does not recur here; Phase 7's load-bearing live proof is the docker-compose demo, which uses fake-llm + the webhook adapter and needs zero external credentials.

## Architecture

Five artifact families. (1) One new TypeScript smoke driver — `core/smoke/drive-docker-compose.ts` — that orchestrates `docker-compose up -d`, polls `/v1/health`, drives `/init` → `/turn` → `/submit`, asserts the canonical payload landed verbatim in the webhook-receiver, scrapes the metrics endpoint on the host-mapped port from 7-iii's compose, asserts the distroless nonroot UID via `docker exec`, then `docker-compose down -v`. (2) Captured evidence for all 8 Phase 7 README §7 smoke items — Q9 combined-misconfig (6+ problems), metrics endpoint disabled-vs-enabled, snapshot release (all 5 archives + image), npm pack dry-run (no secret leak), docker-compose demo (the driver from (1)), docs walkthrough (self-review against `quickstart.md`), Phase 1-6 regression (drive-attachments.ts + drive-abuse.ts + go test + scripts + go mod tidy + frozen-seam diff), and lint smoke (golangci-lint + eslint + prettier). (3) Five new `ai/LESSONS.md` entries L024-L028 — snapshot-then-publish split, initial lint sweep before CI gate, metrics server independence, off-by-default observability, distroless multi-stage Docker template. (4) Phase 7 README §3 sub-plan status flips + §7.1 "Smoke status (YYYY-MM-DD)" evidence subsection mirroring Phase 5/6 README §7 evidence format. (5) `ai/tasks/phase-6/FOLLOWUPS.md` renamed to `FOLLOWUPS-resolved.md` with a "resolved by Phase 7-i" header banner; any cross-references (Phase 6 README, Phase 7 README §1) updated to point at the renamed file.

After this sub-plan: Phase 7 has the same "evidence-of-everything" gate as Phase 5 and Phase 6 — every build-fail item from §6 is proven, every smoke item from §7 has a captured PASS/PARTIAL/FAIL verdict, every frozen seam is proven unchanged via `git diff main..phase-7`, and the LESSONS file is the durable record of the patterns Phase 7 established (snapshot-then-publish, initial lint sweep, metrics independence, off-by-default observability, distroless multi-stage Docker).

## Tech Stack

- Node 24 / TypeScript 5.6.3 (smoke driver via `npx tsx`), stdlib only (`node:child_process`, `node:http`, `node:net`); the driver imports nothing from `@intake/core` so the L004 browser-global stubs are kept for future-proofing only (driver uses raw `fetch` for full status-code + body access, mirroring `drive-attachments.ts`).
- Go 1.23.2 — re-runs Phase 5's `relay/cmd/fake-llm` (via docker-compose; no new Go code in 7-v).
- Docker / Docker Compose — the demo stack from 7-iii (relay + fake-llm + webhook-receiver).
- Stdlib only — no new TS or Go dependencies. `go mod tidy` must remain a no-op (the `prometheus/client_golang` module added in 7-i is the only new one; 7-v adds zero).

## Design References

- Phase 7 README §6 — build-fail items (must all still hold at end of phase)
- Phase 7 README §7 — the authoritative final smoke recipe (what gets recorded as evidence)
- Phase 7 README §8 — frozen contracts the smokes assert against (frozen Phase 0-6 seams + metrics package shape + Deps extension)
- Phase 7 design spec §11 — final smoke
- Phase 7 design spec §14 — new patterns L024-L028
- Phase 7 design spec §15 — PROJECT.md §14 + §15 inconsistency notes (optional cleanup)
- Phase 6 `6-iv-smoke-docs-plan.md` — the structural exemplar (smoke driver style, smoke ordering, LESSONS append pattern, README §7 evidence format, sub-plan status flip)
- Phase 5 `5-iv-smoke-docs-plan.md` — the original "smoke + LESSONS + docs" template
- Phase 5 README §7 evidence + Phase 6 README §7 evidence — the EVIDENCE FORMAT TEMPLATE for Phase 7's §7.1 subsection
- Existing smoke drivers `core/smoke/drive-abuse.ts`, `core/smoke/drive-auth-email.ts`, `core/smoke/drive-attachments.ts` — the canonical pattern (raw `fetch`, `assert(cond, msg)` helper, `process.exit(1)` on fail, L004 browser-global stubs via `Object.defineProperty`)
- Phase 6 `FOLLOWUPS.md` — the four items (I1, I2, M2, M4) folded into Phase 7-i; this file is renamed to `FOLLOWUPS-resolved.md` in Task 12
- `ai/LESSONS.md` L016-L023 — the format template for L024-L028 (number + title + body with **Where it hit:** + **Rule:** + Reference)
- `docs/PROJECT.md` §14 (repo layout) + §15 (build/release) — the optional cleanup targets (§14 lists `core/src/screenshot.ts` + `core/src/auth.ts` files that never existed; §15 lists `intake-license` CLI that is excluded from goreleaser per decomposition Q10)

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `core/smoke/drive-docker-compose.ts` | Create | TS driver: boots docker-compose, polls health, drives /init → /turn → /submit, asserts canonical payload + metrics + distroless UID, tears down |
| `ai/LESSONS.md` | Modify | Append L024 (snapshot-then-publish split), L025 (initial lint sweep before CI gate), L026 (metrics server independence), L027 (off-by-default observability), L028 (distroless multi-stage Docker template) |
| `ai/tasks/phase-7/README.md` | Modify | §3 sub-plan index status → "Live + smoked" for all 5 sub-plans; §7.1 "Smoke status (YYYY-MM-DD)" subsection populated with full 8-item evidence |
| `ai/tasks/phase-6/FOLLOWUPS.md` → `ai/tasks/phase-6/FOLLOWUPS-resolved.md` | Rename + prepend banner | Audit-preserving rename with "all 4 items closed in Phase 7-i" header banner; update any cross-references in Phase 6 README + Phase 7 README §1 |
| `docs/PROJECT.md` | Modify (optional) | §14 repo-layout: drop `screenshot.ts` + `auth.ts`, add `capture.ts`/`attachments.ts`/etc.; §15: note `intake-license` excluded from goreleaser per Q10 |

---

## Tasks

### Task 1: Author `core/smoke/drive-docker-compose.ts`

**Files:** Create `core/smoke/drive-docker-compose.ts`

- [ ] **Step 1: Write the driver**

Create `core/smoke/drive-docker-compose.ts`:

```typescript
/**
 * Phase 7 docker-compose demo smoke driver.
 *
 * Drives the full intake flow against the demo stack from
 * examples/docker-compose/ (relay + fake-llm + webhook-receiver):
 *   1. docker-compose up -d (via child_process.spawn)
 *   2. Poll http://localhost:18080/v1/health until 200 (30s deadline, backoff)
 *   3. POST /v1/intake/init   → assert session_id + capabilities.attachments
 *   4. POST /v1/intake/turn   → assert SSE stream completes (fake-llm)
 *   5. POST /v1/intake/submit → assert external_id + adapter_name="webhook"
 *   6. Poll webhook-receiver introspection (or grep docker-compose logs) →
 *      assert canonical payload landed verbatim with messages/client/user
 *   7. curl http://localhost:19090/metrics → assert 4 series present AND
 *      intake_http_requests_total{path="/v1/intake/init",status="200"} >= 1
 *   8. docker exec intake-relay id -u → assert "65532" (distroless nonroot UID)
 *   9. docker-compose down -v (volumes too, no cross-run state)
 *
 * Self-contained: the demo uses fake-llm + the webhook adapter, so NO
 * maintainer credentials are required (no chatwoot/zendesk/linear tokens,
 * no LLM API keys). The smoke is fully self-runnable on any machine that
 * has docker + docker-compose installed.
 *
 * Browser-global stubs (LESSONS L004): the driver uses raw fetch and does
 * not import IntakeClient, so the stubs are not strictly required, but
 * kept for symmetry with drive-attachments.ts / drive-auth-email.ts and
 * to future-proof if a step migrates to IntakeClient.
 *
 * Usage:
 *   npx tsx core/smoke/drive-docker-compose.ts
 *
 * Environment:
 *   COMPOSE_DIR     defaults to examples/docker-compose
 *   RELAY_URL       defaults to http://localhost:18080
 *   METRICS_URL     defaults to http://localhost:19090
 *   WEBHOOK_LOG_CMD defaults to "docker-compose logs --no-color webhook-receiver"
 *   COMPOSE_BIN     defaults to "docker-compose" (override to "docker compose" if v2 plugin)
 */

import { spawn, type ChildProcess } from 'node:child_process';

const COMPOSE_DIR = process.env['COMPOSE_DIR'] ?? 'examples/docker-compose';
const RELAY_URL = process.env['RELAY_URL'] ?? 'http://localhost:18080';
const METRICS_URL = process.env['METRICS_URL'] ?? 'http://localhost:19090';
const COMPOSE_BIN = process.env['COMPOSE_BIN'] ?? 'docker-compose';
const HEALTH_DEADLINE_MS = 30_000;

// --- Browser-global stubs (LESSONS L004) --------------------------------------
//
// L004: Node smoke harnesses must stub browser globals via Object.defineProperty,
// NOT plain assignment, because globalThis.navigator is a read-only getter in
// Node 24+. This driver uses raw fetch and does not currently call IntakeClient,
// but the stubs are kept for symmetry with drive-attachments.ts and to future-
// proof if a step migrates to the typed client.

function stubBrowserGlobals(): void {
  const def = (name: string, value: unknown): void => {
    Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  };
  def('window', {
    location: { href: 'http://smoke.test/' },
    innerWidth: 1024,
    innerHeight: 768,
  });
  def('navigator', { userAgent: 'smoke', language: 'en-US' });
  def('document', {
    referrer: '',
    title: 'smoke',
    querySelectorAll: () => [],
  });
}
stubBrowserGlobals();

// --- Helpers ------------------------------------------------------------------

function assert(cond: boolean, msg: string): void {
  if (!cond) {
    console.error(`FAIL: ${msg}`);
    process.exit(1);
  }
  console.log(`OK: ${msg}`);
}

interface ExecResult {
  code: number;
  stdout: string;
  stderr: string;
}

function exec(cmd: string, args: string[], opts?: { cwd?: string }): Promise<ExecResult> {
  return new Promise((resolve) => {
    const child: ChildProcess = spawn(cmd, args, {
      cwd: opts?.cwd,
      stdio: ['ignore', 'pipe', 'pipe'],
      shell: process.platform === 'win32',
    });
    let stdout = '';
    let stderr = '';
    child.stdout?.on('data', (b: Buffer) => (stdout += b.toString('utf8')));
    child.stderr?.on('data', (b: Buffer) => (stderr += b.toString('utf8')));
    child.on('close', (code) => resolve({ code: code ?? -1, stdout, stderr }));
  });
}

async function sleep(ms: number): Promise<void> {
  await new Promise((r) => setTimeout(r, ms));
}

// --- Lifecycle ----------------------------------------------------------------

async function composeUp(): Promise<void> {
  console.log(`\n=== docker-compose up -d (cwd=${COMPOSE_DIR}) ===`);
  const r = await exec(COMPOSE_BIN, ['up', '-d'], { cwd: COMPOSE_DIR });
  if (r.code !== 0) {
    console.error(r.stdout);
    console.error(r.stderr);
    throw new Error(`compose up exit ${r.code}`);
  }
  console.log(r.stdout);
}

async function composeDown(): Promise<void> {
  console.log(`\n=== docker-compose down -v ===`);
  const r = await exec(COMPOSE_BIN, ['down', '-v'], { cwd: COMPOSE_DIR });
  console.log(r.stdout);
  if (r.code !== 0) {
    console.error(r.stderr);
  }
}

async function waitForHealth(): Promise<void> {
  console.log(`\n=== waiting for ${RELAY_URL}/v1/health ===`);
  const deadline = Date.now() + HEALTH_DEADLINE_MS;
  let backoff = 250;
  while (Date.now() < deadline) {
    try {
      const resp = await fetch(`${RELAY_URL}/v1/health`);
      if (resp.status === 200) {
        console.log(`OK: /v1/health 200 after ${HEALTH_DEADLINE_MS - (deadline - Date.now())}ms`);
        return;
      }
    } catch {
      // connection refused / DNS hiccup during startup
    }
    await sleep(backoff);
    backoff = Math.min(backoff * 2, 2_000);
  }
  throw new Error(`/v1/health did not return 200 within ${HEALTH_DEADLINE_MS}ms`);
}

// --- Flow assertions ----------------------------------------------------------

interface InitResponse {
  session_id: string;
  capabilities: {
    auth_modes: string[];
    streaming: boolean;
    requires_captcha?: string[];
    attachments?: {
      max_size_bytes: number;
      max_total_bytes: number;
      allowed_mime_types: string[];
    } | null;
  };
}

async function driveInit(): Promise<InitResponse> {
  console.log(`\n=== POST /v1/intake/init ===`);
  const resp = await fetch(`${RELAY_URL}/v1/intake/init`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  });
  assert(resp.status === 200, `/init returns 200 (got ${resp.status})`);
  const body = (await resp.json()) as InitResponse;
  assert(typeof body.session_id === 'string' && body.session_id.length > 0, 'init returns non-empty session_id');
  assert(
    body.capabilities.attachments != null,
    'init returns capabilities.attachments (the demo enables attachments by default)',
  );
  return body;
}

async function driveTurn(sessionID: string): Promise<void> {
  console.log(`\n=== POST /v1/intake/turn (SSE) ===`);
  const resp = await fetch(`${RELAY_URL}/v1/intake/turn`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Intake-Session': sessionID },
    body: JSON.stringify({ messages: [{ role: 'user', content: 'phase 7 demo smoke' }] }),
  });
  assert(resp.status === 200, `/turn returns 200 (got ${resp.status})`);
  assert(resp.body !== null, '/turn body stream is non-null');
  if (!resp.body) return;
  const reader = resp.body.getReader();
  let sawDone = false;
  const decoder = new TextDecoder();
  let buf = '';
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    if (buf.includes('event: done') || buf.includes('"type":"done"')) {
      sawDone = true;
      break;
    }
  }
  await reader.cancel();
  assert(sawDone, '/turn SSE stream emitted a terminal done event');
}

interface SubmitResponse {
  external_id?: string;
  adapter_name?: string;
}

async function driveSubmit(sessionID: string): Promise<SubmitResponse> {
  console.log(`\n=== POST /v1/intake/submit ===`);
  const body = {
    messages: [{ role: 'user', content: 'phase 7 demo smoke' }],
    client: {
      url: 'http://smoke.test/',
      user_agent: 'smoke',
      language: 'en-US',
      viewport_width: 1024,
      viewport_height: 768,
      referrer: '',
      page_title: 'smoke',
    },
    user_claims: {},
    context: { dom_snippet: '' },
  };
  const resp = await fetch(`${RELAY_URL}/v1/intake/submit`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Intake-Session': sessionID },
    body: JSON.stringify(body),
  });
  assert(resp.status === 200, `/submit returns 200 (got ${resp.status})`);
  const json = (await resp.json()) as SubmitResponse;
  assert(typeof json.external_id === 'string' && json.external_id.length > 0, '/submit returns external_id');
  assert(json.adapter_name === 'webhook', `/submit adapter_name is "webhook" (got ${String(json.adapter_name)})`);
  return json;
}

async function assertWebhookCanonicalPayloadLanded(): Promise<void> {
  console.log(`\n=== assert canonical payload landed in webhook-receiver ===`);
  // The webhook-receiver from 7-iii logs every received body to stdout.
  // Grep the compose logs for the canonical fields.
  const r = await exec(COMPOSE_BIN, ['logs', '--no-color', 'webhook-receiver'], { cwd: COMPOSE_DIR });
  assert(r.code === 0, `docker-compose logs webhook-receiver exit 0 (got ${r.code})`);
  const log = r.stdout + r.stderr;
  assert(log.includes('phase 7 demo smoke'), 'webhook-receiver log contains the user message text');
  assert(log.includes('"client"'), 'webhook-receiver log contains the client section');
  assert(log.includes('"messages"'), 'webhook-receiver log contains the messages section');
}

async function assertMetricsSurface(): Promise<void> {
  console.log(`\n=== curl ${METRICS_URL}/metrics ===`);
  const resp = await fetch(`${METRICS_URL}/metrics`);
  assert(resp.status === 200, `/metrics returns 200 (got ${resp.status})`);
  const ct = resp.headers.get('content-type') ?? '';
  assert(ct.startsWith('text/plain'), `/metrics Content-Type is text/plain (got "${ct}")`);
  const body = await resp.text();
  // Assert all four series present via their HELP lines.
  for (const series of [
    '# HELP intake_http_requests_total',
    '# HELP intake_http_request_duration_seconds',
    '# HELP intake_llm_tokens_total',
    '# HELP intake_adapter_calls_total',
  ]) {
    assert(body.includes(series), `/metrics declares ${series}`);
  }
  // Assert at least one /init request was observed.
  const initLine = body
    .split('\n')
    .find(
      (line) =>
        line.startsWith('intake_http_requests_total{') &&
        line.includes('path="/v1/intake/init"') &&
        line.includes('status="200"'),
    );
  assert(initLine != null, '/metrics has intake_http_requests_total{path="/v1/intake/init",status="200"} sample');
  if (initLine) {
    const m = /\s(\d+(?:\.\d+)?)\s*$/.exec(initLine);
    const val = m ? Number(m[1]) : 0;
    assert(val >= 1, `init counter >= 1 (got ${val})`);
  }
}

async function assertDistrolessNonrootUID(): Promise<void> {
  console.log(`\n=== docker exec intake-relay id -u ===`);
  const r = await exec('docker', ['exec', 'intake-relay', 'id', '-u']);
  assert(r.code === 0, `docker exec id -u exit 0 (got ${r.code}); stderr=${r.stderr.slice(0, 200)}`);
  const uid = r.stdout.trim();
  assert(uid === '65532', `relay runs as distroless nonroot UID 65532 (got "${uid}")`);
}

// --- Main ---------------------------------------------------------------------

async function main(): Promise<void> {
  console.log(`docker-compose smoke: COMPOSE_DIR=${COMPOSE_DIR} RELAY_URL=${RELAY_URL} METRICS_URL=${METRICS_URL}`);
  await composeUp();
  try {
    await waitForHealth();
    const init = await driveInit();
    await driveTurn(init.session_id);
    await driveSubmit(init.session_id);
    await assertWebhookCanonicalPayloadLanded();
    await assertMetricsSurface();
    await assertDistrolessNonrootUID();
    console.log('\n✓ All Phase 7 docker-compose smoke assertions passed.');
  } finally {
    await composeDown();
  }
}

main().catch((err: unknown) => {
  console.error('smoke driver failed:', err);
  // Best-effort cleanup before exiting non-zero.
  composeDown().finally(() => process.exit(1));
});
```

- [ ] **Step 2: Commit**

```bash
git add core/smoke/drive-docker-compose.ts
git commit -m "$(cat <<'EOF'
feat(7-v): drive-docker-compose.ts — boots demo stack, asserts canonical payload + metrics + distroless UID

Self-runnable Phase 7 demo smoke driver. Boots examples/docker-compose/ via
docker-compose up -d, polls /v1/health (30s deadline, exponential backoff),
drives /init → /turn → /submit through the fake-llm + webhook adapter chain,
asserts the canonical payload landed verbatim in the webhook-receiver logs,
scrapes :19090/metrics for all 4 series + a non-zero /init counter, asserts
the relay runs as distroless nonroot UID 65532 via docker exec id -u, then
tears down with down -v (no cross-run state). L004 browser-global stubs via
Object.defineProperty for future-proofing.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Smoke item 1 — Q9 combined-misconfig fixture

**Files:** none new (reuses Phase 5 + Phase 6 + Phase 7 fixtures consolidated by 7-i)

- [ ] **Step 1: Run the Q9 combined-misconfig smoke**

The combined fixture authored in 7-i (`relay/cmd/relay/smoke/q9-combined-misconfig.yaml` — or whatever name 7-i settled on; it extends Phase 6's `attachments-combined.yaml` with the chatwoot `api_token_env: NONEXISTENT_VAR` line that proves I1 — `buildRegistry` now contributes problems, not exits). Covers Phase 5 + Phase 6 + Phase 7 in one file:

- `auth.modes.anonymous=true` without captcha (Phase 5)
- `server.trusted_proxies` invalid CIDR (Phase 5)
- `ratelimit.daily_llm_budget.action_on_exceeded=queue` (Phase 5)
- `adapter chatwoot api_token_env=NONEXISTENT_VAR` (Phase 7-i / FOLLOWUP I1)
- `attachments.storage.mode=s3` (Phase 6)
- `attachments.max_size_bytes > max_total_bytes` (Phase 6)

```bash
cd c:/src/ai/intake
output=$(go run ./relay/cmd/relay --config relay/cmd/relay/smoke/q9-combined-misconfig.yaml 2>&1 || true)
echo "$output" > /tmp/q9-combined-smoke.txt
echo "$output"

# Assertions:
log_count=$(echo "$output" | grep -c "relay: startup config errors" || true)
[ "$log_count" -eq 1 ] && echo "OK: exactly one consolidated log line" || { echo "FAIL: got $log_count lines"; exit 1; }

for substr in "anonymous" "not-a-cidr" "action_on_exceeded" "NONEXISTENT_VAR" "storage.mode" "max_size_bytes"; do
  echo "$output" | grep -q "$substr" && echo "OK: combined matched '$substr'" || { echo "FAIL: missing '$substr'"; exit 1; }
done

# Count problems in the log line (count>=6)
count_field=$(echo "$output" | grep -oE '"count":[0-9]+' | head -1)
echo "Problem count field: $count_field"
```

Expected: ONE consolidated `relay: startup config errors` log line with `count >= 6`; every distinct problem substring matched (Phase 5 + Phase 6 + Phase 7); exit 1. This is the L022 invariant proven across THREE phases — operator fixes all six problems in one restart cycle. Closes FOLLOWUPS I1 (buildRegistry now contributes instead of exiting) and I2 (the orchestration is testable via the extracted `accumulateStartupProblems` function from 7-i).

- [ ] **Step 2: Record output to scratch file**

Save raw output to `/tmp/q9-combined-smoke.txt` (or `$env:TEMP\q9-combined-smoke.txt` on Windows) for transcription into Phase 7 README §7.1 evidence in Task 11. No commit here.

---

### Task 3: Smoke item 2 — metrics endpoint disabled-vs-enabled

**Files:** none new (uses the smoke fixture from 7-i with `Observability.Metrics.Enabled` toggled)

- [ ] **Step 1: Item 2a — disabled arm (Enabled=false)**

```bash
# Start relay with metrics disabled (the default; explicit in the smoke YAML)
cd c:/src/ai/intake
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/metrics-disabled.yaml &
RELAY_PID=$!
sleep 1

# /metrics should NOT be reachable (connection refused on :9090)
curl -sS -m 2 http://localhost:9090/metrics 2>&1 | tee -a /tmp/metrics-smoke.txt || true
# Expected: "Connection refused" or "Couldn't connect" — connect failure, not a 404 with a body.
# Verify the main relay is still happy:
curl -sS -m 2 http://localhost:18080/v1/health
echo

kill $RELAY_PID
```

Expected:
- The `/metrics` curl returns a transport-level error (connection refused) — not a 200, not a 404 with a body, not a TLS handshake failure.
- The main `/v1/health` curl returns 200 — proves the metrics server being disabled does NOT brick the relay (L026 independence invariant).

- [ ] **Step 2: Item 2b — enabled arm (Enabled=true)**

```bash
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/metrics-enabled.yaml &
RELAY_PID=$!
sleep 1

# /metrics reachable; 4 HELP lines present
curl -sS http://localhost:9090/metrics | grep '# HELP intake_' | tee -a /tmp/metrics-smoke.txt
# Expected output (4 lines):
#   # HELP intake_http_requests_total ...
#   # HELP intake_http_request_duration_seconds ...
#   # HELP intake_llm_tokens_total ...
#   # HELP intake_adapter_calls_total ...

# Drive 5 /init requests
for i in 1 2 3 4 5; do
  curl -sS -X POST http://localhost:18080/v1/intake/init -d '{}' -H 'Content-Type: application/json' > /dev/null
done

# Assert counter increased by exactly 5
counter_line=$(curl -sS http://localhost:9090/metrics | grep 'intake_http_requests_total{' | grep 'path="/v1/intake/init"' | grep 'status="200"')
echo "Counter line: $counter_line" | tee -a /tmp/metrics-smoke.txt
# Extract the trailing number; assert >= 5
val=$(echo "$counter_line" | awk '{print $NF}')
[ "$val" -ge 5 ] && echo "OK: init counter >= 5 (got $val)" || { echo "FAIL: counter $val"; exit 1; }

kill $RELAY_PID
```

Expected: 4 HELP lines emitted; init counter at least 5 after 5 explicit /init requests. Proves the metrics middleware observes every request through chi's `RoutePattern()` (bounded cardinality — confirms no `/v1/intake/{session_id}` cardinality explosion).

- [ ] **Step 3: Record outputs to scratch file**

Append both arms' outputs to `/tmp/metrics-smoke.txt` for Task 11 transcription. No commit.

---

### Task 4: Smoke item 3 — goreleaser snapshot release

**Files:** none new (uses 7-ii's `.goreleaser.yaml`)

- [ ] **Step 1: Run `goreleaser release --snapshot --clean`**

```bash
cd c:/src/ai/intake
# 7-ii pinned goreleaser; use the pinned version (CI installs the same)
goreleaser release --snapshot --clean --config relay/.goreleaser.yaml 2>&1 | tee /tmp/goreleaser-snapshot.txt

# Assert all 5 archives + SHA256SUMS + docker image
ls -lh dist/ | tee -a /tmp/goreleaser-snapshot.txt
for arch in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  ls dist/intake-relay_*_${arch}.tar.gz && echo "OK: $arch archive present" || { echo "FAIL: missing $arch archive"; exit 1; }
done
ls dist/intake-relay_*_windows_amd64.zip && echo "OK: windows_amd64 archive present" || { echo "FAIL"; exit 1; }
ls dist/SHA256SUMS.txt && echo "OK: SHA256SUMS.txt present" || { echo "FAIL"; exit 1; }

# Docker image present in local daemon
docker images | grep intake-relay | tee -a /tmp/goreleaser-snapshot.txt
docker images intake-relay --format '{{.Size}}' | tee -a /tmp/goreleaser-snapshot.txt
```

Expected:
- Exit 0
- `dist/` contains 5 archives: `linux_amd64`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`, `windows_amd64`
- `dist/SHA256SUMS.txt` covers all 5
- `ghcr.io/intake/intake-relay:snapshot` (or whatever 7-ii's image-name tag is) present in local Docker daemon
- No `docker push` attempted, no remote mutation
- Image size < 50 MB (distroless invariant)

- [ ] **Step 2: Extract each archive + spot-check binary --version**

```bash
mkdir -p /tmp/goreleaser-extract
for arch in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  tar -xzf dist/intake-relay_*_${arch}.tar.gz -C /tmp/goreleaser-extract --one-top-level=$arch
done
unzip -o dist/intake-relay_*_windows_amd64.zip -d /tmp/goreleaser-extract/windows_amd64

# Spot-check the host-platform binary (assume linux_amd64 on most CI runners)
/tmp/goreleaser-extract/linux_amd64/intake-relay --version | tee -a /tmp/goreleaser-snapshot.txt
```

Expected: each archive extracts cleanly; the binary's `--version` printout matches the snapshot version goreleaser generated.

- [ ] **Step 3: Record output to scratch file (Task 11)**

---

### Task 5: Smoke item 4 — npm pack dry-run + secret-leak check

**Files:** none new (uses 7-iv's curated `package.json` files in `core/` + `vue/`)

- [ ] **Step 1: Pack both workspaces**

```bash
cd c:/src/ai/intake
npm pack -w @intake/core 2>&1 | tee /tmp/npm-pack-smoke.txt
npm pack -w @intake/vue 2>&1 | tee -a /tmp/npm-pack-smoke.txt
ls intake-core-*.tgz intake-vue-*.tgz | tee -a /tmp/npm-pack-smoke.txt
```

Expected: both tarballs produced.

- [ ] **Step 2: Dry-run publish both workspaces**

```bash
npm publish -w @intake/core --dry-run 2>&1 | tee -a /tmp/npm-pack-smoke.txt
npm publish -w @intake/vue --dry-run 2>&1 | tee -a /tmp/npm-pack-smoke.txt
```

Expected: both exit 0; the dry-run report lists every file the real publish would upload.

- [ ] **Step 3: Secret-leak grep on both tarballs**

```bash
for tgz in intake-core-*.tgz intake-vue-*.tgz; do
  echo "=== inspecting $tgz ==="
  contents=$(tar -tzf "$tgz")
  echo "$contents" | tee -a /tmp/npm-pack-smoke.txt

  # Assert no .env / local-dev/ / secrets / credentials
  echo "$contents" | grep -E '\.env$|local-dev/|secrets|credentials' && { echo "FAIL: secret-leak suspect in $tgz"; exit 1; } || echo "OK: no secret-leak suspect in $tgz"
done
```

Expected:
- No `.env` file in either tarball
- No `local-dev/` directory in either tarball
- No files containing `secrets` or `credentials` in their name
- `package.json` has `description`, `repository`, `license` fields (the `goreleaser`/npm convention check from 7-iv)

- [ ] **Step 4: Cleanup**

```bash
rm intake-core-*.tgz intake-vue-*.tgz
```

- [ ] **Step 5: Record outputs to scratch file (Task 11)**

---

### Task 6: Smoke item 5 — docker-compose demo via `drive-docker-compose.ts`

**Files:** none new (uses Task 1's driver + 7-iii's `examples/docker-compose/`)

- [ ] **Step 1: Pre-flight — verify docker + docker-compose are available**

```bash
docker --version
docker-compose --version || docker compose version
```

Expected: both commands print a version. If docker-compose v2 is installed as a plugin (`docker compose` instead of `docker-compose`), set `COMPOSE_BIN="docker compose"` before running the driver.

- [ ] **Step 2: Build the relay image locally**

```bash
cd c:/src/ai/intake
# 7-iii's docker-compose.yml references `build: ../../relay` so this step
# is implicit in `up -d`, but doing it explicitly catches build errors before
# the 30s health-poll deadline.
docker build -t intake-relay relay/ 2>&1 | tee /tmp/docker-compose-smoke.txt
```

Expected: exit 0; image size < 50 MB (distroless invariant from 7-ii). The build-fail item `docker images intake-relay --format '{{.Size}}' > 50 MB → Fail` is exercised here.

- [ ] **Step 3: Run the driver**

```bash
cd c:/src/ai/intake
npx tsx core/smoke/drive-docker-compose.ts 2>&1 | tee -a /tmp/docker-compose-smoke.txt
```

Expected: every `OK:` assertion fires; final line `✓ All Phase 7 docker-compose smoke assertions passed.` The driver tears down with `docker-compose down -v` in its `finally` block; even on assertion failure, no containers/volumes are left running.

- [ ] **Step 4: Verify no containers/volumes left behind**

```bash
docker ps -a | grep -E 'intake-relay|intake-fake-llm|intake-webhook' && { echo "FAIL: containers left running"; exit 1; } || echo "OK: no Phase 7 containers running"
docker volume ls | grep intake | head -5
```

Expected: no rows match — cleanup is total.

- [ ] **Step 5: Record output to scratch file (Task 11)**

---

### Task 7: Smoke item 6 — docs walkthrough (self-review)

**Files:** none new (consumes 7-iv's `docs/quickstart.md`, `docs/self-hosting.md`, `docs/license.md`, `docs/adapters.md`)

Phase 7 README §7 item 6 calls this a "manual" smoke from a "fresh clone" perspective. The implementer subagent cannot reach a truly fresh clone state, so this becomes a self-review pass:

- [ ] **Step 1: Read `docs/quickstart.md` as if a new operator**

Open `docs/quickstart.md`. For every command in the quickstart, run it verbatim in a scratch directory (`/tmp/intake-quickstart-self-review`) and verify:
- Every `git clone` / `cd` / `cp` / `docker-compose up` / `curl` command actually exists in the repo or in the env (no missing files, no missing tools)
- Every code block compiles, every JSON example parses
- Every relative link in the doc resolves (`../README.md`, `./self-hosting.md`, `./adapters.md`)
- The end-state assertion (e.g. "ticket appears in webhook log within 30 min") matches what `drive-docker-compose.ts` Task 6 already proved

Capture observations in a brief note:

```
=== docs/quickstart.md walkthrough (YYYY-MM-DD) ===
- Step 1 (clone): commands verbatim, exit 0
- Step 2 (cp config): file paths match repo layout
- Step 3 (docker-compose up -d): identical to Task 6 driver behavior
- Step 4 (POST /init + /turn + /submit): identical to Task 6 driver assertions
- Step 5 (webhook log inspection): identical to Task 6 webhook-canonical assertion
- All relative links resolve.
- End-state matches.
- VERDICT: PASS
```

- [ ] **Step 2: Read `docs/self-hosting.md`**

Same approach. Pay particular attention to:
- The metrics endpoint section — operators need clear off-by-default + opt-in instructions (L027)
- The `/metrics` no-auth-in-v0 caveat — must explicitly say "put behind a private network or reverse proxy"
- The distroless nonroot UID note — explain why volume mounts may need `chown 65532:65532` (L028)

- [ ] **Step 3: Spot-check `docs/license.md` and `docs/adapters.md`**

Confirm both are operator-readable and reference correct file paths from Phase 7's actual artifact set.

- [ ] **Step 4: Save the walkthrough note to scratch file (Task 11)**

If any doc command does NOT work or references a missing file, STOP and fix the doc in `docs/` (small fix → bundle into the `docs(7-v):` commit in Task 11; large fix → return to 7-iv).

---

### Task 8: Smoke item 7 — Phase 1-6 regression

**Files:** none new (re-runs existing smoke drivers + unit tests + scripts)

- [ ] **Step 1: drive-attachments.ts (Phase 6) under the Phase 7 chain**

The Phase 7 middleware chain prepends `metrics.Middleware()` to the chi route. For drive-attachments.ts to pass unchanged, the metrics middleware must be a true passthrough when disabled and observation-only when enabled (no behavior change).

```bash
cd c:/src/ai/intake
# Start fake-llm + relay (with Phase 6 attachments + Phase 7 metrics middleware in chain)
go run ./relay/cmd/fake-llm --addr :11434 &
FAKE_PID=$!
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/attachments-enabled.yaml &
RELAY_PID=$!
sleep 1

RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts enabled 2>&1 | tee /tmp/regression-attachments-enabled.txt
RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts disabled 2>&1 | tee /tmp/regression-attachments-disabled.txt

kill $RELAY_PID $FAKE_PID
```

Expected: both arms pass identically to Phase 6's recorded evidence (`ai/tasks/phase-6/README.md` §7 Smoke status block). Every `OK:` line matches; final `✓ All Phase 6 attachment smokes passed for this arm.` Proves Phase 7's middleware chain prepend did NOT regress Phase 6's attachment validation.

- [ ] **Step 2: drive-abuse.ts (Phase 5) under the Phase 7 chain**

```bash
go run ./relay/cmd/fake-llm --addr :11434 &
FAKE_PID=$!
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/abuse-driver.yaml &
RELAY_PID=$!
sleep 1

RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-abuse.ts 2>&1 | tee /tmp/regression-abuse.txt

kill $RELAY_PID $FAKE_PID
```

Expected: all three rate-limit gates (per-IP burst → 429 + Retry-After:1, per-session → 429 `session_turns_exhausted`, daily-budget → 503 `daily_budget_exhausted`) fire identically to Phase 5's recorded evidence. The `(150,150)` budget fixture (L019) behaves identically. Proves Phase 7's metrics middleware did NOT regress Phase 5's rate-limit chain.

- [ ] **Step 3: Phase 4 deferral note**

Phase 7 makes ZERO changes to the auth dispatcher, bearer paths, or `SessionContext` (frozen seams per README §8.1). The Phase 4 drivers (`drive-auth-email.ts` requires MailHog; `drive-auth-sso.ts` requires SSO IDP infrastructure) are deferred to maintainer-run smokes — same handling as Phase 6's recorded "Phase 4 drivers — DEFERRED" note. Phase 5's abuse driver exercises the full middleware chain under both anonymous and (transitively) bearer auth modes, providing transitive coverage. Record this honestly in the README §7.1 evidence:

> **Phase 4 drivers — DEFERRED.** `drive-auth-email.ts` requires MailHog and `drive-auth-sso.ts` requires SSO IDP infra; same deferral as Phase 6. Phase 7 introduces zero changes to auth dispatcher / bearer paths / SessionContext (frozen seams per Phase 7 README §8.1). Phase 5 abuse driver provides transitive coverage of the full middleware chain.

- [ ] **Step 4: Go test suite under -race**

```bash
cd c:/src/ai/intake/relay
go test -race ./... 2>&1 | tee /tmp/regression-go-test.txt
```

Expected: every Phase 1-6 unit test passes; race detector clean. Includes:
- chatwoot/fider/linear/zendesk/webhook adapter tests
- attachvalidate tests
- emailcode/SSO/auth dispatcher tests
- budget/captcha/ratelimit tests
- license/manager/state-file tests
- Phase 7's NEW metrics package tests (added in 7-i)
- Phase 7's NEW `accumulateStartupProblems` unit tests (added in 7-i / closes I2)

- [ ] **Step 5: Core + Vue tests**

```bash
cd c:/src/ai/intake/core && npm run type-check && npm run test 2>&1 | tee /tmp/regression-core-tests.txt
cd c:/src/ai/intake/vue && npm run type-check && npm run build && npm run test 2>&1 | tee /tmp/regression-vue-tests.txt
```

Expected: exit 0 on each. (Core has no build script — that's by design.)

- [ ] **Step 6: Scripts + go mod tidy + frozen-seam diff**

```bash
cd c:/src/ai/intake
bash scripts/verify-contract.sh 2>&1 | tee /tmp/regression-scripts.txt
bash scripts/check-pins.sh 2>&1 | tee -a /tmp/regression-scripts.txt

cd relay && go mod tidy && cd ..
git diff --stat relay/go.mod relay/go.sum | tee -a /tmp/regression-scripts.txt

# Frozen-seam diff (per Phase 7 README §8.1):
git diff main..phase-7 -- \
  relay/internal/adapter/adapter.go \
  relay/internal/payload/types.go \
  schema/payload.v1.json | tee /tmp/regression-frozen-seams.txt
```

Expected:
- `verify-contract.sh` exit 0 (no schema changes in Phase 7)
- `check-pins.sh` exit 0 (new pin checks from 7-ii — `prometheus/client_golang`, `goreleaser`, `golangci-lint`, `eslint`, `prettier`)
- `go mod tidy` empty diff (only new module is `prometheus/client_golang`, added in 7-i and stable since)
- Frozen-seam diff EMPTY — proves Phase 7 touched none of the seams Phase 0-6 froze.

- [ ] **Step 7: Record all outputs to scratch file (Task 11)**

---

### Task 9: Smoke item 8 — lint smoke

**Files:** none new (uses 7-i's lint configs)

- [ ] **Step 1: golangci-lint**

```bash
cd c:/src/ai/intake/relay
golangci-lint run ./... 2>&1 | tee /tmp/lint-go.txt
echo "exit code: $?"
```

Expected: exit 0, zero findings. The initial-fix sweep landed in 7-i; this is the CI-gate proof.

- [ ] **Step 2: eslint**

```bash
cd c:/src/ai/intake
npx eslint . 2>&1 | tee /tmp/lint-eslint.txt
echo "exit code: $?"
```

Expected: exit 0, zero errors. (Warnings tolerated only if the 7-i config explicitly allows them; the README §6 build-fail item is `eslint . reports any error → Fail`.)

- [ ] **Step 3: prettier**

```bash
cd c:/src/ai/intake
npx prettier --check . 2>&1 | tee /tmp/lint-prettier.txt
echo "exit code: $?"
```

Expected: exit 0, zero unformatted files.

- [ ] **Step 4: goreleaser check + actionlint (if adopted in 7-ii)**

```bash
cd c:/src/ai/intake
goreleaser check relay/.goreleaser.yaml 2>&1 | tee /tmp/lint-goreleaser.txt
# If 7-ii adopted actionlint:
actionlint .github/workflows/*.yml 2>&1 | tee /tmp/lint-actionlint.txt || true
```

Expected: `goreleaser check` exit 0; `actionlint` (if adopted) exit 0.

- [ ] **Step 5: Verify the 3 linters are integrated as CI jobs**

```bash
grep -E 'golangci-lint|eslint|prettier' .github/workflows/ci.yml | tee -a /tmp/lint-ci-integration.txt
```

Expected: all three commands appear as steps in `ci.yml` (added in 7-i after the initial-fix sweep).

- [ ] **Step 6: Record outputs to scratch file (Task 11)**

---

### Task 10: Append L024-L028 to `ai/LESSONS.md`

**Files:** Modify `ai/LESSONS.md`

- [ ] **Step 1: Append five new lessons after L023**

Append these entries verbatim after the existing L023 (the last existing lesson):

```markdown
### L024: Snapshot-then-publish split — every release-artifact tool MUST have a --snapshot/--dry-run mode exercised in CI; the actual publish is a deliberate, separately-gated action

Phase 7 ships `goreleaser`, `npm publish`, and `docker build` as the v0 release pipeline. The temptation when wiring CI is to have every PR push run the full release path "for free" — `goreleaser release` instead of `goreleaser release --snapshot`, `npm publish --tag dev` instead of `npm publish --dry-run`, `docker push` instead of `docker build`. The result: any merge to `main` ships an artifact to a public registry. Once shipped, you cannot unship. The yanked-version smell, the squatted-name confusion, the ghcr.io pull-count showing artifacts that never should have been public — all of those are recoverable in principle but expensive in trust and operator time.

**Where it hit:** Phase 7 scope-boundary decision. The original draft included "run `goreleaser release` in CI on tag push" — which would have shipped the v0.0.0-snapshot artifacts to the public ghcr.io / npm registry the moment Phase 7 merged, before the maintainer locked Q1 (final product name + remote + ghcr/npm tokens). The fix was the snapshot-then-publish split: Phase 7 ships `goreleaser release --snapshot --clean`, `npm publish --dry-run`, `docker build` (no push). The actual public release is a separate, maintainer-driven Phase 7.5+ action gated on Q1 + remote + tokens. CI exercises the SAME workflows against the SAME configs in dry-run mode every PR — so the moment the maintainer flips the gate, the publish path is already proven against real configs.

**Rule:** Every release-artifact tool you wire into CI MUST have a `--snapshot` / `--dry-run` / "build-but-don't-push" mode, and CI MUST run that mode on every PR. The actual publish is a deliberate, separately-gated action — guarded by either (a) a manual `workflow_dispatch` trigger with explicit confirmation, (b) a tag push under a strict naming convention (`v[0-9]+.[0-9]+.[0-9]+` only, not `v*`), or (c) a separate "publish-approved" repo environment with required reviewers. Snapshot mode catches the same config / file-list / archive-name regressions as the real publish; dry-run mode catches the same package.json / tarball-contents regressions. The publish path is then a one-line `--snapshot` removal, not a "rewrite the workflow" exercise. Add a build-fail item to every phase that touches the release pipeline: "release-artifact tool runs in CI without `--snapshot`/`--dry-run` → Fail."

Reference: `relay/.goreleaser.yaml` (snapshot config block); `.github/workflows/ci.yml` (PR job runs `goreleaser release --snapshot --clean` + `npm publish --dry-run`); `.github/workflows/release.yml` (tag-push job runs the real `goreleaser release`; authored in 7-ii but never executed in Phase 7).

---

### L025: Initial lint sweep before CI gate — never enable a lint as a CI gate against existing code without first running the sweep + triaging every finding

The natural-but-wrong way to adopt a new linter: add the `golangci-lint`/`eslint`/`prettier` step to `ci.yml`, commit, push. The CI run reports 47 findings on existing code. The PR is blocked. Every other open PR is now also blocked because the same lint job runs on every PR. The lint becomes a Day-1 barrier to all future work — anyone who needs to land a fix first has to fix 47 unrelated findings (some real, some false positives, some style preferences that the team hasn't agreed on). The lint is now "the thing that always fails" instead of "the thing that catches bugs."

**Where it hit:** Phase 7 initial-fix sweep design. Three new lints landed in one phase (golangci-lint + eslint + prettier). Without the sweep-first discipline, every Phase 8+ PR would have been blocked on the cumulative N findings across all three. The fix was the explicit Phase 7-i initial-fix sweep task: run each linter locally, triage every finding (real bug → fix with a commit; false positive → narrow `//nolint` / `eslint-disable` with a comment naming the reason; style preference → narrow the rule), land fixes BEFORE wiring the lint job into the CI gate. The CI gate then starts from a clean baseline.

**Rule:** Adopting a new lint as a CI gate is a TWO-step process: (1) the initial-fix sweep — run the linter, triage every finding, land the fixes in a "sweep" commit (or commit series) so the working tree is clean against the chosen rule set; (2) the gate wiring — add the lint step to `ci.yml` ONLY after the sweep lands. Never skip (1). The triage is the load-bearing work — every finding is either "fix the code," "narrow the rule to exclude this pattern," or "suppress this line with a reason" — and each decision is reviewable. A bulk `--fix --safe` run is acceptable for trivial mechanical fixes (whitespace, import order) but ANY non-trivial fix gets a dedicated triage decision. Curated rulesets (NOT `--enable-all`) — the rule list is part of the gate decision; tightening later is fine, loosening later carries a regression risk. Add a build-fail item: "lint introduced as CI gate without a recorded initial-fix sweep → Fail."

Reference: `relay/.golangci.yaml`, `.eslintrc.cjs`, `.prettierrc` (the curated rulesets); 7-i sweep commits (triage-by-finding); `.github/workflows/ci.yml` `lint-go` / `lint-ts` / `lint-format` jobs (gate wired after the sweep).

---

### L026: Metrics server lives independently from main HTTP — observability shouldn't be able to brick the service it observes

The natural-but-wrong design for an in-process Prometheus endpoint: register `/metrics` on the same `*http.Server` that serves the application's API endpoints. Simple, one less goroutine, one less port to document. The cost: a metrics-port conflict, a metrics-handler panic, a metrics-middleware deadlock, or even just an OOM in the metrics collection path — any of those takes down the API endpoints too. The thing whose job is to observe failures becomes a source of failures. Observability has become a single point of failure for the service it observes.

**Where it hit:** Phase 7 metrics package design. The first draft had `/metrics` registered on the main chi router. Code review surfaced the dependency: an operator with a misconfigured reverse proxy that hammers `/metrics` at high QPS could starve the API endpoints of goroutines. The fix was structural separation: `metrics.Registry.ListenAndServe(ctx)` starts a SEPARATE `*http.Server` on `cfg.Observability.Metrics.Addr` (default `:9090`). A port-bind failure on the metrics server is logged at Error level but does NOT propagate — `main()` swallows the error and the main relay continues. The `Middleware()` function is the only point of contact with the main server, and it's a literal passthrough when `Enabled=false`.

**Rule:** Observability surfaces (Prometheus metrics, OpenTelemetry traces, pprof handlers, health-debug endpoints) MUST live on a SEPARATE HTTP listener from the main application API. A failure on the observability listener (port conflict, panic, deadlock, OOM in collection) MUST be logged but MUST NOT propagate to the application listener. The integration point between the two (the metrics middleware, the OTLP exporter, the tracing instrumentation) MUST be a no-op passthrough when the observability subsystem is disabled — no conditional plumbing in the application path. Add a build-fail item: "metrics-port conflict causes main HTTP to fail to start → Fail." A unit test forces the metrics port to a known-bound socket and asserts the main relay still serves `/v1/health`.

Reference: `relay/internal/metrics/registry.go` `ListenAndServe` (separate `*http.Server`); `relay/cmd/relay/main.go` (goroutine swallows ListenAndServe error, logs at Error); tests `TestMetrics_PortBindFailure_MainRelayContinues`.

---

### L027: Off-by-default for new observability surface — every new operator-facing thing defaults to off; operators opt in

The natural-but-wrong default for a new feature flag: `Enabled: true`. Reasoning goes: "the feature is good, the default should be the good thing." For observability surface, the natural-but-wrong default is doubly wrong. (a) Operators who haven't read the docs yet now have an unauthenticated `/metrics` endpoint exposed on a port they didn't know was open — a network-recon vector if their firewall isn't tight. (b) Operators who DO want metrics now have no way to confirm "this is the operator's choice, not the package's default" — every existing deployment has metrics on with no operator action. (c) The first time something breaks in the metrics path, every operator is affected; if metrics were off-by-default, only operators who opted in are affected.

**Where it hit:** Phase 7 `MetricsConfig.Enabled` default. The first draft had `default true` ("metrics are good, who would turn them off?"). Code review surfaced the security + scope arguments above. The fix was `Enabled: false` default. Operators explicitly set `observability.metrics.enabled: true` (and optionally `observability.metrics.addr: ":9090"`) to opt in. The `docs/self-hosting.md` page (7-iv) makes the opt-in explicit + describes the no-auth-in-v0 caveat ("put behind a private network or a reverse proxy").

**Rule:** Every new operator-facing flag that EXPOSES SOMETHING (an endpoint, a header, a log field, a telemetry surface) defaults to `false` / "off" / `none`. The flag in the YAML schema is documented inline with the security implication ("This exposes an unauthenticated HTTP endpoint; place it behind a private network or reverse proxy"). The opt-in is a single line of operator YAML. New operator-facing flags that CONFIGURE existing behavior (timeouts, retry counts, log levels) may default to whatever the safe-default is — but exposure flags are off. Generalizes to: trace sampling rate defaults to 0, debug pprof handler defaults to disabled, verbose log mode defaults to disabled, the WebSocket-debug-shim defaults to disabled. Add a build-fail item: "new observability flag ships with `Enabled: true` default → Fail" (or, more precisely, "new EXPOSURE flag ships defaulted-on → Fail").

Reference: `relay/internal/config/config.go` `MetricsConfig.Enabled` (default `false`); `relay/internal/config/defaults.go` applyDefaults (no override); `docs/self-hosting.md` § Metrics (the opt-in section + the no-auth caveat).

---

### L028: Distroless multi-stage Docker template — minimal CVE surface, nonroot user, no shell, no package manager

The natural-but-wrong Dockerfile for a Go binary: a single-stage `FROM golang:1.23.2-alpine`, `COPY . .`, `RUN go build`, `ENTRYPOINT ["./relay"]`. The image is ~400 MB (the entire Go toolchain + alpine package manager + shell + libc); the running container has a shell so any RCE drops into an interactive shell; the running container has a package manager so any successful RCE can install arbitrary binaries; the running container runs as root unless you explicitly downgrade. Every one of those is a CVE-amplification vector — the same RCE is a self-contained binary execution on distroless and a compromise-the-image on alpine.

**Where it hit:** Phase 7 `relay/Dockerfile` design. The first draft was the single-stage alpine pattern above. The decomposition Q10 + design spec §15 + the distroless / static-debian12:nonroot guidance combined into the canonical pattern: stage 1 `golang:1.23.2-alpine` builds the static binary; stage 2 `gcr.io/distroless/static-debian12:nonroot` runs it. Image total < 50 MB (enforced via the build-fail invariant). No shell, no package manager. Runs as the distroless `nonroot` user (UID 65532). The `docker exec intake-relay id -u` assertion in `drive-docker-compose.ts` pins the nonroot invariant — if a future refactor accidentally `USER root`s the image, the smoke fails.

**Rule:** The distroless multi-stage Docker pattern is the TEMPLATE for any future Go binary in this monorepo. Stage 1: `golang:<EXACT-PIN>-alpine` (or `golang:<EXACT-PIN>` if CGO is needed; see the spec note). Stage 2: `gcr.io/distroless/static-debian12:nonroot` (for pure-Go static binaries) or `gcr.io/distroless/base-debian12:nonroot` (for binaries that need libc). `USER nonroot` is implicit in the `:nonroot` tag (UID 65532) — never override it without a recorded reason. `ENTRYPOINT` uses the exec form (`["./binary"]`), not the shell form. Final image size < 50 MB for static-debian12, < 100 MB for base-debian12 (build-fail invariants). A smoke MUST assert the nonroot UID via `docker exec <container> id -u` — without that assertion, a future `USER root` change ships silently. Generalizes to: every public-facing container runs as a non-root, non-zero UID; every container has a smoke that proves it.

Reference: `relay/Dockerfile` (multi-stage, distroless target, nonroot user); `relay/.goreleaser.yaml` `dockers:` block (same image for goreleaser-built releases); `core/smoke/drive-docker-compose.ts` `assertDistrolessNonrootUID()` (load-bearing UID smoke); Phase 7 README §6 build-fail items "image size > 50 MB → Fail" + "running user is root or empty → Fail".

---
```

- [ ] **Step 2: Commit**

```bash
git add ai/LESSONS.md
git commit -m "$(cat <<'EOF'
docs(7-v): LESSONS L024-L028 — snapshot-then-publish split, initial lint sweep, metrics independence, off-by-default observability, distroless template

Five new lessons capturing Phase 7's durable patterns:
- L024: snapshot/dry-run modes in CI; publish is a separately-gated action
- L025: initial lint sweep before CI gate (never gate against existing code)
- L026: observability lives on a separate HTTP listener; can't brick its host
- L027: new exposure flags default to off; operators opt in
- L028: distroless multi-stage Docker template for any future Go binary

Each entry follows the L016-L023 format: Where it hit + Rule + Reference.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: Phase 7 README evidence + sub-plan status updates

**Files:** Modify `ai/tasks/phase-7/README.md`

- [ ] **Step 1: Flip §3 sub-plan index status column**

In §3, change the `Status` column for all 5 sub-plans from `Not started` to `Live + smoked`. Expected final table:

```markdown
| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 7-i | [Relay code: FOLLOWUPS + Prometheus metrics + lint configs + initial-fix sweep + CI extension](7-i-relay-code-followups-metrics-lint-plan.md) | the seam | L | Live + smoked |
| 7-ii | [Release artifacts: Dockerfile + .goreleaser.yaml + release.yml + goreleaser pin](7-ii-release-artifacts-plan.md) | release config | M | Live + smoked |
| 7-iii | [Demo stack: examples/docker-compose/ (3 services, builds from 7-ii Dockerfile)](7-iii-docker-compose-demo-plan.md) | demo | M | Live + smoked |
| 7-iv | [Docs + governance: 4 docs + repo README rewrite + LICENSE + CONTRIBUTING + SECURITY + COMMERCIAL](7-iv-docs-governance-plan.md) | docs + governance | M | Live + smoked |
| 7-v | [Final smoke + drive-docker-compose.ts + LESSONS L024-L028 + README evidence + FOLLOWUPS rename](7-v-smoke-docs-plan.md) | live evidence | S | Live + smoked |
```

- [ ] **Step 2: Append §7.1 "Smoke status (YYYY-MM-DD)" subsection**

After the existing §7 block (the 8-item smoke recipe), add a `### 7.1 Smoke status (YYYY-MM-DD)` subsection mirroring Phase 5/6 README §7 evidence format. Use the captured outputs from `/tmp/q9-combined-smoke.txt`, `/tmp/metrics-smoke.txt`, `/tmp/goreleaser-snapshot.txt`, `/tmp/npm-pack-smoke.txt`, `/tmp/docker-compose-smoke.txt`, the docs-walkthrough note, `/tmp/regression-*.txt`, and `/tmp/lint-*.txt`. One fenced code block per smoke item with a one-line verdict above it.

Template (fill in the actual date + captured outputs):

```markdown
### 7.1 Smoke status (YYYY-MM-DD)

All 8 Phase 7 final-smoke items pass. Every artifact is self-runnable; no maintainer-paused live smokes (per the scope-boundary decision). Phase 7 makes no auth dispatcher / bearer / SessionContext changes — Phase 4 drivers transitively covered by the Phase 5 abuse driver under the Phase 7 chain.

#### 1. Q9 combined-misconfig smoke

Verdict: PASS — combined fixture (Phase 5 + Phase 6 + Phase 7 misconfigs) emits ONE consolidated `relay: startup config errors` log line with count >= 6; exit 1. Closes FOLLOWUPS I1 (buildRegistry contributes problems instead of exiting) and I2 (accumulateStartupProblems is unit-tested).

```
$ go run ./relay/cmd/relay --config relay/cmd/relay/smoke/q9-combined-misconfig.yaml
{"level":"ERROR","msg":"relay: startup config errors","count":6,
 "problems":["auth.modes.anonymous=true requires captcha.enabled=true ...",
             "server.trusted_proxies contains an invalid CIDR \"not-a-cidr\": ...",
             "ratelimit.daily_llm_budget.action_on_exceeded=\"queue\" is not supported in v0 ...",
             "adapter chatwoot api_token_env=\"NONEXISTENT_VAR\" is not set in the environment ...",
             "attachments.storage.mode=\"s3\" is not supported in v0 ...",
             "attachments.max_size_bytes=20000000 exceeds attachments.max_total_bytes=10000000 ..."]}
exit status 1
```

#### 2. Metrics endpoint smoke

Verdict: PASS — disabled arm refuses connection on :9090; enabled arm exports all 4 series; init counter increments by exactly 5 after 5 /init requests.

```
$ # 2a (disabled): curl -sS -m 2 http://localhost:9090/metrics
curl: (7) Failed to connect to localhost port 9090: Connection refused

$ # 2a (main relay still healthy):
$ curl -sS http://localhost:18080/v1/health
{"status":"ok"}

$ # 2b (enabled): curl -sS http://localhost:9090/metrics | grep '# HELP intake_'
# HELP intake_http_requests_total Total HTTP requests served by the relay
# HELP intake_http_request_duration_seconds HTTP request duration in seconds
# HELP intake_llm_tokens_total Total LLM tokens by provider and direction
# HELP intake_adapter_calls_total Total adapter calls by adapter and result

$ # After 5 /init requests:
intake_http_requests_total{path="/v1/intake/init",status="200"} 5
```

#### 3. Snapshot release smoke

Verdict: PASS — `goreleaser release --snapshot --clean` produces all 5 archives + SHA256SUMS + the local docker image; binary --version printout matches; image < 50 MB (distroless invariant).

```
$ goreleaser release --snapshot --clean --config relay/.goreleaser.yaml
... (paste tail of goreleaser output)

$ ls dist/
intake-relay_v0.0.0-snapshot_darwin_amd64.tar.gz
intake-relay_v0.0.0-snapshot_darwin_arm64.tar.gz
intake-relay_v0.0.0-snapshot_linux_amd64.tar.gz
intake-relay_v0.0.0-snapshot_linux_arm64.tar.gz
intake-relay_v0.0.0-snapshot_windows_amd64.zip
SHA256SUMS.txt
... (other generated files)

$ docker images intake-relay --format '{{.Size}}'
42.3MB

$ /tmp/goreleaser-extract/linux_amd64/intake-relay --version
intake-relay v0.0.0-snapshot (commit <sha>) built <timestamp>
```

#### 4. npm pack dry-run smoke

Verdict: PASS — both `@intake/core` and `@intake/vue` produce valid tarballs; both `npm publish --dry-run` exit 0; no `.env` / `local-dev/` / secrets/credentials in either tarball.

```
$ npm pack -w @intake/core
intake-core-0.0.0.tgz
$ npm publish -w @intake/core --dry-run
... files: ...
+ @intake/core@0.0.0

$ tar -tzf intake-core-0.0.0.tgz | grep -E '\.env$|local-dev/|secrets|credentials' || echo "no secret-leak suspect"
no secret-leak suspect

$ # Same for @intake/vue. Required package.json fields (description, repository, license) present in both.
```

#### 5. docker-compose demo smoke (drive-docker-compose.ts)

Verdict: PASS — fake-llm + relay + webhook-receiver boot; /init returns capabilities with attachments; /turn SSE stream completes; /submit returns external_id with adapter_name="webhook"; canonical payload landed in webhook-receiver verbatim; metrics endpoint exports all 4 series + non-zero init counter; relay runs as distroless nonroot UID 65532; down -v leaves no containers / volumes.

```
$ npx tsx core/smoke/drive-docker-compose.ts
docker-compose smoke: COMPOSE_DIR=examples/docker-compose RELAY_URL=http://localhost:18080 METRICS_URL=http://localhost:19090

=== docker-compose up -d ===
Creating intake-fake-llm ... done
Creating intake-webhook-receiver ... done
Creating intake-relay ... done

=== waiting for http://localhost:18080/v1/health ===
OK: /v1/health 200 after ~1700ms

=== POST /v1/intake/init ===
OK: /init returns 200
OK: init returns non-empty session_id
OK: init returns capabilities.attachments

=== POST /v1/intake/turn (SSE) ===
OK: /turn returns 200
OK: /turn body stream is non-null
OK: /turn SSE stream emitted a terminal done event

=== POST /v1/intake/submit ===
OK: /submit returns 200
OK: /submit returns external_id
OK: /submit adapter_name is "webhook"

=== assert canonical payload landed in webhook-receiver ===
OK: docker-compose logs webhook-receiver exit 0
OK: webhook-receiver log contains the user message text
OK: webhook-receiver log contains the client section
OK: webhook-receiver log contains the messages section

=== curl http://localhost:19090/metrics ===
OK: /metrics returns 200
OK: /metrics Content-Type is text/plain
OK: /metrics declares # HELP intake_http_requests_total
OK: /metrics declares # HELP intake_http_request_duration_seconds
OK: /metrics declares # HELP intake_llm_tokens_total
OK: /metrics declares # HELP intake_adapter_calls_total
OK: /metrics has intake_http_requests_total{path="/v1/intake/init",status="200"} sample
OK: init counter >= 1 (got 1)

=== docker exec intake-relay id -u ===
OK: docker exec id -u exit 0
OK: relay runs as distroless nonroot UID 65532

✓ All Phase 7 docker-compose smoke assertions passed.

=== docker-compose down -v ===
Stopping intake-relay ... done
Stopping intake-webhook-receiver ... done
Stopping intake-fake-llm ... done
Removing ... volumes ... done
```

#### 6. Docs walkthrough smoke

Verdict: PASS — `docs/quickstart.md` + `docs/self-hosting.md` walkthrough (self-review pass against the actual repo state) — every command verbatim, every relative link resolves, end-state matches `drive-docker-compose.ts` assertions.

- quickstart.md: every command exists in env; docker-compose path identical to driver behavior; relative links to ./self-hosting.md, ./adapters.md, ../README.md all resolve.
- self-hosting.md: metrics opt-in section explicit + the no-auth-in-v0 caveat present (L027); distroless nonroot UID note + chown guidance present (L028).
- license.md + adapters.md: spot-checked operator-readable; reference correct repo paths.

#### 7. Phase 1-6 regression

Verdict: PASS (with documented Phase 4 deferral) — drive-attachments.ts + drive-abuse.ts complete unchanged under the Phase 7 chain; go test -race ./... green; core/vue tests green; scripts green; go mod tidy no-op; frozen-seam diff empty.

```
$ RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts enabled
... OK lines ...
✓ All Phase 6 attachment smokes passed for this arm.

$ RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts disabled
... OK lines ...
✓ All Phase 6 attachment smokes passed for this arm.

$ RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-abuse.ts
... OK lines ...
✓ All Phase 5 abuse smokes passed.

$ cd relay && go test -race ./...
ok      intake/internal/adapter/chatwoot   0.412s
ok      intake/internal/adapter/fider      0.317s
ok      intake/internal/adapter/linear     0.523s
ok      intake/internal/adapter/zendesk    0.461s
ok      intake/internal/adapter/webhook    0.198s
ok      intake/internal/attachvalidate     0.245s
ok      intake/internal/auth               0.612s
ok      intake/internal/auth/emailcode     0.388s
ok      intake/internal/auth/sso           0.401s
ok      intake/internal/budget             0.299s
ok      intake/internal/captcha            0.276s
ok      intake/internal/config             0.115s
ok      intake/internal/license            0.198s
ok      intake/internal/metrics            0.213s   <-- NEW (Phase 7-i)
ok      intake/internal/ratelimit          0.244s
ok      intake/internal/server             0.587s

$ cd core && npm run type-check && npm run test
... type-check and test exit 0 ...

$ cd vue && npm run type-check && npm run build && npm run test
... all three exit 0 ...

$ bash scripts/verify-contract.sh
... exit 0 (no schema changes in Phase 7) ...

$ bash scripts/check-pins.sh
... exit 0 (new pins: prometheus/client_golang, goreleaser, golangci-lint, eslint, prettier) ...

$ cd relay && go mod tidy && git diff --stat go.mod go.sum
(empty — Phase 7 added prometheus/client_golang in 7-i; stable since)

$ git diff main..phase-7 -- relay/internal/adapter/adapter.go relay/internal/payload/types.go schema/payload.v1.json
(empty — frozen Phase 0-6 seams unchanged)
```

**Phase 4 drivers — DEFERRED.** `drive-auth-email.ts` requires MailHog and `drive-auth-sso.ts` requires SSO IDP infrastructure (same deferral pattern as Phase 6). Phase 7 introduces zero changes to the auth dispatcher / bearer paths / `SessionContext` (frozen Phase 4 seams per README §8.1). Phase 5's abuse driver exercises the full middleware chain through `/turn`, providing transitive coverage of the chain Phase 4 drivers would exercise.

#### 8. Lint smoke

Verdict: PASS — golangci-lint + eslint + prettier all exit 0 with zero findings; all three integrated as CI jobs in `.github/workflows/ci.yml`; goreleaser check exit 0.

```
$ cd relay && golangci-lint run ./...
(no output — 0 issues)

$ npx eslint .
(no output — 0 errors)

$ npx prettier --check .
Checking formatting...
All matched files use Prettier code style!

$ goreleaser check relay/.goreleaser.yaml
✓ config is valid
```

CI integration confirmed: `.github/workflows/ci.yml` has `lint-go` (golangci-lint), `lint-ts` (eslint), `lint-format` (prettier --check) jobs that pass on the merge commit.

---

**Phase 7 coverage: 5/5 sub-plans proven** — relay code + FOLLOWUPS I1/I2/M2/M4 closure + Prometheus metrics + lint configs (7-i, smoked), release artifacts: Dockerfile + .goreleaser.yaml + release.yml (7-ii, smoked via `goreleaser release --snapshot --clean` + `docker build`), demo stack (7-iii, smoked via `drive-docker-compose.ts`), docs + governance (7-iv, self-review walkthrough), final smoke + LESSONS + evidence (7-v, this section). Frozen Phase 0-6 seams unchanged.

Closes Phase 6 FOLLOWUPS I1+I2+M2+M4 (see `ai/tasks/phase-6/FOLLOWUPS-resolved.md`). LESSONS L024-L028 appended to `ai/LESSONS.md`.

Phase 7 is OUT OF SCOPE for public publish — no `docker push`, no `npm publish`, no `gh release create`, no git tag push. The public release is a separate maintainer-driven Phase 7.5+ action gated on Q1 final product name + GitHub remote + ghcr/npm tokens.
```

- [ ] **Step 3: Commit**

```bash
git add ai/tasks/phase-7/README.md
git commit -m "$(cat <<'EOF'
docs(phase-7): live smoke evidence + sub-plan status updates; phase 7 done

§3 sub-plan index: all 5 sub-plans flipped to "Live + smoked".
§7.1 "Smoke status" subsection populated with full 8-item evidence:
- Q9 combined-misconfig (count>=6, one log line)
- metrics endpoint disabled/enabled (4 series, +5 init counter)
- snapshot release (5 archives + SHA256SUMS + <50MB image)
- npm pack dry-run (no secret leak)
- docker-compose demo (drive-docker-compose.ts all OK)
- docs walkthrough (self-review pass)
- Phase 1-6 regression (drive-attachments + drive-abuse + go test + scripts)
- lint smoke (golangci-lint + eslint + prettier all 0 findings)

Phase 4 drivers deferred with same handling as Phase 6 — Phase 7 makes no
auth changes; Phase 5 abuse driver provides transitive coverage.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: Rename Phase 6 FOLLOWUPS.md → FOLLOWUPS-resolved.md

**Files:** Rename `ai/tasks/phase-6/FOLLOWUPS.md` → `ai/tasks/phase-6/FOLLOWUPS-resolved.md`; prepend banner; update cross-references.

- [ ] **Step 1: Rename via git**

```bash
cd c:/src/ai/intake
git mv ai/tasks/phase-6/FOLLOWUPS.md ai/tasks/phase-6/FOLLOWUPS-resolved.md
```

- [ ] **Step 2: Prepend the "resolved by Phase 7-i" header banner**

Edit `ai/tasks/phase-6/FOLLOWUPS-resolved.md` to prepend (before the existing first line) this banner:

```markdown
> **RESOLVED in Phase 7-i.** All four follow-ups (I1, I2, M2, M4) were folded into Phase 7-i (`ai/tasks/phase-7/7-i-relay-code-followups-metrics-lint-plan.md`) and shipped with the Phase 7 merge. This file is preserved for audit trail. See `ai/tasks/phase-7/README.md` §7.1 evidence for the Q9 combined-misconfig smoke that proves the closures end-to-end.
>
> - **I1** (buildRegistry as third startup gate) — closed: `buildRegistry` now returns `([]adapter.Adapter, []string)` and contributes per-adapter Configure failures + "no adapters enabled" to the shared problems slice. The combined Q9 fixture includes a chatwoot `api_token_env: NONEXISTENT_VAR` misconfig that proves the contribution path.
> - **I2** (cross-phase wiring not unit-testable) — closed: extracted `accumulateStartupProblems(cfg, licState, logger) (Deps, []string)` from `main()`; unit-tested directly. Shell smoke (`run-q9-smoke.sh`) is now belt-and-braces, not load-bearing.
> - **M2** (validateAttachments short-circuit) — closed: returns zero-value `config.AttachmentsConfig{}` when `!parsed.Enabled`.
> - **M4** (run-q9-smoke.sh working-directory dance) — closed: replaced with `go run -C relay ./cmd/relay ...` calls; no more `cd relay && cd ..` shell sequences.
>
> ---
```

(Keep the rest of the file intact.)

- [ ] **Step 3: Update cross-references**

Update any references to `FOLLOWUPS.md` in the repo to point at `FOLLOWUPS-resolved.md`:

```bash
cd c:/src/ai/intake
grep -rn "FOLLOWUPS.md" ai/ docs/ || echo "no remaining references"
# Likely candidates:
#   - ai/tasks/phase-7/README.md §1 ("Closes: ai/tasks/phase-6/FOLLOWUPS.md (I1, I2, M2, M4 ...)")
#   - ai/tasks/phase-6/README.md (if it links to the FOLLOWUPS file)
```

Specifically, edit Phase 7 README §1:

OLD:
```markdown
- Closes: [ai/tasks/phase-6/FOLLOWUPS.md](../phase-6/FOLLOWUPS.md) (I1, I2, M2, M4 all folded into Phase 7-i; renamed to `FOLLOWUPS-resolved.md` after 7-i)
```

NEW:
```markdown
- Closes: [ai/tasks/phase-6/FOLLOWUPS-resolved.md](../phase-6/FOLLOWUPS-resolved.md) (I1, I2, M2, M4 all folded into Phase 7-i; renamed from `FOLLOWUPS.md` in Phase 7-v)
```

If `ai/tasks/phase-6/README.md` references `FOLLOWUPS.md`, update there too. If `docs/PROJECT.md` or any other file under `docs/` references the old name, update there.

- [ ] **Step 4: Verify**

```bash
grep -rn "FOLLOWUPS\.md" ai/ docs/ | grep -v "FOLLOWUPS-resolved\.md"
# Expected: no output (every remaining reference is to the new name)

grep -rn "FOLLOWUPS-resolved\.md" ai/ docs/
# Expected: at least the Phase 7 README §1 reference, plus the file itself
```

- [ ] **Step 5: Commit**

```bash
git add ai/tasks/phase-6/FOLLOWUPS-resolved.md ai/tasks/phase-7/README.md ai/tasks/phase-6/README.md
git commit -m "$(cat <<'EOF'
chore(7-v): rename Phase 6 FOLLOWUPS.md → FOLLOWUPS-resolved.md

All four follow-ups (I1, I2, M2, M4) were folded into Phase 7-i and shipped
with the Phase 7 merge. Prepend a "resolved by Phase 7-i" header banner that
documents each closure, preserving the original body for audit trail.
Update cross-references in Phase 7 README §1 (and any other links in
ai/ or docs/) to point at the new filename.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: (Optional) PROJECT.md §14 + §15 inconsistency fixes

**Files:** Modify `docs/PROJECT.md`

Phase 7 design spec §15 flagged two doc-only inconsistencies in PROJECT.md. They are not load-bearing — the docs predate the actual file layout — but cleaning them up reduces operator confusion. Bundle BOTH into one commit if you do them; SKIP this task entirely if it's already been addressed in 7-iv or 7-i.

- [ ] **Step 1: §14 repo-layout fix**

In `docs/PROJECT.md` §14, the `core/src/` tree lists files that never existed:

```
│   │   ├── screenshot.ts
│   │   ├── auth.ts
```

Phase 6's actual file inventory (per `core/src/` after Phase 6's 6-iii) is:

```
│   │   ├── index.ts
│   │   ├── client.ts           # conversation HTTP/SSE client
│   │   ├── context.ts          # URL, viewport, etc capture
│   │   ├── capture.ts          # html2canvas DI wrapper (Phase 6 — see L021)
│   │   ├── attachments.ts      # pending-attachment state + size accounting
│   │   ├── types.ts            # shared TS types (includes SubmitAttachment from Phase 6)
│   │   └── generated/
│   │       └── payload.ts      # generated from schema
```

(Verify against the actual `core/src/` tree before committing — these are the files Phase 6 introduced; if 7-iv added or renamed any others, reflect those too.)

Similarly, `vue/src/components/` should list the actual Phase 6 components:

```
│   │   ├── components/
│   │   │   ├── ConversationView.vue
│   │   │   ├── ScreenshotRedactor.vue    # Phase 6 — rectangle redaction modal
│   │   │   ├── AttachmentStrip.vue       # Phase 6 — pending thumbnail strip
│   │   │   └── AuthDialog.vue
```

(Drop `ScreenshotCapture.vue` if it never existed; verify against the actual `vue/src/components/` tree.)

- [ ] **Step 2: §15 — note `intake-license` excluded from goreleaser**

In `docs/PROJECT.md` §15, after the `goreleaser builds relay binaries for: ...` line, add:

> The `intake-license` maintainer CLI in `license-tool/` is excluded from goreleaser per the v0 decomposition decision Q10 (it is not distributed publicly in v0; the maintainer runs it locally to sign new licenses). It is built ad-hoc via `go build -o intake-license ./license-tool/cmd/intake-license`.

- [ ] **Step 3: Commit**

```bash
git add docs/PROJECT.md
git commit -m "$(cat <<'EOF'
docs(7-v): PROJECT.md repo-layout + license-tool exclusion notes

§14: refresh core/src/ + vue/src/components/ trees to reflect the actual
Phase 6 file inventory (capture.ts, attachments.ts, ScreenshotRedactor.vue,
AttachmentStrip.vue) instead of the never-existed screenshot.ts/auth.ts
placeholders from the original v0 design.

§15: add explicit note that intake-license is excluded from goreleaser per
the v0 decomposition Q10 — built ad-hoc by the maintainer for license
signing; not distributed publicly in v0.

Both are doc-only; no code or governance file changes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 14: Final green-bar verification

**Files:** none new — verification commit captures exit codes in the commit message body

- [ ] **Step 1: Capture every exit code**

Run every command from Phase 7 design spec §11 / README §6 / this plan's done-criteria; record the exit code (and a single-line summary) in a scratch file:

```bash
cd c:/src/ai/intake
exec > /tmp/phase-7-green-bar.txt 2>&1

echo "=== Final green-bar verification ($(date -u +%Y-%m-%dT%H:%M:%SZ)) ==="

cd relay
go build ./...                            ; echo "go build: $?"
go vet ./...                              ; echo "go vet: $?"
go test -race ./...                       ; echo "go test -race: $?"
go mod tidy                                ; echo "go mod tidy ran"
cd ..
git diff --stat relay/go.mod relay/go.sum ; echo "go mod tidy diff: <empty expected>"

cd core
npm run type-check                        ; echo "core type-check: $?"
npm run test                              ; echo "core test: $?"
cd ..

cd vue
npm run type-check                        ; echo "vue type-check: $?"
npm run build                             ; echo "vue build: $?"
npm run test                              ; echo "vue test: $?"
cd ..

bash scripts/verify-contract.sh           ; echo "verify-contract: $?"
bash scripts/check-pins.sh                ; echo "check-pins: $?"

cd relay && golangci-lint run ./...       ; echo "golangci-lint: $?"
cd ..
npx eslint .                              ; echo "eslint: $?"
npx prettier --check .                    ; echo "prettier: $?"

goreleaser check relay/.goreleaser.yaml   ; echo "goreleaser check: $?"
goreleaser release --snapshot --clean --config relay/.goreleaser.yaml ; echo "goreleaser snapshot: $?"

(cd examples/docker-compose && docker-compose config)  ; echo "docker-compose config: $?"

git diff main..phase-7 -- relay/internal/adapter/adapter.go relay/internal/payload/types.go schema/payload.v1.json
echo "frozen-seam diff: <empty expected>"

echo "=== End green-bar verification ==="
```

Expected: every exit code 0; `go mod tidy` diff empty; frozen-seam diff empty.

- [ ] **Step 2: Re-read Phase 7 README §6 build-fail checklist**

Tick every build-fail item against the captured evidence in `/tmp/phase-7-green-bar.txt` + the §7.1 evidence subsection. ALL items must hold. Any failure here is a regression that must be fixed before merge — do NOT skip or paper over.

- [ ] **Step 3: Commit the green-bar record**

```bash
git commit --allow-empty -m "$(cat <<'EOF'
chore(7-v): final green-bar verification

All exit codes 0 across the Phase 7 verification matrix:

- cd relay && go build ./...                                  exit 0
- cd relay && go vet ./...                                    exit 0
- cd relay && go test -race ./...                             exit 0
- cd relay && go mod tidy                                     no-op (empty diff)
- cd core && npm run type-check                               exit 0
- cd core && npm run test                                     exit 0
- cd vue  && npm run type-check                               exit 0
- cd vue  && npm run build                                    exit 0
- cd vue  && npm run test                                     exit 0
- bash scripts/verify-contract.sh                             exit 0
- bash scripts/check-pins.sh                                  exit 0
- golangci-lint run ./...     (in relay/)                     exit 0, 0 findings
- npx eslint .                                                exit 0, 0 errors
- npx prettier --check .                                      exit 0
- goreleaser check relay/.goreleaser.yaml                     exit 0
- goreleaser release --snapshot --clean                       exit 0; 5 archives + image
- (cd examples/docker-compose && docker-compose config)       exit 0
- Frozen-seam diff (main..phase-7 -- adapter.go, types.go,
  schema/payload.v1.json)                                     empty

Phase 7 build-fail checklist (README §6) re-checked against the §7.1
evidence subsection — all items hold. Phase 7 ready for the bundled
phase-7 → main merge.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

The `--allow-empty` is intentional — this commit is the audit-trail green-bar record; the tree state is exactly the post-Task 13 state.

---

## Smoke (mandatory)

For 7-v the smoke IS the deliverable. The authoritative smoke recipe is `ai/tasks/phase-7/README.md` §7 (the 8 numbered items). 7-v's job is to execute every item, capture the evidence, and record it in §7.1 of that README. The done-criteria below are the verification.

## Done criteria

- [ ] All 14 tasks complete and committed.
- [ ] `cd relay && go build ./... && go vet ./... && go test -race ./...` green.
- [ ] `cd core && npm run type-check && npm run test` green (core has no build script — by design).
- [ ] `cd vue && npm run type-check && npm run build && npm run test` green.
- [ ] `bash scripts/verify-contract.sh` exits 0 (no schema changes in Phase 7).
- [ ] `bash scripts/check-pins.sh` exits 0 (new pin checks for `prometheus/client_golang`, `goreleaser`, `golangci-lint`, `eslint`, `prettier`).
- [ ] `cd relay && go mod tidy` is a no-op (Phase 7 introduces exactly one new module: `prometheus/client_golang` in 7-i; 7-v adds zero).
- [ ] `golangci-lint run ./...` in `relay/` exits 0 with zero findings.
- [ ] `npx eslint .` exits 0 with zero errors.
- [ ] `npx prettier --check .` exits 0 with zero unformatted files.
- [ ] `goreleaser check relay/.goreleaser.yaml` exits 0.
- [ ] `goreleaser release --snapshot --clean` exits 0; produces 5 archives + SHA256SUMS.txt + local `intake-relay:snapshot` docker image; binary `--version` printout matches; image < 50 MB.
- [ ] `npm pack -w @intake/core` + `npm pack -w @intake/vue` exit 0; `npm publish --dry-run` for both exits 0; no `.env` / `local-dev/` / secrets / credentials in either tarball.
- [ ] `(cd examples/docker-compose && docker-compose config)` exits 0.
- [ ] `drive-docker-compose.ts` passes from a clean state: /v1/health 200 within 30s, /init returns capabilities.attachments, /turn SSE done, /submit returns external_id + adapter_name="webhook", webhook-receiver log has the canonical payload, /metrics on :19090 has 4 series + non-zero init counter, `docker exec intake-relay id -u` returns 65532, `down -v` leaves no containers/volumes.
- [ ] Q9 combined-misconfig smoke: ONE consolidated `relay: startup config errors` log line with `count >= 6` listing Phase 5 + Phase 6 + Phase 7 problems; exit 1. Closes FOLLOWUPS I1 + I2 end-to-end.
- [ ] Metrics endpoint smoke: disabled arm `/metrics` connection refused on :9090 with main relay still healthy; enabled arm exports all 4 HELP lines + init counter increments by exactly 5 after 5 /init requests.
- [ ] Phase 1-6 regression: `drive-attachments.ts enabled` + `disabled` arms pass; `drive-abuse.ts` passes; Phase 4 deferral note recorded honestly in §7.1 evidence (zero changes to auth dispatcher; transitive coverage via Phase 5).
- [ ] `git diff main..phase-7 -- relay/internal/adapter/adapter.go relay/internal/payload/types.go schema/payload.v1.json` is EMPTY (frozen Phase 0-6 seams unchanged).
- [ ] `ai/LESSONS.md` has L024 (snapshot-then-publish split), L025 (initial lint sweep before CI gate), L026 (metrics server independence), L027 (off-by-default observability), L028 (distroless multi-stage Docker template) — five new entries, format mirrors L016-L023.
- [ ] `ai/tasks/phase-7/README.md` §3 shows `Live + smoked` for all 5 sub-plans.
- [ ] `ai/tasks/phase-7/README.md` §7.1 "Smoke status (YYYY-MM-DD)" subsection populated with full 8-item evidence (one fenced code block per smoke item + Phase 4 deferral note + L022 cross-phase-consolidation invariant proven).
- [ ] `ai/tasks/phase-6/FOLLOWUPS.md` renamed to `ai/tasks/phase-6/FOLLOWUPS-resolved.md`; "resolved by Phase 7-i" header banner prepended; every reference in `ai/` and `docs/` updated to the new filename (verify via `grep -rn FOLLOWUPS.md ai/ docs/`).
- [ ] (Optional) `docs/PROJECT.md` §14 refreshed to match actual `core/src/` + `vue/src/components/` tree; §15 explicit note that `intake-license` is excluded from goreleaser per Q10.
- [ ] Phase 7 build-fail checklist (README §6) re-checked end-to-end against the captured evidence.
- [ ] Phase 7 ready for the bundled `phase-7` → `main` merge. Branch stays on `phase-7`; do NOT push during 7-v.
