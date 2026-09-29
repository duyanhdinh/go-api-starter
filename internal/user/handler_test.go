package user

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"server/api"

	"github.com/go-chi/chi/v5"
)

func testRouter(repository Repository) http.Handler {
	router := chi.NewRouter()
	router.Route("/api/v1/users", NewHandler(NewService(repository), slog.New(slog.NewTextHandler(io.Discard, nil)), 5*time.Second).RegisterRoutes)
	return router
}

type deadlineRepository struct {
	Repository
	deadline    time.Time
	hasDeadline bool
}

func (repository *deadlineRepository) List(ctx context.Context, limit, offset int) ([]User, error) {
	repository.deadline, repository.hasDeadline = ctx.Deadline()
	return []User{}, nil
}

func TestHandlerRequestTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{time.Second, 9 * time.Second} {
		repository := &deadlineRepository{}
		handler := NewHandler(NewService(repository), slog.New(slog.NewTextHandler(io.Discard, nil)), timeout)
		router := chi.NewRouter()
		router.Route("/users", handler.RegisterRoutes)
		started := time.Now()
		response := requestJSON(router, "GET", "/users", "")
		finished := time.Now()
		if response.Code != http.StatusOK || !repository.hasDeadline {
			t.Fatalf("status=%d, hasDeadline=%v", response.Code, repository.hasDeadline)
		}
		if repository.deadline.Before(started.Add(timeout)) || repository.deadline.After(finished.Add(timeout)) {
			t.Fatalf("repository deadline does not match configured timeout %s", timeout)
		}
	}
}

func TestHandlerBodyLimitBoundary(t *testing.T) {
	body := `{"email":"an@example.com","name":"An"}`
	for _, size := range []int{4096, 4097} {
		for _, method := range []string{"POST", "PUT"} {
			repository := newMemoryRepository()
			router := testRouter(repository)
			path := "/api/v1/users"
			want := http.StatusCreated
			if method == "PUT" {
				requestJSON(router, "POST", path, body)
				path += "/1"
				want = http.StatusOK
			}
			if size > 4096 {
				want = http.StatusRequestEntityTooLarge
			}
			response := requestJSON(router, method, path, body+strings.Repeat(" ", size-len(body)))
			if response.Code != want {
				t.Fatalf("%s size=%d: got %d, want %d", method, size, response.Code, want)
			}
		}
	}
}

func TestHandlerDefaultPageSize(t *testing.T) {
	repository := newMemoryRepository()
	for id := int64(1); id <= 21; id++ {
		repository.users[id] = User{ID: id, Email: "an@example.com", Name: "An"}
	}
	response := requestJSON(testRouter(repository), "GET", "/api/v1/users", "")
	var records []User
	if err := json.Unmarshal(response.Body.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(records) != 20 {
		t.Fatalf("status=%d, count=%d", response.Code, len(records))
	}
}

func requestJSON(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestCRUDHTTP(t *testing.T) {
	handler := testRouter(newMemoryRepository())
	data, err := api.Files.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	document, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		method, path, body string
		status             int
		contains           string
	}{
		{"GET", "/api/v1/users", "", 200, "[]"},
		{"POST", "/api/v1/users", `{"email":"AN@example.com","name":" An "}`, 201, `"email":"an@example.com"`},
		{"GET", "/api/v1/users/1", "", 200, `"name":"An"`},
		{"POST", "/api/v1/users", `{"email":"an@example.com","name":"Other"}`, 409, "email_taken"},
		{"POST", "/api/v1/users", `{"email":"other@example.com","name":"Other"}`, 201, `"id":2`},
		{"PUT", "/api/v1/users/1", `{"email":"other@example.com","name":"Changed"}`, 409, "email_taken"},
		{"GET", "/api/v1/users/1", "", 200, `"name":"An"`},
		{"PUT", "/api/v1/users/1", `{"email":"new@example.com","name":" New "}`, 200, `"name":"New"`},
		{"GET", "/api/v1/users?limit=1&offset=1", "", 200, `"id":2`},
		{"DELETE", "/api/v1/users/1", "", 204, ""},
		{"GET", "/api/v1/users/1", "", 404, "not_found"},
		{"PUT", "/api/v1/users/1", `{"email":"new@example.com","name":"New"}`, 404, "not_found"},
		{"DELETE", "/api/v1/users/1", "", 404, "not_found"},
	} {
		response := requestJSON(handler, step.method, step.path, step.body)
		request := httptest.NewRequest(step.method, step.path, strings.NewReader(step.body))
		contractPath := "/api/v1/users"
		if strings.HasPrefix(request.URL.Path, "/api/v1/users/") {
			contractPath = "/api/v1/users/{id}"
		}
		pathItem := document.Paths.Value(contractPath)
		validation := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, Route: &routers.Route{Spec: document, Path: contractPath, PathItem: pathItem, Method: step.method, Operation: pathItem.GetOperation(step.method)}},
			Status:                 response.Code, Header: response.Header(),
		}
		validation.SetBodyBytes(response.Body.Bytes())
		if err := openapi3filter.ValidateResponse(context.Background(), validation); err != nil {
			t.Fatalf("%s %s contract: %v", step.method, step.path, err)
		}
		if response.Code != step.status || !strings.Contains(response.Body.String(), step.contains) {
			t.Fatalf("%s %s: %d %s", step.method, step.path, response.Code, response.Body.String())
		}
		if step.status == 204 && response.Body.Len() != 0 {
			t.Fatal("204 has body")
		}
		if step.status == 201 {
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result) != 3 || response.Header().Get("Location") == "" {
				t.Fatal("unexpected create contract")
			}
		}
	}
}

