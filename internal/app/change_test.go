package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
)

// OnChange is called after every successful changing API request, also one
// without an event, and not after reading or failed requests
// (architecture.md, 11.5).
func TestOnChange(t *testing.T) {
	var calls atomic.Int64
	h, db := newAppWithDeps(t, app.Deps{
		Publisher: events.Nop{}, Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(),
		OnChange: func() { calls.Add(1) },
	})
	// See insertProducts: p1 "Zucker", p2 "kidneybohnen", p4 "Mehl".
	insertProducts(t, db)

	tests := []struct {
		name, method, path, body string
		status                   int
		calls                    int64
	}{
		{"GET", http.MethodGet, "/api/v1/products", "", http.StatusOK, 0},
		{"HEAD", http.MethodHead, "/api/v1/openapi.yaml", "", http.StatusOK, 0},
		{"rename", http.MethodPatch, "/api/v1/products/p1", `{"name": "Rohrzucker"}`, http.StatusOK, 1},
		{"unknown product", http.MethodPatch, "/api/v1/products/unknown", `{"name": "Rohrzucker"}`, http.StatusNotFound, 0},
		{"invalid body", http.MethodPatch, "/api/v1/products/p1", `{`, http.StatusBadRequest, 0},
		{"booking", http.MethodPost, "/api/v1/movements", `{"product_id": "p2", "kind": "add", "quantity": 1}`, http.StatusCreated, 1},
		{"snapshot without MQTT", http.MethodPost, "/api/v1/shopping-list/snapshot", "", http.StatusConflict, 0},
		{"delete", http.MethodDelete, "/api/v1/products/p4", "", http.StatusNoContent, 1},
		{"web UI", http.MethodPost, "/vorrat", "", http.StatusMethodNotAllowed, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := calls.Load()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tt.status, rec.Body.String())
			}
			if got := calls.Load() - before; got != tt.calls {
				t.Errorf("OnChange called %d times, want %d", got, tt.calls)
			}
		})
	}
}
