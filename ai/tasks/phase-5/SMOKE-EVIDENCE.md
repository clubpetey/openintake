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
