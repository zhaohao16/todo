package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/todo-api/internal/todo"
)

// TestE2ETodoLifecycle drives the real stack over a real TCP connection: create,
// read, list, update, delete, and the 404 that follows.
func TestE2ETodoLifecycle(t *testing.T) {
	e := newEnv(t)
	srv := httptest.NewServer(e.router)
	t.Cleanup(srv.Close)

	client := srv.Client()

	// Create.
	resp, body := mustDo(t, client, http.MethodPost, srv.URL+"/api/v1/todos", "application/json", `{"title":"写端到端测试"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201 (body %s)", resp.StatusCode, body)
	}
	created := decodeJSONBody[todo.Todo](t, body)
	location := resp.Header.Get("Location")
	if location != "/api/v1/todos/1" {
		t.Fatalf("Location = %q, want /api/v1/todos/1", location)
	}

	// Read it back through the Location it advertised.
	resp, body = mustDo(t, client, http.MethodGet, srv.URL+location, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if got := decodeJSONBody[todo.Todo](t, body); got.Title != "写端到端测试" {
		t.Errorf("Title = %q, want %q", got.Title, "写端到端测试")
	}

	// List contains exactly it.
	resp, body = mustDo(t, client, http.MethodGet, srv.URL+"/api/v1/todos", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LIST status = %d, want 200", resp.StatusCode)
	}
	if got := decodeJSONBody[[]todo.Todo](t, body); len(got) != 1 || got[0].ID != created.ID {
		t.Errorf("list = %+v, want exactly todo %d", got, created.ID)
	}

	// Partial update.
	resp, body = mustDo(t, client, http.MethodPatch, srv.URL+location, "application/json", `{"completed":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	updated := decodeJSONBody[todo.Todo](t, body)
	if !updated.Completed || updated.Title != "写端到端测试" {
		t.Errorf("updated = %+v, want completed=true and title unchanged", updated)
	}

	// Delete.
	resp, _ = mustDo(t, client, http.MethodDelete, srv.URL+location, "", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
	}

	// It is gone.
	resp, body = mustDo(t, client, http.MethodGet, srv.URL+location, "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after DELETE status = %d, want 404 (body %s)", resp.StatusCode, body)
	}
	if got := decodeJSONBody[errEnvelope](t, body); got.Error.Code != "NOT_FOUND" {
		t.Errorf("error.code = %q, want NOT_FOUND", got.Error.Code)
	}
}

func TestE2EHealthz(t *testing.T) {
	e := newEnv(t)
	srv := httptest.NewServer(e.router)
	t.Cleanup(srv.Close)

	resp, body := mustDo(t, srv.Client(), http.MethodGet, srv.URL+"/healthz", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
}

func TestE2ENoRoute(t *testing.T) {
	e := newEnv(t)
	srv := httptest.NewServer(e.router)
	t.Cleanup(srv.Close)

	resp, _ := mustDo(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/nope", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func mustDo(t *testing.T, client *http.Client, method, url, contentType, body string) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s body: %v", method, url, err)
	}
	return resp, data
}

func decodeJSONBody[T any](t *testing.T, body []byte) T {
	t.Helper()

	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode body %q: %v", body, err)
	}
	return v
}
