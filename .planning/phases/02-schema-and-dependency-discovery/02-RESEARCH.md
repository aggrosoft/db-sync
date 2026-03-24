# Phase 2: Schema And Dependency Discovery - Research

**Researched:** 2026-03-24
**Domain:** Read-only schema introspection, drift classification, and foreign-key dependency discovery for a Go CLI
**Confidence:** MEDIUM

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

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

### Deferred Ideas (OUT OF SCOPE)
- Cross-engine schema mapping remains out of scope for v1 and should not shape Phase 2 abstractions.
- Discovery of triggers, procedures, views, and non-table dependency objects is deferred until a later phase proves the need.
- Persisting full discovery snapshots for offline reuse is deferred; current preference is live recomputation from the connected databases.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCMA-01 | Tool can introspect source and target schemas for supported PostgreSQL and MySQL/MariaDB databases. | Reuse existing adapter registry, add read-only discovery methods, and normalize catalog results into a shared snapshot model. |
| SCMA-02 | Tool can detect missing, extra, or incompatible columns between source and target tables. | Compare normalized column metadata by canonical table ID and column name, not raw SQL text or DDL strings. |
| SCMA-03 | Tool can classify drift outcomes as writable, writable-with-warning, or blocked. | Add a table-level classifier with reason codes backed by nullable/default/identity metadata. |
| SCMA-04 | Tool warns when source columns will be skipped because no writable target mapping exists. | Preserve column-level explanation records and render them in drift summaries and CLI review output. |
| SCMA-05 | Tool recognizes when target defaults or nullable columns can satisfy missing source-to-target values. | Capture nullable, default, generated, and identity metadata explicitly in the shared schema model. |
| TABL-01 | Tool automatically discovers foreign key dependencies relevant to selected tables. | Build a directed FK graph from metadata queries, preserving composite-key ordering and referenced table identity. |
| TABL-02 | Operator can interactively include or exclude tables while seeing dependency implications. | Integrate dependency-aware selection into the existing wizard/profile edit flow, with visible blocked states instead of silent auto-fixes. |
| TABL-03 | Tool can suggest the minimum required dependent tables for a valid sync plan. | Add graph closure logic that computes the smallest required dependency set for explicit operator selections. |
</phase_requirements>

## Project Constraints (from copilot-instructions.md)

- Use the matching GSD workflow only because the user explicitly asked for phase research.
- Treat `gsd-*` work as skill-driven workflow work, not ad hoc planning.
- Do not recommend a research or planning flow that bypasses the GSD phase artifacts already in `.planning/`.
- After delivering this research artifact, the natural next step is phase planning, not implementation.

## Summary

Phase 2 should be planned as an extension of the existing adapter-and-service pattern, not as a new standalone discovery subsystem. The current code already has the right seams for this: PostgreSQL and MySQL/MariaDB adapters exist, connection and secret resolution are centralized in validation, profiles already persist `selection.tables`, and the CLI is wizard-first. The missing piece is a shared discovery model that can represent tables, columns, keys, defaults, and FK edges in a way later scan and sync phases can reuse.

The two structural risks that will create rework if ignored are table identity and connection resolution. First, PostgreSQL can expose multiple non-system schemas in a single database, but the current profile model only stores bare table names. Planning should assume Phase 2 persists canonical qualified identifiers such as `schema.table`, or an equivalent structured table ID, otherwise duplicate table names across schemas will become ambiguous immediately. Second, discovery should not duplicate DSN/env resolution logic from `internal/validate/service.go`; the planner should extract or share that path first so validation and discovery always connect with identical endpoint semantics.

Engine-specific metadata is manageable if the phase stays disciplined. Use `information_schema` as the baseline for both engines, then add narrow supplements only where the baseline loses fidelity: PostgreSQL needs `udt_*` and `pg_constraint` help for accurate type and FK handling, and MySQL/MariaDB needs careful treatment of default, generated, and FK-order metadata. The correct plan split remains the roadmap's three slices: adapters and snapshots first, drift analysis second, dependency-aware selection third.

