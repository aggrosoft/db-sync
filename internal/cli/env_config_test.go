package cli

import (
	"strings"
	"testing"

	"db-sync/internal/model"
)

func TestLoadProfileFromEnvironmentMatchesDotEnvContract(t *testing.T) {
	loaded, err := LoadProfileFromEnvironment(map[string]string{
		"DB_SYNC_SOURCE_HOST":     "localhost",
		"DB_SYNC_SOURCE_PORT":     "3306",
		"DB_SYNC_SOURCE_USER":     "dev",
		"DB_SYNC_SOURCE_PASSWORD": "dev",
		"DB_SYNC_SOURCE_DB":       "db",
		"DB_SYNC_TARGET_HOST":     "localhost",
		"DB_SYNC_TARGET_PORT":     "3307",
		"DB_SYNC_TARGET_USER":     "dev",
		"DB_SYNC_TARGET_PASSWORD": "dev",
		"DB_SYNC_TARGET_DB":       "db",
		"DB_SYNC_TABLES":          "users,orders",
		"DB_SYNC_EXCLUDE_TABLES":  "logs, audit_log ",
		"DB_SYNC_MIRROR_DELETE":   "true",
	})
	if err != nil {
		t.Fatalf("LoadProfileFromEnvironment() error = %v", err)
	}
	if loaded.Source.Engine != model.EngineMariaDB {
		t.Fatalf("source engine = %q, want %q", loaded.Source.Engine, model.EngineMariaDB)
	}
	if loaded.Target.Engine != model.EngineMariaDB {
		t.Fatalf("target engine = %q, want %q", loaded.Target.Engine, model.EngineMariaDB)
	}
	if loaded.Source.Connection.Details.Port != 3306 {
		t.Fatalf("source port = %d, want 3306", loaded.Source.Connection.Details.Port)
	}
	if loaded.Target.Connection.Details.Port != 3307 {
		t.Fatalf("target port = %d, want 3307", loaded.Target.Connection.Details.Port)
	}
	if got, want := strings.Join(loaded.Selection.Tables, ","), "users,orders"; got != want {
		t.Fatalf("selection tables = %q, want %q", got, want)
	}
	if got, want := strings.Join(loaded.Selection.ExcludedTables, ","), "logs,audit_log"; got != want {
		t.Fatalf("excluded tables = %q, want %q", got, want)
	}
	if !loaded.Sync.MirrorDelete {
		t.Fatal("mirror delete = false, want true")
	}
}

func TestLoadProfileFromEnvironmentRejectsMissingVariables(t *testing.T) {
	_, err := LoadProfileFromEnvironment(map[string]string{
		"DB_SYNC_SOURCE_HOST":     "localhost",
		"DB_SYNC_SOURCE_USER":     "dev",
		"DB_SYNC_SOURCE_PASSWORD": "dev",
		"DB_SYNC_SOURCE_DB":       "db",
	})
	if err == nil {
		t.Fatal("LoadProfileFromEnvironment() error = nil, want missing env error")
	}
	if !strings.Contains(err.Error(), "DB_SYNC_TARGET_HOST") {
		t.Fatalf("error = %q, want target env keys", err.Error())
	}
}

func TestLoadProfileFromEnvironmentRejectsInvalidMirrorDelete(t *testing.T) {
	_, err := LoadProfileFromEnvironment(map[string]string{
		"DB_SYNC_SOURCE_HOST":     "localhost",
		"DB_SYNC_SOURCE_PORT":     "3306",
		"DB_SYNC_SOURCE_USER":     "dev",
		"DB_SYNC_SOURCE_PASSWORD": "dev",
		"DB_SYNC_SOURCE_DB":       "db",
		"DB_SYNC_TARGET_HOST":     "localhost",
		"DB_SYNC_TARGET_PORT":     "3307",
		"DB_SYNC_TARGET_USER":     "dev",
		"DB_SYNC_TARGET_PASSWORD": "dev",
		"DB_SYNC_TARGET_DB":       "db",
		"DB_SYNC_MIRROR_DELETE":   "maybe",
	})
	if err == nil {
		t.Fatal("LoadProfileFromEnvironment() error = nil, want invalid mirror delete error")
	}
	if !strings.Contains(err.Error(), "invalid boolean") {
		t.Fatalf("error = %q, want boolean error", err.Error())
	}
}
