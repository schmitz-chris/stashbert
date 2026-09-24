package app_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/outbox"
)

// mqttWait is the longest the MQTT tests wait for something to happen.
const mqttWait = 5 * time.Second

// startBroker starts an in-process broker on a random port of 127.0.0.1
// that every client may connect to, and returns it with its URL.
func startBroker(t *testing.T) (*mochi.Server, string) {
	t.Helper()
	srv := mochi.New(&mochi.Options{InlineClient: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := srv.AddHook(new(auth.AllowHook), nil); err != nil {
		t.Fatalf("add auth hook: %v", err)
	}
	tcp := listeners.NewTCP(listeners.Config{ID: "tcp", Address: "127.0.0.1:0"})
	if err := srv.AddListener(tcp); err != nil {
		t.Fatalf("add listener: %v", err)
	}
	if err := srv.Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	t.Cleanup(func() {
		// Close of mochi v2.7.9 deadlocks if the broker removes a client at
		// the same time (see internal/mqtt). The clients have stopped before
		// this cleanup runs; wait until only the inline client is left.
		deadline := time.Now().Add(mqttWait)
		for srv.Clients.Len() > 1 {
			if time.Now().After(deadline) {
				t.Errorf("broker still has %d clients, not closing it", srv.Clients.Len()-1)
				return
			}
			time.Sleep(time.Millisecond)
		}
		_ = srv.Close()
	})
	return srv, "mqtt://" + tcp.Address()
}

// startJob runs job in a goroutine until the test ends; the cleanup cancels
// its context and waits until it has returned.
func startJob(t *testing.T, job func(ctx context.Context)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		job(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(mqttWait):
			t.Error("timeout waiting for a background job to end")
		}
	})
}

// published is a message that the broker has received.
type published struct{ topic, payload string }

