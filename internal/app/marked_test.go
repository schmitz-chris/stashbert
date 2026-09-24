package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// setMarked sets marked of the product id with direct SQL, without changing
// its updated_at.
func setMarked(t *testing.T, db *sql.DB, id string, marked int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "UPDATE products SET marked = ? WHERE id = ?", marked, id); err != nil {
		t.Fatalf("set marked of %s: %v", id, err)
	}
}

// checkMarked asserts that product, a product from a response, and the
// stored product with its id have marked want, and that GET returns product.
func checkMarked(t *testing.T, h http.Handler, db *sql.DB, product map[string]any, want bool) {
	t.Helper()
	id, _ := product["id"].(string)
	if product["marked"] != want {
		t.Errorf("marked of %s = %v, want %v", id, product["marked"], want)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var stored bool
	if err := db.QueryRowContext(ctx, "SELECT marked FROM products WHERE id = ?", id).Scan(&stored); err != nil {
		t.Fatalf("read marked of %s: %v", id, err)
	}
	if stored != want {
		t.Errorf("stored marked of %s = %v, want %v", id, stored, want)
	}
	if got := decodeProduct(t, get(h, "/api/v1/products/"+id)); !reflect.DeepEqual(got, product) {
		t.Errorf("stored product = %v\nwant response %v", got, product)
	}
}

func TestGetShoppingListMarked(t *testing.T) {
	h, db := newApp(t)
	insertShoppingProducts(t, db,
		shoppingProduct{"p1", "Spaghetti", "Barilla", 6, 5, nil},
		shoppingProduct{"p2", "Zucker", nil, 0, 0, nil},
		shoppingProduct{"p3", "Kidneybohnen", nil, 2, 5, nil},
		shoppingProduct{"p4", "Mehl", nil, 2, 4, 1},
		shoppingProduct{"p5", "apfelmus", nil, 0, 2, nil})
	for _, id := range []string{"p1", "p2", "p3"} {
		setMarked(t, db, id, 1)
	}

	// The marked p1 and p2 without shortfall are listed with missing 0, the
	// marked p3 with its shortfall. p4 is neither marked nor short. The order
	// stays by name.
	checkShoppingList(t, h, `{"items": [
		{"product_id": "p5", "name": "apfelmus", "brand": null, "missing": 2, "stock": 0, "target": 2, "marked": false},
		{"product_id": "p3", "name": "Kidneybohnen", "brand": null, "missing": 3, "stock": 2, "target": 5, "marked": true},
		{"product_id": "p1", "name": "Spaghetti", "brand": "Barilla", "missing": 0, "stock": 6, "target": 5, "marked": true},
		{"product_id": "p2", "name": "Zucker", "brand": null, "missing": 0, "stock": 0, "target": 0, "marked": true}
	]}`)
	checkFields(t, decodeProduct(t, get(h, "/api/v1/products/p1")), `{"missing": 0, "marked": true}`)
	checkFields(t, decodeProduct(t, get(h, "/api/v1/products/p4")), `{"missing": 0, "marked": false}`)
}

func TestCreateMovementEndsMarking(t *testing.T) {
	// See insertProducts: p2 "kidneybohnen" has stock 2 and target 5, its
	// barcode 4001686301265 has 1 unit and 0034000470693 has 6 units.
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"add by product_id", `{"product_id": "p2", "kind": "add"}`, false},
		{"add by barcode", `{"barcode": "0034000470693", "kind": "add"}`, false},
		{"add below target", `{"barcode": "4001686301265", "kind": "add"}`, false},
		{"consume by product_id", `{"product_id": "p2", "kind": "consume"}`, true},
		{"consume by barcode", `{"barcode": "4001686301265", "kind": "consume"}`, true},
		{"inventory", `{"product_id": "p2", "kind": "inventory", "stock": 7}`, true},
		{"inventory without change", `{"barcode": "0034000470693", "kind": "inventory", "stock": 2}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)
			setMarked(t, db, "p2", 1)

			result, _ := decodeMovementResult(t, post(h, "/api/v1/movements", tt.body))

			product, _ := result["product"].(map[string]any)
			checkMarked(t, h, db, product, tt.want)
		})
	}
}

func TestCreateMovementUnknownBarcodeMarked(t *testing.T) {
	t.Run("created product", func(t *testing.T) {
		h, db := newAppWithLookuper(t, events.Nop{}, &fakeLookuper{result: goldbaeren})

		result, _ := decodeMovementResult(t, post(h, "/api/v1/movements", `{"barcode": "4006381333931", "kind": "add"}`))

		checkFields(t, result, `{"product_created": true}`)
		product, _ := result["product"].(map[string]any)
		checkMarked(t, h, db, product, false)
	})

	t.Run("barcode stored during lookup", func(t *testing.T) {
		f := &fakeLookuper{result: goldbaeren}
		h, db := newAppWithLookuper(t, events.Nop{}, f)
		insertProducts(t, db)
		setMarked(t, db, "p2", 1)
		// A concurrent scan assigns the barcode to the marked p2 while the
		// lookup runs, so the add is booked on p2.
		f.before = func(ctx context.Context, code string) {
			if _, err := db.ExecContext(ctx, "INSERT INTO barcodes (code, product_id, units, created_at) VALUES (?, 'p2', 1, ?)",
				code, store.FormatTime(time.Now())); err != nil {
				t.Errorf("insert barcode during lookup: %v", err)
			}
		}

		result, _ := decodeMovementResult(t, post(h, "/api/v1/movements", `{"barcode": "4006381333931", "kind": "add"}`))

		checkFields(t, result, `{"product_created": false}`)
		product, _ := result["product"].(map[string]any)
		checkFields(t, product, `{"id": "p2"}`)
		checkMarked(t, h, db, product, false)
	})
}

func TestReverseMovementKeepsMarking(t *testing.T) {
	// See insertProducts: p2 "kidneybohnen" has stock 2.
	tests := []struct {
		name string
		// marked is set before the booking, markAgain after it.
		marked, markAgain bool
		body              string
		want              bool
	}{
		{"consume while marked", true, false, `{"product_id": "p2", "kind": "consume"}`, true},
		{"inventory while not marked", false, false, `{"product_id": "p2", "kind": "inventory", "stock": 4}`, false},
		{"add that ended the marking", true, false, `{"product_id": "p2", "kind": "add"}`, false},
		{"add, then marked again", false, true, `{"barcode": "0034000470693", "kind": "add"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)
			if tt.marked {
				setMarked(t, db, "p2", 1)
			}
			id := bookMovement(t, h, tt.body)
			if tt.markAgain {
				setMarked(t, db, "p2", 1)
			}

			result, _ := decodeMovementResult(t, reverse(h, id))

			product, _ := result["product"].(map[string]any)
			checkMarked(t, h, db, product, tt.want)
		})
	}
}

