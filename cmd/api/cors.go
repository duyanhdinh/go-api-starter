package main

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"server/internal/platform/config"
)

func withCORS(next http.Handler, configuration config.CORSConfig) http.Handler {
	if !configuration.Enabled {
		return next
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		allowed := len(request.Header.Values("Origin")) == 1 && slices.Contains(configuration.AllowedOrigins, origin)
		preflight := request.Method == http.MethodOptions && origin != "" && request.Header.Get("Access-Control-Request-Method") != ""
		vary := []string{"Origin"}
		if preflight {
			vary = append(vary, "Access-Control-Request-Method", "Access-Control-Request-Headers")
			allowed = allowed && len(request.Header.Values("Access-Control-Request-Method")) == 1 && slices.Contains(configuration.AllowedMethods, request.Header.Get("Access-Control-Request-Method"))
			for _, line := range request.Header.Values("Access-Control-Request-Headers") {
				for _, header := range strings.Split(line, ",") {
					header = strings.TrimSpace(header)
					if !slices.ContainsFunc(configuration.AllowedHeaders, func(candidate string) bool { return strings.EqualFold(candidate, header) }) {
						allowed = false
					}
				}
			}
		}
		apply := func(headers http.Header) {
			for _, name := range vary {
				addVary(headers, name)
			}
			if !allowed {
				return
			}
			headers.Set("Access-Control-Allow-Origin", origin)
			if configuration.AllowCredentials {
				headers.Set("Access-Control-Allow-Credentials", "true")
			}
			if preflight {
				headers.Set("Access-Control-Allow-Methods", strings.Join(configuration.AllowedMethods, ", "))
				if len(configuration.AllowedHeaders) > 0 {
					headers.Set("Access-Control-Allow-Headers", strings.Join(configuration.AllowedHeaders, ", "))
				}
				headers.Set("Access-Control-Max-Age", strconv.FormatInt(int64(configuration.MaxAge/time.Second), 10))
			} else if len(configuration.ExposedHeaders) > 0 {
				headers.Set("Access-Control-Expose-Headers", strings.Join(configuration.ExposedHeaders, ", "))
			}
		}
		if preflight {
			apply(writer.Header())
			if !allowed {
				writer.WriteHeader(http.StatusForbidden)
			} else {
				writer.WriteHeader(http.StatusNoContent)
			}
			return
		}
		wrapped := &corsResponseWriter{ResponseWriter: writer, apply: apply}
		next.ServeHTTP(wrapped, request)
		if !wrapped.committed {
			apply(writer.Header())
		}
	})
}

func addVary(headers http.Header, name string) {
	for _, line := range headers.Values("Vary") {
		for _, value := range strings.Split(line, ",") {
			if strings.EqualFold(strings.TrimSpace(value), name) || strings.TrimSpace(value) == "*" {
				return
			}
		}
	}
	headers.Add("Vary", name)
}

type corsResponseWriter struct {
	http.ResponseWriter
	apply     func(http.Header)
	committed bool
}

func (writer *corsResponseWriter) WriteHeader(status int) {
	if !writer.committed {
		writer.apply(writer.Header())
	}
	if status >= 200 || status == http.StatusSwitchingProtocols {
		writer.committed = true
	}
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *corsResponseWriter) Write(body []byte) (int, error) {
	if !writer.committed {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(body)
}

func (writer *corsResponseWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

func (writer *corsResponseWriter) FlushError() error {
	if !writer.committed {
		writer.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(writer.ResponseWriter).Flush()
}
