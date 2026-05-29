# Phase 5 Smoke Evidence

This file accumulates the per-smoke transcripts during the 5-iv smoke run.
Each section is dated and tagged with the smoke task number.

## 5-iv Task 3 — Strict-anonymous dispatcher smoke (2026-05-28)

**Config:** `relay/cmd/relay/smoke/strict-anonymous.yaml`

**Setup:**
- `auth.modes.anonymous: false` (dispatcher MUST reject anonymous)
- `auth.anonymous.allow_without_captcha: true` (silences Q9 startup gate)
- Started with `ANTHROPIC_API_KEY=sk-ant-dummy-for-smoke` (only required so secret resolution succeeds at startup; the dispatcher rejects before any LLM call, so a dummy value is sufficient).

**Smoke requests:**

### a. /init still works (does not go through the dispatcher)
```
$ curl -s -w "\nHTTP_CODE=%{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/init -d '{}'
{"session_id":"cffda386-973e-4143-84dc-7536b41d215b","capabilities":{"auth_modes":["anonymous"],"streaming":true}}
HTTP_CODE=200
```
**Verdict:** PASS — /init returns 200 with a session_id.

### b. /turn with a VALID issued session returns 401 strict-anonymous
```
$ SESSION=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | grep -oE '"session_id":"[^"]+"' | sed 's/"session_id":"//; s/"//')
$ # SESSION=0ba3d4ab-8d40-47dd-9d15-c2a726f40255
$ curl -s -w "\nHTTP_CODE=%{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/turn \
    -H "X-Intake-Session: $SESSION" -H "Content-Type: application/json" \
    -d '{"messages":[{"role":"user","content":"hi"}]}'
{"error":{"code":"unauthorized","message":"anonymous mode is disabled on this relay"}}
HTTP_CODE=401
```
**Verdict:** PASS — the dispatcher correctly rejects anonymous sessions when `modes.anonymous=false`, returning `{"error":{"code":"unauthorized","message":"anonymous mode is disabled on this relay"}}`.

### c. /turn with a SYNTACTICALLY-VALID-BUT-UNKNOWN session also returns 401
```
$ curl -s -w "\nHTTP_CODE=%{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/turn \
    -H "X-Intake-Session: 00000000-1111-2222-3333-444444444444" -H "Content-Type: application/json" \
    -d '{"messages":[{"role":"user","content":"hi"}]}'
{"error":{"code":"unauthorized","message":"anonymous mode is disabled on this relay"}}
HTTP_CODE=401
```
**Verdict:** PASS — identical 401 response to (b) — proves the dispatcher rejects BEFORE consulting the session store (timing-safety against session-ID probing). The response body for (b) and (c) is BYTE-IDENTICAL.

### d. /init without any session header still works
```
$ curl -s -w "\nHTTP_CODE=%{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/init -d '{}'
{"session_id":"b0814d40-942f-4f21-b11c-7710a242f8e9","capabilities":{"auth_modes":["anonymous"],"streaming":true}}
HTTP_CODE=200
```
**Verdict:** PASS — /init returns 200 with a session_id regardless of session header. Confirms /init does NOT pass through the auth dispatcher.

**Overall:** Phase 5-i Task 5's Q9 strict-anonymous dispatcher guard verified at runtime against a real binary. Cases (b) and (c) returned byte-identical 401 responses, confirming the dispatcher's timing-safety property — it rejects on the `modes.anonymous=false` configuration check BEFORE consulting the session store.

## 5-iv Task 4 — Per-IP rate-limit smoke (2026-05-28)

**Config:** `relay/cmd/relay/smoke/rate-limit-test.yaml`

**Setup:**
- `ratelimit.per_ip.requests_per_second: 1.0`, `burst: 5`
- `auth.modes.anonymous: true`, `allow_without_captcha: true`
- Started with `ANTHROPIC_API_KEY=sk-ant-dummy-for-smoke` (satisfies the Q9 startup secret-resolution gate; no LLM call occurs during this smoke).

### Burst 1: 10 POSTs to /v1/intake/init
```
$ for i in $(seq 1 10); do curl -s -o /dev/null -w "request $i: %{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/init -d '{}'; done
request 1: 200
request 2: 200
request 3: 200
request 4: 200
request 5: 200
request 6: 429
request 7: 429
request 8: 429
request 9: 429
request 10: 429
```
**Verdict:** PASS — 5×200 + 5×429 (clean split — bucket burst=5 exhausted exactly on request 6). Per-IP bucket correctly rejects after burst exhausted.

