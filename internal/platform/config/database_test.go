package config

import (
	"strings"
	"testing"
)

func TestDatabaseConfig(t *testing.T) {
	defaults, err := LoadDatabase(func(string) string { return "" })
	if err != nil || defaults.Enabled || defaults.Provider != "postgres" {
		t.Fatal("unexpected defaults")
	}
	for _, setting := range []struct{ name, value string }{
		{"DB_ENABLED", "secret"}, {"DB_PROVIDER", "secret"}, {"DATABASE_URL", ""},
		{"DB_MAX_OPEN_CONNS", "0"}, {"DB_MAX_OPEN_CONNS", "secret"}, {"DB_MAX_IDLE_CONNS", "11"}, {"DB_MAX_IDLE_CONNS", "-1"},
		{"DB_CONN_MAX_LIFETIME", "0s"}, {"DB_CONN_MAX_IDLE_TIME", "31m"}, {"DB_CONNECT_TIMEOUT", "secret"}, {"DB_PING_TIMEOUT", "-1s"},
	} {
		t.Run(setting.name+setting.value, func(t *testing.T) {
			values := map[string]string{"DB_ENABLED": "true", "DATABASE_URL": "postgres://user:secret@localhost/test"}
			values[setting.name] = setting.value
			_, err := LoadDatabase(func(name string) string { return values[name] })
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("expected sanitized error, got %v", err)
			}
		})
	}
	configuration, err := LoadDatabase(func(name string) string {
		if name == "DB_ENABLED" {
			return "false"
		}
		return "invalid-secret"
	})
	if err != nil || configuration.Enabled || configuration.URL != "" {
		t.Fatal("disabled database parsed credentials or options")
	}
	configuration, err = LoadDatabase(func(name string) string {
		return map[string]string{"DB_ENABLED": "true", "DATABASE_URL": "postgres://user@localhost/test", "DB_MAX_IDLE_CONNS": "0"}[name]
	})
	if err != nil || !configuration.Enabled || configuration.MaxIdleConns != 0 {
		t.Fatal("valid configuration rejected")
	}
}