**Primary recommendation:** Build Phase 2 around a shared `SchemaSnapshot -> DriftReport -> DependencySelection` pipeline, while keeping live metadata reads inside the existing per-engine adapters and persisting only operator intent, not discovery results.

## Standard Stack

### Core
| Library / Asset | Version | Purpose | Why Standard |
|-----------------|---------|---------|--------------|
| Existing PostgreSQL adapter + `github.com/jackc/pgx/v5` | v5.9.1 | Read-only PostgreSQL metadata queries and connection handling | Already used in validation, supports direct catalog queries without adding another abstraction layer. |
| Existing MySQL/MariaDB adapter + `github.com/go-sql-driver/mysql` | v1.9.3 | Read-only MySQL/MariaDB metadata queries and connection handling | Already used in validation and is the natural place to keep MySQL/MariaDB-specific discovery SQL. |
| Shared schema model in repo code | new internal package or existing model package extension | Canonical tables, columns, constraints, FK edges, drift results | Required to keep drift analysis and dependency selection engine-agnostic. |
| Existing validation-style service pattern | current repo pattern | Resolve endpoints once, dispatch per engine, return typed reports | Matches current `validate.Service` design and avoids switch-heavy orchestration. |

### Supporting
| Library / Asset | Version | Purpose | When to Use |
|-----------------|---------|---------|-------------|
| `github.com/spf13/cobra` | v1.10.2 | CLI command plumbing | Reuse for any new profile or discovery subcommands if Phase 2 surfaces explicit actions outside the default wizard path. |
| `github.com/charmbracelet/huh` | v1.0.0 | Interactive prompts for table selection and dependency review | Reuse for accessible wizard/edit selection flows. |
| `github.com/testcontainers/testcontainers-go` | v0.41.0 | Integration tests against real PostgreSQL/MySQL engines | Use for metadata-query verification and drift fixtures when Docker is available. |
| Existing `internal/testkit` helpers | current repo code | Container bootstrap for integration tests | Extend with schema fixture setup rather than creating a new test harness. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Repo-local metadata model plus adapter SQL | External schema-diff or migration library | Overkill for read-only sync planning; adds abstraction mismatch and pushes the project toward migration semantics that are explicitly out of scope. |
| Runtime recomputation of discovery results | Persisted schema snapshots in profiles | Snapshot persistence creates staleness and migration problems; it contradicts locked decision D-10. |
| Canonical qualified table IDs | Bare table-name strings only | Bare names are simpler for MySQL, but they are insufficient for PostgreSQL multi-schema discovery and create ambiguity in later phases. |

**Installation:**
```bash
# No new third-party dependency is required for the recommended Phase 2 shape.
# Reuse the versions already pinned in go.mod.
go test ./...
```

**Version verification:** Versions above were verified from the repository's `go.mod` on 2026-03-24. The recommended approach is to keep the existing pinned dependencies and add no new discovery-specific library.

## Architecture Patterns

### Recommended Project Structure
```text
internal/
├── schema/             # shared snapshot model, drift classifier, dependency graph, closure logic
├── db/
│   ├── postgres/       # PostgreSQL discovery SQL and row mapping
│   └── mysql/          # MySQL/MariaDB discovery SQL and row mapping
├── validate/           # shared endpoint resolution reused by validation and discovery
├── cli/                # command orchestration and rendering of reports
├── wizard/             # interactive selection/review prompts
└── model/              # durable profile model fields for persisted operator intent
```

### Pattern 1: Adapter-Backed Schema Snapshot
**What:** Each engine adapter exposes read-only methods that return a shared `SchemaSnapshot` model for one endpoint.
**When to use:** Plan 02-01, when implementing SCMA-01 and creating later inputs for drift and dependency logic.
**Example:**
```go
type DiscoveryAdapter interface {
    DiscoverSchema(ctx context.Context, resolvedDSN string, engine model.Engine) (schema.Snapshot, error)
}
```

