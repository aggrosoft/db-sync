# DB Sync CLI

## What This Is

DB Sync CLI is a terminal-first tool for developers and engineers who need to safely move data from one database into another when schemas are similar but not identical. It focuses on guided sync setup, dependency-aware table selection, preflight scanning for write blockers, and high-throughput live runs with rollback preparation.

## Core Value

Operators can understand what will happen before a sync runs and recover safely if the run fails.

## Requirements

### Validated

- [x] Configure reusable source and target sync profiles from the CLI — validated in Phase 01

### Active

- [ ] Detect schema drift and warn about unmapped or missing columns before writes begin
- [ ] Discover table dependencies automatically and help operators pick only the tables they need
- [ ] Run a preflight scan that reports rows or tables that cannot be written because of constraints or schema issues
- [ ] Execute large sync runs efficiently for same-engine PostgreSQL and MySQL/MariaDB databases
- [ ] Prepare rollback artifacts so failed syncs can restore the target to its prior state

### Out of Scope

- Cross-engine sync between PostgreSQL and MySQL/MariaDB — defer until the write model and type mapping rules are proven
- Bidirectional sync and conflict resolution — not needed for the initial one-way migration workflow
- Real-time CDC or streaming replication — batch sync is the immediate need
- Automatic schema migrations — too risky for v1; the operator must stay in control
- Cross-table transformations — v1 is about faithful sync, not ETL pipelines
- Web UI or desktop GUI — the product is intentionally CLI-first

## Context

- Greenfield repository with no existing application code.
- Primary operators are developers and engineers performing one-off or repeatable migration/sync tasks.
- Initial database support is limited to PostgreSQL to PostgreSQL and MySQL/MariaDB to MySQL/MariaDB.
- Saved profiles are part of v1 so operators can rerun established sync definitions.
- Desired UX includes an interactive CLI setup flow, clear progress bars, colors, and readable summaries.
- The target environment includes large tables, roughly 1 to 20 million rows per table.

## Current State

Phase 01 is complete. The CLI now supports a wizard-first root entrypoint, connection-first create/edit flows, env-backed profile persistence, compatibility with legacy template-based profiles, and save-gated validation for PostgreSQL and MySQL/MariaDB endpoints.

## Constraints

- **Platform**: Single cross-platform CLI binary — distribution should stay simple for operators.
- **Language**: Go — chosen for packaging, concurrency, and operational simplicity.
- **Database Scope**: Same-engine PostgreSQL and MySQL/MariaDB only — limits type coercion and semantic mismatch risk in v1.
- **Safety**: Rollback must be prepared before live writes — failures cannot leave the target in an undefined state.
- **Schema Drift**: Unmapped source columns should be skipped with warnings, and target defaults/nullability may satisfy missing values — keeps drift handling explicit without auto-migrating schemas.
- **Execution Mode**: One-shot manual runs with saved profiles — scheduling and continuous sync are deferred.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Build a Go CLI first | Fits the single-binary distribution and performance goals | — Pending |
| Support same-engine sync only in v1 | Reduces complexity around type mapping and write semantics | — Pending |
| Use scan-before-live as a core workflow | Prevents avoidable failures during large sync operations | — Pending |
| Keep schema changes operator-driven | Automatic target mutations are too risky for the first release | — Pending |
| Use snapshot plus compensating restore for rollback | Large sync runs need a recovery path beyond simple transactions | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition**:
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone**:
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-03-23 after Phase 01 gap closure and verification*