# db-sync

Small CLI tool for analyzing and synchronizing selected tables from a source database into a target database.

Currently supported:

- schema analysis without write access
- insert-missing for rows that are missing in the target
- upsert for existing rows based on the primary key
- optional mirror-delete for target rows that no longer exist in the source
- MySQL, MariaDB, and PostgreSQL endpoints

## Quick Start

### 1. Start local example databases

```bash
docker compose up -d
```

The Compose setup starts:

- source on port 3306
- target on port 3307
- Adminer on port 8080

See also [docker-compose.yaml](docker-compose.yaml).

### 2. Create `.env`

Use [.env.example](.env.example) as a starting point.

Important variables:

```env
DB_SYNC_SOURCE_HOST=localhost
DB_SYNC_SOURCE_PORT=3306
DB_SYNC_SOURCE_USER=dev
DB_SYNC_SOURCE_PASSWORD=dev
DB_SYNC_SOURCE_DB=db
DB_SYNC_SOURCE_ENGINE=mariadb

DB_SYNC_TARGET_HOST=localhost
DB_SYNC_TARGET_PORT=3307
DB_SYNC_TARGET_USER=dev
DB_SYNC_TARGET_PASSWORD=dev
DB_SYNC_TARGET_DB=db
DB_SYNC_TARGET_ENGINE=mariadb

DB_SYNC_TABLES=customer,customer_address,order,order_line_item
DB_SYNC_EXCLUDE_TABLES=app,integration,plugin
DB_SYNC_MIRROR_DELETE=false
DB_SYNC_MERGE_TABLES=
```

## CLI Usage

Without building:

```bash
go run ./cmd/db-sync analyze --env-file .env
go run ./cmd/db-sync run --dry-run --env-file .env
go run ./cmd/db-sync run --env-file .env
```

With a built binary:

```bash
go build -o db-sync ./cmd/db-sync
./db-sync analyze --env-file .env
./db-sync run --dry-run --env-file .env
./db-sync version
```

## Releases

Pushing a tag matching `v*` triggers the release workflow in GitHub Actions.
It builds release artifacts for Linux, macOS, and Windows and attaches them directly to the GitHub release for that tag.

## Commands

### `db-sync analyze`

Validates the configuration and does not write anything to the target.

The analysis includes:

- loading configuration from environment variables
- connection and metadata validation for source and target
- schema discovery for the selected tables
- automatic inclusion of required dependencies
- separate reporting for explicitly selected and implicitly required tables
- drift report for the final synchronized tables

If blockers are found, `analyze` exits with an error.

### `db-sync run`

Executes the synchronization.

Options:

- `--dry-run`: counts planned changes but does not write anything to the target

`run` uses the same preflight checks as `analyze`, but on success it only prints the sync report. With `--dry-run`, that report contains the planned inserts, updates, and deletes.

## Sync Semantics

- Explicitly selected tables from `DB_SYNC_TABLES`: source-authoritative by default; rows missing from source are removed, missing target rows are inserted, and changed rows are updated
- Implicitly required relation tables: insert missing rows only, no updates
- `DB_SYNC_EXCLUDE_TABLES`: excludes tables hard, even if they would otherwise be pulled in as implicit dependencies

For MySQL and MariaDB, `run` always disables foreign key checks temporarily on the pinned target session during the write phase. This is necessary because insert order alone cannot reliably resolve cyclic or mutually nested references between tables. Re-enabling foreign key checks does not retroactively validate rows that were already written.

### `DB_SYNC_MIRROR_DELETE=true`

- Deletes target rows that no longer exist in the source only for explicit merge tables
- Does not delete implicitly required relation tables
- Runs deletions before inserts and updates, in reverse dependency order for explicit merge tables, to reduce foreign key and unique key conflicts

### `DB_SYNC_MERGE_TABLES=table_a,table_b`

- Applies only to explicitly selected tables
- Opts the configured tables out of source-authoritative replace mode and back into merge semantics
- Keeps target-only rows unless `DB_SYNC_MIRROR_DELETE=true`
- Updates existing target rows when non-primary-key columns differ and inserts missing rows from source

## Current Limitations

- Sync expects primary keys on synchronized tables
- Comparison and updates are currently row-oriented, not chunked
- Upsert currently relies on primary keys, not separate unique keys
- There are no rollback artifacts or checkpoints yet

## Development

Relevant commands:

```bash
go test ./...
go test -tags integration ./internal/sync -run TestRunProfileIntegrationUpsertAndMirrorDelete -v
```