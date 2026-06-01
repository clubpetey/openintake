# Self-hosting intake

This document is the production operator's reference. It assumes you already ran the quickstart (`docs/quickstart.md`) and want to deploy intake for real: behind a load balancer, with TLS, with secrets managed properly, with metrics scraped into Prometheus, and with the right abuse / attachment / auth posture for your environment.

Sections:

- [Deployment paths](#deployment-paths) — bare-metal binary vs. Docker
- [Configuration file](#configuration-file) — the `relay.yaml` schema
- [Environment variables and secrets](#environment-variables-and-secrets) — every `*_env` key
- [LLM providers](#llm-providers) — Anthropic, OpenAI, Gemini, Ollama
- [Authentication modes](#authentication-modes) — anonymous, email magic-link, SSO/JWKS
- [Abuse and rate-limiting](#abuse-and-rate-limiting) — per-IP, per-session, daily budget, CAPTCHA
- [Attachments](#attachments) — size caps, MIME allowlist, storage mode
- [Adapters](#adapters) — enabling adapters, routing rules
- [Observability — Prometheus metrics](#observability--prometheus-metrics) — opt-in metrics endpoint
- [Trusted proxies](#trusted-proxies) — CIDR-based client-IP resolution
- [Logging](#logging) — JSON to stdout; shipping to Loki/Datadog/Splunk
- [License](#license) — license file path and trial behavior
- [Reverse proxy and TLS](#reverse-proxy-and-tls) — Caddy and nginx examples
- [The startup gate](#the-startup-gate) — consolidated misconfig reporting
- [Troubleshooting](#troubleshooting)

## Deployment paths

### Bare-metal binary

The fastest production path is a single static binary supervised by systemd. Download the prebuilt binary for your platform from the releases page (when public; see `docs/PROJECT.md` §15 for the release-pipeline status), or build from source:

```bash
git clone <repo-url>
cd intake
npm ci
npm run codegen
cd relay
go build -ldflags '-s -w' -trimpath -o intake-relay ./cmd/relay
sudo install -m 0755 intake-relay /usr/local/bin/intake-relay
```

A minimal systemd unit at `/etc/systemd/system/intake-relay.service`:

```ini
[Unit]
Description=intake relay
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=intake
Group=intake
ExecStart=/usr/local/bin/intake-relay --config /etc/intake/relay.yaml
EnvironmentFile=/etc/intake/relay.env
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
NoNewPrivileges=true
ReadWritePaths=/var/lib/intake

[Install]
WantedBy=multi-user.target
```

Create the user, config dir, and state dir:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin intake
sudo install -d -o intake -g intake -m 0750 /etc/intake /var/lib/intake
sudo install -m 0640 -o root -g intake my-relay.yaml /etc/intake/relay.yaml
sudo install -m 0640 -o root -g intake my-license.json /etc/intake/license.json
sudo install -m 0640 -o root -g intake my-relay.env /etc/intake/relay.env

sudo systemctl daemon-reload
sudo systemctl enable --now intake-relay
sudo systemctl status intake-relay
```

The `relay.env` file holds secrets, one per line in `KEY=value` shell syntax. Restrict it to mode `0640` and ownership `root:intake`.

### Docker

The release ships a multi-stage distroless image at `ghcr.io/intake/intake-relay:vX.Y.Z` (when public). Image properties:

- Base: `gcr.io/distroless/static-debian12:nonroot`
- Runs as UID `65532` (`nonroot`)
- No shell, no package manager
- Total size < 50 MB
- Exposes `8080` (relay HTTP) and `9090` (metrics, when enabled)

Run it with a config file and environment variables:

```bash
docker run -d \
  --name intake-relay \
  -p 8080:8080 \
  -p 9090:9090 \
  -v /etc/intake/relay.yaml:/etc/intake/relay.yaml:ro \
  -v /etc/intake/license.json:/etc/intake/license.json:ro \
  --env-file /etc/intake/relay.env \
  --read-only \
  --restart unless-stopped \
  ghcr.io/intake/intake-relay:vX.Y.Z \
  --config /etc/intake/relay.yaml
```

Use `docker-compose` for multi-service deployments; the `examples/docker-compose/` directory in the repo has a working template.

## Configuration file

The relay reads a YAML config file at the path passed via `--config`. Every block is documented below; the canonical sample is `relay/internal/config/testdata/sample.yaml`.

Top-level structure (each block is detailed in its own section):

```yaml
server:        # HTTP server, CORS, trusted proxies
llm:           # LLM provider selection + per-provider config
auth:          # anonymous / email / sso modes
adapters:      # webhook / chatwoot / fider / zendesk / linear
routing:       # default adapter + classification-based rules
license:       # license file path override
captcha:       # CAPTCHA provider (Cloudflare Turnstile)
ratelimit:     # per-IP / per-session / daily-budget caps
attachments:   # size caps, MIME allowlist, storage mode
observability: # log level/format, Prometheus metrics
```

## Environment variables and secrets

intake follows a strict "secrets via env, not in config" pattern. The config file references env var **names** via `*_env` keys; the relay resolves them at startup. This means the config file is safe to commit, mount, or check into source control — no secret material is in it.

The `config.ResolveSecret` / `RequireSecret` contract:

- Every `*_env` field in the config file names an environment variable. The relay reads `os.Getenv(<name>)` at startup; the value (the actual secret) is held in process memory and never logged.
- If a required env var is missing or empty at startup, the relay's consolidated startup-gate adds an entry like `adapter "chatwoot": api_token_env="CHATWOOT_TOKEN" is not set in the environment` to the problems slice and exits 1.
- Empty `*_env` values are treated as "not set" (per L016-adjacent contract); set a real value or remove the field if optional.
- Tokens are never echoed in error messages — see LESSONS L005 / L011.

### Full env var reference

The table below lists every `*_env` field in the canonical config. Set the named env var to the actual secret value (e.g. set `ANTHROPIC_API_KEY=sk-ant-...` if `llm.anthropic.api_key_env: "ANTHROPIC_API_KEY"`).

| Config key | Env var (default name) | Required when | Holds |
|---|---|---|---|
| `llm.anthropic.api_key_env` | `ANTHROPIC_API_KEY` | `llm.provider: "anthropic"` | Anthropic API key |
| `llm.openai.api_key_env` | `OPENAI_API_KEY` | `llm.provider: "openai"` | OpenAI API key |
| `llm.gemini.api_key_env` | `GEMINI_API_KEY` | `llm.provider: "gemini"` | Google Gemini API key |
| `llm.ollama.bearer_token_env` | (empty) | optional, when fronting Ollama with auth | Ollama bearer token |
| `auth.email.smtp_pass_env` | `INTAKE_SMTP_PASS` | `auth.modes.email: true` | SMTP password |
| `auth.email.jwt_secret_env` | `INTAKE_EMAIL_JWT_SECRET` | `auth.modes.email: true` | Email-mode session JWT signing secret (32+ bytes) |
| `auth.sso.hs256_secret_env` | (empty) | `auth.modes.sso: true` with HS256 | SSO HS256 secret (when not using JWKS) |
| `adapters.webhook.headers.*` | (any) | optional | Webhook auth headers (use `${ENV_VAR}` interpolation) |
| `adapters.chatwoot.api_token_env` | `CHATWOOT_TOKEN` | `adapters.chatwoot.enabled: true` | Chatwoot agent API token |
| `adapters.fider.api_key_env` | `FIDER_API_KEY` | `adapters.fider.enabled: true` | Fider API key |
| `adapters.zendesk.api_token_env` | `ZENDESK_API_TOKEN` | `adapters.zendesk.enabled: true` | Zendesk API token (paired with `email`) |
| `adapters.linear.api_key_env` | `LINEAR_API_KEY` | `adapters.linear.enabled: true` | Linear API key |
| `captcha.secret_key_env` | `INTAKE_TURNSTILE_SECRET` | `captcha.enabled: true` | Cloudflare Turnstile server-side secret |
| `INTAKE_LICENSE` | (env-only) | optional | License JSON inline (one-line) |
| `INTAKE_LICENSE_FILE` | (env-only) | optional | Path to license file (overrides `license.file`) |

Two env vars (`INTAKE_LICENSE`, `INTAKE_LICENSE_FILE`) are env-only — they are not referenced from the config file. See `docs/license.md` § "License file path resolution" for the resolution order.

### Secret management patterns

| Deployment | Recommended pattern |
|---|---|
| systemd + bare metal | `EnvironmentFile=/etc/intake/relay.env`, mode `0640`, owned `root:intake` |
| Docker | `--env-file /etc/intake/relay.env` or Docker secrets (`secrets:` block in compose) |
| Kubernetes | mount a `Secret` as env vars on the pod spec |
| Hashicorp Vault | sidecar template renders `/etc/intake/relay.env` from Vault KV |
| Cloud KMS | container init script fetches secrets, writes `relay.env`, then execs the relay |

Never put secrets in the YAML config file. Even if you encrypt the YAML at rest, the running process logs config-load events at `slog.Info` and an operator misconfigured to log the loaded config would leak the secret. The `*_env` indirection is the only supported pattern.

## LLM providers

The `llm:` block configures the upstream LLM. Exactly one provider is active per relay process (set via `llm.provider`).

```yaml
llm:
  provider: "anthropic"        # anthropic | openai | gemini | ollama
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-sonnet-4-6"
    max_tokens: 2048
  openai:
    api_key_env: "OPENAI_API_KEY"
    model: "gpt-4o-mini"
    max_tokens: 512
  gemini:
    api_key_env: "GEMINI_API_KEY"
    model: "gemini-2.0-flash"
    max_tokens: 512
  ollama:
    base_url: "http://localhost:11434"
    model: "llama3.1"
    bearer_token_env: ""        # optional, when fronting with auth
    max_tokens: 512
  system_prompt_file: ""        # optional override; uses the built-in prompt when empty
```

`max_tokens` caps each turn's output; combine with the daily LLM budget (see § Abuse and rate-limiting) for cost control.

`system_prompt_file` lets you override the built-in classification/summarization prompt. Leave empty for the default. Custom prompts must produce JSON-shaped classification output per `docs/PROJECT.md` §6.

## Authentication modes

intake supports three auth modes; multiple can be enabled simultaneously, and the widget picks one based on the user's identity state.

```yaml
auth:
  modes:
    anonymous: true
    email: true
    sso: true
  email:
    smtp_host: "smtp.example.com"
    smtp_port: 587
    smtp_user: "intake@example.com"
    smtp_pass_env: "INTAKE_SMTP_PASS"
    from: "Intake <intake@example.com>"
    code_ttl: "10m"                       # magic-link code lifetime
    jwt_ttl: "15m"                        # session JWT lifetime after verification
    jwt_secret_env: "INTAKE_EMAIL_JWT_SECRET"
  sso:
    issuer: "https://acme.us.auth0.com/"
    audience: "https://api.acme.com"
    jwks_url: "https://acme.us.auth0.com/.well-known/jwks.json"
    hs256_secret_env: ""                  # OR a JWKS URL above
    claims:
      user_id: "sub"
      email: "email"
      display_name: "name"
  anonymous:
    allow_without_captcha: false          # true => no captcha required for anonymous submit
```

### Mode interactions

- **Anonymous + CAPTCHA** — when `auth.modes.anonymous: true`, you SHOULD set `captcha.enabled: true` and `captcha.required_for: ["anonymous"]`. The Phase 5 startup gate refuses to start with `anonymous: true` and no CAPTCHA unless `anonymous.allow_without_captcha: true` is explicit. This invariant prevents accidental open-internet relay deployments.
- **Email magic-link** — the user enters their email, receives a one-time code via SMTP, and exchanges it for a session JWT signed with `jwt_secret_env`. The token is marked consumed BEFORE the JWT is issued (LESSONS L018), so a 5xx retry is an intentional duplicate.
- **SSO** — JWKS or HS256. JWKS pulls public keys from `jwks_url` and rotates automatically. HS256 is for environments where you control the signer; the secret must be 32+ bytes. The algorithm is pinned via `WithValidMethods` to prevent alg-confusion attacks (LESSONS L013).

### Claim mapping

The `auth.sso.claims` block maps your IDP's JWT claim names to intake's canonical fields. Adjust for your IDP (Auth0, Okta, Azure AD, Cognito, etc.). The defaults match Auth0's OIDC claim names.

## Abuse and rate-limiting

Phase 5 introduced multi-layer abuse gates. All limits are evaluated in order; the first one tripped returns 429.

```yaml
ratelimit:
  per_ip:
    requests_per_second: 2.0
    burst: 10
    idle_ttl: "5m"
  per_session:
    max_turns: 30
    max_input_tokens: 12000
    session_ttl: "30m"
  daily_llm_budget:
    max_input_tokens: 1000000
    max_output_tokens: 200000
    action_on_exceeded: "reject"          # only "reject" supported in v0
captcha:
  enabled: false
  provider: "turnstile"                   # only "turnstile" supported in v0
  site_key: "0x4AAA000000ExampleSiteKey"
  secret_key_env: "INTAKE_TURNSTILE_SECRET"
  required_for: ["anonymous"]             # ["anonymous"] | ["email"] | ["anonymous","email"]
```

### Per-IP

Token-bucket via `golang.org/x/time/rate`. `requests_per_second` is the long-run rate; `burst` allows short spikes. `idle_ttl` is the inactivity timeout before the bucket is reaped from memory. Tune by:

- Light public exposure (a documentation site widget) — `rps: 2.0, burst: 10` (the defaults).
- Heavy public exposure (a marketing site with high abandonment) — bump to `rps: 5.0, burst: 30`.
- Internal-only deployment behind SSO — `rps: 50.0, burst: 200` (the per-session caps still apply).

### Per-session

Per-session caps protect against a single user (or a single fraudulent session) running up an LLM bill. `max_turns` caps how many `/turn` calls one session can make; `max_input_tokens` caps cumulative input tokens; `session_ttl` is the absolute session lifetime.

### Daily budget

A global guardrail on LLM cost. `max_input_tokens` + `max_output_tokens` are summed across all sessions per UTC day. `action_on_exceeded` is the response when the budget is hit; **only `"reject"` is supported in v0** — the relay returns 429 to any new `/turn` request until the next UTC midnight. The startup-gate refuses to start with `action_on_exceeded: "queue"` or any other value.

### CAPTCHA

When enabled, sessions in the modes listed in `required_for` must include a verified CAPTCHA token in their `/init` request. The widget integrates Cloudflare Turnstile by default. Set:

- `site_key` — the public site key shown in the widget.
- `secret_key_env` — the env var holding the server-side secret used to verify tokens against Cloudflare.

Self-hosted CAPTCHA providers are not supported in v0.

## Attachments

The full attachment config and per-adapter behavior is documented in `docs/attachments.md`. The operator-facing summary:

```yaml
attachments:
  enabled: true                          # default true
  max_size_bytes: 5242880                # 5 MB per attachment
  max_total_bytes: 10485760              # 10 MB aggregate per request
  allowed_mime_types: ["image/png", "image/jpeg", "image/webp"]
  storage:
    mode: "forward"                      # only "" or "forward" supported in v0
```

- `storage.mode: "forward"` means "no local storage; every attachment is forwarded to the chosen adapter via its native upload mechanism." This is the only supported mode in v0; `"s3"` is a v1+ hook.
- When attachments are enabled, the submit body cap rises from 1 MB to 14 MB; when disabled, the 1 MB cap is preserved (a body-cap regression test asserts this).
- The published `capabilities.attachments.allowed_mime_types` is the **union** of every enabled adapter's `Capabilities().AcceptedMIMETypes` **intersected** with `cfg.attachments.allowed_mime_types`. When the intersection is empty (or `enabled: false`), the `attachments` block is omitted from `/init`, and the widget hides the Attach button.

See `docs/attachments.md` for validation error codes, per-adapter upload mechanics, and the widget redactor UI.

## Adapters

See `docs/adapters.md` for the full per-adapter config matrix. The operator-facing structure:

```yaml
adapters:
  webhook:
    enabled: true
    url: "https://example.com/intake"
    headers:
      X-Custom-Auth: "Bearer ..."
  chatwoot:
    enabled: true
    base_url: "https://app.chatwoot.com"
    account_id: 1
    inbox_id: 3
    api_token_env: "CHATWOOT_TOKEN"
  fider:
    enabled: true
    base_url: "https://feedback.example.com"
    api_key_env: "FIDER_API_KEY"
  zendesk:
    enabled: true
    subdomain: "acme"
    email: "agent@acme.com"
    api_token_env: "ZENDESK_API_TOKEN"
    default_priority: "normal"
  linear:
    enabled: true
    api_key_env: "LINEAR_API_KEY"
    team_id: "TEAM_ID_HERE"

routing:
  default_adapter: "chatwoot"
  rules:
    - when:
        classification: "bug"
      to: "linear"
    - when:
        classification: ["question", "other"]
      to: "chatwoot"
```

`zendesk` and `linear` are paid adapters; see `docs/license.md`.

The relay refuses to start with `enabled: true` for any adapter whose `*_env` secrets are not set, or whose `Configure()` fails (post Phase 7-i: these are consolidated startup-problems entries, not silent disablements). The relay also refuses to start when no adapter is enabled — the relay is non-functional without one.

## Observability — Prometheus metrics

Phase 7 added an opt-in Prometheus metrics endpoint on a separate HTTP server.

```yaml
observability:
  log_level: "info"           # reserved for v1+
  log_format: "json"          # "json" or "text"
  metrics:
    enabled: false            # default false (off-by-default invariant)
    addr: ":9090"             # default port; bind interface = all interfaces
```

When `metrics.enabled: true`:

- The metrics server listens on `addr` (default `:9090`).
- `GET /metrics` returns text/plain in Prometheus exposition format.
- Four series are exported (see below).
- The metrics server is **operationally independent of the main relay**: a port-bind failure on the metrics server is logged at `Error` level but does NOT crash the relay. Observability cannot be allowed to brick the service it observes.

When `metrics.enabled: false`:

- The metrics server is not started. The port is not bound.
- The metrics middleware is a literal passthrough; zero observable cost compared to Phase 6.
- All `Record*` hooks in the request path are no-ops.

### The four series

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `intake_http_requests_total` | counter | `path`, `status` | Total HTTP requests by chi route pattern and HTTP status |
| `intake_http_request_duration_seconds` | histogram | `path` | Request duration by chi route pattern |
| `intake_llm_tokens_total` | counter | `provider`, `direction` | LLM tokens consumed; `direction` in `{input, output}` |
| `intake_adapter_calls_total` | counter | `adapter`, `result` | Adapter `Create()` invocations; `result` in `{success, error}` |

The `path` label uses chi's `RoutePattern()` to bound cardinality — every request to `/v1/intake/submit?session_id=...` is one label value, not one per session ID.

### PromQL examples

```promql
# 5xx error rate (5-minute window)
sum by (path) (rate(intake_http_requests_total{status=~"5.."}[5m]))

# p95 latency on /submit (5-minute window)
histogram_quantile(0.95,
  sum by (path, le) (rate(intake_http_request_duration_seconds_bucket{path="/v1/intake/submit"}[5m])))

# LLM output token burn rate per hour
sum by (provider) (rate(intake_llm_tokens_total{direction="output"}[1h]))

# Adapter failure rate
sum by (adapter) (rate(intake_adapter_calls_total{result="error"}[5m]))
```

### Scrape config

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'intake-relay'
    scrape_interval: 30s
    static_configs:
      - targets: ['intake.internal:9090']
```

### Securing the metrics endpoint

The metrics endpoint is **unauthenticated** in v0 — there is no token check, no client cert verification, no IP allowlist baked into the relay. Operators are expected to put it behind a private network or an authenticated reverse proxy. **Do NOT expose `:9090` to the public internet.**

Common patterns:

- **Bind to localhost only** — `addr: "127.0.0.1:9090"` and scrape from a Prometheus instance on the same host.
- **Bind to a private network** — `addr: "10.0.0.5:9090"` and put a network ACL between the public internet and the metrics port.
- **Authenticated reverse proxy** — set `addr: "127.0.0.1:9090"` and put nginx / Caddy in front with basic auth or mTLS.

No metric is sensitive in itself, but the cardinality of `path` labels can reveal endpoint structure, and the `intake_llm_tokens_total` series reveals usage volume.

## Trusted proxies

When intake is behind a load balancer or reverse proxy, the client IP must be resolved from `X-Forwarded-For`. The `server.trusted_proxies` config controls which proxy IPs are trusted to set that header.

```yaml
server:
  addr: ":8080"
  external_url: "https://intake.example.com"
  cors_origins:
    - "https://app.example.com"
  trusted_proxies:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
    - "172.16.0.0/12"
```

- Every entry MUST be a valid CIDR block. Invalid CIDRs are caught by the consolidated startup gate (`server.trusted_proxies contains an invalid CIDR "not-a-cidr"`).
- Requests from non-trusted addresses ignore any `X-Forwarded-For` header — the connection's `RemoteAddr` wins. This prevents IP spoofing from outside the trusted network.
- Set to an empty list `[]` if intake is directly internet-facing (no proxy). The per-IP rate limiter will use the connection's `RemoteAddr` directly.

Behind a public CDN (Cloudflare, Fastly, etc.), include the CDN's published IP ranges. Refresh those ranges periodically — the relay does not auto-update them.

## Logging

The relay emits structured JSON logs to stdout via `slog`. One line per event; `level` in `DEBUG`, `INFO`, `WARN`, `ERROR`.

```json
{"time":"2026-06-01T14:32:00.123Z","level":"INFO","msg":"relay listening","addr":":8080"}
{"time":"2026-06-01T14:32:01.456Z","level":"INFO","msg":"submit accepted","session_id":"sess-...","adapter":"chatwoot","external_id":"42"}
{"time":"2026-06-01T14:32:02.789Z","level":"WARN","msg":"per-IP rate limit","ip":"203.0.113.5","path":"/v1/intake/turn"}
```

Set `observability.log_format: "text"` for human-readable output in development.

### Shipping logs

Because logs are line-delimited JSON to stdout, any standard log-shipping tool works:

- **Loki + Promtail / Alloy** — pick up stdout from the systemd journal or Docker logs; parse with the `json` stage.
- **Datadog Agent** — install the Datadog Agent on the host; configure `logs.processing_rules` for the `intake-relay` source.
- **Splunk Universal Forwarder** — point at the journal or Docker logs; use `INDEXED_EXTRACTIONS = json`.
- **Fluentd / Fluent Bit / Vector** — same shape; any JSON-aware shipper works.

### Sensitive data

intake's logging discipline (LESSONS L005 / L011):

- API tokens, magic-link codes, and CAPTCHA secrets are **never** logged verbatim — they are scrubbed before any log emission.
- Adapter errors are redacted-before-truncated: a truncated middle cannot leak the front of a token.
- Submit payloads are NOT logged in full at `INFO`. The submit handler logs `session_id`, `adapter`, `external_id`, and (on error) a redacted error summary.

If you need richer log shipping for support escalations, set `log_level: "debug"` temporarily — but expect higher log volume and an INFO-level reminder logged at startup.

## License

See `docs/license.md` for the full license model. The operator-facing summary:

- License file path is resolved via (in order) the `--license` CLI flag, `INTAKE_LICENSE` env (inline JSON), `INTAKE_LICENSE_FILE` env (path), then default paths starting with `/etc/intake/license.json`.
- Without a license, the relay enters a 14-day trial. All adapters work during the trial.
- After trial or license expiry, free adapters continue, paid adapters (`zendesk`, `linear`) are disabled with a `slog.Warn` line each.
- The license check is **fail-open in favor of availability** — a signature mismatch, missing file, or expired license never bricks startup.

## Reverse proxy and TLS

intake does not terminate TLS itself in v0 — put it behind a reverse proxy. Two reference configurations:

### Caddy

```caddyfile
intake.example.com {
    encode gzip zstd

    @widget_origin {
        header Origin "https://app.example.com"
    }

    handle /v1/intake/* {
        reverse_proxy 127.0.0.1:8080
    }

    handle /v1/health {
        reverse_proxy 127.0.0.1:8080
    }

    log {
        output file /var/log/caddy/intake.log
        format json
    }
}
```

Caddy handles TLS via Let's Encrypt automatically. Add CIDR-based ACLs at the firewall layer; Caddy's `remote_ip` matcher is also available for soft restrictions.

### nginx

```nginx
upstream intake {
    server 127.0.0.1:8080;
    keepalive 16;
}

server {
    listen 443 ssl http2;
    server_name intake.example.com;

    ssl_certificate     /etc/letsencrypt/live/intake.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/intake.example.com/privkey.pem;

    location /v1/intake/turn {
        # SSE — disable buffering
        proxy_buffering off;
        proxy_cache off;
        proxy_pass http://intake;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 300s;
    }

    location /v1/intake/ {
        proxy_pass http://intake;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /v1/health {
        proxy_pass http://intake;
        access_log off;
    }
}
```

**Critical for SSE**: the `/turn` endpoint streams Server-Sent Events; `proxy_buffering off` and `proxy_cache off` are required, plus a `proxy_read_timeout` long enough to cover a turn's latency.

Add `nginx`'s `set_real_ip_from` matching your `trusted_proxies` CIDRs if you want nginx to rewrite the source IP before the relay sees it.

## The startup gate

intake's consolidated startup gate (Phase 5 introduced; Phase 7-i extended) collects every misconfiguration across every subsystem into a single ERROR log line at startup, then exits with code 1. **One restart cycle reveals every misconfig.**

Example output for a maximally-broken config:

```json
{
  "time": "2026-06-01T14:23:01Z",
  "level": "ERROR",
  "msg": "relay: startup config errors",
  "count": 6,
  "problems": [
    "auth.modes.anonymous=true requires captcha.enabled=true OR auth.anonymous.allow_without_captcha=true",
    "server.trusted_proxies contains an invalid CIDR \"not-a-cidr\"",
    "ratelimit.daily_llm_budget.action_on_exceeded=\"queue\" is not supported in v0 (only \"reject\")",
    "adapter \"chatwoot\": api_token_env=\"NONEXISTENT_VAR\" is not set in the environment",
    "attachments.storage.mode=\"s3\" is not supported in v0 (only \"\" or \"forward\")",
    "attachments.max_size_bytes=20000000 exceeds attachments.max_total_bytes=10000000"
  ]
}
```

Fix all six entries in one edit, restart, the relay is up.

The gate enforces invariants across:

- **Phase 4** — `auth.modes.anonymous=true` requires CAPTCHA unless explicitly overridden.
- **Phase 5** — `server.trusted_proxies` CIDR parsing; `ratelimit.daily_llm_budget.action_on_exceeded` validation.
- **Phase 6** — `attachments.storage.mode` is `""` or `"forward"`; `max_size_bytes <= max_total_bytes`.
- **Phase 7-i** — per-adapter `Configure()` failures (e.g. missing env var, missing required key); "no adapters enabled" check.

License-gate disablements are NOT fatal (they are `slog.Warn` lines) — free-mode is a valid operating state.

Metrics-port conflicts are NOT fatal either (they are runtime warnings) — observability cannot brick the service it observes.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `relay: startup config errors` with N problems | One or more misconfigs across phases | Fix every entry in the `problems` slice; restart |
| `adapter "chatwoot": api_token_env="X" is not set in the environment` | Env var not exported to the relay process | `systemctl edit intake-relay` to add `Environment=X=...`, or update `EnvironmentFile` |
| `auth.modes.anonymous=true requires captcha.enabled=true` | Forgot to enable CAPTCHA on a public-facing relay | Set `captcha.enabled: true` + provide site/secret keys, OR set `auth.anonymous.allow_without_captcha: true` if you really want unauthenticated anonymous |
| `attachments.storage.mode="s3" is not supported in v0` | Tried to use the v1+ S3 hook | Set `storage.mode: "forward"` (the only supported v0 value) |
| `/metrics` returns "connection refused" on port 9090 | `observability.metrics.enabled: false` (default) | Set `observability.metrics.enabled: true` and restart |
| `/metrics` works but Prometheus shows 0 series | Scrape config wrong; check `prometheus.yml` `targets` | Run `curl http://intake:9090/metrics` from the Prometheus host to verify reachability |
| 429 on every request after the first 10 | Per-IP rate limit too tight | Bump `ratelimit.per_ip.requests_per_second` / `burst` |
| 429 after ~30 turns from one user | Per-session cap hit (the expected behavior) | Either raise `per_session.max_turns` or use email/SSO mode to identify legitimate heavy users |
| SSE `/turn` connections hang at 60 seconds | Reverse proxy buffering | See § Reverse proxy and TLS — `proxy_buffering off` + long `proxy_read_timeout` |
| 502 from `/submit` after a long pause | Adapter timeout (Chatwoot / Zendesk / Linear unreachable) | Check the downstream system; intake retries 5xx per `adapters.<name>.retry` if configured |
| Empty `capabilities.attachments` block in `/init` | Attachments disabled, OR the intersected MIME allowlist is empty | See `docs/attachments.md` § Capabilities discovery |
| `license: signature verification failed` | License file is from a different keypair than the relay binary | Confirm you have the right license for this release; contact licensing |

## See also

- `docs/quickstart.md` — fresh-clone-to-running-stack in 30 minutes.
- `docs/adapters.md` — per-adapter config and downstream API references.
- `docs/attachments.md` — attachment validation, per-adapter forwarding, redactor UI.
- `docs/license.md` — license file resolution, trial mode, paid-adapter gate.
- `docs/PROJECT.md` — source-of-truth design document.
- `SECURITY.md` — vulnerability reporting, existing security stance.
- `CONTRIBUTING.md` — for operators who also want to contribute upstream.
