# 6-iii Widget Capture + Redaction Modal + Attachment Strip + DTO Wiring — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the user-facing widget UX for Phase 6 against the **frozen** 6-i wire contract: html2canvas page capture (DI-wrapped for tests), a full-screen `ScreenshotRedactor` modal with rectangle redactions, a pending `AttachmentStrip` with thumbnails + remove, the IntakeWidget Attach button that is visible iff `capabilities.attachments` is non-null, useIntake extensions for pending state + capabilities-gated `canAttach` + error-code-to-friendly-string mapping, and the core TS plumbing (`types.ts` additive fields, `client.submit()` threading attachments, `client.init()` parsing the new caps block). After this sub-plan: the Vue widget can capture → redact → strip → submit attachments end-to-end against a stubbed `IntakeClient`; relay-side wiring (validated bytes, native adapter forwarding) is covered by 6-i + 6-ii.

**Architecture:** Three layers wired by dependency injection. (1) `core/src/capture.ts` exports `setHtml2Canvas(fn)` so tests register a fake (production registers the real `html2canvas` once on first `capturePage()`); `canvasToDataURL(canvas, mime)` wraps `canvas.toBlob` + `FileReader.readAsDataURL` in a promise. (2) `core/src/attachments.ts` exports a `PendingAttachment` type + `AttachmentList` class with `add/remove/clear/items/aggregateSizeBytes` and accept/reject logic against an injected limits config — three named errors (`AttachmentTooLargeError`, `AggregateTooLargeError`, `MimeNotAllowedError`) each carry a `code` string the widget maps to a banner string. (3) `vue/src/components/ScreenshotRedactor.vue` + `AttachmentStrip.vue` are mount-in-isolation Vue 3 SFCs with stubbed canvas-2d contexts in tests; `useIntake.ts` orchestrates them via a `redactorSource: Ref<HTMLCanvasElement | null>` watched by `IntakeWidget.vue`, plus `pendingAttachments`, `canAttach`, `attachLimits`, `attachAndRedact()`, `commitRedacted()`, `removeAttachment()`, `clearAttachments()`. `submit()` threads `pendingAttachments.value` into `client.submit()` and clears the list on success; on failure it parses the relay ErrorEnvelope and maps known attachment codes to friendly banner strings.

**Tech Stack:** Vue 3.5.34 (`<script setup lang="ts">`), TypeScript 5.6.3, Vitest 4.1.7, `@vue/test-utils` 2.4.10, `jsdom` 29.1.1 (existing). One new runtime browser dep: `html2canvas` exact `1.4.1` in `core/package.json` `dependencies` — no caret. Zero new dev deps. No new test runners. No Go changes in this sub-plan (6-i + 6-ii cover relay).

---

## Design References

- Phase 6 design spec §3.5 — widget redaction is a dedicated modal; `html2canvas` DI via `core/src/capture.ts setHtml2Canvas(fn)`; canvas-2d context stubbed in tests
- Phase 6 design spec §5.7 — widget file inventory frozen here (`core/src/capture.ts`, `core/src/attachments.ts`, `core/src/client.ts`, `core/src/types.ts`, `vue/src/components/ScreenshotRedactor.vue`, `vue/src/components/AttachmentStrip.vue`, `vue/src/components/IntakeWidget.vue`, `vue/src/composables/useIntake.ts`)
- Phase 6 design spec §6 — `html2canvas` pin `1.4.1` exact; `scripts/check-pins.sh` gets one new line (no caret in `core/package.json`)
- Phase 6 design spec §8.3 — error-code → friendly-string mapping in `useIntake.ts`
- Phase 6 README §8.3 — `SubmitRequest.attachments` shape (`{type, mime_type, url, label?}`); FROZEN in 6-i, consumed unchanged here
- Phase 6 README §8.4 — `Capabilities.attachments` shape (`{max_size_bytes, max_total_bytes, allowed_mime_types}`); FROZEN in 6-i, consumed unchanged here
- Phase 6 README §8.8 — endpoint envelope codes (`request_body_too_large`, `attachment_too_large`, `attachments_exceed_total`, `attachment_mime_not_allowed`, `attachment_mime_mismatch`, `attachment_malformed`, `attachment_type_unsupported`, `attachments_disabled`)
- Phase 5 5-i Task 1 — `scripts/check-pins.sh` extension pattern (mirrored here for `html2canvas` no-caret check)
- Phase 5 5-iii — overall cadence + TS-heavy task ordering exemplar (5-step TDD pattern, exact code blocks, exact diffs)
- Existing widget code reference: `core/src/client.ts` (POST shape), `core/src/types.ts` (interfaces frozen in 1-v; this sub-plan ADDS additive fields), `vue/src/composables/useIntake.ts` (template for the new ref+computed surface), `vue/src/composables/useIntake.spec.ts` (template for how Vitest mounts the composable + stubs `@intake/core`)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `core/package.json` | Modify | Add `"html2canvas": "1.4.1"` (exact, no caret) to `dependencies` |
| `scripts/check-pins.sh` | Modify | Add a no-caret check for `html2canvas` in `core/package.json` |
| `core/src/types.ts` | Modify | Additive: `InitResponse.capabilities.attachments?` + `SubmitRequest.attachments?` |
| `core/src/capture.ts` | Create | `setHtml2Canvas(fn)`, `capturePage()`, `canvasToDataURL(canvas, mime)`; rejects on 0×0 canvas |
| `core/src/capture.test.ts` | Create | Stubs `html2canvas`, FileReader; covers DI, capture invocation, blob→data: URL, 0×0 rejection |
| `core/src/attachments.ts` | Create | `PendingAttachment` type + `AttachmentList` class + three named errors with `code` strings |
| `core/src/attachments.test.ts` | Create | add/remove/clear/items, aggregate accounting, per-attachment cap, aggregate cap, MIME allowlist |
| `core/src/client.ts` | Modify | `submit()` accepts optional `attachments` param and includes when non-empty; non-2xx parses ErrorEnvelope and attaches `code` to thrown Error |
| `core/src/client.test.ts` | Modify | `submit()` includes `attachments[]` when non-empty / omits when empty; `init()` parses `capabilities.attachments`; non-2xx with attachment code → Error has `code` property |
| `core/src/index.ts` | Modify | Re-export `PendingAttachment`, `AttachmentList`, `capturePage`, `canvasToDataURL` |
| `vue/src/components/ScreenshotRedactor.vue` | Create | Full-screen modal — overlay canvas, rectangle draw, Clear/Save/Cancel, ESC, basic focus trap |
| `vue/src/components/ScreenshotRedactor.spec.ts` | Create | Mounts with stubbed 2d ctx; mouse draw → rect; Clear resets; Save emits dataUrl; Cancel emits, no toDataURL; ESC cancels; clamp out-of-bounds |
| `vue/src/components/AttachmentStrip.vue` | Create | Thumbnail per item, Remove button per item, aggregate-size badge, hidden when empty |
| `vue/src/components/AttachmentStrip.spec.ts` | Create | One thumb per item, remove emits index, badge correct, empty hidden |
| `vue/src/composables/useIntake.ts` | Modify | `pendingAttachments`, `canAttach`, `attachLimits`, `attachAndRedact()`, `commitRedacted()`, `removeAttachment()`, `clearAttachments()`, `redactorSource`; `submit()` threads attachments + maps error codes |
| `vue/src/composables/useIntake.spec.ts` | Modify | Cover the new surface incl. code→friendly mapping; capability-gated `canAttach`; submit threads attachments + clears on success |
| `vue/src/components/IntakeWidget.vue` | Modify | Attach button (visible iff `canAttach`); click → `attachAndRedact()` → modal opens; AttachmentStrip rendered between conversation and input when items non-empty |
| `vue/src/components/IntakeWidget.spec.ts` | Create | Attach hidden when caps null; visible when non-null; click → capture + modal; Save → strip updates; Submit threads attachments |
| `vue/src/index.ts` | Modify | Re-export `ScreenshotRedactor` and `AttachmentStrip` |

---

## Tasks

### Task 1: Pin `html2canvas` 1.4.1 + extend `check-pins.sh`

**Files:** Modify `core/package.json`, `scripts/check-pins.sh`

- [ ] **Step 1: Add `html2canvas` to `core/package.json` dependencies (exact pin, no caret)**

The current `core/package.json` has no `dependencies` block (only `devDependencies`). Insert a `dependencies` block between `"smoke"` and `"devDependencies"`.

Apply this hunk to `core/package.json`:

```diff
@@
   "scripts": {
     "type-check": "tsc --noEmit",
     "test": "vitest run",
     "test:watch": "vitest",
     "smoke": "tsx smoke/drive.ts"
   },
+  "dependencies": {
+    "html2canvas": "1.4.1"
+  },
   "devDependencies": {
     "@types/node": "22.15.21",
     "typescript": "5.6.3",
     "vitest": "4.1.7",
     "tsx": "4.22.3"
   }
 }
```

- [ ] **Step 2: Install (no-op if cached) and verify exact version landed**

Run from `core/`:

```
npm install
```

Verify with `cat core/package.json | grep html2canvas` — line MUST read `"html2canvas": "1.4.1"` (no `^`, no `~`).

Verify with `cat core/package-lock.json | grep -A1 '"html2canvas"' | head -10` — the resolved `"version"` MUST equal `"1.4.1"`.

- [ ] **Step 3: Add the no-caret gate to `scripts/check-pins.sh`**

After the existing `golang.org/x/time` gate (line ~49) and BEFORE the `# Gate: no go install/get ...@latest` block (line ~50), insert this block (mirrors the existing `typescript` gate in style):

```diff
@@
 # Gate: golang.org/x/time must be exact-pinned (no caret, no @latest) in go.mod. Phase 5.
 if grep -E 'golang.org/x/time' relay/go.mod | grep -E '(\^|@latest)'; then
   echo "ERROR: golang.org/x/time is caret/latest-pinned in relay/go.mod; PHASE_PLANNING §5 requires exact pins" >&2
   fail=1
 fi
+# Gate: html2canvas must be exact-pinned (no caret, no ~) in core/package.json. Phase 6.
+if grep -E '"html2canvas":\s*"[\^~]' core/package.json; then
+  echo "ERROR: html2canvas in core/package.json is caret/tilde-pinned; PHASE_PLANNING §5 requires exact pins" >&2
+  fail=1
+fi
 # Gate: no go install/get ...@latest in install scripts (excludes this file to avoid self-match).
```

- [ ] **Step 4: Run `scripts/check-pins.sh` and confirm OK**

```
bash scripts/check-pins.sh
```

Expected: exit 0; stdout ends with `OK: all codegen tools are exact-pinned`.

Negative regression: temporarily edit `core/package.json` to `"html2canvas": "^1.4.1"`; rerun — expected exit 1 with the new ERROR line; revert.

- [ ] **Step 5: Type-check + build core/ to confirm no break from the new dep**

```
cd core && npm run type-check && cd ..
```

Expected: clean.

- [ ] **Step 6: Commit**

```
git add core/package.json core/package-lock.json scripts/check-pins.sh
git commit -m "$(cat <<'EOF'
feat(6-iii): pin html2canvas 1.4.1 (exact) and extend check-pins.sh

- core/package.json: add dependencies.html2canvas = "1.4.1" (no caret per PHASE_PLANNING §5)
- scripts/check-pins.sh: gate caret/tilde on html2canvas in core/package.json
EOF
)"
```

---

### Task 2: Extend `core/src/types.ts` additively for Phase 6

**Files:** Modify `core/src/types.ts`

- [ ] **Step 1: Add the optional Capabilities.attachments + SubmitRequest.attachments fields**

The existing file is frozen in 1-v but Phase 6 explicitly grows shapes additively (Phase 6 README §8.3, §8.4 — both fields are `omitempty` on the Go side and optional on the TS side).

Apply this hunk to `core/src/types.ts`:

