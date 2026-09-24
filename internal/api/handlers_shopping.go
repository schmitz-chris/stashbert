package api

import (
	"context"
	"fmt"

	"github.com/schmitz-chris/stashbert/internal/domain"
)

// GetShoppingList returns the products of which something is missing or
// which are marked for shopping, sorted by name like ListProducts.
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
			Marked:    it.Marked,
		}
	}
	return GetShoppingList200JSONResponse{Items: items}, nil
}

// MarkShoppingItem marks a product for shopping by product_id or barcode and
// returns it. An unknown barcode creates the product first.
func (s *Server) MarkShoppingItem(ctx context.Context, request MarkShoppingItemRequestObject) (MarkShoppingItemResponseObject, error) {
	r, err := domain.Mark(ctx, s.deps.DB, s.deps.Publisher, s.deps.Lookuper, domain.NewMark{
		ProductID: request.Body.ProductId,
		Barcode:   request.Body.Barcode,
	})
	if err != nil {
		return nil, err
	}
	return MarkShoppingItem200JSONResponse{
		Product:        productResponse(r.Product),
		ProductCreated: r.ProductCreated,
		AlreadyListed:  r.AlreadyListed,
		Message:        r.Message,
	}, nil
}

// UnmarkShoppingItem ends the marking for shopping of a product.
func (s *Server) UnmarkShoppingItem(ctx context.Context, request UnmarkShoppingItemRequestObject) (UnmarkShoppingItemResponseObject, error) {
	if err := domain.Unmark(ctx, s.deps.DB, request.ProductId); err != nil {
		return nil, err
	}
	return UnmarkShoppingItem204Response{}, nil
}
