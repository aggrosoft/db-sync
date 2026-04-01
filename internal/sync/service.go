package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"db-sync/internal/model"
	"db-sync/internal/schema"
	"db-sync/internal/validate"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Analysis interface {
	SelectionPreview() schema.SelectionPreview
	DiscoveryReport() schema.DiscoveryReport
	DriftReport() schema.DriftReport
}

type Report struct {
	DryRun       bool
	Tables       []TableReport
	MissingRows  int
	InsertedRows int
	UpdatedRows  int
	DeletedRows  int
	Summary      string
}

type ProgressUpdate struct {
	Completed int
	Total     int
	TableID   schema.TableID
	Scope     string
	DryRun    bool
}

type TableReport struct {
	TableID      schema.TableID
	Scope        string
	SourceRows   int
	MissingRows  int
	InsertedRows int
	UpdatedRows  int
	DeletedRows  int
}

type Service struct {
	envProvider func() map[string]string
}

type sqlExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type dialect interface {
	quote(identifier string) string
	placeholder(position int) string
}

type mysqlDialect struct{}
type postgresDialect struct{}

type tableState struct {
	report        TableReport
	table         schema.Table
	columns       []schema.Column
	primaryKey    []string
	sourceSeen    map[string]struct{}
	targetRows    map[string][]any
	targetDialect dialect
}

type syncActionType string

const (
	syncActionInsert syncActionType = "insert"
	syncActionUpdate syncActionType = "update"
)

type syncAction struct {
	typ          syncActionType
	values       []any
	key          string
	targetValues []any
}

func NewService(envProvider func() map[string]string) *Service {
	return &Service{envProvider: envProvider}
}

