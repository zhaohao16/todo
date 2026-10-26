package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/todo-api/internal/todo"
)

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()

	writeJSON(rr, http.StatusCreated, map[string]string{"hello": "世界"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if ct := rr.Header().Get("Content-Type"); ct != jsonContentType {
		t.Errorf("Content-Type = %q, want %q", ct, jsonContentType)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != `{"hello":"世界"}` {
		t.Errorf("body = %q, want %q", body, `{"hello":"世界"}`)
	}
}

// failingWriter fails every write, simulating a client that hung up.
type failingWriter struct {
	header http.Header
}

func (f *failingWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func (f *failingWriter) WriteHeader(int)           {}

func TestWriteJSONDoesNotPanicOnWriteError(t *testing.T) {
	// The status line is already committed by the time the body fails, so the
	// only safe behaviour is to swallow the error.
	writeJSON(&failingWriter{}, http.StatusOK, map[string]string{"a": "b"})
}

func TestMapError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "wrapped not found",
			err:        fmt.Errorf("get todo 7: %w", todo.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "validation",
			err:        &todo.ValidationError{Fields: []todo.FieldError{{Field: "title", Message: "must not be empty"}}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "unknown error becomes internal",
			err:        errors.New("disk on fire"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapError(tt.err)

			if got.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", got.Status, tt.wantStatus)
			}
			if got.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tt.wantCode)
			}
			if got.Status == http.StatusInternalServerError {
				if strings.Contains(got.Message, "disk on fire") {
					t.Errorf("Message = %q leaks internal detail", got.Message)
				}
				if len(got.Details) != 0 {
					t.Errorf("Details = %v, want none", got.Details)
				}
			}
		})
	}
}

func TestMapErrorIncludesFieldDetails(t *testing.T) {
	err := &todo.ValidationError{Fields: []todo.FieldError{
		{Field: "title", Message: "must not be empty"},
		{Field: "description", Message: "must be at most 2000 characters"},
	}}

	got := mapError(err)

	if got.Message != "invalid request body" {
		t.Errorf("Message = %q, want %q", got.Message, "invalid request body")
	}
	if len(got.Details) != 2 {
		t.Fatalf("len(Details) = %d, want 2", len(got.Details))
	}
	if got.Details[0].Field != "title" || got.Details[1].Field != "description" {
		t.Errorf("Details = %+v, want title then description", got.Details)
	}
}

func TestMapErrorCarriesRequestLevelMessage(t *testing.T) {
	err := &todo.ValidationError{Message: "at least one field must be provided"}

	got := mapError(err)

	if got.Message != "at least one field must be provided" {
		t.Errorf("Message = %q, want the request-level message", got.Message)
	}
	if len(got.Details) != 0 {
		t.Errorf("Details = %v, want none", got.Details)
	}
}

// errEnvelope mirrors the wire format of an error response.
type errEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"details"`
	} `json:"error"`
}

func decodeBody[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()

	var v T
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode body %q: %v", rr.Body.String(), err)
	}
	return v
}