func TestMergeProductMarked(t *testing.T) {
	// See insertProducts: p2 has stock 2, p3 has stock 0, p4 has stock 2
	// which is set to 0 for a merge without stock.
	tests := []struct {
		name                       string
		source, target             string
		sourceStock                int
		sourceMarked, targetMarked bool
		want                       bool
		// updated reports whether the updated_at of the target changes.
		updated bool
	}{
		{"source marked", "p2", "p3", 2, true, false, true, true},
		{"target marked", "p2", "p3", 2, false, true, true, true},
		{"both marked", "p2", "p3", 2, true, true, true, true},
		{"neither marked", "p2", "p3", 2, false, false, false, true},
		{"source without stock marked", "p4", "p2", 0, true, false, true, true},
		{"source without stock and target marked", "p4", "p2", 0, false, true, true, false},
		{"source without stock and both marked", "p4", "p2", 0, true, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)
			setStock(t, db, tt.source, tt.sourceStock)
			if tt.sourceMarked {
				setMarked(t, db, tt.source, 1)
			}
			if tt.targetMarked {
				setMarked(t, db, tt.target, 1)
			}
			before := decodeProduct(t, get(h, "/api/v1/products/"+tt.target))
			start := time.Now().UTC().Truncate(time.Millisecond)

			got := decodeProduct(t, mergeProduct(h, tt.source, mergeInto(tt.target)))

			checkFields(t, got, fmt.Sprintf(`{"id": %q}`, tt.target))
			checkMarked(t, h, db, got, tt.want)
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
