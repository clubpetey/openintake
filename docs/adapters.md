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

None. The webhook adapter does not resolve any secrets via env vars by default. The `headers` map values are static strings — NOT env-var-interpolated by the relay. Pre-render the config file externally (e.g., with `envsubst`) if you need env-driven values. See `docs/self-hosting.md` § "Secret management patterns" for the recommended deployment patterns.

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
