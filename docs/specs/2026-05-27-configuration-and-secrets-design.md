# Configuration & Secrets (production) — Design

> **Status:** Approved design decision
> **Date:** 2026-05-27
> **Applies to:** the `intake-relay` binary across all distributions (container image, native binary, `go install`)
> **Implements / refines:** [docs/PROJECT.md](../PROJECT.md) §9 (configuration), §17 (security); [Phase 1 design](2026-05-26-phase-1-walking-skeleton-design.md) §2, §7

## 1. The model: config and secrets are separate sources

| | Non-secret structural config | Secrets |
|---|---|---|
| **Examples** | `server.addr`, `cors_origins`, enabled adapters, routing rules, rate limits, model names, SSO `issuer`/`audience`/`jwks_url`, retry policy | LLM API keys, adapter API tokens, SMTP password, email-JWT secret, CAPTCHA secret, webhook auth header values |
| **Where it lives** | A YAML file (`config.yaml`) — safe to commit to git / put in a k8s ConfigMap | NEVER in the YAML. Supplied at runtime via env var or a mounted file |
| **How the YAML references it** | the value directly | the *name* of the env var that carries it (`api_key_env: "ANTHROPIC_API_KEY"`), never the value |

This split is the rule from PROJECT.md §17 ("all SMTP and adapter credentials sourced from env vars, never from the config file directly"). It exists so the structural config can be version-controlled and reviewed without ever exposing a secret, and so secrets flow from a secret store rather than a config file.

**We deliberately do NOT co-locate secrets with config** (the Spring `application.properties` style). Mixing them is the anti-pattern that leads to secrets committed to git.

## 2. Precedence

For **structural config**: defaults (compiled in) < YAML file < environment variables < CLI flags.

For **secrets**, the relay resolves each via `resolveSecret(NAME)`:

```
CLI flag (if the secret has one)
  > $NAME                  (the env var named in the YAML, e.g. ANTHROPIC_API_KEY)
  > contents of $NAME_FILE (a file path; the file's trimmed contents are the secret)
```

- If **both** `$NAME` and `$NAME_FILE` are set (non-empty), it is an **error** — no silent ambiguity about which wins.
- A **required** secret that resolves to empty → **fail fast at startup** with a clear, redacted error (the error names the env var, never the value).
- Resolved secret values are **never logged** and are redacted in error messages (PROJECT.md §17).

## 3. Why no bespoke "secrets file" parser

A single `KEY=value` file of all secrets (`.env` / Java `.properties` style) is a reasonable ergonomic for simple deployments — but it is **just environment variables in a file**, and every deployment target already turns such a file into env vars with zero application code:

- Docker: `docker run --env-file secrets.env …`
- Compose: `env_file: ./secrets.env`
- systemd (native binary): `EnvironmentFile=/etc/intake/secrets.env`
- shell/dev: `set -a; source secrets.env; set +a`

So the "one file for all secrets" experience is delivered by the platform's env-file mechanism. Building a properties/dotenv parser into the relay would add a format to maintain, invite format bikeshedding, and re-introduce a secrets-in-a-file coupling — for no capability gain. The `$NAME` path already covers it.

## 4. Where each input comes from, per deployment

- **YAML config:** mounted into the container (`-v ./config.yaml:/etc/intake/config.yaml` or a k8s ConfigMap mounted as a file), referenced via `--config`. Default lookup path `/etc/intake/config.yaml`. Not baked into the image, so config changes don't require a rebuild. For a native binary, a local path or the default.
- **Secrets:**
  - **Single combined file (simple self-host):** the operator's env-file (`--env-file` / compose `env_file:` / systemd `EnvironmentFile=`) → populates `$NAME`. *(This is the "single properties file" option.)*
  - **Per-secret mounted files (orchestrated):** Docker secrets (`/run/secrets/…`), k8s `Secret` mounted as files, Vault Agent file sinks → referenced via `$NAME_FILE`.
  - **Injected env vars (cloud secret managers):** AWS Secrets Manager/SSM, GCP Secret Manager, Azure Key Vault, Vault — the platform (ECS task `secrets:`, External Secrets Operator, Vault sidecar) injects them as `$NAME`. **No secret-manager SDK in the relay** — preserves the single-binary, no-SaaS-dependency ethos.

## 5. Consistency notes

- The **license** loader (PROJECT.md §12) already uses an env-or-file shape (`INTAKE_LICENSE` base64 env, `INTAKE_LICENSE_FILE` path). Adopting `resolveSecret` for all secrets makes the relay internally consistent with that.
- The **trial-state file** path uses `os.UserConfigDir()` (Phase-0 decomposition §4 Q3); in a container that is ephemeral, so production relies on a license (no trial), or a mounted volume if trial state must persist.

## 6. Phasing

- **Now (Phase 1):** introduce `config.resolveSecret(name)` (env-or-`_FILE`, error-on-both, fail-fast) and route the only current secret read (`ANTHROPIC_API_KEY`) through it, with unit tests. This sets the seam so every later secret read inherits it.
- **Phase 3 (adapters):** chatwoot/fider/zendesk/linear/webhook credentials resolve via `resolveSecret`.
- **Phase 4 (auth):** SMTP password, email-JWT secret resolve via `resolveSecret`.
- **Phase 5:** CAPTCHA secret resolves via `resolveSecret`.
- **Phase 7 (release/ops):** `relay/Dockerfile`, `examples/docker-compose/`, and `docs/self-hosting.md` document all three secret-delivery methods; default config path; non-root container user; read-only rootfs guidance.

## 7. Decision

Adopt the `resolveSecret` env-or-`_FILE` helper (§2) as the single secret-resolution path for the relay. Keep structural config in YAML. Do not build a secrets-file parser. Implement the helper + reroute the Phase-1 `ANTHROPIC_API_KEY` read now.
