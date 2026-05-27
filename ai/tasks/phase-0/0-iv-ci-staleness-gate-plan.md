# Sub-plan 0-iv — CI Staleness Gate

## 1. Goal

Add a GitHub Actions workflow that, on every PR and push to `main`, regenerates types from the schema and fails if the committed generated files differ — the contract that keeps `payload.ts` / `types.go` honest. The same workflow validates the schema fixtures, compiles both targets, and enforces the build-fail-checklist items from the phase README §6 (including the "no caret-pinned codegen tool" grep gate).

## 2. Design references

- CI requirement: [docs/PROJECT.md](../../../docs/PROJECT.md) §15 ("Codegen check: regenerate types from schema; fail if diff")
- Build-fail discipline: [ai/PHASE_PLANNING.md](../../../ai/PHASE_PLANNING.md) §4, §6; phase README §6
- This sub-plan implements the CI half of the phase final smoke (README §7)

## 3. Files touched

| File | Create/Modify | Why |
|---|---|---|
| `.github/workflows/ci.yml` | Create | the PR/push gate: install, codegen, diff-check, validate, compile, pin-grep |
| `.github/workflows/codegen-negative-test.yml` | Create | proves the gate actually catches drift (a deliberately-stale edit must turn CI red) |

## 4. Tasks

- [ ] **Step 1: Write the CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: ci
on:
  pull_request:
  push:
    branches: [main]

jobs:
  contract:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
          cache: npm

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.23.5"

      - name: Install deps
        run: npm ci

      - name: Forbid caret-pinned codegen tools
        run: |
          if grep -E '"(json-schema-to-typescript|ajv-cli|ajv-formats)":\s*"\^' package.json; then
            echo "::error::codegen tool is caret-pinned; PHASE_PLANNING §5 requires exact pins"
            exit 1
          fi
          if grep -E 'go install .*@latest' scripts/codegen-go.sh; then
            echo "::error::go-jsonschema installed with @latest; pin an exact version"
            exit 1
          fi

      - name: Validate schema fixtures
        run: |
          npx ajv validate --spec=draft2020 -c ajv-formats -s schema/payload.v1.json -d schema/testdata/payload.valid.json
          if npx ajv validate --spec=draft2020 -c ajv-formats -s schema/payload.v1.json -d schema/testdata/payload.invalid.json; then
            echo "::error::invalid fixture unexpectedly passed schema validation"
            exit 1
          fi

      - name: Regenerate types from schema
        run: npm run codegen

      - name: Fail if generated types are stale
        run: |
          if ! git diff --exit-code core/src/generated/payload.ts relay/internal/payload/types.go; then
            echo "::error::generated types are stale — run 'npm run codegen' and commit the result"
            exit 1
          fi

      - name: Type-check generated TS
        run: npm run -w @intake/core type-check

      - name: Build + vet generated Go
        working-directory: relay
        run: |
          go build ./...
          go vet ./internal/payload/...
```

- [ ] **Step 2: Push a branch and verify CI is green on a fresh tree**

Run:

```bash
git checkout -b phase-0-ci
git add .github/workflows/ci.yml
git commit -m "ci(phase-0): schema codegen staleness gate"
git push -u origin phase-0-ci
gh pr create --fill
gh pr checks --watch
```

Expected: the `contract` job passes — fixtures validate, codegen produces no diff, both targets compile.

- [ ] **Step 3: Write the negative test (prove the gate catches drift)**

This is a scheduled/manual workflow that deliberately makes the schema stale and asserts the diff-check step fails — so we know the gate isn't a no-op.

Create `.github/workflows/codegen-negative-test.yml`:

```yaml
name: codegen-negative-test
on:
  workflow_dispatch:
  schedule:
    - cron: "0 6 * * 1"   # weekly Monday 06:00 UTC

jobs:
  gate-catches-drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version-file: .nvmrc
          cache: npm
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23.5"
      - run: npm ci

      - name: Mutate schema WITHOUT regenerating
        run: |
          node -e "const fs=require('fs');const p='schema/payload.v1.json';const s=JSON.parse(fs.readFileSync(p));s.properties.client.properties.debug_note={type:'string'};fs.writeFileSync(p,JSON.stringify(s,null,2)+'\n');"

      - name: Regenerate and assert the diff-check WOULD fail
        run: |
          npm run codegen
          if git diff --exit-code core/src/generated/payload.ts relay/internal/payload/types.go; then
            echo "::error::staleness gate did NOT detect drift after a schema change — the gate is broken"
            exit 1
          fi
          echo "OK: gate correctly detects drift (diff is non-empty after schema mutation)."
```

- [ ] **Step 4: Run the negative test manually and verify it passes**

Run:

```bash
git add .github/workflows/codegen-negative-test.yml
git commit -m "ci(phase-0): negative test proving staleness gate catches drift"
git push
gh workflow run codegen-negative-test.yml --ref phase-0-ci
gh run watch
```

Expected: the `gate-catches-drift` job passes — meaning it successfully detected the deliberate drift (non-empty diff), confirming the gate works.

- [ ] **Step 5: Merge the PR**

```bash
gh pr checks --watch   # confirm green
gh pr merge --squash --delete-branch
```

Expected: `ci` is green on `main`.

## 5. Smoke

This sub-plan's smoke IS the phase final smoke's CI arm (README §7).

```
1. Pre-condition: PR open from phase-0-ci with all of 0-i..0-iii merged into the branch.
2. Execution:
   a. Observe the `contract` job on the PR.
   b. Locally: edit schema/payload.v1.json (add an optional property), commit WITHOUT running codegen, push.
   c. Observe the `contract` job re-run.
   d. Run `npm run codegen`, commit the regenerated files, push.
   e. Trigger codegen-negative-test.yml via workflow_dispatch.
3. Verification:
   - (a) green.
   - (c) RED at the "Fail if generated types are stale" step (proves the gate blocks un-regenerated schema edits).
   - (d) green again.
   - (e) green (proves the gate detects drift by construction).
4. Teardown / repeat: revert the throwaway schema edit; re-runnable on any branch.
```

## 6. Done criteria

- [ ] `ci.yml` runs on PR + push to main; green on a fresh tree.
- [ ] A schema edit without regeneration turns the `contract` job RED at the staleness step (smoke §5c).
- [ ] Caret-pin grep gate and `@latest` grep gate are present and pass.
- [ ] Schema-fixture validation (accept + reject) runs in CI.
- [ ] `codegen-negative-test.yml` passes via manual dispatch, proving the gate is not a no-op.
- [ ] This satisfies the phase README §7 final smoke (CI arm).
```
