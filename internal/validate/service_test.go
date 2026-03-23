package validate

import (
	"context"
	"errors"
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/profile"
)

type fakeAdapter struct {
	source profile.EndpointValidation
	target profile.EndpointValidation
	srcErr error
	tgtErr error
}

func (adapter fakeAdapter) ValidateSource(_ context.Context, _ string, _ model.Engine) (profile.EndpointValidation, error) {
	return adapter.source, adapter.srcErr
}

func (adapter fakeAdapter) ValidateTarget(_ context.Context, _ string, _ model.Engine) (profile.EndpointValidation, error) {
	return adapter.target, adapter.tgtErr
}

func TestValidateProfileMissingRequiredEnvironmentVariables(t *testing.T) {
	store := profile.NewFilesystemStore(t.TempDir(), "db-sync")
	service := NewService(store, func() map[string]string { return map[string]string{} }, Registry{})
	candidate := model.DefaultProfile("missing-env")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString = model.ConnectionString{EnvVar: profile.ConnectionStringEnvVar(candidate.Name, "source")}
	candidate.Target.Engine = model.EnginePostgres
	candidate.Target.Connection.Mode = model.ConnectionModeDetails
	candidate.Target.Connection.Details = model.ConnectionDetails{Host: "localhost", Port: 5432, Database: "target", Username: "app", PasswordEnv: profile.PasswordEnvVar(candidate.Name, "target"), SSLMode: "disable"}

	report, err := service.ValidateProfile(context.Background(), candidate)
	if err == nil {
		t.Fatal("ValidateProfile() error = nil, want missing env error")
	}
	if len(report.MissingEnv) != 2 {
		t.Fatalf("MissingEnv = %v, want two entries", report.MissingEnv)
	}
}

func TestValidateProfileConnectionFirstModes(t *testing.T) {
	store := profile.NewFilesystemStore(t.TempDir(), "db-sync")
	service := NewService(store, func() map[string]string { return map[string]string{} }, Registry{
		model.EnginePostgres: fakeAdapter{
			source: profile.EndpointValidation{Role: "source", Engine: model.EnginePostgres, Status: profile.StatusPassed},
			target: profile.EndpointValidation{Role: "target", Engine: model.EnginePostgres, Status: profile.StatusPassed},
		},
	})
	candidate := model.DefaultProfile("connection-first")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeDetails
	candidate.Source.Connection.Details = model.ConnectionDetails{Host: "localhost", Port: 5432, Database: "source", Username: "app", Password: "source-secret", PasswordEnv: profile.PasswordEnvVar(candidate.Name, "source"), SSLMode: "disable"}
	candidate.Target.Engine = model.EnginePostgres
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString = model.ConnectionString{Value: "postgres://app:target-secret@localhost/target", EnvVar: profile.ConnectionStringEnvVar(candidate.Name, "target")}

	report, err := service.ValidateProfile(context.Background(), candidate)
	if err != nil {
		t.Fatalf("ValidateProfile() error = %v", err)
	}
	if report.Blocked {
		t.Fatal("ValidateProfile() blocked = true, want false")
	}
}

func TestValidateAndSaveBlockedSave(t *testing.T) {
	store := profile.NewFilesystemStore(t.TempDir(), "db-sync")
	service := NewService(store, func() map[string]string {
		return map[string]string{profile.ConnectionStringEnvVar("blocked-save", "source"): "postgres://app:source@localhost/source"}
	}, Registry{
		model.EnginePostgres: fakeAdapter{
			source: profile.EndpointValidation{Role: "source", Engine: model.EnginePostgres, Status: profile.StatusPassed},
			target: profile.EndpointValidation{Role: "target", Engine: model.EnginePostgres, Status: profile.StatusFailed},
			tgtErr: errors.New("blocked save"),
		},
	})
	candidate := model.DefaultProfile("blocked-save")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString = model.ConnectionString{EnvVar: profile.ConnectionStringEnvVar(candidate.Name, "source")}
	candidate.Target.Engine = model.EnginePostgres
	candidate.Target.Connection.Mode = model.ConnectionModeDetails
	candidate.Target.Connection.Details = model.ConnectionDetails{Host: "localhost", Port: 5432, Database: "target", Username: "app", Password: "target", PasswordEnv: profile.PasswordEnvVar(candidate.Name, "target"), SSLMode: "disable"}

	report, err := service.ValidateAndSave(context.Background(), candidate)
	if err == nil {
		t.Fatal("ValidateAndSave() error = nil, want blocked save")
	}
	if report.SavedPath != "" {
		t.Fatalf("SavedPath = %q, want empty", report.SavedPath)
	}
	if _, loadErr := store.Load(context.Background(), candidate.Name); !errors.Is(loadErr, profile.ErrProfileNotFound) {
		t.Fatalf("Load() error = %v, want ErrProfileNotFound", loadErr)
	}
}
