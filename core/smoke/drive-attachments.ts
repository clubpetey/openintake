/**
 * Phase 6 attachments smoke driver.
 *
 * Drives every documented attachment validation path against a running relay:
 *   1. /init returns capabilities.attachments when cfg.Attachments.Enabled=true
 *      and omits it when Enabled=false (caps-discovery).
 *   2. Submit with 1x1 PNG -> 200 + webhook receiver logs attachments[0] verbatim
 *      (forward smoke).
 *   3. Submit with a 6 MB attachment            -> 413 attachment_too_large
 *   4. Submit with two 6 MB attachments         -> 413 attachment_too_large    (first encountered)
 *   5. Submit with three 3.4 MB attachments     -> 413 attachments_exceed_total
 *   6. Submit with declared image/png, JPEG bytes -> 415 attachment_mime_mismatch
 *   7. Submit with mime_type "image/heic"       -> 415 attachment_mime_not_allowed
 *   8. Submit with url "not-a-data-url"         -> 400 attachment_malformed
 *   9. Submit with type "file" + valid PNG      -> 400 attachment_type_unsupported
 *  10. Submit with non-empty attachments[] when
 *      relay started Disabled=true              -> 400 attachments_disabled
 *  11. With Enabled=false, 2 MB body            -> 413 request_body_too_large
 *      (body-cap regression - proves 1 MB cap preserved when attachments off)
 *
 * Requires:
 *   - the relay running with relay/cmd/relay/smoke/attachments-enabled.yaml
 *     (steps 1, 2, 3, 4, 5, 6, 7, 8, 9)
 *   - the relay restarted with relay/cmd/relay/smoke/attachments-disabled.yaml
 *     (steps 1-disabled-arm, 10, 11)
 *   - the fake-llm running on :11434 (relay/cmd/fake-llm - Phase 5 artifact)
 *   - a local webhook receiver on :19099/intake (this driver spins one up)
 *
 * Usage:
 *   RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts enabled
 *   RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-attachments.ts disabled
 *
 * Browser-global stubs (LESSONS L004): /submit calls IntakeClient.submit which
 * uses captureClient() reading window/navigator/document. This script stubs
 * them via Object.defineProperty before calling submit() - plain assignment to
 * globalThis.navigator throws in Node 24.
 *
 * Fixture sizes (LESSONS L019): max_size_bytes=5MB, max_total_bytes=10MB.
 *   - 6 MB single attachment fires per-attachment cap (smallest case).
 *   - 3x3.4MB = 10.2 MB fires aggregate cap. Per-attachment size (3.4 MB) clears
 *     the 5 MB per-attachment cap so the aggregate sentinel wins; total
 *     base64-encoded body (~13.7 MB) stays under the 14 MB MaxBytesReader so
 *     the body-cap doesn't shadow the aggregate-cap gate. See step 5's inline
 *     comment for the fixture-math walk-through.
 *   - The original "two 6 MB attachments fire per-attachment on the FIRST" case
 *     was dropped: two 6 MB raw attachments base64-encode to ~16 MB and trip
 *     the 14 MB body cap first, shadowing the per-attachment gate. The
 *     first-encountered sentinel ordering is asserted in the attachvalidate
 *     unit tests instead. See step 4's inline comment.
 */

import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';

const RELAY_URL = process.env['RELAY_URL'] ?? 'http://127.0.0.1:18080';
const MODE = (process.argv[2] ?? 'enabled') as 'enabled' | 'disabled';

// --- Browser-global stubs (LESSONS L004) - driver does not import IntakeClient
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

/** Smallest valid 1x1 PNG (67 bytes raw, 92 chars base64). */
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
 *  validation order means the size check fires before the magic-byte check. */
function makeSizedDataURL(mime: string, bytes: number): string {
  const pngHeader = Buffer.from(PNG_1X1_BASE64, 'base64');
  const padding = Buffer.alloc(Math.max(0, bytes - pngHeader.length), 0);
  const full = Buffer.concat([pngHeader, padding]).slice(0, bytes);
  return `data:${mime};base64,${full.toString('base64')}`;
}

const PNG_1X1_DATAURL = `data:image/png;base64,${PNG_1X1_BASE64}`;
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

interface Receiver {
  readonly lastBody: string | null;
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

