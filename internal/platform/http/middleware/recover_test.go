package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverPanics(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		t.Run(method, func(t *testing.T) {
			var logs bytes.Buffer
			applicationLogger := slog.New(slog.NewTextHandler(&logs, nil))
			handler := Recover(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
			handler := Recover(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
