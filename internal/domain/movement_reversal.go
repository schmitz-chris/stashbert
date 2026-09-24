package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// ReverseMovement reverses the movement with id in one transaction
// (architecture.md 6.3, Storno): it reads the movement and its product,
// inserts a movement of kind reversal with reverses_id id and without barcode
// and stores the new stock and updated_at at the product; its marking for
// shopping stays as it is, also for the reversal of an add. The reversal aims
// at the change -delta of the movement; if the stock would become negative,
// it becomes 0 with the warning clamped_to_zero. The delta of the reversal is
// the actual change of the stock. After the commit it publishes the events of
// the reversal to pub (see publishMovementEvents), so a reversal with delta 0
// publishes none.
//
// With idem, the reversal keeps its key and request hash. Before it reads the
// movement, ReverseMovement looks up a movement with the key in the same
// transaction like Book: the same request hash results in the reconstructed
// result (see repeatedMovement), another one in 422 idempotency_key_mismatch.
// Neither publishes events.
//
// An unknown id results in 404 not_found, a movement of kind reversal or
// merge in 409 not_reversible and a movement that is reversed already in 409
// already_reversed; nothing is booked then. The UNIQUE index on reverses_id
// backs the last check against concurrent requests.
func ReverseMovement(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, id string, idem *Idempotency) (MovementResult, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return MovementResult{}, fmt.Errorf("reverse movement %s: begin transaction: %w", id, err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	var key, hash *string
	if idem != nil {
		key, hash = &idem.Key, &idem.RequestHash
		if r, ok, err := repeatedMovement(ctx, q, *idem); err != nil || ok {
			return r, err
		}
	}
	original, err := q.GetMovement(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return MovementResult{}, httpx.NotFound("Buchung nicht gefunden")
	}
	if err != nil {
		return MovementResult{}, fmt.Errorf("reverse movement %s: %w", id, err)
	}
	if original.Kind == "reversal" || original.Kind == "merge" {
		return MovementResult{}, httpx.NewError(http.StatusConflict, "not_reversible",
			fmt.Sprintf("Buchungen der Art %s sind nicht stornierbar", original.Kind))
	}
	reversed, err := q.IsMovementReversed(ctx, &original.ID)
	if err != nil {
		return MovementResult{}, fmt.Errorf("reverse movement %s: look up reversal: %w", id, err)
	}
	if reversed {
		return MovementResult{}, errAlreadyReversed()
	}
	cur, err := q.GetProduct(ctx, original.ProductID)
	if err != nil {
		return MovementResult{}, fmt.Errorf("reverse movement %s: product %s: %w", id, original.ProductID, err)
	}

	stockAfter, warnings := cur.Stock-original.Delta, []string{}
	if stockAfter < 0 {
		stockAfter, warnings = 0, append(warnings, warningClampedToZero)
	}
	now := store.FormatTime(time.Now())
	row, err := q.InsertMovement(ctx, db.InsertMovementParams{
		ID:             uuid.Must(uuid.NewV7()).String(),
		ProductID:      cur.ID,
		Kind:           "reversal",
		Delta:          stockAfter - cur.Stock,
		StockAfter:     stockAfter,
		ReversesID:     &original.ID,
		IdempotencyKey: key,
		RequestHash:    hash,
		CreatedAt:      now,
	})
	if store.IsUniqueViolation(err) {
		// The UNIQUE index on idempotency_key or reverses_id: a concurrent
		// request has stored a movement with the same key or reversed the
		// movement since the checks above. The movement of the key is read
		// after the rollback; without one, the movement is reversed.
		if rbErr := tx.Rollback(); rbErr != nil {
			return MovementResult{}, fmt.Errorf("reverse movement %s: rollback: %w", id, rbErr)
		}
		if idem != nil {
			if r, ok, err := repeatedMovement(ctx, db.New(sqlDB), *idem); err != nil || ok {
				return r, err
			}
		}
		return MovementResult{}, errAlreadyReversed()
	}
	if err != nil {
		return MovementResult{}, fmt.Errorf("reverse movement %s: %w", id, err)
	}
	p, err := updateStock(ctx, q, cur.ID, stockAfter, cur.Marked, now)
	if err != nil {
		return MovementResult{}, fmt.Errorf("reverse movement %s: %w", id, err)
	}
	m, err := movementFromDB(row)
	if err != nil {
		return MovementResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MovementResult{}, fmt.Errorf("reverse movement %s: commit: %w", id, err)
	}

	publishMovementEvents(ctx, pub, cur, p, m)
	return MovementResult{
		Movement: m,
		Product:  p,
		Warnings: warnings,
		Message:  movementMessage(p.Name, m),
	}, nil
}

// errAlreadyReversed returns the 409 already_reversed error of a movement
// that is reversed already (architecture.md, 6.4).
func errAlreadyReversed() error {
	return httpx.NewError(http.StatusConflict, "already_reversed", "Die Buchung ist schon storniert")
}
