package outbox_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/outbox"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// insertSnapshotProducts inserts products for the snapshot tests, not in the
// order of their names.
func insertSnapshotProducts(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	for _, p := range []product{
		// Nothing missing and not marked: not in the snapshot.
		{id: "p1", name: "Zucker", stock: 0, target: 0},
		// Something missing.
		{id: "p3", name: "Kidneybohnen", stock: 2, target: 5},
		// The same name in lower case: sorted before p3 by id.
		{id: "p2", name: "kidneybohnen", stock: 0, target: 4, minStock: new(int64(1))},
		// Marked without target.
		{id: "p4", name: "apfelsaft", stock: 0, target: 0, marked: 1},
		// Crate size, the example of architecture.md 11.3.
		{id: "p5", name: "Jever Pilsener", stock: 7, target: 24, crateSize: new(int64(20))},
		// Target, but nothing missing: in the snapshot, not on the list.
		{id: "p6", name: "Mehl", stock: 2, target: 4, minStock: new(int64(1))},
		{id: "p7", name: "Wasser", stock: 30, target: 24, crateSize: new(int64(6))},
		// Marked and something missing.
		{id: "p8", name: "Milch", stock: 1, target: 3, marked: 1},
	} {
		insertProduct(t, sqlDB, p)
	}
}

// snapshotItems is the snapshot of insertSnapshotProducts.
const snapshotItems = `[` +
	`{"product_id":"p4","name":"apfelsaft","on_list":true,"missing":0,"quantity":0,"unit":"piece"},` +
	`{"product_id":"p5","name":"Jever Pilsener","on_list":true,"missing":17,"quantity":1,"unit":"crate"},` +
	`{"product_id":"p2","name":"kidneybohnen","on_list":true,"missing":4,"quantity":4,"unit":"piece"},` +
	`{"product_id":"p3","name":"Kidneybohnen","on_list":true,"missing":3,"quantity":3,"unit":"piece"},` +
	`{"product_id":"p6","name":"Mehl","on_list":false,"missing":0,"quantity":0,"unit":"piece"},` +
	`{"product_id":"p8","name":"Milch","on_list":true,"missing":2,"quantity":2,"unit":"piece"},` +
	`{"product_id":"p7","name":"Wasser","on_list":false,"missing":0,"quantity":0,"unit":"crate"}]`

// clearedItems is the snapshot of insertSnapshotProducts that clears a list.
const clearedItems = `[` +
	`{"product_id":"p4","name":"apfelsaft","on_list":false,"missing":0,"quantity":0,"unit":"piece"},` +
	`{"product_id":"p5","name":"Jever Pilsener","on_list":false,"missing":17,"quantity":0,"unit":"crate"},` +
	`{"product_id":"p2","name":"kidneybohnen","on_list":false,"missing":4,"quantity":0,"unit":"piece"},` +
	`{"product_id":"p3","name":"Kidneybohnen","on_list":false,"missing":3,"quantity":0,"unit":"piece"},` +
	`{"product_id":"p6","name":"Mehl","on_list":false,"missing":0,"quantity":0,"unit":"piece"},` +
	`{"product_id":"p8","name":"Milch","on_list":false,"missing":2,"quantity":0,"unit":"piece"},` +
	`{"product_id":"p7","name":"Wasser","on_list":false,"missing":0,"quantity":0,"unit":"crate"}]`

