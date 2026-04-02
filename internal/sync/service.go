package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
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
	Phase     string
	Detail    string
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
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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

type deletePlan struct {
	table          schema.Table
	primaryKey     []string
	tempTableName  string
	candidateCount int
}

type excludedReferenceRule struct {
	foreignKey schema.ForeignKey
	nullable   bool
}

type syncActionType string

const (
	syncActionInsert syncActionType = "insert"
	syncActionUpdate syncActionType = "update"
	deleteBatchSize                 = 500
)

var mirrorDeleteTempCounter uint64

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
	if progress != nil {
		progress(ProgressUpdate{Phase: "preparing sync", Detail: "opening source and target database connections", DryRun: dryRun})
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
	targetConn, err := targetDB.Conn(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("pin target connection: %w", err)
	}
	defer targetConn.Close()
	targetExecutor := sqlExecutor(targetConn)
	if !dryRun {
		switch candidate.Target.Engine {
		case model.EngineMySQL, model.EngineMariaDB:
			if progress != nil {
				progress(ProgressUpdate{Phase: "preparing sync", Detail: "disabling target foreign key checks", DryRun: dryRun})
			}
			if _, err := targetConn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
				return Report{}, fmt.Errorf("disable foreign key checks on target: %w", err)
			}
			defer func() {
				_, _ = targetConn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 1")
			}()
		}
	}

	report := Report{DryRun: dryRun, Tables: make([]TableReport, 0, len(preview.FinalTables))}
	explicitSet := tableSet(preview.ExplicitIncludes)
	mergeTables, err := resolveConfiguredTableIDs(preview.ExplicitIncludes, candidate.Sync.MergeTables, "merge")
	if err != nil {
		return report, err
	}
	mergeSet := tableSet(mergeTables)
	excludedTables, err := resolveAvailableTableIDs(snapshotTableIDs(discovery.Source.Snapshot), candidate.Selection.ExcludedTables, "excluded")
	if err != nil {
		return report, err
	}
	excludedSet := tableSet(excludedTables)
	explicitTables := make([]schema.TableID, 0, len(preview.FinalTables))
	for _, tableID := range preview.FinalTables {
		if _, ok := explicitSet[tableID]; ok {
			explicitTables = append(explicitTables, tableID)
		}
	}
	deletePlans := make(map[schema.TableID]deletePlan, len(preview.ExplicitIncludes))
	deleteCounts := make(map[schema.TableID]int, len(preview.ExplicitIncludes))
	if candidate.Sync.MirrorDelete {
		for index, tableID := range explicitTables {
			if _, merge := mergeSet[tableID]; !merge {
				continue
			}
			if progress != nil {
				progress(ProgressUpdate{Phase: "scanning delete candidates", Detail: displaySyncTable(tableID, "explicit"), Completed: index + 1, Total: len(explicitTables), TableID: tableID, Scope: "explicit", DryRun: dryRun})
			}
			sourceTable, ok := discovery.Source.Snapshot.TableByID(tableID)
			if !ok {
				return report, fmt.Errorf("selected source table %s is not available", tableID.String())
			}
			targetTable, ok := discovery.Target.Snapshot.TableByID(tableID)
			if !ok {
				return report, fmt.Errorf("selected target table %s is not available", tableID.String())
			}
			plan, err := inspectTableForDelete(ctx, sourceDB, sourceDialect, sourceTable, targetExecutor, targetDialect, targetTable)
			if err != nil {
				return report, err
			}
			deletePlans[tableID] = plan
			deleteCounts[tableID] = plan.candidateCount
		}
		defer func() {
			for _, plan := range deletePlans {
				_ = dropMirrorDeleteTempTable(context.Background(), targetExecutor, targetDialect, plan.tempTableName)
			}
		}()
		if !dryRun {
			for index := len(explicitTables) - 1; index >= 0; index-- {
				tableID := explicitTables[index]
				plan, ok := deletePlans[tableID]
				if !ok {
					continue
				}
				if progress != nil {
					progress(ProgressUpdate{Phase: "applying mirror deletes", Detail: fmt.Sprintf("%s, %d row(s)", displaySyncTable(tableID, "explicit"), plan.candidateCount), Completed: len(explicitTables) - index, Total: len(explicitTables), TableID: tableID, Scope: "explicit", DryRun: dryRun})
				}
				tx, err := targetConn.BeginTx(ctx, nil)
				if err != nil {
					return report, fmt.Errorf("begin mirror delete transaction for %s: %w", tableID.String(), err)
				}
				deletedRows, err := deleteMissingRows(ctx, tx, targetDialect, plan, false)
				if err != nil {
					_ = tx.Rollback()
					return report, err
				}
				if err := tx.Commit(); err != nil {
					return report, fmt.Errorf("commit mirror delete transaction for %s: %w", tableID.String(), err)
				}
				if deletedRows == 0 && plan.candidateCount > 0 {
					return report, fmt.Errorf("table %s deleted 0 rows but expected %d", tableID.String(), plan.candidateCount)
				}
				if deletedRows > 0 {
					deleteCounts[tableID] = deletedRows
				}
				delete(deletePlans, tableID)
				if err := dropMirrorDeleteTempTable(ctx, targetExecutor, targetDialect, plan.tempTableName); err != nil {
					return report, fmt.Errorf("drop mirror delete temp table for %s: %w", tableID.String(), err)
				}
			}
		}
	}
	states := make([]tableState, 0, len(preview.FinalTables))
	totalTables := len(preview.FinalTables)
	enforceSelfReferenceOrdering := shouldEnforceSelfReferenceOrdering(dryRun, candidate.Target.Engine)
	for index, tableID := range preview.FinalTables {
		sourceTable, ok := discovery.Source.Snapshot.TableByID(tableID)
		if !ok {
			return report, fmt.Errorf("selected source table %s is not available", tableID.String())
		}
		targetTable, ok := discovery.Target.Snapshot.TableByID(tableID)
		if !ok {
			return report, fmt.Errorf("selected target table %s is not available", tableID.String())
		}
		scope := "implicit"
		mergeMode := false
		if _, ok := explicitSet[tableID]; ok {
			scope = "explicit"
			_, mergeMode = mergeSet[tableID]
		}
		if progress != nil {
			progress(ProgressUpdate{Phase: "syncing table", Detail: displaySyncTable(tableID, scope), Completed: index + 1, Total: totalTables, TableID: tableID, Scope: scope, DryRun: dryRun})
		}
		tableExecutor := targetExecutor
		var tableTx *sql.Tx
		if !dryRun {
			tableTx, err = targetConn.BeginTx(ctx, nil)
			if err != nil {
				return report, fmt.Errorf("begin sync transaction for %s: %w", tableID.String(), err)
			}
			tableExecutor = tableTx
		}
		state, err := tableState{}, error(nil)
		if scope == "explicit" && !mergeMode {
			state, err = replaceTable(ctx, sourceDB, sourceDialect, sourceTable, tableExecutor, targetDialect, targetTable, excludedSet, scope, dryRun, enforceSelfReferenceOrdering)
		} else {
			state, err = syncTable(ctx, sourceDB, sourceDialect, sourceTable, tableExecutor, targetDialect, targetTable, excludedSet, mergeMode, scope, dryRun, enforceSelfReferenceOrdering)
		}
		if err != nil {
			if tableTx != nil {
				_ = tableTx.Rollback()
			}
			return report, err
		}
		if tableTx != nil {
			if err := tableTx.Commit(); err != nil {
				return report, fmt.Errorf("commit sync transaction for %s: %w", tableID.String(), err)
			}
		}
		if deletedRows, ok := deleteCounts[tableID]; ok {
			state.report.DeletedRows = deletedRows
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

func displaySyncTable(tableID schema.TableID, scope string) string {
	name := tableID.String()
	if tableID.Name != "" {
		name = tableID.Name
	}
	if scope == "" {
		return name
	}
	return fmt.Sprintf("%s [%s]", name, scope)
}

func inspectTableForDelete(ctx context.Context, sourceDB *sql.DB, sourceDialect dialect, sourceTable schema.Table, targetDB sqlExecutor, targetDialect dialect, targetTable schema.Table) (deletePlan, error) {
	if len(sourceTable.PrimaryKey.Columns) == 0 {
		return deletePlan{}, fmt.Errorf("table %s has no primary key; sync requires primary keys", sourceTable.ID.String())
	}
	sourceColumns := columnsByNames(sourceTable, sourceTable.PrimaryKey.Columns)
	if len(sourceColumns) != len(sourceTable.PrimaryKey.Columns) {
		return deletePlan{}, fmt.Errorf("table %s is missing one or more source primary key columns for mirror delete", sourceTable.ID.String())
	}
	targetColumns := columnsByNames(targetTable, targetTable.PrimaryKey.Columns)
	if len(targetColumns) != len(targetTable.PrimaryKey.Columns) {
		return deletePlan{}, fmt.Errorf("table %s is missing one or more target primary key columns for mirror delete", targetTable.ID.String())
	}
	tempTableName := nextMirrorDeleteTempTableName(targetTable.ID)
	if err := createMirrorDeleteTempTable(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, targetColumns); err != nil {
		return deletePlan{}, err
	}
	defer func() {
		if ctx.Err() != nil {
			_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
		}
	}()
	rows, err := sourceDB.QueryContext(ctx, buildSelectQuery(sourceDialect, sourceTable.ID, sourceColumns, sourceTable.PrimaryKey.Columns))
	if err != nil {
		_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
		return deletePlan{}, fmt.Errorf("query source table %s: %w", sourceTable.ID.String(), err)
	}
	defer rows.Close()
	buffer := make([][]any, 0, deleteBatchSize)
	for rows.Next() {
		values, err := scanRowValues(rows, sourceColumns)
		if err != nil {
			_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
			return deletePlan{}, fmt.Errorf("scan source row for %s: %w", sourceTable.ID.String(), err)
		}
		buffer = append(buffer, values)
		if len(buffer) == deleteBatchSize {
			if err := insertMirrorDeleteRows(ctx, targetDB, targetDialect, tempTableName, sourceColumns, buffer); err != nil {
				_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
				return deletePlan{}, err
			}
			buffer = buffer[:0]
		}
	}
	if len(buffer) > 0 {
		if err := insertMirrorDeleteRows(ctx, targetDB, targetDialect, tempTableName, sourceColumns, buffer); err != nil {
			_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
			return deletePlan{}, err
		}
	}
	if err := rows.Err(); err != nil {
		_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
		return deletePlan{}, fmt.Errorf("iterate source rows for %s: %w", sourceTable.ID.String(), err)
	}
	candidateCount, err := countMirrorDeleteRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, targetTable.PrimaryKey.Columns)
	if err != nil {
		_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
		return deletePlan{}, err
	}
	return deletePlan{table: targetTable, primaryKey: append([]string(nil), targetTable.PrimaryKey.Columns...), tempTableName: tempTableName, candidateCount: candidateCount}, nil
}

func replaceTable(ctx context.Context, sourceDB *sql.DB, sourceDialect dialect, sourceTable schema.Table, targetDB sqlExecutor, targetDialect dialect, targetTable schema.Table, excludedSet map[schema.TableID]struct{}, scope string, dryRun bool, enforceSelfReferenceOrdering bool) (tableState, error) {
	if len(sourceTable.PrimaryKey.Columns) == 0 {
		return tableState{}, fmt.Errorf("table %s has no primary key; sync requires primary keys", sourceTable.ID.String())
	}
	columns, err := sharedWritableColumns(sourceTable, targetTable)
	if err != nil {
		return tableState{}, err
	}
	tempTableName := nextReplaceTempTableName(targetTable.ID)
	if err := createReplaceTempTable(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, columns, sourceTable.PrimaryKey.Columns); err != nil {
		return tableState{}, err
	}
	defer func() {
		_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
	}()

	sourceRowCount, err := stageSourceRows(ctx, sourceDB, sourceDialect, sourceTable, targetDB, targetDialect, tempTableName, columns)
	if err != nil {
		return tableState{}, err
	}
	if err := applyExcludedReferencePoliciesToTempTable(ctx, targetDB, targetDialect, sourceTable, targetTable, tempTableName, columns, excludedSet); err != nil {
		return tableState{}, err
	}
	missingRows, err := countMissingStageRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, sourceTable.PrimaryKey.Columns)
	if err != nil {
		return tableState{}, err
	}
	updatedRows, err := countChangedStageRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, nonPrimaryKeyColumns(columns, sourceTable.PrimaryKey.Columns), sourceTable.PrimaryKey.Columns)
	if err != nil {
		return tableState{}, err
	}
	deletedRows, err := countMirrorDeleteRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, sourceTable.PrimaryKey.Columns)
	if err != nil {
		return tableState{}, err
	}

	state := tableState{
		report: TableReport{
			TableID:      sourceTable.ID,
			Scope:        scope,
			SourceRows:   sourceRowCount,
			MissingRows:  missingRows,
			InsertedRows: missingRows,
			UpdatedRows:  updatedRows,
			DeletedRows:  deletedRows,
		},
		table:         sourceTable,
		columns:       columns,
		primaryKey:    append([]string(nil), sourceTable.PrimaryKey.Columns...),
		sourceSeen:    map[string]struct{}{},
		targetRows:    map[string][]any{},
		targetDialect: targetDialect,
	}
	if dryRun {
		return state, nil
	}

	deletedRows, err = deleteMissingRows(ctx, targetDB, targetDialect, deletePlan{table: targetTable, primaryKey: append([]string(nil), sourceTable.PrimaryKey.Columns...), tempTableName: tempTableName, candidateCount: deletedRows}, false)
	if err != nil {
		return tableState{}, err
	}
	insertedRows, err := insertMissingStageRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, columns, sourceTable.PrimaryKey.Columns)
	if err != nil {
		return tableState{}, err
	}
	updatedRows, err = updateChangedStageRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, nonPrimaryKeyColumns(columns, sourceTable.PrimaryKey.Columns), sourceTable.PrimaryKey.Columns)
	if err != nil {
		return tableState{}, err
	}
	state.report.InsertedRows = insertedRows
	state.report.UpdatedRows = updatedRows
	state.report.DeletedRows = deletedRows
	if state.report.InsertedRows != state.report.MissingRows {
		return tableState{}, fmt.Errorf("table %s inserted %d rows but expected %d", targetTable.ID.String(), state.report.InsertedRows, state.report.MissingRows)
	}
	if state.report.UpdatedRows != updatedRows {
		return tableState{}, fmt.Errorf("table %s updated %d rows but expected %d", targetTable.ID.String(), state.report.UpdatedRows, updatedRows)
	}
	return state, nil
}

