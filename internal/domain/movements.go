package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// warningClampedToZero reports that the stock was limited to 0 (architecture.md, 6.3).
const warningClampedToZero = "clamped_to_zero"

// NewMovement is a movement to book for a product (architecture.md 6.3,
// MovementCreate).
type NewMovement struct {
	ProductID string
	// Kind is add, consume or inventory.
	Kind string
	// Quantity is the amount for add and consume.
	Quantity int64
	// Stock is the new stock for inventory. It must be nil for add and consume.
	Stock *int64
}

// Movement is a stored movement (architecture.md, 5).
type Movement struct {
	ID         string
	ProductID  string
	Kind       string
	Delta      int64
	StockAfter int64
	Barcode    *string
	ReversesID *string
	CreatedAt  time.Time
}

// MovementResult is the result of a booking (architecture.md 6.3, MovementResult).
type MovementResult struct {
	Movement       Movement
	Product        Product
	ProductCreated bool
	// Warnings is never nil.
	Warnings []string
	Message  string
}

// Book books in for the product with in.ProductID in one transaction: it
// reads the product, applies the rules of architecture.md 6.3, inserts the
// movement and stores the new stock and updated_at at the product. The delta
// of the movement is the actual change of the stock.
//
// Invalid input results in an *httpx.Error and nothing is booked: 400
// invalid_request for inventory without stock or add and consume with stock,
// 404 not_found for an unknown product and 409 stock_already_zero for
// consume on a stock of 0.
func Book(ctx context.Context, sqlDB *sql.DB, in NewMovement) (MovementResult, error) {
	if err := checkNewMovement(in); err != nil {
		return MovementResult{}, err
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return MovementResult{}, fmt.Errorf("book movement: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	cur, err := q.GetProduct(ctx, in.ProductID)
	if errors.Is(err, sql.ErrNoRows) {
		return MovementResult{}, httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return MovementResult{}, fmt.Errorf("book movement: product %s: %w", in.ProductID, err)
	}
	stockAfter, warnings, err := bookedStock(cur.Stock, in)
	if err != nil {
		return MovementResult{}, err
	}

	now := store.FormatTime(time.Now())
	row, err := q.InsertMovement(ctx, db.InsertMovementParams{
		ID:         uuid.Must(uuid.NewV7()).String(),
		ProductID:  cur.ID,
		Kind:       in.Kind,
		Delta:      stockAfter - cur.Stock,
		StockAfter: stockAfter,
		CreatedAt:  now,
	})
	if err != nil {
		return MovementResult{}, fmt.Errorf("book movement for product %s: %w", cur.ID, err)
	}
	updated, err := q.UpdateProductStock(ctx, db.UpdateProductStockParams{
		Stock:     stockAfter,
		UpdatedAt: now,
		ID:        cur.ID,
	})
	if err != nil {
		return MovementResult{}, fmt.Errorf("book movement: update stock of product %s: %w", cur.ID, err)
	}
	barcodes, err := q.ListProductBarcodes(ctx, cur.ID)
	if err != nil {
		return MovementResult{}, fmt.Errorf("book movement: list barcodes of product %s: %w", cur.ID, err)
	}
	p, err := ProductFromDB(updated, barcodes)
	if err != nil {
		return MovementResult{}, err
	}
	m, err := movementFromDB(row)
	if err != nil {
		return MovementResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MovementResult{}, fmt.Errorf("book movement: commit: %w", err)
	}

	return MovementResult{
		Movement: m,
		Product:  p,
		Warnings: warnings,
		Message:  movementMessage(p.Name, m),
	}, nil
}

// checkNewMovement returns a 400 invalid_request error if in.Kind is unknown,
// inventory has no stock or add or consume has a stock.
func checkNewMovement(in NewMovement) error {
	switch in.Kind {
	case "add", "consume":
		if in.Stock != nil {
			return httpx.BadRequest(fmt.Sprintf("stock ist bei %s nicht erlaubt", in.Kind))
		}
	case "inventory":
		if in.Stock == nil {
			return httpx.BadRequest("stock ist bei inventory Pflicht")
		}
	default:
		return httpx.BadRequest(fmt.Sprintf("kind %q ist unbekannt", in.Kind))
	}
	return nil
}

// bookedStock returns the stock after booking in on stock and the warnings
// of the booking (architecture.md, 6.3). in must have passed checkNewMovement.
// consume on a stock of 0 results in 409 stock_already_zero; consume of more
// than stock results in 0 with the warning clamped_to_zero.
func bookedStock(stock int64, in NewMovement) (int64, []string, error) {
	warnings := []string{}
	switch in.Kind {
	case "add":
		return stock + in.Quantity, warnings, nil
	case "consume":
		if stock == 0 {
			return 0, nil, httpx.NewError(http.StatusConflict, "stock_already_zero", "Der Bestand ist schon 0")
		}
		if stock < in.Quantity {
			return 0, append(warnings, warningClampedToZero), nil
		}
		return stock - in.Quantity, warnings, nil
	}
	// inventory
	return *in.Stock, warnings, nil
}

// movementFromDB converts the stored movement m.
func movementFromDB(m db.Movement) (Movement, error) {
	createdAt, err := store.ParseTime(m.CreatedAt)
	if err != nil {
		return Movement{}, fmt.Errorf("movement %s: %w", m.ID, err)
	}
	return Movement{
		ID:         m.ID,
		ProductID:  m.ProductID,
		Kind:       m.Kind,
		Delta:      m.Delta,
		StockAfter: m.StockAfter,
		Barcode:    m.Barcode,
		ReversesID: m.ReversesID,
		CreatedAt:  createdAt,
	}, nil
}

// movementMessage returns the display text of m for the product name
// (architecture.md 6.3, message): "<name> <stock before> → <stock after>".
func movementMessage(name string, m Movement) string {
	return fmt.Sprintf("%s %d → %d", name, m.StockAfter-m.Delta, m.StockAfter)
}
