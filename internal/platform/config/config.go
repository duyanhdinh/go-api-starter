package config

import (
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"
)

//go:embed profiles/*.json
var profiles embed.FS

type Config struct {
	Environment       string
	LogLevel          slog.Level
	HTTPAddr          string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	HTTPClient        HTTPClientConfig
	Database          DatabaseConfig
	CORS              CORSConfig
}

func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	environment := getenv("APP_ENV")
	if environment == "" {
		environment = "dev"
	}
	switch environment {
	case "dev", "test", "prod":
	default:
		return Config{}, fmt.Errorf("APP_ENV must be dev, test, or prod")
	}

	data, err := profiles.ReadFile("profiles/" + environment + ".json")
	if err != nil {
		return Config{}, fmt.Errorf("read configuration profile: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return Config{}, fmt.Errorf("decode configuration profile: %w", err)
	}
	value := func(name string) string {
		if override := getenv(name); override != "" {
			return override
		}
		return values[name]
	}
	configuration := Config{Environment: environment, HTTPAddr: value("HTTP_ADDR")}
	if err := configuration.LogLevel.UnmarshalText([]byte(value("LOG_LEVEL"))); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL must be a slog level (DEBUG, INFO, WARN, ERROR, optionally with a numeric offset)")
	}
	_, port, err := net.SplitHostPort(configuration.HTTPAddr)
	if err != nil {
		return Config{}, fmt.Errorf("HTTP_ADDR must be a host:port address")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		return Config{}, fmt.Errorf("HTTP_ADDR port must be between 0 and 65535")
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"HTTP_READ_HEADER_TIMEOUT", &configuration.ReadHeaderTimeout},
		{"HTTP_READ_TIMEOUT", &configuration.ReadTimeout},
		{"HTTP_WRITE_TIMEOUT", &configuration.WriteTimeout},
		{"HTTP_IDLE_TIMEOUT", &configuration.IdleTimeout},
		{"HTTP_SHUTDOWN_TIMEOUT", &configuration.ShutdownTimeout},
		{"HTTP_CLIENT_TIMEOUT", &configuration.HTTPClient.Timeout},
		{"HTTP_CLIENT_CONNECT_TIMEOUT", &configuration.HTTPClient.ConnectTimeout},
		{"HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT", &configuration.HTTPClient.TLSHandshakeTimeout},
		{"HTTP_CLIENT_RESPONSE_HEADER_TIMEOUT", &configuration.HTTPClient.ResponseHeaderTimeout},
		{"HTTP_CLIENT_IDLE_CONN_TIMEOUT", &configuration.HTTPClient.IdleConnTimeout},
	} {
		duration, err := time.ParseDuration(value(setting.name))
		if err != nil || duration <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration (for example 5s)", setting.name)
		}
		*setting.target = duration
	}
	for _, setting := range []struct {
		name   string
		target *int
	}{
		{"HTTP_CLIENT_MAX_IDLE_CONNS", &configuration.HTTPClient.MaxIdleConns},
		{"HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST", &configuration.HTTPClient.MaxIdleConnsPerHost},
		{"HTTP_CLIENT_MAX_CONNS_PER_HOST", &configuration.HTTPClient.MaxConnsPerHost},
	} {
		count, err := strconv.Atoi(value(setting.name))
		if err != nil {
			return Config{}, fmt.Errorf("%s must be a positive integer", setting.name)
		}
		*setting.target = count
	}
	if err := configuration.HTTPClient.Validate(); err != nil {
		return Config{}, err
	}
	configuration.CORS, err = loadCORS(value)
	if err != nil {
		return Config{}, err
	}
	configuration.Database, err = LoadDatabase(getenv)
	if err != nil {
		return Config{}, err
	}
	return configuration, nil
}
