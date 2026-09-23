package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// Product is a product with its derived values and barcodes (architecture.md, 6.5).
type Product struct {
	ID          string
	Name        string
	Brand       *string
	PackageSize *string
	Note        *string
	Stock       int64
	Target      int64
	MinStock    *int64
	Missing     int64
	NeedsReview bool
	Origin      string
	LookupState string
	HasImage    bool
	Barcodes    []Barcode
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Barcode is a barcode of a product.
type Barcode struct {
	Code  string
	Units int64
}

// ProductFromDB converts the stored product p and its barcodes. It computes
// Missing, sets HasImage if p has an image file and sorts the barcodes by code.
// All barcodes must belong to p.
func ProductFromDB(p db.Product, barcodes []db.Barcode) (Product, error) {
	createdAt, err := store.ParseTime(p.CreatedAt)
	if err != nil {
		return Product{}, fmt.Errorf("product %s: %w", p.ID, err)
	}
	updatedAt, err := store.ParseTime(p.UpdatedAt)
	if err != nil {
		return Product{}, fmt.Errorf("product %s: %w", p.ID, err)
	}
	codes := make([]Barcode, len(barcodes))
	for i, b := range barcodes {
		codes[i] = Barcode{Code: b.Code, Units: b.Units}
	}
	slices.SortFunc(codes, func(a, b Barcode) int { return strings.Compare(a.Code, b.Code) })

	return Product{
		ID:          p.ID,
		Name:        p.Name,
		Brand:       p.Brand,
		PackageSize: p.PackageSize,
		Note:        p.Note,
		Stock:       p.Stock,
		Target:      p.Target,
		MinStock:    p.MinStock,
		Missing:     Missing(p.Stock, p.Target, p.MinStock),
		NeedsReview: p.NeedsReview != 0,
		Origin:      p.Origin,
		LookupState: p.LookupState,
		HasImage:    p.ImageFile != nil,
		Barcodes:    codes,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

// ProductsFromDB converts the stored products like ProductFromDB and keeps
// their order. barcodes may contain the barcodes of all products; each one is
// attached to its product.
func ProductsFromDB(products []db.Product, barcodes []db.Barcode) ([]Product, error) {
	byProduct := make(map[string][]db.Barcode)
	for _, b := range barcodes {
		byProduct[b.ProductID] = append(byProduct[b.ProductID], b)
	}
	out := make([]Product, len(products))
	for i, p := range products {
		var err error
		if out[i], err = ProductFromDB(p, byProduct[p.ID]); err != nil {
			return nil, err
		}
	}
	return out, nil
}
