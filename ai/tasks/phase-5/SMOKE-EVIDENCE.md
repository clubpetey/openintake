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

