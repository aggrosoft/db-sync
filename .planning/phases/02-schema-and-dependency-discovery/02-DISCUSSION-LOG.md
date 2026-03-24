# Phase 2: Schema And Dependency Discovery - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-24
**Phase:** 02-schema-and-dependency-discovery
**Areas discussed:** Metadata acquisition, Drift analysis, Dependency discovery and table selection, Persistence and operator flow

---

## Metadata acquisition

| Option | Description | Selected |
|--------|-------------|----------|
| Adapter extension | Extend the existing per-engine adapters with discovery methods so validation and discovery share connection handling. | ✓ |
| Separate discovery service | Build a new subsystem outside the adapter registry and bridge it back into the CLI later. | |
| Database-specific tooling only | Skip a shared abstraction and let each engine expose unrelated discovery behavior. | |

**User's choice:** `[auto]` Adapter extension
**Notes:** Recommended default selected automatically because Phase 1 already established the adapter registry and validation orchestration paths.

---

## Drift analysis

| Option | Description | Selected |
|--------|-------------|----------|
| Three-state table classification | Report each table as `writable`, `writable-with-warning`, or `blocked`, with column-level reasons. | ✓ |
| Binary pass/fail | Only distinguish writable vs blocked and leave warnings implicit. | |
| Free-form text only | Emit prose descriptions without a stable classification model. | |

**User's choice:** `[auto]` Three-state table classification
**Notes:** Recommended default selected automatically because the requirements explicitly call for classification clarity and warning handling.

---

## Dependency discovery and table selection

| Option | Description | Selected |
|--------|-------------|----------|
| Foreign-key graph with explicit operator choices | Discover FK dependencies, suggest the minimum required table set, and show blocked selections when required dependencies are excluded. | ✓ |
| Auto-include everything required | Silently add dependent tables and prevent exclusions that would make the plan invalid. | |
| No dependency graph | Leave the operator to infer dependent tables manually. | |

**User's choice:** `[auto]` Foreign-key graph with explicit operator choices
**Notes:** Recommended default selected automatically because it satisfies dependency-awareness while preserving operator control and transparency.

---

## Persistence and operator flow

| Option | Description | Selected |
|--------|-------------|----------|
| Persist only operator intent | Store selected tables and inclusion choices in profiles, but recompute live discovery data on demand. | ✓ |
| Persist full schema snapshots | Save drift and discovery metadata inside profiles for later reuse. | |
| Separate discovery workspace | Keep profile persistence unchanged and store all discovery outputs in external artifacts only. | |

**User's choice:** `[auto]` Persist only operator intent
**Notes:** Recommended default selected automatically because stale schema snapshots would undermine operator trust, while durable table intent belongs in the saved profile.

---

## the agent's Discretion

- Exact interface and type names for metadata discovery APIs.
- Exact CLI presentation details for table search, pagination, and selection summaries.

## Deferred Ideas

- Non-table dependency objects such as views, triggers, and procedures.
- Cross-engine mapping or compatibility rules.
- Offline reuse of persisted schema discovery snapshots.