```diff
@@
 export interface InitResponse {
   session_id: string;
   capabilities: {
     auth_modes: string[];
     streaming: boolean;
+    /**
+     * Attachment capabilities. Null/absent → relay has attachments disabled OR
+     * no enabled adapter accepts any allowed MIME; widget MUST hide its Attach
+     * button entirely (no "disabled but visible" state). Phase 6 §3.3.
+     */
+    attachments?: {
+      max_size_bytes: number;
+      max_total_bytes: number;
+      allowed_mime_types: string[];
+    };
   };
 }
@@
 export interface SubmitRequest {
   messages: TurnMessage[];
   client: ClientInfo;
   user_claims: Record<string, unknown>;
   context: ContextInfo;
   routing_hint: string | null;
+  /**
+   * Optional attachments. Each entry's url is a data: URL (data:<mime>;base64,...).
+   * Relay attachvalidate enforces magic-byte + per-attachment + aggregate caps.
+   * Omitted when no pending attachments. Phase 6 README §8.3.
+   */
+  attachments?: Array<{
+    type: 'screenshot';
+    mime_type: string;
+    url: string;
+    label?: string;
+  }>;
 }
```

- [ ] **Step 2: Type-check core/**

```
cd core && npm run type-check && cd ..
```

Expected: clean. (No tests yet — `client.ts` will tighten in Task 5; tests in Task 5 too.)

- [ ] **Step 3: Commit**

```
git add core/src/types.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): extend core types — InitResponse.capabilities.attachments + SubmitRequest.attachments

Both fields are additive and optional, mirroring the relay's omitempty JSON tags
frozen in Phase 6 README §8.3 + §8.4. Widget hides Attach UI when caps.attachments
is absent.
EOF
)"
```

---

### Task 3: `core/src/capture.ts` + tests (html2canvas DI + canvas→data URL)

**Files:** Create `core/src/capture.ts`, Create `core/src/capture.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `core/src/capture.test.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  setHtml2Canvas,
  capturePage,
  canvasToDataURL,
  __resetCaptureForTests,
} from './capture.js';

beforeEach(() => {
  __resetCaptureForTests();
});

describe('setHtml2Canvas / capturePage', () => {
  it('capturePage() invokes the injected html2canvas with document.body', async () => {
    const fakeCanvas = { width: 100, height: 50 } as unknown as HTMLCanvasElement;
    const stub = vi.fn().mockResolvedValue(fakeCanvas);
    setHtml2Canvas(stub);

    const canvas = await capturePage();

    expect(stub).toHaveBeenCalledOnce();
    expect(stub).toHaveBeenCalledWith(document.body, expect.any(Object));
    expect(canvas).toBe(fakeCanvas);
  });

  it('capturePage() rejects when no html2canvas has been registered and no auto-loader is available', async () => {
    // No setHtml2Canvas() call; production would dynamic-import. Tests must
    // register a stub; this test asserts the explicit-failure surface.
    await expect(capturePage()).rejects.toThrow(/html2canvas not registered/i);
  });

  it('capturePage() rejects on a 0x0 canvas', async () => {
    const fakeCanvas = { width: 0, height: 0 } as unknown as HTMLCanvasElement;
    setHtml2Canvas(vi.fn().mockResolvedValue(fakeCanvas));
    await expect(capturePage()).rejects.toThrow(/0x0/);
  });
});

describe('canvasToDataURL', () => {
  it('converts a canvas to a data: URL via toBlob + FileReader', async () => {
    const blob = new Blob([new Uint8Array([1, 2, 3])], { type: 'image/png' });
    const fakeCanvas = {
      width: 10,
      height: 10,
      toBlob(cb: (b: Blob | null) => void, mime: string) {
        expect(mime).toBe('image/png');
        cb(blob);
      },
    } as unknown as HTMLCanvasElement;

    // Stub FileReader to deterministically yield a known data URL
    const expected = 'data:image/png;base64,AQID';
    class FakeReader {
      result: string | null = null;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      readAsDataURL(_b: Blob) {
        this.result = expected;
        queueMicrotask(() => this.onload && this.onload());
      }
    }
    const realFR = globalThis.FileReader;
    (globalThis as unknown as { FileReader: typeof FileReader }).FileReader =
      FakeReader as unknown as typeof FileReader;

    try {
      const url = await canvasToDataURL(fakeCanvas, 'image/png');
      expect(url).toBe(expected);
    } finally {
      (globalThis as unknown as { FileReader: typeof FileReader }).FileReader = realFR;
    }
  });

  it('rejects when toBlob yields null', async () => {
    const fakeCanvas = {
      width: 10,
      height: 10,
      toBlob(cb: (b: Blob | null) => void) {
        cb(null);
      },
    } as unknown as HTMLCanvasElement;
    await expect(canvasToDataURL(fakeCanvas, 'image/png')).rejects.toThrow(
      /toBlob returned null/,
    );
  });

  it('rejects when FileReader errors', async () => {
    const blob = new Blob([new Uint8Array([1])], { type: 'image/png' });
    const fakeCanvas = {
      width: 10,
      height: 10,
      toBlob(cb: (b: Blob | null) => void) {
        cb(blob);
      },
    } as unknown as HTMLCanvasElement;
    class FailingReader {
      result: string | null = null;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      readAsDataURL(_b: Blob) {
        queueMicrotask(() => this.onerror && this.onerror());
      }
    }
    const realFR = globalThis.FileReader;
    (globalThis as unknown as { FileReader: typeof FileReader }).FileReader =
      FailingReader as unknown as typeof FileReader;
    try {
      await expect(canvasToDataURL(fakeCanvas, 'image/png')).rejects.toThrow(
        /FileReader/,
      );
    } finally {
      (globalThis as unknown as { FileReader: typeof FileReader }).FileReader = realFR;
    }
  });
});
```

Run:

```
cd core && npm run test -- capture && cd ..
```

Expected: tests fail (module does not exist).

- [ ] **Step 2: Implement `core/src/capture.ts` to pass the tests**

Create `core/src/capture.ts`:

```ts
// Phase 6 §3.5 — html2canvas is dependency-injected so tests do not load the
// real library. Production calls setHtml2Canvas(real) once on first capture
// via a dynamic import; tests call setHtml2Canvas(stub) before capturePage().

export type Html2CanvasFn = (
  el: HTMLElement,
  opts?: Record<string, unknown>,
) => Promise<HTMLCanvasElement>;

let registered: Html2CanvasFn | null = null;

/**
 * Register the html2canvas implementation. Production callers invoke this
 * once during widget bootstrap with the real `html2canvas` module's default
 * export; tests invoke it with a stub before calling `capturePage()`.
 */
export function setHtml2Canvas(fn: Html2CanvasFn): void {
  registered = fn;
}

/**
 * TEST-ONLY helper. Resets the registered html2canvas so each test starts
 * from a known clean state. Production never calls this.
 */
export function __resetCaptureForTests(): void {
  registered = null;
}

/**
 * Captures `document.body` to a canvas via the registered html2canvas.
 * Throws if no implementation has been registered or if the resulting
 * canvas is 0x0 (defensive — a 0x0 canvas cannot be turned into a useful
 * data URL and would mislead the redactor modal).
 */
export async function capturePage(): Promise<HTMLCanvasElement> {
  if (registered === null) {
    throw new Error(
      'capture: html2canvas not registered; call setHtml2Canvas(fn) first',
    );
  }
  const canvas = await registered(document.body, {
    // Conservative defaults — tests pass an opts object through but do not
    // depend on its contents.
    useCORS: true,
    backgroundColor: null,
  });
  if (canvas.width === 0 || canvas.height === 0) {
    throw new Error('capture: refusing 0x0 canvas');
  }
  return canvas;
}

/**
 * Converts a canvas to a `data:` URL of the requested MIME via canvas.toBlob
 * + FileReader.readAsDataURL. Promise-wraps the callback APIs and rejects on
 * either toBlob yielding null or FileReader erroring.
 */
export function canvasToDataURL(
  canvas: HTMLCanvasElement,
  mime: 'image/png' | 'image/jpeg' | 'image/webp',
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob === null) {
        reject(new Error('canvasToDataURL: toBlob returned null'));
        return;
      }
      const reader = new FileReader();
      reader.onload = () => {
        const r = reader.result;
        if (typeof r === 'string') {
          resolve(r);
        } else {
          reject(new Error('canvasToDataURL: FileReader yielded non-string result'));
        }
      };
      reader.onerror = () => {
        reject(new Error('canvasToDataURL: FileReader errored'));
      };
      reader.readAsDataURL(blob);
    }, mime);
  });
}
```

- [ ] **Step 3: Re-run tests; confirm green**

```
cd core && npm run test -- capture && cd ..
```

Expected: all `capture` tests pass.

- [ ] **Step 4: Type-check + build core**

```
cd core && npm run type-check && cd ..
```

Expected: clean.

- [ ] **Step 5: Commit**

```
git add core/src/capture.ts core/src/capture.test.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): core/src/capture.ts — html2canvas DI + canvas→data URL helper

setHtml2Canvas(fn) lets tests inject a stub before capturePage() runs;
capturePage() throws on 0x0 canvases; canvasToDataURL wraps toBlob +
FileReader.readAsDataURL with explicit reject paths for both failure modes.
EOF
)"
```

---

### Task 4: `core/src/attachments.ts` + tests (PendingAttachment + AttachmentList + named errors)

**Files:** Create `core/src/attachments.ts`, Create `core/src/attachments.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `core/src/attachments.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import {
  AttachmentList,
  AttachmentTooLargeError,
  AggregateTooLargeError,
  MimeNotAllowedError,
  type PendingAttachment,
} from './attachments.js';

const LIMITS = {
  maxSizeBytes: 5_000_000,
  maxTotalBytes: 10_000_000,
  allowedMimeTypes: ['image/png', 'image/jpeg', 'image/webp'],
};

function mkAttachment(sizeBytes: number, mime = 'image/png'): PendingAttachment {
  return {
    type: 'screenshot',
    mimeType: mime,
    dataUrl: `data:${mime};base64,AAAA`,
    sizeBytes,
  };
}

describe('AttachmentList', () => {
  it('starts empty', () => {
    const list = new AttachmentList(LIMITS);
    expect(list.items()).toEqual([]);
    expect(list.aggregateSizeBytes()).toBe(0);
  });

  it('add() appends and tracks aggregate', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(1000));
    list.add(mkAttachment(2000));
    expect(list.items()).toHaveLength(2);
    expect(list.aggregateSizeBytes()).toBe(3000);
  });

  it('remove(index) removes the entry at index', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(1000));
    list.add(mkAttachment(2000));
    list.remove(0);
    expect(list.items()).toHaveLength(1);
    expect(list.items()[0].sizeBytes).toBe(2000);
    expect(list.aggregateSizeBytes()).toBe(2000);
  });

  it('clear() empties the list and resets aggregate', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(1000));
    list.clear();
    expect(list.items()).toEqual([]);
    expect(list.aggregateSizeBytes()).toBe(0);
  });

  it('add() throws AttachmentTooLargeError when single exceeds maxSizeBytes', () => {
    const list = new AttachmentList(LIMITS);
    expect(() => list.add(mkAttachment(6_000_000))).toThrow(AttachmentTooLargeError);
    expect(list.items()).toEqual([]);
  });

  it('AttachmentTooLargeError carries code "attachment_too_large"', () => {
    const list = new AttachmentList(LIMITS);
    try {
      list.add(mkAttachment(6_000_000));
      throw new Error('should have thrown');
    } catch (e) {
      expect(e).toBeInstanceOf(AttachmentTooLargeError);
      expect((e as AttachmentTooLargeError).code).toBe('attachment_too_large');
    }
  });

  it('add() throws AggregateTooLargeError when adding would push past maxTotalBytes', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(4_000_000));
    list.add(mkAttachment(4_000_000));
    expect(() => list.add(mkAttachment(4_000_000))).toThrow(AggregateTooLargeError);
    expect(list.items()).toHaveLength(2);
    expect(list.aggregateSizeBytes()).toBe(8_000_000);
  });

  it('AggregateTooLargeError carries code "attachments_exceed_total"', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(8_000_000));
    try {
      list.add(mkAttachment(4_000_000));
      throw new Error('should have thrown');
    } catch (e) {
      expect(e).toBeInstanceOf(AggregateTooLargeError);
      expect((e as AggregateTooLargeError).code).toBe('attachments_exceed_total');
    }
  });

  it('add() throws MimeNotAllowedError when mime not in allowedMimeTypes', () => {
    const list = new AttachmentList(LIMITS);
    expect(() => list.add(mkAttachment(1000, 'image/heic'))).toThrow(MimeNotAllowedError);
  });

  it('MimeNotAllowedError carries code "attachment_mime_not_allowed"', () => {
    const list = new AttachmentList(LIMITS);
    try {
      list.add(mkAttachment(1000, 'image/heic'));
      throw new Error('should have thrown');
    } catch (e) {
      expect(e).toBeInstanceOf(MimeNotAllowedError);
      expect((e as MimeNotAllowedError).code).toBe('attachment_mime_not_allowed');
    }
  });

  it('boundary: single attachment at exactly maxSizeBytes is accepted', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(5_000_000));
    expect(list.items()).toHaveLength(1);
  });

  it('boundary: aggregate at exactly maxTotalBytes is accepted', () => {
    const list = new AttachmentList(LIMITS);
    list.add(mkAttachment(5_000_000));
    list.add(mkAttachment(5_000_000));
    expect(list.aggregateSizeBytes()).toBe(10_000_000);
  });

  it('remove(index) on out-of-range index throws RangeError', () => {
    const list = new AttachmentList(LIMITS);
    expect(() => list.remove(0)).toThrow(RangeError);
    list.add(mkAttachment(100));
    expect(() => list.remove(5)).toThrow(RangeError);
  });
});
```

