# Database migrations

Migration filenames use a unique four-digit order and lowercase description. Once applied, a SQL file is immutable: the runner records its SHA-256 checksum and rejects edits. Add a new numbered migration for every schema change.

Run `go run ./cmd/migrate` explicitly after the database is available and before deploying application code. The API and worker never migrate on startup. Each migration and its history row commit in one transaction under a PostgreSQL advisory lock.

Migrations are forward-only by default. Before release, document the application rollback or forward repair alongside any migration that changes stored data. Do not run a reverse migration automatically; it can destroy data written by the new application version.
