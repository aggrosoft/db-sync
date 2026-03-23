# Requirements: DB Sync CLI

**Defined:** 2026-03-23
**Core Value:** Operators can understand what will happen before a sync runs and recover safely if the run fails.

## v1 Requirements

### Profiles

- [ ] **PROF-01**: Operator can create a saved sync profile that stores source connection settings, target connection settings, selected tables, and sync behavior choices.
- [ ] **PROF-02**: Operator can review and update a saved sync profile through an interactive CLI flow.
- [ ] **PROF-03**: Operator can validate source and target connectivity before a profile is saved.

### Schema Analysis

- [ ] **SCMA-01**: Tool can introspect source and target schemas for supported PostgreSQL and MySQL/MariaDB databases.
- [ ] **SCMA-02**: Tool can detect missing, extra, or incompatible columns between source and target tables.
- [ ] **SCMA-03**: Tool can classify drift outcomes as writable, writable-with-warning, or blocked.
- [ ] **SCMA-04**: Tool warns when source columns will be skipped because no writable target mapping exists.
- [ ] **SCMA-05**: Tool recognizes when target defaults or nullable columns can satisfy missing source-to-target values.

### Table Selection

- [ ] **TABL-01**: Tool automatically discovers foreign key dependencies relevant to selected tables.
- [ ] **TABL-02**: Operator can interactively include or exclude tables while seeing dependency implications.
- [ ] **TABL-03**: Tool can suggest the minimum required dependent tables for a valid sync plan.

### Preflight Scan

- [ ] **SCAN-01**: Operator can run a scan-only mode without writing live target data.
- [ ] **SCAN-02**: Scan mode reports constraint, nullability, key, and dependency issues that would block writes.
- [ ] **SCAN-03**: Scan mode reports row counts and issue counts per table.
- [ ] **SCAN-04**: Scan mode produces actionable remediation guidance for each detected blocker or warning.

### Live Sync

- [ ] **SYNC-01**: Tool can copy rows for selected tables from source to target in dependency-safe order.
- [ ] **SYNC-02**: Tool supports insert-missing behavior for rows absent from the target.
- [ ] **SYNC-03**: Tool supports mirror-delete behavior for target rows that are missing from the source when enabled.
- [ ] **SYNC-04**: Live run shows clear progress indicators, table-level progress, and final status summaries.
- [ ] **SYNC-05**: Tool can resume large table processing through chunked execution and checkpoints within a run design.

### Rollback And Recovery

- [ ] **ROLL-01**: Tool prepares rollback artifacts before live writes begin.
- [ ] **ROLL-02**: Tool can halt a failing run and guide restoration of the target toward its pre-run state.
- [ ] **ROLL-03**: Tool records enough run metadata to audit what changed and what must be restored.

### Performance

- [ ] **PERF-01**: Tool is designed for efficient sync of tables in the 1 to 20 million row range.
- [ ] **PERF-02**: Tool uses bulk-oriented read and write strategies appropriate for the supported databases.
- [ ] **PERF-03**: Scan and sync execution avoid loading entire large tables into memory at once.

### UX And Reporting

- [ ] **UX-01**: Interactive CLI uses readable prompts, colors, and summaries suitable for technical operators.
- [ ] **UX-02**: Tool presents warnings and blockers in language that makes next steps clear.
- [ ] **UX-03**: Tool persists run outputs or summaries that can be reviewed after the terminal session.

## v2 Requirements

### Sync Semantics

- **SEM-01**: Tool supports upsert behavior based on primary or unique keys.
- **SEM-02**: Tool supports cross-engine sync between PostgreSQL and MySQL/MariaDB.
- **SEM-03**: Tool supports selective column mapping rules beyond like-for-like schema sync.

### Automation

- **AUTO-01**: Tool supports scheduled recurring sync runs.
- **AUTO-02**: Tool supports non-interactive automation-oriented execution profiles.
- **AUTO-03**: Tool supports real-time CDC or streaming replication.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Bidirectional sync and conflict resolution | Adds complex reconciliation logic that is not required for initial migrations |
| Automatic schema migrations | Too risky for v1 and conflicts with operator-controlled schema changes |
| Cross-table transformations | Turns the tool into a broader ETL platform |
| Web UI or desktop GUI | CLI-first experience is the current product focus |
| Cloud-hosted control plane | Not needed to solve the immediate operator workflow |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| PROF-01 | Phase 1 | Pending |
| PROF-02 | Phase 1 | Pending |
| PROF-03 | Phase 1 | Pending |
| SCMA-01 | Phase 2 | Pending |
| SCMA-02 | Phase 2 | Pending |
| SCMA-03 | Phase 2 | Pending |
| SCMA-04 | Phase 2 | Pending |
| SCMA-05 | Phase 2 | Pending |
| TABL-01 | Phase 2 | Pending |
| TABL-02 | Phase 2 | Pending |
| TABL-03 | Phase 2 | Pending |
| SCAN-01 | Phase 3 | Pending |
| SCAN-02 | Phase 3 | Pending |
| SCAN-03 | Phase 3 | Pending |
| SCAN-04 | Phase 3 | Pending |
| SYNC-01 | Phase 4 | Pending |
| SYNC-02 | Phase 4 | Pending |
| SYNC-03 | Phase 4 | Pending |
| SYNC-05 | Phase 4 | Pending |
| ROLL-01 | Phase 5 | Pending |
| ROLL-02 | Phase 5 | Pending |
| ROLL-03 | Phase 5 | Pending |
| PERF-01 | Phase 6 | Pending |
| PERF-02 | Phase 4 | Pending |
| PERF-03 | Phase 4 | Pending |
| UX-01 | Phase 6 | Pending |
| UX-02 | Phase 3 | Pending |
| UX-03 | Phase 5 | Pending |
| SYNC-04 | Phase 6 | Pending |

**Coverage:**
- v1 requirements: 29 total
- Mapped to phases: 29
- Unmapped: 0 ✓

---
*Requirements defined: 2026-03-23*
*Last updated: 2026-03-23 after initial definition*