package domain

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// SummaryShoppingLimit is the number of shopping list entries a Summary
// holds at most (architecture.md, 11.5).
const SummaryShoppingLimit = 100

// Summary sums up the stock (architecture.md, 11.5).
type Summary struct {
	// ProductCount is the number of all products.
	ProductCount int64
	// ShoppingCount is the number of entries of the shopping list.
	ShoppingCount int64
	// EmptyCount is the number of products with stock 0, like the filter
	// "Leer" of the stock list.
	EmptyCount int64
	// ReviewCount is the number of products that need a review, like the
	// filter "Prüfen" of the stock list.
	ReviewCount int64
	// Shopping holds the first SummaryShoppingLimit entries of the shopping
	// list in its order. It is never nil.
	Shopping []SummaryItem
	// ShoppingTruncated is set if the shopping list has more entries than
	// Shopping.
	ShoppingTruncated bool
}

// SummaryItem is an entry of the shopping list in a Summary.
type SummaryItem struct {
	Name    string
	Missing int64
	// Quantity and Unit are the quantity to buy (ShoppingQuantity).
	Quantity int64
	Unit     string
}

// GetSummary reads all products and returns their summary.
func GetSummary(ctx context.Context, sqlDB *sql.DB) (Summary, error) {
	products, err := db.New(sqlDB).ListProducts(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("summary: list products: %w", err)
	}
	return SummaryFromDB(products), nil
}

// SummaryFromDB returns the summary of the stored products, which are in the
// order of the shopping list (ListProducts). The shopping list is the one of
// ShoppingListFromDB.
func SummaryFromDB(products []db.Product) Summary {
	s := Summary{ProductCount: int64(len(products))}
	for _, p := range products {
		if p.Stock == 0 {
			s.EmptyCount++
		}
		if p.NeedsReview != 0 {
			s.ReviewCount++
		}
	}
	list := ShoppingListFromDB(products)
	s.ShoppingCount = int64(len(list))
	s.ShoppingTruncated = len(list) > SummaryShoppingLimit
	list = list[:min(len(list), SummaryShoppingLimit)]
	s.Shopping = make([]SummaryItem, len(list))
	for i, it := range list {
		s.Shopping[i] = SummaryItem{Name: it.Name, Missing: it.Missing}
		s.Shopping[i].Quantity, s.Shopping[i].Unit = ShoppingQuantity(it.Missing, it.CrateSize)
	}
	return s
}
