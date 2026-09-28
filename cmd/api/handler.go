package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func newHandler(applicationLogger *slog.Logger, readiness ...func(context.Context) error) http.Handler {
	router := chi.NewRouter()
	ready := func(writer http.ResponseWriter, request *http.Request) {
		for _, check := range readiness {
			if err := check(request.Context()); err != nil {
				writeResponse(writer, request, applicationLogger, http.StatusServiceUnavailable, `{"status":"unavailable"}`)
				return
			}
		}
		writeResponse(writer, request, applicationLogger, http.StatusOK, `{"status":"ok"}`)
	}
	router.Get("/ready", ready)
	router.Head("/ready", ready)
	health := func(writer http.ResponseWriter, request *http.Request) {
		writeResponse(writer, request, applicationLogger, http.StatusOK, `{"status":"ok"}`)
	}
	router.Get("/health", health)
	router.Head("/health", health)
	router.MethodNotAllowed(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Allow", "GET, HEAD")
		writeError(writer, request, applicationLogger, http.StatusMethodNotAllowed)
	})
	router.NotFound(func(writer http.ResponseWriter, request *http.Request) {
		writeError(writer, request, applicationLogger, http.StatusNotFound)
	})
	return recoverPanics(router, applicationLogger)
}

func writeError(writer http.ResponseWriter, request *http.Request, applicationLogger *slog.Logger, status int) {
	var body string
	switch status {
	case http.StatusNotFound:
		body = `{"error":{"code":"not_found","message":"Resource not found"}}`
	case http.StatusMethodNotAllowed:
		body = `{"error":{"code":"method_not_allowed","message":"Method not allowed"}}`
	default:
		status = http.StatusInternalServerError
		body = `{"error":{"code":"internal_error","message":"Internal server error"}}`
	}
	writeResponse(writer, request, applicationLogger, status, body)
}

func writeResponse(writer http.ResponseWriter, request *http.Request, applicationLogger *slog.Logger, status int, body string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	if request.Method == http.MethodHead {
		return
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		applicationLogger.ErrorContext(request.Context(), "HTTP response write failed", "status", status)
	}
}

func recoverPanics(next http.Handler, applicationLogger *slog.Logger) http.Handler {
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
				writeError(tracked, request, applicationLogger, http.StatusInternalServerError)
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
