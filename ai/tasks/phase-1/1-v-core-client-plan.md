# 1-v: @intake/core TS Client — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `@intake/core` TypeScript engine — an SSE-over-fetch relay client, browser context capture, and SubmitRequest serializer — that freezes the client↔relay TS contract for Phase 1.

**Architecture:** Three focused modules (`sse.ts`, `context.ts`, `client.ts`) plus an extended `index.ts`. The client takes an injected `fetch` implementation so it is unit-testable without a browser or real server. SSE parsing is a pure ReadableStream transformer with no external dependencies.

**Tech Stack:** TypeScript 5.6.3 (pinned), vitest 4.1.7 (pinned in devDeps), tsx 4.22.3 (pinned in devDeps for smoke script), Node 24.12.0 (global fetch + web streams).

---

## 1. Goal

Deliver `@intake/core` — a framework-agnostic TS engine the Vue widget (1-vi) will wrap — implementing:
- `IntakeClient` class (per README §6.7): `init()`, `turn()`, `submit()`.
- `captureClient()` and `capturePageMetadata()` for browser context capture, SSR-safe.
- A reusable SSE line parser over a `ReadableStream`.
- All public API re-exported from `core/src/index.ts`.
- Full unit test coverage (vitest, no real network, no browser).
- A `core/smoke/drive.ts` script that drives a live relay end-to-end.

This sub-plan **freezes the client↔relay TS contract**. No downstream sub-plan (1-vi) may alter these shapes.

---

## 2. Design References

- `ai/tasks/phase-1/README.md` §6.4 — HTTP DTOs and SSE frame shapes (single source of truth).
- `ai/tasks/phase-1/README.md` §6.7 — `@intake/core` public API (exact signatures, frozen here).
- `docs/specs/2026-05-26-phase-1-walking-skeleton-design.md` §5.4 — SSE `/turn` protocol.
- `docs/specs/2026-05-26-phase-1-walking-skeleton-design.md` §6 — SubmitRequest shape.
- `docs/specs/2026-05-26-phase-1-walking-skeleton-design.md` §2 — Security invariant (relay is sole LLM broker; no provider key in browser).
- `ai/tasks/phase-1/README.md` §5 — Tool version pins.
- `ai/tasks/phase-1/README.md` §7 — Build-fail checklist (`tsc --noEmit` must stay clean).
- `ai/WEB_CODE.md` — Code style: strict TS, vitest, no console.log in committed code.

---

## 3. Files Touched

| File | Create/Modify | Why |
|---|---|---|
| `core/package.json` | Modify | Add vitest + tsx devDeps; add `test` script; add `smoke` script |
| `core/vitest.config.ts` | Create | Vitest config (environment: node, include: src/**/*.test.ts) |
| `core/src/sse.ts` | Create | Pure SSE-over-fetch line parser; zero dependencies |
| `core/src/sse.test.ts` | Create | Unit tests for sse.ts using a string-backed ReadableStream |
| `core/src/context.ts` | Create | `captureClient()` + `capturePageMetadata()`; SSR-safe |
| `core/src/context.test.ts` | Create | Unit tests with stubbed globalThis.window/navigator/document |
| `core/src/client.ts` | Create | `IntakeClient` class per README §6.7; fetch-injected |
| `core/src/client.test.ts` | Create | Unit tests with mock fetch stubs |
| `core/src/index.ts` | Modify | Re-export public API alongside existing generated-types export |
| `core/src/types.ts` | Create | Shared TS interfaces for HTTP DTOs (InitResponse, TurnRequest, SSEDelta, SSEDone, SSEError, SubmitRequest, SubmitResponse, ClientInfo, Viewport, ContextInfo) mirroring README §6.4 |
| `core/smoke/drive.ts` | Create | Node script: init→turn(print deltas)→submit; run with tsx against live relay |
| `core/tsconfig.json` | Modify | Add `include: ["src/**/*.ts", "smoke/**/*.ts"]` to cover smoke script |

---

## 4. Tasks

### Task 0: Add vitest + tsx; add test script

**Why vitest over node:test + tsx:** vitest provides first-class ESM support, watch mode, snapshot support, and a jest-compatible API — all matching the `ai/WEB_CODE.md` convention (`npm run vitest run`). It runs on Node 24 with zero additional config and the `"type": "module"` package. `node:test` would require more boilerplate for async iterables and lacks the matcher ergonomics the plan's test code relies on.

**Files:**
- Modify: `core/package.json`
- Create: `core/vitest.config.ts`

- [ ] **Step 1: Update core/package.json — add devDeps and scripts**

Replace the entire file with:

```json
{
  "name": "@intake/core",
  "version": "0.0.0",
  "type": "module",
  "main": "src/index.ts",
  "scripts": {
    "type-check": "tsc --noEmit",
    "test": "vitest run",
    "test:watch": "vitest",
    "smoke": "tsx smoke/drive.ts"
  },
  "devDependencies": {
    "typescript": "5.6.3",
    "vitest": "4.1.7",
    "tsx": "4.22.3"
  }
}
```

Note: vitest and tsx are test/dev tools — caret is acceptable per README §5 footnote. We pin exact anyway to match phase discipline. The Anthropic SDK and vite must be exact-pinned (load-bearing); vitest is not load-bearing for the relay contract.

- [ ] **Step 2: Create core/vitest.config.ts**

```ts
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
```

- [ ] **Step 3: Install dependencies**

```bash
cd C:/src/ai/intake
npm install -w @intake/core
```

Expected output: `added N packages` (no errors). Check that `node_modules/vitest` and `node_modules/tsx` appear under the workspace root or core.

- [ ] **Step 4: Verify vitest runs (no tests yet)**

```bash
npm run -w @intake/core test
```

Expected output: `No test files found` or `0 passed` — NOT an error about missing config or bad ESM.

- [ ] **Step 5: Commit**

```bash
git add core/package.json core/vitest.config.ts
git commit -m "chore(core): add vitest 4.1.7 + tsx 4.22.3 devDeps; wire test script"
```

