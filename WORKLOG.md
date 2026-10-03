# PEERGIT Worklog

This is a concise engineering record of meaningful repository changes made by coding agents. It complements Git history: commits show the exact diff; this file records intent, validation, and remaining risk.

## Entry format

Add new entries in `reverse order means new entry at the top of previous entry` i.e. immediately below `## Entries` so the latest entry is first. And also don't delete previous entries title.

```markdown
### YYYY-MM-DDTHH:MMZ — Short outcome

- **Phase/area:** Phase N / component
- **Summary:** What changed and why, in one or two sentences.
- **Files/components:** `path`, package, service, migration, or `None`.
- **Validation:** Commands/checks run and their results. For anything not run, state why and the remaining risk.
- **Follow-up:** Next action, known limitation, or `None`.
- **References:** Issue, PR, commit, ADR, or `None`.
```

Keep entries brief. Combine tightly related work completed together; create separate entries for independent changes.

## Entries

### 2026-10-03T13:17Z — Repair Phase 3 migration collision in CI

- **Phase/area:** Phase 3 / PostgreSQL migration and shared skills.
- **Summary:** Removed the duplicate `CREATE TABLE skills` from migration 0005 so project skills reference the table created in 0004. Added an integration assertion that a project reuses a skill already assigned to a user, and clarified failed-migration retry instructions. The failed 0005 transaction leaves no migration history row, so the corrected migration can be retried without clearing local data.
- **Files/components:** `migrations/0005_projects_recruitment.sql`, `migrations/README.md`, `tests/integration/phase3_test.go`, `plans/peergit-run.md`.
- **Validation:** `go test ./... -count=1`, `go vet ./...`, `staticcheck ./...`, Compose configuration, `git diff --check`, and a scan of migration table/index declarations passed. The focused Phase 3 test compiled but skipped because `TEST_DATABASE_URL` is unset; `docker info` cannot reach the Docker Desktop Linux engine here, so the corrected SQL has not yet executed against PostgreSQL in this environment.
- **Follow-up:** Rerun `go run ./cmd/migrate` and the focused PostgreSQL integration tests on the local stack, then rerun CI. Do not reset the database volume for this transactionally rolled-back migration.
- **References:** Reported CI failure at `0005_projects_recruitment.sql`; `plans/implementation-phases.md` Phase 3.

### 2026-10-03T13:06Z — Document local Phase 3 startup and services

- **Phase/area:** Phase 3 / local development handoff.
- **Summary:** Checked the four implemented Phase 3 roadmap items while leaving its live completion gate open. Added Windows PowerShell instructions for the host API/web/worker, individually started Docker services, local Google/campus setup, browser walkthrough, integration checks, and shutdown. Documented each container's current purpose and production counterpart; clarified that Phase 3 invitation email is not yet sent. Updated migration history coverage to derive its expected count from the embedded SQL files.
- **Files/components:** `plans/implementation-phases.md`, `plans/peergit-run.md`, `plans/local-services.md`, `.gitignore`, `.env.example`, `ops/local-development.md`, `tests/integration/proofs_test.go`.
- **Validation:** `go test ./... -count=1`, `go vet ./...`, `staticcheck ./...`, `docker compose -f deploy/compose/local.yml config --quiet`, and `git diff --check` passed. All 15 PowerShell examples parsed and local guide links resolved. Both focused PostgreSQL tests compiled but reported `SKIP` because `TEST_DATABASE_URL` is unset; `docker info` could not connect to the Docker Desktop Linux engine, so migrations, concurrency, and interactive browser journeys were not run live. Graphify code-only refresh and clustering completed (616 nodes, 1,761 edges, 34 communities); SQL extraction and Graphify MCP were unavailable.
- **Follow-up:** Start Docker Desktop, follow `plans/peergit-run.md`, run the two focused PostgreSQL tests and the Phase 3 browser walkthrough, then sign off the completion gate only if they pass. Production email, storage, hosting, and recovery remain later gates.
- **References:** `plans/implementation-phases.md` Phase 3; `plans/plan-new.md` §§4.2, 8, 13.

### 2026-10-03T12:44Z — Implement Phase 3 projects and recruitment

