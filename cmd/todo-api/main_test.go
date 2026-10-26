package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func noEnv(string) string { return "" }

// freeAddr reserves an ephemeral port and releases it, giving the server under
// test an address nobody else is using.
func freeAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release the reserved port: %v", err)
	}
	return addr
}

// waitForHealth polls /healthz until the server answers 200. It is the portable
// way to wait for a listener to come up without a fixed sleep.
func waitForHealth(t *testing.T, base string) {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/healthz") //nolint:noctx // short-lived poll
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("server at %s never became healthy", base)
}

// serve runs the real entry point in the background and waits until it answers.
func serve(t *testing.T, args []string, getenv func(string) string) (context.CancelFunc, chan error) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- run(ctx, args, getenv) }()

	return cancel, errc
}

// stop cancels the context and asserts the server returned cleanly.
func stop(t *testing.T, cancel context.CancelFunc, errc chan error) {
	t.Helper()

	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("run() = %v, want nil after a graceful shutdown", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run() did not return after cancellation")
	}
}

func TestRunServesRequestsThenShutsDown(t *testing.T) {
	addr := freeAddr(t)
	cancel, errc := serve(t, []string{"-addr", addr}, noEnv)
	base := "http://" + addr

	waitForHealth(t, base)

	// A real request through the fully wired stack: flags, store, service,
	// router, handlers and JSON encoding.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(base+"/api/v1/todos", "application/json", strings.NewReader(`{"title":"wired"}`))
	if err != nil {
		t.Fatalf("POST /api/v1/todos: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read POST body: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201 (body %s)", resp.StatusCode, body)
	}

	stop(t, cancel, errc)
}

func TestRunUsesTODOADDRWhenNoFlag(t *testing.T) {
	addr := freeAddr(t)
	getenv := func(key string) string {
		if key == "TODO_ADDR" {
			return addr
		}
		return ""
	}

	cancel, errc := serve(t, nil, getenv)
	waitForHealth(t, "http://"+addr)

	stop(t, cancel, errc)
}

func TestRunFlagOverridesEnvironment(t *testing.T) {
	flagAddr := freeAddr(t)
	envAddr := freeAddr(t)
	getenv := func(key string) string {
		if key == "TODO_ADDR" {
			return envAddr
		}
		return ""
	}

	cancel, errc := serve(t, []string{"-addr", flagAddr}, getenv)
	waitForHealth(t, "http://"+flagAddr)

	// The environment address must stay untouched: the flag wins.
	if conn, err := net.DialTimeout("tcp", envAddr, 300*time.Millisecond); err == nil {
		conn.Close()
		t.Errorf("something listens on TODO_ADDR (%s); the flag should have taken precedence", envAddr)
	}

	stop(t, cancel, errc)
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	if err := run(context.Background(), []string{"-nope"}, noEnv); err == nil {
		t.Fatal("run() = nil, want an error for an unknown flag")
	}
}

func TestRunReportsListenFailure(t *testing.T) {
	err := run(context.Background(), []string{"-addr", "not-an-address"}, noEnv)
	if err == nil {
		t.Fatal("run() = nil, want an error for an unusable address")
	}
}
