package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// The backoff after failed deliveries (architecture.md, 11.4).
const (
	MinBackoff = time.Second
	MaxBackoff = 5 * time.Minute
)

const (
	// batchSize is the number of entries read at once at most.
	batchSize = 50
	// maxAge is the age after which an entry is deleted without delivery.
	maxAge = 7 * 24 * time.Hour
)

// Client publishes to the MQTT broker. *mqtt.Client implements it.
type Client interface {
	// State reports the state of the connection.
	State() mqtt.State
	// Publish sends payload to topic with QoS 1 and waits for the PUBACK.
	Publish(ctx context.Context, topic string, payload []byte, retain bool) error
}

// Deliverer publishes the entries of the outbox in their order and deletes
// them after the PUBACK (architecture.md, 11.4).
type Deliverer struct {
	db     *sql.DB
	client Client
	prefix string
	logger *slog.Logger
	wake   chan struct{}
}

// NewDeliverer returns a Deliverer for the outbox in sqlDB that publishes
// with client to the topics below prefix, MQTT_TOPIC_PREFIX. Wake should be
// called after every connection of client, so that the delivery starts.
func NewDeliverer(sqlDB *sql.DB, client Client, prefix string, logger *slog.Logger) *Deliverer {
	return &Deliverer{db: sqlDB, client: client, prefix: prefix, logger: logger, wake: make(chan struct{}, 1)}
}

// Wake makes Run look at the outbox again. It never blocks and may be called
// from any goroutine, also before Run.
func (d *Deliverer) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Run delivers the outbox until ctx ends. It makes a pass right away and
// after every Wake: it deletes the entries older than 7 days with a warn log
// of their count, and while the client is connected it reads the oldest
// entries, at most 50 at once, publishes each to <prefix>/<topic> with QoS 1
// and without retain, and deletes it after the PUBACK. A failed publish sets
// attempts and last_error of the entry and ends the pass, so the order stays.
// Run then logs the error with level warn and makes the next pass after
// minBackoff, doubled after every further failed pass up to maxBackoff; a
// Wake during that wait takes effect after it. A pass without error resets
// the backoff. Run blocks until ctx ends.
func (d *Deliverer) Run(ctx context.Context, minBackoff, maxBackoff time.Duration) {
	backoff := minBackoff
	for {
		err := d.pass(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			d.logger.LogAttrs(ctx, slog.LevelWarn, "deliver outbox",
				slog.String("error", err.Error()),
				slog.Duration("retry_in", backoff),
			)
			if !sleep(ctx, backoff) {
				return
			}
			backoff = min(2*backoff, maxBackoff)
			continue
		}
		backoff = minBackoff
		select {
		case <-ctx.Done():
			return
		case <-d.wake:
		}
	}
}

// pass deletes the old entries and, while the client is connected,
// publishes all entries in order until the outbox is empty or an error
// occurs.
func (d *Deliverer) pass(ctx context.Context) error {
	q := db.New(d.db)
	n, err := q.DeleteOutboxEntriesBefore(ctx, store.FormatTime(time.Now().Add(-maxAge)))
	if err != nil {
		return fmt.Errorf("delete old outbox entries: %w", err)
	}
	if n > 0 {
		d.logger.LogAttrs(ctx, slog.LevelWarn, "deleted undelivered outbox entries older than 7 days",
			slog.Int64("count", n))
	}
	for d.client.State() == mqtt.StateConnected {
		entries, err := q.ListOutboxEntries(ctx, batchSize)
		if err != nil {
			return fmt.Errorf("read outbox: %w", err)
		}
		for _, e := range entries {
			if err := d.deliver(ctx, q, e); err != nil {
				return err
			}
		}
		if len(entries) < batchSize {
			return nil
		}
	}
	return nil
}

// deliver publishes the entry e and deletes it. If the publish fails, it
// sets attempts and last_error of e unless ctx has ended.
func (d *Deliverer) deliver(ctx context.Context, q *db.Queries, e db.Outbox) error {
	if err := d.client.Publish(ctx, d.prefix+"/"+e.Topic, []byte(e.Payload), false); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := err.Error()
		if recErr := q.RecordOutboxFailure(ctx, db.RecordOutboxFailureParams{LastError: &msg, Seq: e.Seq}); recErr != nil {
			return fmt.Errorf("publish outbox entry %d: %w; record failure: %w", e.Seq, err, recErr)
		}
		return fmt.Errorf("publish outbox entry %d: %w", e.Seq, err)
	}
	if err := q.DeleteOutboxEntry(ctx, e.Seq); err != nil {
		return fmt.Errorf("delete delivered outbox entry %d: %w", e.Seq, err)
	}
	return nil
}

// sleep waits for d and reports false if ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
