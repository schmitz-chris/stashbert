package outbox_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/outbox"
)

// discoveryJSON is the payload of architecture.md 11.6, with <p> for the
// topic prefix and <v> for the version.
const discoveryJSON = `{
  "dev": {"ids": ["<p>"], "name": "StashBert", "mf": "StashBert", "mdl": "Pantry inventory", "sw": "<v>"},
  "o": {"name": "StashBert", "sw": "<v>", "url": "https://github.com/schmitz-chris/stashbert"},
  "avty_t": "<p>/status",
  "qos": 1,
  "cmps": {
    "shopping": {"p": "sensor", "uniq_id": "<p>_shopping", "def_ent_id": "sensor.<p>_shopping", "name": "Shopping list", "ic": "mdi:cart", "stat_t": "<p>/state/summary", "val_tpl": "{{ value_json.shopping_count }}", "stat_cla": "measurement", "json_attr_t": "<p>/state/summary", "json_attr_tpl": "{{ {'items': value_json.shopping, 'truncated': value_json.shopping_truncated} | tojson }}"},
    "empty": {"p": "sensor", "uniq_id": "<p>_empty", "def_ent_id": "sensor.<p>_empty", "name": "Empty products", "ic": "mdi:package-variant", "stat_t": "<p>/state/summary", "val_tpl": "{{ value_json.empty_count }}", "stat_cla": "measurement"},
    "review": {"p": "sensor", "uniq_id": "<p>_review", "def_ent_id": "sensor.<p>_review", "name": "Products to review", "ic": "mdi:clipboard-alert-outline", "stat_t": "<p>/state/summary", "val_tpl": "{{ value_json.review_count }}", "stat_cla": "measurement"},
    "products": {"p": "sensor", "uniq_id": "<p>_products", "def_ent_id": "sensor.<p>_products", "name": "Products", "ic": "mdi:archive", "stat_t": "<p>/state/summary", "val_tpl": "{{ value_json.product_count }}", "stat_cla": "measurement"},
    "stock": {"p": "event", "uniq_id": "<p>_stock", "def_ent_id": "event.<p>_stock", "name": "Stock change", "ic": "mdi:barcode-scan", "stat_t": "<p>/events/+", "evt_typ": ["stock.added", "stock.consumed", "stock.adjusted"], "val_tpl": "{% if value_json.type in ['stock.added', 'stock.consumed', 'stock.adjusted'] %}{{ {'event_type': value_json.type, 'product_id': value_json.data.product_id, 'delta': value_json.data.delta, 'stock_after': value_json.data.stock_after} | tojson }}{% endif %}"},
    "resend": {"p": "button", "uniq_id": "<p>_resend", "def_ent_id": "button.<p>_resend", "name": "Resend shopping list", "ic": "mdi:send", "ent_cat": "config", "cmd_t": "<p>/in/snapshot", "pl_prs": "{}"}
  }
}`

// wantDiscovery returns discoveryJSON for prefix and version, decoded.
func wantDiscovery(t *testing.T, prefix, version string) any {
	t.Helper()
	s := strings.NewReplacer("<p>", prefix, "<v>", version).Replace(discoveryJSON)
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode want: %v", err)
	}
	return v
}

// findKey reports the path of the first key in v named one of names, or ""
// if there is none.
func findKey(v any, path string, names ...string) string {
	switch v := v.(type) {
	case map[string]any:
		for k, e := range v {
			for _, n := range names {
				if k == n {
					return path + "/" + k
				}
			}
			if p := findKey(e, path+"/"+k, names...); p != "" {
				return p
			}
		}
	case []any:
		for _, e := range v {
			if p := findKey(e, path, names...); p != "" {
				return p
			}
		}
	}
	return ""
}

// checkDiscoveryPayload checks that payload is valid JSON, equal to 11.6 for
// prefix and version, without object_id.
func checkDiscoveryPayload(t *testing.T, payload, prefix, version string) {
	t.Helper()
	var got any
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("payload is not valid JSON: %v\n%s", err, payload)
	}
	if want := wantDiscovery(t, prefix, version); !reflect.DeepEqual(got, want) {
		t.Errorf("payload:\n got %s\nwant %v", payload, want)
	}
	if p := findKey(got, "", "object_id", "obj_id"); p != "" {
		t.Errorf("payload has the key %s", p)
	}
}

