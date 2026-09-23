package lookup_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// storedBefore is created_at and updated_at of the test products.
const storedBefore = "2026-09-01T12:00:00.000Z"

// enrichContext returns a context for a test of the enricher. Its deadline
// is later than the lookup timeout of 10 s.
func enrichContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// openDB returns a migrated database in t.TempDir().
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := enrichContext(t)
	sqlDB, err := store.Open(ctx, filepath.Join(t.TempDir(), "stashbert.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := store.Migrate(ctx, sqlDB, store.Migrations); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	return sqlDB
}

// product holds the columns of a stored product that the enricher reads or
// writes.
type product struct {
	ID             string
	Name           string
	Brand          *string
	PackageSize    *string
	Origin         string
	LookupState    string
	NeedsReview    int
	ImageSourceURL *string
	CreatedAt      string
	UpdatedAt      string
}

// placeholder returns a pending placeholder for code as created by a scan.
func placeholder(id, code string) product {
	return product{
		ID:          id,
		Name:        "Neues Produkt " + code,
		Origin:      "placeholder",
		LookupState: "pending",
		NeedsReview: 1,
		CreatedAt:   storedBefore,
		UpdatedAt:   storedBefore,
	}
}

// exec runs a statement on sqlDB.
func exec(t *testing.T, sqlDB *sql.DB, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := sqlDB.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// insertProduct stores p with the barcodes codes in this order.
func insertProduct(t *testing.T, sqlDB *sql.DB, p product, codes ...string) {
	t.Helper()
	exec(t, sqlDB, `INSERT INTO products (id, name, brand, package_size, origin, lookup_state,
		needs_review, image_source_url, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Brand, p.PackageSize, p.Origin, p.LookupState, p.NeedsReview, p.ImageSourceURL,
		p.CreatedAt, p.UpdatedAt)
	for _, code := range codes {
		exec(t, sqlDB, "INSERT INTO barcodes (code, product_id, units, created_at) VALUES (?, ?, 1, ?)",
			code, p.ID, p.CreatedAt)
	}
}

// storedProduct returns the product id, and false if there is none.
func storedProduct(t *testing.T, sqlDB *sql.DB, id string) (product, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var p product
	err := sqlDB.QueryRowContext(ctx, `SELECT id, name, brand, package_size, origin, lookup_state,
		needs_review, image_source_url, created_at, updated_at FROM products WHERE id = ?`, id).Scan(
		&p.ID, &p.Name, &p.Brand, &p.PackageSize, &p.Origin, &p.LookupState,
		&p.NeedsReview, &p.ImageSourceURL, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return product{}, false
	}
	if err != nil {
		t.Fatalf("read product %s: %v", id, err)
	}
	return p, true
}

// checkProduct asserts that the product want.ID is stored with the fields of
// want. An empty want.UpdatedAt means an updated_at at or after start.
func checkProduct(t *testing.T, sqlDB *sql.DB, want product, start time.Time) {
	t.Helper()
	got, ok := storedProduct(t, sqlDB, want.ID)
	if !ok {
		t.Fatalf("product %s is not stored", want.ID)
	}
	if want.UpdatedAt == "" {
		at, err := store.ParseTime(got.UpdatedAt)
		if err != nil || at.Before(start) || at.After(time.Now()) {
			t.Errorf("product %s: updated_at = %q, want the time of the run", want.ID, got.UpdatedAt)
		}
		want.UpdatedAt = got.UpdatedAt
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("product %s:\n got %s\nwant %s", want.ID, format(got), format(want))
	}
}

// format returns p with the values of its pointers.
func format(p product) string {
	b, _ := json.Marshal(p)
	return string(b)
}

// checkCached asserts that the cache entry of code holds result and was
// fetched at or after start.
func checkCached(t *testing.T, sqlDB *sql.DB, code string, result lookup.Result, start time.Time) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var found int
	var payload sql.NullString
	var fetchedAt string
	err := sqlDB.QueryRowContext(ctx, "SELECT found, payload, fetched_at FROM lookups WHERE code = ? AND source = 'off'",
		code).Scan(&found, &payload, &fetchedAt)
	if err != nil {
		t.Fatalf("read lookup of %s: %v", code, err)
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
		t.Errorf("cached found, payload of %s = %d, %v, want %d, %v", code, found, payload, wantFound, wantPayload)
	}
	if at, err := store.ParseTime(fetchedAt); err != nil || at.Before(start) || at.After(time.Now()) {
		t.Errorf("cached fetched_at of %s = %q, want the time of the run", code, fetchedAt)
	}
}

// countLookups returns the number of cache entries.
func countLookups(t *testing.T, sqlDB *sql.DB) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var n int
	if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM lookups").Scan(&n); err != nil {
		t.Fatalf("count lookups: %v", err)
	}
	return n
}

// fakeSource is a lookup.TryLookuper. It records the codes it is asked for
// and the time left until the deadline of each lookup context (-1 without
// deadline), then answers with answer.
type fakeSource struct {
	answer func(ctx context.Context, code string) (lookup.Result, error)

	mu        sync.Mutex
	codes     []string
	remaining []time.Duration
}

func (f *fakeSource) TryLookup(ctx context.Context, code string) (lookup.Result, error) {
	remaining := time.Duration(-1)
	if deadline, ok := ctx.Deadline(); ok {
		remaining = time.Until(deadline)
	}
	f.mu.Lock()
	f.codes = append(f.codes, code)
	f.remaining = append(f.remaining, remaining)
	f.mu.Unlock()
	return f.answer(ctx, code)
}

// calls returns the codes of all lookups so far.
func (f *fakeSource) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.codes)
}

// answering returns a fakeSource that answers every lookup with r and err.
func answering(r lookup.Result, err error) *fakeSource {
	return &fakeSource{answer: func(context.Context, string) (lookup.Result, error) { return r, err }}
}

// runOnce runs one pass of an enricher for sqlDB with src and a discarding
// logger.
func runOnce(t *testing.T, sqlDB *sql.DB, src lookup.TryLookuper) {
	t.Helper()
	if err := lookup.NewEnricher(sqlDB, src, slog.New(slog.DiscardHandler)).RunOnce(enrichContext(t)); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
}

// shower is a found Open Beauty Facts product.
var shower = lookup.Result{
	Found:       true,
	Name:        "Duschgel Sensitiv",
	Brand:       "Balea",
	PackageSize: "300 ml",
	ImageURL:    "https://images.openbeautyfacts.org/images/products/871/044/744/5990/front_de.jpg",
	ProductType: "beauty",
}

func TestRunOnceFound(t *testing.T) {
	const code = "8710447445990"
	tests := []struct {
		name   string
		stored product
		result lookup.Result
		// want is the stored product afterwards, with an updated_at of the run.
		want product
	}{
		{
			name:   "needs review",
			stored: placeholder("p1", code),
			result: shower,
			want: product{ID: "p1", Name: "Duschgel Sensitiv", Brand: new("Balea"), PackageSize: new("300 ml"),
				Origin: "openbeautyfacts", LookupState: "done", NeedsReview: 1, ImageSourceURL: new(shower.ImageURL),
				CreatedAt: storedBefore},
		},
		{
			name:   "needs review with empty texts",
			stored: placeholder("p1", code),
			result: lookup.Result{Found: true},
			want: product{ID: "p1", Name: "Neues Produkt " + code, Origin: "openfoodfacts", LookupState: "done",
				NeedsReview: 1, CreatedAt: storedBefore},
		},
		{
			name:   "needs review with long texts",
			stored: placeholder("p1", code),
			result: lookup.Result{Found: true, Name: strings.Repeat("Ä", 200), PackageSize: strings.Repeat("g", 50),
				ProductType: "petfood"},
			want: product{ID: "p1", Name: strings.Repeat("Ä", 120), PackageSize: new(strings.Repeat("g", 40)),
				Origin: "openpetfoodfacts", LookupState: "done", NeedsReview: 1, CreatedAt: storedBefore},
		},
		{
			name: "reviewed",
			stored: product{ID: "p1", Name: "Duschgel", Brand: new("Eigenmarke"), Origin: "placeholder",
				LookupState: "pending", NeedsReview: 0, CreatedAt: storedBefore, UpdatedAt: storedBefore},
			result: shower,
			want: product{ID: "p1", Name: "Duschgel", Brand: new("Eigenmarke"), Origin: "placeholder",
				LookupState: "done", NeedsReview: 0, CreatedAt: storedBefore},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB := openDB(t)
			insertProduct(t, sqlDB, tt.stored, code)
			src := answering(tt.result, nil)
			start := time.Now().UTC().Truncate(time.Millisecond)

			runOnce(t, sqlDB, src)

			if got := src.calls(); !slices.Equal(got, []string{code}) {
				t.Errorf("lookups = %v, want [%s]", got, code)
			}
			checkProduct(t, sqlDB, tt.want, start)
			checkCached(t, sqlDB, code, tt.result, start)
		})
	}
}

func TestRunOnceLookupTimeout(t *testing.T) {
	sqlDB := openDB(t)
	insertProduct(t, sqlDB, placeholder("p1", "4001686301265"), "4001686301265")
	src := answering(shower, nil)

	runOnce(t, sqlDB, src)

	src.mu.Lock()
	defer src.mu.Unlock()
	// The lower bound only allows for the time between creating the context
	// and the call of the source.
	if len(src.remaining) != 1 || src.remaining[0] <= 9*time.Second || src.remaining[0] > 10*time.Second {
		t.Errorf("lookup deadlines in %v, want one of at most 10 s", src.remaining)
	}
}

func TestRunOnceNotFound(t *testing.T) {
	const code = "4009998877669"
	sqlDB := openDB(t)
	insertProduct(t, sqlDB, placeholder("p1", code), code)
	start := time.Now().UTC().Truncate(time.Millisecond)

	runOnce(t, sqlDB, answering(lookup.Result{}, nil))

	want := placeholder("p1", code)
	want.LookupState, want.UpdatedAt = "not_found", ""
	checkProduct(t, sqlDB, want, start)
	checkCached(t, sqlDB, code, lookup.Result{}, start)
}

func TestRunOnceLookupError(t *testing.T) {
	sqlDB := openDB(t)
	failing := placeholder("p1", "4001686301265")
	insertProduct(t, sqlDB, failing, "4001686301265")
	found := placeholder("p2", "8710447445990")
	found.CreatedAt = "2026-09-02T12:00:00.000Z"
	insertProduct(t, sqlDB, found, "8710447445990")
	src := &fakeSource{answer: func(_ context.Context, code string) (lookup.Result, error) {
		if code == "4001686301265" {
			return lookup.Result{}, fmt.Errorf("%w: status 503", lookup.ErrUnavailable)
		}
		return lookup.Result{Found: true}, nil
	}}
	var logs bytes.Buffer
	start := time.Now().UTC().Truncate(time.Millisecond)

	err := lookup.NewEnricher(sqlDB, src, slog.New(slog.NewJSONHandler(&logs, nil))).RunOnce(enrichContext(t))
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// The error leaves the first product as it is; the run goes on.
	checkProduct(t, sqlDB, failing, start)
	found.Name, found.Origin, found.LookupState, found.UpdatedAt = "Neues Produkt 8710447445990", "openfoodfacts", "done", ""
	checkProduct(t, sqlDB, found, start)
	if n := countLookups(t, sqlDB); n != 1 {
		t.Errorf("lookups cached = %d, want 1 (only the found product)", n)
	}

	var record struct {
		Level     string `json:"level"`
		ProductID string `json:"product_id"`
		Code      string `json:"code"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatalf("log = %q, want one JSON record: %v", logs.String(), err)
	}
	if record.Level != "WARN" || record.ProductID != "p1" || record.Code != "4001686301265" ||
		!strings.Contains(record.Error, "status 503") {
		t.Errorf("log record = %+v, want a warning about p1", record)
	}
}

func TestRunOnceEndsRun(t *testing.T) {
	for _, sentinel := range []error{lookup.ErrRateLimited, lookup.ErrDisabled} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			sqlDB := openDB(t)
			var stored []product
			for i, code := range []string{"4001686301265", "4009998877669", "8710447445990"} {
				p := placeholder(fmt.Sprintf("p%d", i), code)
				p.CreatedAt = fmt.Sprintf("2026-09-0%dT12:00:00.000Z", i+1)
				insertProduct(t, sqlDB, p, code)
				stored = append(stored, p)
			}
			src := answering(lookup.Result{}, fmt.Errorf("%w: test", sentinel))

			runOnce(t, sqlDB, src)

			if got := src.calls(); !slices.Equal(got, []string{"4001686301265"}) {
				t.Errorf("lookups = %v, want only the first product", got)
			}
			for _, p := range stored {
				checkProduct(t, sqlDB, p, time.Time{})
			}
			if n := countLookups(t, sqlDB); n != 0 {
				t.Errorf("lookups cached = %d, want 0", n)
			}
		})
	}
}

