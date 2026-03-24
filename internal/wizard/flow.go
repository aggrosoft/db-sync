package wizard

import (
	"context"
	"fmt"
	"io"
	"strings"

	"db-sync/internal/model"
	profilepkg "db-sync/internal/profile"
	"db-sync/internal/schema"

	"github.com/charmbracelet/huh"
)

type Service struct {
	output io.Writer
}

func NewService(output io.Writer) *Service {
	return &Service{output: output}
}

func (service *Service) StartNew(ctx context.Context) (model.Profile, error) {
	draft := NewDraft()
	return service.run(ctx, draft)
}

func (service *Service) StartEdit(ctx context.Context, existing model.Profile) (model.Profile, error) {
	return service.run(ctx, FromProfile(existing))
}

func (service *Service) SelectTables(ctx context.Context, profile model.Profile, discovery schema.DiscoveryReport) (model.Profile, error) {
	graph := schema.BuildDependencyGraph(discovery.Source.Snapshot)
	selectedInput := strings.Join(profile.Selection.Tables, ", ")
	excludedInput := strings.Join(profile.Selection.ExcludedTables, ", ")
	for {
		if service.output != nil && len(discovery.Source.Snapshot.Tables) > 0 {
			available := make([]string, 0, len(discovery.Source.Snapshot.Tables))
			for _, table := range discovery.Source.Snapshot.Tables {
				available = append(available, table.ID.String())
			}
			if _, err := fmt.Fprintf(service.output, "Discovered source tables: %s\n", strings.Join(available, ", ")); err != nil {
				return model.Profile{}, err
			}
		}

		selectionForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().Title("Selected tables").Description(FutureTablesHelp).Value(&selectedInput),
				huh.NewInput().Title("Excluded required tables").Description(TableExclusionHelp).Value(&excludedInput),
			),
		).WithAccessible(true)
		if err := selectionForm.RunWithContext(ctx); err != nil {
			return model.Profile{}, err
		}

		updated := profile.WithDefaults()
		updated.Selection.Tables = schema.ParseSelectionInput(selectedInput)
		updated.Selection.ExcludedTables = schema.ParseSelectionInput(excludedInput)
		preview, err := schema.PreviewSelection(graph, updated.Selection.Tables, updated.Selection.ExcludedTables)
		if err != nil {
			if service.output != nil {
				if _, writeErr := fmt.Fprintf(service.output, "Selection error: %v\n", err); writeErr != nil {
					return model.Profile{}, writeErr
				}
			}
			continue
		}
		if service.output != nil {
			if _, err := fmt.Fprintln(service.output, RenderReview(updated, &preview)); err != nil {
				return model.Profile{}, err
			}
		}
		approved := false
		confirm := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title("Save this reviewed profile?").Value(&approved))).WithAccessible(true)
		if err := confirm.RunWithContext(ctx); err != nil {
			return model.Profile{}, err
		}
		if !approved {
			return model.Profile{}, context.Canceled
		}
		return updated, nil
	}
}

func (service *Service) run(ctx context.Context, draft ProfileDraft) (model.Profile, error) {
	baseForm := huh.NewForm(huh.NewGroup(huh.NewInput().Title("Profile name").Value(&draft.Name))).WithAccessible(true)
	if err := baseForm.RunWithContext(ctx); err != nil {
		return model.Profile{}, err
	}
	if err := service.captureEndpoint(ctx, draft.Name, "Source", "source", &draft.Source); err != nil {
		return model.Profile{}, err
	}
	if err := service.captureEndpoint(ctx, draft.Name, "Target", "target", &draft.Target); err != nil {
		return model.Profile{}, err
	}
	syncForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().Title("Sync mode").Description(FutureTablesHelp).Options(huh.NewOption("Insert missing", model.SyncModeInsertMissing)).Value(&draft.SyncMode),
			huh.NewConfirm().Title("Enable mirror delete").Description(MirrorDeleteHelp).Value(&draft.MirrorDelete),
		),
	).WithAccessible(true)
	if err := syncForm.RunWithContext(ctx); err != nil {
		return model.Profile{}, err
	}
	return draft.ToProfile(), nil
}

func (service *Service) captureEndpoint(ctx context.Context, profileName string, title string, role string, draft *EndpointDraft) error {
	modeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[model.Engine]().Title(title+" engine").Options(engineOptions()...).Value(&draft.Engine),
			huh.NewSelect[model.ConnectionMode]().Title(title+" input mode").Description(ConnectionModeHelp).Options(
				huh.NewOption("Paste connection string", model.ConnectionModeConnectionString),
				huh.NewOption("Enter connection details", model.ConnectionModeDetails),
				huh.NewOption("Keep legacy DSN template", model.ConnectionModeLegacyTemplate),
			).Value(&draft.Mode),
		),
	).WithAccessible(true)
	if err := modeForm.RunWithContext(ctx); err != nil {
		return err
	}
	draft.ConnectionEnvVar = profilepkg.ConnectionStringEnvVar(profileName, role)
	draft.PasswordEnvVar = profilepkg.PasswordEnvVar(profileName, role)
	switch draft.Mode {
	case model.ConnectionModeDetails:
		return service.captureDetails(ctx, title, draft)
	case model.ConnectionModeLegacyTemplate:
		legacyForm := huh.NewForm(huh.NewGroup(huh.NewInput().Title(title + " legacy DSN template").Value(&draft.LegacyTemplate))).WithAccessible(true)
		return legacyForm.RunWithContext(ctx)
	default:
		connectionForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().Title(title + " connection string").Description(ConnectionStringHelp + " Stored as " + draft.ConnectionEnvVar + ".").Value(&draft.ConnectionString),
			),
		).WithAccessible(true)
		return connectionForm.RunWithContext(ctx)
	}
}

func (service *Service) captureDetails(ctx context.Context, title string, draft *EndpointDraft) error {
	fields := []huh.Field{
		huh.NewInput().Title(title + " host").Description(DetailsHelp).Value(&draft.Host),
		huh.NewInput().Title(title + " port").Value(&draft.Port),
		huh.NewInput().Title(title + " database").Value(&draft.Database),
		huh.NewInput().Title(title + " username").Value(&draft.Username),
		huh.NewInput().Title(title + " password").Description("Stored as " + draft.PasswordEnvVar + ".").Value(&draft.Password).EchoMode(huh.EchoModePassword),
	}
	if draft.Engine == model.EnginePostgres {
		fields = append(fields, huh.NewInput().Title(title+" sslmode").Value(&draft.SSLMode))
	}
	form := huh.NewForm(huh.NewGroup(fields...)).WithAccessible(true)
	return form.RunWithContext(ctx)
}

func engineOptions() []huh.Option[model.Engine] {
	return []huh.Option[model.Engine]{
		huh.NewOption("PostgreSQL", model.EnginePostgres),
		huh.NewOption("MySQL", model.EngineMySQL),
		huh.NewOption("MariaDB", model.EngineMariaDB),
	}
}
