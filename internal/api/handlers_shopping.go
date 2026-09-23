package api

import (
	"context"
	"fmt"

	"github.com/schmitz-chris/stashbert/internal/domain"
)

// GetShoppingList returns the products of which something is missing, sorted
// by name like ListProducts.
func (s *Server) GetShoppingList(ctx context.Context, request GetShoppingListRequestObject) (GetShoppingListResponseObject, error) {
	rows, err := s.queries.ListProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	list := domain.ShoppingListFromDB(rows)
	items := make([]ShoppingItem, len(list))
	for i, it := range list {
		items[i] = ShoppingItem{
			ProductId: it.ProductID,
			Name:      it.Name,
			Brand:     toNullable(it.Brand),
			Missing:   int(it.Missing),
			Stock:     int(it.Stock),
			Target:    int(it.Target),
		}
	}
	return GetShoppingList200JSONResponse{Items: items}, nil
}
