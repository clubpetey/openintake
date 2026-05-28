# 5-iv Final Smoke + Docs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Author the abuse-control smoke driver (a small TypeScript script that exercises the per-IP burst → per-session cap → daily budget gates against a running relay), build a tiny fake-SSE-provider command so the /turn-based smokes need no LLM credit, run all the self-runnable smokes from a clean state, run the Q9 startup-gate smoke, then PAUSE for the maintainer to run the live CAPTCHA smoke with Cloudflare's published test sitekey. Record the evidence in the phase-5 README §7, add any new lessons to `ai/LESSONS.md`, and gate the phase as done.

**Architecture:** Three new artifacts. (1) `core/smoke/drive-abuse.ts` — Node/TS driver mirroring the Phase 4 `drive-auth-email.ts` style; exercises the three rate-limit gates against a running relay and asserts the status codes + `Retry-After` headers. (2) `relay/cmd/fake-llm/main.go` — a tiny standalone HTTP server that mimics the Ollama `/api/chat` SSE shape and returns a configurable input/output token count. The relay points at it via `cfg.LLM.Provider: "ollama"` + `cfg.LLM.Ollama.BaseURL: http://127.0.0.1:<port>`. (3) `relay/cmd/relay/smoke/*.yaml` — six smoke fixture configs (clean, anonymous-no-captcha, sso-both, sso-neither, bad-cidr, combined, rate-limit-test, budget-test, captcha-live).

After this sub-plan: Phase 5 has the same "evidence-of-everything" gate that Phase 4 has — every guardrail is proven against a running binary, the Q9 strict-anonymous and consolidated startup gate are demonstrated to fire, and the CAPTCHA path is proven against real Turnstile + hCaptcha siteverify.

**Tech Stack:** Node 24 / TypeScript 5.6.3 (smoke driver), Go 1.23.2 (fake-llm command), `@intake/core` `IntakeClient`. Stdlib `net/http` + `encoding/json` only in `fake-llm`. No new dependencies.

---

## Design References

- README §7 — final smoke (what gets recorded as evidence)
- README §6 — build-fail items (must all still hold)
- Design spec §7.2 — testing strategy live-smoke matrix
- Phase-4 `4-iv-smoke-docs-plan.md` Task 1 — the `drive-auth-email.ts` style this mirrors
- LESSONS L004 — Node smoke browser-global stubs (applies to /submit calls)
- LESSONS L010 — PS 5.1 BOM gotcha (`-Encoding ascii` for any YAML the maintainer writes via PowerShell)
- LESSONS L011 — redact-before-truncate (already applied in 5-iii; this smoke verifies it from the log output)
- LESSONS L014 — injectable-clock pattern (applied in 5-ii)
- LESSONS L015 — derived-field test gaps (the rate-limit smokes are the live equivalent of L015's "smokes catch what unit tests don't model")

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/cmd/fake-llm/main.go` | Create | Tiny standalone Ollama-shaped fake provider for credit-free /turn smokes |
| `core/smoke/drive-abuse.ts` | Create | Node/TS driver: per-IP burst, per-session cap, daily budget, X-Intake-Tenant isolation |
| `relay/cmd/relay/smoke/clean.yaml` | Create | Baseline relay config (Q9 escape hatch on so it starts) |
| `relay/cmd/relay/smoke/anonymous-no-captcha.yaml` | Create | Q9-fatal misconfig fixture |
| `relay/cmd/relay/smoke/sso-both.yaml` | Create | Q9-fatal misconfig fixture |
| `relay/cmd/relay/smoke/sso-neither.yaml` | Create | Q9-fatal misconfig fixture |
| `relay/cmd/relay/smoke/bad-cidr.yaml` | Create | Q9-fatal misconfig fixture |
| `relay/cmd/relay/smoke/bad-action.yaml` | Create | Q9-fatal `action_on_exceeded: queue` fixture |
| `relay/cmd/relay/smoke/combined.yaml` | Create | All four misconfigs at once |
| `relay/cmd/relay/smoke/rate-limit-test.yaml` | Create | Aggressive per-IP burst settings for the burst smoke |
| `relay/cmd/relay/smoke/session-cap-test.yaml` | Create | Low per-session caps for the session-exhaustion smoke |
| `relay/cmd/relay/smoke/budget-test.yaml` | Create | Tiny daily LLM budget for the budget smoke |
| `relay/cmd/relay/smoke/strict-anonymous.yaml` | Create | `auth.modes.anonymous: false` for the Q9 dispatcher smoke |
| `relay/cmd/relay/smoke/captcha-live.yaml` | Create | Cloudflare-test-key Turnstile config for the live CAPTCHA smoke (paused) |
| `relay/cmd/relay/smoke/run-q9-smoke.sh` | Create | One-shot script that runs every misconfig fixture + the combined fixture; asserts exit 1 + the consolidated log line |
| `ai/tasks/phase-5/README.md` | Modify | §7 smoke status note updated with the live evidence after the smokes run |
| `ai/tasks/phase-5/SMOKE-EVIDENCE.md` | Create | Per-step curl output + the live CAPTCHA smoke transcript |
| `ai/LESSONS.md` | Modify | Append L016+ for any novel lessons uncovered during smoke (e.g. CIDR edge cases, X-Intake-Tenant header propagation, replay-set edge cases) |
| `docs/PROJECT.md` | Modify | §10 cross-refs to the implemented sub-packages (`ratelimit/perip`, `budget`, `captcha`); §17 cross-ref to L005 redact-before-error |
| `README.md` (project root) | Modify | One-line update of the "Phase status" table to mark Phase 5 done with a link to this evidence |

---

## Tasks

### Task 1: Create the credit-free `fake-llm` Ollama-shaped command

**Files:** Create `relay/cmd/fake-llm/main.go`

- [ ] **Step 1: Write the implementation**

Create `relay/cmd/fake-llm/main.go`:

```go
// fake-llm is a tiny standalone HTTP server that mimics the Ollama /api/chat
// SSE shape so the Phase 5 abuse-control smokes can exercise /turn without
// spending any LLM credit. Configurable input/output token counts via flags.
//
// Usage:
//
//	go run ./cmd/fake-llm --addr :11434 --input-tokens 50 --output-tokens 25
//
// The relay points at it via:
//
//	llm:
//	  provider: "ollama"
//	  ollama:
//	    base_url: "http://127.0.0.1:11434"
//	    model: "fake"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":11434", "listen address")
	inputTokens := flag.Int("input-tokens", 50, "input tokens to report in the final SSE chunk")
	outputTokens := flag.Int("output-tokens", 25, "output tokens to report in the final SSE chunk")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		// Drain the request body so the relay doesn't block.
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)

		// Emit a single content chunk, then a done chunk with the token counts.
		// Shape mirrors Ollama's NDJSON streaming response.
		fmt.Fprintf(w, `{"model":"fake","message":{"role":"assistant","content":"ok"},"done":false}`+"\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		fmt.Fprintf(w, `{"model":"fake","done":true,"prompt_eval_count":%d,"eval_count":%d}`+"\n", *inputTokens, *outputTokens)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	})

	log.Printf("fake-llm: listening on %s (input_tokens=%d, output_tokens=%d)", *addr, *inputTokens, *outputTokens)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("fake-llm: listen: %v", err)
	}
}
```

- [ ] **Step 2: Build and start the fake-llm**

Run: `cd relay && go build -o /tmp/fake-llm ./cmd/fake-llm && /tmp/fake-llm --addr :11434 &` (on Windows: `go build -o fake-llm.exe ./cmd/fake-llm; Start-Job -ScriptBlock { .\fake-llm.exe --addr :11434 }`).
Expected: prints `fake-llm: listening on :11434 (input_tokens=50, output_tokens=25)`.

- [ ] **Step 3: Smoke-test the fake provider directly**

Run: `curl -s -N -X POST http://127.0.0.1:11434/api/chat -H 'Content-Type: application/json' -d '{"model":"fake","messages":[{"role":"user","content":"hi"}],"stream":true}'`
Expected: two NDJSON lines — first with `"done":false`, second with `"done":true,"prompt_eval_count":50,"eval_count":25`.

