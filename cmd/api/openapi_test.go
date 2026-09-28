package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"server/api"
	"server/internal/platform/config"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/go-chi/chi/v5"
)

func TestAPIContractMatchesRoutesAndResponses(t *testing.T) {
	document := loadAPIContract(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := newRouter(logger, false)
	actualRoutes := map[string]bool{}
	if err := chi.Walk(router, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		actualRoutes[method+" "+path] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expectedRoutes := map[string]bool{
		"GET /health": true, "HEAD /health": true,
		"GET /ready": true, "HEAD /ready": true,
	}
	if !sameSet(actualRoutes, expectedRoutes) {
		t.Fatalf("handler routes = %v, expected %v", actualRoutes, expectedRoutes)
	}
	documentedRoutes := make(map[string]bool)
	for path, item := range document.Paths.Map() {
		for method := range item.Operations() {
			documentedRoutes[strings.ToUpper(method)+" "+path] = true
		}
	}
	if !sameSet(documentedRoutes, actualRoutes) {
		t.Fatalf("OpenAPI routes = %v, handler routes = %v", documentedRoutes, actualRoutes)
	}

	readyFailure := func(context.Context) error { return errNotReady }
	for _, testCase := range []struct {
		method      string
		path        string
		readiness   []func(context.Context) error
		status      int
		body        string
		contentType string
	}{
		{http.MethodGet, "/health", nil, http.StatusOK, `{"status":"ok"}`, "application/json"},
		{http.MethodGet, "/ready", nil, http.StatusOK, `{"status":"ok"}`, "application/json"},
		{http.MethodGet, "/ready", []func(context.Context) error{readyFailure}, http.StatusServiceUnavailable, `{"status":"unavailable"}`, "application/json"},
		{http.MethodHead, "/health", nil, http.StatusOK, "", "application/json"},
		{http.MethodHead, "/ready", nil, http.StatusOK, "", "application/json"},
		{http.MethodHead, "/ready", []func(context.Context) error{readyFailure}, http.StatusServiceUnavailable, "", "application/json"},
	} {
		t.Run(testCase.method+" "+testCase.path+" "+http.StatusText(testCase.status), func(t *testing.T) {
			handler := newHandlerWithDocs(logger, false, testCase.readiness...)
			request := httptest.NewRequest(testCase.method, testCase.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.status || response.Body.String() != testCase.body {
				t.Fatalf("response = %d %q", response.Code, response.Body.String())
			}
			if response.Header().Get("Content-Type") != testCase.contentType || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("response headers = %v", response.Header())
			}
			validateContractResponse(t, document, request, response)
		})
	}

	t.Run("405", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/health", nil)
		response := httptest.NewRecorder()
		newHandlerWithDocs(logger, false).ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" || response.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("405 response = %v", response)
		}
		validateContractResponse(t, document, request, response)
	})
	t.Run("HEAD 500", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodHead, "/ready", nil)
		response := httptest.NewRecorder()
		newHandlerWithDocs(logger, false, func(context.Context) error { panic("private panic data") }).ServeHTTP(response, request)
		if response.Code != http.StatusInternalServerError || response.Body.Len() != 0 || response.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("HEAD 500 response = %v", response)
		}
		validateContractResponse(t, document, request, response)
	})
	t.Run("500", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/ready", nil)
		response := httptest.NewRecorder()
		newHandlerWithDocs(logger, false, func(context.Context) error { panic("private panic data") }).ServeHTTP(response, request)
		if response.Code != http.StatusInternalServerError || response.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("500 response = %v", response)
		}
		validateContractResponse(t, document, request, response)
	})
}

