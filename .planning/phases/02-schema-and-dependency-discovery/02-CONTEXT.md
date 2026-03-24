# Phase 2: Schema And Dependency Discovery - Context

**Gathered:** 2026-03-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2 adds read-only schema introspection, drift classification, dependency discovery, and dependency-aware table selection for supported PostgreSQL and MySQL/MariaDB profiles. This phase is about understanding source and target structure well enough to explain what can be synced safely and to persist operator table-selection intent, not about scanning live data rows or executing writes.

</domain>

<decisions>
## Implementation Decisions

### Metadata acquisition
- **D-01:** Phase 2 should extend the existing per-engine adapter model with read-only schema metadata capabilities rather than introducing a separate discovery subsystem.
- **D-02:** Schema discovery should use a shared information-schema baseline first, with engine-specific queries added only where PostgreSQL or MySQL/MariaDB require extra fidelity for keys, defaults, or dependency metadata.
- **D-03:** Discovery must stay non-mutating and work with the same resolved connection information already used by profile validation.

### Drift analysis model
- **D-04:** Drift should be classified per table as `writable`, `writable-with-warning`, or `blocked`, with concrete column-level reasons attached.
- **D-05:** Missing target mappings for source columns should produce warnings instead of implicit schema changes, and those warnings must name the skipped source columns.
- **D-06:** Missing source-to-target values may still be treated as writable when the target column is nullable or has a default the adapter can prove from metadata.

### Dependency discovery and table selection
- **D-07:** Dependency discovery should focus on foreign-key relationships in v1 and produce a directed dependency graph that downstream scan and sync phases can reuse.
- **D-08:** Interactive table selection should let the operator start from explicit table choices, then show required dependent tables and the minimum additional set needed for a valid plan.
- **D-09:** When the operator excludes a required dependent table, the CLI should keep the choice visible but classify the resulting selection as blocked instead of silently correcting it.

### Persistence and operator flow
- **D-10:** Persist only durable operator intent in the profile, such as selected tables and dependency-related inclusion choices; recompute schema snapshots and drift results at runtime rather than storing stale discovery artifacts.
- **D-11:** Phase 2 should integrate with the existing interactive CLI and profile-edit flow instead of adding a separate standalone configuration system.

### Safety and scope limits
- **D-12:** Insufficient metadata permissions should surface as blocked discovery results with remediation guidance, not as partial silent success.
- **D-13:** Dependency handling in this phase does not expand to triggers, procedures, views, or cross-engine mapping rules; those remain outside the current roadmap scope unless later phases explicitly require them.

### the agent's Discretion
- Exact Go interface names for schema metadata and dependency graph APIs.
- Exact CLI prompt layout, search/filter affordances, and summary formatting for large schema selection screens.
- Exact internal data structures used to cache discovery results during a single command execution.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Product scope and requirements
- `.planning/PROJECT.md` — Product constraints, supported engines, operator safety principles, and current phase framing.
- `.planning/REQUIREMENTS.md` — Phase 2 requirement IDs `SCMA-01` through `SCMA-05` and `TABL-01` through `TABL-03`.
- `.planning/ROADMAP.md` — Phase 2 goal, success criteria, and current three-plan breakdown.

### Prior locked decisions
- `.planning/phases/01-foundation-cli/01-CONTEXT.md` — Reuse the wizard-first CLI direction, env-backed profile persistence, and save-gated validation model established in Phase 1.

### External specs
- No external specs — requirements are fully captured in the planning artifacts and decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/db/postgres/adapter.go`: Existing PostgreSQL adapter already proves metadata reachability and can be extended with schema introspection methods.
- `internal/db/mysql/adapter.go`: Existing MySQL/MariaDB adapter follows the same registry model and is the natural home for engine-specific discovery logic.
- `internal/validate/service.go`: Central place to resolve connection details and dispatch per-engine operations without duplicating secret handling.
- `internal/model/profile.go`: Canonical profile model that can absorb table-selection intent and future scan/sync options.
- `internal/profile/schema.go`: Normalization layer for any new persisted profile fields added in this phase.
- `internal/wizard/flow.go`: Existing interactive form flow that can host dependency-aware table-selection steps in create and edit paths.

### Established Patterns
- Engine behavior is organized behind adapter registration rather than switch-heavy orchestration.
- Connection data is resolved once through profile/env conventions before engine work begins.
- Profiles persist durable configuration, while runtime validation and probing results remain computed behavior.
- Operator-facing flows prefer guided CLI interaction over flags-only setup.

### Integration Points
- Extend the adapter registry and validation/discovery orchestration so schema reads reuse existing endpoint resolution.
- Add profile fields and normalization for selected tables and dependency-aware inclusion choices.
- Integrate discovery summaries and table-selection prompts into the CLI app and wizard edit/create flows.
- Produce drift and dependency results in a form that later scan and sync phases can consume without redefining the model.

</code_context>

<specifics>
## Specific Ideas

- Auto-selected default: use information-schema-first discovery with engine-specific supplements only when required for correctness.
- Auto-selected default: treat foreign keys as the phase boundary for dependency discovery, because they directly support dependency-aware table selection without pulling in broader database object management.
- Auto-selected default: recompute discovery results on demand so operators are not relying on stale schema snapshots inside saved profiles.

</specifics>

<deferred>
## Deferred Ideas

- Cross-engine schema mapping remains out of scope for v1 and should not shape Phase 2 abstractions.
- Discovery of triggers, procedures, views, and non-table dependency objects is deferred until a later phase proves the need.
- Persisting full discovery snapshots for offline reuse is deferred; current preference is live recomputation from the connected databases.

</deferred>

---

*Phase: 02-schema-and-dependency-discovery*
*Context gathered: 2026-03-24*