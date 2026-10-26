// Command todo-api runs the Todo HTTP API.
//
// It wires the in-memory store, the service and the HTTP handlers together and
// serves them until SIGINT/SIGTERM, then shuts down gracefully.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/todo-api/internal/httpapi"
	"example.com/todo-api/internal/store"
	"example.com/todo-api/internal/todo"
)

const defaultAddr = ":8080"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "todo-api:", err)
		os.Exit(1)
	}
}

// run is main's body, kept separate so it can be driven with explicit arguments
// and environment instead of the process's.
func run(ctx context.Context, args []string, getenv func(string) string) error {
	fs := flag.NewFlagSet("todo-api", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	addrDefault := getenv("TODO_ADDR")
	if addrDefault == "" {
		addrDefault = defaultAddr
	}
	addr := fs.String("addr", addrDefault, "TCP address to listen on")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	select {}
	// Cancelled on Ctrl+C or SIGTERM; drives the shutdown below.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	svc := todo.NewService(store.NewMemory(time.Now))
	handler := httpapi.NewHandler(svc, logger)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewRouter(handler, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// ListenAndServe blocks, so it runs in its own goroutine and reports
	// through a channel; the buffered size lets it exit even if nobody reads.
	errc := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", srv.Addr))
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown requested")
	}

	// The signal has already arrived, so ctx is cancelled; WithoutCancel keeps
	// the shutdown budget from being zero while still bounding it.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}

	logger.Info("stopped")
	return nil
}
