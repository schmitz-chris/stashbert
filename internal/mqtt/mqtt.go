// Package mqtt connects StashBert to an MQTT 5 broker (architecture.md, 11.1).
// It wraps autopaho; no other package knows paho.
package mqtt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"github.com/schmitz-chris/stashbert/internal/config"
)

// State is the state of the connection to the broker.
type State string

// The states of the connection (architecture.md, 11.1).
const (
	StateDisabled   State = "disabled"   // without MQTT_URL
	StateConnecting State = "connecting" // no connection, the client tries again
	StateConnected  State = "connected"
)

// Payloads of <p>/status.
const (
	statusOnline  = "online"
	statusOffline = "offline"
)

// shutdownTimeout limits publishing offline and disconnecting together.
const shutdownTimeout = 5 * time.Second

var errNotConnected = errors.New("not connected to the MQTT broker")

// Message is a message received on a subscribed topic.
type Message struct {
	Topic   string
	Payload []byte
	// Retained is set if the broker delivered a stored retained message,
	// for example right after subscribing.
	Retained bool
}

// Client keeps the connection to the broker. Create it with New, register
// receivers with OnMessage and OnConnect, then call Run.
type Client struct {
	broker        *url.URL
	cfg           config.MQTT
	statusTopic   string
	subscriptions []string
	logger        *slog.Logger

	mu        sync.Mutex
	state     State
	cm        *autopaho.ConnectionManager // set by Run
	stopping  bool                        // set when Run begins to shut down
	receivers []func(Message)
	onConnect []func(context.Context)
}

// New returns a client for cfg, which config.Load has validated. The
// password in cfg is never logged.
func New(cfg config.MQTT, logger *slog.Logger) (*Client, error) {
	broker, err := url.Parse(cfg.URL)
	if err != nil || broker.Host == "" {
		// The error of url.Parse repeats the URL, which must not be logged.
		return nil, errors.New("mqtt: invalid broker URL")
	}
	subscriptions := []string{cfg.TopicPrefix + "/in/#"}
	if cfg.HADiscovery {
		subscriptions = append(subscriptions, cfg.HAPrefix+"/status")
	}
	return &Client{
		broker:        broker,
		cfg:           cfg,
		statusTopic:   cfg.TopicPrefix + "/status",
		subscriptions: subscriptions,
		logger:        logger,
		state:         StateConnecting,
	}, nil
}

// OnMessage registers f for all messages on the subscribed topics
// <p>/in/# and, with discovery, <ha>/status. f runs on the receiving
// goroutine of the client and must not block. Register before Run.
func (c *Client) OnMessage(f func(Message)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.receivers = append(c.receivers, f)
}

// OnConnect registers f to run after every connection, once <p>/status is
// online and the subscriptions are sent. All such functions run one after
// another in one goroutine; ctx ends when Run begins to shut down. Register
// before Run.
func (c *Client) OnConnect(f func(ctx context.Context)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConnect = append(c.onConnect, f)
}

