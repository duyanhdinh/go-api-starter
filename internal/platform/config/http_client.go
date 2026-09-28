package config

import (
	"fmt"
	"time"
)

type HTTPClientConfig struct {
	Timeout               time.Duration
	ConnectTimeout        time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
}

func (configuration HTTPClientConfig) Validate() error {
	for _, setting := range []struct {
		name  string
		value time.Duration
	}{
		{"HTTP_CLIENT_TIMEOUT", configuration.Timeout},
		{"HTTP_CLIENT_CONNECT_TIMEOUT", configuration.ConnectTimeout},
		{"HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT", configuration.TLSHandshakeTimeout},
		{"HTTP_CLIENT_RESPONSE_HEADER_TIMEOUT", configuration.ResponseHeaderTimeout},
		{"HTTP_CLIENT_IDLE_CONN_TIMEOUT", configuration.IdleConnTimeout},
	} {
		if setting.value <= 0 {
			return fmt.Errorf("%s must be a positive duration", setting.name)
		}
	}
	for _, setting := range []struct {
		name  string
		value int
	}{
		{"HTTP_CLIENT_MAX_IDLE_CONNS", configuration.MaxIdleConns},
		{"HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST", configuration.MaxIdleConnsPerHost},
		{"HTTP_CLIENT_MAX_CONNS_PER_HOST", configuration.MaxConnsPerHost},
	} {
		if setting.value <= 0 {
			return fmt.Errorf("%s must be a positive integer", setting.name)
		}
	}
	if configuration.MaxIdleConnsPerHost > configuration.MaxIdleConns || configuration.MaxIdleConnsPerHost > configuration.MaxConnsPerHost {
		return fmt.Errorf("HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST must not exceed HTTP_CLIENT_MAX_IDLE_CONNS or HTTP_CLIENT_MAX_CONNS_PER_HOST")
	}
	return nil
}
