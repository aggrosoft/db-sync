package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"
	syncapp "db-sync/internal/sync"
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

type fakeRunner struct {
	report   syncapp.Report
	err      error
	inputs   []model.Profile
	dryRuns  []bool
	analyses []Analysis
	progress [][]syncapp.ProgressUpdate
}

func (runner *fakeRunner) RunProfile(_ context.Context, candidate model.Profile, analysis interface {
	SelectionPreview() schema.SelectionPreview
	DiscoveryReport() schema.DiscoveryReport
	DriftReport() schema.DriftReport
}, dryRun bool, progress func(syncapp.ProgressUpdate)) (syncapp.Report, error) {
	runner.inputs = append(runner.inputs, candidate)
	runner.dryRuns = append(runner.dryRuns, dryRun)
	runner.analyses = append(runner.analyses, Analysis{Preview: analysis.SelectionPreview(), Discovery: analysis.DiscoveryReport(), Drift: analysis.DriftReport()})
	updates := []syncapp.ProgressUpdate{}
	if progress != nil {
		for index, table := range runner.report.Tables {
			update := syncapp.ProgressUpdate{Completed: index + 1, Total: len(runner.report.Tables), TableID: table.TableID, Scope: table.Scope, DryRun: dryRun}
			progress(update)
			updates = append(updates, update)
		}
	}
	runner.progress = append(runner.progress, updates)
	return runner.report, runner.err
}

func TestAnalyzeFromEnvironmentBuildsProfileAndRendersPreview(t *testing.T) {
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
				{ID: schema.TableID{Schema: "shop", Name: "logs"}},
				{ID: schema.TableID{Schema: "shop", Name: "users"}},
				{ID: schema.TableID{Schema: "shop", Name: "orders"}, ForeignKeys: []schema.ForeignKey{{Name: "orders_users_fk", Columns: []string{"user_id"}, ReferencedTable: schema.TableID{Schema: "shop", Name: "users"}, ReferencedColumns: []string{"id"}}}},
			},
		}},
		Target: schema.EndpointDiscovery{Role: "target", Engine: model.EngineMariaDB, Snapshot: schema.Snapshot{
			Role:   "target",
			Engine: model.EngineMariaDB,
			Tables: []schema.Table{
				{ID: schema.TableID{Schema: "shop", Name: "users"}},
				{ID: schema.TableID{Schema: "shop", Name: "orders"}, ForeignKeys: []schema.ForeignKey{{Name: "orders_users_fk", Columns: []string{"user_id"}, ReferencedTable: schema.TableID{Schema: "shop", Name: "users"}, ReferencedColumns: []string{"id"}}}},
			},
		}},
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
	})
	app.SetValidator(validator)
	app.SetDiscoverer(discoverer)

	if err := app.AnalyzeFromEnvironment(context.Background()); err != nil {
		t.Fatalf("AnalyzeFromEnvironment() error = %v", err)
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
	output := stdout.String()
	for _, want := range []string{"Selected tables", "2 total, 2 explicit, 0 implicit", "TABLE", "SCOPE", "WARNINGS", "BLOCKERS", "orders", "users", "Drift details: no warnings or blockers."} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want substring %q", output, want)
		}
	}
	for _, unwanted := range []string{"Loaded configuration from environment.", "Validation passed for both endpoints.", "Schema discovery succeeded for both endpoints.", "shop.orders", "shop.users"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("stdout = %q, want output without %q", output, unwanted)
		}
	}
}
func TestRunFromEnvironmentPassesDryRunToRunner(t *testing.T) {
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
			Tables: []schema.Table{{
				ID:         schema.TableID{Name: "users"},
				Columns:    []schema.Column{{Name: "id", Ordinal: 1, DataType: "int", NativeType: "int", Writable: true}},
				PrimaryKey: schema.PrimaryKey{Columns: []string{"id"}},
			}},
		}},
		Target: schema.EndpointDiscovery{Role: "target", Engine: model.EngineMariaDB, Snapshot: schema.Snapshot{
			Role:   "target",
			Engine: model.EngineMariaDB,
			Tables: []schema.Table{{
				ID:         schema.TableID{Name: "users"},
				Columns:    []schema.Column{{Name: "id", Ordinal: 1, DataType: "int", NativeType: "int", Writable: true}},
				PrimaryKey: schema.PrimaryKey{Columns: []string{"id"}},
			}},
		}},
		Summary: "Schema discovery succeeded for both endpoints.",
	}}
	runner := &fakeRunner{report: syncapp.Report{DryRun: true, Tables: []syncapp.TableReport{{TableID: schema.TableID{Name: "users"}, Scope: "explicit", SourceRows: 3, MissingRows: 1, InsertedRows: 1, UpdatedRows: 0, DeletedRows: 0}}, MissingRows: 1, InsertedRows: 1, UpdatedRows: 0, DeletedRows: 0, Summary: "Sync dry-run completed for 1 table(s)."}}
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
		"DB_SYNC_TABLES":          "users",
	})
	app.SetValidator(validator)
	app.SetDiscoverer(discoverer)
	app.SetRunner(runner)

	if err := app.RunFromEnvironment(context.Background(), true); err != nil {
		t.Fatalf("RunFromEnvironment() error = %v", err)
	}
	if len(runner.inputs) != 1 {
		t.Fatalf("RunProfile calls = %d, want 1", len(runner.inputs))
	}
	if len(runner.dryRuns) != 1 || !runner.dryRuns[0] {
		t.Fatalf("dryRun flags = %v, want [true]", runner.dryRuns)
	}
	if len(runner.progress) != 1 || len(runner.progress[0]) != 1 {
		t.Fatalf("progress updates = %#v, want one update", runner.progress)
	}
	if got := runner.progress[0][0]; got.Completed != 1 || got.Total != 1 || got.TableID.Name != "users" || !got.DryRun {
		t.Fatalf("progress update = %#v, want completed update for users dry-run", got)
	}
	output := stdout.String()
	if !strings.Contains(output, "Sync dry-run: 1 table(s), 1 missing row(s), 1 inserted row(s), 0 updated row(s), 0 deleted row(s)") || !strings.Contains(output, "explicit") || !strings.Contains(output, "users") {
		t.Fatalf("stdout = %q, want dry-run summary", output)
	}
	if strings.Contains(output, "Selected tables") {
		t.Fatalf("stdout = %q, want run output without analysis preview", output)
	}
}

