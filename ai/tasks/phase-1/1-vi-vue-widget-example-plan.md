# 1-vi — @intake/vue Widget + examples/vue-anonymous Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `@intake/vue` stub with a real Vite library-mode package (launcher+panel widget wrapping `@intake/core`), create the `examples/vue-anonymous` Vite+Vue dev app that exercises it end-to-end, and provide `examples/webhook-receiver` — together enabling the Phase 1 final smoke.

**Architecture:** `@intake/vue` is built in Vite library mode (ESM, `vue` external/peer) and exports `IntakeWidget.vue` (floating launcher button + chat panel), `ConversationView.vue` (message bubbles + streaming indicator), and the `useIntake` composable that wraps `@intake/core`'s `IntakeClient`. The example app is a minimal Vite+Vue SPA that mounts `IntakeWidget` pointed at the relay on `http://localhost:8080`. The webhook receiver is a ~30-line Node script that logs POSTed payloads to stdout.

**Tech Stack:** Vue 3.5.x, Vite 5.4.x (exact-pinned), @vitejs/plugin-vue (exact-pinned), vue-tsc (exact-pinned), vitest, @vue/test-utils, TypeScript 5.6.x, Node 24.12.0, @intake/core (workspace dep).

---

## Environment Notes

- **OS:** Windows 10, Node 24.12.0, bash (Git Bash / WSL) or PowerShell both available.
- **Working directory root:** `C:\src\ai\intake` (monorepo root).
- **Relay:** Go relay runs at `http://localhost:8080` (started separately per the smoke).
- **Vite dev server default:** `http://localhost:5173` — this MUST appear in the relay's `cors_origins`.
- **LESSONS L001:** Type-check MUST use the build path (`vue-tsc -b` or `npm run build`) — not just `vue-tsc --noEmit` — because project-reference / SFC errors that slip through `--noEmit` are caught by the build path. The plan's verification steps all use `npm run build` which invokes `vue-tsc -b` internally.
- **Security invariant (from README §2 and design §2):** The widget NEVER handles provider API keys. It only calls the relay through `@intake/core`. No code path contacts Anthropic directly from the browser.

---

## 1. Goal

Deliver:
1. `@intake/vue` — a real npm library package (replaces the Phase-0 stub) that builds a clean ESM bundle via Vite library mode. It exports `IntakeWidget`, `ConversationView`, `useIntake`, and a Vue plugin install helper.
2. `examples/vue-anonymous` — a minimal Vite+Vue SPA demonstrating an anonymous integration, runnable with `npm run -w examples/vue-anonymous dev`.
3. `examples/webhook-receiver` — a tiny (~30-line) Node server that logs POST bodies on `:9099/intake`, used by the phase smoke.
4. The **Phase 1 final smoke** (README §8): relay + example + webhook receiver running together, full guided triage conversation, Submit → schema-valid canonical payload logged, widget shows `external_id`, relay logs do NOT contain the `ANTHROPIC_API_KEY` value.

---

## 2. Design References

- Phase README §6.7 — `@intake/core` public API (`IntakeClient`, `IntakeConfig`, `ChatMessage`, `SubmitResult`) — **frozen interface** this widget wraps.
- Phase README §8 — Final smoke (verbatim smoke steps reproduced in §5 of this plan).
- Phase README §5 — Tool version pins (vue 3.5.x, vite 5.4.x, @vitejs/plugin-vue, tsx — exact/no-caret).
- Phase README §7 — Build-fail checklist.
- Design spec §2 — Security invariant: relay is the sole LLM broker; no API keys in the browser.
- Design spec §3 — Architecture diagram: browser owns conversation state, relay is stateless between turns.
- Design spec §7 — `config.yaml` sample: `cors_origins: ["http://localhost:5173"]`.
- `ai/WEB_CODE.md` — `<script setup lang="ts">`, scoped CSS, `data-testid`, prefer small focused files, no `console.log`, PascalCase components, `useXxx` composables.
- `ai/LESSONS.md` L001 — Use build path for type-check.
- **Note for implementer:** The `impeccable` and `emil-design-eng` skills are available for component polish — but Phase 1 is a skeleton. Keep styling clean and functional, not polished. Polish is a later phase.

---

## 3. Files Touched

| File | Create/Modify | Responsibility |
|---|---|---|
| `vue/package.json` | **Modify** | Add real deps (vue peer, vite, @vitejs/plugin-vue, vue-tsc, vitest, @vue/test-utils), build/type-check/test scripts, exports map, peerDependencies |
| `vue/vite.config.ts` | **Create** | Vite library-mode config: entry `src/index.ts`, format ESM, external `vue`, output `dist/` |
| `vue/tsconfig.json` | **Create** | Vue-aware TS config (composite, `vue-tsc`, `moduleResolution: Bundler`) |
| `vue/tsconfig.app.json` | **Create** | App/library source tsconfig (includes `src/`) |
| `vue/src/index.ts` | **Create** | Public exports: `IntakeWidget`, `ConversationView`, `useIntake`, `IntakePlugin` (Vue install) |
| `vue/src/composables/useIntake.ts` | **Create** | Composable wrapping `IntakeClient`: reactive `messages`, `streaming`, `submitting`, `result`; methods `start()`, `sendTurn(text)`, `submit()` |
| `vue/src/composables/useIntake.spec.ts` | **Create** | Vitest unit test for `useIntake` with a mocked `IntakeClient` |
| `vue/src/components/ConversationView.vue` | **Create** | Renders message list (user/assistant bubbles) + streaming indicator; `data-testid` attrs |
| `vue/src/components/ConversationView.spec.ts` | **Create** | Vitest + @vue/test-utils component test for ConversationView |
| `vue/src/components/IntakeWidget.vue` | **Create** | Launcher button + panel container; props `relayUrl`, `appContext`; uses `useIntake`; input + Send + Submit buttons; shows ticket result |
| `package.json` (root) | **Modify** | Add `examples/vue-anonymous` and `examples/webhook-receiver` to `workspaces` array |
| `examples/vue-anonymous/package.json` | **Create** | Vite+Vue app package; `@intake/vue` and `@intake/core` workspace deps |
| `examples/vue-anonymous/vite.config.ts` | **Create** | Standard Vite+Vue app config (port 5173) |
| `examples/vue-anonymous/tsconfig.json` | **Create** | Vue app TS config |
| `examples/vue-anonymous/index.html` | **Create** | Vite app HTML entry |
| `examples/vue-anonymous/src/main.ts` | **Create** | Vue app bootstrap: `createApp(App).mount('#app')` |
| `examples/vue-anonymous/src/App.vue` | **Create** | Mounts `IntakeWidget` with `relayUrl="http://localhost:8080"` |
| `examples/webhook-receiver/package.json` | **Create** | Minimal Node package (no deps; `type: module`) |
| `examples/webhook-receiver/server.mjs` | **Create** | ~30-line Node HTTP server logging POST bodies on :9099/intake |
| `examples/README.md` | **Create** | Run steps for all three pieces + CORS note + smoke checklist |
| `config.yaml` (monorepo root) | **Create** | Sample relay config for local dev with `cors_origins: ["http://localhost:5173"]` and `webhook.url: "http://localhost:9099/intake"` |