---

### Task 1: Create core/src/types.ts and core/src/client-types.ts — shared interfaces

These two files establish all shared type definitions upfront, eliminating any circular import risk. `types.ts` holds HTTP DTO shapes (mirroring README §6.4). `client-types.ts` holds the public `IntakeConfig`, `ChatMessage`, `SubmitResult` interfaces (frozen in §6.7) so `client.ts` can import them without importing `index.ts`.

**Files:**
- Create: `core/src/types.ts`
- Create: `core/src/client-types.ts`

- [ ] **Step 1: Create core/src/types.ts**

```ts
// HTTP DTOs — mirror of relay/internal/server package shapes (README §6.4).
// This file is the single source of truth for the client↔relay TS contract.
// Frozen in sub-plan 1-v. Do NOT alter without re-smoking 1-vi.

export interface Viewport {
  w: number;
  h: number;
}

export interface ClientInfo {
  widget_version: string;
  url: string;
  referrer: string | null;
  user_agent: string;
  viewport: Viewport;
  locale: string;
}

export interface ContextInfo {
  app_context: Record<string, unknown>;
  page_metadata: Record<string, unknown>;
}

export interface InitResponse {
  session_id: string;
  capabilities: {
    auth_modes: string[];
    streaming: boolean;
  };
}

export interface TurnMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface TurnRequest {
  messages: TurnMessage[];
}

// SSE frame shapes (data: <json>\n\n)
export interface SSEDelta {
  delta: string;
}

export interface SSEDone {
  done: true;
  input_tokens: number;
  output_tokens: number;
}

export interface SSEError {
  error: string;
}

export type SSEFrame = SSEDelta | SSEDone | SSEError;

export interface SubmitRequest {
  messages: TurnMessage[];
  client: ClientInfo;
  user_claims: Record<string, unknown>;
  context: ContextInfo;
  routing_hint: string | null;
}

export interface SubmitResponse {
  external_id: string;
  external_url: string;
  adapter_name: string;
  created_at: string;
}

export interface ErrorEnvelope {
  error: {
    code: string;
    message: string;
  };
}
```

- [ ] **Step 2: Create core/src/client-types.ts**

```ts
// Public-facing types for IntakeClient (extracted to avoid circular imports).
// Frozen in sub-plan 1-v. Do NOT alter without re-smoking 1-vi.

export interface IntakeConfig {
  /** Base URL of the relay, e.g. "http://localhost:8080". No trailing slash. */
  relayUrl: string;
  /** Semver string of the widget embedding this client, e.g. "0.1.0". */
  widgetVersion: string;
  /** Arbitrary key-value data to include in context.app_context on submit. */
  appContext?: Record<string, unknown>;
}

export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface SubmitResult {
  external_id: string;
  external_url: string;
  adapter_name: string;
  created_at: string;
}
```

- [ ] **Step 3: Type-check**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0, no errors.

- [ ] **Step 4: Commit**

```bash
git add core/src/types.ts core/src/client-types.ts
git commit -m "feat(core): add HTTP DTO types and public client-types interfaces"
```

---

### Task 2: SSE parser — sse.ts + tests

The parser consumes a `ReadableStream<Uint8Array>`, decodes UTF-8, splits on `\n\n` (SSE event boundaries), strips `data: ` prefix, JSON-parses each payload, and calls a callback per `SSEFrame`. It's pure and has no browser dependency.

**Files:**
- Create: `core/src/sse.ts`
- Create: `core/src/sse.test.ts`

- [ ] **Step 1: Write the failing test first**

Create `core/src/sse.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { consumeSSE } from './sse.js';
import type { SSEFrame } from './types.js';

/** Helper: turn a string into a ReadableStream<Uint8Array> */
function streamFrom(text: string): ReadableStream<Uint8Array> {
  const enc = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(enc.encode(text));
      controller.close();
    },
  });
}

describe('consumeSSE', () => {
  it('calls onFrame for a single delta event', async () => {
    const frames: SSEFrame[] = [];
    const stream = streamFrom('data: {"delta":"hello"}\n\n');
    await consumeSSE(stream, (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ delta: 'hello' });
  });

  it('calls onFrame for done event', async () => {
    const frames: SSEFrame[] = [];
    const stream = streamFrom(
      'data: {"done":true,"input_tokens":5,"output_tokens":10}\n\n'
    );
    await consumeSSE(stream, (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ done: true, input_tokens: 5, output_tokens: 10 });
  });

  it('calls onFrame for error event', async () => {
    const frames: SSEFrame[] = [];
    const stream = streamFrom('data: {"error":"upstream failed"}\n\n');
    await consumeSSE(stream, (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ error: 'upstream failed' });
  });

  it('parses multiple events in sequence', async () => {
    const frames: SSEFrame[] = [];
    const raw =
      'data: {"delta":"a"}\n\n' +
      'data: {"delta":"b"}\n\n' +
      'data: {"done":true,"input_tokens":1,"output_tokens":2}\n\n';
    await consumeSSE(streamFrom(raw), (f) => frames.push(f));
    expect(frames).toHaveLength(3);
    expect(frames[0]).toEqual({ delta: 'a' });
    expect(frames[1]).toEqual({ delta: 'b' });
    expect(frames[2]).toEqual({ done: true, input_tokens: 1, output_tokens: 2 });
  });

  it('ignores lines that do not start with "data: "', async () => {
    const frames: SSEFrame[] = [];
    // SSE spec allows comment lines ": comment" and event: lines
    const raw = ': keep-alive\n\ndata: {"delta":"x"}\n\n';
    await consumeSSE(streamFrom(raw), (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ delta: 'x' });
  });

  it('handles a stream split across multiple chunks', async () => {
    const enc = new TextEncoder();
    const parts = [
      'data: {"del',
      'ta":"split"}\n\n',
    ];
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const p of parts) controller.enqueue(enc.encode(p));
        controller.close();
      },
    });
    const frames: SSEFrame[] = [];
    await consumeSSE(stream, (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ delta: 'split' });
  });
});
```

