package response

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

func TestResponseFormatsAndHEAD(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, testCase := range []struct {
		name, body string
		write      func(http.ResponseWriter, *http.Request)
	}{
		{"json", "{\"name\":\"An\"}\n", func(writer http.ResponseWriter, request *http.Request) {
			JSON(writer, request, logger, 200, map[string]string{"name": "An"})
		}},
		{"raw", "{\"status\":\"ok\"}", func(writer http.ResponseWriter, request *http.Request) {
			RawJSON(writer, request, logger, 200, `{"status":"ok"}`)
		}},
		{"error", "{\"error\":{\"code\":\"invalid_input\",\"message\":\"Invalid input\"}}", func(writer http.ResponseWriter, request *http.Request) {
			Error(writer, request, logger, 200, "invalid_input", "Invalid input")
		}},
		{"error with newline", "{\"error\":{\"code\":\"invalid_input\",\"message\":\"Invalid input\"}}\n", func(writer http.ResponseWriter, request *http.Request) {
			JSON(writer, request, logger, 200, ErrorBody("invalid_input", "Invalid input"))
		}},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(testCase.name+method, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				testCase.write(recorder, httptest.NewRequest(method, "/", nil))
				expected := testCase.body
				if method == http.MethodHead {
					expected = ""
				}
				if recorder.Code != 200 || recorder.Body.String() != expected {
					t.Fatalf("response = %d %q, want %q", recorder.Code, recorder.Body.String(), expected)
				}
				if recorder.Header().Get("Content-Type") != "application/json" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("headers = %v", recorder.Header())
				}
			})
		}
	}
}

type failingWriter struct{ *httptest.ResponseRecorder }

func (writer *failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("private transport details")
}

func TestWriteFailuresDoNotExposeDetails(t *testing.T) {
	for _, raw := range []bool{false, true} {
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		writer := &failingWriter{httptest.NewRecorder()}
		incoming := httptest.NewRequest(http.MethodGet, "/?token=private", nil)
		if raw {
			RawJSON(writer, incoming, logger, 201, "{}")
		} else {
			JSON(writer, incoming, logger, 201, map[string]string{})
		}
		if writer.Code != 201 || !strings.Contains(logs.String(), "HTTP response write failed") || strings.Contains(logs.String(), "private") {
			t.Fatalf("status = %d, logs = %s", writer.Code, logs.String())
		}
	}
}
