package main

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"server/internal/platform/config"
	"server/internal/platform/database"
	"server/internal/platform/http/middleware"
	"server/internal/user"
)

type applicationHandlers struct {
	Users *user.Handler
}

func newApplicationHandler(configuration config.Config, applicationLogger *slog.Logger, pool *database.Pool) http.Handler {
	router := newRouter(applicationLogger, configuration.DocsEnabled, pool.Ping)
	if pool != nil {
		repository := user.NewPostgresRepository(pool.DB)
		service := user.NewService(repository)
		registerApplicationRoutes(router, applicationHandlers{
			Users: user.NewHandler(service, applicationLogger, configuration.UserRequestTimeout),
		})
	}
	return middleware.CORS(withRateLimit(middleware.Recover(router, applicationLogger), configuration.RateLimit, applicationLogger), configuration.CORS)
}

func registerApplicationRoutes(router chi.Router, handlers applicationHandlers) {
	router.Route("/api/v1", func(router chi.Router) {
		router.Route("/users", handlers.Users.RegisterRoutes)
	})
}