---

## 4. Tasks

---

### Task 1: Pin exact tool versions and update `vue/package.json`

**Files:**
- Modify: `vue/package.json`

This is the first task. Before writing any code, install and pin exact versions — no caret on load-bearing build tools (per Phase README §5 and PHASE_PLANNING tool pin rule).

- [ ] **Step 1.1: Resolve exact current latest patch versions**

Run these commands and record the exact versions printed:
```bash
cd /c/src/ai/intake
npm info vue version          # expect 3.5.x
npm info vite version         # expect 5.4.x
npm info @vitejs/plugin-vue version
npm info vue-tsc version
npm info vitest version
npm info @vue/test-utils version
```

Record results. If `vue` is not 3.5.x or `vite` is not 5.4.x, note the discrepancy and use whatever the actual latest patch of those minor lines is. Exact pinning is the requirement — not these specific patch numbers.

- [ ] **Step 1.2: Write `vue/package.json` with pinned deps**

Replace the entire file with (substitute `X.Y.Z` with the actual versions from Step 1.1 — these are the versions as of plan authoring, verify before using):

```json
{
  "name": "@intake/vue",
  "version": "0.1.0",
  "type": "module",
  "private": false,
  "description": "Intake Vue widget — launcher + panel triage UI",
  "exports": {
    ".": {
      "import": "./dist/intake-vue.js",
      "types": "./dist/index.d.ts"
    }
  },
  "main": "./dist/intake-vue.js",
  "types": "./dist/index.d.ts",
  "files": ["dist"],
  "scripts": {
    "build": "vite build && vue-tsc -b",
    "type-check": "vue-tsc -b",
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "peerDependencies": {
    "vue": ">=3.5.0"
  },
  "dependencies": {
    "@intake/core": "*"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "EXACT_VERSION_FROM_STEP_1.1",
    "@vue/test-utils": "EXACT_VERSION_FROM_STEP_1.1",
    "vite": "EXACT_VERSION_FROM_STEP_1.1",
    "vitest": "EXACT_VERSION_FROM_STEP_1.1",
    "vue": "EXACT_VERSION_FROM_STEP_1.1",
    "vue-tsc": "EXACT_VERSION_FROM_STEP_1.1"
  }
}
```

**Important:** `@vitejs/plugin-vue` and `vite` and `vue-tsc` are load-bearing build tools — NO caret prefix. `vitest` and `@vue/test-utils` are dev-tooling-only and caret is acceptable per the README, but exact is also fine. Use exact for all.

- [ ] **Step 1.3: Install deps from monorepo root**

```bash
cd /c/src/ai/intake
npm install
```

Expected: no errors, `node_modules/@intake/vue` symlink resolves, `node_modules/vue` is present.

- [ ] **Step 1.4: Update the Phase README §5 pin table**

Open `ai/tasks/phase-1/README.md` and fill in the actual pinned versions for `vue`, `vite`, `@vitejs/plugin-vue`, `vue-tsc`, and any new tools (vitest, @vue/test-utils) in the Tool Version Pin table (§5). Add a row for `vitest` and `@vue/test-utils` if not present, noting caret is acceptable for these.

- [ ] **Step 1.5: Commit**

```bash
cd /c/src/ai/intake
git add vue/package.json ai/tasks/phase-1/README.md package-lock.json
git commit -m "feat(1-vi): pin @intake/vue deps (vue, vite, @vitejs/plugin-vue, vue-tsc exact)"
```

---

### Task 2: Vite library-mode build config and TypeScript config

**Files:**
- Create: `vue/vite.config.ts`
- Create: `vue/tsconfig.json`
- Create: `vue/tsconfig.app.json`

- [ ] **Step 2.1: Create `vue/tsconfig.json`**

This is the root tsconfig for the package — it enables composite/build mode so `vue-tsc -b` works:

```json
{
  "files": [],
  "references": [
    { "path": "./tsconfig.app.json" }
  ]
}
```

- [ ] **Step 2.2: Create `vue/tsconfig.app.json`**

```json
{
  "compilerOptions": {
    "composite": true,
    "tsBuildInfoFile": "./node_modules/.tmp/tsconfig.app.tsbuildinfo",
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "forceConsistentCasingInFileNames": true,
    "declaration": true,
    "declarationDir": "../dist",
    "outDir": "../dist",
    "jsx": "preserve"
  },
  "include": ["src/**/*.ts", "src/**/*.vue"]
}
```

- [ ] **Step 2.3: Create `vue/vite.config.ts`**

```typescript
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { resolve } from 'path';

export default defineConfig({
  plugins: [vue()],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      name: 'IntakeVue',
      fileName: 'intake-vue',
      formats: ['es'],
    },
    rollupOptions: {
      // vue is a peer dep — do not bundle it
      external: ['vue'],
      output: {
        globals: {
          vue: 'Vue',
        },
      },
    },
    // emit type declarations via vue-tsc in the build script; vite itself
    // does not emit .d.ts — the "build" npm script runs vue-tsc -b after vite build.
  },
  test: {
    environment: 'jsdom',
    globals: true,
  },
});
```

- [ ] **Step 2.4: Verify vite config is valid (dry-run)**

```bash
cd /c/src/ai/intake/vue
npx vite build --dry-run 2>&1 || true
```

Expected: either "dry-run" not supported (ignore) or no import errors. The real verification is in Task 7.

- [ ] **Step 2.5: Commit**

```bash
cd /c/src/ai/intake
git add vue/vite.config.ts vue/tsconfig.json vue/tsconfig.app.json
git commit -m "feat(1-vi): add vite lib-mode config and vue-tsc tsconfig for @intake/vue"
```

---

### Task 3: `useIntake` composable + unit test (TDD)

**Files:**
- Create: `vue/src/composables/useIntake.ts`
- Create: `vue/src/composables/useIntake.spec.ts`

