package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// mergeProduct sends POST /api/v1/products/{id}/merge with the JSON body to h.
func mergeProduct(h http.Handler, id, body string) *httptest.ResponseRecorder {
	return post(h, "/api/v1/products/"+id+"/merge", body)
}

// mergeInto returns the body of a merge into the product target.
func mergeInto(target string) string {
	return fmt.Sprintf(`{"target_product_id": %q}`, target)
}

// insertMergeMovements inserts movements with direct SQL. movementID(1) to
// movementID(3) belong to p2 and lead to its stock 2: an add by barcode with
// an idempotency key, a consume and the reversal of the consume.
// movementID(4) belongs to p4.
func insertMergeMovements(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `INSERT INTO movements (id, product_id, kind, delta, stock_after,
		barcode, reverses_id, idempotency_key, request_hash, created_at) VALUES
		(?1, 'p2', 'add', 2, 2, '4001686301265', NULL, 'key-1', 'hash-1', '2026-09-23T12:00:00.000Z'),
		(?2, 'p2', 'consume', -1, 1, NULL, NULL, NULL, NULL, '2026-09-23T12:01:00.000Z'),
		(?3, 'p2', 'reversal', 1, 2, NULL, ?2, NULL, NULL, '2026-09-23T12:02:00.000Z'),
		(?4, 'p4', 'add', 2, 2, NULL, NULL, NULL, NULL, '2026-09-23T12:03:00.000Z')`,
		movementID(1), movementID(2), movementID(3), movementID(4)); err != nil {
		t.Fatalf("insert movements: %v", err)
	}
}

// storedRow is a stored barcode or movement: the product it belongs to and
// its other columns.
type storedRow struct {
	productID, rest string
}

// storedRows returns the rows of query, which selects the key, the product
// and the other columns as one text, by key.
func storedRows(t *testing.T, db *sql.DB, query string) map[string]storedRow {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	defer rows.Close()
	out := map[string]storedRow{}
	for rows.Next() {
		var key string
		var r storedRow
		if err := rows.Scan(&key, &r.productID, &r.rest); err != nil {
			t.Fatalf("scan %q: %v", query, err)
		}
		out[key] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return out
}

// storedBarcodes returns the stored barcodes by code.
func storedBarcodes(t *testing.T, db *sql.DB) map[string]storedRow {
	t.Helper()
	return storedRows(t, db, `SELECT code, product_id, units || ' ' || created_at FROM barcodes`)
}

// storedMovements returns the stored movements by id. The other columns are
// kind, delta, stock_after, barcode, reverses_id, idempotency_key,
// request_hash and created_at, with NULL as "NULL".
func storedMovements(t *testing.T, db *sql.DB) map[string]storedRow {
	t.Helper()
	return storedRows(t, db, `SELECT id, product_id, kind || ' ' || delta || ' ' || stock_after || ' ' ||
		coalesce(barcode, 'NULL') || ' ' || coalesce(reverses_id, 'NULL') || ' ' ||
		coalesce(idempotency_key, 'NULL') || ' ' || coalesce(request_hash, 'NULL') || ' ' ||
		created_at FROM movements`)
}

// moved returns a copy of rows in which the rows of the product source
// belong to target.
func moved(rows map[string]storedRow, source, target string) map[string]storedRow {
	out := maps.Clone(rows)
	for key, r := range out {
		if r.productID == source {
			r.productID = target
			out[key] = r
		}
	}
	return out
}

// mergeMovementID returns the id of the stored movement of kind merge, or ""
// if there is none.
func mergeMovementID(t *testing.T, db *sql.DB) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var id string
	err := db.QueryRowContext(ctx, "SELECT id FROM movements WHERE kind = 'merge'").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatalf("read merge movement: %v", err)
	}
	return id
}

// imageFiles returns the names of the files in dir.
func imageFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read image dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// withoutFields returns a copy of product without the fields names.
func withoutFields(product map[string]any, names ...string) map[string]any {
	out := maps.Clone(product)
	for _, name := range names {
		delete(out, name)
	}
	return out
}