func (service *Service) RunProfile(ctx context.Context, candidate model.Profile, analysis interface {
	SelectionPreview() schema.SelectionPreview
	DiscoveryReport() schema.DiscoveryReport
	DriftReport() schema.DriftReport
}, dryRun bool, progress func(ProgressUpdate)) (Report, error) {
	preview := analysis.SelectionPreview()
	drift := analysis.DriftReport()
	if len(drift.Blockers) > 0 {
		return Report{}, errors.New("schema drift blocks sync for one or more selected tables")
	}
	discovery := analysis.DiscoveryReport()
	env := service.envProvider()
	sourceDSN, _, err := validate.ResolveEndpoint(candidate.Source, env)
	if err != nil {
		return Report{}, err
	}
	targetDSN, _, err := validate.ResolveEndpoint(candidate.Target, env)
	if err != nil {
		return Report{}, err
	}
	sourceDB, sourceDialect, err := openDatabase(candidate.Source.Engine, sourceDSN)
	if err != nil {
		return Report{}, err
	}
	defer sourceDB.Close()
	targetDB, targetDialect, err := openDatabase(candidate.Target.Engine, targetDSN)
	if err != nil {
		return Report{}, err
	}
	defer targetDB.Close()
	targetExecutor := sqlExecutor(targetDB)
	if !dryRun {
		switch candidate.Target.Engine {
		case model.EngineMySQL, model.EngineMariaDB:
			targetConn, err := targetDB.Conn(ctx)
			if err != nil {
				return Report{}, fmt.Errorf("pin target connection for foreign key checks: %w", err)
			}
			if _, err := targetConn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
				_ = targetConn.Close()
				return Report{}, fmt.Errorf("disable foreign key checks on target: %w", err)
			}
			defer func() {
				_, _ = targetConn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 1")
				_ = targetConn.Close()
			}()
			targetExecutor = targetConn
		}
	}

	report := Report{DryRun: dryRun, Tables: make([]TableReport, 0, len(preview.FinalTables))}
	explicitSet := tableSet(preview.ExplicitIncludes)
	deleteStates := make(map[schema.TableID]tableState, len(preview.ExplicitIncludes))
	if candidate.Sync.MirrorDelete {
		for _, tableID := range preview.FinalTables {
			if _, ok := explicitSet[tableID]; !ok {
				continue
			}
			sourceTable, ok := discovery.Source.Snapshot.TableByID(tableID)
			if !ok {
				return report, fmt.Errorf("selected source table %s is not available", tableID.String())
			}
			targetTable, ok := discovery.Target.Snapshot.TableByID(tableID)
			if !ok {
				return report, fmt.Errorf("selected target table %s is not available", tableID.String())
			}
			state, err := inspectTableForDelete(ctx, sourceDB, sourceDialect, sourceTable, targetExecutor, targetDialect, targetTable)
			if err != nil {
				return report, err
			}
			deleteStates[tableID] = state
		}
		if !dryRun {
			for index := len(preview.FinalTables) - 1; index >= 0; index-- {
				tableID := preview.FinalTables[index]
				state, ok := deleteStates[tableID]
				if !ok {
					continue
				}
				if _, err := deleteMissingRows(ctx, targetExecutor, state, false); err != nil {
					return report, err
				}
			}
		}
	}
	states := make([]tableState, 0, len(preview.FinalTables))
	totalTables := len(preview.FinalTables)
	enforceSelfReferenceOrdering := shouldEnforceSelfReferenceOrdering(dryRun, candidate.Target.Engine)
	for _, tableID := range preview.FinalTables {
		sourceTable, ok := discovery.Source.Snapshot.TableByID(tableID)
		if !ok {
			return report, fmt.Errorf("selected source table %s is not available", tableID.String())
		}
		targetTable, ok := discovery.Target.Snapshot.TableByID(tableID)
		if !ok {
			return report, fmt.Errorf("selected target table %s is not available", tableID.String())
		}
		scope := "implicit"
		allowUpdate := false
		if _, ok := explicitSet[tableID]; ok {
			scope = "explicit"
			allowUpdate = true
		}
		state, err := syncTable(ctx, sourceDB, sourceDialect, sourceTable, targetExecutor, targetDialect, targetTable, allowUpdate, scope, dryRun, enforceSelfReferenceOrdering)
		if err != nil {
			return report, err
		}
		if deleteState, ok := deleteStates[tableID]; ok {
			state.report.DeletedRows = countDeleteCandidates(deleteState)
		}
		states = append(states, state)
		if progress != nil {
			progress(ProgressUpdate{Completed: len(states), Total: totalTables, TableID: tableID, Scope: scope, DryRun: dryRun})
		}
	}
	for _, state := range states {
		report.Tables = append(report.Tables, state.report)
		report.MissingRows += state.report.MissingRows
		report.InsertedRows += state.report.InsertedRows
		report.UpdatedRows += state.report.UpdatedRows
		report.DeletedRows += state.report.DeletedRows
	}
	mode := "executed"
	if dryRun {
		mode = "dry-run completed"
	}
	report.Summary = fmt.Sprintf("Sync %s for %d table(s).", mode, len(report.Tables))
	return report, nil
}

func inspectTableForDelete(ctx context.Context, sourceDB *sql.DB, sourceDialect dialect, sourceTable schema.Table, targetDB sqlExecutor, targetDialect dialect, targetTable schema.Table) (tableState, error) {
	if len(sourceTable.PrimaryKey.Columns) == 0 {
		return tableState{}, fmt.Errorf("table %s has no primary key; sync requires primary keys", sourceTable.ID.String())
	}
	columns, err := sharedWritableColumns(sourceTable, targetTable)
	if err != nil {
		return tableState{}, err
	}
	targetRows, err := loadTargetRows(ctx, targetDB, targetDialect, targetTable, columns)
	if err != nil {
		return tableState{}, err
	}
	rows, err := sourceDB.QueryContext(ctx, buildSelectQuery(sourceDialect, sourceTable.ID, columns, sourceTable.PrimaryKey.Columns))
	if err != nil {
		return tableState{}, fmt.Errorf("query source table %s: %w", sourceTable.ID.String(), err)
	}
	defer rows.Close()

	state := tableState{
		report:        TableReport{TableID: sourceTable.ID, Scope: "explicit"},
		table:         sourceTable,
		columns:       columns,
		primaryKey:    append([]string(nil), sourceTable.PrimaryKey.Columns...),
		sourceSeen:    map[string]struct{}{},
		targetRows:    targetRows,
		targetDialect: targetDialect,
	}
	for rows.Next() {
		values, err := scanRowValues(rows, columns)
		if err != nil {
			return tableState{}, fmt.Errorf("scan source row for %s: %w", sourceTable.ID.String(), err)
		}
		key, err := encodePrimaryKey(values, columns, sourceTable.PrimaryKey.Columns)
		if err != nil {
			return tableState{}, fmt.Errorf("encode primary key for %s: %w", sourceTable.ID.String(), err)
		}
		state.sourceSeen[key] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return tableState{}, fmt.Errorf("iterate source rows for %s: %w", sourceTable.ID.String(), err)
	}
	return state, nil
}