func syncTable(ctx context.Context, sourceDB *sql.DB, sourceDialect dialect, sourceTable schema.Table, targetDB sqlExecutor, targetDialect dialect, targetTable schema.Table, excludedSet map[schema.TableID]struct{}, allowUpdate bool, scope string, dryRun bool, enforceSelfReferenceOrdering bool) (tableState, error) {
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
	selfReferences := selfReferencingForeignKeys(sourceTable, columns)
	if enforceSelfReferenceOrdering && len(selfReferences) > 0 {
		return syncTableInMemory(ctx, sourceDB, sourceDialect, sourceTable, targetDB, targetDialect, targetTable, columns, selfReferences, excludedSet, allowUpdate, scope, dryRun, enforceSelfReferenceOrdering)
	}
	return syncTableViaStage(ctx, sourceDB, sourceDialect, sourceTable, targetDB, targetDialect, targetTable, columns, excludedSet, allowUpdate, scope, dryRun)
}

func syncTableViaStage(ctx context.Context, sourceDB *sql.DB, sourceDialect dialect, sourceTable schema.Table, targetDB sqlExecutor, targetDialect dialect, targetTable schema.Table, columns []schema.Column, excludedSet map[schema.TableID]struct{}, allowUpdate bool, scope string, dryRun bool) (tableState, error) {
	tempTableName := nextSyncTempTableName(targetTable.ID)
	if err := createReplaceTempTable(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, columns, sourceTable.PrimaryKey.Columns); err != nil {
		return tableState{}, err
	}
	defer func() {
		_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
	}()

	sourceRowCount, err := stageSourceRows(ctx, sourceDB, sourceDialect, sourceTable, targetDB, targetDialect, tempTableName, columns)
	if err != nil {
		return tableState{}, err
	}
	if err := applyExcludedReferencePoliciesToTempTable(ctx, targetDB, targetDialect, sourceTable, targetTable, tempTableName, columns, excludedSet); err != nil {
		return tableState{}, err
	}
	missingRows, err := countMissingStageRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, sourceTable.PrimaryKey.Columns)
	if err != nil {
		return tableState{}, err
	}
	updateColumns := nonPrimaryKeyColumns(columns, sourceTable.PrimaryKey.Columns)
	updatedRows := 0
	if allowUpdate {
		updatedRows, err = countChangedStageRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, updateColumns, sourceTable.PrimaryKey.Columns)
		if err != nil {
			return tableState{}, err
		}
	}

	state := tableState{
		report: TableReport{
			TableID:      sourceTable.ID,
			Scope:        scope,
			SourceRows:   sourceRowCount,
			MissingRows:  missingRows,
			InsertedRows: missingRows,
			UpdatedRows:  updatedRows,
		},
		table:         sourceTable,
		columns:       columns,
		primaryKey:    append([]string(nil), sourceTable.PrimaryKey.Columns...),
		sourceSeen:    map[string]struct{}{},
		targetRows:    map[string][]any{},
		targetDialect: targetDialect,
	}
	if dryRun {
		return state, nil
	}

	insertedRows, err := insertMissingStageRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, columns, sourceTable.PrimaryKey.Columns)
	if err != nil {
		return tableState{}, err
	}
	if allowUpdate {
		updatedRows, err = updateChangedStageRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, updateColumns, sourceTable.PrimaryKey.Columns)
		if err != nil {
			return tableState{}, err
		}
	}
	state.report.InsertedRows = insertedRows
	state.report.UpdatedRows = updatedRows
	if state.report.InsertedRows != state.report.MissingRows {
		return tableState{}, fmt.Errorf("table %s inserted %d rows but expected %d", targetTable.ID.String(), state.report.InsertedRows, state.report.MissingRows)
	}
	return state, nil
}

