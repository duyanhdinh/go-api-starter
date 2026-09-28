package config

import (
	"strings"
	"testing"
	"time"
)

func TestRateLimitConfiguration(t *testing.T) {
	configuration, err := load(func(name string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if configuration.RateLimit != (RateLimitConfig{}) {
		t.Fatalf("rate limit should default off: %+v", configuration.RateLimit)
	}

	configuration, err = load(func(name string) string {
		return map[string]string{
			"RATE_LIMIT_ENABLED": "true", "RATE_LIMIT_REQUESTS_PER_SECOND": "2.5",
			"RATE_LIMIT_BURST": "7", "RATE_LIMIT_MAX_CLIENTS": "123",
			"RATE_LIMIT_ENTRY_TTL": "3m",
		}[name]
	})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.RateLimit != (RateLimitConfig{true, 2.5, 7, 123, 3 * time.Minute}) {
		t.Fatalf("unexpected rate limit config: %+v", configuration.RateLimit)
	}
}

func TestInvalidRateLimitConfiguration(t *testing.T) {
	for _, testCase := range []struct{ name, value string }{
		{"RATE_LIMIT_ENABLED", "invalid"},
		{"RATE_LIMIT_REQUESTS_PER_SECOND", "0"}, {"RATE_LIMIT_REQUESTS_PER_SECOND", "NaN"},
		{"RATE_LIMIT_REQUESTS_PER_SECOND", "+Inf"}, {"RATE_LIMIT_REQUESTS_PER_SECOND", "bad"},
		{"RATE_LIMIT_BURST", "0"}, {"RATE_LIMIT_BURST", "-1"}, {"RATE_LIMIT_BURST", "bad"},
		{"RATE_LIMIT_MAX_CLIENTS", "0"}, {"RATE_LIMIT_MAX_CLIENTS", "-1"},
		{"RATE_LIMIT_MAX_CLIENTS", "bad"},
		{"RATE_LIMIT_ENTRY_TTL", "0s"}, {"RATE_LIMIT_ENTRY_TTL", "-1s"}, {"RATE_LIMIT_ENTRY_TTL", "bad"},
	} {
		t.Run(testCase.name+"="+testCase.value, func(t *testing.T) {
			_, err := load(func(name string) string {
				if name == testCase.name {
					return testCase.value
				}
				if name == "RATE_LIMIT_ENABLED" {
					return "true"
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), testCase.name) {
				t.Fatalf("expected error identifying %s, got %v", testCase.name, err)
			}
			if strings.Contains(err.Error(), testCase.value) {
				t.Fatalf("error exposes invalid value: %v", err)
			}
		})
	}
	if _, err := load(func(name string) string {
		if name == "RATE_LIMIT_REQUESTS_PER_SECOND" {
			return "bad"
		}
		return ""
	}); err != nil {
		t.Fatalf("disabled rate limit should ignore related settings: %v", err)
	}
}
