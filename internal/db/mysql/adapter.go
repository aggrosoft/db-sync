package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"

	_ "github.com/go-sql-driver/mysql"
)

type Adapter struct{}

type columnRow struct {
	tableSchema          string
	tableName            string
	columnName           string
	ordinal              int
	defaultSQL           sql.NullString
	isNullable           string
	dataType             string
	columnType           string
	columnKey            string
	extra                string
	generationExpression sql.NullString
}

type primaryKeyRow struct {
	tableSchema    string
	tableName      string
	constraintName string
	columnName     string
	ordinal        int
}

type foreignKeyRow struct {
	constraintSchema      string
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
	db, err := sql.Open("mysql", resolvedDSN)
	if err != nil {
		return failed(role, engine, "authentication", err.Error()), err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return failed(role, engine, "authentication", err.Error()), err
	}
	checks := []profile.CheckResult{{Name: "authentication", Status: profile.StatusPassed, Detail: "connection established"}}
	var tableCount int
	if err := db.QueryRowContext(ctx, "select count(*) from information_schema.tables").Scan(&tableCount); err != nil {
		validation := failed(role, engine, "metadata", err.Error())
		validation.Checks = append(checks, profile.CheckResult{Name: "metadata", Status: profile.StatusFailed, Detail: err.Error()})
		return validation, err
	}
	checks = append(checks, profile.CheckResult{Name: "metadata", Status: profile.StatusPassed, Detail: fmt.Sprintf("information_schema visible (%d rows)", tableCount)})
	if requireWritable {
		var readOnlyValue any
		if err := db.QueryRowContext(ctx, "select @@global.read_only").Scan(&readOnlyValue); err != nil {
			validation := failed(role, engine, "target capability", err.Error())
			validation.Checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusFailed, Detail: err.Error()})
			return validation, err
		}
		readOnly, err := parseReadOnlyValue(readOnlyValue)
		if err != nil {
			validation := failed(role, engine, "target capability", err.Error())
			validation.Checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusFailed, Detail: err.Error()})
			return validation, err
		}
		if readOnly {
			validation := failed(role, engine, "target capability", "target is read-only")
			validation.Checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusFailed, Detail: "target is read-only"})
			return validation, fmt.Errorf("target is read-only")
		}
		checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusPassed, Detail: "target accepts non-mutating probe"})
	}
	return profile.EndpointValidation{Role: role, Engine: engine, Status: profile.StatusPassed, Checks: checks}, nil
}

func parseReadOnlyValue(value any) (bool, error) {
	switch typed := value.(type) {
	case nil:
		return false, fmt.Errorf("read_only probe returned no value")
	case []byte:
		return parseReadOnlyString(string(typed))
	case sql.RawBytes:
		return parseReadOnlyString(string(typed))
	default:
		return parseReadOnlyString(fmt.Sprint(typed))
	}
}

func parseReadOnlyString(value string) (bool, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "1", "on", "true", "yes":
		return true, nil
	case "0", "off", "false", "no":
		return false, nil
	default:
		return false, fmt.Errorf("unrecognized read_only value %q", value)
	}
}

func failed(role string, engine model.Engine, name string, detail string) profile.EndpointValidation {
	return profile.EndpointValidation{Role: role, Engine: engine, Status: profile.StatusFailed, Checks: []profile.CheckResult{{Name: name, Status: profile.StatusFailed, Detail: detail}}, Message: detail}
}

