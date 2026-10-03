# Run PeerGit locally (Windows PowerShell)

This guide runs the Go API, optional worker, and Next.js web app on your Windows host. Docker runs the supporting services. Use synthetic projects and test accounts only. [Local services](local-services.md) explains each container and its production counterpart.

Run every command from the repository root, `D:\projects\peergit`, unless a step says otherwise. The Go commands load `.env` from the current directory automatically. Next.js needs no separate API URL because Caddy routes `/api/*` to the API on the same origin.

## 1. Check prerequisites

Install Go 1.27.1 or a compatible 1.27 patch, Node.js 24, npm, Git, and Docker Desktop with the Linux container engine. Start Docker Desktop before using Compose.

```powershell
go version
node --version
npm --version
git --version
docker version
docker compose version
```

`docker version` must show a **Server** section. If it reports that `dockerDesktopLinuxEngine` cannot be found, start Docker Desktop and select Linux containers, then retry. Ensure ports 80, 443, 3000, 5432, 8025, 8080, 8333, and 1025 are free.

## 2. Configure the local application

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
```

Edit the ignored `.env` file. Keep the local `DATABASE_URL`, `OBJECT_*`, `APP_ORIGIN=https://localhost`, and `COOKIE_SECURE=true` values from the example for this Compose stack. The default keys and passwords are for local development only. `SMTP_HOST` is reserved; the current Phase 3 app does not send invitation email. Do not put real credentials in chat or Git.

For browser sign-in, create a Google OAuth **Web application** client as described in [identity and media setup](../ops/identity-and-media.md). Its authorized JavaScript origin must be `https://localhost` and its redirect URI must be `https://localhost/api/v1/auth/callback`. Put its values in `.env`:

```dotenv
GOOGLE_OIDC_ISSUER=https://accounts.google.com
GOOGLE_OIDC_CLIENT_ID=<your local test client ID>
GOOGLE_OIDC_CLIENT_SECRET=<your local test client secret>
GOOGLE_OIDC_REDIRECT_URL=https://localhost/api/v1/auth/callback
```

If the Google app is in testing mode, allow the test accounts in its test-user list. Phase 3 browser testing needs at least two controlled Google accounts whose verified email domains are admitted to the same local campus. Without Google setup, the API, web preview, and database integration tests can run, but interactive project creation and applications cannot.

GitHub App credentials and Cloudflare R2 are **not** needed for Phase 3. Those integrations belong to later repository-evidence work.

## 3. Start each Docker service

The Compose definition is [deploy/compose/local.yml](../deploy/compose/local.yml). Start the services separately to see which dependency fails:

```powershell
docker compose -f deploy/compose/local.yml up -d --wait postgres
docker compose -f deploy/compose/local.yml up -d --wait objects
docker compose -f deploy/compose/local.yml up -d --wait mailpit
docker compose -f deploy/compose/local.yml up -d --wait caddy
docker compose -f deploy/compose/local.yml ps
```

PostgreSQL is required for sign-in, projects, sessions, jobs, and audit records. `objects` is SeaweedFS, used when testing profile images or project media. Caddy provides browser-visible `https://localhost` and forwards API and web traffic to the host. Mailpit is an optional local email sink; Phase 3 invitations currently produce a one-time link for manual sharing, so Mailpit may stay stopped for project-only testing. Caddy depends on the PostgreSQL container; start the Go API and web app next even if Caddy initially returns an upstream error.

Check the database and object service:

```powershell
docker compose -f deploy/compose/local.yml exec -T postgres pg_isready -U peergit -d peergit
docker compose -f deploy/compose/local.yml exec -T objects wget -q -O - http://127.0.0.1:9333/cluster/status
```

## 4. Apply database migrations

```powershell
go run ./cmd/migrate
```

The migrator applies all SQL files through `0005_projects_recruitment.sql` and is safe to rerun. The API does not migrate on startup. If the command cannot connect, check `docker compose -f deploy/compose/local.yml ps` and the `DATABASE_URL` in `.env`. If a migration fails, its SQL transaction rolls back; fix or update the migration source and rerun the command. Do not erase the database volume to recover from a failed migration.

## 5. Add one local campus for browser testing

Do this **once** in a disposable local database. Replace `your-test-domain.edu` below with the verified email domain of the Google test accounts you will use; keep it lowercase. The campus domain is what permits those accounts to join after Google sign-in. This is local fixture setup, not a way to claim a real institution in production.

```powershell
@'
WITH new_college AS (
  INSERT INTO colleges(slug,name)
  VALUES ('local-campus','Local Test Campus')
  RETURNING id
)
INSERT INTO college_domains(college_id,domain,verified_at)
SELECT id,'your-test-domain.edu',now() FROM new_college;
'@ | docker compose -f deploy/compose/local.yml exec -T postgres psql -v ON_ERROR_STOP=1 -U peergit -d peergit
```

