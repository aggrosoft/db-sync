package profile

import (
	"fmt"
	"strings"

	"db-sync/internal/model"
)

func NormalizeProfile(profile model.Profile) (model.Profile, error) {
	profile = profile.WithDefaults()
	var err error
	profile.Source, err = normalizeEndpoint("source", profile.Source)
	if err != nil {
		return model.Profile{}, err
	}
	profile.Target, err = normalizeEndpoint("target", profile.Target)
	if err != nil {
		return model.Profile{}, err
	}
	profile.Selection.Tables = normalizeSelectionValues(profile.Selection.Tables)
	profile.Selection.ExcludedTables = normalizeSelectionValues(profile.Selection.ExcludedTables)
	profile.Sync.MergeTables = normalizeSelectionValues(profile.Sync.MergeTables)
	return profile, nil
}

func normalizeSelectionValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func normalizeEndpoint(role string, endpoint model.Endpoint) (model.Endpoint, error) {
	endpoint = endpoint.WithDefaults()
	mode := endpoint.EffectiveConnectionMode()
	if endpoint.Engine == "" {
		return model.Endpoint{}, fmt.Errorf("%s engine is required", role)
	}
	switch mode {
	case model.ConnectionModeConnectionString:
		endpoint.Connection.Mode = model.ConnectionModeConnectionString
		if strings.TrimSpace(endpoint.Connection.ConnectionString.Value) == "" && strings.TrimSpace(endpoint.Connection.ConnectionString.EnvVar) == "" {
			return model.Endpoint{}, fmt.Errorf("%s connection string is required", role)
		}
	case model.ConnectionModeDetails:
		endpoint.Connection.Mode = model.ConnectionModeDetails
		if strings.TrimSpace(endpoint.Connection.Details.Host) == "" {
			return model.Endpoint{}, fmt.Errorf("%s host is required", role)
		}
		if strings.TrimSpace(endpoint.Connection.Details.Database) == "" {
			return model.Endpoint{}, fmt.Errorf("%s database is required", role)
		}
		if strings.TrimSpace(endpoint.Connection.Details.Username) == "" {
			return model.Endpoint{}, fmt.Errorf("%s username is required", role)
		}
		if endpoint.Connection.Details.Port == 0 {
			endpoint.Connection.Details.Port = defaultPort(endpoint.Engine)
		}
		if strings.TrimSpace(endpoint.Connection.Details.Password) == "" && strings.TrimSpace(endpoint.Connection.Details.PasswordEnv) == "" {
			return model.Endpoint{}, fmt.Errorf("%s password is required", role)
		}
		if endpoint.Engine == model.EnginePostgres && strings.TrimSpace(endpoint.Connection.Details.SSLMode) == "" {
			endpoint.Connection.Details.SSLMode = "disable"
		}
	default:
		return model.Endpoint{}, fmt.Errorf("%s connection mode is required", role)
	}
	return endpoint, nil
}

func defaultPort(engine model.Engine) int {
	switch engine {
	case model.EnginePostgres:
		return 5432
	case model.EngineMySQL, model.EngineMariaDB:
		return 3306
	default:
		return 0
	}
}
