package app_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
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
		DB: db, Publisher: outbox.NewWriter(db, deliverer.Wake, logger),
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
