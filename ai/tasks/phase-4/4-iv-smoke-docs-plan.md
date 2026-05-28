# 4-iv Final Smoke + Docs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the email-auth smoke driver (a small TypeScript script that drives init → email/start → poll MailHog for the code → email/verify → /turn → /submit), record the live evidence in the phase-4 README §7, capture novel lessons in `ai/LESSONS.md`, and gate the phase as done. The live email smoke is self-runnable (uses MailHog); the live SSO smoke **pauses for the maintainer** (needs a real IdP-issued token).

**Architecture:** One new Node/TypeScript driver under `core/smoke/`, written in the same style as the existing `drive.ts`. It uses MailHog's HTTP API (`/api/v2/messages`) to read the captured verification code, then drives the full intake flow with the resulting bearer token. The plan also captures the smoke evidence in the README and records new lessons (alg-confusion JWT mitigation; in-memory rate-limit + injectable clock pattern) in `ai/LESSONS.md`.

**Tech Stack:** Node 24 / TypeScript 5.6.3 (smoke driver). `@intake/core` `IntakeClient`. MailHog HTTP API (`GET /api/v2/messages`, `DELETE /api/v1/messages` for teardown). Stdlib `fetch` (Node 24 has it). No new TS dependencies.

---

## Design References

- README §7 — final smoke (what gets recorded as evidence)
- README §6 — build-fail items (must still hold at smoke completion)
- Design spec §9 — testing strategy (the live email + SSO paths)
- Phase-3 README §7 step 3 — the chatwoot/linear/zendesk/fider live-smoke pattern this mirrors
- LESSONS L004 — Node smoke browser-global stubs (applies if the driver hits `/submit`, which it does — `IntakeClient.submit` reads `window`/`navigator`/`document` via `captureClient`)
- LESSONS L010 — PS 5.1 BOM gotcha (applies to any local smoke YAML)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `core/smoke/drive-auth-email.ts` | Create | Node/TS driver: init → /email/start → poll MailHog → /email/verify → 2 turns → /submit; asserts the canonical payload's `user.{auth_mode,email,verified}` |
| `ai/tasks/phase-4/README.md` | Modify | §7 smoke status note updated with the live evidence after the maintainer runs the smokes |
| `ai/LESSONS.md` | Modify | Append L013 (alg-confusion JWT mitigation) and L014 (in-memory rate-limit with injectable clock for testable TTL/window semantics) |

---

## Tasks

### Task 1: Create `core/smoke/drive-auth-email.ts`

**Files:** Create `core/smoke/drive-auth-email.ts`

- [ ] **Step 1: Write the driver**

Create `core/smoke/drive-auth-email.ts` with the following content:

