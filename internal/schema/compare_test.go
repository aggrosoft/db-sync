package schema

import (
	"testing"

	"db-sync/internal/model"
)

func TestCompareSnapshots(t *testing.T) {
	source := Snapshot{
		Role:   "source",
		Engine: model.EnginePostgres,
		Tables: []Table{{
			ID: TableID{Schema: "public", Name: "users"},
			Columns: []Column{
				{Name: "id", Ordinal: 1, DataType: "bigint", NativeType: "int8"},
				{Name: "email", Ordinal: 2, DataType: "text", NativeType: "text"},
				{Name: "nickname", Ordinal: 3, DataType: "text", NativeType: "text", Nullable: true},
			},
			PrimaryKey: PrimaryKey{Name: "users_pkey", Columns: []string{"id"}},
		}},
	}
	target := Snapshot{
		Role:   "target",
		Engine: model.EnginePostgres,
		Tables: []Table{{
			ID: TableID{Schema: "public", Name: "users"},
			Columns: []Column{
				{Name: "id", Ordinal: 1, DataType: "bigint", NativeType: "int8"},
				{Name: "email", Ordinal: 2, DataType: "integer", NativeType: "int4"},
				{Name: "created_at", Ordinal: 3, DataType: "timestamp", NativeType: "timestamp", Nullable: false},
			},
			PrimaryKey: PrimaryKey{Name: "users_pkey", Columns: []string{"email", "id"}},
		}},
	}

	report := CompareSnapshots(source, target)
	users, ok := report.TableByID(TableID{Schema: "public", Name: "users"})
	if !ok {
		t.Fatal("expected public.users table drift")
	}
	if users.Classification != ClassificationBlocked {
		t.Fatalf("classification = %q, want %q", users.Classification, ClassificationBlocked)
	}
	if !containsString(users.SkippedColumns, "nickname") {
		t.Fatalf("SkippedColumns = %v, want nickname", users.SkippedColumns)
	}

	email, ok := users.ColumnByName("email")
	if !ok {
		t.Fatal("expected email column drift")
	}
	assertReason(t, email.Blockers, ReasonIncompatibleType, TableID{Schema: "public", Name: "users"}, "email")

	createdAt, ok := users.ColumnByName("created_at")
	if !ok {
		t.Fatal("expected created_at column drift")
	}
	assertReason(t, createdAt.Blockers, ReasonExtraTargetColumn, TableID{Schema: "public", Name: "users"}, "created_at")
	assertReason(t, users.Blockers, ReasonIncompatiblePrimaryKey, TableID{Schema: "public", Name: "users"}, "")
	assertReason(t, users.Warnings, ReasonSkippedSourceColumn, TableID{Schema: "public", Name: "users"}, "nickname")
}