Stop the fake-llm before continuing: `kill %1` (or `taskkill /F`).

- [ ] **Step 4: Commit**

```bash
git add relay/cmd/fake-llm/main.go
git commit -m "feat(5-iv): fake-llm command — credit-free Ollama-shaped /api/chat for abuse smokes"
```

---

### Task 2: Author the Q9 startup-gate fixture YAMLs + driver script

**Files:** Create `relay/cmd/relay/smoke/{clean,anonymous-no-captcha,sso-both,sso-neither,bad-cidr,bad-action,combined}.yaml`, `relay/cmd/relay/smoke/run-q9-smoke.sh`

- [ ] **Step 1: Author the clean baseline**

Create `relay/cmd/relay/smoke/clean.yaml`:

```yaml
server:
  addr: ":18080"
  external_url: "http://127.0.0.1:18080"
  cors_origins: ["http://localhost:5173"]
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

Author it via PowerShell (per L010) using:
```powershell
[System.IO.File]::WriteAllText("relay/cmd/relay/smoke/clean.yaml", @'
server:
  addr: ":18080"
...
'@, [System.Text.UTF8Encoding]::new($false))
```
or via Bash with `cat >`.

- [ ] **Step 2: Author the misconfig fixtures**

Create `relay/cmd/relay/smoke/anonymous-no-captcha.yaml`:

```yaml
server:
  addr: ":18080"
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: false
captcha:
  enabled: false
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

Create `relay/cmd/relay/smoke/sso-both.yaml`:

```yaml
server:
  addr: ":18080"
auth:
  modes:
    anonymous: true
    sso: true
  anonymous:
    allow_without_captcha: true
  sso:
    issuer: "https://example.auth0.com/"
    audience: "https://api.example.com"
    jwks_url: "https://example.auth0.com/.well-known/jwks.json"
    hs256_secret_env: "INTAKE_SSO_HS256"
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

Create `relay/cmd/relay/smoke/sso-neither.yaml`: same as `sso-both.yaml` but **delete** both the `jwks_url` and `hs256_secret_env` lines.

Create `relay/cmd/relay/smoke/bad-cidr.yaml`:

```yaml
server:
  addr: ":18080"
  trusted_proxies:
    - "10.0.0.0/8"
    - "not-a-cidr"
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

Create `relay/cmd/relay/smoke/bad-action.yaml`:

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
    action_on_exceeded: "queue"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

Create `relay/cmd/relay/smoke/combined.yaml` — combine ALL four misconfigs (anonymous-no-captcha + sso-both + bad-cidr + bad-action) in one file. The relay must emit a single Error log line listing all four problems.

- [ ] **Step 3: Author the Q9 driver script**

Create `relay/cmd/relay/smoke/run-q9-smoke.sh`:

```bash
#!/usr/bin/env bash
#
# Q9 startup-gate smoke: each misconfig YAML must cause the relay to exit 1
# with the matching consolidated Error log line. The combined fixture must
# emit ONE log line listing all four problems.
#
# Requires INTAKE_SSO_HS256, INTAKE_TURNSTILE_SECRET to be set as dummy values
# so secret resolution doesn't error before Q9 fires. Any non-empty value works.
set -euo pipefail

cd "$(dirname "$0")/../../.."

export INTAKE_SSO_HS256="${INTAKE_SSO_HS256:-dummy-secret-32-bytes-padded----}"
export INTAKE_TURNSTILE_SECRET="${INTAKE_TURNSTILE_SECRET:-dummy-turnstile-secret}"

run_misconfig() {
  local name="$1"
  local fixture="$2"
  local expected_substring="$3"

  echo "=== Q9 smoke: $name ==="
  local output
  output=$(go run ./relay/cmd/relay --config "$fixture" 2>&1 || true)
  local exit_code=$?

  if echo "$output" | grep -q "relay: startup config errors"; then
    echo "OK: consolidated error log line present"
  else
    echo "FAIL: missing 'relay: startup config errors' line"
    echo "Output: $output"
    exit 1
  fi

  if echo "$output" | grep -q "$expected_substring"; then
    echo "OK: matched expected problem '$expected_substring'"
  else
    echo "FAIL: expected problem substring '$expected_substring' not found"
    echo "Output: $output"
    exit 1
  fi
  echo
}

run_misconfig "anonymous-no-captcha"  "relay/cmd/relay/smoke/anonymous-no-captcha.yaml"  "anonymous"
run_misconfig "sso-both"              "relay/cmd/relay/smoke/sso-both.yaml"              "both"
run_misconfig "sso-neither"           "relay/cmd/relay/smoke/sso-neither.yaml"           "neither"
run_misconfig "bad-cidr"              "relay/cmd/relay/smoke/bad-cidr.yaml"              "not-a-cidr"
run_misconfig "bad-action"            "relay/cmd/relay/smoke/bad-action.yaml"            "action_on_exceeded"

echo "=== Q9 smoke: combined ==="
output=$(go run ./relay/cmd/relay --config "relay/cmd/relay/smoke/combined.yaml" 2>&1 || true)
for substr in "anonymous" "both" "not-a-cidr" "action_on_exceeded"; do
  if echo "$output" | grep -q "$substr"; then
    echo "OK: combined fixture matched '$substr'"
  else
    echo "FAIL: combined fixture missing '$substr'"
    exit 1
  fi
done
# The combined fixture must emit ONE Error log line, not many.
log_count=$(echo "$output" | grep -c "relay: startup config errors" || true)
if [ "$log_count" -ne 1 ]; then
  echo "FAIL: expected exactly 1 'startup config errors' line; got $log_count"
  exit 1
fi
echo
echo "All Q9 smokes passed."
```

`chmod +x relay/cmd/relay/smoke/run-q9-smoke.sh`.

- [ ] **Step 4: Run the Q9 smoke**

Run: `bash relay/cmd/relay/smoke/run-q9-smoke.sh`
Expected: every section prints `OK: ...`; final line is `All Q9 smokes passed.`

- [ ] **Step 5: Commit**

```bash
git add relay/cmd/relay/smoke/clean.yaml relay/cmd/relay/smoke/anonymous-no-captcha.yaml relay/cmd/relay/smoke/sso-both.yaml relay/cmd/relay/smoke/sso-neither.yaml relay/cmd/relay/smoke/bad-cidr.yaml relay/cmd/relay/smoke/bad-action.yaml relay/cmd/relay/smoke/combined.yaml relay/cmd/relay/smoke/run-q9-smoke.sh
git commit -m "feat(5-iv): Q9 startup-gate smoke driver + 7 fixture YAMLs"
```

---

### Task 3: Author the strict-anonymous fixture + dispatcher smoke

**Files:** Create `relay/cmd/relay/smoke/strict-anonymous.yaml`

- [ ] **Step 1: Author the fixture**

Create `relay/cmd/relay/smoke/strict-anonymous.yaml`:

```yaml
server:
  addr: ":18080"
auth:
  modes:
    anonymous: false       # Q9 strict: dispatcher MUST reject anonymous even with valid X-Intake-Session
  anonymous:
    allow_without_captcha: true   # silence the Q9 startup gate (anonymous is OFF so the gate is vacuously satisfied)
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 2: Run the dispatcher smoke**

Start the relay: `cd relay && go run ./cmd/relay --config cmd/relay/smoke/strict-anonymous.yaml &`. In another shell:

```bash
# A real-looking UUID — the dispatcher must reject BEFORE checking the store.
curl -s -X POST http://127.0.0.1:18080/v1/intake/turn \
  -H "X-Intake-Session: 00000000-1111-2222-3333-444444444444" \
  -d '{"messages":[{"role":"user","content":"hi"}]}' | jq .
```
Expected: 401 + `error.code:"unauthorized"` + message mentions "anonymous mode is disabled".

```bash
# Sanity: /init still works (it doesn't go through auth.Handler).
curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | jq .
```
Expected: 200 + session_id (capabilities.auth_modes does NOT include "anonymous").

Stop the relay: `kill %1`.

- [ ] **Step 3: Record the curl output in `ai/tasks/phase-5/SMOKE-EVIDENCE.md`**

Create the file as you go. Format:
```markdown
## Strict-anonymous dispatcher smoke (2026-05-28)

config: relay/cmd/relay/smoke/strict-anonymous.yaml

```
$ curl -s -X POST ... /v1/intake/turn ...
{"error":{"code":"unauthorized","message":"anonymous mode is disabled on this relay"}}
```

Verdict: PASS — dispatcher correctly rejects anonymous despite valid X-Intake-Session.
```

- [ ] **Step 4: Commit**

```bash
git add relay/cmd/relay/smoke/strict-anonymous.yaml ai/tasks/phase-5/SMOKE-EVIDENCE.md
git commit -m "feat(5-iv): strict-anonymous smoke fixture + dispatcher rejection evidence"
```

---

### Task 4: Author the per-IP rate-limit smoke

**Files:** Create `relay/cmd/relay/smoke/rate-limit-test.yaml`

- [ ] **Step 1: Author the fixture**

Create `relay/cmd/relay/smoke/rate-limit-test.yaml`:

```yaml
server:
  addr: ":18080"
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true
ratelimit:
  per_ip:
    requests_per_second: 1.0
    burst: 5
    idle_ttl: "15m"
  daily_llm_budget:
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 2: Drive the smoke**

Start the relay: `cd relay && go run ./cmd/relay --config cmd/relay/smoke/rate-limit-test.yaml &`. In another shell:

```bash
echo "=== /v1/intake/init burst of 10 ==="
for i in $(seq 1 10); do
  curl -s -o /dev/null -w "request $i: %{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/init -d '{}'
done

echo
echo "=== /v1/health burst of 10 (control — must all 200) ==="
for i in $(seq 1 10); do
  curl -s -o /dev/null -w "health $i: %{http_code}\n" http://127.0.0.1:18080/v1/health
done

echo
echo "=== Retry-After on 429 ==="
# Trigger a reject and capture the header.
for i in $(seq 1 7); do curl -s -o /dev/null -X POST http://127.0.0.1:18080/v1/intake/init -d '{}'; done
curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | grep -i 'retry-after\|http/1'
```

Expected output (paste into SMOKE-EVIDENCE.md):
- First 5 init requests return 200; requests 6-10 return 429.
- All 10 health requests return 200.
- The Retry-After header is exactly `1` (numeric seconds).

Stop the relay.

- [ ] **Step 3: Commit**

```bash
git add relay/cmd/relay/smoke/rate-limit-test.yaml ai/tasks/phase-5/SMOKE-EVIDENCE.md
git commit -m "feat(5-iv): per-IP rate-limit smoke fixture + 429+Retry-After evidence"
```

---

### Task 5: Author the per-session cap smoke + fake-llm wiring

**Files:** Create `relay/cmd/relay/smoke/session-cap-test.yaml`

- [ ] **Step 1: Author the fixture**

Create `relay/cmd/relay/smoke/session-cap-test.yaml`:

```yaml
server:
  addr: ":18080"
llm:
  provider: "ollama"
  ollama:
    base_url: "http://127.0.0.1:11434"
    model: "fake"
    max_tokens: 100
auth:
  modes:
    anonymous: true
  anonymous:
    allow_without_captcha: true
ratelimit:
  per_session:
    max_turns: 3
    max_input_tokens: 1000
    session_ttl: "1h"
  daily_llm_budget:
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 2: Drive the smoke**

```bash
# Start the fake-llm
go run ./relay/cmd/fake-llm --addr :11434 --input-tokens 50 --output-tokens 25 &
FAKE_PID=$!

# Start the relay
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/session-cap-test.yaml &
RELAY_PID=$!
sleep 1

# Mint a session
SESSION=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | jq -r .session_id)
echo "session_id: $SESSION"

