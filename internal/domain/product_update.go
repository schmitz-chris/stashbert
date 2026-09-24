package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oapi-codegen/nullable"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// ProductPatch is a JSON Merge Patch of a product (architecture.md 6.1 and
// 6.5, ProductPatch). A nil pointer or an unspecified Nullable leaves its
// field unchanged; a null Nullable clears the field.
type ProductPatch struct {
	Name        *string
	Brand       nullable.Nullable[string]
	PackageSize nullable.Nullable[string]
	Target      *int64
	MinStock    nullable.Nullable[int64]
	Note        nullable.Nullable[string]
	CrateSize   nullable.Nullable[int64]
}

// UpdateProduct applies patch to the product with id and sets its updated_at,
// in one transaction. needs_review becomes false if patch contains name, brand
// or package_size. If missing changes, UpdateProduct publishes
// shopping.changed to pub after the commit.
//
// Name and texts are trimmed like in CreateProduct; empty texts are stored as
// NULL. An unknown id results in 404 not_found; a text of the wrong length or
// min_stock > target after the change results in 400 invalid_request.
func UpdateProduct(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, id string, patch ProductPatch) (Product, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Product{}, fmt.Errorf("update product: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	cur, err := q.GetProduct(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return Product{}, fmt.Errorf("update product %s: %w", id, err)
	}
	arg, err := applyPatch(cur, patch)
	if err != nil {
		return Product{}, err
	}
	arg.UpdatedAt = store.FormatTime(time.Now())
	row, err := q.UpdateProduct(ctx, arg)
	if err != nil {
		return Product{}, fmt.Errorf("update product %s: %w", id, err)
	}
	barcodes, err := q.ListProductBarcodes(ctx, row.ID)
	if err != nil {
		return Product{}, fmt.Errorf("update product %s: list barcodes: %w", id, err)
	}
	p, err := ProductFromDB(row, barcodes)
	if err != nil {
		return Product{}, err
	}
	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf("update product %s: commit: %w", id, err)
	}

	if before := Missing(cur.Stock, cur.Target, cur.MinStock); before != p.Missing {
		pub.Publish(ctx, events.New(events.TypeShoppingChanged, events.ShoppingChangedData{
			ProductID:     p.ID,
			Name:          p.Name,
			MissingBefore: before,
			MissingAfter:  p.Missing,
		}))
	}
	return p, nil
}

// DeleteProduct deletes the product with id in one transaction; the foreign
// keys delete its barcodes and movements. After the commit it removes the
// image file of the product from imageDir, if the product has one, and
// publishes shopping.changed with missing_after 0 to pub if the product had
// missing > 0 (architecture.md, 6.6). An unknown id results in 404 not_found.
func DeleteProduct(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, imageDir, id string) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete product: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	cur, err := q.GetProduct(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return fmt.Errorf("delete product %s: %w", id, err)
	}
	if _, err := q.DeleteProduct(ctx, id); err != nil {
		return fmt.Errorf("delete product %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete product %s: commit: %w", id, err)
	}

	productDeleted(ctx, pub, imageDir, cur)
	return nil
}

// productDeleted finishes the deletion of the stored product p after the
// commit: it removes the image file of p from imageDir, if p has one, and
// publishes shopping.changed with missing_after 0 to pub if p had missing > 0
// (architecture.md, 6.6).
func productDeleted(ctx context.Context, pub events.Publisher, imageDir string, p db.Product) {
	if p.ImageFile != nil {
		removeImage(imageDir, *p.ImageFile)
	}
	if missing := Missing(p.Stock, p.Target, p.MinStock); missing > 0 {
		pub.Publish(ctx, events.New(events.TypeShoppingChanged, events.ShoppingChangedData{
			ProductID:     p.ID,
			Name:          p.Name,
			MissingBefore: missing,
			MissingAfter:  0,
		}))
	}
}

// applyPatch returns the stored product cur with patch applied, as parameters
// for db.UpdateProduct without UpdatedAt. It checks the patched values like
// CreateProduct and min_stock against the patched target.
func applyPatch(cur db.Product, patch ProductPatch) (db.UpdateProductParams, error) {
	arg := db.UpdateProductParams{
		ID:          cur.ID,
		Name:        cur.Name,
		Target:      cur.Target,
		NeedsReview: cur.NeedsReview,
	}
	var err error
	if patch.Name != nil {
		if arg.Name, err = productName(*patch.Name); err != nil {
			return db.UpdateProductParams{}, err
		}
	}
	if arg.Brand, err = patchText("brand", cur.Brand, patch.Brand, maxBrandLength); err != nil {
		return db.UpdateProductParams{}, err
	}
	if arg.PackageSize, err = patchText("package_size", cur.PackageSize, patch.PackageSize, maxPackageSizeLength); err != nil {
		return db.UpdateProductParams{}, err
	}
	if arg.Note, err = patchText("note", cur.Note, patch.Note, maxNoteLength); err != nil {
		return db.UpdateProductParams{}, err
	}
	if patch.Target != nil {
		arg.Target = *patch.Target
	}
	arg.MinStock = patchValue(cur.MinStock, patch.MinStock)
	if arg.MinStock != nil && *arg.MinStock > arg.Target {
		return db.UpdateProductParams{}, httpx.BadRequest("min_stock darf nicht größer als target sein")
	}
	arg.CrateSize = patchValue(cur.CrateSize, patch.CrateSize)
	if patch.Name != nil || patch.Brand.IsSpecified() || patch.PackageSize.IsSpecified() {
		arg.NeedsReview = 0
	}
	return arg, nil
}

// patchValue returns cur if n is unspecified, nil if n is null and the value
// of n otherwise.
func patchValue[T any](cur *T, n nullable.Nullable[T]) *T {
	switch {
	case !n.IsSpecified():
		return cur
	case n.IsNull():
		return nil
	}
	return new(n.GetOrEmpty())
}

// patchText is like patchValue for the optional text cur of field, but checks
// a given value with optionalText.
func patchText(field string, cur *string, n nullable.Nullable[string], maxLength int) (*string, error) {
	if !n.IsSpecified() {
		return cur, nil
	}
	return optionalText(field, patchValue(cur, n), maxLength)
}