func countDeleteCandidates(state tableState) int {
	count := 0
	for key := range state.targetRows {
		if _, ok := state.sourceSeen[key]; ok {
			continue
		}
		count++
	}
	return count
}

func syncTable(ctx context.Context, sourceDB *sql.DB, sourceDialect dialect, sourceTable schema.Table, targetDB sqlExecutor, targetDialect dialect, targetTable schema.Table, allowUpdate bool, scope string, dryRun bool, enforceSelfReferenceOrdering bool) (tableState, error) {
	if len(sourceTable.PrimaryKey.Columns) == 0 {
		return tableState{}, fmt.Errorf("table %s has no primary key; sync requires primary keys", sourceTable.ID.String())
	}
	columns, err := sharedWritableColumns(sourceTable, targetTable)
	if err != nil {
		return tableState{}, err
	}
	pkIndexes := make(map[string]int, len(sourceTable.PrimaryKey.Columns))
	for index, column := range columns {
		pkIndexes[column.Name] = index
	}
	for _, keyColumn := range sourceTable.PrimaryKey.Columns {
		if _, ok := pkIndexes[keyColumn]; !ok {
			return tableState{}, fmt.Errorf("table %s primary key column %s is not writable on the target", sourceTable.ID.String(), keyColumn)
		}
	}
	targetRows, err := loadTargetRows(ctx, targetDB, targetDialect, targetTable, columns)
	if err != nil {
		return tableState{}, err
	}
	rows, err := sourceDB.QueryContext(ctx, buildSelectQuery(sourceDialect, sourceTable.ID, columns, sourceTable.PrimaryKey.Columns))
	if err != nil {
		return tableState{}, fmt.Errorf("query source table %s: %w", sourceTable.ID.String(), err)
	}
	defer rows.Close()

	insertSQL := buildInsertQuery(targetDialect, targetTable.ID, columns)
	var insertStmt *sql.Stmt
	if !dryRun {
		insertStmt, err = targetDB.PrepareContext(ctx, insertSQL)
		if err != nil {
			return tableState{}, fmt.Errorf("prepare target insert for %s: %w", targetTable.ID.String(), err)
		}
		defer insertStmt.Close()
	}
	updateColumns := nonPrimaryKeyColumns(columns, sourceTable.PrimaryKey.Columns)
	var updateStmt *sql.Stmt
	if !dryRun && allowUpdate && len(updateColumns) > 0 {
		updateStmt, err = targetDB.PrepareContext(ctx, buildUpdateQuery(targetDialect, targetTable.ID, updateColumns, sourceTable.PrimaryKey.Columns))
		if err != nil {
			return tableState{}, fmt.Errorf("prepare target update for %s: %w", targetTable.ID.String(), err)
		}
		defer updateStmt.Close()
	}

	state := tableState{
		report:        TableReport{TableID: sourceTable.ID, Scope: scope},
		table:         sourceTable,
		columns:       columns,
		primaryKey:    append([]string(nil), sourceTable.PrimaryKey.Columns...),
		sourceSeen:    map[string]struct{}{},
		targetRows:    targetRows,
		targetDialect: targetDialect,
	}
	selfReferences := selfReferencingForeignKeys(sourceTable, columns)
	pending := make([]syncAction, 0)
	for rows.Next() {
		values, err := scanRowValues(rows, columns)
		if err != nil {
			return tableState{}, fmt.Errorf("scan source row for %s: %w", sourceTable.ID.String(), err)
		}
		state.report.SourceRows++
		key, err := encodePrimaryKey(values, columns, sourceTable.PrimaryKey.Columns)
		if err != nil {
			return tableState{}, fmt.Errorf("encode primary key for %s: %w", sourceTable.ID.String(), err)
		}
		state.sourceSeen[key] = struct{}{}
		targetValues, exists := targetRows[key]
		if !exists {
			state.report.MissingRows++
			if dryRun {
				state.report.InsertedRows++
				continue
			}
			pending = append(pending, syncAction{typ: syncActionInsert, values: cloneValues(values), key: key})
			continue
		}
		if allowUpdate && rowChanged(values, targetValues, columns, sourceTable.PrimaryKey.Columns) {
			if dryRun {
				state.report.UpdatedRows++
				continue
			}
			pending = append(pending, syncAction{typ: syncActionUpdate, values: cloneValues(values), key: key, targetValues: cloneValues(targetValues)})
		}
	}
	if err := rows.Err(); err != nil {
		return tableState{}, fmt.Errorf("iterate source rows for %s: %w", sourceTable.ID.String(), err)
	}
	if !dryRun {
		if err := executePendingActions(ctx, targetTable, &state, insertStmt, updateStmt, pending, selfReferences, enforceSelfReferenceOrdering); err != nil {
			return tableState{}, err
		}
	}
	if !dryRun && state.report.InsertedRows != state.report.MissingRows {
		return tableState{}, fmt.Errorf("table %s inserted %d rows but expected %d", targetTable.ID.String(), state.report.InsertedRows, state.report.MissingRows)
	}
	return state, nil
}

