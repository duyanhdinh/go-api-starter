package config

import (
	"fmt"
	"strconv"
	"time"
)

type DatabaseConfig struct {
	Enabled         bool
	Provider        string
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ConnectTimeout  time.Duration
	PingTimeout     time.Duration
}

func LoadDatabase(getenv func(string) string) (DatabaseConfig, error) {
	configuration := DatabaseConfig{Provider: "postgres", MaxOpenConns: 10, MaxIdleConns: 2, ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute, ConnectTimeout: 5 * time.Second, PingTimeout: 2 * time.Second}
	if value := getenv("DB_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("DB_ENABLED must be a boolean")
		}
		configuration.Enabled = enabled
	}
	if !configuration.Enabled {
		return configuration, nil
	}
	configuration.URL = getenv("DATABASE_URL")
	if value := getenv("DB_PROVIDER"); value != "" {
		configuration.Provider = value
	}
	for _, setting := range []struct {
		name   string
		target *int
	}{
		{"DB_MAX_OPEN_CONNS", &configuration.MaxOpenConns}, {"DB_MAX_IDLE_CONNS", &configuration.MaxIdleConns},
	} {
		if value := getenv(setting.name); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return DatabaseConfig{}, fmt.Errorf("%s must be an integer", setting.name)
			}
			*setting.target = parsed
		}
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"DB_CONN_MAX_LIFETIME", &configuration.ConnMaxLifetime}, {"DB_CONN_MAX_IDLE_TIME", &configuration.ConnMaxIdleTime},
		{"DB_CONNECT_TIMEOUT", &configuration.ConnectTimeout}, {"DB_PING_TIMEOUT", &configuration.PingTimeout},
	} {
		if value := getenv(setting.name); value != "" {
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return DatabaseConfig{}, fmt.Errorf("%s must be a positive duration", setting.name)
			}
			*setting.target = parsed
		}
	}
	if err := configuration.Validate(); err != nil {
		return DatabaseConfig{}, err
	}
	return configuration, nil
}

func (configuration DatabaseConfig) Validate() error {
	if !configuration.Enabled {
		return nil
	}
	if configuration.Provider != "postgres" {
		return fmt.Errorf("DB_PROVIDER is unsupported; use postgres")
	}
	if configuration.URL == "" {
		return fmt.Errorf("DATABASE_URL is required when DB_ENABLED=true")
	}
	if configuration.MaxOpenConns <= 0 || configuration.MaxIdleConns < 0 || configuration.MaxIdleConns > configuration.MaxOpenConns {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be positive; DB_MAX_IDLE_CONNS must be between zero and DB_MAX_OPEN_CONNS")
	}
	if configuration.ConnMaxLifetime <= 0 || configuration.ConnMaxIdleTime <= 0 || configuration.ConnMaxIdleTime > configuration.ConnMaxLifetime {
		return fmt.Errorf("DB_CONN_MAX_LIFETIME and DB_CONN_MAX_IDLE_TIME must be positive; idle time must not exceed lifetime")
	}
	if configuration.ConnectTimeout <= 0 || configuration.PingTimeout <= 0 {
		return fmt.Errorf("DB_CONNECT_TIMEOUT and DB_PING_TIMEOUT must be positive")
	}
	return nil
}
