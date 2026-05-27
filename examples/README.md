# Intake Examples

This directory contains runnable examples for the Intake project.

## Prerequisites

- Node 24.12.0+
- Go 1.23.2+
- `ANTHROPIC_API_KEY` exported in your shell
- Monorepo deps installed: `npm install` from the repo root

## Running the Phase 1 Smoke

These three processes must all run simultaneously (use separate terminals):

### Terminal 1 — Webhook receiver

```bash
npm run -w examples-webhook-receiver start
```

Expected: `Webhook receiver listening on http://localhost:9099/intake`

### Terminal 2 — Relay

```bash
cd relay
go run ./cmd/relay --config ../config.yaml
```

Expected: server starts on `:8080`; `GET http://localhost:8080/v1/health` returns `200`.

### Terminal 3 — Vue example

```bash
npm run -w examples-vue-anonymous dev
```

Expected: Vite dev server starts on `http://localhost:5173`.

### Browser

Open `http://localhost:5173`. You will see the demo page.

- Click the **chat bubble** button in the bottom-right corner.
- Type a message describing an issue (e.g. "The login button is broken").
- Send it. The assistant will ask clarifying questions.
- Send at least one more reply.
- Click **Submit**.

The widget should display the returned ticket ID. The webhook receiver (Terminal 1) should log the full canonical payload as formatted JSON.

## CORS Note

The Vite dev server runs on `http://localhost:5173` by default. This origin is already in the sample `config.yaml`:

```yaml
server:
  cors_origins:
    - "http://localhost:5173"
```

If you change the port (`vite --port XXXX`), update `cors_origins` in `config.yaml` to match.

## Security Note

The widget NEVER holds or transmits the `ANTHROPIC_API_KEY`. The key lives only in the relay process (read from env). The browser only ever talks to the relay at `http://localhost:8080`.

## examples/vue-anonymous

A minimal Vite+Vue SPA demonstrating anonymous integration. Mounts `IntakeWidget` from `@intake/vue` pointed at `http://localhost:8080`.

## examples/webhook-receiver

A ~30-line Node HTTP server that logs POST bodies on `:9099/intake`. Used by the smoke to verify the relay delivers a schema-valid canonical payload.
