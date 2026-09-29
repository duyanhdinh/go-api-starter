package middleware

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

func TestRateLimitExemptionsBelongToCaller(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, customExemption := range []bool{false, true} {
		configuration := testRateLimitConfig()
		configuration.Burst = 1
		configuration.RequestsPerSecond = 0.000001
		var exempt func(*http.Request) bool
		if customExemption {
			exempt = func(request *http.Request) bool { return request.URL.Path == "/custom-ready" }
		}
		handler := RateLimit(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusAccepted)
		}), configuration, logger, exempt)
		for index, path := range []string{"/health", "/health", "/custom-ready"} {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			expected := http.StatusTooManyRequests
			if index == 0 || customExemption && path == "/custom-ready" {
				expected = http.StatusAccepted
			}
			if recorder.Code != expected {
				t.Fatalf("custom=%v path=%s: status=%d want=%d", customExemption, path, recorder.Code, expected)
			}
		}
	}
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

func TestRateLimitHTTPSmokeRecoversAndKeepsHealthAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configuration := testRateLimitConfig()
	configuration.Burst = 1
	limiter := newRateLimiter(configuration)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return clock }
	server := httptest.NewServer(withRateLimiter(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}), limiter, logger, func(request *http.Request) bool { return request.URL.Path == "/health" }))
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