The composable wraps `IntakeClient` from `@intake/core`. It manages reactive state and exposes `start()`, `sendTurn(text)`, and `submit()`.

- [ ] **Step 3.1: Write the failing test first**

Create `vue/src/composables/useIntake.spec.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useIntake } from './useIntake';

// --- Mock @intake/core ---
// We mock the IntakeClient class so no real HTTP calls happen.
const mockInit = vi.fn();
const mockTurn = vi.fn();
const mockSubmit = vi.fn();

vi.mock('@intake/core', () => {
  return {
    IntakeClient: vi.fn().mockImplementation(() => ({
      init: mockInit,
      turn: mockTurn,
      submit: mockSubmit,
    })),
  };
});

describe('useIntake', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('initializes with empty messages and idle state', () => {
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    expect(intake.messages.value).toEqual([]);
    expect(intake.streaming.value).toBe(false);
    expect(intake.submitting.value).toBe(false);
    expect(intake.result.value).toBeNull();
  });

  it('start() calls client.init() and returns session_id', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    const session = await intake.start();
    expect(mockInit).toHaveBeenCalledOnce();
    expect(session.session_id).toBe('sess-abc');
  });

  it('sendTurn() appends user message, streams assistant deltas, sets streaming=true then false', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });

    // turn() invokes onDelta twice then resolves
    mockTurn.mockImplementation(async (_messages: unknown, onDelta: (d: string) => void) => {
      onDelta('Hello ');
      onDelta('world');
      return { input_tokens: 10, output_tokens: 5 };
    });

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.sendTurn('Hi there');

    expect(intake.messages.value).toHaveLength(2);
    expect(intake.messages.value[0]).toEqual({ role: 'user', content: 'Hi there' });
    expect(intake.messages.value[1]).toEqual({ role: 'assistant', content: 'Hello world' });
    expect(intake.streaming.value).toBe(false); // reset after done
  });

  it('streaming is true during sendTurn and false after', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });

    let capturedStreaming: boolean | null = null;
    mockTurn.mockImplementation(async (_messages: unknown, onDelta: (d: string) => void) => {
      onDelta('x');
      // By the time onDelta is called, streaming should be true
      // We'll check via the composable's reactive state captured in a closure
      return { input_tokens: 1, output_tokens: 1 };
    });

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();

    // Patch onDelta to capture the state at delta time
    mockTurn.mockImplementation(async (_messages: unknown, onDelta: (d: string) => void) => {
      capturedStreaming = intake.streaming.value;
      onDelta('x');
      return { input_tokens: 1, output_tokens: 1 };
    });

    await intake.sendTurn('test');
    expect(capturedStreaming).toBe(true);
    expect(intake.streaming.value).toBe(false);
  });

  it('submit() sets submitting=true, calls client.submit(), stores result, resets submitting', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });
    mockTurn.mockResolvedValue({ input_tokens: 1, output_tokens: 1 });
    mockSubmit.mockResolvedValue({
      external_id: 'ticket-123',
      external_url: 'http://localhost:9099/intake/ticket-123',
      adapter_name: 'webhook',
      created_at: '2026-05-26T00:00:00Z',
    });

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.sendTurn('My app crashes');
    await intake.submit();

    expect(mockSubmit).toHaveBeenCalledOnce();
    expect(intake.result.value).not.toBeNull();
    expect(intake.result.value?.external_id).toBe('ticket-123');
    expect(intake.submitting.value).toBe(false);
  });

  it('submit() passes all accumulated messages to client.submit()', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });
    mockTurn.mockResolvedValue({ input_tokens: 1, output_tokens: 1 });
    mockSubmit.mockResolvedValue({
      external_id: 'ticket-xyz',
      external_url: '',
      adapter_name: 'webhook',
      created_at: '2026-05-26T00:00:00Z',
    });

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.sendTurn('First message');
    await intake.submit();

    const submitCall = mockSubmit.mock.calls[0];
    // First arg is the messages array
    expect(submitCall[0]).toContainEqual({ role: 'user', content: 'First message' });
  });
});
```

- [ ] **Step 3.2: Run the test — expect failure (module not found)**

```bash
cd /c/src/ai/intake/vue
npx vitest run src/composables/useIntake.spec.ts
```

Expected output: test runner fails with something like `Cannot find module '@intake/core'` or `Cannot find module './useIntake'` — confirming the test is wired up and will fail for the right reason once we write the implementation.

- [ ] **Step 3.3: Create `vue/src/composables/useIntake.ts`**

```typescript
import { ref } from 'vue';
import { IntakeClient } from '@intake/core';
import type { IntakeConfig, ChatMessage, SubmitResult } from '@intake/core';

export interface UseIntakeOptions {
  relayUrl: string;
  widgetVersion?: string;
  appContext?: Record<string, unknown>;
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

  async function start() {
    error.value = null;
    return client.init();
  }

  async function sendTurn(text: string) {
    // Append user message
    messages.value = [...messages.value, { role: 'user', content: text }];

    // Add a placeholder assistant message we will stream into
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
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      streaming.value = false;
    }
  }

  async function submit(routingHint?: string) {
    submitting.value = true;
    error.value = null;
    try {
      result.value = await client.submit(messages.value, routingHint);
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
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
    start,
    sendTurn,
    submit,
  };
}
```

- [ ] **Step 3.4: Run the test — expect green**

```bash
cd /c/src/ai/intake/vue
npx vitest run src/composables/useIntake.spec.ts
```

Expected output:
```
✓ useIntake
  ✓ initializes with empty messages and idle state
  ✓ start() calls client.init() and returns session_id
  ✓ sendTurn() appends user message, streams assistant deltas, sets streaming=true then false
  ✓ streaming is true during sendTurn and false after
  ✓ submit() sets submitting=true, calls client.submit(), stores result, resets submitting
  ✓ submit() passes all accumulated messages to client.submit()

Test Files  1 passed (1)
Tests       6 passed (6)
```

If any test fails, fix `useIntake.ts` until all 6 pass before moving on.

- [ ] **Step 3.5: Commit**

```bash
cd /c/src/ai/intake
git add vue/src/composables/useIntake.ts vue/src/composables/useIntake.spec.ts
git commit -m "feat(1-vi): useIntake composable wrapping IntakeClient with reactive state + tests"
```

---

### Task 4: `ConversationView.vue` component + test (TDD)

**Files:**
- Create: `vue/src/components/ConversationView.vue`
- Create: `vue/src/components/ConversationView.spec.ts`

This component receives a `messages` prop and a `streaming` prop, renders user/assistant bubbles, and shows a streaming indicator when `streaming=true`.