func TestRunOnceWithoutBarcode(t *testing.T) {
	sqlDB := openDB(t)
	// All barcodes of p1 were removed.
	withoutBarcode := placeholder("p1", "4001686301265")
	insertProduct(t, sqlDB, withoutBarcode)
	withBarcode := placeholder("p2", "4009998877669")
	withBarcode.CreatedAt = "2026-09-02T12:00:00.000Z"
	insertProduct(t, sqlDB, withBarcode, "4009998877669")
	src := answering(lookup.Result{}, nil)
	start := time.Now().UTC().Truncate(time.Millisecond)

	runOnce(t, sqlDB, src)

	if got := src.calls(); !slices.Equal(got, []string{"4009998877669"}) {
		t.Errorf("lookups = %v, want only the product with barcode", got)
	}
	withoutBarcode.LookupState, withoutBarcode.UpdatedAt = "none", ""
	checkProduct(t, sqlDB, withoutBarcode, start)
}

func TestRunOnceOrder(t *testing.T) {
	sqlDB := openDB(t)
	// Products in other lookup states are older than all pending ones.
	for i, state := range []string{"none", "done", "not_found"} {
		p := placeholder(fmt.Sprintf("other%d", i), "")
		p.LookupState, p.CreatedAt = state, "2026-08-01T12:00:00.000Z"
		insertProduct(t, sqlDB, p, fmt.Sprintf("100000000000%d", i))
	}
	// 25 pending products, inserted in another order than created. Each has
	// the barcode 20000000000<minute> and the product of minute 3 also a
	// greater one, stored first.
	var want []string
	for i := range 25 {
		minute := (i * 7) % 25
		p := placeholder(fmt.Sprintf("p%02d", minute), "")
		p.CreatedAt = fmt.Sprintf("2026-09-01T12:%02d:00.000Z", minute)
		codes := []string{fmt.Sprintf("20000000000%02d", minute)}
		if minute == 3 {
			codes = []string{"3000000000003", codes[0]}
		}
		insertProduct(t, sqlDB, p, codes...)
	}
	for minute := range 20 {
		want = append(want, fmt.Sprintf("20000000000%02d", minute))
	}
	src := answering(lookup.Result{}, lookup.ErrUnavailable)

	runOnce(t, sqlDB, src)

	if got := src.calls(); !slices.Equal(got, want) {
		t.Errorf("lookups:\n got %v\nwant %v", got, want)
	}
}

