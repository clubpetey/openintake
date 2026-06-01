# Quickstart

This guide gets you from a fresh clone to "I just submitted a ticket and saw it land in a webhook log" in about 30 minutes. After this, see `docs/self-hosting.md` for production configuration, `docs/license.md` for the free/paid tier model, and `docs/adapters.md` for setting up real downstream systems like Chatwoot or Zendesk.

## Prerequisites

You need **either**:

- **Docker** — Docker Desktop on macOS / Windows, or the Docker engine on Linux. This is the recommended path; the demo stack uses docker-compose and brings up everything you need.

**OR**, for the bare-metal path:

- **Go 1.23.2** — for building the relay
- **Node 24.12.0** — for the widget tooling and codegen (`nvm use` if you use nvm)
- **POSIX shell** — Git Bash or WSL on Windows; required by `scripts/codegen-go.sh`, `scripts/verify-contract.sh`, `scripts/check-pins.sh`

A real LLM API key is **not** required for the quickstart — the demo stack uses a `fake-llm` stub that emits canned SSE responses. You can wire in Anthropic, OpenAI, Gemini, or Ollama later; see `docs/self-hosting.md` § LLM providers.

## The 60-second path (docker-compose)

```bash
git clone <repo-url>
cd intake/examples/docker-compose
docker-compose up -d

# Wait ~10 seconds for the stack to come up, then:
curl -s -X POST http://localhost:18080/v1/intake/init -d '{}' | jq
```

You should see a JSON response with a `session_id` and a `capabilities` block. The relay is up.

Now submit a ticket:

```bash
SESSION=$(curl -s -X POST http://localhost:18080/v1/intake/init -d '{}' | jq -r .session_id)

curl -s -X POST http://localhost:18080/v1/intake/submit \
  -H "Content-Type: application/json" \
  -H "X-Intake-Session: $SESSION" \
  -d '{
    "messages": [{"role":"user","content":"quickstart test ticket"}],
    "client": {
      "url":"http://quickstart.test/",
      "user_agent":"curl",
      "language":"en-US",
      "viewport_width":1024,
      "viewport_height":768,
      "referrer":"",
      "page_title":"quickstart"
    },
    "user_claims": {},
    "context": {"dom_snippet":""}
  }'
```

The relay responds with `{"external_id":"...","external_url":"..."}`. The submission was forwarded to the webhook receiver in the docker-compose stack. Verify it landed:

```bash
docker-compose logs webhook-receiver | tail -50
```

You should see the canonical submission payload logged — messages, client metadata, user claims, and the (empty) attachments array. **You just ran the full intake stack end-to-end.**

When you're done:

```bash
docker-compose down -v
```

## What just happened?

The docker-compose stack ran three services:

- **relay** — the `intake-relay` Go binary, listening on port `18080` (HTTP) and `19090` (Prometheus metrics).
- **fake-llm** — a stub LLM server on port `11434` that emits canned SSE responses. The relay's `/v1/intake/turn` endpoint uses this in place of a real Anthropic / OpenAI / Gemini / Ollama call.
- **webhook-receiver** — a tiny Node.js HTTP server on port `19099` that logs every POST it receives. The relay's `webhook` adapter forwards submissions here.

Your `curl` calls drove the canonical flow:

1. **`POST /v1/intake/init`** — the widget (or, in your case, `curl`) asks the relay for a session ID and the server's published capabilities. The capabilities include the enabled auth modes, the streaming flag, and (when attachments are enabled) the per-attachment + aggregate size caps and the allowed MIME types.
2. **`POST /v1/intake/turn`** — *(skipped in this quickstart)* the widget streams a turn through the LLM. The relay handles classification, summarization, and follow-up question generation.
3. **`POST /v1/intake/submit`** — the widget posts the final canonical payload. The relay validates against `schema/payload.v1.json`, runs Phase 5 abuse gates (per-IP, per-session, budget), runs Phase 6 attachment validation, picks an adapter from `routing:`, and calls the adapter's `Create()` method. The adapter (in this case `webhook`) forwards the payload to the configured URL — the docker-compose stack's webhook-receiver service.

The `external_id` and `external_url` in the response come from the downstream system; the webhook-receiver returns a stub but a real adapter (chatwoot, zendesk, linear) returns the actual conversation / ticket / issue identifier.

## The bare-metal path

If you don't want Docker:

