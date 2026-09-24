package domain_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// stored returns a stored product with the given values, not marked, without
// minimum stock and crate size and without review.
func stored(name string, stock, target int64) db.Product {
	return db.Product{ID: "id-" + name, Name: name, Stock: stock, Target: target}
}

func TestSummaryFromDB(t *testing.T) {
	marked := func(p db.Product) db.Product { p.Marked = 1; return p }
	review := func(p db.Product) db.Product { p.NeedsReview = 1; return p }
	crates := func(p db.Product, size int64) db.Product { p.CrateSize = &size; return p }
	minStock := func(p db.Product, v int64) db.Product { p.MinStock = &v; return p }
	// In the order of ListProducts.
	products := []db.Product{
		// Marked without target and empty: on the list with nothing missing.
		marked(stored("apfelsaft", 0, 0)),
		// Marked with crates and nothing missing.
		marked(crates(stored("Cola", 24, 12), 6)),
		// Crates, the example of architecture.md 11.3.
		crates(stored("Jever Pilsener", 7, 24), 20),
		// Something missing and to review.
		review(stored("Kidneybohnen", 2, 5)),
		// Above the minimum stock: not on the list.
		minStock(stored("Mehl", 2, 4), 1),
		// Marked and something missing.
		marked(stored("Milch", 1, 3)),
		// Crates, nothing missing: not on the list.
		crates(stored("Wasser", 30, 24), 6),
		// Without target, empty and to review: not on the list.
		review(stored("Zucker", 0, 0)),
		// Empty with target: on the list.
		stored("Zwieback", 0, 2),
	}

	got := domain.SummaryFromDB(products)

	want := domain.Summary{
		ProductCount:  9,
		ShoppingCount: 6,
		EmptyCount:    3,
		ReviewCount:   2,
		Shopping: []domain.SummaryItem{
			{Name: "apfelsaft", Missing: 0, Quantity: 0, Unit: "piece"},
			{Name: "Cola", Missing: 0, Quantity: 0, Unit: "crate"},
			{Name: "Jever Pilsener", Missing: 17, Quantity: 1, Unit: "crate"},
			{Name: "Kidneybohnen", Missing: 3, Quantity: 3, Unit: "piece"},
			{Name: "Milch", Missing: 2, Quantity: 2, Unit: "piece"},
			{Name: "Zwieback", Missing: 2, Quantity: 2, Unit: "piece"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SummaryFromDB =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSummaryFromDBWithoutProducts(t *testing.T) {
	got := domain.SummaryFromDB(nil)

	if got.Shopping == nil {
		t.Error("Shopping = nil, want an empty slice")
	}
	if want := (domain.Summary{Shopping: []domain.SummaryItem{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("SummaryFromDB(nil) = %+v, want %+v", got, want)
	}
}

// The summary holds the first 100 entries of the shopping list; products
// that are not on the list do not count.
func TestSummaryFromDBLimit(t *testing.T) {
	tests := []struct {
		onList    int
		truncated bool
	}{
		{99, false},
		{100, false},
		{101, true},
		{150, true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.onList), func(t *testing.T) {
			var products []db.Product
			for i := range tt.onList {
				// Every product on the list is followed by one that is not.
				products = append(products,
					stored(fmt.Sprintf("p%03d", i), 0, 1),
					stored(fmt.Sprintf("p%03d voll", i), 1, 1))
			}

			got := domain.SummaryFromDB(products)

			if got.ProductCount != int64(2*tt.onList) || got.ShoppingCount != int64(tt.onList) || got.ShoppingTruncated != tt.truncated {
				t.Errorf("product_count, shopping_count, shopping_truncated = %d, %d, %v, want %d, %d, %v",
					got.ProductCount, got.ShoppingCount, got.ShoppingTruncated, 2*tt.onList, tt.onList, tt.truncated)
			}
			n := min(tt.onList, domain.SummaryShoppingLimit)
			if len(got.Shopping) != n {
				t.Fatalf("len(Shopping) = %d, want %d", len(got.Shopping), n)
			}
			for i, it := range got.Shopping {
				if want := fmt.Sprintf("p%03d", i); it.Name != want {
					t.Errorf("Shopping[%d].Name = %q, want %q", i, it.Name, want)
				}
			}
		})
	}
}