// State reports the state of the connection. A nil client reports
// StateDisabled, as does a client whose Run has returned.
func (c *Client) State() State {
	if c == nil {
		return StateDisabled
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Publish sends payload to topic with QoS 1 and waits for the PUBACK. It
// returns an error without a connection, when the broker rejects the message
// (reason code 0x80 or higher, for example 0x87 when not authorized) or when
// ctx ends first.
func (c *Client) Publish(ctx context.Context, topic string, payload []byte, retain bool) error {
	c.mu.Lock()
	cm, stopping := c.cm, c.stopping
	c.mu.Unlock()
	if cm == nil || stopping {
		return fmt.Errorf("publish %s: %w", topic, errNotConnected)
	}
	return publish(ctx, cm, topic, payload, retain)
}

func publish(ctx context.Context, cm *autopaho.ConnectionManager, topic string, payload []byte, retain bool) error {
	_, err := cm.Publish(ctx, &paho.Publish{QoS: 1, Retain: retain, Topic: topic, Payload: payload})
	if errors.Is(err, autopaho.ConnectionDownError) {
		err = errNotConnected
	}
	if err != nil {
		return fmt.Errorf("publish %s: %w", topic, err)
	}
	return nil
}

// Run connects to the broker and keeps the connection until ctx ends
// (architecture.md, 11.1). The first attempt after the start or after a
// loss is immediate, every further one follows an exponential backoff from
// 1 s to 5 min. The Last Will sets <p>/status to offline. After every connection
// Run publishes <p>/status = online, subscribes and calls the OnConnect
// functions. When ctx ends, Run publishes <p>/status = offline and
// disconnects, both within 5 s, and returns. Run must be called only once.
func (c *Client) Run(ctx context.Context) error {
	c.mu.Lock()
	if c.cm != nil {
		c.mu.Unlock()
		return errors.New("mqtt: Run called twice")
	}
	up := make(chan struct{}, 1)
	// The connection outlives ctx until the offline status is published.
	cm, err := autopaho.NewConnection(context.WithoutCancel(ctx), c.clientConfig(up))
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("mqtt: start connection: %w", err)
	}
	c.cm = cm
	c.mu.Unlock()

	worker := make(chan struct{})
	go func() {
		defer close(worker)
		for {
			select {
			case <-ctx.Done():
				return
			case <-up:
				c.connected(ctx, cm)
			}
		}
	}()

	<-ctx.Done()
	c.shutdown(context.WithoutCancel(ctx), cm, worker)
	return nil
}

func (c *Client) clientConfig(up chan<- struct{}) autopaho.ClientConfig {
	return autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{c.broker},
		KeepAlive:                     30,
		CleanStartOnInitialConnection: true,
		SessionExpiryInterval:         0,
		ReconnectBackoff:              autopaho.NewExponentialBackoff(time.Second, 5*time.Minute, 2*time.Second, 2),
		ConnectUsername:               c.cfg.Username,
		ConnectPassword:               []byte(c.cfg.Password),
		WillMessage: &paho.WillMessage{
			Topic: c.statusTopic, Payload: []byte(statusOffline), QoS: 1, Retain: true,
		},
		// OnConnectionUp must not block; the worker in Run does the rest.
		OnConnectionUp: func(*autopaho.ConnectionManager, *paho.Connack) {
			c.setState(StateConnected)
			c.log(slog.LevelInfo, "mqtt connected")
			select {
			case up <- struct{}{}:
			default:
			}
		},
		OnConnectionDown: func() bool {
			c.setState(StateConnecting)
			c.log(slog.LevelInfo, "mqtt connection lost")
			return true
		},
		OnConnectError: func(err error) {
			c.log(slog.LevelWarn, "mqtt connect failed", slog.String("error", err.Error()))
		},
		ClientConfig: paho.ClientConfig{
			ClientID:          c.cfg.ClientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){c.receive},
			OnClientError: func(err error) {
				c.log(slog.LevelWarn, "mqtt connection error", slog.String("error", err.Error()))
			},
			OnServerDisconnect: func(d *paho.Disconnect) {
				c.log(slog.LevelWarn, "mqtt broker disconnected", slog.Int("reason_code", int(d.ReasonCode)))
			},
		},
	}
}

// connected runs after every connection: status online, subscriptions, then
// the OnConnect functions.
func (c *Client) connected(ctx context.Context, cm *autopaho.ConnectionManager) {
	if err := publish(ctx, cm, c.statusTopic, []byte(statusOnline), true); err != nil && ctx.Err() == nil {
		c.log(slog.LevelWarn, "mqtt publish status", slog.String("status", statusOnline), slog.String("error", err.Error()))
	}
	for _, topic := range c.subscriptions {
		if ctx.Err() != nil {
			return
		}
		sub := &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{{Topic: topic, QoS: 1}}}
		if _, err := cm.Subscribe(ctx, sub); err != nil && ctx.Err() == nil {
			c.log(slog.LevelWarn, "mqtt subscribe", slog.String("topic", topic), slog.String("error", err.Error()))
		}
	}
	c.mu.Lock()
	onConnect := c.onConnect
	c.mu.Unlock()
	for _, f := range onConnect {
		if ctx.Err() != nil {
			return
		}
		f(ctx)
	}
}

// shutdown publishes offline and disconnects within shutdownTimeout. It
// first stops Publish and waits for the worker, so that no online can
// follow the offline.
func (c *Client) shutdown(ctx context.Context, cm *autopaho.ConnectionManager, worker <-chan struct{}) {
	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()
	c.mu.Lock()
	c.stopping = true
	c.mu.Unlock()
	select {
	case <-worker:
	case <-ctx.Done():
	}
	// Without a connection, the broker has already published the Last Will.
	if err := publish(ctx, cm, c.statusTopic, []byte(statusOffline), true); err != nil && !errors.Is(err, errNotConnected) {
		c.log(slog.LevelWarn, "mqtt publish status", slog.String("status", statusOffline), slog.String("error", err.Error()))
	}
	if err := cm.Disconnect(ctx); err != nil {
		c.log(slog.LevelWarn, "mqtt disconnect", slog.String("error", err.Error()))
	}
	c.setState(StateDisabled)
	c.log(slog.LevelInfo, "mqtt disconnected")
}

func (c *Client) receive(pr paho.PublishReceived) (bool, error) {
	m := Message{Topic: pr.Packet.Topic, Payload: pr.Packet.Payload, Retained: pr.Packet.Retain}
	c.mu.Lock()
	receivers := c.receivers
	c.mu.Unlock()
	for _, f := range receivers {
		f(m)
	}
	return true, nil
}

func (c *Client) setState(s State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = s
}

// log adds the broker as host and port. The URL has no credentials
// (config.Load rejects them), and the password is never logged.
func (c *Client) log(level slog.Level, msg string, attrs ...slog.Attr) {
	attrs = append(attrs, slog.String("broker", c.broker.Host))
	c.logger.LogAttrs(context.Background(), level, msg, attrs...)
}