Run:

```
cd core && npm run test -- attachments && cd ..
```

Expected: tests fail (module does not exist).

- [ ] **Step 2: Implement `core/src/attachments.ts` to pass the tests**

Create `core/src/attachments.ts`:

```ts
// Pending attachment state for the widget. Errors carry a `code` string so
// useIntake.ts can map them to user-readable banner text (Phase 6 design §8.3).

export interface PendingAttachment {
  type: 'screenshot';
  mimeType: string;
  dataUrl: string;
  label?: string;
  sizeBytes: number;
}

export interface AttachmentLimits {
  maxSizeBytes: number;
  maxTotalBytes: number;
  allowedMimeTypes: string[];
}

/**
 * AttachmentTooLargeError — single attachment exceeds maxSizeBytes.
 * Maps to relay code "attachment_too_large".
 */
export class AttachmentTooLargeError extends Error {
  readonly code = 'attachment_too_large';
  constructor(message?: string) {
    super(message ?? 'attachment exceeds max_size_bytes');
    this.name = 'AttachmentTooLargeError';
  }
}

/**
 * AggregateTooLargeError — adding an attachment would push total past maxTotalBytes.
 * Maps to relay code "attachments_exceed_total".
 */
export class AggregateTooLargeError extends Error {
  readonly code = 'attachments_exceed_total';
  constructor(message?: string) {
    super(message ?? 'attachments exceed total cap');
    this.name = 'AggregateTooLargeError';
  }
}

/**
 * MimeNotAllowedError — declared MIME not in allowedMimeTypes.
 * Maps to relay code "attachment_mime_not_allowed".
 */
export class MimeNotAllowedError extends Error {
  readonly code = 'attachment_mime_not_allowed';
  constructor(message?: string) {
    super(message ?? 'attachment mime_type not allowed');
    this.name = 'MimeNotAllowedError';
  }
}

export class AttachmentList {
  private readonly limits: AttachmentLimits;
  private list: PendingAttachment[] = [];

  constructor(limits: AttachmentLimits) {
    this.limits = limits;
  }

  add(att: PendingAttachment): void {
    if (!this.limits.allowedMimeTypes.includes(att.mimeType)) {
      throw new MimeNotAllowedError();
    }
    if (att.sizeBytes > this.limits.maxSizeBytes) {
      throw new AttachmentTooLargeError();
    }
    const projected = this.aggregateSizeBytes() + att.sizeBytes;
    if (projected > this.limits.maxTotalBytes) {
      throw new AggregateTooLargeError();
    }
    this.list = [...this.list, att];
  }

  remove(index: number): void {
    if (index < 0 || index >= this.list.length) {
      throw new RangeError(`AttachmentList.remove: index ${index} out of range`);
    }
    this.list = this.list.filter((_, i) => i !== index);
  }

  clear(): void {
    this.list = [];
  }

  items(): PendingAttachment[] {
    return this.list;
  }

  aggregateSizeBytes(): number {
    return this.list.reduce((s, a) => s + a.sizeBytes, 0);
  }
}
```

- [ ] **Step 3: Re-run tests; confirm green**

```
cd core && npm run test -- attachments && cd ..
```

Expected: all attachments tests pass.

- [ ] **Step 4: Type-check core**

```
cd core && npm run type-check && cd ..
```

Expected: clean.

- [ ] **Step 5: Commit**

```
git add core/src/attachments.ts core/src/attachments.test.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): core/src/attachments.ts — PendingAttachment + AttachmentList + named errors

Three named errors (AttachmentTooLargeError, AggregateTooLargeError,
MimeNotAllowedError) each carry a `code` string matching the relay's
ErrorEnvelope code, so useIntake.ts can map both server-side and
client-side rejections to the same user-readable banner text.
EOF
)"
```

---

### Task 5: `client.submit()` threads attachments + parses ErrorEnvelope code on non-2xx

**Files:** Modify `core/src/client.ts`, Modify `core/src/client.test.ts`

- [ ] **Step 1: Write the failing tests**

Append to `core/src/client.test.ts` (after the existing `describe` blocks):

```ts
describe('IntakeClient.submit() — attachments threading (Phase 6)', () => {
  it('omits attachments[] from the POST body when not provided', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const fetchStub = vi.fn().mockImplementation((url: string, opts: RequestInit) => {
      calls.push([url, opts]);
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () =>
          Promise.resolve({
            external_id: 'id-1',
            external_url: '',
            adapter_name: 'webhook',
            created_at: '2026-05-28T00:00:00Z',
          }),
      } as unknown as Response);
    });
    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    // Need a session_id before submit() is allowed; reuse init() flow.
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    await client.init();

    await client.submit([{ role: 'user', content: 'hi' }]);

    const submitCall = calls[calls.length - 1];
    const body = JSON.parse(submitCall[1].body as string) as Record<string, unknown>;
    expect('attachments' in body).toBe(false);
  });

  it('includes attachments[] in the POST body when non-empty', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const fetchStub = vi.fn().mockImplementation((url: string, opts: RequestInit) => {
      calls.push([url, opts]);
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () =>
          Promise.resolve({
            external_id: 'id-1',
            external_url: '',
            adapter_name: 'webhook',
            created_at: '2026-05-28T00:00:00Z',
          }),
      } as unknown as Response);
    });
    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    await client.init();

    await client.submit(
      [{ role: 'user', content: 'hi' }],
      undefined,
      [
        {
          type: 'screenshot',
          mime_type: 'image/png',
          url: 'data:image/png;base64,AAAA',
          label: 'screenshot 1',
        },
      ],
    );

    const submitCall = calls[calls.length - 1];
    const body = JSON.parse(submitCall[1].body as string) as { attachments: unknown[] };
    expect(body.attachments).toHaveLength(1);
    expect(body.attachments[0]).toMatchObject({
      type: 'screenshot',
      mime_type: 'image/png',
      url: 'data:image/png;base64,AAAA',
      label: 'screenshot 1',
    });
  });

  it('omits attachments[] when an empty array is passed', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const fetchStub = vi.fn().mockImplementation((url: string, opts: RequestInit) => {
      calls.push([url, opts]);
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () =>
          Promise.resolve({
            external_id: 'id-1',
            external_url: '',
            adapter_name: 'webhook',
            created_at: '2026-05-28T00:00:00Z',
          }),
      } as unknown as Response);
    });
    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    await client.init();

    await client.submit([{ role: 'user', content: 'hi' }], undefined, []);

    const body = JSON.parse(calls[calls.length - 1][1].body as string) as Record<string, unknown>;
    expect('attachments' in body).toBe(false);
  });

  it('non-2xx with attachment ErrorEnvelope: thrown Error carries `code` property', async () => {
    const fetchStub = vi.fn();
    // First call: init OK
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    // Second call: submit returns 413 with an ErrorEnvelope
    fetchStub.mockResolvedValueOnce({
      ok: false,
      status: 413,
      headers: { get: () => 'application/json' },
      text: () =>
        Promise.resolve(
          JSON.stringify({
            error: { code: 'attachment_too_large', message: 'attachment exceeds max_size_bytes' },
          }),
        ),
    } as unknown as Response);

    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    await client.init();
    try {
      await client.submit([{ role: 'user', content: 'hi' }]);
      throw new Error('expected submit to throw');
    } catch (e) {
      expect(e).toBeInstanceOf(Error);
      expect((e as Error & { code?: string }).code).toBe('attachment_too_large');
    }
  });

  it('non-2xx with non-JSON body: thrown Error has no `code`, retains status in message', async () => {
    const fetchStub = vi.fn();
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    fetchStub.mockResolvedValueOnce({
      ok: false,
      status: 500,
      headers: { get: () => 'text/plain' },
      text: () => Promise.resolve('internal'),
    } as unknown as Response);

    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    await client.init();
    try {
      await client.submit([{ role: 'user', content: 'hi' }]);
      throw new Error('expected submit to throw');
    } catch (e) {
      expect((e as Error & { code?: string }).code).toBeUndefined();
      expect((e as Error).message).toMatch(/500/);
    }
  });
});

describe('IntakeClient.init() — capabilities.attachments parse (Phase 6)', () => {
  it('parses capabilities.attachments when present', async () => {
    const mockFetch = makeFetch(200, {
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5242880,
          max_total_bytes: 10485760,
          allowed_mime_types: ['image/png', 'image/jpeg', 'image/webp'],
        },
      },
    });
    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    const result = await client.init();
    expect(result.capabilities.attachments).toBeDefined();
    expect(result.capabilities.attachments?.max_size_bytes).toBe(5242880);
    expect(result.capabilities.attachments?.allowed_mime_types).toEqual([
      'image/png',
      'image/jpeg',
      'image/webp',
    ]);
  });

  it('leaves capabilities.attachments undefined when relay omits the block', async () => {
    const mockFetch = makeFetch(200, {
      session_id: 'sess-1',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    const result = await client.init();
    expect(result.capabilities.attachments).toBeUndefined();
  });
});
```

Run:

```
cd core && npm run test -- client && cd ..
```

Expected: the new tests fail (submit() does not accept an attachments parameter and does not parse ErrorEnvelope code yet).

- [ ] **Step 2: Modify `core/src/client.ts`**

Apply this hunk to `core/src/client.ts`:

```diff
@@
   async submit(
     messages: ChatMessage[],
-    routingHint?: string
+    routingHint?: string,
+    attachments?: SubmitRequest['attachments'],
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
+    if (attachments !== undefined && attachments.length > 0) {
+      body.attachments = attachments;
+    }

     const url = `${this.config.relayUrl}/v1/intake/submit`;
     const submitHeaders: Record<string, string> = {
       'Content-Type': 'application/json',
       'X-Intake-Session': this.sessionId,
     };
     if (this.bearerToken !== null) {
       submitHeaders['Authorization'] = `Bearer ${this.bearerToken}`;
     }
     const res = await this.fetch(url, {
       method: 'POST',
       headers: submitHeaders,
       body: JSON.stringify(body),
     });

     if (!res.ok) {
       const body = await res.text().catch(() => '');
-      throw new Error(`submit failed: ${res.status} ${body}`);
+      // Phase 6: parse ErrorEnvelope when present and attach `code` to the
+      // thrown Error so useIntake can map it to a friendly banner string.
+      const err = new Error(`submit failed: ${res.status} ${body}`) as Error & {
+        code?: string;
+      };
+      try {
+        const parsed = JSON.parse(body) as { error?: { code?: string } };
+        if (parsed?.error?.code && typeof parsed.error.code === 'string') {
+          err.code = parsed.error.code;
+        }
+      } catch {
+        // Non-JSON body — leave `code` undefined; message retains status + body.
+      }
+      throw err;
     }

     return (await res.json()) as SubmitResponse;
   }
```

