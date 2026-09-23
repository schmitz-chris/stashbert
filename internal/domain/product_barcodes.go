package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/schmitz-chris/stashbert/internal/gtin"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// AddBarcode assigns b to the product with productID in one transaction and
// returns the stored barcode. The code is normalized with gtin.Normalize.
//
// An invalid code results in 422 invalid_barcode, an unknown product in 404
// not_found and a code that already belongs to a product, including this
// one, in 409 barcode_in_use.
func AddBarcode(ctx context.Context, sqlDB *sql.DB, productID string, b Barcode) (Barcode, error) {
	code, err := normalizeBarcode(b.Code)
	if err != nil {
		return Barcode{}, err
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Barcode{}, fmt.Errorf("add barcode: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	if _, err := q.GetProduct(ctx, productID); errors.Is(err, sql.ErrNoRows) {
		return Barcode{}, httpx.NotFound("Produkt nicht gefunden")
	} else if err != nil {
		return Barcode{}, fmt.Errorf("add barcode: product %s: %w", productID, err)
	}
	row, err := q.InsertBarcode(ctx, db.InsertBarcodeParams{
		Code:      code,
		ProductID: productID,
		Units:     b.Units,
		CreatedAt: store.FormatTime(time.Now()),
	})
	if store.IsUniqueViolation(err) {
		return Barcode{}, httpx.NewError(http.StatusConflict, "barcode_in_use",
			fmt.Sprintf("Barcode %s ist schon einem Produkt zugeordnet", code))
	}
	if err != nil {
		return Barcode{}, fmt.Errorf("add barcode %s to product %s: %w", code, productID, err)
	}
	if err := tx.Commit(); err != nil {
		return Barcode{}, fmt.Errorf("add barcode: commit: %w", err)
	}
	return Barcode{Code: row.Code, Units: row.Units}, nil
}

// RemoveBarcode removes the barcode code from the product with productID.
// The code is normalized with gtin.Normalize first. An invalid code, an
// unknown product or a code that does not belong to this product results in
// 404 not_found.
func RemoveBarcode(ctx context.Context, sqlDB *sql.DB, productID, code string) error {
	normalized, err := gtin.Normalize(code)
	if err != nil {
		return httpx.NotFound("Barcode nicht gefunden")
	}
	n, err := db.New(sqlDB).DeleteProductBarcode(ctx, db.DeleteProductBarcodeParams{
		Code:      normalized,
		ProductID: productID,
	})
	if err != nil {
		return fmt.Errorf("remove barcode %s from product %s: %w", normalized, productID, err)
	}
	if n == 0 {
		return httpx.NotFound("Barcode nicht gefunden")
	}
	return nil
}