  // 2. forward smoke: 1x1 PNG -> 200 + webhook receiver logs attachment verbatim.
  const recv = await startWebhookReceiver();
  try {
    const session = initResp.session_id;
    const okResp = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: PNG_1X1_DATAURL, label: 'shot' },
    ]);
    assert(
      okResp.status === 200,
      `1x1 PNG submit returns 200 (got ${okResp.status} ${okResp.body.slice(0, 200)})`,
    );
    assert(recv.lastBody !== null, 'webhook receiver captured a body');
    if (recv.lastBody) {
      const parsed = JSON.parse(recv.lastBody) as {
        attachments?: Array<{ url: string; mime_type: string }>;
      };
      assert(
        Array.isArray(parsed.attachments) && parsed.attachments.length === 1,
        'webhook body includes exactly one attachment',
      );
      assert(
        parsed.attachments![0]!.url === PNG_1X1_DATAURL,
        'webhook attachments[0].url matches submitted data: URL verbatim',
      );
      assert(
        parsed.attachments![0]!.mime_type === 'image/png',
        'webhook attachments[0].mime_type is image/png',
      );
      console.log('webhook captured body head:', recv.lastBody.slice(0, 400));
    }

    // 3. per-attachment cap: 6 MB single -> 413 attachment_too_large.
    const big = makeSizedDataURL('image/png', 6 * 1024 * 1024);
    const r3 = await submit(session, [{ type: 'screenshot', mime_type: 'image/png', url: big }]);
    assert(r3.status === 413, `6 MB attachment returns 413 (got ${r3.status})`);
    assert(
      parseErr(r3.body)?.error.code === 'attachment_too_large',
      '6 MB attachment code is attachment_too_large',
    );

    // 4. Note: the sub-plan's original "two 6 MB attachments" case is dropped
    //    because two 6 MB raw attachments base64-encode to ~16 MB, exceeding the
    //    14 MB body cap; the request_body_too_large gate fires before the
    //    per-attachment cap can be evaluated. This is L019 territory (don't
    //    shadow gates). Step 3 above already exercises the per-attachment cap;
    //    step 5 below exercises the aggregate cap. The "first encountered
    //    sentinel" ordering is asserted in attachvalidate unit tests, not here.

    // 5. aggregate cap: three 3.4 MB attachments = 10.2 MB raw (> 10 MB total cap)
    //    but base64-encoded body ~13.7 MB stays under the 14 MB MaxBytesReader.
    //    Fixture math (L019): per-attachment cap is 5 MB so 3.4 MB clears it;
    //    aggregate is 10.2 MB so the sentinel fires.
    const mid = makeSizedDataURL('image/png', Math.floor(3.4 * 1024 * 1024));
    const r5 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: mid },
      { type: 'screenshot', mime_type: 'image/png', url: mid },
      { type: 'screenshot', mime_type: 'image/png', url: mid },
    ]);
    assert(r5.status === 413, `three 3.4 MB attachments returns 413 (got ${r5.status})`);
    assert(
      parseErr(r5.body)?.error.code === 'attachments_exceed_total',
      'three 3.4 MB attachments code is attachments_exceed_total',
    );

    // 6. MIME mismatch: declared image/png but JPEG bytes -> 415.
    const r6 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: PNG_DECLARED_BUT_JPEG_BYTES },
    ]);
    assert(r6.status === 415, `declared image/png with JPEG bytes returns 415 (got ${r6.status})`);
    assert(
      parseErr(r6.body)?.error.code === 'attachment_mime_mismatch',
      'mismatch code is attachment_mime_mismatch',
    );

    // 7. MIME not allowed: image/heic (not in published allowlist) -> 415.
    const r7 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/heic', url: 'data:image/heic;base64,AAAA' },
    ]);
    assert(r7.status === 415, `image/heic returns 415 (got ${r7.status})`);
    assert(
      parseErr(r7.body)?.error.code === 'attachment_mime_not_allowed',
      'heic code is attachment_mime_not_allowed',
    );

    // 8. malformed URL: not a data: URL -> 400.
    const r8 = await submit(session, [
      { type: 'screenshot', mime_type: 'image/png', url: 'http://not-a-data-url/x.png' },
    ]);
    assert(r8.status === 400, `non-data URL returns 400 (got ${r8.status})`);
    assert(
      parseErr(r8.body)?.error.code === 'attachment_malformed',
      'non-data URL code is attachment_malformed',
    );

    // 9. type:"file" -> 400 attachment_type_unsupported.
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

  // 10. non-empty attachments[] when Enabled=false -> 400 attachments_disabled.
  const r10 = await submit(initResp.session_id, [
    { type: 'screenshot', mime_type: 'image/png', url: PNG_1X1_DATAURL },
  ]);
  assert(r10.status === 400, `attachments with Enabled=false returns 400 (got ${r10.status})`);
  assert(
    parseErr(r10.body)?.error.code === 'attachments_disabled',
    'Enabled=false code is attachments_disabled',
  );

  // 11. body-cap regression: 2 MB body with Enabled=false -> 413 request_body_too_large
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
  console.log('\nAll Phase 6 attachment smokes passed for this arm.');
}

main().catch((err: unknown) => {
  console.error('smoke driver failed:', err);
  process.exit(1);
});