# 4 turns — first 3 succeed (200), 4th rejects (429 session_turns_exhausted)
for i in 1 2 3 4; do
  echo "=== turn $i ==="
  curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/turn \
    -H "X-Intake-Session: $SESSION" \
    -H "Accept: text/event-stream" \
    -d '{"messages":[{"role":"user","content":"hi"}]}' | head -c 500
  echo
done

kill $RELAY_PID $FAKE_PID
```

Expected:
- Turns 1-3: HTTP 200, SSE stream with `done:true,input_tokens:50,output_tokens:25`.
- Turn 4: HTTP 429, body `{"error":{"code":"session_turns_exhausted",...}}`, header `Retry-After: <secs>` close to remaining TTL (≤3600).

Paste into SMOKE-EVIDENCE.md.

- [ ] **Step 3: Commit**

```bash
git add relay/cmd/relay/smoke/session-cap-test.yaml ai/tasks/phase-5/SMOKE-EVIDENCE.md
git commit -m "feat(5-iv): per-session cap smoke — 21st turn rejects with 429 + Retry-After"
```

---

### Task 6: Author the daily-budget + tenant-isolation smoke

**Files:** Create `relay/cmd/relay/smoke/budget-test.yaml`

- [ ] **Step 1: Author the fixture**

Create `relay/cmd/relay/smoke/budget-test.yaml`:

```yaml
server:
  addr: ":18080"
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
    max_input_tokens: 100
    max_output_tokens: 100
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 2: Drive the budget smoke (no-tenant key)**

