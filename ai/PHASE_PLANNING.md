# Phase Planning Template

How to structure a phase (e.g. `phase-1`, `phase-2`) and its sub-plans so deliverables actually work end-to-end and don't repeat the silent-failure mistakes of past phases. Every new phase MUST follow this structure. Read it before authoring a phase README or sub-plan.

---

## Why this exists

Past work took ~24 wall-clock hours of debugging that should have taken ~6. Postmortem traced the cost to four root causes, each of which this template addresses:

1. Phases shipped Auth0 IaC that was never end-to-end smoke-tested. `auth0-deploy-cli` v8 silently dropped its YAML files. The "Import Successful" looked identical to a real success. → addressed by **Smoke checklist requirement** below.
2. Architectural decisions were inherited rather than chosen with eyes open, surfacing only at deploy time. → addressed by **Architectural Decision Record requirement**.
3. Tool versions were unpinned. → addressed by **Tool Version Pinning** requirement.
4. Silent config failures  were treated as informational rather than fatal. → addressed by **Build-Fail Discipline**.

---

## Required structure for a phase

Every phase lives in `ai/tasks/phase-XX/` and contains:

```
ai/tasks/phase-XX/
├── README.md                  # this template's index
├── XX-i-<name>-plan.md         # first sub-plan
├── XX-ii-<name>-plan.md
├── ...
└── XX-N-<name>-plan.md
```

The phase README MUST have these sections in this order:

### 1. Spec link
A link to the design spec (in `docs/specs/`) that this phase implements.

### 2. Architectural Decision Record (ADR) summary
A 3-5 bullet summary of WHICH architectural choices this phase locks in, and the TRIGGERS for revisiting them. Example from Phase 0g:

> - Single-container Quinoa+RESTEasy hosting (Pattern A) — frontend + backend in one Docker image. Triggers for revisit: (a) need a second SPA, (b) frontend deploy cadence diverges from backend, (c) wall-clock-cost of FE iterations crosses ~5 min/round-trip.
> - SPA route filter at Vertx order 1000 to catch client-side routes — required because Quinoa's order-40000 fallback runs after RESTEasy's NotFoundException terminates the chain. Triggers for revisit: (a) move to Pattern B (split frontend), (b) Quinoa changes its routing model.

If the phase doesn't lock in any architectural decisions (e.g. a pure feature phase), explicitly state "No new architectural decisions; inherits Phase XX's ADRs."

### 3. Sub-plan index table
The same `| # | Plan | Driver | Effort | Status |` table that Phase 0g uses.

### 4. Dependency graph
ASCII or mermaid diagram showing the order sub-plans must execute.

### 5. Tool version pin list
For every external tool the phase introduces or upgrades, an exact version with a one-line justification:

```
| Tool | Version | Reason |
|---|---|---|
| auth0-deploy-cli | 8.33.0 (exact) | tested with this version; semver-caret allowed silent break in YAML support |
| aws-cdk-lib | 2.165.0 (exact) | matches the BOM the synth was tested against |
```

Caret-versioning (`^X.Y.Z`) is forbidden for any tool that produces deploy-time artifacts (CDK, deploy CLIs, codegen). Caret is fine for libraries with strict semver guarantees and active CI.

### 6. Build-fail checklist
List all the warnings/silent-failure shapes the phase's CI pipeline must fail on. Examples:

- [ ] `Unrecognized configuration key` from Quarkus startup → fail build
- [ ] `Vite warns: ignoring property` → fail build
- [ ] Any `--config_file ... .yaml` invocation of an `import`-style CLI → fail build (use JSON or env-vars only)

### 7. Final smoke (mandatory)
Concrete steps that prove the phase delivers what the spec promised, exercised against a real (or staging-real) environment:

```
1. Pre-condition: <fresh state, e.g. "DataStack RDS empty, no users in users table">
2. Execution: <user-visible action, e.g. "navigate to https://staging.refsquare.com, click Login, complete Auth0 flow">
3. Verification: <observable assertion, e.g. "Hello page renders 'Hello from RefSquare staging.', users table now contains a row with sub=google-oauth2|...">
4. Teardown / repeat: <can the smoke be re-run? what cleanup is needed?>
```

A phase is NOT done until this smoke passes from a clean state. Empty smoke section = phase not allowed to merge.

---

## Required structure for a sub-plan

Each `XX-<roman>-<name>-plan.md` must contain these sections:

### 1. Goal (one paragraph)
What this sub-plan delivers, in plain English.

### 2. Design references
Pointers to spec section, parent phase ADR, etc.

### 3. Files touched
A table listing each file the sub-plan creates or modifies, with a one-line "why."

### 4. Tasks
Numbered checklist of implementation tasks, each tight enough to be a single PR or single subagent dispatch.

### 5. Smoke (mandatory)
The same as the phase-level smoke but scoped to THIS sub-plan's deliverable. If this sub-plan can't be smoked in isolation (because it depends on a later sub-plan to produce a runnable artifact), state that explicitly: "Smoke runs only via the parent phase's final smoke (sub-plan XX-vii)."

### 6. Done criteria
Bulleted checklist of what "done" means for this sub-plan. The smoke pass MUST be one of the done criteria.

---

## Anti-patterns (don't do these)

These are real failure modes from past phases. Avoid them in your phase planning:

1. **"It compiles → done."** Phase 0f shipped Auth0 YAML that compiled fine, was committed, and was silently ignored at deploy time. Compilation is necessary, not sufficient.
2. **"The IaC was reviewed → done."** Phase 0f's auth0 IaC was code-reviewed; nobody ran `auth0-deploy.sh staging` to verify the YAML actually applied. Code review is necessary, not sufficient.
3. **"My local dev environment works → staging will work."** Phase 0g local dev exercised mock JWTs (`test-jwt` profile) which auto-coerce `email_verified` to Java `Boolean`. Real Auth0 access tokens carry custom claims as `JsonValue`. Different code path; only staging exercised it.
4. **Caret-pinned deploy CLIs.** If a tool's behavior is load-bearing for your deploy, pin the exact version. The cost of a manual upgrade is one PR; the cost of a silent breaking change at deploy time is hours of debugging.
5. **Empty smoke sections.** A sub-plan that doesn't define its smoke won't be smoke-tested. The smoke is the only thing that proves the deliverable works.
6. **Inheriting architectural decisions silently.** If your phase touches deploy topology, frontend hosting, or auth flow, write an ADR section even if you're not changing the decision. Document WHY you're keeping the inherited choice; that creates the trigger list for revisiting it.

---

## Mandatory phase-end actions

Before opening a bundled PR for a phase:

- [ ] Final smoke from the README has been executed and passed against a real environment
- [ ] Any LESSONS.md entries from new failure modes have been added (add to `ai/LESSONS.md`)
- [ ] If memory was relied on (memory files in `~/.claude/projects/.../memory/`), update affected entries
- [ ] If new tools were introduced, exact versions are pinned in their package manager files (`package.json`, `build.gradle`, etc.)
- [ ] If architectural decisions were made, an ADR section is in the phase README

---

## Quick reference: the "is this phase done?" checklist

Use this at the END of every phase, before opening the bundled PR:

```
[ ] All sub-plans marked complete in the phase README
[ ] All sub-plan smokes pass (or are documented as deferred to parent smoke)
[ ] Phase-level final smoke passes against a real environment
[ ] No outstanding "Unrecognized configuration key" or equivalent silent-failure warnings
[ ] LESSONS.md updated with anything novel learned this phase
[ ] Tool versions pinned exactly in package files
[ ] ADR section in phase README captures architectural decisions + trigger list
[ ] Bundled PR opened with link to phase README
```
