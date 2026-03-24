package postgres

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"

	"github.com/jackc/pgx/v5"
)

type Adapter struct{}

type columnRow struct {
	tableSchema          string
	tableName            string
	columnName           string
	ordinal              int
	defaultSQL           *string
	isNullable           string
	dataType             string
	udtSchema            string
	udtName              string
	isIdentity           string
	isGenerated          string
	generationExpression *string
}

type primaryKeyRow struct {
	tableSchema    string
	tableName      string
	constraintName string
	columnName     string
	ordinal        int
}

type foreignKeyRow struct {
	constraintName        string
	tableSchema           string
	tableName             string
	columnName            string
	ordinal               int
	referencedTableSchema string
	referencedTableName   string
	referencedColumnName  string
	updateRule            string
	deleteRule            string
}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (adapter *Adapter) ValidateSource(ctx context.Context, resolvedDSN string, engine model.Engine) (profile.EndpointValidation, error) {
	return adapter.validate(ctx, resolvedDSN, engine, "source", false)
}

func (adapter *Adapter) ValidateTarget(ctx context.Context, resolvedDSN string, engine model.Engine) (profile.EndpointValidation, error) {
	return adapter.validate(ctx, resolvedDSN, engine, "target", true)
}

func (adapter *Adapter) DiscoverSourceSchema(ctx context.Context, resolvedDSN string, engine model.Engine) (schema.Snapshot, error) {
	return adapter.discover(ctx, resolvedDSN, engine, "source")
}

func (adapter *Adapter) DiscoverTargetSchema(ctx context.Context, resolvedDSN string, engine model.Engine) (schema.Snapshot, error) {
	return adapter.discover(ctx, resolvedDSN, engine, "target")
}

func (adapter *Adapter) validate(ctx context.Context, resolvedDSN string, engine model.Engine, role string, requireWritable bool) (profile.EndpointValidation, error) {
	conn, err := pgx.Connect(ctx, resolvedDSN)
	if err != nil {
		return failed(role, engine, "authentication", err.Error()), err
	}
	defer conn.Close(ctx)

	checks := []profile.CheckResult{{Name: "authentication", Status: profile.StatusPassed, Detail: "connection established"}}
	var tableCount int
	if err := conn.QueryRow(ctx, "select count(*) from information_schema.tables").Scan(&tableCount); err != nil {
		failedValidation := failed(role, engine, "metadata", err.Error())
		failedValidation.Checks = append(checks, profile.CheckResult{Name: "metadata", Status: profile.StatusFailed, Detail: err.Error()})
		return failedValidation, err
	}
	checks = append(checks, profile.CheckResult{Name: "metadata", Status: profile.StatusPassed, Detail: fmt.Sprintf("information_schema visible (%d rows)", tableCount)})
	if requireWritable {
		var readOnly string
		if err := conn.QueryRow(ctx, "show transaction_read_only").Scan(&readOnly); err != nil {
			return failed(role, engine, "target capability", err.Error()), err
		}
		var inRecovery bool
		if err := conn.QueryRow(ctx, "select pg_is_in_recovery()").Scan(&inRecovery); err != nil {
			return failed(role, engine, "target capability", err.Error()), err
		}
		if readOnly == "on" || inRecovery {
			validation := failed(role, engine, "target capability", "target is read-only or in recovery")
			validation.Checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusFailed, Detail: "target is read-only or in recovery"})
			return validation, fmt.Errorf("target is read-only or in recovery")
		}
		checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusPassed, Detail: "target accepts non-mutating probe"})
	}
	return profile.EndpointValidation{Role: role, Engine: engine, Status: profile.StatusPassed, Checks: checks}, nil
}

func failed(role string, engine model.Engine, name string, detail string) profile.EndpointValidation {
	return profile.EndpointValidation{Role: role, Engine: engine, Status: profile.StatusFailed, Checks: []profile.CheckResult{{Name: name, Status: profile.StatusFailed, Detail: detail}}, Message: detail}
}

