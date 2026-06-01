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

### L013: When verifying JWTs, ALWAYS pin the algorithm via `WithValidMethods` to mitigate alg-confusion attacks

The classic JWT alg-confusion attack: an attacker takes a token expected to be RS256 (verifier holds the public key), changes the header `alg` to HS256, and signs the modified token using the RS256 public key as the HMAC secret. If the verifier passes the RS256 public key into the HMAC verification path without checking `alg`, the signature validates. Result: the attacker forges arbitrary claims with only the public key.

**Where it hit:** Phase 4 SSO design. Both `RS256Verifier` and `HS256Verifier` in `relay/internal/auth/sso/` consume tokens via the same `golang-jwt/jwt/v5` parser. Without explicit alg-pinning the parser would accept either alg.

**Rule:** every `jwt.ParseWithClaims` (or `jwt.Parse`) call MUST pass `jwt.WithValidMethods([]string{"<expected-alg>"})`. Test the rejection explicitly — for an RS256 verifier, mint an HS256 token using the RSA public-key bytes as the HMAC secret and assert rejection. Same in reverse for HS256. The rejection test is a load-bearing security assertion; if it ever flakes or gets disabled, the verifier is broken.

Reference: `relay/internal/auth/sso/{rs256.go,hs256.go}`; tests `TestRS256_AlgConfusion_Rejected`, `TestHS256_AlgConfusion_Rejected`.

---

### L014: In-memory rate-limiters (per-key TTL + sliding window cap) need an injectable clock for testable semantics

A naive in-memory rate-limiter that reads `time.Now()` directly inside `Issue`/`Verify` cannot be tested deterministically — TTL expiry and sliding-window resets require either real wall-clock waits (`time.Sleep` makes tests slow and racy) or compromising the production code path with conditional test hooks. The clean answer is a single injectable `now func() time.Time` field set at construction.

**Where it hit:** Phase 4 `relay/internal/auth/emailcode`. The Store has a 10-min code TTL + a 3-codes-per-10-min sliding window. Tests need to advance virtual time past the window to assert reset, past the TTL to assert eviction, and to a specific instant to assert single-use post-verify. With `now func() time.Time` injected, the test passes a closure that returns a controlled `time.Time`; production passes `time.Now`.

**Rule:** any in-memory TTL/window primitive must take `now func() time.Time` (or equivalent) at construction. The internal code path always calls `s.now()` rather than `time.Now()` directly. Eager-eviction (prune on Issue/Verify) is preferred over a background goroutine for v0 — simpler, no race surface, and the per-op cost is trivial for the small key counts we expect (one entry per pending email).

Reference: `relay/internal/auth/emailcode/emailcode.go`; tests in `relay/internal/auth/emailcode/emailcode_test.go`.

---

### L015: When a derived field is written by multiple code paths, every path must populate it — unit tests pass on what they assert; only end-to-end smokes catch what they don't model

Phase 4's auth dispatcher had three paths producing `auth.SessionContext`: anonymous (sets `SessionID` from `X-Intake-Session`), email-bearer (forgot `SessionID`), SSO-bearer (forgot `SessionID`). The downstream `payloadbuild.Build` reads `SessionContext.SessionID` to populate `IntakePayload.client.session_id`, which the JSON schema requires to be a non-empty UUID. The 11 dispatcher unit tests asserted `AuthMode`/`Verified`/`Email`/`UserID` but **not** `SessionID` — so they passed while the bearer paths shipped a payload that would fail schema validation. The 4-iv live email smoke surfaced this on the first real `/submit` call (`'/client/session_id': '' is not valid uuid`).

**Where it hit:** Phase 4 live email smoke (2026-05-28). Fixed at commit `8f79c76`: dispatcher reads `X-Intake-Session` once at the top of the bearer block and populates `SessionID` in both bearer-success branches; two regression tests pin the behavior.

