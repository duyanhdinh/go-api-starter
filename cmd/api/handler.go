package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"

	"server/api"

	"github.com/go-chi/chi/v5"
)

func newHandler(applicationLogger *slog.Logger, readiness ...func(context.Context) error) http.Handler {
	return newHandlerWithDocs(applicationLogger, true, readiness...)
}

func newHandlerWithDocs(applicationLogger *slog.Logger, docsEnabled bool, readiness ...func(context.Context) error) http.Handler {
	return recoverPanics(newRouter(applicationLogger, docsEnabled, readiness...), applicationLogger)
}

func newRouter(applicationLogger *slog.Logger, docsEnabled bool, readiness ...func(context.Context) error) chi.Router {
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
	if docsEnabled {
		registerDocsRoutes(router, applicationLogger)
	}
	return router
}

func registerDocsRoutes(router chi.Router, applicationLogger *slog.Logger) {
	document, err := fs.ReadFile(api.Files, "openapi.yaml")
	if err != nil {
		panic("embedded OpenAPI spec unavailable")
	}
	router.Get("/openapi.yaml", func(writer http.ResponseWriter, request *http.Request) {
		writeStatic(writer, request, "application/yaml; charset=utf-8", document, applicationLogger)
	})
	router.Head("/openapi.yaml", func(writer http.ResponseWriter, request *http.Request) {
		writeStatic(writer, request, "application/yaml; charset=utf-8", document, applicationLogger)
	})
	docs := func(writer http.ResponseWriter, request *http.Request) {
		writeStatic(writer, request, "text/html; charset=utf-8", []byte(swaggerHTML), applicationLogger)
	}
	router.Get("/docs", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/docs/", http.StatusPermanentRedirect)
	})
	router.Head("/docs", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/docs/", http.StatusPermanentRedirect)
	})
	router.Get("/docs/", docs)
	router.Head("/docs/", docs)
	for _, asset := range []struct {
		name        string
		contentType string
	}{
		{"swagger-ui-bundle.js", "application/javascript; charset=utf-8"},
		{"swagger-ui-standalone-preset.js", "application/javascript; charset=utf-8"},
		{"swagger-ui.css", "text/css; charset=utf-8"},
	} {
		assetPath := "swagger-ui/" + asset.name
		body, err := fs.ReadFile(api.Files, assetPath)
		if err != nil {
			panic("embedded Swagger UI asset unavailable")
		}
		path := "/docs/" + asset.name
		router.Get(path, func(writer http.ResponseWriter, request *http.Request) {
			writeStatic(writer, request, asset.contentType, body, applicationLogger)
		})
		router.Head(path, func(writer http.ResponseWriter, request *http.Request) {
			writeStatic(writer, request, asset.contentType, body, applicationLogger)
		})
	}
}

const swaggerHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>API documentation</title>
<link rel="stylesheet" href="/docs/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="/docs/swagger-ui-bundle.js"></script>
<script src="/docs/swagger-ui-standalone-preset.js"></script>
<script>
window.ui = SwaggerUIBundle({
  url: "/openapi.yaml",
  dom_id: "#swagger-ui",
  deepLinking: true,
  presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
  plugins: [SwaggerUIBundle.plugins.DownloadUrl],
  layout: "StandaloneLayout"
});
</script>
</body>
</html>`

func writeStatic(writer http.ResponseWriter, request *http.Request, contentType string, body []byte, applicationLogger *slog.Logger) {
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
	writer.WriteHeader(http.StatusOK)
	if request.Method == http.MethodHead {
		return
	}
	if _, err := writer.Write(body); err != nil {
		applicationLogger.ErrorContext(request.Context(), "HTTP response write failed", "status", http.StatusOK)
	}
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
