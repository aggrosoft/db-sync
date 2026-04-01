package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"
)

type fakeValidator struct {
	report profile.ValidationReport
	err    error
	inputs []model.Profile
}

func (validator *fakeValidator) ValidateProfile(_ context.Context, candidate model.Profile) (profile.ValidationReport, error) {
	validator.inputs = append(validator.inputs, candidate)
	return validator.report, validator.err
}

type fakeDiscoverer struct {
	report schema.DiscoveryReport
	err    error
	inputs []model.Profile
}

func (discoverer *fakeDiscoverer) DiscoverProfile(_ context.Context, candidate model.Profile) (schema.DiscoveryReport, error) {
	discoverer.inputs = append(discoverer.inputs, candidate)
	return discoverer.report, discoverer.err
}

func TestRunFromEnvironmentBuildsProfileAndRendersPreview(t *testing.T) {
	stdout := &bytes.Buffer{}
	validator := &fakeValidator{report: profile.ValidationReport{
		Source:  profile.EndpointValidation{Role: "source", Engine: model.EngineMariaDB, Status: profile.StatusPassed},
		Target:  profile.EndpointValidation{Role: "target", Engine: model.EngineMariaDB, Status: profile.StatusPassed},
		Summary: "Validation passed for both endpoints.",
	}}
	discoverer := &fakeDiscoverer{report: schema.DiscoveryReport{
		Source: schema.EndpointDiscovery{Role: "source", Engine: model.EngineMariaDB, Snapshot: schema.Snapshot{
			Role:   "source",
			Engine: model.EngineMariaDB,
			Tables: []schema.Table{
				{ID: schema.TableID{Name: "logs"}},
				{ID: schema.TableID{Name: "users"}},
				{ID: schema.TableID{Name: "orders"}, ForeignKeys: []schema.ForeignKey{{Name: "orders_users_fk", Columns: []string{"user_id"}, ReferencedTable: schema.TableID{Name: "users"}, ReferencedColumns: []string{"id"}}}},
			},
		}},
		Target:  schema.EndpointDiscovery{Role: "target", Engine: model.EngineMariaDB, Snapshot: schema.Snapshot{Role: "target", Engine: model.EngineMariaDB}},
		Summary: "Schema discovery succeeded for both endpoints.",
	}}
	app := NewApp(stdout, &bytes.Buffer{})
	app.SetEnvironment(map[string]string{
		"DB_SYNC_SOURCE_HOST":     "localhost",
		"DB_SYNC_SOURCE_PORT":     "3306",
		"DB_SYNC_SOURCE_USER":     "dev",
		"DB_SYNC_SOURCE_PASSWORD": "dev",
		"DB_SYNC_SOURCE_DB":       "db",
		"DB_SYNC_TARGET_HOST":     "localhost",
		"DB_SYNC_TARGET_PORT":     "3307",
		"DB_SYNC_TARGET_USER":     "dev",
		"DB_SYNC_TARGET_PASSWORD": "dev",
		"DB_SYNC_TARGET_DB":       "db",
		"DB_SYNC_TABLES":          "users,orders",
		"DB_SYNC_EXCLUDE_TABLES":  "logs",
	})
	app.SetValidator(validator)
	app.SetDiscoverer(discoverer)

	if err := app.RunFromEnvironment(context.Background()); err != nil {
		t.Fatalf("RunFromEnvironment() error = %v", err)
	}
	if len(validator.inputs) != 1 {
		t.Fatalf("ValidateProfile calls = %d, want 1", len(validator.inputs))
	}
	loaded := validator.inputs[0]
	if loaded.Source.Connection.Details.Host != "localhost" {
		t.Fatalf("source host = %q, want localhost", loaded.Source.Connection.Details.Host)
	}
	if loaded.Target.Connection.Details.Port != 3307 {
		t.Fatalf("target port = %d, want 3307", loaded.Target.Connection.Details.Port)
	}
	if got, want := strings.Join(loaded.Selection.Tables, ","), "users,orders"; got != want {
		t.Fatalf("selection tables = %q, want %q", got, want)
	}
	if got, want := strings.Join(loaded.Selection.ExcludedTables, ","), "logs"; got != want {
		t.Fatalf("selection exclusions = %q, want %q", got, want)
	}
	output := stdout.String()
	for _, want := range []string{"Loaded configuration from environment.", "Validation passed for both endpoints.", "Schema discovery succeeded for both endpoints.", "Final table order: users, orders"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want substring %q", output, want)
		}
	}
}

func TestRunFromEnvironmentBlocksRequiredExclusions(t *testing.T) {
	stdout := &bytes.Buffer{}
	validator := &fakeValidator{report: profile.ValidationReport{
		Source:  profile.EndpointValidation{Role: "source", Engine: model.EngineMariaDB, Status: profile.StatusPassed},
		Target:  profile.EndpointValidation{Role: "target", Engine: model.EngineMariaDB, Status: profile.StatusPassed},
		Summary: "Validation passed for both endpoints.",
	}}
	discoverer := &fakeDiscoverer{report: schema.DiscoveryReport{
		Source: schema.EndpointDiscovery{Role: "source", Engine: model.EngineMariaDB, Snapshot: schema.Snapshot{
			Role:   "source",
			Engine: model.EngineMariaDB,
			Tables: []schema.Table{
				{ID: schema.TableID{Name: "customers"}},
				{ID: schema.TableID{Name: "orders"}, ForeignKeys: []schema.ForeignKey{{Name: "orders_customers_fk", Columns: []string{"customer_id"}, ReferencedTable: schema.TableID{Name: "customers"}, ReferencedColumns: []string{"id"}}}},
				{ID: schema.TableID{Name: "order_items"}, ForeignKeys: []schema.ForeignKey{{Name: "order_items_orders_fk", Columns: []string{"order_id"}, ReferencedTable: schema.TableID{Name: "orders"}, ReferencedColumns: []string{"id"}}}},
			},
		}},
		Target:  schema.EndpointDiscovery{Role: "target", Engine: model.EngineMariaDB, Snapshot: schema.Snapshot{Role: "target", Engine: model.EngineMariaDB}},
		Summary: "Schema discovery succeeded for both endpoints.",
	}}
	app := NewApp(stdout, &bytes.Buffer{})
	app.SetEnvironment(map[string]string{
		"DB_SYNC_SOURCE_HOST":     "localhost",
		"DB_SYNC_SOURCE_PORT":     "3306",
		"DB_SYNC_SOURCE_USER":     "dev",
		"DB_SYNC_SOURCE_PASSWORD": "dev",
		"DB_SYNC_SOURCE_DB":       "db",
		"DB_SYNC_TARGET_HOST":     "localhost",
		"DB_SYNC_TARGET_PORT":     "3307",
		"DB_SYNC_TARGET_USER":     "dev",
		"DB_SYNC_TARGET_PASSWORD": "dev",
		"DB_SYNC_TARGET_DB":       "db",
		"DB_SYNC_TABLES":          "order_items",
		"DB_SYNC_EXCLUDE_TABLES":  "orders",
	})
	app.SetValidator(validator)
	app.SetDiscoverer(discoverer)

	err := app.RunFromEnvironment(context.Background())
	if err == nil {
		t.Fatal("RunFromEnvironment() error = nil, want blocked selection error")
	}
	if err.Error() != "table selection is blocked by required exclusions" {
		t.Fatalf("error = %q, want blocked selection error", err.Error())
	}
	if output := stdout.String(); !strings.Contains(output, "Blocked exclusions: orders (required by order_items)") {
		t.Fatalf("stdout = %q, want blocked exclusion summary", output)
	}
}
