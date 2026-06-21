# Commercial License

This document sets out the commercial license terms for the paid adapters in
the OpenIntake project. It is the operative agreement governing production use
of those adapters unless superseded by a separately executed written contract
between you and the Licensor. We recommend you review it with your own legal
counsel before purchase.

- **Licensor:** Mantichor LLC, an Arizona limited liability company ("Licensor").
- **Contact:** `info@mantichor.com`
- **Last updated:** 2026-06-21

## What requires a commercial license

The **code** in this repository is licensed under Apache 2.0 (see `LICENSE`). You
may read, fork, modify, and redistribute the source code freely under those terms.

The **use** of certain adapter functionality in production deployments requires a
paid commercial license. Specifically, the following adapters are gated at runtime
by the OpenIntake license-key check:

- `zendesk` — Zendesk ticketing adapter
- `linear` — Linear issue adapter

The following adapters and components are **always free** under Apache 2.0:

- `webhook`, `chatwoot`, `fider` adapters
- The relay binary and source code
- The `@openintake/core` and `@openintake/vue` widget packages
- The schema, codegen pipeline, and developer tooling
- All four LLM providers (`anthropic`, `openai`, `gemini`, `ollama`)
- All three authentication modes (`anonymous`, `email`, `sso`)

This pattern is sometimes called "open core" — the source is fully open, and the
commercial license gates only production *use* of specific adapters. See Sentry
and GitLab for established precedent, and `docs/PROJECT.md` §13 for the rationale.

## What "use" means

A "use" of a paid adapter is one in which:

- the relay process is configured with `adapters.<paid-adapter>.enabled: true`, AND
- the relay process successfully invokes that adapter's `Create()` method against a
  real downstream system in production.

Compiling, running, testing, and debugging the paid adapters against test fixtures
or sandbox environments is permitted under Apache 2.0 — you do not need a license to
develop, contribute, or evaluate. You need a license to **operate** a paid adapter
against a production downstream system.

## Trial period

New installations of OpenIntake automatically enable a 14-day trial during which all
paid adapters operate fully without a license key. The trial begins on the first
relay startup that resolves an installation-state file at
`os.UserConfigDir()/openintake/state.json`. Once the trial expires:

- Free adapters (`webhook`, `chatwoot`, `fider`) continue to operate.
- Paid adapters (`zendesk`, `linear`) are disabled. The relay starts cleanly, logs a
  warning at `slog.Warn` level for each paid adapter that was configured but is now
  license-gated, and routes traffic only through the enabled free adapters.

See `docs/license.md` for the full trial state machine.

## Pricing

A commercial license is **$1,500 USD per year**, which includes:

- Production use of both paid adapters (`zendesk` and `linear`),
- Up to **3 production relay instances** running those paid adapters, and
- The right to operate for the duration of the one-year license term.

Additional production relay instances beyond the included three are **$400 USD per
instance, per year**.

Prices are effective as of the "Last updated" date above and may change for new and
renewing licenses; the price of an active license is honored through its current
term. All free adapters and components remain free under Apache 2.0 regardless of
license status — you pay only for production use of the paid adapters.

### Enterprise and custom deployments

Deployments that need more than three production relay instances, air-gapped or
otherwise restricted operation, or a bundled support agreement are licensed under
custom enterprise terms. Contact `info@mantichor.com` for an enterprise quote.

## License scope

The unit of licensing is the **production relay instance**. The license file issued
to you encodes the number of production relay instances you have licensed in its
`max_relay_instances` field. Operating the paid adapters on more concurrent
production relay instances than you have licensed is outside the scope of your
license. Non-production instances (development, testing, staging, and evaluation)
are not counted and remain free under Apache 2.0.

## Support

A commercial license grants the right to operate the paid adapters; it does **not**
include a support or service-level commitment. Support engagements — including
response-time SLAs, priority issue handling, or deployment assistance — are
optional, separately negotiated, and separately priced. Absent a separate support
agreement, all users (free and commercial) receive community support through the
project's public issue tracker on the same terms.

## License grant

Subject to your timely payment of the applicable license fees and your compliance
with these terms (or the terms of a separately executed commercial agreement), the
Licensor grants you a non-exclusive, non-transferable, non-sublicensable license to
operate the paid adapters in production deployments, up to the number of production
relay instances licensed, for a term of **one (1) year** from the license issue date.
Licenses are renewable for successive one-year terms upon renewal payment.

This commercial license:

- Does NOT modify the Apache 2.0 license under which the underlying code is distributed.
- Does NOT grant any right to redistribute, sublicense, or otherwise transfer the
  paid-adapter operating rights to any third party.
- Terminates upon expiration of the license term, non-renewal, or material breach.

Upon termination or expiration, you must cease operating the paid adapters in any
production deployment. The relay's runtime license check enforces this gate
automatically — you do not need to remove the code; configuring the affected
adapters as disabled is sufficient.

## Source-available vs. open-source

The OpenIntake project intentionally distributes the paid-adapter source code under
Apache 2.0 — not a source-available or otherwise restrictive license. This is deliberate:

- You can audit every line of code that runs in your environment.
- You can build, modify, and run the paid adapters against test fixtures without limitation.
- You can fork the project entirely under Apache 2.0 terms.

The commercial license is a **deployment-time runtime gate on production use**, not a
code-distribution restriction. This open-core model is used by Sentry, GitLab, and
similar projects.

## Warranty disclaimer

Even with a paid commercial license, the software is provided "AS IS" per the Apache
2.0 warranty disclaimer in `LICENSE` Section 7. Commercial support agreements, if any,
are separately negotiated and are not implied by the existence of a runtime license.

## Audit

The Licensor reserves the right, upon reasonable prior written notice and no more than
once per calendar year, to audit your use of the paid adapters to verify license
compliance. Any audit will be conducted during normal business hours, in a manner that
minimizes disruption to your operations, and subject to reasonable protection of your
confidential information.

## Governing law

This commercial license is governed by and construed in accordance with the laws of
the State of Arizona, United States, without regard to its conflict-of-laws principles.

## How to obtain a license

Contact `info@mantichor.com` with:

- **Subject:** "OpenIntake commercial license — \<your organization\>"
- Your organization's legal name and approximate deployment scale (relay instances,
  tickets per month),
- Which paid adapters you need (`zendesk`, `linear`, or both), and
- Any support requirements, if applicable.

## See also

- `LICENSE` — Apache 2.0, governing the code itself.
- `docs/license.md` — how the runtime license check, trial, and load order work.
- `docs/PROJECT.md` §12–§13 — the license model and the free/paid adapter matrix.
