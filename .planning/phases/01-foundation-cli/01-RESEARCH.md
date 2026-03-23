# Phase 1: Foundation CLI - Research

**Researched:** 2026-03-23
**Domain:** Go CLI foundation, interactive profile setup, and safe cross-engine connection validation
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### CLI command shape
- **D-01:** V1 should be wizard-first. Operators should have a guided setup entrypoint for creating profiles, with scan and run behavior added as separate commands in later phases.
- **D-02:** The CLI should optimize for human-operated workflows first rather than a flags-only interface.

#### Profile storage and secrets
- **D-03:** Profiles should be stored per user in the OS config directory as YAML files.
- **D-04:** Profiles must not store raw secrets directly.
- **D-05:** Credential handling should prefer DSN templates with placeholders so runtime resolution can pull values from the environment.

#### Interactive setup flow
- **D-06:** The setup experience should optimize for a fast path first rather than a long explicit wizard.
- **D-07:** Editing an existing profile should reopen the same wizard flow with current values prefilled.
- **D-08:** The workflow should still support a review-before-save step even though the general interaction style is fast-path oriented.

#### Connection validation behavior
- **D-09:** Profile save should be blocked until validation succeeds for both source and target connections.
- **D-10:** Validation must confirm source authentication, target authentication, source/target schema metadata access, and a lightweight non-mutating target capability check.
- **D-11:** Validation in Phase 1 should not depend on early table selection.

### the agent's Discretion
- Exact command names and flag aliases within the wizard-first model.
- Exact YAML schema layout, as long as it supports placeholders and later extension.
- Exact wording and visual presentation of prompts, review screens, and validation output.

### Deferred Ideas (OUT OF SCOPE)
- Scan-only behavior and live run command semantics belong to later phases.
- Non-interactive automation-first command design is deferred until later automation scope is planned.
- Alternative secret providers such as OS keychain support may be revisited in a future phase if environment-placeholder references are insufficient.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PROF-01 | Operator can create a saved sync profile that stores source connection settings, target connection settings, selected tables, and sync behavior choices. | Profile schema includes source/target blocks now, plus forward-compatible `selection` and `sync` sections initialized with safe defaults in Phase 1 to avoid a migration in Phase 2/4. |
| PROF-02 | Operator can review and update a saved sync profile through an interactive CLI flow. | Wizard-first command layout, reusable Huh form groups, and edit flow that reopens the same form with prefilled values. |
| PROF-03 | Operator can validate source and target connectivity before a profile is saved. | Adapter-based validation pipeline with authentication, metadata visibility, and non-mutating target capability checks for PostgreSQL and MySQL/MariaDB before any write-to-disk. |
</phase_requirements>

## Summary

Phase 1 should establish a conventional Go CLI shell with Cobra, but the interactive profile experience should not be built as a full-screen TUI yet. The lowest-risk architecture is Cobra for command routing, Huh for the guided setup and edit wizard, and Lip Gloss for review and status formatting. Bubble Tea should stay available as an extension point rather than the primary Phase 1 control loop because the current scope is form-heavy, not dashboard-heavy. Huh already embeds into Bubble Tea later if Phase 6 needs richer live progress screens.

Profile persistence should be explicit and boring: one YAML file per profile in the user config directory, resolved through XDG helpers instead of hand-built platform logic. Profiles should store DSN templates with `${ENV_VAR}` placeholders and never raw credentials. The schema should include future-facing `selection` and `sync` sections now, even if Phase 1 only writes empty selections and conservative defaults, because `PROF-01` requires those concepts to exist and later phases should not need a breaking profile migration.

Validation should be adapter-driven and read-only. For both source and target, validate in three passes: connect and authenticate, confirm metadata visibility through `information_schema`, then run a target capability probe that proves the target is not obviously read-only or a standby without creating objects or mutating data. The planner should split work exactly along the roadmap lines: 01-01 owns command topology and profile model, 01-02 owns the wizard/edit UX, and 01-03 owns adapter validation plus save/load gating.

