package schema

import (
	"testing"

	"db-sync/internal/model"
)

func TestBuildDependencyGraph(t *testing.T) {
	snapshot := Snapshot{
		Role:   "source",
		Engine: model.EnginePostgres,
		Tables: []Table{
			{
				ID:      TableID{Schema: "public", Name: "customers"},
				Columns: []Column{{Name: "tenant_id", Ordinal: 1}, {Name: "id", Ordinal: 2}},
			},
			{
				ID:      TableID{Schema: "public", Name: "orders"},
				Columns: []Column{{Name: "tenant_id", Ordinal: 1}, {Name: "customer_id", Ordinal: 2}},
				ForeignKeys: []ForeignKey{{
					Name:              "orders_customer_fk",
					Columns:           []string{"tenant_id", "customer_id"},
					ReferencedTable:   TableID{Schema: "public", Name: "customers"},
					ReferencedColumns: []string{"tenant_id", "id"},
				}},
			},
		},
	}

	graph := BuildDependencyGraph(snapshot)
	dependencies := graph.DependenciesFor(TableID{Schema: "public", Name: "orders"})
	if len(dependencies) != 1 {
		t.Fatalf("dependencies = %d, want 1", len(dependencies))
	}
	dependency := dependencies[0]
	if dependency.From != (TableID{Schema: "public", Name: "orders"}) {
		t.Fatalf("dependency.From = %s, want public.orders", dependency.From.String())
	}
	if dependency.To != (TableID{Schema: "public", Name: "customers"}) {
		t.Fatalf("dependency.To = %s, want public.customers", dependency.To.String())
	}
	if got, want := dependency.Columns, []string{"tenant_id", "customer_id"}; !sameStrings(got, want) {
		t.Fatalf("dependency.Columns = %v, want %v", got, want)
	}
	if got, want := dependency.ReferencedColumns, []string{"tenant_id", "id"}; !sameStrings(got, want) {
		t.Fatalf("dependency.ReferencedColumns = %v, want %v", got, want)
	}
}

func TestDependencyClosure(t *testing.T) {
	graph := BuildDependencyGraph(Snapshot{
		Role:   "source",
		Engine: model.EnginePostgres,
		Tables: []Table{
			{ID: TableID{Schema: "public", Name: "customers"}},
			{ID: TableID{Schema: "public", Name: "orders"}, ForeignKeys: []ForeignKey{{Name: "orders_customer_fk", Columns: []string{"customer_id"}, ReferencedTable: TableID{Schema: "public", Name: "customers"}, ReferencedColumns: []string{"id"}}}},
			{ID: TableID{Schema: "public", Name: "order_items"}, ForeignKeys: []ForeignKey{{Name: "order_items_order_fk", Columns: []string{"order_id"}, ReferencedTable: TableID{Schema: "public", Name: "orders"}, ReferencedColumns: []string{"id"}}}},
			{ID: TableID{Schema: "public", Name: "warehouses"}},
		},
	})

	closure := graph.Closure([]TableID{{Schema: "public", Name: "order_items"}})
	if got, want := SelectionStrings(closure), []string{"public.customers", "public.orders", "public.order_items"}; !sameStrings(got, want) {
		t.Fatalf("closure = %v, want %v", got, want)
	}

	preview, err := PreviewSelection(graph, []string{"public.order_items"}, nil)
	if err != nil {
		t.Fatalf("PreviewSelection() error = %v", err)
	}
	if got, want := SelectionStrings(preview.RequiredTables), []string{"public.customers", "public.orders"}; !sameStrings(got, want) {
		t.Fatalf("RequiredTables = %v, want %v", got, want)
	}
	if got, want := SelectionStrings(preview.FinalTables), []string{"public.customers", "public.orders", "public.order_items"}; !sameStrings(got, want) {
		t.Fatalf("FinalTables = %v, want %v", got, want)
	}
}

func TestPreviewSelectionResolvesBareNamesAgainstQualifiedTables(t *testing.T) {
	graph := BuildDependencyGraph(Snapshot{
		Role:   "source",
		Engine: model.EngineMariaDB,
		Tables: []Table{
			{ID: TableID{Schema: "db", Name: "customer"}},
			{ID: TableID{Schema: "db", Name: "order"}, ForeignKeys: []ForeignKey{{Name: "order_customer_fk", Columns: []string{"customer_id"}, ReferencedTable: TableID{Schema: "db", Name: "customer"}, ReferencedColumns: []string{"id"}}}},
		},
	})

	preview, err := PreviewSelection(graph, []string{"order"}, nil)
	if err != nil {
		t.Fatalf("PreviewSelection() error = %v", err)
	}
	if got, want := SelectionStrings(preview.ExplicitIncludes), []string{"db.order"}; !sameStrings(got, want) {
		t.Fatalf("ExplicitIncludes = %v, want %v", got, want)
	}
	if got, want := SelectionStrings(preview.RequiredTables), []string{"db.customer"}; !sameStrings(got, want) {
		t.Fatalf("RequiredTables = %v, want %v", got, want)
	}
}

func TestPreviewSelectionRejectsAmbiguousBareNames(t *testing.T) {
	graph := BuildDependencyGraph(Snapshot{
		Role:   "source",
		Engine: model.EngineMariaDB,
		Tables: []Table{
			{ID: TableID{Schema: "db1", Name: "customer"}},
			{ID: TableID{Schema: "db2", Name: "customer"}},
		},
	})

	if _, err := PreviewSelection(graph, []string{"customer"}, nil); err == nil {
		t.Fatal("PreviewSelection() error = nil, want ambiguous selection error")
	}
}

func TestPreviewSelectionHardExcludesTables(t *testing.T) {
	graph := BuildDependencyGraph(Snapshot{
		Role:   "source",
		Engine: model.EngineMariaDB,
		Tables: []Table{
			{ID: TableID{Schema: "db", Name: "customer"}},
			{ID: TableID{Schema: "db", Name: "plugin"}},
			{ID: TableID{Schema: "db", Name: "order"}, ForeignKeys: []ForeignKey{
				{Name: "order_customer_fk", Columns: []string{"customer_id"}, ReferencedTable: TableID{Schema: "db", Name: "customer"}, ReferencedColumns: []string{"id"}},
				{Name: "order_plugin_fk", Columns: []string{"plugin_id"}, ReferencedTable: TableID{Schema: "db", Name: "plugin"}, ReferencedColumns: []string{"id"}},
			}},
		},
	})

	preview, err := PreviewSelection(graph, []string{"order"}, []string{"plugin"})
	if err != nil {
		t.Fatalf("PreviewSelection() error = %v", err)
	}
	if got, want := SelectionStrings(preview.ExplicitIncludes), []string{"db.order"}; !sameStrings(got, want) {
		t.Fatalf("ExplicitIncludes = %v, want %v", got, want)
	}
	if got, want := SelectionStrings(preview.RequiredTables), []string{"db.customer"}; !sameStrings(got, want) {
		t.Fatalf("RequiredTables = %v, want %v", got, want)
	}
	if got, want := SelectionStrings(preview.FinalTables), []string{"db.customer", "db.order"}; !sameStrings(got, want) {
		t.Fatalf("FinalTables = %v, want %v", got, want)
	}
	for _, value := range preview.FinalTables {
		if value == (TableID{Schema: "db", Name: "plugin"}) {
			t.Fatal("FinalTables contains excluded table db.plugin")
		}
	}
}