- [ ] **Step 4.1: Write the failing component test first**

Create `vue/src/components/ConversationView.spec.ts`:

```typescript
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import ConversationView from './ConversationView.vue';
import type { ChatMessage } from '@intake/core';

describe('ConversationView', () => {
  it('renders nothing when messages is empty', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [], streaming: false },
    });
    expect(wrapper.findAll('[data-testid="message"]')).toHaveLength(0);
  });

  it('renders a user message with correct role class', () => {
    const messages: ChatMessage[] = [{ role: 'user', content: 'Hello!' }];
    const wrapper = mount(ConversationView, {
      props: { messages, streaming: false },
    });
    const msgs = wrapper.findAll('[data-testid="message"]');
    expect(msgs).toHaveLength(1);
    expect(msgs[0].classes()).toContain('message--user');
    expect(msgs[0].text()).toContain('Hello!');
  });

  it('renders an assistant message with correct role class', () => {
    const messages: ChatMessage[] = [{ role: 'assistant', content: 'How can I help?' }];
    const wrapper = mount(ConversationView, {
      props: { messages, streaming: false },
    });
    const msgs = wrapper.findAll('[data-testid="message"]');
    expect(msgs).toHaveLength(1);
    expect(msgs[0].classes()).toContain('message--assistant');
  });

  it('renders multiple messages in order', () => {
    const messages: ChatMessage[] = [
      { role: 'user', content: 'First' },
      { role: 'assistant', content: 'Second' },
      { role: 'user', content: 'Third' },
    ];
    const wrapper = mount(ConversationView, {
      props: { messages, streaming: false },
    });
    const msgs = wrapper.findAll('[data-testid="message"]');
    expect(msgs).toHaveLength(3);
    expect(msgs[0].text()).toContain('First');
    expect(msgs[1].text()).toContain('Second');
    expect(msgs[2].text()).toContain('Third');
  });

  it('shows streaming indicator when streaming=true', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [], streaming: true },
    });
    expect(wrapper.find('[data-testid="streaming-indicator"]').exists()).toBe(true);
  });

  it('hides streaming indicator when streaming=false', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [], streaming: false },
    });
    expect(wrapper.find('[data-testid="streaming-indicator"]').exists()).toBe(false);
  });
});
```

- [ ] **Step 4.2: Run the test — expect failure (component not found)**

```bash
cd /c/src/ai/intake/vue
npx vitest run src/components/ConversationView.spec.ts
```

Expected: fails because `ConversationView.vue` does not exist yet.

- [ ] **Step 4.3: Create `vue/src/components/ConversationView.vue`**

```vue
<script setup lang="ts">
import type { ChatMessage } from '@intake/core';

defineProps<{
  messages: ChatMessage[];
  streaming: boolean;
}>();
</script>

<template>
  <div class="conversation-view">
    <div
      v-for="(msg, i) in messages"
      :key="i"
      :class="['message', `message--${msg.role}`]"
      data-testid="message"
    >
      <span class="message__content">{{ msg.content }}</span>
    </div>
    <div
      v-if="streaming"
      class="streaming-indicator"
      data-testid="streaming-indicator"
      aria-label="Assistant is typing"
    >
      <span class="streaming-indicator__dot" />
      <span class="streaming-indicator__dot" />
      <span class="streaming-indicator__dot" />
    </div>
  </div>
</template>

<style scoped>
.conversation-view {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  overflow-y: auto;
  flex: 1;
}

.message {
  max-width: 80%;
  padding: 8px 12px;
  border-radius: 8px;
  word-break: break-word;
  font-size: 14px;
  line-height: 1.5;
}

.message--user {
  align-self: flex-end;
  background-color: #2563eb;
  color: #fff;
}

.message--assistant {
  align-self: flex-start;
  background-color: #f1f5f9;
  color: #1e293b;
}

.message__content {
  white-space: pre-wrap;
}

.streaming-indicator {
  display: flex;
  gap: 4px;
  align-self: flex-start;
  padding: 8px 12px;
}

.streaming-indicator__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: #94a3b8;
  animation: pulse 1s ease-in-out infinite;
}

.streaming-indicator__dot:nth-child(2) {
  animation-delay: 0.2s;
}

.streaming-indicator__dot:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes pulse {
  0%, 100% { opacity: 0.3; }
  50% { opacity: 1; }
}
</style>
```

- [ ] **Step 4.4: Run the test — expect green**

```bash
cd /c/src/ai/intake/vue
npx vitest run src/components/ConversationView.spec.ts
```

Expected output:
```
✓ ConversationView
  ✓ renders nothing when messages is empty
  ✓ renders a user message with correct role class
  ✓ renders an assistant message with correct role class
  ✓ renders multiple messages in order
  ✓ shows streaming indicator when streaming=true
  ✓ hides streaming indicator when streaming=false

Test Files  1 passed (1)
Tests       6 passed (6)
```

- [ ] **Step 4.5: Commit**

```bash
cd /c/src/ai/intake
git add vue/src/components/ConversationView.vue vue/src/components/ConversationView.spec.ts
git commit -m "feat(1-vi): ConversationView component with message bubbles and streaming indicator"
```

---

### Task 5: `IntakeWidget.vue` — launcher button + panel

**Files:**
- Create: `vue/src/components/IntakeWidget.vue`

This is the main widget component. It renders a floating launcher button (bottom-right). Clicking it opens a panel containing `ConversationView` + an input box + Send button + Submit button. On submit, it shows the returned `external_id`. It accepts `relayUrl` and `appContext` as props, and internally uses `useIntake`.

No test here beyond what the phase smoke provides — the full widget interaction is best verified in the browser. (The composable and ConversationView are unit-tested above.)

- [ ] **Step 5.1: Create `vue/src/components/IntakeWidget.vue`**

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue';
import ConversationView from './ConversationView.vue';
import { useIntake } from '../composables/useIntake';

const props = defineProps<{
  relayUrl: string;
  appContext?: Record<string, unknown>;
}>();

const isOpen = ref(false);
const inputText = ref('');

const { messages, streaming, submitting, result, error, start, sendTurn, submit } = useIntake({
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

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault();
    handleSend();
  }
}
</script>

