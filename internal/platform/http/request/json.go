package request

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"server/internal/platform/http/response"
)

var (
	ErrUnsupportedMediaType = errors.New("Content-Type must be application/json")
	ErrMultipleJSONValues   = errors.New("request body must contain a single JSON value")
)

func ReadJSON[Value any](writer http.ResponseWriter, request *http.Request, logger *slog.Logger, maxBytes int64, invalidMessage string) (Value, bool) {
	var input Value
	if err := DecodeJSON(writer, request, &input, maxBytes); err != nil {
		var tooLarge *http.MaxBytesError
		status, code, message := http.StatusBadRequest, "invalid_input", invalidMessage
		switch {
		case errors.Is(err, ErrUnsupportedMediaType):
			status, code, message = http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json"
		case errors.As(err, &tooLarge):
			status, code, message = http.StatusRequestEntityTooLarge, "body_too_large", fmt.Sprintf("Request body exceeds %d bytes", maxBytes)
		}
		response.JSON(writer, request, logger, status, response.ErrorBody(code, message))
		var zero Value
		return zero, false
	}
	return input, true
}

func DecodeJSON(writer http.ResponseWriter, request *http.Request, destination any, maxBytes int64) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return ErrUnsupportedMediaType
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	err = decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return ErrMultipleJSONValues
	}
	return err
}
