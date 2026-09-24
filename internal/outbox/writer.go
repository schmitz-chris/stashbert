// Package outbox writes the domain events into the table outbox and delivers
// them to the MQTT broker (ADR-0018, architecture.md 11.3 and 11.4). It also
// publishes the summary and the discovery for Home Assistant, which do not
// go through the outbox (architecture.md, 11.5 and 11.6).
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// Writer is an events.Publisher that writes every event into the outbox
// (architecture.md, 11.4).
type Writer struct {
	db      *sql.DB
	wake    func()
	onEvent func()
	logger  *slog.Logger
}

// NewWriter returns a Writer for the outbox in sqlDB that calls wake after
// every written entry, usually Deliverer.Wake, and onEvent after every event
// given to Publish, usually SummaryPublisher.Request.
func NewWriter(sqlDB *sql.DB, wake, onEvent func(), logger *slog.Logger) *Writer {
	return &Writer{db: sqlDB, wake: wake, onEvent: onEvent, logger: logger}
}

// shoppingChangedData is the data of shopping.changed in the outbox: the data
// of the business logic and the state of the product after the commit
// (architecture.md, 11.3).
type shoppingChangedData struct {
	events.ShoppingChangedData
	Marked   bool   `json:"marked"`
	OnList   bool   `json:"on_list"`
	Quantity int64  `json:"quantity"`
	Unit     string `json:"unit"`
	List     string `json:"list"`
}

// Publish writes e as JSON into the outbox with the topic events/<type>,
// relative to the topic prefix, and then calls wake. For shopping.changed it
// first adds the state of the product and the chosen target list
// (architecture.md, 11.3). The write is not cancelled with ctx, because the
// business logic has already committed. An error is logged with level warn
// and never reaches the caller; the event is then lost. At the end Publish
// calls onEvent, also after an error, because the commit before the event
// may have changed the summary.
func (w *Writer) Publish(ctx context.Context, e events.Event) {
	defer w.onEvent()
	if err := w.write(context.WithoutCancel(ctx), e); err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "write event to outbox",
			slog.String("event_id", e.ID),
			slog.String("type", e.Type),
			slog.String("error", err.Error()),
		)
		return
	}
	w.wake()
}

// write reads the state of the product for shopping.changed and inserts the
// entry in one transaction.
func (w *Writer) write(ctx context.Context, e events.Event) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	if e.Type == events.TypeShoppingChanged {
		d, ok := e.Data.(events.ShoppingChangedData)
		if !ok {
			return fmt.Errorf("data of %s is %T, want events.ShoppingChangedData", e.Type, e.Data)
		}
		if e.Data, err = shoppingChanged(ctx, q, d); err != nil {
			return err
		}
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if err := q.InsertOutboxEntry(ctx, db.InsertOutboxEntryParams{
		Topic:     "events/" + e.Type,
		Payload:   string(payload),
		CreatedAt: store.FormatTime(time.Now()),
	}); err != nil {
		return fmt.Errorf("insert outbox entry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// shoppingChanged adds to d the state of its product and the id of the
// chosen target list, or "" if there is none (architecture.md, 11.3). A
// product that no longer exists is neither marked nor on the list, and its
// quantity is 0.
func shoppingChanged(ctx context.Context, q *db.Queries, d events.ShoppingChangedData) (shoppingChangedData, error) {
	out := shoppingChangedData{ShoppingChangedData: d}
	p, err := q.GetProduct(ctx, d.ProductID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		out.Quantity, out.Unit = domain.ShoppingQuantity(0, nil)
	case err != nil:
		return out, fmt.Errorf("read product %s: %w", d.ProductID, err)
	default:
		missing := domain.Missing(p.Stock, p.Target, p.MinStock)
		out.Marked = p.Marked != 0
		out.OnList = missing > 0 || out.Marked
		out.Quantity, out.Unit = domain.ShoppingQuantity(missing, p.CrateSize)
	}
	out.List, err = targetList(ctx, q)
	return out, err
}

// targetList returns the id of the chosen target list, or "" if there is
// none (architecture.md, 11.7).
func targetList(ctx context.Context, q *db.Queries) (string, error) {
	t, err := domain.StoredShoppingTarget(ctx, q)
	if err != nil || t == nil {
		return "", err
	}
	return t.ID, nil
}
