package logger

import (
	"io"
	"log/slog"
	"os"
)

func New(environment string, level slog.Level) *slog.Logger {
	return newLogger(os.Stdout, environment, level)
}

func newLogger(output io.Writer, environment string, level slog.Level) *slog.Logger {
	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler = slog.NewTextHandler(output, options)
	if environment == "prod" {
		handler = slog.NewJSONHandler(output, options)
	}
	return slog.New(handler)
}
