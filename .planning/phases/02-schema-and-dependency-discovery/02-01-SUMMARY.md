---
phase: 02-schema-and-dependency-discovery
plan: 01
subsystem: database
tags: [postgres, mysql, mariadb, schema-discovery, validation]
requires:
  - phase: 01-foundation-cli
    provides: validated profile normalization, saved endpoint configuration, adapter registry seams
provides:
  - canonical schema snapshot model with qualified table identity
  - shared endpoint resolver reused by validation and discovery
  - read-only PostgreSQL and MySQL/MariaDB schema discovery adapters
  - Wave 0 schema and CLI test scaffolding for later Phase 2 work
affects: [phase-02, drift-analysis, dependency-selection, scan-planning]
tech-stack:
  added: []
  patterns: [adapter-backed discovery, shared endpoint resolution, canonical snapshot normalization]
key-files:
  created: [internal/schema/snapshot.go, internal/schema/service.go, internal/validate/endpoint.go, internal/schema/snapshot_test.go, internal/schema/service_test.go, internal/schema/compare_test.go, internal/schema/graph_test.go, internal/cli/table_selection_test.go, internal/db/postgres/adapter_integration_test.go, internal/db/mysql/adapter_integration_test.go, internal/db/mysql/mariadb_integration_test.go]
  modified: [internal/validate/service.go, internal/db/postgres/adapter.go, internal/db/mysql/adapter.go]
key-decisions:
  - "Keep schema discovery in the existing engine adapters and normalize into shared runtime structs."
  - "Inject the shared endpoint resolver into schema discovery to avoid validate/schema package cycles while preserving a single resolution implementation."
  - "Treat incomplete metadata visibility as blocked discovery with remediation guidance rather than partial success."
patterns-established:
  - "Qualified table identity uses schema.table via schema.TableID."
  - "Discovery adapters expose separate source and target methods but share read-only catalog queries underneath."
requirements-completed: [SCMA-01]
duration: session
completed: 2026-03-24
---

# Phase 2 Plan 01: Schema Discovery Foundation Summary

**Canonical PostgreSQL and MySQL/MariaDB schema snapshots with shared endpoint resolution and Wave 0 test coverage for downstream drift and dependency work**

## Performance

- **Duration:** session
- **Started:** 2026-03-24
- **Completed:** 2026-03-24
- **Tasks:** 3
- **Files modified:** 14

## Accomplishments

- Added a canonical schema snapshot package that preserves qualified table IDs, ordered columns, primary keys, and foreign keys.
- Extracted endpoint resolution into a shared helper and reused it in validation while making discovery consume the same semantics.
- Extended PostgreSQL and MySQL/MariaDB adapters with read-only schema discovery plus integration coverage and Wave 0 test scaffolding for later Phase 2 plans.

## Task Commits

None. Git commits were intentionally not created because execution was performed under an explicit no-commit user instruction.

## Files Created/Modified

- `internal/schema/snapshot.go` - Canonical schema snapshot model and blocked-discovery error type.
- `internal/schema/service.go` - Discovery orchestration for normalized profiles and adapter-backed endpoint discovery.
- `internal/validate/endpoint.go` - Shared endpoint resolution helper reused by validation and discovery callers.
- `internal/validate/service.go` - Validation updated to call the extracted endpoint resolver.
- `internal/db/postgres/adapter.go` - PostgreSQL catalog-backed schema discovery with blocked metadata visibility detection.
- `internal/db/mysql/adapter.go` - MySQL/MariaDB information-schema discovery for columns, primary keys, and foreign keys.
- `internal/schema/snapshot_test.go` - Real assertions for normalization, qualified IDs, and blocked error behavior.
- `internal/schema/service_test.go` - Unit coverage for endpoint-resolution blocking and remediation reporting.
- `internal/schema/compare_test.go` - Wave 0 placeholder tests for drift work in 02-02.
- `internal/schema/graph_test.go` - Wave 0 placeholder tests for dependency work in 02-03.
- `internal/cli/table_selection_test.go` - Wave 0 placeholder test entry point for dependency-aware CLI selection.
- `internal/db/postgres/adapter_integration_test.go` - PostgreSQL discovery integration coverage, including blocked metadata visibility behavior.
- `internal/db/mysql/adapter_integration_test.go` - MySQL discovery integration coverage for ordered columns, generated metadata, and foreign keys.
- `internal/db/mysql/mariadb_integration_test.go` - MariaDB discovery smoke coverage through the shared MySQL adapter path.

## Decisions Made

- Used `schema.TableID` with `schema.table` string rendering so PostgreSQL multi-schema discovery is unambiguous.
- Kept the shared resolver in `internal/validate/endpoint.go`, but injected it into `internal/schema/service.go` instead of importing `validate` directly, which preserves reuse without creating a package cycle.
- Flagged metadata visibility problems as blocked discovery errors with remediation strings so later CLI/reporting layers can render actionable guidance.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Removed a validate/schema import cycle introduced by the shared discovery service**
- **Found during:** Final integration verification
- **Issue:** `internal/schema/service.go` imported `internal/validate`, while validate integration tests import engine adapters that depend on `internal/schema`, creating a package cycle.
- **Fix:** Changed the schema discovery service to accept an injected endpoint resolver function and passed `validate.ResolveEndpoint` from tests/callers.
- **Files modified:** `internal/schema/service.go`, `internal/schema/service_test.go`
- **Verification:** `go test ./internal/schema/... ./internal/validate/... ./internal/cli/...` and `go test -tags=integration ./internal/db/... ./internal/validate/... -run 'Test(Postgres|MySQL|MariaDB|ValidateProfileIntegration)'`

**2. [Rule 1 - Bug] Corrected the MySQL generated-column integration fixture**
- **Found during:** Integration verification for `TestMySQLDiscoverSchema`
- **Issue:** The initial generated column referenced an auto-increment column, which MySQL rejects.
- **Fix:** Changed the generated expression to derive from `account_id` instead of the auto-increment primary key.
- **Files modified:** `internal/db/mysql/adapter_integration_test.go`
- **Verification:** `go test -tags=integration ./internal/db/... -run 'Test(Postgres|MySQL|MariaDB)'`

---

**Total deviations:** 2 auto-fixed
**Impact on plan:** Both fixes were required to complete 02-01 verification cleanly. Scope remained inside the planned discovery foundation.

## Issues Encountered

- MySQL testcontainer startup produced repeated driver `unexpected EOF` log lines during boot, but the final integration suite passed once the fixture DDL was corrected.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 02-02 can now build drift comparison and classification directly on the canonical snapshot model.
- 02-03 can reuse the foreign-key metadata already emitted by the discovery adapters and fill in the Wave 0 placeholder tests.

---
*Phase: 02-schema-and-dependency-discovery*
*Completed: 2026-03-24*