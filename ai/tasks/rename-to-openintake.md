# Rename: `intake` → `openintake`

Final product name chosen: **OpenIntake**. Repo will be pushed to `github.com/clubpetey/openintake`.
Resolves PROJECT.md Open Question #1 (final product name).

## Decisions (locked)

| Surface | Old | New |
|---|---|---|
| GitHub repo | (none) | `github.com/clubpetey/openintake` |
| Go module (relay) | `module intake` | `module github.com/clubpetey/openintake/relay` |
| Go module (license-tool) | `module intake-license-tool` | `module github.com/clubpetey/openintake/license-tool` |
| Go imports | `"intake/..."` | `"github.com/clubpetey/openintake/relay/..."` |
| npm scope | `@intake/*` | `@openintake/*` |
| npm root pkg | `intake-monorepo` | `openintake-monorepo` |
| Relay binary | `intake-relay` | `openintake-relay` |
| License tool binary + dir | `intake-license` (`license-tool/cmd/intake-license/`) | `openintake-license` (`license-tool/cmd/openintake-license/`) |
| Container image | `ghcr.io/intake/intake-relay` | `ghcr.io/clubpetey/openintake-relay` |
| Config dir | `/etc/intake`, `~/.config/intake`, `%APPDATA%\intake`, `~/Library/Application Support/intake` | `…/openintake` |
| Pricing URL placeholder | `intake.example.com` | `openintake.example.com` |
| Docs product name (prose) | `Intake` | `OpenIntake` |

## KEEP unchanged (domain word / wire contract — per user Q2/Q3)

- API paths `/v1/intake/turn`, `/v1/intake/submit`, `/v1/intake/init`, `/v1/intake/auth/*`
- HTTP header `X-Intake-Session`
- Env-var prefix `INTAKE_*` (INTAKE_LICENSE, INTAKE_SMTP_PASS, INTAKE_EMAIL_JWT_SECRET, INTAKE_TURNSTILE_SECRET, …)
- Component / type / composable names: `IntakeWidget`, `useIntake`, `IntakeConfig`
- Webhook example path `…/intake`
- Domain prose: "intake conversation", "intake system", "the intake widget"

## Default decisions (no separate sign-off; reversible via git)

1. **Config filesystem paths** renamed to `openintake` (binary is `openintake-relay`, so `/etc/openintake/` is consistent). Nothing is deployed yet.
2. **`ai/` excluded** from the rename — historical task plans + LESSONS.md are a record of what was done at the time; rewriting them would be misleading.
3. **Local working directory** `c:\src\ai\intake` is NOT renamed — not needed for the GitHub push and would disrupt the session's working-dir config.
4. **vite bundle filename** `dist/intake-vue.js` left as-is (internal build artifact, not a typed identity surface).

## Execution waves (each gated by verification)

1. **Go** — module lines + `"intake/` import rewrite + ldflags module path + dir rename. Gate: `go build ./...` + `go test ./...` in `relay/` and `license-tool/`.
2. **npm** — `@intake/` → `@openintake/`, root pkg name, regenerate `package-lock.json`. Gate: `npm run type-check`.
3. **Docker / goreleaser / CI / config-path code** — binary/image/owner/paths. Gate: `goreleaser check` (if available), `go test ./internal/config/...`.
4. **Docs prose** — PROJECT.md, README, docs/*, examples/* — product name only, keep domain words.

Final gate: full `go test ./...`, `npm run type-check`, and a grep sweep for stray identity-`intake` tokens (excluding the KEEP list + `ai/`).