// checkSnapshotEntry checks that e is a shopping.snapshot, written after
// start, with the JSON data.
func checkSnapshotEntry(t *testing.T, e entry, start time.Time, data string) {
	t.Helper()
	if e.topic != "events/shopping.snapshot" {
		t.Errorf("topic = %q, want events/shopping.snapshot", e.topic)
	}
	var ev map[string]json.RawMessage
	if err := json.Unmarshal([]byte(e.payload), &ev); err != nil {
		t.Fatalf("decode payload %s: %v", e.payload, err)
	}
	if keys, want := slices.Sorted(maps.Keys(ev)), []string{"data", "id", "source", "time", "type"}; !slices.Equal(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
	var id, typ, source, tm string
	for k, v := range map[string]*string{"id": &id, "type": &typ, "source": &source, "time": &tm} {
		if err := json.Unmarshal(ev[k], v); err != nil {
			t.Errorf("%s = %s, want a string", k, ev[k])
		}
	}
	if u, err := uuid.Parse(id); err != nil || u.Version() != 7 || id != strings.ToLower(id) {
		t.Errorf("id = %q, want UUIDv7 in lower case", id)
	}
	if typ != "shopping.snapshot" || source != "stashbert" {
		t.Errorf("type, source = %q, %q, want shopping.snapshot, stashbert", typ, source)
	}
	if at, err := time.Parse(time.RFC3339Nano, tm); err != nil || at.Before(start) || at.After(time.Now()) || !strings.HasSuffix(tm, "Z") {
		t.Errorf("time = %q, want the time of the write in UTC", tm)
	}
	if got := string(ev["data"]); got != data {
		t.Errorf("data:\n got %s\nwant %s", got, data)
	}
	if e.attempts != 0 || e.lastError != nil {
		t.Errorf("attempts, last_error = %d, %v, want 0, NULL", e.attempts, e.lastError)
	}
	createdAt, err := store.ParseTime(e.createdAt)
	if err != nil || createdAt.Before(start.Truncate(time.Millisecond)) || createdAt.After(time.Now()) {
		t.Errorf("created_at = %q, want the time of the write in %s", e.createdAt, store.TimeLayout)
	}
}

func TestWriteShoppingSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		products bool
		target   string // stored setting shopping_target_id, "" for none
		data     string
	}{
		{
			name: "chosen target list", products: true, target: "todo.bring_zuhause",
			data: `{"list":"todo.bring_zuhause","items":` + snapshotItems + `}`,
		},
		{
			name: "without target list", products: true,
			data: `{"list":"","items":` + snapshotItems + `}`,
		},
		{
			name: "without products",
			data: `{"list":"","items":[]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB := openDB(t)
			if tt.products {
				insertSnapshotProducts(t, sqlDB)
			}
			if tt.target != "" {
				exec(t, sqlDB, "INSERT INTO settings (key, value) VALUES ('shopping_target_id', ?)", tt.target)
			}
			var wakes, reported atomic.Int64
			logger, _ := newLogger()
			w := outbox.NewWriter(sqlDB, func() { wakes.Add(1) }, func() { reported.Add(1) }, logger)
			start := time.Now()

			if err := w.WriteShoppingSnapshot(testContext(t)); err != nil {
				t.Fatalf("write snapshot: %v", err)
			}

			got := entries(t, sqlDB)
			if len(got) != 1 {
				t.Fatalf("outbox = %+v, want one entry", got)
			}
			checkSnapshotEntry(t, got[0], start, tt.data)
			if n := wakes.Load(); n != 1 {
				t.Errorf("wake called %d times, want 1", n)
			}
			// A snapshot changes nothing, so it is no event for the summary.
			if n := reported.Load(); n != 0 {
				t.Errorf("onEvent called %d times, want 0", n)
			}
		})
	}
}

// A failed write returns the error and does not wake the deliverer.
func TestWriteShoppingSnapshotError(t *testing.T) {
	sqlDB := openDB(t)
	logger, _ := newLogger()
	var wakes atomic.Int64
	w := outbox.NewWriter(sqlDB, func() { wakes.Add(1) }, func() {}, logger)
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	if err := w.WriteShoppingSnapshot(testContext(t)); err == nil {
		t.Error("WriteShoppingSnapshot = nil, want an error")
	}
	if n := wakes.Load(); n != 0 {
		t.Errorf("wake called %d times, want 0", n)
	}
}

// snapshotWindow is the window of the Snapshotter in the tests.
const snapshotWindow = 200 * time.Millisecond

// runSnapshotter starts s.Run with window and ends it when the test ends.
func runSnapshotter(t *testing.T, s *outbox.Snapshotter, window time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx, window)
	}()
	t.Cleanup(func() {
		cancel()
		receive(t, done, "Run to return")
	})
}

// checkEntries asserts that the outbox holds n entries and keeps holding
// them for d.
func checkEntries(t *testing.T, sqlDB *sql.DB, n int, d time.Duration) {
	t.Helper()
	for deadline := time.Now().Add(d); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if got := len(entries(t, sqlDB)); got != n {
			t.Fatalf("outbox has %d entries, want %d", got, n)
		}
	}
}

// Requests within the window result in one snapshot, written when the
// window has passed; a later request results in another one.
func TestSnapshotterCoalescesRequests(t *testing.T) {
	sqlDB := openDB(t)
	insertSnapshotProducts(t, sqlDB)
	logger, logs := newLogger()
	s := outbox.NewSnapshotter(outbox.NewWriter(sqlDB, func() {}, func() {}, logger), "vorrat", logger)
	runSnapshotter(t, s, snapshotWindow)
	start := time.Now()

	s.Request()
	// Meanwhile Run has taken the first request, and the next ones come
	// within its window.
	time.Sleep(snapshotWindow / 4)
	s.Receive(mqtt.Message{Topic: "vorrat/in/snapshot", Payload: []byte("{}")})
	s.Request()
	s.Request()

	eventually(t, "the snapshot is written", func() bool { return len(entries(t, sqlDB)) > 0 })
	if elapsed := time.Since(start); elapsed < snapshotWindow {
		t.Errorf("snapshot written after %v, want after the window of %v", elapsed, snapshotWindow)
	}
	checkEntries(t, sqlDB, 1, 2*snapshotWindow)
	checkSnapshotEntry(t, entries(t, sqlDB)[0], start, `{"list":"","items":`+snapshotItems+`}`)

	s.Request()

	eventually(t, "the second snapshot is written", func() bool { return len(entries(t, sqlDB)) > 1 })
	checkEntries(t, sqlDB, 2, 2*snapshotWindow)
	if logs.String() != "" {
		t.Errorf("log = %s, want none", logs)
	}
}

// Only messages on <prefix>/in/snapshot without the retain flag request a
// snapshot; their payload does not matter (architecture.md, 11.2).
func TestSnapshotterReceive(t *testing.T) {
	sqlDB := openDB(t)
	logger, _ := newLogger()
	s := outbox.NewSnapshotter(outbox.NewWriter(sqlDB, func() {}, func() {}, logger), "vorrat", logger)
	runSnapshotter(t, s, snapshotWindow/10)

	for _, m := range []mqtt.Message{
		{Topic: "vorrat/in/snapshot", Payload: []byte("{}"), Retained: true},
		{Topic: "vorrat/in/snapshot", Retained: true},
		{Topic: "vorrat/in/targets", Payload: []byte("[]")},
		{Topic: "vorrat/in/snapshot/x", Payload: []byte("{}")},
		{Topic: "stashbert/in/snapshot", Payload: []byte("{}")},
		{Topic: "homeassistant/status", Payload: []byte("online")},
	} {
		s.Receive(m)
	}
	checkEntries(t, sqlDB, 0, 2*snapshotWindow)

	s.Receive(mqtt.Message{Topic: "vorrat/in/snapshot", Payload: []byte("not json")})

	eventually(t, "the snapshot is written", func() bool { return len(entries(t, sqlDB)) > 0 })
	checkEntries(t, sqlDB, 1, 2*snapshotWindow)
}

// A failed write is logged with level warn, and Run goes on.
func TestSnapshotterError(t *testing.T) {
	sqlDB := openDB(t)
	logger, logs := newLogger()
	s := outbox.NewSnapshotter(outbox.NewWriter(sqlDB, func() {}, func() {}, logger), "vorrat", logger)
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	runSnapshotter(t, s, time.Millisecond)

	s.Request()

	eventually(t, "the error is logged", func() bool { return strings.Contains(logs.String(), "write shopping snapshot") })
	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(logs.String())), &line); err != nil {
		t.Fatalf("log = %q, want one JSON line: %v", logs, err)
	}
	if msg, _ := line["error"].(string); line["level"] != "WARN" || line["msg"] != "write shopping snapshot" || msg == "" {
		t.Errorf("log = %s, want a warning with the error", logs)
	}
}
