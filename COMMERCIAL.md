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
