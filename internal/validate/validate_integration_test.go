//go:build integration

package validate

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"db-sync/internal/db/mysql"
	"db-sync/internal/db/postgres"
	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/testkit"
)

func TestValidateProfileIntegration(t *testing.T) {
	ctx := context.Background()
	postgresContainer := testkit.StartPostgresContainer(ctx, t)
	defer postgresContainer.Cleanup()
	mysqlContainer := testkit.StartMySQLContainer(ctx, t)
	defer mysqlContainer.Cleanup()

	store := profile.NewFilesystemStore(t.TempDir(), "db-sync")
	service := NewService(store, func() map[string]string {
		return map[string]string{"PG_PASSWORD": "app-secret", "MYSQL_PASSWORD": "app-secret"}
	}, Registry{
		model.EnginePostgres: postgres.NewAdapter(),
		model.EngineMySQL:    mysql.NewAdapter(),
		model.EngineMariaDB:  mysql.NewAdapter(),
	})

	postgresProfile := model.DefaultProfile("pg-profile")
	postgresHost, postgresPort, postgresDatabase := parsePostgresDSN(t, strings.ReplaceAll(postgresContainer.DSN, "${PG_PASSWORD}", "app-secret"))
	postgresProfile.Source.Engine = model.EnginePostgres
	postgresProfile.Source.Connection.Mode = model.ConnectionModeDetails
	postgresProfile.Source.Connection.Details = model.ConnectionDetails{Host: postgresHost, Port: postgresPort, Database: postgresDatabase, Username: "app", Password: "app-secret", PasswordEnv: profile.PasswordEnvVar(postgresProfile.Name, "source"), SSLMode: "disable"}
	postgresProfile.Target.Engine = model.EnginePostgres
	postgresProfile.Target.Connection.Mode = model.ConnectionModeDetails
	postgresProfile.Target.Connection.Details = model.ConnectionDetails{Host: postgresHost, Port: postgresPort, Database: postgresDatabase, Username: "app", Password: "app-secret", PasswordEnv: profile.PasswordEnvVar(postgresProfile.Name, "target"), SSLMode: "disable"}
	if _, err := service.ValidateProfile(ctx, postgresProfile); err != nil {
		t.Fatalf("ValidateProfile(postgres) error = %v", err)
	}

	mysqlProfile := model.DefaultProfile("mysql-profile")
	mysqlProfile.Source.Engine = model.EngineMySQL
	mysqlProfile.Source.Connection.Mode = model.ConnectionModeConnectionString
	mysqlProfile.Source.Connection.ConnectionString = model.ConnectionString{Value: strings.ReplaceAll(mysqlContainer.DSN, "${MYSQL_PASSWORD}", "app-secret"), EnvVar: profile.ConnectionStringEnvVar(mysqlProfile.Name, "source")}
	mysqlProfile.Target.Engine = model.EngineMySQL
	mysqlProfile.Target.Connection.Mode = model.ConnectionModeConnectionString
	mysqlProfile.Target.Connection.ConnectionString = model.ConnectionString{Value: strings.ReplaceAll(mysqlContainer.DSN, "${MYSQL_PASSWORD}", "app-secret"), EnvVar: profile.ConnectionStringEnvVar(mysqlProfile.Name, "target")}
	if _, err := service.ValidateProfile(ctx, mysqlProfile); err != nil {
		t.Fatalf("ValidateProfile(mysql) error = %v", err)
	}

	blocked := model.DefaultProfile("blocked-save")
	blocked.Source.Engine = model.EnginePostgres
	blocked.Source.Connection.Mode = model.ConnectionModeConnectionString
	blocked.Source.Connection.ConnectionString = model.ConnectionString{Value: strings.ReplaceAll(postgresContainer.DSN, "${PG_PASSWORD}", "app-secret"), EnvVar: profile.ConnectionStringEnvVar(blocked.Name, "source")}
	blocked.Target.Engine = model.EngineMySQL
	blocked.Target.Connection.Mode = model.ConnectionModeDetails
	blocked.Target.Connection.Details = model.ConnectionDetails{Host: "127.0.0.1", Port: 1, Database: "app", Username: "app", Password: "app-secret", PasswordEnv: profile.PasswordEnvVar(blocked.Name, "target")}
	if report, err := service.ValidateAndSave(ctx, blocked); err == nil {
		t.Fatal("ValidateAndSave(blocked) error = nil, want failure")
	} else {
		if report.SavedPath != "" {
			t.Fatalf("blocked save persisted file: %q", report.SavedPath)
		}
		if _, loadErr := store.Load(ctx, blocked.Name); !errors.Is(loadErr, profile.ErrProfileNotFound) {
			t.Fatalf("Load() error = %v, want ErrProfileNotFound", loadErr)
		}
	}
}

func parsePostgresDSN(t *testing.T, dsn string) (string, int, string) {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", dsn, err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", parsed.Host, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi(%q) error = %v", portText, err)
	}
	return host, port, strings.TrimPrefix(parsed.Path, "/")
}
