package main

import (
	"log/slog"
	"net/http"

	"server/internal/platform/config"
	"server/internal/platform/http/middleware"
)

func withRateLimit(next http.Handler, configuration config.RateLimitConfig, logger *slog.Logger) http.Handler {
	return middleware.RateLimit(next, configuration, logger, isRateLimitExempt)
}

func isRateLimitExempt(request *http.Request) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	return request.URL.Path == "/health" || request.URL.Path == "/ready"
}
