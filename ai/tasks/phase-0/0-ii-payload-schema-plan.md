# Sub-plan 0-ii — payload.v1.json Schema

## 1. Goal

Author `schema/payload.v1.json` — the canonical JSON Schema (draft 2020-12) for the widget→relay wire contract, faithful to PROJECT.md §4. Add a known-good and a known-bad sample payload as test fixtures, and an `ajv` validation step that proves the schema accepts the good one and rejects the bad one. This schema is the input every generator (0-iii) and validator consumes.

## 2. Design references

- Payload structure: [docs/PROJECT.md](../../../docs/PROJECT.md) §4
- Strict top-level fields / reject-unknown: PROJECT.md §17 ("reject unknown top-level fields in v1.0")
- Additive-tolerance nuance resolved below (Task 1 note)

## 3. Files touched

| File | Create/Modify | Why |
|---|---|---|
| `schema/payload.v1.json` | Create | the canonical wire contract |
| `schema/testdata/payload.valid.json` | Create | known-good fixture for the accept-case smoke |
| `schema/testdata/payload.invalid.json` | Create | known-bad fixture for the reject-case smoke |
| `package.json` (root) | Modify | add `ajv-cli` devDep + `validate-schema` script |

## 4. Tasks

- [ ] **Step 1: Author the schema**

Design note baked into the schema: PROJECT.md §17 requires rejecting **unknown top-level fields**, so the root sets `additionalProperties: false`. The free-form maps (`user.custom`, `context.app_context`, `context.page_metadata`) intentionally allow extra keys (`additionalProperties: true`) — that is where host-app-specific data lives.

