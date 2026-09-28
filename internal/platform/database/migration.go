package database

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migrationpgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func NewMigrator(pool *Pool, directory string) (*migrate.Migrate, error) {
	if pool == nil {
		return nil, errors.New("migrations require DB_ENABLED=true")
	}
	source, err := iofs.New(os.DirFS(directory), ".")
	if err != nil {
		return nil, errors.New("cannot read migration directory")
	}
	driver, err := migrationpgx.WithInstance(pool.DB, &migrationpgx.Config{StatementTimeout: time.Minute})
	if err != nil {
		_ = source.Close()
		return nil, errors.New("cannot initialize migration database")
	}
	migrator, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		_ = source.Close()
		_ = driver.Close()
		return nil, errors.New("cannot initialize migrations")
	}
	migrator.LockTimeout = 10 * time.Second
	return migrator, nil
}

func MigrationError(err error) error {
	if err == nil || errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	var dirty migrate.ErrDirty
	if errors.As(err, &dirty) {
		return fmt.Errorf("migration version %d is dirty; inspect and repair before forcing a version", dirty.Version)
	}
	if errors.Is(err, migrate.ErrLockTimeout) {
		return errors.New("migration lock timed out; another migration may be running")
	}
	return errors.New("migration failed; inspect database state and server diagnostics before retrying (driver details suppressed)")
}
