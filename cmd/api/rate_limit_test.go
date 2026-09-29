package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"server/internal/platform/config"
	"server/internal/platform/http/middleware"
)

const rateLimitedBody = `{"error":{"code":"rate_limited","message":"Rate limit exceeded"}}`

func testRateLimitConfig() config.RateLimitConfig {
	return config.RateLimitConfig{Enabled: true, RequestsPerSecond: 2, Burst: 2, MaxClients: 10, EntryTTL: time.Minute}
}

func TestRateLimitMiddlewareResponsesAndExemptions(t *testing.T) {
	applicationLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configuration := testRateLimitConfig()
	limited := withRateLimit(newHandler(applicationLogger), configuration, applicationLogger)
	for index := 0; index < configuration.Burst; index++ {
		response := httptest.NewRecorder()
		limited.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/missing", nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("request within burst returned %d", response.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request := httptest.NewRequest(method, "/missing", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		response := httptest.NewRecorder()
		limited.ServeHTTP(response, request)
		if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "1" || response.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected limited response: %v", response)
		}
		wantBody := rateLimitedBody
		if method == http.MethodHead {
			wantBody = ""
		}
		if response.Body.String() != wantBody {
			t.Fatalf("body = %q, want %q", response.Body.String(), wantBody)
		}
		if method == http.MethodGet {
			request = httptest.NewRequest(method, "/health/child", nil)
			request.RemoteAddr = "192.0.2.1:1234"
			response = httptest.NewRecorder()
			limited.ServeHTTP(response, request)
			if response.Code != http.StatusTooManyRequests {
				t.Fatal("health subpath was exempt")
			}
			request = httptest.NewRequest(method, "/docs/", nil)
			request.RemoteAddr = "192.0.2.1:1234"
			response = httptest.NewRecorder()
			limited.ServeHTTP(response, request)
			if response.Code != http.StatusTooManyRequests {
				t.Fatal("docs route was unexpectedly exempt")
			}
			request = httptest.NewRequest(http.MethodPost, "/health", nil)
			request.RemoteAddr = "192.0.2.1:1234"
			response = httptest.NewRecorder()
			limited.ServeHTTP(response, request)
			if response.Code != http.StatusTooManyRequests {
				t.Fatal("non-GET health method was exempt")
			}
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, path := range []string{"/health", "/ready"} {
			request := httptest.NewRequest(method, path, nil)
			request.RemoteAddr = "192.0.2.1:1234"
			response := httptest.NewRecorder()
			limited.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("%s %s was not exempt: %d", method, path, response.Code)
			}
		}
	}
}

func TestRateLimitDisabledPreservesHandler(t *testing.T) {
	want := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusAccepted) })
	got := withRateLimit(want, config.RateLimitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	got.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if response.Code != http.StatusAccepted {
		t.Fatal("disabled limiter changed the handler response")
	}
}

func TestRateLimitUsesRemoteAddressAndIgnoresForwardedHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	limited := withRateLimit(newHandler(logger), testRateLimitConfig(), logger)
	for index := 0; index < 2; index++ {
		request := httptest.NewRequest(http.MethodPost, "/missing", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		request.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('1'+index)))
		request.Header.Set("X-Real-IP", "198.51.100."+string(rune('1'+index)))
		response := httptest.NewRecorder()
		limited.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("request %d returned %d", index, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/missing", nil)
	request.RemoteAddr = "192.0.2.1:5678"
	response := httptest.NewRecorder()
	limited.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatal("spoofed forwarding headers changed rate limit key")
	}
}

func TestRateLimitCORSAndPreflight(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configuration := testCORSConfig()
	configuration.AllowedMethods = []string{"POST"}
	inner := withRateLimit(newHandler(logger), testRateLimitConfig(), logger)
	server := middleware.CORS(inner, configuration)

	for index := 0; index < 2; index++ {
		request := httptest.NewRequest(http.MethodPost, "/missing", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		request.Header.Set("Origin", "http://localhost:3000")
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
			t.Fatal("allowed request lost its CORS response")
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/missing", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" || response.Header().Get("Retry-After") != "1" {
		t.Fatalf("limited response lost CORS or retry header: %v", response)
	}
	request = httptest.NewRequest(http.MethodOptions, "/missing", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", "POST")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight consumed limiter capacity: %d", response.Code)
	}
}
