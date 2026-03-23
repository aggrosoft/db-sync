package wizard

import (
	"fmt"
	"strings"

	"db-sync/internal/model"
	profilepkg "db-sync/internal/profile"
	"db-sync/internal/secrets"

	"github.com/charmbracelet/lipgloss"
)

func RenderReview(profile model.Profile) string {
	profile = profile.WithDefaults()
	header := lipgloss.NewStyle().Bold(true).Underline(true).Render("Review before save")
	sections := []string{
		header,
		fmt.Sprintf("Name: %s", profile.Name),
		fmt.Sprintf("Source: %s", renderEndpoint(profile.Name, profile.Source)),
		fmt.Sprintf("Target: %s", renderEndpoint(profile.Name, profile.Target)),
		fmt.Sprintf("Tables: %v", profile.Selection.Tables),
		fmt.Sprintf("Sync: mode=%s mirror_delete=%t", profile.Sync.Mode, profile.Sync.MirrorDelete),
	}
	return strings.Join(sections, "\n")
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
