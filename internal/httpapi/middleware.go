package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// responseRecorder remembers the status code that reached the client so logging
// and panic recovery can tell whether the response has already started.
type responseRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.wrote {
		return
	}
	r.status = status
	r.wrote = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// recoverer keeps one bad request from taking down the process. It must be the
// outermost middleware so it also covers panics raised by other middleware.
func recoverer(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec, ok := w.(*responseRecorder)
		if !ok {
			rec = &responseRecorder{ResponseWriter: w}
			w = rec
		}

		defer func() {
			panicValue := recover()
			if panicValue == nil {
				return
			}

			log.Error("panic recovered",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Any("panic", panicValue),
				slog.String("stack", string(debug.Stack())),
			)

			// The status line cannot be changed once written; adding a JSON
			// envelope now would corrupt a response the client may be reading.
			if rec.wrote {
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorEnvelope{
				Error: errorBody{Code: "INTERNAL", Message: "internal server error"},
			})
		}()

		next.ServeHTTP(w, r)
	})
}

// requestLogger records one structured line per request.
func requestLogger(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec, ok := w.(*responseRecorder)
		if !ok {
			rec = &responseRecorder{ResponseWriter: w}
			w = rec
		}

		start := time.Now()
		defer func() {
			status := rec.status
			if !rec.wrote {
				// No handler wrote anything, so net/http will send 200.
				status = http.StatusOK
			}
			log.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Duration("duration", time.Since(start)),
			)
		}()

		next.ServeHTTP(w, r)
	})
}