func TestClassifyTableDrift(t *testing.T) {
	tests := []struct {
		name  string
		table TableDrift
		want  Classification
	}{
		{
			name:  "writable without reasons",
			table: TableDrift{TableID: TableID{Schema: "public", Name: "users"}},
			want:  ClassificationWritable,
		},
		{
			name: "warning when skipped source columns exist",
			table: TableDrift{
				TableID:        TableID{Schema: "public", Name: "users"},
				SkippedColumns: []string{"nickname"},
				Warnings:       []DriftReason{{Code: ReasonSkippedSourceColumn, TableID: TableID{Schema: "public", Name: "users"}, Column: "nickname"}},
			},
			want: ClassificationWritableWithWarning,
		},
		{
			name: "blocked when required target column is missing source data",
			table: TableDrift{
				TableID:  TableID{Schema: "public", Name: "users"},
				Blockers: []DriftReason{{Code: ReasonExtraTargetColumn, TableID: TableID{Schema: "public", Name: "users"}, Column: "created_at", Blocking: true}},
			},
			want: ClassificationBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyTableDrift(tt.table); got != tt.want {
				t.Fatalf("ClassifyTableDrift() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSkippedColumnsProduceWarnings(t *testing.T) {
	report := CompareSnapshots(
		Snapshot{
			Role:   "source",
			Engine: model.EngineMySQL,
			Tables: []Table{{
				ID: TableID{Name: "accounts"},
				Columns: []Column{
					{Name: "id", Ordinal: 1, DataType: "bigint", NativeType: "bigint"},
					{Name: "legacy_code", Ordinal: 2, DataType: "varchar", NativeType: "varchar(32)"},
				},
				PrimaryKey: PrimaryKey{Columns: []string{"id"}},
			}},
		},
		Snapshot{
			Role:   "target",
			Engine: model.EngineMySQL,
			Tables: []Table{{
				ID:         TableID{Name: "accounts"},
				Columns:    []Column{{Name: "id", Ordinal: 1, DataType: "bigint", NativeType: "bigint"}},
				PrimaryKey: PrimaryKey{Columns: []string{"id"}},
			}},
		},
	)

	accounts, ok := report.TableByID(TableID{Name: "accounts"})
	if !ok {
		t.Fatal("expected accounts table drift")
	}
	if accounts.Classification != ClassificationWritableWithWarning {
		t.Fatalf("classification = %q, want %q", accounts.Classification, ClassificationWritableWithWarning)
	}
	if len(accounts.Blockers) != 0 {
		t.Fatalf("Blockers = %v, want none", accounts.Blockers)
	}
	if got := accounts.SkippedColumns; len(got) != 1 || got[0] != "legacy_code" {
		t.Fatalf("SkippedColumns = %v, want [legacy_code]", got)
	}
	legacy, ok := accounts.ColumnByName("legacy_code")
	if !ok {
		t.Fatal("expected legacy_code column drift")
	}
	assertReason(t, legacy.Warnings, ReasonSkippedSourceColumn, TableID{Name: "accounts"}, "legacy_code")
}

func TestTargetOptionalColumns(t *testing.T) {
	tests := []struct {
		name        string
		targetExtra Column
	}{
		{name: "nullable target column", targetExtra: Column{Name: "nickname", Ordinal: 2, DataType: "varchar", NativeType: "varchar(64)", Nullable: true}},
		{name: "default backed target column", targetExtra: Column{Name: "created_at", Ordinal: 2, DataType: "timestamp", NativeType: "timestamp", HasProvenDefault: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := CompareSnapshots(
				Snapshot{
					Role:   "source",
					Engine: model.EnginePostgres,
					Tables: []Table{{
						ID:         TableID{Schema: "app", Name: "customers"},
						Columns:    []Column{{Name: "id", Ordinal: 1, DataType: "bigint", NativeType: "int8"}},
						PrimaryKey: PrimaryKey{Columns: []string{"id"}},
					}},
				},
				Snapshot{
					Role:   "target",
					Engine: model.EnginePostgres,
					Tables: []Table{{
						ID:         TableID{Schema: "app", Name: "customers"},
						Columns:    []Column{{Name: "id", Ordinal: 1, DataType: "bigint", NativeType: "int8"}, tt.targetExtra},
						PrimaryKey: PrimaryKey{Columns: []string{"id"}},
					}},
				},
			)

			customers, ok := report.TableByID(TableID{Schema: "app", Name: "customers"})
			if !ok {
				t.Fatal("expected app.customers table drift")
			}
			if customers.Classification != ClassificationWritableWithWarning {
				t.Fatalf("classification = %q, want %q", customers.Classification, ClassificationWritableWithWarning)
			}
			if len(customers.Blockers) != 0 {
				t.Fatalf("Blockers = %v, want none", customers.Blockers)
			}
			extra, ok := customers.ColumnByName(tt.targetExtra.Name)
			if !ok {
				t.Fatalf("expected %s column drift", tt.targetExtra.Name)
			}
			assertReason(t, extra.Warnings, ReasonExtraTargetColumn, TableID{Schema: "app", Name: "customers"}, tt.targetExtra.Name)
		})
	}
}

func assertReason(t *testing.T, reasons []DriftReason, code DriftReasonCode, tableID TableID, column string) {
	t.Helper()
	for _, reason := range reasons {
		if reason.Code == code && reason.TableID == tableID && reason.Column == column {
			return
		}
	}
	t.Fatalf("reasons %v missing code=%q table=%s column=%q", reasons, code, tableID.String(), column)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
