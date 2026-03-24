package schema

import (
	"fmt"
	"sort"
	"strings"
)

type Classification string

const (
	ClassificationWritable            Classification = "writable"
	ClassificationWritableWithWarning Classification = "writable-with-warning"
	ClassificationBlocked             Classification = "blocked"
)

type DriftReasonCode string

const (
	ReasonMissingTargetTable      DriftReasonCode = "missing-target-table"
	ReasonExtraTargetTable        DriftReasonCode = "extra-target-table"
	ReasonSkippedSourceColumn     DriftReasonCode = "skipped-source-column"
	ReasonExtraTargetColumn       DriftReasonCode = "extra-target-column"
	ReasonIncompatibleType        DriftReasonCode = "incompatible-type"
	ReasonIncompatibleNullability DriftReasonCode = "incompatible-nullability"
	ReasonIncompatiblePrimaryKey  DriftReasonCode = "incompatible-primary-key"
)

type DriftReason struct {
	Code     DriftReasonCode
	TableID  TableID
	Column   string
	Detail   string
	Blocking bool
}

type ColumnDrift struct {
	Column   string
	Source   *Column
	Target   *Column
	Warnings []DriftReason
	Blockers []DriftReason
}

type TableDrift struct {
	TableID        TableID
	Classification Classification
	SkippedColumns []string
	ColumnDrifts   []ColumnDrift
	Warnings       []DriftReason
	Blockers       []DriftReason
}

func CompareSnapshots(source Snapshot, target Snapshot) DriftReport {
	source = NormalizeSnapshot(source)
	target = NormalizeSnapshot(target)

	ids := unionTableIDs(source, target)
	tables := make([]TableDrift, 0, len(ids))
	warnings := make([]DriftReason, 0)
	blockers := make([]DriftReason, 0)

	for _, id := range ids {
		sourceTable, hasSource := source.TableByID(id)
		targetTable, hasTarget := target.TableByID(id)
		table := compareTable(id, sourceTable, hasSource, targetTable, hasTarget)
		tables = append(tables, table)
		warnings = append(warnings, table.Warnings...)
		blockers = append(blockers, table.Blockers...)
	}

	return NewDriftReport(source, target, tables, warnings, blockers)
}

func ClassifyTableDrift(table TableDrift) Classification {
	if len(table.Blockers) > 0 {
		return ClassificationBlocked
	}
	if len(table.Warnings) > 0 {
		return ClassificationWritableWithWarning
	}
	return ClassificationWritable
}

func compareTable(id TableID, source Table, hasSource bool, target Table, hasTarget bool) TableDrift {
	table := TableDrift{TableID: id}
	columns := map[string]*ColumnDrift{}

	if hasSource && !hasTarget {
		table.addTableReason(DriftReason{
			Code:     ReasonMissingTargetTable,
			TableID:  id,
			Detail:   fmt.Sprintf("target table %s is missing", id.String()),
			Blocking: true,
		})
		table.Classification = ClassifyTableDrift(table)
		return table
	}

	if !hasSource && hasTarget {
		table.addTableReason(DriftReason{
			Code:     ReasonExtraTargetTable,
			TableID:  id,
			Detail:   fmt.Sprintf("target table %s has no source counterpart", id.String()),
			Blocking: false,
		})
		table.Classification = ClassifyTableDrift(table)
		return table
	}

	sourceColumns := columnIndex(source.Columns)
	targetColumns := columnIndex(target.Columns)

	for _, sourceColumn := range source.Columns {
		targetColumn, ok := targetColumns[sourceColumn.Name]
		if !ok {
			reason := DriftReason{
				Code:     ReasonSkippedSourceColumn,
				TableID:  id,
				Column:   sourceColumn.Name,
				Detail:   fmt.Sprintf("source column %s.%s has no writable target mapping and will be skipped", id.String(), sourceColumn.Name),
				Blocking: false,
			}
			table.SkippedColumns = append(table.SkippedColumns, sourceColumn.Name)
			table.addColumnReason(columns, sourceColumn.Name, cloneColumn(sourceColumn), nil, reason)
			continue
		}

		if mismatchedColumnType(sourceColumn, targetColumn) {
			table.addColumnReason(columns, sourceColumn.Name, cloneColumn(sourceColumn), cloneColumn(targetColumn), DriftReason{
				Code:     ReasonIncompatibleType,
				TableID:  id,
				Column:   sourceColumn.Name,
				Detail:   fmt.Sprintf("source column %s.%s type %q is incompatible with target type %q", id.String(), sourceColumn.Name, sourceColumn.NativeType, targetColumn.NativeType),
				Blocking: true,
			})
		}

		if sourceColumn.Nullable && !targetColumn.Nullable {
			table.addColumnReason(columns, sourceColumn.Name, cloneColumn(sourceColumn), cloneColumn(targetColumn), DriftReason{
				Code:     ReasonIncompatibleNullability,
				TableID:  id,
				Column:   sourceColumn.Name,
				Detail:   fmt.Sprintf("source column %s.%s allows NULL but target column does not", id.String(), sourceColumn.Name),
				Blocking: true,
			})
		}
	}

	for _, targetColumn := range target.Columns {
		if _, ok := sourceColumns[targetColumn.Name]; ok {
			continue
		}

		reason := DriftReason{
			Code:    ReasonExtraTargetColumn,
			TableID: id,
			Column:  targetColumn.Name,
		}
		if targetColumn.Nullable || targetColumn.HasProvenDefault || targetColumn.Identity || targetColumn.Generated {
			reason.Detail = fmt.Sprintf("target column %s.%s has no source value but remains writable because it is nullable, default-backed, identity, or generated", id.String(), targetColumn.Name)
			reason.Blocking = false
		} else {
			reason.Detail = fmt.Sprintf("target column %s.%s has no source value and is required because it is non-nullable without a proven default, identity, or generated value", id.String(), targetColumn.Name)
			reason.Blocking = true
		}
		table.addColumnReason(columns, targetColumn.Name, nil, cloneColumn(targetColumn), reason)
	}

	if !sameStrings(source.PrimaryKey.Columns, target.PrimaryKey.Columns) {
		table.addTableReason(DriftReason{
			Code:     ReasonIncompatiblePrimaryKey,
			TableID:  id,
			Detail:   fmt.Sprintf("primary key columns differ for %s: source=%s target=%s", id.String(), strings.Join(source.PrimaryKey.Columns, ","), strings.Join(target.PrimaryKey.Columns, ",")),
			Blocking: true,
		})
	}

	table.ColumnDrifts = flattenColumnDrifts(columns)
	sort.Strings(table.SkippedColumns)
	table.Classification = ClassifyTableDrift(table)
	return table
}