func TestAnalyzeFromEnvironmentRendersValidationFailureAndStops(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	validator := &fakeValidator{
		report: profile.ValidationReport{
			Source: profile.EndpointValidation{
				Role:   "source",
				Engine: model.EnginePostgres,
				Status: profile.StatusFailed,
				Checks: []profile.CheckResult{{Name: "authentication", Status: profile.StatusFailed, Detail: "dial tcp 127.0.0.1:5432: connect: connection refused"}},
			},
			Target: profile.EndpointValidation{
				Role:   "target",
				Engine: model.EngineMySQL,
				Status: profile.StatusPassed,
				Checks: []profile.CheckResult{{Name: "authentication", Status: profile.StatusPassed, Detail: "connection established"}},
			},
			Blocked: true,
			Summary: "dial tcp 127.0.0.1:5432: connect: connection refused",
		},
		err: errors.New("dial tcp 127.0.0.1:5432: connect: connection refused"),
	}
	discoverer := &fakeDiscoverer{}
	app := NewApp(stdout, stderr)
	app.SetEnvironment(map[string]string{
		"DB_SYNC_SOURCE_HOST":     "localhost",
		"DB_SYNC_SOURCE_PORT":     "5432",
		"DB_SYNC_SOURCE_USER":     "dev",
		"DB_SYNC_SOURCE_PASSWORD": "dev",
		"DB_SYNC_SOURCE_DB":       "db",
		"DB_SYNC_TARGET_HOST":     "localhost",
		"DB_SYNC_TARGET_PORT":     "3307",
		"DB_SYNC_TARGET_USER":     "dev",
		"DB_SYNC_TARGET_PASSWORD": "dev",
		"DB_SYNC_TARGET_DB":       "db",
		"DB_SYNC_TABLES":          "users",
	})
	app.SetValidator(validator)
	app.SetDiscoverer(discoverer)

	err := app.AnalyzeFromEnvironment(context.Background())
	if err == nil {
		t.Fatal("AnalyzeFromEnvironment() error = nil, want validation error")
	}
	if !IsRenderedError(err) {
		t.Fatalf("AnalyzeFromEnvironment() error = %T, want rendered error", err)
	}
	if len(discoverer.inputs) != 0 {
		t.Fatalf("DiscoverProfile calls = %d, want 0", len(discoverer.inputs))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output", stdout.String())
	}
	output := stderr.String()
	for _, want := range []string{"Source [postgres]: failed", "authentication: failed", "Target [mysql]: passed", "dial tcp 127.0.0.1:5432: connect: connection refused"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr = %q, want substring %q", output, want)
		}
	}
	for _, unwanted := range []string{"Selected tables", "Drift details"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("stderr = %q, want output without %q", output, unwanted)
		}
	}
}

