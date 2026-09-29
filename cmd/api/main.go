package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"server/internal/platform/config"
	"server/internal/platform/database"
	"server/internal/platform/logger"
)

func main() {
	configuration, err := config.Load()
	if err != nil {
		logger.New(os.Getenv("APP_ENV"), slog.LevelInfo).Error("Invalid configuration", "error", err)
		os.Exit(1)
	}
	applicationLogger := logger.New(configuration.Environment, configuration.LogLevel).With("service", "api", "env", configuration.Environment)
	if err := run(configuration, applicationLogger); err != nil {
		applicationLogger.Error("API server failed", "error", err)
		os.Exit(1)
	}
}

func run(configuration config.Config, applicationLogger *slog.Logger) error {
	pool, err := database.Open(context.Background(), configuration.Database)
	if err != nil {
		return err
	}
	defer func() {
		if err := pool.Close(); err != nil {
			applicationLogger.Error("Database shutdown failed", "error", err)
		}
	}()
	server := &http.Server{
		Addr:              configuration.HTTPAddr,
		Handler:           newApplicationHandler(configuration, applicationLogger, pool),
		ReadHeaderTimeout: configuration.ReadHeaderTimeout,
		ReadTimeout:       configuration.ReadTimeout,
		WriteTimeout:      configuration.WriteTimeout,
		IdleTimeout:       configuration.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(applicationLogger.Handler(), slog.LevelError),
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen for HTTP requests: %w", err)
	}
	defer listener.Close()

	serverErrors := make(chan error, 1)
	go func() {
		applicationLogger.Info("API server listening", "addr", listener.Addr().String())
		serverErrors <- server.Serve(listener)
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP requests: %w", err)
	case <-shutdownSignal.Done():
		stop()
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), configuration.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("shut down HTTP server: %w", errors.Join(err, server.Close()))
	}
	return nil
}
