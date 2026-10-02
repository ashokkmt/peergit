# Database migrations

Migration filenames use a unique four-digit order and lowercase description. Once applied, a SQL file is immutable: the runner records its SHA-256 checksum and rejects edits. Add a new numbered migration for every schema change.

Run `go run ./cmd/migrate` explicitly after the database is available and before deploying application code. The API and worker never migrate on startup. Each migration and its history row commit in one transaction under a PostgreSQL advisory lock.

Migrations are forward-only by default. Before release, document the application rollback or forward repair alongside any migration that changes stored data. Do not run a reverse migration automatically; it can destroy data written by the new application version.

`0004_identity_campus_media.sql` adds campus tenants/domains, OIDC identities and single-use login state, hashed server sessions, campus verification/roles, email-bound invitations, organizations, profiles/skills, versioned consent, encrypted-MFA storage, database rate-limit buckets, and tenant-owned media metadata. The migration is additive and contains no production seed data. Roll application artifacts back without dropping these tables; forward repair is required if identity rows have been written.
