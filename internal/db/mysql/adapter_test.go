package mysql

import (
	"database/sql"
	"testing"

	"db-sync/internal/schema"
)

func TestParseReadOnlyValue(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    bool
		wantErr bool
	}{
		{name: "mariadb off bytes", value: []byte("OFF"), want: false},
		{name: "mariadb on bytes", value: []byte("ON"), want: true},
		{name: "mysql zero int", value: int64(0), want: false},
		{name: "mysql one int", value: int64(1), want: true},
		{name: "bool false", value: false, want: false},
		{name: "bool true", value: true, want: true},
		{name: "raw bytes", value: sql.RawBytes("0"), want: false},
		{name: "invalid string", value: "maybe", wantErr: true},
		{name: "nil value", value: nil, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseReadOnlyValue(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatal("parseReadOnlyValue() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseReadOnlyValue() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parseReadOnlyValue() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMySQLTableIDUsesNameOnly(t *testing.T) {
	if got := mysqlTableID("orders"); got != (schema.TableID{Name: "orders"}) {
		t.Fatalf("mysqlTableID() = %#v, want name-only table ID", got)
	}
}
