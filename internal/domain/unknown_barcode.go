package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/gtin"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// warningPlaceholderCreated reports that the product created for an unknown
// barcode is a placeholder (architecture.md, 6.3).
const warningPlaceholderCreated = "placeholder_created"

const (
	// lookupTimeout is the time budget of a lookup while booking (architecture.md, 7.2).
	lookupTimeout = 2500 * time.Millisecond
	// lookupCacheAge is the age below which a cached lookup is used (architecture.md, 7.2).
	lookupCacheAge = 30 * 24 * time.Hour
)

// Lookuper looks up a normalized barcode in an external source
// (architecture.md, 7.2). A code that is not found results in Found false
// without an error. *lookup.Client implements it.
type Lookuper interface {
	Lookup(ctx context.Context, code string) (lookup.Result, error)
}

// scannedProduct is the product to create for an unknown barcode.
type scannedProduct struct {
	name           string
	brand          *string
	packageSize    *string
	origin         string
	lookupState    string
	imageSourceURL *string
	// cache is the entry to store in the lookups cache, nil for none.
	cache *db.UpsertLookupParams
}

// lookupProduct returns the product to create for the unknown normalized
// barcode code (architecture.md, 7.2). It runs outside of any transaction.
//
// A local code results in a placeholder with lookup state none. Otherwise a
// cache entry younger than lookupCacheAge is used, also a negative one. Without
// one, lookuper looks up code within lookupTimeout; its result is cached. Any
// error of lookuper results in a placeholder with lookup state pending and no
// cache entry.
func lookupProduct(ctx context.Context, sqlDB *sql.DB, lookuper Lookuper, code string) (scannedProduct, error) {
	if gtin.IsLocal(code) {
		return placeholderProduct(code, "none"), nil
	}
	r, ok, err := cachedLookup(ctx, db.New(sqlDB), code, time.Now())
	if err != nil {
		return scannedProduct{}, err
	}
	if ok {
		return productFromLookup(r, code), nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	r, err = lookuper.Lookup(lookupCtx, code)
	if err != nil {
		// The background job looks up pending products again.
		return placeholderProduct(code, "pending"), nil
	}
	p := productFromLookup(r, code)
	entry, err := lookup.CacheEntry(r, code, time.Now())
	if err != nil {
		return scannedProduct{}, err
	}
	p.cache = &entry
	return p, nil
}

// cachedLookup returns the cached lookup result of code if it was fetched
// less than lookupCacheAge before now. A negative entry results in Found false.
func cachedLookup(ctx context.Context, q *db.Queries, code string, now time.Time) (lookup.Result, bool, error) {
	row, err := q.GetLookup(ctx, db.GetLookupParams{Code: code, Source: lookup.CacheSource})
	if errors.Is(err, sql.ErrNoRows) {
		return lookup.Result{}, false, nil
	}
	if err != nil {
		return lookup.Result{}, false, fmt.Errorf("cached lookup of %s: %w", code, err)
	}
	fetchedAt, err := store.ParseTime(row.FetchedAt)
	if err != nil {
		return lookup.Result{}, false, fmt.Errorf("cached lookup of %s: %w", code, err)
	}
	if now.Sub(fetchedAt) >= lookupCacheAge {
		return lookup.Result{}, false, nil
	}
	if row.Found == 0 {
		return lookup.Result{}, true, nil
	}
	if row.Payload == nil {
		return lookup.Result{}, false, fmt.Errorf("cached lookup of %s: found without payload", code)
	}
	var r lookup.Result
	if err := json.Unmarshal([]byte(*row.Payload), &r); err != nil {
		return lookup.Result{}, false, fmt.Errorf("cached lookup of %s: decode payload: %w", code, err)
	}
	return r, true, nil
}

// productFromLookup returns the product to create from the lookup result r of
// code (architecture.md 7.2, steps 3 and 4). A result that was not found
// results in a placeholder with lookup state not_found. A found one is
// prepared with lookup.Finalize and gets the origin of its product type and
// lookup state done; empty texts become NULL.
func productFromLookup(r lookup.Result, code string) scannedProduct {
	if !r.Found {
		return placeholderProduct(code, "not_found")
	}
	r = lookup.Finalize(r, code)
	return scannedProduct{
		name:           r.Name,
		brand:          textOrNil(r.Brand),
		packageSize:    textOrNil(r.PackageSize),
		origin:         lookup.Origin(r.ProductType),
		lookupState:    "done",
		imageSourceURL: textOrNil(r.ImageURL),
	}
}

// placeholderProduct returns a placeholder for code with lookupState: the
// name "Neues Produkt <code>" and origin placeholder.
func placeholderProduct(code, lookupState string) scannedProduct {
	return scannedProduct{
		name:        lookup.Finalize(lookup.Result{}, code).Name,
		origin:      "placeholder",
		lookupState: lookupState,
	}
}

// textOrNil returns nil for an empty s and a pointer to s otherwise.
func textOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// insertScannedProduct stores s in the transaction of q as a new product for
// the unknown normalized barcode code, with needs_review, target 0 and stock
// 0, together with the barcode with 1 unit and the cache entry of s. now is
// the formatted time of the booking or marking. It returns false without
// storing the cache entry if a concurrent request has stored the barcode
// meanwhile.
func insertScannedProduct(ctx context.Context, q *db.Queries, s scannedProduct, code, now string) (db.Product, bool, error) {
	row, err := q.InsertProduct(ctx, db.InsertProductParams{
		ID:             uuid.Must(uuid.NewV7()).String(),
		Name:           s.name,
		Brand:          s.brand,
		PackageSize:    s.packageSize,
		Target:         0,
		Origin:         s.origin,
		LookupState:    s.lookupState,
		NeedsReview:    1,
		ImageSourceUrl: s.imageSourceURL,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		return db.Product{}, false, fmt.Errorf("create product for barcode %s: %w", code, err)
	}
	_, err = q.InsertBarcode(ctx, db.InsertBarcodeParams{
		Code:      code,
		ProductID: row.ID,
		Units:     1,
		CreatedAt: now,
	})
	if store.IsUniqueViolation(err) {
		return db.Product{}, false, nil
	}
	if err != nil {
		return db.Product{}, false, fmt.Errorf("create product: barcode %s: %w", code, err)
	}
	if s.cache != nil {
		if err := q.UpsertLookup(ctx, *s.cache); err != nil {
			return db.Product{}, false, fmt.Errorf("create product: cache lookup of %s: %w", code, err)
		}
	}
	return row, true, nil
}

// publishProductCreated publishes product.created for the product p created
// for an unknown barcode to pub (architecture.md, 6.6).
func publishProductCreated(ctx context.Context, pub events.Publisher, p Product) {
	pub.Publish(ctx, events.New(events.TypeProductCreated, events.ProductCreatedData{
		ProductID: p.ID,
		Name:      p.Name,
		Origin:    p.Origin,
	}))
}

// isUnknownBarcode reports whether err is the 404 unknown_barcode error of
// movementProduct.
func isUnknownBarcode(err error) bool {
	var e *httpx.Error
	return errors.As(err, &e) && e.Code == codeUnknownBarcode
}