- **Phase/area:** Phase 3 / projects, teams, and recruitment.
- **Summary:** Added the tenant-safe projects/team/recruitment schema and usable project discovery, lifecycle, membership, invitation, role, and application flows. Split role and application APIs into the planned `internal/recruitment` module; locked project mutations against concurrent manager revocation, protected private-project roles, and added optimistic versions, audit/outbox writes, and capacity/last-owner safeguards.
- **Files/components:** `migrations/0005_projects_recruitment.sql`, `internal/project`, `internal/recruitment`, `cmd/api`, `tests/integration/phase3_test.go`, `apps/web/app/projects.tsx`, `api/openapi.yaml`, migration docs.
- **Validation:** `go test ./... -count=1`, `go vet ./...`, `staticcheck ./...`, frontend typecheck/build, `docker compose -f deploy/compose/local.yml config --quiet`, and `git diff --check` passed. The Phase 3 database integration test compiled but skipped because `TEST_DATABASE_URL` is unset; Docker reports no running Desktop Linux engine, so migration/concurrency behavior was not exercised against PostgreSQL in this turn. Graphify CLI code-only refresh and clustering completed (616 nodes, 1,760 edges, 35 communities); SQL graph extraction and Graphify MCP were unavailable.
- **Follow-up:** Start the local Docker stack and run `TEST_DATABASE_URL=... go test ./tests/integration -run TestPhase3ProjectRecruitmentAndOwnershipFlows -count=1 -v`; keep the Phase 3 completion gate open until it passes. Then run the broader integration suite and update the Phase 3 checkboxes based on evidence.
- **References:** `plans/implementation-phases.md` Phase 3; `plans/plan-new.md` §§3.1–3.3, 3.9, 4.2, 4.5–4.6, 8.

### 2026-10-02T14:47Z — Implement Phase 2 identity, campus, and media foundation

- **Phase/area:** Phase 2 / identity, campus authorization, and media.
- **Summary:** Added Google OIDC with state/nonce/S256 PKCE and verified-email checks; opaque hashed sessions with CSRF/origin enforcement and immediate suspension/domain-revocation checks; campus invitations, organizations, profiles, skills, consent, MFA, audited administration, and private image uploads with raster re-encoding and hash-verified downloads. Added host-configured local/staging/production identity settings and responsive account/policy screens. Fixed live-test defects in invitation expiry binding and nullable MFA freshness; normalized client ports for rate limiting and pruned expired buckets.
- **Files/components:** `internal/identity`, `internal/campus`, `internal/media`, `migrations/0004_identity_campus_media.sql`, `cmd/api`, `apps/web`, `api/openapi.yaml`, `ops/identity-and-media.md`, `tests/integration`, config and migration docs.
- **Validation:** With Docker PostgreSQL 18 and local object storage running, `go test ./... -count=1` passed with database integration enabled, including blank-to-head/repeat migrations, session/consent/profile, tenant constraints, audited campus-role grant, unverified-MFA admin denial, private-media denial, and a local fake OIDC provider checking state, nonce, verified email, S256 PKCE, invited external acceptance/scoped access, session issue, and replay. `go vet ./...`, `staticcheck ./...`, `git diff --check`, and `docker compose -f deploy/compose/local.yml config --quiet` passed. Next.js typecheck/build and `npm --prefix tests/e2e test` passed at desktop and mobile sizes. Graphify code-only and clustering refreshed the map to 531 nodes/1,408 edges/26 communities; its SQL parser and Graphify MCP were unavailable, so migration relationships are not represented.
- **Follow-up:** Google production credentials and legally approved terms/privacy versions remain release setup. Campus-admin promotion is still bootstrap/operator-only: automatic approval review rejected enabling a one-person campus-admin grant through the role endpoint because it could enable privilege escalation; use a safer dual-control workflow. Login rate limits normalize source ports, but a reverse-proxy deployment still needs a trusted-client-address policy to avoid treating all proxy traffic as one address. The local Compose stack remains running for development.
- **References:** `plans/implementation-phases.md` Phase 2; `plans/plan-new.md` §§4.1, 8, 11–14.

### 2026-10-02T06:20Z — Complete local Phase 1 platform foundation