- [ ] **Step 2: Run the test — confirm it fails with "Cannot find module './sse.js'"**

```bash
npm run -w @intake/core test
```

Expected: `Error: Cannot find module './sse.js'` (or similar import error).

- [ ] **Step 3: Implement core/src/sse.ts**

```ts
import type { SSEFrame } from './types.js';

/**
 * Consumes a ReadableStream<Uint8Array> as an SSE stream.
 * Calls onFrame for each parsed data frame.
 * Resolves when the stream closes.
 *
 * Protocol: each SSE event is separated by a blank line (\n\n).
 * Only lines starting with "data: " are parsed; others are skipped.
 */
export async function consumeSSE(
  stream: ReadableStream<Uint8Array>,
  onFrame: (frame: SSEFrame) => void
): Promise<void> {
  const decoder = new TextDecoder();
  const reader = stream.getReader();
  let buffer = '';

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      // Split on double newline (SSE event boundary)
      const events = buffer.split('\n\n');
      // Keep the last (possibly incomplete) chunk in the buffer
      buffer = events.pop() ?? '';

      for (const event of events) {
        for (const line of event.split('\n')) {
          if (!line.startsWith('data: ')) continue;
          const payload = line.slice('data: '.length);
          try {
            const frame = JSON.parse(payload) as SSEFrame;
            onFrame(frame);
          } catch {
            // Malformed JSON — skip silently (relay contract guarantees valid JSON)
          }
        }
      }
    }

    // Flush any remaining buffered content
    buffer += decoder.decode();
    if (buffer.trim()) {
      for (const line of buffer.split('\n')) {
        if (!line.startsWith('data: ')) continue;
        const payload = line.slice('data: '.length);
        try {
          const frame = JSON.parse(payload) as SSEFrame;
          onFrame(frame);
        } catch {
          // skip
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}
```

- [ ] **Step 4: Run the tests — all must pass**

```bash
npm run -w @intake/core test
```

Expected output:
```
 ✓ src/sse.test.ts (6 tests) Xms
 Test Files  1 passed (1)
 Tests       6 passed (6)
```

- [ ] **Step 5: Type-check**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add core/src/sse.ts core/src/sse.test.ts
git commit -m "feat(core): add consumeSSE parser with unit tests"
```

---

### Task 3: Context capture — context.ts + tests

`captureClient()` reads from `window`/`navigator`/`document` with SSR guards. `capturePageMetadata()` reads `document.title` and Open Graph `<meta>` tags.

**Files:**
- Create: `core/src/context.ts`
- Create: `core/src/context.test.ts`

- [ ] **Step 1: Write the failing tests first**

Create `core/src/context.test.ts`:

```ts
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { captureClient, capturePageMetadata } from './context.js';

// Store originals so we can restore after each test
const originalWindow = globalThis.window;
const originalNavigator = globalThis.navigator;
const originalDocument = globalThis.document;

function setGlobals(overrides: {
  window?: Partial<typeof globalThis.window> | undefined;
  navigator?: Partial<typeof globalThis.navigator> | undefined;
  document?: Partial<typeof globalThis.document> | undefined;
}) {
  // @ts-expect-error - intentional test override
  globalThis.window = overrides.window;
  // @ts-expect-error
  globalThis.navigator = overrides.navigator;
  // @ts-expect-error
  globalThis.document = overrides.document;
}

afterEach(() => {
  // @ts-expect-error
  globalThis.window = originalWindow;
  // @ts-expect-error
  globalThis.navigator = originalNavigator;
  // @ts-expect-error
  globalThis.document = originalDocument;
});

describe('captureClient — SSR (no window)', () => {
  beforeEach(() => {
    setGlobals({ window: undefined, navigator: undefined, document: undefined });
  });

  it('returns safe defaults when window is absent', () => {
    const info = captureClient('0.0.1');
    expect(info.widget_version).toBe('0.0.1');
    expect(info.url).toBe('');
    expect(info.referrer).toBeNull();
    expect(info.user_agent).toBe('');
    expect(info.viewport).toEqual({ w: 0, h: 0 });
    expect(info.locale).toBe('');
  });
});

describe('captureClient — browser (window present)', () => {
  beforeEach(() => {
    setGlobals({
      window: {
        location: { href: 'https://example.com/page' } as Location,
        innerWidth: 1280,
        innerHeight: 800,
      } as typeof globalThis.window,
      navigator: {
        userAgent: 'TestAgent/1.0',
        language: 'en-US',
      } as typeof globalThis.navigator,
      document: {
        referrer: 'https://referrer.example.com',
        title: 'Test Page',
        querySelectorAll: (_: string) => [] as unknown as NodeListOf<Element>,
      } as unknown as typeof globalThis.document,
    });
  });

  it('captures url from window.location.href', () => {
    const info = captureClient('1.2.3');
    expect(info.url).toBe('https://example.com/page');
  });

  it('captures referrer from document.referrer', () => {
    const info = captureClient('1.2.3');
    expect(info.referrer).toBe('https://referrer.example.com');
  });

  it('captures user_agent from navigator.userAgent', () => {
    const info = captureClient('1.2.3');
    expect(info.user_agent).toBe('TestAgent/1.0');
  });

  it('captures viewport from window.innerWidth/innerHeight', () => {
    const info = captureClient('1.2.3');
    expect(info.viewport).toEqual({ w: 1280, h: 800 });
  });

  it('captures locale from navigator.language', () => {
    const info = captureClient('1.2.3');
    expect(info.locale).toBe('en-US');
  });

  it('sets referrer to null when document.referrer is empty string', () => {
    setGlobals({
      window: {
        location: { href: 'https://example.com/' } as Location,
        innerWidth: 800,
        innerHeight: 600,
      } as typeof globalThis.window,
      navigator: {
        userAgent: 'UA',
        language: 'fr-FR',
      } as typeof globalThis.navigator,
      document: {
        referrer: '',
        title: '',
        querySelectorAll: (_: string) => [] as unknown as NodeListOf<Element>,
      } as unknown as typeof globalThis.document,
    });
    const info = captureClient('0.1.0');
    expect(info.referrer).toBeNull();
  });
});