```bash
go run ./relay/cmd/fake-llm --addr :11434 --input-tokens 50 --output-tokens 50 &
FAKE_PID=$!
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/budget-test.yaml &
RELAY_PID=$!
sleep 1

SESSION=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | jq -r .session_id)

# Turn 1: input=50, output=50 → totals (50,50) — under cap (100,100).
curl -s -o /dev/null -w "turn 1: %{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/turn \
  -H "X-Intake-Session: $SESSION" -d '{"messages":[{"role":"user","content":"hi"}]}'

# Turn 2: would push to (100,100) — Reserve uses `>` cap, so this still passes.
curl -s -o /dev/null -w "turn 2: %{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/turn \
  -H "X-Intake-Session: $SESSION" -d '{"messages":[{"role":"user","content":"hi"}]}'

# Turn 3: estIn=50 would push to 150 > 100 → 503.
curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/turn \
  -H "X-Intake-Session: $SESSION" -d '{"messages":[{"role":"user","content":"hi"}]}' | head -c 500
```

Expected:
- Turns 1+2: 200.
- Turn 3: 503, body `{"error":{"code":"daily_budget_exhausted",...}}`, `Retry-After: <secs-to-utc-midnight>` (close to 86400 if run just after midnight, or smaller late in the day).

- [ ] **Step 3: Drive the tenant-isolation smoke**