func syncTableInMemory(ctx context.Context, sourceDB *sql.DB, sourceDialect dialect, sourceTable schema.Table, targetDB sqlExecutor, targetDialect dialect, targetTable schema.Table, columns []schema.Column, selfReferences []schema.ForeignKey, excludedSet map[schema.TableID]struct{}, allowUpdate bool, scope string, dryRun bool, enforceSelfReferenceOrdering bool) (tableState, error) {
	targetRows, err := loadTargetRows(ctx, targetDB, targetDialect, targetTable, columns)
	if err != nil {
		return tableState{}, err
	}
	excludedRules := excludedReferenceRules(sourceTable, columns, excludedSet)
	excludedExistenceCache := map[string]bool{}
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
		targetValues, exists := targetRows[key]
		if err := applyExcludedReferencePoliciesInMemory(ctx, targetDB, targetDialect, targetTable, columns, values, targetValues, exists, excludedRules, excludedExistenceCache); err != nil {
			return tableState{}, err
		}
		state.sourceSeen[key] = struct{}{}
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

func countTableRows(ctx context.Context, db sqlExecutor, dialect dialect, tableID schema.TableID) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, buildCountTableRowsQuery(dialect, tableID)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count target rows for %s: %w", tableID.String(), err)
	}
	return count, nil
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