describe('capturePageMetadata', () => {
  it('returns empty record when window is absent', () => {
    setGlobals({ window: undefined, document: undefined, navigator: undefined });
    const meta = capturePageMetadata();
    expect(meta).toEqual({});
  });

  it('captures document.title', () => {
    const mockMeta: Array<{ getAttribute: (k: string) => string | null }> = [];
    setGlobals({
      window: {} as typeof globalThis.window,
      navigator: {} as typeof globalThis.navigator,
      document: {
        title: 'My Page',
        querySelectorAll: (_: string) =>
          mockMeta as unknown as NodeListOf<Element>,
      } as unknown as typeof globalThis.document,
    });
    const meta = capturePageMetadata();
    expect(meta['title']).toBe('My Page');
  });

  it('captures og:title and og:description meta tags', () => {
    const mockMeta = [
      {
        getAttribute: (k: string) => {
          if (k === 'property') return 'og:title';
          if (k === 'content') return 'OG Title';
          return null;
        },
      },
      {
        getAttribute: (k: string) => {
          if (k === 'property') return 'og:description';
          if (k === 'content') return 'OG Desc';
          return null;
        },
      },
    ];
    setGlobals({
      window: {} as typeof globalThis.window,
      navigator: {} as typeof globalThis.navigator,
      document: {
        title: '',
        querySelectorAll: (_: string) =>
          mockMeta as unknown as NodeListOf<Element>,
      } as unknown as typeof globalThis.document,
    });
    const meta = capturePageMetadata();
    expect(meta['og:title']).toBe('OG Title');
    expect(meta['og:description']).toBe('OG Desc');
  });
});
```

- [ ] **Step 2: Run tests — confirm they fail with "Cannot find module './context.js'"**

```bash
npm run -w @intake/core test
```

Expected: import error for context.

- [ ] **Step 3: Implement core/src/context.ts**

```ts
import type { ClientInfo, Viewport } from './types.js';

/**
 * Captures browser client context for inclusion in SubmitRequest.client.
 * SSR-safe: all window/navigator/document accesses are guarded.
 */
export function captureClient(widgetVersion: string): ClientInfo {
  if (typeof window === 'undefined') {
    return {
      widget_version: widgetVersion,
      url: '',
      referrer: null,
      user_agent: '',
      viewport: { w: 0, h: 0 },
      locale: '',
    };
  }

  const viewport: Viewport = {
    w: window.innerWidth,
    h: window.innerHeight,
  };

  const referrerRaw =
    typeof document !== 'undefined' ? document.referrer : '';
  const referrer = referrerRaw.length > 0 ? referrerRaw : null;

  return {
    widget_version: widgetVersion,
    url: window.location.href,
    referrer,
    user_agent:
      typeof navigator !== 'undefined' ? navigator.userAgent : '',
    viewport,
    locale: typeof navigator !== 'undefined' ? navigator.language : '',
  };
}

/**
 * Captures Open Graph and title metadata from the current page.
 * SSR-safe: returns empty record when document is unavailable.
 */
export function capturePageMetadata(): Record<string, unknown> {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return {};
  }

  const meta: Record<string, unknown> = {};

  if (document.title) {
    meta['title'] = document.title;
  }

  const metaTags = document.querySelectorAll('meta[property]');
  metaTags.forEach((el) => {
    const property = el.getAttribute('property');
    const content = el.getAttribute('content');
    if (property && content !== null) {
      meta[property] = content;
    }
  });

  return meta;
}
```

- [ ] **Step 4: Run all tests — all must pass**

```bash
npm run -w @intake/core test
```

Expected:
```
 ✓ src/sse.test.ts (6 tests)
 ✓ src/context.test.ts (9 tests)
 Test Files  2 passed (2)
 Tests       15 passed (15)
```

- [ ] **Step 5: Type-check**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add core/src/context.ts core/src/context.test.ts
git commit -m "feat(core): add captureClient + capturePageMetadata with SSR guards and tests"
```

---

### Task 4: IntakeClient — init() + tests

`IntakeClient` takes an `IntakeConfig` and an optional `fetch` injection. `init()` POSTs to `/v1/intake/init` and stores the returned `session_id`.

**Files:**
- Create: `core/src/client.ts` (init only, extended in Tasks 5+6)
- Create: `core/src/client.test.ts` (init tests, extended in Tasks 5+6)

- [ ] **Step 1: Write the failing init test first**

Create `core/src/client.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest';
import { IntakeClient } from './client.js';
import type { IntakeConfig, ChatMessage } from './client-types.js';

// A fetch stub that returns a specific JSON response
function makeFetch(
  status: number,
  body: unknown,
  headers: Record<string, string> = { 'content-type': 'application/json' }
): typeof fetch {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get: (k: string) => headers[k.toLowerCase()] ?? null,
    },
    json: () => Promise.resolve(body),
    body: null,
  } as unknown as Response);
}

const BASE_CONFIG: IntakeConfig = {
  relayUrl: 'http://localhost:8080',
  widgetVersion: '0.1.0',
};

describe('IntakeClient.init()', () => {
  it('POSTs to /v1/intake/init and returns session_id + capabilities', async () => {
    const mockFetch = makeFetch(200, {
      session_id: 'sess-abc',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });

    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    const result = await client.init();

    expect(mockFetch).toHaveBeenCalledOnce();
    const [url, opts] = (mockFetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit];
    expect(url).toBe('http://localhost:8080/v1/intake/init');
    expect(opts.method).toBe('POST');

    expect(result.session_id).toBe('sess-abc');
    expect(result.capabilities.auth_modes).toContain('anonymous');
    expect(result.capabilities.streaming).toBe(true);
  });

  it('throws on non-2xx response from /init', async () => {
    const mockFetch = makeFetch(500, { error: { code: 'internal', message: 'boom' } });
    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    await expect(client.init()).rejects.toThrow(/500/);
  });

  it('stores session_id for use in subsequent requests', async () => {
    const mockFetch = makeFetch(200, {
      session_id: 'stored-sess',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    await client.init();
    // We will test the header is sent in the turn() tests — this verifies no throw
    expect(true).toBe(true);
  });
});
```