- **Phase/area:** Phase 1 / local platform foundation.
- **Summary:** Added the explicit migration runner, PostgreSQL pool/readiness, HTTP idempotency response replay and expiry cleanup, independent job deduplication, fenced job/outbox leases and repair commands, append-only audit, cursors/version helpers, metrics, and the host-run worker/API/web stack. Added the responsive accessible app shell, OpenAPI contract, local operations/backup/alerts guides, and CI/browser smoke workflow. Kept the existing logger, error manager, and request/response helpers.
- **Files/components:** `cmd/api`, `cmd/worker`, `cmd/migrate`, `cmd/jobctl`, `internal/platform`, `internal/health`, `migrations/0001_platform.sql`–`0003_optional_job_request_link.sql`, `apps/web`, `api/openapi.yaml`, `deploy/compose/local.yml`, `deploy/caddy`, `ops`, `.github/workflows/ci.yml`, `tests/integration`, `tests/e2e`, `.env.example`, `.gitignore`, `plans/implementation-phases.md` (local ignored roadmap only).
- **Validation:** `docker compose -f deploy/compose/local.yml up -d --wait` and config checks passed; PostgreSQL 18, SeaweedFS, Mailpit and Caddy were healthy. The current migrator applied all three migrations to a fresh schema and repeated successfully; the existing local database upgraded additively. `go test ./... -count=1`, `go vet ./...`, and `staticcheck ./...` passed with PostgreSQL integration enabled, including an abruptly exited child worker after a durable side effect, lease reclaim/stale fencing, HTTP replay/expiry, concurrent claims, outbox retry/repair, and append-only audit. Next.js typecheck/build passed, `npm audit` reported zero vulnerabilities, and Playwright passed desktop/mobile keyboard/search/request-ID/unavailable-state checks. Live API/Caddy readiness returned 200 with request IDs; Caddy and backup-script syntax, OpenAPI/CI YAML parsing, and `git diff --check` passed. Graphify code-only refresh and clustering completed (390 nodes/914 edges/19 communities); its SQL parser is unavailable, so it omitted the three migration files from the graph. Graphify MCP is not exposed in this environment.
- **Follow-up:** Hosted CI has not run yet. Domain workflows remain in their planned Phases 2–8; production hosting/provider qualification and timed independent backup/restore remain later gates. The two local migration smoke schemas are preserved; no data volumes were removed.
- **References:** `plans/implementation-phases.md` Phase 1; `plans/plan-new.md` repository layout and local environment contract.

### 2026-10-02T04:54Z — Complete local Phase 0 and GitHub App revocation proofs

- **Phase/area:** Phase 0 risk proofs and local storage decision.
- **Summary:** Switched the aggregate verifier and environment example to local S3-compatible storage by default; moved actual production-provider retention/deletion testing to P1. GitHub selected/excluded access, least-privilege read permissions, exact-SHA capture after branch movement, repository removal, suspension, uninstall and reinstallation all passed. Restricted `.env.verify` and PEM ACLs after detecting inherited broad read access. Ashok accepted the Phase 0 scope/risks; automated browser checks covered the walkthrough.
- **Files/components:** `.env.verify.example`, ignored local `cmd/verify`, `docs/verification`, `plans/phase0-requirements.md`, `plans/implementation-phases.md`, `.env.verify` ACL and PEM ACL. No source-plan or API behavior changes.
- **Validation:** Docker Compose fixtures healthy; `VERIFY_INTEGRATION=true go test ./tests/integration -count=1 -v` passed check/queue/capture/ten simultaneous 100 MiB captures. Focused GitHub/verifier tests, `go test ./... -count=1`, `go vet ./...`, `staticcheck ./...`, `npm --prefix tests/e2e test`, and `git diff --check` passed. `go run ./cmd/verify all --target local` passed with zero missing gates; `production_ready=false` by design. GitHub proof receipts record each passing operator checkpoint and the read-only installation permissions; the first baseline had a transient unclassified failure and passed on rerun. Protected ACL checks showed no broad read ACEs. Graphify code-only refresh and clustering passed (240 nodes/542 edges/12 communities); ignored verifier files are outside its current map, and no Graphify MCP tool is exposed.
- **Follow-up:** P0/P1 must select and test the eventual production object service, retention/deletion and independent recovery. `cmd/verify/`, `docs/verification/` and `plans/` are ignored locally and absent from a fresh clone under current Git rules.
- **References:** `plans/implementation-phases.md` Phase 0/P1; `docs/verification/results.md`; current decision to use local storage for development.

### 2026-09-28T18:26Z — Ignore local verification command and documentation