func (table *TableDrift) addTableReason(reason DriftReason) {
	if reason.Blocking {
		table.Blockers = append(table.Blockers, reason)
		return
	}
	table.Warnings = append(table.Warnings, reason)
}

func (table *TableDrift) addColumnReason(columns map[string]*ColumnDrift, name string, source *Column, target *Column, reason DriftReason) {
	column := ensureColumnDrift(columns, name)
	if column.Source == nil && source != nil {
		column.Source = source
	}
	if column.Target == nil && target != nil {
		column.Target = target
	}
	if reason.Blocking {
		column.Blockers = append(column.Blockers, reason)
		table.Blockers = append(table.Blockers, reason)
		return
	}
	column.Warnings = append(column.Warnings, reason)
	table.Warnings = append(table.Warnings, reason)
}

func ensureColumnDrift(columns map[string]*ColumnDrift, name string) *ColumnDrift {
	column, ok := columns[name]
	if ok {
		return column
	}
	column = &ColumnDrift{Column: name}
	columns[name] = column
	return column
}

func flattenColumnDrifts(columns map[string]*ColumnDrift) []ColumnDrift {
	result := make([]ColumnDrift, 0, len(columns))
	for _, column := range columns {
		sortReasons(column.Warnings)
		sortReasons(column.Blockers)
		result = append(result, *column)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Column < result[j].Column
	})
	return result
}

func sortReasons(reasons []DriftReason) {
	sort.Slice(reasons, func(i, j int) bool {
		if reasons[i].Column == reasons[j].Column {
			return reasons[i].Code < reasons[j].Code
		}
		return reasons[i].Column < reasons[j].Column
	})
}

func unionTableIDs(source Snapshot, target Snapshot) []TableID {
	seen := map[string]TableID{}
	for _, table := range source.Tables {
		seen[table.ID.String()] = table.ID
	}
	for _, table := range target.Tables {
		seen[table.ID.String()] = table.ID
	}
	ids := make([]TableID, 0, len(seen))
	for _, id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
	return ids
}

func columnIndex(columns []Column) map[string]Column {
	indexed := make(map[string]Column, len(columns))
	for _, column := range columns {
		indexed[column.Name] = column
	}
	return indexed
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneColumn(column Column) *Column {
	copy := column
	return &copy
}

func mismatchedColumnType(source Column, target Column) bool {
	if canonicalType(source.DataType) != canonicalType(target.DataType) {
		return true
	}
	if canonicalType(source.NativeType) != canonicalType(target.NativeType) {
		return true
	}
	return false
}

func canonicalType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
