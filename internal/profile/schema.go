package profile

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"db-sync/internal/model"

	"gopkg.in/yaml.v3"
)

var whitespacePattern = regexp.MustCompile(`\s+`)
var slugCleanupPattern = regexp.MustCompile(`[^a-z0-9-]+`)

func MarshalProfile(profile model.Profile) ([]byte, error) {
	normalized, err := NormalizeProfile(profile)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(normalized)
}

func UnmarshalProfile(data []byte) (model.Profile, error) {
	var profile model.Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return model.Profile{}, err
	}
	return NormalizeProfile(profile)
}

func NormalizeProfile(profile model.Profile) (model.Profile, error) {
	profile = profile.WithDefaults()
	if strings.TrimSpace(profile.Name) == "" {
		return model.Profile{}, errors.New("profile name is required")
	}
	if err := ValidateProfileName(profile.Name); err != nil {
		return model.Profile{}, err
	}
	var err error
	profile.Source, err = normalizeEndpoint(profile.Name, "source", profile.Source)
	if err != nil {
		return model.Profile{}, err
	}
	profile.Target, err = normalizeEndpoint(profile.Name, "target", profile.Target)
	if err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func normalizeEndpoint(profileName string, role string, endpoint model.Endpoint) (model.Endpoint, error) {
	endpoint = endpoint.WithDefaults()
	mode := endpoint.EffectiveConnectionMode()
	if endpoint.Engine == "" {
		return model.Endpoint{}, fmt.Errorf("%s engine is required", role)
	}
	switch mode {
	case model.ConnectionModeLegacyTemplate:
		endpoint.Connection.Mode = model.ConnectionModeLegacyTemplate
		if strings.TrimSpace(endpoint.DSNTemplate) == "" {
			return model.Endpoint{}, fmt.Errorf("%s legacy template is required", role)
		}
	case model.ConnectionModeConnectionString:
		endpoint.Connection.Mode = model.ConnectionModeConnectionString
		endpoint.DSNTemplate = ""
		if strings.TrimSpace(endpoint.Connection.ConnectionString.EnvVar) == "" {
			endpoint.Connection.ConnectionString.EnvVar = ConnectionStringEnvVar(profileName, role)
		}
		if strings.TrimSpace(endpoint.Connection.ConnectionString.Value) == "" && strings.TrimSpace(endpoint.Connection.ConnectionString.EnvVar) == "" {
			return model.Endpoint{}, fmt.Errorf("%s connection string is required", role)
		}
	case model.ConnectionModeDetails:
		endpoint.Connection.Mode = model.ConnectionModeDetails
		endpoint.DSNTemplate = ""
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
		if strings.TrimSpace(endpoint.Connection.Details.PasswordEnv) == "" {
			endpoint.Connection.Details.PasswordEnv = PasswordEnvVar(profileName, role)
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

func ValidateProfileName(name string) error {
	slug := Slugify(name)
	if slug == "" {
		return fmt.Errorf("profile name %q does not produce a valid slug", name)
	}
	return nil
}

func Slugify(name string) string {
	slug := strings.TrimSpace(strings.ToLower(name))
	slug = whitespacePattern.ReplaceAllString(slug, "-")
	slug = slugCleanupPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return slug
}

func ConnectionStringEnvVar(profileName string, role string) string {
	return fmt.Sprintf("DB_SYNC_%s_%s_DSN", envToken(profileName), strings.ToUpper(strings.TrimSpace(role)))
}

func PasswordEnvVar(profileName string, role string) string {
	return fmt.Sprintf("DB_SYNC_%s_%s_PASSWORD", envToken(profileName), strings.ToUpper(strings.TrimSpace(role)))
}

func EnvFileName(profileName string) string {
	return Slugify(profileName) + ".env"
}

func EnvPathForProfilePath(profilePath string) string {
	base := strings.TrimSuffix(profilePath, filepathExt(profilePath))
	return base + ".env"
}

func filepathExt(path string) string {
	index := strings.LastIndex(path, ".")
	if index == -1 {
		return ""
	}
	return path[index:]
}

func envToken(name string) string {
	slug := strings.ToUpper(strings.ReplaceAll(Slugify(name), "-", "_"))
	if slug == "" {
		return "PROFILE"
	}
	return slug
}

func PortString(port int) string {
	if port == 0 {
		return ""
	}
	return strconv.Itoa(port)
}