func (adapter *Adapter) discover(ctx context.Context, resolvedDSN string, engine model.Engine, role string) (schema.Snapshot, error) {
	conn, err := pgx.Connect(ctx, resolvedDSN)
	if err != nil {
		return schema.Snapshot{}, schema.NewBlockedError(role, engine, "schema discovery could not connect", []string{"verify endpoint credentials and network reachability"}, err)
	}
	defer conn.Close(ctx)

	visibleTables, err := adapter.visibleTableCount(ctx, conn)
	if err != nil {
		return schema.Snapshot{}, metadataBlocked(role, engine, err)
	}
	totalTables, err := adapter.totalTableCount(ctx, conn)
	if err != nil {
		return schema.Snapshot{}, metadataBlocked(role, engine, err)
	}
	if totalTables > visibleTables {
		return schema.Snapshot{}, schema.NewBlockedError(role, engine, "metadata visibility is incomplete", []string{"grant SELECT access on application tables", "grant catalog visibility for non-system schemas"}, fmt.Errorf("visible tables %d of %d", visibleTables, totalTables))
	}

	tables := map[string]*schema.Table{}
	if err := adapter.loadColumns(ctx, conn, tables); err != nil {
		return schema.Snapshot{}, metadataBlocked(role, engine, err)
	}
	if err := adapter.loadPrimaryKeys(ctx, conn, tables); err != nil {
		return schema.Snapshot{}, metadataBlocked(role, engine, err)
	}
	if err := adapter.loadForeignKeys(ctx, conn, tables); err != nil {
		return schema.Snapshot{}, metadataBlocked(role, engine, err)
	}
	return schema.NormalizeSnapshot(schema.Snapshot{Role: role, Engine: engine, Tables: flattenTables(tables)}), nil
}

func (adapter *Adapter) visibleTableCount(ctx context.Context, conn *pgx.Conn) (int, error) {
	var count int
	err := conn.QueryRow(ctx, `
		select count(*)
		from information_schema.tables
		where table_schema not in ('pg_catalog', 'information_schema')
		  and table_type = 'BASE TABLE'
	`).Scan(&count)
	return count, err
}

func (adapter *Adapter) totalTableCount(ctx context.Context, conn *pgx.Conn) (int, error) {
	var count int
	err := conn.QueryRow(ctx, `
		select count(*)
		from pg_class cls
		join pg_namespace ns on ns.oid = cls.relnamespace
		where cls.relkind in ('r', 'p')
		  and ns.nspname not in ('pg_catalog', 'information_schema')
	`).Scan(&count)
	return count, err
}

func (adapter *Adapter) loadColumns(ctx context.Context, conn *pgx.Conn, tables map[string]*schema.Table) error {
	rows, err := conn.Query(ctx, `
		select
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
		from information_schema.columns
		where table_schema not in ('pg_catalog', 'information_schema')
		order by table_schema, table_name, ordinal_position
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row columnRow
		if err := rows.Scan(&row.tableSchema, &row.tableName, &row.columnName, &row.ordinal, &row.defaultSQL, &row.isNullable, &row.dataType, &row.udtSchema, &row.udtName, &row.isIdentity, &row.isGenerated, &row.generationExpression); err != nil {
			return err
		}
		table := ensureTable(tables, schema.TableID{Schema: row.tableSchema, Name: row.tableName})
		defaultSQL := ""
		if row.defaultSQL != nil {
			defaultSQL = *row.defaultSQL
		}
		table.Columns = append(table.Columns, schema.Column{
			Name:             row.columnName,
			Ordinal:          row.ordinal,
			DataType:         row.dataType,
			NativeType:       row.udtSchema + "." + row.udtName,
			Nullable:         strings.EqualFold(row.isNullable, "YES"),
			DefaultSQL:       defaultSQL,
			HasProvenDefault: strings.TrimSpace(defaultSQL) != "",
			Identity:         strings.EqualFold(row.isIdentity, "YES"),
			Generated:        !strings.EqualFold(row.isGenerated, "NEVER"),
			Writable:         !strings.EqualFold(row.isGenerated, "ALWAYS"),
		})
	}
	return rows.Err()
}

func (adapter *Adapter) loadPrimaryKeys(ctx context.Context, conn *pgx.Conn, tables map[string]*schema.Table) error {
	rows, err := conn.Query(ctx, `
		select
			tc.table_schema,
			tc.table_name,
			tc.constraint_name,
			kcu.column_name,
			kcu.ordinal_position
		from information_schema.table_constraints tc
		join information_schema.key_column_usage kcu
		  on tc.constraint_schema = kcu.constraint_schema
		 and tc.constraint_name = kcu.constraint_name
		 and tc.table_schema = kcu.table_schema
		 and tc.table_name = kcu.table_name
		where tc.constraint_type = 'PRIMARY KEY'
		  and tc.table_schema not in ('pg_catalog', 'information_schema')
		order by tc.table_schema, tc.table_name, tc.constraint_name, kcu.ordinal_position
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row primaryKeyRow
		if err := rows.Scan(&row.tableSchema, &row.tableName, &row.constraintName, &row.columnName, &row.ordinal); err != nil {
			return err
		}
		table := ensureTable(tables, schema.TableID{Schema: row.tableSchema, Name: row.tableName})
		table.PrimaryKey.Name = row.constraintName
		table.PrimaryKey.Columns = append(table.PrimaryKey.Columns, row.columnName)
	}
	return rows.Err()
}

