// Package domain contains the business rules of StashBert.
package domain

import "github.com/schmitz-chris/stashbert/internal/store/db"

// Missing returns how many units of a product have to be bought
// (architecture.md, 5). Nothing is missing if target is 0 or if stock has not
// fallen below the threshold, which is minStock or, if that is nil, target.
// Otherwise the result refills the stock up to target.
func Missing(stock, target int64, minStock *int64) int64 {
	if target <= 0 {
		return 0
	}
	threshold := target
	if minStock != nil {
		threshold = *minStock
	}
	if stock >= threshold {
		return 0
	}
	return target - stock
}

// Units of the quantity to buy in MQTT messages (architecture.md, 11.3).
const (
	UnitPiece = "piece"
	UnitCrate = "crate"
)

// ShoppingQuantity returns how much of a product to buy (architecture.md,
// 11.3). With a crate size the unit is UnitCrate and the quantity is missing
// divided by crateSize, rounded up (ADR-0017); otherwise the unit is
// UnitPiece and the quantity is missing. Without anything missing the
// quantity is 0.
func ShoppingQuantity(missing int64, crateSize *int64) (quantity int64, unit string) {
	if crateSize != nil && *crateSize > 0 {
		if missing <= 0 {
			return 0, UnitCrate
		}
		return (missing + *crateSize - 1) / *crateSize, UnitCrate
	}
	return max(missing, 0), UnitPiece
}

// ShoppingItem is a product on the shopping list (architecture.md, 6.5).
type ShoppingItem struct {
	ProductID string
	Name      string
	Brand     *string
	Missing   int64
	Stock     int64
	Target    int64
	Marked    bool
	// CrateSize is the number of bottles per crate or nil (ADR-0017).
	CrateSize *int64
}

// ShoppingListFromDB returns the stored products of which something is
// missing (architecture.md, 5), computed with Missing, or which are marked
// for shopping (ADR-0015), and keeps their order. Missing stays the computed
// amount, so it is 0 for a marked product of which nothing is missing.
func ShoppingListFromDB(products []db.Product) []ShoppingItem {
	var items []ShoppingItem
	for _, p := range products {
		missing := Missing(p.Stock, p.Target, p.MinStock)
		if missing <= 0 && p.Marked == 0 {
			continue
		}
		items = append(items, ShoppingItem{
			ProductID: p.ID,
			Name:      p.Name,
			Brand:     p.Brand,
			Missing:   missing,
			Stock:     p.Stock,
			Target:    p.Target,
			Marked:    p.Marked != 0,
			CrateSize: p.CrateSize,
		})
	}
	return items
}