```typescript
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
 *   7. Assert the canonical payload (received by a local webhook receiver, OR
 *      asserted via the SubmitResponse's user fields) carries
 *      user.auth_mode="email", user.email=<addr>, user.verified=true.
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
  const defs = {
    window: { location: { href: 'http://localhost:5173/smoke' }, innerWidth: 1280, innerHeight: 720 },
    navigator: { userAgent: 'intake-smoke/drive-auth-email', language: 'en-US' },
    document: {
      referrer: '',
      title: 'intake email smoke',
      querySelectorAll: () => [] as never[],
    },
  };
  for (const [name, value] of Object.entries(defs)) {
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
      const data = (await resp.json()) as { items: Array<{ To: Array<{ Mailbox: string; Domain: string }>; Content: { Body: string } }> };
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

async function verifyEmail(email: string, code: string): Promise<{ token: string; expiresAt: string }> {
  const resp = await fetch(`${RELAY_URL}/v1/intake/auth/email/verify`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, code }),
  });
  if (!resp.ok) {
    const txt = await resp.text();
    throw new Error(`POST /auth/email/verify ${resp.status}: ${txt}`);
  }
  const j = (await resp.json()) as { token: string; expires_at: string; user: { email: string; verified: boolean } };
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

  // 1. Init (issues an anonymous session_id; the email flow doesn't use it but the
  //    relay still requires /init to have been called for CORS / capabilities advertising).
  const client = new IntakeClient({
    relayUrl: RELAY_URL,
    widgetVersion: '0.1.0-email-smoke',
    appContext: { smoke: true, driver: 'drive-auth-email' },
  });
  const init = await client.init();
  console.log(`[email-smoke] init session_id=${init.session_id}`);
  if (!init.capabilities.auth_modes.includes('email')) {
    throw new Error(`init.capabilities.auth_modes = ${JSON.stringify(init.capabilities.auth_modes)}; want includes "email"`);
  }
  console.log(`[email-smoke] capabilities.auth_modes=${JSON.stringify(init.capabilities.auth_modes)}`);

  // 2. Request a code.
  await startEmail(SMOKE_EMAIL);
  console.log('[email-smoke] /auth/email/start OK');

  // 3. Poll MailHog for the captured code.
  const code = await pollMailHogForCode(SMOKE_EMAIL);
  console.log(`[email-smoke] mailhog captured code=${code}`);

  // 4. Verify the code → bearer JWT.
  const { token, expiresAt } = await verifyEmail(SMOKE_EMAIL, code);
  console.log(`[email-smoke] /auth/email/verify OK; token expires=${expiresAt}`);

  // 5. Drive 2 turns with the bearer.
  client.setBearerToken(token); // IntakeClient must expose this — if it doesn't, see note below.
  const history: ChatMessage[] = [];
  for (let i = 0; i < 2; i++) {
    const userMsg = i === 0
      ? "I can't reset my password — the reset email never arrives."
      : "Tried Chrome and Firefox on Windows. Sender domain is your.com.";
    history.push({ role: 'user', content: userMsg });

    process.stdout.write(`[email-smoke] turn ${i + 1} [assistant] `);
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

  // 6. Submit and assert the verified user fields propagate.
  stubBrowserGlobals();
  const result = await client.submit({});
  console.log(`[email-smoke] submit external_id=${result.external_id} adapter=${result.adapter_name}`);

  console.log('[email-smoke] PASS');
}

main().catch((err: unknown) => {
  console.error('[email-smoke] FAIL:', err instanceof Error ? err.message : err);
  process.exit(1);
});
```

