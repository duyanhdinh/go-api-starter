package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"server/internal/platform/config"
)

func testCORSConfig() config.CORSConfig {
	return config.CORSConfig{Enabled: true, AllowedOrigins: []string{"http://localhost:3000"}, AllowedMethods: []string{"GET", "HEAD"}, AllowedHeaders: []string{"Authorization", "Content-Type"}, ExposedHeaders: []string{"X-Request-ID"}, MaxAge: 10 * time.Minute}
}

func TestCORSSmoke(t *testing.T) {
	for _, enabled := range []bool{false, true, false} {
		for _, credentials := range []bool{false, true} {
			configuration := testCORSConfig()
			configuration.Enabled, configuration.AllowCredentials = enabled, credentials
			server := httptest.NewServer(withCORS(newHandler(slog.New(slog.NewTextHandler(io.Discard, nil))), configuration))
			for _, testCase := range []struct {
				method, path, origin, requestedMethod, headers string
				status                                         int
				allowed                                        bool
			}{
				{"GET", "/health", "", "", "", 200, false},
				{"GET", "/health", "http://localhost:3000", "", "", 200, true},
				{"HEAD", "/health", "http://localhost:3000", "", "", 200, true},
				{"GET", "/missing", "http://localhost:3000", "", "", 404, true},
				{"GET", "/health", "https://denied.example", "", "", 200, false},
				{"GET", "/health", "null", "", "", 200, false},
				{"OPTIONS", "/health", "", "GET", "", 405, false},
				{"OPTIONS", "/health", "http://localhost:3000", "", "", 405, true},
				{"OPTIONS", "/health", "http://localhost:3000", "GET", "aUtHoRiZaTiOn, content-TYPE", 204, true},
				{"OPTIONS", "/health", "http://localhost:3000", "HEAD", "", 204, true},
				{"OPTIONS", "/health", "https://denied.example", "GET", "", 403, false},
				{"OPTIONS", "/health", "http://localhost:3000", "POST", "", 403, false},
				{"OPTIONS", "/health", "http://localhost:3000", "get", "", 403, false},
				{"OPTIONS", "/health", "http://localhost:3000", "GET", "X-Denied", 403, false},
			} {
				request, err := http.NewRequest(testCase.method, server.URL+testCase.path, nil)
				if err != nil {
					t.Fatal(err)
				}
				if testCase.origin != "" {
					request.Header.Set("Origin", testCase.origin)
				}
				if testCase.requestedMethod != "" {
					request.Header.Set("Access-Control-Request-Method", testCase.requestedMethod)
				}
				if testCase.headers != "" {
					request.Header.Set("Access-Control-Request-Headers", testCase.headers)
				}
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				status := testCase.status
				if !enabled && testCase.method == "OPTIONS" {
					status = 405
				}
				if response.StatusCode != status {
					t.Fatalf("%+v enabled=%v: status %d", testCase, enabled, response.StatusCode)
				}
				allowedOrigin := ""
				if enabled && testCase.allowed {
					allowedOrigin = testCase.origin
				}
				if response.Header.Get("Access-Control-Allow-Origin") != allowedOrigin {
					t.Fatalf("unexpected CORS headers: %v", response.Header)
				}
				wantCredentials := ""
				if allowedOrigin != "" && credentials {
					wantCredentials = "true"
				}
				if response.Header.Get("Access-Control-Allow-Credentials") != wantCredentials {
					t.Fatal("credentials mismatch")
				}
				if status == 204 || testCase.method == "HEAD" {
					if len(body) != 0 {
						t.Fatal("unexpected body")
					}
				}
				if testCase.method == "GET" && testCase.path == "/health" && string(body) != `{"status":"ok"}` {
					t.Fatal("health contract changed")
				}
				if enabled {
					vary := strings.Join(response.Header.Values("Vary"), ",")
					if !strings.Contains(vary, "Origin") {
						t.Fatal("missing origin Vary")
					}
					if status == 204 || status == 403 {
						if !strings.Contains(vary, "Access-Control-Request-Method") || !strings.Contains(vary, "Access-Control-Request-Headers") {
							t.Fatal("missing preflight Vary")
						}
					}
					if status == 204 && (response.Header.Get("Access-Control-Max-Age") != "600" || response.Header.Get("Access-Control-Allow-Methods") != "GET, HEAD" || response.Header.Get("Access-Control-Allow-Headers") != "Authorization, Content-Type") {
						t.Fatal("invalid preflight headers")
					}
					if allowedOrigin != "" && testCase.method != "OPTIONS" && response.Header.Get("Access-Control-Expose-Headers") != "X-Request-ID" {
						t.Fatal("missing exposed headers")
					}
				} else if len(response.Header.Values("Vary")) != 0 {
					t.Fatal("disabled CORS changed response")
				}
			}
			server.Close()
		}
	}
}

