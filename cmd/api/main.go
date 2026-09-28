package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"server/internal/platform/config"
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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:              configuration.HTTPAddr,
		Handler:           mux,
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
		return err
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
		return err
	case <-shutdownSignal.Done():
		stop()
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), configuration.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}
