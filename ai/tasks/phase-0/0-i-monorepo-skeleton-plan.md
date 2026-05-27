# Sub-plan 0-i — Monorepo Skeleton & Tooling

## 1. Goal

Stand up the empty monorepo: directory layout matching PROJECT.md §14, the npm workspace (`core`, `vue`), the relay Go module, the maintainer-only `license-tool` Go module, version-pinning files (`.nvmrc`, `engines`, `go.mod` toolchain), and the root `package.json` that will host the `codegen` script. No business logic — just a tree that installs and builds cleanly so later sub-plans have somewhere to write generated code.

## 2. Design references

- Repo layout: [docs/PROJECT.md](../../../docs/PROJECT.md) §14
- Tool pins: phase README [§5](README.md)
- Name-placeholder gate: design doc §6 — isolate `intake` tokens

## 3. Files touched

| File | Create/Modify | Why |
|---|---|---|
| `package.json` (root) | Create | npm workspace root; hosts `codegen`/`type-check` scripts; declares `engines` |
| `.nvmrc` | Create | pins Node 20.18 for contributors and CI |
| `.gitignore` | Create | ignore `node_modules`, build output, `*.local` |
| `core/package.json` | Create | `@intake/core` package manifest (workspace member) |
| `core/tsconfig.json` | Create | TS config; `noEmit` type-check target for generated code |
| `core/src/index.ts` | Create | empty entrypoint so the package resolves |
| `core/src/generated/.gitkeep` | Create | reserve the generated dir (0-iii writes here) |
| `vue/package.json` | Create | `@intake/vue` package manifest (workspace member, stub) |
| `relay/go.mod` | Create | relay Go module `intake`, Go 1.23.5 toolchain |
| `relay/cmd/relay/main.go` | Create | minimal `package main` so `go build ./...` has a target |
| `relay/internal/payload/.gitkeep` | Create | reserve dir for generated `types.go` (0-iii) |
| `license-tool/go.mod` | Create | maintainer-only module `intake-license-tool` |
| `license-tool/cmd/intake-license/main.go` | Create | minimal `package main` |
| `README.md` (root) | Create | one-paragraph repo stub + build prerequisites |

## 4. Tasks

- [ ] **Step 1: Confirm and record tool versions**

Run each and record actual versions; if any differs from phase README §5, update that table in this PR.

```bash
go version          # expect go1.23.5
node --version      # expect v20.18.x
npm --version       # expect 10.x
```

Expected: versions match README §5 (or table is updated to the confirmed versions).

- [ ] **Step 2: Write the root `package.json` (npm workspace)**

Create `package.json`:

```json
{
  "name": "intake-monorepo",
  "private": true,
  "version": "0.0.0",
  "workspaces": ["core", "vue"],
  "engines": { "node": ">=20.18 <21" },
  "scripts": {
    "type-check": "npm run -w @intake/core type-check",
    "codegen": "echo \"codegen wired in sub-plan 0-iii\" && exit 1"
  },
  "devDependencies": {}
}
```

The `codegen` script is a deliberate failing stub until 0-iii replaces it — so an accidental early run fails loudly rather than silently no-op'ing.

- [ ] **Step 3: Write `.nvmrc` and `.gitignore`**

Create `.nvmrc`:

```
20.18
```

Create `.gitignore`:

```
node_modules/
dist/
*.local
.DS_Store
relay/intake-relay
relay/relay
*.out
```

- [ ] **Step 4: Write the `@intake/core` package**

Create `core/package.json`:

```json
{
  "name": "@intake/core",
  "version": "0.0.0",
  "type": "module",
  "main": "src/index.ts",
  "scripts": {
    "type-check": "tsc --noEmit"
  },
  "devDependencies": {
    "typescript": "5.6.3"
  }
}
```

Create `core/tsconfig.json`:

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
  "include": ["src/**/*.ts"]
}
```

Create `core/src/index.ts`:

```typescript
// @intake/core — shared TS engine. Populated in Phase 1.
export {};
```

Create `core/src/generated/.gitkeep` (empty file).

- [ ] **Step 5: Write the `@intake/vue` package stub**

Create `vue/package.json`:

```json
{
  "name": "@intake/vue",
  "version": "0.0.0",
  "type": "module",
  "private": true,
  "scripts": {
    "type-check": "echo \"vue type-check added in Phase 1\""
  }
}
```

- [ ] **Step 6: Write the relay Go module**

Create `relay/go.mod`:

```
module intake

go 1.23

toolchain go1.23.5
```

Create `relay/cmd/relay/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("intake-relay: skeleton (Phase 0). Server lands in Phase 1.")
}
```

Create `relay/internal/payload/.gitkeep` (empty file).

- [ ] **Step 7: Write the license-tool Go module**

Create `license-tool/go.mod`:

```
module intake-license-tool

go 1.23

toolchain go1.23.5
```

Create `license-tool/cmd/intake-license/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("intake-license: maintainer-only tool (Phase 0 stub). Implemented in Phase 3.")
}
```

- [ ] **Step 8: Write the root README stub**

Create `README.md`:

```markdown
# intake

> Working name — final name TBD (see docs/specs decomposition design §6).

AI-native, self-hostable feedback & support intake: an embeddable widget + a single-binary Go relay.

## Repo layout
- `core/` — `@intake/core` shared TypeScript engine
- `vue/` — `@intake/vue` widget
- `relay/` — `intake-relay` Go binary
- `license-tool/` — maintainer-only license signer (not published)
- `schema/` — `payload.v1.json` wire contract (source of truth)

## Prerequisites
- Node 20.18 (`nvm use`)
- Go 1.23.5

## Build
```bash
npm ci
npm run codegen     # regenerate types from schema
cd relay && go build ./...
```

See `docs/` for full documentation.
```

- [ ] **Step 9: Verify the workspace installs**

Run:

```bash
npm install
```

Expected: completes without error; `node_modules/` created; `core` and `vue` resolved as workspaces (no "workspace not found" warning).

- [ ] **Step 10: Verify both Go modules build**

Run:

```bash
cd relay && go build ./... && cd ../license-tool && go build ./... && cd ..
```

Expected: both build with no error; prints nothing (build success).

- [ ] **Step 11: Verify TS type-check runs**

Run:

```bash
npm run -w @intake/core type-check
```

Expected: `tsc --noEmit` exits 0 (no `.ts` errors; `generated/` is empty so far).

- [ ] **Step 12: Commit**

```bash
git add package.json .nvmrc .gitignore core vue relay license-tool README.md
git commit -m "feat(phase-0): monorepo skeleton, workspaces, Go modules, tool pins"
```

## 5. Smoke

```
1. Pre-condition: clean checkout at this sub-plan's commit; Node 20.18 + Go 1.23.5 installed; node_modules absent.
2. Execution: npm install; cd relay && go build ./...; cd ../license-tool && go build ./...; cd ..; npm run -w @intake/core type-check
3. Verification: npm install completes with core+vue recognized as workspaces; both go builds exit 0; tsc --noEmit exits 0; the repo tree matches PROJECT.md §14 for the directories created here.
4. Teardown / repeat: rm -rf node_modules; re-run — idempotent.
```

## 6. Done criteria

- [ ] All 12 steps complete and committed.
- [ ] Smoke (§5) passes from a clean checkout.
- [ ] Tool versions confirmed and README §5 matches reality.
- [ ] `npm run codegen` is a *failing* stub (not a silent no-op).
- [ ] No name-bearing token outside the isolated set (`module intake`, `@intake/*`, README title) — verified by `git grep -n "intake"` returning only expected lines.