func TestRunFromEnvironmentRendersValidationFailureAndStops(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	validator := &fakeValidator{
		report: profile.ValidationReport{
			Source: profile.EndpointValidation{
				Role:   "source",
				Engine: model.EnginePostgres,
				Status: profile.StatusPassed,
				Checks: []profile.CheckResult{{Name: "authentication", Status: profile.StatusPassed, Detail: "connection established"}},
			},
			Target: profile.EndpointValidation{
				Role:   "target",
				Engine: model.EngineMySQL,
				Status: profile.StatusFailed,
				Checks: []profile.CheckResult{{Name: "target capability", Status: profile.StatusFailed, Detail: "target is read-only"}},
			},
			Blocked: true,
			Summary: "target is read-only",
		},
		err: errors.New("target is read-only"),
	}
	discoverer := &fakeDiscoverer{}
	runner := &fakeRunner{}
	app := NewApp(stdout, stderr)
	app.SetEnvironment(map[string]string{
		"DB_SYNC_SOURCE_HOST":     "localhost",
		"DB_SYNC_SOURCE_PORT":     "5432",
		"DB_SYNC_SOURCE_USER":     "dev",
		"DB_SYNC_SOURCE_PASSWORD": "dev",
		"DB_SYNC_SOURCE_DB":       "db",
		"DB_SYNC_TARGET_HOST":     "localhost",
		"DB_SYNC_TARGET_PORT":     "3307",
		"DB_SYNC_TARGET_USER":     "dev",
		"DB_SYNC_TARGET_PASSWORD": "dev",
		"DB_SYNC_TARGET_DB":       "db",
		"DB_SYNC_TABLES":          "users",
	})
	app.SetValidator(validator)
	app.SetDiscoverer(discoverer)
	app.SetRunner(runner)

	err := app.RunFromEnvironment(context.Background(), false)
	if err == nil {
		t.Fatal("RunFromEnvironment() error = nil, want validation error")
	}
	if !IsRenderedError(err) {
		t.Fatalf("RunFromEnvironment() error = %T, want rendered error", err)
	}
	if len(discoverer.inputs) != 0 {
		t.Fatalf("DiscoverProfile calls = %d, want 0", len(discoverer.inputs))
	}
	if len(runner.inputs) != 0 {
		t.Fatalf("RunProfile calls = %d, want 0", len(runner.inputs))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output", stdout.String())
	}
	output := stderr.String()
	for _, want := range []string{"Source [postgres]: passed", "Target [mysql]: failed", "target capability: failed", "target is read-only"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr = %q, want substring %q", output, want)
		}
	}
	for _, unwanted := range []string{"Selected tables", "Sync executed", "Sync dry-run"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("stderr = %q, want output without %q", output, unwanted)
		}
	}
}
