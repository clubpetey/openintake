/**
 * Phase 4 email-magic-link smoke driver.
 *
 * Drives the full email-mode flow against a relay running with auth.modes.email=true
 * and a MailHog (or Mailpit) SMTP sink:
 *
 *   1. POST /v1/intake/init                                    → session_id
 *   2. POST /v1/intake/auth/email/start { email }              → 200 message_sent
 *   3. Poll MailHog /api/v2/messages for the captured email,
 *      parse out the 6-digit code from the body
 *   4. POST /v1/intake/auth/email/verify { email, code }       → { token, expires_at, user }
 *   5. POST /v1/intake/turn (×2) with Authorization: Bearer    → SSE streams
 *   6. POST /v1/intake/submit                                  → SubmitResponse
 *   7. Assert /auth/email/verify response shape (user.verified=true) and a
 *      successful /submit (external_id + adapter_name present); SessionContext-
 *      AuthMode propagation is covered by the server-side test suite.
 *
 * Usage:
 *   RELAY_URL=http://localhost:8099 \
 *   MAILHOG_URL=http://192.168.1.102:8025 \
 *   SMOKE_EMAIL=pete@mantichor.com \
 *   npx tsx core/smoke/drive-auth-email.ts
 *
 * Defaults: RELAY_URL=http://localhost:8080, MAILHOG_URL=http://localhost:8025.
 *
 * Browser-global stubs (LESSONS L004): /submit calls IntakeClient.submit which
 * uses captureClient() reading window/navigator/document. This script stubs
 * them via Object.defineProperty before calling submit() — plain assignment to
 * globalThis.navigator throws in Node 24.
 */

import { IntakeClient } from '../src/index.js';
import type { ChatMessage } from '../src/index.js';

const RELAY_URL   = process.env['RELAY_URL']   ?? 'http://localhost:8080';
const MAILHOG_URL = process.env['MAILHOG_URL'] ?? 'http://localhost:8025';
const SMOKE_EMAIL = process.env['SMOKE_EMAIL'] ?? 'pete@mantichor.com';

// --- Browser-global stubs for submit() (LESSONS L004) ---
function stubBrowserGlobals(): void {
  const defs: Array<[string, unknown]> = [
    ['window', { location: { href: 'http://localhost:5173/smoke' }, innerWidth: 1280, innerHeight: 720 }],
    ['navigator', { userAgent: 'intake-smoke/drive-auth-email', language: 'en-US' }],
    ['document', {
      referrer: '',
      title: 'intake email smoke',
      querySelectorAll: () => [] as never[],
    }],
  ];
  for (const [name, value] of defs) {
    Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  }
}

async function clearMailHogInbox(): Promise<void> {
  // MailHog v1 delete-all endpoint:
  await fetch(`${MAILHOG_URL}/api/v1/messages`, { method: 'DELETE' });
}

async function pollMailHogForCode(email: string, timeoutMs = 15_000): Promise<string> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const resp = await fetch(`${MAILHOG_URL}/api/v2/messages`);
    if (resp.ok) {
      const data = (await resp.json()) as {
        items: Array<{
          To: Array<{ Mailbox: string; Domain: string }>;
          Content: { Body: string };
        }>;
      };
      for (const msg of data.items ?? []) {
        const to = msg.To.map((t) => `${t.Mailbox}@${t.Domain}`).join(',');
        if (to.toLowerCase().includes(email.toLowerCase())) {
          const body = msg.Content.Body;
          // The relay's email body has a line like "Your intake verification code is: 123456"
          const m = /\b(\d{6})\b/.exec(body);
          if (m && m[1]) {
            return m[1];
          }
        }
      }
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`mailhog poll timed out after ${timeoutMs}ms — no email for ${email} found`);
}