**Rules:**
- When multiple code paths produce a shared struct destined for downstream validation, write the **shared output contract as a test fixture** (a `requireFullSessionContext(t, sess, AuthMode, SessionID, Verified, Email, ...)` helper) and call it from every per-path test. This makes "every path populates every required field" structural, not a per-test convention.
- For every required field in a downstream schema (canonical payload, k8s manifest, IaC), trace backwards: every place the producer writes it must be exercised by at least one test that asserts the field is non-empty. A test that only asserts the field this path uniquely owns leaves the shared-required fields un-pinned.
- **Live end-to-end smokes are not optional.** Unit tests pass on the assertions they make; downstream schema validation catches the fields they don't. The chatwoot 404 in Phase 3 and the SessionID bug in Phase 4 both surfaced only at live smoke. Budget time for live smokes; treat them as the load-bearing proof, not a victory lap.
- When the live smoke surfaces a bug like this, fix it AND add the regression test AND record the shape lesson — three artifacts, one commit.

Reference: `relay/internal/auth/middleware.go` (bearer-branch `SessionID` population); tests `TestDispatcher_EmailMode_SessionIDFromHeader`, `TestDispatcher_SSOMode_SessionIDFromHeader`.

---

### L016: When a startup gate validates inputs that consumers also need to parse, return the parsed values — never re-parse with the error discarded

The 5-i Task 9 code review caught a subtle silent-failure shape: `startupProblems` in `main.go` validated `server.trusted_proxies` CIDRs via `netip.ParsePrefix`, then the consumer (the `clientIPMiddleware` wiring further down `main.go`) re-parsed the same strings with the error discarded. If a future refactor reordered initialization so the consumer ran first, OR if a code path bypassed the gate, malformed CIDRs would silently become zero-value `netip.Prefix` entries that match no IPs — the per-IP limiter would still install, the trusted-proxy allowlist would be effectively empty, and X-Forwarded-For walking would no-op. No build/test/vet failure; the misbehavior surfaces only at runtime when a real proxy presents X-Forwarded-For. The same pattern surfaced again in 5-ii Task 5 (duration parsing: `idle_ttl`, `session_ttl`) and 5-iii Task 3 (no separate parse, but the lesson generalized).

**Where it hit:** Phase 5 5-i Task 9 code review (commit `6e4e873` — Q9 startup gate returns parsed `[]netip.Prefix`); generalized to duration fields at commit `b09385d` (Q9 returns parsed `time.Duration` values for idle_ttl/session_ttl). Reference: `relay/cmd/relay/main.go` `startupProblems` return shape.

**Rule:** When a startup gate validates inputs that consumers also need to parse, EITHER (a) have the gate RETURN the parsed values for the consumer to reuse — one parse site, one error path, no possibility of silent divergence — OR (b) add a runtime assertion in the consumer that panics if the value is the zero-value-when-invalid form. A re-parse with the error discarded (`v, _ := parse(s)`) is the silent-failure shape PHASE_PLANNING §4 forbids. Add the "re-parse with discarded error" pattern to the build-fail checklist for any phase touching startup configuration. Reference: `relay/cmd/relay/main.go` `startupProblems`.

---

### L017: Soft-cap operators (> vs >=) differ between Reserve-style pre-flight and CheckSession-style post-completion checks — document the asymmetry inline

Phase 5-ii's `budget.Tracker.Reserve` uses `>` (allow exactly-at-cap) because Reserve adds a not-yet-charged estimate to the current counter and rejects only when the projected total would EXCEED the cap; at-cap with zero new estimate is still permitted. Phase 5-ii's `auth.Store.CheckSession` uses `>=` (reject at-cap) because `meta.turns` is the count of COMPLETED turns; if `turns >= max`, the next turn is the (max+1)th and must be rejected. Both operators are correct for their use case, but a reviewer flagged the asymmetry as a "consistency bug" — the natural reading is "same cap, same operator". The fix was a one-line inline comment at each operator site explaining the timing-driven asymmetry.

**Where it hit:** Phase 5-ii code review (commit `d63aa5e` added the inline comment to `CheckSession`). Reference: `relay/internal/budget/budget.go` Reserve vs `relay/internal/auth/store.go` CheckSession.

**Rule:** When the SAME conceptual cap (e.g. "N turns allowed") appears in BOTH a Reserve-style pre-flight check (gate BEFORE the work, counter is what's-already-charged) AND a CheckSession-style post-completion check (gate AFTER the work, counter is what's-already-completed), the comparison operators differ by one based on counter timing — Reserve uses `>` (project-and-compare), CheckSession uses `>=` (count-and-compare). Document the asymmetry inline at BOTH operator sites with a one-line comment so a future reviewer or refactorer does not "fix" the consistency that doesn't actually exist. Reference: `relay/internal/budget/budget.go` Reserve comment + `relay/internal/auth/store.go` CheckSession comment.