Use `information_schema.columns` for the common column baseline, then supplement where needed:

```sql
SELECT
  table_schema,
  table_name,
  column_name,
  ordinal_position,
  column_default,
  is_nullable,
  data_type,
  udt_schema,
  udt_name,
  is_identity,
  is_generated,
  generation_expression
FROM information_schema.columns
WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name, ordinal_position;
```

Source: PostgreSQL official docs for `information_schema.columns` and current repo adapter pattern.

### Pattern 2: Normalize First, Compare Second
**What:** Convert engine rows into canonical column facts before comparing source and target.
**When to use:** Plan 02-02, when implementing SCMA-02 through SCMA-05.
**Example:**
```go
type Column struct {
    Name                 string
    Ordinal              int
    DataType             string
    NativeType           string
    Nullable             bool
    DefaultSQL           string
    HasProvenDefault     bool
    Identity             bool
    Generated            bool
}
```

Comparison should use canonical facts such as `Nullable`, `HasProvenDefault`, and normalized type identity. It should not compare raw DDL strings.

### Pattern 3: Directed Dependency Graph With Closure
**What:** Build a graph from child table to required parent table and compute the minimal closure for operator-selected tables.
**When to use:** Plan 02-03, when implementing TABL-01 through TABL-03.
**Example:**
```sql
SELECT
  kcu.table_schema,
  kcu.table_name,
  kcu.column_name,
  kcu.ordinal_position,
  kcu.position_in_unique_constraint,
  kcu.referenced_table_schema,
  kcu.referenced_table_name,
  kcu.referenced_column_name,
  rc.update_rule,
  rc.delete_rule
FROM information_schema.key_column_usage AS kcu
JOIN information_schema.referential_constraints AS rc
  ON rc.constraint_schema = kcu.constraint_schema
 AND rc.constraint_name = kcu.constraint_name
WHERE kcu.referenced_table_name IS NOT NULL
ORDER BY
  kcu.table_schema,
  kcu.table_name,
  kcu.constraint_name,
  kcu.ordinal_position;
```

Source: MySQL official docs for `KEY_COLUMN_USAGE` and `REFERENTIAL_CONSTRAINTS`; PostgreSQL official docs for `key_column_usage` and `pg_constraint`.

### Anti-Patterns to Avoid
- **Comparing raw type strings only:** PostgreSQL domains and user-defined types make `data_type` insufficient on its own.
- **Persisting schema snapshots in profile YAML:** Discovery results go stale and create migration burden for little gain.
- **Duplicating endpoint resolution logic:** Validation and discovery must not drift on env-file or secret semantics.
- **Silently auto-including required tables:** Locked decision D-09 requires visible blocked state when the operator excludes a required dependency.
- **Treating table name as globally unique:** PostgreSQL makes this false inside one database.

## Reusable Assets

- `internal/db/postgres/adapter.go` and `internal/db/mysql/adapter.go` already contain engine-specific connection handling and metadata-permission probes.
- `internal/validate/service.go` already centralizes env-backed DSN resolution and adapter dispatch. The planner should extract shared endpoint resolution from here instead of duplicating it.
- `internal/model/profile.go` already persists `selection.tables`, which gives Phase 2 a durable intent anchor.
- `internal/profile/schema.go` already normalizes profile data and is the correct place to evolve any new persisted selection fields.
- `internal/wizard/flow.go`, `internal/wizard/draft.go`, and `internal/wizard/review.go` already define the wizard-first edit/review loop that Phase 2 must extend.
- `internal/testkit/containers.go` already provides PostgreSQL and MySQL testcontainers, which is the preferred integration-test pattern for metadata queries.

## Engine-Specific Metadata Concerns

