---
phase: 01-foundation-cli
plan: 03
subsystem: database
tags: [go, postgres, mysql, testcontainers, validation]
requires:
  - phase: 01-01
    provides: profile schema and secret contract
  - phase: 01-02
    provides: reviewed new and edit wizard output
provides:
  - XDG-backed YAML profile persistence
  - Save-gated dual-endpoint validation service
  - Live PostgreSQL and MySQL integration verification
affects: [phase-2-discovery, operator-validation, profile-reuse]
tech-stack:
  added: [pgx/v5, go-sql-driver/mysql, testcontainers-go]
  patterns: [adapter-based validation, blocked-save reporting, live container integration tests]
key-files:
  created: [internal/profile/filesystem_store.go, internal/profile/filesystem_store_test.go, internal/validate/service.go, internal/validate/service_test.go, internal/validate/validate_integration_test.go, internal/testkit/containers.go, internal/db/postgres/adapter.go, internal/db/mysql/adapter.go, internal/cli/profile_validate.go, internal/cli/profile_save.go]
  modified: [internal/cli/app.go, internal/cli/profile_command.go, internal/secrets/template.go]
key-decisions:
  - "Validation reports are rendered even on failure so operators see which endpoint or check blocked the save."
  - "Profiles without ${NAME} placeholders are rejected to keep raw credentials out of persisted YAML."
patterns-established:
  - "Validation orchestration is engine-agnostic and dispatches through per-engine adapters."
  - "Integration tests use disposable databases to prove save-gated validation across supported engines."
requirements-completed: [PROF-01, PROF-02, PROF-03]
duration: 35 min
completed: 2026-03-23
---

# Phase 01 Plan 03: Validation And Persistence Summary

**XDG-backed profile persistence with adapter-based PostgreSQL or MySQL validation, blocked-save reporting, and live integration coverage**

## Performance

- **Duration:** 35 min
- **Started:** 2026-03-23T09:26:00Z
- **Completed:** 2026-03-23T10:01:00Z
- **Tasks:** 3
- **Files modified:** 13

## Accomplishments
- Implemented profile save, load, and list behavior in the per-user config directory.
- Added save-gated validation for both endpoints with PostgreSQL and MySQL adapters plus structured operator-facing reports.
- Added passing live integration tests that exercise disposable PostgreSQL and MySQL databases with testcontainers.

## Task Commits

This inline execution did not create per-task git commits.

## Files Created/Modified
- `internal/profile/filesystem_store.go` - YAML-backed store with slugged filenames in the config directory.
- `internal/validate/service.go` - placeholder enforcement, validation orchestration, and save gating.
- `internal/db/postgres/adapter.go` and `internal/db/mysql/adapter.go` - engine-specific validation probes.
- `internal/validate/validate_integration_test.go` and `internal/testkit/containers.go` - live validation coverage against disposable databases.
- `internal/cli/profile_validate.go` and `internal/cli/profile_save.go` - CLI save and validate wiring.

## Decisions Made
- Chose to enforce placeholder presence during validation and persistence rather than trusting the wizard alone.
- Used real testcontainers-backed databases because Phase 1 claims dual-endpoint validation proof, and skip-only tests were insufficient.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Rendered blocked validation reports on CLI error paths**
- **Found during:** Task 2
- **Issue:** Validation failures returned immediately, hiding the structured endpoint report from the operator.
- **Fix:** Rendered the report before returning validation errors from save and validate flows.
- **Files modified:** `internal/cli/app.go`
- **Verification:** `go test ./...`
- **Committed in:** Not committed in this inline execution

**2. [Rule 2 - Missing Critical] Enforced placeholder-only DSN policy before persistence**
- **Found during:** Task 1
- **Issue:** Raw credentials could still be stored in YAML if a DSN contained no placeholders.
- **Fix:** Added placeholder-policy validation in secrets, validation orchestration, and filesystem persistence.
- **Files modified:** `internal/secrets/template.go`, `internal/validate/service.go`, `internal/profile/filesystem_store.go`, `internal/profile/filesystem_store_test.go`
- **Verification:** `go test ./internal/profile/... ./internal/secrets/... ./internal/validate/...`
- **Committed in:** Not committed in this inline execution

**3. [Rule 3 - Blocking] Replaced skipped integration helpers with live database containers**
- **Found during:** Task 3
- **Issue:** The integration-tag suite passed without actually exercising PostgreSQL or MySQL validation.
- **Fix:** Implemented real testcontainers-based helpers and live validation tests, including blocked-save coverage.
- **Files modified:** `internal/testkit/containers.go`, `internal/validate/validate_integration_test.go`, `go.mod`, `go.sum`
- **Verification:** `go test ./... -tags=integration`
- **Committed in:** Not committed in this inline execution

---

**Total deviations:** 3 auto-fixed (1 bug, 1 missing critical, 1 blocking)
**Impact on plan:** All deviations were required to meet the phase safety contract and complete verification. No unrelated scope was added.

## Issues Encountered
- The first live integration run used a PostgreSQL wait strategy without the registered stdlib driver. Adding the `pgx/v5/stdlib` registration resolved the container readiness check.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 1 is verified and leaves reusable profile persistence plus live endpoint validation in place for schema-discovery work.
- Future phases can build on saved profiles and the validation registry without revisiting core CLI or secret-handling contracts.

---
*Phase: 01-foundation-cli*
*Completed: 2026-03-23*