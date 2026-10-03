# Local supporting services and production counterparts

PeerGit currently runs its Go API, Go worker, and Next.js frontend on the development host. [The local run guide](peergit-run.md) gives the commands. [Compose](../deploy/compose/local.yml) starts four support containers, each bound to `127.0.0.1` so they are not exposed on the network.

| Docker service | Local endpoint and data | What it does now | Production counterpart |
|---|---|---|---|
| `postgres` — PostgreSQL 18 | `127.0.0.1:5432`; `postgres-data` volume | Required source of truth for users, sessions, campuses, projects, roles, applications, invitations, audit, jobs, and outbox. | Qualified PostgreSQL 18 deployment with protected credentials, tested backups/restore, monitoring, and later replication or managed hosting when capacity requires it. |
| `objects` — SeaweedFS `mini` S3 API | `127.0.0.1:8333`; `object-data` volume | Private S3-compatible local object storage for profile images and project media. Phase 3 projects work without media, but image upload needs this service. | Approved private S3-compatible object provider or qualified self-hosted object storage, with chosen residency, retention/deletion policy, independent backups, and restore tests. Cloudflare R2 is a possible later choice, not a current dependency. |
| `mailpit` — Mailpit | SMTP `127.0.0.1:1025`; inbox UI `127.0.0.1:8025` | Safe sink for development email when an email sender is added. The current Phase 3 code returns invitation links for manual sharing and does not send them through Mailpit. | Approved transactional email provider and a small sending adapter, with delivery failures retried from durable work. No local inbox or development credentials in production. |
| `caddy` — Caddy 2 | `http://localhost` redirects to `https://localhost`; `caddy-data` and `caddy-config` volumes | Supplies local HTTPS and same-origin routing: `/api/*`, `/healthz`, `/readyz`, `/metrics` go to the host API on port 8080; other requests go to Next.js on port 3000. | Caddy with a real domain/certificate on a qualified host, or an approved TLS reverse proxy/load balancer with equivalent routing and trusted client-address handling. |

## Why these boundaries exist

PostgreSQL is the only authoritative transactional database at launch. Jobs, outbox events, audit entries, sessions, search documents, and application decisions stay there until a measured need justifies another service. Redis, NATS, OpenSearch, ClickHouse, and Forgejo are not required to run Phase 3.

The Go API and Next.js app run on the host for quick development and debugging. The worker is another host process that polls PostgreSQL. Its Phase 3 event handlers are not registered yet, so a running worker is not a prerequisite for project creation or recruitment screens. Production can package API, worker, frontend, and Caddy as separate immutable containers while keeping the same application contracts.

Google OpenID Connect is an **external sign-in provider**, not a Docker service. Full browser testing needs a Google Web application client and local campus-domain fixture. The GitHub App belongs to the later repository-evidence phase and is not needed to create, publish, recruit for, or join a project.

## Operational differences to preserve

- Local Compose credentials and `sslmode=disable` work only because PostgreSQL is bound to the developer machine. Production requires separately managed secrets and an approved encrypted connection.
- The local SeaweedFS bucket is for development media. Production object retention, access control, residency, and independent copies must be qualified against the actual provider before launch.
- Mailpit deliberately keeps test mail local. Its presence does not mean PeerGit currently sends invitations or notifications.
- Caddy's `tls internal` certificate is a development CA. Production must use a trusted certificate and an approved public domain.
- `docker compose down` preserves named volumes; `down --volumes` erases the local PostgreSQL, object, and Caddy data. Do not copy production data into this stack.

Production qualification and recovery gates remain in [the implementation roadmap](implementation-phases.md) under P0, P1, and Phase 9.
