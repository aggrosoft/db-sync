package testkit

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type DatabaseContainer struct {
	DSN     string
	Cleanup func()
}

func StartPostgresContainer(ctx context.Context, t *testing.T) DatabaseContainer {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "app",
			"POSTGRES_USER":     "app",
			"POSTGRES_PASSWORD": "app-secret",
		},
		WaitingFor: wait.ForSQL(nat.Port("5432/tcp"), "pgx", func(host string, port nat.Port) string {
			return fmt.Sprintf("postgres://app:app-secret@%s:%s/app?sslmode=disable", host, port.Port())
		}).WithStartupTimeout(45 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		if shouldSkipContainerTests(err) {
			t.Skipf("postgres testcontainer unavailable: %v", err)
		}
		t.Fatalf("GenericContainer(postgres) error = %v", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("Host() error = %v", err)
	}
	port, err := container.MappedPort(ctx, nat.Port("5432/tcp"))
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("MappedPort() error = %v", err)
	}
	return DatabaseContainer{
		DSN: fmt.Sprintf("postgres://app:${PG_PASSWORD}@%s:%s/app?sslmode=disable", host, port.Port()),
		Cleanup: func() {
			_ = container.Terminate(ctx)
		},
	}
}

func StartMySQLContainer(ctx context.Context, t *testing.T) DatabaseContainer {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "mysql:8.4",
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MYSQL_DATABASE":      "app",
			"MYSQL_USER":          "app",
			"MYSQL_PASSWORD":      "app-secret",
			"MYSQL_ROOT_PASSWORD": "root-secret",
		},
		WaitingFor: wait.ForSQL(nat.Port("3306/tcp"), "mysql", func(host string, port nat.Port) string {
			return fmt.Sprintf("app:app-secret@tcp(%s:%s)/app?parseTime=true", host, port.Port())
		}).WithStartupTimeout(60 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		if shouldSkipContainerTests(err) {
			t.Skipf("mysql testcontainer unavailable: %v", err)
		}
		t.Fatalf("GenericContainer(mysql) error = %v", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("Host() error = %v", err)
	}
	port, err := container.MappedPort(ctx, nat.Port("3306/tcp"))
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("MappedPort() error = %v", err)
	}
	return DatabaseContainer{
		DSN: fmt.Sprintf("app:${MYSQL_PASSWORD}@tcp(%s:%s)/app?parseTime=true", host, port.Port()),
		Cleanup: func() {
			_ = container.Terminate(ctx)
		},
	}
}

func shouldSkipContainerTests(err error) bool {
	message := err.Error()
	return containsAny(message, []string{"Cannot connect to the Docker daemon", "Docker Desktop", "daemon", "docker"})
}

func containsAny(input string, parts []string) bool {
	for _, part := range parts {
		if strings.Contains(strings.ToLower(input), strings.ToLower(part)) {
			return true
		}
	}
	return false
}
