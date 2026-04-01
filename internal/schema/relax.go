package schema

func RelaxReportForSelection(preview SelectionPreview, report DriftReport) DriftReport {
	implicitSet := make(map[TableID]struct{}, len(preview.RequiredTables))
	for _, tableID := range preview.RequiredTables {
		implicitSet[tableID] = struct{}{}
	}
	if len(implicitSet) == 0 {
		return report
	}
	tables := make([]TableDrift, 0, len(report.Tables))
	warnings := make([]DriftReason, 0, len(report.Warnings))
	blockers := make([]DriftReason, 0, len(report.Blockers))
	for _, table := range report.Tables {
		if _, ok := implicitSet[table.TableID]; ok {
			table = relaxImplicitInsertOnlyTable(table)
		}
		tables = append(tables, table)
		warnings = append(warnings, table.Warnings...)
		blockers = append(blockers, table.Blockers...)
	}
	report.Tables = tables
	report.Warnings = warnings
	report.Blockers = blockers
	return report
}

func relaxImplicitInsertOnlyTable(table TableDrift) TableDrift {
	table.ColumnDrifts = append([]ColumnDrift(nil), table.ColumnDrifts...)
	table.Warnings = append([]DriftReason(nil), table.Warnings...)
	table.Blockers = append([]DriftReason(nil), table.Blockers...)
	for columnIndex, column := range table.ColumnDrifts {
		keptBlockers := make([]DriftReason, 0, len(column.Blockers))
		movedWarnings := make([]DriftReason, 0)
		for _, reason := range column.Blockers {
			if canRelaxImplicitInsertOnly(reason) {
				reason.Blocking = false
				movedWarnings = append(movedWarnings, reason)
				continue
			}
			keptBlockers = append(keptBlockers, reason)
		}
		if len(movedWarnings) > 0 {
			table.ColumnDrifts[columnIndex].Warnings = append(table.ColumnDrifts[columnIndex].Warnings, movedWarnings...)
		}
		table.ColumnDrifts[columnIndex].Blockers = keptBlockers
	}
	table.Warnings = nil
	table.Blockers = nil
	for _, column := range table.ColumnDrifts {
		table.Warnings = append(table.Warnings, column.Warnings...)
		table.Blockers = append(table.Blockers, column.Blockers...)
	}
	table.Classification = ClassifyTableDrift(table)
	return table
}

func canRelaxImplicitInsertOnly(reason DriftReason) bool {
	switch reason.Code {
	case ReasonIncompatibleType, ReasonIncompatibleNullability:
		return true
	default:
		return false
	}
}
