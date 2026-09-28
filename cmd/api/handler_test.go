package main

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutes(t *testing.T) {
	handler := newHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, testCase := range []struct {
		method string
		path   string
		status int
		body   string
		allow  string
	}{
		{"GET", "/health", 200, `{"status":"ok"}`, ""},
		{"HEAD", "/health", 200, "", ""},
		{"POST", "/health", 405, `{"error":{"code":"method_not_allowed","message":"Method not allowed"}}`, "GET, HEAD"},
		{"OPTIONS", "/health", 405, `{"error":{"code":"method_not_allowed","message":"Method not allowed"}}`, "GET, HEAD"},
		{"GET", "/missing", 404, `{"error":{"code":"not_found","message":"Resource not found"}}`, ""},
		{"POST", "/missing", 404, `{"error":{"code":"not_found","message":"Resource not found"}}`, ""},
		{"GET", "/health/", 404, `{"error":{"code":"not_found","message":"Resource not found"}}`, ""},
		{"HEAD", "/missing", 404, "", ""},
	} {
		t.Run(testCase.method+testCase.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(testCase.method, testCase.path, nil))
			if response.Code != testCase.status || response.Body.String() != testCase.body {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Allow") != testCase.allow {
				t.Fatalf("unexpected headers: %v", response.Header())
			}
		})
	}
}

func TestRecoverPanics(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		t.Run(method, func(t *testing.T) {
			var logs bytes.Buffer
			applicationLogger := slog.New(slog.NewTextHandler(&logs, nil))
			handler := recoverPanics(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Length", "999")
				writer.Header().Set("Set-Cookie", "secret")
				panic("secret panic data")
			}), applicationLogger)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(method, "/?token=secret", nil))
			if response.Code != 500 || response.Header().Get("Content-Length") != "" || response.Header().Get("Set-Cookie") != "" {
				t.Fatalf("unexpected response: %v", response)
			}
			wantBody := `{"error":{"code":"internal_error","message":"Internal server error"}}`
			if method == "HEAD" {
				wantBody = ""
			}
			if response.Body.String() != wantBody || !strings.Contains(logs.String(), "HTTP handler panicked") || strings.Contains(logs.String(), "secret") {
				t.Fatalf("unexpected body or logs: %s %s", response.Body.String(), logs.String())
			}
		})
	}
}

func TestPanicAfterResponseAborts(t *testing.T) {
	for _, action := range []string{"write", "header", "flush", "abort"} {
		t.Run(action, func(t *testing.T) {
			var logs bytes.Buffer
			handler := recoverPanics(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch action {
				case "write":
					_, _ = writer.Write([]byte("partial"))
				case "header":
					writer.WriteHeader(http.StatusAccepted)
				case "flush":
					if err := http.NewResponseController(writer).Flush(); err != nil {
						t.Fatal(err)
					}
				case "abort":
					panic(http.ErrAbortHandler)
				}
				panic("secret")
			}), slog.New(slog.NewTextHandler(&logs, nil)))
			response := httptest.NewRecorder()
			defer func() {
				if recovered := recover(); recovered != http.ErrAbortHandler {
					t.Fatalf("expected ErrAbortHandler, got %v", recovered)
				}
				if strings.Contains(response.Body.String(), "internal_error") {
					t.Fatal("error appended to committed response")
				}
				if action == "abort" && logs.Len() != 0 {
					t.Fatal("intentional abort logged as panic")
				}
			}()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
		})
	}
}

type failingWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (writer *failingWriter) Write(body []byte) (int, error) {
	writer.writes++
	return 0, errors.New("secret transport error")
}

func TestWriteFailure(t *testing.T) {
	var logs bytes.Buffer
	writer := &failingWriter{ResponseRecorder: httptest.NewRecorder()}
	newHandler(slog.New(slog.NewTextHandler(&logs, nil))).ServeHTTP(writer, httptest.NewRequest("GET", "/health", nil))
	if writer.writes != 1 || !strings.Contains(logs.String(), "HTTP response write failed") || strings.Contains(logs.String(), "secret") {
		t.Fatalf("writes = %d, logs = %s", writer.writes, logs.String())
	}
}

func TestHealthSmoke(t *testing.T) {
	server := httptest.NewServer(newHandler(slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || string(body) != `{"status":"ok"}` {
		t.Fatalf("health response = %d %s, error = %v", response.StatusCode, body, err)
	}
}
