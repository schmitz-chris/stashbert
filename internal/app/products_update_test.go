package app_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
)

// patchProduct sends a PATCH request with the JSON body for the product id to h.
func patchProduct(h http.Handler, id, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/products/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

// deleteProduct sends a DELETE request for the product id to h.
func deleteProduct(h http.Handler, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/products/"+id, nil))
	return rec
}

// decodeProduct asserts that rec is a 200 response with JSON and returns the decoded product.
func decodeProduct(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var p map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body.String(), err)
	}
	return p
}

// checkPatch sends the patch body for the product id to h. It asserts that the
// response and the stored product equal the product before the patch with the
// fields of changes (a JSON object) and updated_at set to the time of the request.
func checkPatch(t *testing.T, h http.Handler, id, body, changes string) {
	t.Helper()
	want := decodeProduct(t, get(h, "/api/v1/products/"+id))
	var c map[string]any
	if err := json.Unmarshal([]byte(changes), &c); err != nil {
		t.Fatalf("decode changes: %v", err)
	}
	maps.Copy(want, c)
	start := time.Now().UTC().Truncate(time.Millisecond)

	got := decodeProduct(t, patchProduct(h, id, body))

	s, _ := got["updated_at"].(string)
	updatedAt, err := time.Parse(time.RFC3339, s)
	if err != nil || updatedAt.Before(start) || updatedAt.After(time.Now()) {
		t.Errorf("updated_at = %v, want the time of the request", got["updated_at"])
	}
	stored := decodeProduct(t, get(h, "/api/v1/products/"+id))
	if !reflect.DeepEqual(stored, got) {
		t.Errorf("stored product = %v\nwant response %v", stored, got)
	}
	delete(got, "updated_at")
	delete(want, "updated_at")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("product = %v\n     want %v", got, want)
	}
}

// checkShoppingChanged asserts that recorded is empty if want is nil and
// otherwise exactly one shopping.changed event with the data want.
func checkShoppingChanged(t *testing.T, recorded []events.Event, want *events.ShoppingChangedData) {
	t.Helper()
	if want == nil {
		if len(recorded) != 0 {
			t.Errorf("events = %+v, want none", recorded)
		}
		return
	}
	if len(recorded) != 1 {
		t.Fatalf("events = %+v, want one event", recorded)
	}
	e := recorded[0]
	if e.Type != events.TypeShoppingChanged || e.Source != events.Source {
		t.Errorf("event type = %q, source = %q, want %q and %q", e.Type, e.Source, events.TypeShoppingChanged, events.Source)
	}
	if data, ok := e.Data.(events.ShoppingChangedData); !ok || data != *want {
		t.Errorf("event data = %#v, want %#v", e.Data, *want)
	}
}

func TestUpdateProductNameOnly(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	insertProducts(t, db)

	// p2 needs review; all other fields stay as they are.
	checkPatch(t, h, "p2", `{"name": "  Rote Bohnen  "}`, `{"name": "Rote Bohnen", "needs_review": false}`)

	checkShoppingChanged(t, recorder.Events(), nil)
}

func TestUpdateProductTargetOnly(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	insertProducts(t, db)

	// p2 has stock 2, target 5, no min_stock and needs review, which stays true.
	checkPatch(t, h, "p2", `{"target": 8}`, `{"target": 8, "missing": 6}`)

	checkShoppingChanged(t, recorder.Events(), &events.ShoppingChangedData{
		ProductID: "p2", Name: "kidneybohnen", MissingBefore: 3, MissingAfter: 6,
	})
}

func TestUpdateProductFields(t *testing.T) {
	// See insertProducts: p2 has stock 2, target 5, no min_stock (missing 3) and
	// needs review; p4 has stock 2, target 4, min_stock 1 (missing 0).
	tests := []struct {
		name     string
		id       string
		body     string
		changes  string
		shopping *events.ShoppingChangedData
	}{
		{"brand null", "p2", `{"brand": null}`, `{"brand": null, "needs_review": false}`, nil},
		{"brand blank", "p2", `{"brand": "  "}`, `{"brand": null, "needs_review": false}`, nil},
		{"brand trimmed", "p2", `{"brand": " Kaiser "}`, `{"brand": "Kaiser", "needs_review": false}`, nil},
		{"package_size null", "p2", `{"package_size": null}`, `{"package_size": null, "needs_review": false}`, nil},
		{"note null", "p2", `{"note": null}`, `{"note": null}`, nil},
		{"note trimmed", "p2", `{"note": " Im Regal "}`, `{"note": "Im Regal"}`, nil},
		{"min_stock set", "p2", `{"min_stock": 2}`, `{"min_stock": 2, "missing": 0}`,
			&events.ShoppingChangedData{ProductID: "p2", Name: "kidneybohnen", MissingBefore: 3, MissingAfter: 0}},
		{"min_stock null", "p4", `{"min_stock": null}`, `{"min_stock": null, "missing": 2}`,
			&events.ShoppingChangedData{ProductID: "p4", Name: "Mehl", MissingBefore: 0, MissingAfter: 2}},
		{"target and min_stock", "p2", `{"target": 6, "min_stock": 6}`, `{"target": 6, "min_stock": 6, "missing": 4}`,
			&events.ShoppingChangedData{ProductID: "p2", Name: "kidneybohnen", MissingBefore: 3, MissingAfter: 4}},
		{"target without change of missing", "p4", `{"target": 5}`, `{"target": 5}`, nil},
		{"event with new name", "p2", `{"name": "Rote Bohnen", "target": 8}`, `{"name": "Rote Bohnen", "target": 8, "missing": 6, "needs_review": false}`,
			&events.ShoppingChangedData{ProductID: "p2", Name: "Rote Bohnen", MissingBefore: 3, MissingAfter: 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)

			checkPatch(t, h, tt.id, tt.body, tt.changes)

			checkShoppingChanged(t, recorder.Events(), tt.shopping)
		})
	}
}