- [ ] **Step 3: Re-run tests; confirm green**

```
cd core && npm run test -- client && cd ..
```

Expected: all client tests (existing + new) pass.

- [ ] **Step 4: Type-check + build core**

```
cd core && npm run type-check && cd ..
```

Expected: clean.

- [ ] **Step 5: Commit**

```
git add core/src/client.ts core/src/client.test.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): IntakeClient.submit() threads attachments + parses ErrorEnvelope.code

submit(messages, routingHint, attachments?) — when attachments is non-empty it
is included verbatim in the POST body; empty or omitted → field absent. On
non-2xx the thrown Error gains an optional `code` property when the body parses
as an ErrorEnvelope. init() parses the new capabilities.attachments block
(additive in Phase 6 README §8.4); absent → undefined, present → typed object.
EOF
)"
```

---

### Task 6: `ScreenshotRedactor.vue` + spec (full-screen modal, rectangle redaction)

**Files:** Create `vue/src/components/ScreenshotRedactor.vue`, Create `vue/src/components/ScreenshotRedactor.spec.ts`

- [ ] **Step 1: Write the failing tests**

Create `vue/src/components/ScreenshotRedactor.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import ScreenshotRedactor from './ScreenshotRedactor.vue';

// Stub HTMLCanvasElement.prototype.getContext to return a fake 2d ctx that
// records calls and yields a deterministic toDataURL.
function installCanvasStub() {
  const ctx = {
    drawImage: vi.fn(),
    fillRect: vi.fn(),
    clearRect: vi.fn(),
    strokeRect: vi.fn(),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    setLineDash: vi.fn(),
  };
  const realGet = HTMLCanvasElement.prototype.getContext;
  HTMLCanvasElement.prototype.getContext = function () {
    return ctx as unknown as CanvasRenderingContext2D;
  } as typeof HTMLCanvasElement.prototype.getContext;

  const realToDataURL = HTMLCanvasElement.prototype.toDataURL;
  const toDataURLSpy = vi.fn().mockReturnValue('data:image/png;base64,REDACTED');
  HTMLCanvasElement.prototype.toDataURL = toDataURLSpy as unknown as typeof HTMLCanvasElement.prototype.toDataURL;

  return {
    ctx,
    toDataURLSpy,
    restore() {
      HTMLCanvasElement.prototype.getContext = realGet;
      HTMLCanvasElement.prototype.toDataURL = realToDataURL;
    },
  };
}

function makeSourceCanvas(width = 400, height = 300): HTMLCanvasElement {
  const c = document.createElement('canvas');
  c.width = width;
  c.height = height;
  return c;
}

describe('ScreenshotRedactor', () => {
  let stub: ReturnType<typeof installCanvasStub>;

  beforeEach(() => {
    stub = installCanvasStub();
  });

  afterEach(() => {
    stub.restore();
  });

  it('does not render when source is null', () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: null } });
    expect(wrapper.find('[data-testid="redactor-modal"]').exists()).toBe(false);
  });

  it('renders the modal when source is a canvas', () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() } });
    expect(wrapper.find('[data-testid="redactor-modal"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="redactor-save"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="redactor-cancel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="redactor-clear"]').exists()).toBe(true);
  });

  it('mousedown → mousemove → mouseup draws one rectangle (fillRect called on save)', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas(400, 300) } });
    const canvas = wrapper.find<HTMLCanvasElement>('[data-testid="redactor-canvas"]').element;
    canvas.dispatchEvent(new MouseEvent('mousedown', { clientX: 10, clientY: 10 }));
    canvas.dispatchEvent(new MouseEvent('mousemove', { clientX: 60, clientY: 50 }));
    canvas.dispatchEvent(new MouseEvent('mouseup', { clientX: 60, clientY: 50 }));
    await nextTick();
    // Save flattens overlay onto a copy of source.
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    // Save path uses an offscreen flatten canvas; on it, fillRect must run at
    // least once for our single rectangle.
    expect(stub.ctx.fillRect).toHaveBeenCalled();
  });

  it('Clear All button empties the rectangle array', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas(400, 300) } });
    const canvas = wrapper.find<HTMLCanvasElement>('[data-testid="redactor-canvas"]').element;
    canvas.dispatchEvent(new MouseEvent('mousedown', { clientX: 10, clientY: 10 }));
    canvas.dispatchEvent(new MouseEvent('mousemove', { clientX: 60, clientY: 50 }));
    canvas.dispatchEvent(new MouseEvent('mouseup', { clientX: 60, clientY: 50 }));
    await nextTick();
    await wrapper.find('[data-testid="redactor-clear"]').trigger('click');
    // After clear, save should produce a dataUrl but the offscreen ctx
    // must NOT have been asked to fillRect for the cleared rectangle.
    stub.ctx.fillRect.mockClear();
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    expect(stub.ctx.fillRect).not.toHaveBeenCalled();
  });

  it('Save emits save(dataUrl) with the flattened PNG data URL', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() } });
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    const emitted = wrapper.emitted('save');
    expect(emitted).toBeTruthy();
    expect(emitted?.[0]?.[0]).toBe('data:image/png;base64,REDACTED');
  });

  it('Cancel emits cancel and does NOT call toDataURL', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() } });
    stub.toDataURLSpy.mockClear();
    await wrapper.find('[data-testid="redactor-cancel"]').trigger('click');
    expect(wrapper.emitted('cancel')).toBeTruthy();
    expect(wrapper.emitted('save')).toBeUndefined();
    expect(stub.toDataURLSpy).not.toHaveBeenCalled();
  });

  it('ESC key cancels (emits cancel)', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() }, attachTo: document.body });
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await nextTick();
    expect(wrapper.emitted('cancel')).toBeTruthy();
    wrapper.unmount();
  });

  it('rectangles outside the source canvas bounds are clamped to source dimensions', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas(100, 100) } });
    const canvas = wrapper.find<HTMLCanvasElement>('[data-testid="redactor-canvas"]').element;
    // Draw a rectangle that overflows the source on the right + bottom.
    canvas.dispatchEvent(new MouseEvent('mousedown', { clientX: 80, clientY: 80 }));
    canvas.dispatchEvent(new MouseEvent('mousemove', { clientX: 200, clientY: 200 }));
    canvas.dispatchEvent(new MouseEvent('mouseup', { clientX: 200, clientY: 200 }));
    await nextTick();
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    // The last fillRect call args must be clamped: x+w ≤ 100, y+h ≤ 100.
    const calls = stub.ctx.fillRect.mock.calls;
    const last = calls[calls.length - 1] as [number, number, number, number];
    expect(last[0] + last[2]).toBeLessThanOrEqual(100);
    expect(last[1] + last[3]).toBeLessThanOrEqual(100);
  });

  it('focuses the Save button on open (basic focus trap entry)', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() }, attachTo: document.body });
    await nextTick();
    const saveBtn = wrapper.find<HTMLButtonElement>('[data-testid="redactor-save"]').element;
    expect(document.activeElement).toBe(saveBtn);
    wrapper.unmount();
  });
});
```

Run:

```
cd vue && npm run test -- ScreenshotRedactor && cd ..
```

Expected: tests fail (component does not exist).

- [ ] **Step 2: Implement `vue/src/components/ScreenshotRedactor.vue` to pass the tests**

Create `vue/src/components/ScreenshotRedactor.vue`:

```vue
<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue';

interface Rect { x: number; y: number; w: number; h: number }

const props = defineProps<{
  source: HTMLCanvasElement | null;
}>();

const emit = defineEmits<{
  (e: 'save', dataUrl: string): void;
  (e: 'cancel'): void;
}>();

const overlayRef = ref<HTMLCanvasElement | null>(null);
const saveBtnRef = ref<HTMLButtonElement | null>(null);
const modalRef = ref<HTMLDivElement | null>(null);

const rects = ref<Rect[]>([]);
const dragging = ref(false);
const dragStart = ref<{ x: number; y: number } | null>(null);
const dragCurrent = ref<{ x: number; y: number } | null>(null);

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(Math.max(v, lo), hi);
}

function clampRect(r: Rect, w: number, h: number): Rect {
  const x1 = clamp(r.x, 0, w);
  const y1 = clamp(r.y, 0, h);
  const x2 = clamp(r.x + r.w, 0, w);
  const y2 = clamp(r.y + r.h, 0, h);
  return { x: Math.min(x1, x2), y: Math.min(y1, y2), w: Math.abs(x2 - x1), h: Math.abs(y2 - y1) };
}

function repaintOverlay() {
  const overlay = overlayRef.value;
  const src = props.source;
  if (!overlay || !src) return;
  const ctx = overlay.getContext('2d');
  if (!ctx) return;
  ctx.clearRect(0, 0, overlay.width, overlay.height);
  // Draw the source image as a backdrop the user is annotating over.
  ctx.drawImage(src, 0, 0);
  // Solid black redaction overlays.
  ctx.fillStyle = '#000';
  for (const r of rects.value) {
    const c = clampRect(r, src.width, src.height);
    ctx.fillRect(c.x, c.y, c.w, c.h);
  }
  // In-progress drag preview (dashed outline).
  if (dragging.value && dragStart.value && dragCurrent.value) {
    const live: Rect = {
      x: Math.min(dragStart.value.x, dragCurrent.value.x),
      y: Math.min(dragStart.value.y, dragCurrent.value.y),
      w: Math.abs(dragCurrent.value.x - dragStart.value.x),
      h: Math.abs(dragCurrent.value.y - dragStart.value.y),
    };
    ctx.strokeStyle = '#000';
    ctx.lineWidth = 1;
    ctx.setLineDash([4, 4]);
    ctx.strokeRect(live.x, live.y, live.w, live.h);
    ctx.setLineDash([]);
  }
}

function onMouseDown(ev: MouseEvent) {
  const overlay = overlayRef.value;
  if (!overlay) return;
  const rect = overlay.getBoundingClientRect();
  const x = ev.clientX - rect.left;
  const y = ev.clientY - rect.top;
  dragging.value = true;
  dragStart.value = { x, y };
  dragCurrent.value = { x, y };
}

function onMouseMove(ev: MouseEvent) {
  if (!dragging.value) return;
  const overlay = overlayRef.value;
  if (!overlay) return;
  const rect = overlay.getBoundingClientRect();
  dragCurrent.value = { x: ev.clientX - rect.left, y: ev.clientY - rect.top };
  repaintOverlay();
}

function onMouseUp(ev: MouseEvent) {
  if (!dragging.value || !dragStart.value) return;
  const overlay = overlayRef.value;
  if (!overlay) return;
  const rect = overlay.getBoundingClientRect();
  const endX = ev.clientX - rect.left;
  const endY = ev.clientY - rect.top;
  const r: Rect = {
    x: Math.min(dragStart.value.x, endX),
    y: Math.min(dragStart.value.y, endY),
    w: Math.abs(endX - dragStart.value.x),
    h: Math.abs(endY - dragStart.value.y),
  };
  if (r.w > 0 && r.h > 0) {
    rects.value = [...rects.value, r];
  }
  dragging.value = false;
  dragStart.value = null;
  dragCurrent.value = null;
  repaintOverlay();
}

function onClearAll() {
  rects.value = [];
  repaintOverlay();
}

function onSave() {
  const src = props.source;
  if (!src) {
    emit('cancel');
    return;
  }
  // Flatten redactions onto an offscreen copy of source, then emit data URL.
  const out = document.createElement('canvas');
  out.width = src.width;
  out.height = src.height;
  const ctx = out.getContext('2d');
  if (!ctx) {
    emit('cancel');
    return;
  }
  ctx.drawImage(src, 0, 0);
  ctx.fillStyle = '#000';
  for (const r of rects.value) {
    const c = clampRect(r, src.width, src.height);
    if (c.w > 0 && c.h > 0) {
      ctx.fillRect(c.x, c.y, c.w, c.h);
    }
  }
  emit('save', out.toDataURL('image/png'));
}

function onCancel() {
  emit('cancel');
}

function onKeyDown(ev: KeyboardEvent) {
  if (ev.key === 'Escape') {
    ev.preventDefault();
    emit('cancel');
    return;
  }
  // Basic focus trap: keep Tab inside the modal.
  if (ev.key === 'Tab' && modalRef.value) {
    const focusables = modalRef.value.querySelectorAll<HTMLElement>(
      'button, [tabindex]:not([tabindex="-1"])',
    );
    if (focusables.length === 0) return;
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    if (ev.shiftKey && document.activeElement === first) {
      ev.preventDefault();
      last.focus();
    } else if (!ev.shiftKey && document.activeElement === last) {
      ev.preventDefault();
      first.focus();
    }
  }
}

watch(
  () => props.source,
  async (src) => {
    if (!src) return;
    rects.value = [];
    await nextTick();
    const overlay = overlayRef.value;
    if (overlay) {
      overlay.width = src.width;
      overlay.height = src.height;
      repaintOverlay();
    }
    saveBtnRef.value?.focus();
  },
  { immediate: true },
);

onMounted(() => {
  document.addEventListener('keydown', onKeyDown);
});

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeyDown);
});
</script>

<template>
  <div
    v-if="source !== null"
    ref="modalRef"
    class="redactor"
    role="dialog"
    aria-modal="true"
    aria-label="Redact screenshot"
    data-testid="redactor-modal"
  >
    <div class="redactor__backdrop" @click="onCancel" />
    <div class="redactor__panel" @click.stop>
      <div class="redactor__header">
        <span class="redactor__title">Redact screenshot</span>
        <span class="redactor__hint">Drag to draw black boxes over sensitive areas.</span>
      </div>
      <div class="redactor__canvas-wrap">
        <canvas
          ref="overlayRef"
          class="redactor__canvas"
          data-testid="redactor-canvas"
          @mousedown="onMouseDown"
          @mousemove="onMouseMove"
          @mouseup="onMouseUp"
        />
      </div>
      <div class="redactor__actions">
        <button
          type="button"
          class="redactor__btn redactor__btn--ghost"
          data-testid="redactor-clear"
          @click="onClearAll"
        >
          Clear all
        </button>
        <button
          type="button"
          class="redactor__btn redactor__btn--ghost"
          data-testid="redactor-cancel"
          @click="onCancel"
        >
          Cancel
        </button>
        <button
          ref="saveBtnRef"
          type="button"
          class="redactor__btn redactor__btn--primary"
          data-testid="redactor-save"
          @click="onSave"
        >
          Save
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.redactor {
  position: fixed;
  inset: 0;
  z-index: 100000;
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: system-ui, sans-serif;
}
.redactor__backdrop {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
}
.redactor__panel {
  position: relative;
  background: #fff;
  border-radius: 8px;
  padding: 16px;
  max-width: 90vw;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.3);
}
.redactor__header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.redactor__title {
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}
.redactor__hint {
  font-size: 12px;
  color: #64748b;
}
.redactor__canvas-wrap {
  overflow: auto;
  max-height: 70vh;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
}
.redactor__canvas {
  display: block;
  cursor: crosshair;
}
.redactor__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.redactor__btn {
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border: 1px solid transparent;
}
.redactor__btn--ghost {
  background: #fff;
  color: #334155;
  border-color: #cbd5e1;
}
.redactor__btn--primary {
  background: #2563eb;
  color: #fff;
}
</style>
```