**Primary recommendation:** Use Cobra + Huh + Lip Gloss + xdg + yaml.v3, persist one YAML profile per user config file with env placeholders only, and gate every save through engine-specific, non-mutating validation adapters.

## Project Constraints (from copilot-instructions.md)

- Use the get-shit-done workflow only because the user explicitly invoked a GSD research task.
- Treat `gsd-*` style work as workflow-driven deliverables that should feed the next planning step.
- Prefer the matching GSD phase flow and keep this document prescriptive so the planner can consume it directly.
- After this deliverable, offer the next workflow step instead of assuming the user wants execution.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.26.1 | Language/runtime | Current stable release on go.dev; matches current ecosystem packages and avoids planning against a missing toolchain. |
| github.com/spf13/cobra | v1.10.2 | Command tree and help/flag handling | Standard Go CLI command framework with clear subcommand boundaries and good long-term ergonomics. |
| github.com/charmbracelet/huh | v1.0.0 | Interactive form flow | Best fit for a wizard-first setup flow; supports accessible mode, dynamic forms, and later Bubble Tea embedding. |
| github.com/charmbracelet/lipgloss | v1.1.0 | Styled review/status output | Gives readable terminal summaries without forcing a full-screen TUI architecture. |
| github.com/adrg/xdg | v0.5.3 | Cross-platform config path resolution | Avoids hand-rolled Windows/macOS/Linux config directory logic. |
| gopkg.in/yaml.v3 | v3.0.1 | YAML encode/decode | Stable Go YAML package; sufficient for explicit schema-controlled persistence. |
| github.com/jackc/pgx/v5 | v5.9.1 | PostgreSQL connectivity and validation | Native PostgreSQL driver with strong Postgres-specific support and direct access to server/session features needed by validation. |
| github.com/go-sql-driver/mysql | v1.9.3 | MySQL/MariaDB connectivity and validation | Standard MySQL driver for Go; also works for MariaDB with engine-specific query handling. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/charmbracelet/bubbletea | v1.3.10 | Full-screen TUI runtime | Add later when scan/live progress becomes a real stateful TUI need; do not make it Phase 1's primary flow. |
| github.com/google/go-cmp | v0.7.0 | Test assertions for structured values | Use in unit tests for profile round-trips and validation reports. |
| github.com/testcontainers/testcontainers-go | v0.41.0 | Disposable DB integration tests | Use for PostgreSQL/MySQL/MariaDB validation tests once Go is installed; Docker is already available locally. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Cobra | kong | Simpler flag modeling, but weaker fit for a future multi-command operator CLI. |
| Huh-first forms | Bubble Tea as the primary app loop | More flexible later, but more control-flow and state-management cost than Phase 1 needs. |
| xdg + yaml.v3 | Viper | Viper adds implicit config/env merging magic that is counterproductive when secrets must stay explicit and placeholder-driven. |
| pgx for PostgreSQL | database/sql + pq-like generic stack | More generic, but loses Postgres-specific validation hooks and modern maintenance posture. |

**Installation:**
```bash
go mod init db-sync
go get github.com/spf13/cobra@v1.10.2 github.com/charmbracelet/huh@v1.0.0 github.com/charmbracelet/lipgloss@v1.1.0 github.com/adrg/xdg@v0.5.3 gopkg.in/yaml.v3@v3.0.1 github.com/jackc/pgx/v5@v5.9.1 github.com/go-sql-driver/mysql@v1.9.3
```

**Version verification:**
- Go 1.26.1 is the current stable release listed on go.dev.
- Cobra v1.10.2 published 2025-12-03.
- Huh v1.0.0 published 2026-02-23.
- Bubble Tea v1.3.10 published 2025-09-17.
- Lip Gloss v1.1.0 published 2025-03-12.
- xdg v0.5.3 published 2024-10-31.
- pgx/v5 v5.9.1 published 2026-03-22.
- go-sql-driver/mysql v1.9.3 published 2025-06-13.
- yaml.v3 v3.0.1 published 2022-05-27.
- go-cmp v0.7.0 published 2025-01-14.
- testcontainers-go v0.41.0 published 2026-03-10.

