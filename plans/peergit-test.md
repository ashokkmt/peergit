# Test PeerGit before committing (Windows PowerShell)

Run commands from the repository root. This is the test command checklist for agents and contributors. Use [the local run guide](peergit-run.md) to start the app when you need to inspect it in a browser. Run the **baseline** for every code change, then the checks whose trigger matches your change. A check that reports `SKIP` is not a pass for the behavior it skipped. Record commands, results, and unavailable checks in `WORKLOG.md`.

## Baseline for every code change

```powershell
$goFiles = git ls-files --cached --others --exclude-standard -- '*.go'; $unformatted = gofmt -l $goFiles; if ($unformatted) { $unformatted; throw 'Go files need gofmt' }
go test ./... -count=1
go vet ./...
staticcheck ./...
git diff --check
```

Install the repository's CI version of staticcheck if the command is missing:

```powershell
go install honnef.co/go/tools/cmd/staticcheck@2026.2.1
```

Each command must finish successfully. `go test ./...` runs all Go unit tests, but PostgreSQL integration tests **skip** if `TEST_DATABASE_URL` is unset; use the database section below whenever those tests cover the change. The formatting check examines tracked and new, non-ignored Go files. Format a changed file with `gofmt -w <path>` before rerunning it. For documentation-only changes, validate links and examples you changed and run `git diff --check`; the Go baseline is unnecessary.

## PostgreSQL integration — required for database, migration, identity, project, recruitment, job, or outbox changes

Start Docker Desktop with Linux containers, then run:

```powershell
docker compose -f deploy/compose/local.yml up -d --wait postgres
$env:TEST_DATABASE_URL = 'postgres://peergit:local-development-only@127.0.0.1:5432/peergit?sslmode=disable'
go test ./tests/integration -count=1 -v
```

Confirm the relevant named tests say `PASS` and none say `SKIP`. The integration tests create unique temporary PostgreSQL schemas and delete only those schemas afterward. Use the disposable local database from Compose; never point `TEST_DATABASE_URL` at production. To run the Phase 3 regression alone first:

```powershell
go test ./tests/integration -run '^TestPhase3ProjectRecruitmentAndOwnershipFlows$' -count=1 -v
```

After the focused test passes, run the full integration package and baseline Go checks. Remove the variable when finished in that shell:

```powershell
Remove-Item Env:TEST_DATABASE_URL
```

If Docker Desktop or PostgreSQL is unavailable, the database check remains **unverified**. Record the exact failing command and reason in `WORKLOG.md`, and rely on CI to run it before merging. Do not claim that a skipped local test passed.

## Migration command — only when migration files or the migrator change

Use the disposable local PostgreSQL service. The application migrator records checksums in `schema_migrations`: rerunning an unchanged migration is a safe no-op, while editing an already applied migration is rejected. Running it again does **not** recreate existing tables. The isolated integration test also checks blank-to-head and repeat application.

```powershell
docker compose -f deploy/compose/local.yml up -d --wait postgres
$env:DATABASE_URL = 'postgres://peergit:local-development-only@127.0.0.1:5432/peergit?sslmode=disable'
$env:TEST_DATABASE_URL = $env:DATABASE_URL
go run ./cmd/migrate
go run ./cmd/migrate
go test ./tests/integration -run '^TestPlatformMigrationsAndDurableWork$' -count=1 -v
Remove-Item Env:DATABASE_URL
Remove-Item Env:TEST_DATABASE_URL
```

The second migration run proves repeat behavior against the local database. This section is conditional because most code changes add no SQL; it is not unsafe to repeat migrations. Do not reset volumes or run migrations against production as a pre-commit check.

## Frontend — when `apps/web` or browser behavior changes

```powershell
npm --prefix apps/web ci
npm --prefix apps/web run typecheck
npm --prefix apps/web run build
```

For the responsive browser walkthrough, install its dependencies, start the built web app in a **separate** PowerShell window with `npm --prefix apps/web run start`, then run:

```powershell
npm --prefix tests/e2e ci
Push-Location tests/e2e
npx playwright install chromium
Pop-Location
$env:CI = '1'
npm --prefix tests/e2e test
Remove-Item Env:CI
```

The browser check needs `http://127.0.0.1:3000`; it intercepts `/readyz`, so it does not need the API or PostgreSQL. Stop the web server after the test. Running with `CI=1` selects Playwright's installed Chromium instead of the local Edge channel.

## Infrastructure or local configuration — when affected files change

For `deploy/compose/local.yml`, run:

```powershell
docker compose -f deploy/compose/local.yml config --quiet
```

For API/worker runtime or service configuration changes, start the affected local services and processes using [the run guide](peergit-run.md), then smoke-test the relevant health endpoint or behavior. This is conditional because a pure code or documentation edit need not start every service.

Phase 0 live verification in ignored `cmd/verify` uses external fixtures and credentials. It is **not** a pre-commit check. Run it only when changing that verifier or its integration, following `plans/phase0-requirements.md` locally; never commit `.env.verify`, `.secrets/`, or raw proof artifacts.
