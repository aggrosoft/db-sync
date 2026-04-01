package profile

import (
	"testing"

	"db-sync/internal/model"

	"github.com/google/go-cmp/cmp"
)

func TestNormalizeProfileAppliesDefaultsWithoutName(t *testing.T) {
	candidate := model.DefaultProfile("")
	candidate.Source.Engine = model.EnginePostgres
	candidate.Source.Connection.Mode = model.ConnectionModeDetails
	candidate.Source.Connection.Details = model.ConnectionDetails{
		Host:     "localhost",
		Database: "source",
		Username: "app",
		Password: "source-secret",
	}
	candidate.Target.Engine = model.EngineMariaDB
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString = model.ConnectionString{Value: "dev:dev@tcp(localhost:3307)/target"}
	candidate.Selection.Tables = []string{"public.orders", "public.orders", " public.order_items "}
	candidate.Selection.ExcludedTables = []string{" public.logs ", "public.logs", "public.audit"}

	normalized, err := NormalizeProfile(candidate)
	if err != nil {
		t.Fatalf("NormalizeProfile() error = %v", err)
	}
	if normalized.Name != "" {
		t.Fatalf("NormalizeProfile().Name = %q, want empty", normalized.Name)
	}
	if normalized.Source.Connection.Details.Port != 5432 {
		t.Fatalf("source port = %d, want 5432", normalized.Source.Connection.Details.Port)
	}
	if normalized.Source.Connection.Details.SSLMode != "disable" {
		t.Fatalf("source sslmode = %q, want disable", normalized.Source.Connection.Details.SSLMode)
	}
	if normalized.Target.Connection.Mode != model.ConnectionModeConnectionString {
		t.Fatalf("target mode = %q, want %q", normalized.Target.Connection.Mode, model.ConnectionModeConnectionString)
	}
	if diff := cmp.Diff([]string{"public.orders", "public.order_items"}, normalized.Selection.Tables); diff != "" {
		t.Fatalf("selection tables mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"public.logs", "public.audit"}, normalized.Selection.ExcludedTables); diff != "" {
		t.Fatalf("excluded tables mismatch (-want +got):\n%s", diff)
	}
}

func TestNormalizeProfileRequiresPasswordOrReference(t *testing.T) {
	candidate := model.DefaultProfile("")
	candidate.Source.Engine = model.EngineMariaDB
	candidate.Source.Connection.Mode = model.ConnectionModeDetails
	candidate.Source.Connection.Details = model.ConnectionDetails{
		Host:     "localhost",
		Database: "source",
		Username: "app",
	}
	candidate.Target.Engine = model.EngineMariaDB
	candidate.Target.Connection.Mode = model.ConnectionModeDetails
	candidate.Target.Connection.Details = model.ConnectionDetails{
		Host:     "localhost",
		Database: "target",
		Username: "app",
		Password: "target-secret",
	}

	if _, err := NormalizeProfile(candidate); err == nil {
		t.Fatal("NormalizeProfile() error = nil, want password validation error")
	}
}