- [ ] **Step 3: Re-run tests; confirm green**

```
cd vue && npm run test -- ScreenshotRedactor && cd ..
```

Expected: all ScreenshotRedactor tests pass.

- [ ] **Step 4: Type-check + build vue**

```
cd vue && npm run type-check && cd ..
```

Expected: clean.

- [ ] **Step 5: Commit**

```
git add vue/src/components/ScreenshotRedactor.vue vue/src/components/ScreenshotRedactor.spec.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): ScreenshotRedactor.vue — full-screen rectangle redaction modal

Closed when source prop is null; opens with focus on Save; mouse drag draws
solid black redaction rectangles; Clear resets; Save flattens onto an offscreen
copy of source and emits a PNG data URL; Cancel + ESC + backdrop click all
emit cancel without calling toDataURL. Rectangles clamp to source bounds.
EOF
)"
```

---

### Task 7: `AttachmentStrip.vue` + spec (thumbnails + remove + aggregate badge)

**Files:** Create `vue/src/components/AttachmentStrip.vue`, Create `vue/src/components/AttachmentStrip.spec.ts`

- [ ] **Step 1: Write the failing tests**

Create `vue/src/components/AttachmentStrip.spec.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import AttachmentStrip from './AttachmentStrip.vue';
import type { PendingAttachment } from '@intake/core';

function mkAtt(sizeBytes: number, label?: string): PendingAttachment {
  return {
    type: 'screenshot',
    mimeType: 'image/png',
    dataUrl: 'data:image/png;base64,AAAA',
    label,
    sizeBytes,
  };
}

describe('AttachmentStrip', () => {
  it('is hidden when items is empty', () => {
    const wrapper = mount(AttachmentStrip, {
      props: { items: [], maxTotalBytes: 10_000_000 },
    });
    expect(wrapper.find('[data-testid="attachment-strip"]').exists()).toBe(false);
  });

  it('renders one thumbnail per item', () => {
    const wrapper = mount(AttachmentStrip, {
      props: {
        items: [mkAtt(1000), mkAtt(2000), mkAtt(3000)],
        maxTotalBytes: 10_000_000,
      },
    });
    const thumbs = wrapper.findAll('[data-testid="attachment-thumb"]');
    expect(thumbs).toHaveLength(3);
  });

  it('clicking remove on item N emits remove(N)', async () => {
    const wrapper = mount(AttachmentStrip, {
      props: {
        items: [mkAtt(1000), mkAtt(2000), mkAtt(3000)],
        maxTotalBytes: 10_000_000,
      },
    });
    const removes = wrapper.findAll('[data-testid="attachment-remove"]');
    await removes[1].trigger('click');
    expect(wrapper.emitted('remove')).toBeTruthy();
    expect(wrapper.emitted('remove')?.[0]?.[0]).toBe(1);
  });

  it('aggregate-size badge shows human-readable used/total (MB)', () => {
    const wrapper = mount(AttachmentStrip, {
      props: {
        items: [mkAtt(4_200_000)],
        maxTotalBytes: 10_485_760,
      },
    });
    const badge = wrapper.find('[data-testid="attachment-aggregate"]');
    expect(badge.exists()).toBe(true);
    // "4.2 MB / 10.0 MB" or similar; assert both numbers + "MB" appear.
    const text = badge.text();
    expect(text).toMatch(/4\.\d\s*MB/);
    expect(text).toMatch(/10(\.\d)?\s*MB/);
  });

  it('aggregate-size badge handles tiny payloads in KB', () => {
    const wrapper = mount(AttachmentStrip, {
      props: { items: [mkAtt(2_000)], maxTotalBytes: 10_000 },
    });
    const text = wrapper.find('[data-testid="attachment-aggregate"]').text();
    expect(text).toMatch(/2\.\d\s*KB/);
    expect(text).toMatch(/9\.\d\s*KB/);
  });
});
```

Run:

```
cd vue && npm run test -- AttachmentStrip && cd ..
```

Expected: tests fail (component does not exist).

- [ ] **Step 2: Implement `vue/src/components/AttachmentStrip.vue`**

Create `vue/src/components/AttachmentStrip.vue`:

```vue
<script setup lang="ts">
import { computed } from 'vue';
import type { PendingAttachment } from '@intake/core';

const props = defineProps<{
  items: PendingAttachment[];
  maxTotalBytes: number;
}>();

const emit = defineEmits<{
  (e: 'remove', index: number): void;
}>();

const aggregate = computed(() => props.items.reduce((s, a) => s + a.sizeBytes, 0));

function humanBytes(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_048_576).toFixed(1)} MB`;
  if (n >= 1_000) return `${(n / 1_024).toFixed(1)} KB`;
  return `${n} B`;
}

const badgeText = computed(() => `${humanBytes(aggregate.value)} / ${humanBytes(props.maxTotalBytes)}`);

function onRemove(i: number) {
  emit('remove', i);
}
</script>

<template>
  <div v-if="items.length > 0" class="strip" data-testid="attachment-strip">
    <ul class="strip__list">
      <li
        v-for="(att, i) in items"
        :key="i"
        class="strip__item"
        data-testid="attachment-thumb"
      >
        <img
          :src="att.dataUrl"
          :alt="att.label ?? `screenshot ${i + 1}`"
          class="strip__thumb"
        />
        <button
          type="button"
          class="strip__remove"
          :aria-label="`Remove attachment ${i + 1}`"
          data-testid="attachment-remove"
          @click="onRemove(i)"
        >
          ×
        </button>
      </li>
    </ul>
    <div class="strip__badge" data-testid="attachment-aggregate">
      {{ badgeText }}
    </div>
  </div>
</template>

<style scoped>
.strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-top: 1px solid #e2e8f0;
  background: #f8fafc;
  flex-shrink: 0;
}
.strip__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  gap: 6px;
  overflow-x: auto;
}
.strip__item {
  position: relative;
  width: 48px;
  height: 48px;
  border-radius: 4px;
  overflow: hidden;
  border: 1px solid #cbd5e1;
  flex-shrink: 0;
}
.strip__thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.strip__remove {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 18px;
  height: 18px;
  border-radius: 9px;
  border: none;
  background: rgba(15, 23, 42, 0.75);
  color: #fff;
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}
.strip__badge {
  font-size: 12px;
  color: #475569;
  margin-left: 12px;
  flex-shrink: 0;
}
</style>
```

- [ ] **Step 3: Re-run tests; confirm green**

```
cd vue && npm run test -- AttachmentStrip && cd ..
```

Expected: all AttachmentStrip tests pass.

- [ ] **Step 4: Type-check vue**

```
cd vue && npm run type-check && cd ..
```

Expected: clean. (Will fail until `PendingAttachment` is re-exported from `@intake/core` — do that now if not already done in core's `index.ts`. Task 8 covers it for the composable; for the strict order here, also update `core/src/index.ts` now.)

Apply this hunk to `core/src/index.ts`:

```diff
@@
 // Context capture utilities (exported for widget use)
 export { captureClient, capturePageMetadata } from './context.js';
+
+// Phase 6 — attachments + capture
+export {
+  AttachmentList,
+  AttachmentTooLargeError,
+  AggregateTooLargeError,
+  MimeNotAllowedError,
+} from './attachments.js';
+export type { PendingAttachment, AttachmentLimits } from './attachments.js';
+export { setHtml2Canvas, capturePage, canvasToDataURL } from './capture.js';
+export type { Html2CanvasFn } from './capture.js';
```

Re-run `cd core && npm run type-check && cd ..` and `cd vue && npm run type-check && cd ..` — both expected clean.

- [ ] **Step 5: Commit**

```
git add vue/src/components/AttachmentStrip.vue vue/src/components/AttachmentStrip.spec.ts core/src/index.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): AttachmentStrip.vue + re-export PendingAttachment/AttachmentList/capture from @intake/core

Strip renders one img-thumbnail per pending attachment + a remove button +
an aggregate-size badge ("N.N MB / M.M MB"); hidden entirely when items is
empty. PendingAttachment and the capture/list surface are re-exported from
@intake/core so vue/ can consume them via the package surface.
EOF
)"
```

---

### Task 8: `useIntake.ts` extensions (pending state, canAttach, attachAndRedact, commitRedacted, removeAttachment, clearAttachments, submit threading, error-code mapping)

**Files:** Modify `vue/src/composables/useIntake.ts`, Modify `vue/src/composables/useIntake.spec.ts`

- [ ] **Step 1: Write the failing tests**

Apply this hunk to `vue/src/composables/useIntake.spec.ts` (append new `describe` block at the bottom):

```diff
@@
   it('start() rejection: sets error and re-throws', async () => {
     mockInit.mockRejectedValue(new Error('relay down'));

     const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
     await expect(intake.start()).rejects.toThrow('relay down');
     expect(intake.error.value).toBeTruthy();
   });
 });
