package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"db-sync/internal/db/mysql"
	"db-sync/internal/db/postgres"
	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"
	"db-sync/internal/secrets"
	"db-sync/internal/validate"
	"db-sync/internal/wizard"

	"github.com/charmbracelet/huh"
)

var ErrInteractiveWizardUnavailable = errors.New("interactive profile workflow is not available yet")

type validationFailureAction string

const (
	validationFailureRetry  validationFailureAction = "retry"
	validationFailureModify validationFailureAction = "modify"
	validationFailureCancel validationFailureAction = "cancel"
)

type validationFailurePrompter func(context.Context, model.Profile, profile.ValidationReport) (validationFailureAction, error)

type WizardService interface {
	StartNew(ctx context.Context) (model.Profile, error)
	StartEdit(ctx context.Context, existing model.Profile) (model.Profile, error)
	SelectTables(ctx context.Context, candidate model.Profile, discovery schema.DiscoveryReport) (model.Profile, error)
}

type SchemaDiscoverer interface {
	DiscoverProfile(ctx context.Context, candidate model.Profile) (schema.DiscoveryReport, error)
}

type App struct {
	stdin            io.Reader
	stdout           io.Writer
	stderr           io.Writer
	store            profile.ProfileStore
	validator        profile.ProfileValidator
	discoverer       SchemaDiscoverer
	wizard           WizardService
	validationPrompt validationFailurePrompter
	env              map[string]string
	envFile          string
	envFileSupported bool
}

func NewApp(stdin io.Reader, stdout io.Writer, stderr io.Writer) *App {
	app := &App{
		stdin:            stdin,
		stdout:           stdout,
		stderr:           stderr,
		env:              secrets.CurrentEnvironment(),
		envFileSupported: true,
		validationPrompt: defaultValidationFailurePrompt(),
	}
	store := profile.NewFilesystemStore("", "db-sync")
	postgresAdapter := postgres.NewAdapter()
	mySQLAdapter := mysql.NewAdapter()
	app.store = store
	app.validator = validate.NewService(store, func() map[string]string { return app.Environment() }, validate.Registry{
		model.EnginePostgres: postgresAdapter,
		model.EngineMySQL:    mySQLAdapter,
		model.EngineMariaDB:  mySQLAdapter,
	})
	app.discoverer = schema.NewService(func() map[string]string { return app.Environment() }, schema.Registry{
		model.EnginePostgres: postgresAdapter,
		model.EngineMySQL:    mySQLAdapter,
		model.EngineMariaDB:  mySQLAdapter,
	}, validate.ResolveEndpoint)
	app.wizard = wizard.NewService(stdout)
	return app
}

func (app *App) SetStore(store profile.ProfileStore) {
	app.store = store
}

func (app *App) SetValidator(validator profile.ProfileValidator) {
	app.validator = validator
}

func (app *App) SetDiscoverer(discoverer SchemaDiscoverer) {
	app.discoverer = discoverer
}

func (app *App) SetWizard(wizard WizardService) {
	app.wizard = wizard
}

func (app *App) SetValidationFailurePrompt(prompt validationFailurePrompter) {
	app.validationPrompt = prompt
}

