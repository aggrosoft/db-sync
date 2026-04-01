package sync

import (
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/schema"
)

func TestSelfReferencesSatisfiedBlocksUntilParentExists(t *testing.T) {
	columns := []schema.Column{
		{Name: "id", Ordinal: 1},
		{Name: "version_id", Ordinal: 2},
		{Name: "parent_id", Ordinal: 3},
		{Name: "parent_version_id", Ordinal: 4},
	}
	foreignKeys := []schema.ForeignKey{{
		Name:              "fk.category.parent_id",
		Columns:           []string{"parent_id", "parent_version_id"},
		ReferencedTable:   schema.TableID{Name: "category"},
		ReferencedColumns: []string{"id", "version_id"},
	}}
	values := []any{"child", "live", "parent", "live"}

	if selfReferencesSatisfied(values, columns, []string{"id", "version_id"}, foreignKeys, map[string][]any{}) {
		t.Fatal("selfReferencesSatisfied() = true, want false when parent row is missing")
	}
	if !selfReferencesSatisfied(values, columns, []string{"id", "version_id"}, foreignKeys, map[string][]any{"parent|live": {"parent", "live", nil, nil}}) {
		t.Fatal("selfReferencesSatisfied() = false, want true when parent row already exists")
	}
}

func TestSelfReferencesSatisfiedAllowsNullParent(t *testing.T) {
	columns := []schema.Column{
		{Name: "id", Ordinal: 1},
		{Name: "version_id", Ordinal: 2},
		{Name: "parent_id", Ordinal: 3},
		{Name: "parent_version_id", Ordinal: 4},
	}
	foreignKeys := []schema.ForeignKey{{
		Name:              "fk.category.parent_id",
		Columns:           []string{"parent_id", "parent_version_id"},
		ReferencedTable:   schema.TableID{Name: "category"},
		ReferencedColumns: []string{"id", "version_id"},
	}}
	values := []any{"root", "live", nil, nil}

	if !selfReferencesSatisfied(values, columns, []string{"id", "version_id"}, foreignKeys, map[string][]any{}) {
		t.Fatal("selfReferencesSatisfied() = false, want true for root rows without parent")
	}
}

func TestShouldEnforceSelfReferenceOrdering(t *testing.T) {
	if shouldEnforceSelfReferenceOrdering(false, model.EngineMariaDB) {
		t.Fatal("shouldEnforceSelfReferenceOrdering(false, mariadb) = true, want false")
	}
	if shouldEnforceSelfReferenceOrdering(false, model.EngineMySQL) {
		t.Fatal("shouldEnforceSelfReferenceOrdering(false, mysql) = true, want false")
	}
	if !shouldEnforceSelfReferenceOrdering(true, model.EngineMariaDB) {
		t.Fatal("shouldEnforceSelfReferenceOrdering(true, mariadb) = false, want true")
	}
	if !shouldEnforceSelfReferenceOrdering(false, model.EnginePostgres) {
		t.Fatal("shouldEnforceSelfReferenceOrdering(false, postgres) = false, want true")
	}
}
