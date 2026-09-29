package user

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"server/internal/platform/http/middleware"
	httprequest "server/internal/platform/http/request"
	"server/internal/platform/http/response"

	"github.com/go-chi/chi/v5"
)

const (
	maxRequestBodyBytes = 4096
	invalidInputMessage = "Invalid user input or pagination"
)

type Handler struct {
	service        *Service
	logger         *slog.Logger
	requestTimeout time.Duration
}

func NewHandler(service *Service, logger *slog.Logger, requestTimeout time.Duration) *Handler {
	return &Handler{service: service, logger: logger, requestTimeout: requestTimeout}
}

func (handler *Handler) RegisterRoutes(router chi.Router) {
	router.Use(middleware.Timeout(handler.requestTimeout))
	router.MethodNotAllowed(response.MethodNotAllowed(allowedMethods, handler.writeError))
	router.Post("/", handler.create)
	router.Get("/", handler.list)
	router.Get("/{id}", handler.get)
	router.Put("/{id}", handler.update)
	router.Delete("/{id}", handler.delete)
}

func allowedMethods(request *http.Request) string {
	routePath := chi.RouteContext(request.Context()).RoutePath
	if routePath == "" {
		routePath = request.URL.Path
	}
	if strings.Trim(routePath, "/") != "" {
		return "GET, PUT, DELETE"
	}
	return "GET, POST"
}

func (handler *Handler) create(writer http.ResponseWriter, request *http.Request) {
	input, ok := httprequest.ReadJSON[Input](writer, request, handler.logger, maxRequestBodyBytes, invalidInputMessage)
	if !ok {
		return
	}
	result, err := handler.service.Create(request.Context(), input)
	if err != nil {
		handler.fail(writer, request, err)
		return
	}
	writer.Header().Set("Location", strings.TrimRight(request.URL.EscapedPath(), "/")+"/"+strconv.FormatInt(result.ID, 10))
	response.JSON(writer, request, handler.logger, http.StatusCreated, result)
}

func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	if err != nil {
		handler.fail(writer, request, ErrInvalid)
		return
	}
	result, err := handler.service.Get(request.Context(), id)
	if err != nil {
		handler.fail(writer, request, err)
		return
	}
	response.JSON(writer, request, handler.logger, http.StatusOK, result)
}

func (handler *Handler) list(writer http.ResponseWriter, request *http.Request) {
	limit, offset := DefaultPageSize, 0
	for key, target := range map[string]*int{"limit": &limit, "offset": &offset} {
		if values, exists := request.URL.Query()[key]; exists {
			value, err := strconv.Atoi(values[0])
			if err != nil || len(values) != 1 {
				handler.fail(writer, request, ErrInvalid)
				return
			}
			*target = value
		}
	}
	result, err := handler.service.List(request.Context(), limit, offset)
	if err != nil {
		handler.fail(writer, request, err)
		return
	}
	response.JSON(writer, request, handler.logger, http.StatusOK, result)
}

func (handler *Handler) update(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	if err != nil {
		handler.fail(writer, request, ErrInvalid)
		return
	}
	input, ok := httprequest.ReadJSON[Input](writer, request, handler.logger, maxRequestBodyBytes, invalidInputMessage)
	if !ok {
		return
	}
	result, err := handler.service.Update(request.Context(), id, input)
	if err != nil {
		handler.fail(writer, request, err)
		return
	}
	response.JSON(writer, request, handler.logger, http.StatusOK, result)
}

func (handler *Handler) delete(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	if err != nil {
		handler.fail(writer, request, ErrInvalid)
		return
	}
	if err := handler.service.Delete(request.Context(), id); err != nil {
		handler.fail(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) fail(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		handler.writeError(writer, request, http.StatusBadRequest, "invalid_input", invalidInputMessage)
	case errors.Is(err, ErrNotFound):
		handler.writeError(writer, request, http.StatusNotFound, "not_found", "User not found")
	case errors.Is(err, ErrEmailTaken):
		handler.writeError(writer, request, http.StatusConflict, "email_taken", "Email already exists")
	default:
		handler.logger.ErrorContext(request.Context(), "User operation failed", "method", request.Method)
		handler.writeError(writer, request, http.StatusInternalServerError, "internal_error", "Internal server error")
	}
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	response.JSON(writer, request, handler.logger, status, response.ErrorBody(code, message))
}
