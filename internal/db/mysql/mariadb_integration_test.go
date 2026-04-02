//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"db-sync/internal/model"
	"db-sync/internal/schema"

	"github.com/docker/go-connections/nat"
	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMariaDBDiscoverSchema(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "mariadb:11.4",
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MARIADB_DATABASE":      "app",
			"MARIADB_USER":          "app",
			"MARIADB_PASSWORD":      "app-secret",
			"MARIADB_ROOT_PASSWORD": "root-secret",
		},
		WaitingFor: wait.ForSQL(nat.Port("3306/tcp"), "mysql", func(host string, port nat.Port) string {
			return fmt.Sprintf("app:app-secret@tcp(%s:%s)/app?parseTime=true", host, port.Port())
		}).WithStartupTimeout(60 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Skipf("mariadb testcontainer unavailable: %v", err)
	}
	defer func() { _ = container.Terminate(ctx) }()
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Host() error = %v", err)
	}
	port, err := container.MappedPort(ctx, nat.Port("3306/tcp"))
	if err != nil {
		t.Fatalf("MappedPort() error = %v", err)
	}
	dsn := fmt.Sprintf("app:app-secret@tcp(%s:%s)/app?parseTime=true", host, port.Port())
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "create table if not exists accounts (id bigint primary key auto_increment, email varchar(255) not null)"); err != nil {
		t.Fatalf("create accounts error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "create table if not exists invoices (id bigint primary key auto_increment, account_id bigint not null, created_at timestamp not null default current_timestamp, constraint invoices_account_fk foreign key (account_id) references accounts(id))"); err != nil {
		t.Fatalf("create invoices error = %v", err)
	}
	adapter := NewAdapter()
	if _, err := adapter.ValidateTarget(ctx, dsn, model.EngineMariaDB); err != nil {
		t.Fatalf("ValidateTarget() error = %v", err)
	}
	snapshot, err := adapter.DiscoverSourceSchema(ctx, dsn, model.EngineMariaDB)
	if err != nil {
		t.Fatalf("DiscoverSourceSchema() error = %v", err)
	}
	if _, ok := snapshot.TableByID(schema.ParseTableID("invoices")); !ok {
		t.Fatal("expected invoices table in MariaDB discovery snapshot")
	}
}
