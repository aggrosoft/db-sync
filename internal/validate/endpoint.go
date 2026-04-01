package validate

import (
	"errors"
	"fmt"
	"net/url"

	"db-sync/internal/model"

	mysqlcfg "github.com/go-sql-driver/mysql"
)

func ResolveEndpoint(endpoint model.Endpoint, env map[string]string) (string, []string, error) {
	switch endpoint.EffectiveConnectionMode() {
	case model.ConnectionModeConnectionString:
		value := firstConfiguredValue(env[endpoint.Connection.ConnectionString.EnvVar], endpoint.Connection.ConnectionString.Value)
		if value == "" {
			return "", []string{endpoint.Connection.ConnectionString.EnvVar}, fmt.Errorf("missing required environment variables: %s", endpoint.Connection.ConnectionString.EnvVar)
		}
		return value, nil, nil
	case model.ConnectionModeDetails:
		password := firstConfiguredValue(env[endpoint.Connection.Details.PasswordEnv], endpoint.Connection.Details.Password)
		if password == "" {
			return "", []string{endpoint.Connection.Details.PasswordEnv}, fmt.Errorf("missing required environment variables: %s", endpoint.Connection.Details.PasswordEnv)
		}
		return buildDetailsDSN(endpoint, password)
	default:
		return "", nil, errors.New("unsupported connection mode")
	}
}

func buildDetailsDSN(endpoint model.Endpoint, password string) (string, []string, error) {
	details := endpoint.Connection.Details
	switch endpoint.Engine {
	case model.EnginePostgres:
		query := url.Values{}
		if details.SSLMode != "" {
			query.Set("sslmode", details.SSLMode)
		}
		return (&url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword(details.Username, password),
			Host:     fmt.Sprintf("%s:%d", details.Host, details.Port),
			Path:     details.Database,
			RawQuery: query.Encode(),
		}).String(), nil, nil
	case model.EngineMySQL, model.EngineMariaDB:
		cfg := mysqlcfg.NewConfig()
		cfg.User = details.Username
		cfg.Passwd = password
		cfg.Net = "tcp"
		cfg.Addr = fmt.Sprintf("%s:%d", details.Host, details.Port)
		cfg.DBName = details.Database
		cfg.ParseTime = true
		return cfg.FormatDSN(), nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported engine %q", endpoint.Engine)
	}
}
