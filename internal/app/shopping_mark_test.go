package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// markItem sends POST /api/v1/shopping-list/items with the JSON body to h.
func markItem(h http.Handler, body string) *httptest.ResponseRecorder {
	return post(h, "/api/v1/shopping-list/items", body)
}

// unmarkItem sends DELETE /api/v1/shopping-list/items/{id} to h.
func unmarkItem(h http.Handler, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/shopping-list/items/"+id, nil))
	return rec
}

// decodeMarkResult asserts that rec is a 200 response with JSON and returns
// the decoded MarkResult and its product.
func decodeMarkResult(t *testing.T, rec *httptest.ResponseRecorder) (result, product map[string]any) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body.String(), err)
	}
	product, ok := result["product"].(map[string]any)
	if !ok {
		t.Fatalf("product = %v, want an object", result["product"])
	}
	return result, product
}

// checkUpdatedSince asserts that updated_at of product lies between start
// and now.
func checkUpdatedSince(t *testing.T, product map[string]any, start time.Time) {
	t.Helper()
	s, _ := product["updated_at"].(string)
	if updatedAt, err := time.Parse(time.RFC3339, s); err != nil || updatedAt.Before(start) || updatedAt.After(time.Now()) {
		t.Errorf("updated_at = %v, want the time of the request", product["updated_at"])
	}
}

// shoppingItem returns the item of the product id on the shopping list of h,
// or nil if the product is not on it.
func shoppingItem(t *testing.T, h http.Handler, id string) map[string]any {
	t.Helper()
	body := getShoppingList(t, h)
	var list struct{ Items []map[string]any }
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	for _, it := range list.Items {
		if it["product_id"] == id {
			return it
		}
	}
	return nil
}

// checkNoEvents asserts that recorder holds no events.
func checkNoEvents(t *testing.T, recorder *events.Recorder) {
	t.Helper()
	if recorded := recorder.Events(); len(recorded) != 0 {
		t.Errorf("events = %+v, want none", recorded)
	}
}

func TestMarkShoppingItem(t *testing.T) {
	// See insertProducts: p1 "Zucker" has stock 0 and target 0, p4 "Mehl"
	// stock 2, target 4 and min_stock 1, so nothing is missing of both. p2
	// "kidneybohnen" misses 3 and p3 "Kidneybohnen" misses 4.
	tests := []struct {
		name string
		body string
		id   string
		// marked is set before the request.
		marked  bool
		result  string
		missing float64
	}{
		{"by product_id", `{"product_id": "p1"}`, "p1", false,
			`{"already_listed": false, "message": "Vorgemerkt: Zucker"}`, 0},
		{"by barcode", `{"barcode": "3017620422003"}`, "p4", false,
			`{"already_listed": false, "message": "Vorgemerkt: Mehl"}`, 0},
		{"missing by product_id", `{"product_id": "p3"}`, "p3", false,
			`{"already_listed": true, "message": "Schon auf der Liste: Kidneybohnen"}`, 4},
		{"missing by barcode", `{"barcode": "4001686301265"}`, "p2", false,
			`{"already_listed": true, "message": "Schon auf der Liste: kidneybohnen"}`, 3},
		{"missing by UPC-A barcode with 6 units", `{"barcode": "034000470693"}`, "p2", false,
			`{"already_listed": true, "message": "Schon auf der Liste: kidneybohnen"}`, 3},
		{"marked by product_id", `{"product_id": "p4"}`, "p4", true,
			`{"already_listed": true, "message": "Schon auf der Liste: Mehl"}`, 0},
		{"marked by barcode", `{"barcode": "3017620422003"}`, "p4", true,
			`{"already_listed": true, "message": "Schon auf der Liste: Mehl"}`, 0},
		{"marked and missing", `{"product_id": "p2"}`, "p2", true,
			`{"already_listed": true, "message": "Schon auf der Liste: kidneybohnen"}`, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			f := &fakeLookuper{result: goldbaeren}
			h, db := newAppWithLookuper(t, &recorder, f)
			insertProducts(t, db)
			if tt.marked {
				setMarked(t, db, tt.id, 1)
			}
			before := decodeProduct(t, get(h, "/api/v1/products/"+tt.id))
			start := time.Now().UTC().Truncate(time.Millisecond)

			result, product := decodeMarkResult(t, markItem(h, tt.body))

			checkFields(t, result, `{"product_created": false}`)
			checkFields(t, result, tt.result)
			checkMarked(t, h, db, product, true)
			// Only marked and, if marked changed, updated_at differ from the
			// product before: the stock stays.
			want := maps.Clone(before)
			want["marked"] = true
			if !tt.marked {
				checkUpdatedSince(t, product, start)
				want["updated_at"] = product["updated_at"]
			}
			if !reflect.DeepEqual(product, want) {
				t.Errorf("product = %v\nwant %v", product, want)
			}
			item := shoppingItem(t, h, tt.id)
			checkFields(t, item, fmt.Sprintf(`{"marked": true, "missing": %v}`, tt.missing))
			if n := countRows(t, db, "movements"); n != 0 {
				t.Errorf("movements = %d, want 0", n)
			}
			if calls := f.calls(); len(calls) != 0 {
				t.Errorf("lookups = %q, want none", calls)
			}
			checkNoEvents(t, &recorder)
		})
	}
}

