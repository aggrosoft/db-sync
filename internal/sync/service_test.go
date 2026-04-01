package sync

import (
	"testing"

	"db-sync/internal/schema"
)

func TestSharedWritableColumnsSkipsNonPrimaryIdentityColumns(t *testing.T) {
	sourceTable := schema.Table{
		ID: schema.TableID{Name: "category"},
		Columns: []schema.Column{
			{Name: "id", Ordinal: 1, Writable: true},
			{Name: "auto_increment", Ordinal: 2, Writable: true, Identity: true},
			{Name: "name", Ordinal: 3, Writable: true},
		},
		PrimaryKey: schema.PrimaryKey{Columns: []string{"id"}},
	}
	targetTable := sourceTable

	columns, err := sharedWritableColumns(sourceTable, targetTable)
	if err != nil {
		t.Fatalf("sharedWritableColumns() error = %v", err)
	}
	if got, want := columnNames(columns), []string{"id", "name"}; !sameColumnNames(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
}

func TestSharedWritableColumnsKeepsPrimaryKeyIdentityColumns(t *testing.T) {
	sourceTable := schema.Table{
		ID: schema.TableID{Name: "accounts"},
		Columns: []schema.Column{
			{Name: "id", Ordinal: 1, Writable: true, Identity: true},
			{Name: "email", Ordinal: 2, Writable: true},
		},
		PrimaryKey: schema.PrimaryKey{Columns: []string{"id"}},
	}
	targetTable := sourceTable

	columns, err := sharedWritableColumns(sourceTable, targetTable)
	if err != nil {
		t.Fatalf("sharedWritableColumns() error = %v", err)
	}
	if got, want := columnNames(columns), []string{"id", "email"}; !sameColumnNames(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
}

func columnNames(columns []schema.Column) []string {
	result := make([]string, 0, len(columns))
	for _, column := range columns {
		result = append(result, column.Name)
	}
	return result
}

func sameColumnNames(left []string, right []string) bool {
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