<template>
  <div class="intake-widget">
    <!-- Launcher button -->
    <button
      class="intake-widget__launcher"
      :class="{ 'intake-widget__launcher--open': isOpen }"
      aria-label="Open support widget"
      data-testid="launcher-button"
      @click="togglePanel"
    >
      <svg v-if="!isOpen" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
      </svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <line x1="18" y1="6" x2="6" y2="18"/>
        <line x1="6" y1="6" x2="18" y2="18"/>
      </svg>
    </button>

    <!-- Chat panel -->
    <div
      v-if="isOpen"
      class="intake-widget__panel"
      data-testid="chat-panel"
      role="dialog"
      aria-label="Support chat"
    >
      <!-- Header -->
      <div class="intake-widget__header">
        <span class="intake-widget__title">Report an Issue</span>
      </div>

      <!-- Result view (shown after submit) -->
      <div v-if="result" class="intake-widget__result" data-testid="submit-result">
        <div class="intake-widget__result-icon" aria-hidden="true">✓</div>
        <p class="intake-widget__result-text">Your report has been submitted.</p>
        <p class="intake-widget__result-id">
          Ticket ID: <code data-testid="external-id">{{ result.external_id }}</code>
        </p>
        <a
          v-if="result.external_url"
          :href="result.external_url"
          target="_blank"
          rel="noopener noreferrer"
          class="intake-widget__result-link"
        >
          View ticket
        </a>
      </div>

      <!-- Conversation view + input (shown before submit) -->
      <template v-else>
        <ConversationView
          :messages="messages"
          :streaming="streaming"
        />

        <!-- Error banner -->
        <div v-if="error" class="intake-widget__error" data-testid="error-banner" role="alert">
          {{ error }}
        </div>

        <!-- Input area -->
        <div class="intake-widget__input-area">
          <textarea
            v-model="inputText"
            class="intake-widget__input"
            placeholder="Describe your issue…"
            rows="2"
            :disabled="streaming || submitting"
            data-testid="message-input"
            @keydown="handleKeydown"
          />
          <div class="intake-widget__actions">
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
  </div>
</template>

<style scoped>
/* ── Launcher button ─────────────────────────────────────── */
.intake-widget {
  position: fixed;
  bottom: 24px;
  right: 24px;
  z-index: 9999;
  font-family: system-ui, sans-serif;
}

.intake-widget__launcher {
  width: 52px;
  height: 52px;
  border-radius: 50%;
  background-color: #2563eb;
  color: #fff;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
  transition: background-color 0.2s ease, transform 0.2s ease;
}

.intake-widget__launcher:hover {
  background-color: #1d4ed8;
  transform: scale(1.05);
}

.intake-widget__launcher--open {
  background-color: #64748b;
}

/* ── Panel ───────────────────────────────────────────────── */
.intake-widget__panel {
  position: absolute;
  bottom: 64px;
  right: 0;
  width: 360px;
  height: 480px;
  background-color: #fff;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.15);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.intake-widget__header {
  padding: 14px 16px;
  background-color: #2563eb;
  color: #fff;
  flex-shrink: 0;
}

.intake-widget__title {
  font-size: 15px;
  font-weight: 600;
}

/* ── Input area ──────────────────────────────────────────── */
.intake-widget__input-area {
  padding: 12px;
  border-top: 1px solid #e2e8f0;
  flex-shrink: 0;
}

.intake-widget__input {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  padding: 8px 10px;
  font-size: 13px;
  resize: none;
  outline: none;
  font-family: inherit;
  transition: border-color 0.15s;
}

.intake-widget__input:focus {
  border-color: #2563eb;
}

.intake-widget__input:disabled {
  background-color: #f8fafc;
  color: #94a3b8;
}

.intake-widget__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}

.intake-widget__btn {
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: background-color 0.15s, opacity 0.15s;
}

.intake-widget__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.intake-widget__btn--send {
  background-color: #2563eb;
  color: #fff;
}

.intake-widget__btn--send:hover:not(:disabled) {
  background-color: #1d4ed8;
}

.intake-widget__btn--submit {
  background-color: #16a34a;
  color: #fff;
}

.intake-widget__btn--submit:hover:not(:disabled) {
  background-color: #15803d;
}

/* ── Result view ─────────────────────────────────────────── */
.intake-widget__result {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  text-align: center;
  gap: 8px;
}

.intake-widget__result-icon {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background-color: #dcfce7;
  color: #16a34a;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  font-weight: bold;
  margin-bottom: 8px;
}

.intake-widget__result-text {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
  margin: 0;
}

.intake-widget__result-id {
  font-size: 13px;
  color: #64748b;
  margin: 0;
}

.intake-widget__result-id code {
  font-family: ui-monospace, monospace;
  background-color: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
}

.intake-widget__result-link {
  font-size: 13px;
  color: #2563eb;
  text-decoration: none;
}

.intake-widget__result-link:hover {
  text-decoration: underline;
}

/* ── Error banner ────────────────────────────────────────── */
.intake-widget__error {
  margin: 0 12px;
  padding: 8px 12px;
  background-color: #fef2f2;
  color: #b91c1c;
  border-radius: 6px;
  font-size: 13px;
  flex-shrink: 0;
}
</style>
```

- [ ] **Step 5.2: Commit**

```bash
cd /c/src/ai/intake
git add vue/src/components/IntakeWidget.vue
git commit -m "feat(1-vi): IntakeWidget launcher+panel with conversation, send, submit, result views"
```

---

### Task 6: `vue/src/index.ts` — public exports + Vue plugin

**Files:**
- Create: `vue/src/index.ts`

- [ ] **Step 6.1: Create `vue/src/index.ts`**

```typescript
import type { App } from 'vue';
import IntakeWidget from './components/IntakeWidget.vue';
import ConversationView from './components/ConversationView.vue';
import { useIntake } from './composables/useIntake';
import type { UseIntakeOptions } from './composables/useIntake';

export { IntakeWidget, ConversationView, useIntake };
export type { UseIntakeOptions };

// Re-export the core types consumers will need
export type { ChatMessage, SubmitResult, IntakeConfig } from '@intake/core';

/**
 * Vue plugin — optional. Registers IntakeWidget globally.
 * Usage: app.use(IntakePlugin)
 */
