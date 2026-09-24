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

// warningClampedToZero reports that the stock was limited to 0 (architecture.md, 6.3).
const warningClampedToZero = "clamped_to_zero"

// codeUnknownBarcode is the error code of an unknown barcode (architecture.md, 6.4).
const codeUnknownBarcode = "unknown_barcode"

// NewMovement is a movement to book for a product (architecture.md 6.3,
// MovementCreate). Exactly one of ProductID and Barcode must be set.
type NewMovement struct {
	ProductID *string
	// Barcode is a barcode of the product as entered, not yet normalized.
	Barcode *string
	// Kind is add, consume or inventory.
	Kind string
	// Quantity is the amount for add and consume. Booked by barcode, each
	// unit of Quantity books the units of the barcode.
	Quantity int64
	// Stock is the new stock for inventory. It must be nil for add and consume.
	Stock *int64
	// Idempotency is nil for a request without Idempotency-Key.
	Idempotency *Idempotency
}

// Idempotency is the Idempotency-Key of a request with the hash of its body
// (architecture.md 5 and 6.1).
type Idempotency struct {
	Key string
	// RequestHash is the SHA-256 of the request body in lower case hex.
	RequestHash string
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

// Book books in for the product with in.ProductID or the product of the
// barcode in.Barcode in one transaction: it reads the product, applies the
// rules of architecture.md 6.3, inserts the movement and stores the new stock
// and updated_at at the product. A movement add also ends the marking for
// shopping of the product (ADR-0015). The delta of the movement is the actual
// change of the stock. Booked by barcode, the amount of add and consume is
// quantity times the units of the barcode and the movement keeps the
// normalized barcode. After the commit it publishes the events of the
// movement to pub (see publishMovementEvents).
//
// add with an unknown barcode creates a product for it (architecture.md,
// 7.2): lookupProduct determines the product outside of any transaction, so
// the lookup with lookuper holds no write lock. Then one transaction checks
// the idempotency key and the barcode again and stores the product with
// needs_review and target 0, its barcode with 1 unit, the movement and the
// cache entry of the lookup. If the barcode is known by then, the movement is
// booked on its product. A created product results in product_created, the
// message prefix "Neu: " and, for a placeholder, the warning
// placeholder_created; product.created is published before the events of
// the movement.
//
// With in.Idempotency, the movement keeps its key and request hash. Before
// it reads the product or barcode, Book looks up a movement with the key in
// the same transaction. If there is one, it books nothing: the same request
// hash results in the reconstructed result (see repeatedMovement), another
// one in 422 idempotency_key_mismatch. Neither publishes events.
//
// Invalid input results in an *httpx.Error and nothing is booked: 400
// invalid_request if not exactly one of product_id and barcode is set, for
// inventory without stock or add and consume with stock, 422 invalid_barcode
// for an invalid barcode, 404 not_found for an unknown product, 404
// unknown_barcode for consume and inventory with an unknown barcode and 409
// stock_already_zero for consume on a stock of 0.
func Book(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, lookuper Lookuper, in NewMovement) (MovementResult, error) {
	if err := checkNewMovement(in); err != nil {
		return MovementResult{}, err
	}
	code, err := normalizeOptionalBarcode(in.Barcode)
	if err != nil {
		return MovementResult{}, err
	}

	r, err := book(ctx, sqlDB, pub, in, code, nil)
	if in.Kind != "add" || !isUnknownBarcode(err) {
		return r, err
	}
	s, err := lookupProduct(ctx, sqlDB, lookuper, *code)
	if err != nil {
		return MovementResult{}, err
	}
	return book(ctx, sqlDB, pub, in, code, &s)
}

// book books in within one transaction as described for Book. code is the
// normalized barcode or nil. An unknown barcode results in 404
// unknown_barcode if s is nil; otherwise book creates the product s for it.
func book(ctx context.Context, sqlDB *sql.DB, pub events.Publisher, in NewMovement, code *string, s *scannedProduct) (MovementResult, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return MovementResult{}, fmt.Errorf("book movement: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	var key, hash *string
	if in.Idempotency != nil {
		key, hash = &in.Idempotency.Key, &in.Idempotency.RequestHash
		if r, ok, err := repeatedMovement(ctx, q, *in.Idempotency); err != nil || ok {
			return r, err
		}
	}
	now := store.FormatTime(time.Now())
	cur, units, err := movementProduct(ctx, q, in.ProductID, code)
	created := false
	if s != nil && isUnknownBarcode(err) {
		cur, created, err = insertScannedProduct(ctx, q, *s, *code, now)
		if err == nil && !created {
			// A concurrent request has stored the barcode since it was read
			// above: book on its product after the rollback.
			if rbErr := tx.Rollback(); rbErr != nil {
				return MovementResult{}, fmt.Errorf("book movement: rollback: %w", rbErr)
			}
			return book(ctx, sqlDB, pub, in, code, nil)
		}
		units = 1
	}
	if err != nil {
		return MovementResult{}, err
	}
	stockAfter, warnings, err := bookedStock(cur.Stock, in.Quantity*units, in)
	if err != nil {
		return MovementResult{}, err
	}

	row, err := q.InsertMovement(ctx, db.InsertMovementParams{
		ID:             uuid.Must(uuid.NewV7()).String(),
		ProductID:      cur.ID,
		Kind:           in.Kind,
		Delta:          stockAfter - cur.Stock,
		StockAfter:     stockAfter,
		Barcode:        code,
		IdempotencyKey: key,
		RequestHash:    hash,
		CreatedAt:      now,
	})
	if in.Idempotency != nil && store.IsUniqueViolation(err) {
		// The UNIQUE index on idempotency_key: a concurrent request with the
		// same key has stored its movement since the lookup above. Its
		// movement is read after the rollback.
		if rbErr := tx.Rollback(); rbErr != nil {
			return MovementResult{}, fmt.Errorf("book movement: rollback: %w", rbErr)
		}
		r, ok, repeatErr := repeatedMovement(ctx, db.New(sqlDB), *in.Idempotency)
		if repeatErr != nil || ok {
			return r, repeatErr
		}
	}
	if err != nil {
		return MovementResult{}, fmt.Errorf("book movement for product %s: %w", cur.ID, err)
	}
	marked := cur.Marked
	if in.Kind == "add" {
		marked = 0
	}
	p, err := updateStock(ctx, q, cur.ID, stockAfter, marked, now)
	if err != nil {
		return MovementResult{}, fmt.Errorf("book movement: %w", err)
	}
	m, err := movementFromDB(row)
	if err != nil {
		return MovementResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MovementResult{}, fmt.Errorf("book movement: commit: %w", err)
	}

	message := movementMessage(p.Name, m)
	if created {
		message = "Neu: " + message
		if p.Origin == "placeholder" {
			warnings = append(warnings, warningPlaceholderCreated)
		}
		publishProductCreated(ctx, pub, p)
	}
	publishMovementEvents(ctx, pub, cur, p, m)
	return MovementResult{
		Movement:       m,
		Product:        p,
		ProductCreated: created,
		Warnings:       warnings,
		Message:        message,
	}, nil
}

// updateStock stores stock, marked (0 or 1) and updated_at now at the
// product with id and returns the changed product with its barcodes.
func updateStock(ctx context.Context, q *db.Queries, id string, stock, marked int64, now string) (Product, error) {
	updated, err := q.UpdateProductStock(ctx, db.UpdateProductStockParams{
		Stock:     stock,
		Marked:    marked,
		UpdatedAt: now,
		ID:        id,
	})
	if err != nil {
		return Product{}, fmt.Errorf("update stock of product %s: %w", id, err)
	}
	barcodes, err := q.ListProductBarcodes(ctx, id)
	if err != nil {
		return Product{}, fmt.Errorf("list barcodes of product %s: %w", id, err)
	}
	return ProductFromDB(updated, barcodes)
}

// repeatedMovement looks up the movement stored with the key of idem. If
// there is none, it returns false. If its request hash equals the one of
// idem, it returns true and the result reconstructed from the movement
// (architecture.md, 6.1): the current product, product_created false, no
// warnings and the message of the movement with the current product name.
// Another request hash results in 422 idempotency_key_mismatch. Book and
// ReverseMovement share the keys, so a key of one of them used for the other
// results in idempotency_key_mismatch, too.
func repeatedMovement(ctx context.Context, q *db.Queries, idem Idempotency) (MovementResult, bool, error) {
	row, err := q.GetMovementByIdempotencyKey(ctx, &idem.Key)
	if errors.Is(err, sql.ErrNoRows) {
		return MovementResult{}, false, nil
	}
	if err != nil {
		return MovementResult{}, false, fmt.Errorf("repeat movement: movement of idempotency key: %w", err)
	}
	if row.RequestHash == nil || *row.RequestHash != idem.RequestHash {
		return MovementResult{}, false, httpx.NewError(http.StatusUnprocessableEntity, "idempotency_key_mismatch",
			"Der Idempotency-Key wurde schon für eine andere Anfrage verwendet")
	}
	cur, err := q.GetProduct(ctx, row.ProductID)
	if err != nil {
		return MovementResult{}, false, fmt.Errorf("repeat movement: product %s of movement %s: %w", row.ProductID, row.ID, err)
	}
	barcodes, err := q.ListProductBarcodes(ctx, cur.ID)
	if err != nil {
		return MovementResult{}, false, fmt.Errorf("repeat movement: list barcodes of product %s: %w", cur.ID, err)
	}
	p, err := ProductFromDB(cur, barcodes)
	if err != nil {
		return MovementResult{}, false, err
	}
	m, err := movementFromDB(row)
	if err != nil {
		return MovementResult{}, false, err
	}
	return MovementResult{
		Movement: m,
		Product:  p,
		Warnings: []string{},
		Message:  movementMessage(p.Name, m),
	}, true, nil
}

// checkNewMovement returns a 400 invalid_request error if not exactly one of
// in.ProductID and in.Barcode is set, in.Kind is unknown, inventory has no
// stock or add or consume has a stock.
func checkNewMovement(in NewMovement) error {
	if err := checkProductOrBarcode(in.ProductID, in.Barcode); err != nil {
		return err
	}
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

// checkProductOrBarcode returns a 400 invalid_request error if not exactly
// one of productID and barcode is set.
func checkProductOrBarcode(productID, barcode *string) error {
	if (productID == nil) == (barcode == nil) {
		return httpx.BadRequest("Genau eines von product_id und barcode muss gesetzt sein")
	}
	return nil
}

// normalizeOptionalBarcode returns nil if barcode is nil and otherwise the
// barcode normalized by normalizeBarcode, which results in 422
// invalid_barcode for an invalid one.
func normalizeOptionalBarcode(barcode *string) (*string, error) {
	if barcode == nil {
		return nil, nil
	}
	code, err := normalizeBarcode(*barcode)
	if err != nil {
		return nil, err
	}
	return &code, nil
}

// movementProduct returns the product to book on and the units that one unit
// of quantity books: the product with productID and 1 if code is nil,
// otherwise the product of the normalized barcode code and its units. An
// unknown product results in 404 not_found, an unknown barcode in 404
// unknown_barcode.
func movementProduct(ctx context.Context, q *db.Queries, productID, code *string) (db.Product, int64, error) {
	if code == nil {
		p, err := q.GetProduct(ctx, *productID)
		if errors.Is(err, sql.ErrNoRows) {
			return db.Product{}, 0, httpx.NotFound("Produkt nicht gefunden")
		}
		if err != nil {
			return db.Product{}, 0, fmt.Errorf("book movement: product %s: %w", *productID, err)
		}
		return p, 1, nil
	}
	b, err := q.GetBarcode(ctx, *code)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Product{}, 0, httpx.NewError(http.StatusNotFound, codeUnknownBarcode,
			fmt.Sprintf("Barcode %s ist unbekannt", *code))
	}
	if err != nil {
		return db.Product{}, 0, fmt.Errorf("book movement: barcode %s: %w", *code, err)
	}
	p, err := q.GetProduct(ctx, b.ProductID)
	if err != nil {
		return db.Product{}, 0, fmt.Errorf("book movement: product %s of barcode %s: %w", b.ProductID, *code, err)
	}
	return p, b.Units, nil
}

// bookedStock returns the stock after booking in on stock and the warnings
// of the booking (architecture.md, 6.3). amount is the amount of add and
// consume, in.Quantity times the units of the barcode; inventory ignores it.
// in must have passed checkNewMovement. consume on a stock of 0 results in
// 409 stock_already_zero; consume of more than stock results in 0 with the
// warning clamped_to_zero.
func bookedStock(stock, amount int64, in NewMovement) (int64, []string, error) {
	warnings := []string{}
	switch in.Kind {
	case "add":
		return stock + amount, warnings, nil
	case "consume":
		if stock == 0 {
			return 0, nil, httpx.NewError(http.StatusConflict, "stock_already_zero", "Der Bestand ist schon 0")
		}
		if stock < amount {
			return 0, append(warnings, warningClampedToZero), nil
		}
		return stock - amount, warnings, nil
	}
	// inventory
	return *in.Stock, warnings, nil
}

// publishMovementEvents publishes the events of the booked movement m to pub,
// in the order of architecture.md 6.6: the stock event of the kind of m,
// product.empty if the stock changed from above 0 to 0 and shopping.changed
// if missing changed. before is the stored product before and p the product
// after the booking. A movement with delta 0 publishes no event.
func publishMovementEvents(ctx context.Context, pub events.Publisher, before db.Product, p Product, m Movement) {
	if m.Delta == 0 {
		return
	}
	pub.Publish(ctx, events.New(stockEventType(m.Kind), events.StockData{
		ProductID:  p.ID,
		MovementID: m.ID,
		Delta:      m.Delta,
		StockAfter: m.StockAfter,
	}))
	if before.Stock > 0 && p.Stock == 0 {
		pub.Publish(ctx, events.New(events.TypeProductEmpty, events.ProductEmptyData{
			ProductID: p.ID,
			Name:      p.Name,
		}))
	}
	if missingBefore := Missing(before.Stock, before.Target, before.MinStock); missingBefore != p.Missing {
		pub.Publish(ctx, events.New(events.TypeShoppingChanged, events.ShoppingChangedData{
			ProductID:     p.ID,
			Name:          p.Name,
			MissingBefore: missingBefore,
			MissingAfter:  p.Missing,
		}))
	}
}

// stockEventType returns the event type of a movement of kind
// (architecture.md, 6.6): stock.added for add, stock.consumed for consume and
// stock.adjusted for inventory, reversal and merge.
func stockEventType(kind string) string {
	switch kind {
	case "add":
		return events.TypeStockAdded
	case "consume":
		return events.TypeStockConsumed
	}
	return events.TypeStockAdjusted
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
