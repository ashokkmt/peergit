# Local development

The API, worker, and Next.js app run on the host. Docker Compose provides PostgreSQL 18, SeaweedFS (S3-compatible), Mailpit, and Caddy. No production data belongs in this environment. For the complete Windows setup and Phase 3 walkthrough, see [the local run guide](../plans/peergit-run.md) and [service guide](../plans/local-services.md).

## Start

In PowerShell at the repository root:

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
docker compose -f deploy/compose/local.yml up -d --wait
go run ./cmd/migrate
```

The Go commands load `.env` from the repository root. The migration command is explicit and repeatable. The API never runs migrations on startup. In separate host terminals:

```powershell
go run ./cmd/api
```

```powershell
go run ./cmd/worker
```

The worker currently starts both durable polling loops with no domain handlers registered; pending work stays durable until its owning feature adds a handler. Inspect or repair dead work with `go run ./cmd/jobctl`.

```powershell
npm --prefix apps/web ci
npm --prefix apps/web run dev
```

Open `https://localhost` through Caddy or `http://127.0.0.1:3000` for the direct web development server. Caddy uses its local internal CA; trust that CA only on this development machine if the browser does not already trust it. API checks are `https://localhost/healthz`, `/readyz`, and `/metrics`; direct API checks use port 8080.

Run the browser smoke checks after starting the web app. The browser test supplies local readiness responses, so it does not need a live API:

```powershell
npm --prefix tests/e2e ci
npm --prefix tests/e2e test
```

They launch an isolated browser profile and cover keyboard navigation, responsive overflow, labeled search, readiness/request IDs, and the API-unavailable state.

## Stop and reset

Stop containers without deleting local database, object, or Caddy data:

```powershell
docker compose -f deploy/compose/local.yml down
```

Deliberately erase local fixture data only when you want a blank environment:

```powershell
docker compose -f deploy/compose/local.yml down --volumes
```

That removes the local PostgreSQL and object-storage volumes. Restart with `up -d --wait`, then run `go run ./cmd/migrate` again. Never point this Compose stack at staging or production credentials or copy production data into it.

## Services

| Service | Local address | Purpose |
|---|---|---|
| PostgreSQL | `127.0.0.1:5432` | Application state and durable jobs/outbox |
| SeaweedFS S3 API | `127.0.0.1:8333` | Local object storage |
| Mailpit SMTP/UI | `127.0.0.1:1025` / `127.0.0.1:8025` | Local email sink; current Phase 3 invitation links are shared manually because no sender is registered yet |
| Caddy | `https://localhost` | Same-origin local HTTPS proxy |

Compose files bind host ports to loopback. The PostgreSQL Compose credentials are development-only.