export const IntakePlugin = {
  install(app: App) {
    app.component('IntakeWidget', IntakeWidget);
  },
};
```

- [ ] **Step 6.2: Commit**

```bash
cd /c/src/ai/intake
git add vue/src/index.ts
git commit -m "feat(1-vi): @intake/vue public exports and Vue plugin install helper"
```

---

### Task 7: Build `@intake/vue` and verify type-check passes

**Files:** no new files — verification only.

- [ ] **Step 7.1: Run the full test suite**

```bash
cd /c/src/ai/intake/vue
npx vitest run
```

Expected: all tests pass (useIntake.spec.ts + ConversationView.spec.ts).

- [ ] **Step 7.2: Run the Vite library build**

```bash
cd /c/src/ai/intake/vue
npx vite build
```

Expected output includes:
```
dist/intake-vue.js  XX.XX kB
```

No errors. The `dist/` directory is created with `intake-vue.js`.

If you get errors like `Cannot find module '@intake/core'`, ensure the monorepo `npm install` has run and the workspace symlink is in place:
```bash
cd /c/src/ai/intake
npm install
cd vue
npx vite build
```

- [ ] **Step 7.3: Run vue-tsc -b type-check (the build path per LESSONS L001)**

```bash
cd /c/src/ai/intake/vue
npx vue-tsc -b
```

Expected: exits 0, no errors. This is the authoritative type-check (per L001 — `--noEmit` alone is insufficient).

If you get errors:
- `Type 'X' is not assignable to type 'Y'` → fix the type in the relevant `.vue` or `.ts` file, then re-run.
- `Cannot find type declarations for module 'vue'` → ensure `vue` is in `node_modules` (run `npm install` from root).
- TS2367 "comparison unintentional" type errors → fix in source; do NOT suppress with `// @ts-ignore`.

- [ ] **Step 7.4: Update `npm run type-check` in root `package.json`**

Open `package.json` at the monorepo root. Update the `type-check` script to also run the vue type-check:

```json
"type-check": "npm run -w @intake/core type-check && npm run -w @intake/vue type-check"
```

- [ ] **Step 7.5: Verify root type-check**

```bash
cd /c/src/ai/intake
npm run type-check
```

Expected: exits 0.

- [ ] **Step 7.6: Commit**

```bash
cd /c/src/ai/intake
git add package.json
git commit -m "feat(1-vi): @intake/vue builds clean (vite lib + vue-tsc -b green), update root type-check"
```

---

### Task 8: Create `examples/vue-anonymous/` — the Vite+Vue example app

**Files:**
- Create: `examples/vue-anonymous/package.json`
- Create: `examples/vue-anonymous/vite.config.ts`
- Create: `examples/vue-anonymous/tsconfig.json`
- Create: `examples/vue-anonymous/index.html`
- Create: `examples/vue-anonymous/src/main.ts`
- Create: `examples/vue-anonymous/src/App.vue`
- Modify: `package.json` (root) — add `examples/vue-anonymous` to workspaces

- [ ] **Step 8.1: Add `examples/vue-anonymous` to root workspaces**

Open the root `package.json` and change the `workspaces` array:

```json
"workspaces": ["core", "vue", "examples/vue-anonymous", "examples/webhook-receiver"]
```

(We add `examples/webhook-receiver` here too since Task 9 will create it.)

- [ ] **Step 8.2: Create `examples/vue-anonymous/package.json`**

```json
{
  "name": "examples-vue-anonymous",
  "version": "0.0.1",
  "type": "module",
  "private": true,
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "@intake/vue": "*",
    "@intake/core": "*",
    "vue": "SAME_VERSION_AS_vue/package.json"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "SAME_VERSION_AS_vue/package.json",
    "vite": "SAME_VERSION_AS_vue/package.json",
    "vue-tsc": "SAME_VERSION_AS_vue/package.json"
  }
}
```

**Important:** Use the same exact version strings as `vue/package.json` (from Task 1). Copy them verbatim — do not guess.

- [ ] **Step 8.3: Create `examples/vue-anonymous/vite.config.ts`**

```typescript
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    // The relay's cors_origins must include http://localhost:5173
    // See config.yaml at the monorepo root
  },
});
```

- [ ] **Step 8.4: Create `examples/vue-anonymous/tsconfig.json`**

```json
{
  "files": [],
  "references": [
    { "path": "./tsconfig.app.json" }
  ]
}
```

Create `examples/vue-anonymous/tsconfig.app.json`:

```json
{
  "compilerOptions": {
    "composite": true,
    "tsBuildInfoFile": "./node_modules/.tmp/tsconfig.app.tsbuildinfo",
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "jsx": "preserve"
  },
  "include": ["src/**/*.ts", "src/**/*.vue", "vite.config.ts"]
}
```

- [ ] **Step 8.5: Create `examples/vue-anonymous/index.html`**

```html
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Intake Vue Anonymous Example</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

- [ ] **Step 8.6: Create `examples/vue-anonymous/src/main.ts`**

```typescript
import { createApp } from 'vue';
import App from './App.vue';

createApp(App).mount('#app');
```

- [ ] **Step 8.7: Create `examples/vue-anonymous/src/App.vue`**

```vue
<script setup lang="ts">
import { IntakeWidget } from '@intake/vue';
</script>

<template>
  <div class="demo-app">
    <header class="demo-app__header">
      <h1 class="demo-app__title">Intake Demo — Anonymous</h1>
      <p class="demo-app__subtitle">
        This is a minimal host app. Click the chat button in the bottom-right corner to open the support widget.
      </p>
    </header>

    <main class="demo-app__main">
      <p>Your app content goes here.</p>
    </main>

    <!-- The Intake widget: points at the local relay -->
    <IntakeWidget
      relay-url="http://localhost:8080"
      :app-context="{ source: 'vue-anonymous-example', version: '0.1.0' }"
    />
  </div>
</template>

<style>
/* Global reset — keeps demo host app clean */
*, *::before, *::after { box-sizing: border-box; }

body {
  margin: 0;
  font-family: system-ui, sans-serif;
  background-color: #f8fafc;
  color: #1e293b;
}
</style>

