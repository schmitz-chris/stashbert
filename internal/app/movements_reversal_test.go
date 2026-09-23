package app_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// reverse sends POST /api/v1/movements/{id}/reversal without body to h and
// returns the response.
func reverse(h http.Handler, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/movements/"+id+"/reversal", nil))
	return rec
}

// reverseWithKey is like reverse but sends the header Idempotency-Key with key.
func reverseWithKey(h http.Handler, id, key string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/movements/"+id+"/reversal", nil)
	req.Header.Set("Idempotency-Key", key)
	h.ServeHTTP(rec, req)
	return rec
}

// bookMovement books body with POST /api/v1/movements on h and returns the id
// of the movement.
func bookMovement(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	_, movement := decodeMovementResult(t, post(h, "/api/v1/movements", body))
	id, _ := movement["id"].(string)
	return id
}

// hashOf returns the request hash of a reversal of the movement id: the
// SHA-256 of the id in lower case hex.
func hashOf(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

// checkStoredReversal asserts that the stored movement with the id of
// movement from a response equals it, reverses reversesID and has no barcode.
// With a key, it has the idempotency key key and the request hash of
// reversesID; without, both are NULL.
func checkStoredReversal(t *testing.T, db *sql.DB, movement map[string]any, reversesID, key string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var (
		productID, kind, createdAt       string
		delta, stockAfter                int
		barcode, storedReverses, k, hash sql.NullString
	)
	if err := db.QueryRowContext(ctx, `SELECT product_id, kind, delta, stock_after, barcode,
		reverses_id, idempotency_key, request_hash, created_at FROM movements WHERE id = ?`, movement["id"]).Scan(
		&productID, &kind, &delta, &stockAfter, &barcode, &storedReverses, &k, &hash, &createdAt); err != nil {
		t.Fatalf("read movement %v: %v", movement["id"], err)
	}
	got := fmt.Sprintf("%s %s %d %d", productID, kind, delta, stockAfter)
	want := fmt.Sprintf("%v %v %v %v", movement["product_id"], movement["kind"], movement["delta"], movement["stock_after"])
	if got != want {
		t.Errorf("stored movement = %s, want %s", got, want)
	}
	stored, err := time.Parse(store.TimeLayout, createdAt)
	if err != nil {
		t.Errorf("stored created_at = %q, want layout %s", createdAt, store.TimeLayout)
	}
	s, _ := movement["created_at"].(string)
	if responded, err := time.Parse(time.RFC3339, s); err != nil || !responded.Equal(stored) {
		t.Errorf("created_at = %v, want stored %s", movement["created_at"], createdAt)
	}
	if barcode.Valid {
		t.Errorf("stored barcode = %q, want NULL", barcode.String)
	}
	if want := (sql.NullString{String: reversesID, Valid: true}); storedReverses != want {
		t.Errorf("stored reverses_id = %v, want %v", storedReverses, want)
	}
	wantKey, wantHash := sql.NullString{}, sql.NullString{}
	if key != "" {
		wantKey, wantHash = sql.NullString{String: key, Valid: true}, sql.NullString{String: hashOf(reversesID), Valid: true}
	}
	if k != wantKey || hash != wantHash {
		t.Errorf("idempotency_key, request_hash = %v, %v, want %v, %v", k, hash, wantKey, wantHash)
	}
}

// reversalEvent is an expected event of a reversal. The movement id of
// events.StockData is set from the response.
type reversalEvent struct {
	typ  string
	data any
}

// checkReversalEvents asserts that recorded are the events want of the
// reversal movement from a response.
func checkReversalEvents(t *testing.T, recorded []events.Event, want []reversalEvent, movement map[string]any) {
	t.Helper()
	if len(recorded) != len(want) {
		t.Fatalf("events = %+v, want %d events", recorded, len(want))
	}
	for i, w := range want {
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
}

func TestReverseMovement(t *testing.T) {
	// See insertProducts: p1 "Zucker" has target 0, so missing is always 0.
	// p2 "kidneybohnen" has target 5 and no min_stock, so missing is 5 - stock
	// below 5; its barcode 0034000470693 has 6 units. p3 "Kidneybohnen" has
	// target 4 and min_stock 1, so missing is 4 at stock 0 and 0 above.
	adjusted := func(id string, delta, stockAfter int64) reversalEvent {
		return reversalEvent{events.TypeStockAdjusted, events.StockData{ProductID: id, Delta: delta, StockAfter: stockAfter}}
	}
	empty := func(id, name string) reversalEvent {
		return reversalEvent{events.TypeProductEmpty, events.ProductEmptyData{ProductID: id, Name: name}}
	}
	shopping := func(id, name string, before, after int64) reversalEvent {
		return reversalEvent{events.TypeShoppingChanged, events.ShoppingChangedData{
			ProductID: id, Name: name, MissingBefore: before, MissingAfter: after,
		}}
	}
	tests := []struct {
		name  string
		id    string
		stock int
		// bookings are booked in this order; the first one is reversed.
		bookings   []string
		delta      int
		stockAfter int
		warnings   string
		message    string
		events     []reversalEvent
	}{
		{"consume", "p2", 2, []string{`{"product_id": "p2", "kind": "consume"}`},
			1, 2, `[]`, "kidneybohnen 1 → 2", []reversalEvent{
				adjusted("p2", 1, 2),
				shopping("p2", "kidneybohnen", 4, 3),
			}},
		{"add", "p2", 2, []string{`{"product_id": "p2", "kind": "add", "quantity": 3}`},
			-3, 2, `[]`, "kidneybohnen 5 → 2", []reversalEvent{
				adjusted("p2", -3, 2),
				shopping("p2", "kidneybohnen", 0, 3),
			}},
		{"consume to 0", "p2", 2, []string{`{"product_id": "p2", "kind": "consume", "quantity": 2}`},
			2, 2, `[]`, "kidneybohnen 0 → 2", []reversalEvent{
				adjusted("p2", 2, 2),
				shopping("p2", "kidneybohnen", 5, 3),
			}},
		{"add back to 0", "p1", 0, []string{`{"product_id": "p1", "kind": "add", "quantity": 2}`},
			-2, 0, `[]`, "Zucker 2 → 0", []reversalEvent{
				adjusted("p1", -2, 0),
				empty("p1", "Zucker"),
			}},
		{"consume clamped to 0", "p2", 1, []string{`{"product_id": "p2", "kind": "consume", "quantity": 3}`},
			1, 1, `[]`, "kidneybohnen 0 → 1", []reversalEvent{
				adjusted("p2", 1, 1),
				shopping("p2", "kidneybohnen", 5, 4),
			}},
		{"inventory", "p2", 2, []string{`{"product_id": "p2", "kind": "inventory", "stock": 7}`},
			-5, 2, `[]`, "kidneybohnen 7 → 2", []reversalEvent{
				adjusted("p2", -5, 2),
				shopping("p2", "kidneybohnen", 0, 3),
			}},
		{"inventory without change", "p2", 2, []string{`{"product_id": "p2", "kind": "inventory", "stock": 2}`},
			0, 2, `[]`, "kidneybohnen 2 → 2", nil},
		{"add by barcode", "p2", 2, []string{`{"barcode": "0034000470693", "kind": "add"}`},
			-6, 2, `[]`, "kidneybohnen 8 → 2", []reversalEvent{
				adjusted("p2", -6, 2),
				shopping("p2", "kidneybohnen", 0, 3),
			}},
		{"consume after a later add", "p2", 2, []string{
			`{"product_id": "p2", "kind": "consume"}`,
			`{"product_id": "p2", "kind": "add", "quantity": 4}`,
		}, 1, 6, `[]`, "kidneybohnen 5 → 6", []reversalEvent{
			adjusted("p2", 1, 6),
		}},
		{"add clamped to 0", "p3", 0, []string{
			`{"product_id": "p3", "kind": "add", "quantity": 3}`,
			`{"product_id": "p3", "kind": "consume", "quantity": 2}`,
		}, -1, 0, `["clamped_to_zero"]`, "Kidneybohnen 1 → 0", []reversalEvent{
			adjusted("p3", -1, 0),
			empty("p3", "Kidneybohnen"),
			shopping("p3", "Kidneybohnen", 0, 4),
		}},
		{"add clamped to unchanged 0", "p2", 0, []string{
			`{"product_id": "p2", "kind": "add", "quantity": 2}`,
			`{"product_id": "p2", "kind": "consume", "quantity": 2}`,
		}, 0, 0, `["clamped_to_zero"]`, "kidneybohnen 0 → 0", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)
			setStock(t, db, tt.id, tt.stock)
			var ids []string
			for _, body := range tt.bookings {
				ids = append(ids, bookMovement(t, h, body))
			}
			published := len(recorder.Events())
			start := time.Now().UTC().Truncate(time.Millisecond)

			rec := reverse(h, ids[0])

			result, movement := decodeMovementResult(t, rec)
			checkFields(t, movement, fmt.Sprintf(`{"product_id": %q, "kind": "reversal", "delta": %d, "stock_after": %d,
				"barcode": null, "reverses_id": %q}`, tt.id, tt.delta, tt.stockAfter, ids[0]))
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
			checkStoredReversal(t, db, movement, ids[0], "")
			if n := countRows(t, db, "movements"); n != len(tt.bookings)+1 {
				t.Errorf("movements = %d, want %d", n, len(tt.bookings)+1)
			}

			// The product in the response is stored with the new stock and
			// the time of the reversal as updated_at, also for delta 0.
			product, _ := result["product"].(map[string]any)
			checkFields(t, product, fmt.Sprintf(`{"id": %q, "stock": %d}`, tt.id, tt.stockAfter))
			if stored := decodeProduct(t, get(h, "/api/v1/products/"+tt.id)); !reflect.DeepEqual(stored, product) {
				t.Errorf("stored product = %v\nwant response %v", stored, product)
			}
			if product["updated_at"] != movement["created_at"] {
				t.Errorf("product updated_at = %v, want created_at of the reversal %v", product["updated_at"], movement["created_at"])
			}

			checkReversalEvents(t, recorder.Events()[published:], tt.events, movement)
		})
	}
}

