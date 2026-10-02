# Phase 1 operator signals

The local platform exposes `/readyz` and these low-cardinality counters from `/metrics`:

- `peergit_http_errors_total`: increase over time indicates API requests returning 5xx responses.
- `peergit_http_requests_total`: request volume for the API process.

During local development, inspect these directly and check the structured API/worker logs. For staging, alert when readiness fails for more than one minute, when any job reaches `dead`, when the oldest ready job or outbox delivery is more than five minutes old, or when an expired lease remains running after one worker poll interval.

Useful operator queries from the repository root:

```powershell
docker compose -f deploy/compose/local.yml exec postgres psql -U peergit -d peergit -c "SELECT state, count(*) FROM jobs GROUP BY state ORDER BY state"
docker compose -f deploy/compose/local.yml exec postgres psql -U peergit -d peergit -c "SELECT count(*) AS overdue_ready FROM jobs WHERE state='ready' AND available_at < now() - interval '5 minutes'"
docker compose -f deploy/compose/local.yml exec postgres psql -U peergit -d peergit -c "SELECT count(*) AS pending_deliveries FROM outbox_deliveries WHERE state IN ('ready','running') AND available_at < now() - interval '5 minutes'"
docker compose -f deploy/compose/local.yml exec postgres psql -U peergit -d peergit -c "SELECT count(*) AS expired_job_leases FROM jobs WHERE state='running' AND lease_until < now()"
```

Use `go run ./cmd/jobctl list-dead` / `requeue-dead <job-id>` for job repair and `list-outbox-dead` / `requeue-outbox-dead <event-id> <handler>` for event delivery repair. Review the recorded safe error code and handler behavior before requeueing.

No alerting cluster is required locally. Configure external monitoring and test its delivery before campus release; keep thresholds adjustable against measured staging load.
