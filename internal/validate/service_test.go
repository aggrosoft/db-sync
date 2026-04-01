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
	service := NewService(func() map[string]string { return map[string]string{} }, Registry{})
	candidate := model.DefaultProfile("missing-env")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString = model.ConnectionString{EnvVar: "SRC_DSN"}
	candidate.Target.Engine = model.EnginePostgres
	candidate.Target.Connection.Mode = model.ConnectionModeDetails
	candidate.Target.Connection.Details = model.ConnectionDetails{Host: "localhost", Port: 5432, Database: "target", Username: "app", PasswordEnv: "TGT_PASSWORD", SSLMode: "disable"}

	report, err := service.ValidateProfile(context.Background(), candidate)
	if err == nil {
		t.Fatal("ValidateProfile() error = nil, want missing env error")
	}
	if len(report.MissingEnv) != 2 {
		t.Fatalf("MissingEnv = %v, want two entries", report.MissingEnv)
	}
}

func TestValidateProfileConnectionFirstModes(t *testing.T) {
	service := NewService(func() map[string]string { return map[string]string{} }, Registry{
		model.EnginePostgres: fakeAdapter{
			source: profile.EndpointValidation{Role: "source", Engine: model.EnginePostgres, Status: profile.StatusPassed},
			target: profile.EndpointValidation{Role: "target", Engine: model.EnginePostgres, Status: profile.StatusPassed},
		},
	})
	candidate := model.DefaultProfile("connection-first")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeDetails
	candidate.Source.Connection.Details = model.ConnectionDetails{Host: "localhost", Port: 5432, Database: "source", Username: "app", Password: "source-secret", SSLMode: "disable"}
	candidate.Target.Engine = model.EnginePostgres
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString = model.ConnectionString{Value: "postgres://app:target-secret@localhost/target"}

	report, err := service.ValidateProfile(context.Background(), candidate)
	if err != nil {
		t.Fatalf("ValidateProfile() error = %v", err)
	}
	if report.Blocked {
		t.Fatal("ValidateProfile() blocked = true, want false")
	}
}

func TestValidateProfileReturnsBlockedReportForAdapterFailures(t *testing.T) {
	service := NewService(func() map[string]string {
		return map[string]string{"SRC_DSN": "postgres://app:source@localhost/source"}
	}, Registry{
		model.EnginePostgres: fakeAdapter{
			source: profile.EndpointValidation{Role: "source", Engine: model.EnginePostgres, Status: profile.StatusPassed},
			target: profile.EndpointValidation{Role: "target", Engine: model.EnginePostgres, Status: profile.StatusFailed},
			tgtErr: errors.New("blocked validation"),
		},
	})
	candidate := model.DefaultProfile("blocked-save")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString = model.ConnectionString{EnvVar: "SRC_DSN"}
	candidate.Target.Engine = model.EnginePostgres
	candidate.Target.Connection.Mode = model.ConnectionModeDetails
	candidate.Target.Connection.Details = model.ConnectionDetails{Host: "localhost", Port: 5432, Database: "target", Username: "app", Password: "target", SSLMode: "disable"}

	report, err := service.ValidateProfile(context.Background(), candidate)
	if err == nil {
		t.Fatal("ValidateProfile() error = nil, want blocked validation")
	}
	if !report.Blocked {
		t.Fatal("ValidateProfile() blocked = false, want true")
	}
	if report.Summary != "blocked validation" {
		t.Fatalf("Summary = %q, want blocked validation", report.Summary)
	}
}
