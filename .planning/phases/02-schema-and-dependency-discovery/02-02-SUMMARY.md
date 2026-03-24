---
phase: 02-schema-and-dependency-discovery
plan: 02
subsystem: database
tags: [schema-drift, classification, postgres, mysql, reporting]
requires:
  - phase: 02-schema-and-dependency-discovery
    provides: canonical schema snapshot model, normalized table identity, discovery orchestration
provides:
  - normalized snapshot comparison with explicit missing, extra, skipped, and incompatible reason codes
  - deterministic table classification as writable, writable-with-warning, or blocked
  - downstream-ready drift report model for later scan and sync planning
affects: [phase-02, scan-planning, sync-planning, operator-reporting]
tech-stack:
  added: []
  patterns: [normalized schema drift analysis, table-level compatibility classification, reusable drift reporting]
key-files:
  created: [internal/schema/drift.go, internal/schema/report.go]
  modified: [internal/schema/compare_test.go]
key-decisions:
  - "Compare normalized table and column facts by canonical table ID and column name instead of raw DDL text."
  - "Treat unmatched source columns as explicit skipped warnings, never as implied schema migrations."
  - "Block only when target-only columns require source data or when matched columns have incompatible type, nullability, or primary-key shape."
patterns-established:
  - "Drift reports aggregate table-level warnings and blockers from column-level reasons."
  - "Safe target-only columns remain visible as extra-column drift while still classifying the table as writable-with-warning."
requirements-completed: [SCMA-02, SCMA-03, SCMA-04, SCMA-05]
duration: session
completed: 2026-03-24
---

# Phase 2 Plan 02: Drift Analysis Summary

**Normalized schema drift comparison with explicit reason codes, compatibility classification, and a reusable report model for downstream scan and sync phases**

## Performance

- **Duration:** session
- **Started:** 2026-03-24
- **Completed:** 2026-03-24
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Added a schema drift engine that compares canonical source and target snapshots per table and column.
- Classified tables deterministically as `writable`, `writable-with-warning`, or `blocked` based on explicit warning and blocker reasons.
- Replaced the Wave 0 placeholder comparison tests with real coverage for skipped source columns, optional target-only columns, incompatible types, and blocking required target columns.

## Task Commits

None. Git commits were intentionally not created because execution was performed under an explicit no-commit user instruction.

## Files Created/Modified

- `internal/schema/drift.go` - Drift reason types, comparison logic, and table classification rules.
- `internal/schema/report.go` - Reusable drift report model with source and target snapshot identities plus table lookup helpers.
- `internal/schema/compare_test.go` - Focused table-driven coverage for comparison, skipped-column warnings, classification, and optional target-column handling.
- `.planning/phases/02-schema-and-dependency-discovery/02-02-SUMMARY.md` - Execution summary for the completed 02-02 plan.

## Decisions Made

- Used canonical `TableID` plus exact column names as the comparison key so PostgreSQL qualified tables remain unambiguous.
- Reported skipped source columns explicitly as warnings and preserved their names on `TableDrift.SkippedColumns` for later operator-facing summaries.
- Treated target-only nullable, default-backed, identity, or generated columns as non-blocking drift while still surfacing them in the report.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 02-03 can build dependency-aware table selection on top of the same canonical table identities used by the new drift report.
- Later scan and sync phases can consume `DriftReport`, `TableDrift`, and per-column reasons without redefining classification semantics.

---
*Phase: 02-schema-and-dependency-discovery*
*Completed: 2026-03-24*