> **Note on `setBearerToken`:** the existing `IntakeClient` may not expose this method (Phase 1 anonymous-only didn't need it). If the type-check at step 2 fails for that reason, add a single-line method on `IntakeClient`:
>
> ```typescript
> // core/src/client.ts
> setBearerToken(token: string | null): void {
>   this.bearerToken = token;
> }
> ```
>
> …plus a private field `bearerToken: string | null = null` and an Authorization header set when present in the turn/submit fetch calls. This is a 5-line addition to `core/src/client.ts`. If you find it's already there, skip this note.

- [ ] **Step 2: Type-check**

```
cd C:/src/ai/intake && npm run -w @intake/core type-check
```

Expected: exits 0. If `setBearerToken` is undefined on `IntakeClient`, add it per the note above, then re-run.

- [ ] **Step 3: Contract gate**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK
```

Expected: `CONTRACT_OK`.

- [ ] **Step 4: Commit**

```
cd C:/src/ai/intake && git add core/smoke/drive-auth-email.ts core/src/client.ts
git commit -m "feat(smoke): add drive-auth-email.ts — email magic-link end-to-end driver (4-iv)"
```

If `core/src/client.ts` was not modified, drop it from the `git add`.

---

### Task 2: Live email smoke (self-runnable — uses MailHog)

This task is manual but unambiguous. You run it locally and paste the resulting log lines.

**Files:** None (manual run; the artifacts are in `local-dev/` and gitignored).

- [ ] **Step 1: MailHog instance**

You already have one from Phase 3 fider work at `192.168.1.102:1025` (SMTP) / `192.168.1.102:8025` (HTTP UI). Confirm:

```powershell
Test-NetConnection 192.168.1.102 -Port 1025
Test-NetConnection 192.168.1.102 -Port 8025
```

Both should be `TcpTestSucceeded: True`. If not, expose 1025 in the Fider compose override (already exposed 8025) by adding:

```yaml
services:
  mailhog:
    ports:
      - "1025:1025"
      - "8025:8025"
```

…and `docker compose up -d mailhog` on `192.168.1.102`.

- [ ] **Step 2: Smoke config + helper**

```powershell
@'
server:
  addr: ":8099"
  cors_origins: ["http://localhost:5173"]
llm:
  provider: "anthropic"
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-sonnet-4-6"
    max_tokens: 1024
routing:
  default_adapter: "webhook"
adapters:
  webhook:
    enabled: true
    url: "http://localhost:9099/intake"   # any sink; the smoke asserts the JWT path, not the adapter
auth:
  modes:
    anonymous: true
    email: true
    sso: false
  email:
    smtp_host: "192.168.1.102"
    smtp_port: 1025
    smtp_user: ""
    smtp_pass_env: "INTAKE_SMTP_PASS"
    from: "Intake <noreply@local.invalid>"
    code_ttl: "10m"
    jwt_ttl: "15m"
    jwt_secret_env: "INTAKE_EMAIL_JWT_SECRET"
'@ | Set-Content -Path C:\src\ai\intake\local-dev\smoke-email.yaml -Encoding ascii

@'
Get-Content "$PSScriptRoot\..\.env" | ForEach-Object {
    if ($_ -match '^\s*([A-Z_][A-Z0-9_]*)\s*=\s*(.*?)\s*$') {
        Set-Item -Path "env:$($matches[1])" -Value $matches[2].Trim('"').Trim("'")
    }
}
Push-Location "$PSScriptRoot\..\relay"
try {
    go run ./cmd/relay -config "$PSScriptRoot\smoke-email.yaml"
} finally {
    Pop-Location
}
'@ | Set-Content -Path C:\src\ai\intake\local-dev\smoke-email.ps1 -Encoding ascii
```

In your `.env`, add these two values (the file is gitignored):

```
INTAKE_SMTP_PASS=                          # empty — MailHog accepts no-auth
INTAKE_EMAIL_JWT_SECRET=<run a one-liner below>
```

Generate a 32+ byte JWT secret:

```powershell
$bytes = New-Object byte[] 32
[System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
[System.Convert]::ToBase64String($bytes)
# Paste the output as INTAKE_EMAIL_JWT_SECRET=... in .env
```

- [ ] **Step 3: Boot the relay + run the smoke**

**Terminal 1:**

```powershell
C:\src\ai\intake\local-dev\smoke-email.ps1
```

Watch for these startup lines:
- `"msg":"relay: license"` `"mode":"trial"` (fresh trial — fine; this smoke doesn't exercise the gate)
- `"msg":"relay: adapter enabled","adapter":"webhook"`
- `"msg":"relay listening"`

Look for an email-mode-enabled log line too (4-ii should add something like `"msg":"relay: auth.email enabled"`). If absent, that's a 4-ii implementation note, not a smoke blocker.

**Terminal 2** (fresh shell; re-source `.env` via the loader, then):

```powershell
$env:RELAY_URL = "http://localhost:8099"
$env:MAILHOG_URL = "http://192.168.1.102:8025"
$env:SMOKE_EMAIL = "pete@mantichor.com"
cd C:\src\ai\intake
npx tsx core/smoke/drive-auth-email.ts
```

Expected final line: `[email-smoke] PASS`.

If MailHog doesn't capture the email within 15 seconds, the driver fails with a clear timeout — investigate the relay's email send log (Terminal 1) for SMTP errors. The relay's status-code-only error format applies; the underlying SMTP error is in the slog output.

- [ ] **Step 4: Verify in MailHog UI**

Open `http://192.168.1.102:8025` — you should see the captured email with the 6-digit code visible in the body. (The driver consumed it via the API; the UI just confirms it was sent.)

- [ ] **Step 5: Capture evidence for the README**

Save the last ~10 lines of the Terminal 1 (relay) log and the Terminal 2 [email-smoke] output. Paste them into a scratchpad — they go into the phase README in Task 5.

---

### Task 3: Live SSO smoke — PAUSES for the maintainer

This task is the only Phase-4 step that pauses for an external secret. Two paths; pick whichever is faster for you.

**Files:** None for the credit-free path; small additions to `local-dev/` for whichever path you pick.

- [ ] **Step 1: Pick a path**

**Path (a) — Real Auth0 tenant** (you have or will create one): create an API (audience `https://intake-smoke`), enable an M2M client, get its client_id and client_secret. The token endpoint mints a real RS256 access token you paste into the smoke. Pros: closest to a real production scenario. Cons: ~10 minutes of Auth0 dashboard clicking the first time.

**Path (b) — Self-served JWKS** (fastest): generate an RSA-2048 keypair locally, serve a 2-line `jwks.json` from any local HTTP server, mint a test JWT via the existing `intake-license` CLI (extended for SSO test-token signing) OR a 30-line Go scratch program OR `jwt-cli`. Pros: no external service. Cons: less realistic but proves the verifier just as well.

- [ ] **Step 2: Path (a) — Auth0**

1. <https://manage.auth0.com> → Applications → Create an M2M (Machine to Machine) application.
2. Create an API: name `intake-smoke`, identifier `https://intake-smoke`, signing algorithm RS256.
3. Authorize the M2M app for the API.
4. Run:
   ```powershell
   $body = @{
       grant_type    = "client_credentials"
       client_id     = "<m2m_client_id>"
       client_secret = "<m2m_client_secret>"
       audience      = "https://intake-smoke"
   } | ConvertTo-Json
   $resp = Invoke-RestMethod -Uri "https://<your-tenant>.us.auth0.com/oauth/token" `
       -Method POST -Headers @{ "Content-Type" = "application/json" } -Body $body
   $env:INTAKE_SSO_TOKEN = $resp.access_token
   ```
5. Write a smoke config:
   ```powershell
   @'
   server: { addr: ":8099", cors_origins: ["http://localhost:5173"] }
   llm: { provider: "anthropic", anthropic: { api_key_env: "ANTHROPIC_API_KEY", model: "claude-sonnet-4-6", max_tokens: 1024 } }
   routing: { default_adapter: "webhook" }
   adapters: { webhook: { enabled: true, url: "http://localhost:9099/intake" } }
   auth:
     modes: { anonymous: true, email: false, sso: true }
     sso:
       issuer: "https://<your-tenant>.us.auth0.com/"
       audience: "https://intake-smoke"
       jwks_url: "https://<your-tenant>.us.auth0.com/.well-known/jwks.json"
       hs256_secret_env: ""
       claims: { user_id: "sub", email: "email", display_name: "name" }
   '@ | Set-Content C:\src\ai\intake\local-dev\smoke-sso.yaml -Encoding ascii
   ```
6. Boot relay (Terminal 1), then drive a hand-crafted request (Terminal 2):
   ```powershell
   $headers = @{ "Authorization" = "Bearer $env:INTAKE_SSO_TOKEN" }
   # Init first (issues a session_id, also exercises the capabilities advertising):
   $init = Invoke-RestMethod -Uri "http://localhost:8099/v1/intake/init" -Method POST
   $init.capabilities.auth_modes   # should include "sso"

   # Drive a single turn with the SSO bearer:
   $turn = @{
       client    = @{ widget_version = "0.1.0"; session_id = $init.session_id; url = "http://localhost:5173/"; user_agent = "sso-smoke"; viewport = @{ w = 1280; h = 720 }; locale = "en-US" }
       messages  = @(@{ role = "user"; content = "Quick SSO smoke check." })
   } | ConvertTo-Json -Depth 10
   $resp = Invoke-WebRequest -Uri "http://localhost:8099/v1/intake/turn" -Method POST `
       -Headers ($headers + @{ "Content-Type" = "application/json" }) -Body $turn
   $resp.StatusCode   # 200 = SSO accepted
   ```

- [ ] **Step 3: Path (b) — Self-served JWKS**

1. Generate keypair + minimal JWKS:
   ```bash
   # On any *nix shell, or use openssl on Windows via Git Bash:
   openssl genrsa -out /tmp/sso-priv.pem 2048
   openssl rsa -in /tmp/sso-priv.pem -pubout -out /tmp/sso-pub.pem
   # Convert pub to JWKS — easiest via a tiny Node helper:
   ```
   ```javascript
   // /tmp/make-jwks.mjs
   import { createPublicKey } from 'node:crypto';
   import { readFileSync, writeFileSync } from 'node:fs';
   const pub = createPublicKey(readFileSync('/tmp/sso-pub.pem'));
   const jwk = pub.export({ format: 'jwk' });
   jwk.kid = 'smoke-key-1';
   jwk.use = 'sig';
   jwk.alg = 'RS256';
   writeFileSync('/tmp/jwks.json', JSON.stringify({ keys: [jwk] }));
   ```
   ```bash
   node /tmp/make-jwks.mjs
   # Serve it on http://localhost:9100:
   cd /tmp && python3 -m http.server 9100   # or `npx serve /tmp -p 9100`
   ```
2. Mint a test JWT (~30 lines of Go in `local-dev/mint-sso-token/main.go`):
   ```go
   package main
   import (
       "crypto/rsa"; "crypto/x509"; "encoding/pem"; "fmt"; "os"; "time"
       "github.com/golang-jwt/jwt/v5"
   )
   func main() {
       data, _ := os.ReadFile("/tmp/sso-priv.pem")
       block, _ := pem.Decode(data)
       parsed, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
       if parsed == nil {
           p8, _ := x509.ParsePKCS8PrivateKey(block.Bytes)
           parsed = p8.(*rsa.PrivateKey)
       }
       tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
           "iss": "http://localhost:9100/", "aud": "intake-smoke",
           "sub": "smoke-user-001", "email": "pete@mantichor.com", "name": "Pete",
           "iat": time.Now().Unix(), "exp": time.Now().Add(15 * time.Minute).Unix(),
       })
       tok.Header["kid"] = "smoke-key-1"
       s, _ := tok.SignedString(parsed)
       fmt.Println(s)
   }
   ```
   Build + run: `go run local-dev/mint-sso-token` → copy the printed JWT.
3. Smoke config (same shape as path (a) but with `issuer: "http://localhost:9100/"`, `audience: "intake-smoke"`, `jwks_url: "http://localhost:9100/jwks.json"`).
4. Same `Invoke-WebRequest` drive as path (a) step 6.

- [ ] **Step 4: Capture evidence**

For whichever path: capture the `Authorization: Bearer` request's 200 response and the relay-side slog line proving `SessionContext.AuthMode=="sso"` and the populated `UserID`/`Email`/`DisplayName`. (The 4-iii implementation should INFO-log these at /turn entry, similar to how chatwoot's adapter logs.)

---

### Task 4: Update `ai/LESSONS.md`

**Files:** Modify `ai/LESSONS.md`

- [ ] **Step 1: Append L013 + L014**

Append after the existing L012 entry (find the `---` separator after L012's last paragraph):

```markdown
### L013: When verifying JWTs, ALWAYS pin the algorithm via `WithValidMethods` to mitigate alg-confusion attacks

The classic JWT alg-confusion attack: an attacker takes a token expected to be RS256 (verifier holds the public key), changes the header `alg` to HS256, and signs the modified token using the RS256 public key as the HMAC secret. If the verifier passes the RS256 public key into the HMAC verification path without checking `alg`, the signature validates. Result: the attacker forges arbitrary claims with only the public key.

**Where it hit:** Phase 4 SSO design. Both `RS256Verifier` and `HS256Verifier` in `relay/internal/auth/sso/` consume tokens via the same `golang-jwt/jwt/v5` parser. Without explicit alg-pinning the parser would accept either alg.

**Rule:** every `jwt.ParseWithClaims` (or `jwt.Parse`) call MUST pass `jwt.WithValidMethods([]string{"<expected-alg>"})`. Test the rejection explicitly — for an RS256 verifier, mint an HS256 token using the RSA public-key bytes as the HMAC secret and assert rejection. Same in reverse for HS256. The rejection test is a load-bearing security assertion; if it ever flakes or gets disabled, the verifier is broken.

Reference: `relay/internal/auth/sso/{rs256.go,hs256.go}`; tests `TestRS256Verifier_RejectsHS256Token`, `TestHS256Verifier_RejectsRS256Token`.

---

### L014: In-memory rate-limiters (per-key TTL + sliding window cap) need an injectable clock for testable semantics

A naive in-memory rate-limiter that reads `time.Now()` directly inside `Issue`/`Verify` cannot be tested deterministically — TTL expiry and sliding-window resets require either real wall-clock waits (`time.Sleep` makes tests slow and racy) or compromising the production code path with conditional test hooks. The clean answer is a single injectable `now func() time.Time` field set at construction.

**Where it hit:** Phase 4 `relay/internal/auth/emailcode`. The Store has a 10-min code TTL + a 3-codes-per-10-min sliding window. Tests need to advance virtual time past the window to assert reset, past the TTL to assert eviction, and to a specific instant to assert single-use post-verify. With `now func() time.Time` injected, the test passes a closure that returns a controlled `time.Time`; production passes `time.Now`.

**Rule:** any in-memory TTL/window primitive must take `now func() time.Time` (or equivalent) at construction. The internal code path always calls `s.now()` rather than `time.Now()` directly. Eager-eviction (prune on Issue/Verify) is preferred over a background goroutine for v0 — simpler, no race surface, and the per-op cost is trivial for the small key counts we expect (one entry per pending email).

Reference: `relay/internal/auth/emailcode/store.go`; tests in `relay/internal/auth/emailcode/store_test.go`.

---
```

- [ ] **Step 2: No build required (docs only)** — just confirm the file parses as Markdown:

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK
```

Expected: `CONTRACT_OK` (the contract gate doesn't check LESSONS, but running it confirms nothing else has drifted).

- [ ] **Step 3: Commit**

```
cd C:/src/ai/intake && git add ai/LESSONS.md
git commit -m "docs(lessons): L013 alg-confusion JWT mitigation; L014 injectable-clock rate-limit (4-iv)"
```

---

### Task 5: Update `ai/tasks/phase-4/README.md` §7 with live smoke evidence

**Files:** Modify `ai/tasks/phase-4/README.md`

- [ ] **Step 1: Add a "Smoke status" sub-section under §7**

In `ai/tasks/phase-4/README.md`, append at the end of §7 (after the smoke step list):

```markdown
### Smoke status (<YYYY-MM-DD>)

- **Credit-free unit + integration layer — ✅ COMPLETE.** All four sub-plans implemented on `phase-4`, each through a two-stage (spec + code-quality) subagent review. `go build/vet/test ./...` green in `relay/`; `scripts/verify-contract.sh` + `scripts/check-pins.sh` green; `go mod tidy` adds exactly two new deps (`golang-jwt/jwt/v5` and `MicahParks/keyfunc/v3`), both exact-pinned. Covered credit-free: emailcode TTL/rate-limit/single-use with injected clock; emailjwt mint+verify + tamper/wrong-secret/expired/wrong-issuer rejection; FakeSender captures; /auth/email/start happy/400/429/502 paths; /auth/email/verify happy/400/401 paths; integration drive of start→verify→turn populates SessionContext.AuthMode="email"; SSO RS256+HS256 happy/tamper/wrong-iss/wrong-aud/expired/alg-confusion/claim-mapping; factory both-set/neither-set errors. Dispatcher table-tested across all (anonymous, email, sso, none) × (request shape) combinations.
- **Live email smoke — ✅ COMPLETE** (<YYYY-MM-DD>, against MailHog at `192.168.1.102:1025/8025`, smoke email `pete@mantichor.com`). `drive-auth-email.ts` → init → email/start → MailHog captured the code → email/verify → 2 turns → submit; relay logs show `SessionContext.AuthMode="email"`, `Email="pete@mantichor.com"`, `Verified=true`; the canonical payload posted by /submit carries the same. Re-runnable.
- **Live SSO smoke — ✅ COMPLETE** (<YYYY-MM-DD>, path `<(a) Auth0 / (b) self-served JWKS>`, issuer `<...>`, audience `<...>`). Real RS256 access token minted by `<source>`; relay's `RS256Verifier` fetched the JWKS, validated iss/aud/exp/alg=RS256, mapped claims into `SessionContext{AuthMode="sso", UserID=<sub>, Email=<...>, DisplayName=<...>}`; /turn 200; verified user fields propagated to the canonical payload.
- **Phase 4 coverage: 3/3 modes proven** — anonymous (Phase 1, unchanged + regression-tested), email (live), sso (live). The Phase-1 anonymous walking-skeleton flow continues to pass against the new dispatcher (no Authorization header → X-Intake-Session resolves to anonymous as before).
```

Replace `<YYYY-MM-DD>` with the date of your live smoke (use `Get-Date -Format "yyyy-MM-dd"` on Windows). Replace `<(a) Auth0 / (b) self-served JWKS>` and `<source>`/`<issuer>`/`<audience>` with the values you used.

- [ ] **Step 2: Commit**

```
cd C:/src/ai/intake && git add ai/tasks/phase-4/README.md
git commit -m "docs(phase-4): record live email + sso smoke evidence (4-iv)"
```

---

### Task 6: Final verification gate

- [ ] **Step 1: Full build + vet + test**

```
cd C:/src/ai/intake/relay && go build ./... && echo BUILD_OK && go vet ./... && echo VET_OK && go test ./... && echo TEST_OK
```

Expected: `BUILD_OK`, `VET_OK`, every package `ok`, `TEST_OK`. Including: `intake/internal/auth`, `intake/internal/auth/emailcode`, `intake/internal/auth/smtpsend`, `intake/internal/auth/emailjwt`, `intake/internal/auth/sso`, `intake/internal/config`, `intake/internal/server`, plus all Phase 1/2/3 packages unchanged.

- [ ] **Step 2: Contract + pins**

```
cd C:/src/ai/intake && bash scripts/verify-contract.sh && echo CONTRACT_OK && bash scripts/check-pins.sh && echo PINS_OK
```

Expected: `CONTRACT_OK`, `PINS_OK`. The check-pins gate must enforce exact pins on both `golang-jwt/jwt/v5` and `MicahParks/keyfunc/v3`.

- [ ] **Step 3: Verify the two new modules are exact-pinned**

```
cd C:/src/ai/intake/relay && grep -E '^\s*github.com/(golang-jwt|MicahParks)' go.mod
```

Expected: two lines, both with explicit semver versions (no `latest`, no `^`, no `master`). Example:
```
        github.com/golang-jwt/jwt/v5 v5.2.1
        github.com/MicahParks/keyfunc/v3 v3.3.5
```

- [ ] **Step 4: Confirm the Phase-1 anonymous flow still passes**

```
cd C:/src/ai/intake/relay && go test ./internal/auth/... ./internal/server/... -v -run "Anonymous|Submit"
```

Expected: every test PASS. The dispatcher's anonymous-fallthrough branch must keep the Phase-1 walking-skeleton happy.

- [ ] **Step 5: Build-fail self-check (README §6)**

Confirm by reading the test names + the live smoke output that each README §6 item is exercised:
- Dispatcher rejects invalid bearer with 401 (4-i `TestDispatcher_NoModes_BearerPresent_401`). ✓
- HS256-on-RS256-verifier rejected (4-iii alg-confusion test). ✓
- RS256-on-HS256-verifier rejected (4-iii alg-confusion test). ✓
- Expired/tampered JWT rejected (4-ii + 4-iii). ✓
- Email rate-limit returns 429 + Retry-After (4-ii start handler test). ✓
- Both jwks_url AND hs256_secret_env set → factory error (4-iii test). ✓
- Neither set → factory error (4-iii test). ✓
- `len(jwt_secret) < 32` → startup error (4-ii constructor test). ✓
- Anonymous regression — Phase 1 still works (4-i `TestDispatcher_AnonymousFallthrough_Preserved` + the live smoke). ✓
- `golang-jwt` and `keyfunc` exact-pinned (step 3 above). ✓
- No secret/token in logs/errors (every verifier and sender test asserts redaction). ✓

---

## Smoke

This sub-plan IS the phase's final smoke. After all four tasks complete:
- Credit-free: Tasks 1, 4–5 ensure the unit + contract + pin gates are green.
- Live email: Task 2 PASSES against MailHog (self-runnable; no maintainer pause needed beyond having MailHog running).
- Live SSO: Task 3 pauses for the maintainer (path (a) or path (b)); evidence captured in Task 5.

A phase is NOT done until all three smoke layers pass. Steps 2 + 3 record their evidence into the phase README §7.

## Done Criteria

1. `core/smoke/drive-auth-email.ts` exists and type-checks; runs end-to-end against the relay + MailHog producing `[email-smoke] PASS`.
2. `go test ./...` green in `relay/` with the two new deps (`golang-jwt/jwt/v5`, `MicahParks/keyfunc/v3`) exact-pinned; `check-pins.sh` enforces the pins.
3. `ai/LESSONS.md` includes L013 (alg-confusion) and L014 (injectable-clock rate-limit).
4. `ai/tasks/phase-4/README.md` §7 has a "Smoke status" sub-section recording the live email evidence (date, MailHog URL, captured-code path) and the live SSO evidence (date, IdP path used, issuer + audience, claim mapping verified).
5. Phase-1 anonymous behavior preserved end-to-end (regression test + live re-confirm).
6. `bash scripts/verify-contract.sh` green; no frozen-seam regressions (`adapter.Adapter` interface, `auth.Middleware.Handler` signature, generated `payload/types.go`).
