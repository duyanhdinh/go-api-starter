package config

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProfiles(t *testing.T) {
	for _, environment := range []string{"", "dev", "test", "prod"} {
		t.Run("env="+environment, func(t *testing.T) {
			configuration, err := load(func(name string) string {
				if name == "APP_ENV" {
					return environment
				}
				return ""
			})
			if err != nil {
				t.Fatal(err)
			}
			expectedEnvironment := environment
			if expectedEnvironment == "" {
				expectedEnvironment = "dev"
			}
			expectedAddress := ":8080"
			if environment == "test" {
				expectedAddress = "127.0.0.1:0"
			}
			expectedLevel := slog.LevelDebug
			if environment == "prod" {
				expectedLevel = slog.LevelInfo
			}
			databaseDefaults, _ := LoadDatabase(func(string) string { return "" })
			expected := Config{expectedEnvironment, expectedLevel, expectedAddress, 5 * time.Second, 15 * time.Second, 15 * time.Second, 60 * time.Second, 10 * time.Second, defaultHTTPClientConfig(), databaseDefaults, CORSConfig{}, RateLimitConfig{}}
			if !reflect.DeepEqual(configuration, expected) {
				t.Fatalf("got %+v, want %+v", configuration, expected)
			}
		})
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	values := map[string]string{
		"APP_ENV": "prod", "HTTP_ADDR": "[::1]:9000",
		"LOG_LEVEL":                "WARN",
		"HTTP_READ_HEADER_TIMEOUT": "1s", "HTTP_READ_TIMEOUT": "2s",
		"HTTP_WRITE_TIMEOUT": "3s", "HTTP_IDLE_TIMEOUT": "4s",
		"HTTP_SHUTDOWN_TIMEOUT": "500ms",
		"HTTP_CLIENT_TIMEOUT":   "12s", "HTTP_CLIENT_CONNECT_TIMEOUT": "750ms",
		"HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT": "2s", "HTTP_CLIENT_RESPONSE_HEADER_TIMEOUT": "3s",
		"HTTP_CLIENT_IDLE_CONN_TIMEOUT": "45s", "HTTP_CLIENT_MAX_IDLE_CONNS": "20",
		"HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST": "4", "HTTP_CLIENT_MAX_CONNS_PER_HOST": "8",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	expected := Config{"prod", slog.LevelWarn, "[::1]:9000", time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 500 * time.Millisecond, HTTPClientConfig{
		Timeout: 12 * time.Second, ConnectTimeout: 750 * time.Millisecond,
		TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 3 * time.Second,
		IdleConnTimeout: 45 * time.Second, MaxIdleConns: 20, MaxIdleConnsPerHost: 4, MaxConnsPerHost: 8,
	}, configuration.Database, CORSConfig{}, RateLimitConfig{}}
	if !reflect.DeepEqual(configuration, expected) {
		t.Fatalf("got %+v, want %+v", configuration, expected)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, testCase := range []struct{ name, value string }{
		{"APP_ENV", "staging"}, {"APP_ENV", "../prod"},
		{"LOG_LEVEL", "invalid-sensitive-value"},
		{"HTTP_ADDR", "localhost"}, {"HTTP_ADDR", ":65536"},
		{"HTTP_ADDR", ":-1"}, {"HTTP_ADDR", ":http"},
		{"HTTP_READ_HEADER_TIMEOUT", "bad"}, {"HTTP_READ_TIMEOUT", "0s"},
		{"HTTP_WRITE_TIMEOUT", "-1s"}, {"HTTP_IDLE_TIMEOUT", "60"},
		{"HTTP_SHUTDOWN_TIMEOUT", "999999999999999999h"},
	} {
		t.Run(testCase.name+"="+testCase.value, func(t *testing.T) {
			_, err := load(func(name string) string {
				if name == testCase.name {
					return testCase.value
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), testCase.name) {
				t.Fatalf("expected error identifying %s, got %v", testCase.name, err)
			}
			if strings.Contains(err.Error(), testCase.value) {
				t.Fatalf("error exposes invalid configuration value: %v", err)
			}
		})
	}
}