func (app *App) SetEnvFile(path string) error {
	app.env = secrets.CurrentEnvironment()
	app.envFile = path
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

func (app *App) StartInteractiveProfile(ctx context.Context) error {
	if app.wizard == nil {
		return ErrInteractiveWizardUnavailable
	}
	profileDraft, err := app.wizard.StartNew(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return app.runInteractiveSaveLoop(ctx, profileDraft)
}

func (app *App) RunProfileNew(ctx context.Context) error {
	return app.StartInteractiveProfile(ctx)
}

func (app *App) RunProfileEdit(ctx context.Context, name string) error {
	if app.store == nil {
		return errors.New("profile store is not configured")
	}
	if app.wizard == nil {
		return ErrInteractiveWizardUnavailable
	}
	existing, err := app.store.Load(ctx, name)
	if err != nil {
		return err
	}
	updated, err := app.wizard.StartEdit(ctx, existing)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return app.runInteractiveSaveLoop(ctx, updated)
}

func (app *App) RunProfileList(ctx context.Context) error {
	if app.store == nil {
		return errors.New("profile store is not configured")
	}
	profiles, err := app.store.List(ctx)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		_, err = fmt.Fprintln(app.stdout, "No saved profiles found.")
		return err
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	for _, stored := range profiles {
		if _, err := fmt.Fprintf(app.stdout, "%s\t%s\n", stored.Name, stored.Path); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) RunProfileValidate(ctx context.Context, name string) error {
	if app.store == nil {
		return errors.New("profile store is not configured")
	}
	if app.validator == nil {
		return errors.New("profile validator is not configured")
	}
	loaded, err := app.store.Load(ctx, name)
	if err != nil {
		return err
	}
	report, err := app.validator.ValidateProfile(ctx, loaded)
	if err != nil {
		if renderErr := RenderValidationReport(app.stdout, report); renderErr != nil {
			return renderErr
		}
		return err
	}
	return RenderValidationReport(app.stdout, report)
}

func (app *App) SaveReviewedProfile(ctx context.Context, profileDraft model.Profile) error {
	_, err := app.validateAndRenderReviewedProfile(ctx, profileDraft)
	return err
}

func (app *App) runInteractiveSaveLoop(ctx context.Context, profileDraft model.Profile) error {
	current := profileDraft
	selectionApplied := false
	for {
		if !selectionApplied {
			report, err := app.validator.ValidateProfile(ctx, current)
			if err != nil {
				if renderErr := RenderValidationReport(app.stdout, report); renderErr != nil {
					return renderErr
				}
				if !report.Blocked || app.validationPrompt == nil {
					return err
				}
				action, promptErr := app.validationPrompt(ctx, current, report)
				if promptErr != nil {
					if errors.Is(promptErr, context.Canceled) {
						return nil
					}
					return promptErr
				}
				switch action {
				case validationFailureRetry:
					continue
				case validationFailureModify:
					updated, editErr := app.wizard.StartEdit(ctx, current)
					if editErr != nil {
						if errors.Is(editErr, context.Canceled) {
							return nil
						}
						return editErr
					}
					current = updated
					continue
				case validationFailureCancel:
					return nil
				default:
					return fmt.Errorf("unsupported validation action %q", action)
				}
			}
			if app.discoverer != nil && app.wizard != nil {
				discovery, err := app.discoverer.DiscoverProfile(ctx, current)
				if err != nil {
					return err
				}
				updated, err := app.wizard.SelectTables(ctx, current, discovery)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return nil
					}
					return err
				}
				current = updated
			}
			selectionApplied = true
		}
		report, err := app.validateAndRenderReviewedProfile(ctx, current)
		if err == nil {
			return nil
		}
		if !report.Blocked || app.validationPrompt == nil {
			return err
		}
		action, promptErr := app.validationPrompt(ctx, current, report)
		if promptErr != nil {
			if errors.Is(promptErr, context.Canceled) {
				return nil
			}
			return promptErr
		}
		switch action {
		case validationFailureRetry:
			selectionApplied = false
			continue
		case validationFailureModify:
			updated, editErr := app.wizard.StartEdit(ctx, current)
			if editErr != nil {
				if errors.Is(editErr, context.Canceled) {
					return nil
				}
				return editErr
			}
			current = updated
			selectionApplied = false
		case validationFailureCancel:
			return nil
		default:
			return fmt.Errorf("unsupported validation action %q", action)
		}
	}
}

func (app *App) validateAndRenderReviewedProfile(ctx context.Context, profileDraft model.Profile) (profile.ValidationReport, error) {
	if app.validator == nil {
		return profile.ValidationReport{}, errors.New("profile validator is not configured")
	}
	report, err := app.validator.ValidateAndSave(ctx, profileDraft)
	if err != nil {
		if renderErr := RenderValidationReport(app.stdout, report); renderErr != nil {
			return profile.ValidationReport{}, renderErr
		}
		return report, err
	}
	if err := RenderValidationReport(app.stdout, report); err != nil {
		return profile.ValidationReport{}, err
	}
	if report.SavedPath != "" {
		_, err = fmt.Fprintf(app.stdout, "Saved profile to %s\n", report.SavedPath)
		return report, err
	}
	return report, nil
}

func defaultValidationFailurePrompt() validationFailurePrompter {
	return func(ctx context.Context, _ model.Profile, report profile.ValidationReport) (validationFailureAction, error) {
		action := string(validationFailureRetry)
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Validation was blocked. What should happen next?").
					Description(report.Summary).
					Options(
						huh.NewOption("Retry validation", string(validationFailureRetry)),
						huh.NewOption("Modify connection details", string(validationFailureModify)),
						huh.NewOption("Cancel without saving", string(validationFailureCancel)),
					).
					Value(&action),
			),
		).WithAccessible(true)
		if err := form.RunWithContext(ctx); err != nil {
			return "", err
		}
		return validationFailureAction(action), nil
	}
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