func TestRunOnceProductChangedDuringLookup(t *testing.T) {
	const code = "8710447445990"
	tests := []struct {
		name string
		// change changes the product p1 while it is looked up.
		change func(t *testing.T, sqlDB *sql.DB)
		// want is the stored product afterwards, nil if it is deleted.
		want *product
	}{
		{
			name: "reviewed",
			change: func(t *testing.T, sqlDB *sql.DB) {
				exec(t, sqlDB, "UPDATE products SET name = 'Duschgel', needs_review = 0, updated_at = ? WHERE id = 'p1'",
					"2026-09-03T12:00:00.000Z")
			},
			want: &product{ID: "p1", Name: "Duschgel", Origin: "placeholder", LookupState: "done", NeedsReview: 0,
				CreatedAt: storedBefore},
		},
		{
			name: "no longer pending",
			change: func(t *testing.T, sqlDB *sql.DB) {
				exec(t, sqlDB, "UPDATE products SET lookup_state = 'none', updated_at = ? WHERE id = 'p1'",
					"2026-09-03T12:00:00.000Z")
			},
			want: &product{ID: "p1", Name: "Neues Produkt " + code, Origin: "placeholder", LookupState: "none",
				NeedsReview: 1, CreatedAt: storedBefore, UpdatedAt: "2026-09-03T12:00:00.000Z"},
		},
		{
			name: "deleted",
			change: func(t *testing.T, sqlDB *sql.DB) {
				exec(t, sqlDB, "DELETE FROM products WHERE id = 'p1'")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB := openDB(t)
			insertProduct(t, sqlDB, placeholder("p1", code), code)
			// The change runs on another connection while the source is
			// asked, so the lookup must not hold a transaction.
			src := &fakeSource{answer: func(context.Context, string) (lookup.Result, error) {
				tt.change(t, sqlDB)
				return shower, nil
			}}
			start := time.Now().UTC().Truncate(time.Millisecond)

			runOnce(t, sqlDB, src)

			if tt.want == nil {
				if _, ok := storedProduct(t, sqlDB, "p1"); ok {
					t.Error("deleted product is stored again")
				}
			} else {
				checkProduct(t, sqlDB, *tt.want, start)
			}
			checkCached(t, sqlDB, code, shower, start)
		})
	}
}

func TestRunOnceDatabaseError(t *testing.T) {
	sqlDB := openDB(t)
	sqlDB.Close()
	err := lookup.NewEnricher(sqlDB, answering(shower, nil), slog.New(slog.DiscardHandler)).RunOnce(enrichContext(t))
	if err == nil {
		t.Error("RunOnce on a closed database: err = nil, want error")
	}
}

func TestStart(t *testing.T) {
	sqlDB := openDB(t)
	insertProduct(t, sqlDB, placeholder("p1", "8710447445990"), "8710447445990")
	asked := make(chan struct{})
	var once sync.Once
	src := &fakeSource{answer: func(context.Context, string) (lookup.Result, error) {
		once.Do(func() { close(asked) })
		return lookup.Result{}, lookup.ErrRateLimited
	}}
	ctx, cancel := context.WithCancel(enrichContext(t))
	defer cancel()
	done := make(chan struct{})

	go func() {
		defer close(done)
		lookup.NewEnricher(sqlDB, src, slog.New(slog.DiscardHandler)).Start(ctx, 10*time.Millisecond)
	}()

	select {
	case <-asked:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not run a lookup within 5 s")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5 s after the end of its context")
	}
}
