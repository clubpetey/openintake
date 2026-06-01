# 7-iv Docs + Governance — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## Goal

Author the operator-facing documentation surface and governance files that any v0 OSS release expects. Four new docs in `docs/` (`quickstart.md`, `self-hosting.md`, `license.md`, `adapters.md`), a rewrite of the repo `README.md`, and four governance files at the repo root (`LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, `COMMERCIAL.md`). Runs in parallel with 7-iii — disjoint file territory (docs vs. demo stack). No production code touched, no schema change, no frozen seam modified. After this sub-plan: an operator can clone the repo, read `quickstart.md`, run the docker-compose demo, and reach "ticket in webhook log" inside 30 minutes; a production operator can configure every Phase 4/5/6/7 feature from `self-hosting.md`; the legal foundation (Apache 2.0 + draft commercial terms) is in the tree.

## Architecture

Three artifact families. (1) Four new `docs/*.md` operator-facing references that link to one another and to existing docs (`docs/attachments.md`, `docs/PROJECT.md`). (2) A repo `README.md` rewrite that leads with what intake IS, the canonical demo command, and a links table to everything else. (3) Four governance files at the repo root — `LICENSE` (verbatim Apache 2.0), `CONTRIBUTING.md` (branch model + commit conventions + the `ai/tasks/` phase model + local pre-commit commands), `SECURITY.md` (vuln-reporting policy + existing security stance from PROJECT.md §17), `COMMERCIAL.md` (draft terms for paid-adapter use, flagged "DRAFT — legal review required").

No dependencies, no schema change, no `go.mod` change, no `package.json` change. Every change in 7-iv is a Markdown or plain-text file under `docs/` or the repo root.

After this sub-plan: every operator entry point has a paved road. 7-v consumes `docs/quickstart.md` for its manual walkthrough smoke (item 6 of the Phase 7 final smoke).

## Tech Stack

- CommonMark / GitHub-Flavored Markdown — matches the existing `docs/attachments.md` style.
- Plain text (`LICENSE`) — verbatim Apache 2.0.
- No new dependencies, no codegen, no Go or TS changes.

## Design References

- Phase 7 README §5.8 (docs + governance file inventory) — the source of truth for what gets written
- Phase 7 README §8.5 (`accumulateStartupProblems` shape) — the source of truth for the consolidated startup error contract that `self-hosting.md` cross-references
- Phase 7 design spec §3.2 (Prometheus metrics decision) + §5.1 (metrics package shape) — source of truth for the metrics section in `self-hosting.md`
- Phase 7 design spec §15 (PROJECT.md inconsistencies to fix) — optional fold-in if convenient during 7-iv (deferred is fine)
- `docs/PROJECT.md` §12 (license model: Ed25519-signed JSON, file path resolution, trial mode) — source of truth for `docs/license.md`
- `docs/PROJECT.md` §13 (free/paid adapter matrix) — source of truth for the tier column in `docs/adapters.md` and for `COMMERCIAL.md`
- `docs/PROJECT.md` §14 (repo layout) — source of truth for the README repo layout section
- `docs/PROJECT.md` §17 (security stance) — source of truth for `SECURITY.md` scope + existing posture
- `docs/attachments.md` — the style exemplar; the new docs MUST match this tone (operator-facing, examples-heavy, no marketing fluff)
- Phase 6 `6-iv-smoke-docs-plan.md` — the structural exemplar for docs-heavy sub-plans (sub-plan cadence, READ-style task ordering, evidence capture)
- LESSONS L005 — redact-before-truncate (referenced in `SECURITY.md` posture)
- LESSONS L011 — Chatwoot two-call contact-then-conversation flow (referenced in `docs/adapters.md` chatwoot section)
- LESSONS L013 — JWT algorithm pinning (referenced in `SECURITY.md` posture)
- LESSONS L018 — replay protection (referenced in `SECURITY.md` posture)
- LESSONS L020 — Chatwoot's multipart-vs-JSON branching (referenced in `docs/adapters.md` chatwoot section; the 3-call shape after Phase 6)
- LESSONS L022 — consolidate Q9 startup-gate problems (referenced in `self-hosting.md` startup-gate section)
- `relay/internal/config/testdata/sample.yaml` — source of truth for the env-var matrix in `self-hosting.md`
- Each adapter's `Configure()` keys (verified in `relay/internal/adapter/{webhook,chatwoot,fider,linear,zendesk}/*.go`) — source of truth for the per-adapter config blocks in `docs/adapters.md`

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `LICENSE` | Create | Verbatim Apache 2.0 text + project copyright line |
| `COMMERCIAL.md` | Create | DRAFT terms for paid-adapter use (flagged for legal review) |
| `SECURITY.md` | Create | Supported-versions matrix + vuln-reporting policy + existing security stance |
| `CONTRIBUTING.md` | Create | Branch model + commit conventions + phase model + local pre-commit commands |
| `docs/license.md` | Create | Free vs paid model + license-file resolution + trial mode + expiry behavior |
| `docs/adapters.md` | Create | 5-adapter overview matrix (tier × purpose × config keys × env vars × notes) |
| `docs/quickstart.md` | Create | 30-minute onboarding path (docker-compose or bare-metal) + "what just happened" explanation |
| `docs/self-hosting.md` | Create | Production operator's reference: binary, Docker, env vars, metrics, abuse gates, attachments, auth modes, logging, license |
| `README.md` | Rewrite | Lead-with-what-intake-IS, canonical demo command, links table, build instructions, dual-license callout |

---

## Tasks

### Task 1: Create `LICENSE` (verbatim Apache 2.0)

**Files:** Create `LICENSE`

- [ ] **Step 1: Write the canonical Apache 2.0 text + project copyright line**

Create `LICENSE` with the verbatim text from <https://www.apache.org/licenses/LICENSE-2.0.txt>. The closing copyright block uses the working-name convention; Q1 final-name lock will rewrite it post-7-iv.

```text
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for describing the origin of the Work and
      reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.

   END OF TERMS AND CONDITIONS

   APPENDIX: How to apply the Apache License to your work.

      To apply the Apache License to your work, attach the following
      boilerplate notice, with the fields enclosed by brackets "[]"
      replaced with your own identifying information. (Don't include
      the brackets!)  The text should be enclosed in the appropriate
      comment syntax for the file format. We also recommend that a
      file or class name and description of purpose be included on the
      same "printed page" as the copyright notice for easier
      identification within third-party archives.

   Copyright 2026 The intake authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
```

> **Note for the implementer:** if any whitespace/character drift occurs vs. the canonical apache.org text, use the canonical source as the tiebreaker. The Apache 2.0 text is legally non-negotiable.

- [ ] **Step 2: Commit**

```bash
git add LICENSE
git commit -m "chore(7-iv): LICENSE — verbatim Apache 2.0 + project copyright"
```

---

### Task 2: Create `COMMERCIAL.md` (DRAFT)

**Files:** Create `COMMERCIAL.md`

- [ ] **Step 1: Author the draft commercial terms with the legal-review banner**

Create `COMMERCIAL.md`:

```markdown
# Commercial License (DRAFT)

> **DRAFT — legal review required before v0.1.0 public release.**
>
> The terms below are a sensible-default starting point for the open-core / paid-adapter pattern. The maintainer's legal counsel MUST review and rewrite this document before any public release, paid contract, or invoice. Nothing in this file constitutes a binding offer, warranty, or contract.

## What requires a commercial license

The **code** in this repository is licensed under Apache 2.0 (see `LICENSE`). You may read, fork, modify, and redistribute the source code freely under those terms.

The **use** of certain adapter functionality in production deployments requires a paid commercial license. Specifically, the following adapters are gated at runtime by the intake license-key check:

- `zendesk` — Zendesk ticketing adapter
- `linear` — Linear issue adapter

The following adapters and components are **always free** under Apache 2.0:

- `webhook`, `chatwoot`, `fider` adapters
- The relay binary and source code
- The `@intake/core` and `@intake/vue` widget packages
- The schema, codegen pipeline, and developer tooling
- All four LLM providers (`anthropic`, `openai`, `gemini`, `ollama`)
- All three authentication modes (`anonymous`, `email`, `sso`)

This pattern is sometimes called "open core" — see Sentry's licensing model (referenced in `docs/PROJECT.md` §13) for the established precedent.

## What "use" means

A "use" of a paid adapter is one in which:

- the relay process is configured with `adapters.<paid-adapter>.enabled: true`, AND
- the relay process successfully invokes that adapter's `Create()` method against a real downstream system in production.

Compiling, running, testing, and debugging the paid adapters against test fixtures or sandbox environments is permitted under Apache 2.0 — you do not need a license to develop, contribute, or evaluate. You need a license to **operate** a paid adapter against a production downstream system.

## Trial period

New installations of intake automatically enable a 14-day trial during which all paid adapters operate fully without a license key. The trial begins on the first relay startup that resolves an installation-state file at `os.UserConfigDir()/intake/state.json`. Once the trial expires:

- Free adapters (`webhook`, `chatwoot`, `fider`) continue to operate.
- Paid adapters (`zendesk`, `linear`) are disabled. The relay starts cleanly, logs a warning at `slog.Warn` level for each paid adapter that was configured but is now license-gated, and routes traffic only through the enabled free adapters.

See `docs/license.md` for the full trial state machine.

## Pricing and contact

> **DRAFT — pricing model to be finalized post-Q1.** The placeholders below are NOT a binding offer. Contact the maintainer for current commercial terms.

To obtain a commercial license, contact:

- **Email:** `licensing@<domain>` *(placeholder — final domain TBD with Q1 final-name lock)*
- **Subject line:** "intake commercial license — \<your organization\>"

Please include in your inquiry:

- Your organization name and the deployment scale (approximate seats / tickets per month)
- Which paid adapters you need to license (`zendesk`, `linear`, or both)
- Your expected support requirements

## Draft license grant

> **DRAFT — replace with legal-team-approved language before v0.1.0 release.**

Subject to your timely payment of the agreed license fees and your compliance with the terms set forth in your executed commercial agreement, the Licensor grants You a non-exclusive, non-transferable, non-sublicensable license to operate the paid adapters identified in your commercial agreement in production deployments for the duration of the license term.

This commercial license:

- Does NOT modify the Apache 2.0 license under which the underlying code is distributed.
- Does NOT grant any right to redistribute, sublicense, or otherwise transfer the paid-adapter operating rights to any third party.
- Terminates immediately upon expiration of the license term or material breach.

Upon termination or expiration, you must disable the paid adapters in any production deployment. The relay's runtime license check enforces this gate automatically — you do not need to remove the code; configuring the affected adapters as disabled is sufficient.

## Source-available vs. open-source

The intake project intentionally distributes the paid-adapter source code under Apache 2.0 (not a source-available or restrictive license). This is deliberate:

- You can audit every line of code that runs in your environment.
- You can build, modify, and run the paid adapters against test fixtures without limitation.
- You can fork the project entirely under Apache 2.0 terms.

The commercial license is a **deployment-time runtime gate**, not a code-distribution restriction. This pattern is used by Sentry, MongoDB SSPL (modified), and similar open-core projects.

## Warranty disclaimer

Even with a paid commercial license, the software is provided "AS IS" per the Apache 2.0 warranty disclaimer in `LICENSE` Section 7. Commercial support agreements (if any) are separately negotiated and are not implied by the existence of a runtime license.

## Audit

> **DRAFT — audit terms to be finalized with legal review.**

The Licensor reserves the right, upon reasonable prior written notice, to audit your use of the paid adapters to verify license compliance. Audits will be conducted in a manner that minimizes disruption to your operations and that protects any confidential information.

## Governing law

> **DRAFT — governing law to be set by legal review based on the Licensor's jurisdiction.**

This commercial license shall be governed by and construed in accordance with the laws of \<jurisdiction TBD\>, without regard to its conflict-of-laws principles.

## Reminder

This entire document is a **DRAFT**. Treat it as a starting point for legal review, not as the operative commercial agreement. The operative commercial agreement is the document executed between you and the Licensor — not this Markdown file.

See also:
- `LICENSE` — Apache 2.0 for the code itself.
- `docs/license.md` — how the runtime license check works.
- `docs/PROJECT.md` §13 — the free/paid matrix and the open-core rationale.
```

- [ ] **Step 2: Commit**

```bash
git add COMMERCIAL.md
git commit -m "docs(7-iv): COMMERCIAL.md DRAFT — paid-adapter open-core terms (legal review required)"
```

---

### Task 3: Create `SECURITY.md`

**Files:** Create `SECURITY.md`

- [ ] **Step 1: Author the vulnerability-reporting policy**

Create `SECURITY.md`:

```markdown
# Security policy

## Supported versions

intake follows semantic versioning. During the v0 development cycle the following support matrix applies:

| Version | Status | Security fixes |
|---|---|---|
| 0.1.x | Active | Yes — patched on the `main` branch and released as patch versions. |
| 0.0.x | Pre-release | No — pre-release builds are not supported; upgrade to the latest 0.1.x. |

Once v1.0.0 ships, the v0.x line will enter maintenance mode for **6 months** with security-only patches. After that window, v0.x is end-of-life.

## Reporting a vulnerability

**Do NOT report security vulnerabilities through public GitHub Issues, public discussions, or any public channel.**

If you discover a security issue in intake — the relay, the widget packages, the `license-tool`, or any supported adapter integration — please email the maintainer privately:

- **Email:** `security@<domain>` *(placeholder — final domain TBD with Q1 final-name lock)*
- **Subject line:** `[SECURITY] <one-line summary>`
- **Encryption:** PGP key available on request; we will share a key fingerprint when you initiate contact.

### What to include in your report

- A clear description of the vulnerability and its impact.
- A minimal reproduction (config file, request payload, command sequence). For widget-side issues, include the browser + version.
- The affected version(s) — output of `intake-relay --version` if applicable.
- Your name and organization (optional, for credit).

### What to expect

- **Triage acknowledgement:** within **5 business days** of receipt.
- **Status update:** at least every **10 business days** until resolution.
- **Disclosure timeline:** coordinated disclosure window of **90 days** by default. We will work with you on extensions for complex issues or on accelerated disclosure for already-public vulnerabilities.
- **CVE coordination:** for high-severity issues, we will request a CVE through MITRE and credit you in the advisory (unless you request anonymity).

We do not currently operate a paid bug bounty. We will acknowledge reporters publicly in the release notes and (if you wish) in this file.

## In-scope

Security issues are in-scope when they affect:

- The relay binary (`relay/cmd/relay`) — including auth, abuse gates, attachment validation, license enforcement, adapter dispatch, license-key signature verification.
- The widget packages (`core/`, `vue/`) — XSS, secret exposure, embedded-page isolation, screenshot redaction integrity.
- The `license-tool/` — license-key generation and signature primitives.
- The supported adapter integrations — when the issue is in intake's adapter code (NOT in the downstream system itself).
- Build & release artifacts — the Dockerfile, goreleaser config, npm tarballs, and GitHub Actions workflows.

## Out-of-scope

The following are explicitly out-of-scope for the intake security policy:

- **Third-party LLM provider security** — vulnerabilities in Anthropic, OpenAI, Google Gemini, or Ollama themselves. Report those to the respective vendors.
- **Downstream adapter system security** — vulnerabilities in Chatwoot, Fider, Linear, Zendesk, or any webhook receiver. Report those to the respective vendors. Adapter *integration* bugs (e.g. an intake adapter mishandling a downstream response) ARE in-scope.
- **Misconfigurations by the operator** — running the relay without TLS, exposing the `/metrics` endpoint to the public internet, choosing a weak SMTP password. Documentation defects that would *cause* such misconfigs ARE in-scope.
- **Denial-of-service from upstream resource exhaustion** — if Chatwoot's API rate-limits us and our retry loop misbehaves, that's a bug; but raw bandwidth/CPU DoS against the relay is mitigated by the operator's reverse proxy (see `docs/self-hosting.md`).
- **Social engineering, physical security, and supply-chain attacks against the maintainer's machines** — these are tracked separately and are not solicited bug-bounty surface.

## Existing security posture

intake ships several invariants that are exercised by the Phase 0-7 smoke suite. Reviewers and reporters should know these are the existing baseline:

- **Redact-before-truncate in adapter error messages.** Tokens, API keys, and other secrets passed to adapters are scrubbed from error logs and HTTP error bodies *before* any string-truncation step. A truncated middle of a JSON object cannot leak the front of an API token. (LESSONS L005.)
- **Never include token material in error responses.** Adapter errors return a redacted summary, not a verbatim downstream body. (LESSONS L011.)
- **JWT algorithm pinning.** SSO mode pins the JWT verification algorithm via `jwt.WithValidMethods` to mitigate alg-confusion attacks. HS256 and RS256/ES256 are accepted only when explicitly configured. (LESSONS L013.)
- **Replay protection on magic-link tokens.** Email auth marks the token as consumed *before* issuing the session JWT, so a retry on a 5xx is intentionally a duplicate token-burn rather than a replay window. (LESSONS L018.)
- **Off-by-default observability.** The Prometheus `/metrics` endpoint is disabled by default and listens on a separate HTTP server (default `:9090`). Operators must explicitly opt in via `observability.metrics.enabled: true` and are responsible for putting it behind a private network or authenticated reverse proxy. (Phase 7 design spec §3.2.)
- **Distroless container image, nonroot user.** The relay's Docker image is `gcr.io/distroless/static-debian12:nonroot`. No shell, no package manager, no apt CVEs. Default UID is 65532. (Phase 7 design spec §3.4.)
- **Consolidated startup-gate exit.** Misconfigurations across auth, CIDR, abuse gates, attachments, observability, and adapter Configure are collected into a single ERROR log line at startup, then the process exits with code 1. Fail-fast prevents partial startup with degraded security posture. (LESSONS L022.)
- **License gate is non-fatal.** A failed license check disables the affected paid adapters but never bricks startup. Free adapters always continue. This avoids a license-server outage taking down the customer.
- **Schema-pinned wire contract.** The `payload.v1.json` schema is the source of truth for all `/v1/intake/submit` shapes. Codegen runs from the schema; any schema change is a `Phase X-i` decision with downstream consumer review.

## Security testing

Run the full smoke suite locally before opening a security-relevant PR:

```bash
cd relay && go test -race ./... && go vet ./...
cd ../core && npm test
cd ../vue && npm test
bash scripts/verify-contract.sh
bash scripts/check-pins.sh
golangci-lint run ./...
```

The smoke drivers in `core/smoke/` cover the live abuse-gate, attachment-validation, magic-link auth, and SSO paths. See `docs/self-hosting.md` for the production operator's checklist.

## Acknowledgements

Security researchers and contributors who have reported issues will be acknowledged here, with their permission, after the issue is publicly disclosed.

*No public disclosures yet — intake is pre-release.*
```

- [ ] **Step 2: Commit**

```bash
git add SECURITY.md
git commit -m "docs(7-iv): SECURITY.md — vuln-reporting policy + existing security stance"
```

---

### Task 4: Create `CONTRIBUTING.md`

**Files:** Create `CONTRIBUTING.md`

- [ ] **Step 1: Author the contributor guide**

Create `CONTRIBUTING.md`:

```markdown
# Contributing to intake

Thanks for your interest in contributing. This document covers how the project is laid out, how changes flow from idea to merge, and the local commands you should run before opening a pull request.

## Code of conduct

This project follows the Contributor Covenant 2.1 (TBD — pending maintainer adoption; until then, treat all contributors with professional respect). If you encounter unacceptable behavior, email the maintainer privately (see `SECURITY.md` for the contact placeholder).

## Project layout

```
intake/
├── core/                # @intake/core — shared TypeScript engine
├── vue/                 # @intake/vue — Vue 3 widget package
├── relay/               # intake-relay Go binary + internal packages
├── license-tool/        # maintainer-only license signer (not published)
├── schema/              # payload.v1.json — wire contract (source of truth)
├── examples/            # vue-anonymous, webhook-receiver, docker-compose
├── scripts/             # codegen-go.sh, verify-contract.sh, check-pins.sh
├── docs/                # operator-facing documentation + design specs
└── ai/                  # task plans, lessons, phase READMEs (developer notes)
```

See `docs/PROJECT.md` §14 for the canonical repo layout description, and `docs/PROJECT.md` §15 for the build & release pipeline overview.

## Branch and merge model

intake develops in long-lived **phase branches** that batch related changes into a single bundled merge to `main`:

- `main` — the integration branch. Always green: every Phase N merge passes the Phase N final smoke before merging.
- `phase-N` — the active development branch for phase N. Sub-plans are implemented as a sequence of commits on this branch (typically one or more commits per sub-plan).
- Smaller fixes that don't belong to an active phase go directly to `main` via a PR.

When a phase completes, the maintainer merges the phase branch into `main` with `git merge --no-ff phase-N` so the merge commit preserves the phase boundary for history.

## Phase-driven development model

Multi-step changes follow the phase model documented in `ai/PHASE_PLANNING.md`:

1. **Spec** — author a design doc under `docs/specs/YYYY-MM-DD-<title>.md` that captures goals, ADRs, scope, and the testing strategy. Use the existing specs (e.g. `2026-05-29-phase-7-release-ops-design.md`) as the template.
2. **Phase README** — author `ai/tasks/phase-N/README.md` with the spec link, ADR summary, sub-plan index, dependency graph, build-fail checklist, and final smoke definition.
3. **Sub-plans** — break the phase into 3–6 sub-plans under `ai/tasks/phase-N/<N-letter>-<title>-plan.md`. Each sub-plan has a Goal, Architecture, Tech Stack, Files Touched table, ordered Tasks with checkboxes, and a mandatory Smoke section.
4. **Smoke** — the final sub-plan (typically `N-v`) runs every smoke and records the evidence inline in the README.
5. **LESSONS** — append any novel patterns or surprises to `ai/LESSONS.md` as numbered L0XX entries.

This model exists because Phase 0f shipped silently-broken Auth0 IaC and Phase 0g spent ~24 hours debugging downstream symptoms. The phase READMEs and build-fail checklists exist to make every silent-failure mode loud.

## Commit conventions

intake uses Conventional Commits with a phase-scoped scope:

```
<type>(<scope>): <short subject>

<optional body>

Co-Authored-By: <coauthor-line>
```

| Type | Use |
|---|---|
| `feat` | New feature or capability. |
| `fix` | Bug fix. |
| `docs` | Documentation only. |
| `test` | Tests only. |
| `chore` | Build, tooling, dependencies, governance files. |
| `refactor` | Refactor with no behavior change. |

The `<scope>` is the active sub-plan when one applies (e.g. `feat(6-iii):`, `chore(7-iv):`). For standalone main-branch fixes, use a directory scope (e.g. `fix(relay):`, `docs(README):`).

Examples from the actual history:

```
feat(6-iii): chatwoot multipart attachments — JSON conv-create + multipart msg-create
chore(7-iv): LICENSE — verbatim Apache 2.0 + project copyright
fix(5-iv): raise abuse-driver budget to (150,150) so per-session fires before budget
docs(7-iv): COMMERCIAL.md DRAFT — paid-adapter open-core terms
```

## Local pre-commit commands

Before opening a PR, run each of these locally and confirm they pass. The `phase-N` branch will not merge until they all pass; CI runs the same set on every push.

### Go

```bash
cd relay
go build ./...                  # compile every package
go vet ./...                    # static analysis
go test -race ./...             # full unit suite with race detector
golangci-lint run ./...         # curated lint ruleset (Phase 7+)
```

### TypeScript (core + vue)

```bash
cd core
npm ci                          # clean install from package-lock.json
npm run type-check              # tsc --noEmit
npm run build                   # production bundle
npm test                        # vitest
npm run lint                    # eslint . (Phase 7+)

cd ../vue
npm ci
npm run type-check
npm run build
npm test
npm run lint
```

### Repo-wide

```bash
npx prettier --check .          # formatting (Phase 7+)
bash scripts/verify-contract.sh # schema and codegen drift check
bash scripts/check-pins.sh      # tool/module pin verification
```

`scripts/verify-contract.sh` regenerates the types from `schema/payload.v1.json` and diffs them against the checked-in `relay/internal/payload/types.go` and `core/src/types.ts`. Any drift fails the check.

`scripts/check-pins.sh` verifies every pinned tool (`goreleaser`, `golangci-lint`, `prettier`, `eslint`, etc.) and Go module (`prometheus/client_golang`, `golang-jwt`, `keyfunc/v3`, `golang.org/x/time`) is exact-pinned. Caret-versioning is forbidden per `ai/PHASE_PLANNING.md` §5.

## Running the demo locally

The fastest way to see the full stack in action:

```bash
cd examples/docker-compose
docker-compose up -d
# In a browser: open http://localhost:5173 (the vue widget)
# In a terminal: docker-compose logs -f webhook-receiver to watch tickets arrive
docker-compose down -v          # tear down + remove volumes when done
```

See `docs/quickstart.md` for a full walkthrough.

## Adding a new adapter

1. **Read an existing adapter** — `relay/internal/adapter/webhook/webhook.go` for the simplest shape; `relay/internal/adapter/chatwoot/chatwoot.go` for a two-call upload-then-create pattern; `relay/internal/adapter/zendesk/zendesk.go` for an upload-token pattern.
2. **Mirror the interface** — implement the frozen `adapter.Adapter` interface from `relay/internal/adapter/adapter.go`: `Name()`, `Configure(map[string]any) error`, `Capabilities() Capabilities`, `Create(ctx, payload) (*Result, error)`.
3. **Implement attachments** — if the downstream system accepts attachments, advertise the MIME types via `Capabilities().AcceptedMIMETypes` and forward them via the downstream's native upload mechanism. See `docs/attachments.md` for the per-adapter behavior contract.
4. **Add tests** — copy the test layout from an existing adapter package. Include `TestConfigure_*`, `TestCreate_*`, `TestAttachments_*` (when applicable), and an error-path test for each downstream failure mode.
5. **Register in main.go** — add the adapter to `buildRegistry` in `relay/cmd/relay/main.go`. After Phase 7-i, registration failures flow into the consolidated startup-problems slice (see `docs/self-hosting.md`).
6. **Document** — add a row to `docs/adapters.md` matrix and a config example to `docs/self-hosting.md`.
7. **Tier decision** — Free or Paid? See `docs/PROJECT.md` §13 for the tiering rationale. Paid adapters require a runtime license check via `intake/license`; free adapters do not.

## Pull request expectations

- **One sub-plan per PR (when in a phase).** PRs against `phase-N` typically implement exactly one sub-plan from `ai/tasks/phase-N/`. Cross-phase PRs are rare and need an explicit rationale.
- **Link the issue or task plan.** Every PR description should link the `ai/tasks/...` plan or GitHub Issue it implements.
- **Smoke evidence in the PR body.** Paste the command + output for the smoke that proves the PR works. Phase smokes include "what command was run" and "what output was observed" — replicate that shape in the PR body.
- **Tests added or modified for every behavior change.** A behavior change with no test diff is a red flag in review.
- **No schema or frozen-seam changes in non-seam phases.** The frozen seams (`adapter.Adapter` interface, `payload.IntakePayload` types, `schema/payload.v1.json`, `auth.Middleware.Handler` signature, the chi route shape, etc.) only change in seam sub-plans (the `-i` plan of each phase) and only with an explicit ADR in the phase design spec.
- **Conventional commits with HEREDOC.** Multi-line commit messages use a single-quoted heredoc for cross-platform compatibility (see existing history).
- **No `--no-verify`, no `--amend`** unless the maintainer asks. Pre-commit hooks exist for a reason; if a hook fails, fix the underlying issue and create a new commit.

## Reviewer expectations

- Read the linked plan or spec before the diff.
- Verify the smoke evidence is real (commands match what would actually run; outputs are not hand-edited).
- Check the build-fail checklist in the phase README — every item should still hold after the PR.
- Run the relevant smoke locally for high-risk changes.

## Getting help

- **Design questions** — open a GitHub Discussion (or file an Issue tagged `design`) before authoring a spec.
- **Implementation questions** — comment on the sub-plan file in `ai/tasks/` with a checkbox-level question, or open an Issue.
- **Security issues** — see `SECURITY.md`. Do NOT file a public Issue.

## License

By contributing to intake, you agree that your contributions are licensed under Apache 2.0 (the project's primary license — see `LICENSE`). You retain copyright to your contributions; the Apache 2.0 license grants the project (and downstream users) the rights needed to use, modify, and distribute them.

Contributions to the paid adapters (`zendesk`, `linear`) are also under Apache 2.0 — the commercial license model (see `COMMERCIAL.md`) gates *runtime use* in production, not contribution or modification of the source.
```

- [ ] **Step 2: Commit**

```bash
git add CONTRIBUTING.md
git commit -m "docs(7-iv): CONTRIBUTING.md — branch model, commit conventions, phase model, local commands"
```

---

### Task 5: Create `docs/license.md`

**Files:** Create `docs/license.md`

- [ ] **Step 1: Author the license model doc**

Create `docs/license.md`:

```markdown
# License model

intake uses a dual licensing model:

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

1. **`--license <path>`** — CLI flag passed to `intake-relay`.
2. **`INTAKE_LICENSE`** environment variable — inline contents of the license JSON (one-line; useful for container deployments).
3. **`INTAKE_LICENSE_FILE`** environment variable — path to a license file.
4. **Default paths**, tried in order:
   - `/etc/intake/license.json` (Linux/Unix production deployments)
   - `$XDG_CONFIG_HOME/intake/license.json` (XDG-compliant Linux desktops)
   - `os.UserConfigDir()/intake/license.json` (Linux: `~/.config/intake/`; macOS: `~/Library/Application Support/intake/`; Windows: `%AppData%/intake/`)

If none of the above is found, the relay enters **trial mode** (see below).

The license file path actually used is logged on startup at `slog.Info` level (without the contents).

## Trial mode

On the first startup with no license file resolved, the relay creates an installation-state file at `os.UserConfigDir()/intake/state.json`:

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
- **Paid adapters are disabled** — `zendesk` and `linear` are removed from the registry. If they were configured (`adapters.zendesk.enabled: true`), the relay logs one `slog.Warn` line per paid adapter: `license: trial expired; adapter \"zendesk\" disabled — see docs/license.md`.
- **Startup continues** — the consolidated startup-problems gate does NOT treat license-gate disablement as fatal. Free-mode is a valid operating state.
- **Routing rules referring to disabled adapters** — if `routing.default_adapter` or any `routing.rules[].to` references a disabled adapter, the relay logs an additional warning and falls back to the first enabled adapter for the affected rules.

The license gate is **fail-open in favor of availability**: a network outage or signature mismatch will never brick the relay. Free adapters always continue.

## License expiry

A loaded-but-expired license behaves the same as trial expiry: paid adapters are disabled with a warning, free adapters continue, the relay starts cleanly. The relay logs `license: <license_id> expired on <date>; paid adapters disabled` at `slog.Warn`.

## Installing a license key

### From a file

```bash
# Linux / production
sudo install -m 0640 -o intake -g intake my-license.json /etc/intake/license.json
sudo systemctl restart intake-relay
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
      INTAKE_LICENSE_FILE: /run/secrets/intake-license
    secrets:
      - intake-license

secrets:
  intake-license:
    file: ./my-license.json
```

### From the CLI flag

```bash
intake-relay --config /etc/intake/relay.yaml --license /etc/intake/license.json
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
- **Subject line:** "intake commercial license — \<your organization\>"

See `COMMERCIAL.md` for the (draft) terms. Note that `COMMERCIAL.md` is currently a draft pending legal review and does NOT constitute a binding offer.

## Troubleshooting

| Symptom | Cause | Resolution |
|---|---|---|
| `license: signature verification failed` on a license you just received | Public-key mismatch — your relay binary is from a release prior to your license. | Upgrade the relay to a release dated on or after your license issue date. |
| `adapter \"zendesk\" disabled — see docs/license.md` after installing a license | License `enabled_adapters` doesn't include `zendesk`. | Contact licensing; you may need a license re-issued with the correct adapter list. |
| Trial expired but you have a license file | `INTAKE_LICENSE_FILE` env var not set, or the default path doesn't contain the file. | Set `INTAKE_LICENSE_FILE=/path/to/your/license.json` and restart. Check the startup log to confirm the path was used. |
| `license: trial active` after installing a license | License file path not picked up. | Set `INTAKE_LICENSE_FILE` explicitly or place the file at `/etc/intake/license.json`. |

## See also

- `LICENSE` — Apache 2.0 text for the framework code.
- `COMMERCIAL.md` — draft commercial license terms for paid adapters.
- `docs/PROJECT.md` §12 — license model design rationale.
- `docs/PROJECT.md` §13 — free vs paid adapter matrix and the open-core pattern.
- `docs/adapters.md` — per-adapter setup, including which adapters require a license.
- `docs/self-hosting.md` — production deployment, including license file path management.
```

- [ ] **Step 2: Commit**

```bash
git add docs/license.md
git commit -m "docs(7-iv): docs/license.md — license-file resolution, trial mode, expiry behavior"
```

---

### Task 6: Create `docs/adapters.md`

**Files:** Create `docs/adapters.md`

- [ ] **Step 1: Author the adapter overview matrix**

Create `docs/adapters.md`. The Configure() keys MUST match the actual signatures verified in `relay/internal/adapter/{webhook,chatwoot,fider,linear,zendesk}/*.go`. The chatwoot section MUST reflect the post-Phase-6 3-call pattern (LESSONS L020).

```markdown
# Adapters

intake routes incoming submissions to a configurable adapter that creates the actual ticket / conversation / issue / post in a downstream system. v0 ships five adapters: three free, two paid. This document is an overview matrix only; for deep per-adapter API specifics, follow the link to each downstream system's own API documentation.

See also: `docs/self-hosting.md` for production config + secret management; `docs/license.md` for the paid-adapter tier; `docs/attachments.md` for the per-adapter attachment-forwarding behavior.

## Matrix at a glance

| Adapter | Tier | Purpose | Target API | Attachments? |
|---|---|---|---|---|
| [`webhook`](#webhook) | Free | Forward submission to an HTTP endpoint as JSON | Any HTTP receiver | Yes (pass-through) |
| [`chatwoot`](#chatwoot) | Free | Open a customer support conversation | Chatwoot Application API | Yes (multipart messages-create) |
| [`fider`](#fider) | Free | Post a feedback / feature-request idea | Fider HTTP API | Yes (markdown-embedded) |
| [`zendesk`](#zendesk) | **Paid** | Create a Zendesk support ticket | Zendesk Ticketing API v2 | Yes (uploads-then-ticket) |
| [`linear`](#linear) | **Paid** | Create a Linear engineering issue | Linear GraphQL API | Yes (asset-upload-then-issueCreate) |

All paid adapters require a license key in production (see `docs/license.md`); during the 14-day trial period, all adapters operate freely.

## Routing

The relay picks an adapter at submission time via the `routing:` config block. Default routing falls back to `routing.default_adapter`; per-classification rules override the default:

```yaml
routing:
  default_adapter: "webhook"
  rules:
    - when:
        classification: "bug"
      to: "linear"
    - when:
        classification: ["question", "other"]
      to: "chatwoot"
```

Classifications are produced by the LLM during `/v1/intake/turn` and surfaced in the final payload. See `docs/PROJECT.md` §8 for the routing semantics.

---

## webhook

**Tier:** Free
**Purpose:** Forward the canonical submission payload to an arbitrary HTTP endpoint as JSON. The simplest possible adapter — useful for piping into your own automation, a custom CRM, a Lambda, or a development webhook receiver.
**Target API:** Whatever HTTP endpoint you configure.

### Configuration

```yaml
adapters:
  webhook:
    enabled: true
    url: "https://example.com/intake-webhook"
    headers:
      X-Custom-Auth: "Bearer <token>"
      X-Tenant-ID: "acme"
    retry:
      max_attempts: 5
      backoff: "fixed"
```

### Required keys

- `url` — destination HTTP endpoint. The relay POSTs JSON to this URL.

### Optional keys

- `headers` — additional headers to include on every POST. Use this for auth tokens or tenant identifiers.
- `retry.max_attempts` — retry budget for 5xx responses (default `3`).
- `retry.backoff` — `fixed` or `exponential` (default `fixed`).

### Required environment variables

None. The webhook adapter does not resolve any secrets via env vars by default. If you put a secret in `headers`, you SHOULD use a `${ENV_VAR}` interpolation pattern documented in `docs/self-hosting.md` § secret management.

### Attachment behavior

The webhook adapter is JSON pass-through: every attachment in the canonical payload is serialized verbatim into the POST body via `json.Marshal(p)`. The receiver is responsible for decoding the `data:` URLs and persisting / forwarding the bytes. See `docs/attachments.md` for the canonical attachment shape.

### Notes

- No native upload mechanism — the entire payload (including any attachment `data:` URLs) goes into a single POST body. Watch your receiver's request-body limit; intake's default 14 MB cap (when attachments are enabled) is the practical upper bound for the request size.
- The `webhook-receiver` example in `examples/webhook-receiver/` is a minimal Node.js receiver suitable for the docker-compose demo and for local development.

### Downstream API documentation

N/A — your endpoint, your contract. Use the canonical payload shape from `schema/payload.v1.json`.

---

## chatwoot

**Tier:** Free
**Purpose:** Open a customer-support conversation in [Chatwoot](https://www.chatwoot.com/), routed to a configured inbox. Suitable for organizations already running Chatwoot for support, including the chatwoot.cloud SaaS.
**Target API:** Chatwoot Application API (the agent-side API; not the public widget API).

### Configuration

```yaml
adapters:
  chatwoot:
    enabled: true
    base_url: "https://app.chatwoot.com"           # or your self-hosted base URL
    account_id: 1                                   # your Chatwoot account ID
    inbox_id: 3                                     # the inbox to route conversations into
    api_token_env: "CHATWOOT_TOKEN"                 # env var holding your API token
```

### Required keys

- `base_url` — Chatwoot base URL (chatwoot.cloud or self-hosted).
- `account_id` — Chatwoot account ID.
- `inbox_id` — target inbox ID.
- `api_token` (resolved from `api_token_env`) — the API access token. Must have agent-level permissions on the target inbox.

### Required environment variables

- The env var named in `api_token_env` (default `CHATWOOT_TOKEN`) must be set to a valid Chatwoot API access token.

### Attachment behavior — 3-call flow (post-Phase 6)

When attachments are present, the chatwoot adapter performs **three** HTTP calls (LESSONS L020):

1. `POST /api/v1/accounts/{account_id}/contacts` — create or look up the contact for this submission (the existing Phase 3 two-call inheritance: contact must exist before a conversation can reference it via `contact_inbox`).
2. `POST /api/v1/accounts/{account_id}/conversations` — JSON body, conversation create. **MUST be JSON**, never multipart. Chatwoot's `ConversationsController#create` silently drops `attachments[]` multipart parts; the documented behavior is "attachments are uploaded on the separate `MessagesController#create` endpoint."
3. `POST /api/v1/accounts/{account_id}/conversations/{id}/messages` — `multipart/form-data` body carrying `content`, `message_type=outgoing`, and one `attachments[]` part per attachment.

When **no** attachments are present, only steps 1 and 2 run — byte-identical to the Phase 3 behavior. The multipart-vs-JSON branching is the key correctness invariant; mixing them in the conversation-create call results in the conversation being created without any attachment.

Failure modes:

- Step 1 failure → 502 to the widget, no conversation created.
- Step 2 failure → 502 to the widget, no conversation created.
- Step 3 failure → 502 to the widget, but the conversation **already exists** (no orphan-prevention — the conversation has the user text without the screenshot). The relay logs both the conversation ID and the failure reason at `slog.Error`.

### Notes

- The contact-then-conversation two-call shape (steps 1-2) was established in Phase 3 (LESSONS L011); the multipart message third call (step 3) was added in Phase 6 (LESSONS L020). The post-Phase 6 chatwoot adapter is the only adapter with a documented "JSON-then-multipart" branch inside a single submission.
- Chatwoot's agent-API token is sensitive — treat it the same way you would a database password. Rotate via the Chatwoot admin UI; the relay reads the env var at startup, so a rotation requires a relay restart.

### Downstream API documentation

- Chatwoot Application API: <https://www.chatwoot.com/developers/api/>
- API access tokens: <https://www.chatwoot.com/docs/product/channels/api/client-apis>
- Attachment-on-messages endpoint: see "Conversations > Messages > Create New Message" in the API reference.

---

## fider

**Tier:** Free
**Purpose:** Post a feedback or feature-request "idea" to [Fider](https://fider.io/), the open-source feedback portal. Suitable when you want public-facing feature voting or roadmap visibility.
**Target API:** Fider HTTP API (`/api/v1/posts`).

### Configuration

```yaml
adapters:
  fider:
    enabled: true
    base_url: "https://feedback.example.com"        # your Fider base URL
    api_key_env: "FIDER_API_KEY"                    # env var holding your API key
```

### Required keys

- `base_url` — Fider base URL.
- `api_key` (resolved from `api_key_env`) — Fider API key. Must have post-create permission.

### Required environment variables

- The env var named in `api_key_env` (default `FIDER_API_KEY`).

### Attachment behavior

Fider has no native file upload in its API. The fider adapter embeds attachments as **markdown image references** in the post description:

```
... user message text ...

![<label or "screenshot N">](data:image/png;base64,iVBORw0KGgo...)
```

Whether the markdown renders inline depends on the Fider deployment's content-security policy. Some Fider installs sanitize `data:` URLs; in that case the post still carries all conversation text (graceful degradation) but the screenshot is not visible. Operators wanting reliable screenshot rendering should choose a different adapter (chatwoot, linear, zendesk all have native file uploads).

Attachment labels are markdown-escaped before insertion (defense-in-depth against label-injection).

### Notes

- Fider's free-form post description makes the markdown-embed approach the simplest correct option; native file upload would require a Fider feature that doesn't exist.
- No additional HTTP roundtrips beyond the existing `POST /api/v1/posts`.

### Downstream API documentation

- Fider API: <https://docs.fider.io/api/>
- Self-hosting Fider: <https://docs.fider.io/self-hosted/>

---

## zendesk

**Tier:** **Paid** (requires commercial license — see `COMMERCIAL.md` and `docs/license.md`)
**Purpose:** Create a Zendesk support ticket. Suitable for organizations already running Zendesk for B2B / enterprise support.
**Target API:** Zendesk Ticketing API v2 (`/api/v2/tickets`).

### Configuration

```yaml
adapters:
  zendesk:
    enabled: true
    subdomain: "acme"                               # your-subdomain.zendesk.com
    email: "agent@acme.com"                         # agent email for basic auth
    api_token_env: "ZENDESK_API_TOKEN"              # env var holding the API token
    default_priority: "normal"                      # normal | low | high | urgent
```

### Required keys

- `subdomain` — your Zendesk subdomain (the relay constructs `https://<subdomain>.zendesk.com`).
- `email` — agent email used for basic auth (paired with the API token).
- `api_token` (resolved from `api_token_env`) — Zendesk API token.

### Optional keys

- `default_priority` — `low`, `normal`, `high`, or `urgent`. Default `normal`.

### Required environment variables

- The env var named in `api_token_env` (default `ZENDESK_API_TOKEN`).

### Attachment behavior — uploads-then-ticket

The zendesk adapter performs **N+1** HTTP calls when attachments are present:

1. For each attachment: `POST /api/v2/uploads.json` with the raw attachment bytes. The first response carries an `upload.token`. Subsequent uploads include `?token=<first-token>` so they all share the same upload token, per the Zendesk docs.
2. `POST /api/v2/tickets.json` with `ticket.comment.uploads: [<token>]`.

The upload calls happen **before** the ticket create — a failure in any upload returns an error before any ticket is created (orphan prevention).

Notes:

- Zendesk garbage-collects unattached uploads after **3 days**. A failed ticket-create after successful uploads leaves the uploads orphaned for that window.
- Upload transport errors are wrapped with `%w` and pass through the same redact-before-truncate sanitization as the ticket-create path (LESSONS L005).

### Notes

- Requires a commercial license in production after the 14-day trial expires. See `docs/license.md`.
- The API token is sensitive — treat it the same as a service-account password. Zendesk supports OAuth as an alternative; intake's v0 only implements basic auth + API token.

### Downstream API documentation

- Zendesk Tickets API: <https://developer.zendesk.com/api-reference/ticketing/tickets/tickets/>
- Zendesk Upload API: <https://developer.zendesk.com/api-reference/ticketing/tickets/ticket-attachments/>
- API token management: <https://support.zendesk.com/hc/en-us/articles/4408889192858>

---

## linear

**Tier:** **Paid** (requires commercial license — see `COMMERCIAL.md` and `docs/license.md`)
**Purpose:** Create a Linear engineering issue. Suitable when bug reports / feature requests should flow directly into the engineering team's issue tracker.
**Target API:** Linear GraphQL API (`https://api.linear.app/graphql`).

### Configuration

```yaml
adapters:
  linear:
    enabled: true
    api_key_env: "LINEAR_API_KEY"                   # env var holding the Linear API key
    team_id: "TEAM_ID_HERE"                         # target team's Linear ID (UUID format)
```

### Required keys

- `api_key` (resolved from `api_key_env`) — Linear personal API key or OAuth token with `write` scope on the target team.
- `team_id` — target team's Linear UUID. Find via Linear → Settings → API → "Your teams" or via the GraphQL `viewer { teams { nodes { id name } } }` query.

### Optional keys

- `endpoint` — GraphQL endpoint override (default `https://api.linear.app/graphql`; test seam only).
- `upload_endpoint` — upload endpoint override (default `https://api.linear.app/upload/file`; test seam only).

### Required environment variables

- The env var named in `api_key_env` (default `LINEAR_API_KEY`).

### Attachment behavior — upload-then-issueCreate

The linear adapter performs **N+1** HTTP calls when attachments are present:

1. For each attachment:
   - `POST <upload_endpoint>` — the `fileUpload` GraphQL mutation returns a signed PUT URL.
   - `PUT <signed-url>` — upload the raw attachment bytes to the signed URL.
   - The upload response's `success` field is checked explicitly; `success: false` rejects before any issue is created (LESSONS L023).
2. `mutation issueCreate(...)` — references each attachment via `attachmentLinks` carrying the asset URLs returned in step 1.

The uploads happen **before** the issue create — a failure in any upload returns an error before any issue is created (orphan prevention; same shape as zendesk).

### Notes

- Requires a commercial license in production after the 14-day trial expires. See `docs/license.md`.
- Linear's `attachmentLinks` accept any URL; the linear adapter uses the asset URLs returned by `fileUpload`. If you need to reference an external asset, the schema permits it but the adapter doesn't expose that path in v0.

### Downstream API documentation

- Linear GraphQL API: <https://developers.linear.app/docs/graphql/working-with-the-graphql-api>
- Linear API keys: <https://developers.linear.app/docs/graphql/working-with-the-graphql-api#personal-api-keys>
- File upload mutation: <https://developers.linear.app/docs/graphql/working-with-the-graphql-api#uploading-files>
- Issue creation: <https://developers.linear.app/docs/graphql/working-with-the-graphql-api#creating-issues>

---

## Adapter selection guidance

| Use case | Recommended adapter |
|---|---|
| Already on Chatwoot for support | `chatwoot` |
| Already on Zendesk for support | `zendesk` (paid) |
| Engineering bug tracking | `linear` (paid) |
| Feature request portal with voting | `fider` |
| Custom automation or CRM | `webhook` |
| Multi-adapter routing (bugs → linear, support → chatwoot) | configure both, use `routing.rules` |

## Extending — adding a new adapter

The adapter interface (`relay/internal/adapter/adapter.go`) is a frozen seam. Adding a new adapter follows the pattern documented in `CONTRIBUTING.md` § "Adding a new adapter":

1. Read an existing adapter package as the template.
2. Implement `Name()`, `Configure(map[string]any) error`, `Capabilities() Capabilities`, `Create(ctx, payload) (*Result, error)`.
3. Add the adapter to `buildRegistry` in `relay/cmd/relay/main.go`.
4. Tier the adapter: free (Apache 2.0 only) or paid (gated via `intake/license`).
5. Document it here and in `docs/self-hosting.md`.

Per-adapter deep documentation (custom field mapping, multi-team routing, complex auth flows) is deferred to v1+ — `docs/adapters.md` stays an overview matrix in v0.
```

- [ ] **Step 2: Commit**

```bash
git add docs/adapters.md
git commit -m "docs(7-iv): docs/adapters.md — 5-adapter overview matrix (chatwoot 3-call post-L020)"
```

---

### Task 7: Create `docs/quickstart.md`

**Files:** Create `docs/quickstart.md`

- [ ] **Step 1: Author the 30-minute onboarding path**

Create `docs/quickstart.md`:

```markdown
# Quickstart

This guide gets you from a fresh clone to "I just submitted a ticket and saw it land in a webhook log" in about 30 minutes. After this, see `docs/self-hosting.md` for production configuration, `docs/license.md` for the free/paid tier model, and `docs/adapters.md` for setting up real downstream systems like Chatwoot or Zendesk.

## Prerequisites

You need **either**:

- **Docker** — Docker Desktop on macOS / Windows, or the Docker engine on Linux. This is the recommended path; the demo stack uses docker-compose and brings up everything you need.

**OR**, for the bare-metal path:

- **Go 1.23.2** — for building the relay
- **Node 24.12.0** — for the widget tooling and codegen (`nvm use` if you use nvm)
- **POSIX shell** — Git Bash or WSL on Windows; required by `scripts/codegen-go.sh`, `scripts/verify-contract.sh`, `scripts/check-pins.sh`

A real LLM API key is **not** required for the quickstart — the demo stack uses a `fake-llm` stub that emits canned SSE responses. You can wire in Anthropic, OpenAI, Gemini, or Ollama later; see `docs/self-hosting.md` § LLM providers.

## The 60-second path (docker-compose)

```bash
git clone <repo-url>
cd intake/examples/docker-compose
docker-compose up -d

# Wait ~10 seconds for the stack to come up, then:
curl -s -X POST http://localhost:18080/v1/intake/init -d '{}' | jq
```

You should see a JSON response with a `session_id` and a `capabilities` block. The relay is up.

Now submit a ticket:

```bash
SESSION=$(curl -s -X POST http://localhost:18080/v1/intake/init -d '{}' | jq -r .session_id)

curl -s -X POST http://localhost:18080/v1/intake/submit \
  -H "Content-Type: application/json" \
  -H "X-Intake-Session: $SESSION" \
  -d '{
    "messages": [{"role":"user","content":"quickstart test ticket"}],
    "client": {
      "url":"http://quickstart.test/",
      "user_agent":"curl",
      "language":"en-US",
      "viewport_width":1024,
      "viewport_height":768,
      "referrer":"",
      "page_title":"quickstart"
    },
    "user_claims": {},
    "context": {"dom_snippet":""}
  }'
```

The relay responds with `{"external_id":"...","external_url":"..."}`. The submission was forwarded to the webhook receiver in the docker-compose stack. Verify it landed:

```bash
docker-compose logs webhook-receiver | tail -50
```

You should see the canonical submission payload logged — messages, client metadata, user claims, and the (empty) attachments array. **You just ran the full intake stack end-to-end.**

When you're done:

```bash
docker-compose down -v
```

## What just happened?

The docker-compose stack ran three services:

- **relay** — the `intake-relay` Go binary, listening on port `18080` (HTTP) and `19090` (Prometheus metrics).
- **fake-llm** — a stub LLM server on port `11434` that emits canned SSE responses. The relay's `/v1/intake/turn` endpoint uses this in place of a real Anthropic / OpenAI / Gemini / Ollama call.
- **webhook-receiver** — a tiny Node.js HTTP server on port `19099` that logs every POST it receives. The relay's `webhook` adapter forwards submissions here.

Your `curl` calls drove the canonical flow:

1. **`POST /v1/intake/init`** — the widget (or, in your case, `curl`) asks the relay for a session ID and the server's published capabilities. The capabilities include the enabled auth modes, the streaming flag, and (when attachments are enabled) the per-attachment + aggregate size caps and the allowed MIME types.
2. **`POST /v1/intake/turn`** — *(skipped in this quickstart)* the widget streams a turn through the LLM. The relay handles classification, summarization, and follow-up question generation.
3. **`POST /v1/intake/submit`** — the widget posts the final canonical payload. The relay validates against `schema/payload.v1.json`, runs Phase 5 abuse gates (per-IP, per-session, budget), runs Phase 6 attachment validation, picks an adapter from `routing:`, and calls the adapter's `Create()` method. The adapter (in this case `webhook`) forwards the payload to the configured URL — the docker-compose stack's webhook-receiver service.

The `external_id` and `external_url` in the response come from the downstream system; the webhook-receiver returns a stub but a real adapter (chatwoot, zendesk, linear) returns the actual conversation / ticket / issue identifier.

## The bare-metal path

If you don't want Docker:

```bash
git clone <repo-url>
cd intake

# Install dependencies and run codegen
npm ci
npm run codegen                    # regenerate types from schema/payload.v1.json

# Build the relay
cd relay
go build -o intake-relay ./cmd/relay
cd ..

# Start the fake-llm in one terminal:
go run ./relay/cmd/fake-llm --addr :11434

# Start the webhook-receiver in another terminal:
node examples/webhook-receiver/server.mjs    # listens on :19099

# Start the relay in a third terminal, using the quickstart config:
./relay/intake-relay --config examples/docker-compose/config.yaml
```

The relay's config (used by docker-compose too) routes submissions to the webhook adapter on `http://127.0.0.1:19099/intake`. Drive it with the same `curl` commands as the 60-second path above.

## Trying the Vue widget

The bare-metal path lets you exercise the actual widget UI:

```bash
cd examples/vue-anonymous
npm install
npm run dev
```

Open `http://localhost:5173` in a browser. Click the widget bubble in the corner, type a message, and submit. The relay (running in the third terminal) processes the request through `/init` → `/turn` → `/submit`, and the webhook-receiver logs the result.

To attach a screenshot:

1. Click **Attach** in the widget panel.
2. The screenshot redactor opens, capturing the current page via `html2canvas`.
3. Draw black rectangles over any region you want to redact.
4. Click **Save**. The attachment appears in the thumbnail strip.
5. Click **Submit**. The relay validates the attachment (size, MIME, magic bytes) and forwards it to the webhook receiver as a `data:` URL inside the canonical payload.

See `docs/attachments.md` for the full attachment behavior — validation errors, per-adapter forwarding, and the redaction UI.

## Next steps

You now have a working intake stack. Where to go next depends on what you want to do:

- **Production-deploy intake** — read `docs/self-hosting.md`. Covers binary deployment via systemd, Docker deployment, reverse-proxy + TLS, env-var management, secret resolution, the Phase 5 abuse gates, the Phase 6 attachments config, and the Phase 4 auth modes.
- **Set up a real downstream system** (Chatwoot, Zendesk, Linear, Fider) — read `docs/adapters.md`. Per-adapter config keys, env vars, attachment behavior, and links to each downstream's own API docs.
- **Understand the licensing** — read `docs/license.md`. The framework is Apache 2.0; the `zendesk` and `linear` adapters are paid, with a 14-day trial. `COMMERCIAL.md` has the (draft) commercial terms.
- **Wire in a real LLM** — `docs/self-hosting.md` § LLM providers covers the four providers (Anthropic, OpenAI, Gemini, Ollama) and their config blocks. The fake-llm we used here is for development only.
- **Embed the widget in your own app** — see the `examples/vue-anonymous/` source and `vue/src/components/IntakeWidget.vue` as the reference embedding.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `docker-compose up` fails with "port already allocated" | Another process is using `18080`, `19090`, `19099`, or `11434` | Stop the conflicting process, or remap ports in `examples/docker-compose/docker-compose.yml` |
| `curl: (52) Empty reply from server` | Relay still starting | Wait 5-10 seconds; the relay logs `relay listening on :18080` when it's ready (`docker-compose logs relay`) |
| `webhook-receiver` log is empty after `/submit` | Adapter routing misconfigured | Check `docker-compose logs relay` for `adapter \"webhook\" called: ...`; verify `routing.default_adapter: \"webhook\"` in `examples/docker-compose/config.yaml` |
| `{"error":{"code":"attachments_disabled",...}}` on a submit with attachments | Attachments are off in this config | The quickstart's docker-compose config has attachments enabled by default; if you've edited the config, set `attachments.enabled: true` and restart |
| `/v1/intake/init` returns no `capabilities.attachments` block | Attachments are off, or the intersected MIME allowlist is empty | See `docs/attachments.md` § Capabilities discovery |
| `curl` works but the Vue widget shows a CORS error | Relay's `server.cors_origins` doesn't include the widget origin | Add the widget's origin (`http://localhost:5173`) to `server.cors_origins` in the relay config |

For deeper issues, see `docs/self-hosting.md` § Logging — the relay emits structured JSON logs to stdout with a `level` field; grep for `"level":"ERROR"` to find startup or runtime failures.

## See also

- `docs/self-hosting.md` — production deployment, env vars, metrics, abuse gates, auth modes.
- `docs/adapters.md` — per-adapter setup for Chatwoot, Zendesk, Linear, Fider, webhook.
- `docs/license.md` — license-file resolution, trial mode, paid-adapter gate.
- `docs/attachments.md` — attachment validation, per-adapter forwarding, the widget redactor UI.
- `docs/PROJECT.md` — the source-of-truth design document.
- `CONTRIBUTING.md` — how to contribute, how phases work, local pre-commit commands.
```

- [ ] **Step 2: Commit**

```bash
git add docs/quickstart.md
git commit -m "docs(7-iv): docs/quickstart.md — 30-minute onboarding path + 'what just happened' explanation"
```

---

### Task 8: Create `docs/self-hosting.md`

**Files:** Create `docs/self-hosting.md`

- [ ] **Step 1: Author the production operator reference**

Create `docs/self-hosting.md`. This is the largest doc in 7-iv — covers every Phase 4/5/6/7 operator config knob. Use the verified `sample.yaml` shape from `relay/internal/config/testdata/sample.yaml` as the canonical source.

```markdown
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
    allow_without_captcha: false          # true ⇒ no captcha required for anonymous submit
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
| `intake_llm_tokens_total` | counter | `provider`, `direction` | LLM tokens consumed; `direction` ∈ `{input, output}` |
| `intake_adapter_calls_total` | counter | `adapter`, `result` | Adapter `Create()` invocations; `result` ∈ `{success, error}` |

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

- Every entry MUST be a valid CIDR block. Invalid CIDRs are caught by the consolidated startup gate (`server.trusted_proxies contains an invalid CIDR \"not-a-cidr\"`).
- Requests from non-trusted addresses ignore any `X-Forwarded-For` header — the connection's `RemoteAddr` wins. This prevents IP spoofing from outside the trusted network.
- Set to an empty list `[]` if intake is directly internet-facing (no proxy). The per-IP rate limiter will use the connection's `RemoteAddr` directly.

Behind a public CDN (Cloudflare, Fastly, etc.), include the CDN's published IP ranges. Refresh those ranges periodically — the relay does not auto-update them.

## Logging

The relay emits structured JSON logs to stdout via `slog`. One line per event; `level` ∈ `DEBUG`, `INFO`, `WARN`, `ERROR`.

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
```

- [ ] **Step 2: Commit**

```bash
git add docs/self-hosting.md
git commit -m "docs(7-iv): docs/self-hosting.md — production deployment, env vars, metrics, abuse gates, auth modes"
```

---

### Task 9: Rewrite `README.md`

**Files:** Modify `README.md` (rewrite from 28 lines to ~120 lines)

- [ ] **Step 1: Rewrite the top-level README**

The current README pre-dates Phase 3 and reads like a scratch note. Replace its content entirely. Keep the working-name disclaimer (Q1 final-name lock is post-7-iv). Lead with what intake IS, the canonical demo, the docs links, and the build instructions.

Overwrite `README.md`:

```markdown
# intake

> **Working name** — final name TBD (see `docs/specs/2026-05-26-v0-decomposition-and-phasing-design.md` §6).

<!-- Status badges — fill in post-public-release:
[![CI](https://github.com/<org>/<repo>/actions/workflows/ci.yml/badge.svg)](https://github.com/<org>/<repo>/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/<org>/<repo>)](https://github.com/<org>/<repo>/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
-->

**intake** is an AI-native, self-hostable feedback & support intake stack: an embeddable Vue 3 widget + a single-binary Go relay. No SaaS dependency, no vendor lock-in, no third-party data plane — install the relay on your own infrastructure, drop the widget into your app, and route submissions into the support system you already use (Chatwoot, Zendesk, Linear, Fider, or any HTTP webhook).

## The 60-second demo

```bash
git clone <repo-url>
cd intake/examples/docker-compose
docker-compose up -d
```

That starts the relay, a fake LLM, and a webhook receiver. Submit a test ticket via `curl` (see `docs/quickstart.md`) or open `http://localhost:5173` in a browser for the Vue widget UI.

## What's in v0

- **5 adapters** — pick which downstream system receives your tickets
  - `webhook` — *(Free)* — POST canonical JSON to any HTTP endpoint
  - `chatwoot` — *(Free)* — open a Chatwoot conversation with the agent API
  - `fider` — *(Free)* — post a Fider idea with markdown-embedded screenshots
  - `zendesk` — *(Paid)* — create a Zendesk ticket via the v2 API
  - `linear` — *(Paid)* — create a Linear issue via the GraphQL API
- **4 LLM providers** — pick whichever you're already using
  - `anthropic` — Claude family models (default in production deployments)
  - `openai` — GPT-4o family and successors
  - `gemini` — Google Gemini family
  - `ollama` — self-hosted local models (no API cost)
- **3 authentication modes** — pick the right shape for your user model
  - `anonymous` — no auth, CAPTCHA-gated; for public marketing-site widgets
  - `email` — magic-link auth via SMTP; for known users with light identity needs
  - `sso` — JWKS or HS256 JWT verification; for SSO-backed customer portals

Plus: AI-driven classification + summarization, screenshot capture with client-side redaction, attachment upload (PNG/JPEG/WebP, 5 MB each / 10 MB aggregate), Phase 5 abuse gates (per-IP / per-session / daily LLM budget), Cloudflare Turnstile CAPTCHA, Prometheus metrics on an opt-in side-channel, consolidated startup-gate that flags every misconfig in one log line.

## Documentation

| Doc | Purpose |
|---|---|
| [`docs/quickstart.md`](docs/quickstart.md) | Fresh-clone to "ticket in webhook log" in 30 minutes. Docker or bare-metal. |
| [`docs/self-hosting.md`](docs/self-hosting.md) | Production deployment: binary + Docker, env vars, metrics, abuse gates, auth modes, TLS. |
| [`docs/license.md`](docs/license.md) | License-file resolution, 14-day trial, paid-adapter gate, expiry behavior. |
| [`docs/adapters.md`](docs/adapters.md) | The 5 adapters: tier, config, env vars, attachment behavior, downstream API links. |
| [`docs/attachments.md`](docs/attachments.md) | Attachment validation, per-adapter forwarding, the widget redactor UI. |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Branch model, commit conventions, the phase model, local pre-commit commands. |
| [`SECURITY.md`](SECURITY.md) | Vulnerability reporting policy + existing security stance. |
| [`docs/PROJECT.md`](docs/PROJECT.md) | Source-of-truth design document for the whole project. |

## License

intake uses a dual licensing model:

- **Apache 2.0** covers the framework — the relay, the widget, the schema, the free adapters (`webhook`, `chatwoot`, `fider`), and all LLM providers. See [`LICENSE`](LICENSE).
- **Commercial license** is required to operate the paid adapters (`zendesk`, `linear`) in production after the 14-day trial. See [`COMMERCIAL.md`](COMMERCIAL.md) for (draft) terms.

The source code is Apache 2.0 either way — the commercial gate is at runtime, not at distribution. You can read, fork, and modify everything; you need a license to **use** the paid adapters in production.

## Repo layout

```
intake/
├── core/                # @intake/core — shared TypeScript engine (capture, client, types)
├── vue/                 # @intake/vue — Vue 3 widget components
├── relay/               # intake-relay Go binary + internal packages
├── license-tool/        # maintainer-only license signer (not published)
├── schema/              # payload.v1.json — wire contract (source of truth)
├── examples/            # vue-anonymous, webhook-receiver, docker-compose
├── scripts/             # codegen-go.sh, verify-contract.sh, check-pins.sh
├── docs/                # operator-facing docs + design specs
└── ai/                  # task plans, lessons, phase READMEs (developer notes)
```

## Prerequisites

- **Node 24.12.0** (run `nvm use` if you use nvm)
- **Go 1.23.2**
- **POSIX shell** (Git Bash or WSL on Windows) for `scripts/codegen-go.sh` and friends

For the demo: **Docker Desktop** (macOS / Windows) or **Docker engine** (Linux).

## Build

```bash
npm ci                              # install workspace dependencies
npm run codegen                     # regenerate types from schema/payload.v1.json
cd relay && go build ./...          # compile the relay and all internal packages
```

To run the full local pre-commit suite (matches CI):

```bash
cd relay && go vet ./... && go test -race ./...
cd ../core && npm test
cd ../vue && npm test
bash scripts/verify-contract.sh
bash scripts/check-pins.sh
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full developer workflow.

## Status

intake is **pre-1.0**. The v0 wire contract is locked (`schema/payload.v1.json`), but the public release infrastructure is still local-only — see `docs/PROJECT.md` §15 for the release-pipeline status. Pin to specific commits if you depend on intake in production today; semver guarantees begin at v1.0.0.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs(7-iv): README.md rewrite — what intake IS, demo cmd, links table, build instructions"
```

---

### Task 10: Cross-reference verification

**Files:** none new (verification pass over Tasks 1-9 output)

This task is a final sweep to confirm every internal link resolves to a file that exists by end-of-7-iv, every `*_env` key in `docs/self-hosting.md` matches the canonical `sample.yaml`, and the chatwoot 3-call note in `docs/adapters.md` matches the post-L020 pattern.

- [ ] **Step 1: Internal link resolution check**

Every Markdown link inside the new docs should resolve to either:

- A file authored in 7-iv (`docs/quickstart.md`, `docs/self-hosting.md`, `docs/license.md`, `docs/adapters.md`, `LICENSE`, `COMMERCIAL.md`, `SECURITY.md`, `CONTRIBUTING.md`, `README.md`).
- A pre-existing file (`docs/attachments.md`, `docs/PROJECT.md`, `ai/LESSONS.md`, `ai/PHASE_PLANNING.md`, the design specs under `docs/specs/`).
- An external URL (Apache 2.0 license, downstream API docs, etc.).

Run a quick scan:

```bash
cd c:/src/ai/intake

# List every markdown link target referenced from the new docs
for f in docs/quickstart.md docs/self-hosting.md docs/license.md docs/adapters.md \
         README.md LICENSE CONTRIBUTING.md SECURITY.md COMMERCIAL.md; do
  echo "=== $f ==="
  grep -oE '\]\(([^)]+)\)' "$f" | sed 's/](\(.*\))/\1/' || true
done | sort -u
```

For every relative path emitted, confirm it exists. For every `https://` URL, spot-check that the canonical reference is current (apache.org License, Chatwoot/Zendesk/Linear/Fider API docs).

- [ ] **Step 2: Env-var matrix consistency**

The env var table in `docs/self-hosting.md` must list every `*_env` field present in `relay/internal/config/testdata/sample.yaml`. Diff manually:

```bash
grep -E '_env:' relay/internal/config/testdata/sample.yaml
```

Every key in the output should appear as a row in the `docs/self-hosting.md` § "Full env var reference" table. If a key is missing, add it; if a documented key is no longer in `sample.yaml`, remove it.

- [ ] **Step 3: Adapter config-key consistency**

For each adapter (`webhook`, `chatwoot`, `fider`, `zendesk`, `linear`), the config block in `docs/adapters.md` must match the actual `Configure()` keys read in `relay/internal/adapter/<name>/<name>.go`. Spot-check each:

- `webhook`: `url` (required), `headers` (optional), `retry.max_attempts`, `retry.backoff`.
- `chatwoot`: `base_url`, `account_id`, `inbox_id`, `api_token` (resolved from `api_token_env`).
- `fider`: `base_url`, `api_key` (resolved from `api_key_env`).
- `zendesk`: `subdomain`, `email`, `api_token` (resolved from `api_token_env`), `default_priority` (optional).
- `linear`: `api_key` (resolved from `api_key_env`), `team_id`, `endpoint` (optional), `upload_endpoint` (optional).

The chatwoot section MUST document the 3-call flow when attachments are present (per LESSONS L020): contact-create → JSON conversation-create → multipart messages-create. The text in `docs/adapters.md` § chatwoot already encodes this; verify the wording is consistent with `docs/attachments.md` § Per-adapter behavior.

- [ ] **Step 4: LICENSE / COMMERCIAL.md / docs/license.md consistency**

The free vs paid matrix is restated in three places:

- `LICENSE` — Apache 2.0 only; no tier discussion.
- `COMMERCIAL.md` — lists paid adapters (`zendesk`, `linear`).
- `docs/license.md` — adapter tier matrix.
- `docs/adapters.md` — adapter tier column.
- `README.md` — paid vs free list.

Every list MUST agree: `zendesk` and `linear` are paid; `webhook`, `chatwoot`, `fider` are free. Walk through each file and confirm.

- [ ] **Step 5: Phase 7 README cross-link**

Add a single line to `ai/tasks/phase-7/README.md` § Sub-plan index updating 7-iv's status from "Not started" to "Plan authored" (the actual execution status is updated by the implementer running this plan). This is optional — the README is updated for real by the executor as they progress through the plan.

- [ ] **Step 6: Final commit**

If any drift was found and fixed during this task, commit the fixes:

```bash
git add docs/ README.md COMMERCIAL.md SECURITY.md CONTRIBUTING.md LICENSE
git commit -m "docs(7-iv): cross-reference sweep — links resolve, env-var matrix matches sample.yaml"
```

If everything was already consistent (the desired outcome of Tasks 1-9), this task is a no-op except for the verification log.

---

## Smoke (mandatory per PHASE_PLANNING §7)

Sub-plan 7-iv's smoke is a documentation-walkthrough proof, since the docs themselves are the deliverable. The Phase 7 final smoke (in 7-v) covers the broader release smoke; 7-iv's smoke is the docs-specific subset.

### Self-runnable smoke items

- [ ] **S1: All 9 new/modified files exist**

```bash
cd c:/src/ai/intake
for f in LICENSE COMMERCIAL.md SECURITY.md CONTRIBUTING.md README.md \
         docs/license.md docs/adapters.md docs/quickstart.md docs/self-hosting.md; do
  test -f "$f" && echo "OK: $f" || { echo "FAIL: $f missing"; exit 1; }
done
```

Expected: all 9 lines say `OK`.

- [ ] **S2: Each new doc is non-trivial (within the target line ranges)**

```bash
wc -l LICENSE COMMERCIAL.md SECURITY.md CONTRIBUTING.md README.md \
      docs/license.md docs/adapters.md docs/quickstart.md docs/self-hosting.md
```

Expected line counts (approximate, per spec):

- `LICENSE` — ~200 lines (verbatim Apache 2.0)
- `COMMERCIAL.md` — 80–150 lines
- `SECURITY.md` — 80–150 lines
- `CONTRIBUTING.md` — 200–350 lines
- `README.md` — 80–150 lines
- `docs/license.md` — 150–250 lines
- `docs/adapters.md` — 250–400 lines
- `docs/quickstart.md` — 150–250 lines
- `docs/self-hosting.md` — 400–600 lines

A file outside its target range is not a hard fail — but is worth re-reading to confirm it's not padded or truncated.

- [ ] **S3: Apache 2.0 text is verbatim (spot-check sentinel phrases)**

```bash
grep -q "Licensed under the Apache License, Version 2.0" LICENSE && echo "OK: appendix" || echo "FAIL"
grep -q "TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION" LICENSE && echo "OK: header" || echo "FAIL"
grep -q "WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND" LICENSE && echo "OK: warranty disclaimer" || echo "FAIL"
grep -q "Copyright 2026 The intake authors" LICENSE && echo "OK: project copyright" || echo "FAIL"
```

Expected: all four `OK` lines.

- [ ] **S4: COMMERCIAL.md DRAFT banner is present**

```bash
grep -q "DRAFT — legal review required before v0.1.0 public release" COMMERCIAL.md && echo "OK" || echo "FAIL"
```

- [ ] **S5: Adapter tier consistency across docs**

```bash
# Free adapters should never appear under "paid" anywhere
for free in webhook chatwoot fider; do
  if grep -E "(^|\s)\*\*$free\*\*" docs/adapters.md | grep -qi "paid"; then
    echo "FAIL: $free shown as paid in docs/adapters.md"
    exit 1
  fi
done

# Paid adapters should appear with the Paid tag in every reference
for paid in zendesk linear; do
  grep -q "Paid" docs/adapters.md && grep -q "$paid" docs/adapters.md && echo "OK: $paid documented" \
    || { echo "FAIL: $paid not properly marked"; exit 1; }
done
```

- [ ] **S6: Chatwoot 3-call note (L020) present**

```bash
grep -q "3-call" docs/adapters.md && echo "OK: 3-call mentioned in adapters.md" || echo "FAIL"
grep -q "multipart" docs/adapters.md && echo "OK: multipart documented" || echo "FAIL"
grep -q "messages" docs/adapters.md && echo "OK: messages endpoint documented" || echo "FAIL"
```

- [ ] **S7: Internal link spot-check (no broken relative links)**

For each of the new docs, every Markdown relative link (`](path)` where path doesn't start with `http`) MUST resolve to a file present in the tree. Sample:

```bash
for f in docs/quickstart.md docs/self-hosting.md docs/license.md docs/adapters.md \
         README.md CONTRIBUTING.md SECURITY.md COMMERCIAL.md; do
  echo "=== $f ==="
  grep -oE '\]\(([^)h][^)]*|h[^t][^)]*)\)' "$f" | sed 's/](\(.*\))/\1/' | while read target; do
    # Strip anchor fragments and resolve relative to file's directory
    base=$(echo "$target" | sed 's/#.*//')
    [ -z "$base" ] && continue
    dir=$(dirname "$f")
    full="$dir/$base"
    test -f "$full" || test -d "$full" && echo "  OK: $target" || echo "  MISS: $target"
  done
done
```

A `MISS:` line on a target that doesn't exist yet is a fail unless it's a placeholder (e.g. badges) or a file authored in a later sub-plan.

- [ ] **S8: Quickstart manual walkthrough (in 7-v's final smoke)**

The actual end-to-end "follow quickstart.md in a fresh directory and reach 'ticket in webhook log' in 30 minutes" check is item 6 of the Phase 7 final smoke (Phase 7 README §7). 7-iv's smoke ends at the static-content checks above; 7-v's smoke does the dynamic walkthrough.

### Recording evidence

Save the output of each smoke step (S1-S7) to a scratch file and transcribe into the Phase 7 README §7 evidence section during 7-v. 7-iv itself doesn't update the Phase 7 README evidence block — that happens in 7-v alongside the other final-smoke evidence.

---

## Build-fail discipline

Specific to this sub-plan:

- [ ] Any new doc smaller than half its lower target → **Fail** (likely a truncated paste).
- [ ] Any new doc with `TODO` / `FIXME` / `XXX` left in the prose → **Fail** (drafts are not deliverables; explicit placeholders for env vars / contact emails are OK and clearly marked).
- [ ] `LICENSE` text differs from the canonical apache.org Apache 2.0 text (other than the copyright appendix) → **Fail**.
- [ ] `COMMERCIAL.md` missing the `DRAFT — legal review required` banner → **Fail**.
- [ ] Any internal Markdown link resolves to a file that does NOT exist by end-of-7-iv → **Fail**.
- [ ] `docs/self-hosting.md` env-var matrix lists a key not in `relay/internal/config/testdata/sample.yaml`, or omits a key that IS in sample.yaml → **Fail**.
- [ ] `docs/adapters.md` documents a config key for an adapter that the adapter's `Configure()` does NOT read → **Fail**.
- [ ] `docs/adapters.md` chatwoot section does not document the 3-call flow with multipart messages-create → **Fail**.
- [ ] `README.md` retains the original 28-line scratch-note structure → **Fail** (the rewrite is the deliverable).
- [ ] Adapter tier disagreement across `README.md`, `docs/adapters.md`, `docs/license.md`, `COMMERCIAL.md` → **Fail**.

---

## Done criteria

- All 9 files exist and are within their target line ranges (S2).
- LICENSE is verbatim Apache 2.0 (S3).
- COMMERCIAL.md carries the legal-review banner (S4).
- Every internal Markdown link resolves (S7).
- The env-var matrix in `docs/self-hosting.md` matches `sample.yaml` (Task 10 Step 2).
- The adapter config-keys in `docs/adapters.md` match the actual `Configure()` keys (Task 10 Step 3).
- The chatwoot 3-call (post-L020) flow is documented in `docs/adapters.md` (S6).
- All commits land on the `phase-7` branch (no push).
- No production code, no schema, no frozen-seam files modified.

---

## Notes

- L010 (PS 5.1 BOM): not applicable in 7-iv — no YAML files written by this sub-plan, all output is Markdown. Implementer should still write files via tools that don't add a BOM (Edit / Write tools, NOT raw `Set-Content -Encoding utf8` from PS 5.1).
- Stay on branch `phase-7`. Do NOT push to any remote in this sub-plan.
- 7-iv runs in parallel with 7-iii under subagent-driven-development. File territories are disjoint (docs in 7-iv vs. `examples/docker-compose/` in 7-iii); the only shared touchpoint is that `docs/quickstart.md` references the demo command `docker-compose up -d` — that command is the responsibility of 7-iii but the doc can be authored from the spec without the demo existing yet.
- Phase 7 design spec §15 lists two PROJECT.md inconsistencies that could be folded in opportunistically during 7-iv. They are explicitly deferred — out of scope for 7-iv unless the executor has extra time. If folded in, add a commit `docs(7-iv): PROJECT.md §14+§15 cleanup (Phase 7 design spec §15)`.
- Trial mode + license file resolution claims in `docs/license.md` must match the `intake/license` package's actual behavior (Phase 3 frozen). If discrepancies are found during cross-reference (Task 10), they are fixed by adjusting the doc to match the code, NOT the other way around.
- The Q1 final-name lock will eventually replace "intake" with the chosen name across all these docs + LICENSE + COMMERCIAL.md + README.md. That replacement is a separate post-7-iv action; the working name stands for now.