+
+describe('useIntake — Phase 6 attachments surface', () => {
+  beforeEach(() => {
+    vi.clearAllMocks();
+  });
+
+  it('canAttach is false before start() and when capabilities.attachments is null', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: { auth_modes: ['anonymous'], streaming: true },
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    expect(intake.canAttach.value).toBe(false);
+    expect(intake.attachLimits.value).toBeNull();
+    await intake.start();
+    expect(intake.canAttach.value).toBe(false);
+    expect(intake.attachLimits.value).toBeNull();
+  });
+
+  it('canAttach is true and attachLimits populated when capabilities.attachments present', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 5_242_880,
+          max_total_bytes: 10_485_760,
+          allowed_mime_types: ['image/png', 'image/jpeg', 'image/webp'],
+        },
+      },
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    expect(intake.canAttach.value).toBe(true);
+    expect(intake.attachLimits.value).toEqual({
+      maxSizeBytes: 5_242_880,
+      maxTotalBytes: 10_485_760,
+      allowedMimeTypes: ['image/png', 'image/jpeg', 'image/webp'],
+    });
+  });
+
+  it('commitRedacted appends to pendingAttachments with size accounting', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 5_242_880,
+          max_total_bytes: 10_485_760,
+          allowed_mime_types: ['image/png'],
+        },
+      },
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    // base64 payload of 4 chars decodes to 3 bytes — easy assertion target.
+    intake.commitRedacted('data:image/png;base64,AAAA');
+    expect(intake.pendingAttachments.value).toHaveLength(1);
+    expect(intake.pendingAttachments.value[0].mimeType).toBe('image/png');
+    expect(intake.pendingAttachments.value[0].sizeBytes).toBeGreaterThan(0);
+  });
+
+  it('commitRedacted on an over-cap dataUrl sets a friendly error and does NOT append', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 4, // 4 bytes — anything non-tiny exceeds it
+          max_total_bytes: 1_000,
+          allowed_mime_types: ['image/png'],
+        },
+      },
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    // ~600-byte base64 payload — well over 4-byte cap.
+    const big = 'data:image/png;base64,' + 'A'.repeat(800);
+    intake.commitRedacted(big);
+    expect(intake.pendingAttachments.value).toHaveLength(0);
+    expect(intake.error.value).toMatch(/too large|smaller region/i);
+  });
+
+  it('removeAttachment removes by index', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 5_242_880,
+          max_total_bytes: 10_485_760,
+          allowed_mime_types: ['image/png'],
+        },
+      },
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    intake.commitRedacted('data:image/png;base64,AAAA');
+    intake.commitRedacted('data:image/png;base64,BBBB');
+    intake.removeAttachment(0);
+    expect(intake.pendingAttachments.value).toHaveLength(1);
+    expect(intake.pendingAttachments.value[0].dataUrl).toBe('data:image/png;base64,BBBB');
+  });
+
+  it('clearAttachments empties the list', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 5_242_880,
+          max_total_bytes: 10_485_760,
+          allowed_mime_types: ['image/png'],
+        },
+      },
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    intake.commitRedacted('data:image/png;base64,AAAA');
+    intake.clearAttachments();
+    expect(intake.pendingAttachments.value).toEqual([]);
+  });
+
+  it('submit() threads pendingAttachments into client.submit and clears on success', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 5_242_880,
+          max_total_bytes: 10_485_760,
+          allowed_mime_types: ['image/png'],
+        },
+      },
+    });
+    mockSubmit.mockResolvedValue({
+      external_id: 't-1',
+      external_url: '',
+      adapter_name: 'webhook',
+      created_at: '2026-05-28T00:00:00Z',
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    intake.commitRedacted('data:image/png;base64,AAAA');
+    await intake.submit();
+    const [, , attachmentsArg] = mockSubmit.mock.calls[0] as [unknown, unknown, unknown[]];
+    expect(attachmentsArg).toHaveLength(1);
+    expect((attachmentsArg[0] as { mime_type: string }).mime_type).toBe('image/png');
+    expect(intake.pendingAttachments.value).toEqual([]);
+  });
+
+  it('submit() does NOT clear pending list on failure', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 5_242_880,
+          max_total_bytes: 10_485_760,
+          allowed_mime_types: ['image/png'],
+        },
+      },
+    });
+    const err = new Error('submit failed: 413 ...') as Error & { code?: string };
+    err.code = 'attachment_too_large';
+    mockSubmit.mockRejectedValue(err);
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    intake.commitRedacted('data:image/png;base64,AAAA');
+    await intake.submit();
+    expect(intake.pendingAttachments.value).toHaveLength(1);
+    expect(intake.error.value).toBe('Screenshot too large — try a smaller region.');
+  });
+
+  it('submit() maps each known relay error code to its friendly banner string', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: { auth_modes: ['anonymous'], streaming: true },
+    });
+    const cases: Array<[string, RegExp]> = [
+      ['attachment_too_large', /smaller region/i],
+      ['attachments_exceed_total', /remove one/i],
+      ['attachment_mime_not_allowed', /isn'?t supported/i],
+      ['attachment_mime_mismatch', /couldn'?t be verified/i],
+      ['attachment_malformed', /couldn'?t be verified/i],
+      ['attachment_type_unsupported', /isn'?t supported/i],
+      ['attachments_disabled', /disabled on this server/i],
+      ['request_body_too_large', /too large to send/i],
+    ];
+    for (const [code, pattern] of cases) {
+      const err = new Error('boom') as Error & { code?: string };
+      err.code = code;
+      mockSubmit.mockRejectedValueOnce(err);
+      const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+      await intake.start();
+      await intake.submit();
+      expect(intake.error.value).toMatch(pattern);
+    }
+  });
+
+  it('submit() with an unknown error code falls back to the raw error message', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: { auth_modes: ['anonymous'], streaming: true },
+    });
+    const err = new Error('original message') as Error & { code?: string };
+    err.code = 'some_future_unknown_code';
+    mockSubmit.mockRejectedValue(err);
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    await intake.submit();
+    expect(intake.error.value).toBe('original message');
+  });
+
+  it('attachAndRedact opens redactorSource and commitRedacted closes it', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 5_242_880,
+          max_total_bytes: 10_485_760,
+          allowed_mime_types: ['image/png'],
+        },
+      },
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    const fakeCanvas = { width: 10, height: 10 } as unknown as HTMLCanvasElement;
+    // Inject a stub capturePage via the composable's option.
+    await intake.attachAndRedact(async () => fakeCanvas);
+    expect(intake.redactorSource.value).toBe(fakeCanvas);
+    intake.commitRedacted('data:image/png;base64,AAAA');
+    expect(intake.redactorSource.value).toBeNull();
+  });
+
+  it('cancelRedactor() closes the modal without committing', async () => {
+    mockInit.mockResolvedValue({
+      session_id: 'sess-1',
+      capabilities: {
+        auth_modes: ['anonymous'],
+        streaming: true,
+        attachments: {
+          max_size_bytes: 5_242_880,
+          max_total_bytes: 10_485_760,
+          allowed_mime_types: ['image/png'],
+        },
+      },
+    });
+    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
+    await intake.start();
+    const fakeCanvas = { width: 10, height: 10 } as unknown as HTMLCanvasElement;
+    await intake.attachAndRedact(async () => fakeCanvas);
+    intake.cancelRedactor();
+    expect(intake.redactorSource.value).toBeNull();
+    expect(intake.pendingAttachments.value).toEqual([]);
+  });
+});
```

Run:

```
cd vue && npm run test -- useIntake && cd ..
```

Expected: new tests fail (the new surface does not exist yet).

- [ ] **Step 2: Rewrite `vue/src/composables/useIntake.ts` with the extended surface**

Replace `vue/src/composables/useIntake.ts` with:

```ts
import { ref, computed, type Ref, type ComputedRef } from 'vue';
import {
  IntakeClient,
  AttachmentList,
  AttachmentTooLargeError,
  AggregateTooLargeError,
  MimeNotAllowedError,
} from '@intake/core';
import type {
  IntakeConfig,
  ChatMessage,
  SubmitResult,
  PendingAttachment,
  AttachmentLimits,
} from '@intake/core';
import type { InitResponse } from '@intake/core';

// Security invariant: the widget NEVER handles provider API keys.
// It only calls the relay through @intake/core's IntakeClient.
// No code path in this file contacts Anthropic or any LLM provider directly.

export interface UseIntakeOptions {
  relayUrl: string;
  widgetVersion?: string;
  appContext?: Record<string, unknown>;
}

// Phase 6 §8.3 — relay error-code → user-readable banner text mapping.
const ATTACHMENT_ERROR_MESSAGES: Record<string, string> = {
  attachment_too_large: 'Screenshot too large — try a smaller region.',
  attachments_exceed_total: 'Too many attachments — remove one.',
  attachment_mime_not_allowed: "This attachment type isn't supported.",
  attachment_mime_mismatch: "This attachment couldn't be verified — try recapturing.",
  attachment_malformed: "This attachment couldn't be verified — try recapturing.",
  attachment_type_unsupported: "This attachment type isn't supported.",
  attachments_disabled: 'Attachments are disabled on this server.',
  request_body_too_large: 'Your submission is too large to send.',
};

function friendlyErrorMessage(e: unknown): string {
  if (e instanceof Error) {
    const code = (e as Error & { code?: string }).code;
    if (typeof code === 'string' && code in ATTACHMENT_ERROR_MESSAGES) {
      return ATTACHMENT_ERROR_MESSAGES[code];
    }
    return e.message;
  }
  return String(e);
}

// Approximate raw byte size of a data:<mime>;base64,<payload> URL.
function approxBytesFromDataUrl(dataUrl: string): number {
  const comma = dataUrl.indexOf(',');
  if (comma < 0) return 0;
  const payload = dataUrl.slice(comma + 1);
  // base64 encodes 3 bytes per 4 chars; subtract padding.
  const pad = payload.endsWith('==') ? 2 : payload.endsWith('=') ? 1 : 0;
  return Math.floor((payload.length * 3) / 4) - pad;
}

function mimeFromDataUrl(dataUrl: string): string {
  const m = dataUrl.match(/^data:([^;]+);base64,/);
  return m ? m[1] : 'application/octet-stream';
}