func TestCORSRecoveryAndVary(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		for _, action := range []string{"normal", "panic", "committed", "flush"} {
			t.Run(method+action, func(t *testing.T) {
				handler := withCORS(recoverPanics(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					writer.Header().Set("Vary", "Accept-Encoding, origin")
					if action == "panic" {
						writer.Header().Set("Set-Cookie", "secret")
						panic("secret")
					}
					if action == "flush" {
						_ = http.NewResponseController(writer).Flush()
						panic("secret")
					}
					writer.WriteHeader(200)
					if action == "committed" {
						panic("secret")
					}
				}), slog.New(slog.NewTextHandler(io.Discard, nil))), testCORSConfig())
				response := httptest.NewRecorder()
				request := httptest.NewRequest(method, "/", nil)
				request.Header.Set("Origin", "http://localhost:3000")
				func() {
					defer func() {
						recovered := recover()
						if action == "committed" || action == "flush" {
							if recovered != http.ErrAbortHandler {
								t.Fatal("expected abort")
							}
						} else if recovered != nil {
							t.Fatalf("unexpected panic: %v", recovered)
						}
					}()
					handler.ServeHTTP(response, request)
				}()
				if response.Result().Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
					t.Fatal("missing CORS header")
				}
				if action == "panic" {
					if response.Code != 500 || response.Header().Get("Set-Cookie") != "" {
						t.Fatal("recovery changed")
					}
					if method == "HEAD" && response.Body.Len() != 0 {
						t.Fatal("HEAD body")
					}
				} else {
					if response.Body.Len() != 0 || strings.Join(response.Header().Values("Vary"), ",") != "Accept-Encoding, origin" {
						t.Fatal("body or Vary changed")
					}
				}
			})
		}
	}
}

func TestCORSPreflightStopsBeforeNext(t *testing.T) {
	for _, denied := range []bool{false, true} {
		handler := withCORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("preflight reached downstream handler")
		}), testCORSConfig())
		request := httptest.NewRequest("OPTIONS", "/health", nil)
		request.Header.Set("Origin", "http://localhost:3000")
		request.Header.Set("Access-Control-Request-Method", "GET")
		request.Header.Add("Access-Control-Request-Headers", "Authorization")
		if denied {
			request.Header.Add("Access-Control-Request-Headers", "X-Denied")
		}
		response := httptest.NewRecorder()
		response.Header().Add("Vary", "Accept-Encoding")
		handler.ServeHTTP(response, request)
		wantStatus := 204
		if denied {
			wantStatus = 403
		}
		if response.Code != wantStatus || response.Body.Len() != 0 || !strings.Contains(strings.Join(response.Header().Values("Vary"), ","), "Accept-Encoding") {
			t.Fatalf("unexpected preflight response: %v", response)
		}
		if denied {
			for name := range response.Header() {
				if strings.HasPrefix(name, "Access-Control-") {
					t.Fatalf("denied preflight grants CORS: %s", name)
				}
			}
		}
	}
}
