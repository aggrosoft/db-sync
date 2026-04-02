package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"db-sync/internal/db/mysql"
	"db-sync/internal/db/postgres"
	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"
	"db-sync/internal/secrets"
	syncapp "db-sync/internal/sync"
	"db-sync/internal/validate"
)

type SchemaDiscoverer interface {
	DiscoverProfile(ctx context.Context, candidate model.Profile) (schema.DiscoveryReport, error)
}

type SyncRunner interface {
	RunProfile(ctx context.Context, candidate model.Profile, analysis interface {
		SelectionPreview() schema.SelectionPreview
		DiscoveryReport() schema.DiscoveryReport
		DriftReport() schema.DriftReport
	}, dryRun bool, progress func(syncapp.ProgressUpdate)) (syncapp.Report, error)
}

type Analysis struct {
	Candidate  model.Profile
	Validation profile.ValidationReport
	Discovery  schema.DiscoveryReport
	Preview    schema.SelectionPreview
	Drift      schema.DriftReport
}

func (analysis Analysis) SelectionPreview() schema.SelectionPreview {
	return analysis.Preview
}

func (analysis Analysis) DiscoveryReport() schema.DiscoveryReport {
	return analysis.Discovery
}

func (analysis Analysis) DriftReport() schema.DriftReport {
	return analysis.Drift
}

type App struct {
	stdout     io.Writer
	stderr     io.Writer
	validator  profile.ProfileValidator
	discoverer SchemaDiscoverer
	runner     SyncRunner
	env        map[string]string
}

type renderedError struct {
	err error
}

func (err renderedError) Error() string {
	if err.err == nil {
		return ""
	}
	return err.err.Error()
}

func (err renderedError) Unwrap() error {
	return err.err
}

func IsRenderedError(err error) bool {
	var rendered renderedError
	return errors.As(err, &rendered)
}

func NewApp(stdout io.Writer, stderr io.Writer) *App {
	app := &App{
		stdout: stdout,
		stderr: stderr,
		env:    secrets.CurrentEnvironment(),
	}
	postgresAdapter := postgres.NewAdapter()
	mySQLAdapter := mysql.NewAdapter()
	app.validator = validate.NewService(func() map[string]string { return app.Environment() }, validate.Registry{
		model.EnginePostgres: postgresAdapter,
		model.EngineMySQL:    mySQLAdapter,
		model.EngineMariaDB:  mySQLAdapter,
	})
	app.discoverer = schema.NewService(func() map[string]string { return app.Environment() }, schema.Registry{
		model.EnginePostgres: postgresAdapter,
		model.EngineMySQL:    mySQLAdapter,
		model.EngineMariaDB:  mySQLAdapter,
	}, validate.ResolveEndpoint)
	app.runner = syncapp.NewService(func() map[string]string { return app.Environment() })
	return app
}

func (app *App) SetValidator(validator profile.ProfileValidator) {
	app.validator = validator
}

func (app *App) SetDiscoverer(discoverer SchemaDiscoverer) {
	app.discoverer = discoverer
}

func (app *App) SetRunner(runner SyncRunner) {
	app.runner = runner
}

func (app *App) SetEnvironment(env map[string]string) {
	app.env = make(map[string]string, len(env))
	for key, value := range env {
		app.env[key] = value
	}
}

func (app *App) SetEnvFile(path string) error {
	app.env = secrets.CurrentEnvironment()
	if strings.TrimSpace(path) == "" {
		return nil
	}
	loaded, err := secrets.LoadEnvFile(path)
	if err != nil {
		return err
	}
	for key, value := range loaded {
		app.env[key] = value
	}
	return nil
}

func (app *App) Environment() map[string]string {
	copyEnv := make(map[string]string, len(app.env))
	for key, value := range app.env {
		copyEnv[key] = value
	}
	return copyEnv
}

func (app *App) AnalyzeFromEnvironment(ctx context.Context) error {
	analysis, err := app.analyzeEnvironment(ctx)
	if err != nil {
		return err
	}
	if err := app.renderAnalysis(analysis); err != nil {
		return err
	}
	if len(analysis.Drift.Blockers) > 0 {
		return errors.New("schema drift blocks sync for one or more selected tables")
	}
	return nil
}

