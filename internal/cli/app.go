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
	"db-sync/internal/validate"
)

type SchemaDiscoverer interface {
	DiscoverProfile(ctx context.Context, candidate model.Profile) (schema.DiscoveryReport, error)
}

type App struct {
	stdout     io.Writer
	stderr     io.Writer
	validator  profile.ProfileValidator
	discoverer SchemaDiscoverer
	env        map[string]string
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
	return app
}

func (app *App) SetValidator(validator profile.ProfileValidator) {
	app.validator = validator
}

func (app *App) SetDiscoverer(discoverer SchemaDiscoverer) {
	app.discoverer = discoverer
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

func (app *App) RunFromEnvironment(ctx context.Context) error {
	if app.validator == nil {
		return errors.New("profile validator is not configured")
	}
	if app.discoverer == nil {
		return errors.New("schema discoverer is not configured")
	}
	candidate, err := LoadProfileFromEnvironment(app.Environment())
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(app.stdout, "Loaded configuration from environment."); err != nil {
		return err
	}
	report, err := app.validator.ValidateProfile(ctx, candidate)
	if renderErr := RenderValidationReport(app.stdout, report); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	discovery, err := app.discoverer.DiscoverProfile(ctx, candidate)
	if renderErr := RenderDiscoveryReport(app.stdout, discovery); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	preview, err := PreviewConfiguredSelection(candidate, discovery)
	if err != nil {
		return err
	}
	if err := RenderSelectionPreview(app.stdout, candidate, preview); err != nil {
		return err
	}
	if preview.Blocked {
		return errors.New("table selection is blocked by required exclusions")
	}
	return nil
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