## Architecture Patterns

### Recommended Project Structure
```text
cmd/db-sync/               # main entrypoint and Cobra bootstrap
internal/cli/              # command wiring and shared terminal output helpers
internal/wizard/           # Huh form definitions and edit/review flow orchestration
internal/profile/          # profile model, validation, load/save, path resolution
internal/secrets/          # placeholder discovery and environment resolution
internal/validate/         # orchestration of source/target validation pipeline
internal/db/postgres/      # PostgreSQL adapter and probes
internal/db/mysql/         # MySQL/MariaDB adapter and probes
internal/model/            # stable domain structs shared across CLI/profile/validation
internal/testkit/          # testcontainers helpers for DB fixtures in later plans
```

### Pattern 1: Wizard-First Command Shell
**What:** Use Cobra as the outer command tree, but make the interactive profile flow the default operator path.
**When to use:** Immediately in Phase 1.
**Example:**
```go
// Source: https://cobra.dev/docs/how-to-guides/working-with-commands/
func NewRootCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db-sync",
		Short: "Create, validate, and later run reusable database sync profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return cmd.Help()
			}
			return app.Wizard.StartNew(cmd.Context())
		},
	}

	cmd.AddCommand(NewProfileCommand(app))
	return cmd
}
```

**Prescriptive guidance:**
- `db-sync` with no args should launch the setup flow only when stdin is interactive.
- `db-sync profile new`, `db-sync profile edit <name>`, `db-sync profile list`, and `db-sync profile validate <name>` should exist explicitly for repeatable operator workflows.
- Do not make Phase 1 depend on a dense flag-only interface.

### Pattern 2: Reusable Form Groups For New And Edit Flows
**What:** Build the wizard as composable Huh groups that can be initialized from an existing profile.
**When to use:** Plan 01-02.
**Example:**
```go
// Source: https://github.com/charmbracelet/huh
form := huh.NewForm(
	huh.NewGroup(
		huh.NewInput().Title("Profile name").Value(&draft.Name),
		huh.NewSelect[string]().Title("Engine").Options(
			huh.NewOption("PostgreSQL", "postgres"),
			huh.NewOption("MySQL", "mysql"),
			huh.NewOption("MariaDB", "mariadb"),
		).Value(&draft.Source.Engine),
	),
)
```

**Prescriptive guidance:**
- Model the wizard around a `ProfileDraft` that can be hydrated from disk for edit mode.
- Keep the fast path short: name, source DSN template, target DSN template, optional labels, review, validate, save.
- Keep the review step as a separate renderer, not more form fields.

### Pattern 3: Explicit Profile Schema With Forward-Compatible Sections
**What:** Persist a stable YAML document with clear top-level blocks for `source`, `target`, `selection`, and `sync`.
**When to use:** Plan 01-01.
**Example:**
```yaml
version: 1
name: customer-copy
source:
  engine: postgres
  dsn_template: postgres://app:${SRC_DB_PASSWORD}@localhost:5432/source?sslmode=disable
target:
  engine: postgres
  dsn_template: postgres://app:${TGT_DB_PASSWORD}@localhost:5432/target?sslmode=disable
selection:
  tables: []
sync:
  mode: insert-missing
  mirror_delete: false
```

**Prescriptive guidance:**
- Store one profile per YAML file named by a slugged profile name.
- Include `selection.tables` and `sync` now, even if Phase 1 only writes defaults.
- Reserve `version` for schema migrations later.
- Do not store validation results, passwords, or resolved DSNs in the profile file.

### Pattern 4: Placeholder Resolution As A Separate Domain Service
**What:** Parse `${ENV_VAR}` placeholders from DSN templates, validate required names, and resolve them only at runtime.
**When to use:** Plans 01-01 and 01-03.
**Example:**
```go
var placeholderPattern = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)

func ResolveTemplate(input string, env map[string]string) (string, []string, error) {
	missing := []string{}
	resolved := placeholderPattern.ReplaceAllStringFunc(input, func(token string) string {
		name := placeholderPattern.FindStringSubmatch(token)[1]
		value, ok := env[name]
		if !ok || value == "" {
			missing = append(missing, name)
			return token
		}
		return value
	})
	if len(missing) > 0 {
		return "", missing, fmt.Errorf("missing required environment variables")
	}
	return resolved, nil, nil
}
```

