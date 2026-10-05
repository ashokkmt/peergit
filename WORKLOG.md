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

### 2026-10-05T12:13Z — Fix GitHub imports without descriptions

- **Phase/area:** Phase 4 repository imports
- **Summary:** GitHub repositories with blank descriptions produced an empty project summary, violating the database's 1–500 character constraint. Imports now use a repository-name fallback, with regression coverage.
- **Files/components:** `internal/repository/imports.go`, `internal/repository/imports_test.go`, `plans/peergit-test.md`, `WORKLOG.md`.
- **Validation:** `gofmt`, focused repository tests, `go test ./... -count=1`, all integration tests with `TEST_DATABASE_URL` against local PostgreSQL, `go vet ./...`, and `staticcheck ./...` passed. `node scripts/test.mjs` was not run because the user's API and frontend were already listening on its required ports (8080 and 3000); stopping them would interrupt the active local session.
- **Follow-up:** Restart the API and retry the import through a fresh authorization flow. The failed transaction rolled back; its one-use OAuth state cannot be reused.
- **References:** API request `c1b711e0689142c86cb41603a03cdbfa`.

### 2026-10-05T11:41Z — Reorganize local testing guides

- **Phase/area:** Local operations documentation
- **Summary:** Moved Repository App redirect/credential instructions into the setup and `.env` sections, placed organization import steps with repository import, and integrated pgAdmin into the Docker startup instructions. Removed appended duplicate sections and patch markers.
- **Files/components:** `plans/live-github-testing.md`, `plans/peergit-run.md`, `WORKLOG.md`.
- **Validation:** `git diff --check` passed. Reviewed section order and verified the plan files remain ignored. No application tests were needed for this documentation-only change.
- **Follow-up:** Run the Compose command locally when convenient.
- **References:** `plans/live-github-testing.md`, `plans/peergit-run.md`.

### 2026-10-05T11:32:22Z — Document pgAdmin in local startup commands
- Added the Compose `tools` profile command to start pgAdmin alongside the local services in the local run and live GitHub testing guides.
- Documented the local PostgreSQL connection values and persistence behavior of the `pgadmin-data` volume.
- Validation: documentation-only; Compose command behavior follows the existing optional `tools` profile. Shell-based checks remain unavailable because the workspace command runner fails to start.
- Follow-up: run the documented command locally if Compose configuration changes.

### 2026-10-05T11:27:52Z — Clarify Repository App setup guide
- Clarified the distinct GitHub App Setup URL, user-authorization Redirect URI, and Webhook URL fields in the local live-testing guide.
- Distinguished the PEM private key, App ID, Client ID, and OAuth client secret, with exact environment-variable mappings and dashboard steps.
- Validation: documentation review against GitHub's official setup and callback documentation; shell-based checks remain unavailable because the workspace command runner fails to start.
- Follow-up: verify the local OAuth flow after configuring Repository App credentials.

### 2026-10-05T11:18:18Z — Update live GitHub testing guide
- Added Repository App user-authorization environment variables and the exact callback URL to the live-testing instructions.
- Documented the optional organization-owned repository installation, authorization, admin-permission, and import flow; the personal two-repository smoke test remains sufficient.
- Validation: documentation-only update; shell-based verification was unavailable in this turn because the workspace command runner failed to start.
- Follow-up: run the local live-testing steps when the Repository App credentials are configured.

### 2026-10-05T11:04Z — Organize web components and add local PostgreSQL GUI