```bash
git clone <repo-url>
cd intake

# Install dependencies and run codegen
npm ci
npm run codegen                    # regenerate types from schema/payload.v1.json

# Build the relay
cd relay
go build -o intake-relay ./cmd/relay
cd ..

# Start the fake-llm in one terminal:
go run ./relay/cmd/fake-llm --addr :11434

# Start the webhook-receiver in another terminal:
node examples/webhook-receiver/server.mjs    # listens on :19099

# Start the relay in a third terminal, using the quickstart config:
./relay/intake-relay --config examples/docker-compose/config.yaml
```

The relay's config (used by docker-compose too) routes submissions to the webhook adapter on `http://127.0.0.1:19099/intake`. Drive it with the same `curl` commands as the 60-second path above.

## Trying the Vue widget

The bare-metal path lets you exercise the actual widget UI:

```bash
cd examples/vue-anonymous
npm install
npm run dev
```

Open `http://localhost:5173` in a browser. Click the widget bubble in the corner, type a message, and submit. The relay (running in the third terminal) processes the request through `/init` → `/turn` → `/submit`, and the webhook-receiver logs the result.

To attach a screenshot:

1. Click **Attach** in the widget panel.
2. The screenshot redactor opens, capturing the current page via `html2canvas`.
3. Draw black rectangles over any region you want to redact.
4. Click **Save**. The attachment appears in the thumbnail strip.
5. Click **Submit**. The relay validates the attachment (size, MIME, magic bytes) and forwards it to the webhook receiver as a `data:` URL inside the canonical payload.

See `docs/attachments.md` for the full attachment behavior — validation errors, per-adapter forwarding, and the redaction UI.

## Next steps

You now have a working intake stack. Where to go next depends on what you want to do:

- **Production-deploy intake** — read `docs/self-hosting.md`. Covers binary deployment via systemd, Docker deployment, reverse-proxy + TLS, env-var management, secret resolution, the Phase 5 abuse gates, the Phase 6 attachments config, and the Phase 4 auth modes.
- **Set up a real downstream system** (Chatwoot, Zendesk, Linear, Fider) — read `docs/adapters.md`. Per-adapter config keys, env vars, attachment behavior, and links to each downstream's own API docs.
- **Understand the licensing** — read `docs/license.md`. The framework is Apache 2.0; the `zendesk` and `linear` adapters are paid, with a 14-day trial. `COMMERCIAL.md` has the (draft) commercial terms.
- **Wire in a real LLM** — `docs/self-hosting.md` § LLM providers covers the four providers (Anthropic, OpenAI, Gemini, Ollama) and their config blocks. The fake-llm we used here is for development only.
- **Embed the widget in your own app** — see the `examples/vue-anonymous/` source and `vue/src/components/IntakeWidget.vue` as the reference embedding.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `docker-compose up` fails with "port already allocated" | Another process is using `18080`, `19090`, `19099`, or `11434` | Stop the conflicting process, or remap ports in `examples/docker-compose/docker-compose.yml` |
| `curl: (52) Empty reply from server` | Relay still starting | Wait 5-10 seconds; the relay logs `relay listening on :18080` when it's ready (`docker-compose logs relay`) |
| `webhook-receiver` log is empty after `/submit` | Adapter routing misconfigured | Check `docker-compose logs relay` for `adapter "webhook" called: ...`; verify `routing.default_adapter: "webhook"` in `examples/docker-compose/config.yaml` |
| `{"error":{"code":"attachments_disabled",...}}` on a submit with attachments | Attachments are off in this config | The quickstart's docker-compose config has attachments enabled by default; if you've edited the config, set `attachments.enabled: true` and restart |
| `/v1/intake/init` returns no `capabilities.attachments` block | Attachments are off, or the intersected MIME allowlist is empty | See `docs/attachments.md` § Capabilities discovery |
| `curl` works but the Vue widget shows a CORS error | Relay's `server.cors_origins` doesn't include the widget origin | Add the widget's origin (`http://localhost:5173`) to `server.cors_origins` in the relay config |

For deeper issues, see `docs/self-hosting.md` § Logging — the relay emits structured JSON logs to stdout with a `level` field; grep for `"level":"ERROR"` to find startup or runtime failures.

## See also

- `docs/self-hosting.md` — production deployment, env vars, metrics, abuse gates, auth modes.
- `docs/adapters.md` — per-adapter setup for Chatwoot, Zendesk, Linear, Fider, webhook.
- `docs/license.md` — license-file resolution, trial mode, paid-adapter gate.
- `docs/attachments.md` — attachment validation, per-adapter forwarding, the widget redactor UI.
- `docs/PROJECT.md` — the source-of-truth design document.
- `CONTRIBUTING.md` — how to contribute, how phases work, local pre-commit commands.