func TestDiscoveryPayload(t *testing.T) {
	tests := []struct{ name, prefix, version string }{
		{"default prefix", "stashbert", "dev"},
		{"own prefix", "vorrat_2", "v0.3.0-4-gabc1234-dirty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := outbox.DiscoveryPayload(tt.prefix, tt.version)
			if err != nil {
				t.Fatalf("DiscoveryPayload: %v", err)
			}
			checkDiscoveryPayload(t, string(payload), tt.prefix, tt.version)
		})
	}
}

// discoveryConfig returns the MQTT configuration for the discovery tests.
func discoveryConfig(enabled bool) config.MQTT {
	return config.MQTT{TopicPrefix: "vorrat", HADiscovery: enabled, HAPrefix: "ha-test"}
}

// newDiscovery returns a Discovery for cfg with version dev and client whose
// summary publisher publishes with client too.
func newDiscovery(t *testing.T, client *fakeClient, cfg config.MQTT) (*outbox.Discovery, *logBuffer) {
	t.Helper()
	logger, logs := newLogger()
	summary := outbox.NewSummaryPublisher(openDB(t), client, "vorrat", logger)
	d, err := outbox.NewDiscovery(client, cfg, "dev", summary.Publish, logger)
	if err != nil {
		t.Fatalf("NewDiscovery: %v", err)
	}
	return d, logs
}

// runDiscovery starts d.Run with the delays and ends it when the test ends.
func runDiscovery(t *testing.T, d *outbox.Discovery, minDelay, maxDelay time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.Run(ctx, minDelay, maxDelay)
	}()
	t.Cleanup(func() {
		cancel()
		receive(t, done, "Run to return")
	})
}

// checkDiscoveryCall receives the next call of Publish of client and checks
// that it publishes the discovery retained on ha-test/device/vorrat/config.
func checkDiscoveryCall(t *testing.T, client *fakeClient) {
	t.Helper()
	m := receive(t, client.calls, "the discovery")
	if m.topic != "ha-test/device/vorrat/config" || !m.retain {
		t.Fatalf("topic, retain = %q, %v, want ha-test/device/vorrat/config, true", m.topic, m.retain)
	}
	checkDiscoveryPayload(t, m.payload, "vorrat", "dev")
}

// checkStatusCall receives the next call of Publish of client and checks
// that it publishes online retained on vorrat/status.
func checkStatusCall(t *testing.T, client *fakeClient) {
	t.Helper()
	m := receive(t, client.calls, "the status")
	if m.topic != "vorrat/status" || m.payload != "online" || !m.retain {
		t.Errorf("Publish(%q, %q, retain %v), want online retained on vorrat/status", m.topic, m.payload, m.retain)
	}
}

func TestDiscoveryPublish(t *testing.T) {
	client := newFakeClient(mqtt.StateConnected)
	d, logs := newDiscovery(t, client, discoveryConfig(true))

	d.Publish(testContext(t))

	checkDiscoveryCall(t, client)
	checkNoCall(t, client, 0)
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want none", out)
	}
}

// With MQTT_HA_DISCOVERY=false, Publish removes the device with an empty
// retained payload.
func TestDiscoveryPublishDisabled(t *testing.T) {
	client := newFakeClient(mqtt.StateConnected)
	d, _ := newDiscovery(t, client, discoveryConfig(false))

	d.Publish(testContext(t))

	m := receive(t, client.calls, "the empty payload")
	if m.topic != "ha-test/device/vorrat/config" || m.payload != "" || !m.retain {
		t.Errorf("Publish(%q, %q, retain %v), want an empty retained payload on ha-test/device/vorrat/config", m.topic, m.payload, m.retain)
	}
	checkNoCall(t, client, 0)
}

