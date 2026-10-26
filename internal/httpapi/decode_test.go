package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int // 0 means the body is expected to decode
		wantCode    string
		wantTitle   string
	}{
		{
			name:        "valid object",
			contentType: "application/json",
			body:        `{"title":"买牛奶"}`,
			wantTitle:   "买牛奶",
		},
		{
			name:        "content type with charset",
			contentType: "application/json; charset=utf-8",
			body:        `{"title":"x"}`,
			wantTitle:   "x",
		},
		{
			name:       "missing content type",
			body:       `{"title":"x"}`,
			wantStatus: http.StatusUnsupportedMediaType,
			wantCode:   "UNSUPPORTED_MEDIA_TYPE",
		},
		{
			name:        "text/plain content type",
			contentType: "text/plain",
			body:        `{"title":"x"}`,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCode:    "UNSUPPORTED_MEDIA_TYPE",
		},
		{
			name:        "empty body",
			contentType: "application/json",
			body:        ``,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "VALIDATION_ERROR",
		},
		{
			name:        "malformed JSON",
			contentType: "application/json",
			body:        `{"title"`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "VALIDATION_ERROR",
		},
		{
			name:        "unknown field",
			contentType: "application/json",
			body:        `{"title":"x","due":"tomorrow"}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "VALIDATION_ERROR",
		},
		{
			name:        "trailing object",
			contentType: "application/json",
			body:        `{"title":"a"}{"title":"b"}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "VALIDATION_ERROR",
		},
		{
			name:        "array instead of object",
			contentType: "application/json",
			body:        `[]`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "VALIDATION_ERROR",
		},
		{
			name:        "body over the limit",
			contentType: "application/json",
			body:        strings.Repeat(" ", maxBodyBytes+1),
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantCode:    "PAYLOAD_TOO_LARGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/todos", strings.NewReader(tt.body))
			if tt.contentType != "" {
				r.Header.Set("Content-Type", tt.contentType)
			}
			w := httptest.NewRecorder()

			got, apiErr := decodeJSON[createTodoRequest](w, r)

			if tt.wantStatus == 0 {
				if apiErr != nil {
					t.Fatalf("decodeJSON() = %v, want success", apiErr)
				}
				if got.Title != tt.wantTitle {
					t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
				}
				return
			}

			if apiErr == nil {
				t.Fatalf("decodeJSON() succeeded, want status %d", tt.wantStatus)
			}
			if apiErr.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", apiErr.Status, tt.wantStatus)
			}
			if apiErr.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", apiErr.Code, tt.wantCode)
			}
		})
	}
}

func TestDecodeJSONRejectsTrailingWhitespaceOnly(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"title":"x"}  `+"\n"))
	r.Header.Set("Content-Type", "application/json")

	if _, apiErr := decodeJSON[createTodoRequest](httptest.NewRecorder(), r); apiErr != nil {
		t.Fatalf("decodeJSON() = %v, want success", apiErr)
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		raw  string
		want int64
		ok   bool
	}{
		{raw: "1", want: 1, ok: true},
		{raw: "42", want: 42, ok: true},
		{raw: "007", want: 7, ok: true},
		{raw: "0", ok: false},
		{raw: "-1", ok: false},
		{raw: "abc", ok: false},
		{raw: "1.5", ok: false},
		{raw: "+1", ok: false},
		{raw: " 1", ok: false},
		{raw: "", ok: false},
		{raw: "99999999999999999999", ok: false},
	}

	for _, tt := range tests {
		t.Run("id="+tt.raw, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/todos/1", nil)
			r.SetPathValue("id", tt.raw)

			got, apiErr := parseID(r)

			if !tt.ok {
				if apiErr == nil {
					t.Fatalf("parseID(%q) = %d, want an error", tt.raw, got)
				}
				if apiErr.Status != http.StatusBadRequest {
					t.Errorf("Status = %d, want 400", apiErr.Status)
				}
				return
			}
			if apiErr != nil {
				t.Fatalf("parseID(%q) = %v, want success", tt.raw, apiErr)
			}
			if got != tt.want {
				t.Errorf("parseID(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}
