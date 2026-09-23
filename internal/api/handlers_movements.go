package api

import (
	"context"

	"github.com/schmitz-chris/stashbert/internal/domain"
)

// CreateMovement books a movement and returns it with the changed product.
func (s *Server) CreateMovement(ctx context.Context, request CreateMovementRequestObject) (CreateMovementResponseObject, error) {
	body := request.Body
	in := domain.NewMovement{
		ProductID: body.ProductId,
		Kind:      string(body.Kind),
		Quantity:  1,
	}
	if body.Quantity != nil {
		in.Quantity = int64(*body.Quantity)
	}
	if body.Stock != nil {
		in.Stock = new(int64(*body.Stock))
	}
	r, err := domain.Book(ctx, s.deps.DB, in)
	if err != nil {
		return nil, err
	}
	return CreateMovement201JSONResponse{
		Movement:       movementResponse(r.Movement),
		Product:        productResponse(r.Product),
		ProductCreated: r.ProductCreated,
		Warnings:       r.Warnings,
		Message:        r.Message,
	}, nil
}

// movementResponse maps m to the schema Movement.
func movementResponse(m domain.Movement) Movement {
	return Movement{
		Id:         m.ID,
		ProductId:  m.ProductID,
		Kind:       MovementKind(m.Kind),
		Delta:      int(m.Delta),
		StockAfter: int(m.StockAfter),
		Barcode:    toNullable(m.Barcode),
		ReversesId: toNullable(m.ReversesID),
		CreatedAt:  m.CreatedAt,
	}
}