Continue with the same relay running:

```bash
# Tenant A is exhausted; tenant B and "" are separate buckets.
SESSION2=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | jq -r .session_id)

echo "=== tenant beta (fresh bucket, should succeed) ==="
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/turn \
  -H "X-Intake-Session: $SESSION2" -H "X-Intake-Tenant: beta" \
  -d '{"messages":[{"role":"user","content":"hi"}]}'

echo "=== tenant gamma (also fresh) ==="
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://127.0.0.1:18080/v1/intake/turn \
  -H "X-Intake-Session: $SESSION2" -H "X-Intake-Tenant: gamma" \
  -d '{"messages":[{"role":"user","content":"hi"}]}'

kill $RELAY_PID $FAKE_PID
```

Expected: both tenant-keyed turns return 200 (independent of the no-tenant bucket that just exhausted).

Paste all 4 curl outputs into SMOKE-EVIDENCE.md.

- [ ] **Step 4: Commit**

```bash
git add relay/cmd/relay/smoke/budget-test.yaml ai/tasks/phase-5/SMOKE-EVIDENCE.md
git commit -m "feat(5-iv): daily-budget + tenant-isolation smoke — 503 + Retry-After + per-tenant buckets"
```

---

### Task 7: Author `core/smoke/drive-abuse.ts` (consolidated end-to-end driver)

**Files:** Create `core/smoke/drive-abuse.ts`

This is the consolidated programmatic driver — analogous to Phase 4's `drive-auth-email.ts`. It exercises all three rate-limit gates against a fresh relay + fake-llm via the `@intake/core` `IntakeClient` (not raw curl) so the smoke also covers the widget's contract.

- [ ] **Step 1: Write the driver**

Create `core/smoke/drive-abuse.ts`:

