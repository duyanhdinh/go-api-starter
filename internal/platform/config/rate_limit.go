package config

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

type RateLimitConfig struct {
	Enabled           bool
	RequestsPerSecond float64
	Burst             int
	MaxClients        int
	EntryTTL          time.Duration
}

func loadRateLimit(value func(string) string) (RateLimitConfig, error) {
	var configuration RateLimitConfig
	var err error
	configuration.Enabled, err = strconv.ParseBool(value("RATE_LIMIT_ENABLED"))
	if err != nil {
		return configuration, fmt.Errorf("RATE_LIMIT_ENABLED must be a boolean")
	}
	if !configuration.Enabled {
		return configuration, nil
	}
	configuration.RequestsPerSecond, err = strconv.ParseFloat(value("RATE_LIMIT_REQUESTS_PER_SECOND"), 64)
	if err != nil || math.IsNaN(configuration.RequestsPerSecond) || math.IsInf(configuration.RequestsPerSecond, 0) || configuration.RequestsPerSecond <= 0 {
		return configuration, fmt.Errorf("RATE_LIMIT_REQUESTS_PER_SECOND must be a positive number")
	}
	for _, setting := range []struct {
		name   string
		target *int
	}{
		{"RATE_LIMIT_BURST", &configuration.Burst},
		{"RATE_LIMIT_MAX_CLIENTS", &configuration.MaxClients},
	} {
		count, err := strconv.Atoi(value(setting.name))
		if err != nil || count <= 0 {
			return configuration, fmt.Errorf("%s must be a positive integer", setting.name)
		}
		*setting.target = count
	}
	configuration.EntryTTL, err = time.ParseDuration(value("RATE_LIMIT_ENTRY_TTL"))
	if err != nil || configuration.EntryTTL <= 0 {
		return configuration, fmt.Errorf("RATE_LIMIT_ENTRY_TTL must be a positive duration (for example 10m)")
	}
	return configuration, nil
}
