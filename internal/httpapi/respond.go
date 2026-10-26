// Package httpapi exposes the todo use cases over HTTP/JSON using only the
// standard library. It translates transport concerns into calls on
// *todo.Service and never touches storage directly.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"example.com/todo-api/internal/todo"
)

const jsonContentType = "application/json; charset=utf-8"

// apiError is the transport-level representation of a failed request. Every
// failure the client may act on is one of these; everything else becomes a
// generic INTERNAL error.
type apiError struct {
	Status  int
	Code    string
	Message string
	Details []fieldDetail
}

// Error implements error so apiError can flow through ordinary error handling.
func (e *apiError) Error() string { return e.Code + ": " + e.Message }

// write sends the error as the standard JSON envelope.
func (e *apiError) write(w http.ResponseWriter) {
	writeJSON(w, e.Status, errorEnvelope{
		Error: errorBody{Code: e.Code, Message: e.Message, Details: e.Details},
	})
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []fieldDetail `json:"details,omitempty"`
}

type fieldDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// errUnsupportedMediaType is returned when the request does not carry a JSON
// body, so the client can fix the call instead of retrying it.
var errUnsupportedMediaType = &apiError{
	Status:  http.StatusUnsupportedMediaType,
	Code:    "UNSUPPORTED_MEDIA_TYPE",
	Message: "Content-Type must be application/json",
}

// mapError converts a domain or transport error into the error a client sees.
// Unexpected errors deliberately lose their detail; they are logged instead.
func mapError(err error) *apiError {
	if errors.Is(err, todo.ErrNotFound) {
		return &apiError{
			Status:  http.StatusNotFound,
			Code:    "NOT_FOUND",
			Message: "todo not found",
		}
	}

	var ve *todo.ValidationError
	if errors.As(err, &ve) {
		message := ve.Message
		if message == "" {
			message = "invalid request body"
		}
		details := make([]fieldDetail, 0, len(ve.Fields))
		for _, f := range ve.Fields {
			details = append(details, fieldDetail{Field: f.Field, Message: f.Message})
		}
		return &apiError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: message,
			Details: details,
		}
	}

	return &apiError{
		Status:  http.StatusInternalServerError,
		Code:    "INTERNAL",
		Message: "internal server error",
	}
}

// writeJSON sets the JSON content type, writes status and encodes v. A failed
// encode is logged rather than panicked on: the status line is already sent.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode JSON response", slog.Any("error", err))
	}
}
