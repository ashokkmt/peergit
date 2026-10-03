# Test PeerGit before committing

Run commands from the repository root. Use [the local run guide](peergit-run.md) to start the app for interactive development.

## Run the full CI-equivalent test script

The one command is the same on Windows, Linux, and macOS:

```text
node scripts/test.mjs
```

Before running it, install Go 1.27.1, Node.js 24 with npm, Git, and Docker Desktop/Engine with the Compose plugin. Start Docker Desktop on Windows or macOS. Check the toolchain and Docker connection with these commands:

```text
go version
node --version
npm --version
git --version
docker version
docker compose version
```

The first run needs network access for Go/npm dependencies and Playwright Chromium. The script installs staticcheck at the version used in CI if it is missing. GitHub Actions always uses `npm ci` and installs Playwright's Linux system packages on its clean runner. Locally, the script runs `npm ci` only when dependencies are absent or either package file changed; it saves a lock hash under ignored `node_modules`. It installs Playwright Chromium only when the version required by the installed package is missing. On Linux, that first browser install also uses `--with-deps` and may request `sudo`. Stop any app already using TCP port 3000.

The script runs Go formatting, starts the Compose services, applies migrations, runs all Go tests with PostgreSQL integration enabled and caching disabled, runs `go vet` and staticcheck, verifies/installs web dependencies, typechecks and builds Next.js, verifies/installs Chromium, starts the production web build, waits up to 30 seconds for port 3000, runs Playwright, validates Compose, and checks `git diff --check`. It uses the local disposable database URL from CI and preserves Docker volumes. It records which services were already running and stops only those it started. In GitHub Actions it brings the Compose stack down at the end.

GitHub Actions calls this same script after setting up Go and Node. A successful local run therefore exercises the same test sequence as CI. A future Makefile can call `node scripts/test.mjs` directly.

If a command fails, the script stops the sequence, prints the Next.js log when relevant, stops its web process, cleans up services it started, and returns a nonzero exit code. Fix the failure and rerun the full script before committing.

## What each test checks

| Check | What it verifies |
|---|---|
| Go formatting | All Go source files pass `gofmt`. |
| PostgreSQL migration | Checked-in migrations apply to the local development database. Repeating unchanged migrations is safe. |
| Go tests | Unit tests plus PostgreSQL integration coverage for OIDC, identity/campus, projects/recruitment, jobs/outbox, and migration blank-to-head/repeat behavior. |
| `go vet` | Go type and common correctness checks. |
| staticcheck | Go static analysis using `honnef.co/go/tools` version `2026.2.1`. |
| Web typecheck/build | TypeScript type checking and optimized Next.js production compilation. |
| Playwright | Keyboard skip link, form labels, desktop/mobile overflow, readiness/request IDs, guest project search behavior, and API-unavailable display. |
| Compose validation | The Docker Compose configuration parses successfully. |
| Diff check | `git diff --check` catches whitespace errors in tracked changes. |

The browser test stubs `/readyz` and the guest `/api/v1/session` response, so it does not sign in or call a live API. It checks that guests are prompted to sign in before project discovery. It does not test OAuth, signed-in project search, or backend behavior; the PostgreSQL integration tests cover backend workflows.

## Run individual checks

Use these commands to narrow a failure. Run the full script before committing.

### Go formatting, unit tests, vet, and staticcheck

PowerShell formatting command:

```powershell
$goFiles = git ls-files --cached --others --exclude-standard -- '*.go'; $unformatted = gofmt -l $goFiles; if ($unformatted) { $unformatted; throw 'Go files need gofmt' }
```

Then run the common Go commands:

```text
go test ./... -count=1
go vet ./...
staticcheck ./...
git diff --check
```

The same common commands work in Bash/zsh. The equivalent Go formatting check in Bash/zsh is:

```sh
unformatted=$(gofmt -l $(git ls-files --cached --others --exclude-standard -- '*.go'))
test -z "$unformatted" || { printf '%s\n' "$unformatted"; exit 1; }
```

Install the repository's CI version of staticcheck if the command is missing:

```powershell
go install honnef.co/go/tools/cmd/staticcheck@2026.2.1
```