func executePendingActions(ctx context.Context, targetTable schema.Table, state *tableState, insertStmt *sql.Stmt, updateStmt *sql.Stmt, pending []syncAction, selfReferences []schema.ForeignKey, enforceSelfReferenceOrdering bool) error {
	remaining := append([]syncAction(nil), pending...)
	for len(remaining) > 0 {
		next := make([]syncAction, 0, len(remaining))
		progressed := false
		for _, action := range remaining {
			if enforceSelfReferenceOrdering && !selfReferencesSatisfied(action.values, state.columns, state.primaryKey, selfReferences, state.targetRows) {
				next = append(next, action)
				continue
			}
			switch action.typ {
			case syncActionInsert:
				if _, err := insertStmt.ExecContext(ctx, action.values...); err != nil {
					return fmt.Errorf("insert row into %s: %w", targetTable.ID.String(), err)
				}
				state.report.InsertedRows++
				state.targetRows[action.key] = cloneValues(action.values)
			case syncActionUpdate:
				if updateStmt == nil {
					return fmt.Errorf("update row in %s: update statement is not prepared", targetTable.ID.String())
				}
				if _, err := updateStmt.ExecContext(ctx, updateArgs(action.values, state.columns, state.primaryKey)...); err != nil {
					return fmt.Errorf("update row in %s: %w", targetTable.ID.String(), err)
				}
				state.report.UpdatedRows++
				state.targetRows[action.key] = cloneValues(action.values)
			default:
				return fmt.Errorf("unsupported sync action %q for %s", action.typ, targetTable.ID.String())
			}
			progressed = true
		}
		if progressed {
			remaining = next
			continue
		}
		if !enforceSelfReferenceOrdering {
			return fmt.Errorf("table %s could not make progress while executing pending actions", targetTable.ID.String())
		}
		return fmt.Errorf("table %s has %d row(s) with unresolved self-referencing foreign keys; cyclic or missing parent rows block insert/update ordering", targetTable.ID.String(), len(remaining))
	}
	return nil
}

func shouldEnforceSelfReferenceOrdering(dryRun bool, engine model.Engine) bool {
	if dryRun {
		return true
	}
	switch engine {
	case model.EngineMySQL, model.EngineMariaDB:
		return false
	default:
		return true
	}
}

