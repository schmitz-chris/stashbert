package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// newApp returns the handler from app.NewHandler with events.Nop, its
// migrated database in t.TempDir() and an empty image directory in
// t.TempDir().
func newApp(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	return newAppWithPublisher(t, events.Nop{})
}

// newAppWithPublisher is like newApp but publishes the domain events to pub.
// Unknown barcodes are looked up with lookup.NewDisabledClient.
func newAppWithPublisher(t *testing.T, pub events.Publisher) (http.Handler, *sql.DB) {
	t.Helper()
	return newAppWithLookuper(t, pub, lookup.NewDisabledClient())
}

// newAppWithLookuper is like newAppWithPublisher but looks up unknown
// barcodes with l.
func newAppWithLookuper(t *testing.T, pub events.Publisher, l domain.Lookuper) (http.Handler, *sql.DB) {
	t.Helper()
	return newAppWithDeps(t, app.Deps{Publisher: pub, Lookuper: l, ImageDir: t.TempDir()})
}

// newAppWithImages is like newAppWithPublisher but uses imageDir as the
// directory of the product images.
func newAppWithImages(t *testing.T, pub events.Publisher, imageDir string) (http.Handler, *sql.DB) {
	t.Helper()
	return newAppWithDeps(t, app.Deps{Publisher: pub, Lookuper: lookup.NewDisabledClient(), ImageDir: imageDir})
}

// newDB returns a migrated database in t.TempDir().
func newDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "stashbert.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(ctx, db, store.Migrations); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	return db
}

// newAppWithDeps returns the handler from app.NewHandler with d and its
// database, a new one from newDB if d.DB is nil. It sets Version of d, and
// Logger to a discarding logger if d.Logger is nil.
func newAppWithDeps(t *testing.T, d app.Deps) (http.Handler, *sql.DB) {
	t.Helper()
	if d.DB == nil {
		d.DB = newDB(t)
	}
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	d.Version = "dev"
	h, err := app.NewHandler(cfg, d)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h, d.DB
}

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	h, _ := newApp(t)
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
		{http.MethodGet, "/api/v1/setup", http.StatusNotFound, "not_found"},
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
