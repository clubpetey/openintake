# License model

OpenIntake uses a dual licensing model:

- **Apache 2.0** covers the framework — the relay binary, the widget packages, the schema, all free adapters, and all LLM providers. You can read, fork, modify, and redistribute under Apache 2.0 terms (see `LICENSE` at the repo root).
- **Commercial license** is required to operate the paid adapters (`zendesk`, `linear`) in production. The source code is still Apache 2.0 — the gate is at runtime, not at distribution. See `COMMERCIAL.md` for the (draft) commercial terms.

This document explains how the runtime license check works, how to install a license key, and what happens when the trial expires or a license expires.

See also: `docs/PROJECT.md` §12 (license model rationale) and §13 (free/paid adapter matrix).

## Adapter tier matrix

| Adapter | Tier | Requires license to operate in production? |
|---|---|---|
| `webhook` | Free | No |
| `chatwoot` | Free | No |
| `fider` | Free | No |
| `zendesk` | **Paid** | Yes — after the 14-day trial expires |
| `linear` | **Paid** | Yes — after the 14-day trial expires |

LLM providers (`anthropic`, `openai`, `gemini`, `ollama`) and auth modes (`anonymous`, `email`, `sso`) are always free.

## License key format

A license key is an Ed25519-signed JSON document. The shape is documented in `docs/PROJECT.md` §12; the canonical fields are:

```json
{
  "license_id": "lic-2026-xxxxx",
  "subject": "Acme Corp",
  "tier": "commercial",
  "enabled_adapters": ["zendesk", "linear"],
  "issued_at": "2026-01-15T00:00:00Z",
  "expires_at": "2027-01-15T00:00:00Z",
  "signature": "base64url-ed25519-sig-over-canonical-form"
}
```

The relay verifies the signature against a baked-in public key on startup (the public key is compiled into the binary at release time; see `intake/license` for the verification primitive). License keys are issued by the maintainer's `license-tool/` CLI, which is not redistributed (per `docs/PROJECT.md` §15).

## License file path resolution

The relay resolves the license file in the following order; the first one found wins:

1. **`--license <path>`** — CLI flag passed to `openintake-relay`.
2. **`INTAKE_LICENSE`** environment variable — inline contents of the license JSON (one-line; useful for container deployments).
3. **`INTAKE_LICENSE_FILE`** environment variable — path to a license file.
4. **Default paths**, tried in order:
   - `/etc/openintake/license.json` (Linux/Unix production deployments)
   - `$XDG_CONFIG_HOME/openintake/license.json` (XDG-compliant Linux desktops)
   - `os.UserConfigDir()/openintake/license.json` (Linux: `~/.config/openintake/`; macOS: `~/Library/Application Support/openintake/`; Windows: `%AppData%/intake/`)

If none of the above is found, the relay enters **trial mode** (see below).

The license file path actually used is logged on startup at `slog.Info` level (without the contents).

## Trial mode

On the first startup with no license file resolved, the relay creates an installation-state file at `os.UserConfigDir()/openintake/state.json`:

```json
{
  "install_id": "uuid-v4",
  "trial_started_at": "2026-06-01T14:32:00Z",
  "trial_ends_at": "2026-06-15T14:32:00Z"
}
```

The state file is **only** consulted to determine whether the trial window is still open. It is not signed; tampering with it would extend the trial window for one install, but the maintainer's terms-of-use (see `COMMERCIAL.md`) explicitly forbid that and the maintainer reserves the right to audit.

During the trial:

- **All adapters are enabled** (free + paid).
- The relay logs `license: trial active, expires <date>` at `slog.Info` on startup.
- The trial duration is **14 days** from the `trial_started_at` timestamp.

## Trial expiry

When the trial expires, the relay starts normally but applies graceful degradation:

- **Free adapters continue** — `webhook`, `chatwoot`, `fider` operate as before.
- **Paid adapters are disabled** — `zendesk` and `linear` are removed from the registry. If they were configured (`adapters.zendesk.enabled: true`), the relay logs one `slog.Warn` line per paid adapter: `license: trial expired; adapter "zendesk" disabled — see docs/license.md`.
- **Startup continues** — the consolidated startup-problems gate does NOT treat license-gate disablement as fatal. Free-mode is a valid operating state.
- **Routing rules referring to disabled adapters** — if `routing.default_adapter` or any `routing.rules[].to` references a disabled adapter, the relay logs an additional warning and falls back to the first enabled adapter for the affected rules.

