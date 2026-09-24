package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
)

// setCrateSize sets crate_size of the product id with direct SQL, without
// changing its updated_at. crateSize is nil for NULL.
func setCrateSize(t *testing.T, db *sql.DB, id string, crateSize any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "UPDATE products SET crate_size = ? WHERE id = ?", crateSize, id); err != nil {
		t.Fatalf("set crate_size of %s: %v", id, err)
	}
}

// checkCrateSize asserts that product, a product from a response, and the
// stored product with its id have the crate size want (a JSON value, null for
// none), and that GET returns product.
func checkCrateSize(t *testing.T, h http.Handler, db *sql.DB, product map[string]any, want string) {
	t.Helper()
	id, _ := product["id"].(string)
	checkFields(t, product, fmt.Sprintf(`{"crate_size": %s}`, want))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var stored sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT crate_size FROM products WHERE id = ?", id).Scan(&stored); err != nil {
		t.Fatalf("read crate_size of %s: %v", id, err)
	}
	got := "null"
	if stored.Valid {
		got = strconv.FormatInt(stored.Int64, 10)
	}
	if got != want {
		t.Errorf("stored crate_size of %s = %s, want %s", id, got, want)
	}
	if got := decodeProduct(t, get(h, "/api/v1/products/"+id)); !reflect.DeepEqual(got, product) {
		t.Errorf("stored product = %v\nwant response %v", got, product)
	}
}

func TestCreateProductCrateSize(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"without crate_size", `{"name": "Wasser"}`, "null"},
		{"null", `{"name": "Wasser", "crate_size": null}`, "null"},
		{"value", `{"name": "Wasser", "crate_size": 12}`, "12"},
		{"minimum", `{"name": "Wasser", "crate_size": 2}`, "2"},
		{"maximum", `{"name": "Wasser", "crate_size": 100}`, "100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)

			got := decodeCreated(t, post(h, "/api/v1/products", tt.body))

			checkCrateSize(t, h, db, got, tt.want)
		})
	}
}

func TestCreateProductCrateSizeErrors(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)

	tests := []struct {
		name, body string
	}{
		{"below minimum", `{"name": "Wasser", "crate_size": 1}`},
		{"above maximum", `{"name": "Wasser", "crate_size": 101}`},
		{"string", `{"name": "Wasser", "crate_size": "12"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(h, "/api/v1/products", tt.body)

			checkProblemCode(t, rec, http.StatusBadRequest, "invalid_request")
			if n := countRows(t, db, "products"); n != 0 {
				t.Errorf("products = %d, want 0", n)
			}
			if n := len(recorder.Events()); n != 0 {
				t.Errorf("events = %d, want 0", n)
			}
		})
	}
}

func TestUpdateProductCrateSize(t *testing.T) {
	// See insertProducts: p2 needs review, which a patch without name, brand
	// and package_size does not change.
	tests := []struct {
		name string
		// stored is the crate size before the patch, nil for NULL.
		stored  any
		body    string
		changes string
	}{
		{"set", nil, `{"crate_size": 20}`, `{"crate_size": 20}`},
		{"change", 24, `{"crate_size": 12}`, `{"crate_size": 12}`},
		{"clear with null", 24, `{"crate_size": null}`, `{"crate_size": null}`},
		{"null without crate size", nil, `{"crate_size": null}`, `{"crate_size": null}`},
		{"minimum", nil, `{"crate_size": 2}`, `{"crate_size": 2}`},
		{"maximum", 24, `{"crate_size": 100}`, `{"crate_size": 100}`},
		{"other field keeps crate size", 24, `{"note": null}`, `{"note": null}`},
		{"with name", 24, `{"name": "Wasser", "crate_size": 6}`, `{"name": "Wasser", "crate_size": 6, "needs_review": false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)
			setCrateSize(t, db, "p2", tt.stored)

			checkPatch(t, h, "p2", tt.body, tt.changes)

			// The crate size does not change missing, so there is no event.
			checkShoppingChanged(t, recorder.Events(), nil)
		})
	}
}

