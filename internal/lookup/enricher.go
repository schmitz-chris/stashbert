package lookup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// CacheSource is the source of the lookups cache; M1 only has Open Food
// Facts (architecture.md 5, lookups).
const CacheSource = "off"

const (
	// maxPendingPerRun is the number of pending products looked up per run at most.
	maxPendingPerRun = 20
	// enrichTimeout is the timeout of a lookup in the background (architecture.md, 7.3).
	enrichTimeout = 10 * time.Second
)

// TryLookuper looks up a normalized barcode only if the shared limiter has a
// free token (architecture.md, 7.3). A code that is not found results in
// Found false without an error. *Client implements it.
type TryLookuper interface {
	TryLookup(ctx context.Context, code string) (Result, error)
}

// Enricher looks up products with lookup state pending again in the
// background (architecture.md, 7.3). It publishes no events.
type Enricher struct {
	db     *sql.DB
	src    TryLookuper
	logger *slog.Logger
}

// NewEnricher returns an Enricher for the products in sqlDB that looks up
// with src and logs to logger.
func NewEnricher(sqlDB *sql.DB, src TryLookuper, logger *slog.Logger) *Enricher {
	return &Enricher{db: sqlDB, src: src, logger: logger}
}

// Start calls RunOnce at every tick of a ticker with interval until ctx ends;
// the first run starts one interval after the call. A failed run is logged
// with level error and the next tick runs again. Start blocks until ctx ends.
func (e *Enricher) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// A run that fails because ctx ended is no error of the job.
			if err := e.RunOnce(ctx); err != nil && ctx.Err() == nil {
				e.logger.LogAttrs(ctx, slog.LevelError, "enrich pending products",
					slog.String("error", err.Error()))
			}
		}
	}
}

// RunOnce looks up at most 20 products with lookup state pending, the oldest
// created_at first (architecture.md, 7.3). Each product is looked up by its
// first barcode in the order of the codes, with src.TryLookup and a timeout
// of 10 s, outside of any transaction. Then save stores the outcome:
//
//   - found: lookup state done; while the product needs review, also the
//     texts, origin and image URL of the result prepared with Finalize,
//   - not found: lookup state not_found,
//   - no barcode: lookup state none without a lookup.
//
// ErrRateLimited and ErrDisabled end the run without an error, so that scans
// keep precedence. Other lookup errors leave the product pending; they are
// logged with level warn and the run goes on. RunOnce returns an error if the
// database fails or ctx ends.
func (e *Enricher) RunOnce(ctx context.Context) error {
	q := db.New(e.db)
	ids, err := q.ListPendingProducts(ctx, maxPendingPerRun)
	if err != nil {
		return fmt.Errorf("enrich products: list pending: %w", err)
	}
	for _, id := range ids {
		code, err := q.FirstProductBarcode(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			// All barcodes were removed, so there is nothing to look up.
			if err := e.save(ctx, id, "", nil); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("enrich product %s: first barcode: %w", id, err)
		}

		lookupCtx, cancel := context.WithTimeout(ctx, enrichTimeout)
		r, err := e.src.TryLookup(lookupCtx, code)
		cancel()
		if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrDisabled) {
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("enrich product %s: %w", id, ctx.Err())
			}
			e.logger.LogAttrs(ctx, slog.LevelWarn, "look up pending product",
				slog.String("product_id", id),
				slog.String("code", code),
				slog.String("error", err.Error()),
			)
			continue
		}
		if err := e.save(ctx, id, code, &r); err != nil {
			return err
		}
	}
	return nil
}

// save stores the outcome of the lookup of the product id in one
// transaction: the cache entry of r, the result for the barcode code, and the
// update of the product from lookupUpdate. r is nil for a product without
// barcode; it has no cache entry. The product is read again within the
// transaction: if it was deleted or is no longer pending, it stays as it is.
func (e *Enricher) save(ctx context.Context, id, code string, r *Result) error {
	now := time.Now()
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("enrich product %s: begin transaction: %w", id, err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	if r != nil {
		entry, err := CacheEntry(*r, code, now)
		if err != nil {
			return err
		}
		if err := q.UpsertLookup(ctx, entry); err != nil {
			return fmt.Errorf("enrich product %s: cache lookup of %s: %w", id, code, err)
		}
	}
	cur, err := q.GetProduct(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Deleted during the lookup.
	case err != nil:
		return fmt.Errorf("enrich product %s: %w", id, err)
	case cur.LookupState == "pending":
		if err := q.UpdateProductLookup(ctx, lookupUpdate(cur, code, r, store.FormatTime(now))); err != nil {
			return fmt.Errorf("enrich product %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("enrich product %s: commit: %w", id, err)
	}
	return nil
}

// lookupUpdate returns the update of the pending product cur for r, the
// result of the lookup of its barcode code, or for r nil if cur has no
// barcode (architecture.md, 7.3). now is the formatted time of the update.
// Only a found result that is stored while cur needs review replaces name,
// brand, package size, origin and image URL; it is prepared with Finalize and
// empty texts become NULL.
func lookupUpdate(cur db.Product, code string, r *Result, now string) db.UpdateProductLookupParams {
	arg := db.UpdateProductLookupParams{
		Name:           cur.Name,
		Brand:          cur.Brand,
		PackageSize:    cur.PackageSize,
		Origin:         cur.Origin,
		ImageSourceUrl: cur.ImageSourceUrl,
		UpdatedAt:      now,
		ID:             cur.ID,
	}
	switch {
	case r == nil:
		arg.LookupState = "none"
	case !r.Found:
		arg.LookupState = "not_found"
	default:
		arg.LookupState = "done"
		if cur.NeedsReview == 1 {
			f := Finalize(*r, code)
			arg.Name = f.Name
			arg.Brand = textOrNil(f.Brand)
			arg.PackageSize = textOrNil(f.PackageSize)
			arg.Origin = Origin(f.ProductType)
			arg.ImageSourceUrl = textOrNil(f.ImageURL)
		}
	}
	return arg
}

// CacheEntry returns the entry of the lookups cache for the lookup result r
// of code, fetched at fetchedAt. The payload of a found result is r as JSON;
// a result that was not found has no payload.
func CacheEntry(r Result, code string, fetchedAt time.Time) (db.UpsertLookupParams, error) {
	entry := db.UpsertLookupParams{
		Code:      code,
		Source:    CacheSource,
		FetchedAt: store.FormatTime(fetchedAt),
	}
	if !r.Found {
		return entry, nil
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return db.UpsertLookupParams{}, fmt.Errorf("cache lookup of %s: %w", code, err)
	}
	entry.Found = 1
	entry.Payload = new(string(payload))
	return entry, nil
}

// textOrNil returns nil for an empty s and a pointer to s otherwise.
func textOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
