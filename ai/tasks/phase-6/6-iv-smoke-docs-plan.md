# 6-iv Final Smoke + Docs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## Goal

Author the credit-free attachment smoke driver and YAML fixtures, run every self-runnable smoke from Phase 6 README §7 (Q9 startup gate, caps discovery, validation, webhook forward, adapter native-sequence rerun, Phase 1+4+5 regression, body-cap regression), PAUSE for the maintainer to run the live Chatwoot attachment smoke against `chatwoot.cloud` with Phase 3's credentials, record every command and output in the phase-6 README's "Smoke status" section, write `docs/attachments.md`, refresh `docs/PROJECT.md` §11 cross-references, append any novel lessons (≥3 entries — multipart-vs-JSON adapter branching, html2canvas SSR-safety DI, live-smoke snags) to `ai/LESSONS.md`, and mark the phase done.

## Architecture

Three artifact families. (1) One new TypeScript smoke driver — `core/smoke/drive-attachments.ts` — mirroring the Phase 5 `core/smoke/drive-abuse.ts` style: drives `/init` → `/turn` → `/submit` against a running relay + the Phase 5 `fake-llm` provider + a local webhook receiver, and exercises every Phase 6 validation sentinel. (2) Five new YAML fixtures under `relay/cmd/relay/smoke/` for the Q9 gate, caps-discovery, validation, webhook-forward, and combined-misconfig smokes. (3) Documentation updates — new `docs/attachments.md`, refreshed `docs/PROJECT.md` §11 cross-refs, ≥3 new `ai/LESSONS.md` entries (L020+), Phase 6 README §3 + §7 evidence updates. No new dependencies (TS smoke driver uses only `fetch` + stdlib, same as `drive-abuse.ts`; YAMLs are plain text).

After this sub-plan: Phase 6 has the same "evidence-of-everything" gate as Phase 4 and Phase 5 — every Q9 startup mismatch is proven fatal, every validation sentinel is proven to return its documented HTTP code, every adapter's native sequence is proven via httptest re-run, and the live Chatwoot path is proven against `chatwoot.cloud` with the redaction visible inline in the conversation.

## Tech Stack

- Node 24 / TypeScript 5.6.3 (smoke driver via `npx tsx`), `@intake/core` `IntakeClient` for the typed `init`/`submit` calls.
- Go 1.23.2 — re-runs Phase 5's `relay/cmd/fake-llm` (no new Go code in 6-iv).
- YAML 1.2 (relay smoke fixtures).
- Stdlib only — no new TS or Go dependencies. `go mod tidy` must remain a no-op.

## Design References