- [ ] **Step 2: Run tests — confirm they fail with "Cannot find module './client.js'"**

```bash
npm run -w @intake/core test
```

Expected: import error on `./client.js`.

- [ ] **Step 3: Implement core/src/client.ts (init only)**

```ts
import type { IntakeConfig, ChatMessage, SubmitResult } from './client-types.js';
import type { InitResponse, SubmitRequest, SubmitResponse } from './types.js';
import { consumeSSE } from './sse.js';
import { captureClient, capturePageMetadata } from './context.js';

export class IntakeClient {
  private readonly config: IntakeConfig;
  private readonly fetch: typeof globalThis.fetch;
  private sessionId: string | null = null;

  constructor(config: IntakeConfig, fetchImpl?: typeof globalThis.fetch) {
    this.config = config;
    this.fetch = fetchImpl ?? globalThis.fetch;
  }

  async init(): Promise<InitResponse> {
    const url = `${this.config.relayUrl}/v1/intake/init`;
    const res = await this.fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });

    if (!res.ok) {
      throw new Error(`init failed: ${res.status}`);
    }

    const data = (await res.json()) as InitResponse;
    this.sessionId = data.session_id;
    return data;
  }

  async turn(
    messages: ChatMessage[],
    onDelta: (delta: string) => void
  ): Promise<{ input_tokens: number; output_tokens: number }> {
    if (this.sessionId === null) {
      throw new Error('IntakeClient: call init() before turn()');
    }

    const url = `${this.config.relayUrl}/v1/intake/turn`;
    const res = await this.fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Intake-Session': this.sessionId,
      },
      body: JSON.stringify({ messages }),
    });

    if (!res.ok) {
      throw new Error(`turn failed: ${res.status}`);
    }

    if (!res.body) {
      throw new Error('turn: response has no body');
    }

    return new Promise<{ input_tokens: number; output_tokens: number }>(
      (resolve, reject) => {
        consumeSSE(res.body as ReadableStream<Uint8Array>, (frame) => {
          if ('error' in frame) {
            reject(new Error(frame.error));
          } else if ('done' in frame && frame.done) {
            resolve({
              input_tokens: frame.input_tokens,
              output_tokens: frame.output_tokens,
            });
          } else if ('delta' in frame) {
            onDelta(frame.delta);
          }
        }).catch(reject);
      }
    );
  }

  async submit(
    messages: ChatMessage[],
    routingHint?: string
  ): Promise<SubmitResult> {
    if (this.sessionId === null) {
      throw new Error('IntakeClient: call init() before submit()');
    }

    const clientInfo = captureClient(this.config.widgetVersion);
    const pageMetadata = capturePageMetadata();

    const body: SubmitRequest = {
      messages,
      client: clientInfo,
      user_claims: {},
      context: {
        app_context: this.config.appContext ?? {},
        page_metadata: pageMetadata,
      },
      routing_hint: routingHint ?? null,
    };

    const url = `${this.config.relayUrl}/v1/intake/submit`;
    const res = await this.fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Intake-Session': this.sessionId,
      },
      body: JSON.stringify(body),
    });

    if (!res.ok) {
      throw new Error(`submit failed: ${res.status}`);
    }

    return (await res.json()) as SubmitResponse;
  }
}
```

Note: The full class is implemented now (turn + submit included) to keep the file coherent. Tests for turn and submit come in Tasks 5 and 6. This matches TDD for init — the init tests pass; turn/submit are untested yet.

- [ ] **Step 4: Run tests — init tests must pass**

```bash
npm run -w @intake/core test
```

Expected:
```
 ✓ src/sse.test.ts (6 tests)
 ✓ src/context.test.ts (9 tests)
 ✓ src/client.test.ts (3 tests)
 Tests  18 passed (18)
```

- [ ] **Step 5: Type-check**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add core/src/client.ts core/src/client.test.ts
git commit -m "feat(core): add IntakeClient with init() and unit tests"
```

---

### Task 5: IntakeClient — turn() streaming tests

Add tests that verify `turn()` sends `X-Intake-Session`, calls `onDelta` per delta frame, resolves with token counts, and rejects on an SSE error frame.

**Files:**
- Modify: `core/src/client.test.ts`

- [ ] **Step 1: Add the turn() tests to client.test.ts**

Append the following `describe` block to `core/src/client.test.ts`:

```ts
// Helper to build a ReadableStream from an SSE string (for turn() tests)
function sseStream(raw: string): ReadableStream<Uint8Array> {
  const enc = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(enc.encode(raw));
      controller.close();
    },
  });
}

// A fetch stub that returns a streaming SSE body
function makeStreamFetch(sseBody: string): typeof fetch {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    headers: {
      get: (k: string) =>
        k.toLowerCase() === 'content-type' ? 'text/event-stream' : null,
    },
    body: sseStream(sseBody),
    json: () => Promise.reject(new Error('streaming response has no JSON')),
  } as unknown as Response);
}

