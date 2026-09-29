package main

import (
	"bytes"
	"context"
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
		{"GET", "/ready", 200, `{"status":"ok"}`, ""},
		{"HEAD", "/ready", 200, "", ""},
		{"POST", "/ready", 405, `{"error":{"code":"method_not_allowed","message":"Method not allowed"}}`, "GET, HEAD"},
		{"HEAD", "/health", 200, "", ""},
		{"POST", "/health", 405, `{"error":{"code":"method_not_allowed","message":"Method not allowed"}}`, "GET, HEAD"},
		{"OPTIONS", "/health", 405, `{"error":{"code":"method_not_allowed","message":"Method not allowed"}}`, "GET, HEAD"},
		{"GET", "/missing", 404, `{"error":{"code":"not_found","message":"Resource not found"}}`, ""},
		{"POST", "/missing", 404, `{"error":{"code":"not_found","message":"Resource not found"}}`, ""},
		{"GET", "/health/", 404, `{"error":{"code":"not_found","message":"Resource not found"}}`, ""},
		{"GET", "//health", 404, `{"error":{"code":"not_found","message":"Resource not found"}}`, ""},
		{"GET", "/%68ealth", 404, `{"error":{"code":"not_found","message":"Resource not found"}}`, ""},
		{"HEAD", "/%68ealth", 404, "", ""},
		{"HEAD", "/missing", 404, "", ""},
	} {
		t.Run(testCase.method+testCase.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(testCase.method, testCase.path, nil))
			if response.Code != testCase.status || response.Body.String() != testCase.body {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Allow") != testCase.allow {
				t.Fatalf("unexpected headers: %v", response.Header())
			}
		})
	}
}

func TestReadinessFailure(t *testing.T) {
	var logs bytes.Buffer
	checks := 0
	handler := newHandler(slog.New(slog.NewTextHandler(&logs, nil)), func(context.Context) error { checks++; return errors.New("secret driver error") })
	for _, path := range []string{"/health", "/ready"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if path == "/health" && (checks != 0 || response.Code != 200 || response.Body.String() != `{"status":"ok"}`) {
			t.Fatal("liveness depends on database")
		}
		if path == "/ready" && (checks != 1 || response.Code != 503 || response.Body.String() != `{"status":"unavailable"}`) {
			t.Fatal("readiness contract changed")
		}
		if strings.Contains(response.Body.String()+logs.String(), "secret") {
			t.Fatal("driver error exposed")
		}
	}
}

func TestReadinessFailureHEAD(t *testing.T) {
	checks := 0
	handler := newHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error {
		checks++
		return errors.New("secret driver error")
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/ready", nil))
	if checks != 1 || response.Code != http.StatusServiceUnavailable || response.Body.Len() != 0 {
		t.Fatalf("HEAD readiness = checks %d, status %d, body %q", checks, response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected HEAD readiness headers: %v", response.Header())
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