func TestContractValidatorRejectsResponseSchemaDrift(t *testing.T) {
	document := loadAPIContract(t)
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"status":"drifted"}`))
	}).ServeHTTP(response, request)
	if err := validateResponse(document, request, response); err == nil {
		t.Fatal("contract accepted a response that violates its schema")
	}
}

func TestContractLoaderRejectsInvalidSpecAndMissingReference(t *testing.T) {
	for _, fixture := range []string{
		"openapi: not-a-version\ninfo: {}\n",
		"openapi: 3.1.1\ninfo:\n  title: invalid reference\n  version: 1.0.0\npaths:\n  /health:\n    get:\n      operationId: getHealth\n      responses:\n        '200':\n          $ref: '#/components/responses/Missing'\n",
	} {
		loader := openapi3.NewLoader()
		loader.IsExternalRefsAllowed = false
		document, err := loader.LoadFromData([]byte(fixture))
		if err == nil && document != nil {
			err = document.Validate(context.Background())
		}
		if err == nil {
			t.Fatalf("contract validator accepted invalid fixture: %s", fixture)
		}
	}
}

func TestDocsRoutesAndDisabledRoutingSmoke(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configuration := config.RateLimitConfig{}
	withDocs := withCORS(withRateLimit(newHandlerWithDocs(logger, true), configuration, logger), config.CORSConfig{})
	server := httptest.NewServer(withDocs)
	defer server.Close()
	client := server.Client()

	redirectClient := *client
	redirectClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := redirectClient.Get(server.URL + "/docs")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusPermanentRedirect || response.Header.Get("Location") != "/docs/" {
		t.Fatalf("/docs redirect = %d %q", response.StatusCode, response.Header.Get("Location"))
	}
	response.Body.Close()
	headRedirect, err := redirectClient.Head(server.URL + "/docs")
	if err != nil {
		t.Fatal(err)
	}
	redirectBody, _ := io.ReadAll(headRedirect.Body)
	headRedirect.Body.Close()
	if headRedirect.StatusCode != http.StatusPermanentRedirect || headRedirect.Header.Get("Location") != "/docs/" || len(redirectBody) != 0 {
		t.Fatalf("HEAD /docs redirect = %d %v", headRedirect.StatusCode, headRedirect.Header)
	}

	for _, testCase := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{"/docs/", "text/html; charset=utf-8", `url: "/openapi.yaml"`},
		{"/openapi.yaml", "application/yaml; charset=utf-8", "openapi: 3.1.1"},
		{"/docs/swagger-ui-bundle.js", "application/javascript; charset=utf-8", "SwaggerUIBundle"},
		{"/docs/swagger-ui-standalone-preset.js", "application/javascript; charset=utf-8", "SwaggerUIStandalonePreset"},
		{"/docs/swagger-ui.css", "text/css; charset=utf-8", ".swagger-ui"},
	} {
		response, err := client.Get(server.URL + testCase.path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != testCase.contentType || !strings.Contains(string(body), testCase.contains) {
			t.Fatalf("GET %s: status=%d type=%q readErr=%v", testCase.path, response.StatusCode, response.Header.Get("Content-Type"), readErr)
		}
		head, err := client.Head(server.URL + testCase.path)
		if err != nil {
			t.Fatal(err)
		}
		head.Body.Close()
		if head.StatusCode != http.StatusOK || head.Header.Get("Content-Type") != testCase.contentType || head.Header.Get("Content-Length") != response.Header.Get("Content-Length") {
			t.Fatalf("HEAD %s: status=%d headers=%v", testCase.path, head.StatusCode, head.Header)
		}
	}
	unknownDocs, err := client.Get(server.URL + "/docs/unlisted")
	if err != nil {
		t.Fatal(err)
	}
	unknownDocs.Body.Close()
	if unknownDocs.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown docs asset returned %d", unknownDocs.StatusCode)
	}

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request, err := http.NewRequest(method, server.URL+"/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s /health with docs enabled returned %d", method, response.StatusCode)
		}
	}

	withoutDocs := withCORS(withRateLimit(newHandlerWithDocs(logger, false), configuration, logger), config.CORSConfig{})
	disabledServer := httptest.NewServer(withoutDocs)
	defer disabledServer.Close()
	disabledClient := disabledServer.Client()
	for _, path := range []string{"/docs", "/docs/", "/docs/swagger-ui.css", "/openapi.yaml"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			request, err := http.NewRequest(method, disabledServer.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := disabledClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusNotFound || (method == http.MethodHead && len(body) != 0) {
				t.Fatalf("%s %s with docs disabled returned %d with body %q", method, path, response.StatusCode, body)
			}
		}
	}
	response, err = disabledClient.Get(disabledServer.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health with docs disabled returned %d", response.StatusCode)
	}
}

func loadAPIContract(t *testing.T) *openapi3.T {
	t.Helper()
	data, err := api.Files.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	document, err := loader.LoadFromData(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return document
}

func validateContractResponse(t *testing.T, document *openapi3.T, request *http.Request, response *httptest.ResponseRecorder) {
	t.Helper()
	if err := validateResponse(document, request, response); err != nil {
		t.Fatal(err)
	}
}

func validateResponse(document *openapi3.T, request *http.Request, response *httptest.ResponseRecorder) error {
	pathItem := document.Paths.Value(request.URL.Path)
	if pathItem == nil {
		return errNoContractPath
	}
	operation := pathItem.GetOperation(request.Method)
	if operation == nil && request.Method == http.MethodPost {
		operation = pathItem.Get
	}
	if operation == nil {
		return errNoContractOperation
	}
	route := &routers.Route{Spec: document, Path: request.URL.Path, PathItem: pathItem, Method: request.Method, Operation: operation}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, Route: route},
		Status:                 response.Code,
		Header:                 response.Header(),
	}
	input.SetBodyBytes(response.Body.Bytes())
	return openapi3filter.ValidateResponse(request.Context(), input)
}

func sameSet(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for item := range left {
		if !right[item] {
			return false
		}
	}
	return true
}

var errNotReady = context.Canceled

var errNoContractPath = errors.New("request path is absent from the OpenAPI contract")

var errNoContractOperation = errors.New("request method is absent from the OpenAPI contract")
