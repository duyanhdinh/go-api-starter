package httpclient

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"server/internal/platform/config"
)

func testConfiguration() config.HTTPClientConfig {
	return config.HTTPClientConfig{
		Timeout: 3 * time.Second, ConnectTimeout: time.Second,
		TLSHandshakeTimeout: time.Second, ResponseHeaderTimeout: time.Second,
		IdleConnTimeout: time.Minute, MaxIdleConns: 10, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 5,
	}
}

func newTestClient(t *testing.T, configuration config.HTTPClientConfig) *http.Client {
	t.Helper()
	client, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func TestTransportIsolation(t *testing.T) {
	defaultTransport := http.DefaultTransport
	transportBefore := transportSettings(defaultTransport.(*http.Transport))
	defaultClient := http.DefaultClient
	clientBefore := *defaultClient
	configuration := testConfiguration()
	client := newTestClient(t, configuration)
	other := newTestClient(t, configuration)
	transport := client.Transport.(*http.Transport)
	if client == defaultClient || client.Transport == defaultTransport || client.Transport == other.Transport {
		t.Fatal("client or transport is shared")
	}
	if client.Timeout != configuration.Timeout || client.CheckRedirect != nil || client.Jar != nil ||
		transport.TLSHandshakeTimeout != configuration.TLSHandshakeTimeout ||
		transport.ResponseHeaderTimeout != configuration.ResponseHeaderTimeout ||
		transport.IdleConnTimeout != configuration.IdleConnTimeout ||
		transport.MaxIdleConns != configuration.MaxIdleConns ||
		transport.MaxIdleConnsPerHost != configuration.MaxIdleConnsPerHost ||
		transport.MaxConnsPerHost != configuration.MaxConnsPerHost ||
		transport.TLSClientConfig != nil || !transport.ForceAttemptHTTP2 ||
		transport.DialContext == nil || transport.ExpectContinueTimeout != time.Second ||
		reflect.ValueOf(transport.Proxy).Pointer() != reflect.ValueOf(http.ProxyFromEnvironment).Pointer() {
		t.Fatal("unexpected client configuration")
	}
	if http.DefaultTransport != defaultTransport || http.DefaultClient != defaultClient ||
		!reflect.DeepEqual(clientBefore, *defaultClient) ||
		!reflect.DeepEqual(transportBefore, transportSettings(defaultTransport.(*http.Transport))) {
		t.Fatal("global HTTP defaults changed")
	}
}

func transportSettings(transport *http.Transport) map[string]any {
	settings := make(map[string]any)
	value := reflect.ValueOf(transport).Elem()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if !field.CanInterface() {
			continue
		}
		name := value.Type().Field(index).Name
		if field.Kind() == reflect.Func {
			settings[name] = field.Pointer()
		} else {
			settings[name] = field.Interface()
		}
	}
	return settings
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	for _, field := range []string{"Timeout", "ConnectTimeout", "TLSHandshakeTimeout", "ResponseHeaderTimeout", "IdleConnTimeout", "MaxIdleConns", "MaxIdleConnsPerHost", "MaxConnsPerHost"} {
		t.Run(field, func(t *testing.T) {
			configuration := testConfiguration()
			reflect.ValueOf(&configuration).Elem().FieldByName(field).SetInt(0)
			if client, err := New(configuration); err == nil || client != nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestResponseStatusAndConnectionReuse(t *testing.T) {
	var connections atomic.Int32
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		status, _ := strconv.Atoi(request.URL.Path[1:])
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, "response")
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	client := newTestClient(t, testConfiguration())
	for _, status := range []int{200, 400, 500} {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/"+strconv.Itoa(status), nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1024))
		closeErr := response.Body.Close()
		if response.StatusCode != status || string(body) != "response" || readErr != nil || closeErr != nil {
			t.Fatalf("unexpected response: status=%d, body=%q, read=%v, close=%v", response.StatusCode, body, readErr, closeErr)
		}
	}
	if connections.Load() != 1 || requests.Load() != 3 {
		t.Fatalf("connections=%d, requests=%d", connections.Load(), requests.Load())
	}
	client.CloseIdleConnections()
	response, err := client.Get(server.URL + "/200")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if connections.Load() != 2 {
		t.Fatal("idle connection was not released")
	}
}

func TestContextCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	client := newTestClient(t, testConfiguration())
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		response, err := client.Do(request)
		if response != nil {
			response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach server")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestTimeouts(t *testing.T) {
	for _, phase := range []string{"total", "headers", "body", "context"} {
		t.Run(phase, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if phase == "body" {
					writer.WriteHeader(http.StatusOK)
					writer.(http.Flusher).Flush()
				}
				<-request.Context().Done()
			}))
			defer server.Close()
			configuration := testConfiguration()
			if phase == "headers" {
				configuration.ResponseHeaderTimeout = 100 * time.Millisecond
			} else if phase != "context" {
				configuration.Timeout = 100 * time.Millisecond
			}
			client := newTestClient(t, configuration)
			requestContext := context.Background()
			if phase == "context" {
				var cancel context.CancelFunc
				requestContext, cancel = context.WithTimeout(requestContext, 100*time.Millisecond)
				defer cancel()
			}
			request, err := http.NewRequestWithContext(requestContext, http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(request)
			if response != nil {
				defer response.Body.Close()
			}
			if phase == "body" {
				if err != nil {
					t.Fatal(err)
				}
				_, err = io.ReadAll(io.LimitReader(response.Body, 1024))
			}
			var timeout net.Error
			if !errors.As(err, &timeout) || !timeout.Timeout() {
				t.Fatalf("expected timeout, got %v", err)
			}
			if phase == "context" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected context deadline, got %v", err)
			}
		})
	}
}

func TestTLSVerification(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	client := newTestClient(t, testConfiguration())
	response, err := client.Get(server.URL)
	if response != nil {
		response.Body.Close()
	}
	if err == nil {
		t.Fatal("untrusted TLS certificate accepted")
	}
}
