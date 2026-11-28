# Local development

The API, worker and Next.js app run on the host. Compose provides PostgreSQL, SeaweedFS, Mailpit and Caddy. Use the `pbl` profile for the optional private Forgejo service. [Local run guide](../plans/peergit-run.md) · [Service guide](../plans/local-services.md).

## Start

In PowerShell at the repository root:

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
docker compose -f deploy/compose/local.yml up -d --wait
go run ./cmd/migrate
```

The Go commands load `.env` from the repository root. For a Phase 6 migration checksum error, use the [disposable database reset](../plans/peergit-run.md#9-stop-inspect-and-reset). For optional Forgejo setup, follow [PBL Git development](../plans/peergit-run.md#pbl-git-development) before starting the Go processes. In separate host terminals:

```powershell
go run ./cmd/api
```

```powershell
go run ./cmd/worker
```

The worker processes verification email, current GitHub activity sync, PBL checkpoint snapshots and configured PBL Git work. Ordinary GitHub projects do not create snapshot jobs or archive objects. Inspect dead jobs with `go run ./cmd/jobctl`.

```powershell
npm --prefix apps/web ci
npm --prefix apps/web run dev
```

Use `https://localhost` through Caddy for browser sign-in and the PeerGit Git remote. Follow the run guide for local CA trust. The API exposes `/healthz`, `/readyz` and `/metrics` through the same origin.

Use the [test guide](../plans/peergit-test.md) for the manual walkthrough and the shared runner. The runner starts its own API, worker, web server and Docker services, so stop the interactive stack first.

## Stop and reset

Stop containers without deleting local database, object, or Caddy data:

```powershell
docker compose -f deploy/compose/local.yml --profile tools --profile pbl down
```

The [run guide](../plans/peergit-run.md#9-stop-inspect-and-reset) documents exactly which volumes a disposable reset removes. PBL recovery pairs database, Forgejo, and PBL checkpoint snapshot data; ordinary GitHub activity is fetched again from its upstream after restore.

## Services

| Service | Local address | Purpose |
|---|---|---|
| PostgreSQL | `127.0.0.1:5432` | Application state and durable jobs/outbox |
| SeaweedFS S3 API | `127.0.0.1:8333` | Local object storage |
| Mailpit SMTP/UI | `127.0.0.1:1025` / `127.0.0.1:8025` | Local sink for campus verification email |
| Caddy | `https://localhost` | Same-origin local HTTPS proxy |
| Forgejo (optional `pbl` profile) | `127.0.0.1:3031` | Private administrative endpoint for the PBL Git service |

Compose files bind host ports to loopback. The PostgreSQL Compose credentials are development-only.
