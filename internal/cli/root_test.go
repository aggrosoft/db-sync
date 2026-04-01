package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"
)

func TestRootCommandLoadsEnvFileFlag(t *testing.T) {
	stdout := &bytes.Buffer{}
	validator := &fakeValidator{report: profile.ValidationReport{
		Source:  profile.EndpointValidation{Role: "source", Engine: model.EngineMariaDB, Status: profile.StatusPassed},
		Target:  profile.EndpointValidation{Role: "target", Engine: model.EngineMariaDB, Status: profile.StatusPassed},
		Summary: "Validation passed for both endpoints.",
	}}
	discoverer := &fakeDiscoverer{report: schema.DiscoveryReport{
		Source: schema.EndpointDiscovery{Role: "source", Engine: model.EngineMariaDB, Snapshot: schema.Snapshot{
			Role:   "source",
			Engine: model.EngineMariaDB,
			Tables: []schema.Table{
				{ID: schema.TableID{Name: "users"}},
				{ID: schema.TableID{Name: "orders"}},
			},
		}},
		Target:  schema.EndpointDiscovery{Role: "target", Engine: model.EngineMariaDB, Snapshot: schema.Snapshot{Role: "target", Engine: model.EngineMariaDB}},
		Summary: "Schema discovery succeeded for both endpoints.",
	}}
	envPath := filepath.Join(t.TempDir(), "db-sync.env")
	envContent := strings.Join([]string{
		"DB_SYNC_SOURCE_HOST=localhost",
		"DB_SYNC_SOURCE_PORT=3306",
		"DB_SYNC_SOURCE_USER=dev",
		"DB_SYNC_SOURCE_PASSWORD=dev",
		"DB_SYNC_SOURCE_DB=db",
		"DB_SYNC_TARGET_HOST=localhost",
		"DB_SYNC_TARGET_PORT=3307",
		"DB_SYNC_TARGET_USER=dev",
		"DB_SYNC_TARGET_PASSWORD=dev",
		"DB_SYNC_TARGET_DB=db",
		"DB_SYNC_TABLES=users,orders",
	}, "\n")
	if err := os.WriteFile(envPath, []byte(envContent), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	app := NewApp(stdout, &bytes.Buffer{})
	app.SetValidator(validator)
	app.SetDiscoverer(discoverer)
	cmd := NewRootCommand(app)
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{"--env-file", envPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(validator.inputs) != 1 {
		t.Fatalf("ValidateProfile calls = %d, want 1", len(validator.inputs))
	}
	if validator.inputs[0].Target.Connection.Details.Port != 3307 {
		t.Fatalf("target port = %d, want 3307", validator.inputs[0].Target.Connection.Details.Port)
	}
	if !strings.Contains(stdout.String(), "Loaded configuration from environment.") {
		t.Fatalf("stdout = %q, want load message", stdout.String())
	}
}

func TestRootCommandReportsInvalidEnvFileValues(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "broken.env")
	content := strings.Join([]string{
		"DB_SYNC_SOURCE_HOST=localhost",
		"DB_SYNC_SOURCE_PORT=abc",
		"DB_SYNC_SOURCE_USER=dev",
		"DB_SYNC_SOURCE_PASSWORD=dev",
		"DB_SYNC_SOURCE_DB=db",
		"DB_SYNC_TARGET_HOST=localhost",
		"DB_SYNC_TARGET_PORT=3307",
		"DB_SYNC_TARGET_USER=dev",
		"DB_SYNC_TARGET_PASSWORD=dev",
		"DB_SYNC_TARGET_DB=db",
	}, "\n")
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	app := NewApp(&bytes.Buffer{}, &bytes.Buffer{})
	cmd := NewRootCommand(app)
	cmd.SetArgs([]string{"--env-file", envPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want invalid port error")
	}
	if !strings.Contains(err.Error(), "invalid port") {
		t.Fatalf("error = %q, want invalid port message", err.Error())
	}
}