The license gate is **fail-open in favor of availability**: a network outage or signature mismatch will never brick the relay. Free adapters always continue.

## License expiry

A loaded-but-expired license behaves the same as trial expiry: paid adapters are disabled with a warning, free adapters continue, the relay starts cleanly. The relay logs `license: <license_id> expired on <date>; paid adapters disabled` at `slog.Warn`.

## Installing a license key

### From a file

```bash
# Linux / production
sudo install -m 0640 -o intake -g intake my-license.json /etc/openintake/license.json
sudo systemctl restart openintake-relay
```

### From an environment variable (containers)

```yaml
# docker-compose.yml
services:
  relay:
    environment:
      INTAKE_LICENSE: |
        {"license_id":"lic-2026-xxxxx","subject":"...",...,"signature":"..."}
```

Or the file-path form:

```yaml
services:
  relay:
    environment:
      INTAKE_LICENSE_FILE: /run/secrets/openintake-license
    secrets:
      - openintake-license

secrets:
  openintake-license:
    file: ./my-license.json
```

### From the CLI flag

```bash
openintake-relay --config /etc/openintake/relay.yaml --license /etc/openintake/license.json
```

## Verifying the license is active

On startup, the relay logs one of:

- `license: trial active, expires <date>` — no license file found; trial active.
- `license: <license_id> active, expires <date>; enabled paid adapters: [zendesk, linear]` — license file found and verified.
- `license: trial expired; paid adapters disabled` — trial window passed; free-mode.
- `license: <license_id> expired on <date>; paid adapters disabled` — license file found but past `expires_at`.
- `license: signature verification failed; falling back to trial mode` — license file found but signature didn't verify against the baked-in public key. Treated identically to "no license file." (Phase 7+ also surfaces this as a single `slog.Error` line at startup; license-gate failures NEVER `os.Exit(1)`.)

For programmatic monitoring, the `/v1/health` endpoint includes a `license` field in its JSON body:

```json
{
  "ok": true,
  "license": {
    "tier": "commercial",
    "subject": "Acme Corp",
    "expires_at": "2027-01-15T00:00:00Z",
    "paid_adapters_enabled": true
  }
}
```

For trial mode, `tier` is `"trial"` and `subject` is `null`. For free-mode (expired), `paid_adapters_enabled` is `false`.

## Obtaining a license

> **Placeholder — final contact details TBD with Q1 final-name lock.**

To purchase a commercial license:

- **Email:** `licensing@<domain>`
- **Subject line:** "OpenIntake commercial license — \<your organization\>"

See `COMMERCIAL.md` for the (draft) terms. Note that `COMMERCIAL.md` is currently a draft pending legal review and does NOT constitute a binding offer.

## Troubleshooting

| Symptom | Cause | Resolution |
|---|---|---|
| `license: signature verification failed` on a license you just received | Public-key mismatch — your relay binary is from a release prior to your license. | Upgrade the relay to a release dated on or after your license issue date. |
| `adapter "zendesk" disabled — see docs/license.md` after installing a license | License `enabled_adapters` doesn't include `zendesk`. | Contact licensing; you may need a license re-issued with the correct adapter list. |
| Trial expired but you have a license file | `INTAKE_LICENSE_FILE` env var not set, or the default path doesn't contain the file. | Set `INTAKE_LICENSE_FILE=/path/to/your/license.json` and restart. Check the startup log to confirm the path was used. |
| `license: trial active` after installing a license | License file path not picked up. | Set `INTAKE_LICENSE_FILE` explicitly or place the file at `/etc/openintake/license.json`. |

## See also

- `LICENSE` — Apache 2.0 text for the framework code.
- `COMMERCIAL.md` — draft commercial license terms for paid adapters.
- `docs/PROJECT.md` §12 — license model design rationale.
- `docs/PROJECT.md` §13 — free vs paid adapter matrix and the open-core pattern.
- `docs/adapters.md` — per-adapter setup, including which adapters require a license.
- `docs/self-hosting.md` — production deployment, including license file path management.