func TestMergeProduct(t *testing.T) {
	imageDir := t.TempDir()
	h, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)
	insertMergeMovements(t, db)
	// See insertProducts: the source p2 has stock 2, two barcodes and the
	// image file p2.jpg. The target p3 has stock 0, target 4 and min_stock 1
	// (missing 4) and no barcodes. p4 keeps its image file.
	setImageFile(t, db, "p4", "p4.webp")
	writeFile(t, imageDir, "p2.jpg", []byte("jpg"))
	writeFile(t, imageDir, "p4.webp", []byte("webp"))
	target := decodeProduct(t, get(h, "/api/v1/products/p3"))
	barcodes, movements := storedBarcodes(t, db), storedMovements(t, db)
	start := time.Now().UTC().Truncate(time.Millisecond)

	got := decodeProduct(t, mergeProduct(h, "p2", mergeInto("p3")))

	// The response is the stored target with the stock and barcodes of p2.
	// Its other fields are unchanged.
	if stored := decodeProduct(t, get(h, "/api/v1/products/p3")); !reflect.DeepEqual(stored, got) {
		t.Errorf("stored product = %v\nwant response %v", stored, got)
	}
	checkFields(t, got, `{"id": "p3", "stock": 2, "missing": 0,
		"barcodes": [{"code": "0034000470693", "units": 6}, {"code": "4001686301265", "units": 1}]}`)
	changed := []string{"stock", "missing", "barcodes", "updated_at"}
	if unchanged, want := withoutFields(got, changed...), withoutFields(target, changed...); !reflect.DeepEqual(unchanged, want) {
		t.Errorf("product = %v\n     want %v", unchanged, want)
	}

	// p2 and its image file are deleted.
	checkProblemCode(t, get(h, "/api/v1/products/p2"), http.StatusNotFound, "not_found")
	if names := imageFiles(t, imageDir); !slices.Equal(names, []string{"p4.webp"}) {
		t.Errorf("image files = %v, want [p4.webp]", names)
	}

	// The barcodes and movements of p2 belong to p3 with all other columns
	// unchanged, including the reversal and the idempotency key.
	if got, want := storedBarcodes(t, db), moved(barcodes, "p2", "p3"); !reflect.DeepEqual(got, want) {
		t.Errorf("barcodes = %v\n    want %v", got, want)
	}
	after := storedMovements(t, db)
	mergeID := mergeMovementID(t, db)
	merge, ok := after[mergeID]
	if !ok {
		t.Fatalf("movements = %v, want a merge movement", after)
	}
	delete(after, mergeID)
	if want := moved(movements, "p2", "p3"); !reflect.DeepEqual(after, want) {
		t.Errorf("movements = %v\n     want %v", after, want)
	}

	// The merge movement books the stock of p2 at p3 at the time of the
	// request, which is also the new updated_at of p3.
	if u, err := uuid.Parse(mergeID); err != nil || u.Version() != 7 || mergeID != strings.ToLower(mergeID) {
		t.Errorf("merge movement id = %q, want UUIDv7 in lower case", mergeID)
	}
	fields := strings.Fields(merge.rest)
	createdAt := fields[len(fields)-1]
	if merge.productID != "p3" || strings.Join(fields[:len(fields)-1], " ") != "merge 2 2 NULL NULL NULL NULL" {
		t.Errorf("merge movement = %s %s, want p3 merge 2 2 NULL NULL NULL NULL <created_at>", merge.productID, merge.rest)
	}
	stored, err := time.Parse(store.TimeLayout, createdAt)
	if err != nil || stored.Before(start) || stored.After(time.Now()) {
		t.Errorf("merge movement created_at = %q, want the time of the request", createdAt)
	}
	s, _ := got["updated_at"].(string)
	if updatedAt, err := time.Parse(time.RFC3339, s); err != nil || !updatedAt.Equal(stored) {
		t.Errorf("updated_at = %v, want created_at of the merge movement %s", got["updated_at"], createdAt)
	}
}

func TestMergeProductWithoutStock(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	insertMergeMovements(t, db)
	// The source p4 has stock 0, the barcode 3017620422003 and one movement.
	// The target p2 has stock 2 and two barcodes.
	setStock(t, db, "p4", 0)
	target := decodeProduct(t, get(h, "/api/v1/products/p2"))
	barcodes, movements := storedBarcodes(t, db), storedMovements(t, db)

	got := decodeProduct(t, mergeProduct(h, "p4", mergeInto("p2")))

	// Only the barcodes of p2 change: no merge movement, the same stock and
	// the same updated_at.
	if stored := decodeProduct(t, get(h, "/api/v1/products/p2")); !reflect.DeepEqual(stored, got) {
		t.Errorf("stored product = %v\nwant response %v", stored, got)
	}
	want := maps.Clone(target)
	var wantBarcodes any
	if err := json.Unmarshal([]byte(`[{"code": "0034000470693", "units": 6},
		{"code": "3017620422003", "units": 1}, {"code": "4001686301265", "units": 1}]`), &wantBarcodes); err != nil {
		t.Fatalf("decode barcodes: %v", err)
	}
	want["barcodes"] = wantBarcodes
	if !reflect.DeepEqual(got, want) {
		t.Errorf("product = %v\n     want %v", got, want)
	}
	checkProblemCode(t, get(h, "/api/v1/products/p4"), http.StatusNotFound, "not_found")
	if got, want := storedBarcodes(t, db), moved(barcodes, "p4", "p2"); !reflect.DeepEqual(got, want) {
		t.Errorf("barcodes = %v\n    want %v", got, want)
	}
	if got, want := storedMovements(t, db), moved(movements, "p4", "p2"); !reflect.DeepEqual(got, want) {
		t.Errorf("movements = %v\n     want %v", got, want)
	}
}

