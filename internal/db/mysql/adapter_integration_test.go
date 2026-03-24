//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/schema"
	"db-sync/internal/testkit"

	_ "github.com/go-sql-driver/mysql"
)

func TestMySQLDiscoverSchema(t *testing.T) {
	ctx := context.Background()
	container := testkit.StartMySQLContainer(ctx, t)
	defer container.Cleanup()
	dsn := strings.ReplaceAll(container.DSN, "${MYSQL_PASSWORD}", "app-secret")
	seedMySQLSchema(ctx, t, dsn)

	adapter := NewAdapter()
	snapshot, err := adapter.DiscoverSourceSchema(ctx, dsn, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverSourceSchema() error = %v", err)
	}
	orders := mustFindMySQLTable(t, snapshot, "app.orders")
	if len(orders.Columns) != 4 {
		t.Fatalf("orders columns = %d, want 4", len(orders.Columns))
	}
	if len(orders.ForeignKeys) != 1 {
		t.Fatalf("orders foreign keys = %d, want 1", len(orders.ForeignKeys))
	}
	if got := orders.ForeignKeys[0].ReferencedTable.String(); got != "app.accounts" {
		t.Fatalf("referenced table = %q, want app.accounts", got)
	}
}

func seedMySQLSchema(ctx context.Context, t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	statements := []string{
		"create table if not exists accounts (id bigint primary key auto_increment, email varchar(255) not null)",
		"create table if not exists orders (id bigint primary key auto_increment, account_id bigint not null, created_at timestamp not null default current_timestamp, computed_account bigint generated always as (account_id + 1) virtual, constraint orders_account_fk foreign key (account_id) references accounts(id))",
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("ExecContext(%q) error = %v", statement, err)
		}
	}
}

func mustFindMySQLTable(t *testing.T, snapshot schema.Snapshot, tableID string) schema.Table {
	t.Helper()
	table, ok := snapshot.TableByID(schema.ParseTableID(tableID))
	if !ok {
		t.Fatalf("table %q not found in snapshot", tableID)
	}
	return table
}
