# LESSONS.md — Self-Annealing Loop

Patterns learned from corrections and mistakes. Review at session start.

---

## Project-Specific

---

## General Patterns

---

### L003: go-jsonschema does not validate JSON Schema `const` (only `enum`)

`go-jsonschema` treats a `{"type":"string","const":"1.0"}` field as a plain Go `string` with no value enforcement. The generator emits typed string consts and `UnmarshalJSON` validators for `enum` values, but `const` is silently downgraded to an unvalidated `string` field.

**Consequence:** `relay/internal/payload/types.go` will accept any value for `schema_version` — e.g. `"9.9"` — without error at unmarshal time. The TypeScript generator (`json-schema-to-typescript`) DOES emit a literal type for `const`, so the two generated targets behave differently.

**Rule:** Phase 1's relay MUST re-validate `schema_version` (and any other `const`-constrained field) at the HTTP/request boundary. Do not rely on the Go type system to enforce `const`-derived invariants. Reference: `relay/internal/payload/types.go`.

---

### L002: go-jsonschema v0.19.0 — correct module path, binary name, and flags

The plan referenced `github.com/omissis/go-jsonschema/cmd/gojsonschema@v0.19.0` but at v0.19.0 the module's own `go.mod` declares `module github.com/atombender/go-jsonschema` and the `cmd/gojsonschema` subpackage does not exist.

**Correct install command:** `go install github.com/atombender/go-jsonschema@v0.19.0`

**Binary name:** `go-jsonschema` (not `gojsonschema`)

**Flag to get `IntakePayload` as root struct name:** add `--struct-name-from-title`. Without it the generator derives the name from the filename (`PayloadV1Json`). Since the schema has `"title": "IntakePayload"`, this flag is required.

**Rule:** When using omissis/go-jsonschema redirect in plans, verify the actual `go.mod` module path matches before using it. Always run `go-jsonschema --help` to confirm flag names after installing, and check the generated root struct name matches the schema title.

---

### L001: `vue-tsc --noEmit` and `vue-tsc -b` catch different errors

