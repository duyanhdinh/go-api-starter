package config

import (
	"strings"
	"testing"
	"time"
)

func TestCORSConfiguration(t *testing.T) {
	for _, testCase := range []struct{ name, value string }{
		{"CORS_ENABLED", "invalid"}, {"CORS_ALLOWED_ORIGINS", ""},
		{"CORS_ALLOWED_ORIGINS", "*"}, {"CORS_ALLOWED_ORIGINS", "null"},
		{"CORS_ALLOWED_ORIGINS", "https://*.example.com"},
		{"CORS_ALLOWED_ORIGINS", "https://example.com/"},
		{"CORS_ALLOWED_ORIGINS", "https://example.com?"},
		{"CORS_ALLOWED_ORIGINS", "https://example.com#"},
		{"CORS_ALLOWED_ORIGINS", "https://user@example.com"},
		{"CORS_ALLOWED_ORIGINS", "https://example.com:65536"},
		{"CORS_ALLOWED_ORIGINS", "https://example.com:"},
		{"CORS_ALLOWED_ORIGINS", "https://[invalid]"},
		{"CORS_ALLOWED_ORIGINS", "https://[127.0.0.1]"},
		{"CORS_ALLOWED_ORIGINS", "https://example.com,"},
		{"CORS_ALLOWED_ORIGINS", "ftp://example.com"},
		{"CORS_ALLOWED_ORIGINS", "https://(example).com"},
		{"CORS_ALLOWED_METHODS", "GET, bad method"},
		{"CORS_ALLOWED_METHODS", "*"},
		{"CORS_ALLOWED_HEADERS", "X-Test\r\nInjected: true"},
		{"CORS_ALLOWED_HEADERS", "*"}, {"CORS_EXPOSED_HEADERS", "bad:header"},
		{"CORS_ALLOW_CREDENTIALS", "invalid"},
		{"CORS_MAX_AGE", "-1s"}, {"CORS_MAX_AGE", "1ms"}, {"CORS_MAX_AGE", "bad"},
	} {
		t.Run(testCase.name+"="+testCase.value, func(t *testing.T) {
			_, err := load(func(name string) string {
				if name == testCase.name {
					return testCase.value
				}
				if name == "CORS_ENABLED" {
					return "true"
				}
				if name == "CORS_ALLOWED_ORIGINS" {
					return "http://localhost:3000"
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), testCase.name) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	for _, credentials := range []string{"", "true"} {
		configuration, err := load(func(name string) string {
			return map[string]string{"CORS_ENABLED": "true", "CORS_ALLOWED_ORIGINS": "http://localhost:3000, https://[::1]:8443", "CORS_ALLOWED_HEADERS": "Authorization, Content-Type", "CORS_EXPOSED_HEADERS": "X-Request-ID", "CORS_ALLOW_CREDENTIALS": credentials, "CORS_MAX_AGE": "0s"}[name]
		})
		if err != nil {
			t.Fatal(err)
		}
		if !configuration.CORS.Enabled || configuration.CORS.AllowCredentials != (credentials == "true") || configuration.CORS.MaxAge != 0*time.Second || len(configuration.CORS.AllowedOrigins) != 2 || len(configuration.CORS.AllowedHeaders) != 2 || len(configuration.CORS.ExposedHeaders) != 1 {
			t.Fatalf("unexpected config: %+v", configuration.CORS)
		}
	}
	if _, err := load(func(name string) string {
		if name == "CORS_ALLOWED_ORIGINS" {
			return "invalid"
		}
		return ""
	}); err != nil {
		t.Fatalf("disabled CORS: %v", err)
	}
}