func TestUpdateProductCrateSizeErrors(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	insertProducts(t, db)
	setCrateSize(t, db, "p2", 24)
	before := get(h, "/api/v1/products").Body.String()

	tests := []struct {
		name, body string
	}{
		{"below minimum", `{"crate_size": 1}`},
		{"above maximum", `{"crate_size": 101}`},
		{"string", `{"crate_size": "12"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := patchProduct(h, "p2", tt.body)

			checkProblemCode(t, rec, http.StatusBadRequest, "invalid_request")
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

func TestListsCrateSize(t *testing.T) {
	h, db := newApp(t)
	insertShoppingProducts(t, db,
		shoppingProduct{"p1", "Wasser", "Gerolsteiner", 3, 24, nil},
		shoppingProduct{"p2", "Mehl", nil, 0, 4, nil},
		shoppingProduct{"p3", "Bier", nil, 30, 20, nil})
	setCrateSize(t, db, "p1", 12)
	setCrateSize(t, db, "p3", 20)

	// missing, stock and target stay in bottles. p3 is not short.
	checkShoppingList(t, h, `{"items": [
		{"product_id": "p2", "name": "Mehl", "brand": null, "missing": 4, "stock": 0, "target": 4, "marked": false, "crate_size": null},
		{"product_id": "p1", "name": "Wasser", "brand": "Gerolsteiner", "missing": 21, "stock": 3, "target": 24, "marked": false, "crate_size": 12}
	]}`)

	rec := get(h, "/api/v1/products")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body.String(), err)
	}
	got := make(map[string]any)
	for _, p := range list.Items {
		crateSize, ok := p["crate_size"]
		if !ok {
			t.Errorf("product %v has no crate_size", p["id"])
		}
		got[p["id"].(string)] = crateSize
	}
	if want := map[string]any{"p1": 12.0, "p2": nil, "p3": 20.0}; !reflect.DeepEqual(got, want) {
		t.Errorf("crate sizes = %v, want %v", got, want)
	}
}

func TestMergeProductCrateSize(t *testing.T) {
	// See insertProducts: p2 has stock 2, p3 has stock 0, p4 has stock 2
	// which is set to 0 for a merge without stock. None of them is marked.
	tests := []struct {
		name           string
		source, target string
		sourceStock    int
		// sourceSize and targetSize are the crate sizes before the merge,
		// nil for NULL.
		sourceSize, targetSize any
		want                   string
		// updated reports whether the updated_at of the target changes.
		updated bool
	}{
		{"source only", "p2", "p3", 2, 20, nil, "20", true},
		{"target only", "p2", "p3", 2, nil, 12, "12", true},
		{"both", "p2", "p3", 2, 20, 12, "12", true},
		{"neither", "p2", "p3", 2, nil, nil, "null", true},
		{"source without stock only", "p4", "p2", 0, 20, nil, "20", true},
		{"source without stock and target only", "p4", "p2", 0, nil, 12, "12", false},
		{"source without stock and both", "p4", "p2", 0, 20, 12, "12", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)
			setStock(t, db, tt.source, tt.sourceStock)
			setCrateSize(t, db, tt.source, tt.sourceSize)
			setCrateSize(t, db, tt.target, tt.targetSize)
			before := decodeProduct(t, get(h, "/api/v1/products/"+tt.target))
			start := time.Now().UTC().Truncate(time.Millisecond)

			got := decodeProduct(t, mergeProduct(h, tt.source, mergeInto(tt.target)))

			checkFields(t, got, fmt.Sprintf(`{"id": %q}`, tt.target))
			checkCrateSize(t, h, db, got, tt.want)
			if !tt.updated {
				if got["updated_at"] != before["updated_at"] {
					t.Errorf("updated_at = %v, want unchanged %v", got["updated_at"], before["updated_at"])
				}
				return
			}
			s, _ := got["updated_at"].(string)
			if updatedAt, err := time.Parse(time.RFC3339, s); err != nil || updatedAt.Before(start) || updatedAt.After(time.Now()) {
				t.Errorf("updated_at = %v, want the time of the request", got["updated_at"])
			}
		})
	}
}
