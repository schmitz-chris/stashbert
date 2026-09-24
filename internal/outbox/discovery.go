package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
)

// The random delay after the birth message of Home Assistant before
// StashBert publishes the discovery again (architecture.md, 11.6).
const (
	MinBirthDelay = time.Second
	MaxBirthDelay = 5 * time.Second
)

const (
	// birthOnline is the birth message of Home Assistant on <ha>/status.
	birthOnline = "online"
	// statusOnline is the payload of <prefix>/status while connected.
	statusOnline = "online"
)

// discoveryPayload is the device discovery of architecture.md 11.6, with
// the abbreviated keys of Home Assistant in the same order. It has no
// object_id, which Home Assistant no longer knows.
type discoveryPayload struct {
	Device            discoveryDevice     `json:"dev"`
	Origin            discoveryOrigin     `json:"o"`
	AvailabilityTopic string              `json:"avty_t"`
	QoS               int                 `json:"qos"`
	Components        discoveryComponents `json:"cmps"`
}

type discoveryDevice struct {
	Identifiers  []string `json:"ids"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"mf"`
	Model        string   `json:"mdl"`
	SWVersion    string   `json:"sw"`
}

type discoveryOrigin struct {
	Name       string `json:"name"`
	SWVersion  string `json:"sw"`
	SupportURL string `json:"url"`
}

type discoveryComponents struct {
	Shopping discoveryComponent `json:"shopping"`
	Empty    discoveryComponent `json:"empty"`
	Review   discoveryComponent `json:"review"`
	Products discoveryComponent `json:"products"`
	Stock    discoveryComponent `json:"stock"`
	Resend   discoveryComponent `json:"resend"`
}

// discoveryComponent is one entity; the fields a platform does not use stay
// empty and are left out.
type discoveryComponent struct {
	Platform               string   `json:"p"`
	UniqueID               string   `json:"uniq_id"`
	DefaultEntityID        string   `json:"def_ent_id"`
	Name                   string   `json:"name"`
	Icon                   string   `json:"ic"`
	EntityCategory         string   `json:"ent_cat,omitempty"`
	StateTopic             string   `json:"stat_t,omitempty"`
	CommandTopic           string   `json:"cmd_t,omitempty"`
	PayloadPress           string   `json:"pl_prs,omitempty"`
	EventTypes             []string `json:"evt_typ,omitempty"`
	ValueTemplate          string   `json:"val_tpl,omitempty"`
	StateClass             string   `json:"stat_cla,omitempty"`
	JSONAttributesTopic    string   `json:"json_attr_t,omitempty"`
	JSONAttributesTemplate string   `json:"json_attr_tpl,omitempty"`
}

// countSensor returns the sensor <prefix>_<id> with the count key of the
// summary as its state.
func countSensor(prefix, id, name, icon, key string) discoveryComponent {
	return discoveryComponent{
		Platform: "sensor", UniqueID: prefix + "_" + id, DefaultEntityID: "sensor." + prefix + "_" + id,
		Name: name, Icon: icon, StateTopic: prefix + "/state/summary",
		ValueTemplate: "{{ value_json." + key + " }}", StateClass: "measurement",
	}
}

// DiscoveryPayload returns the discovery of architecture.md 11.6 for the
// topic prefix MQTT_TOPIC_PREFIX and the version of StashBert.
func DiscoveryPayload(prefix, version string) ([]byte, error) {
	shopping := countSensor(prefix, "shopping", "Shopping list", "mdi:cart", "shopping_count")
	shopping.JSONAttributesTopic = prefix + "/state/summary"
	shopping.JSONAttributesTemplate = "{{ {'items': value_json.shopping, 'truncated': value_json.shopping_truncated} | tojson }}"
	p := discoveryPayload{
		Device: discoveryDevice{
			Identifiers: []string{prefix}, Name: "StashBert", Manufacturer: "StashBert",
			Model: "Pantry inventory", SWVersion: version,
		},
		Origin:            discoveryOrigin{Name: "StashBert", SWVersion: version, SupportURL: "https://github.com/schmitz-chris/stashbert"},
		AvailabilityTopic: prefix + "/status",
		QoS:               1,
		Components: discoveryComponents{
			Shopping: shopping,
			Empty:    countSensor(prefix, "empty", "Empty products", "mdi:package-variant", "empty_count"),
			Review:   countSensor(prefix, "review", "Products to review", "mdi:clipboard-alert-outline", "review_count"),
			Products: countSensor(prefix, "products", "Products", "mdi:archive", "product_count"),
			Stock: discoveryComponent{
				Platform: "event", UniqueID: prefix + "_stock", DefaultEntityID: "event." + prefix + "_stock",
				Name: "Stock change", Icon: "mdi:barcode-scan", StateTopic: prefix + "/events/+",
				EventTypes: []string{"stock.added", "stock.consumed", "stock.adjusted"},
				ValueTemplate: "{% if value_json.type in ['stock.added', 'stock.consumed', 'stock.adjusted'] %}" +
					"{{ {'event_type': value_json.type, 'product_id': value_json.data.product_id, " +
					"'delta': value_json.data.delta, 'stock_after': value_json.data.stock_after} | tojson }}{% endif %}",
			},
			Resend: discoveryComponent{
				Platform: "button", UniqueID: prefix + "_resend", DefaultEntityID: "button." + prefix + "_resend",
				Name: "Resend shopping list", Icon: "mdi:send", EntityCategory: "config",
				CommandTopic: prefix + "/in/snapshot", PayloadPress: "{}",
			},
		},
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode discovery: %w", err)
	}
	return payload, nil
}

