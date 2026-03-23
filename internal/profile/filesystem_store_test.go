package profile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"db-sync/internal/model"
)

func TestFilesystemStoreSaveLoadList(t *testing.T) {
	ctx := context.Background()
	store := NewFilesystemStore(t.TempDir(), "db-sync")
	candidate := model.DefaultProfile("Customer Copy")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeDetails
	candidate.Source.Connection.Details = model.ConnectionDetails{Host: "localhost", Port: 5432, Database: "source", Username: "app", Password: "source-secret"}
	candidate.Target.Engine = model.EnginePostgres
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString = model.ConnectionString{Value: "postgres://app:target-secret@localhost/target"}

	path, err := store.Save(ctx, candidate)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("db-sync", "profiles", "customer-copy.yaml")) {
		t.Fatalf("Save() path = %q", path)
	}
	envPath := EnvPathForProfilePath(path)
	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", envPath, err)
	}
	if !strings.Contains(string(envData), "source-secret") || !strings.Contains(string(envData), "target-secret") {
		t.Fatalf("env file missing expected secrets: %s", string(envData))
	}
	loaded, err := store.Load(ctx, candidate.Name)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Name != candidate.Name {
		t.Fatalf("Load().Name = %q, want %q", loaded.Name, candidate.Name)
	}
	if loaded.Source.Connection.Details.Password == "" || loaded.Target.Connection.ConnectionString.Value == "" {
		t.Fatalf("Load() did not hydrate secret-backed fields: %+v", loaded)
	}
	profiles, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != candidate.Name {
		t.Fatalf("List() = %+v", profiles)
	}
}

func TestFilesystemStoreLoadsLegacyTemplateProfiles(t *testing.T) {
	ctx := context.Background()
	store := NewFilesystemStore(t.TempDir(), "db-sync")
	path, err := store.PathFor("legacy")
	if err != nil {
		t.Fatalf("PathFor() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := []byte("version: 1\nname: legacy\nsource:\n  engine: postgres\n  dsn_template: postgres://app:${SRC_DB_PASSWORD}@localhost/source\ntarget:\n  engine: postgres\n  dsn_template: postgres://app:${TGT_DB_PASSWORD}@localhost/target\nselection:\n  tables: []\nsync:\n  mode: insert-missing\n  mirror_delete: false\n")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded, err := store.Load(ctx, "legacy")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Source.EffectiveConnectionMode() != model.ConnectionModeLegacyTemplate {
		t.Fatalf("source mode = %q, want legacy-template", loaded.Source.EffectiveConnectionMode())
	}
}
