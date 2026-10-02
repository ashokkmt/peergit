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
