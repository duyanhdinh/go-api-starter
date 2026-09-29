package request

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadJSONUsesCallerParameters(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	type payload struct {
		Name string `json:"name"`
	}
	for _, testCase := range []struct {
		name, contentType, body, expectedBody string
		maxBytes                              int64
		status                                int
	}{
		{"valid", "application/json", `{"name":"An"}`, "", 13, 200},
		{"invalid", "application/json", `{"name":"An","extra":1}`, `{"error":{"code":"invalid_input","message":"Invalid product input"}}` + "\n", 64, 400},
		{"multiple", "application/json", `{}{}`, `{"error":{"code":"invalid_input","message":"Invalid product input"}}` + "\n", 64, 400},
		{"oversized", "application/json", `{"name":"An"}`, `{"error":{"code":"body_too_large","message":"Request body exceeds 12 bytes"}}` + "\n", 12, 413},
		{"media", "text/plain", `{}`, `{"error":{"code":"unsupported_media_type","message":"Content-Type must be application/json"}}` + "\n", 64, 415},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			incoming := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(testCase.body))
			incoming.Header.Set("Content-Type", testCase.contentType)
			recorder := httptest.NewRecorder()
			input, ok := ReadJSON[payload](recorder, incoming, logger, testCase.maxBytes, "Invalid product input")
			if ok != (testCase.status == 200) || recorder.Code != testCase.status || recorder.Body.String() != testCase.expectedBody {
				t.Fatalf("ok=%v response=%d %q", ok, recorder.Code, recorder.Body.String())
			}
			if ok {
				if input.Name != "An" || len(recorder.Header()) != 0 {
					t.Fatalf("input=%+v headers=%v", input, recorder.Header())
				}
			} else {
				if input != (payload{}) {
					t.Fatalf("failed decode returned partial input: %+v", input)
				}
				if recorder.Header().Get("Content-Type") != "application/json" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("headers=%v", recorder.Header())
				}
			}
		})
	}
}

func TestDecodeJSON(t *testing.T) {
	for _, testCase := range []struct {
		name, contentType, body, failure string
		maxBytes                         int64
	}{
		{"valid", "application/json", `{"name":"An"}`, "", 64},
		{"charset", "application/json; charset=utf-8", `{"name":"An"}`, "", 64},
		{"exact limit", "application/json", `{"name":"An"}`, "", 13},
		{"whitespace", "application/json", `{"name":"An"}` + " \n", "", 64},
		{"missing content type", "", "{}", "media", 64},
		{"wrong content type", "text/plain", "{}", "media", 64},
		{"malformed content type", "application/json; charset", "{}", "media", 64},
		{"empty", "application/json", "", "invalid", 64},
		{"malformed", "application/json", "{", "invalid", 64},
		{"unknown field", "application/json", `{"other":1}`, "invalid", 64},
		{"wrong type", "application/json", `{"name":1}`, "invalid", 64},
		{"multiple values", "application/json", "{}{}", "multiple", 64},
		{"trailing garbage", "application/json", "{}x", "invalid", 64},
		{"body too large", "application/json", `{"name":"An"}`, "large", 12},
		{"trailing whitespace too large", "application/json", "{}" + strings.Repeat(" ", 64), "large", 64},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			incoming := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(testCase.body))
			incoming.Header.Set("Content-Type", testCase.contentType)
			var input struct {
				Name string `json:"name"`
			}
			err := DecodeJSON(httptest.NewRecorder(), incoming, &input, testCase.maxBytes)
			var tooLarge *http.MaxBytesError
			switch testCase.failure {
			case "":
				if err != nil {
					t.Fatal(err)
				}
			case "media":
				if !errors.Is(err, ErrUnsupportedMediaType) {
					t.Fatalf("error = %v", err)
				}
			case "multiple":
				if !errors.Is(err, ErrMultipleJSONValues) {
					t.Fatalf("error = %v", err)
				}
			case "large":
				if !errors.As(err, &tooLarge) {
					t.Fatalf("error = %v", err)
				}
			default:
				if err == nil {
					t.Fatal("invalid JSON accepted")
				}
			}
		})
	}
}
