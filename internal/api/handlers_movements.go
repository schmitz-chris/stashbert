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
	return CreateMovement201JSONResponse(movementResultResponse(r)), nil
}

// ReverseMovement reverses a movement and returns the reversal with the
// changed product. With an Idempotency-Key, a repeated request returns the
// stored reversal instead of reversing again; the request hash is taken from
// the id of the reversed movement.
func (s *Server) ReverseMovement(ctx context.Context, request ReverseMovementRequestObject) (ReverseMovementResponseObject, error) {
	var idem *domain.Idempotency
	if key := request.Params.IdempotencyKey; key != nil {
		idem = &domain.Idempotency{Key: *key, RequestHash: sha256Hex([]byte(request.Id))}
	}
	r, err := domain.ReverseMovement(ctx, s.deps.DB, s.deps.Publisher, request.Id, idem)
	if err != nil {
		return nil, err
	}
	return ReverseMovement201JSONResponse(movementResultResponse(r)), nil
}

// defaultMovementLimit is the size of a page of movements without the
// parameter limit (architecture.md 6.2, listMovements).
const defaultMovementLimit = 50

// ListMovements returns a page of movements, newest first, optionally only
// those of one product.
func (s *Server) ListMovements(ctx context.Context, request ListMovementsRequestObject) (ListMovementsResponseObject, error) {
	params := request.Params
	in := domain.MovementQuery{
		ProductID: params.ProductId,
		Cursor:    params.Cursor,
		Limit:     defaultMovementLimit,
	}
	if params.Limit != nil {
		in.Limit = int64(*params.Limit)
	}
	page, err := domain.ListMovements(ctx, s.deps.DB, in)
	if err != nil {
		return nil, err
	}
	items := make([]Movement, len(page.Items))
	for i, m := range page.Items {
		items[i] = movementResponse(m)
	}
	return ListMovements200JSONResponse{Items: items, NextCursor: toNullable(page.NextCursor)}, nil
}

// requestHash returns the SHA-256 of json.Marshal of the decoded request body
// in lower case hex (architecture.md 5, request_hash).
func requestHash(body any) (string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("hash request body: %w", err)
	}
	return sha256Hex(b), nil
}

// sha256Hex returns the SHA-256 of b in lower case hex.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// movementResultResponse maps r to the schema MovementResult.
func movementResultResponse(r domain.MovementResult) MovementResult {
	return MovementResult{
		Movement:       movementResponse(r.Movement),
		Product:        productResponse(r.Product),
		ProductCreated: r.ProductCreated,
		Warnings:       r.Warnings,
		Message:        r.Message,
	}
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
