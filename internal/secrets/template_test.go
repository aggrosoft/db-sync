package secrets

import (
	"strings"
	"testing"
)

func TestPlaceholderParsingDoesNotExposeResolvedValue(t *testing.T) {
	resolved, err := ResolveTemplate("postgres://app:${SRC_DB_PASSWORD}@localhost/db", map[string]string{"SRC_DB_PASSWORD": "super-secret"})
	if err != nil {
		t.Fatalf("ResolveTemplate() error = %v", err)
	}
	if resolved.String() == "super-secret" || strings.Contains(resolved.String(), "super-secret") {
		t.Fatalf("resolved template string exposed secret: %q", resolved.String())
	}
	if got := resolved.Placeholders(); len(got) != 1 || got[0] != "SRC_DB_PASSWORD" {
		t.Fatalf("unexpected placeholders: %v", got)
	}
}

func TestInvalidPlaceholdersFail(t *testing.T) {
	tests := []string{"postgres://${db_password}", "postgres://${BAD-NAME}"}
	for _, input := range tests {
		if _, err := ParsePlaceholders(input); err == nil {
			t.Fatalf("ParsePlaceholders(%q) expected error", input)
		}
	}
}
