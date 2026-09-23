package app_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/config"
)

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	h, err := app.NewHandler(cfg, app.Deps{Logger: slog.New(slog.DiscardHandler), Version: "dev"})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func TestHealth(t *testing.T) {
	h := newHandler(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"status":"ok","version":"dev"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestRoutingProblems(t *testing.T) {
	tests := []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "/api/v1/unbekannt", http.StatusNotFound, "not_found"},
		{http.MethodPost, "/api/v1/health", http.StatusMethodNotAllowed, "method_not_allowed"},
	}
	h := newHandler(t)
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", ct)
			}
			var p struct {
				Type, Title, Code string
				Status            int
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("decode body %q: %v", rec.Body.String(), err)
			}
			if p.Type != "about:blank" || p.Title != http.StatusText(tt.status) || p.Status != tt.status || p.Code != tt.code {
				t.Errorf("body = %s, want type about:blank, title %q, status %d, code %s", rec.Body.String(), http.StatusText(tt.status), tt.status, tt.code)
			}
		})
	}
}
