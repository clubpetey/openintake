# CLAUDE.md — General Instructions

This file is the primary context document for all AI agents and Claude Code sessions working on this codebase. Read it fully before making any changes. The general description of the project is found in `docs/PROJECT.md`. It contains guiding principles and concepts. Read it fully before making decisions. The instructions in this file apply to coding, architecture and design.

---

## The AI Directory
- the `ai` directory is not part of the project, but a source of information used for projects.
- the `ai/tasks` directory is where you should store task lists, analysis, plans and other work product
- the `ai/memory` directory is used to store information about interactive sessions


## 1. Plan Mode Default
- Enter plan mode for ANY non-trivial task (3+ steps or architectural decisions)
- If something goes sideways, STOP and replan immediately, don't keep pushing
- Use plan mode for verification steps, not just building
- Write detailed specs upfront to reduce ambiguity

## 2. Subagent Strategy
- Use subagents liberally to keep main context window clean
- Offload research, exploration, and parallel analysis to subagents
- For complex problems, throw more compute at it via subagents
- One task per subagent for focused execution

## 3. Self-improvement (Self-annealing) loop
- After any correct from the user: update `ai/LESSONS.md` with the pattern
- Write rules for yourself that prevent the same mistake
- Ruthlessly iterate on these lessons until mistake rate drops
- Review lessons at session start for relevant instructions

### When things break
1. Read the error message and stack trace
2. Fix the code, test it again, make sure it works (unless it uses paid tokens/credits/etc—in which case you check w user first)
3. Write rules for yourself that prevent the same mistake
4. update `ai/LESSONS.md` with the information, pattern, or rules
5. System is now stronger 

## 4. Verification Before Done
- Never mark a task complete without proving it works
- Diff behavior between `main` branch and your changes when relevant
- Ask yourself: "Would a staff engineer approve this?"
- Run tests, check logs, demonstrate correctness

## 5. Demand Elegance
- For non-trivial changes: pause and ask "Is there a more elegant way?"
- if a fix feels hacky: "Knowning everything I know now, implement the elegant solution"
- Skip this for simple, obvious fixes -- don't over-engineer
- Challenge your own work before presenting it

## 6. Autonomous Bug Fixing
- When given a bug report: just fix it. Don't ask for hand-holding
- Point at logs, errors, failing tests - then resolve them
- Zero context switching require from user
- Go fix failing CI tests without being told how

## Task Management
1. **Plan First**: Write plan to `ai/tasks/<description>.md` with checkable items
2. **Verify Plan**: Check in before starting implmenetation
3. **Track Progress**: Mark items complete as you go
4. **Explain Changes**: High-level summary at each step
5. **Document Results**: Add review section to task file
6. **Capture Lessons**: Update `ai/LESSONS.md` after corrections

## Phase Planning
Multi-sub-plan phases (`phase-Xa` through `phase-Xz`) MUST follow the structure in `ai/PHASE_PLANNING.md`. That document is the authoritative template — read it before authoring a phase README or sub-plan. Key requirements: every sub-plan has a mandatory Smoke section that proves the deliverable works end-to-end, every external tool is exact-version-pinned (no caret), every silent-failure warning shape is added to a build-fail list. These rules exist because Phase 0f shipped silently-broken Auth0 IaC and Phase 0g spent ~24 hours debugging downstream symptoms.

## Core Principles

- **Simplicity First**: Make every change as simple as possible
- **No Laziness**: Find root causes. No temporary fixes. Senior Developer standards.
- **Minimal Impact**: Changes should only touch what's necessary. Avoid introducing bugs

## Code Style
- Prefer writing clear code and use inline comments sparingly
- Read `ai/JAVA_CODE.md` when modifying or creating any Java files.
- Read `ai/WEB_CODE.md` when modifying a JS, TS, or CSS files.
- Read `ai/SQL_CODE.md` when modifying a SQL file, creating DDL, or optimizing SQL queries.

