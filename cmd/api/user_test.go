package main

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"server/internal/platform/config"
	"server/internal/platform/database"
)

func TestUserRouteAvailability(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, testCase := range []struct {
		environment string
		pool        *database.Pool
		status      int
	}{
		{"dev", &database.Pool{}, 400}, {"test", &database.Pool{}, 400},
		{"prod", &database.Pool{}, 400}, {"dev", nil, 404}, {"", &database.Pool{}, 400},
		{"test", nil, 404}, {"prod", nil, 404},
	} {
		handler := newApplicationHandler(config.Config{Environment: testCase.environment, UserRequestTimeout: 5 * time.Second}, logger, testCase.pool)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/users/invalid", nil))
		if response.Code != testCase.status {
			t.Errorf("env=%s pool=%v: got %d want %d", testCase.environment, testCase.pool != nil, response.Code, testCase.status)
		}
		health := httptest.NewRecorder()
		handler.ServeHTTP(health, httptest.NewRequest("GET", "/health", nil))
		if health.Code != 200 {
			t.Fatal("health regressed")
		}
		for _, path := range []string{"/users", "/api/users", "/api/v1/unknown"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if response.Code != 404 {
				t.Errorf("unexpected route at %s: %d", path, response.Code)
			}
		}
	}
}