func (adapter *Adapter) discover(ctx context.Context, resolvedDSN string, engine model.Engine, role string) (schema.Snapshot, error) {
	db, err := sql.Open("mysql", resolvedDSN)
	if err != nil {
		return schema.Snapshot{}, schema.NewBlockedError(role, engine, "schema discovery could not connect", []string{"verify endpoint credentials and network reachability"}, err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return schema.Snapshot{}, schema.NewBlockedError(role, engine, "schema discovery could not connect", []string{"verify endpoint credentials and network reachability"}, err)
	}
	var databaseName string
	if err := db.QueryRowContext(ctx, "select database()").Scan(&databaseName); err != nil {
		return schema.Snapshot{}, mysqlMetadataBlocked(role, engine, err)
	}
	tables := map[string]*schema.Table{}
	if err := adapter.loadColumns(ctx, db, databaseName, tables); err != nil {
		return schema.Snapshot{}, mysqlMetadataBlocked(role, engine, err)
	}
	if err := adapter.loadPrimaryKeys(ctx, db, databaseName, tables); err != nil {
		return schema.Snapshot{}, mysqlMetadataBlocked(role, engine, err)
	}
	if err := adapter.loadForeignKeys(ctx, db, databaseName, tables); err != nil {
		return schema.Snapshot{}, mysqlMetadataBlocked(role, engine, err)
	}
	return schema.NormalizeSnapshot(schema.Snapshot{Role: role, Engine: engine, Tables: flattenTables(tables)}), nil
}

func (adapter *Adapter) loadColumns(ctx context.Context, db *sql.DB, databaseName string, tables map[string]*schema.Table) error {
	rows, err := db.QueryContext(ctx, `
		select
			table_schema,
			table_name,
			column_name,
			ordinal_position,
			column_default,
			is_nullable,
			data_type,
			column_type,
			column_key,
			extra,
			generation_expression
		from information_schema.columns
		where table_schema = ?
		order by table_schema, table_name, ordinal_position
	`, databaseName)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row columnRow
		if err := rows.Scan(&row.tableSchema, &row.tableName, &row.columnName, &row.ordinal, &row.defaultSQL, &row.isNullable, &row.dataType, &row.columnType, &row.columnKey, &row.extra, &row.generationExpression); err != nil {
			return err
		}
		table := ensureTable(tables, mysqlTableID(row.tableName))
		defaultSQL := ""
		if row.defaultSQL.Valid {
			defaultSQL = row.defaultSQL.String
		}
		table.Columns = append(table.Columns, schema.Column{
			Name:             row.columnName,
			Ordinal:          row.ordinal,
			DataType:         row.dataType,
			NativeType:       row.columnType,
			Nullable:         strings.EqualFold(row.isNullable, "YES"),
			DefaultSQL:       defaultSQL,
			HasProvenDefault: row.defaultSQL.Valid && strings.TrimSpace(row.defaultSQL.String) != "",
			Identity:         strings.Contains(strings.ToLower(row.extra), "auto_increment"),
			Generated:        strings.Contains(strings.ToLower(row.extra), "generated") || (row.generationExpression.Valid && strings.TrimSpace(row.generationExpression.String) != ""),
			Writable:         !strings.Contains(strings.ToLower(row.extra), "generated") || strings.Contains(strings.ToLower(row.extra), "default_generated"),
		})
	}
	return rows.Err()
}

func (adapter *Adapter) loadPrimaryKeys(ctx context.Context, db *sql.DB, databaseName string, tables map[string]*schema.Table) error {
	rows, err := db.QueryContext(ctx, `
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
		  and tc.table_schema = ?
		order by tc.table_schema, tc.table_name, tc.constraint_name, kcu.ordinal_position
	`, databaseName)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row primaryKeyRow
		if err := rows.Scan(&row.tableSchema, &row.tableName, &row.constraintName, &row.columnName, &row.ordinal); err != nil {
			return err
		}
		table := ensureTable(tables, mysqlTableID(row.tableName))
		table.PrimaryKey.Name = row.constraintName
		table.PrimaryKey.Columns = append(table.PrimaryKey.Columns, row.columnName)
	}
	return rows.Err()
}

func (adapter *Adapter) loadForeignKeys(ctx context.Context, db *sql.DB, databaseName string, tables map[string]*schema.Table) error {
	rows, err := db.QueryContext(ctx, `
		select
			kcu.constraint_schema,
			kcu.constraint_name,
			kcu.table_schema,
			kcu.table_name,
			kcu.column_name,
			kcu.ordinal_position,
			kcu.referenced_table_schema,
			kcu.referenced_table_name,
			kcu.referenced_column_name,
			rc.update_rule,
			rc.delete_rule
		from information_schema.key_column_usage kcu
		join information_schema.referential_constraints rc
		  on rc.constraint_schema = kcu.constraint_schema
		 and rc.constraint_name = kcu.constraint_name
		where kcu.table_schema = ?
		  and kcu.referenced_table_name is not null
		order by kcu.table_schema, kcu.table_name, kcu.constraint_name, kcu.ordinal_position
	`, databaseName)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row foreignKeyRow
		if err := rows.Scan(&row.constraintSchema, &row.constraintName, &row.tableSchema, &row.tableName, &row.columnName, &row.ordinal, &row.referencedTableSchema, &row.referencedTableName, &row.referencedColumnName, &row.updateRule, &row.deleteRule); err != nil {
			return err
		}
		table := ensureTable(tables, mysqlTableID(row.tableName))
		foreignKey := findOrAppendForeignKey(table, row.constraintName, mysqlTableID(row.referencedTableName), row.updateRule, row.deleteRule)
		foreignKey.Columns = append(foreignKey.Columns, row.columnName)
		foreignKey.ReferencedColumns = append(foreignKey.ReferencedColumns, row.referencedColumnName)
	}
	return rows.Err()
}

func mysqlTableID(name string) schema.TableID {
	return schema.TableID{Name: name}
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

func mysqlMetadataBlocked(role string, engine model.Engine, err error) error {
	return schema.NewBlockedError(role, engine, "metadata visibility is incomplete", []string{"grant metadata access for information_schema.columns and referential_constraints", "ensure the endpoint can read the selected database catalog without partial filtering"}, err)
}
