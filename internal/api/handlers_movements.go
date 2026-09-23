package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/schmitz-chris/stashbert/internal/domain"
)

// CreateMovement books a movement and returns it with the changed product.
// With an Idempotency-Key, a repeated request returns the stored movement
// instead of booking again.
func (s *Server) CreateMovement(ctx context.Context, request CreateMovementRequestObject) (CreateMovementResponseObject, error) {
	body := request.Body
	in := domain.NewMovement{
		ProductID: body.ProductId,
		Barcode:   body.Barcode,
		Kind:      string(body.Kind),
		Quantity:  1,
	}
	if body.Quantity != nil {
		in.Quantity = int64(*body.Quantity)
	}
	if body.Stock != nil {
		in.Stock = new(int64(*body.Stock))
	}
	if key := request.Params.IdempotencyKey; key != nil {
		hash, err := requestHash(body)
		if err != nil {
			return nil, err
		}
		in.Idempotency = &domain.Idempotency{Key: *key, RequestHash: hash}
	}
	r, err := domain.Book(ctx, s.deps.DB, s.deps.Publisher, s.deps.Lookuper, in)
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

// requestHash returns the SHA-256 of json.Marshal of the decoded request body
// in lower case hex (architecture.md 5, request_hash).
func requestHash(body any) (string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("hash request body: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
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