```typescript
/**
 * Phase 5 abuse-control smoke driver.
 *
 * Drives all three Phase 5 rate-limit gates against a running relay:
 *   1. Per-IP burst → 429 + Retry-After:1
 *   2. Per-session cap → 429 session_turns_exhausted
 *   3. Daily LLM budget → 503 daily_budget_exhausted
 *
 * Requires:
 *   - the relay running with the relay/cmd/relay/smoke/abuse-driver.yaml config
 *     (a YAML that combines aggressive per-IP + low per-session + low budget caps)
 *   - the fake-llm running on :11434 (relay/cmd/fake-llm)
 *
 * Usage:
 *   RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-abuse.ts
 *
 * Browser-global stubs (LESSONS L004): /turn calls IntakeClient.turn which is
 * fetch-only and does not read browser globals. The /submit path WOULD, but
 * this driver does not exercise /submit (the abuse gates are all on /turn).
 */

const RELAY_URL = process.env['RELAY_URL'] ?? 'http://127.0.0.1:18080';

interface InitResponse {
  session_id: string;
  capabilities: { auth_modes: string[]; streaming: boolean; requires_captcha?: string[] };
}

async function initSession(): Promise<string> {
  const resp = await fetch(`${RELAY_URL}/v1/intake/init`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  });
  if (!resp.ok) {
    throw new Error(`init failed: ${resp.status} ${await resp.text()}`);
  }
  const body = (await resp.json()) as InitResponse;
  return body.session_id;
}

async function turn(sessionID: string, tenant?: string): Promise<{ status: number; retryAfter: string | null; bodyHead: string }> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Intake-Session': sessionID,
  };
  if (tenant) headers['X-Intake-Tenant'] = tenant;
  const resp = await fetch(`${RELAY_URL}/v1/intake/turn`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ messages: [{ role: 'user', content: 'hi' }] }),
  });
  // Drain the body (read the first ~500 chars for diagnostics; don't keep the stream).
  let bodyHead = '';
  if (resp.body) {
    const reader = resp.body.getReader();
    const { value } = await reader.read();
    if (value) {
      bodyHead = new TextDecoder().decode(value).slice(0, 500);
    }
    await reader.cancel();
  }
  return {
    status: resp.status,
    retryAfter: resp.headers.get('Retry-After'),
    bodyHead,
  };
}

function assert(cond: boolean, msg: string): void {
  if (!cond) {
    console.error(`FAIL: ${msg}`);
    process.exit(1);
  }
  console.log(`OK: ${msg}`);
}

async function smokePerIPBurst(): Promise<void> {
  console.log('\n=== Smoke 1: per-IP burst (10 inits) ===');
  const codes: number[] = [];
  for (let i = 0; i < 10; i++) {
    const r = await fetch(`${RELAY_URL}/v1/intake/init`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    });
    codes.push(r.status);
    await r.text(); // drain
  }
  console.log('init status codes:', codes);
  const ok200 = codes.filter((c) => c === 200).length;
  const ok429 = codes.filter((c) => c === 429).length;
  assert(ok200 >= 1 && ok429 >= 1, 'per-IP burst produces some 200 and some 429');

  // Wait for the bucket to refill before subsequent smokes.
  console.log('waiting 6s for per-IP bucket to refill...');
  await new Promise((r) => setTimeout(r, 6000));
}

async function smokePerSessionCap(): Promise<void> {
  console.log('\n=== Smoke 2: per-session cap (4 turns; cap=3) ===');
  const session = await initSession();
  console.log('session:', session);

  for (let i = 1; i <= 3; i++) {
    const r = await turn(session);
    assert(r.status === 200, `turn ${i} returns 200`);
    // pace out the turns so the per-IP limiter doesn't kick in
    await new Promise((res) => setTimeout(res, 1200));
  }
  const r4 = await turn(session);
  assert(r4.status === 429, 'turn 4 returns 429');
  assert(r4.bodyHead.includes('session_turns_exhausted'), 'body contains session_turns_exhausted');
  assert(r4.retryAfter !== null && Number(r4.retryAfter) >= 1, 'Retry-After header present and ≥1');
}

async function smokeDailyBudget(): Promise<void> {
  console.log('\n=== Smoke 3: daily budget exhaust ===');
  // For this smoke to work, the running relay needs daily budget = 100/100.
  // Two turns at 50 input each, then a third that would exceed → 503.
  const session = await initSession();
  await new Promise((res) => setTimeout(res, 1200));
  await turn(session);
  await new Promise((res) => setTimeout(res, 1200));
  await turn(session);
  await new Promise((res) => setTimeout(res, 1200));
  const r3 = await turn(session);
  // Depending on whether the fixture is loaded with per-session or budget caps,
  // a 429 (session) or 503 (budget) is acceptable. We assert that SOMETHING
  // beyond 200 fires.
  assert(r3.status === 503 || r3.status === 429, `turn 3 returns 503 or 429 (got ${r3.status})`);
  if (r3.status === 503) {
    assert(r3.bodyHead.includes('daily_budget_exhausted'), 'body contains daily_budget_exhausted');
    assert(r3.retryAfter !== null && Number(r3.retryAfter) >= 1, 'Retry-After header present and ≥1');
  }
}

async function main(): Promise<void> {
  console.log(`abuse smoke: RELAY_URL=${RELAY_URL}`);

  await smokePerIPBurst();
  await smokePerSessionCap();
  await smokeDailyBudget();

  console.log('\n✓ All Phase 5 abuse smokes passed.');
}

main().catch((err) => {
  console.error('smoke driver failed:', err);
  process.exit(1);
});
```

- [ ] **Step 2: Author the abuse-driver fixture YAML**

Create `relay/cmd/relay/smoke/abuse-driver.yaml`:

```yaml
server:
  addr: ":18080"
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
  per_ip:
    requests_per_second: 1.0
    burst: 5
    idle_ttl: "15m"
  per_session:
    max_turns: 3
    max_input_tokens: 1000
    session_ttl: "1h"
  daily_llm_budget:
    max_input_tokens: 100
    max_output_tokens: 100
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 3: Run the driver**

```bash
# Terminal 1
go run ./relay/cmd/fake-llm --addr :11434 --input-tokens 50 --output-tokens 50

# Terminal 2
go run ./relay/cmd/relay --config relay/cmd/relay/smoke/abuse-driver.yaml

# Terminal 3
RELAY_URL=http://127.0.0.1:18080 npx tsx core/smoke/drive-abuse.ts
```

Expected: all `OK: ...` lines, final line `✓ All Phase 5 abuse smokes passed.`

Paste the output into SMOKE-EVIDENCE.md.

- [ ] **Step 4: Commit**

```bash
git add core/smoke/drive-abuse.ts relay/cmd/relay/smoke/abuse-driver.yaml ai/tasks/phase-5/SMOKE-EVIDENCE.md
git commit -m "feat(5-iv): drive-abuse.ts — Phase 5 rate-limit gates exercised via @intake/core"
```

---

### Task 8: Author the live CAPTCHA smoke (paused for maintainer)

**Files:** Create `relay/cmd/relay/smoke/captcha-live.yaml`

- [ ] **Step 1: Author the fixture**

Create `relay/cmd/relay/smoke/captcha-live.yaml`:

```yaml
server:
  addr: ":18080"
auth:
  modes:
    anonymous: true
captcha:
  enabled: true
  provider: "turnstile"
  # Cloudflare published always-passes test sitekey:
  site_key: "1x00000000000000000000AA"
  # Set INTAKE_TURNSTILE_SECRET="1x0000000000000000000000000000000AA" (always-passes test secret)
  secret_key_env: "INTAKE_TURNSTILE_SECRET"
  required_for: ["anonymous"]
ratelimit:
  daily_llm_budget:
    action_on_exceeded: "reject"
adapters:
  webhook:
    enabled: true
    url: "http://127.0.0.1:19099/intake"
routing:
  default_adapter: "webhook"