func TestDiscoveryPublishError(t *testing.T) {
	client := newFakeClient(mqtt.StateConnected)
	client.set(mqtt.StateConnected, true)
	d, logs := newDiscovery(t, client, discoveryConfig(true))

	d.Publish(testContext(t))

	checkDiscoveryCall(t, client)
	checkWarning(t, logs, "publish discovery")
}

// The birth message online, with or without the retain flag, results in
// the discovery, the status and the summary after the delay; several within
// the delay result in one answer. Other payloads and topics are ignored.
func TestDiscoveryBirth(t *testing.T) {
	const delay = 200 * time.Millisecond
	client := newFakeClient(mqtt.StateConnected)
	d, logs := newDiscovery(t, client, discoveryConfig(true))
	runDiscovery(t, d, delay, delay)

	d.Receive(mqtt.Message{Topic: "ha-test/status", Payload: []byte("online"), Retained: true})
	start := time.Now()
	d.Receive(mqtt.Message{Topic: "ha-test/status", Payload: []byte("online")})
	d.Receive(mqtt.Message{Topic: "ha-test/status", Payload: []byte("online")})

	checkDiscoveryCall(t, client)
	if elapsed := time.Since(start); elapsed < delay {
		t.Errorf("answered %v after the birth message, want at least %v", elapsed, delay)
	}
	checkStatusCall(t, client)
	checkSummaryCall(t, client, emptySummaryPayload)
	checkNoCall(t, client, 3*delay)

	for _, m := range []mqtt.Message{
		{Topic: "ha-test/status", Payload: []byte("offline")},
		{Topic: "ha-test/status", Payload: []byte("Online")},
		{Topic: "ha-test/status", Payload: nil},
		{Topic: "homeassistant/status", Payload: []byte("online")},
		{Topic: "vorrat/in/status", Payload: []byte("online")},
	} {
		d.Receive(m)
	}
	checkNoCall(t, client, 3*delay)

	d.Receive(mqtt.Message{Topic: "ha-test/status", Payload: []byte("online")})
	checkDiscoveryCall(t, client)
	checkStatusCall(t, client)
	checkSummaryCall(t, client, emptySummaryPayload)
	checkNoCall(t, client, 3*delay)
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want none", out)
	}
}

// Without discovery the birth message is ignored.
func TestDiscoveryBirthDisabled(t *testing.T) {
	client := newFakeClient(mqtt.StateConnected)
	d, _ := newDiscovery(t, client, discoveryConfig(false))
	runDiscovery(t, d, 0, 0)

	d.Receive(mqtt.Message{Topic: "ha-test/status", Payload: []byte("online")})

	checkNoCall(t, client, 200*time.Millisecond)
}

// Without a connection the answer is skipped; the next connection publishes
// all of it.
func TestDiscoveryBirthWithoutConnection(t *testing.T) {
	client := newFakeClient(mqtt.StateConnecting)
	d, logs := newDiscovery(t, client, discoveryConfig(true))
	runDiscovery(t, d, 0, 0)

	d.Receive(mqtt.Message{Topic: "ha-test/status", Payload: []byte("online")})

	receive(t, client.states, "a look at the state")
	checkNoCall(t, client, 200*time.Millisecond)
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want none", out)
	}
}

// A failed publish of the discovery or the status is logged; the rest of the
// answer still goes out.
func TestDiscoveryBirthError(t *testing.T) {
	client := newFakeClient(mqtt.StateConnected)
	client.set(mqtt.StateConnected, true)
	d, logs := newDiscovery(t, client, discoveryConfig(true))
	runDiscovery(t, d, 0, 0)

	d.Receive(mqtt.Message{Topic: "ha-test/status", Payload: []byte("online")})

	checkDiscoveryCall(t, client)
	checkStatusCall(t, client)
	checkSummaryCall(t, client, emptySummaryPayload)
	eventually(t, "three warnings", func() bool { return strings.Count(logs.String(), "\n") == 3 })
	for _, msg := range []string{`"msg":"publish discovery"`, `"msg":"publish status"`, `"msg":"publish summary"`} {
		if !strings.Contains(logs.String(), msg) {
			t.Errorf("log does not contain %s:\n%s", msg, logs.String())
		}
	}
}