export function useIntake(options: UseIntakeOptions) {
  const config: IntakeConfig = {
    relayUrl: options.relayUrl,
    widgetVersion: options.widgetVersion ?? '0.1.0',
    appContext: options.appContext,
  };

  const client = new IntakeClient(config);

  const messages = ref<ChatMessage[]>([]);
  const streaming = ref(false);
  const submitting = ref(false);
  const result = ref<SubmitResult | null>(null);
  const error = ref<string | null>(null);
  const initResponse = ref<InitResponse | null>(null);

  // Phase 6 — attachments state.
  const pendingAttachments = ref<PendingAttachment[]>([]);
  const redactorSource: Ref<HTMLCanvasElement | null> = ref(null);
  let attachmentList: AttachmentList | null = null;

  const attachLimits: ComputedRef<AttachmentLimits | null> = computed(() => {
    const caps = initResponse.value?.capabilities.attachments;
    if (!caps) return null;
    return {
      maxSizeBytes: caps.max_size_bytes,
      maxTotalBytes: caps.max_total_bytes,
      allowedMimeTypes: caps.allowed_mime_types,
    };
  });

  const canAttach: ComputedRef<boolean> = computed(() => attachLimits.value !== null);

  function ensureList(): AttachmentList | null {
    if (attachmentList) return attachmentList;
    const limits = attachLimits.value;
    if (!limits) return null;
    attachmentList = new AttachmentList(limits);
    return attachmentList;
  }

  async function start() {
    error.value = null;
    try {
      const res = await client.init();
      initResponse.value = res;
      // Reset the list whenever caps change (fresh session).
      attachmentList = null;
      return res;
    } catch (e) {
      error.value = "Couldn't connect. Please try again.";
      throw e;
    }
  }

  async function sendTurn(text: string) {
    messages.value = [...messages.value, { role: 'user', content: text }];
    const assistantIndex = messages.value.length;
    messages.value = [...messages.value, { role: 'assistant', content: '' }];

    streaming.value = true;
    error.value = null;

    try {
      await client.turn(messages.value.slice(0, assistantIndex), (delta: string) => {
        const updated = [...messages.value];
        updated[assistantIndex] = {
          role: 'assistant',
          content: updated[assistantIndex].content + delta,
        };
        messages.value = updated;
      });
    } catch (e) {
      error.value = friendlyErrorMessage(e);
      const last = messages.value[messages.value.length - 1];
      if (last && last.role === 'assistant' && last.content === '') {
        messages.value = messages.value.slice(0, -1);
      }
    } finally {
      streaming.value = false;
    }
  }

  /**
   * Runs the supplied capture function (default: dynamically imports
   * @intake/core's capturePage) and opens the redactor modal by setting
   * redactorSource. Tests pass a stub capture; production passes the
   * real capturePage from @intake/core.
   */
  async function attachAndRedact(capture: () => Promise<HTMLCanvasElement>) {
    if (!canAttach.value) {
      error.value = ATTACHMENT_ERROR_MESSAGES.attachments_disabled;
      return;
    }
    error.value = null;
    try {
      const canvas = await capture();
      redactorSource.value = canvas;
    } catch (e) {
      error.value = friendlyErrorMessage(e);
    }
  }

  /**
   * Closes the redactor modal without committing.
   */
  function cancelRedactor() {
    redactorSource.value = null;
  }

  /**
   * Commits a redacted PNG data URL to the pending list and closes the modal.
   * Translates AttachmentList errors via the same code mapping the relay uses.
   */
  function commitRedacted(dataUrl: string) {
    const list = ensureList();
    if (!list) {
      error.value = ATTACHMENT_ERROR_MESSAGES.attachments_disabled;
      redactorSource.value = null;
      return;
    }
    const sizeBytes = approxBytesFromDataUrl(dataUrl);
    const mimeType = mimeFromDataUrl(dataUrl);
    const att: PendingAttachment = {
      type: 'screenshot',
      mimeType,
      dataUrl,
      sizeBytes,
      label: `screenshot ${pendingAttachments.value.length + 1}`,
    };
    try {
      list.add(att);
      pendingAttachments.value = [...list.items()];
      error.value = null;
    } catch (e) {
      if (
        e instanceof AttachmentTooLargeError ||
        e instanceof AggregateTooLargeError ||
        e instanceof MimeNotAllowedError
      ) {
        error.value = ATTACHMENT_ERROR_MESSAGES[e.code] ?? e.message;
      } else {
        error.value = friendlyErrorMessage(e);
      }
    } finally {
      redactorSource.value = null;
    }
  }

  function removeAttachment(index: number) {
    const list = ensureList();
    if (!list) return;
    try {
      list.remove(index);
      pendingAttachments.value = [...list.items()];
    } catch (e) {
      error.value = friendlyErrorMessage(e);
    }
  }

  function clearAttachments() {
    const list = ensureList();
    if (!list) {
      pendingAttachments.value = [];
      return;
    }
    list.clear();
    pendingAttachments.value = [];
  }

  async function submit(routingHint?: string) {
    submitting.value = true;
    error.value = null;
    try {
      const wireAttachments = pendingAttachments.value.map((a) => ({
        type: 'screenshot' as const,
        mime_type: a.mimeType,
        url: a.dataUrl,
        ...(a.label !== undefined ? { label: a.label } : {}),
      }));
      result.value = await client.submit(
        messages.value,
        routingHint,
        wireAttachments.length > 0 ? wireAttachments : undefined,
      );
      // Clear on success only.
      const list = ensureList();
      if (list) list.clear();
      pendingAttachments.value = [];
    } catch (e) {
      error.value = friendlyErrorMessage(e);
    } finally {
      submitting.value = false;
    }
  }

  return {
    messages,
    streaming,
    submitting,
    result,
    error,
    initResponse,
    pendingAttachments,
    redactorSource,
    canAttach,
    attachLimits,
    start,
    sendTurn,
    submit,
    attachAndRedact,
    cancelRedactor,
    commitRedacted,
    removeAttachment,
    clearAttachments,
  };
}
```

- [ ] **Step 3: Re-run tests; confirm green**

```
cd vue && npm run test -- useIntake && cd ..
```

Expected: all existing + new useIntake tests pass.

- [ ] **Step 4: Type-check + build vue**

```
cd vue && npm run type-check && cd ..
```

Expected: clean.

- [ ] **Step 5: Commit**

```
git add vue/src/composables/useIntake.ts vue/src/composables/useIntake.spec.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): useIntake — pendingAttachments + canAttach + attachAndRedact + error-code mapping

Adds pendingAttachments ref, attachLimits + canAttach computed (driven by
init's capabilities.attachments), redactorSource state ref, attachAndRedact
(capture injection point — tests pass a stub, production passes capturePage),
commitRedacted (validates via AttachmentList + maps errors to friendly text),
cancelRedactor, removeAttachment, clearAttachments. submit() threads
pendingAttachments into client.submit and clears the list on success only.
All known relay attachment-error codes map to the design §8.3 banner strings;
unknown codes fall back to the raw Error.message.
EOF
)"
```

---

### Task 9: `IntakeWidget.vue` wiring (Attach button, modal, strip) + spec

**Files:** Modify `vue/src/components/IntakeWidget.vue`, Create `vue/src/components/IntakeWidget.spec.ts`

- [ ] **Step 1: Write the failing tests**

Create `vue/src/components/IntakeWidget.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import IntakeWidget from './IntakeWidget.vue';

const mockInit = vi.fn();
const mockTurn = vi.fn();
const mockSubmit = vi.fn();

vi.mock('@intake/core', async (orig) => {
  const actual = (await orig()) as Record<string, unknown>;
  function IntakeClient() {
    return { init: mockInit, turn: mockTurn, submit: mockSubmit };
  }
  return { ...actual, IntakeClient };
});

// Stub the capture module so clicking Attach doesn't load real html2canvas.
vi.mock('@intake/core', async (orig) => {
  const actual = (await orig()) as Record<string, unknown>;
  function IntakeClient() {
    return { init: mockInit, turn: mockTurn, submit: mockSubmit };
  }
  const fakeCanvas = (() => {
    const c = { width: 200, height: 100 } as unknown as HTMLCanvasElement;
    return c;
  })();
  return {
    ...actual,
    IntakeClient,
    capturePage: vi.fn().mockResolvedValue(fakeCanvas),
  };
});

const CAPS_DISABLED = {
  session_id: 'sess-1',
  capabilities: { auth_modes: ['anonymous'], streaming: true },
};