These commands work in PowerShell, Bash, and zsh. `go test ./...` skips database integration tests when `TEST_DATABASE_URL` is unset; set it as shown below when testing database-related changes. Format a changed file with `gofmt -w <path>`. For documentation-only changes, validate the changed links/examples and run `git diff --check`.

### PostgreSQL integration tests

Start Docker Desktop/Engine with Compose. In PowerShell, run:

```powershell
docker compose -f deploy/compose/local.yml up -d --wait postgres
$env:TEST_DATABASE_URL = 'postgres://peergit:local-development-only@127.0.0.1:5432/peergit?sslmode=disable'
go test ./tests/integration -count=1 -v
Remove-Item Env:TEST_DATABASE_URL
```

In Bash/zsh, run:

```sh
docker compose -f deploy/compose/local.yml up -d --wait postgres
export TEST_DATABASE_URL='postgres://peergit:local-development-only@127.0.0.1:5432/peergit?sslmode=disable'
go test ./tests/integration -count=1 -v
unset TEST_DATABASE_URL
```

Confirm the relevant named tests say `PASS` and none say `SKIP`. The integration tests create unique temporary PostgreSQL schemas and delete only those schemas afterward. Use the disposable local database from Compose; never point `TEST_DATABASE_URL` at production. To run the Phase 3 regression alone first:

```powershell
go test ./tests/integration -run '^TestPhase3ProjectRecruitmentAndOwnershipFlows$' -count=1 -v
```

The Phase 3 test includes role creation with omitted prerequisite skills, application decisions, invitation acceptance, and concurrent capacity checks. After the focused test passes, run the full integration package and core Go checks.

```powershell
Remove-Item Env:TEST_DATABASE_URL
```

If Docker Desktop or PostgreSQL is unavailable, the database check remains **unverified**. Record the exact failing command and reason in `WORKLOG.md`, and rely on CI to run it before merging. Do not claim that a skipped local test passed.

### Migration behavior and focused test

The application migrator records checksums in `schema_migrations`: rerunning an unchanged migration is a safe no-op, while editing an applied migration is rejected. Running it does **not** recreate existing tables. The full script always runs it against the disposable local Compose database, matching CI. To run the migration integration test alone, set both database variables.

```powershell
docker compose -f deploy/compose/local.yml up -d --wait postgres
$env:DATABASE_URL = 'postgres://peergit:local-development-only@127.0.0.1:5432/peergit?sslmode=disable'
$env:TEST_DATABASE_URL = $env:DATABASE_URL
go run ./cmd/migrate
go test ./tests/integration -run '^TestPlatformMigrationsAndDurableWork$' -count=1 -v
Remove-Item Env:DATABASE_URL
Remove-Item Env:TEST_DATABASE_URL
```

The integration test proves blank-to-head and repeat behavior in an isolated temporary schema. These focused commands are useful when debugging migrations; most code changes should use the complete script instead. Never reset the local volume or run these checks against production as a pre-commit step.

### Frontend typecheck and production build

```powershell
npm --prefix apps/web ci
npm --prefix apps/web run typecheck
npm --prefix apps/web run build
```

### Browser walkthrough

The full script installs Chromium, starts the built web app, waits for port 3000, and runs Playwright. To run only this test, install dependencies and the browser, then start `npm --prefix apps/web run start` in another terminal:

```powershell
npm --prefix tests/e2e ci
Push-Location tests/e2e
npx playwright install chromium
Pop-Location
$env:CI = '1'
npm --prefix tests/e2e test
Remove-Item Env:CI
```

The browser check needs `http://127.0.0.1:3000`; it intercepts `/readyz` and `/api/v1/session`, so it does not need the API or PostgreSQL. It uses Chromium on all platforms when `CI=1`. Stop the web server after the test.

### Compose and service smoke checks

For `deploy/compose/local.yml`, run:

```powershell
docker compose -f deploy/compose/local.yml config --quiet
```

For API/worker runtime or service configuration changes, start the affected local services and processes using [the run guide](peergit-run.md), then smoke-test the relevant health endpoint or behavior. Phase 0 live verification in ignored `cmd/verify` uses external fixtures and credentials. It is not part of CI/pre-commit; run it only when changing that verifier or its integration, following `plans/phase0-requirements.md` locally. Never commit `.env.verify`, `.secrets/`, or raw proof artifacts.
