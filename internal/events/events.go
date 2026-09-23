// Package events defines the domain events of StashBert (ADR-0011,
// architecture.md 6.6). The business logic publishes them after a successful
// commit. M1 has no receivers, so production code uses Nop.
package events

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Source is the value of Event.Source for all events of StashBert.
const Source = "stashbert"

// Event types (architecture.md 6.6).
const (
	TypeProductCreated  = "product.created"
	TypeStockAdded      = "stock.added"
	TypeStockConsumed   = "stock.consumed"
	TypeStockAdjusted   = "stock.adjusted"
	TypeProductEmpty    = "product.empty"
	TypeShoppingChanged = "shopping.changed"
)

// Event is a domain event, modelled on CloudEvents 1.0.
type Event struct {
	ID     string    `json:"id"`
	Type   string    `json:"type"`
	Source string    `json:"source"`
	Time   time.Time `json:"time"`
	Data   any       `json:"data"`
}

// New returns an event of type typ with a new UUIDv7 as ID, Source set to
// "stashbert" and the current time in UTC.
func New(typ string, data any) Event {
	// uuid.NewV7 only fails if the random source fails. The default source
	// is crypto/rand, whose reader never returns an error, so Must cannot panic.
	id := uuid.Must(uuid.NewV7())
	return Event{
		ID:     id.String(),
		Type:   typ,
		Source: Source,
		Time:   time.Now().UTC(),
		Data:   data,
	}
}

// ProductCreatedData is the data of TypeProductCreated.
type ProductCreatedData struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Origin    string `json:"origin"`
}

// StockData is the data of TypeStockAdded, TypeStockConsumed and TypeStockAdjusted.
type StockData struct {
	ProductID  string `json:"product_id"`
	MovementID string `json:"movement_id"`
	Delta      int64  `json:"delta"`
	StockAfter int64  `json:"stock_after"`
}

// ProductEmptyData is the data of TypeProductEmpty.
type ProductEmptyData struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
}

// ShoppingChangedData is the data of TypeShoppingChanged.
type ShoppingChangedData struct {
	ProductID     string `json:"product_id"`
	Name          string `json:"name"`
	MissingBefore int64  `json:"missing_before"`
	MissingAfter  int64  `json:"missing_after"`
}

// Publisher receives domain events.
type Publisher interface {
	Publish(ctx context.Context, e Event)
}

// Nop is a Publisher that discards all events.
type Nop struct{}

// Publish does nothing.
func (Nop) Publish(context.Context, Event) {}