func TestReverseMovementErrors(t *testing.T) {
	tests := []struct {
		name string
		// setup books on h and db and returns the id to reverse.
		setup func(t *testing.T, h http.Handler, db *sql.DB) string
		// key is sent as Idempotency-Key unless it is empty.
		key    string
		status int
		code   string
	}{
		{"unknown id", func(*testing.T, http.Handler, *sql.DB) string {
			return movementID(1)
		}, "", http.StatusNotFound, "not_found"},
		{"id that is no UUID", func(*testing.T, http.Handler, *sql.DB) string {
			return "unbekannt"
		}, "", http.StatusNotFound, "not_found"},
		{"unknown id with Idempotency-Key", func(*testing.T, http.Handler, *sql.DB) string {
			return movementID(1)
		}, idempotencyKey, http.StatusNotFound, "not_found"},
		{"already reversed", func(t *testing.T, h http.Handler, _ *sql.DB) string {
			id := bookMovement(t, h, `{"product_id": "p2", "kind": "consume"}`)
			decodeMovementResult(t, reverse(h, id))
			return id
		}, "", http.StatusConflict, "already_reversed"},
		{"already reversed with another Idempotency-Key", func(t *testing.T, h http.Handler, _ *sql.DB) string {
			id := bookMovement(t, h, `{"product_id": "p2", "kind": "add"}`)
			decodeMovementResult(t, reverseWithKey(h, id, "first-key"))
			return id
		}, idempotencyKey, http.StatusConflict, "already_reversed"},
		{"already reversed with delta 0", func(t *testing.T, h http.Handler, _ *sql.DB) string {
			id := bookMovement(t, h, `{"product_id": "p2", "kind": "inventory", "stock": 2}`)
			decodeMovementResult(t, reverse(h, id))
			return id
		}, "", http.StatusConflict, "already_reversed"},
		{"reversal", func(t *testing.T, h http.Handler, _ *sql.DB) string {
			id := bookMovement(t, h, `{"product_id": "p2", "kind": "consume"}`)
			_, reversal := decodeMovementResult(t, reverse(h, id))
			reversalID, _ := reversal["id"].(string)
			return reversalID
		}, "", http.StatusConflict, "not_reversible"},
		{"merge", func(t *testing.T, _ http.Handler, db *sql.DB) string {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			if _, err := db.ExecContext(ctx, `INSERT INTO movements (id, product_id, kind, delta, stock_after, created_at)
				VALUES (?, 'p2', 'merge', 2, 2, '2026-09-23T12:00:00.000Z')`, movementID(1)); err != nil {
				t.Fatalf("insert merge movement: %v", err)
			}
			return movementID(1)
		}, "", http.StatusConflict, "not_reversible"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)
			id := tt.setup(t, h, db)
			movements := countRows(t, db, "movements")
			published := len(recorder.Events())
			before := get(h, "/api/v1/products").Body.String()

			rec := reverse(h, id)
			if tt.key != "" {
				rec = reverseWithKey(h, id, tt.key)
			}

			checkProblemCode(t, rec, tt.status, tt.code)
			// Nothing is booked, no product is changed and no event is published.
			if n := countRows(t, db, "movements"); n != movements {
				t.Errorf("movements = %d, want %d", n, movements)
			}
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant %s", after, before)
			}
			if n := len(recorder.Events()); n != published {
				t.Errorf("events = %d, want %d", n, published)
			}
		})
	}
}