func loadTargetRows(ctx context.Context, db sqlExecutor, dialect dialect, table schema.Table, columns []schema.Column) (map[string][]any, error) {
	rows, err := db.QueryContext(ctx, buildSelectQuery(dialect, table.ID, columns, table.PrimaryKey.Columns))
	if err != nil {
		return nil, fmt.Errorf("query target keys for %s: %w", table.ID.String(), err)
	}
	defer rows.Close()
	valuesByKey := map[string][]any{}
	for rows.Next() {
		values, err := scanRowValues(rows, columns)
		if err != nil {
			return nil, err
		}
		key, err := encodePrimaryKey(values, columns, table.PrimaryKey.Columns)
		if err != nil {
			return nil, err
		}
		valuesByKey[key] = cloneValues(values)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return valuesByKey, nil
}

func deleteMissingRows(ctx context.Context, targetDB sqlExecutor, state tableState, dryRun bool) (int, error) {
	missingKeys := make([]string, 0)
	for key := range state.targetRows {
		if _, ok := state.sourceSeen[key]; ok {
			continue
		}
		missingKeys = append(missingKeys, key)
	}
	if len(missingKeys) == 0 {
		return 0, nil
	}
	if dryRun {
		return len(missingKeys), nil
	}
	stmt, err := targetDB.PrepareContext(ctx, buildDeleteQuery(state.targetDialect, state.table.ID, state.primaryKey))
	if err != nil {
		return 0, fmt.Errorf("prepare target delete for %s: %w", state.table.ID.String(), err)
	}
	defer stmt.Close()
	deleted := 0
	for _, key := range missingKeys {
		values := state.targetRows[key]
		pkValues, err := primaryKeyValues(values, state.columns, state.primaryKey)
		if err != nil {
			return 0, err
		}
		if _, err := stmt.ExecContext(ctx, pkValues...); err != nil {
			return 0, fmt.Errorf("delete row from %s: %w", state.table.ID.String(), err)
		}
		deleted++
	}
	return deleted, nil
}

func scanRowValues(rows *sql.Rows, columns []schema.Column) ([]any, error) {
	values := make([]any, len(columns))
	args := make([]any, len(columns))
	for index := range values {
		args[index] = &values[index]
	}
	if err := rows.Scan(args...); err != nil {
		return nil, err
	}
	for index, value := range values {
		normalized, err := normalizeScannedValue(value, columns[index])
		if err != nil {
			return nil, err
		}
		values[index] = normalized
	}
	return values, nil
}

func sharedWritableColumns(sourceTable schema.Table, targetTable schema.Table) ([]schema.Column, error) {
	targetColumns := make(map[string]schema.Column, len(targetTable.Columns))
	for _, column := range targetTable.Columns {
		targetColumns[column.Name] = column
	}
	primaryKeySet := make(map[string]struct{}, len(sourceTable.PrimaryKey.Columns))
	for _, name := range sourceTable.PrimaryKey.Columns {
		primaryKeySet[name] = struct{}{}
	}
	columns := make([]schema.Column, 0, len(sourceTable.Columns))
	for _, sourceColumn := range sourceTable.Columns {
		targetColumn, ok := targetColumns[sourceColumn.Name]
		if !ok || !targetColumn.Writable {
			continue
		}
		if targetColumn.Identity {
			if _, isPrimaryKey := primaryKeySet[targetColumn.Name]; !isPrimaryKey {
				continue
			}
		}
		columns = append(columns, sourceColumn)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s has no shared writable columns", sourceTable.ID.String())
	}
	return columns, nil
}

func selfReferencingForeignKeys(table schema.Table, columns []schema.Column) []schema.ForeignKey {
	columnSet := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		columnSet[column.Name] = struct{}{}
	}
	result := make([]schema.ForeignKey, 0)
	for _, foreignKey := range table.ForeignKeys {
		if foreignKey.ReferencedTable != table.ID {
			continue
		}
		usable := true
		for _, name := range foreignKey.Columns {
			if _, ok := columnSet[name]; !ok {
				usable = false
				break
			}
		}
		if !usable {
			continue
		}
		for _, name := range foreignKey.ReferencedColumns {
			if _, ok := columnSet[name]; !ok {
				usable = false
				break
			}
		}
		if usable {
			result = append(result, foreignKey)
		}
	}
	return result
}

