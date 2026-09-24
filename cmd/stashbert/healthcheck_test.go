package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// healthServerPort starts a test server on 127.0.0.1 that answers
// GET /api/v1/health with status (other paths get 404) and returns its port.
func healthServerPort(t *testing.T, status int) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strconv.Itoa(srv.Listener.Addr().(*net.TCPAddr).Port)
}

// closedPort returns a port on 127.0.0.1 that nothing listens on.
func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return strconv.Itoa(port)
}

func TestParseFlagsHealthcheck(t *testing.T) {
	tests := []struct {
		name     string
		port     func(t *testing.T) string
		wantExit int
	}{
		{"200", func(t *testing.T) string { return healthServerPort(t, http.StatusOK) }, 0},
		{"500", func(t *testing.T) string { return healthServerPort(t, http.StatusInternalServerError) }, 1},
		{"unreachable", closedPort, 1},
		{"invalid PORT", func(*testing.T) string { return "abc" }, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			port := tt.port(t)
			getenv := func(key string) string {
				if key == "PORT" {
					return port
				}
				return ""
			}

			var stdout, stderr bytes.Buffer
			done, err := parseFlags(ctx, []string{"-healthcheck"}, "dev", getenv, &stdout, &stderr)
			if !done {
				t.Error("done = false, want true (the server must not start)")
			}
			exit := 0
			if err != nil {
				exit = exitCode(err)
			}
			if exit != tt.wantExit {
				t.Errorf("exit code = %d (err = %v), want %d", exit, err, tt.wantExit)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if failed := tt.wantExit != 0; (stderr.Len() != 0) != failed {
				t.Errorf("stderr = %q, want a reason only on failure", stderr.String())
			}
		})
	}
}

func TestHealthURL(t *testing.T) {
	tests := []struct {
		port    string
		want    string
		wantErr bool
	}{
		{"", "http://127.0.0.1:8080/api/v1/health", false},
		{"9090", "http://127.0.0.1:9090/api/v1/health", false},
		{"abc", "", true},
		{"0", "", true},
		{"65536", "", true},
	}
	for _, tt := range tests {
		got, err := healthURL(func(key string) string {
			if key == "PORT" {
				return tt.port
			}
			return ""
		})
		if (err != nil) != tt.wantErr {
			t.Errorf("PORT=%q: err = %v, want error %t", tt.port, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("PORT=%q: url = %q, want %q", tt.port, got, tt.want)
		}
	}
}