func TestReverseMovementIdempotencyKeyRepeats(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	insertProducts(t, db)
	// See insertProducts: p2 "kidneybohnen" has stock 2. The reversal of the
	// add is clamped from 1 - 3 to 0.
	id := bookMovement(t, h, `{"product_id": "p2", "kind": "add", "quantity": 3}`)
	bookMovement(t, h, `{"product_id": "p2", "kind": "consume", "quantity": 4}`)
	first, firstMovement := decodeMovementResult(t, reverseWithKey(h, id, idempotencyKey))
	checkFields(t, first, `{"product_created": false, "warnings": ["clamped_to_zero"], "message": "kidneybohnen 1 → 0"}`)
	checkStoredReversal(t, db, firstMovement, id, idempotencyKey)
	// A movement without key changes the current stock to 2.
	bookMovement(t, h, `{"product_id": "p2", "kind": "add", "quantity": 2}`)
	published := len(recorder.Events())

	second, secondMovement := decodeMovementResult(t, reverseWithKey(h, id, idempotencyKey))

	if !reflect.DeepEqual(secondMovement, firstMovement) {
		t.Errorf("movement = %v\n    want %v", secondMovement, firstMovement)
	}
	// The product is the current one, the message is the one of the reversal.
	checkFields(t, second, `{"product_created": false, "warnings": [], "message": "kidneybohnen 1 → 0"}`)
	product, _ := second["product"].(map[string]any)
	checkFields(t, product, `{"id": "p2", "stock": 2}`)
	if stored := decodeProduct(t, get(h, "/api/v1/products/p2")); !reflect.DeepEqual(product, stored) {
		t.Errorf("product = %v\nwant stored %v", product, stored)
	}
	if n := countRows(t, db, "movements"); n != 4 {
		t.Errorf("movements = %d, want 4", n)
	}
	if n := len(recorder.Events()); n != published {
		t.Errorf("events = %d, want %d", n, published)
	}
}