func selfReferencesSatisfied(values []any, columns []schema.Column, primaryKey []string, foreignKeys []schema.ForeignKey, targetRows map[string][]any) bool {
	if len(foreignKeys) == 0 {
		return true
	}
	indexByName := make(map[string]int, len(columns))
	for index, column := range columns {
		indexByName[column.Name] = index
	}
	for _, foreignKey := range foreignKeys {
		allNull := true
		referencedValues := make(map[string]any, len(foreignKey.ReferencedColumns))
		for index, localName := range foreignKey.Columns {
			value := values[indexByName[localName]]
			if value != nil {
				allNull = false
			}
			referencedValues[foreignKey.ReferencedColumns[index]] = value
		}
		if allNull {
			continue
		}
		keyParts := make([]string, 0, len(primaryKey))
		for _, name := range primaryKey {
			value, ok := referencedValues[name]
			if !ok {
				return true
			}
			keyParts = append(keyParts, normalizeKeyPart(value))
		}
		if _, exists := targetRows[strings.Join(keyParts, "|")]; !exists {
			return false
		}
	}
	return true
}

func columnsByNames(table schema.Table, names []string) []schema.Column {
	indexed := make(map[string]schema.Column, len(table.Columns))
	for _, column := range table.Columns {
		indexed[column.Name] = column
	}
	result := make([]schema.Column, 0, len(names))
	for _, name := range names {
		if column, ok := indexed[name]; ok {
			result = append(result, column)
		}
	}
	return result
}

func nonPrimaryKeyColumns(columns []schema.Column, primaryKey []string) []schema.Column {
	pkSet := make(map[string]struct{}, len(primaryKey))
	for _, name := range primaryKey {
		pkSet[name] = struct{}{}
	}
	result := make([]schema.Column, 0, len(columns))
	for _, column := range columns {
		if _, ok := pkSet[column.Name]; ok {
			continue
		}
		result = append(result, column)
	}
	return result
}

func buildSelectQuery(dialect dialect, tableID schema.TableID, columns []schema.Column, orderBy []string) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, dialect.quote(column.Name))
	}
	query := fmt.Sprintf("select %s from %s", strings.Join(parts, ", "), qualifyTable(dialect, tableID))
	if len(orderBy) == 0 {
		return query
	}
	ordered := make([]string, 0, len(orderBy))
	for _, name := range orderBy {
		ordered = append(ordered, dialect.quote(name))
	}
	return query + " order by " + strings.Join(ordered, ", ")
}

func buildInsertQuery(dialect dialect, tableID schema.TableID, columns []schema.Column) string {
	quoted := make([]string, 0, len(columns))
	placeholders := make([]string, 0, len(columns))
	for index, column := range columns {
		quoted = append(quoted, dialect.quote(column.Name))
		placeholders = append(placeholders, dialect.placeholder(index+1))
	}
	return fmt.Sprintf("insert into %s (%s) values (%s)", qualifyTable(dialect, tableID), strings.Join(quoted, ", "), strings.Join(placeholders, ", "))
}

func buildUpdateQuery(dialect dialect, tableID schema.TableID, columns []schema.Column, primaryKey []string) string {
	setParts := make([]string, 0, len(columns))
	for index, column := range columns {
		setParts = append(setParts, fmt.Sprintf("%s = %s", dialect.quote(column.Name), dialect.placeholder(index+1)))
	}
	whereParts := make([]string, 0, len(primaryKey))
	for index, column := range primaryKey {
		whereParts = append(whereParts, fmt.Sprintf("%s = %s", dialect.quote(column), dialect.placeholder(len(columns)+index+1)))
	}
	return fmt.Sprintf("update %s set %s where %s", qualifyTable(dialect, tableID), strings.Join(setParts, ", "), strings.Join(whereParts, " and "))
}

func buildDeleteQuery(dialect dialect, tableID schema.TableID, primaryKey []string) string {
	whereParts := make([]string, 0, len(primaryKey))
	for index, column := range primaryKey {
		whereParts = append(whereParts, fmt.Sprintf("%s = %s", dialect.quote(column), dialect.placeholder(index+1)))
	}
	return fmt.Sprintf("delete from %s where %s", qualifyTable(dialect, tableID), strings.Join(whereParts, " and "))
}

func qualifyTable(dialect dialect, tableID schema.TableID) string {
	if tableID.Schema == "" {
		return dialect.quote(tableID.Name)
	}
	return dialect.quote(tableID.Schema) + "." + dialect.quote(tableID.Name)
}

