package schema

import (
	"errors"
	"reflect"
	"testing"

	"db-sync/internal/model"
)

func TestNormalizeSnapshot(t *testing.T) {
	input := Snapshot{
		Role:   "source",
		Engine: model.EnginePostgres,
		Tables: []Table{
			{
				ID:          TableID{Schema: "app", Name: "orders"},
				Columns:     []Column{{Name: "created_at", Ordinal: 3}, {Name: "id", Ordinal: 1, Identity: true}, {Name: "user_id", Ordinal: 2}},
				ForeignKeys: []ForeignKey{{Name: "orders_user_fk", Columns: []string{"user_id"}, ReferencedTable: TableID{Schema: "public", Name: "users"}, ReferencedColumns: []string{"id"}}},
			},
			{
				ID:      TableID{Schema: "public", Name: "users"},
				Columns: []Column{{Name: "email", Ordinal: 2, Nullable: false, HasProvenDefault: false, NativeType: "text"}, {Name: "id", Ordinal: 1, Identity: true, NativeType: "int8"}},
			},
		},
	}

	normalized := NormalizeSnapshot(input)
	if got := normalized.Tables[0].ID.String(); got != "app.orders" {
		t.Fatalf("first table = %q, want %q", got, "app.orders")
	}
	if got := normalized.Tables[1].ID.String(); got != "public.users" {
		t.Fatalf("second table = %q, want %q", got, "public.users")
	}
	if got := normalized.Tables[0].Columns[0].Name; got != "id" {
		t.Fatalf("first ordered column = %q, want id", got)
	}
	users := normalized.Tables[1]
	if !reflect.DeepEqual(users.Columns[1], Column{Name: "email", Ordinal: 2, Nullable: false, HasProvenDefault: false, NativeType: "text"}) {
		t.Fatalf("users email column = %+v", users.Columns[1])
	}
	if !users.Columns[0].Identity {
		t.Fatal("expected primary identity metadata to be preserved")
	}
}

func TestParseTableIDPreservesQualifiedIdentity(t *testing.T) {
	tests := []struct {
		input string
		want  TableID
	}{
		{input: "public.users", want: TableID{Schema: "public", Name: "users"}},
		{input: "orders", want: TableID{Name: "orders"}},
	}
	for _, tt := range tests {
		if got := ParseTableID(tt.input); got != tt.want {
			t.Fatalf("ParseTableID(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

func TestBlockedErrorUnwrap(t *testing.T) {
	root := errors.New("metadata denied")
	err := NewBlockedError("source", model.EnginePostgres, "schema discovery blocked", []string{"grant metadata visibility"}, root)
	if !errors.Is(err, root) {
		t.Fatal("blocked error should unwrap the root cause")
	}
}
