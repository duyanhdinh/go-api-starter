package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"server/internal/platform/config"
)

func testRateLimitConfig() config.RateLimitConfig {
	return config.RateLimitConfig{Enabled: true, RequestsPerSecond: 2, Burst: 2, MaxClients: 10, EntryTTL: time.Minute}
}

func TestRateLimiterBurstRefillAndIndependentClients(t *testing.T) {
	limiter := newRateLimiter(testRateLimitConfig())
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return clock }
	for index := 0; index < 2; index++ {
		if allowed, _ := limiter.allow("192.0.2.1"); !allowed {
			t.Fatal("request within burst was rejected")
		}
	}
	if allowed, retryAfter := limiter.allow("192.0.2.1"); allowed || retryAfter != 1 {
		t.Fatalf("exhausted bucket = allowed %v, retry %d", allowed, retryAfter)
	}
	if allowed, _ := limiter.allow("192.0.2.2"); !allowed {
		t.Fatal("independent IP was rejected")
	}
	clock = clock.Add(500 * time.Millisecond)
	if allowed, _ := limiter.allow("192.0.2.1"); !allowed {
		t.Fatal("token did not refill")
	}
}

func TestClientIPCanonicalization(t *testing.T) {
	for address, expected := range map[string]string{
		"192.0.2.1:4321": "192.0.2.1", "192.0.2.1:9999": "192.0.2.1",
		"[2001:0db8::1]:4321": "2001:db8::1", "2001:db8::1": "2001:db8::1",
	} {
		if actual := clientIP(address); actual != expected {
			t.Errorf("clientIP(%q) = %q, want %q", address, actual, expected)
		}
	}
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

func TestRateLimiterCapacityAndTTL(t *testing.T) {
	configuration := testRateLimitConfig()
	configuration.MaxClients = 1
	limiter := newRateLimiter(configuration)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return clock }
	if allowed, _ := limiter.allow("client-a"); !allowed {
		t.Fatal("first client rejected")
	}
	if allowed, retryAfter := limiter.allow("client-b"); allowed || retryAfter != int64(configuration.EntryTTL.Seconds()) {
		t.Fatalf("new client was not rejected with capacity retry time: allowed=%v retry=%d", allowed, retryAfter)
	}
	if allowed, _ := limiter.allow("client-a"); !allowed {
		t.Fatal("existing client rejected at capacity")
	}
	clock = clock.Add(configuration.EntryTTL)
	if allowed, _ := limiter.allow("client-b"); !allowed {
		t.Fatal("expired entry was not reclaimed")
	}
}

func TestRateLimiterConcurrentAccess(t *testing.T) {
	limiter := newRateLimiter(testRateLimitConfig())
	var group sync.WaitGroup
	for index := 0; index < 100; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			limiter.allow("192.0.2.1")
		}()
	}
	group.Wait()
	if len(limiter.buckets) != 1 {
		t.Fatalf("tracked clients = %d, want 1", len(limiter.buckets))
	}
}

func TestRateLimitCORSAndPreflight(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configuration := testCORSConfig()
	configuration.AllowedMethods = []string{"POST"}
	inner := withRateLimit(newHandler(logger), testRateLimitConfig(), logger)
	server := withCORS(inner, configuration)

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

func TestRateLimitHTTPSmokeRecoversAndKeepsHealthAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configuration := testRateLimitConfig()
	configuration.Burst = 1
	limiter := newRateLimiter(configuration)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return clock }
	server := httptest.NewServer(withRateLimiter(newHandler(logger), limiter, logger))
	defer server.Close()
	client := server.Client()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/missing", nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	if first.StatusCode != http.StatusNotFound {
		t.Fatalf("initial request returned %d", first.StatusCode)
	}

	request, err = http.NewRequest(http.MethodPost, server.URL+"/missing", nil)
	if err != nil {
		t.Fatal(err)
	}
	limited, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	limited.Body.Close()
	if limited.StatusCode != http.StatusTooManyRequests || limited.Header.Get("Retry-After") != "1" {
		t.Fatalf("limited response = %d, Retry-After %q", limited.StatusCode, limited.Header.Get("Retry-After"))
	}

	clock = clock.Add(time.Second)
	request, err = http.NewRequest(http.MethodPost, server.URL+"/missing", nil)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	recovered.Body.Close()
	if recovered.StatusCode != http.StatusNotFound {
		t.Fatalf("request after refill returned %d", recovered.StatusCode)
	}

	health, err := client.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health returned %d after rate limiting", health.StatusCode)
	}
}
