package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func ErrorBody(code, message string) map[string]map[string]string {
	return map[string]map[string]string{"error": {"code": code, "message": message}}
}

func Error(writer http.ResponseWriter, request *http.Request, logger *slog.Logger, status int, code, message string) {
	body, _ := json.Marshal(ErrorBody(code, message))
	RawJSON(writer, request, logger, status, string(body))
}
