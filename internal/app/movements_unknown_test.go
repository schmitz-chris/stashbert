package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// fakeLookuper is a domain.Lookuper that returns result and err. It records
// the codes it is asked for and the time left until the deadline of each
// lookup context (-1 without deadline).
type fakeLookuper struct {
	result lookup.Result
	err    error
	// before, if set, runs at the start of each lookup with its context.
	before func(ctx context.Context, code string)

	mu        sync.Mutex
	codes     []string
	remaining []time.Duration
}

func (f *fakeLookuper) Lookup(ctx context.Context, code string) (lookup.Result, error) {
	remaining := time.Duration(-1)
	if deadline, ok := ctx.Deadline(); ok {
		remaining = time.Until(deadline)
	}
	f.mu.Lock()
	f.codes = append(f.codes, code)
	f.remaining = append(f.remaining, remaining)
	f.mu.Unlock()
	if f.before != nil {
		f.before(ctx, code)
	}
	return f.result, f.err
}

// calls returns the codes of all lookups so far.
func (f *fakeLookuper) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.codes)
}

// checkDeadlines asserts that each lookup had a deadline of at most 2.5 s.
// The lower bound only allows for the time between creating the context and
// the call of the lookuper.
func (f *fakeLookuper) checkDeadlines(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.remaining {
		if r <= 2*time.Second || r > 2500*time.Millisecond {
			t.Errorf("lookup %d: deadline in %v, want at most 2.5 s", i, r)
		}
	}
}

// goldbaeren is a found Open Food Facts product.
var goldbaeren = lookup.Result{
	Found:       true,
	Name:        "Goldbären",
	Brand:       "Haribo",
	PackageSize: "200 g",
	ImageURL:    "https://images.openfoodfacts.org/images/products/400/638/133/3931/front_de.jpg",
	ProductType: "food",
}

// goldbaerenProduct are the product fields created from goldbaeren.
const goldbaerenProduct = `{"name": "Goldbären", "brand": "Haribo", "package_size": "200 g",
	"origin": "openfoodfacts", "lookup_state": "done"}`

// placeholderProduct returns the product fields of a placeholder for code
// with lookupState.
func placeholderProduct(code, lookupState string) string {
	return fmt.Sprintf(`{"name": "Neues Produkt %s", "brand": null, "package_size": null,
		"origin": "placeholder", "lookup_state": %q}`, code, lookupState)
}

// storedLookup returns found, payload and fetched_at of the cache entry of
// code with source off, and false if there is none.
func storedLookup(t *testing.T, db *sql.DB, code string) (found int, payload sql.NullString, fetchedAt string, ok bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := db.QueryRowContext(ctx, "SELECT found, payload, fetched_at FROM lookups WHERE code = ? AND source = 'off'",
		code).Scan(&found, &payload, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, sql.NullString{}, "", false
	}
	if err != nil {
		t.Fatalf("read lookup of %s: %v", code, err)
	}
	return found, payload, fetchedAt, true
}

// insertLookup stores a cache entry for code with source off, fetched age ago.
func insertLookup(t *testing.T, db *sql.DB, code string, result lookup.Result, age time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var found int
	var payload *string
	if result.Found {
		found = 1
		b, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("encode payload: %v", err)
		}
		payload = new(string(b))
	}
	fetchedAt := store.FormatTime(time.Now().Add(-age))
	if _, err := db.ExecContext(ctx, "INSERT INTO lookups (code, source, found, payload, fetched_at) VALUES (?, 'off', ?, ?, ?)",
		code, found, payload, fetchedAt); err != nil {
		t.Fatalf("insert lookup of %s: %v", code, err)
	}
	return fetchedAt
}

// checkLookupCached asserts that the cache entry of code holds result and was
// fetched at or after start.
func checkLookupCached(t *testing.T, db *sql.DB, code string, result lookup.Result, start time.Time) {
	t.Helper()
	found, payload, fetchedAt, ok := storedLookup(t, db, code)
	if !ok {
		t.Fatalf("lookup of %s is not cached", code)
	}
	wantFound, wantPayload := 0, sql.NullString{}
	if result.Found {
		b, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("encode payload: %v", err)
		}
		wantFound, wantPayload = 1, sql.NullString{String: string(b), Valid: true}
	}
	if found != wantFound || payload != wantPayload {
		t.Errorf("cached found, payload = %d, %v, want %d, %v", found, payload, wantFound, wantPayload)
	}
	if at, err := time.Parse(store.TimeLayout, fetchedAt); err != nil || at.Before(start) || at.After(time.Now()) {
		t.Errorf("cached fetched_at = %q, want the time of the request", fetchedAt)
	}
}