func TestMarkShoppingItemTwice(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	first, product := decodeMarkResult(t, markItem(h, `{"product_id": "p4"}`))
	checkFields(t, first, `{"product_created": false, "already_listed": false, "message": "Vorgemerkt: Mehl"}`)

	second, again := decodeMarkResult(t, markItem(h, `{"barcode": "3017620422003"}`))

	checkFields(t, second, `{"product_created": false, "already_listed": true, "message": "Schon auf der Liste: Mehl"}`)
	// Nothing changes any more, not even updated_at.
	if !reflect.DeepEqual(again, product) {
		t.Errorf("product = %v\nwant unchanged %v", again, product)
	}
	checkMarked(t, h, db, again, true)
	if n := countRows(t, db, "movements"); n != 0 {
		t.Errorf("movements = %d, want 0", n)
	}
}

func TestMarkShoppingItemUnknownBarcode(t *testing.T) {
	const code = "4006381333931"
	// cached differs from the result of the lookuper.
	cached := lookup.Result{Found: true, Name: "Duschgel", PackageSize: "250 ml", ProductType: "beauty"}
	tests := []struct {
		name    string
		barcode string
		code    string
		result  lookup.Result
		err     error
		// cache is stored as a cache entry of code before the request.
		cache *lookup.Result
		// lookup reports whether the lookuper is called; its result is cached
		// unless err is set.
		lookup  bool
		product string
	}{
		{"found", code, code, goldbaeren, nil, nil, true, goldbaerenProduct},
		{"UPC-A is looked up normalized", "036000291452", "0036000291452", goldbaeren, nil, nil, true, goldbaerenProduct},
		{"not found", code, code, lookup.Result{}, nil, nil, true, placeholderProduct(code, "not_found")},
		{"unavailable", code, code, lookup.Result{}, fmt.Errorf("%w: status 503", lookup.ErrUnavailable), nil,
			true, placeholderProduct(code, "pending")},
		{"timeout", code, code, lookup.Result{}, context.DeadlineExceeded, nil, true, placeholderProduct(code, "pending")},
		{"cached", code, code, goldbaeren, nil, &cached, false, `{"name": "Duschgel", "brand": null,
			"package_size": "250 ml", "origin": "openbeautyfacts", "lookup_state": "done"}`},
		{"local code", "2000000000121", "2000000000121", goldbaeren, nil, nil, false,
			placeholderProduct("2000000000121", "none")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			f := &fakeLookuper{result: tt.result, err: tt.err}
			h, db := newAppWithLookuper(t, &recorder, f)
			if tt.cache != nil {
				insertLookup(t, db, tt.code, *tt.cache, time.Minute)
			}
			start := time.Now().UTC().Truncate(time.Millisecond)

			result, product := decodeMarkResult(t, markItem(h, fmt.Sprintf(`{"barcode": %q}`, tt.barcode)))

			checkFields(t, product, tt.product)
			checkFields(t, product, fmt.Sprintf(`{"stock": 0, "target": 0, "min_stock": null, "missing": 0,
				"note": null, "needs_review": true, "has_image": false, "barcodes": [{"code": %q, "units": 1}]}`, tt.code))
			checkFields(t, result, fmt.Sprintf(`{"product_created": true, "already_listed": false, "message": %q}`,
				fmt.Sprintf("Neu vorgemerkt: %s", product["name"])))
			checkMarked(t, h, db, product, true)
			checkUpdatedSince(t, product, start)
			if product["created_at"] != product["updated_at"] {
				t.Errorf("created_at, updated_at = %v, %v, want equal", product["created_at"], product["updated_at"])
			}
			id, _ := product["id"].(string)
			checkFields(t, shoppingItem(t, h, id), `{"marked": true, "missing": 0, "stock": 0}`)
			if n := countRows(t, db, "products"); n != 1 {
				t.Errorf("products = %d, want 1", n)
			}
			if n := countRows(t, db, "movements"); n != 0 {
				t.Errorf("movements = %d, want 0", n)
			}

			var wantCalls []string
			if tt.lookup {
				wantCalls = []string{tt.code}
			}
			if calls := f.calls(); !slices.Equal(calls, wantCalls) {
				t.Errorf("lookups = %q, want %q", calls, wantCalls)
			}
			f.checkDeadlines(t)
			switch {
			case tt.lookup && tt.err == nil:
				checkLookupCached(t, db, tt.code, tt.result, start)
			case tt.cache != nil:
				checkLookupCached(t, db, tt.code, *tt.cache, time.Time{})
			default:
				if n := countRows(t, db, "lookups"); n != 0 {
					t.Errorf("cached lookups = %d, want 0", n)
				}
			}

			// Only product.created, no movement events.
			origin, _ := product["origin"].(string)
			name, _ := product["name"].(string)
			want := events.ProductCreatedData{ProductID: id, Name: name, Origin: origin}
			recorded := recorder.Events()
			if len(recorded) != 1 || recorded[0].Type != events.TypeProductCreated ||
				recorded[0].Source != events.Source || recorded[0].Data != want {
				t.Errorf("events = %+v, want only %s %#v", recorded, events.TypeProductCreated, want)
			}
		})
	}
}