- Phase 6 README §7 — the authoritative final smoke (what gets recorded as evidence)
- Phase 6 README §6 — build-fail items (must all still hold at end of phase)
- Phase 6 design spec §11.1 + §11.2 — testing strategy + live-smoke matrix
- Phase 6 design spec §13 — final smoke steps 1–8 (this plan executes them in order)
- Phase 6 README §8 — frozen contracts the smokes assert against (`Capabilities.Attachments`, `SubmitRequest.Attachments`, attachvalidate sentinels)
- Phase 5 `5-iv-smoke-docs-plan.md` — the structural exemplar (TS driver style, fixture style, README §7 "Smoke status" evidence format, LESSONS append pattern)
- Phase 5 fixtures `relay/cmd/relay/smoke/{abuse-driver,strict-anonymous,rate-limit-test}.yaml` — mirrored format
- LESSONS L004 — Node smoke browser-global stubs (applies to `/submit` calls in `drive-attachments.ts`)
- LESSONS L005 — redact-before-truncate (the live chatwoot smoke must grep for the token in logs → zero matches)
- LESSONS L010 — PS 5.1 BOM gotcha (`-Encoding ascii` for any YAML written via PowerShell `Set-Content`)
- LESSONS L011 — Chatwoot agent-side `POST /conversations` requires pre-created contact_inbox (Phase 6's chatwoot adapter inherits the two-call flow unchanged)
- LESSONS L015 — derived-field test gaps (the live chatwoot smoke is the canonical "live catches what unit tests don't model" instance for Phase 6)
- LESSONS L016 — return parsed values from startup gate (the Q9 fixtures prove the gate's combined-error log line)
- LESSONS L019 — smoke-fixture math: when 6-iv's `drive-attachments.ts` exercises BOTH per-attachment cap AND aggregate cap in one driver run, fixture sizes must make per-attachment fire on the smallest case AND aggregate fire on a separate larger case — don't shadow gates.

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `core/smoke/drive-attachments.ts` | Create | TS driver: `/init` caps assertion + every validation sentinel + webhook forward + body-cap regression |
| `relay/cmd/relay/smoke/attachments-enabled.yaml` | Create | Caps-discovery smoke (Enabled=true, all 5 adapters registered) + webhook-forward driver fixture + body-cap raise verification |
| `relay/cmd/relay/smoke/attachments-disabled.yaml` | Create | Caps-discovery smoke (Enabled=false → no caps block in /init); body-cap regression to 1 MB |
| `relay/cmd/relay/smoke/attachments-bad-storage-mode.yaml` | Create | Q9 startup-fatal fixture: `storage.mode: "s3"` |
| `relay/cmd/relay/smoke/attachments-cap-inverted.yaml` | Create | Q9 startup-fatal fixture: `max_size_bytes` > `max_total_bytes` |
| `relay/cmd/relay/smoke/attachments-combined.yaml` | Create | Combined Phase-5 + Phase-6 misconfig fixture (exactly one consolidated "relay: startup config errors" log line) |
| `docs/attachments.md` | Create | Operator config + per-adapter behavior matrix + widget UI flow |
| `docs/PROJECT.md` | Modify | §11 cross-refs to `relay/internal/attachvalidate/`, `core/src/capture.ts`, `vue/src/components/ScreenshotRedactor.vue`, `vue/src/components/AttachmentStrip.vue` |
| `ai/LESSONS.md` | Modify | Append L020 (multipart-vs-JSON branching), L021 (html2canvas SSR-safety + DI), L022+ (any live-smoke snag uncovered) |
| `ai/tasks/phase-6/README.md` | Modify | §3 sub-plan index status → "Live + smoked" for 6-i, 6-ii, 6-iii, 6-iv; §7 final §7.1 "Smoke status (YYYY-MM-DD)" subsection with the full evidence (mirrors Phase 5 5-iv format) |

---

## Tasks

### Task 1: Author `core/smoke/drive-attachments.ts`

**Files:** Create `core/smoke/drive-attachments.ts`

- [ ] **Step 1: Write the driver**

Create `core/smoke/drive-attachments.ts`:

```typescript
/**
 * Phase 6 attachments smoke driver.
 *
 * Drives every documented attachment validation path against a running relay:
 *   1. /init returns capabilities.attachments when cfg.Attachments.Enabled=true
 *      and omits it when Enabled=false (caps-discovery).
 *   2. Submit with 1×1 PNG → 200 + webhook receiver logs attachments[0] verbatim
 *      (forward smoke).
 *   3. Submit with a 6 MB attachment            → 413 attachment_too_large
 *   4. Submit with two 6 MB attachments         → 413 attachment_too_large    (first encountered)
 *   5. Submit with three 4 MB attachments       → 413 attachments_exceed_total
 *   6. Submit with declared image/png, JPEG bytes → 415 attachment_mime_mismatch
 *   7. Submit with mime_type "image/heic"       → 415 attachment_mime_not_allowed
 *   8. Submit with url "not-a-data-url"         → 400 attachment_malformed
 *   9. Submit with type "file" + valid PNG      → 400 attachment_type_unsupported
 *  10. Submit with non-empty attachments[] when
 *      relay started Disabled=true              → 400 attachments_disabled
 *  11. With Enabled=false, 2 MB body            → 413 request_body_too_large
 *      (body-cap regression — proves 1 MB cap preserved when attachments off)
 *
 * Requires:
 *   - the relay running with relay/cmd/relay/smoke/attachments-enabled.yaml
 *     (steps 1, 2, 3, 4, 5, 6, 7, 8, 9)
 *   - the relay restarted with relay/cmd/relay/smoke/attachments-disabled.yaml
 *     (steps 1-disabled-arm, 10, 11)
 *   - the fake-llm running on :11434 (relay/cmd/fake-llm — Phase 5 artifact)
 *   - a local webhook receiver on :19099/intake (this driver spins one up)
 *
 * Usage:
 *   RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts enabled
 *   RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts disabled
 *
 * Browser-global stubs (LESSONS L004): /submit calls IntakeClient.submit which
 * uses captureClient() reading window/navigator/document. This script stubs
 * them via Object.defineProperty before calling submit() — plain assignment to
 * globalThis.navigator throws in Node 24.
 *
 * Fixture sizes (LESSONS L019): max_size_bytes=5MB, max_total_bytes=10MB.
 *   - 6 MB single attachment fires per-attachment cap (smallest case).
 *   - 3×4MB = 12 MB fires aggregate cap (separate larger case, well above per-
 *     attachment cap so the aggregate sentinel wins on first-encountered ordering).
 *   - Two 6 MB attachments fire per-attachment on the FIRST (matches spec wording
 *     "first encountered sentinel error" from attachvalidate.ValidateAll).
 */

import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';

const RELAY_URL = process.env['RELAY_URL'] ?? 'http://127.0.0.1:18080';
const MODE = (process.argv[2] ?? 'enabled') as 'enabled' | 'disabled';

// --- Browser-global stubs (LESSONS L004) — driver does not import IntakeClient
//     directly (this smoke uses raw fetch for full status-code + body access),
//     so the stubs are not strictly required, but kept for symmetry with
//     drive-auth-email.ts and to future-proof if a step migrates to IntakeClient.

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

// --- Golden fixtures: smallest-valid byte sequences ----------------------------

/** Smallest valid 1×1 PNG (67 bytes raw, 92 chars base64). */
const PNG_1X1_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNgAAIAAAUAAeImBZsAAAAASUVORK5CYII=';

/** Smallest valid JPEG header bytes (SOI + APP0 + minimal markers + EOI). */
const JPEG_MIN_BASE64 =
  '/9j/4AAQSkZJRgABAQEASABIAAD//gATQ3JlYXRlZCB3aXRoIEdJTVD/2wBDAAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEB' +
  'AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/2wBDAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEB' +
  'AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAA' +
  'AAAAAAAAAAAAAAr/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFAEBAAAAAAAAAAAAAAAAAAAAAP/EABQRAQAAAAAAAAAA' +
  'AAAAAAAAAAD/2gAMAwEAAhEDEQA/AL+B/9k=';

/** Generate a base64 data: URL of N bytes of zero-padded PNG-ish data.
 *  NOT a valid PNG (no IHDR/IDAT chain); used only for size-cap tests where
 *  validation order means the size check fires before the magic-byte check.
 *  attachvalidate.ValidateAll spec (Phase 6 README §8.5): first-encountered
 *  sentinel returned — per-attachment size is checked early; for these size-
 *  cap fixtures the size sentinel fires first. */
function makeSizedDataURL(mime: string, bytes: number): string {
  // For SIZE smokes we use real PNG header + repeated padding so the magic-byte
  // check (if it runs first) would still see a PNG signature. The validator
  // checks size BEFORE magic-bytes per the documented order in attachvalidate.
  const pngHeader = Buffer.from(PNG_1X1_BASE64, 'base64');
  const padding = Buffer.alloc(Math.max(0, bytes - pngHeader.length), 0);
  const full = Buffer.concat([pngHeader, padding]).slice(0, bytes);
  return `data:${mime};base64,${full.toString('base64')}`;
}

const PNG_1X1_DATAURL = `data:image/png;base64,${PNG_1X1_BASE64}`;
const JPEG_MIN_DATAURL = `data:image/jpeg;base64,${JPEG_MIN_BASE64}`;
const PNG_DECLARED_BUT_JPEG_BYTES = `data:image/png;base64,${JPEG_MIN_BASE64}`;

// --- Helpers ------------------------------------------------------------------

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

interface ErrorEnvelope {
  error: { code: string; message: string };
}

async function init(): Promise<InitResponse> {
  const resp = await fetch(`${RELAY_URL}/v1/intake/init`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  });
  if (!resp.ok) {
    throw new Error(`init failed: ${resp.status} ${await resp.text()}`);
  }
  return (await resp.json()) as InitResponse;
}

interface SubmitAttachment {
  type: string;
  mime_type: string;
  url: string;
  label?: string;
}

async function submit(
  sessionID: string,
  attachments: SubmitAttachment[],
): Promise<{ status: number; body: string }> {
  const body = {
    messages: [{ role: 'user', content: 'attachment smoke' }],
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
    attachments,
  };
  const resp = await fetch(`${RELAY_URL}/v1/intake/submit`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Intake-Session': sessionID,
    },
    body: JSON.stringify(body),
  });
  return { status: resp.status, body: await resp.text() };
}

function assert(cond: boolean, msg: string): void {
  if (!cond) {
    console.error(`FAIL: ${msg}`);
    process.exit(1);
  }
  console.log(`OK: ${msg}`);
}

function parseErr(body: string): ErrorEnvelope | null {
  try {
    return JSON.parse(body) as ErrorEnvelope;
  } catch {
    return null;
  }
}

// --- Local webhook receiver ---------------------------------------------------
//
// Captures the most recent /intake POST body so the forward smoke can assert
// the relay-sent payload includes attachments[0] verbatim.

interface Receiver {
  lastBody: string | null;
  close: () => Promise<void>;
  port: number;
}

function startWebhookReceiver(): Promise<Receiver> {
  return new Promise((resolve) => {
    let lastBody: string | null = null;
    const srv = createServer((req: IncomingMessage, res: ServerResponse) => {
      const chunks: Buffer[] = [];
      req.on('data', (c: Buffer) => chunks.push(c));
      req.on('end', () => {
        lastBody = Buffer.concat(chunks).toString('utf8');
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end('{"external_id":"smoke-webhook-1","external_url":"http://smoke/1"}');
      });
    });
    srv.listen(19099, '127.0.0.1', () => {
      const port = (srv.address() as AddressInfo).port;
      resolve({
        get lastBody() {
          return lastBody;
        },
        close: () =>
          new Promise<void>((r) => {
            srv.close(() => r());
          }),
        port,
      });
    });
  });
}

// --- Smoke arms ---------------------------------------------------------------

async function smokeEnabledArm(): Promise<void> {
  console.log('\n=== Smoke arm: cfg.Attachments.Enabled=true ===');

  // 1. caps-discovery: capabilities.attachments must be present.
  const initResp = await init();
  assert(
    initResp.capabilities.attachments != null,
    'init returns capabilities.attachments when Enabled=true',
  );
  const caps = initResp.capabilities.attachments!;
  assert(caps.max_size_bytes === 5_242_880, `max_size_bytes is 5 MB (got ${caps.max_size_bytes})`);
  assert(caps.max_total_bytes === 10_485_760, `max_total_bytes is 10 MB (got ${caps.max_total_bytes})`);
  assert(
    caps.allowed_mime_types.length === 3 &&
      caps.allowed_mime_types.includes('image/png') &&
      caps.allowed_mime_types.includes('image/jpeg') &&
      caps.allowed_mime_types.includes('image/webp'),
    `allowed_mime_types is [png,jpeg,webp] (got ${JSON.stringify(caps.allowed_mime_types)})`,
  );

  // 2. forward smoke: 1×1 PNG → 200 + webhook receiver logs attachment verbatim.
  const recv = await startWebhookReceiver();
  try {
    const session = initResp.session_id;
    const okResp = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: PNG_1X1_DATAURL, label: 'shot' },
    ]);
    assert(okResp.status === 200, `1×1 PNG submit returns 200 (got ${okResp.status} ${okResp.body.slice(0, 200)})`);
    assert(recv.lastBody !== null, 'webhook receiver captured a body');
    if (recv.lastBody) {
      const parsed = JSON.parse(recv.lastBody) as { attachments?: Array<{ url: string; mime_type: string }> };
      assert(Array.isArray(parsed.attachments) && parsed.attachments.length === 1, 'webhook body includes exactly one attachment');
      assert(parsed.attachments![0]!.url === PNG_1X1_DATAURL, 'webhook attachments[0].url matches submitted data: URL verbatim');
      assert(parsed.attachments![0]!.mime_type === 'image/png', 'webhook attachments[0].mime_type is image/png');
    }

    // 3. per-attachment cap: 6 MB single → 413 attachment_too_large.
    const big = makeSizedDataURL('image/png', 6 * 1024 * 1024);
    const r3 = await submit(session, [{ type: 'screenshot', mime_type: 'image/png', url: big }]);
    assert(r3.status === 413, `6 MB attachment returns 413 (got ${r3.status})`);
    assert(parseErr(r3.body)?.error.code === 'attachment_too_large', '6 MB attachment code is attachment_too_large');

    // 4. two 6 MB attachments → 413 attachment_too_large (first encountered).
    const r4 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: big },
      { type: 'screenshot', mime_type: 'image/png', url: big },
    ]);
    assert(r4.status === 413, `two 6 MB attachments returns 413 (got ${r4.status})`);
    assert(parseErr(r4.body)?.error.code === 'attachment_too_large', 'two 6 MB attachments code is attachment_too_large (first sentinel)');

    // 5. three 4 MB attachments → 413 attachments_exceed_total.
    const four = makeSizedDataURL('image/png', 4 * 1024 * 1024);
    const r5 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: four },
      { type: 'screenshot', mime_type: 'image/png', url: four },
      { type: 'screenshot', mime_type: 'image/png', url: four },
    ]);
    assert(r5.status === 413, `three 4 MB attachments returns 413 (got ${r5.status})`);
    assert(
      parseErr(r5.body)?.error.code === 'attachments_exceed_total',
      'three 4 MB attachments code is attachments_exceed_total',
    );

    // 6. MIME mismatch: declared image/png but JPEG bytes → 415.
    const r6 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: PNG_DECLARED_BUT_JPEG_BYTES },
    ]);
    assert(r6.status === 415, `declared image/png with JPEG bytes returns 415 (got ${r6.status})`);
    assert(
      parseErr(r6.body)?.error.code === 'attachment_mime_mismatch',
      'mismatch code is attachment_mime_mismatch',
    );

    // 7. MIME not allowed: image/heic (not in published allowlist) → 415.
    const r7 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/heic', url: 'data:image/heic;base64,AAAA' },
    ]);
    assert(r7.status === 415, `image/heic returns 415 (got ${r7.status})`);
    assert(
      parseErr(r7.body)?.error.code === 'attachment_mime_not_allowed',
      'heic code is attachment_mime_not_allowed',
    );

    // 8. malformed URL: not a data: URL → 400.
    const r8 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: 'http://not-a-data-url/x.png' },
    ]);
    assert(r8.status === 400, `non-data URL returns 400 (got ${r8.status})`);
    assert(parseErr(r8.body)?.error.code === 'attachment_malformed', 'non-data URL code is attachment_malformed');

    // 9. type:"file" → 400 attachment_type_unsupported.
    const r9 = await submit(session, [
      { type: 'file', mime_type: 'image/png', url: PNG_1X1_DATAURL },
    ]);
    assert(r9.status === 400, `type:"file" returns 400 (got ${r9.status})`);
    assert(
      parseErr(r9.body)?.error.code === 'attachment_type_unsupported',
      'type:"file" code is attachment_type_unsupported',
    );
  } finally {
    await recv.close();
  }
}

async function smokeDisabledArm(): Promise<void> {
  console.log('\n=== Smoke arm: cfg.Attachments.Enabled=false ===');

  // 1-disabled-arm. caps-discovery: capabilities.attachments must be absent/null.
  const initResp = await init();
  assert(
    initResp.capabilities.attachments == null,
    'init omits capabilities.attachments when Enabled=false',
  );

  // 10. non-empty attachments[] when Enabled=false → 400 attachments_disabled.
  const r10 = await submit(initResp.session_id, [
    { type: 'screenshot', mime_type: 'image/png', url: PNG_1X1_DATAURL },
  ]);
  assert(r10.status === 400, `attachments with Enabled=false returns 400 (got ${r10.status})`);
  assert(
    parseErr(r10.body)?.error.code === 'attachments_disabled',
    'Enabled=false code is attachments_disabled',
  );

  // 11. body-cap regression: 2 MB body with Enabled=false → 413 request_body_too_large
  //     (the 1 MB cap is preserved when attachments are off).
  const garbage = 'x'.repeat(2 * 1024 * 1024);
  const resp = await fetch(`${RELAY_URL}/v1/intake/submit`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Intake-Session': initResp.session_id,
    },
    body: JSON.stringify({
      messages: [{ role: 'user', content: garbage }],
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
    }),
  });
  const body = await resp.text();
  assert(resp.status === 413, `2 MB body with Enabled=false returns 413 (got ${resp.status})`);
  assert(
    parseErr(body)?.error.code === 'request_body_too_large',
    '2 MB body with Enabled=false code is request_body_too_large',
  );
}

async function main(): Promise<void> {
  console.log(`attachments smoke: RELAY_URL=${RELAY_URL} MODE=${MODE}`);
  if (MODE === 'enabled') {
    await smokeEnabledArm();
  } else if (MODE === 'disabled') {
    await smokeDisabledArm();
  } else {
    console.error(`unknown mode: ${MODE} (expected "enabled" or "disabled")`);
    process.exit(1);
  }
  console.log('\n✓ All Phase 6 attachment smokes passed for this arm.');
}

main().catch((err: unknown) => {
  console.error('smoke driver failed:', err);
  process.exit(1);
});
```

- [ ] **Step 2: Commit**

```bash
git add core/smoke/drive-attachments.ts
git commit -m "feat(6-iv): drive-attachments.ts — every validation sentinel + caps discovery + body-cap regression"
```

---

### Task 2: Author the smoke fixture YAMLs

**Files:** Create `relay/cmd/relay/smoke/attachments-enabled.yaml`, `attachments-disabled.yaml`, `attachments-bad-storage-mode.yaml`, `attachments-cap-inverted.yaml`, `attachments-combined.yaml`

- [ ] **Step 1: Author `attachments-enabled.yaml`**

Create `relay/cmd/relay/smoke/attachments-enabled.yaml`:

```yaml
server:
  addr: ":18080"
  external_url: "http://127.0.0.1:18080"
  cors_origins: ["http://localhost:5173"]
llm:
  provider: "ollama"
  ollama:
    base_url: "http://127.0.0.1:11434"
    model: "fake"
    max_tokens: 50
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
attachments:
  enabled: true
  max_size_bytes: 5242880      # 5 MB
  max_total_bytes: 10485760    # 10 MB
  allowed_mime_types: ["image/png", "image/jpeg", "image/webp"]
  storage:
    mode: "forward"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 2: Author `attachments-disabled.yaml`**

Create `relay/cmd/relay/smoke/attachments-disabled.yaml`:

```yaml
server:
  addr: ":18080"
  external_url: "http://127.0.0.1:18080"
  cors_origins: ["http://localhost:5173"]
llm:
  provider: "ollama"
  ollama:
    base_url: "http://127.0.0.1:11434"
    model: "fake"
    max_tokens: 50
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
attachments:
  enabled: false
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 3: Author `attachments-bad-storage-mode.yaml`**

Create `relay/cmd/relay/smoke/attachments-bad-storage-mode.yaml`:

```yaml
server:
  addr: ":18080"
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
attachments:
  enabled: true
  storage:
    mode: "s3"        # only "" or "forward" valid in v0 → Q9 startup-fatal
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 4: Author `attachments-cap-inverted.yaml`**

Create `relay/cmd/relay/smoke/attachments-cap-inverted.yaml`:

```yaml
server:
  addr: ":18080"
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
attachments:
  enabled: true
  max_size_bytes: 20000000      # > max_total_bytes → Q9 startup-fatal
  max_total_bytes: 10000000
  storage:
    mode: "forward"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 5: Author `attachments-combined.yaml`**

Create `relay/cmd/relay/smoke/attachments-combined.yaml` — combine ALL Phase-6 attachment misconfigs (bad storage mode + inverted caps) WITH at least one Phase-5 misconfig (anonymous-no-captcha) in one file. The relay MUST emit exactly one consolidated `relay: startup config errors` log line listing every problem.

```yaml
server:
  addr: ":18080"
  trusted_proxies:
    - "10.0.0.0/8"
    - "not-a-cidr"               # Phase-5 misconfig (mirrors Phase-5's bad-cidr.yaml)
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: false # Phase-5 misconfig (anonymous on but no captcha)
captcha:
  enabled: false
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "queue"  # Phase-5 misconfig (action_on_exceeded must be "reject")
attachments:
  enabled: true
  max_size_bytes: 20000000       # Phase-6 misconfig (> max_total_bytes)
  max_total_bytes: 10000000
  storage:
    mode: "s3"                   # Phase-6 misconfig (only "" or "forward" allowed)
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

> **L010 reminder:** if authoring via PowerShell, use `[System.IO.File]::WriteAllText($path, $content, [System.Text.UTF8Encoding]::new($false))` or `-Encoding ascii`. Default PS 5.1 `-Encoding utf8` writes a BOM that the relay's YAML decoder rejects with `invalid character 'ï'`.

- [ ] **Step 6: Commit**

```bash
git add relay/cmd/relay/smoke/attachments-enabled.yaml relay/cmd/relay/smoke/attachments-disabled.yaml relay/cmd/relay/smoke/attachments-bad-storage-mode.yaml relay/cmd/relay/smoke/attachments-cap-inverted.yaml relay/cmd/relay/smoke/attachments-combined.yaml
git commit -m "feat(6-iv): Phase 6 attachment smoke fixtures — enabled, disabled, two Q9 fatals, combined"
```

---

### Task 3: Q9 startup smoke (self-runnable)

**Files:** none new (extends Phase 5's `run-q9-smoke.sh` style; inline shell commands)

- [ ] **Step 1: Run each new misconfig fixture, assert exit 1 + expected log substring**

For each of `attachments-bad-storage-mode.yaml`, `attachments-cap-inverted.yaml`:

```bash
cd c:/src/ai/intake
export INTAKE_SSO_HS256="${INTAKE_SSO_HS256:-dummy-secret-32-bytes-padded----}"

for fixture in attachments-bad-storage-mode attachments-cap-inverted; do
  echo "=== Q9 smoke: $fixture ==="
  output=$(go run ./relay/cmd/relay --config relay/cmd/relay/smoke/$fixture.yaml 2>&1 || true)
  echo "$output" | grep -q "relay: startup config errors" && echo "OK: consolidated error line present" || { echo "FAIL"; exit 1; }
done
```

Expected (per fixture):
- `attachments-bad-storage-mode.yaml`: log line contains `storage.mode` substring and the offending value `s3`.
- `attachments-cap-inverted.yaml`: log line contains `max_size_bytes` substring and explanation about exceeding `max_total_bytes`.

- [ ] **Step 2: Run the combined fixture, assert exactly one consolidated log line listing every problem**

```bash
output=$(go run ./relay/cmd/relay --config relay/cmd/relay/smoke/attachments-combined.yaml 2>&1 || true)
# Must contain every problem substring:
for substr in "anonymous" "not-a-cidr" "action_on_exceeded" "storage.mode" "max_size_bytes"; do
  echo "$output" | grep -q "$substr" && echo "OK: combined matched '$substr'" || { echo "FAIL: missing '$substr'"; exit 1; }
done
# And exactly one consolidated line:
log_count=$(echo "$output" | grep -c "relay: startup config errors" || true)
[ "$log_count" -eq 1 ] && echo "OK: exactly one consolidated line" || { echo "FAIL: got $log_count lines"; exit 1; }
```

Expected: every substring matched + exactly one consolidated log line — operator fixes all five problems in one restart cycle.

- [ ] **Step 3: Record the captured output in the phase-6 README §7.1 evidence section (held until Task 14)**

Save the raw output to a scratch file (`/tmp/q9-smoke.txt` or `$env:TEMP\q9-smoke.txt`) for transcription into the README §7.1 evidence block during Task 14. No commit here — the evidence section is updated in one batch at the end.

---

### Task 4: Caps-discovery smoke (self-runnable)

**Files:** none new

- [ ] **Step 1: Start the fake-llm + a noop webhook receiver**

The fake-llm is a Phase 5 artifact (`relay/cmd/fake-llm/main.go`). The webhook receiver is the `startWebhookReceiver()` helper inside `drive-attachments.ts`; for this caps-only step, any HTTP listener on `:19099` that returns 200 is sufficient (or skip — caps discovery only calls `/init`, never `/submit`).

```bash
# Terminal 1 — fake-llm (already built in Phase 5):
go run ./relay/cmd/fake-llm --addr :11434

# Terminal 2 — relay (Enabled=true):
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/attachments-enabled.yaml
```

- [ ] **Step 2: Assert /init returns capabilities.attachments when Enabled=true**

```bash
curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | jq '.capabilities.attachments'
```

Expected:
```json
{
  "max_size_bytes": 5242880,
  "max_total_bytes": 10485760,
  "allowed_mime_types": ["image/png", "image/jpeg", "image/webp"]
}
```

- [ ] **Step 3: Stop the relay, restart with Enabled=false, assert caps omitted**

```bash
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/attachments-disabled.yaml &
sleep 1
curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | jq '.capabilities'
```

Expected: `capabilities` object has no `attachments` key (omitted via `omitempty` on the pointer field; nil pointer → JSON omits the field).

- [ ] **Step 4: Empty-allowlist arm — restart with Enabled=true but `allowed_mime_types: []`**

Edit `attachments-enabled.yaml` in-place to `allowed_mime_types: []` (or use a temp file), restart, assert caps omitted. Then revert.

Expected: `capabilities.attachments` omitted (the `computeAttachmentsCaps` intersection returns `nil` when the allowlist is empty, per Phase 6 README §7.3).

- [ ] **Step 5: Stop the relay; record outputs to the scratch file (Task 14 transcribes them into the README evidence)**

---

### Task 5: Validation smokes via `drive-attachments.ts` (Enabled arm)

**Files:** none new

- [ ] **Step 1: Start fake-llm + relay with `attachments-enabled.yaml`**

```bash
go run ./relay/cmd/fake-llm --addr :11434 &
FAKE_PID=$!
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/attachments-enabled.yaml &
RELAY_PID=$!
sleep 1
```

- [ ] **Step 2: Run the Enabled-arm driver**

```bash
RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts enabled
```

Expected: all `OK: ...` lines; final line `✓ All Phase 6 attachment smokes passed for this arm.` The arm includes:
- caps-discovery assertion (matches Task 4 — driver double-verifies)
- forward smoke with 1×1 PNG → 200 + webhook body assertion
- 6 MB → 413 `attachment_too_large`
- two 6 MB → 413 `attachment_too_large` (first-encountered)
- three 4 MB → 413 `attachments_exceed_total`
- declared PNG + JPEG bytes → 415 `attachment_mime_mismatch`
- `image/heic` → 415 `attachment_mime_not_allowed`
- non-data URL → 400 `attachment_malformed`
- `type:"file"` → 400 `attachment_type_unsupported`

- [ ] **Step 3: Stop relay + fake-llm**

```bash
kill $RELAY_PID $FAKE_PID
```

- [ ] **Step 4: Save the driver output to the scratch file (transcribed in Task 14)**

---

### Task 6: Webhook forward smoke (self-runnable)

**Files:** none new

The Enabled arm of `drive-attachments.ts` already exercises this (step 2 above asserts `webhook receiver captured a body` and `attachments[0].url matches submitted data: URL verbatim`). Task 6 is the bookkeeping step that confirms the assertion fired and records the captured webhook body separately for the README evidence.

- [ ] **Step 1: Re-run the Enabled arm with verbose webhook logging**

Add a temporary `console.log('webhook captured body:', recv.lastBody?.slice(0, 1000))` just before the assertion in the driver, re-run, capture the body. Revert the driver before commit.

- [ ] **Step 2: Verify the captured body contains the verbatim data: URL**

The captured body MUST contain:
```
"attachments":[{"type":"screenshot","mime_type":"image/png","url":"data:image/png;base64,iVBORw0KGgo...","label":"shot"}]
```

No mutation, no re-encoding — the relay must pass the data: URL through unchanged. This is the L015 "derived-field" guard for the webhook adapter: the webhook adapter has zero new code in Phase 6 (the pass-through is implicit via `json.Marshal(p)`), and this smoke proves the implicit behavior holds.

- [ ] **Step 3: Save the captured webhook body to the scratch file (transcribed in Task 14)**

---

### Task 7: Adapter native-sequence test re-run

**Files:** none new (re-runs the unit tests authored in 6-ii)

- [ ] **Step 1: Re-run with `-run TestAttachments` filter to capture evidence**

```bash
cd c:/src/ai/intake/relay
go test -v -race -run TestAttachments ./internal/adapter/... 2>&1 | tee /tmp/attachments-tests.txt
```

Expected: every adapter's `TestAttachments_*` test passes:
- `TestAttachments_Webhook_PassesThroughVerbatim` (no new sequencing — JSON pass-through)
- `TestAttachments_Chatwoot_InlinedInConversationCreate` (asserts `attachments[]` in the conversation-create body)
- `TestAttachments_Fider_MarkdownInDescription` (asserts `![<label>](data:image/png;base64,...)` substring)
- `TestAttachments_Linear_AssetUploadPrecedesIssueCreate` (asserts N asset-upload POSTs BEFORE issueCreate; orphan-prevention)
- `TestAttachments_Zendesk_UploadsThenTicket` (asserts N `/uploads.json` POSTs share one token; ticket body includes `uploads:[<token>]`)
- The failure-injection variants (linear/zendesk return error BEFORE create call) per 6-ii's plan.

- [ ] **Step 2: Save the `go test -v` output to the scratch file**

The full `go test -v` output (PASS lines + per-test names) is recorded as evidence — it documents that every adapter's native-sequence test passes under the consolidated Phase 6 chain.

---

### Task 8: PAUSE + live Chatwoot smoke

**Files:** none new (uses `examples/vue-anonymous` from Phase 3 unchanged)

- [ ] **Step 1: PAUSE for maintainer go-ahead**

In the subagent or executor session, BEFORE proceeding to Step 2, output verbatim:

> **PAUSE FOR MAINTAINER:** the next step requires `CHATWOOT_TOKEN`, `CHATWOOT_INBOX_ID`, `CHATWOOT_ACCOUNT_ID` in the environment (same vars used in Phase 3's live smoke). Do NOT proceed automatically. Wait for explicit maintainer go-ahead.

WAIT for the maintainer to type "go" (or equivalent). Do NOT proceed without explicit confirmation. This is the ONLY step in Phase 6 that consumes a real credential.

- [ ] **Step 2: Maintainer spins up `examples/vue-anonymous` with chatwoot routing**

Configure the relay with `cfg.Adapters.Chatwoot.Enabled=true` and `cfg.Routing.DefaultAdapter="chatwoot"` plus the Phase 3 env vars resolved into the chatwoot adapter's config. Start the relay; start `examples/vue-anonymous` (the Vue widget served via `npm run dev`).

```bash
# Terminal 1 — relay with chatwoot routing
export CHATWOOT_TOKEN="<maintainer-supplied>"
export CHATWOOT_INBOX_ID="<maintainer-supplied>"
export CHATWOOT_ACCOUNT_ID="<maintainer-supplied>"
cd c:/src/ai/intake
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/chatwoot-live.yaml 2>&1 | tee /tmp/chatwoot-live.log

# Terminal 2 — vue widget (the existing Phase 3 example, no Phase 6 modifications)
cd examples/vue-anonymous
npm run dev
```

The `chatwoot-live.yaml` fixture is the Phase 3 live-smoke fixture (already present in `relay/cmd/relay/smoke/` from Phase 3's smoke); Phase 6 adds the `attachments:` block to it inline for the live smoke (Enabled=true, defaults).

- [ ] **Step 3: Maintainer performs the UI flow**

In the browser at `http://localhost:5173`:
1. Click the widget bubble to open the intake panel.
2. Type a one-line message (e.g. "smoke test 6-iv attachment").
3. Click **Attach**. The `ScreenshotRedactor` modal opens with the captured page.
4. Draw one rectangle over a visible region of the page (any region — proves the redaction overlay).
5. Click **Save**. The attachment strip shows one thumbnail.
6. Click **Submit**.

- [ ] **Step 4: Maintainer verifies in Chatwoot UI**

Open `https://app.chatwoot.com/app/accounts/<CHATWOOT_ACCOUNT_ID>/conversations`. Find the new conversation. **Expected:**
- The conversation appears in the configured inbox.
- The user message text is visible.
- The screenshot is rendered inline as an attachment.
- The redaction rectangle is visible in the saved image (solid black fill where the user drew it).

- [ ] **Step 5: L005 confirmation — grep the relay log for the chatwoot token**

```bash
grep -c "$CHATWOOT_TOKEN" /tmp/chatwoot-live.log || echo "0 matches"
```

Expected: `0 matches`. If any match, L005 has a hole in the new Phase 6 attachment-related chatwoot code — STOP and patch before proceeding.

- [ ] **Step 6: Save the redacted screenshot (or a description of it) + the conversation URL + the grep output to the scratch file**

The README §7.1 evidence section will reference the conversation ID and the L005-clean grep output.

---

### Task 9: Phase 1+4+5 regression smokes

**Files:** none new (re-runs existing drivers)

- [ ] **Step 1: Start relay with `attachments-enabled.yaml` but extend it for the regression smokes**

The Phase 1/4/5 drivers (`drive-auth-email.ts`, `drive-auth-sso.ts`, `drive-abuse.ts`) all assume their own relay configs. For the regression check the relay needs to be configured with:
- Phase 4's auth modes (email + sso enabled) for `drive-auth-email.ts` and `drive-auth-sso.ts`
- Phase 5's per-IP + per-session + budget caps for `drive-abuse.ts`
- Phase 6's `attachments.enabled: true` for the body-cap raise + caps emission

Use the Phase 4 + Phase 5 existing YAMLs as a base, add the `attachments:` block from `attachments-enabled.yaml`, save as `phase-6-regression.yaml`. (This is a one-off composite fixture for the regression run; not checked in — the existing per-phase YAMLs cover the per-phase smokes.)

- [ ] **Step 2: Run `drive-auth-email.ts` (Phase 4) under the Phase 6 chain**

```bash
go run ./relay/cmd/fake-llm --addr :11434 &
FAKE_PID=$!
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/phase-6-regression.yaml &
RELAY_PID=$!
sleep 1

RELAY_URL=http://127.0.0.1:18080 \
MAILHOG_URL=http://192.168.1.102:8025 \
SMOKE_EMAIL=pete@mantichor.com \
npx tsx core/smoke/drive-auth-email.ts
```

Expected: Phase 4 email driver completes without error — proves Phase 6 middleware ordering + body-cap raise did NOT regress live email auth.

- [ ] **Step 3: Run `drive-auth-sso.ts` (Phase 4) under the Phase 6 chain**

```bash
RELAY_URL=http://127.0.0.1:18080 \
npx tsx core/smoke/drive-auth-sso.ts
```

Expected: SSO driver completes without error.

- [ ] **Step 4: Run `drive-abuse.ts` (Phase 5) under the Phase 6 chain**

```bash
RELAY_URL=http://127.0.0.1:18080 \
npx tsx core/smoke/drive-abuse.ts
```

Expected: all `OK: ...` lines; final line `✓ All Phase 5 abuse smokes passed.` Proves Phase 6's chain did not regress per-IP / per-session / daily-budget gates.

- [ ] **Step 5: Stop relay + fake-llm; save all three driver outputs to the scratch file**

---

### Task 10: Body-cap regression (Disabled arm)

**Files:** none new

- [ ] **Step 1: Restart relay with `attachments-disabled.yaml`**

```bash
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/attachments-disabled.yaml &
RELAY_PID=$!
sleep 1
```

- [ ] **Step 2: Run the Disabled arm of `drive-attachments.ts`**

```bash
RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts disabled
```

Expected: all `OK: ...` lines including:
- `init omits capabilities.attachments when Enabled=false`
- `attachments with Enabled=false returns 400` + code `attachments_disabled`
- `2 MB body with Enabled=false returns 413` + code `request_body_too_large` — the 1 MB body cap is preserved when attachments are off.

- [ ] **Step 3: Stop relay; save the driver output to the scratch file**

---

### Task 11: Author `docs/attachments.md`

**Files:** Create `docs/attachments.md`

- [ ] **Step 1: Write the operator-facing doc**

Create `docs/attachments.md`:

```markdown
# Attachments

Phase 6 ships screenshot attachments end-to-end through the relay's existing `/v1/intake/submit` endpoint. This document describes the operator configuration, the per-adapter behavior, and the widget UI flow.

## Operator configuration

Add an `attachments:` block to your `relay.yaml`:

```yaml
attachments:
  enabled: true                                              # default true
  max_size_bytes: 5242880                                    # default 5 MB (per attachment)
  max_total_bytes: 10485760                                  # default 10 MB (aggregate per request)
  allowed_mime_types: ["image/png", "image/jpeg", "image/webp"]  # default
  storage:
    mode: "forward"                                          # only "" or "forward" supported in v0
```

All fields are optional; defaults apply when omitted. The defaults match the values published in `/v1/intake/init`'s `capabilities.attachments` block, which the widget reads to decide whether to show the Attach button.

### Startup-fatal misconfigurations

The relay's Q9-style consolidated startup gate (`main.go startupProblems`) refuses to start when:

- `attachments.storage.mode` is set to any value other than `""` or `"forward"`. v0 stores nothing locally — every attachment is forwarded to the chosen adapter via that adapter's native upload mechanism. The hook exists so v1's S3-backed mode can be added without a schema bump.
- `attachments.max_size_bytes > attachments.max_total_bytes`. A per-attachment cap larger than the aggregate cap is unreachable — a single attachment would always trip the aggregate cap first. The check forces operators to fix the inconsistency before startup.

Both problems are surfaced in the same `relay: startup config errors` log line as Phase 5's CIDR/duration/CAPTCHA misconfigs — one consolidated line, fix everything in one restart cycle.

### Body-cap behavior

When `attachments.enabled: true`, the `/v1/intake/submit` `MaxBytesReader` cap rises from 1 MB to 14 MB (10 MB aggregate × 1.37 base64 overhead + ~200 KB headroom for the JSON envelope). When `enabled: false`, the cap stays at 1 MB — operators who disabled attachments are not exposed to the wider attack surface.

A body that exceeds the cap returns:

```json
{"error": {"code": "request_body_too_large", "message": "submission body exceeds limit"}}
```

with HTTP 413.

## Validation errors

The relay validates every attachment in two layers. First, JSON-schema rules (existing since Phase 0) confirm the field shape. Second, the new `attachvalidate` package decodes the `data:` URL, magic-byte-matches the bytes against the declared `mime_type`, and enforces the per-attachment + aggregate caps:

| HTTP | Code | When |
|---|---|---|
| 413 | `request_body_too_large` | Request body exceeds `MaxBytesReader` (14 MB / 1 MB). |
| 413 | `attachment_too_large` | One attachment's raw bytes exceed `max_size_bytes`. |
| 413 | `attachments_exceed_total` | Sum of all attachment raw bytes exceeds `max_total_bytes`. |
| 415 | `attachment_mime_not_allowed` | Declared `mime_type` not in the published allowlist. |
| 415 | `attachment_mime_mismatch` | `net/http.DetectContentType` on the bytes returns a MIME different from the declared one. |
| 400 | `attachment_malformed` | `url` is not a `data:` URL, or base64 decode fails. |
| 400 | `attachment_type_unsupported` | `type` is not `"screenshot"` (v0 scope). Schema permits `"file"` for v1+; the validator rejects it in v0. |
| 400 | `attachments_disabled` | Non-empty `attachments[]` but `attachments.enabled: false`. |

No `Retry-After` is set on any of these — they are user-action errors with no time-based remediation (per Phase 5's RFC 9110 stance).

## Per-adapter behavior

Each enabled adapter handles `p.Attachments` inside its existing `Create()` method using that downstream system's native upload mechanism. The frozen `adapter.Adapter` interface is unchanged.

| Adapter | Sequence | Notes |
|---|---|---|
| **webhook** | JSON pass-through. `p.Attachments` is serialized verbatim into the POST body via `json.Marshal(p)`. | Receiver is responsible for handling the `data:` URL. Zero behavior change in `webhook.go` beyond the new `Capabilities()` method. |
| **chatwoot** | Existing two-call flow (`createContact` → `createConversation`). The conversation-create body gains an inline `attachments[]` field with `[{file_type, data_url}]`. Single transaction — either succeeds entirely or fails entirely. | The body switches between JSON and multipart based on whether attachments are present; see L020 for the discipline (branch body format on attachment-presence, never on something derivable later). |
| **fider** | `renderBody(p)` appends `\n\n![<label or "screenshot N">](data:image/png;base64,...)` per attachment to the post description. Markdown rendering can't fail; if a Fider deployment's sanitizer strips data: URLs, the post still carries all conversation text (graceful degradation). | No additional roundtrips. |
| **linear** | For each attachment: POST raw bytes to the Linear file-upload endpoint → receive asset URL. Then `issueCreate` references the asset URLs. Upload BEFORE create — failure returns error before `issueCreate`, so no orphan issue. | N additional roundtrips per request (one per attachment). |
| **zendesk** | For each attachment: POST raw bytes to `/api/v2/uploads.json` (subsequent uploads pass `?token=<first-token>` to share one token). Then `ticket-create` includes `uploads: [<token>]`. Upload BEFORE create — same orphan-prevention as Linear. | N additional roundtrips per request. Zendesk garbage-collects unattached uploads after 3 days. |

If routing picks an adapter that doesn't accept a given MIME type, the adapter silently drops the attachment with a `slog.Warn` (graceful per-adapter pattern). Operators wanting strict guarantees should configure `allowed_mime_types` to the intersection by hand.

## Capabilities discovery

`/v1/intake/init` returns the relay's published attachment capabilities:

```json
{
  "session_id": "...",
  "capabilities": {
    "auth_modes": ["anonymous"],
    "streaming": true,
    "attachments": {
      "max_size_bytes": 5242880,
      "max_total_bytes": 10485760,
      "allowed_mime_types": ["image/png", "image/jpeg", "image/webp"]
    }
  }
}
```

The `attachments` block is the **union** of every enabled adapter's `Capabilities().AcceptedMIMETypes`, **intersected** with `cfg.attachments.allowed_mime_types`. When the intersection is empty (or `cfg.attachments.enabled: false`), the block is omitted entirely (`omitempty` on the pointer). The widget reads this and hides the Attach button when the block is absent.

## Widget UI flow

The Vue 3 widget (`vue/src/components/IntakeWidget.vue`) ships these new components:

- **`ScreenshotRedactor.vue`** — Full-screen modal overlay opened by clicking Attach. The current page is captured via `html2canvas` (DI-injected through `core/src/capture.ts setHtml2Canvas(fn)` for SSR-safety + testability — see L021), and rendered to a `<canvas>` element. The user draws solid-fill rectangles with the mouse to redact sensitive regions; Save commits the canvas → base64 PNG into the pending attachments list, Cancel discards.
- **`AttachmentStrip.vue`** — Thumbnail strip showing the pending attachments with per-thumb remove buttons and an aggregate-size badge. The Submit button stays enabled when the aggregate is under cap.

The user flow:

1. Widget mounts → calls `/init` → reads `capabilities.attachments`.
2. If the block is null, the Attach button is hidden entirely. Otherwise it is visible.
3. User clicks Attach → `html2canvas(document.body)` captures the host page → `ScreenshotRedactor` modal opens with the canvas.
4. User draws zero or more rectangles → clicks Save → canvas is converted to a base64 PNG and added to the pending strip.
5. User clicks Submit → `IntakeClient.submit()` includes the `attachments[]` array in the POST body; the relay validates, dispatches to one adapter, and the adapter forwards via its native sequence.

Errors mapped to user-readable strings (see `useIntake.ts`):

| Code | Banner text |
|---|---|
| `attachment_too_large` | "Screenshot too large — try a smaller region." |
| `attachments_exceed_total` | "Too many attachments — remove one." |
| `attachment_mime_not_allowed` | "This attachment type isn't supported." |
| `attachment_mime_mismatch` | "This attachment couldn't be verified — try recapturing." |
| `attachment_malformed` | "This attachment couldn't be verified — try recapturing." |
| `attachment_type_unsupported` | "This attachment type isn't supported." |
| `attachments_disabled` | "Attachments are disabled on this server." |
| `request_body_too_large` | "Your submission is too large to send." |

The widget never shows raw relay error messages for attachment paths — only the curated user-readable strings above.

## Cross-references

- Validator: `relay/internal/attachvalidate/` (6-i)
- Adapter `Capabilities()` helpers: `relay/internal/adapter/capabilities.go` + each adapter's `<name>.go` (6-i + 6-ii)
- Widget capture: `core/src/capture.ts` (6-iii)
- Widget pending-state + size accounting: `core/src/attachments.ts` (6-iii)
- Redaction modal: `vue/src/components/ScreenshotRedactor.vue` (6-iii)
- Attachment strip: `vue/src/components/AttachmentStrip.vue` (6-iii)
- Design spec: `docs/specs/2026-05-28-phase-6-attachments-design.md`
- Phase README: `ai/tasks/phase-6/README.md`
```

- [ ] **Step 2: Commit**

```bash
git add docs/attachments.md
git commit -m "docs(6-iv): docs/attachments.md — operator config + per-adapter matrix + widget flow"
```

---

### Task 12: Update `docs/PROJECT.md` §11 cross-references

**Files:** Modify `docs/PROJECT.md`

- [ ] **Step 1: Append implementation cross-refs to §11**

In `docs/PROJECT.md` §11 ("Attachment handling"), at the end of each existing sub-section (Capture / Transport / Forwarding), add an `Implementation:` line pointing at the Phase 6 packages:

Under **Capture**:
> Implementation: `core/src/capture.ts` (DI-injectable `html2canvas` wrapper), `vue/src/components/ScreenshotRedactor.vue` (modal + rectangle redaction), `vue/src/components/AttachmentStrip.vue` (pending strip + aggregate-size badge).

Under **Transport**:
> Implementation: `relay/internal/attachvalidate/` (magic-byte + size-cap validation), `relay/internal/server/submit.go` (orchestration: body-cap raise to 14 MB when `cfg.Attachments.Enabled=true`, `attachvalidate.ValidateAll` after `payloadbuild.Build`, before `Router.Route`), `relay/internal/server/init.go` (capabilities intersection emitted under `capabilities.attachments`).

Under **Forwarding**:
> Implementation: `relay/internal/adapter/capabilities.go` (optional `CapableAdapter` interface), per-adapter `Create()` native sequences in `relay/internal/adapter/{webhook,chatwoot,fider,linear,zendesk}/`. Operator-facing reference: `docs/attachments.md`.

- [ ] **Step 2: Commit**

```bash
git add docs/PROJECT.md
git commit -m "docs(6-iv): PROJECT.md §11 cross-refs to Phase 6 implementation packages"
```

---

### Task 13: LESSONS.md additions

**Files:** Modify `ai/LESSONS.md`

- [ ] **Step 1: Append L020, L021, and any L022+ for live-smoke snags**

Append these entries (verbatim) after the existing L019:

```markdown
### L020: Branch outbound body format (multipart vs JSON) on attachment-presence at the call site — never on something derivable later

Chatwoot's `POST /api/v1/accounts/{id}/conversations` accepts either a JSON body (text-only conversation create) or a multipart body (when the conversation create carries inline attachments via the `attachments[]` form field). Phase 6's chatwoot adapter switches between the two body formats based on whether `p.Attachments` is non-empty. The natural-but-wrong design is to construct a generic "request object" with optional attachments, then late-bind the body format somewhere deep in the HTTP plumbing (e.g. an interceptor that introspects the request and chooses the encoder). That works until a refactor moves the introspection point past a layer that has already serialized the body — then the multipart path silently degrades to JSON-with-base64, the Chatwoot server rejects it (or accepts it but stores the data: URL as a literal string in the message body), and you don't notice until the screenshot doesn't render in the conversation UI.

**Where it hit:** Phase 6-ii chatwoot adapter design (the live smoke would have caught it as "screenshot not visible in conversation"). The fix is to make the branch explicit at the call site: `if len(atts) > 0 { postMultipart(...) } else { postJSON(...) }`. Two named code paths, two test names, no introspection.

**Rule:** When an outbound request body has TWO content types based on a request-shape predicate (attachment-presence, batch-vs-single, streaming-vs-buffered), branch on that predicate AT THE CALL SITE with a named function per branch. Do NOT thread the predicate through a generic builder that decides format later — every layer between the predicate and the encoder is a place a refactor can lose the signal. Each branch gets its own httptest unit test asserting the Content-Type AND the body shape. Reference: `relay/internal/adapter/chatwoot/chatwoot.go` `createConversation` vs `createConversationWithAttachments`; tests `TestChatwoot_Create_NoAttachments_JSONBody`, `TestChatwoot_Create_WithAttachments_MultipartBody`.

---

### L021: SSR-safe browser APIs need dependency injection at construction, NOT lazy module-level imports

`html2canvas` is a browser-only library. Importing it at the top of `core/src/capture.ts` works in the browser bundle but breaks SSR (Vite SSR, Nuxt, Astro, the Vue test-utils `mount` with `global.stubs`), because the import-time side effects touch `window` / `document` / `Image` / etc. Two anti-patterns to avoid: (a) `const html2canvas = typeof window === 'undefined' ? null : require('html2canvas')` — module-level `require` in a TS ESM module is a build-time error in modern tooling; the `typeof window` check runs only at first import and gets cached. (b) `let h2c: any; if (typeof window !== 'undefined') import('html2canvas').then(m => h2c = m.default)` — race condition; first `capturePage()` call may run before the dynamic import resolves; tests that mock `window` AFTER the module imports see the wrong value.

**The clean answer:** dependency-inject the capture function at construction. `core/src/capture.ts` exports `setHtml2Canvas(fn)` and `capturePage()`. Production code calls `setHtml2Canvas(html2canvas)` once at widget bootstrap (inside an `if (typeof window !== 'undefined')` guard that protects the import statement itself via a dynamic `await import('html2canvas')`). Tests call `setHtml2Canvas(stubFn)` to inject a stub canvas — no real library load, no `window` touched.

**Where it hit:** Phase 6-iii widget design. The Vue test-utils mount step for `ScreenshotRedactor.spec.ts` failed under jsdom because the real `html2canvas` import touched `Image.prototype.crossOrigin` which jsdom doesn't fully implement. The DI rewrite made the test trivially passable AND fixed an unrelated SSR-build warning that would have shipped silently into the v1 Nuxt example.

**Rule:** For any browser-only dependency that the widget loads (canvas APIs, ResizeObserver polyfills, Notifications, Service Workers, IndexedDB), inject the capability through a single `setX(fn)` setter and a single `getX()` accessor. The production widget call site is the ONLY place that imports the real module — and it imports it dynamically (`await import('lib')`) inside an `if (typeof window !== 'undefined')` guard. Tests inject stubs through the setter. This pattern also makes "swap to a different capture engine for v1" a one-line config change rather than a refactor. Reference: `core/src/capture.ts` `setHtml2Canvas` + `capturePage`; tests `capture.test.ts` stub-injection cases.

---

### L022: <one-line title — fill in if a live-smoke snag surfaced>

<paragraph describing what went wrong + how it was caught + the rule going forward>

**Where it hit:** Phase 6-iv live chatwoot smoke (commit `<commit-id>`).

Reference: `<file>:<line>`; tests `<TestName>`.
```

> **If the live chatwoot smoke surfaces no novel snag**, DELETE the L022 placeholder block before committing. The minimum for Phase 6 is L020 + L021. If the live smoke surfaces TWO novel snags, append L022 AND L023 using the same template.

- [ ] **Step 2: Commit**

```bash
git add ai/LESSONS.md
git commit -m "docs(6-iv): LESSONS L020+L021 — multipart-vs-JSON branching + html2canvas SSR-safety DI"
```

---

### Task 14: Phase 6 README evidence section + sub-plan status updates

**Files:** Modify `ai/tasks/phase-6/README.md`

- [ ] **Step 1: Update §3 sub-plan index status column**

In the §3 sub-plan index table, change the `Status` column for **all four sub-plans** from `Not started` (or whatever in-progress state they are in) to `Live + smoked`. The expected final table:

```markdown
| # | Plan | Driver | Effort | Status |
|---|---|---|---|---|
| 6-i | [Config + InitResponse caps + attachvalidate + body-cap + Q9 gate + adapter Capabilities()](6-i-config-attachvalidate-seam-plan.md) | the seam | M | Live + smoked |
| 6-ii | [Per-adapter native forwarding (chatwoot/fider/linear/zendesk/webhook)](6-ii-adapters-plan.md) | adapter implementations | M-L | Live + smoked |
| 6-iii | [Widget capture + redaction modal + attachment strip + DTO wiring](6-iii-widget-capture-redact-plan.md) | widget UX | M | Live + smoked |
| 6-iv | [Final live chatwoot smoke + Phase 1/4/5 regressions + docs + LESSONS](6-iv-smoke-docs-plan.md) | live evidence | S | Live + smoked |
```

- [ ] **Step 2: Append §7.1 "Smoke status (YYYY-MM-DD)" subsection**

After the existing §7 block (the smoke recipe), add a `### 7.1 Smoke status (YYYY-MM-DD)` subsection that mirrors Phase 5's evidence format. Transcribe the captured outputs from the scratch file in this exact order — one fenced code block per smoke, with a one-line verdict above each:

```markdown
### 7.1 Smoke status (YYYY-MM-DD)

All eight Phase 6 final-smoke steps pass.

#### 1. Q9 startup smoke

Verdict: PASS — every misconfig fixture exits 1 with the consolidated log line; combined fixture lists every problem in one line.

```
=== Q9 smoke: attachments-bad-storage-mode ===
OK: consolidated error line present
... (paste from scratch file)
```

#### 2. Caps-discovery smoke

Verdict: PASS — Enabled=true emits caps; Enabled=false omits caps; empty allowlist omits caps.

```
$ curl ... /v1/intake/init  (Enabled=true)
{"max_size_bytes":5242880, "max_total_bytes":10485760, "allowed_mime_types":["image/png","image/jpeg","image/webp"]}

$ curl ... /v1/intake/init  (Enabled=false)
... (no attachments key in capabilities)
```

#### 3. Validation smokes (drive-attachments.ts, Enabled arm)

Verdict: PASS — every sentinel returns its documented HTTP code.

```
$ RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts enabled
... (paste full OK lines from scratch file)
✓ All Phase 6 attachment smokes passed for this arm.
```

#### 4. Forward smoke

Verdict: PASS — webhook receiver captured the attachment with the data: URL verbatim and the declared mime_type.

```
webhook captured body: {"attachments":[{"type":"screenshot","mime_type":"image/png","url":"data:image/png;base64,iVBORw0KGgo...","label":"shot"}],...}
```

#### 5. Adapter native-sequence tests

Verdict: PASS — every adapter's TestAttachments_* test passes including the orphan-prevention regressions.

```
=== RUN   TestAttachments_Webhook_PassesThroughVerbatim
--- PASS: TestAttachments_Webhook_PassesThroughVerbatim (0.00s)
=== RUN   TestAttachments_Chatwoot_InlinedInConversationCreate
--- PASS: TestAttachments_Chatwoot_InlinedInConversationCreate (0.00s)
... (paste full go test -v output from scratch file)
PASS
ok   intake/internal/adapter/...
```

#### 6. Live chatwoot attachment smoke

Verdict: PASS — conversation appears in inbox with screenshot inline and redaction visible; chatwoot token absent from relay log.

- Conversation URL: `https://app.chatwoot.com/app/accounts/<account>/conversations/<id>`
- Redaction visible: yes (one black rectangle over <region>)
- Token-leak grep: `0 matches` (L005 confirmation)

#### 7. Phase 1+4+5 regression

Verdict: PASS — drive-auth-email.ts, drive-auth-sso.ts, drive-abuse.ts all complete unchanged under the Phase 6 chain.

```
... (paste each driver's final OK line from scratch file)
```

#### 8. Body-cap regression (Disabled arm)

Verdict: PASS — 2 MB body with Enabled=false returns 413 request_body_too_large; the 1 MB cap is preserved when attachments are off.

```
$ RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts disabled
OK: init omits capabilities.attachments when Enabled=false
OK: attachments with Enabled=false returns 400
OK: Enabled=false code is attachments_disabled
OK: 2 MB body with Enabled=false returns 413
OK: 2 MB body with Enabled=false code is request_body_too_large
✓ All Phase 6 attachment smokes passed for this arm.
```
```

Fill in the actual `YYYY-MM-DD` date, the actual captured outputs from the scratch file, and the actual conversation URL.

- [ ] **Step 3: Commit**

```bash
git add ai/tasks/phase-6/README.md
git commit -m "docs(phase-6): live smoke evidence + sub-plan status updates; phase 6 done"
```

---

### Task 15: Final green-bar verification

**Files:** none new

- [ ] **Step 1: Go build / vet / test**

```bash
cd c:/src/ai/intake/relay
go build ./...
go vet ./...
go test -race ./...
```

Expected: all three exit 0; race detector clean.

- [ ] **Step 2: TS type-check / build / test**

```bash
cd c:/src/ai/intake/core
npm run type-check
npm run build
npm run test
```

```bash
cd c:/src/ai/intake/vue
npm run type-check
npm run build
npm run test
```

Expected: all six exit 0.

- [ ] **Step 3: Contract + pins + tidy**

```bash
cd c:/src/ai/intake
bash scripts/verify-contract.sh
bash scripts/check-pins.sh
cd relay && go mod tidy && cd .. && git diff relay/go.mod relay/go.sum
```

Expected: `verify-contract.sh` exits 0 (no schema changes in Phase 6 → no codegen drift); `check-pins.sh` exits 0 (the new `html2canvas` pin check from 6-iii must already be in `scripts/check-pins.sh`); `go mod tidy` produces an empty diff (Phase 6 introduces zero new Go modules — validation uses stdlib `net/http.DetectContentType` + `encoding/base64`).

- [ ] **Step 4: Confirm Phase 6 build-fail checklist holds**

Re-read `ai/tasks/phase-6/README.md` §6 and tick every item against the captured evidence. All items must hold; any failure here is a regression that must be fixed before merge.

- [ ] **Step 5: No commit — verification only**

This task is the green-bar gate; if anything fails, fix the underlying issue and re-run from the affected task (do NOT skip or paper over). Once everything is green, Phase 6 is ready for the bundled phase-6 → main merge.

---

## Smoke (mandatory)

For 6-iv the smoke IS the deliverable. The authoritative smoke recipe is `ai/tasks/phase-6/README.md` §7 (the eight numbered steps). 6-iv's job is to execute every step in order, capture the evidence, and record it in §7.1 of that README. The done-criteria below are the verification.

## Done criteria

- [ ] All 15 tasks complete and committed.
- [ ] `cd relay && go build ./... && go vet ./... && go test -race ./...` green.
- [ ] `cd core && npm run type-check && npm run build && npm run test` green.
- [ ] `cd vue && npm run type-check && npm run build && npm run test` green.
- [ ] `bash scripts/verify-contract.sh` exits 0 (no schema changes in Phase 6).
- [ ] `bash scripts/check-pins.sh` exits 0 (`html2canvas` exact-pin check passes).
- [ ] `cd relay && go mod tidy` is a no-op (Phase 6 adds zero Go modules).
- [ ] Q9 startup smoke: each of `attachments-bad-storage-mode.yaml`, `attachments-cap-inverted.yaml` exits 1 with the consolidated `relay: startup config errors` log line listing the matching problem; `attachments-combined.yaml` emits exactly ONE consolidated log line listing every Phase-5 + Phase-6 problem.
- [ ] Caps-discovery smoke: Enabled=true → `/init` returns `capabilities.attachments` with `[png,jpeg,webp]`; Enabled=false → block omitted; empty allowlist → block omitted.
- [ ] `drive-attachments.ts enabled` arm: all `OK:` lines for caps + forward + 7 validation sentinels.
- [ ] Forward smoke: webhook receiver body contains the submitted data: URL verbatim and `mime_type: "image/png"`.
- [ ] Adapter native-sequence tests: every `TestAttachments_*` test PASSes including the orphan-prevention regressions for linear + zendesk and the L005 token-echo guard for zendesk.
- [ ] **Live chatwoot smoke executed with explicit maintainer go-ahead**: Chatwoot conversation contains the screenshot inline with the redaction rectangle visible; `grep` over the relay log for the chatwoot token returns 0 matches (L005 confirmation).
- [ ] Phase 1+4+5 regression: `drive-auth-email.ts`, `drive-auth-sso.ts`, `drive-abuse.ts` all pass unchanged under the Phase 6 chain with `cfg.Attachments.Enabled=true` and empty `attachments[]`.
- [ ] Body-cap regression (`drive-attachments.ts disabled` arm): 2 MB body with `Enabled=false` returns 413 `request_body_too_large`; the 1 MB cap is preserved when attachments are off.
- [ ] `docs/attachments.md` written and committed.
- [ ] `docs/PROJECT.md` §11 cross-refs updated to point at `relay/internal/attachvalidate/`, `core/src/capture.ts`, `vue/src/components/ScreenshotRedactor.vue`, `vue/src/components/AttachmentStrip.vue`.
- [ ] `ai/LESSONS.md` has at least L020 (multipart-vs-JSON branching) and L021 (html2canvas SSR-safety DI); L022+ appended if any live-smoke snag surfaced.
- [ ] `ai/tasks/phase-6/README.md` §3 sub-plan index shows `Live + smoked` for all four sub-plans.
- [ ] `ai/tasks/phase-6/README.md` §7.1 "Smoke status (YYYY-MM-DD)" subsection populated with the full captured evidence (one fenced block per smoke).
- [ ] Phase 6 build-fail checklist (§6) re-checked end-to-end against the captured evidence.
- [ ] Phase 6 ready for the bundled phase-6 → main merge.
