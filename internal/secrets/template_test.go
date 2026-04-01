package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFileParsesAssignments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.env")
	content := "# comment\nexport DB_SYNC_SOURCE_HOST=localhost\nDB_SYNC_SOURCE_PASSWORD='dev'\nDB_SYNC_TABLES=users,orders\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	env, err := LoadEnvFile(path)
	if err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}
	if env["DB_SYNC_SOURCE_HOST"] != "localhost" {
		t.Fatalf("source host = %q, want localhost", env["DB_SYNC_SOURCE_HOST"])
	}
	if env["DB_SYNC_SOURCE_PASSWORD"] != "dev" {
		t.Fatalf("source password = %q, want dev", env["DB_SYNC_SOURCE_PASSWORD"])
	}
	if env["DB_SYNC_TABLES"] != "users,orders" {
		t.Fatalf("tables = %q, want users,orders", env["DB_SYNC_TABLES"])
	}
}

func TestLoadEnvFileRejectsInvalidAssignments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.env")
	if err := os.WriteFile(path, []byte("not-an-assignment\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadEnvFile(path); err == nil {
		t.Fatal("LoadEnvFile() error = nil, want invalid assignment error")
	}
}