- **Phase/area:** Git configuration / local risk proofs.
- **Summary:** Added root-scoped ignore rules for `cmd/verify/` and `docs/verification/` at the user's request. Files remain available locally; neither directory was tracked.
- **Files/components:** `.gitignore`, `WORKLOG.md`.
- **Validation:** `git check-ignore -v` confirmed both directories' files are ignored; `git ls-files cmd/verify docs/verification` returned no tracked files; `git diff --check` passed. Go tests not rerun: ignore rules do not change runtime behavior. Existing Graphify map inspected; Graphify MCP remains unavailable.
- **Follow-up:** Fresh clones will omit the verifier and its documentation; tracked integration checks referring to the verifier will require those local files or a later test replacement.
- **References:** User request to ignore verification files.

### 2026-09-28T18:00Z — Implement local risk proofs and external setup guide

- **Phase/area:** Phase 0 contract/risk proofs; semantic project structure from plan-new.md.
- **Summary:** Added the requirements guide first, then host verification commands, concrete GitHub/archive and S3 adapters, PostgreSQL jobs with leases/fencing, disposable Docker fixtures, persona/risk/budget contracts and an accessible browser-tested walkthrough. Local gates pass; real GitHub/R2 experiments and owner review remain pending without credentials. The combined verifier exits nonzero for these pending gates and never certifies production capacity.
- **Files/components:** `plans/phase0-requirements.md`, `.env.verify.example`, `.gitignore`, `cmd/verify`, `internal/github`, `internal/repository`, `internal/platform/jobs`, `internal/platform/storage`, `deploy/compose/proofs.yml`, `tests/integration`, `tests/e2e`, `docs/verification`, Go dependencies; only evidenced roadmap checkboxes updated. Three source plans unchanged.
- **Validation:** `gofmt` and focused Go tests passed; `go test ./... -count=1`, `go vet ./...`, `staticcheck ./...` passed. `VERIFY_INTEGRATION=true go test ./tests/integration -count=1 -v` passed all four cases (including blank/repeat proof schema, 20 concurrent enqueues/claims, lease/attempt guards and killed child workers). `docker compose -f deploy/compose/proofs.yml config --quiet` / `up -d --wait` passed. Ten parallel 100 MiB stored-byte-verified captures passed in roughly 16–19 seconds; initial 768 MiB S3 fixture OOM (exit 137) was resolved by a 2 GiB fixture limit with the workload unchanged. Staticcheck's initial ST1005 finding was corrected. `npm --prefix tests/e2e ci --ignore-scripts --no-audit --no-fund` and `npm --prefix tests/e2e test` passed; 1280×900 and real 390×844 viewports, keyboard/forms/receipt states verified and mobile screenshot inspected. Browser MCP kernel could not start due to its Windows ACL helper; isolated Playwright/Edge validation succeeded instead. `go run ./cmd/verify all` returned expected exit 1/pending for missing live gates and owner review. `git diff --check`, documentation link checks and absence of phase-named project directories passed; `gofmt -l` found only the pre-existing unchanged `internal/health/response.go` formatting, which was preserved outside this change; new/changed Go files are formatted; ignore boundaries verified. Graphify code-only/cluster refresh succeeded; SQL parser is missing, so SQL graph relationships remain incomplete (actual SQL integration tests passed); no Graphify MCP tool is exposed.
- **Follow-up:** Ashok prepares `.env.verify`, local PEM, private GitHub fixtures and scoped R2 bucket credentials via the guide; live revocation/lock/expiry/lifecycle observations and contract acceptance then close remaining gates. Production residency, campus support, hosting funding and independent recovery remain later gates. Supporting containers are running locally; documented `down` / deliberate fixture reset commands provided. Phase 1 still owns complete production migrations/outbox/authorization/operator repair.
- **References:** `plans/implementation-phases.md` Phase 0; `plans/plan-new.md` §§3, 7, 9, 13–16, 19–21; `docs/verification/results.md`.

### 2026-09-28T13:56Z — Give router middleware shared dependency ownership

- **Phase/area:** Phase 1 foundation / HTTP routing.
- **Summary:** Added a small Router struct holding the dedicated logger and error manager. Middleware now uses receiver methods; fallback handlers use the same owner. Kept NewRouter's signature and runtime behavior unchanged, and updated existing regression tests to exercise the methods.
- **Files/components:** `internal/platform/http/router.go`, `internal/platform/http/router_test.go`, `WORKLOG.md`.
- **Validation:** `gofmt -w internal/platform/http/router.go internal/platform/http/router_test.go`, `go test ./internal/platform/http -count=1`, `go test ./... -count=1`, `go vet ./...`, `staticcheck ./...`, and `git diff --check` passed. Graphify graph/query reviewed; no Graphify MCP capability is exposed in this session.
- **Follow-up:** Run `graphify . --code-only` and `graphify cluster-only D:\projects\peergit` to update the method relationships and report. No new dependencies or service abstractions.
- **References:** User-approved router ownership refactor.