func TestReverseMovementIdempotencyKeyMismatch(t *testing.T) {
	// See insertProducts: a adds 1 to p2 "kidneybohnen", b adds 1 to p4 "Mehl".
	const booking = `{"product_id": "p2", "kind": "add"}`
	tests := []struct {
		name string
		// first uses idempotencyKey successfully, second again.
		first, second func(h http.Handler, a, b string) *httptest.ResponseRecorder
	}{
		{"reversal of another movement",
			func(h http.Handler, a, _ string) *httptest.ResponseRecorder {
				return reverseWithKey(h, a, idempotencyKey)
			},
			func(h http.Handler, _, b string) *httptest.ResponseRecorder {
				return reverseWithKey(h, b, idempotencyKey)
			}},
		{"reversal with the key of a booking",
			func(h http.Handler, _, _ string) *httptest.ResponseRecorder {
				return postWithKey(h, "/api/v1/movements", booking, idempotencyKey)
			},
			func(h http.Handler, a, _ string) *httptest.ResponseRecorder {
				return reverseWithKey(h, a, idempotencyKey)
			}},
		{"booking with the key of a reversal",
			func(h http.Handler, a, _ string) *httptest.ResponseRecorder {
				return reverseWithKey(h, a, idempotencyKey)
			},
			func(h http.Handler, _, _ string) *httptest.ResponseRecorder {
				return postWithKey(h, "/api/v1/movements", booking, idempotencyKey)
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)
			a := bookMovement(t, h, booking)
			b := bookMovement(t, h, `{"product_id": "p4", "kind": "add"}`)
			decodeMovementResult(t, tt.first(h, a, b))
			movements := countRows(t, db, "movements")
			published := len(recorder.Events())
			before := get(h, "/api/v1/products").Body.String()

			rec := tt.second(h, a, b)

			checkProblemCode(t, rec, http.StatusUnprocessableEntity, "idempotency_key_mismatch")
			if n := countRows(t, db, "movements"); n != movements {
				t.Errorf("movements = %d, want %d", n, movements)
			}
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant %s", after, before)
			}
			if n := len(recorder.Events()); n != published {
				t.Errorf("events = %d, want %d", n, published)
			}
		})
	}
}