// storedImageSourceURL returns image_source_url of the product id.
func storedImageSourceURL(t *testing.T, db *sql.DB, id string) sql.NullString {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var u sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT image_source_url FROM products WHERE id = ?", id).Scan(&u); err != nil {
		t.Fatalf("read product %s: %v", id, err)
	}
	return u
}

// checkCreatedProduct asserts that result and movement of an add of 1 created
// the only product, for code with the fields of want, stock 1 and
// needs_review, and booked on it. It returns the product.
func checkCreatedProduct(t *testing.T, db *sql.DB, result, movement map[string]any, code, want string) map[string]any {
	t.Helper()
	product, _ := result["product"].(map[string]any)
	checkFields(t, product, fmt.Sprintf(`{"stock": 1, "target": 0, "min_stock": null, "missing": 0, "note": null,
		"needs_review": true, "has_image": false, "barcodes": [{"code": %q, "units": 1}]}`, code))
	checkFields(t, product, want)
	checkFields(t, movement, fmt.Sprintf(`{"product_id": %q, "kind": "add", "delta": 1, "stock_after": 1,
		"barcode": %q, "reverses_id": null}`, product["id"], code))
	checkStoredMovement(t, db, movement)
	if n := countRows(t, db, "products"); n != 1 {
		t.Errorf("products = %d, want 1", n)
	}
	return product
}

