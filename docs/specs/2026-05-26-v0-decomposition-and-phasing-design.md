# Intake v0 — Decomposition & Phasing Design

> **Status:** Approved design (brainstorming output), pre-planning
> **Date:** 2026-05-26
> **Implements:** [docs/PROJECT.md](../PROJECT.md) (v0 spec)
> **Governs:** the `ai/tasks/phase-XX/` structure per [ai/PHASE_PLANNING.md](../../ai/PHASE_PLANNING.md)

This document does **not** redefine the product — [docs/PROJECT.md](../PROJECT.md) remains the source of truth for scope, schema, and interfaces. This document decides **how v0 gets built**: how the work decomposes into independently-buildable phases, in what order, with what smoke proof, and it resolves the spec's blocking open questions.

---

## 1. Build-shape decision (ADR)

**Decision: vertical slices, each ending in a real smoke test.** Phase 1 is a walking skeleton that exercises the entire spine (widget → relay → adapter) on one provider and one adapter. Every later phase adds **one axis of breadth** and ships with its own end-to-end smoke against a real (or staging-real) environment.

**Why not "layered foundations first"** (all schema → all relay infra → all providers → all adapters → widget): [ai/PHASE_PLANNING.md](../../ai/PHASE_PLANNING.md) mandates that every phase end in a smoke that proves the deliverable works against a real environment, and states "empty smoke section = phase not allowed to merge." Layered phases ("all 4 providers," "all of schema") cannot be smoked end-to-end until the very end — precisely the "it compiles → done" anti-pattern (§Anti-patterns #1) the template exists to prevent. Vertical slicing makes every phase smoke-able the day it lands.

**Triggers to revisit this decision:**
- (a) Phase 1's seam interfaces (`Provider`, `Adapter`, auth middleware) prove unstable and keep churning across P2–P5 → reconsider freezing them harder or serializing.
- (b) A breadth phase turns out to require schema changes → that change reopens Phase 0's contract and must re-run P0's smoke before dependents proceed.

---

## 2. Phase decomposition

Eight phases. Phase 0 is the wire-contract foundation; Phase 1 proves the spine end-to-end; Phases 2–7 each add one demonstrable axis.

| Phase | Deliverable | Adds | Final smoke (real environment) |
|---|---|---|---|
| **0 — Contract spine** | Monorepo skeleton, `schema/payload.v1.json`, codegen → `core/src/generated/payload.ts` + `relay/internal/payload/types.go`, CI staleness gate | the wire contract (nothing runnable) | Edit schema → run codegen → both TS+Go regenerate & compile; CI **fails** on stale generated files, **passes** when fresh |
| **1 — Walking skeleton** | Relay (config loader, chi server, `/v1/health`, `/v1/version`, `/v1/intake/init`, `/v1/intake/turn` SSE, `/v1/intake/submit`) + `@intake/core` + minimal `@intake/vue` | **anonymous** auth, **anthropic** provider, **webhook** adapter only | Embed widget in `examples/vue-anonymous`, run relay with `ANTHROPIC_API_KEY`, hold a 2-turn conversation, click Submit → local webhook receiver logs the canonical payload |
| **2 — Provider breadth** | `openai`, `gemini`, `ollama` behind the frozen `Provider` iface; optional Ollama bearer token | 3 more LLM providers | 5-turn conversation completes end-to-end through **each** provider; Ollama runs offline with no API key |
| **3 — Adapters + license** | `chatwoot`, `fider`, `zendesk`, `linear`; Ed25519 license verify, trial/free mode, router (`routing_hint` → rules → default), `license-tool` CLI | 4 adapters, license gate, routing | Route a real ticket into a **live Chatwoot**; paid adapter blocked w/o license, permitted w/ signed test license; free-mode disables paid adapters with a clear startup log |
| **4 — Auth breadth** | Email magic-link (SMTP → 6-digit code → 15-min JWT), host-app SSO (JWKS validation, Auth0/OIDC/custom HS256+RS256) | 2 more auth modes | Email flow end-to-end via Mailpit (`user.verified=true`); SSO validates a real Auth0/OIDC RS256 access token |
| **5 — Abuse & spend control** | Per-IP token bucket, per-session turn/token caps, daily LLM budget cap, CAPTCHA (Turnstile/hCaptcha), strict CORS/origin enforcement | guardrails | Exceed burst → 429; exceed daily budget → 503 + `Retry-After` (next UTC midnight); anonymous requires CAPTCHA; cross-origin request blocked |
| **6 — Attachments** | html2canvas capture + redaction UI in widget; relay MIME magic-byte validation; adapter forwarding via downstream native upload | screenshots | Capture + redact a region in the widget → Submit → image appears on the downstream Chatwoot ticket |
| **7 — Release & ops** | structured JSON logging, Prometheus metrics, goreleaser (5 platforms), Docker/ghcr, npm publish, docs, `docker-compose` demo, three examples | distribution | Tag `vX.Y.Z` → 5 platform binaries + Docker image + npm dry-run all produced; `docker-compose up` → full stack, ticket flows end-to-end |

Each phase becomes one `ai/tasks/phase-XX/` directory with a README + sub-plans authored per [ai/PHASE_PLANNING.md](../../ai/PHASE_PLANNING.md). Every sub-plan carries its own scoped smoke; the phase README carries the final smoke above.

---

## 3. Dependency graph

```
P0 ──► P1 ──► P2
          │
          ├──► P3 ──► P6        (attachments need an adapter to forward to)
          ├──► P4
          └──► P5
P1..P6 ──► P7                    (release bundles everything that exists)
```

- **P0 → P1:** the skeleton consumes generated types; the contract must exist first.
- **P1 → {P2, P3, P4, P5}:** all four breadth phases depend only on P1's seams and are **mutually independent** — parallelizable once P1 lands.
- **P3 → P6:** attachment forwarding needs a real downstream adapter (Chatwoot) to forward to.
- **everything → P7:** release pipeline bundles whatever phases have shipped.

### 3.1 Interface-freeze precondition (enables parallel P2–P5)

Phase 1 must **freeze** these seams as its exit criterion; P2–P5 may extend implementations behind them but must not change the signatures:

- `llm.Provider` interface (`relay/internal/llm/provider.go`) — per PROJECT.md §7.
- `adapter.Adapter` interface (`relay/internal/adapter/adapter.go`) — per PROJECT.md §8.
- Auth middleware contract (how a request carries identity into a turn/submit handler).
- The chi server's route-registration and middleware-chain shape.

If a breadth phase needs to change one of these, it must reopen and re-smoke Phase 1's contract before proceeding (a revisit trigger from §1).

---

## 4. Open-question resolutions

From PROJECT.md §19. Six block specific phases and are resolved here; two are pre-P0 gates; two are deferred business/doc decisions that block no code.

| # | Question | Resolution | Blocks |
|---|---|---|---|
| Q7 | Schema codegen tool | `json-schema-to-typescript` (TS) + `go-jsonschema` (Go), **exact-pinned**. Codegen produces deploy-time artifacts → caret-versioning forbidden per template §5. Chosen over `quicktype`: each is single-purpose with idiomatic per-target output. | **P0** |
| Q5 | Default LLM models | anthropic `claude-sonnet-4-6`; ollama `llama3.1`; openai + gemini ship a documented, configurable default (no hard pin — user-supplied keys, models churn). Override path documented in `docs/llm-providers.md`. | P1, P2 |
| Q4 | Ollama auth | Optional `ollama.bearer_token_env` config for hardened self-host (Ollama has no native auth). | P2 |
| Q3 | Trial-state path (Windows) | Use Go `os.UserConfigDir()`: `%AppData%\intake\state.json` (Windows), `~/.config/intake/` (Linux), `~/Library/Application Support/intake/` (macOS). Cross-platform resolved. | P3 |
| Q10 | License tool OSS? | Maintainer-only. Lives in `license-tool/`; **excluded from all release artifacts** (goreleaser ignore + npm not applicable). | P3 |
| Q9 | Anonymous without CAPTCHA | **Fail-closed:** relay refuses to start if `auth.anonymous=true` and CAPTCHA disabled, unless explicit `auth.anonymous.allow_without_captcha: true`. Safe default with an explicit escape hatch; this is a fatal config error, not a warning (template build-fail discipline §4). | P5 |
| Q8 | Redaction UX | Widget provides redaction tools; **no** forced "no-PII" confirmation by default. Configurable `require_redaction_ack` (default `false`) lets a host app opt into a forced acknowledgment. | P6 |
| Q6 | System prompt IP | Apache 2.0; add a header comment marking it as product prompt-IP for clarity. Doc-only. | none |
| Q1 | Product name | **Keep `intake` placeholder.** Hard **pre-P0 gate** — sets npm scope `@intake/*`, Go module path, GitHub org, domain. Must be locked before Phase 0 begins. **Requires maintainer decision.** | pre-P0 |
| Q2 | Pricing tiers | **Deferred.** Business decision, blocks no code. Free / Pro / Team structure stands; numbers TBD before public launch (a P7-adjacent concern, not a build blocker). | none |

---

## 5. Spec inconsistencies to fix

Three internal contradictions in PROJECT.md that this design settles. These should be corrected in PROJECT.md when convenient.

1. **React widget scope.** Goal #1 (§2) says "Vue 3 **AND React** widget," but the overview (§1), architecture (§3), repo layout (§14 — only `vue/`), and non-goals (§18 — "React widget (v1)") all say **Vue-only for v0**. → **Resolved: v0 is Vue-only.** Goal #1 is an overreach; correct its wording.
2. **LLM provider count.** Goal #4 (§2) says "**Three** LLM providers: anthropic, openai, ollama," but the §7 implementations table lists **four** (adds `gemini`), while the config example (§9) and repo layout (§14) show only three. → **Resolved: four providers including `gemini`** (the §7 table is the most specific statement). Fix goal #4's count, add `gemini` to the config example, and add `relay/internal/llm/gemini/` to the layout. *(Open to dropping gemini to v1 if the maintainer prefers three — flagged, not assumed.)*
3. **Go module path.** The §8 adapter interface imports `intake/internal/payload`, hard-coding the placeholder name. Tied to Q1; resolves automatically when the product name is locked.

---

## 6. Pre-Phase-0 gates (defaults adopted; overridable)

These two items are decided with safe defaults so planning can proceed. Both are cheap to override later:

- **Product name (Q1) — proceeding on the `intake` placeholder.** P0 hard-codes the npm scope `@intake/*`, Go module path `intake/...`, and repo paths. The placeholder is deliberately a single, mechanical find-replace away from a final name. Phase 0's plan will isolate every name-bearing token (module path, package scope) so the rename is one scripted pass. **Override:** lock a final name before P0 implementation begins and the rename cost drops to zero.
- **Provider count (inconsistency #2) — proceeding with four providers including `gemini`.** Matches the most specific statement in PROJECT.md §7. Affects only P2 scope. **Override:** drop gemini to v1 to return to three; removes one implementation from P2, no structural impact.

All other resolutions in §4 are decided and need no further input.

---

## 7. Next step

Per the brainstorming → writing-plans workflow, the immediate follow-on is **not** to plan all eight phases at once. Each phase gets its own spec → plan → build cycle. The next planning action is to author **Phase 0** (`ai/tasks/phase-0/`) per [ai/PHASE_PLANNING.md](../../ai/PHASE_PLANNING.md), gated on the maintainer resolving the §6 pre-P0 items.