<style scoped>
.demo-app {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.demo-app__header {
  padding: 32px 40px 24px;
  border-bottom: 1px solid #e2e8f0;
  background-color: #fff;
}

.demo-app__title {
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 8px;
}

.demo-app__subtitle {
  font-size: 15px;
  color: #64748b;
  margin: 0;
}

.demo-app__main {
  flex: 1;
  padding: 40px;
}
</style>
```

- [ ] **Step 8.8: Install workspace deps from root**

```bash
cd /c/src/ai/intake
npm install
```

Expected: `examples/vue-anonymous` now has `node_modules` with symlinks to `@intake/vue` and `@intake/core`.

- [ ] **Step 8.9: Verify the dev server starts**

```bash
cd /c/src/ai/intake
npm run -w examples-vue-anonymous dev &
```

(Run in background or a new terminal.) Expected output within ~3 seconds:
```
  VITE v5.4.x  ready in XXX ms

  ➜  Local:   http://localhost:5173/
  ➜  Network: ...
```

Stop the server (Ctrl+C or kill the background job) after confirming the URL appears. Full browser verification happens in the smoke.

- [ ] **Step 8.10: Commit**

```bash
cd /c/src/ai/intake
git add package.json examples/vue-anonymous/
git commit -m "feat(1-vi): create examples/vue-anonymous Vite+Vue app with IntakeWidget"
```

---

### Task 9: Create `examples/webhook-receiver/`

**Files:**
- Create: `examples/webhook-receiver/package.json`
- Create: `examples/webhook-receiver/server.mjs`

This is a ~30-line Node HTTP server that logs every POST body to stdout. It is used by the phase smoke and is not a production artifact.

- [ ] **Step 9.1: Create `examples/webhook-receiver/package.json`**

```json
{
  "name": "examples-webhook-receiver",
  "version": "0.0.1",
  "type": "module",
  "private": true,
  "scripts": {
    "start": "node server.mjs"
  }
}
```

- [ ] **Step 9.2: Create `examples/webhook-receiver/server.mjs`**

```javascript
// Tiny webhook receiver for the Phase 1 smoke.
// Logs every POST body to stdout as formatted JSON.
// Usage: node server.mjs   (listens on :9099)

import { createServer } from 'node:http';

const PORT = 9099;
const PATH = '/intake';

const server = createServer((req, res) => {
  if (req.method === 'POST' && req.url === PATH) {
    let body = '';
    req.on('data', (chunk) => { body += chunk; });
    req.on('end', () => {
      const timestamp = new Date().toISOString();
      console.log(`\n[${timestamp}] POST ${PATH}`);
      try {
        const parsed = JSON.parse(body);
        console.log(JSON.stringify(parsed, null, 2));
      } catch {
        console.log('(non-JSON body):', body);
      }
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ received: true }));
    });
  } else {
    res.writeHead(404);
    res.end('Not found');
  }
});

server.listen(PORT, () => {
  console.log(`Webhook receiver listening on http://localhost:${PORT}${PATH}`);
});
```

- [ ] **Step 9.3: Verify the receiver starts**

```bash
cd /c/src/ai/intake
node examples/webhook-receiver/server.mjs &
```

Expected: `Webhook receiver listening on http://localhost:9099/intake`

Send a test POST:
```bash
curl -s -X POST http://localhost:9099/intake \
  -H 'Content-Type: application/json' \
  -d '{"test": true}'
```

Expected receiver stdout:
```
[2026-05-26T...] POST /intake
{
  "test": true
}
```

Kill the receiver after verifying.

- [ ] **Step 9.4: Commit**

```bash
cd /c/src/ai/intake
git add examples/webhook-receiver/
git commit -m "feat(1-vi): add examples/webhook-receiver Node server for phase smoke"
```

---

### Task 10: Create sample `config.yaml` and `examples/README.md`

**Files:**
- Create: `config.yaml` (monorepo root)
- Create: `examples/README.md`

- [ ] **Step 10.1: Create `config.yaml` at the monorepo root**

This is the relay config for local dev. It matches design spec §7 exactly. The webhook URL points at the local receiver used in the smoke. The CORS origin is the Vite dev server default.

```yaml
# config.yaml — local dev relay config for Phase 1 smoke
# Do NOT commit real secrets. ANTHROPIC_API_KEY is read from env only.
server:
  addr: ":8080"
  external_url: "http://localhost:8080"
  cors_origins:
    - "http://localhost:5173"   # examples/vue-anonymous Vite dev server

llm:
  provider: "anthropic"
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"
    model: "claude-sonnet-4-6"
    max_tokens: 1024
  system_prompt_file: ""        # empty = bundled default triage prompt

auth:
  modes:
    anonymous: true

adapters:
  webhook:
    enabled: true
    url: "http://localhost:9099/intake"   # examples/webhook-receiver
    headers: {}
    retry:
      max_attempts: 3
      backoff: "exponential"
```

- [ ] **Step 10.2: Create `examples/README.md`**

```markdown
# Intake Examples

This directory contains runnable examples for the Intake project.

## Prerequisites

- Node 24.12.0+
- Go 1.23.2+
- `ANTHROPIC_API_KEY` exported in your shell
- Monorepo deps installed: `npm install` from the repo root

## Running the Phase 1 Smoke

These three processes must all run simultaneously (use separate terminals):

### Terminal 1 — Webhook receiver

```bash
npm run -w examples-webhook-receiver start
```

Expected: `Webhook receiver listening on http://localhost:9099/intake`

### Terminal 2 — Relay

```bash
cd relay
go run ./cmd/relay --config ../config.yaml
```

Expected: server starts on `:8080`; `GET http://localhost:8080/v1/health` returns `200`.

### Terminal 3 — Vue example

```bash
npm run -w examples-vue-anonymous dev
```

Expected: Vite dev server starts on `http://localhost:5173`.

### Browser

Open `http://localhost:5173`. You will see the demo page.

- Click the **chat bubble** button in the bottom-right corner.
- Type a message describing an issue (e.g. "The login button is broken").
- Send it. The assistant will ask clarifying questions.
- Send at least one more reply.
- Click **Submit**.

The widget should display the returned ticket ID. The webhook receiver (Terminal 1) should log the full canonical payload as formatted JSON.

## CORS Note

The Vite dev server runs on `http://localhost:5173` by default. This origin is already in the sample `config.yaml`:

```yaml
server:
  cors_origins:
    - "http://localhost:5173"
```

If you change the port (`vite --port XXXX`), update `cors_origins` in `config.yaml` to match.

## Security Note

The widget NEVER holds or transmits the `ANTHROPIC_API_KEY`. The key lives only in the relay process (read from env). The browser only ever talks to the relay at `http://localhost:8080`.

## examples/vue-anonymous

A minimal Vite+Vue SPA demonstrating anonymous integration. Mounts `IntakeWidget` from `@intake/vue` pointed at `http://localhost:8080`.

## examples/webhook-receiver