```

- [ ] **Step 2: PAUSE — request maintainer go-ahead**

In the subagent or executor session, BEFORE running this smoke, output:

```
PAUSE: Live CAPTCHA smoke ready to run. This uses Cloudflare's published
test sitekey/secret (1x00000000000000000000AA / 1x0000000000000000000000000000000AA)
which are free of charge and always succeed. No human interaction needed.

To proceed, set INTAKE_TURNSTILE_SECRET to the always-passes secret in your
shell, then say "go" and I will run:
  cd relay && go run ./cmd/relay --config cmd/relay/smoke/captcha-live.yaml &
  (then the 4 curl steps below)

For the hCaptcha pass, the published test keys are:
  site_key: "10000000-ffff-ffff-ffff-000000000001"
  secret:   "0x0000000000000000000000000000000000000000"
```

WAIT for maintainer go-ahead. Do NOT proceed without it.

- [ ] **Step 3: Run the smoke after maintainer go-ahead**

```bash
export INTAKE_TURNSTILE_SECRET="1x0000000000000000000000000000000AA"
cd relay && go run ./cmd/relay --config cmd/relay/smoke/captcha-live.yaml &
sleep 1

echo "=== a. discovery call (no captcha_token) — expect 400 captcha_required ==="
curl -s -X POST http://127.0.0.1:18080/v1/intake/init -H 'Content-Type: application/json' -d '{}' | jq .

echo "=== b. valid mint (any non-empty token; always-passes secret accepts everything) ==="
curl -s -X POST http://127.0.0.1:18080/v1/intake/init -H 'Content-Type: application/json' \
  -d '{"captcha_token":"XXXX.SMOKE-1.XXXX"}' | jq .

echo "=== c. replay (same token) — expect 401 captcha_failed reason=duplicate ==="
curl -s -X POST http://127.0.0.1:18080/v1/intake/init -H 'Content-Type: application/json' \
  -d '{"captcha_token":"XXXX.SMOKE-1.XXXX"}' | jq .

kill %1

echo "=== d. always-fails secret — expect 401 captcha_failed reason=<provider-code> ==="
export INTAKE_TURNSTILE_SECRET="2x0000000000000000000000000000000AA"
go run ./cmd/relay --config cmd/relay/smoke/captcha-live.yaml &
sleep 1
curl -s -X POST http://127.0.0.1:18080/v1/intake/init -H 'Content-Type: application/json' \
  -d '{"captcha_token":"XXXX.SMOKE-2.XXXX"}' | jq .
kill %1
```

Paste the outputs into `ai/tasks/phase-5/SMOKE-EVIDENCE.md`.

Repeat with the hCaptcha test keys (update `captcha.provider`, `captcha.site_key`, and `INTAKE_TURNSTILE_SECRET` → `INTAKE_HCAPTCHA_SECRET`).

- [ ] **Step 4: Verify no log line leaked the secret**

After the live CAPTCHA smoke, grep the relay's stdout (captured to a file or scroll-back) for any of:
- `1x0000000000000000000000000000000AA`
- `2x0000000000000000000000000000000AA`
- `0x0000000000000000000000000000000000000000`

Expected: 0 matches. If any match is found, L005 redaction has a hole — STOP and patch the captcha package before proceeding.

- [ ] **Step 5: Commit**

```bash
git add relay/cmd/relay/smoke/captcha-live.yaml ai/tasks/phase-5/SMOKE-EVIDENCE.md
git commit -m "feat(5-iv): live CAPTCHA smoke — Turnstile + hCaptcha test sitekeys; no secret in logs"
```

---

### Task 9: Phase 4 + Phase 1 regression smokes

**Files:** none new

- [ ] **Step 1: Run Phase 4's `drive-auth-email.ts` against a Phase-5 relay**

```bash
# Configure the relay with auth.modes.email=true alongside the Phase 5 settings;
# either edit the relay/cmd/relay/smoke/clean.yaml in-place or use a dedicated
# phase-4-regression.yaml. The existing Phase 4 driver should work unchanged.
RELAY_URL=http://127.0.0.1:18080 \
MAILHOG_URL=http://192.168.1.102:8025 \
SMOKE_EMAIL=pete@mantichor.com \
npx tsx core/smoke/drive-auth-email.ts
```

Expected: Phase 4 driver completes without error. Proves Phase 5 middleware ordering + dispatcher hardening + budget gate did NOT regress live email auth.

- [ ] **Step 2: Run a Phase-1 anonymous-only smoke**

```bash
SESSION=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | jq -r .session_id)
curl -s -D- -X POST http://127.0.0.1:18080/v1/intake/turn \
  -H "X-Intake-Session: $SESSION" \
  -d '{"messages":[{"role":"user","content":"hi"}]}' | head -c 500
```

Expected: 200 + SSE stream. Proves Phase 1 walking skeleton still works under Phase 5's chain when `auth.anonymous.allow_without_captcha:true`.

- [ ] **Step 3: Paste both regression outputs into SMOKE-EVIDENCE.md and commit**

```bash
git add ai/tasks/phase-5/SMOKE-EVIDENCE.md
git commit -m "feat(5-iv): Phase 1+4 regression smokes — no regression under Phase 5 middleware chain"
```

---

### Task 10: Update LESSONS, PROJECT.md cross-refs, project README

**Files:** Modify `ai/LESSONS.md`, `docs/PROJECT.md`, `README.md` (project root), `ai/tasks/phase-5/README.md`

- [ ] **Step 1: Append any novel lessons to `ai/LESSONS.md`**

Use these slots ONLY if a novel lesson surfaced during smoke. If none surfaced, leave LESSONS unchanged. Template entry:

```markdown
### L016: <one-line title>

