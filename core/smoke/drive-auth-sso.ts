/**
 * Phase 4 host-app SSO smoke driver.
 *
 * Mirrors drive-auth-email.ts but consumes a pre-minted host-app JWT instead
 * of running the email magic-link flow. Use this to validate the relay's SSO
 * verifier end-to-end against a real Auth0 (or other IdP) RS256 token.
 *
 *   1. POST /v1/intake/init                                    → session_id
 *   2. setBearerToken(<the externally minted JWT>)
 *   3. POST /v1/intake/turn (×2)                               → SSE streams
 *   4. POST /v1/intake/submit                                  → SubmitResponse
 *   5. (Visual) Inspect the webhook receiver — the canonical
 *      payload's user.{auth_mode,id,email,verified} fields are
 *      the load-bearing proof.
 *
 * Usage:
 *   $env:RELAY_URL = "http://localhost:8099"
 *   $env:INTAKE_SSO_TOKEN = "<paste real RS256 access token>"
 *   npx tsx core/smoke/drive-auth-sso.ts
 *
 * Defaults: RELAY_URL=http://localhost:8080.
 *
 * Browser-global stubs (LESSONS L004) applied before submit() so
 * IntakeClient.submit's captureClient() produces valid client.* fields.
 */

import { IntakeClient } from '../src/index.js';
import type { ChatMessage } from '../src/index.js';

const RELAY_URL = process.env['RELAY_URL'] ?? 'http://localhost:8080';
const TOKEN     = process.env['INTAKE_SSO_TOKEN'] ?? '';

const USER_TURNS: string[] = [
  "Hi — I'd like to report that exports from the dashboard are coming through empty since the last release.",
  "I'm on Chrome 124 / macOS. The CSV downloads, but it has only the header row.",
];

function stubBrowserGlobals(): void {
  const defs = {
    window: { location: { href: 'http://localhost:5173/sso-smoke' }, innerWidth: 1280, innerHeight: 720 },
    navigator: { userAgent: 'intake-smoke/drive-auth-sso', language: 'en-US' },
    document: {
      referrer: '',
      title: 'intake sso smoke',
      querySelectorAll: () => [] as never[],
    },
  };
  for (const [name, value] of Object.entries(defs)) {
    Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  }
}

async function main(): Promise<void> {
  if (!TOKEN) {
    throw new Error('INTAKE_SSO_TOKEN env var is required (paste a real RS256 access token from your IdP)');
  }
  console.log(`[sso-smoke] relay=${RELAY_URL} token=<${TOKEN.length} chars>`);

  const client = new IntakeClient({
    relayUrl: RELAY_URL,
    widgetVersion: '0.1.0-sso-smoke',
    appContext: { smoke: true, driver: 'drive-auth-sso' },
  });

  // 1. Init — issues a session_id (carried in X-Intake-Session on subsequent
  //    requests; the dispatcher reads it into SessionContext.SessionID even
  //    on bearer paths, so the canonical payload's client.session_id is valid).
  const init = await client.init();
  console.log(`[sso-smoke] init session_id=${init.session_id}`);
  if (!init.capabilities.auth_modes.includes('sso')) {
    throw new Error(`init.capabilities.auth_modes = ${JSON.stringify(init.capabilities.auth_modes)}; want includes "sso"`);
  }
  console.log(`[sso-smoke] capabilities.auth_modes=${JSON.stringify(init.capabilities.auth_modes)}`);

  // 2. Attach the externally minted bearer.
  client.setBearerToken(TOKEN);
  console.log('[sso-smoke] bearer attached');

  // 3. Drive turns with the bearer (the relay's sso verifier validates iss/aud/exp,
  //    pins alg=RS256, fetches the JWKS, and maps claims into SessionContext).
  const history: ChatMessage[] = [];
  for (let i = 0; i < USER_TURNS.length; i++) {
    const userMsg = USER_TURNS[i];
    history.push({ role: 'user', content: userMsg });

    process.stdout.write(`[sso-smoke] turn ${i + 1} [assistant] `);
    const deltas: string[] = [];
    const tokens = await client.turn(history, (d: string) => {
      process.stdout.write(d);
      deltas.push(d);
    });
    process.stdout.write('\n');
    if (tokens.input_tokens <= 0 || tokens.output_tokens <= 0) {
      throw new Error(`turn ${i + 1}: zero token counts (input=${tokens.input_tokens}, output=${tokens.output_tokens})`);
    }
    history.push({ role: 'assistant', content: deltas.join('') });
  }

  // 4. Submit — the canonical payload posted to the configured adapter carries
  //    user.{auth_mode:"sso", id:<sub>, email:<email>?, verified:true}.
  stubBrowserGlobals();
  const result = await client.submit(history);
  console.log(`[sso-smoke] submit complete: external_id=${result.external_id} adapter=${result.adapter_name}`);

  console.log('[sso-smoke] PASS');
  console.log('[sso-smoke] inspect the webhook receiver to confirm user.{auth_mode,id,verified}');
}

main().catch((err: unknown) => {
  console.error('[sso-smoke] FAIL:', err instanceof Error ? err.message : err);
  process.exit(1);
});