func TestReverseMovementIdempotencyKeyLength(t *testing.T) {
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
			id := bookMovement(t, h, `{"product_id": "p2", "kind": "add"}`)

			rec := reverseWithKey(h, id, tt.key)

			if tt.status == http.StatusCreated {
				_, movement := decodeMovementResult(t, rec)
				checkStoredReversal(t, db, movement, id, tt.key)
				return
			}
			checkProblemCode(t, rec, tt.status, "invalid_request")
			if n := countRows(t, db, "movements"); n != 1 {
				t.Errorf("movements = %d, want 1", n)
			}
		})
	}
}

func TestReverseMovementConcurrently(t *testing.T) {
	const n = 8
	tests := []struct {
		name string
		key  string
	}{
		{"without Idempotency-Key", ""},
		{"with the same Idempotency-Key", idempotencyKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)
			id := bookMovement(t, h, `{"product_id": "p2", "kind": "add", "quantity": 3}`)
			published := len(recorder.Events())

			recs := make([]*httptest.ResponseRecorder, n)
			var wg sync.WaitGroup
			for i := range recs {
				wg.Go(func() {
					if tt.key == "" {
						recs[i] = reverse(h, id)
					} else {
						recs[i] = reverseWithKey(h, id, tt.key)
					}
				})
			}
			wg.Wait()

			// Exactly one reversal is stored. Without key, the other requests
			// fail with already_reversed; with the same key, all of them
			// return it.
			reversals := map[any]bool{}
			for _, rec := range recs {
				if tt.key == "" && rec.Code != http.StatusCreated {
					checkProblemCode(t, rec, http.StatusConflict, "already_reversed")
					continue
				}
				_, movement := decodeMovementResult(t, rec)
				reversals[movement["id"]] = true
			}
			if len(reversals) != 1 {
				t.Errorf("reversals in responses = %v, want 1", reversals)
			}
			if n := countRows(t, db, "movements"); n != 2 {
				t.Errorf("movements = %d, want 2", n)
			}
			if stored := decodeProduct(t, get(h, "/api/v1/products/p2")); stored["stock"] != float64(2) {
				t.Errorf("stock = %v, want 2", stored["stock"])
			}
			// The one reversal publishes stock.adjusted and shopping.changed.
			if n := len(recorder.Events()) - published; n != 2 {
				t.Errorf("events = %d, want 2", n)
			}
		})
	}
}