---

### L018: Replay-protection: mark BEFORE the network call when defending against per-token spam — document that retry-after-5xx is intentionally a duplicate

Phase 5-iii's `captcha.providerVerifier` marks tokens in the in-memory replay set BEFORE the siteverify HTTP call. This means a transient 5xx from Cloudflare/hCaptcha "burns" the token: the next retry with the same token returns `duplicate` (from the replay-set check) instead of re-attempting siteverify. This is intentional — the threat model is an attacker spamming siteverify with the same token to exhaust per-IP rate limits on the provider side OR to bypass the relay's per-IP gate by retrying the same proven token across many sessions. Marking before the call closes both attacker vectors. The cost is a real user who hits a transient 5xx must solve a new CAPTCHA challenge (rare; Cloudflare/hCaptcha both have >99.9% siteverify uptime).

**Where it hit:** Phase 5-iii code review (commit `cd25123` added the doc comment to `markUnseenOrEvict`). The reviewer initially flagged "retry-after-5xx returns duplicate" as a bug; the fix was a doc comment, not a behavior change. Reference: `relay/internal/captcha/captcha.go` `markUnseenOrEvict`.

**Rule:** Replay-protection sets layered on top of provider-side single-use semantics should mark BEFORE the network call as the defense-in-depth posture (the provider may go down; your gate must not). Document the behavior at the mark site so an operator debugging "my retry didn't work" knows it's intentional, and so a future refactorer does not "fix" the burn-on-transient by moving the mark after the call. If the operational cost (real users losing tokens on transient 5xx) becomes measurable, add a separate retry-with-fresh-token UX in the widget — do not move the mark. Reference: `relay/internal/captcha/captcha.go` `markUnseenOrEvict` doc comment.

---

### L019: When a smoke fixture has multiple caps that could fire on the same request, run the math BEFORE writing the smoke driver

5-iv Task 7's first run of `drive-abuse.ts` failed because the smoke fixture had `daily_llm_budget=(100, 100)` with the fake-llm provider reporting 50 input + 50 output tokens per turn — so the budget gate fired on turn 3 (Reserve check: 100 already-committed + 50 new estimate = 150 > 100 cap) BEFORE the per-session cap (`max_turns=3`) could fire on turn 4. The smoke driver expected turns 1-3 = 200, turn 4 = 429 `session_turns_exhausted`; it got turn 1 = 200, turn 2 = 200, turn 3 = 503 `daily_budget_exhausted`. The fix (commit `68382c0`) raised budget to `(150, 150)` so per-session fires first. The driver's intent was to combine the two gates in one smoke; the fixture math made the budget gate "shadow" the per-session gate.

**Where it hit:** Phase 5-iv Task 7 first run. Smoke driver assumption did not match the dispatcher's gate-ordering math (`CheckSession` → `Reserve` → `Chat`). Fix: raised budget so per-session fires first; alternative would have been to isolate gates per-smoke (the approach Tasks 5+6 took).

**Rule:** When a smoke fixture has MULTIPLE caps that could fire on the SAME request, run the math BEFORE writing the smoke driver:
- List every cap that gates the endpoint under test (per-IP, per-session, daily-budget, etc.) and the request count at which each fires.
- Identify which cap should fire FIRST under the smoke's intent.
- Verify the fixture values make that the case: raise the others' caps so they don't shadow, OR isolate gates per-smoke if combining is fragile.
- The dispatcher's gate ordering determines which cap fires when both would reject — the smoke driver must align with that ordering, not assume independence.

Reference: `relay/cmd/relay/smoke/abuse-driver.yaml` budget field (commit `68382c0`); `relay/internal/server/server.go` dispatch order (CheckSession → Reserve → Chat).

---

### L020: Chatwoot's conversation-create silently drops `attachments[]`; multipart message-create is a SEPARATE call