The `npm run type-check` script (configured as `vue-tsc --noEmit`) does NOT catch every error that `vue-tsc -b` (project-references / build mode, used by `npm run build` and by Quinoa's `./gradlew build`) catches. Specifically, dead-code TS2367 ("This comparison appears to be unintentional because the types have no overlap") slips through `--noEmit` but trips `-b`.

**Where it hit:** Phase 3-v Task 1 F9 fix (commit `ee765f2`). A 1-line TS2367 in a `.vue` SFC's `<script setup lang="ts">` block passed local `npm run type-check` but failed Quinoa's build step on a subsequent agent's `./gradlew build`. The implementer's local pre-commit gate (`type-check` only) didn't reproduce the failure.

**Rule:** for local pre-commit verification on Vue work, run the build path that mirrors CI — `./gradlew build` (which invokes Quinoa's `vue-tsc -b`), or at minimum `cd src/main/webui && npm run build`. `npm run type-check` is a fast inner-loop check but not a complete pre-commit gate.

---

### L004: Node smoke harnesses must stub browser globals; `navigator` is a read-only getter in Node 24

`@intake/core`'s `captureClient()` is SSR-safe and returns empty defaults (`url: ""`, etc.) when `window` is undefined. Running the `core/smoke/drive.ts` smoke in Node therefore produced an empty `client.url`, which the relay's runtime schema validation **correctly 400-rejected** (`client.url` is `format: uri`; `""` is not a valid URI). This is the L003 mitigation working as designed in a live setting — not a relay bug.

**Where it hit:** Phase 1 final smoke (2026-05-27). First run: `/submit` → 400 because the Node-captured `client.url` was empty.

**Fix:** the Node smoke stubs minimal browser globals (`window.location.href`, `window.innerWidth/Height`, `navigator.userAgent/language`, `document.referrer/title/querySelectorAll`) before calling `submit()`. Use `Object.defineProperty(globalThis, name, { value, configurable: true, writable: true })` — **plain assignment to `globalThis.navigator` throws** `TypeError: Cannot set property navigator ... which has only a getter` in Node 24 (same quirk the 1-v context tests hit).

**Rule:** when driving browser-targeted client code from Node (smokes, scripts), simulate the browser context with `Object.defineProperty` (not assignment) for `navigator`/`window`/`document`. And remember a green relay-side `400` on an invalid `client.url` is the schema gate doing its job.

---

### L005: Redact secrets from adapter error messages even when the downstream echoes them — and redact BEFORE truncating

Adapter `Create` errors commonly include the downstream response body for debuggability. Two ways a secret leaks: (a) a misbehaving downstream/proxy echoes your `Authorization` header back in its error body; (b) you redact the key but `truncate(body, 200)` runs first and splits the key so the exact-string `ReplaceAll` no longer matches.

**Where it hit (Phase 3):** zendesk (basic-auth base64 embeds the token) and linear (raw `Authorization: <key>`). The plan's own `*KeyNeverLeaks` tests construct a server that echoes the key in the body.

**Rules:**
- For header-auth adapters, either omit the response body from non-2xx errors entirely (zendesk's choice) or scrub it. Status code alone is often enough.
- If you redact (`strings.ReplaceAll(s, key, "[REDACTED]")`), do it BEFORE `truncate`, never after — `truncate(redact(s), n)`. A test must exercise the long-prefix case (key pushed past the truncation boundary), or it proves nothing (linear's `TestLinearCreate_KeyNeverLeaks_LongPrefix`).
- GraphQL returns HTTP 200 on logical errors — treat a non-empty `errors[]` / `success:false` / nil result as failure, and redact those messages too.

---

### L006: Gate paid features off the interface method, not hardcoded name strings

The license gate was first written as `licState.Permits("zendesk")` / `Permits("linear")` inline in each paid branch. That makes "paid ⇒ gated" a per-block convention: a future paid adapter that sets `RequiresLicense() → true` but whose author forgets the inline guard ships ungated, and nothing (build/vet/test) catches it.

**Rule:** drive the gate off the adapter's own `RequiresLicense()` via a single helper applied to every adapter uniformly (`func licensed(ad, state, logger) bool`), so the invariant is structural, not conventional. Keep the gate check BEFORE secret resolution so an enabled-but-unlicensed paid adapter in free mode is skipped with a clear warning rather than failing fatally on a missing token. Reference: `relay/cmd/relay/main.go` `buildRegistry`/`licensed`.

---

### L007: Byte-sliced `truncate` can split a UTF-8 rune; use a rune-safe shared helper

`return s[:max] + "…"` slices by byte. A multibyte char straddling the cut yields invalid UTF-8 in the error string. It was duplicated in 5 adapters. Consolidated into one rune-safe `adapter.Truncate` (`r := []rune(s); ...string(r[:max])`).

**Rule:** truncation of any downstream/user text destined for an error/log must be rune-aware. Prefer one shared helper over per-package copies (DRY + fix-once). Reference: `relay/internal/adapter/truncate.go`.

---

### L008: Share one canonicalization source across modules via a `replace` directive (Go `internal/` blocks cross-module imports)

The relay (`intake`) verifies licenses; the maintainer CLI (`intake-license-tool`) signs them. Both must agree byte-for-byte on the signed canonical JSON, but Go's `internal/` rule blocks the separate CLI module from importing `intake/internal/license`.

**Rule:** put the shared struct + `Canonicalize`/`Sign`/`Verify` in an IMPORTABLE package (`relay/license`, not under `internal/`) with zero non-stdlib deps, and have the CLI consume it via `require intake v0.0.0` + `replace intake => ../relay`. A round-trip test in the CLI module (sign → the shared `Verify` accepts; tamper → rejects) locks the two modules together. Relay-only concerns (embedded key, loader, trial/free state, gate) stay in `relay/internal/license`. Reference: `relay/license/`, `license-tool/go.mod`.

---

### L009: License-loader robustness — fail loudly on tamper, degrade gracefully on operational faults

The licensing boundary must distinguish security failures (fail closed/loud) from operational faults (degrade, don't brick):
- Bad signature → FATAL (tamper). Valid-but-expired → downgrade to free + warn (a lapsed paid customer's free adapters keep working).
- A non-empty-but-invalid embedded public key constant must error loudly, NOT silently behave like "no key" (silent-failure class — see PHASE_PLANNING postmortem).
- A malformed `state.json` (trial state) should be treated as absent (restart trial) rather than fatal — a corrupt one-field file must never brick startup. Write it atomically (temp file + `os.Rename`) so a crash/concurrent-start can't truncate it.

Reference: `relay/internal/license/manager.go`, `state_file.go`, `embedded_key.go`.

---

### L010: PowerShell 5.1 `Set-Content -Encoding utf8` writes a BOM that Go's JSON/YAML parsers reject

Windows PowerShell 5.1's `-Encoding utf8` writes UTF-8 **with** a byte-order mark (`EF BB BF`). Go's `encoding/json` (and `gopkg.in/yaml.v3`) treat the BOM as input bytes, not whitespace, and fail with `invalid character 'ï' looking for beginning of value` (the `ï` is byte `0xEF` rendered as Latin-1 — the first byte of the BOM).

**Where it hit:** Phase 3 live smoke (2026-05-27). `intake-license sign --in template.json` failed parsing a template written via `... | Set-Content -Path X -Encoding utf8`. The maintainer hits this whenever they `Set-Content` a JSON/YAML file the relay then reads on the same Windows host.

**Rules:**
- For **ASCII-only** content (most JSON/YAML scaffolding — license templates, scratch configs), use `-Encoding ascii`. No BOM, no fuss.
- For content that may contain **non-ASCII** (e.g. an `IssuedTo.Org` with accented characters), use `[System.IO.File]::WriteAllText($path, $content, [System.Text.UTF8Encoding]::new($false))` to write UTF-8 without a BOM. (`-Encoding utf8NoBOM` does not exist in PS 5.1; it's PS 6+.)
- The CLAUDE.md note "pass `-Encoding utf8` to `Out-File`/`Set-Content`" is correct *for tools that strip a BOM* (most text editors, PowerShell-native readers) but **misleading for raw-byte readers** like Go's JSON/YAML decoders. When in doubt, prefer the no-BOM writers above.
- Quick sanity check after writing a file Go will read: `Get-Content <path> -Encoding Byte -TotalCount 4` — if it starts with `EF BB BF`, the BOM is there.

Reference: maintainer live-smoke `Set-Content` calls in this phase's `README.md` §7 / step 2 instructions.

---

### L011: Chatwoot agent-side `POST /conversations` needs a pre-created contact_inbox, and returns 404 (not 422) when source_id is dangling

For Chatwoot's **agent-side** API (the one keyed by `api_access_token`), `POST /api/v1/accounts/{id}/conversations` does NOT auto-create a contact for an unknown `source_id`. A `source_id` that doesn't already exist as a `contact_inbox` association makes Chatwoot return `404 {"error":"Resource could not be found"}` — a generic-looking 404 that obscures the real cause (it looks like a wrong endpoint/account, but it's actually missing contact state). 422 would have been more accurate; Chatwoot's choice of 404 cost ~10 minutes of debugging in the live smoke.

**Where it hit:** Phase 3 step #3 live smoke against Chatwoot Cloud (2026-05-27). The chatwoot adapter's original single-call flow POSTed a conversation with `source_id = p.Submission.Id` and a fresh UUID; Chatwoot 404'd because that UUID had no contact_inbox row.

**Rule:** For Chatwoot's agent-side API on a `Channel::Api` inbox, use a **two-call flow**:
1. `POST /api/v1/accounts/{id}/contacts` with `{inbox_id, name, identifier, email?}` — returns `{payload:{contact:{id}, contact_inbox:{source_id}}}`. This is the same endpoint Chatwoot's UI uses; it creates the contact AND the contact_inbox link in one call.
2. `POST /api/v1/accounts/{id}/conversations` with `{source_id, inbox_id, contact_id, message:{content}}` using the values returned by step 1.

The **public API channel** path (`/public/api/v1/inboxes/{identifier}/contacts/.../conversations`) uses a different auth model (HMAC) and is a valid alternative, but the agent-side path is simpler when you already have an api_access_token. Reference: `relay/internal/adapter/chatwoot/chatwoot.go` `createContact` + `Create`.

---

### L012: When an external API uses an opaque UUID but the platform exposes a unique human-readable identifier everywhere, accept BOTH forms in config and resolve once at startup

Linear's GraphQL `IssueCreateInput.teamId` requires a UUID like `9ddb7234-31d1-4dd3-b9b0-32ad948b6104`. The UUID is never in a URL, a UI label, or an issue identifier — finding it requires running a `teams { nodes { id key } }` query. The short team **key** (`REF`, `OTHER`, etc.) IS in every URL, every issue identifier (`REF-42`), and every settings screen. Forcing an operator to dig out the UUID is real friction; replacing UUID with key alone breaks IaC configs that already have the UUID pinned.

**Where it hit:** Phase 3 fast-follow live smoke (2026-05-28). The Linear adapter accepted only UUIDs; operator UX immediately surfaced the gap. Solution: `Configure` detects the form (regex on UUID shape) and conditionally resolves keys to UUIDs via one startup-time GraphQL query.

**Rule:**
- Use this dual-form pattern **only when all three are true**: (a) the external API requires an opaque id, (b) a unique human-readable identifier is exposed in URLs/UI everywhere, and (c) the resolution endpoint is cheap and stable. Linear meets all three. Chatwoot's account_id/inbox_id and Zendesk's subdomain don't — they're URL-visible and copy-pasteable, so adding a startup-time HTTP call to resolve them would be pure overhead.
- Implement the detection with a strict regex on the canonical id format (e.g. UUID v4 shape for Linear). Non-match → resolution path; match → store verbatim, no network call.
- Resolution must be **fatal at startup** with a clear error (including the key the operator typed and the available alternatives, capped to a reasonable length) — not deferred to first request. Use `context.WithTimeout(context.Background(), 10*time.Second)` since the frozen adapter `Configure` has no context parameter.
- The resolution call uses the same auth as the per-request API call; the same redact-before-truncate rule (L011) applies to its error paths.
- The resolved id is what every subsequent request uses — **no per-request resolution**, no cache invalidation, no rename detection. The underlying entity is identified by its UUID; if the operator's team is renamed/rekeyed they restart the relay.

Reference: `relay/internal/adapter/linear/linear.go` `resolveTeamKey` + `Configure`; tests `TestLinearConfigure_UUIDPassthrough`, `_KeyResolved_HappyPath`, `_KeyNotFound`, `_ResolveGraphQLErrors`, `_ResolveNon2xx`, `_ResolveNetworkError`.

---