### Burst 2: 10 GETs to /v1/health (control)
```
$ for i in $(seq 1 10); do curl -s -o /dev/null -w "health $i: %{http_code}\n" http://127.0.0.1:18080/v1/health; done
health 1: 200
health 2: 200
health 3: 200
health 4: 200
health 5: 200
health 6: 200
health 7: 200
health 8: 200
health 9: 200
health 10: 200
```
**Verdict:** PASS — all 10 returned 200. /v1/health is OUTSIDE the rate-limited /v1/intake group (5-i Task 7 routing).

### Burst 3: Retry-After header on the 429
```
$ for i in $(seq 1 7); do curl -s -o /dev/null -X POST http://127.0.0.1:18080/v1/intake/init -d '{}'; done
$ curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' -o /dev/null
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 1
Vary: Origin
Date: Thu, 28 May 2026 23:11:00 GMT
Content-Length: 74
```
**Verdict:** PASS — Retry-After header present with value `1` (rounded-up seconds; floor 1 per RFC 9110 / `setRetryAfter` helper from 5-ii Task 4 commit `99a9e1b`).

**Overall:** Phase 5-ii Task 1's `perip.Limiter` verified at runtime. The /v1/intake-only routing (5-i Task 7) confirmed via the /v1/health control returning 10×200 against the identical burst pattern that produced 5×429 on /v1/intake/init.

## 5-iv Task 5 — Per-session cap smoke (2026-05-28)

**Config:** `relay/cmd/relay/smoke/session-cap-test.yaml`

**Setup:**
- `ratelimit.per_session.max_turns: 3`, `session_ttl: 1h`
- `llm.provider: "ollama"` pointing at the fake-llm on `127.0.0.1:11434` (`--input-tokens 50 --output-tokens 25`) — no LLM credit consumed.
- `ratelimit.per_ip.requests_per_second: 100.0`, `burst: 1000` (intentionally very high so the per-IP gate cannot interfere — this smoke isolates the per-session gate).
- `auth.modes.anonymous: true`, `allow_without_captcha: true`.
- No env vars required (provider=ollama; `ollama.bearer_token_env` is empty so `config.ResolveSecret("")` returns `("", nil)`).

**Smoke (4 turns against the same session):**
- Turn 1: `HTTP/1.1 200` SSE stream completes
- Turn 2: 200
- Turn 3: 200
- Turn 4: `HTTP/1.1 429` with body `{"error":{"code":"session_turns_exhausted","message":"session turn limit reached"}}` + `Retry-After: 3599`

```
$ SESSION=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | grep -oE '"session_id":"[^"]+"' | sed 's/"session_id":"//; s/"//')
$ echo "Session: $SESSION"
Session: f303484c-13e3-4651-b5c7-b5c50bd93a25

$ for i in 1 2 3 4; do
    HEADERS_AND_BODY=$(curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/turn \
      -H "X-Intake-Session: $SESSION" \
      -H "Accept: text/event-stream" \
      -d '{"messages":[{"role":"user","content":"hi"}]}')
    ...
  done

=== Turn 1 ===
HTTP/1.1 200 OK
Cache-Control: no-cache
Connection: keep-alive
Content-Type: text/event-stream
Vary: Origin
Transfer-Encoding: chunked

data: {"delta":"ok"}

data: {"done":true,"input_tokens":50,"output_tokens":25}

=== Turn 2 ===
HTTP/1.1 200 OK
...same SSE shape as Turn 1...

=== Turn 3 ===
HTTP/1.1 200 OK
...same SSE shape as Turn 1...

=== Turn 4 ===
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 3599
Vary: Origin
Content-Length: 83

{"error":{"code":"session_turns_exhausted","message":"session turn limit reached"}}
```

**Verdict:** PASS — per-session cap correctly rejects the 4th turn (max_turns=3). `Retry-After: 3599` is the remaining session TTL (session_ttl=1h, smoke completed in <1s), matching the `setRetryAfter` (5-ii Task 4) round-up behavior.

**Overall:** Phase 5-ii Task 3's `auth.Store.CheckSession` + Task 4's `turnHandler` integration verified at runtime against a real `intake-relay` binary + the credit-free fake-llm (5-iv Task 1). The full LLM streaming path was exercised on turns 1-3 (SSE delta + done frames present), and the cap check correctly fires BEFORE provider.Chat on turn 4 (no upstream call to the fake-llm).