const CAPS_ENABLED = {
  session_id: 'sess-1',
  capabilities: {
    auth_modes: ['anonymous'],
    streaming: true,
    attachments: {
      max_size_bytes: 5_242_880,
      max_total_bytes: 10_485_760,
      allowed_mime_types: ['image/png', 'image/jpeg', 'image/webp'],
    },
  },
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe('IntakeWidget Attach button visibility', () => {
  it('Attach button is hidden when capabilities.attachments is absent', async () => {
    mockInit.mockResolvedValue(CAPS_DISABLED);
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');
    expect(wrapper.find('[data-testid="attach-button"]').exists()).toBe(false);
  });

  it('Attach button is visible when capabilities.attachments is present', async () => {
    mockInit.mockResolvedValue(CAPS_ENABLED);
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');
    expect(wrapper.find('[data-testid="attach-button"]').exists()).toBe(true);
  });
});

describe('IntakeWidget Attach → modal → strip → Submit flow', () => {
  it('clicking Attach opens the ScreenshotRedactor modal with the captured canvas', async () => {
    mockInit.mockResolvedValue(CAPS_ENABLED);
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');
    await wrapper.find('[data-testid="attach-button"]').trigger('click');
    await flushPromises();
    expect(wrapper.find('[data-testid="redactor-modal"]').exists()).toBe(true);
  });

  it('saving the redactor pushes into the pending list, strip becomes visible', async () => {
    mockInit.mockResolvedValue(CAPS_ENABLED);
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');
    await wrapper.find('[data-testid="attach-button"]').trigger('click');
    await flushPromises();

    // Stub toDataURL so Save emits a deterministic value.
    HTMLCanvasElement.prototype.toDataURL = vi
      .fn()
      .mockReturnValue('data:image/png;base64,AAAA') as unknown as typeof HTMLCanvasElement.prototype.toDataURL;

    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    await flushPromises();

    expect(wrapper.find('[data-testid="redactor-modal"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="attachment-strip"]').exists()).toBe(true);
    expect(wrapper.findAll('[data-testid="attachment-thumb"]')).toHaveLength(1);
  });

  it('Submit threads attachments through to client.submit', async () => {
    mockInit.mockResolvedValue(CAPS_ENABLED);
    mockSubmit.mockResolvedValue({
      external_id: 't-1',
      external_url: '',
      adapter_name: 'webhook',
      created_at: '2026-05-28T00:00:00Z',
    });
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');

    // Get a message in so Submit is enabled.
    await wrapper.find('[data-testid="message-input"]').setValue('hi');
    mockTurn.mockResolvedValue({ input_tokens: 1, output_tokens: 1 });
    await wrapper.find('[data-testid="send-button"]').trigger('click');
    await flushPromises();

    await wrapper.find('[data-testid="attach-button"]').trigger('click');
    await flushPromises();
    HTMLCanvasElement.prototype.toDataURL = vi
      .fn()
      .mockReturnValue('data:image/png;base64,AAAA') as unknown as typeof HTMLCanvasElement.prototype.toDataURL;
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    await flushPromises();

    await wrapper.find('[data-testid="submit-button"]').trigger('click');
    await flushPromises();

    expect(mockSubmit).toHaveBeenCalledOnce();
    const args = mockSubmit.mock.calls[0] as [unknown, unknown, unknown[]];
    expect(args[2]).toHaveLength(1);
    expect((args[2][0] as { mime_type: string }).mime_type).toBe('image/png');
  });
});
```

Run:

```
cd vue && npm run test -- IntakeWidget && cd ..
```

Expected: tests fail (Attach button + strip + modal not wired yet).

- [ ] **Step 2: Modify `vue/src/components/IntakeWidget.vue` to wire the Attach button, modal, and strip**

Apply this hunk to `vue/src/components/IntakeWidget.vue`:

```diff
@@
 <script setup lang="ts">
 // Security invariant: this widget NEVER handles provider API keys.
 // It only calls the relay through @intake/core's IntakeClient.
 // All LLM calls happen inside the relay process — never from this browser widget.

 import { ref, onMounted } from 'vue';
+import { capturePage } from '@intake/core';
 import ConversationView from './ConversationView.vue';
+import ScreenshotRedactor from './ScreenshotRedactor.vue';
+import AttachmentStrip from './AttachmentStrip.vue';
 import { useIntake } from '../composables/useIntake';

 const props = defineProps<{
   relayUrl: string;
   appContext?: Record<string, unknown>;
 }>();

 const isOpen = ref(false);
 const inputText = ref('');

-const { messages, streaming, submitting, result, error, start, sendTurn, submit } = useIntake({
+const {
+  messages,
+  streaming,
+  submitting,
+  result,
+  error,
+  start,
+  sendTurn,
+  submit,
+  pendingAttachments,
+  redactorSource,
+  canAttach,
+  attachLimits,
+  attachAndRedact,
+  commitRedacted,
+  cancelRedactor,
+  removeAttachment,
+} = useIntake({
   relayUrl: props.relayUrl,
   widgetVersion: '0.1.0',
   appContext: props.appContext,
 });

 // Initialize the session when the widget mounts
 onMounted(async () => {
   try {
     await start();
   } catch {
     // Session init failed — will retry on first send
   }
 });

 function togglePanel() {
   isOpen.value = !isOpen.value;
 }

 async function handleSend() {
   const text = inputText.value.trim();
   if (!text || streaming.value) return;
   inputText.value = '';
   await sendTurn(text);
 }

 async function handleSubmit() {
   if (submitting.value || streaming.value) return;
   await submit();
 }

+async function handleAttach() {
+  if (!canAttach.value) return;
+  await attachAndRedact(capturePage);
+}
+
+function handleRedactorSave(dataUrl: string) {
+  commitRedacted(dataUrl);
+}
+
+function handleRedactorCancel() {
+  cancelRedactor();
+}
+
+function handleStripRemove(index: number) {
+  removeAttachment(index);
+}
+
 function handleKeydown(event: KeyboardEvent) {
   if (event.key === 'Enter' && !event.shiftKey) {
     event.preventDefault();
     handleSend();
   }
 }
 </script>
@@
       <template v-else>
         <ConversationView
           :messages="messages"
           :streaming="streaming"
         />

         <!-- Error banner -->
         <div v-if="error" class="intake-widget__error" data-testid="error-banner" role="alert">
           {{ error }}
         </div>

+        <!-- Pending attachments strip -->
+        <AttachmentStrip
+          :items="pendingAttachments"
+          :max-total-bytes="attachLimits?.maxTotalBytes ?? 0"
+          @remove="handleStripRemove"
+        />
+
         <!-- Input area -->
         <div class="intake-widget__input-area">
           <textarea
             v-model="inputText"
             class="intake-widget__input"
             placeholder="Describe your issue…"
             rows="2"
             :disabled="streaming || submitting"
             data-testid="message-input"
             aria-label="Message"
             @keydown="handleKeydown"
           />
           <div class="intake-widget__actions">
+            <button
+              v-if="canAttach"
+              class="intake-widget__btn intake-widget__btn--ghost"
+              :disabled="streaming || submitting"
+              data-testid="attach-button"
+              aria-label="Attach screenshot"
+              @click="handleAttach"
+            >
+              Attach
+            </button>
             <button
               class="intake-widget__btn intake-widget__btn--send"
               :disabled="!inputText.trim() || streaming"
               data-testid="send-button"
               @click="handleSend"
             >
               Send
             </button>
             <button
               class="intake-widget__btn intake-widget__btn--submit"
               :disabled="messages.length === 0 || streaming || submitting"
               data-testid="submit-button"
               @click="handleSubmit"
             >
               {{ submitting ? 'Submitting…' : 'Submit' }}
             </button>
           </div>
         </div>
       </template>
     </div>
+
+    <!-- Redactor modal (rendered when redactorSource is non-null) -->
+    <ScreenshotRedactor
+      :source="redactorSource"
+      @save="handleRedactorSave"
+      @cancel="handleRedactorCancel"
+    />
   </div>
 </template>
```

Add a ghost-button style next to the existing `.intake-widget__btn--send` / `.intake-widget__btn--submit` rules (append inside `<style scoped>`):

```diff
@@
 .intake-widget__btn--submit:hover:not(:disabled) {
   background-color: #15803d;
 }
+
+.intake-widget__btn--ghost {
+  background-color: #fff;
+  color: #334155;
+  border: 1px solid #cbd5e1;
+}
+
+.intake-widget__btn--ghost:hover:not(:disabled) {
+  background-color: #f1f5f9;
+}
```

- [ ] **Step 3: Re-run tests; confirm green**

```
cd vue && npm run test -- IntakeWidget && cd ..
```

Expected: all IntakeWidget tests pass.

- [ ] **Step 4: Type-check + build vue**

```
cd vue && npm run type-check && cd ..
```

Expected: clean.

- [ ] **Step 5: Commit**

```
git add vue/src/components/IntakeWidget.vue vue/src/components/IntakeWidget.spec.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): IntakeWidget — Attach button (gated by canAttach), modal, strip wiring

Attach button is rendered iff capabilities.attachments is present on the cached
init response (no "disabled but visible" state per design §3.3). Click invokes
attachAndRedact(capturePage), which opens ScreenshotRedactor with the captured
canvas; Save commits the redacted PNG into the pending list (with size accounting),
Cancel closes the modal. AttachmentStrip renders between the conversation view
and the input area when the pending list is non-empty.
EOF
)"
```

---

### Task 10: Re-export ScreenshotRedactor + AttachmentStrip from `vue/src/index.ts`

**Files:** Modify `vue/src/index.ts`

- [ ] **Step 1: Apply the re-export hunk**

Apply this hunk to `vue/src/index.ts`:

```diff
@@
 import type { App } from 'vue';
 import IntakeWidget from './components/IntakeWidget.vue';
 import ConversationView from './components/ConversationView.vue';
+import ScreenshotRedactor from './components/ScreenshotRedactor.vue';
+import AttachmentStrip from './components/AttachmentStrip.vue';
 import { useIntake } from './composables/useIntake';
 import type { UseIntakeOptions } from './composables/useIntake';

-export { IntakeWidget, ConversationView, useIntake };
+export { IntakeWidget, ConversationView, ScreenshotRedactor, AttachmentStrip, useIntake };
 export type { UseIntakeOptions };

 // Re-export the core types consumers will need
-export type { ChatMessage, SubmitResult, IntakeConfig } from '@intake/core';
+export type { ChatMessage, SubmitResult, IntakeConfig, PendingAttachment, AttachmentLimits } from '@intake/core';
```

- [ ] **Step 2: Type-check + build vue**

```
cd vue && npm run type-check && npm run build && cd ..
```

Expected: clean. `dist/` should contain the new components.

- [ ] **Step 3: Commit**

```
git add vue/src/index.ts
git commit -m "$(cat <<'EOF'
feat(6-iii): re-export ScreenshotRedactor + AttachmentStrip + Phase 6 core types

@intake/vue now exposes the redactor and strip as standalone components for
consumers that want to compose their own widget shell. Core types
PendingAttachment + AttachmentLimits are re-exported for the same audience.
EOF
)"
```

---

## Smoke (mandatory)

Proves the Phase 6 widget deliverable end-to-end. Fully self-runnable; no LLM credit, no live API.

```
1. Pin gate (no LLM credit; self-runnable):
   bash scripts/check-pins.sh
   Expected: exit 0; stdout ends with "OK: all codegen tools are exact-pinned".
   Negative regression: replace "html2canvas": "1.4.1" with "^1.4.1" in
   core/package.json; rerun — expected exit 1 with the new html2canvas ERROR
   line; revert.

2. core/ unit suite (no LLM credit; self-runnable):
   cd core && npm run type-check && npm run test && cd ..
   Expected: all green. Coverage includes capture (DI, capturePage, 0×0 reject,
   canvasToDataURL), attachments (add/remove/clear, per-attachment cap,
   aggregate cap, MIME allowlist, named errors with `code`), client (submit
   threads attachments, omits when empty, parses ErrorEnvelope.code on non-2xx,
   init parses capabilities.attachments).

3. vue/ unit suite (no LLM credit; self-runnable):
   cd vue && npm run type-check && npm run test && cd ..
   Expected: all green. Coverage includes ScreenshotRedactor (mouse-draw rect,
   Clear resets, Save emits dataUrl, Cancel emits cancel + no toDataURL, ESC
   cancels, focus-trap entry, clamp out-of-bounds), AttachmentStrip (thumbnail
   per item, remove emits index, aggregate badge correct, empty hidden),
   useIntake (canAttach gated by caps, commitRedacted appends + clamps via
   AttachmentList, code → friendly mapping for all 8 codes, submit clears on
   success only), IntakeWidget (Attach hidden when caps null, visible when
   non-null, click → modal, Save → strip, Submit threads attachments).

4. Widget-level integration smoke (no LLM credit; self-runnable; IntakeWidget.spec.ts
   "IntakeWidget Attach → modal → strip → Submit flow" test block):
   With mockInit returning capabilities.attachments populated, mount IntakeWidget.
   - Assert "attach-button" is absent when caps.attachments=null.
   - Assert "attach-button" appears when caps.attachments is non-null.
   - Click launcher → panel opens.
   - Type a message + click Send → conversation has one user + one assistant message.
   - Click Attach → redactor-modal appears with the stubbed captured canvas.
   - Click Save (with a stubbed toDataURL) → modal closes; attachment-strip is
     visible with one thumb.
   - Click Submit → mockSubmit received exactly one attachment with mime_type
     "image/png" as the third argument.

5. Regression: vue/ Phase 1+4 existing tests still pass.
   The Phase 1 IntakeWidget + ConversationView + useIntake tests must continue
   to pass unchanged under the new useIntake surface. Specifically, the existing
   useIntake "initializes with empty messages and idle state" / "start() calls
   client.init()" / "sendTurn() ..." / "submit() ..." / error-path tests must
   all still pass — this confirms the additive extension does not regress the
   Phase 1 contract.

6. Build smoke (no LLM credit; self-runnable):
   cd core && npm run type-check && cd ..
   cd vue && npm run type-check && npm run build && cd ..
   Expected: both green; vue/dist/ contains the new components.
```

Smokes 1–6 are fully self-runnable. No live API credentials consumed; `html2canvas` is never loaded by tests (always DI-stubbed via `setHtml2Canvas` or `vi.mock('@intake/core', ...)`).

---

## Done criteria

- [ ] `html2canvas` is pinned EXACTLY at `1.4.1` in `core/package.json` `dependencies`; `core/package-lock.json` resolved version matches; no caret/tilde.
- [ ] `scripts/check-pins.sh` includes the no-caret gate for `html2canvas` and passes from a clean state; negative regression (carret-pin) reports the new ERROR line and exits 1.
- [ ] `core/src/types.ts` has the additive `InitResponse.capabilities.attachments?` and `SubmitRequest.attachments?` fields; type-check is clean.
- [ ] `core/src/capture.ts` exports `setHtml2Canvas`, `capturePage`, `canvasToDataURL`, `__resetCaptureForTests`; all unit tests pass; 0×0 canvas rejection covered.
- [ ] `core/src/attachments.ts` exports `PendingAttachment`, `AttachmentLimits`, `AttachmentList`, `AttachmentTooLargeError`, `AggregateTooLargeError`, `MimeNotAllowedError`; named errors each expose a `code` string; all unit tests pass; boundary conditions covered.
- [ ] `core/src/client.ts` `submit()` accepts optional `attachments` and includes it in the POST body when non-empty; non-2xx with a JSON ErrorEnvelope produces an Error with `code`; non-JSON body produces an Error without `code`; `init()` parses `capabilities.attachments` when present.
- [ ] `core/src/index.ts` re-exports `PendingAttachment`, `AttachmentLimits`, `AttachmentList`, the three named errors, `setHtml2Canvas`, `capturePage`, `canvasToDataURL`.
- [ ] `vue/src/components/ScreenshotRedactor.vue` renders only when `source` is non-null; mouse-drag draws solid black rectangles; Clear empties the array; Save flattens onto an offscreen copy of source and emits the data URL; Cancel + ESC + backdrop-click emit `cancel` and do NOT call `toDataURL`; rectangles outside source bounds are clamped; focus moves to Save on open.
- [ ] `vue/src/components/AttachmentStrip.vue` renders one `<img>` thumbnail per item, a Remove button per item that emits `remove(index)`, and an aggregate-size badge in human-readable form; hidden entirely when `items` is empty.
- [ ] `vue/src/composables/useIntake.ts` exposes `pendingAttachments`, `redactorSource`, `canAttach`, `attachLimits`, `attachAndRedact`, `cancelRedactor`, `commitRedacted`, `removeAttachment`, `clearAttachments` plus the existing surface; `submit()` threads attachments into `client.submit` and clears the pending list on success only; error codes map to design §8.3 friendly strings; unknown codes fall back to the raw message.
- [ ] `vue/src/components/IntakeWidget.vue` shows the Attach button iff `canAttach.value` is true (no "disabled but visible" state); click invokes `attachAndRedact(capturePage)`; modal opens with the captured canvas; Save → strip updates; Submit threads attachments through.
- [ ] `vue/src/index.ts` re-exports `ScreenshotRedactor`, `AttachmentStrip`, `PendingAttachment`, `AttachmentLimits`.
- [ ] `cd core && npm run type-check && npm run test` is green.
- [ ] `cd vue && npm run type-check && npm run build && npm run test` is green.
- [ ] Existing Phase 1 `useIntake.spec.ts` / `IntakeWidget` / `ConversationView` / `client.test.ts` tests pass unchanged.
- [ ] All commits use `feat(6-iii): ...` / (none in this plan require `test(6-iii): ...` — tests are committed together with the code that satisfies them, per the 5-step TDD pattern).
- [ ] Smoke section above passes from a clean state.

*End of 6-iii plan.*
