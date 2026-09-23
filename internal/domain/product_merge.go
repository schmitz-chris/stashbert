package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// MergeProduct merges the product with sourceID into the product with
// targetID (architecture.md 6.5, Merge) and returns the target. In one
// transaction it moves the barcodes and movements of the source to the
// target, books the stock of the source on the target (see mergeStock) and
// deletes the source. The other fields of the target stay as they are.
//
// After the commit it publishes the events of the merge movement to pub (see
// publishMovementEvents), if there is one. Then it finishes the deletion of
// the source like DeleteProduct: it removes the image file of the source from
// imageDir and publishes shopping.changed with missing_after 0 if the source
// had missing > 0 (architecture.md, 6.6).
//
// sourceID equal to targetID results in 400 invalid_request before the
// database is accessed; an unknown source or target results in 404
// not_found. Nothing is changed then.
func MergeProduct(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, imageDir, sourceID, targetID string) (Product, error) {
	if sourceID == targetID {
		return Product{}, httpx.BadRequest("Quelle und Ziel sind dasselbe Produkt")
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Product{}, fmt.Errorf("merge product: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	source, err := q.GetProduct(ctx, sourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return Product{}, fmt.Errorf("merge product %s: %w", sourceID, err)
	}
	target, err := q.GetProduct(ctx, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, httpx.NotFound("Zielprodukt nicht gefunden")
	}
	if err != nil {
		return Product{}, fmt.Errorf("merge product %s: target %s: %w", sourceID, targetID, err)
	}

	if err := q.MoveProductBarcodes(ctx, db.MoveProductBarcodesParams{TargetID: target.ID, SourceID: source.ID}); err != nil {
		return Product{}, fmt.Errorf("merge product %s: move barcodes to %s: %w", source.ID, target.ID, err)
	}
	if err := q.MoveProductMovements(ctx, db.MoveProductMovementsParams{TargetID: target.ID, SourceID: source.ID}); err != nil {
		return Product{}, fmt.Errorf("merge product %s: move movements to %s: %w", source.ID, target.ID, err)
	}
	p, m, err := mergeStock(ctx, q, source.Stock, target)
	if err != nil {
		return Product{}, fmt.Errorf("merge product %s: %w", source.ID, err)
	}
	if _, err := q.DeleteProduct(ctx, source.ID); err != nil {
		return Product{}, fmt.Errorf("merge product %s: delete: %w", source.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf("merge product %s: commit: %w", source.ID, err)
	}

	if m != nil {
		publishMovementEvents(ctx, pub, target, p, *m)
	}
	productDeleted(ctx, pub, imageDir, source)
	return p, nil
}

// mergeStock books stock, the stock of the merged source, on the stored
// product target and returns the target with its barcodes. A stock above 0
// results in a movement of kind merge with delta stock, the new stock of the
// target as stock_after and neither barcode nor reverses_id; the new stock
// and updated_at are stored at the target, and the movement is returned.
// Otherwise nothing is booked and target stays unchanged, including its
// updated_at; the movement is nil then.
func mergeStock(ctx context.Context, q *db.Queries, stock int64, target db.Product) (Product, *Movement, error) {
	if stock <= 0 {
		barcodes, err := q.ListProductBarcodes(ctx, target.ID)
		if err != nil {
			return Product{}, nil, fmt.Errorf("list barcodes of product %s: %w", target.ID, err)
		}
		p, err := ProductFromDB(target, barcodes)
		return p, nil, err
	}

	now := store.FormatTime(time.Now())
	row, err := q.InsertMovement(ctx, db.InsertMovementParams{
		ID:         uuid.Must(uuid.NewV7()).String(),
		ProductID:  target.ID,
		Kind:       "merge",
		Delta:      stock,
		StockAfter: target.Stock + stock,
		CreatedAt:  now,
	})
	if err != nil {
		return Product{}, nil, fmt.Errorf("insert merge movement for product %s: %w", target.ID, err)
	}
	p, err := updateStock(ctx, q, target.ID, row.StockAfter, now)
	if err != nil {
		return Product{}, nil, err
	}
	m, err := movementFromDB(row)
	if err != nil {
		return Product{}, nil, err
	}
	return p, &m, nil
}
