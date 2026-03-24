package wizard

import (
	"strings"
	"testing"

	"db-sync/internal/model"
	profilepkg "db-sync/internal/profile"
	"db-sync/internal/schema"
)

func TestDraftDefaults(t *testing.T) {
	draft := NewDraft()
	if draft.SyncMode != model.SyncModeInsertMissing {
		t.Fatalf("SyncMode = %q, want %q", draft.SyncMode, model.SyncModeInsertMissing)
	}
	if draft.Source.Mode != model.ConnectionModeConnectionString || draft.Target.Mode != model.ConnectionModeConnectionString {
		t.Fatalf("draft endpoint modes = %q/%q, want connection-string defaults", draft.Source.Mode, draft.Target.Mode)
	}
	if draft.MirrorDelete {
		t.Fatal("MirrorDelete = true, want false")
	}
}

func TestEditPrefillUsesProfileValues(t *testing.T) {
	profile := model.DefaultProfile("existing")
	profile.Source.Engine = model.EnginePostgres
	profile.Source.Connection.Mode = model.ConnectionModeDetails
	profile.Source.Connection.Details = model.ConnectionDetails{
		Host:        "localhost",
		Port:        5432,
		Database:    "source",
		Username:    "app",
		Password:    "source-secret",
		PasswordEnv: profilepkg.PasswordEnvVar(profile.Name, "source"),
		SSLMode:     "disable",
	}
	profile.Target.Engine = model.EngineMySQL
	profile.Target.Connection.Mode = model.ConnectionModeConnectionString
	profile.Target.Connection.ConnectionString = model.ConnectionString{
		Value:  "app:secret@tcp(localhost:3306)/target",
		EnvVar: profilepkg.ConnectionStringEnvVar(profile.Name, "target"),
	}
	profile.Sync.MirrorDelete = true

	draft := FromProfile(profile)
	if draft.Name != "existing" || draft.Source.Engine != model.EnginePostgres || draft.Target.Engine != model.EngineMySQL {
		t.Fatalf("draft did not prefill expected values: %+v", draft)
	}
	if draft.Source.Host != "localhost" || draft.Target.ConnectionString == "" {
		t.Fatalf("draft did not preserve endpoint values: %+v", draft)
	}
	if !draft.MirrorDelete {
		t.Fatal("draft MirrorDelete = false, want true")
	}
}

func TestReviewRendererShowsPlaceholdersAndSyncSettings(t *testing.T) {
	profile := model.DefaultProfile("review-me")
	profile.Source.Engine = model.EnginePostgres
	profile.Source.Connection.Mode = model.ConnectionModeDetails
	profile.Source.Connection.Details = model.ConnectionDetails{
		Host:        "db.local",
		Port:        5432,
		Database:    "source",
		Username:    "app",
		PasswordEnv: profilepkg.PasswordEnvVar(profile.Name, "source"),
		SSLMode:     "disable",
	}
	profile.Target.Engine = model.EnginePostgres
	profile.Target.Connection.Mode = model.ConnectionModeConnectionString
	profile.Target.Connection.ConnectionString = model.ConnectionString{
		Value:  "postgres://app:super-secret@localhost/target",
		EnvVar: profilepkg.ConnectionStringEnvVar(profile.Name, "target"),
	}
	preview := schema.SelectionPreview{
		ExplicitIncludes:  []schema.TableID{{Schema: "public", Name: "orders"}},
		RequiredTables:    []schema.TableID{{Schema: "public", Name: "customers"}},
		BlockedExclusions: []schema.BlockedExclusion{{Table: schema.TableID{Schema: "public", Name: "regions"}, RequiredBy: []schema.TableID{{Schema: "public", Name: "orders"}}}},
		FinalTables:       []schema.TableID{{Schema: "public", Name: "customers"}, {Schema: "public", Name: "orders"}},
		Blocked:           true,
	}
	profile.Sync.MirrorDelete = true

	review := RenderReview(profile, &preview)
	for _, want := range []string{profilepkg.PasswordEnvVar(profile.Name, "source"), profilepkg.ConnectionStringEnvVar(profile.Name, "target"), "mirror_delete=true", profilepkg.EnvFileName(profile.Name), "Explicit selections: public.orders", "Required additions: public.customers", "Blocked exclusions: public.regions (required by public.orders)"} {
		if !strings.Contains(review, want) {
			t.Fatalf("review output missing %q: %s", want, review)
		}
	}
	if strings.Contains(review, "super-secret") {
		t.Fatalf("review output leaked resolved secret: %s", review)
	}
	if strings.Contains(review, "Tables: [") {
		t.Fatalf("review output still uses raw table slice formatting: %s", review)
	}
}
