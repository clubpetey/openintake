# Phase 6 — Deferred follow-ups (Phase 7 candidates)

The 6-iv code quality review (APPROVED) flagged four items that do NOT block the
phase-6 → main merge but should be addressed in a follow-up phase.

## I1 (Important) — buildRegistry is a third startup gate not folded into the L022 consolidation

`relay/cmd/relay/main.go` `buildRegistry` has its own `os.Exit(1)` sites (when
an adapter's `Configure` fails OR when no adapters are enabled). These bypass
the shared `problems` slice that Phase 5 `startupProblems` + Phase 6
`validateAttachments` accumulate into. Concrete failure: an operator with both
Phase-5 misconfigs (anonymous + no captcha) AND an adapter configure failure
(e.g., chatwoot `api_token_env: NONEXISTENT`) sees only the adapter error,
fixes it, restarts, then sees the auth problem — two restart cycles instead
of one.

**Suggested fix (Phase 7):** refactor `buildRegistry` to return
`([]adapter.Adapter, []string)` and accumulate per-adapter Configure failures
into the shared slice. The `len(registry) == 0` check becomes a problem entry.

## I2 (Important) — TestStartupGates_CombinedPhase5AndPhase6Problems doesn't test cross-phase wiring in main()

The Go test calls `startupProblems()` + `validateAttachments()` independently
and appends in test code — proves `append([]string,...)` works, not that
`main()` invokes both gates and produces ONE consolidated line. The actual
cross-phase wiring is only exercised by `run-q9-smoke.sh` (the shell smoke).

**Suggested fix (Phase 7):** extract the gate orchestration from `main()`
into a testable function (e.g.,
`accumulateStartupProblems(cfg, licState, logger) (Deps, []string)`) and
test that directly. Then the shell smoke becomes belt-and-braces rather
than load-bearing.

## M2 (Minor) — validateAttachments short-circuit returns full parsed value when Enabled=false

`relay/cmd/relay/main.go` `validateAttachments` returns `parsed` even when
`!parsed.Enabled` — carrying any bad `Storage.Mode` value. Defensive choice
would be `config.AttachmentsConfig{}` zero-value. Functionally fine today
since downstream consumers gate on `Enabled` first; defensive future-proofing.

## M4 (Minor) — run-q9-smoke.sh working-directory dance

The script repeats `cd relay && go run ...` patterns. A single subshell
wrapper `(cd relay && go run ...)` or `go run -C relay ...` would be cleaner.

---

These follow-ups are tracked here rather than in an issue tracker because the
project's working pattern is task-files under ai/tasks/. A Phase 7 README
can simply reference this file as a backlog source.
