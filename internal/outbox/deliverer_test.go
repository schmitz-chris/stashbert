package outbox_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/outbox"
	"github.com/schmitz-chris/stashbert/internal/store"
)

var _ outbox.Client = (*mqtt.Client)(nil)

// Short backoff for the tests.
const (
	minBackoff = 5 * time.Millisecond
	maxBackoff = 20 * time.Millisecond
)

// message is one call of Publish.
type message struct {
	topic, payload string
	retain         bool
	failed         bool
}

// fakeClient is an outbox.Client. Every call of Publish is sent to calls and
// fails while failing is set.
type fakeClient struct {
	calls  chan message
	states chan struct{} // signals every call of State

	mu      sync.Mutex
	state   mqtt.State
	failing bool
	// calledWhile counts the calls of Publish while not connected.
	calledWhile int
}

func newFakeClient(state mqtt.State) *fakeClient {
	return &fakeClient{calls: make(chan message, 1000), states: make(chan struct{}, 1000), state: state}
}

func (f *fakeClient) State() mqtt.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case f.states <- struct{}{}:
	default:
	}
	return f.state
}

func (f *fakeClient) Publish(_ context.Context, topic string, payload []byte, retain bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != mqtt.StateConnected {
		f.calledWhile++
	}
	f.calls <- message{topic, string(payload), retain, f.failing}
	if f.failing {
		return errors.New("broker rejected the message")
	}
	return nil
}

func (f *fakeClient) set(state mqtt.State, failing bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state, f.failing = state, failing
}

func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(wait):
		t.Fatalf("timeout waiting for %s", what)
	}
	var zero T
	return zero
}

// eventually waits until cond is true.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting until %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// run starts d.Run with the short backoff and ends it when the test ends.
func run(t *testing.T, d *outbox.Deliverer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.Run(ctx, minBackoff, maxBackoff)
	}()
	t.Cleanup(func() {
		cancel()
		receive(t, done, "Run to return")
	})
}

// insertEntry inserts an outbox entry with a direct SQL insert.
func insertEntry(t *testing.T, sqlDB *sql.DB, topic, payload string, createdAt time.Time) {
	t.Helper()
	exec(t, sqlDB, "INSERT INTO outbox (topic, payload, created_at) VALUES (?, ?, ?)",
		topic, payload, store.FormatTime(createdAt))
}

// More entries than one batch of 50 are published in the order of seq, to
// the topic below the prefix, without retain, and deleted.
func TestDeliverInOrder(t *testing.T) {
	sqlDB := openDB(t)
	const n = 120
	for i := range n {
		insertEntry(t, sqlDB, fmt.Sprintf("events/test.%d", i%3), fmt.Sprintf(`{"n":%d}`, i), time.Now())
	}
	client := newFakeClient(mqtt.StateConnected)
	logger, logs := newLogger()
	run(t, outbox.NewDeliverer(sqlDB, client, "vorrat", logger))

	for i := range n {
		want := message{fmt.Sprintf("vorrat/events/test.%d", i%3), fmt.Sprintf(`{"n":%d}`, i), false, false}
		if got := receive(t, client.calls, fmt.Sprintf("message %d", i)); got != want {
			t.Fatalf("message %d = %+v, want %+v", i, got, want)
		}
	}
	eventually(t, "the outbox is empty", func() bool { return len(entries(t, sqlDB)) == 0 })
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want nothing", out)
	}
}

// The writer wakes the deliverer, which publishes the events right away.
func TestWriterWakesDeliverer(t *testing.T) {
	sqlDB := openDB(t)
	client := newFakeClient(mqtt.StateConnected)
	logger, logs := newLogger()
	d := outbox.NewDeliverer(sqlDB, client, "stashbert", logger)
	w := outbox.NewWriter(sqlDB, d.Wake, func() {}, logger)
	run(t, d)
	receive(t, client.states, "the first pass")

	insertProduct(t, sqlDB, product{id: "p1", name: "Kidneybohnen", stock: 0, target: 2})
	published := []events.Event{
		events.New(events.TypeStockConsumed, events.StockData{ProductID: "p1", MovementID: "m1", Delta: -1, StockAfter: 0}),
		events.New(events.TypeProductEmpty, events.ProductEmptyData{ProductID: "p1", Name: "Kidneybohnen"}),
		events.New(events.TypeShoppingChanged, events.ShoppingChangedData{ProductID: "p1", Name: "Kidneybohnen", MissingBefore: 1, MissingAfter: 2}),
	}
	for _, e := range published {
		w.Publish(testContext(t), e)
	}

	for _, e := range published {
		got := receive(t, client.calls, e.Type)
		if want := "stashbert/events/" + e.Type; got.topic != want || got.retain {
			t.Errorf("message = %s retain %v, want %s without retain", got.topic, got.retain, want)
		}
		if !strings.Contains(got.payload, `"id":"`+e.ID+`"`) {
			t.Errorf("payload of %s = %s, want the event %s", e.Type, got.payload, e.ID)
		}
	}
	eventually(t, "the outbox is empty", func() bool { return len(entries(t, sqlDB)) == 0 })
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want nothing", out)
	}
}