// TestMovementEventsOverMQTT books a movement through the HTTP API with the
// outbox wired as in main and receives its events on the broker
// (architecture.md, 11.3).
func TestMovementEventsOverMQTT(t *testing.T) {
	srv, url := startBroker(t)
	msgs := make(chan published, 16)
	if err := srv.Subscribe("vorrat/events/#", 1, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
		msgs <- published{pk.TopicName, string(pk.Payload)}
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	db := newDB(t)
	logger := slog.New(slog.DiscardHandler)
	client, err := mqtt.New(config.MQTT{URL: url, ClientID: "stashbert", TopicPrefix: "vorrat", HAPrefix: "homeassistant"}, logger)
	if err != nil {
		t.Fatalf("mqtt.New: %v", err)
	}
	deliverer := outbox.NewDeliverer(db, client, "vorrat", logger)
	client.OnConnect(func(context.Context) { deliverer.Wake() })
	h, _ := newAppWithDeps(t, app.Deps{
		DB: db, Publisher: outbox.NewWriter(db, deliverer.Wake, func() {}, logger),
		Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(),
	})
	// Stopped in reverse order: the deliverer first, then the client.
	startJob(t, func(ctx context.Context) {
		if err := client.Run(ctx); err != nil {
			t.Errorf("mqtt Run: %v", err)
		}
	})
	startJob(t, func(ctx context.Context) { deliverer.Run(ctx, outbox.MinBackoff, outbox.MaxBackoff) })

	// See insertProducts: p2 "kidneybohnen" has stock 2 and target 5.
	insertProducts(t, db)
	start := time.Now()
	_, movement := decodeMovementResult(t, post(h, "/api/v1/movements", `{"product_id": "p2", "kind": "add", "quantity": 3}`))

	stockData := map[string]any{"product_id": "p2", "movement_id": movement["id"], "delta": 3.0, "stock_after": 5.0}
	checkEventMessage(t, msgs, start, "vorrat/events/stock.added", "stock.added", stockData)
	shoppingData := map[string]any{
		"product_id": "p2", "name": "kidneybohnen", "missing_before": 3.0, "missing_after": 0.0,
		"marked": false, "on_list": false, "quantity": 0.0, "unit": "piece", "list": "",
	}
	checkEventMessage(t, msgs, start, "vorrat/events/shopping.changed", "shopping.changed", shoppingData)

	deadline := time.Now().Add(mqttWait)
	for countRows(t, db, "outbox") != 0 {
		if time.Now().After(deadline) {
			t.Fatal("outbox not empty after delivery")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestShoppingSnapshotOverMQTT requests shopping.snapshot through the HTTP
// API and on <p>/in/snapshot with the outbox and the snapshotter wired as in
// main, and receives it on the broker (architecture.md, 11.3).
func TestShoppingSnapshotOverMQTT(t *testing.T) {
	const window = 100 * time.Millisecond
	srv, url := startBroker(t)
	// Stored before StashBert subscribes, so it arrives with the retain flag
	// and is ignored.
	if err := srv.Publish("vorrat/in/snapshot", []byte("{}"), true, 1); err != nil {
		t.Fatalf("publish retained: %v", err)
	}
	msgs := make(chan published, 16)
	if err := srv.Subscribe("vorrat/events/#", 1, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
		msgs <- published{pk.TopicName, string(pk.Payload)}
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	db := newDB(t)
	// See insertProducts: all products but p1 "Zucker" have a target; p1 is
	// marked here.
	insertProducts(t, db)
	setMarked(t, db, "p1", 1)
	if _, err := db.ExecContext(t.Context(), "INSERT INTO settings (key, value) VALUES ('shopping_target_id', 'todo.bring_zuhause')"); err != nil {
		t.Fatalf("store target list: %v", err)
	}
	logger := slog.New(slog.DiscardHandler)
	client, err := mqtt.New(config.MQTT{URL: url, ClientID: "stashbert", TopicPrefix: "vorrat", HAPrefix: "homeassistant"}, logger)
	if err != nil {
		t.Fatalf("mqtt.New: %v", err)
	}
	deliverer := outbox.NewDeliverer(db, client, "vorrat", logger)
	client.OnConnect(func(context.Context) { deliverer.Wake() })
	writer := outbox.NewWriter(db, deliverer.Wake, func() {}, logger)
	snapshotter := outbox.NewSnapshotter(writer, "vorrat", logger)
	client.OnMessage(snapshotter.Receive)
	incoming := make(chan mqtt.Message, 16)
	client.OnMessage(func(m mqtt.Message) {
		select {
		case incoming <- m:
		default:
		}
	})
	h, _ := newAppWithDeps(t, app.Deps{
		DB: db, Publisher: writer, Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(), Snapshots: snapshotter,
	})
	// Stopped in reverse order: the snapshotter and the deliverer first,
	// then the client.
	startJob(t, func(ctx context.Context) {
		if err := client.Run(ctx); err != nil {
			t.Errorf("mqtt Run: %v", err)
		}
	})
	startJob(t, func(ctx context.Context) { deliverer.Run(ctx, outbox.MinBackoff, outbox.MaxBackoff) })
	startJob(t, func(ctx context.Context) { snapshotter.Run(ctx, window) })

	select {
	case m := <-incoming:
		if m.Topic != "vorrat/in/snapshot" || !m.Retained {
			t.Fatalf("message = %s, retained %v, want the retained one on vorrat/in/snapshot", m.Topic, m.Retained)
		}
	case <-time.After(mqttWait):
		t.Fatal("timeout waiting for the retained message")
	}
	checkNoMessage(t, msgs, 5*window)

	var data map[string]any
	if err := json.Unmarshal([]byte(`{"list": "todo.bring_zuhause", "items": [
		{"product_id": "p2", "name": "kidneybohnen", "on_list": true, "missing": 3, "quantity": 3, "unit": "piece"},
		{"product_id": "p3", "name": "Kidneybohnen", "on_list": true, "missing": 4, "quantity": 4, "unit": "piece"},
		{"product_id": "p4", "name": "Mehl", "on_list": false, "missing": 0, "quantity": 0, "unit": "piece"},
		{"product_id": "p1", "name": "Zucker", "on_list": true, "missing": 0, "quantity": 0, "unit": "piece"}
	]}`), &data); err != nil {
		t.Fatalf("decode want: %v", err)
	}

	// Two requests within the window result in one snapshot.
	start := time.Now()
	for range 2 {
		if rec := sendSnapshot(h); rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusAccepted, rec.Body.String())
		}
	}
	checkEventMessage(t, msgs, start, "vorrat/events/shopping.snapshot", "shopping.snapshot", data)
	checkNoMessage(t, msgs, 5*window)

	// Any payload on vorrat/in/snapshot requests one.
	start = time.Now()
	if err := srv.Publish("vorrat/in/snapshot", []byte("resend"), false, 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	checkEventMessage(t, msgs, start, "vorrat/events/shopping.snapshot", "shopping.snapshot", data)
	checkNoMessage(t, msgs, 5*window)
}

// checkNoMessage asserts that msgs receives nothing for d.
func checkNoMessage(t *testing.T, msgs <-chan published, d time.Duration) {
	t.Helper()
	select {
	case m := <-msgs:
		t.Fatalf("message on %s: %s, want none", m.topic, m.payload)
	case <-time.After(d):
	}
}

// checkEventMessage receives the next message from msgs and checks that it
// has topic and is the JSON of an event of type typ with data, created
// after start.
func checkEventMessage(t *testing.T, msgs <-chan published, start time.Time, topic, typ string, data map[string]any) {
	t.Helper()
	var m published
	select {
	case m = <-msgs:
	case <-time.After(mqttWait):
		t.Fatalf("timeout waiting for %s", topic)
	}
	if m.topic != topic {
		t.Fatalf("topic = %q, want %q", m.topic, topic)
	}
	var e map[string]any
	if err := json.Unmarshal([]byte(m.payload), &e); err != nil {
		t.Fatalf("decode payload %s: %v", m.payload, err)
	}
	if keys, want := slices.Sorted(maps.Keys(e)), []string{"data", "id", "source", "time", "type"}; !slices.Equal(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
	id, _ := e["id"].(string)
	if u, err := uuid.Parse(id); err != nil || u.Version() != 7 || id != strings.ToLower(id) {
		t.Errorf("id = %v, want UUIDv7 in lower case", e["id"])
	}
	if e["type"] != typ || e["source"] != "stashbert" {
		t.Errorf("type, source = %v, %v, want %s, stashbert", e["type"], e["source"], typ)
	}
	s, _ := e["time"].(string)
	if tm, err := time.Parse(time.RFC3339Nano, s); err != nil || tm.Before(start) || tm.After(time.Now()) || !strings.HasSuffix(s, "Z") {
		t.Errorf("time = %v, want the time of the booking in UTC", e["time"])
	}
	if !reflect.DeepEqual(e["data"], data) {
		t.Errorf("data = %v\n     want %v", e["data"], data)
	}
}

// TestSummaryOverMQTT publishes the summary with the outbox and the summary
// publisher wired as in main: retained after every connection and once
// after several events in quick succession, each time the JSON of
// GET /summary (architecture.md, 11.5).
func TestSummaryOverMQTT(t *testing.T) {
	const delay = 200 * time.Millisecond
	srv, url := startBroker(t)
	msgs := make(chan published, 16)
	if err := srv.Subscribe("vorrat/state/#", 1, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
		msgs <- published{pk.TopicName, string(pk.Payload)}
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	db := newDB(t)
	insertSummaryProducts(t, db, summaryTestProducts...)
	logger := slog.New(slog.DiscardHandler)
	client, err := mqtt.New(config.MQTT{URL: url, ClientID: "stashbert", TopicPrefix: "vorrat", HAPrefix: "homeassistant"}, logger)
	if err != nil {
		t.Fatalf("mqtt.New: %v", err)
	}
	deliverer := outbox.NewDeliverer(db, client, "vorrat", logger)
	summary := outbox.NewSummaryPublisher(db, client, "vorrat", logger)
	client.OnConnect(func(context.Context) { deliverer.Wake() })
	client.OnConnect(summary.Publish)
	h, _ := newAppWithDeps(t, app.Deps{
		DB: db, Publisher: outbox.NewWriter(db, deliverer.Wake, summary.Request, logger),
		Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(),
	})
	// Stopped in reverse order: the summary publisher and the deliverer
	// first, then the client.
	startJob(t, func(ctx context.Context) {
		if err := client.Run(ctx); err != nil {
			t.Errorf("mqtt Run: %v", err)
		}
	})
	startJob(t, func(ctx context.Context) { deliverer.Run(ctx, outbox.MinBackoff, outbox.MaxBackoff) })
	startJob(t, func(ctx context.Context) { summary.Run(ctx, delay, time.Hour) })

	// After the connection.
	checkSummaryMessage(t, srv, h, msgs, summaryTestJSON)

	// Several events in quick succession result in one publish.
	var last time.Time
	for _, body := range []string{
		// Kidneybohnen 2 → 5: nothing missing any more.
		`{"product_id": "s3", "kind": "add", "quantity": 3}`,
		// Apfelsaft 0 → 2: not empty, and the marking ends.
		`{"product_id": "s6", "kind": "add", "quantity": 2}`,
		// Mehl 2 → 0: empty and below the minimum stock.
		`{"product_id": "s7", "kind": "consume", "quantity": 2}`,
	} {
		last = time.Now()
		if rec := post(h, "/api/v1/movements", body); rec.Code != http.StatusCreated {
			t.Fatalf("book %s: status %d, body %s", body, rec.Code, rec.Body.String())
		}
	}
	checkSummaryMessage(t, srv, h, msgs, `{
		"product_count": 9, "shopping_count": 5, "empty_count": 3, "review_count": 2,
		"shopping": [
			{"name": "Cola", "missing": 0, "quantity": 0, "unit": "crate"},
			{"name": "Jever", "missing": 17, "quantity": 1, "unit": "crate"},
			{"name": "kidneybohnen", "missing": 4, "quantity": 4, "unit": "piece"},
			{"name": "Mehl", "missing": 4, "quantity": 4, "unit": "piece"},
			{"name": "Milch", "missing": 2, "quantity": 2, "unit": "piece"}
		],
		"shopping_truncated": false
	}`)
	if d := time.Since(last); d < delay {
		t.Errorf("summary published %v after the last booking began, want at least %v", d, delay)
	}
	checkNoMessage(t, msgs, 3*delay)

	// A change without an event is published with the next connection.
	insertSummaryProducts(t, db, summaryProduct{id: "s10", name: "Zwieback", target: 2})
	checkNoMessage(t, msgs, 3*delay)
	cl, ok := srv.Clients.Get("stashbert")
	if !ok {
		t.Fatal("client stashbert not connected to the broker")
	}
	// DisconnectClient returns the reason code as error.
	_ = srv.DisconnectClient(cl, packets.ErrServerShuttingDown)
	checkSummaryMessage(t, srv, h, msgs, `{
		"product_count": 10, "shopping_count": 6, "empty_count": 4, "review_count": 2,
		"shopping": [
			{"name": "Cola", "missing": 0, "quantity": 0, "unit": "crate"},
			{"name": "Jever", "missing": 17, "quantity": 1, "unit": "crate"},
			{"name": "kidneybohnen", "missing": 4, "quantity": 4, "unit": "piece"},
			{"name": "Mehl", "missing": 4, "quantity": 4, "unit": "piece"},
			{"name": "Milch", "missing": 2, "quantity": 2, "unit": "piece"},
			{"name": "Zwieback", "missing": 2, "quantity": 2, "unit": "piece"}
		],
		"shopping_truncated": false
	}`)
	checkNoMessage(t, msgs, 3*delay)
}

// checkSummaryMessage receives the next message from msgs and checks that
// it is the JSON want on vorrat/state/summary, the same as the response of
// GET /summary of h, and retained on srv.
func checkSummaryMessage(t *testing.T, srv *mochi.Server, h http.Handler, msgs <-chan published, want string) {
	t.Helper()
	var m published
	select {
	case m = <-msgs:
	case <-time.After(mqttWait):
		t.Fatal("timeout waiting for the summary")
	}
	if m.topic != "vorrat/state/summary" {
		t.Fatalf("topic = %q, want vorrat/state/summary", m.topic)
	}
	checkJSON(t, m.payload, want)
	checkJSON(t, m.payload, getSummary(t, h))
	stored := srv.Topics.Messages("vorrat/state/summary")
	if len(stored) != 1 || !stored[0].FixedHeader.Retain || string(stored[0].Payload) != m.payload {
		t.Errorf("retained messages on vorrat/state/summary = %v, want exactly the last one", stored)
	}
}

// subscribeAll subscribes the broker's inline client to the filters and
// returns all messages it sees in one channel, including retained ones.
func subscribeAll(t *testing.T, srv *mochi.Server, filters ...string) <-chan published {
	t.Helper()
	msgs := make(chan published, 32)
	for i, filter := range filters {
		if err := srv.Subscribe(filter, i+1, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
			msgs <- published{pk.TopicName, string(pk.Payload)}
		}); err != nil {
			t.Fatalf("subscribe %s: %v", filter, err)
		}
	}
	return msgs
}

// checkMessage receives the next message from msgs and checks that it is
// payload on topic.
func checkMessage(t *testing.T, msgs <-chan published, topic, payload string) {
	t.Helper()
	select {
	case m := <-msgs:
		if m.topic != topic || m.payload != payload {
			t.Fatalf("message on %s: %q\nwant on %s: %q", m.topic, m.payload, topic, payload)
		}
	case <-time.After(mqttWait):
		t.Fatalf("timeout waiting for %s", topic)
	}
}

// startDiscovery wires the client, the summary publisher and the discovery
// for cfg as in main, and runs the client and the discovery with delay
// until the test ends.
func startDiscovery(t *testing.T, cfg config.MQTT, delay time.Duration) {
	t.Helper()
	db := newDB(t)
	logger := slog.New(slog.DiscardHandler)
	client, err := mqtt.New(cfg, logger)
	if err != nil {
		t.Fatalf("mqtt.New: %v", err)
	}
	summary := outbox.NewSummaryPublisher(db, client, cfg.TopicPrefix, logger)
	discovery, err := outbox.NewDiscovery(client, cfg, "dev", summary.Publish, logger)
	if err != nil {
		t.Fatalf("NewDiscovery: %v", err)
	}
	client.OnConnect(discovery.Publish)
	client.OnConnect(summary.Publish)
	client.OnMessage(discovery.Receive)
	// Stopped in reverse order: the discovery first, then the client.
	startJob(t, func(ctx context.Context) {
		if err := client.Run(ctx); err != nil {
			t.Errorf("mqtt Run: %v", err)
		}
	})
	startJob(t, func(ctx context.Context) { discovery.Run(ctx, delay, delay) })
}

// emptySummaryJSON is the summary on the broker without products.
const emptySummaryJSON = `{"product_count":0,"shopping_count":0,"empty_count":0,"review_count":0,"shopping":[],"shopping_truncated":false}`

// TestDiscoveryOverMQTT announces StashBert to Home Assistant with the
// discovery wired as in main: retained after the connection, and after the
// birth message online again together with the status and the summary
// (architecture.md, 11.6).
func TestDiscoveryOverMQTT(t *testing.T) {
	const delay = 200 * time.Millisecond
	srv, url := startBroker(t)
	msgs := subscribeAll(t, srv, "homeassistant/device/#", "vorrat/status", "vorrat/state/#")
	payload, err := outbox.DiscoveryPayload("vorrat", "dev")
	if err != nil {
		t.Fatalf("DiscoveryPayload: %v", err)
	}
	startDiscovery(t, config.MQTT{
		URL: url, ClientID: "stashbert", TopicPrefix: "vorrat", HADiscovery: true, HAPrefix: "homeassistant",
	}, delay)

	// After the connection.
	checkMessage(t, msgs, "vorrat/status", "online")
	checkMessage(t, msgs, "homeassistant/device/vorrat/config", string(payload))
	checkMessage(t, msgs, "vorrat/state/summary", emptySummaryJSON)
	stored := srv.Topics.Messages("homeassistant/device/vorrat/config")
	if len(stored) != 1 || !stored[0].FixedHeader.Retain || string(stored[0].Payload) != string(payload) {
		t.Errorf("retained messages on homeassistant/device/vorrat/config = %v, want the discovery", stored)
	}
	checkNoMessage(t, msgs, 3*delay)

	// Home Assistant goes offline: no answer.
	if err := srv.Publish("homeassistant/status", []byte("offline"), false, 1); err != nil {
		t.Fatalf("publish offline: %v", err)
	}
	checkNoMessage(t, msgs, 3*delay)

	// Home Assistant is back: one answer after the delay for several birth
	// messages.
	start := time.Now()
	for range 3 {
		if err := srv.Publish("homeassistant/status", []byte("online"), false, 1); err != nil {
			t.Fatalf("publish online: %v", err)
		}
	}
	checkMessage(t, msgs, "homeassistant/device/vorrat/config", string(payload))
	if elapsed := time.Since(start); elapsed < delay {
		t.Errorf("discovery published %v after the birth message, want at least %v", elapsed, delay)
	}
	checkMessage(t, msgs, "vorrat/status", "online")
	checkMessage(t, msgs, "vorrat/state/summary", emptySummaryJSON)
	checkNoMessage(t, msgs, 3*delay)
}

// TestDiscoveryDisabledOverMQTT removes the announcement with
// MQTT_HA_DISCOVERY=false: an empty retained payload after the connection,
// and no answer to the birth message (architecture.md, 11.6).
func TestDiscoveryDisabledOverMQTT(t *testing.T) {
	const delay = 100 * time.Millisecond
	srv, url := startBroker(t)
	// From an earlier start with discovery.
	if err := srv.Publish("homeassistant/device/vorrat/config", []byte(`{"dev": {}}`), true, 1); err != nil {
		t.Fatalf("publish retained: %v", err)
	}
	msgs := subscribeAll(t, srv, "homeassistant/device/#", "vorrat/state/#")
	checkMessage(t, msgs, "homeassistant/device/vorrat/config", `{"dev": {}}`)
	startDiscovery(t, config.MQTT{
		URL: url, ClientID: "stashbert", TopicPrefix: "vorrat", HADiscovery: false, HAPrefix: "homeassistant",
	}, delay)

	checkMessage(t, msgs, "homeassistant/device/vorrat/config", "")
	checkMessage(t, msgs, "vorrat/state/summary", emptySummaryJSON)
	if stored := srv.Topics.Messages("homeassistant/device/vorrat/config"); len(stored) != 0 {
		t.Errorf("retained messages on homeassistant/device/vorrat/config = %v, want none", stored)
	}

	if err := srv.Publish("homeassistant/status", []byte("online"), false, 1); err != nil {
		t.Fatalf("publish online: %v", err)
	}
	checkNoMessage(t, msgs, 5*delay)
}

// TestSummaryAfterChangeOverMQTT publishes the summary after a successful
// changing API request without an event, but not after reading or failed
// requests (architecture.md, 11.5).
func TestSummaryAfterChangeOverMQTT(t *testing.T) {
	const delay = 200 * time.Millisecond
	srv, url := startBroker(t)
	msgs := subscribeAll(t, srv, "vorrat/state/#")

	db := newDB(t)
	insertSummaryProducts(t, db, summaryTestProducts...)
	logger := slog.New(slog.DiscardHandler)
	client, err := mqtt.New(config.MQTT{URL: url, ClientID: "stashbert", TopicPrefix: "vorrat", HAPrefix: "homeassistant"}, logger)
	if err != nil {
		t.Fatalf("mqtt.New: %v", err)
	}
	summary := outbox.NewSummaryPublisher(db, client, "vorrat", logger)
	client.OnConnect(summary.Publish)
	// Without events, only the handler can request the summary.
	h, _ := newAppWithDeps(t, app.Deps{
		DB: db, Publisher: events.Nop{}, Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(),
		OnChange: summary.Request,
	})
	// Stopped in reverse order: the summary publisher first, then the client.
	startJob(t, func(ctx context.Context) {
		if err := client.Run(ctx); err != nil {
			t.Errorf("mqtt Run: %v", err)
		}
	})
	startJob(t, func(ctx context.Context) { summary.Run(ctx, delay, time.Hour) })

	// After the connection.
	checkSummaryMessage(t, srv, h, msgs, summaryTestJSON)

	// Reading and failed requests.
	for _, tt := range []struct {
		rec    *httptest.ResponseRecorder
		status int
	}{
		{get(h, "/api/v1/products"), http.StatusOK},
		{get(h, "/api/v1/summary"), http.StatusOK},
		{patchProduct(h, "unknown", `{"name": "Vollmilch"}`), http.StatusNotFound},
		{patchProduct(h, "s5", `{"name": " "}`), http.StatusBadRequest},
	} {
		if tt.rec.Code != tt.status {
			t.Fatalf("status = %d, want %d, body %s", tt.rec.Code, tt.status, tt.rec.Body.String())
		}
	}
	checkNoMessage(t, msgs, 3*delay)

	// Renaming Milch: no event, but a new summary.
	start := time.Now()
	decodeProduct(t, patchProduct(h, "s5", `{"name": "Vollmilch"}`))
	checkSummaryMessage(t, srv, h, msgs, strings.Replace(summaryTestJSON, `"Milch"`, `"Vollmilch"`, 1))
	if elapsed := time.Since(start); elapsed < delay {
		t.Errorf("summary published %v after the request, want at least %v", elapsed, delay)
	}
	checkNoMessage(t, msgs, 3*delay)
}
