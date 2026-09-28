//go:build integration

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"server/internal/platform/config"
	"server/internal/platform/database"
)

func TestPostgresIntegration(t *testing.T) {
	connectionURL := os.Getenv("TEST_DATABASE_URL")
	if connectionURL == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated disposable PostgreSQL database")
	}
	configuration, err := config.LoadDatabase(func(name string) string {
		return map[string]string{"DB_ENABLED": "true", "DATABASE_URL": connectionURL, "DB_PING_TIMEOUT": "100ms", "DB_MAX_OPEN_CONNS": "1", "DB_MAX_IDLE_CONNS": "1"}[name]
	})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	if _, err := admin.DB.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal("cannot create isolated test schema")
	}
	defer func() {
		if _, err := admin.DB.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error("cannot remove generated test schema")
		}
	}()
	parsed, err := url.Parse(connectionURL)
	if err != nil {
		t.Fatal("invalid test URL")
	}
	parameters := parsed.Query()
	parameters.Set("search_path", schema)
	parsed.RawQuery = parameters.Encode()
	configuration.URL = parsed.String()
	pool, err := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var result int
	if err := pool.DB.QueryRow("SELECT 1").Scan(&result); err != nil || result != 1 {
		t.Fatal("query failed")
	}
	directory := t.TempDir()
	for filename, contents := range map[string]string{"1_probe.up.sql": "CREATE TABLE migration_probe (id integer PRIMARY KEY);", "1_probe.down.sql": "DROP TABLE migration_probe;"} {
		if err := os.WriteFile(filepath.Join(directory, filename), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	migrator, err := database.NewMigrator(pool, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	if err := migrator.Up(); err != nil {
		t.Fatal(database.MigrationError(err))
	}
	version, dirty, err := migrator.Version()
	if err != nil || dirty || version != 1 {
		t.Fatal("unexpected migration status after up")
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(database.MigrationError(err))
	}
	if _, _, err := migrator.Version(); err != migrate.ErrNilVersion {
		t.Fatal("unexpected migration status after down")
	}
	if sourceErr, databaseErr := migrator.Close(); sourceErr != nil || databaseErr != nil {
		t.Fatal("migration close failed")
	}
	pool, err = database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	handler := newHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), pool.Ping)
	server := httptest.NewServer(handler)
	defer server.Close()
	check := func(path string, status int) {
		t.Helper()
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("%s status=%d want=%d", path, response.StatusCode, status)
		}
	}
	check("/health", 200)
	check("/ready", 200)
	connection, err := pool.DB.Conn(context.Background())
	if err != nil {
		t.Fatal("cannot occupy pool")
	}
	start := time.Now()
	check("/ready", 503)
	if time.Since(start) > time.Second {
		t.Fatal("readiness timeout not bounded")
	}
	check("/health", 200)
	connection.Close()
	check("/ready", 200)
	pool.Close()
	check("/ready", 503)
}