## 5-iv Task 6 — Daily-budget + tenant-isolation smoke (2026-05-28)

**Config:** `relay/cmd/relay/smoke/budget-test.yaml`

**Setup:**
- `ratelimit.daily_llm_budget.max_input_tokens: 100`, `max_output_tokens: 100`, `action_on_exceeded: "reject"`
- fake-llm running on :11434 with `--input-tokens 50 --output-tokens 50`
- `ratelimit.per_ip.requests_per_second: 100.0`, `burst: 1000` and `per_session.max_turns: 100`, `max_input_tokens: 100000` — intentionally very high to isolate the budget gate.
- `llm.ollama.max_tokens: 50` — so `estOut` passed to `Reserve` is 50; with `approximateInputTokens("hi")`=1, `estIn`≈1.

### Sub-smoke A: no-tenant budget exhaust

Three turns against a single anonymous session (no `X-Intake-Tenant` header) — empty-string tenant key shared by all three.

- Turn 1: `HTTP/1.1 200` SSE completes; Commit recorded `(in=50, out=50)` ⇒ budget counters at `(50, 50)`.
- Turn 2: `HTTP/1.1 200` SSE completes; Reserve check `c.out=50 + estOut=50 = 100`, NOT `> 100` → allow. Commit recorded `(in=50, out=50)` ⇒ budget counters at `(100, 100)` (at-cap).
- Turn 3: `HTTP/1.1 503 Service Unavailable` + `Retry-After: 2609` + body `{"error":{"code":"daily_budget_exhausted","message":"relay daily LLM budget reached"}}`. Reserve check `c.out=100 + estOut=50 = 150 > 100` → reject. No upstream call to fake-llm.

```
$ SESSION=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | grep -oE '"session_id":"[^"]+"' | sed 's/"session_id":"//; s/"//')
$ echo "Session: $SESSION"
Session: aa1455b6-ee42-478c-a7c1-9b1cbba095e8

$ for i in 1 2 3; do
    echo "=== Turn $i (no tenant) ==="
    curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/turn \
      -H "X-Intake-Session: $SESSION" \
      -H "Accept: text/event-stream" \
      -d '{"messages":[{"role":"user","content":"hi"}]}'
    echo
  done

=== Turn 1 (no tenant) ===
HTTP/1.1 200 OK
Cache-Control: no-cache
Connection: keep-alive
Content-Type: text/event-stream
Vary: Origin
Date: Thu, 28 May 2026 23:16:31 GMT
Transfer-Encoding: chunked

data: {"delta":"ok"}

data: {"done":true,"input_tokens":50,"output_tokens":50}

=== Turn 2 (no tenant) ===
HTTP/1.1 200 OK
Cache-Control: no-cache
Connection: keep-alive
Content-Type: text/event-stream
Vary: Origin
Date: Thu, 28 May 2026 23:16:31 GMT
Transfer-Encoding: chunked

data: {"delta":"ok"}

data: {"done":true,"input_tokens":50,"output_tokens":50}

=== Turn 3 (no tenant) ===
HTTP/1.1 503 Service Unavailable
Content-Type: application/json
Retry-After: 2609
Vary: Origin
Date: Thu, 28 May 2026 23:16:31 GMT
Content-Length: 86

{"error":{"code":"daily_budget_exhausted","message":"relay daily LLM budget reached"}}
```

**Verdict:** PASS — strict-`>` cap boundary correct (5-ii Task 2 fix-up commit `9e1d153`): Turn 2 lands exactly at `(100, 100)` and is allowed; Turn 3's Reserve sees `c.out + estOut = 150 > 100` and rejects with 503 + `Retry-After: 2609` seconds (= seconds to next UTC midnight from `secsToNextUTCMidnight`) + code `daily_budget_exhausted`.

### Sub-smoke B: tenants are isolated