A ~30-line Node HTTP server that logs POST bodies on `:9099/intake`. Used by the smoke to verify the relay delivers a schema-valid canonical payload.
```

- [ ] **Step 10.3: Commit**

```bash
cd /c/src/ai/intake
git add config.yaml examples/README.md
git commit -m "docs(1-vi): add sample config.yaml and examples/README.md with smoke run steps"
```

---

### Task 11: Final verification — all tests green + build clean

- [ ] **Step 11.1: Run all TS/Vue tests**

```bash
cd /c/src/ai/intake/vue
npx vitest run
```

Expected: `Test Files 2 passed (2)` — useIntake.spec.ts + ConversationView.spec.ts, all tests green.

- [ ] **Step 11.2: Build @intake/vue (Vite library mode)**

```bash
cd /c/src/ai/intake/vue
npx vite build
```

Expected: `dist/intake-vue.js` produced, no errors.

- [ ] **Step 11.3: Type-check @intake/vue via build path (LESSONS L001)**

```bash
cd /c/src/ai/intake/vue
npx vue-tsc -b
```

Expected: exits 0, no type errors.

- [ ] **Step 11.4: Root-level type-check**

```bash
cd /c/src/ai/intake
npm run type-check
```

Expected: exits 0.

- [ ] **Step 11.5: Verify pin discipline — no caret on load-bearing tools**

Open `vue/package.json` and verify that `vite`, `@vitejs/plugin-vue`, and `vue-tsc` in `devDependencies` have NO leading `^` or `~`. If caret is present, remove it and re-run `npm install` from root.

- [ ] **Step 11.6: Commit**

```bash
cd /c/src/ai/intake
git add -u
git commit -m "chore(1-vi): verify all tests pass, build clean, no caret on build tools"
```

---

## 5. Smoke — Phase 1 Final Smoke (Verbatim from README §8)

This sub-plan delivers the Phase 1 final smoke. The steps below are copied verbatim from `ai/tasks/phase-1/README.md §8` with minor operational detail added.

```
1. Pre-condition: Phase-1 merged; Node 24.12 + Go 1.23.2; ANTHROPIC_API_KEY exported;
   a local webhook receiver logging POST bodies on http://localhost:9099/intake
   (provided at examples/webhook-receiver/); relay config (config.yaml at monorepo root)
   points adapters.webhook.url at it.

2. Execution:

   a. Terminal 1 — start the webhook receiver:
        npm run -w examples-webhook-receiver start
      Confirm: "Webhook receiver listening on http://localhost:9099/intake"

   b. Terminal 2 — start the relay:
        cd relay && go run ./cmd/relay --config ../config.yaml
      Confirm: GET http://localhost:8080/v1/health -> 200
               GET http://localhost:8080/v1/version -> build info JSON

   c. Terminal 3 — start the example:
        npm run -w examples-vue-anonymous dev
      Open the printed URL (default: http://localhost:5173).

   d. In the browser:
      - Click the launcher button (bottom-right chat bubble). The panel opens.
      - Type "The login button is not working on the checkout page." and click Send.
      - Wait for the assistant to respond and ask a clarifying question (SSE streams
        incrementally — you should see tokens appear word by word).
      - Type a second reply (e.g. "It happens on Chrome 125, mobile view.") and Send.
      - Wait for the assistant's second response.
      - Click Submit.

3. Verification:

   - [X] Assistant tokens stream into the panel incrementally (SSE working end-to-end).

   - [X] The webhook receiver (Terminal 1) logs exactly ONE POST whose body is a
         schema-valid canonical payload:
         - conversation.summary is a non-empty string
         - conversation.title_suggestion is a non-empty string
         - conversation.classification is one of: bug|feature_request|question|other
         - conversation.severity_guess is one of: low|medium|high|critical|unknown
         - conversation.tags_suggested is an array (may be empty)
         - conversation.language is a non-empty string
         - client.url is the example app's URL (http://localhost:5173)
         - client.viewport.w and client.viewport.h are positive integers
         - client.locale is a non-empty string
         - client.user_agent is a non-empty string
         - user.auth_mode = "anonymous"
         - user.verified = false
         - schema_version = "1.0"

   - [X] The widget shows the returned ticket result (external_id) in the panel.

   - [X] Grep the relay's stdout/stderr (Terminal 2) for the ANTHROPIC_API_KEY value:
           grep "$ANTHROPIC_API_KEY" <relay-log-file>
         -> NOT present. Zero matches. (The key never appears in logs or HTTP responses.)

4. Teardown / repeat: stop relay + example + receiver; re-runnable (relay is stateless,
   no persistent storage to reset).
```

---

## 6. Done Criteria

- [ ] `vue/package.json` has exact-pinned versions for `vite`, `@vitejs/plugin-vue`, `vue-tsc` (no caret).
- [ ] `npx vite build` in `vue/` produces `dist/intake-vue.js` with no errors.
- [ ] `npx vue-tsc -b` in `vue/` exits 0 (the build path type-check, per LESSONS L001).
- [ ] `npx vitest run` in `vue/` passes all tests (useIntake + ConversationView).
- [ ] `npm run type-check` from the monorepo root exits 0.
- [ ] `npm run -w examples-vue-anonymous dev` starts the Vite dev server on `:5173`.
- [ ] `npm run -w examples-webhook-receiver start` starts the Node server on `:9099`.
- [ ] The Phase 1 final smoke (§5 above) passes end-to-end: launcher opens → multi-turn streams → Submit → webhook receives schema-valid payload with classify-derived fields → widget shows `external_id` → relay logs do NOT contain the API key value.
- [ ] Phase README §5 tool pin table updated with actual versions used.

---

## Appendix A: `@intake/core` Types Used by This Sub-Plan

Per README §6.7 (frozen interface from 1-v), these are the exact types the widget imports:

```typescript
// From @intake/core — frozen in 1-v

export interface IntakeConfig {
  relayUrl: string;
  widgetVersion: string;
  appContext?: Record<string, unknown>;
}

export interface ChatMessage { role: 'user' | 'assistant'; content: string; }

export interface SubmitResult {
  external_id: string;
  external_url: string;
  adapter_name: string;
  created_at: string;
}

export class IntakeClient {
  constructor(config: IntakeConfig);
  init(): Promise<{ session_id: string; capabilities: { auth_modes: string[]; streaming: boolean } }>;
  turn(messages: ChatMessage[], onDelta: (delta: string) => void): Promise<{ input_tokens: number; output_tokens: number }>;
  submit(messages: ChatMessage[], routingHint?: string): Promise<SubmitResult>;
}
```

Do NOT alter these signatures. If `@intake/core` (1-v) has not been implemented yet, the widget build will fail with module-not-found — implement 1-v first (per the dependency graph: `1-v → 1-vi`).

---

## Appendix B: Security Invariant Reminder

**The widget NEVER handles API keys.** There is no code path in `@intake/vue` that accepts, stores, transmits, or logs a provider API key. The only credential the widget ever sees is the anonymous `session_id` issued by the relay (a UUID, not a secret). All LLM calls happen inside the relay process. This is a load-bearing security invariant (design spec §2, README §2 ADR) — do not add any `apiKey` prop or similar to `IntakeWidget.vue` or `useIntake.ts`.
