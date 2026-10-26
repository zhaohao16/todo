package httpapi

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/todo-api/internal/store"
	"example.com/todo-api/internal/todo"
)

// env is a router wired to a real in-memory store and a controllable clock, so
// tests exercise the true handler → service → store path.
type env struct {
	t      *testing.T
	router http.Handler
	now    time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()

	e := &env{t: t, now: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return e.now }

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := todo.NewService(store.NewMemory(clock))
	e.router = NewRouter(NewHandler(svc, logger), logger)

	return e
}

func (e *env) advance(d time.Duration) { e.now = e.now.Add(d) }

func (e *env) do(method, path, contentType, body string) *httptest.ResponseRecorder {
	e.t.Helper()

	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, r)

	return rr
}

func (e *env) doJSON(method, path, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(method, path, "application/json", body)
}

// wantError asserts status, JSON content type and the error envelope code.
func (e *env) wantError(rr *httptest.ResponseRecorder, status int, code string) errEnvelope {
	e.t.Helper()

	if rr.Code != status {
		e.t.Fatalf("status = %d, want %d (body %s)", rr.Code, status, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		e.t.Errorf("Content-Type = %q, want JSON", ct)
	}

	got := decodeBody[errEnvelope](e.t, rr)
	if got.Error.Code != code {
		e.t.Errorf("error.code = %q, want %q", got.Error.Code, code)
	}
	return got
}

func (e *env) create(body string) todo.Todo {
	e.t.Helper()

	rr := e.doJSON(http.MethodPost, "/api/v1/todos", body)
	if rr.Code != http.StatusCreated {
		e.t.Fatalf("POST /api/v1/todos = %d, want 201 (body %s)", rr.Code, rr.Body.String())
	}
	return decodeBody[todo.Todo](e.t, rr)
}

func todoPath(id int64) string { return fmt.Sprintf("/api/v1/todos/%d", id) }

func TestCreateTodo(t *testing.T) {
	e := newEnv(t)

	rr := e.doJSON(http.MethodPost, "/api/v1/todos", `{"title":"  买牛奶  ","description":"低脂"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rr.Code, rr.Body.String())
	}
	if got, want := rr.Header().Get("Location"), "/api/v1/todos/1"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	got := decodeBody[todo.Todo](t, rr)
	if got.ID != 1 {
		t.Errorf("ID = %d, want 1", got.ID)
	}
	if got.Title != "买牛奶" {
		t.Errorf("Title = %q, want %q", got.Title, "买牛奶")
	}
	if got.Description != "低脂" {
		t.Errorf("Description = %q, want %q", got.Description, "低脂")
	}
	if got.Completed {
		t.Error("Completed = true, want false by default")
	}
	if !got.CreatedAt.Equal(e.now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, e.now)
	}
	if !got.UpdatedAt.Equal(e.now) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, e.now)
	}
}

func TestCreateTodoWithCompleted(t *testing.T) {
	e := newEnv(t)

	got := e.create(`{"title":"already done","completed":true}`)

	if !got.Completed {
		t.Error("Completed = false, want true")
	}
}

func TestCreateValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantMsg    string
		wantFields []string
	}{
		{
			name:       "empty object",
			body:       `{}`,
			wantFields: []string{"title"},
		},
		{
			name:       "blank title",
			body:       `{"title":"   "}`,
			wantFields: []string{"title"},
		},
		{
			name:       "title too long",
			body:       `{"title":"` + strings.Repeat("字", 201) + `"}`,
			wantFields: []string{"title"},
		},
		{
			name:       "description too long",
			body:       `{"title":"x","description":"` + strings.Repeat("a", 2001) + `"}`,
			wantFields: []string{"description"},
		},
		{
			name:    "unknown field",
			body:    `{"title":"x","priority":3}`,
			wantMsg: "invalid request body",
		},
		{
			name:    "malformed JSON",
			body:    `{"title":`,
			wantMsg: "invalid request body",
		},
		{
			name:    "two objects",
			body:    `{"title":"a"}{"title":"b"}`,
			wantMsg: "request body must contain a single JSON object",
		},
		{
			name:    "array body",
			body:    `[]`,
			wantMsg: "invalid request body",
		},
		{
			name:    "wrong field type",
			body:    `{"title":123}`,
			wantMsg: "invalid request body",
		},
		{
			name:       "null body",
			body:       `null`,
			wantFields: []string{"title"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)

			rr := e.doJSON(http.MethodPost, "/api/v1/todos", tt.body)

			got := e.wantError(rr, http.StatusBadRequest, "VALIDATION_ERROR")
			if tt.wantMsg != "" && got.Error.Message != tt.wantMsg {
				t.Errorf("error.message = %q, want %q", got.Error.Message, tt.wantMsg)
			}
			if tt.wantFields != nil {
				if len(got.Error.Details) != len(tt.wantFields) {
					t.Fatalf("details = %+v, want fields %v", got.Error.Details, tt.wantFields)
				}
				for i, want := range tt.wantFields {
					if got.Error.Details[i].Field != want {
						t.Errorf("details[%d].field = %q, want %q", i, got.Error.Details[i].Field, want)
					}
				}
			}
		})
	}
}

func TestCreateWrongContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
	}{
		{name: "missing", contentType: ""},
		{name: "text/plain", contentType: "text/plain"},
		{name: "form", contentType: "application/x-www-form-urlencoded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)

			rr := e.do(http.MethodPost, "/api/v1/todos", tt.contentType, `{"title":"x"}`)

			e.wantError(rr, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE")
		})
	}
}

func TestCreateBodyTooLarge(t *testing.T) {
	e := newEnv(t)

	rr := e.doJSON(http.MethodPost, "/api/v1/todos", strings.Repeat(" ", maxBodyBytes+1))

	e.wantError(rr, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE")
}

func TestListEmpty(t *testing.T) {
	e := newEnv(t)

	rr := e.do(http.MethodGet, "/api/v1/todos", "", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != "[]" {
		t.Errorf("body = %q, want %q", body, "[]")
	}
}

func TestListAfterCreates(t *testing.T) {
	e := newEnv(t)
	e.create(`{"title":"first"}`)
	e.create(`{"title":"second"}`)

	rr := e.do(http.MethodGet, "/api/v1/todos", "", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got := decodeBody[[]todo.Todo](t, rr)
	if len(got) != 2 {
		t.Fatalf("len(body) = %d, want 2", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("IDs = %d, %d; want ascending 1, 2", got[0].ID, got[1].ID)
	}
}

func TestGet(t *testing.T) {
	e := newEnv(t)
	created := e.create(`{"title":"买牛奶"}`)

	rr := e.do(http.MethodGet, todoPath(created.ID), "", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got := decodeBody[todo.Todo](t, rr)
	if got.ID != created.ID || got.Title != created.Title {
		t.Errorf("body = %+v, want id=%d title=%q", got, created.ID, created.Title)
	}
}

func TestGetNotFound(t *testing.T) {
	e := newEnv(t)

	rr := e.do(http.MethodGet, "/api/v1/todos/42", "", "")

	got := e.wantError(rr, http.StatusNotFound, "NOT_FOUND")
	if got.Error.Message != "todo not found" {
		t.Errorf("error.message = %q, want %q", got.Error.Message, "todo not found")
	}
}

func TestInvalidID(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-1", "1.5", "+1", "99999999999999999999"} {
		t.Run("id="+raw, func(t *testing.T) {
			e := newEnv(t)

			rr := e.do(http.MethodGet, "/api/v1/todos/"+raw, "", "")

			got := e.wantError(rr, http.StatusBadRequest, "VALIDATION_ERROR")
			if got.Error.Message != "invalid todo id" {
				t.Errorf("error.message = %q, want %q", got.Error.Message, "invalid todo id")
			}
		})
	}
}

func TestUpdateCompleted(t *testing.T) {
	e := newEnv(t)
	created := e.create(`{"title":"买牛奶","description":"低脂"}`)
	e.advance(time.Minute)

	rr := e.doJSON(http.MethodPatch, todoPath(created.ID), `{"completed":true}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	got := decodeBody[todo.Todo](t, rr)
	if !got.Completed {
		t.Error("Completed = false, want true")
	}
	if got.Title != "买牛奶" || got.Description != "低脂" {
		t.Errorf("untouched fields changed: title=%q description=%q", got.Title, got.Description)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want unchanged %v", got.CreatedAt, created.CreatedAt)
	}
	if !got.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want after %v", got.UpdatedAt, created.UpdatedAt)
	}
}

func TestUpdateFieldsAreTrimmed(t *testing.T) {
	e := newEnv(t)
	created := e.create(`{"title":"old"}`)

	rr := e.doJSON(http.MethodPatch, todoPath(created.ID), `{"title":"  新标题  "}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := decodeBody[todo.Todo](t, rr); got.Title != "新标题" {
		t.Errorf("Title = %q, want %q", got.Title, "新标题")
	}
}

func TestUpdateEmptyPatch(t *testing.T) {
	e := newEnv(t)
	created := e.create(`{"title":"买牛奶"}`)

	rr := e.doJSON(http.MethodPatch, todoPath(created.ID), `{}`)

	got := e.wantError(rr, http.StatusBadRequest, "VALIDATION_ERROR")
	if got.Error.Message != "at least one field must be provided" {
		t.Errorf("error.message = %q, want %q", got.Error.Message, "at least one field must be provided")
	}
	if len(got.Error.Details) != 0 {
		t.Errorf("details = %+v, want none", got.Error.Details)
	}
}

func TestUpdateEmptyTitle(t *testing.T) {
	e := newEnv(t)
	created := e.create(`{"title":"买牛奶"}`)

	rr := e.doJSON(http.MethodPatch, todoPath(created.ID), `{"title":"  "}`)

	got := e.wantError(rr, http.StatusBadRequest, "VALIDATION_ERROR")
	if len(got.Error.Details) != 1 || got.Error.Details[0].Field != "title" {
		t.Errorf("details = %+v, want one title error", got.Error.Details)
	}
}

func TestUpdateUnknownField(t *testing.T) {
	e := newEnv(t)
	created := e.create(`{"title":"买牛奶"}`)

	rr := e.doJSON(http.MethodPatch, todoPath(created.ID), `{"done":true}`)

	e.wantError(rr, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestUpdateNotFound(t *testing.T) {
	e := newEnv(t)

	rr := e.doJSON(http.MethodPatch, "/api/v1/todos/42", `{"completed":true}`)

	e.wantError(rr, http.StatusNotFound, "NOT_FOUND")
}

func TestUpdateInvalidID(t *testing.T) {
	e := newEnv(t)

	rr := e.doJSON(http.MethodPatch, "/api/v1/todos/abc", `{"completed":true}`)

	e.wantError(rr, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	created := e.create(`{"title":"doomed"}`)

	rr := e.do(http.MethodDelete, todoPath(created.ID), "", "")

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if body := rr.Body.String(); body != "" {
		t.Errorf("body = %q, want empty", body)
	}

	after := e.do(http.MethodGet, todoPath(created.ID), "", "")
	e.wantError(after, http.StatusNotFound, "NOT_FOUND")
}

func TestDeleteNotFound(t *testing.T) {
	e := newEnv(t)

	rr := e.do(http.MethodDelete, "/api/v1/todos/42", "", "")

	e.wantError(rr, http.StatusNotFound, "NOT_FOUND")
}

func TestDeleteInvalidID(t *testing.T) {
	e := newEnv(t)

	rr := e.do(http.MethodDelete, "/api/v1/todos/abc", "", "")

	e.wantError(rr, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestMethodNotAllowed(t *testing.T) {
	e := newEnv(t)

	rr := e.do(http.MethodDelete, "/api/v1/todos", "", "")

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
	allow := rr.Header().Get("Allow")
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if !strings.Contains(allow, method) {
			t.Errorf("Allow = %q, want it to contain %s", allow, method)
		}
	}
}

func TestHealth(t *testing.T) {
	e := newEnv(t)

	rr := e.do(http.MethodGet, "/healthz", "", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != `{"status":"ok"}` {
		t.Errorf("body = %q, want %q", body, `{"status":"ok"}`)
	}
}