### 2026-09-28T13:51Z — Fix API foundation audit findings

- **Phase/area:** Phase 1 foundation / shared HTTP and API startup.
- **Summary:** Fixed all four audit findings: preserve abort panics and terminate incomplete streams, reject null DTOs before validation, route 404/405 through the common error manager/envelope, and install the dedicated logger before startup can fail. Added regression coverage and verified graceful shutdown drains an active request.
- **Files/components:** `cmd/api/main.go`, `cmd/api/main_test.go`, `internal/platform/http/router.go`, `internal/platform/http/router_test.go`, `internal/platform/http/request/request.go`, `internal/platform/http/request/request_test.go`.
- **Validation:** `gofmt -w` on the six changed Go files passed; `go test ./internal/platform/http/... ./cmd/api -count=1`, `go test ./... -count=1`, `go vet ./...`, `staticcheck ./...` (2026.2.1 / 0.8.1), and `git diff --check` passed. Network regression confirms unexpected EOF after a streamed-response panic; startup regression confirms production JSON error logging. No dependencies added.
- **Follow-up:** Refresh the existing stale graph with `graphify . --code-only` and `graphify cluster-only D:\projects\peergit`. Graphify MCP remains unavailable in this session; existing Graphify analysis informed the audit. No Phase 0 or source-plan changes.
- **References:** Current Git-change audit; `plans/implementation-phases.md` Phase 1.

### 2026-09-28T13:34Z — Harden shared HTTP foundation and add error manager

- **Phase/area:** Phase 1 foundation / API platform.
- **Summary:** Kept the dedicated slog logger and shared request/response functions; added bounded JSON decoding, validated config/log levels, a typed error manager with safe envelopes, request IDs/access logs/panic recovery, and server timeouts/graceful shutdown.
- **Files/components:** `cmd/api`, `internal/health`, `internal/platform/config`, `internal/platform/errormanager`, `internal/platform/http`, `internal/platform/logging`, `go.mod`, focused tests.
- **Validation:** `gofmt`, affected package tests, `go test ./... -count=1` and `go vet ./...` passed. `staticcheck ./...` unavailable (`staticcheck` is not installed; static-analysis findings remain unverified). `go mod tidy -diff` showed only the repository's CRLF checkout of `go.sum` versus the tool's LF output; no dependency delta. No database/Phase 0 work.
- **Follow-up:** Run `graphify . --code-only` and `graphify cluster-only D:\projects\peergit` to refresh the structural map/report; add domain-specific error codes with their endpoints.
- **References:** `plans/implementation-phases.md` Phase 1.

### 2026-09-28T12:42Z — Rewrite launch and future implementation roadmap

- **Phase/area:** Documentation / launch and future delivery planning.
- **Summary:** Audited newer plans against the original and replaced the roadmap with dependency-based launch phases, separate production tracks, and gated Stages A–D. Kept college Git/publication in Stage B independently of scale services; retained all minimum launch personas and strengthened durability, authorization, recovery, and accessible UI gates.
- **Files/components:** `plans/implementation-phases.md`, `WORKLOG.md`; source plans and application/configuration files unchanged.
- **Validation:** PowerShell assertions passed for 22 phase blocks with all nine required fields, 62 original catalog rows, 12 persona rows, local source links, exact workload/recovery markers, acyclic diagram, and unchanged SHA256 of all three source plans. Manually compared release checks against plan-new.md §§15–16 and dependency/milestone consistency. `git diff --check` passed for tracked files; `plans/` is ignored by existing `.gitignore`, so direct file validation is required. No Go/integration/infrastructure tests run: documentation-only change, no runtime behavior changed. Graphify CLI analysis was used; Graphify MCP was unavailable in the exposed tool inventory.
- **Follow-up:** Phase 0 proofs and later phase gates require implementation evidence; no existing work was marked complete. Roadmap remains ignored under existing repository policy; no ignore rules changed.
- **References:** `plans/plan-new.md`, `plans/plan-future-scale.md`, `plans/plan.md`; approved user roadmap rewrite.