**Prescriptive guidance:**
- Support exactly `${NAME}` in Phase 1. Do not implement shell-style defaults or nested expansion.
- Validate placeholder names against `^[A-Z][A-Z0-9_]*$`.
- Show referenced variable names in review output; never echo resolved secret values.

### Pattern 5: Engine-Specific Validation Adapters Behind One Pipeline
**What:** Orchestrate validation as a common sequence with engine-specific probes.
**When to use:** Plan 01-03.
**Example:**
```go
type Adapter interface {
	ValidateSource(ctx context.Context, resolvedDSN string) (Report, error)
	ValidateTarget(ctx context.Context, resolvedDSN string) (Report, error)
}

func ValidateProfile(ctx context.Context, profile Profile, resolver Resolver, adapters Registry) (ProfileReport, error) {
	sourceDSN, _, err := resolver.Resolve(profile.Source.DSNTemplate)
	if err != nil {
		return ProfileReport{}, err
	}
	targetDSN, _, err := resolver.Resolve(profile.Target.DSNTemplate)
	if err != nil {
		return ProfileReport{}, err
	}

	sourceReport, err := adapters.For(profile.Source.Engine).ValidateSource(ctx, sourceDSN)
	if err != nil {
		return ProfileReport{}, err
	}
	targetReport, err := adapters.For(profile.Target.Engine).ValidateTarget(ctx, targetDSN)
	if err != nil {
		return ProfileReport{}, err
	}
	return ProfileReport{Source: sourceReport, Target: targetReport}, nil
}
```

**Prescriptive guidance:**
- Keep validation orchestration engine-agnostic and put SQL differences in adapter packages.
- Return structured probe results, not just strings, so the CLI can format success, warning, and blocker states consistently.
- Save only after both reports are successful.

### Anti-Patterns to Avoid
- **Full-screen TUI too early:** Bubble Tea as the primary Phase 1 runtime adds state-management cost before the tool needs dashboards or streaming progress.
- **Implicit env/config merging:** Do not let a config library auto-inject secrets from env into saved structs.
- **Profile schema tied to the wizard UI:** The profile model should be stable domain data; the form layer should map to it, not define it.
- **Generic SQL validation with no engine branches:** PostgreSQL and MySQL/MariaDB expose different read-only and metadata behavior; a single SQL path will either be brittle or misleading.
- **Saving before validation:** This directly violates locked decision D-09 and creates unusable profiles that later phases must clean up.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CLI command parsing | Custom `os.Args` router | Cobra | Help text, subcommands, completions, and future extensibility are already solved. |
| Interactive forms | Manual stdin prompt state machine | Huh | Validation, select/input widgets, accessible mode, and later Bubble Tea integration already exist. |
| Config dir discovery | Platform `if runtime.GOOS` branches | xdg | Windows/macOS/Linux path behavior is subtle and not worth custom logic. |
| PostgreSQL protocol handling | Raw `database/sql` plumbing or custom DSN parsing | pgx | Postgres-specific state and connection semantics are first-class. |
| MySQL/MariaDB DSN parsing | String concatenation and hand parsing | go-sql-driver/mysql `Config` helpers | Avoid quoting, timeout, and parameter edge cases. |
| YAML parsing | Manual map marshaling | yaml.v3 with explicit structs | Predictable schema control and better long-term migrations. |
| DB integration environment | Shell-scripted ad hoc local DBs | testcontainers-go | Reproducible database validation tests across engines. |

**Key insight:** Phase 1 succeeds by being explicit, not clever. The only custom logic worth owning here is profile schema design, placeholder resolution, and validation orchestration.

## Common Pitfalls

