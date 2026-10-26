package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func TestRecovererReturnsJSON500(t *testing.T) {
	var buf bytes.Buffer
	h := recoverer(newTestLogger(&buf), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	got := decodeBody[errEnvelope](t, rr)
	if got.Error.Code != "INTERNAL" {
		t.Errorf("error.code = %q, want INTERNAL", got.Error.Code)
	}
	if strings.Contains(rr.Body.String(), "boom") {
		t.Errorf("body = %q leaks the panic value", rr.Body.String())
	}
	if !strings.Contains(buf.String(), "panic recovered") {
		t.Errorf("log = %q, want it to record the panic", buf.String())
	}
}

func TestRecovererDoesNotRewriteStartedResponse(t *testing.T) {
	var buf bytes.Buffer
	h := recoverer(newTestLogger(&buf), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		panic("late panic")
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (the header was already sent)", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "internal server error") {
		t.Errorf("body = %q, want the original body untouched", rr.Body.String())
	}
	if !strings.Contains(buf.String(), "panic recovered") {
		t.Errorf("log = %q, want it to record the panic", buf.String())
	}
}

func TestRequestLoggerRecordsStatus(t *testing.T) {
	var buf bytes.Buffer
	h := requestLogger(newTestLogger(&buf), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))

	out := buf.String()
	if !strings.Contains(out, "status=404") {
		t.Errorf("log = %q, want status=404", out)
	}
	if !strings.Contains(out, "/missing") {
		t.Errorf("log = %q, want the request path", out)
	}
}

func TestRequestLoggerAssumesImplicit200(t *testing.T) {
	var buf bytes.Buffer
	h := requestLogger(newTestLogger(&buf), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if out := buf.String(); !strings.Contains(out, "status=200") {
		t.Errorf("log = %q, want status=200", out)
	}
}
