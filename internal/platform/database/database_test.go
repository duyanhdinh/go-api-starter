package database

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"server/internal/platform/config"
)

func TestDisabled(t *testing.T) {
	pool, err := Open(context.Background(), config.DatabaseConfig{})
	if err != nil || pool != nil {
		t.Fatal("disabled database opened")
	}
	if pool.Ping(context.Background()) != nil || pool.Close() != nil {
		t.Fatal("disabled lifecycle failed")
	}
}

func TestInvalidURL(t *testing.T) {
	for _, connectionURL := range []string{"secret", "postgres://user:secret@host", "postgres://user:secret@host/db?sslmode=secret", "mysql://user:secret@host/db"} {
		configuration, _ := config.LoadDatabase(func(name string) string {
			return map[string]string{"DB_ENABLED": "true", "DATABASE_URL": connectionURL}[name]
		})
		_, err := Open(context.Background(), configuration)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("expected sanitized error, got %v", err)
		}
	}
}

func TestStartupTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	configuration, _ := config.LoadDatabase(func(name string) string {
		return map[string]string{"DB_ENABLED": "true", "DATABASE_URL": "postgres://user:secret@" + listener.Addr().String() + "/test?sslmode=disable", "DB_PING_TIMEOUT": "30ms", "DB_CONNECT_TIMEOUT": "30ms"}[name]
	})
	start := time.Now()
	pool, err := Open(context.Background(), configuration)
	if err == nil || pool != nil || strings.Contains(err.Error(), "secret") || time.Since(start) > time.Second {
		t.Fatalf("startup did not fail safely within timeout: %v", err)
	}
}

func TestMigrationErrors(t *testing.T) {
	for _, err := range []error{errors.New("password=secret SQL secret"), migrate.ErrDirty{Version: 3}, migrate.ErrLockTimeout} {
		if cleaned := MigrationError(err); cleaned == nil || strings.Contains(cleaned.Error(), "secret") {
			t.Fatalf("unsafe migration error: %v", cleaned)
		}
	}
	if MigrationError(migrate.ErrNoChange) != nil || MigrationError(nil) != nil {
		t.Fatal("no-change treated as failure")
	}
}
