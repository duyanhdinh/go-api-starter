package middleware

import (
	"log/slog"
	"net/http"

	"server/internal/platform/http/response"
)

func Recover(next http.Handler, applicationLogger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		tracked := &responseWriter{ResponseWriter: writer}
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(http.ErrAbortHandler)
				}
				applicationLogger.ErrorContext(request.Context(), "HTTP handler panicked")
				if tracked.committed {
					panic(http.ErrAbortHandler)
				}
				clear(writer.Header())
				response.Error(tracked, request, applicationLogger, http.StatusInternalServerError, "internal_error", "Internal server error")
			}
		}()
		next.ServeHTTP(tracked, request)
	})
}

type responseWriter struct {
	http.ResponseWriter
	committed bool
}

func (writer *responseWriter) WriteHeader(status int) {
	if status >= 200 || status == http.StatusSwitchingProtocols {
		writer.committed = true
	}
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *responseWriter) Write(body []byte) (int, error) {
	writer.committed = true
	return writer.ResponseWriter.Write(body)
}

func (writer *responseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func (writer *responseWriter) FlushError() error {
	writer.committed = true
	return http.NewResponseController(writer.ResponseWriter).Flush()
}