// Without a connection the deliverer publishes nothing; after the connection
// and a Wake it delivers.
func TestDeliverWaitsForConnection(t *testing.T) {
	sqlDB := openDB(t)
	insertEntry(t, sqlDB, "events/a", `{"n":1}`, time.Now())
	client := newFakeClient(mqtt.StateConnecting)
	logger, _ := newLogger()
	d := outbox.NewDeliverer(sqlDB, client, "stashbert", logger)
	run(t, d)

	receive(t, client.states, "the first pass")
	d.Wake()
	receive(t, client.states, "the pass after Wake")
	if got := len(entries(t, sqlDB)); got != 1 {
		t.Errorf("outbox has %d entries without connection, want 1", got)
	}

	client.set(mqtt.StateConnected, false)
	d.Wake()
	if got := receive(t, client.calls, "message after the connection"); got.topic != "stashbert/events/a" {
		t.Errorf("topic = %q, want stashbert/events/a", got.topic)
	}
	eventually(t, "the outbox is empty", func() bool { return len(entries(t, sqlDB)) == 0 })
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.calledWhile != 0 {
		t.Errorf("Publish called %d times without connection, want 0", client.calledWhile)
	}
}

// A failed publish keeps the entry with attempts and last_error, stops the
// batch and is tried again after the backoff; the order stays.
func TestDeliverRetryAfterFailure(t *testing.T) {
	sqlDB := openDB(t)
	insertEntry(t, sqlDB, "events/a", `{"n":1}`, time.Now())
	insertEntry(t, sqlDB, "events/b", `{"n":2}`, time.Now())
	client := newFakeClient(mqtt.StateConnected)
	client.set(mqtt.StateConnected, true)
	logger, logs := newLogger()
	run(t, outbox.NewDeliverer(sqlDB, client, "stashbert", logger))

	// Each failure is recorded before the backoff, so after the third call
	// the first two are stored.
	for i := range 3 {
		if got := receive(t, client.calls, "failing call"); got.topic != "stashbert/events/a" {
			t.Fatalf("call %d = %s, want stashbert/events/a", i, got.topic)
		}
	}
	got := entries(t, sqlDB)
	if len(got) != 2 {
		t.Fatalf("outbox = %+v, want both entries", got)
	}
	if a := got[0]; a.attempts < 2 || a.lastError == nil ||
		*a.lastError != "broker rejected the message" {
		t.Errorf("entry a: attempts %d, last_error %v, want at least 2 and the error", a.attempts, a.lastError)
	}
	if b := got[1]; b.attempts != 0 || b.lastError != nil {
		t.Errorf("entry b: attempts %d, last_error %v, want 0 and NULL", b.attempts, b.lastError)
	}

	// From now on the broker accepts: a follows until one call succeeds, then
	// b, and nothing else.
	client.set(mqtt.StateConnected, false)
	var last message
	for {
		m := receive(t, client.calls, "stashbert/events/b")
		if m.topic == "stashbert/events/b" {
			if last.topic != "stashbert/events/a" || last.failed || m.failed {
				t.Errorf("b published after %+v, want after a successful a", last)
			}
			break
		}
		if m.topic != "stashbert/events/a" || (last.topic == "stashbert/events/a" && !last.failed) {
			t.Fatalf("call %+v after %+v, want a until it succeeds", m, last)
		}
		last = m
	}
	eventually(t, "the outbox is empty", func() bool { return len(entries(t, sqlDB)) == 0 })
	select {
	case m := <-client.calls:
		t.Errorf("call %+v after delivering all entries", m)
	default:
	}
	if out := logs.String(); !strings.Contains(out, `"level":"WARN","msg":"deliver outbox"`) ||
		!strings.Contains(out, "broker rejected the message") {
		t.Errorf("log = %s, want a warning with the error", out)
	}
}

// Entries older than 7 days are deleted with a warning of their count, also
// without a connection, and never published.
func TestDeliverDeletesOldEntries(t *testing.T) {
	sqlDB := openDB(t)
	now := time.Now()
	insertEntry(t, sqlDB, "events/old", `{"n":1}`, now.Add(-8*24*time.Hour))
	insertEntry(t, sqlDB, "events/recent", `{"n":2}`, now.Add(-6*24*time.Hour))
	insertEntry(t, sqlDB, "events/old", `{"n":3}`, now.Add(-7*24*time.Hour-time.Minute))
	insertEntry(t, sqlDB, "events/recent", `{"n":4}`, now)
	client := newFakeClient(mqtt.StateConnecting)
	logger, logs := newLogger()
	d := outbox.NewDeliverer(sqlDB, client, "stashbert", logger)
	run(t, d)

	receive(t, client.states, "the first pass")
	got := entries(t, sqlDB)
	if len(got) != 2 || got[0].payload != `{"n":2}` || got[1].payload != `{"n":4}` {
		t.Errorf("outbox = %+v, want the two recent entries", got)
	}
	if out := logs.String(); !strings.Contains(out, `"level":"WARN","msg":"deleted undelivered outbox entries older than 7 days","count":2`) {
		t.Errorf("log = %s, want a warning with count 2", out)
	}

	client.set(mqtt.StateConnected, false)
	d.Wake()
	for _, want := range []string{`{"n":2}`, `{"n":4}`} {
		if m := receive(t, client.calls, want); m.topic != "stashbert/events/recent" || m.payload != want {
			t.Errorf("message = %+v, want %s on stashbert/events/recent", m, want)
		}
	}
	eventually(t, "the outbox is empty", func() bool { return len(entries(t, sqlDB)) == 0 })
	if n := strings.Count(logs.String(), "older than 7 days"); n != 1 {
		t.Errorf("%d warnings about old entries, want 1", n)
	}
}