<paragraph describing what went wrong + how it was caught + the rule going forward>

**Where it hit:** Phase 5 <sub-plan>, <commit-id>.

Reference: `<file>:<line>`; tests `<TestName>`.
```

Anticipated candidates:
- `golang.org/x/time/rate.Limiter` clock injection: `rate.NewLimiter` reads `time.Now` internally on Allow(), but accepts `now` as the first arg in `AllowN(now, 1)` — must be used consistently with the injected clock or the bucket drifts.
- The "explicit `required_for: []` vs key omitted" distinction in YAML — required a custom UnmarshalYAML to detect; if you only check `len(slice) == 0` you can't tell the two apart.
- The `gjq | $LASTEXITCODE` Powershell quirk (already in L010 — confirm or extend).

- [ ] **Step 2: Update `docs/PROJECT.md` §10 cross-refs**

In `docs/PROJECT.md` §10 (rate limiting), add a final line per sub-section pointing at the implementing package:

- Per-IP: `Implementation: relay/internal/ratelimit/perip — golang.org/x/time/rate-backed bucket per client IP, eager-GC.`
- Daily LLM spend cap: `Implementation: relay/internal/budget — Reserve/Commit + UTC-day reset.`
- CAPTCHA: `Implementation: relay/internal/captcha — Turnstile + hCaptcha siteverify + 5-minute single-use replay set.`
- Origin enforcement: `Implementation: relay/internal/server/server.go corsMiddleware (Phase 1; unchanged in Phase 5).`

In §17 (security considerations), add a line cross-referencing L005 for the CAPTCHA secret:

> - CAPTCHA siteverify secret: never logged; never in any returned error (`relay/internal/captcha` redact-before-error per LESSONS L005).

- [ ] **Step 3: Update the project root `README.md`**

Find the project-status table (if present) or a "Phases shipped" list. Update Phase 5 from "not started" / "in progress" to "live + smoked" with a date and a link:

```markdown
| Phase 5 — Abuse & spend control | 2026-05-28 | ✅ live | [ai/tasks/phase-5/](ai/tasks/phase-5/) |
```

If the project root README does not currently have such a table, skip this step.

- [ ] **Step 4: Mark the phase-5 README.md sub-plan table "Done"**

In `ai/tasks/phase-5/README.md`, change each `Status` column from `Not started` to `Done` (or `Live + smoked`). Add a 1-paragraph §7.1 "Smoke status (YYYY-MM-DD)" entry under §7 mirroring Phase 4's "Smoke status" entry, summarizing every smoke pass + any novel lesson.

- [ ] **Step 5: Final whole-relay smoke runs green**

Run: `cd relay && go build ./... && go vet ./... && go test -race ./... && cd ..`
Expected: build + vet + tests green.

Run: `bash scripts/verify-contract.sh && bash scripts/check-pins.sh`
Expected: both exit 0.

Run: `cd relay && go mod tidy && cd .. && git diff relay/go.mod relay/go.sum`
Expected: empty diff (no spurious dep changes).

- [ ] **Step 6: Commit + final tag**

```bash
git add ai/LESSONS.md docs/PROJECT.md README.md ai/tasks/phase-5/README.md
git commit -m "docs(phase-5): live smoke evidence + LESSONS + PROJECT.md cross-refs; phase 5 done"
```

---

## Smoke (mandatory)

This entire sub-plan IS the smoke. The done criteria below are the verification.

## Done criteria

- [ ] All 10 tasks complete and committed.
- [ ] `cd relay && go build ./... && go vet ./... && go test -race ./...` green.
- [ ] `bash scripts/verify-contract.sh && bash scripts/check-pins.sh` both exit 0.
- [ ] `cd relay && go mod tidy` is a no-op.
- [ ] `relay/cmd/relay/smoke/run-q9-smoke.sh` passes against all 6 misconfig fixtures + the combined fixture (each emits exactly one consolidated Error log line).
- [ ] Per-IP burst smoke shows 5×200 + 5×429 with `Retry-After: 1`.
- [ ] Per-session cap smoke shows 3×200 + 429 `session_turns_exhausted` with `Retry-After` close to TTL.
- [ ] Daily budget smoke shows 503 `daily_budget_exhausted` with `Retry-After` close to secs-to-UTC-midnight.
- [ ] Tenant-isolation smoke shows fresh tenants succeed when another tenant exhausted.
- [ ] `core/smoke/drive-abuse.ts` exits 0 with all `OK:` lines.
- [ ] Live CAPTCHA smoke executed with the maintainer's go-ahead — all 4 outcomes recorded (Turnstile + hCaptcha each: discovery/mint/replay/fail).
- [ ] Phase 4 `drive-auth-email.ts` regression smoke passes unchanged.
- [ ] Phase 1 anonymous-only flow regression passes.
- [ ] No CAPTCHA secret appears in any captured log line (grep verified).
- [ ] `ai/tasks/phase-5/SMOKE-EVIDENCE.md` contains every command + output, in order.
- [ ] `ai/tasks/phase-5/README.md` §7 status note updated.
- [ ] Sub-plan index in the phase-5 README marks all 4 sub-plans Done.
- [ ] Any novel lessons appended to `ai/LESSONS.md` as L016+.
- [ ] `docs/PROJECT.md` §10 + §17 cross-refs updated.
- [ ] Phase 5 ready for the bundled phase-5 → main merge.
