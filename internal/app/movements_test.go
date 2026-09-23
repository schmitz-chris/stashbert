package app_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
// from a response, including its barcode, and has no reversal, idempotency
// key or hash.
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
	var storedBarcode any
	if barcode != nil {
		storedBarcode = *barcode
	}
	if storedBarcode != movement["barcode"] {
		t.Errorf("stored barcode = %v, want %v", storedBarcode, movement["barcode"])
	}
	if reversesID != nil || key != nil || hash != nil {
		t.Errorf("reverses_id, idempotency_key, request_hash = %v, %v, %v, want NULL", reversesID, key, hash)
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

func TestCreateMovementByBarcode(t *testing.T) {
	// See insertProducts: p2 "kidneybohnen" has the barcodes 4001686301265
	// with 1 unit and 0034000470693 with 6 units, p4 "Mehl" has 3017620422003
	// with 1 unit.
	tests := []struct {
		name       string
		id         string
		stock      int
		body       string
		kind       string
		delta      int
		stockAfter int
		barcode    string
		warnings   string
		message    string
	}{
		{"add with 6 units", "p2", 2, `{"barcode": "0034000470693", "kind": "add"}`,
			"add", 6, 8, "0034000470693", `[]`, "kidneybohnen 2 → 8"},
		{"add quantity 2 with 6 units", "p2", 2, `{"barcode": "0034000470693", "kind": "add", "quantity": 2}`,
			"add", 12, 14, "0034000470693", `[]`, "kidneybohnen 2 → 14"},
		{"add with 1 unit", "p2", 2, `{"barcode": "4001686301265", "kind": "add", "quantity": 3}`,
			"add", 3, 5, "4001686301265", `[]`, "kidneybohnen 2 → 5"},
		{"consume with 6 units", "p2", 10, `{"barcode": "0034000470693", "kind": "consume"}`,
			"consume", -6, 4, "0034000470693", `[]`, "kidneybohnen 10 → 4"},
		{"consume with 1 unit", "p2", 2, `{"barcode": "4001686301265", "kind": "consume"}`,
			"consume", -1, 1, "4001686301265", `[]`, "kidneybohnen 2 → 1"},
		{"consume with 6 units more than stock", "p2", 2, `{"barcode": "0034000470693", "kind": "consume"}`,
			"consume", -2, 0, "0034000470693", `["clamped_to_zero"]`, "kidneybohnen 2 → 0"},
		{"inventory ignores units", "p2", 2, `{"barcode": "0034000470693", "kind": "inventory", "stock": 3}`,
			"inventory", 1, 3, "0034000470693", `[]`, "kidneybohnen 2 → 3"},
		{"inventory ignores units and quantity", "p2", 2, `{"barcode": "0034000470693", "kind": "inventory", "stock": 1, "quantity": 4}`,
			"inventory", -1, 1, "0034000470693", `[]`, "kidneybohnen 2 → 1"},
		{"inventory unchanged", "p2", 2, `{"barcode": "0034000470693", "kind": "inventory", "stock": 2}`,
			"inventory", 0, 2, "0034000470693", `[]`, "kidneybohnen 2 → 2"},
		{"UPC-A is normalized", "p2", 2, `{"barcode": "034000470693", "kind": "add"}`,
			"add", 6, 8, "0034000470693", `[]`, "kidneybohnen 2 → 8"},
		{"GTIN-14 with leading 0 is normalized", "p2", 10, `{"barcode": "00034000470693", "kind": "consume"}`,
			"consume", -6, 4, "0034000470693", `[]`, "kidneybohnen 10 → 4"},
		{"barcode of another product", "p4", 2, `{"barcode": "3017620422003", "kind": "add"}`,
			"add", 1, 3, "3017620422003", `[]`, "Mehl 2 → 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)
			setStock(t, db, tt.id, tt.stock)

			rec := post(h, "/api/v1/movements", tt.body)

			result, movement := decodeMovementResult(t, rec)
			checkFields(t, movement, fmt.Sprintf(`{"product_id": %q, "kind": %q, "delta": %d, "stock_after": %d,
				"barcode": %q, "reverses_id": null}`, tt.id, tt.kind, tt.delta, tt.stockAfter, tt.barcode))
			checkFields(t, result, fmt.Sprintf(`{"product_created": false, "warnings": %s, "message": %q}`, tt.warnings, tt.message))
			checkStoredMovement(t, db, movement)

			product, _ := result["product"].(map[string]any)
			checkFields(t, product, fmt.Sprintf(`{"id": %q, "stock": %d}`, tt.id, tt.stockAfter))
			if stored := decodeProduct(t, get(h, "/api/v1/products/"+tt.id)); !reflect.DeepEqual(stored, product) {
				t.Errorf("stored product = %v\nwant response %v", stored, product)
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
		// Movements by barcode publish the same events; 0034000470693 has 6 units.
		{"consume by barcode to 0", "p2", 2, `{"barcode": "0034000470693", "kind": "consume"}`, []event{
			stock(events.TypeStockConsumed, "p2", -2, 0),
			empty("p2", "kidneybohnen"),
			shopping("p2", "kidneybohnen", 3, 5),
		}},
		{"add by barcode", "p2", 2, `{"barcode": "0034000470693", "kind": "add"}`, []event{
			stock(events.TypeStockAdded, "p2", 6, 8),
			shopping("p2", "kidneybohnen", 3, 0),
		}},
		{"inventory by barcode", "p4", 2, `{"barcode": "3017620422003", "kind": "inventory", "stock": 5}`, []event{
			stock(events.TypeStockAdjusted, "p4", 3, 5),
		}},
		{"inventory by barcode unchanged", "p2", 2, `{"barcode": "0034000470693", "kind": "inventory", "stock": 2}`, nil},
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
		{"neither product_id nor barcode", `{"kind": "add"}`, http.StatusBadRequest, "invalid_request"},
		{"neither product_id nor barcode with inventory", `{"kind": "inventory", "stock": 1}`, http.StatusBadRequest, "invalid_request"},
		{"product_id and barcode", `{"product_id": "p2", "barcode": "4001686301265", "kind": "add"}`, http.StatusBadRequest, "invalid_request"},
		{"product_id and invalid barcode", `{"product_id": "p2", "barcode": "123", "kind": "add"}`, http.StatusBadRequest, "invalid_request"},
		{"barcode null", `{"barcode": null, "kind": "add"}`, http.StatusBadRequest, "invalid_request"},
		{"barcode with inventory without stock", `{"barcode": "4001686301265", "kind": "inventory"}`, http.StatusBadRequest, "invalid_request"},
		{"barcode with add with stock", `{"barcode": "4001686301265", "kind": "add", "stock": 3}`, http.StatusBadRequest, "invalid_request"},
		{"invalid barcode check digit", `{"barcode": "4001686301266", "kind": "add"}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"invalid barcode length", `{"barcode": "40016863012", "kind": "consume"}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"invalid barcode letters", `{"barcode": "400168630126A", "kind": "inventory", "stock": 1}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"empty barcode", `{"barcode": "", "kind": "add"}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"unknown barcode with add", `{"barcode": "4006381333931", "kind": "add"}`, http.StatusNotFound, "unknown_barcode"},
		{"unknown barcode with consume", `{"barcode": "4006381333931", "kind": "consume"}`, http.StatusNotFound, "unknown_barcode"},
		{"unknown barcode with inventory", `{"barcode": "4006381333931", "kind": "inventory", "stock": 1}`, http.StatusNotFound, "unknown_barcode"},
		{"unknown barcode as UPC-A", `{"barcode": "036000291452", "kind": "add"}`, http.StatusNotFound, "unknown_barcode"},
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

// postWithKey is like post but sends the header Idempotency-Key with key.
func postWithKey(h http.Handler, path, body, key string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	h.ServeHTTP(rec, req)
	return rec
}

// storedIdempotency returns idempotency_key and request_hash of the stored
// movement id.
func storedIdempotency(t *testing.T, db *sql.DB, id string) (key, hash sql.NullString) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := db.QueryRowContext(ctx, "SELECT idempotency_key, request_hash FROM movements WHERE id = ?", id).Scan(
		&key, &hash); err != nil {
		t.Fatalf("read movement %s: %v", id, err)
	}
	return key, hash
}

// idempotencyKey is a key as a client would send it.
const idempotencyKey = "0199a5c4-7b1e-7c3a-9f00-3d2b1a0c9e11"

func TestCreateMovementIdempotencyKeyRepeats(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	insertProducts(t, db)

	// See insertProducts: p2 "kidneybohnen" has stock 2, so consume 3 is clamped to 0.
	first, firstMovement := decodeMovementResult(t, postWithKey(h, "/api/v1/movements",
		`{"product_id": "p2", "kind": "consume", "quantity": 3}`, idempotencyKey))
	checkFields(t, first, `{"product_created": false, "warnings": ["clamped_to_zero"], "message": "kidneybohnen 2 → 0"}`)
	published := len(recorder.Events())
	if published == 0 {
		t.Fatal("events = 0, want the events of the first request")
	}

	// The same body with another order and spacing of the fields.
	second, secondMovement := decodeMovementResult(t, postWithKey(h, "/api/v1/movements",
		`{"quantity":3,"kind":"consume","product_id":"p2"}`, idempotencyKey))

	if !reflect.DeepEqual(secondMovement, firstMovement) {
		t.Errorf("movement = %v\n    want %v", secondMovement, firstMovement)
	}
	checkFields(t, second, `{"product_created": false, "warnings": [], "message": "kidneybohnen 2 → 0"}`)
	product, _ := second["product"].(map[string]any)
	if stored := decodeProduct(t, get(h, "/api/v1/products/p2")); !reflect.DeepEqual(product, stored) {
		t.Errorf("product = %v\nwant stored %v", product, stored)
	}
	if !reflect.DeepEqual(product, first["product"]) {
		t.Errorf("product = %v\nwant unchanged %v", product, first["product"])
	}
	if n := countRows(t, db, "movements"); n != 1 {
		t.Errorf("movements = %d, want 1", n)
	}
	if n := len(recorder.Events()); n != published {
		t.Errorf("events = %d, want only the %d of the first request", n, published)
	}

	// The hash is taken from json.Marshal of the decoded body.
	id, _ := firstMovement["id"].(string)
	key, hash := storedIdempotency(t, db, id)
	sum := sha256.Sum256([]byte(`{"kind":"consume","product_id":"p2","quantity":3}`))
	if want := (sql.NullString{String: idempotencyKey, Valid: true}); key != want {
		t.Errorf("idempotency_key = %v, want %v", key, want)
	}
	if want := (sql.NullString{String: hex.EncodeToString(sum[:]), Valid: true}); hash != want {
		t.Errorf("request_hash = %v, want %v", hash, want)
	}
}

func TestCreateMovementIdempotencyKeyMismatch(t *testing.T) {
	// See insertProducts: the first request adds 1 to p2 "kidneybohnen", which
	// has the barcode 4001686301265 with 1 unit.
	const first = `{"product_id": "p2", "kind": "add"}`
	tests := []struct {
		name string
		body string
	}{
		{"other quantity", `{"product_id": "p2", "kind": "add", "quantity": 2}`},
		{"other kind", `{"product_id": "p2", "kind": "consume"}`},
		{"other product", `{"product_id": "p4", "kind": "add"}`},
		{"same product by barcode", `{"barcode": "4001686301265", "kind": "add"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)
			decodeMovementResult(t, postWithKey(h, "/api/v1/movements", first, idempotencyKey))
			published := len(recorder.Events())
			before := get(h, "/api/v1/products").Body.String()

			rec := postWithKey(h, "/api/v1/movements", tt.body, idempotencyKey)

			checkProblemCode(t, rec, http.StatusUnprocessableEntity, "idempotency_key_mismatch")
			if n := countRows(t, db, "movements"); n != 1 {
				t.Errorf("movements = %d, want 1", n)
			}
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant %s", after, before)
			}
			if n := len(recorder.Events()); n != published {
				t.Errorf("events = %d, want only the %d of the first request", n, published)
			}
		})
	}
}

func TestCreateMovementWithoutIdempotencyKeyBooksTwice(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	const body = `{"product_id": "p2", "kind": "add"}`

	_, first := decodeMovementResult(t, post(h, "/api/v1/movements", body))
	result, second := decodeMovementResult(t, post(h, "/api/v1/movements", body))

	if first["id"] == second["id"] {
		t.Errorf("movement id = %v twice, want two movements", first["id"])
	}
	checkFields(t, result, `{"message": "kidneybohnen 3 → 4"}`)
	if n := countRows(t, db, "movements"); n != 2 {
		t.Errorf("movements = %d, want 2", n)
	}
	for _, m := range []map[string]any{first, second} {
		id, _ := m["id"].(string)
		if key, hash := storedIdempotency(t, db, id); key.Valid || hash.Valid {
			t.Errorf("movement %s: idempotency_key, request_hash = %v, %v, want NULL", id, key, hash)
		}
	}
}

func TestCreateMovementIdempotencyKeyLength(t *testing.T) {
	const body = `{"product_id": "p2", "kind": "add"}`
	tests := []struct {
		name   string
		key    string
		status int
	}{
		{"255 characters", strings.Repeat("k", 255), http.StatusCreated},
		{"256 characters", strings.Repeat("k", 256), http.StatusBadRequest},
		{"empty", "", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)

			rec := postWithKey(h, "/api/v1/movements", body, tt.key)

			if tt.status == http.StatusCreated {
				_, movement := decodeMovementResult(t, rec)
				id, _ := movement["id"].(string)
				if key, _ := storedIdempotency(t, db, id); key.String != tt.key {
					t.Errorf("idempotency_key = %q, want %q", key.String, tt.key)
				}
				return
			}
			checkProblemCode(t, rec, tt.status, "invalid_request")
			if n := countRows(t, db, "movements"); n != 0 {
				t.Errorf("movements = %d, want 0", n)
			}
		})
	}
}

func TestCreateMovementIdempotencyKeyAfterBarcodeRemoved(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	insertProducts(t, db)
	// See insertProducts: p2 "kidneybohnen" has stock 2 and the barcodes
	// 0034000470693 with 6 units and 4001686301265 with 1 unit.
	const body = `{"barcode": "0034000470693", "kind": "add"}`
	first, movement := decodeMovementResult(t, postWithKey(h, "/api/v1/movements", body, idempotencyKey))
	checkFields(t, first, `{"message": "kidneybohnen 2 → 8"}`)
	if rec := removeBarcode(h, "p2", "0034000470693"); rec.Code != http.StatusNoContent {
		t.Fatalf("remove barcode: status = %d, body %s", rec.Code, rec.Body.String())
	}
	// A movement without key changes the current stock to 7.
	decodeMovementResult(t, post(h, "/api/v1/movements", `{"product_id": "p2", "kind": "consume"}`))
	published := len(recorder.Events())

	result, repeated := decodeMovementResult(t, postWithKey(h, "/api/v1/movements", body, idempotencyKey))

	if !reflect.DeepEqual(repeated, movement) {
		t.Errorf("movement = %v\n    want %v", repeated, movement)
	}
	// The product is the current one, the message is the one of the movement.
	checkFields(t, result, `{"product_created": false, "warnings": [], "message": "kidneybohnen 2 → 8"}`)
	product, _ := result["product"].(map[string]any)
	checkFields(t, product, `{"id": "p2", "stock": 7, "barcodes": [{"code": "4001686301265", "units": 1}]}`)
	if stored := decodeProduct(t, get(h, "/api/v1/products/p2")); !reflect.DeepEqual(product, stored) {
		t.Errorf("product = %v\nwant stored %v", product, stored)
	}
	if n := countRows(t, db, "movements"); n != 2 {
		t.Errorf("movements = %d, want 2", n)
	}
	if n := len(recorder.Events()); n != published {
		t.Errorf("events = %d, want %d", n, published)
	}
}

func TestCreateMovementIdempotencyKeyAfterError(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)

	// See insertProducts: p3 "Kidneybohnen" has stock 0.
	checkProblemCode(t, postWithKey(h, "/api/v1/movements", `{"product_id": "p3", "kind": "consume"}`, idempotencyKey),
		http.StatusConflict, "stock_already_zero")
	// The failed request has stored nothing, so the key books another body.
	result, _ := decodeMovementResult(t, postWithKey(h, "/api/v1/movements",
		`{"product_id": "p3", "kind": "add"}`, idempotencyKey))

	checkFields(t, result, `{"warnings": [], "message": "Kidneybohnen 0 → 1"}`)
	if n := countRows(t, db, "movements"); n != 1 {
		t.Errorf("movements = %d, want 1", n)
	}
}
