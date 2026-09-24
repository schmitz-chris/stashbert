package mqtt_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
)

// wait is the longest a test waits for a signal.
const wait = 5 * time.Second

// startBroker starts an in-process broker on a random port of 127.0.0.1
// and returns it with its URL. Without a ledger, every client may connect.
func startBroker(t *testing.T, ledger *auth.Ledger) (*mochi.Server, string) {
	t.Helper()
	srv := mochi.New(&mochi.Options{InlineClient: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	var err error
	if ledger == nil {
		err = srv.AddHook(new(auth.AllowHook), nil)
	} else {
		err = srv.AddHook(new(auth.Hook), &auth.Options{Ledger: ledger})
	}
	if err != nil {
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
		// the same time (Clients.GetByListener takes the read lock twice).
		// The clients have stopped before this cleanup runs; wait until the
		// broker has removed them and only its inline client is left.
		deadline := time.Now().Add(wait)
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

// watch subscribes the broker's inline client to filter and returns the
// payloads of all messages it sees, including a retained one.
func watch(t *testing.T, srv *mochi.Server, filter string, id int) <-chan string {
	t.Helper()
	ch := make(chan string, 64)
	err := srv.Subscribe(filter, id, func(_ *mochi.Client, _ packets.Subscription, pk packets.Packet) {
		ch <- string(pk.Payload)
	})
	if err != nil {
		t.Fatalf("subscribe %s: %v", filter, err)
	}
	return ch
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

// retained returns the payload of the retained message on topic.
func retained(t *testing.T, srv *mochi.Server, topic string) string {
	t.Helper()
	msgs := srv.Topics.Messages(topic)
	if len(msgs) != 1 || !msgs[0].FixedHeader.Retain {
		t.Fatalf("retained messages on %s = %v, want exactly one", topic, msgs)
	}
	return string(msgs[0].Payload)
}

// dropClient makes the broker close the connection of the client
// stashbert, as on a broker restart.
func dropClient(t *testing.T, srv *mochi.Server) {
	t.Helper()
	cl, ok := srv.Clients.Get("stashbert")
	if !ok {
		t.Fatal("client stashbert not connected to the broker")
	}
	// DisconnectClient returns the reason code as error.
	_ = srv.DisconnectClient(cl, packets.ErrServerShuttingDown)
}

func mqttConfig(url string) config.MQTT {
	return config.MQTT{
		URL: url, ClientID: "stashbert", TopicPrefix: "stashbert", HADiscovery: true, HAPrefix: "homeassistant",
	}
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// run starts c.Run and returns a function that ends it and waits until it
// has returned.
func run(t *testing.T, c *mqtt.Client) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			if err := receive(t, done, "Run to return"); err != nil {
				t.Errorf("Run: %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

// connectSignal registers an OnConnect function that signals every
// connection once the subscriptions are sent.
func connectSignal(c *mqtt.Client) <-chan struct{} {
	ch := make(chan struct{}, 8)
	c.OnConnect(func(context.Context) { ch <- struct{}{} })
	return ch
}

func TestStatusOnlineThenOffline(t *testing.T) {
	srv, url := startBroker(t, nil)
	cfg := mqttConfig(url)
	cfg.TopicPrefix = "vorrat"
	status := watch(t, srv, "vorrat/status", 1)

	c, err := mqtt.New(cfg, discard())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	connected := connectSignal(c)
	stop := run(t, c)

	receive(t, connected, "connection")
	if got := receive(t, status, "status online"); got != "online" {
		t.Fatalf("status = %q, want online", got)
	}
	if got := retained(t, srv, "vorrat/status"); got != "online" {
		t.Errorf("retained status = %q, want online", got)
	}
	if got := c.State(); got != mqtt.StateConnected {
		t.Errorf("State() = %q, want %q", got, mqtt.StateConnected)
	}

	stop()
	if got := receive(t, status, "status offline"); got != "offline" {
		t.Fatalf("status = %q, want offline", got)
	}
	if got := retained(t, srv, "vorrat/status"); got != "offline" {
		t.Errorf("retained status after Run = %q, want offline", got)
	}
	if got := c.State(); got != mqtt.StateDisabled {
		t.Errorf("State() after Run = %q, want %q", got, mqtt.StateDisabled)
	}
	if err := c.Publish(t.Context(), "vorrat/x", []byte("x"), false); err == nil {
		t.Error("Publish after Run: got nil error, want error")
	}
}

func TestReceiveWithRetainFlag(t *testing.T) {
	srv, url := startBroker(t, nil)
	// Stored before the client subscribes: delivered with the retain flag.
	if err := srv.Publish("stashbert/in/snapshot", []byte("old"), true, 1); err != nil {
		t.Fatalf("publish retained: %v", err)
	}

	c, err := mqtt.New(mqttConfig(url), discard())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	msgs := make(chan mqtt.Message, 8)
	c.OnMessage(func(m mqtt.Message) { msgs <- m })
	connected := connectSignal(c)
	run(t, c)

	want := mqtt.Message{Topic: "stashbert/in/snapshot", Payload: []byte("old"), Retained: true}
	if got := receive(t, msgs, "retained message"); !equalMessage(got, want) {
		t.Errorf("message = %+v, want %+v", got, want)
	}

	receive(t, connected, "connection")
	if err := srv.Publish("stashbert/in/snapshot", []byte("{}"), false, 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	want = mqtt.Message{Topic: "stashbert/in/snapshot", Payload: []byte("{}"), Retained: false}
	if got := receive(t, msgs, "live message"); !equalMessage(got, want) {
		t.Errorf("message = %+v, want %+v", got, want)
	}
}

func equalMessage(a, b mqtt.Message) bool {
	return a.Topic == b.Topic && bytes.Equal(a.Payload, b.Payload) && a.Retained == b.Retained
}

func TestSubscriptions(t *testing.T) {
	tests := []struct {
		discovery bool
		want      []string
	}{
		{true, []string{"p/in/#", "ha/status"}},
		{false, []string{"p/in/#"}},
	}
	for _, tt := range tests {
		t.Run(map[bool]string{true: "discovery", false: "without discovery"}[tt.discovery], func(t *testing.T) {
			srv, url := startBroker(t, nil)
			cfg := mqttConfig(url)
			cfg.TopicPrefix, cfg.HAPrefix, cfg.HADiscovery = "p", "ha", tt.discovery
			c, err := mqtt.New(cfg, discard())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			msgs := make(chan mqtt.Message, 8)
			c.OnMessage(func(m mqtt.Message) { msgs <- m })
			connected := connectSignal(c)
			run(t, c)
			receive(t, connected, "connection")

			cl, ok := srv.Clients.Get("stashbert")
			if !ok {
				t.Fatal("client stashbert not connected to the broker")
			}
			subs := cl.State.Subscriptions.GetAll()
			if len(subs) != len(tt.want) {
				t.Errorf("subscriptions = %v, want %v", subs, tt.want)
			}
			for _, filter := range tt.want {
				if sub, ok := subs[filter]; !ok || sub.Qos != 1 {
					t.Errorf("subscription %s = %+v, %v, want QoS 1", filter, sub, ok)
				}
			}
			if tt.discovery {
				if err := srv.Publish("ha/status", []byte("online"), false, 1); err != nil {
					t.Fatalf("publish: %v", err)
				}
				if got := receive(t, msgs, "birth message"); got.Topic != "ha/status" || string(got.Payload) != "online" {
					t.Errorf("message = %+v, want online on ha/status", got)
				}
			}
		})
	}
}

// After the broker drops the connection, the Last Will sets the status to
// offline; the client reconnects, publishes online and subscribes again.
func TestReconnect(t *testing.T) {
	srv, url := startBroker(t, nil)
	status := watch(t, srv, "stashbert/status", 1)
	c, err := mqtt.New(mqttConfig(url), discard())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	msgs := make(chan mqtt.Message, 8)
	c.OnMessage(func(m mqtt.Message) { msgs <- m })
	connected := connectSignal(c)
	run(t, c)

	receive(t, connected, "connection")
	if got := receive(t, status, "status online"); got != "online" {
		t.Fatalf("status = %q, want online", got)
	}
	dropClient(t, srv)
	if got := receive(t, status, "last will"); got != "offline" {
		t.Fatalf("status = %q, want offline (last will)", got)
	}

	receive(t, connected, "reconnection")
	if got := receive(t, status, "status online again"); got != "online" {
		t.Fatalf("status = %q, want online", got)
	}
	if err := srv.Publish("stashbert/in/snapshot", []byte("{}"), false, 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := receive(t, msgs, "message after reconnection"); got.Topic != "stashbert/in/snapshot" {
		t.Errorf("message topic = %q, want stashbert/in/snapshot", got.Topic)
	}
}

func TestPublish(t *testing.T) {
	srv, url := startBroker(t, &auth.Ledger{
		Users: auth.Users{"stashbert": {Password: "pw", ACL: auth.Filters{"denied/#": auth.ReadOnly}}},
	})
	cfg := mqttConfig(url)
	cfg.Username, cfg.Password = "stashbert", "pw"
	c, err := mqtt.New(cfg, discard())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Publish(t.Context(), "stashbert/events/x", []byte("{}"), false); err == nil {
		t.Error("Publish before Run: got nil error, want error")
	}
	connected := connectSignal(c)
	run(t, c)
	receive(t, connected, "connection")

	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()
	if err := c.Publish(ctx, "stashbert/state/summary", []byte(`{"n":1}`), true); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got := retained(t, srv, "stashbert/state/summary"); got != `{"n":1}` {
		t.Errorf("retained summary = %q, want {\"n\":1}", got)
	}
	// The broker answers with PUBACK 0x87 (not authorized).
	if err := c.Publish(ctx, "denied/x", []byte("x"), false); err == nil {
		t.Error("Publish to a denied topic: got nil error, want error")
	}
}

// Without a broker, Run keeps trying, State reports connecting, Publish
// fails, and Run still ends right away.
func TestRunWithoutBroker(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	c, err := mqtt.New(mqttConfig("mqtt://"+addr), discard())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stop := run(t, c)
	if got := c.State(); got != mqtt.StateConnecting {
		t.Errorf("State() = %q, want %q", got, mqtt.StateConnecting)
	}
	if err := c.Publish(t.Context(), "stashbert/events/x", []byte("{}"), false); err == nil {
		t.Error("Publish without connection: got nil error, want error")
	}
	start := time.Now()
	stop()
	if d := time.Since(start); d > time.Second {
		t.Errorf("Run took %v to return, want less than 1 s", d)
	}
}

func TestStateDisabled(t *testing.T) {
	var c *mqtt.Client
	if got := c.State(); got != mqtt.StateDisabled {
		t.Errorf("State() of nil client = %q, want %q", got, mqtt.StateDisabled)
	}
}

// syncBuffer collects log output and signals every write.
type syncBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	wrote chan struct{}
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case b.wrote <- struct{}{}:
	default:
	}
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitFor waits until the log contains s.
func (b *syncBuffer) waitFor(t *testing.T, s string) {
	t.Helper()
	deadline := time.After(wait)
	for !strings.Contains(b.String(), s) {
		select {
		case <-b.wrote:
		case <-deadline:
			t.Fatalf("log does not contain %q:\n%s", s, b.String())
		}
	}
}

func TestPasswordNotLogged(t *testing.T) {
	const password = "hunter2-geheim"
	srv, url := startBroker(t, &auth.Ledger{
		Users: auth.Users{"stashbert": {Password: password}},
	})
	logs := &syncBuffer{wrote: make(chan struct{}, 1)}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := mqttConfig(url)
	cfg.Username = "stashbert"

	// A wrong password: the broker denies the connection.
	cfg.Password = password + "-falsch"
	c, err := mqtt.New(cfg, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stop := run(t, c)
	logs.waitFor(t, "mqtt connect failed")
	stop()

	// The right password: connect, publish, disconnect.
	cfg.Password = password
	c, err = mqtt.New(cfg, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	connected := connectSignal(c)
	stop = run(t, c)
	receive(t, connected, "connection")
	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()
	if err := c.Publish(ctx, "stashbert/events/x", []byte("{}"), false); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	dropClient(t, srv)
	receive(t, connected, "reconnection")
	stop()

	out := logs.String()
	for _, want := range []string{"mqtt connect failed", "mqtt connected", "mqtt connection lost", "mqtt disconnected"} {
		if !strings.Contains(out, want) {
			t.Errorf("log does not contain %q", want)
		}
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("log contains the password:\n%s", out)
	}
}
