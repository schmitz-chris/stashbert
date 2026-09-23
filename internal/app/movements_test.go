package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// setStock sets the stock of the product id with direct SQL.
func setStock(t *testing.T, db *sql.DB, id string, stock int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "UPDATE products SET stock = ? WHERE id = ?", stock, id); err != nil {
		t.Fatalf("set stock of %s: %v", id, err)
	}
}

// decodeMovementResult asserts that rec is a 201 response with JSON and
// without Location, and returns the decoded MovementResult and its movement.
func decodeMovementResult(t *testing.T, rec *httptest.ResponseRecorder) (result, movement map[string]any) {
	t.Helper()
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("Location = %q, want none", loc)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body.String(), err)
	}
	movement, ok := result["movement"].(map[string]any)
	if !ok {
		t.Fatalf("movement = %v, want an object", result["movement"])
	}
	return result, movement
}

// checkStoredMovement asserts that the only stored movement equals movement
// from a response and has no barcode, reversal, idempotency key or hash.
func checkStoredMovement(t *testing.T, db *sql.DB, movement map[string]any) {
	t.Helper()
	if n := countRows(t, db, "movements"); n != 1 {
		t.Fatalf("movements = %d, want 1", n)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var (
		id, productID, kind, createdAt string
		delta, stockAfter              int
		barcode, reversesID, key, hash *string
	)
	if err := db.QueryRowContext(ctx, `SELECT id, product_id, kind, delta, stock_after, barcode,
		reverses_id, idempotency_key, request_hash, created_at FROM movements`).Scan(
		&id, &productID, &kind, &delta, &stockAfter, &barcode, &reversesID, &key, &hash, &createdAt); err != nil {
		t.Fatalf("read movement: %v", err)
	}
	stored, err := time.Parse(store.TimeLayout, createdAt)
	if err != nil {
		t.Errorf("stored created_at = %q, want layout %s", createdAt, store.TimeLayout)
	}
	s, _ := movement["created_at"].(string)
	if responded, err := time.Parse(time.RFC3339, s); err != nil || !responded.Equal(stored) {
		t.Errorf("created_at = %v, want stored %s", movement["created_at"], createdAt)
	}
	got := fmt.Sprintf("%s %s %s %d %d", id, productID, kind, delta, stockAfter)
	want := fmt.Sprintf("%v %v %v %v %v", movement["id"], movement["product_id"], movement["kind"], movement["delta"], movement["stock_after"])
	if got != want {
		t.Errorf("stored movement = %s, want %s", got, want)
	}
	if barcode != nil || reversesID != nil || key != nil || hash != nil {
		t.Errorf("barcode, reverses_id, idempotency_key, request_hash = %v, %v, %v, %v, want NULL", barcode, reversesID, key, hash)
	}
}

func TestCreateMovement(t *testing.T) {
	// See insertProducts: p2 is "kidneybohnen" with target 5 and no min_stock,
	// so missing is 5 - stock below 5. p3 is "Kidneybohnen" with target 4 and
	// min_stock 1.
	tests := []struct {
		name       string
		id         string
		stock      int
		body       string
		kind       string
		delta      int
		stockAfter int
		missing    int
		warnings   string
		message    string
	}{
		{"add", "p2", 2, `{"product_id": "p2", "kind": "add", "quantity": 3}`,
			"add", 3, 5, 0, `[]`, "kidneybohnen 2 → 5"},
		{"add with default quantity", "p2", 2, `{"product_id": "p2", "kind": "add"}`,
			"add", 1, 3, 2, `[]`, "kidneybohnen 2 → 3"},
		{"add on stock 0", "p2", 0, `{"product_id": "p2", "kind": "add", "quantity": 2}`,
			"add", 2, 2, 3, `[]`, "kidneybohnen 0 → 2"},
		{"consume", "p2", 2, `{"product_id": "p2", "kind": "consume", "quantity": 1}`,
			"consume", -1, 1, 4, `[]`, "kidneybohnen 2 → 1"},
		{"consume with default quantity like in architecture", "p3", 3, `{"product_id": "p3", "kind": "consume"}`,
			"consume", -1, 2, 0, `[]`, "Kidneybohnen 3 → 2"},
		{"consume all", "p2", 2, `{"product_id": "p2", "kind": "consume", "quantity": 2}`,
			"consume", -2, 0, 5, `[]`, "kidneybohnen 2 → 0"},
		{"consume more than stock", "p2", 1, `{"product_id": "p2", "kind": "consume", "quantity": 3}`,
			"consume", -1, 0, 5, `["clamped_to_zero"]`, "kidneybohnen 1 → 0"},
		{"inventory higher", "p2", 2, `{"product_id": "p2", "kind": "inventory", "stock": 7}`,
			"inventory", 5, 7, 0, `[]`, "kidneybohnen 2 → 7"},
		{"inventory lower", "p2", 2, `{"product_id": "p2", "kind": "inventory", "stock": 0}`,
			"inventory", -2, 0, 5, `[]`, "kidneybohnen 2 → 0"},
		{"inventory unchanged", "p2", 2, `{"product_id": "p2", "kind": "inventory", "stock": 2}`,
			"inventory", 0, 2, 3, `[]`, "kidneybohnen 2 → 2"},
		{"inventory ignores quantity", "p2", 2, `{"product_id": "p2", "kind": "inventory", "stock": 4, "quantity": 9}`,
			"inventory", 2, 4, 1, `[]`, "kidneybohnen 2 → 4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)
			setStock(t, db, tt.id, tt.stock)
			want := decodeProduct(t, get(h, "/api/v1/products/"+tt.id))
			start := time.Now().UTC().Truncate(time.Millisecond)

			rec := post(h, "/api/v1/movements", tt.body)

			result, movement := decodeMovementResult(t, rec)
			checkFields(t, movement, fmt.Sprintf(`{"product_id": %q, "kind": %q, "delta": %d, "stock_after": %d,
				"barcode": null, "reverses_id": null}`, tt.id, tt.kind, tt.delta, tt.stockAfter))
			checkFields(t, result, fmt.Sprintf(`{"product_created": false, "warnings": %s, "message": %q}`, tt.warnings, tt.message))
			id, _ := movement["id"].(string)
			if u, err := uuid.Parse(id); err != nil || u.Version() != 7 || id != strings.ToLower(id) {
				t.Errorf("movement id = %q, want UUIDv7 in lower case", id)
			}
			s, _ := movement["created_at"].(string)
			createdAt, err := time.Parse(time.RFC3339, s)
			if err != nil || createdAt.Before(start) || createdAt.After(time.Now()) {
				t.Errorf("created_at = %v, want the time of the request", movement["created_at"])
			}
			checkStoredMovement(t, db, movement)

			// The product in the response is stored; only stock, missing and
			// updated_at have changed.
			product, _ := result["product"].(map[string]any)
			if stored := decodeProduct(t, get(h, "/api/v1/products/"+tt.id)); !reflect.DeepEqual(stored, product) {
				t.Errorf("stored product = %v\nwant response %v", stored, product)
			}
			if product["updated_at"] != movement["created_at"] {
				t.Errorf("product updated_at = %v, want created_at of the movement %v", product["updated_at"], movement["created_at"])
			}
			want["stock"] = float64(tt.stockAfter)
			want["missing"] = float64(tt.missing)
			delete(want, "updated_at")
			delete(product, "updated_at")
			if !reflect.DeepEqual(product, want) {
				t.Errorf("product = %v\n     want %v", product, want)
			}
		})
	}
}

