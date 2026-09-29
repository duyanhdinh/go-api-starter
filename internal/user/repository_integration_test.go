//go:build integration

package user

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"server/internal/platform/config"
	"server/internal/platform/database"
)

func TestPostgresCRUD(t *testing.T) {
	connectionURL := os.Getenv("TEST_DATABASE_URL")
	if connectionURL == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated disposable PostgreSQL database")
	}
	configuration, err := config.LoadDatabase(func(name string) string {
		return map[string]string{"DB_ENABLED": "true", "DATABASE_URL": connectionURL}[name]
	})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("user_test_%d", time.Now().UnixNano())
	if _, err := admin.DB.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal("cannot create test schema")
	}
	defer func() {
		if _, err := admin.DB.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error("cannot remove test schema")
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
	migrator, err := database.NewMigrator(pool, filepath.Join("..", "..", "migrations", "postgres"))
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	if err := migrator.Up(); err != nil {
		t.Fatal(database.MigrationError(err))
	}
	service := NewService(NewPostgresRepository(pool.DB))
	handler := testRouter(NewPostgresRepository(pool.DB))
	ctx := context.Background()
	created, err := service.Create(ctx, Input{Email: "An@EXAMPLE.com", Name: " Nguyễn An "})
	if err != nil {
		t.Fatal("create failed")
	}
	path := "/api/v1/users/" + strconv.FormatInt(created.ID, 10)
	if response := requestJSON(handler, "GET", path, ""); response.Code != 200 || !strings.Contains(response.Body.String(), "Nguyễn An") {
		t.Fatal("read over HTTP failed")
	}
	other, err := service.Create(ctx, Input{Email: "other@example.com", Name: "Other"})
	if err != nil {
		t.Fatal("second create failed")
	}
	if _, err := service.Update(ctx, other.ID, Input{Email: "AN@example.com", Name: "Conflict"}); err != ErrEmailTaken {
		t.Fatal("unique update failed")
	}
	if response := requestJSON(handler, "PUT", path, `{"email":"new@example.com","name":"New"}`); response.Code != 200 {
		t.Fatal("update over HTTP failed")
	}
	if response := requestJSON(handler, "GET", "/api/v1/users?limit=1&offset=1", ""); response.Code != 200 || !strings.Contains(response.Body.String(), "other@example.com") {
		t.Fatal("pagination failed")
	}
	var wait sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := service.Create(ctx, Input{Email: "race@example.com", Name: "Race"})
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	createdCount, conflictCount := 0, 0
	for err := range errors {
		switch err {
		case nil:
			createdCount++
		case ErrEmailTaken:
			conflictCount++
		default:
			t.Fatal("unexpected concurrent create error")
		}
	}
	if createdCount != 1 || conflictCount != 1 {
		t.Fatal("concurrent uniqueness failed")
	}
	if response := requestJSON(handler, "DELETE", path, ""); response.Code != 204 {
		t.Fatal("delete failed")
	}
	if response := requestJSON(handler, "GET", path, ""); response.Code != 404 {
		t.Fatal("deleted user still exists")
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(database.MigrationError(err))
	}
	var tableMissing bool
	if err := pool.DB.QueryRow("SELECT to_regclass('users') IS NULL").Scan(&tableMissing); err != nil || !tableMissing {
		t.Fatal("down migration did not remove users")
	}
	if err := migrator.Up(); err != nil {
		t.Fatal(database.MigrationError(err))
	}
	if records, err := service.List(ctx, 20, 0); err != nil || len(records) != 0 {
		t.Fatal("up after down failed")
	}
}