### PostgreSQL
- `information_schema.columns` only returns columns the current user can access. Partial visibility can look like schema drift when it is actually a permission problem. Treat that as blocked discovery, not as a normal comparison result.
- `data_type` alone is not enough for non-built-in types. PostgreSQL docs explicitly direct consumers to `udt_name` and related fields for accurate type identity.
- `is_identity`, `identity_generation`, `is_generated`, and `generation_expression` are available in `information_schema.columns` and should be captured because they influence whether an omitted target column is still writable.
- For FK discovery, `information_schema.key_column_usage` is enough for ordering metadata, but `pg_constraint` is the authoritative catalog for FK actions and advanced fidelity. Use it as the PostgreSQL supplement instead of adding broad catalog coupling elsewhere.
- The planner should explicitly decide schema scope. Because current profiles do not store a PostgreSQL schema name, the safe recommendation is to discover all non-system schemas in the connected database and persist qualified table IDs.

### MySQL 8.4
- `SELECT` from `information_schema.columns` is not automatically ordered; discovery queries must `ORDER BY ordinal_position`.
- `COLUMN_DEFAULT` is `NULL` both when the explicit default is `NULL` and when no default clause exists. Do not infer a usable non-null default from `COLUMN_DEFAULT` alone.
- `EXTRA` carries important writeability clues such as `auto_increment`, `DEFAULT_GENERATED`, `STORED GENERATED`, and `VIRTUAL GENERATED`.
- `KEY_COLUMN_USAGE.ordinal_position` and `position_in_unique_constraint` are required to preserve composite FK ordering; omitting them will produce wrong dependency metadata.
- Generated invisible primary key columns can appear in `INFORMATION_SCHEMA.COLUMNS` depending on server configuration. Phase 2 should at minimum detect and avoid misclassifying them as user-authored drift.

### MariaDB
- The MariaDB `COLUMNS` documentation confirms support for nullability, defaults, generation metadata, and `IS_GENERATED`, but the surface is not identical to MySQL 8.4.
- Because MariaDB is currently routed through the MySQL adapter, Phase 2 should keep its shared query projection to the MySQL/MariaDB intersection unless a MariaDB-specific branch is proven necessary.
- The biggest planner risk is assuming every MySQL 8.4 metadata column exists unchanged in MariaDB. Keep the baseline narrow and add MariaDB-specific tests or smoke coverage before expanding the query surface.

## Profile And Schema Model Implications

- The current `model.Selection.Tables []string` shape is acceptable only if entries are treated as canonical table IDs. For PostgreSQL, that should mean at least `schema.table`.
- If the CLI must preserve blocked exclusion choices per D-09, the profile model likely needs one additional persisted field for explicit exclusions or manual dependency overrides. Persist intent only, not computed closure.
- Discovery results should live in runtime-only structs such as `Snapshot`, `DriftReport`, and `SelectionPreview`. They should not be marshaled into profile YAML.
- Table identity should be represented consistently across all layers: adapter output, drift reports, dependency graphs, CLI rendering, and persisted selection.
- Planning should include backward-compatible normalization for existing profiles that store an empty table list. That is already the default and should remain valid.

## CLI Interaction Implications

- The current wizard is optimized for a fast connection-first path. Phase 2 should preserve that by making schema discovery a post-validation step inside the create/edit flow, not by front-loading complex table selection before connections are known to work.
- Large schemas will make a naive giant checklist unusable. The planner should prefer a staged interaction: seed-table selection, dependency impact summary, blocked/warning summary, final confirmation.
- Review output should evolve from `Tables: %v` to a concise summary showing explicit selections, required additions, and blocked exclusions.
- Discovery should surface remediation in operator language, especially for permission failures and blocked dependencies, because Phase 3 will build directly on this explainability model.
- If selection is exposed through `profile edit`, the same dependency-aware review must round-trip cleanly when editing an existing profile.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Schema comparison | Raw DDL parser or migration-style diff engine | Canonical metadata structs plus rule-based comparison | DDL parsing is brittle, engine-specific, and outside the MVP's read-only planning needs. |
| Dependency discovery | Heuristic table-name matching | FK metadata from `information_schema` and `pg_constraint` | FK catalogs already provide the source of truth for required table dependencies. |
| Endpoint resolution | A second env/secret resolution path inside discovery | Shared helper extracted from current validation service | Duplicate resolution logic will drift and create false differences between validation and discovery. |
| Table closure state | Persisted computed dependency graph in YAML | Runtime graph rebuild and closure calculation | The graph is live database state and becomes stale immediately if stored. |

