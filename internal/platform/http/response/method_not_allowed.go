package response

import "net/http"

func MethodNotAllowed(allowedMethods func(*http.Request) string, writeError func(http.ResponseWriter, *http.Request, int, string, string)) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Allow", allowedMethods(request))
		writeError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
	}
}