func TestCreateMovementUnknownBarcode(t *testing.T) {
	const code = "4006381333931"
	withType := func(productType string) lookup.Result {
		r := goldbaeren
		r.ProductType = productType
		return r
	}
	withOrigin := func(origin string) string {
		return fmt.Sprintf(`{"name": "Goldbären", "origin": %q, "lookup_state": "done"}`, origin)
	}
	long := lookup.Result{
		Found:       true,
		Name:        strings.Repeat("Ä", 200),
		Brand:       strings.Repeat("b", 130),
		PackageSize: strings.Repeat("g", 50),
		ProductType: "food",
	}
	image := sql.NullString{String: goldbaeren.ImageURL, Valid: true}
	tests := []struct {
		name    string
		barcode string
		code    string
		result  lookup.Result
		err     error
		// lookup reports whether the lookuper is called; its result is cached
		// unless err is set.
		lookup   bool
		product  string
		image    sql.NullString
		warnings string
	}{
		{"local EAN-13", "2000000000121", "2000000000121", goldbaeren, nil,
			false, placeholderProduct("2000000000121", "none"), sql.NullString{}, `["placeholder_created"]`},
		{"local UPC-A", "212345678909", "0212345678909", goldbaeren, nil,
			false, placeholderProduct("0212345678909", "none"), sql.NullString{}, `["placeholder_created"]`},
		{"food", code, code, goldbaeren, nil,
			true, goldbaerenProduct, image, `[]`},
		{"UPC-A is looked up normalized", "036000291452", "0036000291452", goldbaeren, nil,
			true, goldbaerenProduct, image, `[]`},
		{"beauty", code, code, withType("beauty"), nil,
			true, withOrigin("openbeautyfacts"), image, `[]`},
		{"petfood", code, code, withType("petfood"), nil,
			true, withOrigin("openpetfoodfacts"), image, `[]`},
		{"product", code, code, withType("product"), nil,
			true, withOrigin("openproductsfacts"), image, `[]`},
		{"empty product type", code, code, withType(""), nil,
			true, withOrigin("openfoodfacts"), image, `[]`},
		{"unknown product type", code, code, withType("cosmetics"), nil,
			true, withOrigin("openfoodfacts"), image, `[]`},
		{"long texts are cut", code, code, long, nil,
			true, fmt.Sprintf(`{"name": %q, "brand": %q, "package_size": %q, "origin": "openfoodfacts", "lookup_state": "done"}`,
				strings.Repeat("Ä", 120), strings.Repeat("b", 120), strings.Repeat("g", 40)), sql.NullString{}, `[]`},
		{"empty name and texts", code, code, lookup.Result{Found: true, ProductType: "food"}, nil,
			true, `{"name": "Neues Produkt 4006381333931", "brand": null, "package_size": null,
				"origin": "openfoodfacts", "lookup_state": "done"}`, sql.NullString{}, `[]`},
		{"not found", code, code, lookup.Result{}, nil,
			true, placeholderProduct(code, "not_found"), sql.NullString{}, `["placeholder_created"]`},
		{"unavailable", code, code, lookup.Result{}, fmt.Errorf("%w: status 503", lookup.ErrUnavailable),
			true, placeholderProduct(code, "pending"), sql.NullString{}, `["placeholder_created"]`},
		{"rate limited", code, code, lookup.Result{}, fmt.Errorf("%w: status 429", lookup.ErrRateLimited),
			true, placeholderProduct(code, "pending"), sql.NullString{}, `["placeholder_created"]`},
		{"disabled", code, code, lookup.Result{}, lookup.ErrDisabled,
			true, placeholderProduct(code, "pending"), sql.NullString{}, `["placeholder_created"]`},
		{"timeout", code, code, lookup.Result{}, context.DeadlineExceeded,
			true, placeholderProduct(code, "pending"), sql.NullString{}, `["placeholder_created"]`},
		{"other error", code, code, goldbaeren, errors.New("unexpected"),
			true, placeholderProduct(code, "pending"), sql.NullString{}, `["placeholder_created"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLookuper{result: tt.result, err: tt.err}
			h, db := newAppWithLookuper(t, events.Nop{}, f)
			start := time.Now().UTC().Truncate(time.Millisecond)

			result, movement := decodeMovementResult(t, post(h, "/api/v1/movements",
				fmt.Sprintf(`{"barcode": %q, "kind": "add"}`, tt.barcode)))
			product := checkCreatedProduct(t, db, result, movement, tt.code, tt.product)
			checkFields(t, result, fmt.Sprintf(`{"product_created": true, "warnings": %s, "message": %q}`,
				tt.warnings, fmt.Sprintf("Neu: %s 0 → 1", product["name"])))
			id, _ := product["id"].(string)
			if stored := decodeProduct(t, get(h, "/api/v1/products/"+id)); !reflect.DeepEqual(stored, product) {
				t.Errorf("stored product = %v\nwant response %v", stored, product)
			}
			if u := storedImageSourceURL(t, db, id); u != tt.image {
				t.Errorf("image_source_url = %v, want %v", u, tt.image)
			}

			var wantCalls []string
			if tt.lookup {
				wantCalls = []string{tt.code}
			}
			if calls := f.calls(); !slices.Equal(calls, wantCalls) {
				t.Errorf("lookups = %q, want %q", calls, wantCalls)
			}
			f.checkDeadlines(t)
			if tt.lookup && tt.err == nil {
				checkLookupCached(t, db, tt.code, tt.result, start)
			} else if n := countRows(t, db, "lookups"); n != 0 {
				t.Errorf("cached lookups = %d, want 0", n)
			}
		})
	}
}

func TestCreateMovementUnknownBarcodeCache(t *testing.T) {
	const code = "4006381333931"
	// The cached result differs from the one of the lookuper.
	cached := lookup.Result{Found: true, Name: "Duschgel", PackageSize: "250 ml", ProductType: "beauty"}
	const cachedProduct = `{"name": "Duschgel", "brand": null, "package_size": "250 ml",
		"origin": "openbeautyfacts", "lookup_state": "done"}`
	const day = 24 * time.Hour
	tests := []struct {
		name    string
		cached  lookup.Result
		age     time.Duration
		lookup  bool
		product string
	}{
		{"found", cached, time.Minute, false, cachedProduct},
		{"found 29 days ago", cached, 29 * day, false, cachedProduct},
		{"not found", lookup.Result{}, time.Minute, false, placeholderProduct(code, "not_found")},
		{"not found 29 days ago", lookup.Result{}, 29 * day, false, placeholderProduct(code, "not_found")},
		{"found 31 days ago", cached, 31 * day, true, goldbaerenProduct},
		{"not found 31 days ago", lookup.Result{}, 31 * day, true, goldbaerenProduct},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLookuper{result: goldbaeren}
			h, db := newAppWithLookuper(t, events.Nop{}, f)
			fetchedAt := insertLookup(t, db, code, tt.cached, tt.age)
			start := time.Now().UTC().Truncate(time.Millisecond)

			result, movement := decodeMovementResult(t, post(h, "/api/v1/movements", `{"barcode": "4006381333931", "kind": "add"}`))
			checkCreatedProduct(t, db, result, movement, code, tt.product)
			checkFields(t, result, `{"product_created": true}`)
			if !tt.lookup {
				if calls := f.calls(); len(calls) != 0 {
					t.Errorf("lookups = %q, want none", calls)
				}
				// The cache entry is unchanged.
				checkLookupCached(t, db, code, tt.cached, time.Time{})
				if _, _, at, _ := storedLookup(t, db, code); at != fetchedAt {
					t.Errorf("cached fetched_at = %q, want unchanged %q", at, fetchedAt)
				}
				return
			}
			if calls := f.calls(); !slices.Equal(calls, []string{code}) {
				t.Errorf("lookups = %q, want %q", calls, []string{code})
			}
			checkLookupCached(t, db, code, goldbaeren, start)
		})
	}
}

func TestCreateMovementUnknownBarcodeEvents(t *testing.T) {
	tests := []struct {
		name    string
		barcode string
		product string
		origin  string
		result  string
	}{
		{"found", "4006381333931", "Goldbären", "openfoodfacts",
			`{"product_created": true, "warnings": [], "message": "Neu: Goldbären 0 → 3"}`},
		{"placeholder", "2000000000121", "Neues Produkt 2000000000121", "placeholder",
			`{"product_created": true, "warnings": ["placeholder_created"], "message": "Neu: Neues Produkt 2000000000121 0 → 3"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			h, db := newAppWithLookuper(t, &recorder, &fakeLookuper{result: goldbaeren})

			result, movement := decodeMovementResult(t, post(h, "/api/v1/movements",
				fmt.Sprintf(`{"barcode": %q, "kind": "add", "quantity": 3}`, tt.barcode)))

			checkFields(t, result, tt.result)
			checkFields(t, movement, `{"kind": "add", "delta": 3, "stock_after": 3}`)
			checkStoredMovement(t, db, movement)
			product, _ := result["product"].(map[string]any)
			checkFields(t, product, fmt.Sprintf(`{"name": %q, "origin": %q, "stock": 3, "target": 0, "missing": 0}`,
				tt.product, tt.origin))

			id, _ := product["id"].(string)
			movementID, _ := movement["id"].(string)
			want := []events.Event{
				{Type: events.TypeProductCreated, Data: events.ProductCreatedData{ProductID: id, Name: tt.product, Origin: tt.origin}},
				{Type: events.TypeStockAdded, Data: events.StockData{ProductID: id, MovementID: movementID, Delta: 3, StockAfter: 3}},
			}
			recorded := recorder.Events()
			if len(recorded) != len(want) {
				t.Fatalf("events = %+v, want %d events", recorded, len(want))
			}
			for i, w := range want {
				if e := recorded[i]; e.Type != w.Type || e.Source != events.Source || e.Data != w.Data {
					t.Errorf("event %d = %s %s %#v, want %s %s %#v", i, e.Type, e.Source, e.Data, w.Type, events.Source, w.Data)
				}
			}
		})
	}
}