func (app *App) RunFromEnvironment(ctx context.Context, dryRun bool) error {
	if app.validator == nil {
		return errors.New("profile validator is not configured")
	}
	if app.discoverer == nil {
		return errors.New("schema discoverer is not configured")
	}
	if app.runner == nil {
		return errors.New("sync runner is not configured")
	}
	analysis, err := app.analyzeEnvironment(ctx)
	if err != nil {
		return err
	}
	if len(analysis.Drift.Blockers) > 0 {
		if err := app.renderAnalysis(analysis); err != nil {
			return err
		}
		return errors.New("schema drift blocks sync for one or more selected tables")
	}
	progress := newRunProgressBar(app.stderr, len(analysis.Preview.FinalTables), dryRun)
	defer progress.Finish()
	report, err := app.runner.RunProfile(ctx, analysis.Candidate, analysis, dryRun, progress.Advance)
	if renderErr := RenderSyncReport(app.stdout, report); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	return nil
}

func (app *App) analyzeEnvironment(ctx context.Context) (Analysis, error) {
	if app.validator == nil {
		return Analysis{}, errors.New("profile validator is not configured")
	}
	if app.discoverer == nil {
		return Analysis{}, errors.New("schema discoverer is not configured")
	}
	candidate, err := LoadProfileFromEnvironment(app.Environment())
	if err != nil {
		return Analysis{}, err
	}
	validationReport, err := app.validator.ValidateProfile(ctx, candidate)
	if err != nil {
		if renderErr := RenderValidationReport(app.stderr, validationReport); renderErr != nil {
			return Analysis{Candidate: candidate, Validation: validationReport}, renderErr
		}
		return Analysis{Candidate: candidate, Validation: validationReport}, renderedError{err: err}
	}
	discoveryReport, err := app.discoverer.DiscoverProfile(ctx, candidate)
	if err != nil {
		return Analysis{Candidate: candidate, Validation: validationReport, Discovery: discoveryReport}, err
	}
	preview, err := PreviewConfiguredSelection(candidate, discoveryReport)
	if err != nil {
		return Analysis{Candidate: candidate, Validation: validationReport, Discovery: discoveryReport}, err
	}
	drift := schema.CompareSnapshots(
		filterSnapshotToTables(discoveryReport.Source.Snapshot, preview.FinalTables),
		filterSnapshotToTables(discoveryReport.Target.Snapshot, preview.FinalTables),
	)
	drift = schema.RelaxReportForSelection(preview, drift)
	return Analysis{
		Candidate:  candidate,
		Validation: validationReport,
		Discovery:  discoveryReport,
		Preview:    preview,
		Drift:      drift,
	}, nil
}

func (app *App) renderAnalysis(analysis Analysis) error {
	if err := RenderSelectionPreview(app.stdout, analysis.Candidate, analysis.Preview, analysis.Drift); err != nil {
		return err
	}
	if err := RenderDriftReport(app.stdout, analysis.Preview, analysis.Drift); err != nil {
		return err
	}
	return nil
}

func filterSnapshotToTables(snapshot schema.Snapshot, tableIDs []schema.TableID) schema.Snapshot {
	if len(tableIDs) == 0 {
		snapshot.Tables = []schema.Table{}
		return snapshot
	}
	selected := make(map[schema.TableID]struct{}, len(tableIDs))
	for _, tableID := range tableIDs {
		selected[tableID] = struct{}{}
	}
	filtered := make([]schema.Table, 0, len(tableIDs))
	for _, table := range snapshot.Tables {
		if _, ok := selected[table.ID]; !ok {
			continue
		}
		filtered = append(filtered, table)
	}
	snapshot.Tables = filtered
	return schema.NormalizeSnapshot(snapshot)
}

func RenderValidationReport(writer io.Writer, report profile.ValidationReport) error {
	if len(report.MissingEnv) > 0 {
		if _, err := fmt.Fprintf(writer, "Missing required environment variables: %s\n", strings.Join(report.MissingEnv, ", ")); err != nil {
			return err
		}
	}
	for _, endpoint := range []profile.EndpointValidation{report.Source, report.Target} {
		if endpoint.Role == "" {
			continue
		}
		if _, err := fmt.Fprintf(writer, "%s [%s]: %s\n", strings.Title(endpoint.Role), endpoint.Engine, endpoint.Status); err != nil {
			return err
		}
		for _, check := range endpoint.Checks {
			if _, err := fmt.Fprintf(writer, "  - %s: %s", check.Name, check.Status); err != nil {
				return err
			}
			if check.Detail != "" {
				if _, err := fmt.Fprintf(writer, " (%s)", check.Detail); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(writer); err != nil {
				return err
			}
		}
	}
	if report.Summary != "" {
		_, err := fmt.Fprintln(writer, report.Summary)
		return err
	}
	return nil
}
