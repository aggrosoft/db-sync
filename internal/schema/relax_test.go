package schema

import "testing"

func TestRelaxReportForSelectionDowngradesImplicitInsertOnlyRisk(t *testing.T) {
	tableID := TableID{Name: "authors"}
	report := NewDriftReport(
		Snapshot{},
		Snapshot{},
		[]TableDrift{{
			TableID: tableID,
			ColumnDrifts: []ColumnDrift{{
				Column: "display_name",
				Blockers: []DriftReason{{
					Code:     ReasonIncompatibleType,
					TableID:  tableID,
					Column:   "display_name",
					Detail:   "source type is incompatible with target",
					Blocking: true,
				}, {
					Code:     ReasonIncompatibleNullability,
					TableID:  tableID,
					Column:   "display_name",
					Detail:   "source allows nulls but target does not",
					Blocking: true,
				}},
			}},
			Blockers: []DriftReason{{
				Code:     ReasonIncompatibleType,
				TableID:  tableID,
				Column:   "display_name",
				Detail:   "source type is incompatible with target",
				Blocking: true,
			}, {
				Code:     ReasonIncompatibleNullability,
				TableID:  tableID,
				Column:   "display_name",
				Detail:   "source allows nulls but target does not",
				Blocking: true,
			}},
			Classification: ClassificationBlocked,
		}},
		nil,
		[]DriftReason{{
			Code:     ReasonIncompatibleType,
			TableID:  tableID,
			Column:   "display_name",
			Detail:   "source type is incompatible with target",
			Blocking: true,
		}, {
			Code:     ReasonIncompatibleNullability,
			TableID:  tableID,
			Column:   "display_name",
			Detail:   "source allows nulls but target does not",
			Blocking: true,
		}},
	)

	relaxed := RelaxReportForSelection(SelectionPreview{RequiredTables: []TableID{tableID}}, report)
	authors, ok := relaxed.TableByID(tableID)
	if !ok {
		t.Fatal("expected authors table drift")
	}
	if authors.Classification != ClassificationWritableWithWarning {
		t.Fatalf("classification = %q, want %q", authors.Classification, ClassificationWritableWithWarning)
	}
	if len(authors.Blockers) != 0 {
		t.Fatalf("Blockers = %v, want none", authors.Blockers)
	}
	if len(authors.Warnings) != 2 {
		t.Fatalf("Warnings = %v, want 2 warnings", authors.Warnings)
	}
	if len(relaxed.Blockers) != 0 {
		t.Fatalf("report blockers = %v, want none", relaxed.Blockers)
	}
	if len(relaxed.Warnings) != 2 {
		t.Fatalf("report warnings = %v, want 2 warnings", relaxed.Warnings)
	}
	column, ok := authors.ColumnByName("display_name")
	if !ok {
		t.Fatal("expected display_name column drift")
	}
	if len(column.Blockers) != 0 {
		t.Fatalf("column blockers = %v, want none", column.Blockers)
	}
	if len(column.Warnings) != 2 {
		t.Fatalf("column warnings = %v, want 2 warnings", column.Warnings)
	}
}
