// Package domain contains the business rules of StashBert.
package domain

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
