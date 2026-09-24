package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

func TestProductsFromDB(t *testing.T) {
	const stored = "2026-09-23T12:00:00.123Z"
	storedTime := time.Date(2026, 9, 23, 12, 0, 0, 123_000_000, time.UTC)
	image := "a.jpg"
	products := []db.Product{
		{ID: "a", Name: "Kidneybohnen", Stock: 2, Target: 5, Origin: "manual", LookupState: "none",
			NeedsReview: 1, Marked: 1, ImageFile: &image, CreatedAt: stored, UpdatedAt: stored},
		{ID: "b", Name: "Mehl", Stock: 2, Target: 4, MinStock: new(int64(1)), Origin: "manual", LookupState: "none",
			CreatedAt: stored, UpdatedAt: stored},
	}
	// Not sorted by code and mixed across products.
	barcodes := []db.Barcode{
		{Code: "4001686301265", ProductID: "a", Units: 1},
		{Code: "3017620422003", ProductID: "b", Units: 1},
		{Code: "0034000470693", ProductID: "a", Units: 6},
	}

	got, err := domain.ProductsFromDB(products, barcodes)
	if err != nil {
		t.Fatalf("ProductsFromDB: %v", err)
	}

	want := []domain.Product{
		{ID: "a", Name: "Kidneybohnen", Stock: 2, Target: 5, Missing: 3, Marked: true, NeedsReview: true, Origin: "manual",
			LookupState: "none", HasImage: true, CreatedAt: storedTime, UpdatedAt: storedTime,
			Barcodes: []domain.Barcode{{Code: "0034000470693", Units: 6}, {Code: "4001686301265", Units: 1}}},
		{ID: "b", Name: "Mehl", Stock: 2, Target: 4, MinStock: new(int64(1)), Missing: 0, Origin: "manual",
			LookupState: "none", CreatedAt: storedTime, UpdatedAt: storedTime,
			Barcodes: []domain.Barcode{{Code: "3017620422003", Units: 1}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProductsFromDB:\n got %+v\nwant %+v", got, want)
	}
}

func TestProductFromDBInvalidTime(t *testing.T) {
	_, err := domain.ProductFromDB(db.Product{ID: "a", CreatedAt: "2026-09-23 12:00:00", UpdatedAt: "2026-09-23T12:00:00.000Z"}, nil)
	if err == nil {
		t.Error("ProductFromDB with invalid created_at: err = nil, want error")
	}
}