**Key insight:** The hard parts of this phase are metadata fidelity and explainability, not transport or UI plumbing. The plan should spend complexity budget on canonical models and reason codes, not on new libraries or persistent caches.

## Common Pitfalls

### Pitfall 1: Permission-Filtered Metadata Looks Like Drift
**What goes wrong:** A source or target appears to be missing tables or columns, but the real issue is insufficient metadata privileges.
**Why it happens:** Both PostgreSQL information schema views and engine catalogs only expose objects visible to the current user.
**How to avoid:** Distinguish discovery failure from schema drift. If the metadata read is incomplete or denied, return blocked discovery with remediation guidance.
**Warning signs:** The validation probe succeeds, but discovery yields unexpectedly tiny table counts or empty key metadata.

### Pitfall 2: PostgreSQL Type Comparison Is Too Shallow
**What goes wrong:** Domains, arrays, or user-defined types are misclassified as incompatible or compatible based on `data_type` alone.
**Why it happens:** PostgreSQL docs explicitly note that `data_type` is only the underlying built-in classification in many cases.
**How to avoid:** Store both normalized type class and native type identity using `udt_*` and domain fields.
**Warning signs:** Drift logic treats multiple distinct PostgreSQL column definitions as the same string or as all `USER-DEFINED`.

### Pitfall 3: MySQL/MariaDB Default Handling Is Overconfident
**What goes wrong:** A target-only column is treated as safely writable because `COLUMN_DEFAULT` was read as `NULL`, even though no usable default exists.
**Why it happens:** MySQL overloads `COLUMN_DEFAULT = NULL`, and MariaDB has its own default-expression nuances.
**How to avoid:** Mark `HasProvenDefault` only when metadata clearly proves it. Otherwise rely on `Nullable` or classify as warning/blocked.
**Warning signs:** Tables with required target-only columns are labeled writable without any explicit default evidence.

### Pitfall 4: Composite FK Order Gets Lost
**What goes wrong:** Dependency edges are created with the right tables but the wrong column mapping order.
**Why it happens:** Composite constraints require `ordinal_position` and referenced-position metadata, not just column names.
**How to avoid:** Always sort and store FK pairs by constraint order.
**Warning signs:** Multi-column FKs render as scrambled pairs or fail round-trip tests.

### Pitfall 5: Persisting Computed Closure Instead Of Intent
**What goes wrong:** Saved profiles become stale or misleading after schema changes because they store auto-added dependent tables as if the operator selected them directly.
**Why it happens:** It feels simpler to persist the entire resolved selection.
**How to avoid:** Persist explicit includes and explicit excludes only. Recompute closure and blocked status at runtime.
**Warning signs:** Editing a profile cannot tell which tables were chosen by the operator versus inferred from dependencies.

### Pitfall 6: MySQL Adapter Assumptions Leak Into MariaDB
**What goes wrong:** A query or field projection works in MySQL 8.4 but fails or misclassifies metadata in MariaDB.
**Why it happens:** MariaDB support currently rides through the same adapter and driver, which makes compatibility issues easy to miss.
**How to avoid:** Keep the common projection narrow, add MariaDB-specific verification, and avoid querying optional MySQL-only metadata unless needed.
**Warning signs:** MariaDB support is declared complete without any MariaDB-specific fixtures or smoke coverage.

