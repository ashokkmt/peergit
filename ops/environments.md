# Environment boundaries

| Environment | Runtime and data |
|---|---|
| Local | Go API/worker and Next.js on the host; Compose PostgreSQL, SeaweedFS, Mailpit, and Caddy. Synthetic data only. |
| Staging | Production-shaped immutable images with separate database, object storage, secrets, and test provider accounts. |
| Production | Qualified host and approved providers, encrypted storage, independent backups, external monitoring, and a named operator. |

Keep separate environment files and secret stores. Never copy production data to local or staging. Production must not use local credentials, debug logging, development bind mounts, or captured email. Deploy the same built artifacts from staging to production without rebuilding.
