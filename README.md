# OpenIntake

> AI-native, self-hostable feedback & support intake — Apache-2.0 core, single-binary Go relay + embeddable Vue widget.

<!-- Status badges — fill in post-public-release:
[![CI](https://github.com/clubpetey/openintake/actions/workflows/ci.yml/badge.svg)](https://github.com/clubpetey/openintake/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/clubpetey/openintake)](https://github.com/clubpetey/openintake/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
-->

**OpenIntake** is an AI-native, self-hostable feedback & support intake stack: an embeddable Vue 3 widget + a single-binary Go relay. No SaaS dependency, no vendor lock-in, no third-party data plane — install the relay on your own infrastructure, drop the widget into your app, and route submissions into the support system you already use (Chatwoot, Zendesk, Linear, Fider, or any HTTP webhook).

## The 60-second demo

```bash
git clone https://github.com/clubpetey/openintake.git
cd openintake/examples/docker-compose
docker-compose up -d
```

That starts the relay (on `http://localhost:18080`), a fake LLM, and a webhook receiver. Submit a test ticket via `curl` (see `docs/quickstart.md`) and watch it land in the webhook-receiver logs. For the Vue widget UI, run the bare-metal `examples/vue-anonymous` path documented in the quickstart.

## What's in v0

- **5 adapters** — pick which downstream system receives your tickets
  - `webhook` — *(Free)* — POST canonical JSON to any HTTP endpoint
  - `chatwoot` — *(Free)* — open a Chatwoot conversation with the agent API
  - `fider` — *(Free)* — post a Fider idea with markdown-embedded screenshots
  - `zendesk` — *(Paid)* — create a Zendesk ticket via the v2 API
  - `linear` — *(Paid)* — create a Linear issue via the GraphQL API
- **4 LLM providers** — pick whichever you're already using
  - `anthropic` — Claude family models (default in production deployments)
  - `openai` — GPT-4o family and successors
  - `gemini` — Google Gemini family
  - `ollama` — self-hosted local models (no API cost)
- **3 authentication modes** — pick the right shape for your user model
  - `anonymous` — no auth, CAPTCHA-gated; for public marketing-site widgets
  - `email` — magic-link auth via SMTP; for known users with light identity needs
  - `sso` — JWKS or HS256 JWT verification; for SSO-backed customer portals

Plus: AI-driven classification + summarization, screenshot capture with client-side redaction, attachment upload (PNG/JPEG/WebP, 5 MB each / 10 MB aggregate), abuse gates (per-IP / per-session / daily LLM budget), Cloudflare Turnstile CAPTCHA, Prometheus metrics on an opt-in side-channel, consolidated startup-gate that flags every misconfig in one log line.

## Documentation

| Doc | Purpose |
|---|---|
| [`docs/quickstart.md`](docs/quickstart.md) | Fresh-clone to "ticket in webhook log" in 30 minutes. Docker or bare-metal. |
| [`docs/self-hosting.md`](docs/self-hosting.md) | Production deployment: binary + Docker, env vars, metrics, abuse gates, auth modes, TLS. |
| [`docs/license.md`](docs/license.md) | License-file resolution, 14-day trial, paid-adapter gate, expiry behavior. |
| [`docs/adapters.md`](docs/adapters.md) | The 5 adapters: tier, config, env vars, attachment behavior, downstream API links. |
| [`docs/attachments.md`](docs/attachments.md) | Attachment validation, per-adapter forwarding, the widget redactor UI. |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Branch model, commit conventions, the phase model, local pre-commit commands. |
| [`SECURITY.md`](SECURITY.md) | Vulnerability reporting policy + existing security stance. |
| [`docs/PROJECT.md`](docs/PROJECT.md) | Source-of-truth design document for the whole project. |

## License

OpenIntake uses a dual licensing model:

- **Apache 2.0** covers the framework — the relay, the widget, the schema, the free adapters (`webhook`, `chatwoot`, `fider`), and all LLM providers. See [`LICENSE`](LICENSE).
- **Commercial license** is required to operate the paid adapters (`zendesk`, `linear`) in production after the 14-day trial. See [`COMMERCIAL.md`](COMMERCIAL.md) for (draft) terms.

The source code is Apache 2.0 either way — the commercial gate is at runtime, not at distribution. You can read, fork, and modify everything; you need a license to **use** the paid adapters in production.

## Repo layout

```
openintake/
├── core/                # @openintake/core — shared TypeScript engine (capture, client, types)
├── vue/                 # @openintake/vue — Vue 3 widget components
├── relay/               # openintake-relay Go binary + internal packages
├── license-tool/        # maintainer-only license signer (not published)
├── schema/              # payload.v1.json — wire contract (source of truth)
├── examples/            # vue-anonymous, webhook-receiver, docker-compose
├── scripts/             # codegen-go.sh, verify-contract.sh, check-pins.sh
├── docs/                # operator-facing docs + design specs
└── ai/                  # task plans, lessons, phase READMEs (a view into how I guided claude to approach this project)
```

## Prerequisites

- **Node 24.12.0** (run `nvm use` if you use nvm)
- **Go 1.23.2**
- **POSIX shell** (Git Bash or WSL on Windows) for `scripts/codegen-go.sh` and friends

For the demo: **Docker Desktop** (macOS / Windows) or **Docker engine** (Linux).

## Build

```bash
npm ci                              # install workspace dependencies
npm run codegen                     # regenerate types from schema/payload.v1.json
cd relay && go build ./...          # compile the relay and all internal packages
```

To run the full local pre-commit suite (matches CI):

```bash
cd relay && go vet ./... && go test -race ./...
cd ../core && npm test
cd ../vue && npm test
bash scripts/verify-contract.sh
bash scripts/check-pins.sh
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full developer workflow.

## Status

OpenIntake is **pre-1.0**. The v0 wire contract is locked (`schema/payload.v1.json`), but the public release infrastructure is still local-only — see `docs/PROJECT.md` §15 for the release-pipeline status. Pin to specific commits if you depend on OpenIntake in production today; semver guarantees begin at v1.0.0.
