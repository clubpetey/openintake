# intake

> Working name — final name TBD (see docs/specs decomposition design §6).

AI-native, self-hostable feedback & support intake: an embeddable widget + a single-binary Go relay.

## Repo layout

- `core/` — `@intake/core` shared TypeScript engine
- `vue/` — `@intake/vue` widget
- `relay/` — `intake-relay` Go binary
- `license-tool/` — maintainer-only license signer (not published)
- `schema/` — `payload.v1.json` wire contract (source of truth)

## Prerequisites

- Node 24.12.0 (`nvm use`)
- Go 1.23.2

## Build

```bash
npm ci
npm run codegen     # regenerate types from schema
cd relay && go build ./...
```

See `docs/` for full documentation.
