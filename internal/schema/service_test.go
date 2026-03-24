package schema

import (
	"context"
	"errors"
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/validate"
)

type fakeDiscoveryAdapter struct {
	source Snapshot
	target Snapshot
	srcErr error
	tgtErr error
}

func (adapter fakeDiscoveryAdapter) DiscoverSourceSchema(_ context.Context, _ string, _ model.Engine) (Snapshot, error) {
	return adapter.source, adapter.srcErr
}

func (adapter fakeDiscoveryAdapter) DiscoverTargetSchema(_ context.Context, _ string, _ model.Engine) (Snapshot, error) {
	return adapter.target, adapter.tgtErr
}

func TestDiscoverProfileBlocksMissingEndpointEnvironment(t *testing.T) {
	service := NewService(func() map[string]string { return map[string]string{} }, Registry{}, validate.ResolveEndpoint)
	candidate := model.DefaultProfile("missing-env")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString.EnvVar = profile.ConnectionStringEnvVar(candidate.Name, "source")
	candidate.Target.Engine = model.EnginePostgres
	candidate.Target.Connection.Mode = model.ConnectionModeDetails
	candidate.Target.Connection.Details = model.ConnectionDetails{Host: "localhost", Database: "app", Username: "app", PasswordEnv: profile.PasswordEnvVar(candidate.Name, "target"), SSLMode: "disable"}

	report, err := service.DiscoverProfile(context.Background(), candidate)
	if err == nil {
		t.Fatal("DiscoverProfile() error = nil, want missing env error")
	}
	if !report.Blocked {
		t.Fatal("DiscoverProfile() blocked = false, want true")
	}
	if len(report.MissingEnv) != 2 {
		t.Fatalf("MissingEnv = %v, want 2 entries", report.MissingEnv)
	}
}

func TestDiscoverEndpointsBlocksMetadataEndpointFailures(t *testing.T) {
	service := NewService(func() map[string]string {
		return map[string]string{"SRC_DSN": "postgres://source", "TGT_DSN": "postgres://target"}
	}, Registry{
		model.EnginePostgres: fakeDiscoveryAdapter{
			srcErr: NewBlockedError("source", model.EnginePostgres, "metadata visibility is incomplete", []string{"grant SELECT on tables and catalog visibility"}, errors.New("permission denied for pg_constraint")),
		},
	}, validate.ResolveEndpoint)
	source := model.Endpoint{Engine: model.EnginePostgres, Connection: model.Connection{Mode: model.ConnectionModeConnectionString, ConnectionString: model.ConnectionString{EnvVar: "SRC_DSN"}}}
	target := model.Endpoint{Engine: model.EnginePostgres, Connection: model.Connection{Mode: model.ConnectionModeConnectionString, ConnectionString: model.ConnectionString{EnvVar: "TGT_DSN"}}}

	report, err := service.DiscoverEndpoints(context.Background(), source, target)
	if err == nil {
		t.Fatal("DiscoverEndpoints() error = nil, want blocked metadata error")
	}
	if !report.Blocked {
		t.Fatal("DiscoverEndpoints() blocked = false, want true")
	}
	if report.Source.Summary != "metadata visibility is incomplete" {
		t.Fatalf("Source summary = %q", report.Source.Summary)
	}
	if len(report.Source.Remediation) == 0 {
		t.Fatal("expected remediation guidance for blocked discovery")
	}
}