func TestCreateMovementSequence(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)

	// p3 "Kidneybohnen" starts with stock 0.
	steps := []struct {
		body    string
		message string
	}{
		{`{"product_id": "p3", "kind": "add", "quantity": 3}`, "Kidneybohnen 0 → 3"},
		{`{"product_id": "p3", "kind": "consume"}`, "Kidneybohnen 3 → 2"},
		{`{"product_id": "p3", "kind": "inventory", "stock": 1}`, "Kidneybohnen 2 → 1"},
		{`{"product_id": "p3", "kind": "consume", "quantity": 3}`, "Kidneybohnen 1 → 0"},
	}
	for _, s := range steps {
		result, _ := decodeMovementResult(t, post(h, "/api/v1/movements", s.body))
		if result["message"] != s.message {
			t.Errorf("%s: message = %v, want %q", s.body, result["message"], s.message)
		}
	}
	checkProblemCode(t, post(h, "/api/v1/movements", `{"product_id": "p3", "kind": "consume"}`),
		http.StatusConflict, "stock_already_zero")
	if n := countRows(t, db, "movements"); n != len(steps) {
		t.Errorf("movements = %d, want %d", n, len(steps))
	}
}

func TestCreateMovementPublishesEvents(t *testing.T) {
	// See insertProducts: p1 "Zucker" has target 0, so missing is always 0.
	// p2 "kidneybohnen" has target 5 and no min_stock, so missing is 5 - stock
	// below 5. p4 "Mehl" has target 4 and min_stock 1, so missing is 4 - stock
	// below 1.
	type event struct {
		typ  string
		data any
	}
	// The movement id of events.StockData is set from the response.
	stock := func(typ, id string, delta, stockAfter int64) event {
		return event{typ, events.StockData{ProductID: id, Delta: delta, StockAfter: stockAfter}}
	}
	empty := func(id, name string) event {
		return event{events.TypeProductEmpty, events.ProductEmptyData{ProductID: id, Name: name}}
	}
	shopping := func(id, name string, before, after int64) event {
		return event{events.TypeShoppingChanged, events.ShoppingChangedData{
			ProductID: id, Name: name, MissingBefore: before, MissingAfter: after,
		}}
	}
	tests := []struct {
		name  string
		id    string
		stock int
		body  string
		want  []event
	}{
		{"consume to 0", "p2", 2, `{"product_id": "p2", "kind": "consume", "quantity": 2}`, []event{
			stock(events.TypeStockConsumed, "p2", -2, 0),
			empty("p2", "kidneybohnen"),
			shopping("p2", "kidneybohnen", 3, 5),
		}},
		{"consume clamped to 0", "p2", 1, `{"product_id": "p2", "kind": "consume", "quantity": 3}`, []event{
			stock(events.TypeStockConsumed, "p2", -1, 0),
			empty("p2", "kidneybohnen"),
			shopping("p2", "kidneybohnen", 4, 5),
		}},
		{"consume to 0 with target 0", "p1", 2, `{"product_id": "p1", "kind": "consume", "quantity": 2}`, []event{
			stock(events.TypeStockConsumed, "p1", -2, 0),
			empty("p1", "Zucker"),
		}},
		{"consume changing missing", "p2", 2, `{"product_id": "p2", "kind": "consume"}`, []event{
			stock(events.TypeStockConsumed, "p2", -1, 1),
			shopping("p2", "kidneybohnen", 3, 4),
		}},
		{"consume without change of missing", "p4", 2, `{"product_id": "p4", "kind": "consume"}`, []event{
			stock(events.TypeStockConsumed, "p4", -1, 1),
		}},
		{"add changing missing", "p2", 2, `{"product_id": "p2", "kind": "add", "quantity": 3}`, []event{
			stock(events.TypeStockAdded, "p2", 3, 5),
			shopping("p2", "kidneybohnen", 3, 0),
		}},
		{"add without change of missing", "p4", 2, `{"product_id": "p4", "kind": "add"}`, []event{
			stock(events.TypeStockAdded, "p4", 1, 3),
		}},
		{"inventory higher", "p2", 2, `{"product_id": "p2", "kind": "inventory", "stock": 7}`, []event{
			stock(events.TypeStockAdjusted, "p2", 5, 7),
			shopping("p2", "kidneybohnen", 3, 0),
		}},
		{"inventory to 0", "p4", 2, `{"product_id": "p4", "kind": "inventory", "stock": 0}`, []event{
			stock(events.TypeStockAdjusted, "p4", -2, 0),
			empty("p4", "Mehl"),
			shopping("p4", "Mehl", 0, 4),
		}},
		{"inventory unchanged", "p2", 2, `{"product_id": "p2", "kind": "inventory", "stock": 2}`, nil},
		{"inventory unchanged at 0", "p2", 0, `{"product_id": "p2", "kind": "inventory", "stock": 0}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)
			setStock(t, db, tt.id, tt.stock)

			_, movement := decodeMovementResult(t, post(h, "/api/v1/movements", tt.body))

			recorded := recorder.Events()
			if len(recorded) != len(tt.want) {
				t.Fatalf("events = %+v, want %d events", recorded, len(tt.want))
			}
			for i, w := range tt.want {
				if data, ok := w.data.(events.StockData); ok {
					data.MovementID, _ = movement["id"].(string)
					w.data = data
				}
				e := recorded[i]
				if e.Type != w.typ || e.Source != events.Source {
					t.Errorf("event %d: type = %q, source = %q, want %q and %q", i, e.Type, e.Source, w.typ, events.Source)
				}
				if e.Data != w.data {
					t.Errorf("event %d: data = %#v, want %#v", i, e.Data, w.data)
				}
			}
		})
	}
}

func TestCreateMovementErrors(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	insertProducts(t, db)
	before := get(h, "/api/v1/products").Body.String()

	// See insertProducts: p1 and p3 have stock 0, p2 has stock 2.
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"consume on stock 0", `{"product_id": "p3", "kind": "consume"}`, http.StatusConflict, "stock_already_zero"},
		{"consume more on stock 0", `{"product_id": "p1", "kind": "consume", "quantity": 3}`, http.StatusConflict, "stock_already_zero"},
		{"unknown product", `{"product_id": "unbekannt", "kind": "add"}`, http.StatusNotFound, "not_found"},
		{"unknown product with inventory", `{"product_id": "unbekannt", "kind": "inventory", "stock": 1}`, http.StatusNotFound, "not_found"},
		{"inventory without stock", `{"product_id": "p2", "kind": "inventory"}`, http.StatusBadRequest, "invalid_request"},
		{"add with stock", `{"product_id": "p2", "kind": "add", "stock": 3}`, http.StatusBadRequest, "invalid_request"},
		{"consume with stock", `{"product_id": "p2", "kind": "consume", "stock": 1}`, http.StatusBadRequest, "invalid_request"},
		{"quantity 0", `{"product_id": "p2", "kind": "add", "quantity": 0}`, http.StatusBadRequest, "invalid_request"},
		{"negative stock", `{"product_id": "p2", "kind": "inventory", "stock": -1}`, http.StatusBadRequest, "invalid_request"},
		{"quantity above maximum", `{"product_id": "p2", "kind": "add", "quantity": 1001}`, http.StatusBadRequest, "invalid_request"},
		{"stock above maximum", `{"product_id": "p2", "kind": "inventory", "stock": 100001}`, http.StatusBadRequest, "invalid_request"},
		{"quantity overflowing int64", `{"product_id": "p2", "kind": "add", "quantity": 9223372036854775807}`, http.StatusBadRequest, "invalid_request"},
		{"kind reversal", `{"product_id": "p2", "kind": "reversal"}`, http.StatusBadRequest, "invalid_request"},
		{"kind missing", `{"product_id": "p2"}`, http.StatusBadRequest, "invalid_request"},
		{"product_id missing", `{"kind": "add"}`, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(h, "/api/v1/movements", tt.body)

			checkProblemCode(t, rec, tt.status, tt.code)
			// Nothing is booked, no product is changed and no event is published.
			if n := countRows(t, db, "movements"); n != 0 {
				t.Errorf("movements = %d, want 0", n)
			}
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant %s", after, before)
			}
			if n := len(recorder.Events()); n != 0 {
				t.Errorf("events = %d, want 0", n)
			}
		})
	}
}