func deleteMissingRows(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, plan deletePlan, dryRun bool) (int, error) {
	if plan.candidateCount == 0 {
		return 0, nil
	}
	if dryRun {
		return plan.candidateCount, nil
	}
	result, err := targetDB.ExecContext(ctx, buildMirrorDeleteQuery(targetDialect, plan.table.ID, plan.tempTableName, plan.primaryKey))
	if err != nil {
		return 0, fmt.Errorf("delete rows from %s: %w", plan.table.ID.String(), err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return plan.candidateCount, nil
	}
	return int(deleted), nil
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

func columnNames(columns []schema.Column) []string {
	result := make([]string, 0, len(columns))
	for _, column := range columns {
		result = append(result, column.Name)
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

func buildDeleteAllQuery(dialect dialect, tableID schema.TableID) string {
	return fmt.Sprintf("delete from %s", qualifyTable(dialect, tableID))
}

func buildCountTableRowsQuery(dialect dialect, tableID schema.TableID) string {
	return fmt.Sprintf("select count(*) from %s", qualifyTable(dialect, tableID))
}

func buildDeleteBatchQuery(dialect dialect, tableID schema.TableID, primaryKey []string, rows [][]any) (string, []any) {
	clauses := make([]string, 0, len(rows))
	args := make([]any, 0, len(rows)*len(primaryKey))
	placeholderIndex := 1
	for _, row := range rows {
		parts := make([]string, 0, len(primaryKey))
		for columnIndex, column := range primaryKey {
			parts = append(parts, fmt.Sprintf("%s = %s", dialect.quote(column), dialect.placeholder(placeholderIndex)))
			args = append(args, row[columnIndex])
			placeholderIndex++
		}
		clauses = append(clauses, "("+strings.Join(parts, " and ")+")")
	}
	return fmt.Sprintf("delete from %s where %s", qualifyTable(dialect, tableID), strings.Join(clauses, " or ")), args
}

func buildInsertBatchQuery(dialect dialect, tableID schema.TableID, columns []schema.Column, rows [][]any) (string, []any) {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, dialect.quote(column.Name))
	}
	valueClauses := make([]string, 0, len(rows))
	args := make([]any, 0, len(rows)*len(columns))
	placeholderIndex := 1
	for _, row := range rows {
		placeholders := make([]string, 0, len(columns))
		for columnIndex := range columns {
			placeholders = append(placeholders, dialect.placeholder(placeholderIndex))
			args = append(args, row[columnIndex])
			placeholderIndex++
		}
		valueClauses = append(valueClauses, "("+strings.Join(placeholders, ", ")+")")
	}
	return fmt.Sprintf("insert into %s (%s) values %s", qualifyTable(dialect, tableID), strings.Join(quoted, ", "), strings.Join(valueClauses, ", ")), args
}

func buildCreateTempTableQuery(dialect dialect, targetTable schema.TableID, tempTableName string, columns []schema.Column) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, dialect.quote(column.Name))
	}
	return fmt.Sprintf("create temporary table %s as select %s from %s where 1 = 0", dialect.quote(tempTableName), strings.Join(parts, ", "), qualifyTable(dialect, targetTable))
}

func buildCreateTempIndexQuery(dialect dialect, tempTableName string, columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, dialect.quote(column))
	}
	indexName := dialect.quote(tempTableName + "_pk_idx")
	return fmt.Sprintf("create index %s on %s (%s)", indexName, dialect.quote(tempTableName), strings.Join(quoted, ", "))
}

func buildCreateMirrorDeleteTempTableQuery(dialect dialect, targetTable schema.TableID, tempTableName string, columns []schema.Column) string {
	return buildCreateTempTableQuery(dialect, targetTable, tempTableName, columns)
}

func buildCreateMirrorDeleteTempIndexQuery(dialect dialect, tempTableName string, columns []string) string {
	return buildCreateTempIndexQuery(dialect, tempTableName, columns)
}

func buildCountMirrorDeleteQuery(dialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string) string {
	return fmt.Sprintf("select count(*) from %s as target where not exists (select 1 from %s as source_keys where %s)", qualifyTable(dialect, targetTable), dialect.quote(tempTableName), buildMirrorDeleteJoinCondition(dialect, "target", "source_keys", primaryKey))
}

func buildMirrorDeleteQuery(dialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string) string {
	joinCondition := buildMirrorDeleteJoinCondition(dialect, "target", "source_keys", primaryKey)
	switch dialect.(type) {
	case mysqlDialect:
		return fmt.Sprintf("delete target from %s as target where not exists (select 1 from %s as source_keys where %s)", qualifyTable(dialect, targetTable), dialect.quote(tempTableName), joinCondition)
	default:
		return fmt.Sprintf("delete from %s as target where not exists (select 1 from %s as source_keys where %s)", qualifyTable(dialect, targetTable), dialect.quote(tempTableName), joinCondition)
	}
}