If you already created a campus in this database, do not insert it again. Verify the configured domain with:

```powershell
docker compose -f deploy/compose/local.yml exec -T postgres psql -U peergit -d peergit -c "SELECT c.slug,d.domain,d.verified_at FROM colleges c JOIN college_domains d ON d.college_id=c.id;"
```

For campus invitations, organization administration, or MFA testing, follow the separate [operator bootstrap steps](../ops/identity-and-media.md). A regular verified campus account can exercise the Phase 3 project journey without an administrator role.

## 6. Run API and frontend in separate host terminals

Keep each process running in its own PowerShell window, with the repository root as the current directory.

**Terminal A — Go API:**

```powershell
go run ./cmd/api
```

**Terminal B — Next.js frontend:**

```powershell
npm --prefix apps/web ci
npm --prefix apps/web run dev
```

**Terminal C — optional Go worker:**

```powershell
go run ./cmd/worker
```

The worker polls PostgreSQL jobs and outbox records. Phase 3 browser screens do not need it, and no Phase 3 notification/email consumer is registered yet; project events remain durable for later processing. The API listens on `127.0.0.1:8080`, Next.js on `127.0.0.1:3000`, and Caddy connects them at `https://localhost`.

Check the backend directly, then the full proxy path:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:8080/readyz
curl.exe -k https://localhost/readyz
```

The `-k` flag is only for checking Caddy's local development certificate. Use **`https://localhost` in the browser** for sign-in and Phase 3 testing. Opening port 3000 directly bypasses the API proxy and the configured secure-cookie origin.

If the browser warns about the certificate, trust only the local Caddy CA on this development machine. One option is to copy its certificate to the ignored `.secrets` folder and import it to the current user's Windows trust store:

```powershell
New-Item -ItemType Directory -Force .secrets | Out-Null
docker compose -f deploy/compose/local.yml cp caddy:/data/caddy/pki/authorities/local/root.crt .secrets/caddy-local-root.crt
Import-Certificate -FilePath .secrets/caddy-local-root.crt -CertStoreLocation Cert:\CurrentUser\Root
```

## 7. Exercise the Phase 3 screens

1. Open `https://localhost` and sign in as test account A. Accept the current Terms and Privacy drafts in the Account panel.
2. Create a draft project with a title, slug, summary, visibility, and skills. Open it, add a role with openings/difficulty, and change the lifecycle to **Active**. Active campus-visible projects appear in discovery; private projects remain available to their team.
3. In a separate browser profile, sign in as account B and accept the policies. Find the project, apply to an open role, and try a duplicate pending application; the duplicate must be rejected. Account B can also withdraw a pending application.
4. Return to account A. Review the private application and accept or reject it. Acceptance adds B to the team. A role closes when its capacity is filled.
5. To test invitations, have a third test account sign in once so it appears in the campus member search. Account A invites it and shares the one-time link shown by the UI. The invitee opens that link in its own browser profile and explicitly accepts. The same link cannot be accepted twice.
6. Check membership role changes and removal. The last owner must not be removable; transfer ownership first. Try changing a project's visibility or moving it on hold and verify that recruiting closes.

The current screen uses sections rather than separate tabs. It shows API conflict/error messages; you may need to reopen a project after a change to refresh its latest version. The project invitation link is returned once and is not emailed by the app.

## 8. Run the PostgreSQL integration checks

The Phase 3 integration test creates a temporary schema in the local PostgreSQL database and drops it afterward. Run it only against this local development database:

```powershell
$env:TEST_DATABASE_URL = 'postgres://peergit:local-development-only@127.0.0.1:5432/peergit?sslmode=disable'
go test ./tests/integration -run TestPhase3ProjectRecruitmentAndOwnershipFlows -count=1 -v
go test ./tests/integration -run TestPlatformMigrationsAndDurableWork -count=1 -v
Remove-Item Env:TEST_DATABASE_URL
```

The first test covers project/application/invitation flows, private-role and cross-campus denial, duplicate applications, last-owner rules, and concurrent last-slot acceptance. The second checks blank-to-head and repeat migrations. A `SKIP` result means the test did not run; check `TEST_DATABASE_URL` and Docker.

## 9. Stop, inspect, and reset

Press `Ctrl+C` in the host API, web, and worker terminals. Then stop the containers while keeping database, object, and Caddy volumes:

```powershell
docker compose -f deploy/compose/local.yml down
```

To inspect a container, use `docker compose -f deploy/compose/local.yml logs --tail=100 postgres` and replace `postgres` with `objects`, `mailpit`, or `caddy`. Mailpit's UI is at `http://127.0.0.1:8025` when that container is running.

Only when you deliberately want to erase all local fixture data, run:

```powershell
docker compose -f deploy/compose/local.yml down --volumes
```

That deletes the local PostgreSQL, object, and Caddy volumes. Start the services again, rerun migrations, and recreate the local campus. Do not use this Compose project for staging or production data.