## Code Examples

Verified patterns from official sources:

### PostgreSQL Column Discovery Baseline
```sql
SELECT
  table_schema,
  table_name,
  column_name,
  ordinal_position,
  column_default,
  is_nullable,
  data_type,
  udt_schema,
  udt_name,
  is_identity,
  identity_generation,
  is_generated,
  generation_expression
FROM information_schema.columns
WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name, ordinal_position;
```

Source: https://www.postgresql.org/docs/current/infoschema-columns.html

### PostgreSQL FK Fidelity Supplement
```sql
SELECT
  con.conname,
  ns.nspname AS table_schema,
  cls.relname AS table_name,
  refns.nspname AS referenced_table_schema,
  refcls.relname AS referenced_table_name,
  con.confupdtype,
  con.confdeltype,
  con.conkey,
  con.confkey
FROM pg_constraint AS con
JOIN pg_class AS cls ON cls.oid = con.conrelid
JOIN pg_namespace AS ns ON ns.oid = cls.relnamespace
JOIN pg_class AS refcls ON refcls.oid = con.confrelid
JOIN pg_namespace AS refns ON refns.oid = refcls.relnamespace
WHERE con.contype = 'f';
```

Source: https://www.postgresql.org/docs/current/catalog-pg-constraint.html

### MySQL/MariaDB Column Discovery Baseline
```sql
SELECT
  table_schema,
  table_name,
  column_name,
  ordinal_position,
  column_default,
  is_nullable,
  data_type,
  character_maximum_length,
  numeric_precision,
  numeric_scale,
  datetime_precision,
  column_type,
  column_key,
  extra,
  generation_expression
FROM information_schema.columns
WHERE table_schema = DATABASE()
ORDER BY table_schema, table_name, ordinal_position;
```

Source: https://dev.mysql.com/doc/refman/8.4/en/information-schema-columns-table.html and https://mariadb.com/kb/en/information-schema-columns-table/

### FK Closure Input Query
```sql
SELECT
  kcu.constraint_schema,
  kcu.constraint_name,
  kcu.table_schema,
  kcu.table_name,
  kcu.column_name,
  kcu.ordinal_position,
  kcu.position_in_unique_constraint,
  kcu.referenced_table_schema,
  kcu.referenced_table_name,
  kcu.referenced_column_name,
  rc.update_rule,
  rc.delete_rule
FROM information_schema.key_column_usage AS kcu
JOIN information_schema.referential_constraints AS rc
  ON rc.constraint_schema = kcu.constraint_schema
 AND rc.constraint_name = kcu.constraint_name
WHERE kcu.referenced_table_name IS NOT NULL
ORDER BY kcu.table_schema, kcu.table_name, kcu.constraint_name, kcu.ordinal_position;
```

Source: https://dev.mysql.com/doc/refman/8.4/en/information-schema-key-column-usage-table.html and https://dev.mysql.com/doc/refman/8.4/en/information-schema-referential-constraints-table.html

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Compare DDL strings or ORM models | Compare catalog-backed canonical metadata | Current best practice reflected in 2026 official docs | Safer explainability and less engine-specific parsing logic |
| Persist discovery snapshots | Recompute live metadata and persist only intent | Current repo decision D-10 | Avoids stale profile state and schema migration burden |
| Manual table picking without closure | Dependency-aware seed selection plus minimal closure | Required by Phase 2 roadmap | Makes later scan/sync planning deterministic |

**Deprecated/outdated:**
- Treating PostgreSQL `data_type` alone as authoritative type identity: official docs now clearly frame `udt_*` and domain fields as necessary for deeper type handling.
- Treating a missing `COLUMN_DEFAULT` value in MySQL as proof that a target column is safely optional: official docs do not support that inference.

## Open Questions