func TestMergeProductErrors(t *testing.T) {
	var recorder events.Recorder
	imageDir := t.TempDir()
	h, db := newAppWithImages(t, &recorder, imageDir)
	insertProducts(t, db)
	insertMergeMovements(t, db)
	// See insertProducts: p2 has the image file p2.jpg.
	writeFile(t, imageDir, "p2.jpg", []byte("jpg"))
	before := get(h, "/api/v1/products").Body.String()
	movements := storedMovements(t, db)

	tests := []struct {
		name   string
		id     string
		body   string
		status int
		code   string
	}{
		{"same product", "p2", mergeInto("p2"), http.StatusBadRequest, "invalid_request"},
		{"same unknown product", "unbekannt", mergeInto("unbekannt"), http.StatusBadRequest, "invalid_request"},
		{"unknown source", "unbekannt", mergeInto("p2"), http.StatusNotFound, "not_found"},
		{"unknown target", "p2", mergeInto("unbekannt"), http.StatusNotFound, "not_found"},
		{"without target_product_id", "p2", `{}`, http.StatusBadRequest, "invalid_request"},
		{"target_product_id null", "p2", `{"target_product_id": null}`, http.StatusBadRequest, "invalid_request"},
		{"without body", "p2", ``, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := mergeProduct(h, tt.id, tt.body)

			checkProblemCode(t, rec, tt.status, tt.code)
			// Nothing is changed or removed and no event is published.
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant %s", after, before)
			}
			if after := storedMovements(t, db); !reflect.DeepEqual(after, movements) {
				t.Errorf("movements = %v\n     want %v", after, movements)
			}
			if names := imageFiles(t, imageDir); !slices.Equal(names, []string{"p2.jpg"}) {
				t.Errorf("image files = %v, want [p2.jpg]", names)
			}
			if n := len(recorder.Events()); n != 0 {
				t.Errorf("events = %d, want 0", n)
			}
		})
	}
}

func TestMergeProductPublishesEvents(t *testing.T) {
	// See insertProducts: p1 "Zucker" has stock 0 and target 0, so missing is
	// always 0. p2 "kidneybohnen" has stock 2, target 5 and no min_stock
	// (missing 3). p3 "Kidneybohnen" has stock 0, target 4 and min_stock 1
	// (missing 4). p4 "Mehl" has stock 2, target 4 and min_stock 1 (missing 0).
	// The data of stock.adjusted gets the id of the merge movement.
	adjusted := func(id string, delta, stockAfter int64) reversalEvent {
		return reversalEvent{events.TypeStockAdjusted, events.StockData{ProductID: id, Delta: delta, StockAfter: stockAfter}}
	}
	shopping := func(id, name string, before, after int64) reversalEvent {
		return reversalEvent{events.TypeShoppingChanged, events.ShoppingChangedData{
			ProductID: id, Name: name, MissingBefore: before, MissingAfter: after,
		}}
	}
	tests := []struct {
		name           string
		source, target string
		events         []reversalEvent
	}{
		{"target and source leave the shopping list", "p2", "p3", []reversalEvent{
			adjusted("p3", 2, 2),
			shopping("p3", "Kidneybohnen", 4, 0),
			shopping("p2", "kidneybohnen", 3, 0),
		}},
		{"source leaves the shopping list", "p2", "p4", []reversalEvent{
			adjusted("p4", 2, 4),
			shopping("p2", "kidneybohnen", 3, 0),
		}},
		{"target stays on the shopping list", "p4", "p2", []reversalEvent{
			adjusted("p2", 2, 4),
			shopping("p2", "kidneybohnen", 3, 1),
		}},
		{"source without stock", "p3", "p2", []reversalEvent{
			shopping("p3", "Kidneybohnen", 4, 0),
		}},
		{"target without shopping list", "p4", "p1", []reversalEvent{
			adjusted("p1", 2, 2),
		}},
		{"no change", "p1", "p4", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)

			decodeProduct(t, mergeProduct(h, tt.source, mergeInto(tt.target)))

			checkReversalEvents(t, recorder.Events(), tt.events, map[string]any{"id": mergeMovementID(t, db)})
		})
	}
}

func TestMergeProductThenReverse(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	// See insertProducts: p2 "kidneybohnen" has stock 2, p3 "Kidneybohnen"
	// has stock 0. The bookings change the stock of p2 to 5, 4 and back to 5.
	add := bookMovement(t, h, `{"product_id": "p2", "kind": "add", "quantity": 3}`)
	consume := bookMovement(t, h, `{"product_id": "p2", "kind": "consume"}`)
	decodeMovementResult(t, reverse(h, consume))
	checkFields(t, decodeProduct(t, mergeProduct(h, "p2", mergeInto("p3"))), `{"id": "p3", "stock": 5}`)

	// The moved add is reversed at p3.
	result, movement := decodeMovementResult(t, reverse(h, add))

	checkFields(t, movement, fmt.Sprintf(`{"product_id": "p3", "kind": "reversal", "delta": -3, "stock_after": 2,
		"barcode": null, "reverses_id": %q}`, add))
	checkFields(t, result, `{"warnings": [], "message": "Kidneybohnen 5 → 2"}`)
	product, _ := result["product"].(map[string]any)
	checkFields(t, product, `{"id": "p3", "stock": 2}`)
	// The moved consume stays reversed and the merge movement is not reversible.
	checkProblemCode(t, reverse(h, consume), http.StatusConflict, "already_reversed")
	checkProblemCode(t, reverse(h, mergeMovementID(t, db)), http.StatusConflict, "not_reversible")
}
