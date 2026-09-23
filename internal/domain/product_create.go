package domain

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/gtin"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// Maximum lengths of product texts in characters (architecture.md, 5).
const (
	maxNameLength        = 120
	maxBrandLength       = 120
	maxPackageSizeLength = 40
	maxNoteLength        = 500
)

// NewProduct is a product to create manually (architecture.md 6.5, ProductCreate).
type NewProduct struct {
	Name        string
	Brand       *string
	PackageSize *string
	Target      int64
	MinStock    *int64
	Note        *string
	Barcodes    []Barcode
}

// CreateProduct stores in as a product with origin manual, lookup state none
// and needs_review false, together with its barcodes, in one transaction.
// After the commit it publishes product.created to pub.
//
// Name and texts are trimmed; empty texts are stored as NULL. Barcodes are
// normalized with gtin.Normalize. Invalid input results in an *httpx.Error:
// 422 invalid_barcode for an invalid barcode, 400 invalid_request for a
// barcode given twice, min_stock > target or a text of the wrong length, and
// 409 barcode_in_use for a barcode that belongs to another product.
func CreateProduct(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, in NewProduct) (Product, error) {
	name, err := productName(in.Name)
	if err != nil {
		return Product{}, err
	}
	brand, err := optionalText("brand", in.Brand, maxBrandLength)
	if err != nil {
		return Product{}, err
	}
	packageSize, err := optionalText("package_size", in.PackageSize, maxPackageSizeLength)
	if err != nil {
		return Product{}, err
	}
	note, err := optionalText("note", in.Note, maxNoteLength)
	if err != nil {
		return Product{}, err
	}
	if in.MinStock != nil && *in.MinStock > in.Target {
		return Product{}, httpx.BadRequest("min_stock darf nicht größer als target sein")
	}
	barcodes, err := normalizeBarcodes(in.Barcodes)
	if err != nil {
		return Product{}, err
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Product{}, fmt.Errorf("create product: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	now := store.FormatTime(time.Now())
	row, err := q.InsertProduct(ctx, db.InsertProductParams{
		ID:          uuid.Must(uuid.NewV7()).String(),
		Name:        name,
		Brand:       brand,
		PackageSize: packageSize,
		Target:      in.Target,
		MinStock:    in.MinStock,
		Note:        note,
		Origin:      "manual",
		LookupState: "none",
		NeedsReview: 0,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return Product{}, fmt.Errorf("create product: %w", err)
	}
	stored := make([]db.Barcode, len(barcodes))
	for i, b := range barcodes {
		stored[i], err = q.InsertBarcode(ctx, db.InsertBarcodeParams{
			Code:      b.Code,
			ProductID: row.ID,
			Units:     b.Units,
			CreatedAt: now,
		})
		if store.IsUniqueViolation(err) {
			return Product{}, httpx.NewError(http.StatusConflict, "barcode_in_use",
				fmt.Sprintf("Barcode %s gehört schon zu einem anderen Produkt", b.Code))
		}
		if err != nil {
			return Product{}, fmt.Errorf("create product: barcode %s: %w", b.Code, err)
		}
	}
	p, err := ProductFromDB(row, stored)
	if err != nil {
		return Product{}, err
	}
	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf("create product: commit: %w", err)
	}

	pub.Publish(ctx, events.New(events.TypeProductCreated, events.ProductCreatedData{
		ProductID: p.ID,
		Name:      p.Name,
		Origin:    p.Origin,
	}))
	return p, nil
}

// productName trims name. It returns a 400 invalid_request error if the
// trimmed name does not have 1 to maxNameLength characters.
func productName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameLength {
		return "", httpx.BadRequest(fmt.Sprintf("name muss 1 bis %d Zeichen haben", maxNameLength))
	}
	return name, nil
}

// optionalText trims the optional text s of field. It returns nil if s is nil
// or empty after trimming, and a 400 invalid_request error if the trimmed
// text has more than maxLength characters.
func optionalText(field string, s *string, maxLength int) (*string, error) {
	if s == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(t) > maxLength {
		return nil, httpx.BadRequest(fmt.Sprintf("%s darf höchstens %d Zeichen haben", field, maxLength))
	}
	return &t, nil
}

// normalizeBarcodes returns barcodes with their codes normalized by
// gtin.Normalize. An invalid code results in 422 invalid_barcode, a code that
// occurs twice after normalization in 400 invalid_request.
func normalizeBarcodes(barcodes []Barcode) ([]Barcode, error) {
	out := make([]Barcode, len(barcodes))
	for i, b := range barcodes {
		code, err := normalizeBarcode(b.Code)
		if err != nil {
			return nil, err
		}
		out[i] = Barcode{Code: code, Units: b.Units}
	}
	seen := make(map[string]bool, len(out))
	for _, b := range out {
		if seen[b.Code] {
			return nil, httpx.BadRequest(fmt.Sprintf("Barcode %s ist mehrfach angegeben", b.Code))
		}
		seen[b.Code] = true
	}
	return out, nil
}

// normalizeBarcode returns code normalized by gtin.Normalize. An invalid code
// results in 422 invalid_barcode.
func normalizeBarcode(code string) (string, error) {
	normalized, err := gtin.Normalize(code)
	if err != nil {
		return "", httpx.NewError(http.StatusUnprocessableEntity, "invalid_barcode",
			fmt.Sprintf("Barcode %q ist ungültig", code))
	}
	return normalized, nil
}
