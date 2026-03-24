package schema

import "db-sync/internal/model"

type SnapshotIdentity struct {
	Role   string
	Engine model.Engine
}

type DriftReport struct {
	Source   SnapshotIdentity
	Target   SnapshotIdentity
	Tables   []TableDrift
	Warnings []DriftReason
	Blockers []DriftReason
}

func NewDriftReport(source Snapshot, target Snapshot, tables []TableDrift, warnings []DriftReason, blockers []DriftReason) DriftReport {
	return DriftReport{
		Source:   SnapshotIdentity{Role: source.Role, Engine: source.Engine},
		Target:   SnapshotIdentity{Role: target.Role, Engine: target.Engine},
		Tables:   append([]TableDrift(nil), tables...),
		Warnings: append([]DriftReason(nil), warnings...),
		Blockers: append([]DriftReason(nil), blockers...),
	}
}

func (report DriftReport) TableByID(id TableID) (TableDrift, bool) {
	for _, table := range report.Tables {
		if table.TableID == id {
			return table, true
		}
	}
	return TableDrift{}, false
}

func (table TableDrift) ColumnByName(name string) (ColumnDrift, bool) {
	for _, column := range table.ColumnDrifts {
		if column.Column == name {
			return column, true
		}
	}
	return ColumnDrift{}, false
}