func (adapter *Adapter) loadForeignKeys(ctx context.Context, conn *pgx.Conn, tables map[string]*schema.Table) error {
	rows, err := conn.Query(ctx, `
		select
			con.conname,
			ns.nspname as table_schema,
			cls.relname as table_name,
			att2.attname as column_name,
			pos.n as ordinal_position,
			refns.nspname as referenced_table_schema,
			refcls.relname as referenced_table_name,
			att.attname as referenced_column_name,
			case con.confupdtype
				when 'a' then 'NO ACTION'
				when 'r' then 'RESTRICT'
				when 'c' then 'CASCADE'
				when 'n' then 'SET NULL'
				when 'd' then 'SET DEFAULT'
				else 'UNKNOWN'
			end as update_rule,
			case con.confdeltype
				when 'a' then 'NO ACTION'
				when 'r' then 'RESTRICT'
				when 'c' then 'CASCADE'
				when 'n' then 'SET NULL'
				when 'd' then 'SET DEFAULT'
				else 'UNKNOWN'
			end as delete_rule
		from pg_constraint con
		join pg_class cls on cls.oid = con.conrelid
		join pg_namespace ns on ns.oid = cls.relnamespace
		join pg_class refcls on refcls.oid = con.confrelid
		join pg_namespace refns on refns.oid = refcls.relnamespace
		join lateral generate_subscripts(con.conkey, 1) as pos(n) on true
		join pg_attribute att2 on att2.attrelid = con.conrelid and att2.attnum = con.conkey[pos.n]
		join pg_attribute att on att.attrelid = con.confrelid and att.attnum = con.confkey[pos.n]
		where con.contype = 'f'
		  and ns.nspname not in ('pg_catalog', 'information_schema')
		order by ns.nspname, cls.relname, con.conname, pos.n
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row foreignKeyRow
		if err := rows.Scan(&row.constraintName, &row.tableSchema, &row.tableName, &row.columnName, &row.ordinal, &row.referencedTableSchema, &row.referencedTableName, &row.referencedColumnName, &row.updateRule, &row.deleteRule); err != nil {
			return err
		}
		table := ensureTable(tables, schema.TableID{Schema: row.tableSchema, Name: row.tableName})
		foreignKey := findOrAppendForeignKey(table, row.constraintName, schema.TableID{Schema: row.referencedTableSchema, Name: row.referencedTableName}, row.updateRule, row.deleteRule)
		foreignKey.Columns = append(foreignKey.Columns, row.columnName)
		foreignKey.ReferencedColumns = append(foreignKey.ReferencedColumns, row.referencedColumnName)
	}
	return rows.Err()
}

func ensureTable(tables map[string]*schema.Table, id schema.TableID) *schema.Table {
	key := id.String()
	table, ok := tables[key]
	if !ok {
		table = &schema.Table{ID: id}
		tables[key] = table
	}
	return table
}

func flattenTables(tables map[string]*schema.Table) []schema.Table {
	keys := make([]string, 0, len(tables))
	for key := range tables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]schema.Table, 0, len(keys))
	for _, key := range keys {
		result = append(result, *tables[key])
	}
	return result
}

func findOrAppendForeignKey(table *schema.Table, name string, referencedTable schema.TableID, updateRule string, deleteRule string) *schema.ForeignKey {
	for index := range table.ForeignKeys {
		if table.ForeignKeys[index].Name == name {
			return &table.ForeignKeys[index]
		}
	}
	table.ForeignKeys = append(table.ForeignKeys, schema.ForeignKey{Name: name, ReferencedTable: referencedTable, UpdateRule: updateRule, DeleteRule: deleteRule})
	return &table.ForeignKeys[len(table.ForeignKeys)-1]
}

func metadataBlocked(role string, engine model.Engine, err error) error {
	return schema.NewBlockedError(role, engine, "metadata visibility is incomplete", []string{"grant metadata access for information_schema and pg_constraint", "ensure the endpoint can read non-system schemas without partial catalog filtering"}, err)
}
