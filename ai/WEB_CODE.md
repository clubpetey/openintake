# WEB_CODE.md — Instructions for editing/creating HTML, CSS, JS and TS files

This is an application for referee scheduling using the Timefold constraint solver. Read `PROJECT.md` for the full specification. Read `DEV.md` for architecture and development decisions.

### Do
- use apache echarts for charts. do not supply custom html
- default to small components. prefer focused modules over god components
- default to small files and diffs. avoid repo wide rewrites unless asked
- use quasar components first, if they exist

### Don't
- do not hard code colors
- do not use `div`s if we have a component already
- do not add new heavy dependencies without approval

### Commands

# Type check a single file by path
npm run tsc --noEmit path/to/file.tsx

# Format a single file by path
npm run prettier --write path/to/file.tsx

# Lint a single file by path
npm run eslint --fix path/to/file.tsx

# Unit tests - pick one
npm run vitest run path/to/file.test.tsx

# Full build when explicitly requested
yarn build:app

Note: Always lint, test, and typecheck updated files. Use project-wide build sparingly.

### Safety and permissions

Allowed without prompt:
- read files, list files
- tsc single file, prettier, eslint,
- vitest single test

Ask first: 
- package installs,
- git push
- deleting files, chmod
- running full build or end to end suites

### PR checklist
- title: `feat: short description`
- lint, type check, unit tests - all green before commit
- diff is small and focused. include a brief summary of what changed and why
- remove any excessive logs or comments before sending a PR

### When stuck
- ask a clarifying question, propose a short plan, or open a draft PR with notes
- do not push large speculative changes without confirmation

### Test first mode
- when adding new features: write or update unit tests first, then code to green
- prefer component tests for UI state changes
- for regressions: add a failing test that reproduces the bug, then fix to green


### Code Style — JavaScript & Vue

- **Linting & formatting:** Use `ESLint` with `plugin:vue/vue3-recommended`, `eslint:recommended`, `@typescript-eslint/recommended` (if using TS) and `prettier` integration. Run `prettier` and `eslint --fix` as part of pre-commit hooks (e.g., `husky` + `lint-staged`).
- **Prettier config:** Recommend sensible defaults: `printWidth: 100`, `singleQuote: true`, `trailingComma: 'es5'`, `semi: true`.
- **TypeScript:** Prefer TypeScript for new code. Enable strict mode and use `<script setup lang="ts">` for components.
- **Editor settings:** Include an `.editorconfig` and share `vscode` recommended settings (format on save using the workspace Prettier/ESLint).
- **Naming conventions:** Components in `PascalCase` (both file and component name), composables named `useXxx` (file `useXxx.ts`), stores prefixed with `use` or `<domain>Store`, composable hooks return named exports.
- **File layout:** Keep one main component per file. Co-locate tests (`Component.spec.ts`) next to implementation.
- **Style & tokens:** Do not hard-code colors or spacing — use the project's design tokens (see `DynamicStyles.tsx` or design system tokens). Prefer scoped styles or CSS modules and avoid global style leakage.
- **Code quality rules:** Avoid `console.log` in committed code, prefer early returns, keep functions small and pure where reasonable, and prefer descriptive names over abbreviations.

### UI Testing Best Practices

- **Test types:** Unit tests for composables and utilities using `Vitest` + `Vue Test Utils`. Component tests for interactions, emits, and slot behavior. E2E tests for critical user flows using `Playwright`.
- **Mocking & network:** Use `msw` (Mock Service Worker) to mock network calls for unit/component tests; keep E2E tests against a realistic environment (staging or test fixtures).
- **Selectors & stability:** Use `data-testid` attributes for test selectors; avoid relying on implementation-specific classes or fragile DOM positions.
- **Accessibility:** Run automated accessibility checks (axe-core) in unit/component tests and part of E2E flows for key pages.
- **Deterministic tests:** Avoid timing-based assertions; use `await`/`nextTick` and explicit waits for elements. Keep tests isolated and reset global state between tests.
- **Visual regressions:** For UI-critical surfaces, add visual snapshots (Playwright snapshots or a visual regression service) and review diffs as part of PR review for design changes.
- **Co-located tests & naming:** Place test files next to components (`MyComponent.vue` → `MyComponent.spec.ts`). Name test files `*.spec.ts` and keep tests focused and fast.
- **CI:** Run unit tests and linters on every PR; run E2E in a separate pipeline or gated job against a test environment. Fail PRs on flaky tests so they are fixed early.