- **Phase/area:** Frontend structure and local development services.
- **Summary:** Moved UI components out of the Next.js route tree into `components/layout`, `components/account`, `components/github`, and `components/projects`; removed the unused project-first repository panel. Added optional, loopback-only pgAdmin 4 to the Compose `tools` profile with a separate data volume and documented login/connection steps and local-only usage.
- **Files/components:** `apps/web/app`, `apps/web/components`, `.env.example`, `deploy/compose/local.yml`, ignored `plans/peergit-run.md`, ignored `plans/local-services.md`, `WORKLOG.md`.
- **Validation:** `node scripts/test.mjs` passed all Go, database, vet, staticcheck, frontend, E2E, Compose, and diff checks. Default and tools-profile Compose configs validated. Started pgAdmin, confirmed its local login endpoint returned HTTP 200, and checked container logs after correcting its sample login address.
- **Follow-up:** pgAdmin is intentionally excluded from production and uses the local database owner; do not connect it to production. Graphify should be refreshed after this component move.
- **References:** Official [pgAdmin container deployment docs](https://www.pgadmin.org/docs/pgadmin4/latest/container_deployment.html); pinned image `dpage/pgadmin4:9.18.0`.

### 2026-10-05T10:43Z — Import-first GitHub project workflow

- **Phase/area:** Signup sessions, GitHub repository imports, project discovery, and project collaboration.
- **Summary:** Replaced project-first repository setup with account-scoped GitHub installation/authorization and private imports, preserved verified campus membership across login, and moved account navigation/profile/settings to dedicated routes. Discovery now excludes repository-free drafts; imported project pages support metadata edits, explicit publication, team openings, and applications. Removed obsolete project-first handlers and corrected the campus-restoration SQL.
- **Files/components:** `.env.example`, `cmd/api`, `internal/github`, `internal/identity`, `internal/platform/config`, `internal/project`, `internal/recruitment`, `internal/repository`, `migrations/0008_github_project_imports.sql`, `apps/web/app`, `tests/integration`, `tests/e2e/walkthrough.cjs`, `scripts/test.mjs`, ignored `plans/*` guides.
- **Validation:** `node scripts/test.mjs` passed end to end: Go formatting/tests with PostgreSQL integration (including migration blank-to-head/repeat), `go vet`, `staticcheck`, frontend typecheck/build, desktop/mobile browser journeys, API/worker/Mailpit onboarding, project edit/publish/opening flow, Compose validation, and `git diff --check`.
- **Follow-up:** Real GitHub personal/organization import and revocation smoke checks still require operator-owned App credentials/accounts. Production storage/recovery gates remain separate. Do not claim all Phase 4 provider-backed acceptance complete.
- **References:** Local import-first product decision and updated ignored `plans/implementation-phases.md`.

### 2026-10-05T07:08Z — Document local live GitHub walkthrough

- **Phase/area:** Phase 2–4 manual live-provider testing.
- **Summary:** Added an ignored end-to-end setup guide for creator-owned signup/repository GitHub Apps, a separate student GitHub account with selected/excluded private fixtures, every application environment value, local Docker services, HTTPS signup/Mailpit campus verification, signed webhook relay, evidence sync, snapshots, and revoked downloads. Corrected adjacent OAuth/run instructions and documented the existing `/projects` installation redirect workaround.
- **Files/components:** Ignored `plans/live-github-testing.md`, `plans/setup-github-oauth.md`, `plans/peergit-run.md`, `plans/peergit-test.md`; tracked `WORKLOG.md`. No application or service configuration changed.
- **Validation:** Checked guide variables against every `.env.example` key and local documentation links; `docker compose -f deploy/compose/local.yml config --quiet`, `npm exec --yes --package=smee-client -- smee --help`, `git diff --check`, and ignore checks passed. Documentation-only checks followed the authoritative test guide; no code suite was required. No live account/App secrets were read or live provider tests performed.
- **Follow-up:** User creates the two development Apps and completes the manual live smoke checks. Known installation redirect targets a missing frontend `/projects` route; guide returns to `/` after confirmed linking. Full Phase 4 gates remain separate from manual smoke testing.
- **References:** `plans/live-github-testing.md`; official GitHub registration, visibility, setup URL, and webhook documentation linked there.

### 2026-10-04T19:10Z — Avoid GitHub Actions environment collision

- **Phase/area:** Test configuration and CI migrations.
- **Summary:** GitHub Actions sets its own `GITHUB_API_URL`, which PeerGit mistakenly read as a test endpoint and rejected during migration. Renamed PeerGit’s fake GitHub endpoint variables with a `PEERGIT_` prefix and added a regression test; production/test-only endpoint validation remains enforced.
- **Files/components:** `internal/platform/config/config.go`, `internal/platform/config/config_test.go`, `scripts/test.mjs`, ignored `plans/peergit-test.md`, `WORKLOG.md`.
- **Validation:** `go test ./internal/platform/config ./cmd/migrate -count=1`, `node scripts/test.mjs`, and `go run ./cmd/migrate` with `APP_ENV=development` and GitHub Actions-style `GITHUB_API_URL=https://api.github.com` passed; the full script included integration, staticcheck, frontend, browser, Compose, and diff checks.
- **Follow-up:** Push the fix and rerun GitHub Actions; local tests simulate the reserved variable but cannot replace the hosted-run result.
- **References:** User-provided CI log.

### 2026-10-04T18:44Z — Isolate OTP browser contract test

- **Phase/area:** Phase 2 campus verification browser tests.
- **Summary:** Investigated an intermittent Playwright timeout waiting for the mocked campus OTP success state. Reproduced the full script successfully; the isolated test had an authenticated session stub without a matching `/api/v1/me` stub, causing unrelated 401 log traffic. Stubbed that endpoint and added failure context for confirmation-request count and visible page state. The unrelated 401s were test leakage, but the available evidence does not prove they caused the timeout.
- **Files/components:** `tests/e2e/walkthrough.cjs`, ignored `plans/peergit-test.md`, `WORKLOG.md`.
- **Validation:** `node scripts/test.mjs` passed both before and after the focused test adjustment, including migrations, Go tests/integration, vet, staticcheck, web typecheck/build, Playwright (OTP and live Mailpit journeys), Compose validation, and diff check.
- **Follow-up:** If the timeout recurs, use the added request count/page details to determine whether the mocked POST ran and whether the UI rendered its response.
- **References:** User-provided Playwright run log.

### 2026-10-04T17:14Z — Implement Phase 4 GitHub evidence foundation

- **Phase/area:** Phase 4 GitHub repository evidence and immutable snapshots.
- **Summary:** Added tenant-scoped GitHub App installations and project repository bindings, HMAC-verified durable webhook intake, PostgreSQL-driven reconciliation/contribution capture, exact-SHA snapshot receipts with fenced worker capture and stored-byte verification, and the responsive project repository panel. Added additive migrations; academic/event submission foreign keys remain deferred to Phases 6–7.
- **Files/components:** `migrations/0006_github_evidence.sql`, `migrations/0007_snapshot_job_link.sql`, `internal/repository`, `internal/github`, `internal/platform/jobs`, `internal/platform/storage`, `cmd/api`, `cmd/worker`, `apps/web/app/repository-panel.tsx`, project styles, integration/unit/E2E tests. Locally updated ignored `plans/implementation-phases.md` and `plans/peergit-test.md` without force-tracking plan files.
- **Validation:** `node scripts/test.mjs` passed (migrations, Go tests including PostgreSQL integration, vet, staticcheck, Next.js typecheck/build, Playwright, Compose validation, and diff check). `TestLocalS3TenMaximumCaptures` passed against local SeaweedFS: ten concurrent 100 MiB uploads and stored-byte verifications completed in 14.755 seconds; temporary objects were removed. Graphify refreshed the code map.
- **Follow-up:** Full real-GitHub install/revocation and 100→130 replay acceptance remains unverified; LFS/submodule contents are not captured. The Phase 4 roadmap gate remains open until the full provider-backed failure/replay suite is recorded. No production capacity claim is made from the local storage measurement.
- **References:** `plans/implementation-phases.md` Phase 4; `plans/plan-new.md` §§8–9, 15–16.

### 2026-10-04T06:56Z — Implement GitHub signup and campus verification

- **Phase/area:** Phase 2 identity, campus verification, media and Phase 3 regression.
- **Summary:** Replaced Google login with GitHub authorization-code/S256 PKCE using immutable GitHub IDs and verified primary emails, introduced unverified accounts and explicit campus-email onboarding, encrypted asynchronous link/OTP delivery, audited administrator review, limited-account/public-access guards, and short-lived sanitized profile-image uploads. Fixed a real onboarding race where concurrent `/session` reads rotated CSRF tokens and caused profile saves to fail; tokens are now stable and session-bound. Made the live E2E fixture unique per run.
- **Files/components:** Identity/campus/media/project handlers, worker/config, rewritten pre-production migration, signup/onboarding/admin screens, API/OpenAPI, test runner, `plans/setup-github-oauth.md`, operations/run/test guides, Phase 2/3 roadmap and integration/E2E tests.
- **Validation:** `node scripts/test.mjs` passed: fresh/repeat migrations, `go test ./... -count=1` with PostgreSQL integration enabled, `go vet ./...`, staticcheck, web typecheck/build, fake GitHub PKCE browser sign-in, OTP lockout/resend invalidation integration, real worker-to-Mailpit link delivery/confirmation, responsive Playwright, Compose validation and `git diff --check`. Focused campus tests also passed. `graphify . --code-only` and `graphify cluster-only D:\projects\peergit` refreshed the project graph and report; Graphify warned its optional SQL parser is not installed.
- **Follow-up:** Phase 2 remains open pending explicit expiry/provider-error/email-conflict/cross-account/concurrent-consumption and SMTP crash/retry/fencing tests; the two-student project/recruitment browser flow also remains. Production GitHub App/email-provider credentials and production qualification are separate gates. No production database reset or live OAuth/SMTP test was performed.
- **References:** User-approved Phase 2 implementation plan; [new-sign-up.md](plans/new-sign-up.md), [setup-github-oauth.md](plans/setup-github-oauth.md).

### 2026-10-04T04:53Z — Rename signup implementation guide

- **Phase/area:** Identity planning documentation.
- **Summary:** Renamed the signup refactor guide to `plans/new-sign-up.md` and updated all markdown references throughout the repository.
- **Files/components:** `plans/new-sign-up.md`, planning references, `WORKLOG.md`.
- **Validation:** Repository-wide ignored-file-inclusive search found no old filename references; `git diff --check` passed. Documentation-only change; no runtime tests run.
- **Follow-up:** The new guide remains ignored by `plans/*` in `.gitignore`, consistent with the existing source-plan policy.
- **References:** User request.


### 2026-10-04T04:47Z — Plan GitHub signup and independent campus verification

- **Phase/area:** Identity planning / reopened Phase 2 acceptance.
- **Summary:** Replaced launch signup assumptions with GitHub App OAuth and limited accounts followed by independent ten-minute link/OTP or audited campus approval. Retained historical Google results, added unchecked refactor gates, moved minimal verification SMTP/worker delivery into Phase 2, and documented current-versus-planned setup.
- **Files/components:** `plans/plan-new.md`, `plans/plan.md`, `plans/plan-future-scale.md`, `plans/implementation-phases.md`, `plans/new-sign-up.md`, local service/run/test guides and `plans/phase0-requirements.md`.
- **Validation:** Parsed Graphify layout; reviewed relevant current implementation/config and official GitHub/Mailpit documentation. All relative links and balanced Markdown fences across nine planning documents passed; contract/limits/phase-reopening assertions and `git diff --check` passed. No runtime tests were run because this change contains documentation only; no new application behavior is claimed.
- **Follow-up:** Implement `plans/new-sign-up.md` and revalidate Phase 2 plus affected Phase 3 flows. No source code, migrations, config, Compose or test runner changed in this task; preserve the pre-existing `apps/web/next-env.d.ts` change. Source plans, new refactor guide and proof setup guide remain ignored under existing Git rules; use deliberate force-add if they should be committed.
- **References:** User-approved 2026-10-04 signup/campus-verification contract; GitHub App user authorization/email API and Mailpit documentation.


### 2026-10-04T03:34Z — Verify and clarify local startup guide

- **Phase/area:** Local development operations documentation.
- **Summary:** Checked the run guide against the current Compose stack, `.env` loading, Go entry points, Next.js rewrite, and Caddy routing. Clarified which containers are needed for core workflows and removed `npm ci` from the recurring frontend startup commands.
- **Files/components:** `plans/peergit-run.md`.
- **Validation:** `docker compose -f deploy/compose/local.yml config --quiet`, `git diff --check`, and relative-link checks passed. Runtime startup was not run; this was a documentation update.
- **Follow-up:** Start the stack using the updated guide and confirm Google OAuth with the configured local test client.
- **References:** Current local Compose and application configuration.

### 2026-10-03T14:16Z — Add shared cross-platform CI test runner

- **Phase/area:** CI and pre-commit verification.
- **Summary:** Added `scripts/test.mjs` as the common Windows/macOS/Linux test entry point and changed GitHub Actions to call it. CI always makes clean npm/browser installs; local runs reuse node_modules when package files match and reuse the installed Playwright browser. Fixed the browser test to use explicit guest-session mocks and scope readiness checks to the readiness status element; documented the script and its focused checks.
- **Files/components:** `scripts/test.mjs`, `.github/workflows/ci.yml`, `tests/e2e/walkthrough.cjs`, `plans/peergit-test.md`.
- **Validation:** Two final `node scripts/test.mjs` runs passed on Windows through Go formatting, migrations, all Go tests with PostgreSQL and caching disabled, vet, staticcheck, web typecheck/build, Playwright desktop/mobile and unavailable-service checks, Compose validation, and `git diff --check`. The second run confirmed local npm packages and Chromium were reused. Node syntax checks passed. Graphify refreshed to 634 nodes and 1,788 edges; cluster report regenerated.
- **Follow-up:** The GitHub workflow now runs this same script after installing Go and Node. macOS execution has not been exercised locally.
- **References:** CI browser failure log provided 2026-10-03.

### 2026-10-03T13:44Z — Fix role creation without prerequisite skills

- **Phase/area:** Phase 3 / recruitment.
- **Summary:** The CI migration command succeeded; the following integration test failed because an omitted `prerequisite_skills` request decoded to a nil slice and was explicitly inserted as SQL NULL into a NOT NULL array column. Normalize omission to an empty slice and assert that role creation stores an empty array.
- **Files/components:** `internal/recruitment/handler.go`, `tests/integration/phase3_test.go`, `plans/peergit-test.md`.
- **Validation:** Local PostgreSQL started healthy. The focused Phase 3 test passed end to end; all five verbose integration tests passed with no skips; `go test ./... -count=1` with `TEST_DATABASE_URL`, `go vet ./...`, and `staticcheck ./...` passed. `go run ./cmd/migrate` passed twice against the local database. Formatting and diff checks passed.
- **Follow-up:** Rerun CI on this revision to confirm the hosted environment matches the local PostgreSQL result.
- **References:** Attached CI log from 2026-10-03T13:37Z.

### 2026-10-03T13:29Z — Repair Phase 3 CSRF integration fixture and document pre-commit checks

- **Phase/area:** Phase 3 / integration testing and contributor workflow.
- **Summary:** The Phase 3 test now obtains each actor's real CSRF token from the session endpoint instead of sending a short placeholder rejected by the API. Added a canonical PowerShell pre-commit test guide and required agents to follow and update it; clarified that repeated unchanged migrations are safe and that database/browser checks are conditional.
- **Files/components:** `tests/integration/phase3_test.go`, `plans/peergit-test.md`, `plans/peergit-run.md`, `AGENTS.md`, `.gitignore`.
- **Validation:** Focused Phase 3 test compiled but skipped without `TEST_DATABASE_URL`. `go test ./... -count=1`, `go vet ./...`, `staticcheck ./...`, `gofmt` check, Compose config, `git diff --check`, and parsing all nine PowerShell examples passed. `docker info` could not reach the Docker Desktop Linux engine, so the PostgreSQL-backed Phase 3 flow remains unverified locally.
- **Follow-up:** Run the focused Phase 3 test and full integration suite with `TEST_DATABASE_URL` against the local Compose PostgreSQL service, then rerun CI. A skipped integration test is not a passing database check.
- **References:** Reported CI `csrf_rejected` failure; `plans/peergit-test.md`.

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
