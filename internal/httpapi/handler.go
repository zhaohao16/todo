package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"example.com/todo-api/internal/todo"
)

// Handler serves the todo HTTP API. It holds no per-request state.
type Handler struct {
	svc *todo.Service
	log *slog.Logger
}

// NewHandler returns a Handler backed by svc and logging to log.
func NewHandler(svc *todo.Service, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Handler{svc: svc, log: log}
}

type createTodoRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
}

// updateTodoRequest mirrors PATCH semantics: an absent field stays untouched,
// which is why every field is a pointer.
type updateTodoRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Completed   *bool   `json:"completed"`
}

// Create handles POST /api/v1/todos.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	req, apiErr := decodeJSON[createTodoRequest](w, r)
	if apiErr != nil {
		apiErr.write(w)
		return
	}

	t, err := h.svc.Create(r.Context(), todo.CreateInput{
		Title:       req.Title,
		Description: req.Description,
		Completed:   req.Completed,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/todos/"+strconv.FormatInt(t.ID, 10))
	writeJSON(w, http.StatusCreated, t)
}

// List handles GET /api/v1/todos.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// Get handles GET /api/v1/todos/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, apiErr := parseID(r)
	if apiErr != nil {
		apiErr.write(w)
		return
	}

	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// Update handles PATCH /api/v1/todos/{id}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, apiErr := parseID(r)
	if apiErr != nil {
		apiErr.write(w)
		return
	}

	req, apiErr := decodeJSON[updateTodoRequest](w, r)
	if apiErr != nil {
		apiErr.write(w)
		return
	}

	t, err := h.svc.Update(r.Context(), id, todo.Patch{
		Title:       req.Title,
		Description: req.Description,
		Completed:   req.Completed,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// Delete handles DELETE /api/v1/todos/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, apiErr := parseID(r)
	if apiErr != nil {
		apiErr.write(w)
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Health handles GET /healthz.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// fail maps err to a client-facing error and writes it, logging server-side
// failures with the detail the client is not told.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := mapError(err)
	if apiErr.Status >= http.StatusInternalServerError {
		h.log.Error("request failed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Any("error", err),
		)
	}
	apiErr.write(w)
}