Create `schema/payload.v1.json`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://intake.dev/schema/payload.v1.json",
  "title": "IntakePayload",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema_version", "submission", "client", "user", "conversation"],
  "properties": {
    "schema_version": { "const": "1.0" },
    "submission": {
      "type": "object",
      "additionalProperties": false,
      "required": ["id", "submitted_at"],
      "properties": {
        "id": { "type": "string", "format": "uuid" },
        "submitted_at": { "type": "string", "format": "date-time" },
        "tenant_id": { "type": ["string", "null"] }
      }
    },
    "client": {
      "type": "object",
      "additionalProperties": false,
      "required": ["widget_version", "session_id", "url", "user_agent", "viewport", "locale"],
      "properties": {
        "widget_version": { "type": "string" },
        "session_id": { "type": "string", "format": "uuid" },
        "url": { "type": "string", "format": "uri" },
        "referrer": { "type": ["string", "null"] },
        "user_agent": { "type": "string" },
        "viewport": {
          "type": "object",
          "additionalProperties": false,
          "required": ["w", "h"],
          "properties": {
            "w": { "type": "integer", "minimum": 0 },
            "h": { "type": "integer", "minimum": 0 }
          }
        },
        "locale": { "type": "string" }
      }
    },
    "user": {
      "type": "object",
      "additionalProperties": false,
      "required": ["auth_mode", "verified"],
      "properties": {
        "auth_mode": { "enum": ["anonymous", "email", "sso"] },
        "id": { "type": ["string", "null"] },
        "email": { "type": ["string", "null"] },
        "display_name": { "type": ["string", "null"] },
        "verified": { "type": "boolean" },
        "custom": { "type": "object", "additionalProperties": true }
      }
    },
    "conversation": {
      "type": "object",
      "additionalProperties": false,
      "required": ["messages", "summary", "title_suggestion", "classification", "severity_guess", "tags_suggested", "language"],
      "properties": {
        "messages": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["role", "content", "ts"],
            "properties": {
              "role": { "enum": ["user", "assistant"] },
              "content": { "type": "string" },
              "ts": { "type": "string", "format": "date-time" }
            }
          }
        },
        "summary": { "type": "string" },
        "title_suggestion": { "type": "string", "maxLength": 80 },
        "classification": { "enum": ["bug", "feature_request", "question", "other"] },
        "severity_guess": { "enum": ["low", "medium", "high", "critical", "unknown"] },
        "tags_suggested": { "type": "array", "items": { "type": "string" } },
        "language": { "type": "string" }
      }
    },
    "context": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "app_context": { "type": "object", "additionalProperties": true },
        "page_metadata": { "type": "object", "additionalProperties": true }
      }
    },
    "attachments": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["type", "mime_type", "size_bytes", "url"],
        "properties": {
          "type": { "enum": ["screenshot", "file"] },
          "mime_type": { "type": "string" },
          "size_bytes": { "type": "integer", "minimum": 0 },
          "url": { "type": "string" },
          "label": { "type": "string" }
        }
      }
    },
    "routing_hint": { "type": ["string", "null"] }
  }
}
```

- [ ] **Step 2: Write the known-good fixture (the failing test, accept-case)**

Create `schema/testdata/payload.valid.json`:

```json
{
  "schema_version": "1.0",
  "submission": {
    "id": "3f9a2b7c-1d4e-4a6b-8c2d-9e0f1a2b3c4d",
    "submitted_at": "2026-05-26T12:00:00Z",
    "tenant_id": null
  },
  "client": {
    "widget_version": "0.0.0",
    "session_id": "7c1e2d3f-4a5b-4c6d-8e9f-0a1b2c3d4e5f",
    "url": "https://app.example.com/dashboard",
    "referrer": null,
    "user_agent": "Mozilla/5.0",
    "viewport": { "w": 1440, "h": 900 },
    "locale": "en-US"
  },
  "user": {
    "auth_mode": "anonymous",
    "id": null,
    "email": null,
    "display_name": null,
    "verified": false,
    "custom": {}
  },
  "conversation": {
    "messages": [
      { "role": "user", "content": "The export button does nothing.", "ts": "2026-05-26T11:59:00Z" },
      { "role": "assistant", "content": "Which page were you on?", "ts": "2026-05-26T11:59:05Z" }
    ],
    "summary": "Export button unresponsive on dashboard.",
    "title_suggestion": "Export button does nothing on dashboard",
    "classification": "bug",
    "severity_guess": "medium",
    "tags_suggested": ["export", "dashboard"],
    "language": "en"
  },
  "context": {
    "app_context": { "plan": "pro" },
    "page_metadata": { "title": "Dashboard" }
  },
  "attachments": [],
  "routing_hint": null
}
```

- [ ] **Step 3: Write the known-bad fixture (reject-case)**

Violates three rules: `schema_version` wrong const, `user.auth_mode` not in enum, and an unknown top-level key `evil`.

Create `schema/testdata/payload.invalid.json`:

```json
{
  "schema_version": "9.9",
  "submission": {
    "id": "3f9a2b7c-1d4e-4a6b-8c2d-9e0f1a2b3c4d",
    "submitted_at": "2026-05-26T12:00:00Z"
  },
  "client": {
    "widget_version": "0.0.0",
    "session_id": "7c1e2d3f-4a5b-4c6d-8e9f-0a1b2c3d4e5f",
    "url": "https://app.example.com/dashboard",
    "user_agent": "Mozilla/5.0",
    "viewport": { "w": 1440, "h": 900 },
    "locale": "en-US"
  },
  "user": {
    "auth_mode": "superuser",
    "verified": false
  },
  "conversation": {
    "messages": [],
    "summary": "",
    "title_suggestion": "",
    "classification": "bug",
    "severity_guess": "medium",
    "tags_suggested": [],
    "language": "en"
  },
  "evil": true
}
```

- [ ] **Step 4: Add ajv to the workspace and a validate script**

Modify root `package.json` — add to `devDependencies` and `scripts`:

```json
{
  "devDependencies": {
    "ajv-cli": "5.0.0"
  },
  "scripts": {
    "validate-schema": "ajv validate --spec=draft2020 -s schema/payload.v1.json -d schema/testdata/payload.valid.json"
  }
}
```

Then run:

```bash
npm install
```

Expected: `ajv-cli@5.0.0` installed.

- [ ] **Step 5: Run the accept-case — verify it passes**

Run:

```bash
npx ajv validate --spec=draft2020 -c ajv-formats -s schema/payload.v1.json -d schema/testdata/payload.valid.json
```

Expected: prints `schema/testdata/payload.valid.json valid` and exits 0.

> Note: `-c ajv-formats` enables `format` keywords (uuid/date-time/uri). If `ajv-formats` is not auto-resolved, add `"ajv-formats": "3.0.1"` to root devDependencies and `npm install` before re-running.

- [ ] **Step 6: Run the reject-case — verify it fails**

Run:

```bash
npx ajv validate --spec=draft2020 -c ajv-formats -s schema/payload.v1.json -d schema/testdata/payload.invalid.json
```

Expected: prints validation errors (const mismatch on `schema_version`, enum failure on `auth_mode`, additionalProperties failure on `evil`) and exits non-zero.

- [ ] **Step 7: Commit**

```bash
git add schema package.json package-lock.json
git commit -m "feat(phase-0): payload.v1.json schema + valid/invalid fixtures + ajv validation"
```

## 5. Smoke

```
1. Pre-condition: 0-i merged; npm install run; ajv-cli@5.0.0 + ajv-formats present.
2. Execution: run the Step 5 accept-case command, then the Step 6 reject-case command.
3. Verification: accept-case exits 0 and reports "valid"; reject-case exits non-zero and reports errors for schema_version const, auth_mode enum, and the unknown top-level `evil` key.
4. Teardown / repeat: no state; re-runnable.
```

## 6. Done criteria

- [ ] `schema/payload.v1.json` covers every field in PROJECT.md §4 with correct types/enums/formats.
- [ ] Top-level `additionalProperties: false`; free-form maps (`custom`, `app_context`, `page_metadata`) allow extra keys.
- [ ] Accept-case fixture validates; reject-case fixture fails — smoke (§5) passes.
- [ ] `validate-schema` script present in root `package.json`.
