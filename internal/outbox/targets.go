package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// SetShoppingTarget stores t as the chosen target list, or removes the
// choice if t is nil, and writes the snapshots of the switch into the
// outbox, all in one transaction (architecture.md, 11.7): first
// shopping.snapshot for the old list with on_list false and quantity 0 for
// every item, if a list was chosen, then shopping.snapshot for the new list,
// if there is one. Afterwards shopping.changed carries the new list. If t
// has the id of the stored choice, or neither exists, nothing is changed
// and no snapshot is written. After written snapshots it calls wake.
func (w *Writer) SetShoppingTarget(ctx context.Context, t *domain.ShoppingTarget) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set shopping target: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	old, err := domain.StoredShoppingTarget(ctx, q)
	if err != nil {
		return fmt.Errorf("set shopping target: %w", err)
	}
	if (old == nil && t == nil) || (old != nil && t != nil && old.ID == t.ID) {
		return nil
	}
	if err := domain.StoreShoppingTarget(ctx, q, t); err != nil {
		return fmt.Errorf("set shopping target: %w", err)
	}
	if old != nil {
		if err := insertSnapshot(ctx, q, old.ID, true); err != nil {
			return fmt.Errorf("set shopping target: clear list %s: %w", old.ID, err)
		}
	}
	if t != nil {
		if err := insertSnapshot(ctx, q, t.ID, false); err != nil {
			return fmt.Errorf("set shopping target: fill list %s: %w", t.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set shopping target: commit: %w", err)
	}
	w.wake()
	return nil
}

// Targets keeps in memory the target lists that Home Assistant offers
// retained on <prefix>/in/targets, and chooses one of them (architecture.md,
// 11.7).
type Targets struct {
	writer *Writer
	client Client
	topic  string
	logger *slog.Logger

	mu    sync.Mutex
	offer []domain.ShoppingTarget
}

// NewTargets returns Targets for the offer on the topic <prefix>/in/targets,
// where prefix is MQTT_TOPIC_PREFIX, that reports the state of client and
// writes the choice with w.
func NewTargets(w *Writer, client Client, prefix string, logger *slog.Logger) *Targets {
	return &Targets{
		writer: w, client: client, topic: prefix + "/in/targets", logger: logger,
		offer: []domain.ShoppingTarget{},
	}
}

// Receive takes a message on <prefix>/in/targets as the new offer, also one
// with the retain flag. An invalid payload is logged with level warn and
// ignored; the last valid offer stays. Messages on other topics are
// ignored. Register it with mqtt.Client.OnMessage.
func (t *Targets) Receive(m mqtt.Message) {
	if m.Topic != t.topic {
		return
	}
	offer, err := domain.ParseShoppingTargets(m.Payload)
	if err != nil {
		t.logger.LogAttrs(context.Background(), slog.LevelWarn, "ignore invalid target lists",
			slog.String("topic", m.Topic), slog.String("error", err.Error()))
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.offer = offer
}

// Offer returns the target lists of the last valid offer in their order,
// empty before the first one.
func (t *Targets) Offer() []domain.ShoppingTarget {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.offer)
}

// State reports the state of the connection to the broker.
func (t *Targets) State() mqtt.State {
	return t.client.State()
}

// SetTarget stores target as the chosen target list, or removes the choice
// if target is nil, and writes the snapshots of the switch
// (Writer.SetShoppingTarget).
func (t *Targets) SetTarget(ctx context.Context, target *domain.ShoppingTarget) error {
	return t.writer.SetShoppingTarget(ctx, target)
}