1. **How much key metadata should Phase 2 store beyond foreign keys?**
   - What we know: FK edges are mandatory now, and later scan/sync phases will need PK/unique awareness.
   - What's unclear: Whether PK/unique metadata should be fully modeled in 02-01 or deferred until scan planning.
   - Recommendation: Include primary-key and unique-constraint identity in the shared snapshot now, even if Phase 2 only consumes FKs directly.

2. **What exact persisted shape should blocked dependency intent use?**
   - What we know: D-09 requires visible blocked exclusions, and D-10 requires persisting durable operator intent only.
   - What's unclear: Whether that should be an exclusion list, a per-table selection state enum, or another minimal structure.
   - Recommendation: Prefer a minimal explicit-exclusion list keyed by canonical table ID unless planner finds a stronger CLI need.

3. **How much MariaDB-specific verification should Phase 2 require?**
   - What we know: MariaDB is in scope, but the repo currently has MySQL integration helpers only.
   - What's unclear: Whether the phase should add a MariaDB container test immediately or accept lower confidence through MySQL-only coverage.
   - Recommendation: Treat MariaDB metadata verification as a Wave 0 or Plan 02-01 gap if strong support confidence is required.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| Go toolchain | Build, unit tests, implementation | ✓ | 1.26.1 | — |
| Docker daemon | Existing testcontainers integration tests | ✗ | CLI/server unavailable | Unit tests only until Docker Desktop or daemon is running |
| PostgreSQL server | Manual metadata smoke tests | ✗ not probed directly | — | Use testcontainers when Docker is available |
| MySQL/MariaDB server | Manual metadata smoke tests | ✗ not probed directly | — | Use testcontainers when Docker is available |

**Missing dependencies with no fallback:**
- None for writing the phase plan itself.

**Missing dependencies with fallback:**
- Docker daemon is unavailable on this machine, so integration-test execution is currently blocked. Planner should still specify the integration coverage, but execution will require Docker or a manually provisioned database environment.

## Recommended Plan Slices

### 02-01: Build Schema Introspection Adapters For Supported Databases
- Extract shared endpoint resolution from `internal/validate/service.go` into a reusable helper or package.
- Define the shared runtime schema model: canonical table ID, table metadata, column metadata, PK/unique metadata, FK metadata.
- Extend PostgreSQL and MySQL/MariaDB adapters with read-only discovery methods that return the shared model.
- Use `information_schema` as the common baseline and add PostgreSQL `pg_constraint` supplementation only where needed for FK fidelity and actions.
- Add unit tests for row-to-model normalization and integration tests against real PostgreSQL/MySQL engines. Add MariaDB verification if feasible.

### 02-02: Build Drift Analysis And Compatibility Classification
- Implement source-vs-target comparison on canonical snapshots keyed by canonical table ID.
- Produce typed reason codes for missing tables, missing columns, incompatible types, target-only nullable/default/generated columns, and skipped source columns.
- Classify each table as `writable`, `writable-with-warning`, or `blocked` with column-level evidence suitable for CLI rendering and later scan reuse.
- Add unit tests covering the requirement matrix for SCMA-02 through SCMA-05.

