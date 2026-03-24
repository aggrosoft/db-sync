package cli

import (
	"bytes"
	"context"
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"
)

func TestDependencySelection(t *testing.T) {
	stdout := &bytes.Buffer{}
	initial := model.DefaultProfile("dependency-flow")
	initial.Source.Engine = model.EnginePostgres
	initial.Target.Engine = model.EnginePostgres

	selected := initial
	selected.Selection.Tables = []string{"public.order_items"}
	selected.Selection.ExcludedTables = []string{"public.orders"}

	wizard := &tableSelectionWizard{
		scriptedWizard: &scriptedWizard{startNewProfile: initial},
		selected:       selected,
	}
	validator := &scriptedValidator{
		reports: []profile.ValidationReport{{SavedPath: "memory://dependency-flow", Summary: "Validation passed and profile was saved."}},
	}
	discoverer := scriptedDiscoverer{report: schema.DiscoveryReport{Source: schema.EndpointDiscovery{Role: "source", Engine: model.EnginePostgres, Snapshot: schema.Snapshot{
		Role:   "source",
		Engine: model.EnginePostgres,
		Tables: []schema.Table{
			{ID: schema.TableID{Schema: "public", Name: "customers"}},
			{ID: schema.TableID{Schema: "public", Name: "orders"}, ForeignKeys: []schema.ForeignKey{{Name: "orders_customer_fk", Columns: []string{"customer_id"}, ReferencedTable: schema.TableID{Schema: "public", Name: "customers"}, ReferencedColumns: []string{"id"}}}},
			{ID: schema.TableID{Schema: "public", Name: "order_items"}, ForeignKeys: []schema.ForeignKey{{Name: "order_items_order_fk", Columns: []string{"order_id"}, ReferencedTable: schema.TableID{Schema: "public", Name: "orders"}, ReferencedColumns: []string{"id"}}}},
		},
	}}}}

	app := NewApp(bytes.NewBuffer(nil), stdout, &bytes.Buffer{})
	app.SetWizard(wizard)
	app.SetValidator(validator)
	app.SetDiscoverer(discoverer)

	if err := app.StartInteractiveProfile(context.Background()); err != nil {
		t.Fatalf("StartInteractiveProfile() error = %v", err)
	}
	if len(validator.inputs) != 1 {
		t.Fatalf("ValidateAndSave calls = %d, want 1", len(validator.inputs))
	}
	if got, want := validator.inputs[0].Selection.Tables, []string{"public.order_items"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("saved selection tables = %v, want %v", got, want)
	}
	if got, want := validator.inputs[0].Selection.ExcludedTables, []string{"public.orders"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("saved selection exclusions = %v, want %v", got, want)
	}
}

type tableSelectionWizard struct {
	*scriptedWizard
	selected model.Profile
}

func (wizard *tableSelectionWizard) SelectTables(context.Context, model.Profile, schema.DiscoveryReport) (model.Profile, error) {
	return wizard.selected, nil
}
