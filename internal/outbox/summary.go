package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
)

// SummaryDelay is the time after the last of several events in quick
// succession after which the summary is published (architecture.md, 11.5).
const SummaryDelay = time.Second

// SummaryInterval is the time between the periodic publishes of the
// summary, which catch the changes of background jobs (architecture.md,
// 11.5).
const SummaryInterval = 5 * time.Minute

// summaryMessage is the summary on <p>/state/summary, the same JSON as the
// response of GET /summary (architecture.md, 11.5).
type summaryMessage struct {
	ProductCount      int64         `json:"product_count"`
	ShoppingCount     int64         `json:"shopping_count"`
	EmptyCount        int64         `json:"empty_count"`
	ReviewCount       int64         `json:"review_count"`
	Shopping          []summaryItem `json:"shopping"`
	ShoppingTruncated bool          `json:"shopping_truncated"`
}

// summaryItem is an entry of the shopping list in summaryMessage.
type summaryItem struct {
	Name     string `json:"name"`
	Missing  int64  `json:"missing"`
	Quantity int64  `json:"quantity"`
	Unit     string `json:"unit"`
}

// SummaryPublisher publishes the summary retained on <prefix>/state/summary
// (architecture.md, 11.5). It does not use the outbox: without a connection
// it skips the publish, and the next connection publishes the summary again.
type SummaryPublisher struct {
	db       *sql.DB
	client   Client
	topic    string
	logger   *slog.Logger
	requests chan struct{}
	// turn lets one publish run at a time, so that an older summary never
	// replaces a newer one.
	turn chan struct{}
}

// NewSummaryPublisher returns a SummaryPublisher that reads the products
// from sqlDB and publishes with client to the topic <prefix>/state/summary,
// where prefix is MQTT_TOPIC_PREFIX.
func NewSummaryPublisher(sqlDB *sql.DB, client Client, prefix string, logger *slog.Logger) *SummaryPublisher {
	return &SummaryPublisher{
		db: sqlDB, client: client, topic: prefix + "/state/summary", logger: logger,
		requests: make(chan struct{}, 1), turn: make(chan struct{}, 1),
	}
}

// Request asks Run for a publish after an event. It never blocks and may be
// called from any goroutine, also before Run. Pass it to NewWriter as
// onEvent.
func (s *SummaryPublisher) Request() {
	select {
	case s.requests <- struct{}{}:
	default:
	}
}

// Publish reads the summary and publishes it right away, retained with QoS
// 1, and waits for the PUBACK. Without a connection it does nothing. An
// error is logged with level warn unless ctx has ended. Publishes run one
// after another; Publish may be called from any goroutine. Register it with
// mqtt.Client.OnConnect, so that every connection publishes the summary.
func (s *SummaryPublisher) Publish(ctx context.Context) {
	select {
	case s.turn <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-s.turn }()
	if s.client.State() != mqtt.StateConnected {
		return
	}
	if err := s.publish(ctx); err != nil && ctx.Err() == nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "publish summary", slog.String("error", err.Error()))
	}
}

func (s *SummaryPublisher) publish(ctx context.Context) error {
	sum, err := domain.GetSummary(ctx, s.db)
	if err != nil {
		return err
	}
	msg := summaryMessage{
		ProductCount:      sum.ProductCount,
		ShoppingCount:     sum.ShoppingCount,
		EmptyCount:        sum.EmptyCount,
		ReviewCount:       sum.ReviewCount,
		Shopping:          make([]summaryItem, len(sum.Shopping)),
		ShoppingTruncated: sum.ShoppingTruncated,
	}
	for i, it := range sum.Shopping {
		msg.Shopping[i] = summaryItem{Name: it.Name, Missing: it.Missing, Quantity: it.Quantity, Unit: it.Unit}
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	return s.client.Publish(ctx, s.topic, payload, true)
}

// Run publishes the summary delay after the last of several requests in
// quick succession, until ctx ends: every request starts the delay anew,
// and when it has passed without a request, Run calls Publish. A request
// during Publish starts the next delay. In addition, Run calls Publish
// every interval, which must be greater than 0. A pending request is
// dropped when ctx ends. Run blocks until ctx ends.
func (s *SummaryPublisher) Run(ctx context.Context, delay, interval time.Duration) {
	timer := time.NewTimer(delay)
	timer.Stop()
	defer timer.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.requests:
			// Since Go 1.23, Reset drops a value that the timer has not
			// delivered yet.
			timer.Reset(delay)
		case <-timer.C:
			s.Publish(ctx)
		case <-ticker.C:
			s.Publish(ctx)
		}
	}
}