func TestMarkShoppingItemUnknownBarcodeStoredDuringLookup(t *testing.T) {
	var recorder events.Recorder
	f := &fakeLookuper{result: goldbaeren}
	h, db := newAppWithLookuper(t, &recorder, f)
	insertProducts(t, db)
	// A concurrent scan assigns the barcode to p4 "Mehl" (stock 2, nothing
	// missing) while the lookup runs. The insert only succeeds within the
	// time budget of the lookup if marking holds no write lock meanwhile.
	f.before = func(ctx context.Context, code string) {
		if _, err := db.ExecContext(ctx, "INSERT INTO barcodes (code, product_id, units, created_at) VALUES (?, 'p4', 1, ?)",
			code, store.FormatTime(time.Now())); err != nil {
			t.Errorf("insert barcode during lookup: %v", err)
		}
	}
	products := countRows(t, db, "products")

	result, product := decodeMarkResult(t, markItem(h, `{"barcode": "4006381333931"}`))

	checkFields(t, result, `{"product_created": false, "already_listed": false, "message": "Vorgemerkt: Mehl"}`)
	checkFields(t, product, `{"id": "p4", "stock": 2}`)
	checkMarked(t, h, db, product, true)
	if n := countRows(t, db, "products"); n != products {
		t.Errorf("products = %d, want %d", n, products)
	}
	if n := countRows(t, db, "movements"); n != 0 {
		t.Errorf("movements = %d, want 0", n)
	}
	if calls := f.calls(); len(calls) != 1 {
		t.Errorf("lookups = %q, want one", calls)
	}
	checkNoEvents(t, &recorder)
}

func TestMarkShoppingItemErrors(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"both", `{"product_id": "p4", "barcode": "3017620422003"}`, http.StatusBadRequest, "invalid_request"},
		{"both with invalid barcode", `{"product_id": "p4", "barcode": "12345"}`, http.StatusBadRequest, "invalid_request"},
		{"neither", `{}`, http.StatusBadRequest, "invalid_request"},
		{"product_id is no string", `{"product_id": 4}`, http.StatusBadRequest, "invalid_request"},
		{"wrong check digit", `{"barcode": "4006381333932"}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"wrong length", `{"barcode": "12345"}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"unknown product_id", `{"product_id": "unbekannt"}`, http.StatusNotFound, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			f := &fakeLookuper{result: goldbaeren}
			h, db := newAppWithLookuper(t, &recorder, f)
			insertProducts(t, db)
			before := get(h, "/api/v1/products").Body.String()

			rec := markItem(h, tt.body)

			checkProblemCode(t, rec, tt.status, tt.code)
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant unchanged %s", after, before)
			}
			for _, table := range []string{"movements", "lookups"} {
				if n := countRows(t, db, table); n != 0 {
					t.Errorf("%s = %d, want 0", table, n)
				}
			}
			if calls := f.calls(); len(calls) != 0 {
				t.Errorf("lookups = %q, want none", calls)
			}
			checkNoEvents(t, &recorder)
		})
	}
}