func TestUpdateProductErrors(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	insertProducts(t, db)
	before := get(h, "/api/v1/products").Body.String()

	tests := []struct {
		name   string
		id     string
		body   string
		status int
		code   string
	}{
		{"unknown id", "unbekannt", `{"name": "Mehl"}`, http.StatusNotFound, "not_found"},
		{"name blank", "p2", `{"name": "   "}`, http.StatusBadRequest, "invalid_request"},
		{"name null", "p2", `{"name": null}`, http.StatusBadRequest, "invalid_request"},
		{"name too long", "p2", `{"name": "` + strings.Repeat("a", 121) + `"}`, http.StatusBadRequest, "invalid_request"},
		{"brand too long", "p2", `{"brand": "` + strings.Repeat("b", 121) + `"}`, http.StatusBadRequest, "invalid_request"},
		{"package_size too long", "p2", `{"package_size": "` + strings.Repeat("p", 41) + `"}`, http.StatusBadRequest, "invalid_request"},
		{"note too long", "p2", `{"note": "` + strings.Repeat("n", 501) + `"}`, http.StatusBadRequest, "invalid_request"},
		{"min_stock greater than stored target", "p2", `{"min_stock": 6}`, http.StatusBadRequest, "invalid_request"},
		{"target less than stored min_stock", "p4", `{"target": 0}`, http.StatusBadRequest, "invalid_request"},
		{"min_stock greater than new target", "p2", `{"target": 3, "min_stock": 4}`, http.StatusBadRequest, "invalid_request"},
		{"negative target", "p2", `{"target": -1}`, http.StatusBadRequest, "invalid_request"},
		{"target null", "p2", `{"target": null}`, http.StatusBadRequest, "invalid_request"},
		{"negative min_stock", "p2", `{"min_stock": -1}`, http.StatusBadRequest, "invalid_request"},
		{"not an object", "p2", `[]`, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := patchProduct(h, tt.id, tt.body)

			checkProblemCode(t, rec, tt.status, tt.code)
			// Nothing is changed and no event is published.
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant %s", after, before)
			}
			if n := len(recorder.Events()); n != 0 {
				t.Errorf("events = %d, want 0", n)
			}
		})
	}
}

func TestDeleteProduct(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	// m3 reverses m2, both belong to p2 like m1. m4 belongs to p4.
	if _, err := db.ExecContext(ctx, `INSERT INTO movements (id, product_id, kind, delta, stock_after,
		barcode, reverses_id, idempotency_key, request_hash, created_at) VALUES
		('m1', 'p2', 'add', 3, 3, '4001686301265', NULL, NULL, NULL, '2026-09-23T12:00:00.000Z'),
		('m2', 'p2', 'consume', -1, 2, NULL, NULL, NULL, NULL, '2026-09-23T12:01:00.000Z'),
		('m3', 'p2', 'reversal', 1, 3, NULL, 'm2', NULL, NULL, '2026-09-23T12:02:00.000Z'),
		('m4', 'p4', 'add', 2, 2, NULL, NULL, NULL, NULL, '2026-09-23T12:03:00.000Z')`); err != nil {
		t.Fatalf("insert movements: %v", err)
	}

	rec := deleteProduct(h, "p2")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
	checkProblemCode(t, get(h, "/api/v1/products/p2"), http.StatusNotFound, "not_found")
	checkProblemCode(t, deleteProduct(h, "p2"), http.StatusNotFound, "not_found")
	// Only the barcode and the movement of p4 are left.
	for table, want := range map[string]int{"products": 3, "barcodes": 1, "movements": 1} {
		if n := countRows(t, db, table); n != want {
			t.Errorf("%s = %d, want %d", table, n, want)
		}
	}
	var productID string
	if err := db.QueryRowContext(ctx, "SELECT product_id FROM barcodes JOIN movements USING (product_id)").Scan(&productID); err != nil || productID != "p4" {
		t.Errorf("remaining barcode and movement belong to %q (%v), want p4", productID, err)
	}
}

func TestDeleteProductUnknownID(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)

	rec := deleteProduct(h, "unbekannt")

	checkProblemCode(t, rec, http.StatusNotFound, "not_found")
	if n := countRows(t, db, "products"); n != 4 {
		t.Errorf("products = %d, want 4", n)
	}
}