func buildMirrorDeleteJoinCondition(dialect dialect, targetAlias string, sourceAlias string, primaryKey []string) string {
	parts := make([]string, 0, len(primaryKey))
	for _, column := range primaryKey {
		parts = append(parts, fmt.Sprintf("%s.%s = %s.%s", dialect.quote(targetAlias), dialect.quote(column), dialect.quote(sourceAlias), dialect.quote(column)))
	}
	return strings.Join(parts, " and ")
}

func buildCountMissingStageRowsQuery(dialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string) string {
	return fmt.Sprintf("select count(*) from %s as source where not exists (select 1 from %s as target where %s)", dialect.quote(tempTableName), qualifyTable(dialect, targetTable), buildMirrorDeleteJoinCondition(dialect, "target", "source", primaryKey))
}

func buildInsertMissingStageRowsQuery(dialect dialect, targetTable schema.TableID, tempTableName string, columns []schema.Column, primaryKey []string) string {
	selectParts := make([]string, 0, len(columns))
	quotedColumns := make([]string, 0, len(columns))
	for _, column := range columns {
		quotedColumns = append(quotedColumns, dialect.quote(column.Name))
		selectParts = append(selectParts, fmt.Sprintf("%s.%s", dialect.quote("source"), dialect.quote(column.Name)))
	}
	return fmt.Sprintf("insert into %s (%s) select %s from %s as source where not exists (select 1 from %s as target where %s)", qualifyTable(dialect, targetTable), strings.Join(quotedColumns, ", "), strings.Join(selectParts, ", "), dialect.quote(tempTableName), qualifyTable(dialect, targetTable), buildMirrorDeleteJoinCondition(dialect, "target", "source", primaryKey))
}

func buildCountChangedStageRowsQuery(dialect dialect, targetTable schema.TableID, tempTableName string, updateColumns []schema.Column, primaryKey []string) string {
	if len(updateColumns) == 0 {
		return "select 0"
	}
	return fmt.Sprintf("select count(*) from %s as target join %s as source on %s where %s", qualifyTable(dialect, targetTable), dialect.quote(tempTableName), buildMirrorDeleteJoinCondition(dialect, "target", "source", primaryKey), buildColumnDifferenceCondition(dialect, "target", "source", updateColumns))
}

func buildUpdateChangedStageRowsQuery(dialect dialect, targetTable schema.TableID, tempTableName string, updateColumns []schema.Column, primaryKey []string) string {
	if len(updateColumns) == 0 {
		return ""
	}
	setParts := make([]string, 0, len(updateColumns))
	for _, column := range updateColumns {
		setParts = append(setParts, fmt.Sprintf("%s = %s.%s", dialect.quote(column.Name), dialect.quote("source"), dialect.quote(column.Name)))
	}
	joinCondition := buildMirrorDeleteJoinCondition(dialect, "target", "source", primaryKey)
	differenceCondition := buildColumnDifferenceCondition(dialect, "target", "source", updateColumns)
	switch dialect.(type) {
	case mysqlDialect:
		qualifiedTarget := qualifyTable(dialect, targetTable)
		return fmt.Sprintf("update %s as target join %s as source on %s set %s where %s", qualifiedTarget, dialect.quote(tempTableName), joinCondition, strings.Join(prefixAliasedAssignments(dialect, "target", setParts), ", "), differenceCondition)
	default:
		qualifiedTarget := qualifyTable(dialect, targetTable)
		return fmt.Sprintf("update %s as target set %s from %s as source where %s and %s", qualifiedTarget, strings.Join(setParts, ", "), dialect.quote(tempTableName), joinCondition, differenceCondition)
	}
}

func buildColumnDifferenceCondition(dialect dialect, leftAlias string, rightAlias string, columns []schema.Column) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		left := fmt.Sprintf("%s.%s", dialect.quote(leftAlias), dialect.quote(column.Name))
		right := fmt.Sprintf("%s.%s", dialect.quote(rightAlias), dialect.quote(column.Name))
		switch dialect.(type) {
		case mysqlDialect:
			parts = append(parts, fmt.Sprintf("not (%s <=> %s)", left, right))
		default:
			parts = append(parts, fmt.Sprintf("%s is distinct from %s", left, right))
		}
	}
	return strings.Join(parts, " or ")
}

func prefixAliasedAssignments(dialect dialect, alias string, assignments []string) []string {
	qualified := make([]string, 0, len(assignments))
	prefix := dialect.quote(alias) + "."
	for _, assignment := range assignments {
		qualified = append(qualified, prefix+assignment)
	}
	return qualified
}

func nextMirrorDeleteTempTableName(tableID schema.TableID) string {
	return nextTempTableName("db_sync_delete", tableID)
}

func nextReplaceTempTableName(tableID schema.TableID) string {
	return nextTempTableName("db_sync_replace", tableID)
}

func nextSyncTempTableName(tableID schema.TableID) string {
	return nextTempTableName("db_sync_sync", tableID)
}

func snapshotTableIDs(snapshot schema.Snapshot) []schema.TableID {
	result := make([]schema.TableID, 0, len(snapshot.Tables))
	for _, table := range snapshot.Tables {
		result = append(result, table.ID)
	}
	return result
}

func resolveAvailableTableIDs(available []schema.TableID, configured []string, modeName string) ([]schema.TableID, error) {
	resolved := make([]schema.TableID, 0, len(configured))
	seen := map[schema.TableID]struct{}{}
	for _, value := range configured {
		id, err := resolveAvailableTableID(available, value, modeName)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		resolved = append(resolved, id)
	}
	return resolved, nil
}