func TestHTTPRejectsInvalidRequests(t *testing.T) {
	handler := testRouter(newMemoryRepository())
	for _, testCase := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/v1/users/0", "", 400}, {"GET", "/api/v1/users/9223372036854775808", "", 400},
		{"DELETE", "/api/v1/users/abc", "", 400}, {"PUT", "/api/v1/users/abc", `{}`, 400},
		{"GET", "/api/v1/users?limit=101", "", 400}, {"GET", "/api/v1/users?offset=-1", "", 400},
		{"GET", "/api/v1/users?limit=", "", 400}, {"GET", "/api/v1/users?limit=1&limit=2", "", 400},
		{"POST", "/api/v1/users", `{`, 400}, {"POST", "/api/v1/users", `null`, 400},
		{"POST", "/api/v1/users", `{"email":"a@example.com","name":"A","id":1}`, 400},
		{"POST", "/api/v1/users", `{"email":"a@example.com","name":"A"}{}`, 400},
		{"PUT", "/api/v1/users/1", `{"name":"A"}`, 400},
		{"POST", "/api/v1/users", `{"email":"a@example.com","name":"` + strings.Repeat("a", 5000) + `"}`, 413},
		{"POST", "/api/v1/users", `{"email":"a@example.com","name":"A"}` + strings.Repeat(" ", 5000), 413},
	} {
		response := requestJSON(handler, testCase.method, testCase.path, testCase.body)
		if response.Code != testCase.status {
			t.Errorf("%s %s: got %d want %d", testCase.method, testCase.path, response.Code, testCase.status)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(`{}`)))
	if response.Code != 415 {
		t.Fatal("missing content type accepted")
	}
	for path, allow := range map[string]string{"/api/v1/users": "GET, POST", "/api/v1/users/1": "GET, PUT, DELETE"} {
		response := requestJSON(handler, "PATCH", path, "")
		if response.Code != 405 || response.Header().Get("Allow") != allow {
			t.Fatalf("missing 405 Allow: %v", response)
		}
	}
}

func TestHTTPHidesStorageErrors(t *testing.T) {
	repository := newMemoryRepository()
	repository.failure = errors.New("private connection string")
	response := requestJSON(testRouter(repository), "GET", "/api/v1/users/1", "")
	if response.Code != 500 || strings.Contains(response.Body.String(), "private") || !strings.Contains(response.Body.String(), "internal_error") {
		t.Fatal("incorrect storage error response")
	}
}

func TestRoutesCanBeMountedAtAnyPrefix(t *testing.T) {
	for _, prefix := range []string{"", "/api/v1/users", "/admin/accounts"} {
		t.Run(prefix, func(t *testing.T) {
			router := chi.NewRouter()
			handler := NewHandler(NewService(newMemoryRepository()), slog.New(slog.NewTextHandler(io.Discard, nil)), 5*time.Second)
			if prefix == "" {
				handler.RegisterRoutes(router)
			} else {
				router.Route(prefix, handler.RegisterRoutes)
			}
			collection := prefix
			if collection == "" {
				collection = "/"
			}
			created := requestJSON(router, "POST", collection+"?source=test", `{"email":"a@example.com","name":"A"}`)
			location := prefix + "/1"
			if created.Code != 201 || created.Header().Get("Location") != location {
				t.Fatalf("create: status=%d location=%q want=%q", created.Code, created.Header().Get("Location"), location)
			}
			if response := requestJSON(router, "GET", location, ""); response.Code != 200 {
				t.Fatal("Location does not resolve")
			}
			for path, allow := range map[string]string{collection: "GET, POST", prefix + "/": "GET, POST", location: "GET, PUT, DELETE"} {
				response := requestJSON(router, "PATCH", path, "")
				if response.Code != 405 || response.Header().Get("Allow") != allow {
					t.Fatalf("%s: status=%d Allow=%q", path, response.Code, response.Header().Get("Allow"))
				}
			}
			created = requestJSON(router, "POST", prefix+"/", `{"email":"b@example.com","name":"B"}`)
			if created.Code != 201 || created.Header().Get("Location") != prefix+"/2" {
				t.Fatal("trailing slash produced incorrect Location")
			}
		})
	}
}
