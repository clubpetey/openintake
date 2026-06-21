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
 *   8. docker exec openintake-relay id -u → assert "65532" (distroless nonroot UID)
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
  assert(
    typeof body.session_id === 'string' && body.session_id.length > 0,
    'init returns non-empty session_id',
  );
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
  assert(
    typeof json.external_id === 'string' && json.external_id.length > 0,
    '/submit returns external_id',
  );
  assert(
    json.adapter_name === 'webhook',
    `/submit adapter_name is "webhook" (got ${String(json.adapter_name)})`,
  );
  return json;
}

async function assertWebhookCanonicalPayloadLanded(): Promise<void> {
  console.log(`\n=== assert canonical payload landed in webhook-receiver ===`);
  // The webhook-receiver from 7-iii logs every received body to stdout.
  // Grep the compose logs for the canonical fields.
  const r = await exec(COMPOSE_BIN, ['logs', '--no-color', 'webhook-receiver'], {
    cwd: COMPOSE_DIR,
  });
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
  assert(
    initLine != null,
    '/metrics has intake_http_requests_total{path="/v1/intake/init",status="200"} sample',
  );
  if (initLine) {
    const m = /\s(\d+(?:\.\d+)?)\s*$/.exec(initLine);
    const val = m ? Number(m[1]) : 0;
    assert(val >= 1, `init counter >= 1 (got ${val})`);
  }
}

async function assertDistrolessNonrootUID(): Promise<void> {
  console.log(`\n=== docker exec openintake-relay id -u ===`);
  const r = await exec('docker', ['exec', 'openintake-relay', 'id', '-u']);
  assert(
    r.code === 0,
    `docker exec id -u exit 0 (got ${r.code}); stderr=${r.stderr.slice(0, 200)}`,
  );
  const uid = r.stdout.trim();
  assert(uid === '65532', `relay runs as distroless nonroot UID 65532 (got "${uid}")`);
}

// --- Main ---------------------------------------------------------------------

async function main(): Promise<void> {
  console.log(
    `docker-compose smoke: COMPOSE_DIR=${COMPOSE_DIR} RELAY_URL=${RELAY_URL} METRICS_URL=${METRICS_URL}`,
  );
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
