package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func JSON(writer http.ResponseWriter, request *http.Request, logger *slog.Logger, status int, value any) {
	writeHeaders(writer, status)
	if request.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		logger.ErrorContext(request.Context(), "HTTP response write failed", "status", status)
	}
}

func RawJSON(writer http.ResponseWriter, request *http.Request, logger *slog.Logger, status int, body string) {
	writeHeaders(writer, status)
	if request.Method == http.MethodHead {
		return
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		logger.ErrorContext(request.Context(), "HTTP response write failed", "status", status)
	}
}

func writeHeaders(writer http.ResponseWriter, status int) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
}
