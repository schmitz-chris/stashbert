// Package domain contains the business rules of StashBert.
package domain

import (
	"context"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

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

// listing is what decides whether a product is on the shopping list
// (architecture.md, 5): what is missing of it and whether it is marked
// (ADR-0015). A product that does not exist has the zero listing.
type listing struct {
	missing int64
	marked  bool
}

// onList reports whether a product with the listing l is on the shopping list.
func (l listing) onList() bool {
	return l.missing > 0 || l.marked
}

// storedListing returns the listing of the stored product p.
func storedListing(p db.Product) listing {
	return listing{missing: Missing(p.Stock, p.Target, p.MinStock), marked: p.Marked != 0}
}

// productListing returns the listing of p.
func productListing(p Product) listing {
	return listing{missing: p.Missing, marked: p.Marked}
}

// publishShoppingChanged publishes shopping.changed for the product id with
// name to pub if its missing or whether it is on the shopping list differs
// between before and after (architecture.md, 6.6). Callers call it at most
// once per product and operation, after the commit, so a change of both
// results in one event. If only whether it is on the list changed,
// missing_before and missing_after are equal.
func publishShoppingChanged(ctx context.Context, pub events.Publisher, id, name string, before, after listing) {
	if before.missing == after.missing && before.onList() == after.onList() {
		return
	}
	pub.Publish(ctx, events.New(events.TypeShoppingChanged, events.ShoppingChangedData{
		ProductID:     id,
		Name:          name,
		MissingBefore: before.missing,
		MissingAfter:  after.missing,
	}))
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
