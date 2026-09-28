package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"server/internal/platform/config"
	"server/internal/platform/database"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, getenv func(string) string, output io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("usage: migrate create NAME | status | up | down COUNT | force VERSION")
	}
	directory := "migrations/postgres"
	if value := getenv("MIGRATIONS_DIR"); value != "" {
		directory = value
	}
	command := arguments[0]
	count := 0
	switch command {
	case "create":
		if len(arguments) != 2 || !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(arguments[1]) {
			return errors.New("create requires a lowercase snake_case name")
		}
		return createMigration(directory, arguments[1])
	case "status", "up":
		if len(arguments) != 1 {
			return errors.New("status and up accept no arguments")
		}
	case "down", "force":
		if len(arguments) != 2 {
			return errors.New("down requires a positive count; force requires a version")
		}
		parsed, err := strconv.Atoi(arguments[1])
		if err != nil || (command == "down" && parsed <= 0) || (command == "force" && parsed < -1) {
			return errors.New("invalid migration count or version")
		}
		count = parsed
	default:
		return errors.New("unknown migration command")
	}
	configuration, err := config.LoadDatabase(getenv)
	if err != nil {
		return err
	}
	pool, err := database.Open(context.Background(), configuration)
	if err != nil {
		return err
	}
	defer pool.Close()
	migrator, err := database.NewMigrator(pool, directory)
	if err != nil {
		return err
	}
	defer migrator.Close()
	switch command {
	case "status":
		version, dirty, err := migrator.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			_, err = fmt.Fprintln(output, "version=none dirty=false")
			return err
		}
		if err != nil {
			return database.MigrationError(err)
		}
		_, err = fmt.Fprintf(output, "version=%d dirty=%t\n", version, dirty)
		return err
	case "up":
		err = migrator.Up()
	case "down":
		err = migrator.Steps(-count)
	case "force":
		err = migrator.Force(count)
	}
	return database.MigrationError(err)
}

func createMigration(directory, name string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return errors.New("cannot create migration directory")
	}
	version := time.Now().UTC().Format("20060102150405")
	existing, err := filepath.Glob(filepath.Join(directory, version+"_*.sql"))
	if err != nil || len(existing) != 0 {
		return errors.New("migration timestamp already exists; retry in the next second")
	}
	prefix := version + "_" + name
	for _, direction := range []string{"up", "down"} {
		file, err := os.OpenFile(filepath.Join(directory, prefix+"."+direction+".sql"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return errors.New("cannot create migration file; check for existing files")
		}
		if err := file.Close(); err != nil {
			return errors.New("cannot close migration file")
		}
	}
	return nil
}
