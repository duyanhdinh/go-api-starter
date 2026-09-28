package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"server/internal/platform/config"
)

var ErrUnavailable = errors.New("database unavailable")

type Pool struct {
	DB          *sql.DB
	pingTimeout time.Duration
}

func Open(ctx context.Context, configuration config.DatabaseConfig) (*Pool, error) {
	if err := configuration.Validate(); err != nil {
		return nil, err
	}
	if !configuration.Enabled {
		return nil, nil
	}
	connectionURL, err := url.Parse(configuration.URL)
	if err != nil || (connectionURL.Scheme != "postgres" && connectionURL.Scheme != "postgresql") || connectionURL.Hostname() == "" || connectionURL.Path == "" || connectionURL.Path == "/" || connectionURL.User == nil || connectionURL.User.Username() == "" {
		return nil, errors.New("DATABASE_URL must be a PostgreSQL URL with host, user and database")
	}
	parameters := connectionURL.Query()
	if parameters.Get("sslmode") == "" {
		parameters.Set("sslmode", "verify-full")
	}
	connectionURL.RawQuery = parameters.Encode()
	driverConfig, err := pgx.ParseConfig(connectionURL.String())
	if err != nil {
		return nil, errors.New("DATABASE_URL contains invalid PostgreSQL configuration")
	}
	driverConfig.ConnectTimeout = configuration.ConnectTimeout
	pool := &Pool{DB: stdlib.OpenDB(*driverConfig), pingTimeout: configuration.PingTimeout}
	pool.DB.SetMaxOpenConns(configuration.MaxOpenConns)
	pool.DB.SetMaxIdleConns(configuration.MaxIdleConns)
	pool.DB.SetConnMaxLifetime(configuration.ConnMaxLifetime)
	pool.DB.SetConnMaxIdleTime(configuration.ConnMaxIdleTime)
	if err := pool.Ping(ctx); err != nil {
		_ = pool.Close()
		return nil, errors.New("database startup ping failed; check connectivity, credentials and TLS")
	}
	return pool, nil
}

func (pool *Pool) Ping(ctx context.Context) error {
	if pool == nil {
		return nil
	}
	pingContext, cancel := context.WithTimeout(ctx, pool.pingTimeout)
	defer cancel()
	if err := pool.DB.PingContext(pingContext); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (pool *Pool) Close() error {
	if pool == nil {
		return nil
	}
	if err := pool.DB.Close(); err != nil {
		return errors.New("database close failed")
	}
	return nil
}
