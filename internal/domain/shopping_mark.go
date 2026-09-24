package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// NewMark is a product to mark for shopping (architecture.md 6.5,
// MarkCreate). Exactly one of ProductID and Barcode must be set.
type NewMark struct {
	ProductID *string
	// Barcode is a barcode of the product as entered, not yet normalized.
	Barcode *string
}

// MarkResult is the result of marking a product for shopping
// (architecture.md 6.5, MarkResult).
type MarkResult struct {
	Product        Product
	ProductCreated bool
	// AlreadyListed reports whether the product was on the shopping list
	// before, because something was missing or it was marked.
	AlreadyListed bool
	Message       string
}

// Mark marks the product with in.ProductID or the product of the barcode
// in.Barcode for shopping (ADR-0015) in one transaction. It stores marked and
// updated_at only if the product was not marked yet, never changes the stock
// and books no movement. The message is "Vorgemerkt: <name>", or "Schon auf
// der Liste: <name>" if the product was on the shopping list before.
// Otherwise marking puts it on the list, and after the commit
// shopping.changed is published to pub with missing_before and
// missing_after both the current missing (architecture.md, 6.6).
//
// An unknown barcode creates a product for it like Book does for add
// (architecture.md, 7.2), but without a movement: lookupProduct determines
// the product outside of any transaction, then one transaction stores it
// with needs_review, target 0 and stock 0, its barcode with 1 unit and the
// cache entry of the lookup, and marks it. If the barcode is known by then,
// its product is marked. A created product results in product_created and
// the message "Neu vorgemerkt: <name>"; after the commit, product.created is
// published to pub before shopping.changed. Marking publishes no other
// events.
//
// Invalid input results in an *httpx.Error and nothing is changed: 400
// invalid_request if not exactly one of product_id and barcode is set, 422
// invalid_barcode for an invalid barcode and 404 not_found for an unknown
// product.
func Mark(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, lookuper Lookuper, in NewMark) (MarkResult, error) {
	if err := checkProductOrBarcode(in.ProductID, in.Barcode); err != nil {
		return MarkResult{}, err
	}
	code, err := normalizeOptionalBarcode(in.Barcode)
	if err != nil {
		return MarkResult{}, err
	}

	r, err := mark(ctx, sqlDB, pub, in.ProductID, code, nil)
	if !isUnknownBarcode(err) {
		return r, err
	}
	s, err := lookupProduct(ctx, sqlDB, lookuper, *code)
	if err != nil {
		return MarkResult{}, err
	}
	return mark(ctx, sqlDB, pub, in.ProductID, code, &s)
}

// mark marks the product with productID or of the normalized barcode code
// within one transaction as described for Mark. code is nil for productID.
// An unknown barcode results in 404 unknown_barcode if s is nil; otherwise
// mark creates the product s for it.
func mark(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, productID, code *string, s *scannedProduct) (MarkResult, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return MarkResult{}, fmt.Errorf("mark product: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	now := store.FormatTime(time.Now())
	cur, _, err := movementProduct(ctx, q, productID, code)
	created := false
	if s != nil && isUnknownBarcode(err) {
		cur, created, err = insertScannedProduct(ctx, q, *s, *code, now)
		if err == nil && !created {
			// A concurrent request has stored the barcode since it was read
			// above: mark its product after the rollback.
			if rbErr := tx.Rollback(); rbErr != nil {
				return MarkResult{}, fmt.Errorf("mark product: rollback: %w", rbErr)
			}
			return mark(ctx, sqlDB, pub, productID, code, nil)
		}
	}
	if err != nil {
		return MarkResult{}, err
	}

	listed := storedListing(cur).onList()
	row := cur
	if cur.Marked == 0 {
		row, err = q.SetProductMarked(ctx, db.SetProductMarkedParams{Marked: 1, UpdatedAt: now, ID: cur.ID})
		if err != nil {
			return MarkResult{}, fmt.Errorf("mark product %s: %w", cur.ID, err)
		}
	}
	barcodes, err := q.ListProductBarcodes(ctx, cur.ID)
	if err != nil {
		return MarkResult{}, fmt.Errorf("mark product: list barcodes of product %s: %w", cur.ID, err)
	}
	p, err := ProductFromDB(row, barcodes)
	if err != nil {
		return MarkResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MarkResult{}, fmt.Errorf("mark product: commit: %w", err)
	}

	message := "Vorgemerkt: " + p.Name
	switch {
	case created:
		message = "Neu vorgemerkt: " + p.Name
		publishProductCreated(ctx, pub, p)
	case listed:
		message = "Schon auf der Liste: " + p.Name
	}
	publishShoppingChanged(ctx, pub, p.ID, p.Name, storedListing(cur), productListing(p))
	return MarkResult{
		Product:        p,
		ProductCreated: created,
		AlreadyListed:  listed,
		Message:        message,
	}, nil
}

// Unmark ends the marking for shopping of the product with productID
// (ADR-0015): in one transaction it stores marked 0 and updated_at. A product
// that is not marked stays unchanged, including its updated_at; something
// missing keeps it on the shopping list. If nothing is missing, unmarking
// takes it off the list, and after the commit shopping.changed is published
// to pub with missing_before and missing_after 0 (architecture.md, 6.6). An
// unknown product results in 404 not_found.
func Unmark(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, productID string) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("unmark product: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	cur, err := q.GetProduct(ctx, productID)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return fmt.Errorf("unmark product %s: %w", productID, err)
	}
	if cur.Marked == 0 {
		return nil
	}
	row, err := q.SetProductMarked(ctx, db.SetProductMarkedParams{
		Marked:    0,
		UpdatedAt: store.FormatTime(time.Now()),
		ID:        cur.ID,
	})
	if err != nil {
		return fmt.Errorf("unmark product %s: %w", cur.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("unmark product: commit: %w", err)
	}
	publishShoppingChanged(ctx, pub, row.ID, row.Name, storedListing(cur), storedListing(row))
	return nil
}