func resolveAvailableTableID(available []schema.TableID, value string, modeName string) (schema.TableID, error) {
	id := schema.ParseTableID(value)
	for _, candidate := range available {
		if candidate == id {
			return candidate, nil
		}
	}
	if id.Schema != "" {
		return schema.TableID{}, fmt.Errorf("unknown %s table %q", modeName, value)
	}
	matches := make([]schema.TableID, 0)
	for _, candidate := range available {
		if candidate.Name == id.Name {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return schema.TableID{}, fmt.Errorf("unknown %s table %q", modeName, value)
	case 1:
		return matches[0], nil
	default:
		return schema.TableID{}, fmt.Errorf("%s table %q is ambiguous; qualify one of: %s", modeName, value, strings.Join(schema.SelectionStrings(matches), ", "))
	}
}

func excludedReferenceRules(table schema.Table, columns []schema.Column, excludedSet map[schema.TableID]struct{}) []excludedReferenceRule {
	if len(excludedSet) == 0 {
		return nil
	}
	columnByName := make(map[string]schema.Column, len(columns))
	for _, column := range columns {
		columnByName[column.Name] = column
	}
	rules := make([]excludedReferenceRule, 0)
	for _, foreignKey := range table.ForeignKeys {
		if _, excluded := excludedSet[foreignKey.ReferencedTable]; !excluded {
			continue
		}
		supported := true
		nullable := true
		for _, name := range foreignKey.Columns {
			column, ok := columnByName[name]
			if !ok {
				supported = false
				break
			}
			if !column.Nullable {
				nullable = false
			}
		}
		if !supported {
			continue
		}
		rules = append(rules, excludedReferenceRule{foreignKey: foreignKey, nullable: nullable})
	}
	return rules
}

func applyExcludedReferencePoliciesToTempTable(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, sourceTable schema.Table, targetTable schema.Table, tempTableName string, columns []schema.Column, excludedSet map[schema.TableID]struct{}) error {
	rules := excludedReferenceRules(sourceTable, columns, excludedSet)
	for _, rule := range rules {
		if err := preserveExcludedReferenceValues(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, sourceTable.PrimaryKey.Columns, rule); err != nil {
			return err
		}
		if rule.nullable {
			if err := nullExcludedReferenceValues(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, sourceTable.PrimaryKey.Columns, rule); err != nil {
				return err
			}
			continue
		}
		invalidRows, err := countInvalidExcludedReferenceRows(ctx, targetDB, targetDialect, targetTable.ID, tempTableName, sourceTable.PrimaryKey.Columns, rule)
		if err != nil {
			return err
		}
		if invalidRows > 0 {
			return fmt.Errorf("table %s has %d row(s) referencing excluded table %s through non-nullable foreign key %s", targetTable.ID.String(), invalidRows, rule.foreignKey.ReferencedTable.String(), rule.foreignKey.Name)
		}
	}
	return nil
}

func preserveExcludedReferenceValues(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string, rule excludedReferenceRule) error {
	query := buildPreserveExcludedReferenceQuery(targetDialect, targetTable, tempTableName, primaryKey, rule.foreignKey)
	if _, err := targetDB.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("preserve references to excluded table %s for %s: %w", rule.foreignKey.ReferencedTable.String(), targetTable.String(), err)
	}
	return nil
}

func nullExcludedReferenceValues(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string, rule excludedReferenceRule) error {
	query := buildNullExcludedReferenceQuery(targetDialect, targetTable, tempTableName, primaryKey, rule.foreignKey)
	if _, err := targetDB.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("null references to excluded table %s for %s: %w", rule.foreignKey.ReferencedTable.String(), targetTable.String(), err)
	}
	return nil
}

func countInvalidExcludedReferenceRows(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string, rule excludedReferenceRule) (int, error) {
	var count int
	query := buildCountInvalidExcludedReferenceQuery(targetDialect, targetTable, tempTableName, primaryKey, rule.foreignKey)
	if err := targetDB.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count invalid references to excluded table %s for %s: %w", rule.foreignKey.ReferencedTable.String(), targetTable.String(), err)
	}
	return count, nil
}

func applyExcludedReferencePoliciesInMemory(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.Table, columns []schema.Column, values []any, targetValues []any, targetExists bool, rules []excludedReferenceRule, cache map[string]bool) error {
	for _, rule := range rules {
		localValues, present, err := foreignKeyLocalValues(values, columns, rule.foreignKey)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		exists, err := excludedReferenceExists(ctx, targetDB, targetDialect, rule.foreignKey, localValues, cache)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if targetExists {
			copyForeignKeyValues(values, targetValues, columns, rule.foreignKey.Columns)
			continue
		}
		if rule.nullable {
			setForeignKeyValuesToNull(values, columns, rule.foreignKey.Columns)
			continue
		}
		return fmt.Errorf("table %s references excluded table %s through non-nullable foreign key %s", targetTable.ID.String(), rule.foreignKey.ReferencedTable.String(), rule.foreignKey.Name)
	}
	return nil
}

func foreignKeyLocalValues(values []any, columns []schema.Column, foreignKey schema.ForeignKey) ([]any, bool, error) {
	indexByName := make(map[string]int, len(columns))
	for index, column := range columns {
		indexByName[column.Name] = index
	}
	result := make([]any, 0, len(foreignKey.Columns))
	for _, name := range foreignKey.Columns {
		index, ok := indexByName[name]
		if !ok {
			return nil, false, fmt.Errorf("foreign key column %s is missing from selected values", name)
		}
		if values[index] == nil {
			return nil, false, nil
		}
		result = append(result, values[index])
	}
	return result, true, nil
}

func copyForeignKeyValues(dst []any, src []any, columns []schema.Column, foreignKeyColumns []string) {
	indexByName := make(map[string]int, len(columns))
	for index, column := range columns {
		indexByName[column.Name] = index
	}
	for _, name := range foreignKeyColumns {
		index, ok := indexByName[name]
		if !ok {
			continue
		}
		dst[index] = src[index]
	}
}

func setForeignKeyValuesToNull(values []any, columns []schema.Column, foreignKeyColumns []string) {
	indexByName := make(map[string]int, len(columns))
	for index, column := range columns {
		indexByName[column.Name] = index
	}
	for _, name := range foreignKeyColumns {
		index, ok := indexByName[name]
		if !ok {
			continue
		}
		values[index] = nil
	}
}

func excludedReferenceExists(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, foreignKey schema.ForeignKey, localValues []any, cache map[string]bool) (bool, error) {
	cacheKey := foreignKey.ReferencedTable.String() + ":" + encodeCacheValues(localValues)
	if exists, ok := cache[cacheKey]; ok {
		return exists, nil
	}
	query := buildExcludedReferenceExistsQuery(targetDialect, foreignKey)
	row := targetDB.QueryRowContext(ctx, query, localValues...)
	var marker int
	if err := row.Scan(&marker); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			cache[cacheKey] = false
			return false, nil
		}
		return false, fmt.Errorf("query excluded reference %s: %w", foreignKey.ReferencedTable.String(), err)
	}
	cache[cacheKey] = true
	return true, nil
}

