package config

import (
	"strings"
	"testing"
	"time"
)

func defaultHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		Timeout: 30 * time.Second, ConnectTimeout: 5 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout: 90 * time.Second, MaxIdleConns: 100, MaxIdleConnsPerHost: 10, MaxConnsPerHost: 50,
	}
}

func TestInvalidHTTPClientConfiguration(t *testing.T) {
	settings := map[string][]string{
		"HTTP_CLIENT_TIMEOUT":                 {"0s", "-1s", "bad", "999999999999999999h"},
		"HTTP_CLIENT_CONNECT_TIMEOUT":         {"0s", "-1s", "bad"},
		"HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT":   {"0s", "-1s", "bad"},
		"HTTP_CLIENT_RESPONSE_HEADER_TIMEOUT": {"0s", "-1s", "bad"},
		"HTTP_CLIENT_IDLE_CONN_TIMEOUT":       {"0s", "-1s", "bad"},
		"HTTP_CLIENT_MAX_IDLE_CONNS":          {"0", "-1", "bad", "1.5", "999999999999999999999", "5"},
		"HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST": {"0", "-1", "bad", "101", "51"},
		"HTTP_CLIENT_MAX_CONNS_PER_HOST":      {"0", "-1", "bad", "5"},
	}
	for name, values := range settings {
		for _, value := range values {
			t.Run(name+"="+value, func(t *testing.T) {
				_, err := load(func(key string) string {
					if key == name {
						return value
					}
					return ""
				})
				if err == nil || !strings.Contains(err.Error(), name) {
					t.Fatalf("expected error identifying %s, got %v", name, err)
				}
				if strings.Contains(err.Error(), value) {
					t.Fatalf("error exposes invalid configuration value: %v", err)
				}
			})
		}
	}
}