func TestCreateMovementUnknownBarcodeWithoutAdd(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"consume", `{"barcode": "4006381333931", "kind": "consume"}`},
		{"inventory", `{"barcode": "4006381333931", "kind": "inventory", "stock": 1}`},
		{"consume local code", `{"barcode": "2000000000121", "kind": "consume"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorder events.Recorder
			f := &fakeLookuper{result: goldbaeren}
			h, db := newAppWithLookuper(t, &recorder, f)

			rec := post(h, "/api/v1/movements", tt.body)

			checkProblemCode(t, rec, http.StatusNotFound, "unknown_barcode")
			if calls := f.calls(); len(calls) != 0 {
				t.Errorf("lookups = %q, want none", calls)
			}
			for _, table := range []string{"products", "barcodes", "movements", "lookups"} {
				if n := countRows(t, db, table); n != 0 {
					t.Errorf("%s = %d, want 0", table, n)
				}
			}
			if n := len(recorder.Events()); n != 0 {
				t.Errorf("events = %d, want 0", n)
			}
		})
	}
}

func TestCreateMovementUnknownBarcodeIdempotencyKey(t *testing.T) {
	var recorder events.Recorder
	f := &fakeLookuper{result: goldbaeren}
	h, db := newAppWithLookuper(t, &recorder, f)
	const body = `{"barcode": "4006381333931", "kind": "add"}`
	first, firstMovement := decodeMovementResult(t, postWithKey(h, "/api/v1/movements", body, idempotencyKey))
	checkFields(t, first, `{"product_created": true, "message": "Neu: Goldbären 0 → 1"}`)
	published := len(recorder.Events())

	second, secondMovement := decodeMovementResult(t, postWithKey(h, "/api/v1/movements", body, idempotencyKey))

	if !reflect.DeepEqual(secondMovement, firstMovement) {
		t.Errorf("movement = %v\n    want %v", secondMovement, firstMovement)
	}
	// The reconstructed result (architecture.md, 6.1).
	checkFields(t, second, `{"product_created": false, "warnings": [], "message": "Goldbären 0 → 1"}`)
	if !reflect.DeepEqual(second["product"], first["product"]) {
		t.Errorf("product = %v\nwant %v", second["product"], first["product"])
	}
	if calls := f.calls(); len(calls) != 1 {
		t.Errorf("lookups = %q, want one", calls)
	}
	for _, table := range []string{"products", "barcodes", "movements", "lookups"} {
		if n := countRows(t, db, table); n != 1 {
			t.Errorf("%s = %d, want 1", table, n)
		}
	}
	if n := len(recorder.Events()); n != published {
		t.Errorf("events = %d, want only the %d of the first request", n, published)
	}
	id, _ := firstMovement["id"].(string)
	if key, hash := storedIdempotency(t, db, id); key.String != idempotencyKey || !hash.Valid {
		t.Errorf("idempotency_key, request_hash = %v, %v, want the key and a hash", key, hash)
	}
}

func TestCreateMovementUnknownBarcodeStoredDuringLookup(t *testing.T) {
	var recorder events.Recorder
	f := &fakeLookuper{result: goldbaeren}
	h, db := newAppWithLookuper(t, &recorder, f)
	insertProducts(t, db)
	// A concurrent scan assigns the barcode to p2 "kidneybohnen" (stock 2,
	// target 5) while the lookup runs. The insert only succeeds within the
	// time budget of the lookup if the booking holds no write lock meanwhile.
	f.before = func(ctx context.Context, code string) {
		if _, err := db.ExecContext(ctx, "INSERT INTO barcodes (code, product_id, units, created_at) VALUES (?, 'p2', 1, ?)",
			code, store.FormatTime(time.Now())); err != nil {
			t.Errorf("insert barcode during lookup: %v", err)
		}
	}
	products := countRows(t, db, "products")

	result, movement := decodeMovementResult(t, post(h, "/api/v1/movements", `{"barcode": "4006381333931", "kind": "add"}`))

	checkFields(t, result, `{"product_created": false, "warnings": [], "message": "kidneybohnen 2 → 3"}`)
	checkFields(t, movement, `{"product_id": "p2", "kind": "add", "delta": 1, "stock_after": 3, "barcode": "4006381333931"}`)
	checkStoredMovement(t, db, movement)
	if n := countRows(t, db, "products"); n != products {
		t.Errorf("products = %d, want %d", n, products)
	}
	if calls := f.calls(); len(calls) != 1 {
		t.Errorf("lookups = %q, want one", calls)
	}
	for _, e := range recorder.Events() {
		if e.Type == events.TypeProductCreated {
			t.Errorf("event %s, want none", e.Type)
		}
	}
}