func TestMarkShoppingItemEndedByAdd(t *testing.T) {
	t.Run("known product", func(t *testing.T) {
		h, db := newApp(t)
		insertProducts(t, db)
		decodeMarkResult(t, markItem(h, `{"barcode": "3017620422003"}`))
		checkShoppingList(t, h, `{"items": [
			{"product_id": "p2", "name": "kidneybohnen", "brand": "Bonduelle", "missing": 3, "stock": 2, "target": 5, "marked": false},
			{"product_id": "p3", "name": "Kidneybohnen", "brand": null, "missing": 4, "stock": 0, "target": 4, "marked": false},
			{"product_id": "p4", "name": "Mehl", "brand": null, "missing": 0, "stock": 2, "target": 4, "marked": true}
		]}`)

		result, _ := decodeMovementResult(t, post(h, "/api/v1/movements", `{"barcode": "3017620422003", "kind": "add"}`))

		product, _ := result["product"].(map[string]any)
		checkFields(t, product, `{"id": "p4", "stock": 3}`)
		checkMarked(t, h, db, product, false)
		if item := shoppingItem(t, h, "p4"); item != nil {
			t.Errorf("shopping item p4 = %v, want none", item)
		}
	})

	t.Run("created product", func(t *testing.T) {
		f := &fakeLookuper{result: goldbaeren}
		h, db := newAppWithLookuper(t, events.Nop{}, f)
		_, marked := decodeMarkResult(t, markItem(h, `{"barcode": "2000000000121"}`))

		result, _ := decodeMovementResult(t, post(h, "/api/v1/movements", `{"barcode": "2000000000121", "kind": "add"}`))

		checkFields(t, result, `{"product_created": false, "message": "Neues Produkt 2000000000121 0 → 1"}`)
		product, _ := result["product"].(map[string]any)
		checkFields(t, product, fmt.Sprintf(`{"id": %q, "stock": 1}`, marked["id"]))
		checkMarked(t, h, db, product, false)
		id, _ := product["id"].(string)
		if item := shoppingItem(t, h, id); item != nil {
			t.Errorf("shopping item %s = %v, want none", id, item)
		}
	})
}

func TestUnmarkShoppingItem(t *testing.T) {
	// See insertProducts: nothing is missing of p4 "Mehl", p2
	// "kidneybohnen" misses 3.
	tests := []struct {
		name string
		id   string
		// marked reports whether the product is marked before the request.
		marked bool
		// listed reports whether the product stays on the shopping list.
		listed bool
	}{
		{"marked", "p4", true, false},
		{"marked and missing", "p2", true, true},
		{"not marked", "p4", false, false},
		{"not marked and missing", "p2", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithPublisher(t, &recorder)
			insertProducts(t, db)
			if tt.marked {
				decodeMarkResult(t, markItem(h, fmt.Sprintf(`{"product_id": %q}`, tt.id)))
			}
			before := decodeProduct(t, get(h, "/api/v1/products/"+tt.id))
			start := time.Now().UTC().Truncate(time.Millisecond)

			rec := unmarkItem(h, tt.id)

			if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
				t.Fatalf("status = %d, body %q, want %d without body", rec.Code, rec.Body.String(), http.StatusNoContent)
			}
			after := decodeProduct(t, get(h, "/api/v1/products/"+tt.id))
			checkMarked(t, h, db, after, false)
			// Only marked and, if it changed, updated_at differ.
			want := maps.Clone(before)
			want["marked"] = false
			if tt.marked {
				checkUpdatedSince(t, after, start)
				want["updated_at"] = after["updated_at"]
			}
			if !reflect.DeepEqual(after, want) {
				t.Errorf("product = %v\nwant %v", after, want)
			}
			item := shoppingItem(t, h, tt.id)
			if tt.listed {
				checkFields(t, item, `{"marked": false, "missing": 3}`)
			} else if item != nil {
				t.Errorf("shopping item %s = %v, want none", tt.id, item)
			}
			if n := countRows(t, db, "movements"); n != 0 {
				t.Errorf("movements = %d, want 0", n)
			}
			checkNoEvents(t, &recorder)
		})
	}
}

func TestUnmarkShoppingItemUnknownProduct(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)

	rec := unmarkItem(h, "unbekannt")

	checkProblemCode(t, rec, http.StatusNotFound, "not_found")
}
