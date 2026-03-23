package profile

import (
	"testing"

	"db-sync/internal/model"

	"github.com/google/go-cmp/cmp"
)

func TestProfileYAMLRoundTripPreservesDefaults(t *testing.T) {
	original := model.DefaultProfile("customer-copy")
	original.Source.Engine = model.EnginePostgres
	original.Source.DSNTemplate = "postgres://app:${SRC_DB_PASSWORD}@localhost:5432/source?sslmode=disable"
	original.Target.Engine = model.EnginePostgres
	original.Target.DSNTemplate = "postgres://app:${TGT_DB_PASSWORD}@localhost:5432/target?sslmode=disable"

	encoded, err := MarshalProfile(original)
	if err != nil {
		t.Fatalf("MarshalProfile() error = %v", err)
	}

	decoded, err := UnmarshalProfile(encoded)
	if err != nil {
		t.Fatalf("UnmarshalProfile() error = %v", err)
	}

	want, err := NormalizeProfile(original)
	if err != nil {
		t.Fatalf("NormalizeProfile() error = %v", err)
	}
	if diff := cmp.Diff(want, decoded); diff != "" {
		t.Fatalf("round trip mismatch (-want +got):\n%s", diff)
	}
}
