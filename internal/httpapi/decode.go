package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
)

// maxBodyBytes bounds request bodies so a single client cannot exhaust memory.
const maxBodyBytes = 1 << 20 // 1 MiB

// decodeJSON reads exactly one JSON object from the request body into T.
//
// It rejects a missing or non-JSON Content-Type, unknown fields, malformed
// JSON, trailing data and bodies over maxBodyBytes, mapping each to a
// client-actionable error. The decoded value is not validated semantically;
// that is the domain layer's job.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, *apiError) {
	var v T

	if err := requireJSONContentType(r); err != nil {
		return v, err
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return v, &apiError{
				Status:  http.StatusRequestEntityTooLarge,
				Code:    "PAYLOAD_TOO_LARGE",
				Message: "request body must not exceed 1 MiB",
			}
		}
		return v, &apiError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: "invalid request body",
		}
	}

	// A second value means the body is not a single object.
	if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return v, &apiError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: "request body must contain a single JSON object",
		}
	}

	return v, nil
}

// requireJSONContentType enforces an application/json body, tolerating media
// type parameters such as "charset=utf-8".
func requireJSONContentType(r *http.Request) *apiError {
	raw := r.Header.Get("Content-Type")
	if raw == "" {
		return errUnsupportedMediaType
	}

	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil || mediaType != "application/json" {
		return errUnsupportedMediaType
	}

	return nil
}

// parseID reads the {id} path wildcard as a positive decimal integer. Only
// plain digits are accepted, so "+1", " 1", "1.5" and "0x10" are all invalid.
func parseID(r *http.Request) (int64, *apiError) {
	raw := r.PathValue("id")

	invalid := &apiError{
		Status:  http.StatusBadRequest,
		Code:    "VALIDATION_ERROR",
		Message: "invalid todo id",
		Details: []fieldDetail{{Field: "id", Message: "must be a positive integer"}},
	}

	if !isDigits(raw) {
		return 0, invalid
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, invalid
	}

	return id, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
