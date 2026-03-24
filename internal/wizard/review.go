package wizard

import (
	"fmt"
	"strings"

	"db-sync/internal/model"
	profilepkg "db-sync/internal/profile"
	"db-sync/internal/schema"
	"db-sync/internal/secrets"

	"github.com/charmbracelet/lipgloss"
)

func RenderReview(profile model.Profile, preview *schema.SelectionPreview) string {
	profile = profile.WithDefaults()
	header := lipgloss.NewStyle().Bold(true).Underline(true).Render("Review before save")
	sections := []string{
		header,
		fmt.Sprintf("Name: %s", profile.Name),
		fmt.Sprintf("Source: %s", renderEndpoint(profile.Name, profile.Source)),
		fmt.Sprintf("Target: %s", renderEndpoint(profile.Name, profile.Target)),
		renderSelectionReview(profile, preview),
		fmt.Sprintf("Sync: mode=%s mirror_delete=%t", profile.Sync.Mode, profile.Sync.MirrorDelete),
	}
	return strings.Join(sections, "\n")
}

func renderSelectionReview(profile model.Profile, preview *schema.SelectionPreview) string {
	if preview == nil {
		sections := []string{
			fmt.Sprintf("Explicit selections: %s", selectionSummary(profile.Selection.Tables, "all discovered tables")),
			fmt.Sprintf("Blocked exclusions: %s", selectionSummary(profile.Selection.ExcludedTables, "none")),
		}
		return strings.Join(sections, "\n")
	}
	blocked := "none"
	if len(preview.BlockedExclusions) > 0 {
		parts := make([]string, 0, len(preview.BlockedExclusions))
		for _, exclusion := range preview.BlockedExclusions {
			parts = append(parts, fmt.Sprintf("%s (required by %s)", exclusion.Table.String(), strings.Join(schema.SelectionStrings(exclusion.RequiredBy), ", ")))
		}
		blocked = strings.Join(parts, "; ")
	}
	sections := []string{
		fmt.Sprintf("Explicit selections: %s", selectionSummary(schema.SelectionStrings(preview.ExplicitIncludes), "all discovered tables")),
		fmt.Sprintf("Required additions: %s", selectionSummary(schema.SelectionStrings(preview.RequiredTables), "none")),
		fmt.Sprintf("Blocked exclusions: %s", blocked),
		fmt.Sprintf("Final table order: %s", selectionSummary(schema.SelectionStrings(preview.FinalTables), "none")),
	}
	if preview.Blocked {
		sections = append(sections, "Selection status: blocked until the required exclusions are removed or the seed list changes")
	}
	return strings.Join(sections, "\n")
}

func selectionSummary(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}

func renderEndpoint(profileName string, endpoint model.Endpoint) string {
	endpoint = endpoint.WithDefaults()
	switch endpoint.EffectiveConnectionMode() {
	case model.ConnectionModeConnectionString:
		return fmt.Sprintf("%s connection string -> %s in %s", endpoint.Engine, endpoint.Connection.ConnectionString.EnvVar, profilepkg.EnvFileName(profileName))
	case model.ConnectionModeDetails:
		details := endpoint.Connection.Details
		parts := []string{
			fmt.Sprintf("%s details", endpoint.Engine),
			fmt.Sprintf("host=%s", details.Host),
			fmt.Sprintf("port=%d", details.Port),
			fmt.Sprintf("database=%s", details.Database),
			fmt.Sprintf("username=%s", details.Username),
			fmt.Sprintf("password=%s", secrets.EnvReference(details.PasswordEnv)),
			fmt.Sprintf("env-file=%s", profilepkg.EnvFileName(profileName)),
		}
		if details.SSLMode != "" {
			parts = append(parts, fmt.Sprintf("sslmode=%s", details.SSLMode))
		}
		return strings.Join(parts, " ")
	default:
		placeholders, err := secrets.ParsePlaceholders(endpoint.DSNTemplate)
		if err != nil {
			return fmt.Sprintf("%s legacy template (%s)", endpoint.Engine, secrets.RedactedValue(endpoint.DSNTemplate))
		}
		return fmt.Sprintf("%s legacy template placeholders: %s", endpoint.Engine, strings.Join(placeholders, ", "))
	}
}