func encodeCacheValues(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, normalizeKeyPart(value))
	}
	return strings.Join(parts, "|")
}

func buildExcludedReferenceExistsQuery(dialect dialect, foreignKey schema.ForeignKey) string {
	parts := make([]string, 0, len(foreignKey.ReferencedColumns))
	for index, column := range foreignKey.ReferencedColumns {
		parts = append(parts, fmt.Sprintf("%s = %s", dialect.quote(column), dialect.placeholder(index+1)))
	}
	return fmt.Sprintf("select 1 from %s where %s limit 1", qualifyTable(dialect, foreignKey.ReferencedTable), strings.Join(parts, " and "))
}

func buildPreserveExcludedReferenceQuery(dialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string, foreignKey schema.ForeignKey) string {
	assignments := buildAliasedAssignmentList(dialect, "source", "current", foreignKey.Columns)
	where := buildExcludedReferenceMissingCondition(dialect, targetTable, tempTableName, primaryKey, foreignKey)
	switch dialect.(type) {
	case mysqlDialect:
		return fmt.Sprintf("update %s as source join %s as current on %s set %s where %s", dialect.quote(tempTableName), qualifyTable(dialect, targetTable), buildTableJoinCondition(dialect, "source", "current", primaryKey), strings.Join(assignments, ", "), where)
	default:
		return fmt.Sprintf("update %s as source set %s from %s as current where %s and %s", dialect.quote(tempTableName), strings.Join(stripAssignmentLeftHand(assignments), ", "), qualifyTable(dialect, targetTable), buildTableJoinCondition(dialect, "source", "current", primaryKey), where)
	}
}

func buildNullExcludedReferenceQuery(dialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string, foreignKey schema.ForeignKey) string {
	assignments := make([]string, 0, len(foreignKey.Columns))
	for _, column := range foreignKey.Columns {
		assignments = append(assignments, fmt.Sprintf("%s = NULL", dialect.quote(column)))
	}
	where := buildExcludedReferenceMissingCondition(dialect, targetTable, tempTableName, primaryKey, foreignKey) + " and not exists (select 1 from " + qualifyTable(dialect, targetTable) + " as current where " + buildTableJoinCondition(dialect, "current", "source", primaryKey) + ")"
	return fmt.Sprintf("update %s as source set %s where %s", dialect.quote(tempTableName), strings.Join(assignments, ", "), where)
}

func buildCountInvalidExcludedReferenceQuery(dialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string, foreignKey schema.ForeignKey) string {
	where := buildExcludedReferenceMissingCondition(dialect, targetTable, tempTableName, primaryKey, foreignKey) + " and not exists (select 1 from " + qualifyTable(dialect, targetTable) + " as current where " + buildTableJoinCondition(dialect, "current", "source", primaryKey) + ")"
	return fmt.Sprintf("select count(*) from %s as source where %s", dialect.quote(tempTableName), where)
}

func buildExcludedReferenceMissingCondition(dialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string, foreignKey schema.ForeignKey) string {
	_ = targetTable
	_ = tempTableName
	parts := []string{buildColumnsNotNullCondition(dialect, "source", foreignKey.Columns)}
	parts = append(parts, fmt.Sprintf("not exists (select 1 from %s as excluded where %s)", qualifyTable(dialect, foreignKey.ReferencedTable), buildForeignKeyJoinCondition(dialect, "source", "excluded", foreignKey)))
	return strings.Join(parts, " and ")
}

func buildColumnsNotNullCondition(dialect dialect, alias string, columns []string) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, fmt.Sprintf("%s.%s is not null", dialect.quote(alias), dialect.quote(column)))
	}
	return strings.Join(parts, " and ")
}

func buildForeignKeyJoinCondition(dialect dialect, localAlias string, referencedAlias string, foreignKey schema.ForeignKey) string {
	parts := make([]string, 0, len(foreignKey.Columns))
	for index, column := range foreignKey.Columns {
		parts = append(parts, fmt.Sprintf("%s.%s = %s.%s", dialect.quote(localAlias), dialect.quote(column), dialect.quote(referencedAlias), dialect.quote(foreignKey.ReferencedColumns[index])))
	}
	return strings.Join(parts, " and ")
}

func buildTableJoinCondition(dialect dialect, leftAlias string, rightAlias string, columns []string) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, fmt.Sprintf("%s.%s = %s.%s", dialect.quote(leftAlias), dialect.quote(column), dialect.quote(rightAlias), dialect.quote(column)))
	}
	return strings.Join(parts, " and ")
}

func buildAliasedAssignmentList(dialect dialect, leftAlias string, rightAlias string, columns []string) []string {
	assignments := make([]string, 0, len(columns))
	for _, column := range columns {
		assignments = append(assignments, fmt.Sprintf("%s.%s = %s.%s", dialect.quote(leftAlias), dialect.quote(column), dialect.quote(rightAlias), dialect.quote(column)))
	}
	return assignments
}

func stripAssignmentLeftHand(assignments []string) []string {
	result := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		parts := strings.SplitN(assignment, " = ", 2)
		if len(parts) != 2 {
			continue
		}
		result = append(result, parts[0][strings.LastIndex(parts[0], ".")+1:]+" = "+parts[1])
	}
	return result
}

func nextTempTableName(prefix string, tableID schema.TableID) string {
	name := tableID.Name
	if name == "" {
		name = tableID.String()
	}
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, name)
	if clean == "" {
		clean = "table"
	}
	return fmt.Sprintf("%s_%s_%d", prefix, clean, atomic.AddUint64(&mirrorDeleteTempCounter, 1))
}

func resolveConfiguredTableIDs(available []schema.TableID, configured []string, modeName string) ([]schema.TableID, error) {
	resolved := make([]schema.TableID, 0, len(configured))
	seen := map[schema.TableID]struct{}{}
	for _, value := range configured {
		id, err := resolveConfiguredTableID(available, value, modeName)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		resolved = append(resolved, id)
	}
	return resolved, nil
}

