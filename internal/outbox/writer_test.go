package outbox_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/outbox"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// wait is the longest a test waits for something to happen.
const wait = 5 * time.Second

// testContext returns a context that ends with the test or after 30 s.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// openDB returns a migrated database in t.TempDir().
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := testContext(t)
	sqlDB, err := store.Open(ctx, filepath.Join(t.TempDir(), "stashbert.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := store.Migrate(ctx, sqlDB, store.Migrations); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	return sqlDB
}

func exec(t *testing.T, sqlDB *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := sqlDB.ExecContext(testContext(t), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// product is a product for insertProduct.
type product struct {
	id, name              string
	stock, target, marked int64
	minStock, crateSize   *int64
}

func insertProduct(t *testing.T, sqlDB *sql.DB, p product) {
	t.Helper()
	exec(t, sqlDB, `INSERT INTO products (id, name, stock, target, min_stock, marked, crate_size,
		origin, lookup_state, needs_review, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'manual', 'none', 0, '2026-09-24T12:00:00.000Z', '2026-09-24T12:00:00.000Z')`,
		p.id, p.name, p.stock, p.target, p.minStock, p.marked, p.crateSize)
}

// entry is a row of the outbox.
type entry struct {
	seq       int64
	topic     string
	payload   string
	createdAt string
	attempts  int64
	lastError *string
}

// entries returns the rows of the outbox ordered by seq.
func entries(t *testing.T, sqlDB *sql.DB) []entry {
	t.Helper()
	rows, err := sqlDB.QueryContext(testContext(t),
		"SELECT seq, topic, payload, created_at, attempts, last_error FROM outbox ORDER BY seq")
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()
	var out []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.seq, &e.topic, &e.payload, &e.createdAt, &e.attempts, &e.lastError); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	return out
}

// logBuffer collects log output.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newLogger returns a JSON logger that writes into a new buffer.
func newLogger() (*slog.Logger, *logBuffer) {
	b := &logBuffer{}
	return slog.New(slog.NewJSONHandler(b, nil)), b
}

// eventAt returns an event with a fixed id and time, like the example in
// architecture.md 11.3.
func eventAt(typ string, data any) events.Event {
	return events.Event{
		ID:     "01a0d5c2-7f3e-7a41-9c1d-3b2f4e5a6b7c",
		Type:   typ,
		Source: "stashbert",
		Time:   time.Date(2026, 9, 24, 18, 3, 11, 482_000_000, time.UTC),
		Data:   data,
	}
}

var _ events.Publisher = (*outbox.Writer)(nil)

func TestWriterShoppingChanged(t *testing.T) {
	const jever = "01a0cfbb-6b1f-7943-a616-68709e8c19b1"
	tests := []struct {
		name    string
		product *product // nil: the product does not exist (deleted or merged)
		target  string   // stored setting shopping_target_id, "" for none
		data    events.ShoppingChangedData
		want    string // data of the payload
	}{
		{
			name:    "product on the list",
			product: &product{id: "p1", name: "Kidneybohnen", stock: 2, target: 5},
			data:    events.ShoppingChangedData{ProductID: "p1", Name: "Kidneybohnen", MissingBefore: 2, MissingAfter: 3},
			want: `{"product_id":"p1","name":"Kidneybohnen","missing_before":2,"missing_after":3,` +
				`"marked":false,"on_list":true,"quantity":3,"unit":"piece","list":""}`,
		},
		{
			name:    "crate size and stored target list, the example of architecture.md 11.3",
			product: &product{id: jever, name: "Jever Pilsener", stock: 7, target: 24, crateSize: new(int64(20))},
			target:  "todo.bring_zuhause",
			data:    events.ShoppingChangedData{ProductID: jever, Name: "Jever Pilsener", MissingBefore: 0, MissingAfter: 17},
			want: `{"product_id":"01a0cfbb-6b1f-7943-a616-68709e8c19b1","name":"Jever Pilsener","missing_before":0,"missing_after":17,` +
				`"marked":false,"on_list":true,"quantity":1,"unit":"crate","list":"todo.bring_zuhause"}`,
		},
		{
			name: "deleted product",
			data: events.ShoppingChangedData{ProductID: "p1", Name: "Kidneybohnen", MissingBefore: 3, MissingAfter: 0},
			want: `{"product_id":"p1","name":"Kidneybohnen","missing_before":3,"missing_after":0,` +
				`"marked":false,"on_list":false,"quantity":0,"unit":"piece","list":""}`,
		},
		{
			name:   "deleted product with stored target list",
			target: "todo.einkauf",
			data:   events.ShoppingChangedData{ProductID: "p1", Name: "Kidneybohnen", MissingBefore: 3, MissingAfter: 0},
			want: `{"product_id":"p1","name":"Kidneybohnen","missing_before":3,"missing_after":0,` +
				`"marked":false,"on_list":false,"quantity":0,"unit":"piece","list":"todo.einkauf"}`,
		},
		{
			name:    "marked without anything missing",
			product: &product{id: "p1", name: "Kidneybohnen", stock: 5, target: 5, marked: 1},
			data:    events.ShoppingChangedData{ProductID: "p1", Name: "Kidneybohnen", MissingBefore: 5, MissingAfter: 0},
			want: `{"product_id":"p1","name":"Kidneybohnen","missing_before":5,"missing_after":0,` +
				`"marked":true,"on_list":true,"quantity":0,"unit":"piece","list":""}`,
		},
		{
			name:    "crate size without anything missing",
			product: &product{id: "p1", name: "Jever Pilsener", stock: 30, target: 24, crateSize: new(int64(20))},
			data:    events.ShoppingChangedData{ProductID: "p1", Name: "Jever Pilsener", MissingBefore: 17, MissingAfter: 0},
			want: `{"product_id":"p1","name":"Jever Pilsener","missing_before":17,"missing_after":0,` +
				`"marked":false,"on_list":false,"quantity":0,"unit":"crate","list":""}`,
		},
		{
			name:    "min stock",
			product: &product{id: "p1", name: "Mehl", stock: 0, target: 4, minStock: new(int64(1))},
			data:    events.ShoppingChangedData{ProductID: "p1", Name: "Mehl", MissingBefore: 0, MissingAfter: 4},
			want: `{"product_id":"p1","name":"Mehl","missing_before":0,"missing_after":4,` +
				`"marked":false,"on_list":true,"quantity":4,"unit":"piece","list":""}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB := openDB(t)
			if tt.product != nil {
				insertProduct(t, sqlDB, *tt.product)
			}
			if tt.target != "" {
				exec(t, sqlDB, "INSERT INTO settings (key, value) VALUES ('shopping_target_id', ?)", tt.target)
			}
			var wakes atomic.Int64
			logger, logs := newLogger()
			w := outbox.NewWriter(sqlDB, func() { wakes.Add(1) }, logger)
			start := time.Now().UTC().Truncate(time.Millisecond)

			w.Publish(testContext(t), eventAt(events.TypeShoppingChanged, tt.data))

			got := entries(t, sqlDB)
			if len(got) != 1 {
				t.Fatalf("outbox = %+v, want one entry; log:\n%s", got, logs)
			}
			e := got[0]
			if e.topic != "events/shopping.changed" {
				t.Errorf("topic = %q, want events/shopping.changed", e.topic)
			}
			want := `{"id":"01a0d5c2-7f3e-7a41-9c1d-3b2f4e5a6b7c","type":"shopping.changed","source":"stashbert",` +
				`"time":"2026-09-24T18:03:11.482Z","data":` + tt.want + `}`
			if e.payload != want {
				t.Errorf("payload:\n got %s\nwant %s", e.payload, want)
			}
			if e.attempts != 0 || e.lastError != nil {
				t.Errorf("attempts, last_error = %d, %v, want 0, NULL", e.attempts, e.lastError)
			}
			createdAt, err := store.ParseTime(e.createdAt)
			if err != nil || createdAt.Before(start) || createdAt.After(time.Now()) {
				t.Errorf("created_at = %q, want the time of Publish in %s", e.createdAt, store.TimeLayout)
			}
			if n := wakes.Load(); n != 1 {
				t.Errorf("wake called %d times, want 1", n)
			}
		})
	}
}

// Other events go into the outbox unchanged, in the order of Publish.
func TestWriterOtherEvents(t *testing.T) {
	sqlDB := openDB(t)
	var wakes atomic.Int64
	logger, logs := newLogger()
	w := outbox.NewWriter(sqlDB, func() { wakes.Add(1) }, logger)
	published := []events.Event{
		events.New(events.TypeProductCreated, events.ProductCreatedData{ProductID: "p1", Name: "Kidneybohnen", Origin: "manual"}),
		events.New(events.TypeStockAdded, events.StockData{ProductID: "p1", MovementID: "m1", Delta: 3, StockAfter: 3}),
		events.New(events.TypeStockConsumed, events.StockData{ProductID: "p1", MovementID: "m2", Delta: -3, StockAfter: 0}),
		events.New(events.TypeProductEmpty, events.ProductEmptyData{ProductID: "p1", Name: "Kidneybohnen"}),
	}
	for _, e := range published {
		w.Publish(testContext(t), e)
	}

	got := entries(t, sqlDB)
	if len(got) != len(published) {
		t.Fatalf("outbox has %d entries, want %d; log:\n%s", len(got), len(published), logs)
	}
	for i, e := range published {
		want, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("encode event: %v", err)
		}
		if got[i].topic != "events/"+e.Type || got[i].payload != string(want) {
			t.Errorf("entry %d = %s %s, want events/%s %s", i, got[i].topic, got[i].payload, e.Type, want)
		}
		if i > 0 && got[i].seq <= got[i-1].seq {
			t.Errorf("seq of entry %d = %d, not after %d", i, got[i].seq, got[i-1].seq)
		}
	}
	if n := wakes.Load(); n != int64(len(published)) {
		t.Errorf("wake called %d times, want %d", n, len(published))
	}
}

// The business logic has committed before it publishes, so a request that
// ends right then still gets its event written.
func TestWriterIgnoresCancelledContext(t *testing.T) {
	sqlDB := openDB(t)
	logger, logs := newLogger()
	w := outbox.NewWriter(sqlDB, func() {}, logger)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	w.Publish(ctx, events.New(events.TypeStockAdded, events.StockData{ProductID: "p1", MovementID: "m1", Delta: 1, StockAfter: 1}))

	if got := entries(t, sqlDB); len(got) != 1 {
		t.Errorf("outbox = %+v, want one entry; log:\n%s", got, logs)
	}
}

// A failed write is logged with level warn and does not wake the deliverer;
// Publish returns normally.
func TestWriterError(t *testing.T) {
	sqlDB := openDB(t)
	logger, logs := newLogger()
	var wakes atomic.Int64
	w := outbox.NewWriter(sqlDB, func() { wakes.Add(1) }, logger)
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	e := events.New(events.TypeShoppingChanged, events.ShoppingChangedData{ProductID: "p1", Name: "Kidneybohnen"})
	w.Publish(testContext(t), e)

	out := strings.TrimSpace(logs.String())
	var line map[string]any
	if err := json.Unmarshal([]byte(out), &line); err != nil {
		t.Fatalf("log = %q, want one JSON line: %v", out, err)
	}
	if msg, _ := line["error"].(string); line["level"] != "WARN" || line["msg"] != "write event to outbox" ||
		line["event_id"] != e.ID || line["type"] != "shopping.changed" || msg == "" {
		t.Errorf("log = %s, want a warning with event_id, type and error", out)
	}
	if n := wakes.Load(); n != 0 {
		t.Errorf("wake called %d times, want 0", n)
	}
}