### Pitfall 1: Letting The Wizard Define The Domain Model
**What goes wrong:** Form field order becomes the de facto schema, and later phases struggle to add data without rewriting prompts and migrations together.
**Why it happens:** Greenfield CLI projects often start with prompt code before defining domain structs.
**How to avoid:** Define `Profile`, `Connection`, `Selection`, and `SyncOptions` structs first, then map the wizard to them.
**Warning signs:** YAML keys mirror prompt wording, or edit mode requires special-case mapping logic.

### Pitfall 2: Treating DSN Templates As Plain Strings
**What goes wrong:** Secret placeholders are inconsistently resolved, validation logs leak sensitive values, or invalid placeholder names become silent runtime failures.
**Why it happens:** String replacement feels simple until env validation, preview rendering, and error messaging are needed.
**How to avoid:** Centralize parsing and resolution in `internal/secrets`, support only one placeholder format, and test it heavily.
**Warning signs:** Multiple packages call `os.Getenv` directly for DSN assembly.

### Pitfall 3: Using The Same Validation Query For Every Engine
**What goes wrong:** Read-only detection, metadata checks, or capability checks work for one database and give false confidence for another.
**Why it happens:** `information_schema` looks portable, but engine behavior around privilege visibility and read-only state differs.
**How to avoid:** Keep a shared validation contract but separate PostgreSQL and MySQL/MariaDB SQL probes.
**Warning signs:** One adapter package imports both pgx and MySQL driver code paths.

### Pitfall 4: Blocking On Table Selection Too Early
**What goes wrong:** Phase 1 drifts into schema-analysis scope and cannot finish without Phase 2 logic.
**Why it happens:** It is tempting to validate "real" target writeability against selected tables before those tables exist in the profile UX.
**How to avoid:** Keep Phase 1 validation table-agnostic: auth, current database/schema visibility, metadata row visibility, and target read-only/standby checks only.
**Warning signs:** Validation code starts requiring chosen tables or drift analysis.

### Pitfall 5: Saving Profiles With Unverifiable Connections
**What goes wrong:** Operators accumulate broken profiles and later commands must explain whether the file is invalid, the env is missing, or the DB is unreachable.
**Why it happens:** Teams try to preserve draft progress by saving too early.
**How to avoid:** Keep draft state in memory during the wizard and only write YAML after validation succeeds for both endpoints.
**Warning signs:** Profile files appear on disk even when validation failed.

## Code Examples

Verified patterns from official sources:

### XDG-Backed Profile Path Resolution
```go
// Source: https://github.com/adrg/xdg
path, err := xdg.ConfigFile(filepath.Join("db-sync", "profiles", slug+".yaml"))
if err != nil {
	return err
}
```

### Huh Form As The Wizard Primitive
```go
// Source: https://github.com/charmbracelet/huh
err := huh.NewForm(
	huh.NewGroup(
		huh.NewInput().Title("Profile name").Value(&draft.Name),
		huh.NewInput().Title("Source DSN template").Value(&draft.Source.DSNTemplate),
		huh.NewInput().Title("Target DSN template").Value(&draft.Target.DSNTemplate),
	),
).Run()
```

### PostgreSQL Connection Validation Bootstrap
```go
// Source: https://pkg.go.dev/github.com/jackc/pgx/v5
config, err := pgx.ParseConfig(resolvedDSN)
if err != nil {
	return err
}
conn, err := pgx.ConnectConfig(ctx, config)
if err != nil {
	return err
}
defer conn.Close(ctx)
```

