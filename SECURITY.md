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