async function startEmail(email: string): Promise<void> {
  const resp = await fetch(`${RELAY_URL}/v1/intake/auth/email/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  });
  if (!resp.ok) {
    const txt = await resp.text();
    throw new Error(`POST /auth/email/start ${resp.status}: ${txt}`);
  }
  const j = (await resp.json()) as { message_sent: boolean };
  if (!j.message_sent) {
    throw new Error(`POST /auth/email/start: message_sent=false (body=${JSON.stringify(j)})`);
  }
}

async function verifyEmail(
  email: string,
  code: string,
): Promise<{ token: string; expiresAt: string }> {
  const resp = await fetch(`${RELAY_URL}/v1/intake/auth/email/verify`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, code }),
  });
  if (!resp.ok) {
    const txt = await resp.text();
    throw new Error(`POST /auth/email/verify ${resp.status}: ${txt}`);
  }
  const j = (await resp.json()) as {
    token: string;
    expires_at: string;
    user: { email: string; verified: boolean };
  };
  if (!j.token) throw new Error('verify response missing token');
  if (j.user.email !== email) throw new Error(`verify user.email = ${j.user.email}; want ${email}`);
  if (!j.user.verified) throw new Error('verify user.verified = false');
  return { token: j.token, expiresAt: j.expires_at };
}

async function main(): Promise<void> {
  console.log(`[email-smoke] relay=${RELAY_URL} mailhog=${MAILHOG_URL} email=${SMOKE_EMAIL}`);

  // 0. Clear MailHog inbox so we don't read a stale code.
  await clearMailHogInbox();
  console.log('[email-smoke] mailhog inbox cleared');

  // 1. Init (issues a session_id; the email flow authenticates separately but the
  //    relay still requires /init to have been called for capabilities advertising).
  const client = new IntakeClient({
    relayUrl: RELAY_URL,
    widgetVersion: '0.1.0-email-smoke',
    appContext: { smoke: true, driver: 'drive-auth-email' },
  });
  const init = await client.init();
  console.log(`[email-smoke] init session_id=${init.session_id}`);
  if (!init.capabilities.auth_modes.includes('email')) {
    throw new Error(
      `init.capabilities.auth_modes = ${JSON.stringify(init.capabilities.auth_modes)}; want includes "email"`,
    );
  }
  console.log(`[email-smoke] capabilities.auth_modes=${JSON.stringify(init.capabilities.auth_modes)}`);

  // 2. Request a verification code.
  await startEmail(SMOKE_EMAIL);
  console.log('[email-smoke] /auth/email/start OK');

  // 3. Poll MailHog for the captured code.
  const code = await pollMailHogForCode(SMOKE_EMAIL);
  console.log(`[email-smoke] mailhog captured code=${code}`);

  // 4. Verify the code → bearer JWT.
  const { token, expiresAt } = await verifyEmail(SMOKE_EMAIL, code);
  console.log(`[email-smoke] /auth/email/verify OK; token expires=${expiresAt}`);

  // 5. Drive 2 turns with the bearer token attached.
  client.setBearerToken(token);
  const history: ChatMessage[] = [];
  for (let i = 0; i < 2; i++) {
    const userMsg =
      i === 0
        ? "I can't reset my password — the reset email never arrives."
        : 'Tried Chrome and Firefox on Windows. Sender domain is your.com.';
    history.push({ role: 'user', content: userMsg });

    process.stdout.write(`[email-smoke] turn ${i + 1} [assistant] `);
    const deltas: string[] = [];
    const tokens = await client.turn(history, (d: string) => {
      process.stdout.write(d);
      deltas.push(d);
    });
    process.stdout.write('\n');
    if (tokens.input_tokens <= 0 || tokens.output_tokens <= 0) {
      throw new Error(
        `turn ${i + 1}: zero token counts (input=${tokens.input_tokens}, output=${tokens.output_tokens})`,
      );
    }
    history.push({ role: 'assistant', content: deltas.join('') });
  }

  // 6. Submit and assert the verified user fields propagate.
  //    Stub browser globals first (LESSONS L004 — /submit calls captureClient()).
  stubBrowserGlobals();
  const result = await client.submit(history);
  console.log(`[email-smoke] submit external_id=${result.external_id} adapter=${result.adapter_name}`);

  console.log('[email-smoke] PASS');
}

main().catch((err: unknown) => {
  console.error('[email-smoke] FAIL:', err instanceof Error ? err.message : err);
  process.exit(1);
});