### MySQL/MariaDB Validation Bootstrap
```go
// Source: https://pkg.go.dev/github.com/go-sql-driver/mysql
cfg, err := mysql.ParseDSN(resolvedDSN)
if err != nil {
	return err
}
db := sql.OpenDB(mysql.NewConnector(cfg))
defer db.Close()
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Flags-only CLIs for all workflows | Guided interactive setup for operator-first workflows, with explicit commands still available | Current CLI UX practice for operator tools | Better onboarding and fewer invalid configurations for infrequent users. |
| Secrets stored directly in config files | DSN templates with env placeholders | Current security baseline for local operator tools | Reduces accidental secret disclosure and makes profile files safer to share. |
| Generic database/sql abstraction for every engine | Native pgx for PostgreSQL plus MySQL driver for MySQL/MariaDB | Mature current Go DB ecosystem | Keeps validation logic accurate where engines diverge. |
| One monolithic TUI app from day one | Form-first CLI now, Bubble Tea later if runtime dashboards become necessary | Current Charm stack usage | Faster delivery for Phase 1 without blocking future UX polish. |

**Deprecated/outdated:**
- Saving plaintext credentials into reusable profile files: conflicts with the locked security model and creates avoidable exposure.
- Building a full-screen terminal app before scan/live progress exists: introduces architecture debt without Phase 1 user value.

## Open Questions

1. **Should Phase 1 allow DSNs without a selected database/schema?**
   - What we know: Metadata validation is much more meaningful when the connection identifies the target database explicitly.
   - What's unclear: Some operators may expect to provide host credentials first and choose databases later.
   - Recommendation: Require an explicit database in both source and target DSN templates for Phase 1.

2. **How strict should MySQL/MariaDB read-only checks be when the account cannot read every server variable?**
   - What we know: `read_only`, `super_read_only`, and `transaction_read_only` are useful non-mutating indicators, but visibility can vary by server and privilege level.
   - What's unclear: Whether the product should hard-fail on missing variable access or downgrade to a warning.
   - Recommendation: Treat inability to run the capability probe as a blocker for target validation in Phase 1; do not guess writeability.

3. **Should profile edits preserve the original file name when the profile name changes?**
   - What we know: One-file-per-profile is simple, but rename semantics affect discoverability and references.
   - What's unclear: Whether future commands will reference profiles by name only or by file path.
   - Recommendation: Use profile name as the canonical identifier and rename the file on successful save if the slug changes.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Building, testing, and running the CLI | ✗ | — | None; Phase 1 execution is blocked until Go is installed. |
| Docker | Integration tests for PostgreSQL/MySQL/MariaDB validation | ✓ | 29.2.1 | If unavailable later, use manually managed local DB instances. |
| PostgreSQL server | Adapter integration testing | ✗ | — | Use Docker containers via testcontainers-go. |
| MySQL/MariaDB server | Adapter integration testing | ✗ | — | Use Docker containers via testcontainers-go. |

**Missing dependencies with no fallback:**
- Go toolchain on the current machine.

**Missing dependencies with fallback:**
- PostgreSQL/MySQL/MariaDB local instances can be replaced with Docker containers.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` with `go-cmp` for structural comparisons and `testcontainers-go` for DB integration coverage |
| Config file | none — standard `go test` conventions are sufficient |
| Quick run command | `go test ./...` |
| Full suite command | `go test ./... -tags=integration` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PROF-01 | Profile structs round-trip to YAML with forward-compatible `selection` and `sync` defaults and no secret value persistence | unit | `go test ./internal/profile/...` | ❌ Wave 0 |
| PROF-02 | New and edit flows reuse the same draft model and prefill existing values correctly | unit | `go test ./internal/wizard/...` | ❌ Wave 0 |
| PROF-03 | Save is blocked unless both source and target adapter validations pass for PostgreSQL/MySQL/MariaDB | integration | `go test ./internal/validate/... -tags=integration` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./...`
- **Per wave merge:** `go test ./... -tags=integration`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] Install Go 1.26.1 on the machine running this workspace.
- [ ] Create `go.mod` and basic package layout before implementation.
- [ ] Add unit test files for `internal/profile` and `internal/wizard`.
- [ ] Add integration test files for PostgreSQL and MySQL/MariaDB adapters using Docker-backed containers.
- [ ] Add a CI-friendly convention for integration tags so quick runs stay fast.

## Validation Strategy By Engine

### PostgreSQL
**Authentication and connectivity:**
- Connect with `pgx.ParseConfig` and `pgx.ConnectConfig`, then run `Ping`.

**Metadata access:**
- Query `information_schema.tables` and `information_schema.columns` for non-system schemas.
- PostgreSQL documents that `information_schema.columns` only shows columns the current user can access, so a non-empty metadata probe is meaningful.

**Lightweight non-mutating target capability check:**
- Query `show transaction_read_only`.
- Query `select pg_is_in_recovery()`.
- Fail target validation if the session is read-only or the server is in recovery/standby.

**Planner implication:**
- No table-level write probe is needed in Phase 1.
- Keep the capability check database-wide and read-only.

### MySQL
**Authentication and connectivity:**
- Parse DSN with driver helpers, open a connector-backed `database/sql` pool, then `PingContext`.

**Metadata access:**
- Query `information_schema.tables` and `information_schema.columns` scoped to `DATABASE()`.
- Require a selected database in the DSN so metadata access is meaningful.

**Lightweight non-mutating target capability check:**
- Query `@@global.read_only`.
- Query `@@global.super_read_only` when supported.
- Query `@@session.transaction_read_only` when supported.
- Fail target validation if the server/session is read-only or if the probe cannot determine capability.

### MariaDB
**Authentication and connectivity:**
- Use the same Go driver path as MySQL.

**Metadata access:**
- Query `information_schema.tables` and `information_schema.columns` scoped to the current database.

**Lightweight non-mutating target capability check:**
- Query `@@global.read_only`.
- Attempt `@@session.transaction_read_only` if the server supports it.
- If a capability variable is unsupported, treat that as engine-specific and fall back to the remaining documented read-only indicators; if the overall result is ambiguous, block save.

## Sequencing Implications For The Planner

1. **01-01 should finish the domain model first.**
   - Define `Profile`, `Connection`, `Selection`, `SyncOptions`, config-path helpers, and placeholder parsing before any wizard code.
2. **01-02 should build the wizard on top of a `ProfileDraft`.**
   - The same draft object must support new and edit mode.
   - Review rendering belongs here, but disk persistence does not.
3. **01-03 should own all adapter validation and save gating.**
   - Implement engine registries, validation reports, and persistence only after the profile schema is stable.
   - Save/load behavior should land together with validation so D-09 is never violated.

## Sources

### Primary (HIGH confidence)
- https://go.dev/dl/ - current stable Go release availability
- https://cobra.dev/docs/ - Cobra command structure patterns
- https://github.com/charmbracelet/huh - Huh form model, accessible mode, and Bubble Tea embedding
- https://github.com/charmbracelet/lipgloss - terminal styling approach
- https://github.com/adrg/xdg - cross-platform config path helpers
- https://pkg.go.dev/github.com/jackc/pgx/v5 - PostgreSQL driver APIs
- https://pkg.go.dev/github.com/go-sql-driver/mysql - MySQL/MariaDB driver APIs
- https://www.postgresql.org/docs/current/functions-info.html - PostgreSQL session/system info functions
- https://www.postgresql.org/docs/current/infoschema-columns.html - PostgreSQL metadata visibility semantics
- https://www.postgresql.org/docs/current/hot-standby.html - PostgreSQL standby read-only behavior
- https://dev.mysql.com/doc/refman/en/information-schema-columns-table.html - MySQL metadata access
- https://dev.mysql.com/doc/refman/en/server-system-variables.html - MySQL read-only and transaction state variables
- https://mariadb.com/kb/en/information-schema-columns-table/ - MariaDB metadata access

### Secondary (MEDIUM confidence)
- https://proxy.golang.org/ module metadata endpoints for current tagged Go package versions and publish timestamps
- https://pkg.go.dev/github.com/google/go-cmp/cmp - test comparison package behavior

### Tertiary (LOW confidence)
- None.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - current versions were checked against Go module proxy and official package docs.
- Architecture: HIGH - recommendations align with the locked product decisions and the current Go CLI ecosystem.
- Pitfalls: HIGH - directly derived from the locked phase scope and the documented engine differences around metadata and read-only state.

**Research date:** 2026-03-23
**Valid until:** 2026-04-22
