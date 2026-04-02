package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"
)

const (
	sourceEngineKey = "DB_SYNC_SOURCE_ENGINE"
	sourceHostKey   = "DB_SYNC_SOURCE_HOST"
	sourcePortKey   = "DB_SYNC_SOURCE_PORT"
	sourceUserKey   = "DB_SYNC_SOURCE_USER"
	sourcePassKey   = "DB_SYNC_SOURCE_PASSWORD"
	sourceDBKey     = "DB_SYNC_SOURCE_DB"

	targetEngineKey = "DB_SYNC_TARGET_ENGINE"
	targetHostKey   = "DB_SYNC_TARGET_HOST"
	targetPortKey   = "DB_SYNC_TARGET_PORT"
	targetUserKey   = "DB_SYNC_TARGET_USER"
	targetPassKey   = "DB_SYNC_TARGET_PASSWORD"
	targetDBKey     = "DB_SYNC_TARGET_DB"

	tablesKey        = "DB_SYNC_TABLES"
	excludeTablesKey = "DB_SYNC_EXCLUDE_TABLES"
	mirrorDeleteKey  = "DB_SYNC_MIRROR_DELETE"
	mergeTablesKey   = "DB_SYNC_MERGE_TABLES"
)

func LoadProfileFromEnvironment(env map[string]string) (model.Profile, error) {
	source, sourceMissing, err := loadEndpointFromEnvironment(env, "source")
	if err != nil {
		return model.Profile{}, err
	}
	target, targetMissing, err := loadEndpointFromEnvironment(env, "target")
	if err != nil {
		return model.Profile{}, err
	}
	missing := dedupeMissingEnv(append(sourceMissing, targetMissing...))
	if len(missing) > 0 {
		return model.Profile{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	candidate := model.DefaultProfile("")
	candidate.Source = source
	candidate.Target = target
	candidate.Selection.Tables = schema.ParseSelectionInput(env[tablesKey])
	candidate.Selection.ExcludedTables = schema.ParseSelectionInput(env[excludeTablesKey])
	candidate.Sync.MergeTables = schema.ParseSelectionInput(env[mergeTablesKey])
	mirrorDelete, err := parseBool(env[mirrorDeleteKey], false, mirrorDeleteKey)
	if err != nil {
		return model.Profile{}, err
	}
	candidate.Sync.MirrorDelete = mirrorDelete
	return profile.NormalizeProfile(candidate)
}

func loadEndpointFromEnvironment(env map[string]string, role string) (model.Endpoint, []string, error) {
	keys := endpointKeys(role)
	engine, err := parseEngine(env[keys.engine], env[keys.port])
	if err != nil {
		return model.Endpoint{}, nil, err
	}
	missing := missingEnvValues(env, keys.host, keys.user, keys.password, keys.database)
	if len(missing) > 0 {
		return model.Endpoint{}, missing, nil
	}
	port, err := parsePort(env[keys.port], defaultPortForEngine(engine), keys.port)
	if err != nil {
		return model.Endpoint{}, nil, err
	}
	return model.Endpoint{
		Engine: engine,
		Connection: model.Connection{
			Mode: model.ConnectionModeDetails,
			Details: model.ConnectionDetails{
				Host:     strings.TrimSpace(env[keys.host]),
				Port:     port,
				Database: strings.TrimSpace(env[keys.database]),
				Username: strings.TrimSpace(env[keys.user]),
				Password: env[keys.password],
			},
		},
	}, nil, nil
}

type endpointEnvKeys struct {
	engine   string
	host     string
	port     string
	user     string
	password string
	database string
}

func endpointKeys(role string) endpointEnvKeys {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "source":
		return endpointEnvKeys{engine: sourceEngineKey, host: sourceHostKey, port: sourcePortKey, user: sourceUserKey, password: sourcePassKey, database: sourceDBKey}
	default:
		return endpointEnvKeys{engine: targetEngineKey, host: targetHostKey, port: targetPortKey, user: targetUserKey, password: targetPassKey, database: targetDBKey}
	}
}

func parseEngine(value string, portText string) (model.Engine, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "mariadb":
		if strings.TrimSpace(value) == "" && strings.TrimSpace(portText) == "5432" {
			return model.EnginePostgres, nil
		}
		return model.EngineMariaDB, nil
	case "mysql":
		return model.EngineMySQL, nil
	case "postgres", "postgresql":
		return model.EnginePostgres, nil
	default:
		return "", fmt.Errorf("unsupported database engine %q", value)
	}
}

func parsePort(value string, fallback int, key string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	port, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid port in %s: %w", key, err)
	}
	if port <= 0 {
		return 0, fmt.Errorf("invalid port in %s: must be greater than zero", key)
	}
	return port, nil
}

func parseBool(value string, fallback bool, key string) (bool, error) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return fallback, nil
	}
	switch trimmed {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean in %s: %q", key, value)
	}
}

func defaultPortForEngine(engine model.Engine) int {
	switch engine {
	case model.EnginePostgres:
		return 5432
	case model.EngineMySQL, model.EngineMariaDB:
		return 3306
	default:
		return 0
	}
}

func missingEnvValues(env map[string]string, keys ...string) []string {
	missing := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(env[key]) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func dedupeMissingEnv(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
