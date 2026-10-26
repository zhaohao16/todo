package httpapi

import (
	"log/slog"
	"net/http"
)

// NewRouter returns the handler for the whole service. Method-aware patterns
// are provided by the Go 1.22+ ServeMux, so no routing dependency is needed.
// Middleware is applied outermost-first: recoverer, then logging.
func NewRouter(h *Handler, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/todos", h.Create)
	mux.HandleFunc("GET /api/v1/todos", h.List)
	mux.HandleFunc("GET /api/v1/todos/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/todos/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/todos/{id}", h.Delete)
	mux.HandleFunc("GET /healthz", h.Health)

	return recoverer(log, requestLogger(log, mux))
}
