package api

import (
	"context"

	"github.com/schmitz-chris/stashbert/internal/domain"
)

// GetSummary returns the counts over all products and the first 100 entries
// of the shopping list (architecture.md, 11.5).
func (s *Server) GetSummary(ctx context.Context, request GetSummaryRequestObject) (GetSummaryResponseObject, error) {
	sum, err := domain.GetSummary(ctx, s.deps.DB)
	if err != nil {
		return nil, err
	}
	items := make([]SummaryItem, len(sum.Shopping))
	for i, it := range sum.Shopping {
		items[i] = SummaryItem{
			Name:     it.Name,
			Missing:  int(it.Missing),
			Quantity: int(it.Quantity),
			Unit:     SummaryShoppingUnit(it.Unit),
		}
	}
	return GetSummary200JSONResponse{
		ProductCount:      int(sum.ProductCount),
		ShoppingCount:     int(sum.ShoppingCount),
		EmptyCount:        int(sum.EmptyCount),
		ReviewCount:       int(sum.ReviewCount),
		Shopping:          items,
		ShoppingTruncated: sum.ShoppingTruncated,
	}, nil
}
