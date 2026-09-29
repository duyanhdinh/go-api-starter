package response

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMethodNotAllowedUsesCallerPolicyAndWriter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, method := range []string{http.MethodPatch, http.MethodHead} {
		for _, path := range []string{"/search", "/items/1"} {
			for _, compact := range []bool{false, true} {
				calls := 0
				handler := MethodNotAllowed(func(request *http.Request) string {
					if request.URL.Path == "/search" {
						return "GET"
					}
					return "GET, DELETE"
				}, func(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
					calls++
					if compact {
						Error(writer, request, logger, status, code, message)
					} else {
						JSON(writer, request, logger, status, ErrorBody(code, message))
					}
				})
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
				expectedAllow := "GET"
				if path != "/search" {
					expectedAllow = "GET, DELETE"
				}
				expectedBody := `{"error":{"code":"method_not_allowed","message":"Method not allowed"}}`
				if !compact {
					expectedBody += "\n"
				}
				if method == http.MethodHead {
					expectedBody = ""
				}
				if calls != 1 || recorder.Code != 405 || recorder.Header().Get("Allow") != expectedAllow || recorder.Body.String() != expectedBody {
					t.Fatalf("%s %s compact=%v: calls=%d status=%d Allow=%q body=%q", method, path, compact, calls, recorder.Code, recorder.Header().Get("Allow"), recorder.Body.String())
				}
			}
		}
	}
}
