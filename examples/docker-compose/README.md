# OpenIntake — Docker-Compose Demo

A complete working intake instance you can boot in 60 seconds with one
command. Three services share a private docker-compose network: the **relay**
(openintake-relay binary, distroless), a **fake-llm** that impersonates the
Ollama API (no LLM credit consumed), and a **webhook-receiver** that logs
every submitted ticket to its stdout. Use this stack to evaluate OpenIntake,
exercise the `/init → /turn → /submit` flow against an adapter, or as a
template for your own self-hosted deployment.

## Prerequisites

- Docker Desktop (Windows/macOS) **OR** the Linux Docker engine
  (docker-engine + docker-compose-plugin)
- Free host ports: **18080** (relay HTTP), **19090** (Prometheus metrics),
  **19099** (webhook receiver), **11434** (fake-llm, Ollama-standard port).
  All four are non-standard offsets to avoid conflicts with developer-local
  processes; if any of them collide with your machine, edit
  `docker-compose.yml` and remap.

## Quickstart

```bash
cd examples/docker-compose

# Optional: copy .env.example to .env (the demo defaults to a fake LLM and
# does NOT require a real ANTHROPIC_API_KEY; the placeholder value is enough).
cp .env.example .env

# Build and start all three services in the background.
docker-compose up -d

# Verify all three containers are running.
docker-compose ps
```

You should see three rows: `openintake-relay`, `openintake-fake-llm`, and
`openintake-webhook-receiver`, all with `State: running`. The relay takes ~1
second to start after the receiver is healthy.

## Submit a ticket

The full intake flow is three HTTP calls: `/init → /turn → /submit`. Open
the receiver's log in one terminal:

```bash
docker-compose logs -f webhook-receiver
```

Then in another terminal:

```bash
# 1. /init — obtain a session_id + capabilities snapshot.
INIT=$(curl -s -X POST http://localhost:18080/v1/intake/init \
  -H "Content-Type: application/json" \
  -d '{}')
echo "$INIT"
SESSION=$(echo "$INIT" | jq -r .session_id)

# 2. /turn — stream one assistant turn (SSE). The fake-llm returns a single
#    "ok" content chunk and a done frame with token counts.
curl -N -X POST "http://localhost:18080/v1/intake/turn" \
  -H "Content-Type: application/json" \
  -H "X-Intake-Session: ${SESSION}" \
  -d '{"messages":[{"role":"user","content":"Hello, intake!"}]}'

# 3. /submit — POST the final message list. The webhook adapter forwards
#    the canonical payload to webhook-receiver:9099/intake.
curl -s -X POST "http://localhost:18080/v1/intake/submit" \
  -H "Content-Type: application/json" \
  -H "X-Intake-Session: ${SESSION}" \
  -d '{
    "messages": [
      {"role": "user", "content": "Hello, intake!"},
      {"role": "assistant", "content": "ok"}
    ],
    "client": {
      "widget_version": "demo",
      "url": "http://localhost:5173/",
      "user_agent": "curl/8.0",
      "viewport": {"w": 1280, "h": 800},
      "locale": "en-US"
    },
    "user_claims": {},
    "context": {"app_context": {}, "page_metadata": {}},
    "routing_hint": null
  }'
```

Switch back to the receiver's log terminal — you should see the canonical
payload printed as formatted JSON, including the `messages`, `client`,
`user_claims`, and `context` blocks.

## Verify the Prometheus metrics endpoint

The demo enables the off-by-default metrics endpoint so you can see the four
v0 series:

```bash
curl http://localhost:19090/metrics | head -40
```

Look for `# HELP intake_http_requests_total`, `# HELP
intake_http_request_duration_seconds`, `# HELP intake_llm_tokens_total`,
`# HELP intake_adapter_calls_total`. After driving the curl flow above,
the request counter for `/v1/intake/init`, `/v1/intake/turn`, and
`/v1/intake/submit` will each have incremented by one.

## Teardown

```bash
docker-compose down -v
```

The `-v` flag also removes the named volumes (there are none in this demo,
but it's good hygiene). The network and containers are deleted in one step;
re-running `docker-compose up -d` rebuilds from scratch.

## What's next

- See the [self-hosting guide](../../docs/self-hosting.md) for production
  deployment patterns (reverse proxy, real LLM credentials, persistent
  attachment storage).
- See the [adapters overview](../../docs/adapters.md) for the matrix of
  built-in adapters (chatwoot, fider, linear, zendesk, webhook).
- The demo's metrics endpoint is opt-in **for the demo only**. Production
  deployments should keep `observability.metrics.enabled: false` until a
  Prometheus scrape target is wired up — there is no built-in authentication
  on `/metrics`.