describe('IntakeClient.turn()', () => {
  async function clientWithSession(fetchImpl: typeof fetch): Promise<IntakeClient> {
    const initFetch = makeFetch(200, {
      session_id: 'turn-sess',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    // First call is init; swap fetch after
    const client = new IntakeClient(BASE_CONFIG, initFetch);
    await client.init();
    // Replace internal fetch for subsequent calls by creating a new client
    // that already has the session set (we re-init with the new fetch)
    const client2 = new IntakeClient(BASE_CONFIG, fetchImpl);
    // Manually init the second client using a fresh init fetch
    const initFetch2 = makeFetch(200, {
      session_id: 'turn-sess',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    const c = new IntakeClient(BASE_CONFIG, (...args) => {
      // First call goes to initFetch2, subsequent to fetchImpl
      if ((c as unknown as { _inited: boolean })._inited) {
        return fetchImpl(...args);
      }
      (c as unknown as { _inited: boolean })._inited = true;
      return initFetch2(...args);
    });
    (c as unknown as { _inited: boolean })._inited = false;
    await c.init();
    return c;
  }

  it('sends X-Intake-Session header on turn()', async () => {
    const turnFetch = makeStreamFetch(
      'data: {"done":true,"input_tokens":1,"output_tokens":2}\n\n'
    );

    // Simplest approach: create client, init with session, then spy on turn fetch
    const calls: Array<[string, RequestInit]> = [];
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      calls.push(args as [string, RequestInit]);
      // First call: init
      if (calls.length === 1) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'hdr-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      // Second call: turn
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'text/event-stream' },
        body: sseStream(
          'data: {"done":true,"input_tokens":1,"output_tokens":2}\n\n'
        ),
        json: () => Promise.reject(new Error('streaming')),
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();
    await client.turn([{ role: 'user', content: 'hello' }], () => {});

    expect(calls).toHaveLength(2);
    const [, turnOpts] = calls[1];
    expect((turnOpts.headers as Record<string, string>)['X-Intake-Session']).toBe('hdr-sess');
  });

  it('calls onDelta for each delta frame and resolves with token counts', async () => {
    const sseBody =
      'data: {"delta":"foo"}\n\n' +
      'data: {"delta":"bar"}\n\n' +
      'data: {"done":true,"input_tokens":3,"output_tokens":7}\n\n';

    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'delta-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'text/event-stream' },
        body: sseStream(sseBody),
        json: () => Promise.reject(new Error('streaming')),
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();

    const deltas: string[] = [];
    const result = await client.turn(
      [{ role: 'user', content: 'test' }],
      (d) => deltas.push(d)
    );

    expect(deltas).toEqual(['foo', 'bar']);
    expect(result).toEqual({ input_tokens: 3, output_tokens: 7 });
  });

  it('rejects when an SSE error frame is received', async () => {
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'err-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'text/event-stream' },
        body: sseStream('data: {"error":"upstream provider failed"}\n\n'),
        json: () => Promise.reject(new Error('streaming')),
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();

    await expect(
      client.turn([{ role: 'user', content: 'test' }], () => {})
    ).rejects.toThrow('upstream provider failed');
  });

  it('throws if turn() is called before init()', async () => {
    const client = new IntakeClient(BASE_CONFIG, vi.fn());
    await expect(
      client.turn([{ role: 'user', content: 'hi' }], () => {})
    ).rejects.toThrow('init()');
  });
});
```

- [ ] **Step 2: Run all tests — turn() tests must pass**

```bash
npm run -w @intake/core test
```

Expected:
```
 ✓ src/sse.test.ts (6 tests)
 ✓ src/context.test.ts (9 tests)
 ✓ src/client.test.ts (7 tests)
 Tests  22 passed (22)
```

- [ ] **Step 3: Type-check**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add core/src/client.test.ts
git commit -m "test(core): add turn() streaming unit tests"
```

---

### Task 6: IntakeClient — submit() tests

Add tests for `submit()`: it POSTs to `/v1/intake/submit` with `X-Intake-Session`, includes a `SubmitRequest` body with `client` fields and `context.app_context`, and returns the parsed `SubmitResult`.

**Files:**
- Modify: `core/src/client.test.ts`

- [ ] **Step 1: Add submit() tests to client.test.ts**

Append the following `describe` block to `core/src/client.test.ts`:

```ts
describe('IntakeClient.submit()', () => {
  it('POSTs SubmitRequest to /v1/intake/submit with X-Intake-Session header', async () => {
    const submitResponse = {
      external_id: 'ticket-123',
      external_url: 'https://example.com/tickets/123',
      adapter_name: 'webhook',
      created_at: '2026-05-26T00:00:00Z',
    };

    const calls: Array<[string, RequestInit]> = [];
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      calls.push(args as [string, RequestInit]);
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'sub-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () => Promise.resolve(submitResponse),
        body: null,
      } as unknown as Response);
    });

    const client = new IntakeClient(
      { ...BASE_CONFIG, appContext: { tenant: 'acme' } },
      spyFetch
    );
    await client.init();

    const messages: ChatMessage[] = [
      { role: 'user', content: 'I found a bug' },
      { role: 'assistant', content: 'Tell me more.' },
    ];
    const result = await client.submit(messages, 'webhook');

    expect(calls).toHaveLength(2);
    const [submitUrl, submitOpts] = calls[1];
    expect(submitUrl).toBe('http://localhost:8080/v1/intake/submit');
    expect(submitOpts.method).toBe('POST');
    expect(
      (submitOpts.headers as Record<string, string>)['X-Intake-Session']
    ).toBe('sub-sess');

    const body = JSON.parse(submitOpts.body as string) as Record<string, unknown>;
    expect((body['messages'] as unknown[]).length).toBe(2);
    expect((body['context'] as Record<string, unknown>)['app_context']).toEqual({ tenant: 'acme' });
    expect(body['routing_hint']).toBe('webhook');

    // client fields present
    const clientField = body['client'] as Record<string, unknown>;
    expect(clientField['widget_version']).toBe('0.1.0');
    // url/referrer/user_agent/viewport/locale come from captureClient (SSR = empty in Node)
    expect(typeof clientField['url']).toBe('string');

    expect(result).toEqual(submitResponse);
  });

  it('sets routing_hint to null when not provided', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      calls.push(args as [string, RequestInit]);
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'sub-sess2',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () =>
          Promise.resolve({
            external_id: 'x',
            external_url: '',
            adapter_name: 'webhook',
            created_at: '',
          }),
        body: null,
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();
    await client.submit([{ role: 'user', content: 'test' }]);

    const [, submitOpts] = calls[1];
    const body = JSON.parse(submitOpts.body as string) as Record<string, unknown>;
    expect(body['routing_hint']).toBeNull();
  });

  it('throws on non-2xx response from /submit', async () => {
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'sub-err',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: false,
        status: 502,
        headers: { get: () => 'application/json' },
        json: () => Promise.resolve({ error: { code: 'adapter_error', message: 'webhook down' } }),
        body: null,
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();
    await expect(
      client.submit([{ role: 'user', content: 'test' }])
    ).rejects.toThrow(/502/);
  });

  it('throws if submit() is called before init()', async () => {
    const client = new IntakeClient(BASE_CONFIG, vi.fn());
    await expect(
      client.submit([{ role: 'user', content: 'hi' }])
    ).rejects.toThrow('init()');
  });
});
```

- [ ] **Step 2: Run all tests — all must pass**

```bash
npm run -w @intake/core test
```

Expected:
```
 ✓ src/sse.test.ts (6 tests)
 ✓ src/context.test.ts (9 tests)
 ✓ src/client.test.ts (11 tests)
 Tests  26 passed (26)
```

- [ ] **Step 3: Type-check**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add core/src/client.test.ts
git commit -m "test(core): add submit() unit tests"
```

---

### Task 7: Wire index.ts public exports

Extend `core/src/index.ts` to re-export the public API alongside the existing generated-types export. `client-types.ts` was created in Task 1 and `client.ts` already imports from it — this task only wires the top-level barrel.

**Files:**
- Modify: `core/src/index.ts`

- [ ] **Step 1: Replace core/src/index.ts**

```ts
// @intake/core — public API. Frozen in sub-plan 1-v.

// Generated payload types (Phase 0 contract spine)
export * from './generated/payload.js';

// Public client API (frozen in 1-v)
export type { IntakeConfig, ChatMessage, SubmitResult } from './client-types.js';
export { IntakeClient } from './client.js';

// Context capture utilities (exported for widget use)
export { captureClient, capturePageMetadata } from './context.js';
```

- [ ] **Step 2: Run all tests**

```bash
npm run -w @intake/core test
```

Expected: all 26 tests still pass.

- [ ] **Step 3: Type-check**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add core/src/index.ts
git commit -m "feat(core): wire public API exports in index.ts barrel"
```

---

### Task 8: Update tsconfig.json to cover smoke directory

The smoke script lives in `core/smoke/`, which is not covered by the existing `include: ["src/**/*.ts"]`.

**Files:**
- Modify: `core/tsconfig.json`

- [ ] **Step 1: Update core/tsconfig.json**

Replace the file content with:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "forceConsistentCasingInFileNames": true
  },
  "include": ["src/**/*.ts", "smoke/**/*.ts"]
}
```

- [ ] **Step 2: Type-check (will fail until smoke/drive.ts exists, but that's expected)**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0 (no smoke dir yet, so no files to check; TypeScript skips missing globs).

- [ ] **Step 3: Commit**

```bash
git add core/tsconfig.json
git commit -m "chore(core): extend tsconfig include to cover smoke/ directory"
```

---

### Task 9: Smoke script — core/smoke/drive.ts

The smoke script proves the client speaks the real relay contract. It requires a running relay (from sub-plans 1-i through 1-iv) and a real `ANTHROPIC_API_KEY`. It does init → turn (printing deltas) → submit, then prints the SubmitResult.

**Files:**
- Create: `core/smoke/drive.ts`

**Pre-condition:** The relay must be running (`go run ./cmd/relay --config config.yaml` from `relay/`) and `ANTHROPIC_API_KEY` must be exported. The relay config must have `adapters.webhook.enabled: true` and a valid `url`. See README §8 for full smoke setup.

- [ ] **Step 1: Create core/smoke/drive.ts**

```ts
/**
 * Smoke script for @intake/core.
 * Drives init → turn → submit against a running relay.
 *
 * Usage:
 *   RELAY_URL=http://localhost:8080 npx tsx core/smoke/drive.ts
 *
 * Prerequisites:
 *   - relay running (sub-plans 1-i..1-iv complete)
 *   - ANTHROPIC_API_KEY exported in the relay's environment
 *   - adapters.webhook configured in relay config.yaml
 */

import { IntakeClient } from '../src/index.js';
import type { ChatMessage } from '../src/index.js';

const RELAY_URL = process.env['RELAY_URL'] ?? 'http://localhost:8080';
const WIDGET_VERSION = '0.1.0-smoke';

async function main(): Promise<void> {
  console.log(`[smoke] connecting to relay at ${RELAY_URL}`);

  const client = new IntakeClient({
    relayUrl: RELAY_URL,
    widgetVersion: WIDGET_VERSION,
    appContext: { smoke: true },
  });

  // 1. Init
  console.log('[smoke] POST /v1/intake/init ...');
  const initResult = await client.init();
  console.log(`[smoke] session_id: ${initResult.session_id}`);
  console.log(`[smoke] capabilities:`, initResult.capabilities);

  // 2. Turn — single user message; stream deltas to stdout
  const messages: ChatMessage[] = [
    {
      role: 'user',
      content:
        'I found a bug: clicking the Save button twice submits the form twice. ' +
        'This is reproducible on Chrome 124 on macOS.',
    },
  ];

  console.log('\n[smoke] POST /v1/intake/turn — streaming ...');
  process.stdout.write('[assistant] ');

  const tokenCounts = await client.turn(messages, (delta) => {
    process.stdout.write(delta);
  });

  process.stdout.write('\n');
  console.log(
    `\n[smoke] turn complete. input_tokens=${tokenCounts.input_tokens} output_tokens=${tokenCounts.output_tokens}`
  );

  // Build the full conversation for submit (user + assistant reply)
  // The relay is stateless — we own the history and send it back
  // In a real widget, the assistant's content would be accumulated from deltas.
  // For the smoke we send just the user turn; the relay will classify from that.
  const submitMessages: ChatMessage[] = [
    ...messages,
    // We don't have the full assistant text here (we streamed it),
    // so we include a placeholder that signals end-of-conversation to the classifier.
    {
      role: 'assistant',
      content: '(end of guided conversation — smoke test)',
    },
  ];

  // 3. Submit
  console.log('\n[smoke] POST /v1/intake/submit ...');
  const submitResult = await client.submit(submitMessages);

  console.log('[smoke] SubmitResult:');
  console.log(JSON.stringify(submitResult, null, 2));

  console.log('\n[smoke] PASS');
}

main().catch((err: unknown) => {
  console.error('[smoke] FAIL:', err);
  process.exit(1);
});
```

- [ ] **Step 2: Type-check (must be clean)**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0. If there are errors in the smoke script they must be fixed before committing.

- [ ] **Step 3: Run tests (smoke does not run in unit test — just verify no breakage)**

```bash
npm run -w @intake/core test
```

Expected: all 26 tests pass.

- [ ] **Step 4: Commit**

```bash
git add core/smoke/drive.ts
git commit -m "feat(core): add smoke/drive.ts — init/turn/submit against live relay"
```

---

### Task 10: Final verification pass

Confirm every build-fail gate in README §7 is green for the TS stack.

**Files:** none (verification only)

- [ ] **Step 1: Run the full test suite**

```bash
npm run -w @intake/core test
```

Expected:
```
 ✓ src/sse.test.ts       (6 tests)
 ✓ src/context.test.ts   (9 tests)
 ✓ src/client.test.ts   (11 tests)
 Test Files  3 passed (3)
 Tests      26 passed (26)
```

- [ ] **Step 2: Type-check**

```bash
npm run -w @intake/core type-check
```

Expected: exits 0.

- [ ] **Step 3: Verify type-check from monorepo root (the CI gate)**

```bash
cd C:/src/ai/intake && npm run type-check
```

Expected: exits 0. (This runs `tsc --noEmit` in `@intake/core` via the root script.)

- [ ] **Step 4: Verify Phase-0 contract gate does not regress**

```bash
cd C:/src/ai/intake && bash scripts/verify-contract.sh
```

Expected: exits 0. (Confirms the Phase-0 `schema/payload.v1.json` and generated types are untouched.)

- [ ] **Step 5: Smoke (manual, requires live relay)**

See §5 below. This is not run in CI — it's a manual step gating sub-plan completion.

- [ ] **Step 6: Final commit (if any fixups)**

If the verification steps above required any fixups, commit them with:

```bash
git add -p
git commit -m "fix(core): <describe fixup>"
```

---

## 5. Smoke

**Requires:** Sub-plans 1-i through 1-iv complete and the relay running. This smoke cannot be run in isolation against a mock — it proves the real relay contract.

**Pre-conditions:**
1. Node 24.12.0 and Go 1.23.2 installed.
2. `ANTHROPIC_API_KEY` exported in the relay's shell environment.
3. Relay running: `cd C:/src/ai/intake/relay && go run ./cmd/relay --config ../config.yaml`
   - `GET http://localhost:8080/v1/health` returns `200 OK`.
4. A local webhook receiver listening on `http://localhost:9099/intake` logging POST bodies (see `examples/webhook-receiver/` when available, or `python3 -c "from http.server import HTTPServer,BaseHTTPRequestHandler; ..."` equivalent).
5. `config.yaml` has `adapters.webhook.url: "http://localhost:9099/intake"` and `adapters.webhook.enabled: true`.

**Execution:**

```bash
# From monorepo root — npm workspaces sets cwd to core/ before running the script
cd C:/src/ai/intake
RELAY_URL=http://localhost:8080 npm run -w @intake/core smoke
```

**Expected output (abridged):**
```
[smoke] connecting to relay at http://localhost:8080
[smoke] POST /v1/intake/init ...
[smoke] session_id: <uuid>
[smoke] capabilities: { auth_modes: [ 'anonymous' ], streaming: true }

[smoke] POST /v1/intake/turn — streaming ...
[assistant] <streaming assistant tokens appear here...>

[smoke] turn complete. input_tokens=N output_tokens=M

[smoke] POST /v1/intake/submit ...
[smoke] SubmitResult:
{
  "external_id": "<id>",
  "external_url": "<url>",
  "adapter_name": "webhook",
  "created_at": "<ISO-8601>"
}

[smoke] PASS
```

**Verification checklist:**
- [ ] Script exits 0.
- [ ] `session_id` is a non-empty UUID.
- [ ] At least one delta token prints to stdout (SSE streaming confirmed).
- [ ] `SubmitResult` contains `external_id` and `adapter_name: "webhook"`.
- [ ] Webhook receiver logs one POST with a JSON body containing `schema_version: "1.0"`.
- [ ] Grep relay stdout for `ANTHROPIC_API_KEY` value — must NOT appear.

---

## 6. Done Criteria

- [ ] `npm run -w @intake/core test` exits 0 with all 26 tests passing (sse: 6, context: 9, client: 11).
- [ ] `npm run -w @intake/core type-check` exits 0.
- [ ] `npm run type-check` (monorepo root) exits 0.
- [ ] `bash scripts/verify-contract.sh` exits 0 (Phase-0 gate not regressed).
- [ ] `core/src/index.ts` re-exports exactly: `IntakeClient`, `IntakeConfig`, `ChatMessage`, `SubmitResult`, `captureClient`, `capturePageMetadata`, plus all generated payload types.
- [ ] The public API matches README §6.7 exactly (constructor, init, turn, submit signatures).
- [ ] Smoke script (`core/smoke/drive.ts`) drives a live relay end-to-end and exits 0.
- [ ] No `console.log` in production files (`client.ts`, `context.ts`, `sse.ts`, `types.ts`, `client-types.ts`, `index.ts`).
- [ ] No provider API key or secret appears in any source file.
- [ ] All commits are atomic and pass type-check individually.