func resolveConfiguredTableID(available []schema.TableID, value string, modeName string) (schema.TableID, error) {
	id := schema.ParseTableID(value)
	for _, candidate := range available {
		if candidate == id {
			return candidate, nil
		}
	}
	if id.Schema != "" {
		return schema.TableID{}, fmt.Errorf("%s table %q must be explicitly selected", modeName, value)
	}
	matches := make([]schema.TableID, 0)
	for _, candidate := range available {
		if candidate.Name == id.Name {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return schema.TableID{}, fmt.Errorf("%s table %q must be explicitly selected", modeName, value)
	case 1:
		return matches[0], nil
	default:
		return schema.TableID{}, fmt.Errorf("%s table %q is ambiguous; qualify one of: %s", modeName, value, strings.Join(schema.SelectionStrings(matches), ", "))
	}
}

func createMirrorDeleteTempTable(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, columns []schema.Column) error {
	return createTempTable(ctx, targetDB, targetDialect, targetTable, tempTableName, columns, columnNames(columns), "mirror delete")
}

func createReplaceTempTable(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, columns []schema.Column, primaryKey []string) error {
	return createTempTable(ctx, targetDB, targetDialect, targetTable, tempTableName, columns, primaryKey, "replace")
}

func createTempTable(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, columns []schema.Column, indexColumns []string, purpose string) error {
	if _, err := targetDB.ExecContext(ctx, buildCreateTempTableQuery(targetDialect, targetTable, tempTableName, columns)); err != nil {
		return fmt.Errorf("create %s temp table for %s: %w", purpose, targetTable.String(), err)
	}
	if len(indexColumns) == 0 {
		return nil
	}
	if _, err := targetDB.ExecContext(ctx, buildCreateTempIndexQuery(targetDialect, tempTableName, indexColumns)); err != nil {
		_ = dropMirrorDeleteTempTable(context.Background(), targetDB, targetDialect, tempTableName)
		return fmt.Errorf("index %s temp table for %s: %w", purpose, targetTable.String(), err)
	}
	return nil
}

func insertMirrorDeleteRows(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, tempTableName string, columns []schema.Column, rows [][]any) error {
	query, args := buildInsertBatchQuery(targetDialect, schema.TableID{Name: tempTableName}, columns, rows)
	if _, err := targetDB.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("stage mirror delete rows in %s: %w", tempTableName, err)
	}
	return nil
}

func stageSourceRows(ctx context.Context, sourceDB *sql.DB, sourceDialect dialect, sourceTable schema.Table, targetDB sqlExecutor, targetDialect dialect, tempTableName string, columns []schema.Column) (int, error) {
	rows, err := sourceDB.QueryContext(ctx, buildSelectQuery(sourceDialect, sourceTable.ID, columns, sourceTable.PrimaryKey.Columns))
	if err != nil {
		return 0, fmt.Errorf("query source table %s: %w", sourceTable.ID.String(), err)
	}
	defer rows.Close()
	rowCount := 0
	buffer := make([][]any, 0, deleteBatchSize)
	for rows.Next() {
		values, err := scanRowValues(rows, columns)
		if err != nil {
			return 0, fmt.Errorf("scan source row for %s: %w", sourceTable.ID.String(), err)
		}
		rowCount++
		buffer = append(buffer, values)
		if len(buffer) == deleteBatchSize {
			if err := insertMirrorDeleteRows(ctx, targetDB, targetDialect, tempTableName, columns, buffer); err != nil {
				return 0, err
			}
			buffer = buffer[:0]
		}
	}
	if len(buffer) > 0 {
		if err := insertMirrorDeleteRows(ctx, targetDB, targetDialect, tempTableName, columns, buffer); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate source rows for %s: %w", sourceTable.ID.String(), err)
	}
	return rowCount, nil
}

func countMissingStageRows(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string) (int, error) {
	var count int
	if err := targetDB.QueryRowContext(ctx, buildCountMissingStageRowsQuery(targetDialect, targetTable, tempTableName, primaryKey)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count missing rows for %s: %w", targetTable.String(), err)
	}
	return count, nil
}

func countChangedStageRows(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, updateColumns []schema.Column, primaryKey []string) (int, error) {
	if len(updateColumns) == 0 {
		return 0, nil
	}
	var count int
	if err := targetDB.QueryRowContext(ctx, buildCountChangedStageRowsQuery(targetDialect, targetTable, tempTableName, updateColumns, primaryKey)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count changed rows for %s: %w", targetTable.String(), err)
	}
	return count, nil
}

func insertMissingStageRows(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, columns []schema.Column, primaryKey []string) (int, error) {
	result, err := targetDB.ExecContext(ctx, buildInsertMissingStageRowsQuery(targetDialect, targetTable, tempTableName, columns, primaryKey))
	if err != nil {
		return 0, fmt.Errorf("insert missing rows into %s: %w", targetTable.String(), err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(inserted), nil
}

func updateChangedStageRows(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, updateColumns []schema.Column, primaryKey []string) (int, error) {
	if len(updateColumns) == 0 {
		return 0, nil
	}
	result, err := targetDB.ExecContext(ctx, buildUpdateChangedStageRowsQuery(targetDialect, targetTable, tempTableName, updateColumns, primaryKey))
	if err != nil {
		return 0, fmt.Errorf("update changed rows in %s: %w", targetTable.String(), err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(updated), nil
}

func countMirrorDeleteRows(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, targetTable schema.TableID, tempTableName string, primaryKey []string) (int, error) {
	var count int
	if err := targetDB.QueryRowContext(ctx, buildCountMirrorDeleteQuery(targetDialect, targetTable, tempTableName, primaryKey)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count mirror delete rows for %s: %w", targetTable.String(), err)
	}
	return count, nil
}

func dropMirrorDeleteTempTable(ctx context.Context, targetDB sqlExecutor, targetDialect dialect, tempTableName string) error {
	_, err := targetDB.ExecContext(ctx, fmt.Sprintf("drop table if exists %s", targetDialect.quote(tempTableName)))
	return err
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