func encodePrimaryKey(values []any, columns []schema.Column, primaryKey []string) (string, error) {
	indexByName := make(map[string]int, len(columns))
	for index, column := range columns {
		indexByName[column.Name] = index
	}
	parts := make([]string, 0, len(primaryKey))
	for _, name := range primaryKey {
		index, ok := indexByName[name]
		if !ok {
			return "", fmt.Errorf("primary key column %s is missing from the selected values", name)
		}
		parts = append(parts, normalizeKeyPart(values[index]))
	}
	return strings.Join(parts, "|"), nil
}

func primaryKeyValues(values []any, columns []schema.Column, primaryKey []string) ([]any, error) {
	indexByName := make(map[string]int, len(columns))
	for index, column := range columns {
		indexByName[column.Name] = index
	}
	result := make([]any, 0, len(primaryKey))
	for _, name := range primaryKey {
		index, ok := indexByName[name]
		if !ok {
			return nil, fmt.Errorf("primary key column %s is missing from the selected values", name)
		}
		result = append(result, values[index])
	}
	return result, nil
}

func updateArgs(values []any, columns []schema.Column, primaryKey []string) []any {
	updatable := nonPrimaryKeyColumns(columns, primaryKey)
	args := make([]any, 0, len(updatable)+len(primaryKey))
	indexByName := make(map[string]int, len(columns))
	for index, column := range columns {
		indexByName[column.Name] = index
	}
	for _, column := range updatable {
		args = append(args, values[indexByName[column.Name]])
	}
	for _, name := range primaryKey {
		args = append(args, values[indexByName[name]])
	}
	return args
}

func rowChanged(source []any, target []any, columns []schema.Column, primaryKey []string) bool {
	pkSet := make(map[string]struct{}, len(primaryKey))
	for _, name := range primaryKey {
		pkSet[name] = struct{}{}
	}
	for index, column := range columns {
		if _, ok := pkSet[column.Name]; ok {
			continue
		}
		if !valuesEqual(source[index], target[index]) {
			return true
		}
	}
	return false
}

func valuesEqual(left any, right any) bool {
	switch leftTyped := left.(type) {
	case time.Time:
		rightTyped, ok := right.(time.Time)
		if !ok {
			return false
		}
		return leftTyped.UTC().Equal(rightTyped.UTC())
	case nil:
		return right == nil
	default:
		return fmt.Sprint(left) == fmt.Sprint(right)
	}
}

func cloneValues(values []any) []any {
	cloned := make([]any, len(values))
	copy(cloned, values)
	return cloned
}

func normalizeKeyPart(value any) string {
	switch typed := value.(type) {
	case nil:
		return "<nil>"
	case []byte:
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(typed)
	}
}

func normalizeScannedValue(value any, column schema.Column) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return convertTextValue(string(typed), column)
	case string:
		return convertTextValue(typed, column)
	default:
		return typed, nil
	}
}

func convertTextValue(value string, column schema.Column) (any, error) {
	normalizedType := strings.ToLower(strings.TrimSpace(column.DataType))
	switch normalizedType {
	case "tinyint", "smallint", "mediumint", "int", "integer", "bigint", "serial", "bigserial":
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil, err
		}
		return parsed, nil
	case "real", "float", "double", "double precision":
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil, err
		}
		return parsed, nil
	case "bool", "boolean":
		trimmed := strings.TrimSpace(strings.ToLower(value))
		switch trimmed {
		case "1", "t", "true", "yes", "y":
			return true, nil
		case "0", "f", "false", "no", "n":
			return false, nil
		default:
			return nil, fmt.Errorf("unsupported boolean value %q", value)
		}
	default:
		return value, nil
	}
}

func openDatabase(engine model.Engine, dsn string) (*sql.DB, dialect, error) {
	switch engine {
	case model.EngineMySQL, model.EngineMariaDB:
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, nil, err
		}
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, nil, err
		}
		return db, mysqlDialect{}, nil
	case model.EnginePostgres:
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, nil, err
		}
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, nil, err
		}
		return db, postgresDialect{}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported engine %q", engine)
	}
}

func tableSet(values []schema.TableID) map[schema.TableID]struct{} {
	set := make(map[schema.TableID]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func (mysqlDialect) quote(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func (mysqlDialect) placeholder(_ int) string {
	return "?"
}

func (postgresDialect) quote(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func (postgresDialect) placeholder(position int) string {
	return "$" + strconv.Itoa(position)
}