// Discovery announces StashBert to Home Assistant on <ha>/device/<prefix>/config
// and answers the birth message of Home Assistant (architecture.md, 11.6).
// Without discovery (MQTT_HA_DISCOVERY=false) it removes the announcement
// instead.
type Discovery struct {
	client Client
	topic  string
	// payload is the discovery, or empty without discovery, which makes
	// Home Assistant remove the device.
	payload []byte
	// birthTopic is <ha>/status, or "" without discovery.
	birthTopic  string
	statusTopic string
	summary     func(context.Context)
	logger      *slog.Logger
	births      chan struct{}
}

// NewDiscovery returns a Discovery for cfg and the version of StashBert
// that publishes with client. summary publishes the summary; pass
// SummaryPublisher.Publish.
func NewDiscovery(client Client, cfg config.MQTT, version string, summary func(context.Context), logger *slog.Logger) (*Discovery, error) {
	d := &Discovery{
		client:      client,
		topic:       cfg.HAPrefix + "/device/" + cfg.TopicPrefix + "/config",
		statusTopic: cfg.TopicPrefix + "/status",
		summary:     summary,
		logger:      logger,
		births:      make(chan struct{}, 1),
	}
	if cfg.HADiscovery {
		payload, err := DiscoveryPayload(cfg.TopicPrefix, version)
		if err != nil {
			return nil, err
		}
		d.payload = payload
		d.birthTopic = cfg.HAPrefix + "/status"
	}
	return d, nil
}

// Publish publishes the discovery retained with QoS 1, or without discovery
// an empty retained payload, and waits for the PUBACK. An error is logged
// with level warn unless ctx has ended. Register it with
// mqtt.Client.OnConnect, so that every connection publishes it.
func (d *Discovery) Publish(ctx context.Context) {
	if err := d.client.Publish(ctx, d.topic, d.payload, true); err != nil && ctx.Err() == nil {
		d.logger.LogAttrs(ctx, slog.LevelWarn, "publish discovery", slog.String("error", err.Error()))
	}
}

// Receive asks Run for an answer to the birth message: the payload online
// on <ha>/status, with or without the retain flag. It ignores other
// payloads and topics, and every message without discovery. Register it
// with mqtt.Client.OnMessage.
func (d *Discovery) Receive(m mqtt.Message) {
	if d.birthTopic == "" || m.Topic != d.birthTopic || string(m.Payload) != birthOnline {
		return
	}
	select {
	case d.births <- struct{}{}:
	default:
	}
}

// Run answers the birth messages until ctx ends. After a birth message it
// waits a random delay between minDelay and maxDelay; further birth
// messages within it get the same answer. Then, while connected, it
// publishes the discovery, <prefix>/status = online retained with QoS 1,
// and the summary. Without a connection it skips the answer; the next
// connection publishes all of it anyway. A birth message during the answer
// starts the next delay. Run blocks until ctx ends.
func (d *Discovery) Run(ctx context.Context, minDelay, maxDelay time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.births:
		}
		if !sleep(ctx, randomDelay(minDelay, maxDelay)) {
			return
		}
		// The birth messages since the first one get this answer.
		select {
		case <-d.births:
		default:
		}
		if d.client.State() != mqtt.StateConnected {
			continue
		}
		d.Publish(ctx)
		if err := d.client.Publish(ctx, d.statusTopic, []byte(statusOnline), true); err != nil && ctx.Err() == nil {
			d.logger.LogAttrs(ctx, slog.LevelWarn, "publish status", slog.String("error", err.Error()))
		}
		d.summary(ctx)
	}
}

// randomDelay returns a random duration between minDelay and maxDelay, or
// minDelay if maxDelay is not greater.
func randomDelay(minDelay, maxDelay time.Duration) time.Duration {
	if maxDelay <= minDelay {
		return minDelay
	}
	return minDelay + rand.N(maxDelay-minDelay+1)
}