Phase 6 6-ii shipped a chatwoot adapter that switched `POST /api/v1/accounts/{id}/conversations` from JSON to `multipart/form-data` with an `attachments[]` part whenever `p.Attachments` was non-empty. The 6-ii unit tests verified our multipart shape (the test server's `ParseMultipartForm` found the part and the bytes matched). They did NOT verify what Chatwoot's `ConversationsController#create` actually does with that part — Chatwoot accepts the multipart, persists only the known fields (`inbox_id`, `source_id`, `contact_id`, `message[content]`), and silently drops the `attachments[]` file parts on the floor. Result: conversation created in chatwoot with the transcript text but NO image visible in the UI.

**Where it hit:** Phase 6 6-iv Task 8 live chatwoot smoke (2026-05-29). Maintainer's visual check showed the conversation existed with `user:`/`assistant:` lines but no attached image. Fix: revert conversation-create to byte-identical Phase 3 JSON, and add a third call `POST /api/v1/accounts/{id}/conversations/{conv_id}/messages` with `multipart/form-data` carrying `content` + `message_type=outgoing` + `attachments[]` parts, only when `len(p.Attachments) > 0`. The unit tests were rewritten to assert the THREE-call order (contacts → conversations(JSON) → messages(multipart)) and to assert the conversation-create body is JSON, not multipart. A new test pins the upload-failure error contract (conversation already exists; upload error surfaces from `Create()` mapped to 502 by `submitHandler`; no orphan-prevention attempt).

**Rule:** When an adapter sends a multipart body to an endpoint that may or may not consume specific fields, contract-test assertions on OUR multipart shape are insufficient — a permissive multipart parser on the server side will accept fields it doesn't know what to do with, and the test won't detect the drop. Two mitigations required:
- The unit-test fixture's handler must include a comment naming the source-of-truth documentation page (Chatwoot API reference URL: `https://www.chatwoot.com/developers/api/`) and the controller name (`ConversationsController#create`, `MessagesController#create`) being modeled, so a reviewer can spot the wrong-endpoint assumption from the test alone.
- A live smoke against the real downstream is the load-bearing proof — never skip it for adapter changes that alter request shape or split a single call into multiple calls. Generalizes L015: unit tests pass on the assertions they make; only end-to-end smokes against the real downstream catch silent-drop semantics on the receiving side.

Reference: `relay/internal/adapter/chatwoot/chatwoot.go` `uploadAttachments` + `buildMessageMultipart` (the second multipart helper; the original `buildConversationMultipart` was renamed and its body shape changed to remove the conversation-create-only fields).

---

### L021: SSR-safe browser APIs need dependency injection at construction, NOT lazy module-level imports

`html2canvas` is a browser-only library. Importing it at the top of `core/src/capture.ts` works in the browser bundle but breaks SSR (Vite SSR, Nuxt, Astro, the Vue test-utils `mount` with `global.stubs`), because the import-time side effects touch `window` / `document` / `Image` / etc. Two anti-patterns to avoid: (a) `const html2canvas = typeof window === 'undefined' ? null : require('html2canvas')` — module-level `require` in a TS ESM module is a build-time error in modern tooling; the `typeof window` check runs only at first import and gets cached. (b) `let h2c: any; if (typeof window !== 'undefined') import('html2canvas').then(m => h2c = m.default)` — race condition; first `capturePage()` call may run before the dynamic import resolves; tests that mock `window` AFTER the module imports see the wrong value.

**The clean answer:** dependency-inject the capture function at construction. `core/src/capture.ts` exports `setHtml2Canvas(fn)` and `capturePage()`. Production code calls `setHtml2Canvas(html2canvas)` once at widget bootstrap (inside an `if (typeof window !== 'undefined')` guard that protects the import statement itself via a dynamic `await import('html2canvas')`). Tests call `setHtml2Canvas(stubFn)` to inject a stub canvas — no real library load, no `window` touched.

**Where it hit:** Phase 6-iii widget design. The Vue test-utils mount step for `ScreenshotRedactor.spec.ts` failed under jsdom because the real `html2canvas` import touched `Image.prototype.crossOrigin` which jsdom doesn't fully implement. The DI rewrite made the test trivially passable AND fixed an unrelated SSR-build warning that would have shipped silently into the v1 Nuxt example.

**Rule:** For any browser-only dependency that the widget loads (canvas APIs, ResizeObserver polyfills, Notifications, Service Workers, IndexedDB), inject the capability through a single `setX(fn)` setter and a single `getX()` accessor. The production widget call site is the ONLY place that imports the real module — and it imports it dynamically (`await import('lib')`) inside an `if (typeof window !== 'undefined')` guard. Tests inject stubs through the setter. This pattern also makes "swap to a different capture engine for v1" a one-line config change rather than a refactor.

Reference: `core/src/capture.ts` `setHtml2Canvas` + `capturePage`; `vue/src/composables/useIntake.ts` (production bootstrap inside `onMounted`); tests in `core/src/capture.test.ts` stub-injection cases.

---

### L022: Stage Q9 startup gates so EVERY subsystem contributes to ONE consolidated log line — never `os.Exit(1)` after the first subsystem's problems

Phase 5's Q9 contract (L016) is "one consolidated `relay: startup config errors` log line listing every distinct problem so the operator fixes everything in one restart cycle." Phase 6 added a SECOND startup gate (`validateAttachments` in `main.go`) for the new `attachments:` config block. The first 6-i implementation called `validateAttachments` AFTER Phase 5's `startupProblems`, and each gate independently called `os.Exit(1)` when its own `problems` slice was non-empty. Result: a YAML with BOTH Phase-5 misconfigs (bad CIDR, anonymous-no-captcha, bad action_on_exceeded) AND Phase-6 misconfigs (storage.mode:"s3", inverted caps) emitted the Phase 5 log line, exited, and the operator never saw the Phase 6 problems on the same restart — fixed Phase 5, restarted, then saw Phase 6 problems, fixed those, restarted again. Two restart cycles where Q9 promises one.

**Where it hit:** Phase 6 6-iv Task 3 Q9 combined-fixture smoke (2026-05-29). The combined fixture (`attachments-combined.yaml` — three Phase-5 + two Phase-6 misconfigs in one file) failed the "exactly one consolidated log line listing every problem" assertion. Fix at commit `5275070`: accumulate problems from EVERY startup gate into a single `startupProblems []string` slice, log ONE consolidated line, exit ONCE. Each subsystem's gate function appends to the shared slice and RETURNS the parsed/defaulted values (L016 — no re-parse-with-discarded-error); the single `if len(startupProblems) > 0 { log + exit }` block at the end of startup is the only exit site.

**Rule:** Every startup gate function (`startupProblems`, `validateAttachments`, anything Phase 7+ adds) MUST accumulate into a SHARED `[]string` and return parsed values for consumers. There is exactly ONE `os.Exit(1)` site in `main.go`, called once after all gates have run. Add the "second `os.Exit(1)` site introduced for a new subsystem's gate" pattern to the build-fail checklist for any phase that adds a startup-time config validation. The combined-fixture smoke (Phase 6 `attachments-combined.yaml`; Phase 5 `combined-misconfig.yaml`) is the load-bearing regression test — every new phase that adds a startup gate MUST extend the combined fixture with at least one of its own misconfigs and assert the consolidated log line contains substrings from EVERY subsystem.

Reference: `relay/cmd/relay/main.go` `startupProblems` accumulation + `validateAttachments` returning parsed `AttachmentsConfig`; combined fixture `relay/cmd/relay/smoke/attachments-combined.yaml`; commit `5275070` (fix).

---

### L023: When an adapter's downstream returns HTTP 200 with a logical-failure field (`success:false`), the unused-but-parsed field IS the load-bearing assertion

Phase 6-ii's first Linear adapter implementation parsed the file-upload response's `success` boolean field into a local struct but never read it — the adapter only checked HTTP status. The Linear file-upload endpoint returns HTTP 200 with `{"success":false, "fileUpload":null, ...}` on logical failure (asset URL not minted, quota exceeded, MIME rejected by Linear's own validator). Without checking `success`, the adapter would proceed to `issueCreate` with a nil/empty asset URL and create an orphan Linear issue with broken attachment references. The 6-ii code review caught this — the `success` field was in the struct definition, in the JSON unmarshal, but never in an `if !resp.Success` branch. Fix at commit `78bba55`: read `success`, reject the upload (without calling `issueCreate`) when false. L011 orphan-prevention preserved.

**Where it hit:** Phase 6-ii code review (commit `78bba55`). Same family of bugs as L005's "200-with-errors[]" pattern (GraphQL returns 200 on logical errors). Generalizes beyond Linear: any downstream that signals success/failure in the JSON body (not the HTTP status) requires explicit parsing AND explicit branching on that field.

**Rule:** When defining a Go struct to parse a downstream response that signals logical success in a body field (`success`, `ok`, `errors[]`, `error_code`, `status:"failed"`), the body-field check MUST run BEFORE any downstream-state-changing call (issue create, ticket create, conversation create). The smoke test for that adapter MUST include a fixture where the downstream returns HTTP 200 with the logical-failure field set, and assert that the state-changing call is NOT made. If the field exists in the struct definition but no `if` branch reads it, that is a silent-failure shape — add a build-fail item: "any struct field parsed from a downstream success/failure body must be read in a control-flow branch before any state-changing call."

Reference: `relay/internal/adapter/linear/linear.go` upload-response `Success` field check; tests `TestLinear_Upload_SuccessFalse_NoIssueCreate`; commit `78bba55` (fix).

---

### L024: Snapshot-then-publish split — every release-artifact tool MUST have a --snapshot/--dry-run mode exercised in CI; the actual publish is a deliberate, separately-gated action

Phase 7 ships `goreleaser`, `npm publish`, and `docker build` as the v0 release pipeline. The temptation when wiring CI is to have every PR push run the full release path "for free" — `goreleaser release` instead of `goreleaser release --snapshot`, `npm publish --tag dev` instead of `npm publish --dry-run`, `docker push` instead of `docker build`. The result: any merge to `main` ships an artifact to a public registry. Once shipped, you cannot unship. The yanked-version smell, the squatted-name confusion, the ghcr.io pull-count showing artifacts that never should have been public — all of those are recoverable in principle but expensive in trust and operator time.

**Where it hit:** Phase 7 scope-boundary decision. The original draft included "run `goreleaser release` in CI on tag push" — which would have shipped the v0.0.0-snapshot artifacts to the public ghcr.io / npm registry the moment Phase 7 merged, before the maintainer locked Q1 (final product name + remote + ghcr/npm tokens). The fix was the snapshot-then-publish split: Phase 7 ships `goreleaser release --snapshot --clean`, `npm publish --dry-run`, `docker build` (no push). The actual public release is a separate, maintainer-driven Phase 7.5+ action gated on Q1 + remote + tokens. CI exercises the SAME workflows against the SAME configs in dry-run mode every PR — so the moment the maintainer flips the gate, the publish path is already proven against real configs.

**Rule:** Every release-artifact tool you wire into CI MUST have a `--snapshot` / `--dry-run` / "build-but-don't-push" mode, and CI MUST run that mode on every PR. The actual publish is a deliberate, separately-gated action — guarded by either (a) a manual `workflow_dispatch` trigger with explicit confirmation, (b) a tag push under a strict naming convention (`v[0-9]+.[0-9]+.[0-9]+` only, not `v*`), or (c) a separate "publish-approved" repo environment with required reviewers. Snapshot mode catches the same config / file-list / archive-name regressions as the real publish; dry-run mode catches the same package.json / tarball-contents regressions. The publish path is then a one-line `--snapshot` removal, not a "rewrite the workflow" exercise. Add a build-fail item to every phase that touches the release pipeline: "release-artifact tool runs in CI without `--snapshot`/`--dry-run` → Fail."

Reference: `relay/.goreleaser.yaml` (snapshot config block); `.github/workflows/ci.yml` (PR job runs `goreleaser release --snapshot --clean` + `npm publish --dry-run`); `.github/workflows/release.yml` (tag-push job runs the real `goreleaser release`; authored in 7-ii but never executed in Phase 7).

---

### L025: Initial lint sweep before CI gate — never enable a lint as a CI gate against existing code without first running the sweep + triaging every finding

The natural-but-wrong way to adopt a new linter: add the `golangci-lint`/`eslint`/`prettier` step to `ci.yml`, commit, push. The CI run reports 47 findings on existing code. The PR is blocked. Every other open PR is now also blocked because the same lint job runs on every PR. The lint becomes a Day-1 barrier to all future work — anyone who needs to land a fix first has to fix 47 unrelated findings (some real, some false positives, some style preferences that the team hasn't agreed on). The lint is now "the thing that always fails" instead of "the thing that catches bugs."

**Where it hit:** Phase 7 initial-fix sweep design. Three new lints landed in one phase (golangci-lint + eslint + prettier). Without the sweep-first discipline, every Phase 8+ PR would have been blocked on the cumulative N findings across all three. The fix was the explicit Phase 7-i initial-fix sweep task: run each linter locally, triage every finding (real bug → fix with a commit; false positive → narrow `//nolint` / `eslint-disable` with a comment naming the reason; style preference → narrow the rule), land fixes BEFORE wiring the lint job into the CI gate. The CI gate then starts from a clean baseline.

**Rule:** Adopting a new lint as a CI gate is a TWO-step process: (1) the initial-fix sweep — run the linter, triage every finding, land the fixes in a "sweep" commit (or commit series) so the working tree is clean against the chosen rule set; (2) the gate wiring — add the lint step to `ci.yml` ONLY after the sweep lands. Never skip (1). The triage is the load-bearing work — every finding is either "fix the code," "narrow the rule to exclude this pattern," or "suppress this line with a reason" — and each decision is reviewable. A bulk `--fix --safe` run is acceptable for trivial mechanical fixes (whitespace, import order) but ANY non-trivial fix gets a dedicated triage decision. Curated rulesets (NOT `--enable-all`) — the rule list is part of the gate decision; tightening later is fine, loosening later carries a regression risk. Add a build-fail item: "lint introduced as CI gate without a recorded initial-fix sweep → Fail."

Reference: `relay/.golangci.yaml`, `.eslintrc.cjs`, `.prettierrc` (the curated rulesets); 7-i sweep commits (triage-by-finding); `.github/workflows/ci.yml` `lint-go` / `lint-ts` / `lint-format` jobs (gate wired after the sweep).

---

### L026: Metrics server lives independently from main HTTP — observability shouldn't be able to brick the service it observes

The natural-but-wrong design for an in-process Prometheus endpoint: register `/metrics` on the same `*http.Server` that serves the application's API endpoints. Simple, one less goroutine, one less port to document. The cost: a metrics-port conflict, a metrics-handler panic, a metrics-middleware deadlock, or even just an OOM in the metrics collection path — any of those takes down the API endpoints too. The thing whose job is to observe failures becomes a source of failures. Observability has become a single point of failure for the service it observes.

**Where it hit:** Phase 7 metrics package design. The first draft had `/metrics` registered on the main chi router. Code review surfaced the dependency: an operator with a misconfigured reverse proxy that hammers `/metrics` at high QPS could starve the API endpoints of goroutines. The fix was structural separation: `metrics.Registry.ListenAndServe(ctx)` starts a SEPARATE `*http.Server` on `cfg.Observability.Metrics.Addr` (default `:9090`). A port-bind failure on the metrics server is logged at Error level but does NOT propagate — `main()` swallows the error and the main relay continues. The `Middleware()` function is the only point of contact with the main server, and it's a literal passthrough when `Enabled=false`.

**Rule:** Observability surfaces (Prometheus metrics, OpenTelemetry traces, pprof handlers, health-debug endpoints) MUST live on a SEPARATE HTTP listener from the main application API. A failure on the observability listener (port conflict, panic, deadlock, OOM in collection) MUST be logged but MUST NOT propagate to the application listener. The integration point between the two (the metrics middleware, the OTLP exporter, the tracing instrumentation) MUST be a no-op passthrough when the observability subsystem is disabled — no conditional plumbing in the application path. Add a build-fail item: "metrics-port conflict causes main HTTP to fail to start → Fail." A unit test forces the metrics port to a known-bound socket and asserts the main relay still serves `/v1/health`.

Reference: `relay/internal/metrics/registry.go` `ListenAndServe` (separate `*http.Server`); `relay/cmd/relay/main.go` (goroutine swallows ListenAndServe error, logs at Error); tests `TestMetrics_PortBindFailure_MainRelayContinues`.

---

### L027: Off-by-default for new observability surface — every new operator-facing thing defaults to off; operators opt in

The natural-but-wrong default for a new feature flag: `Enabled: true`. Reasoning goes: "the feature is good, the default should be the good thing." For observability surface, the natural-but-wrong default is doubly wrong. (a) Operators who haven't read the docs yet now have an unauthenticated `/metrics` endpoint exposed on a port they didn't know was open — a network-recon vector if their firewall isn't tight. (b) Operators who DO want metrics now have no way to confirm "this is the operator's choice, not the package's default" — every existing deployment has metrics on with no operator action. (c) The first time something breaks in the metrics path, every operator is affected; if metrics were off-by-default, only operators who opted in are affected.

**Where it hit:** Phase 7 `MetricsConfig.Enabled` default. The first draft had `default true` ("metrics are good, who would turn them off?"). Code review surfaced the security + scope arguments above. The fix was `Enabled: false` default. Operators explicitly set `observability.metrics.enabled: true` (and optionally `observability.metrics.addr: ":9090"`) to opt in. The `docs/self-hosting.md` page (7-iv) makes the opt-in explicit + describes the no-auth-in-v0 caveat ("put behind a private network or a reverse proxy").

**Rule:** Every new operator-facing flag that EXPOSES SOMETHING (an endpoint, a header, a log field, a telemetry surface) defaults to `false` / "off" / `none`. The flag in the YAML schema is documented inline with the security implication ("This exposes an unauthenticated HTTP endpoint; place it behind a private network or reverse proxy"). The opt-in is a single line of operator YAML. New operator-facing flags that CONFIGURE existing behavior (timeouts, retry counts, log levels) may default to whatever the safe-default is — but exposure flags are off. Generalizes to: trace sampling rate defaults to 0, debug pprof handler defaults to disabled, verbose log mode defaults to disabled, the WebSocket-debug-shim defaults to disabled. Add a build-fail item: "new observability flag ships with `Enabled: true` default → Fail" (or, more precisely, "new EXPOSURE flag ships defaulted-on → Fail").

Reference: `relay/internal/config/config.go` `MetricsConfig.Enabled` (default `false`); `relay/internal/config/defaults.go` applyDefaults (no override); `docs/self-hosting.md` § Metrics (the opt-in section + the no-auth caveat).

---

### L028: Distroless multi-stage Docker template — minimal CVE surface, nonroot user, no shell, no package manager

The natural-but-wrong Dockerfile for a Go binary: a single-stage `FROM golang:1.23.2-alpine`, `COPY . .`, `RUN go build`, `ENTRYPOINT ["./relay"]`. The image is ~400 MB (the entire Go toolchain + alpine package manager + shell + libc); the running container has a shell so any RCE drops into an interactive shell; the running container has a package manager so any successful RCE can install arbitrary binaries; the running container runs as root unless you explicitly downgrade. Every one of those is a CVE-amplification vector — the same RCE is a self-contained binary execution on distroless and a compromise-the-image on alpine.

**Where it hit:** Phase 7 `relay/Dockerfile` design. The first draft was the single-stage alpine pattern above. The decomposition Q10 + design spec §15 + the distroless / static-debian12:nonroot guidance combined into the canonical pattern: stage 1 `golang:1.23.2-alpine` builds the static binary; stage 2 `gcr.io/distroless/static-debian12:nonroot` runs it. Image total < 50 MB (enforced via the build-fail invariant). No shell, no package manager. Runs as the distroless `nonroot` user (UID 65532). The `docker exec intake-relay id -u` assertion in `drive-docker-compose.ts` pins the nonroot invariant — if a future refactor accidentally `USER root`s the image, the smoke fails.

**Rule:** The distroless multi-stage Docker pattern is the TEMPLATE for any future Go binary in this monorepo. Stage 1: `golang:<EXACT-PIN>-alpine` (or `golang:<EXACT-PIN>` if CGO is needed; see the spec note). Stage 2: `gcr.io/distroless/static-debian12:nonroot` (for pure-Go static binaries) or `gcr.io/distroless/base-debian12:nonroot` (for binaries that need libc). `USER nonroot` is implicit in the `:nonroot` tag (UID 65532) — never override it without a recorded reason. `ENTRYPOINT` uses the exec form (`["./binary"]`), not the shell form. Final image size < 50 MB for static-debian12, < 100 MB for base-debian12 (build-fail invariants). A smoke MUST assert the nonroot UID via `docker exec <container> id -u` — without that assertion, a future `USER root` change ships silently. Generalizes to: every public-facing container runs as a non-root, non-zero UID; every container has a smoke that proves it.

Reference: `relay/Dockerfile` (multi-stage, distroless target, nonroot user); `relay/.goreleaser.yaml` `dockers:` block (same image for goreleaser-built releases); `core/smoke/drive-docker-compose.ts` `assertDistrolessNonrootUID()` (load-bearing UID smoke); Phase 7 README §6 build-fail items "image size > 50 MB → Fail" + "running user is root or empty → Fail".

---
