package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// typeShoppingSnapshot is the type of the event with the whole shopping
// list. The reconciliation writes it, not the business logic
// (architecture.md, 6.6 and 11.3).
const typeShoppingSnapshot = "shopping.snapshot"

// SnapshotWindow is the time within which all requests for a snapshot result
// in one (architecture.md, 11.3).
const SnapshotWindow = 2 * time.Second

// shoppingSnapshotData is the data of shopping.snapshot (architecture.md,
// 11.3).
type shoppingSnapshotData struct {
	List  string                 `json:"list"`
	Items []shoppingSnapshotItem `json:"items"`
}

// shoppingSnapshotItem is a product in shopping.snapshot.
type shoppingSnapshotItem struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	OnList    bool   `json:"on_list"`
	Missing   int64  `json:"missing"`
	Quantity  int64  `json:"quantity"`
	Unit      string `json:"unit"`
}

// WriteShoppingSnapshot writes shopping.snapshot for the chosen target list,
// or with list "" if there is none, into the outbox and then calls wake
// (architecture.md, 11.3). It reads the target list and the products in the
// same transaction.
func (w *Writer) WriteShoppingSnapshot(ctx context.Context) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("write shopping snapshot: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	list, err := targetList(ctx, q)
	if err != nil {
		return fmt.Errorf("write shopping snapshot: %w", err)
	}
	if err := insertSnapshot(ctx, q, list, false); err != nil {
		return fmt.Errorf("write shopping snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("write shopping snapshot: commit: %w", err)
	}
	w.wake()
	return nil
}

// insertSnapshot reads the products with q and inserts shopping.snapshot
// for the target list with the id list into the outbox. If cleared is set,
// every item has on_list false and quantity 0, so that the list is cleared
// when another one is chosen (architecture.md, 11.7).
func insertSnapshot(ctx context.Context, q *db.Queries, list string, cleared bool) error {
	rows, err := q.ListShoppingSnapshotProducts(ctx)
	if err != nil {
		return fmt.Errorf("list products: %w", err)
	}
	e := events.New(typeShoppingSnapshot, shoppingSnapshotData{List: list, Items: snapshotItems(rows, cleared)})
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
	return nil
}

// snapshotItems returns the items of shopping.snapshot for rows in their
// order, never nil: on_list, quantity and unit as in shopping.changed
// (architecture.md, 11.3), but with cleared every item has on_list false and
// quantity 0.
func snapshotItems(rows []db.ListShoppingSnapshotProductsRow, cleared bool) []shoppingSnapshotItem {
	items := make([]shoppingSnapshotItem, 0, len(rows))
	for _, p := range rows {
		missing := domain.Missing(p.Stock, p.Target, p.MinStock)
		it := shoppingSnapshotItem{
			ProductID: p.ID,
			Name:      p.Name,
			OnList:    missing > 0 || p.Marked != 0,
			Missing:   missing,
		}
		it.Quantity, it.Unit = domain.ShoppingQuantity(missing, p.CrateSize)
		if cleared {
			it.OnList, it.Quantity = false, 0
		}
		items = append(items, it)
	}
	return items
}

// Snapshotter writes shopping.snapshot for the chosen target list on
// request, one for all requests within a window (architecture.md, 11.3).
type Snapshotter struct {
	writer   *Writer
	topic    string
	logger   *slog.Logger
	requests chan struct{}
}

// NewSnapshotter returns a Snapshotter that writes with w and takes requests
// from the topic <prefix>/in/snapshot, where prefix is MQTT_TOPIC_PREFIX.
func NewSnapshotter(w *Writer, prefix string, logger *slog.Logger) *Snapshotter {
	return &Snapshotter{writer: w, topic: prefix + "/in/snapshot", logger: logger, requests: make(chan struct{}, 1)}
}

// Request asks for a snapshot. It never blocks and may be called from any
// goroutine, also before Run.
func (s *Snapshotter) Request() {
	select {
	case s.requests <- struct{}{}:
	default:
	}
}

// Receive requests a snapshot for a message on <prefix>/in/snapshot with
// any payload. It ignores messages with the retain flag and on other topics
// (architecture.md, 11.2). Register it with mqtt.Client.OnMessage.
func (s *Snapshotter) Receive(m mqtt.Message) {
	if m.Topic == s.topic && !m.Retained {
		s.Request()
	}
}

// Run writes the snapshots until ctx ends. A request starts a window of the
// given length; requests within it are part of the same snapshot, which Run
// writes when the window has passed. A request during the write starts the
// next window. A failed write is logged with level warn. A request that is
// pending when ctx ends is dropped. Run blocks until ctx ends.
func (s *Snapshotter) Run(ctx context.Context, window time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.requests:
		}
		if !sleep(ctx, window) {
			return
		}
		// The requests since the first one are part of this snapshot.
		select {
		case <-s.requests:
		default:
		}
		if err := s.writer.WriteShoppingSnapshot(ctx); err != nil && ctx.Err() == nil {
			s.logger.LogAttrs(ctx, slog.LevelWarn, "write shopping snapshot", slog.String("error", err.Error()))
		}
	}
}
