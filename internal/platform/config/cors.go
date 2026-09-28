package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type CORSConfig struct {
	Enabled          bool
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           time.Duration
}

func loadCORS(value func(string) string) (CORSConfig, error) {
	var configuration CORSConfig
	var err error
	configuration.Enabled, err = strconv.ParseBool(value("CORS_ENABLED"))
	if err != nil {
		return configuration, fmt.Errorf("CORS_ENABLED must be a boolean")
	}
	if !configuration.Enabled {
		return configuration, nil
	}
	configuration.AllowCredentials, err = strconv.ParseBool(value("CORS_ALLOW_CREDENTIALS"))
	if err != nil {
		return configuration, fmt.Errorf("CORS_ALLOW_CREDENTIALS must be a boolean")
	}
	configuration.MaxAge, err = time.ParseDuration(value("CORS_MAX_AGE"))
	if err != nil || configuration.MaxAge < 0 || configuration.MaxAge%time.Second != 0 {
		return configuration, fmt.Errorf("CORS_MAX_AGE must be a non-negative duration in whole seconds")
	}
	for _, setting := range []struct {
		name     string
		target   *[]string
		required bool
	}{
		{"CORS_ALLOWED_ORIGINS", &configuration.AllowedOrigins, true},
		{"CORS_ALLOWED_METHODS", &configuration.AllowedMethods, true},
		{"CORS_ALLOWED_HEADERS", &configuration.AllowedHeaders, false},
		{"CORS_EXPOSED_HEADERS", &configuration.ExposedHeaders, false},
	} {
		raw := value(setting.name)
		if raw == "" && !setting.required {
			continue
		}
		for _, entry := range strings.Split(raw, ",") {
			entry = strings.TrimSpace(entry)
			valid := validCORSToken(entry)
			if setting.name == "CORS_ALLOWED_ORIGINS" {
				valid = validOrigin(entry)
			}
			if !valid {
				return configuration, fmt.Errorf("%s must contain valid explicit values separated by commas (no wildcards)", setting.name)
			}
			*setting.target = append(*setting.target, entry)
		}
	}
	return configuration, nil
}

func validCORSToken(value string) bool {
	if value == "" || strings.Contains(value, "*") {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'+-.^_`|~", character) {
			continue
		}
		return false
	}
	return true
}

func validOrigin(value string) bool {
	origin, err := url.Parse(value)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || strings.ContainsAny(value, "*#\\") {
		return false
	}
	host := origin.Hostname()
	if strings.Contains(origin.Host, "[") && (!strings.Contains(host, ":") || net.ParseIP(host) == nil) {
		return false
	}
	if net.ParseIP(host) == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return false
			}
			for _, character := range label {
				if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' {
					continue
				}
				return false
			}
		}
	}
	if strings.HasSuffix(origin.Host, ":") {
		return false
	}
	if port := origin.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return false
		}
	}
	return true
}
