package httpclient

import (
	"net"
	"net/http"
	"time"

	"server/internal/platform/config"
)

func New(configuration config.HTTPClientConfig) (*http.Client, error) {
	if err := configuration.Validate(); err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   configuration.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          configuration.MaxIdleConns,
		MaxIdleConnsPerHost:   configuration.MaxIdleConnsPerHost,
		MaxConnsPerHost:       configuration.MaxConnsPerHost,
		IdleConnTimeout:       configuration.IdleConnTimeout,
		TLSHandshakeTimeout:   configuration.TLSHandshakeTimeout,
		ResponseHeaderTimeout: configuration.ResponseHeaderTimeout,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{Transport: transport, Timeout: configuration.Timeout}, nil
}