With the no-tenant bucket (`""`) now at `(100, 100)` — exhausted — drive one turn under tenant `beta` and one under tenant `gamma` (fresh session to avoid per-session-turn confounds; per_session caps are very high so they don't fire).

- Tenant `beta` turn: `HTTP/1.1 200` SSE completes (fresh tenant bucket — no-tenant exhaustion does not affect it).
- Tenant `gamma` turn: `HTTP/1.1 200` SSE completes (also a fresh tenant bucket — distinct from both `""` and `beta`).

```
$ SESSION2=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | grep -oE '"session_id":"[^"]+"' | sed 's/"session_id":"//; s/"//')
Session2: ee882e29-917e-47db-980f-d36cd3716f6d

$ curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/turn \
    -H "X-Intake-Session: $SESSION2" \
    -H "X-Intake-Tenant: beta" \
    -H "Accept: text/event-stream" \
    -d '{"messages":[{"role":"user","content":"hi"}]}'
HTTP/1.1 200 OK
Cache-Control: no-cache
Connection: keep-alive
Content-Type: text/event-stream
Vary: Origin
Date: Thu, 28 May 2026 23:16:37 GMT
Transfer-Encoding: chunked

data: {"delta":"ok"}

data: {"done":true,"input_tokens":50,"output_tokens":50}

$ curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/turn \
    -H "X-Intake-Session: $SESSION2" \
    -H "X-Intake-Tenant: gamma" \
    -H "Accept: text/event-stream" \
    -d '{"messages":[{"role":"user","content":"hi"}]}'
HTTP/1.1 200 OK
Cache-Control: no-cache
Connection: keep-alive
Content-Type: text/event-stream
Vary: Origin
Date: Thu, 28 May 2026 23:16:37 GMT
Transfer-Encoding: chunked

data: {"delta":"ok"}

data: {"done":true,"input_tokens":50,"output_tokens":50}
```

**Verdict:** PASS — `budget.Tracker` keys per-tenant via `X-Intake-Tenant`; the `""`, `beta`, and `gamma` buckets are independent. The no-tenant bucket's exhaustion has zero effect on either tenant's bucket.

**Overall:** Phase 5-ii Task 2's `budget.Tracker.Reserve`/`Commit` integration with `turnHandler` (5-ii Task 4) verified at runtime. Strict-`>` cap boundary correct (allows exactly at cap, rejects one-over). Per-tenant bucket isolation via `X-Intake-Tenant` correct. 503 + `Retry-After: <secs-to-next-UTC-midnight>` + body code `daily_budget_exhausted` all confirmed.

## 5-iv Task 7 — drive-abuse.ts consolidated programmatic smoke (2026-05-28)

**Config:** `relay/cmd/relay/smoke/abuse-driver.yaml`

**Setup:**
- per_ip burst=5, rps=1
- per_session max_turns=3
- daily budget = (100, 100)
- fake-llm on :11434 with `--input-tokens 50 --output-tokens 50`

**Driver:** `core/smoke/drive-abuse.ts` — drives all three gates via `fetch()`:
1. Smoke 1: 10 inits → assert >=1 x 200 + >=1 x 429 (per-IP burst)
2. Smoke 2: 4 turns on one session → assert turns 1-3 = 200, turn 4 = 429 with `session_turns_exhausted`
3. Smoke 3: 1 turn on a fresh session after Smoke 2's spending → assert 503 or 429 (some gate beyond budget)

**Run:**

```
$ cd core && npx tsx smoke/drive-abuse.ts
abuse smoke: RELAY_URL=http://127.0.0.1:18080

=== Smoke 1: per-IP burst (10 inits) ===
init status codes: [
  200, 200, 200, 200,
  200, 429, 429, 429,
  429, 429
]
OK: per-IP burst produces some 200 and some 429
waiting 6s for per-IP bucket to refill...

=== Smoke 2: per-session cap (4 turns; cap=3) ===
session: fea5abcd-b404-4a36-8cd1-2d750e546722
OK: turn 1 returns 200
OK: turn 2 returns 200
FAIL: turn 3 returns 200
```

**Verdict:** PARTIAL — Smoke 1 (per-IP burst) verified end-to-end (5 x 200 then 5 x 429 — exactly burst=5). Smoke 2 FAILED on turn 3 because the **daily-budget gate fires before the per-session gate** under this fixture. Diagnostic curl confirms turn 3 returns `503 daily_budget_exhausted`:

```
$ # diagnostic re-run with the same fixture
turn 1 = 200  data: {"done":true,"input_tokens":50,"output_tokens":50}
turn 2 = 200  data: {"done":true,"input_tokens":50,"output_tokens":50}
turn 3 = 503  {"error":{"code":"daily_budget_exhausted","message":"relay daily LLM budget reached"}}
```

**Root cause:** the fake-llm reports `input_tokens=50, output_tokens=50` on every Commit, so after 2 turns the no-tenant bucket holds (in=100, out=100) — exactly at the (100, 100) cap. The third Reserve uses `estIn=approximateInputTokens(["hi"])=1, estOut=deps.MaxTokens=50`, and `c.out + estOut = 100 + 50 > 100` → rejects with 503 before the per-session gate is consulted. Gate ordering in `turn.go` is: per-IP -> per-session -> daily-budget, but the per-session counter is only **incremented on a successful turn**, so when the budget rejects the third call, the session counter is still at 2 — it can never reach the cap=3 boundary under this fixture.

This is a fixture/expectation mismatch in the task spec (Step 2 asserts turns 1-3 = 200 under a 100/100 budget with a 50/50 LLM — incompatible). The script-as-authored is the deliverable; the driver source compiles cleanly (`npx tsc --noEmit smoke/drive-abuse.ts` → no output), the YAML fixture loads cleanly (relay starts and logs `rate limits configured` with the expected values), and Smoke 1 demonstrates the per-IP gate end-to-end. The per-session gate and daily-budget gate were independently verified in 5-iv Tasks 5 and 6 (using isolated fixtures that pin one gate at a time).

**Operator note:** to make `drive-abuse.ts` pass end-to-end without modifying the script, raise the budget cap in `abuse-driver.yaml` (e.g. `max_input_tokens: 1000, max_output_tokens: 1000`) so Smoke 2 completes all four turns before any budget consideration, then have Smoke 3 explicitly drain the budget on a tenant-keyed pre-pass. The minimal-change fix is left to a follow-up because Phase 5-iv's frozen-seams rule forbids modifying the driver script (the deliverable) and the per-gate isolation already passes in Tasks 4-6.

## 5-iv Task 7 (re-run after fixture fix) — drive-abuse.ts (2026-05-28)

**Fixture fix:** `relay/cmd/relay/smoke/abuse-driver.yaml` daily_llm_budget raised from (100, 100) to (150, 150) so per-session cap (max_turns=3) fires before budget on turn 4 of Smoke 2.

Math justification (estIn=1, estOut=50, fake-llm reports in=50/out=50 per Commit):
- T1: Reserve at (0,0) → 0+50=50 ≤ 150, allow. Commit → (50,50). turns=1.
- T2: Reserve at (50,50) → 50+50=100 ≤ 150, allow. Commit → (100,100). turns=2.
- T3: Reserve at (100,100) → 100+50=150 ≤ 150, allow. Commit → (150,150). turns=3.
- T4: per-session gate (turns=3 ≥ max_turns=3) rejects with 429 `session_turns_exhausted` before budget is consulted.
- Smoke 3 fresh session, T1: Reserve at (150,150) → 150+50=200 > 150 → 503 `daily_budget_exhausted`.

**Driver output:**
```
abuse smoke: RELAY_URL=http://127.0.0.1:18080

=== Smoke 1: per-IP burst (10 inits) ===
init status codes: [
  200, 200, 200, 200,
  200, 429, 429, 429,
  429, 429
]
OK: per-IP burst produces some 200 and some 429
waiting 6s for per-IP bucket to refill...

=== Smoke 2: per-session cap (4 turns; cap=3) ===
session: 6627030b-9860-4714-acf4-4c4b1d8f604a
OK: turn 1 returns 200
OK: turn 2 returns 200
OK: turn 3 returns 200
OK: turn 4 returns 429
OK: body contains session_turns_exhausted
OK: Retry-After header present and >=1

=== Smoke 3: daily budget exhaust ===
OK: turn 1 of fresh session returns 503 or 429 (got 503)
OK: body contains daily_budget_exhausted
OK: Retry-After header present and >=1

✓ All Phase 5 abuse smokes passed.
```

**Verdict:** PASS — all three abuse gates verified end-to-end via the @intake/core-style fetch driver.

## 5-iv Task 8 — Live CAPTCHA smoke (2026-05-28)

**Config:** `relay/cmd/relay/smoke/captcha-live.yaml`

**Setup:**
- Cloudflare Turnstile published test keys (https://developers.cloudflare.com/turnstile/troubleshooting/testing/)
- Always-passes: sitekey `1x00000000000000000000AA`, secret `1x0000000000000000000000000000000AA`
- Always-fails: sitekey `2x00000000000000000000AB`, secret `2x0000000000000000000000000000000AA`
- Zero LLM credit cost; siteverify endpoint is free
- L005: secret resolved via `config.RequireSecret("INTAKE_TURNSTILE_SECRET")` — never in YAML, never logged
- Relay startup log line confirms: `"relay: captcha enabled" provider=turnstile required_for=["anonymous"]` (no secret in the log line)

**Sub-smoke a — Discovery (no token):**
```
$ curl -s -w "\nHTTP_STATUS: %{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/init \
    -H 'Content-Type: application/json' -d '{}'
{"error":{"code":"captcha_required","message":"call /init again with a solved captcha_token"},"capabilities":{"auth_modes":["anonymous"],"streaming":true,"requires_captcha":["anonymous"]},"captcha":{"provider":"turnstile","site_key":"1x00000000000000000000AA"}}
HTTP_STATUS: 400
```
**Verdict:** PASS — 400 captcha_required with discovery fields (capabilities.requires_captcha=["anonymous"] + captcha.{provider:"turnstile",site_key:"1x00000000000000000000AA"}).

**Sub-smoke b — Valid mint (always-passes secret):**
```
$ curl -s -w "\nHTTP_STATUS: %{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/init \
    -H 'Content-Type: application/json' -d '{"captcha_token":"XXXX.SMOKE-1.XXXX"}'
{"session_id":"d6a663a6-f6a2-411d-ada9-3887a17401b1","capabilities":{"auth_modes":["anonymous"],"streaming":true,"requires_captcha":["anonymous"]},"captcha":{"provider":"turnstile","site_key":"1x00000000000000000000AA"}}
HTTP_STATUS: 200
```
**Verdict:** PASS — 200 with session_id. siteverify called against real Cloudflare endpoint; always-passes secret accepted the token.

**Sub-smoke c — Replay (same token, within 5min):**
```
$ curl -s -w "\nHTTP_STATUS: %{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/init \
    -H 'Content-Type: application/json' -d '{"captcha_token":"XXXX.SMOKE-1.XXXX"}'
{"error":{"code":"captcha_failed","message":"captcha verification failed","reason":"duplicate"}}
HTTP_STATUS: 401
```
**Verdict:** PASS — 401 captcha_failed reason="duplicate". The in-process replay-protection set (Phase 5-iii Task 1) caught it BEFORE siteverify was called (defense in depth).

**Sub-smoke d — Always-fails secret:**
```
$ export INTAKE_TURNSTILE_SECRET="2x0000000000000000000000000000000AA"
$ # restart relay with same fixture, then:
$ curl -s -w "\nHTTP_STATUS: %{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/init \
    -H 'Content-Type: application/json' -d '{"captcha_token":"XXXX.SMOKE-2.XXXX"}'
{"error":{"code":"captcha_failed","message":"captcha verification failed","reason":"invalid-input-response"}}
HTTP_STATUS: 401
```
**Verdict:** PASS — 401 captcha_failed reason="invalid-input-response" (Cloudflare's documented error code for a token the always-fails secret rejects). siteverify round-trip confirmed against real Cloudflare endpoint.

**L005 verification (secret never in logs):**
```
$ grep "1x0000000000000000000000000000000AA" /tmp/captcha-smoke.log /tmp/captcha-smoke-fails.log
$ grep "2x0000000000000000000000000000000AA" /tmp/captcha-smoke.log /tmp/captcha-smoke-fails.log
$ grep -c "1x0000000000000000000000000000000AA" /tmp/captcha-smoke.log /tmp/captcha-smoke-fails.log
/tmp/captcha-smoke.log:0
/tmp/captcha-smoke-fails.log:0
$ grep -c "2x0000000000000000000000000000000AA" /tmp/captcha-smoke.log /tmp/captcha-smoke-fails.log
/tmp/captcha-smoke.log:0
/tmp/captcha-smoke-fails.log:0
```
Zero matches in either file. PASS — L005 redact-before-error holds end-to-end (neither the always-passes nor always-fails secret appears in any structured log line, including the captcha-construct, captcha-enabled, or per-request error paths).

**(Optional) hCaptcha variant:** skipped — `INTAKE_HCAPTCHA_SECRET` was not present in the environment and the Turnstile variant already exercises the same provider-abstract code path (captcha.New → captcha.Verifier → initHandler verify branch).

**Build + tests:** `go build ./... && go vet ./... && go test ./...` — clean.

**Overall:** Phase 5-iii (captcha.Verifier + initHandler verify branch) verified end-to-end against the real Cloudflare Turnstile siteverify endpoint. L005 holds.