### 02-03: Build Dependency Graphing And Interactive Table Selection Model
- Build directed FK graph and closure/minimum-required-set logic from adapter metadata.
- Extend the profile model to persist explicit selection intent using canonical table IDs and explicit exclusion intent when needed.
- Add CLI/wizard interaction that lets operators choose seed tables, inspect required additions, keep blocked exclusions visible, and review the saved selection.
- Add unit tests for graph closure and CLI tests for round-tripping selection and blocked states.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` with `go test` |
| Config file | none |
| Quick run command | `go test ./internal/cli ./internal/model ./internal/profile ./internal/validate ./internal/wizard` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SCMA-01 | Discover columns/tables/keys for PostgreSQL and MySQL/MariaDB | integration + unit | `go test -tags=integration ./internal/db/... ./internal/validate/...` | ❌ Wave 0 |
| SCMA-02 | Detect missing, extra, and incompatible columns | unit | `go test ./internal/schema/... -run TestCompareSnapshots` | ❌ Wave 0 |
| SCMA-03 | Classify drift as writable / warning / blocked | unit | `go test ./internal/schema/... -run TestClassifyTableDrift` | ❌ Wave 0 |
| SCMA-04 | Warn on skipped source columns | unit | `go test ./internal/schema/... -run TestSkippedColumnsProduceWarnings` | ❌ Wave 0 |
| SCMA-05 | Recognize nullable/default/generated target columns as satisfiable | unit | `go test ./internal/schema/... -run TestTargetOptionalColumns` | ❌ Wave 0 |
| TABL-01 | Discover FK dependencies for selected tables | integration + unit | `go test ./internal/schema/... -run TestBuildDependencyGraph` | ❌ Wave 0 |
| TABL-02 | Interactive include/exclude flow shows dependency implications | unit | `go test ./internal/cli/... ./internal/wizard/... -run TestDependencySelection` | ❌ Wave 0 |
| TABL-03 | Compute minimum required dependent set | unit | `go test ./internal/schema/... -run TestDependencyClosure` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/...`
- **Per wave merge:** `go test ./...`
- **Phase gate:** `go test ./...` plus integration coverage for metadata queries when Docker or equivalent database environment is available

### Wave 0 Gaps
- [ ] `internal/schema/snapshot_test.go` — shared metadata normalization and canonical ID coverage
- [ ] `internal/schema/compare_test.go` — SCMA-02 through SCMA-05 drift cases
- [ ] `internal/schema/graph_test.go` — TABL-01 through TABL-03 dependency closure and blocked exclusion cases
- [ ] `internal/db/postgres/adapter_integration_test.go` — PostgreSQL discovery query fidelity
- [ ] `internal/db/mysql/adapter_integration_test.go` — MySQL discovery query fidelity
- [ ] `internal/db/mysql/mariadb_integration_test.go` or equivalent smoke path — MariaDB compatibility confidence
- [ ] `internal/cli/table_selection_test.go` — wizard/profile edit round-trip for dependency-aware selection

## Sources

### Primary (HIGH confidence)
- Repository code: `internal/db/postgres/adapter.go`, `internal/db/mysql/adapter.go`, `internal/validate/service.go`, `internal/model/profile.go`, `internal/profile/schema.go`, `internal/wizard/flow.go`, `internal/testkit/containers.go`
- PostgreSQL official docs: https://www.postgresql.org/docs/current/infoschema-columns.html
- PostgreSQL official docs: https://www.postgresql.org/docs/current/infoschema-key-column-usage.html
- PostgreSQL official docs: https://www.postgresql.org/docs/current/catalog-pg-constraint.html
- MySQL 8.4 official docs: https://dev.mysql.com/doc/refman/8.4/en/information-schema-columns-table.html
- MySQL 8.4 official docs: https://dev.mysql.com/doc/refman/8.4/en/information-schema-key-column-usage-table.html
- MySQL 8.4 official docs: https://dev.mysql.com/doc/refman/8.4/en/information-schema-referential-constraints-table.html

### Secondary (MEDIUM confidence)
- MariaDB docs: https://mariadb.com/kb/en/information-schema-columns-table/

### Tertiary (LOW confidence)
- None. No unverified community-only sources were used.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - The recommendation is to reuse the repository's existing pinned libraries and patterns.
- Architecture: HIGH - The current repo already establishes adapter, validation, profile, and wizard seams that Phase 2 should extend.
- Pitfalls: MEDIUM - PostgreSQL and MySQL concerns are strongly sourced; MariaDB-specific pitfalls are less thoroughly verified because no MariaDB runtime was available.

**Research date:** 2026-03-24
**Valid until:** 2026-04-23