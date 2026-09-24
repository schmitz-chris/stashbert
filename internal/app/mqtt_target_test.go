package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/outbox"
)

// targetOffer is the offer of the tests, the first entry as Home Assistant
// publishes it on the real system.
const targetOffer = `[{"id":"todo.stashbert_test","name":"StashBert Test"},{"id":"todo.bring_zuhause","name":"Zuhause"}]`

// putTarget sends PUT /api/v1/integrations/mqtt/target with the JSON body to h.
func putTarget(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/integrations/mqtt/target", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

// checkMqttStatus asserts that rec is a 200 JSON response with the
// MqttStatus want.
func checkMqttStatus(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	checkJSON(t, rec.Body.String(), want)
}

// storeTarget stores the chosen target list id with name as settings.
func storeTarget(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(),
		"INSERT INTO settings (key, value) VALUES ('shopping_target_id', ?), ('shopping_target_name', ?)", id, name); err != nil {
		t.Fatalf("store target list: %v", err)
	}
}

// storedSettings returns all rows of the table settings.
func storedSettings(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT key, value FROM settings")
	if err != nil {
		t.Fatalf("query settings: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatalf("scan settings: %v", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read settings: %v", err)
	}
	return out
}

// checkSettings asserts that the table settings holds the choice id with
// name, or nothing if id is "".
func checkSettings(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	want := map[string]string{}
	if id != "" {
		want = map[string]string{"shopping_target_id": id, "shopping_target_name": name}
	}
	if got := storedSettings(t, db); !maps.Equal(got, want) {
		t.Errorf("settings = %v, want %v", got, want)
	}
}

// snapshotItem is the part of an item of shopping.snapshot that
// outboxSnapshots needs.
type snapshotItem struct {
	OnList bool `json:"on_list"`
}

// outboxSnapshots describes the shopping.snapshot entries in the outbox in
// their order as "clear <list>", if no item is on the list, or
// "fill <list>". Other entries are "other <topic>".
func outboxSnapshots(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT topic, payload FROM outbox ORDER BY seq")
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var topic, payload string
		if err := rows.Scan(&topic, &payload); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		if topic != "events/shopping.snapshot" {
			out = append(out, "other "+topic)
			continue
		}
		var e struct {
			Data struct {
				List  string
				Items []snapshotItem
			}
		}
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			t.Fatalf("decode payload %s: %v", payload, err)
		}
		kind := "clear "
		if slices.ContainsFunc(e.Data.Items, func(it snapshotItem) bool { return it.OnList }) {
			kind = "fill "
		}
		out = append(out, kind+e.Data.List)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	return out
}

// checkOutboxSnapshots asserts the snapshots in the outbox (outboxSnapshots).
func checkOutboxSnapshots(t *testing.T, db *sql.DB, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := outboxSnapshots(t, db); !slices.Equal(got, want) {
		t.Errorf("outbox = %q, want %q", got, want)
	}
}

// fixedClient is an outbox.Client with a fixed state that never publishes.
type fixedClient mqtt.State

func (c fixedClient) State() mqtt.State { return mqtt.State(c) }

func (fixedClient) Publish(context.Context, string, []byte, bool) error {
	return errors.New("not connected to the MQTT broker")
}

// newTargetsApp returns the handler with the outbox writer and the target
// lists wired as in main, for a client in state connecting, and its
// database with insertProducts.
func newTargetsApp(t *testing.T) (http.Handler, *sql.DB, *outbox.Targets) {
	t.Helper()
	db := newDB(t)
	insertProducts(t, db)
	logger := slog.New(slog.DiscardHandler)
	writer := outbox.NewWriter(db, func() {}, func() {}, logger)
	targets := outbox.NewTargets(writer, fixedClient(mqtt.StateConnecting), "vorrat", logger)
	h, _ := newAppWithDeps(t, app.Deps{
		DB: db, Publisher: writer, Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(), MQTT: targets,
	})
	return h, db, targets
}

// Without MQTT, GET /integrations/mqtt reports disabled, offers nothing and
// shows the stored choice (architecture.md, 11.7).
func TestGetMqttStatusWithoutMQTT(t *testing.T) {
	h, db := newApp(t)

	checkMqttStatus(t, get(h, "/api/v1/integrations/mqtt"), `{"status": "disabled", "targets": [], "target": null}`)

	storeTarget(t, db, "todo.bring_zuhause", "Zuhause")

	checkMqttStatus(t, get(h, "/api/v1/integrations/mqtt"),
		`{"status": "disabled", "targets": [], "target": {"id": "todo.bring_zuhause", "name": "Zuhause"}}`)
}

// Without MQTT, PUT /integrations/mqtt/target answers 409 mqtt_disabled and
// changes nothing.
func TestSetShoppingTargetWithoutMQTT(t *testing.T) {
	h, db := newApp(t)
	storeTarget(t, db, "todo.bring_zuhause", "Zuhause")

	for _, body := range []string{`{"id": "todo.stashbert_test"}`, `{"id": "todo.bring_zuhause"}`, `{"id": null}`} {
		checkProblemCode(t, putTarget(h, body), http.StatusConflict, "mqtt_disabled")
	}
	checkSettings(t, db, "todo.bring_zuhause", "Zuhause")
	checkOutboxSnapshots(t, db)
}

// A malformed body is 400 invalid_request and changes nothing.
func TestSetShoppingTargetInvalidBody(t *testing.T) {
	h, db, targets := newTargetsApp(t)
	targets.Receive(mqtt.Message{Topic: "vorrat/in/targets", Payload: []byte(targetOffer), Retained: true})

	for _, body := range []string{``, `todo.stashbert_test`, `[]`, `{}`, `{"list": "todo.stashbert_test"}`, `{"id": 1}`, `{"id": ["todo.stashbert_test"]}`} {
		t.Run(body, func(t *testing.T) {
			checkProblemCode(t, putTarget(h, body), http.StatusBadRequest, "invalid_request")
		})
	}
	t.Run("without JSON content type", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/integrations/mqtt/target", strings.NewReader(`{"id": "todo.stashbert_test"}`))
		req.Header.Set("Content-Type", "text/plain")
		h.ServeHTTP(rec, req)
		checkProblemCode(t, rec, http.StatusBadRequest, "invalid_request")
	})
	checkSettings(t, db, "", "")
	checkOutboxSnapshots(t, db)
}

// TestSetShoppingTarget chooses, switches and removes the target list
// through the API: only offered lists, the stored choice also when it is no
// longer offered, and the snapshots of every switch in the outbox
// (architecture.md, 11.7).
func TestSetShoppingTarget(t *testing.T) {
	h, db, targets := newTargetsApp(t)
	const (
		test       = `{"id": "todo.stashbert_test", "name": "StashBert Test"}`
		zuhause    = `{"id": "todo.bring_zuhause", "name": "Zuhause"}`
		fullOffer  = `[` + test + `, ` + zuhause + `]`
		statusWith = `{"status": "connecting", "targets": `
	)

	// Before Home Assistant offers anything, no list can be chosen.
	checkMqttStatus(t, get(h, "/api/v1/integrations/mqtt"), statusWith+`[], "target": null}`)
	checkProblemCode(t, putTarget(h, `{"id": "todo.stashbert_test"}`), http.StatusUnprocessableEntity, "unknown_target")

	targets.Receive(mqtt.Message{Topic: "vorrat/in/targets", Payload: []byte(targetOffer), Retained: true})
	checkMqttStatus(t, get(h, "/api/v1/integrations/mqtt"), statusWith+fullOffer+`, "target": null}`)

	// Unknown ids.
	for _, body := range []string{`{"id": "todo.einkauf"}`, `{"id": ""}`, `{"id": "StashBert Test"}`} {
		checkProblemCode(t, putTarget(h, body), http.StatusUnprocessableEntity, "unknown_target")
	}
	checkSettings(t, db, "", "")
	checkOutboxSnapshots(t, db)

	// The first choice fills the list.
	checkMqttStatus(t, putTarget(h, `{"id": "todo.stashbert_test"}`), statusWith+fullOffer+`, "target": `+test+`}`)
	checkSettings(t, db, "todo.stashbert_test", "StashBert Test")
	checkOutboxSnapshots(t, db, "fill todo.stashbert_test")

	// The same list again changes nothing.
	checkMqttStatus(t, putTarget(h, `{"id": "todo.stashbert_test"}`), statusWith+fullOffer+`, "target": `+test+`}`)
	checkOutboxSnapshots(t, db, "fill todo.stashbert_test")

	// A switch first clears the old list, then fills the new one.
	checkMqttStatus(t, putTarget(h, `{"id": "todo.bring_zuhause"}`), statusWith+fullOffer+`, "target": `+zuhause+`}`)
	checkSettings(t, db, "todo.bring_zuhause", "Zuhause")
	checkOutboxSnapshots(t, db, "fill todo.stashbert_test", "clear todo.stashbert_test", "fill todo.bring_zuhause")

	// The stored choice is shown although it is no longer offered.
	targets.Receive(mqtt.Message{Topic: "vorrat/in/targets", Payload: []byte(`[` + test + `]`), Retained: true})
	checkMqttStatus(t, get(h, "/api/v1/integrations/mqtt"), statusWith+`[`+test+`], "target": `+zuhause+`}`)

	// Removing the choice clears the list; removing it again changes nothing.
	for range 2 {
		checkMqttStatus(t, putTarget(h, `{"id": null}`), statusWith+`[`+test+`], "target": null}`)
		checkSettings(t, db, "", "")
		checkOutboxSnapshots(t, db, "fill todo.stashbert_test", "clear todo.stashbert_test", "fill todo.bring_zuhause",
			"clear todo.bring_zuhause")
	}
}

// TestShoppingTargetOverMQTT receives the retained offer of Home Assistant
// with everything wired as in main, switches the target list through the
// API and receives the snapshots of the switch and the shopping.changed of
// a booking with the new list on the broker (architecture.md, 11.3 and 11.7).
func TestShoppingTargetOverMQTT(t *testing.T) {
	srv, url := startBroker(t)
	// Home Assistant publishes the offer retained before StashBert connects.
	if err := srv.Publish("vorrat/in/targets", []byte(targetOffer), true, 1); err != nil {
		t.Fatalf("publish offer: %v", err)
	}
	msgs := subscribeAll(t, srv, "vorrat/events/#")

	db := newDB(t)
	// See insertProducts: p2 and p3 are on the list, p4 has a target but
	// nothing missing, p1 has no target.
	insertProducts(t, db)
	logger := slog.New(slog.DiscardHandler)
	client, err := mqtt.New(config.MQTT{URL: url, ClientID: "stashbert", TopicPrefix: "vorrat", HAPrefix: "homeassistant"}, logger)
	if err != nil {
		t.Fatalf("mqtt.New: %v", err)
	}
	deliverer := outbox.NewDeliverer(db, client, "vorrat", logger)
	client.OnConnect(func(context.Context) { deliverer.Wake() })
	writer := outbox.NewWriter(db, deliverer.Wake, func() {}, logger)
	targets := outbox.NewTargets(writer, client, "vorrat", logger)
	client.OnMessage(targets.Receive)
	h, _ := newAppWithDeps(t, app.Deps{
		DB: db, Publisher: writer, Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(), MQTT: targets,
	})
	// Stopped in reverse order: the deliverer first, then the client.
	startJob(t, func(ctx context.Context) {
		if err := client.Run(ctx); err != nil {
			t.Errorf("mqtt Run: %v", err)
		}
	})
	startJob(t, func(ctx context.Context) { deliverer.Run(ctx, outbox.MinBackoff, outbox.MaxBackoff) })

	connected := `{"status": "connected", "targets": [
		{"id": "todo.stashbert_test", "name": "StashBert Test"}, {"id": "todo.bring_zuhause", "name": "Zuhause"}
	], "target": null}`
	for deadline := time.Now().Add(mqttWait); ; time.Sleep(10 * time.Millisecond) {
		rec := get(h, "/api/v1/integrations/mqtt")
		if rec.Code == http.StatusOK && jsonEqual(t, rec.Body.String(), connected) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET /integrations/mqtt = %d %s, want %s", rec.Code, rec.Body.String(), connected)
		}
	}

	items := `[
		{"product_id": "p2", "name": "kidneybohnen", "on_list": true, "missing": 3, "quantity": 3, "unit": "piece"},
		{"product_id": "p3", "name": "Kidneybohnen", "on_list": true, "missing": 4, "quantity": 4, "unit": "piece"},
		{"product_id": "p4", "name": "Mehl", "on_list": false, "missing": 0, "quantity": 0, "unit": "piece"}
	]`
	cleared := `[
		{"product_id": "p2", "name": "kidneybohnen", "on_list": false, "missing": 3, "quantity": 0, "unit": "piece"},
		{"product_id": "p3", "name": "Kidneybohnen", "on_list": false, "missing": 4, "quantity": 0, "unit": "piece"},
		{"product_id": "p4", "name": "Mehl", "on_list": false, "missing": 0, "quantity": 0, "unit": "piece"}
	]`
	snapshot := func(list, items string) map[string]any {
		data, _ := decodeJSON(t, `{"list": "`+list+`", "items": `+items+`}`).(map[string]any)
		return data
	}

	// The first choice fills the list.
	start := time.Now()
	if rec := putTarget(h, `{"id": "todo.stashbert_test"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	checkEventMessage(t, msgs, start, "vorrat/events/shopping.snapshot", "shopping.snapshot", snapshot("todo.stashbert_test", items))
	checkNoMessage(t, msgs, 200*time.Millisecond)

	// A switch clears the old list first, then fills the new one.
	start = time.Now()
	if rec := putTarget(h, `{"id": "todo.bring_zuhause"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	checkEventMessage(t, msgs, start, "vorrat/events/shopping.snapshot", "shopping.snapshot", snapshot("todo.stashbert_test", cleared))
	checkEventMessage(t, msgs, start, "vorrat/events/shopping.snapshot", "shopping.snapshot", snapshot("todo.bring_zuhause", items))
	checkNoMessage(t, msgs, 200*time.Millisecond)

	// shopping.changed carries the new list.
	start = time.Now()
	_, movement := decodeMovementResult(t, post(h, "/api/v1/movements", `{"product_id": "p2", "kind": "add", "quantity": 3}`))
	checkEventMessage(t, msgs, start, "vorrat/events/stock.added", "stock.added",
		map[string]any{"product_id": "p2", "movement_id": movement["id"], "delta": 3.0, "stock_after": 5.0})
	checkEventMessage(t, msgs, start, "vorrat/events/shopping.changed", "shopping.changed", map[string]any{
		"product_id": "p2", "name": "kidneybohnen", "missing_before": 3.0, "missing_after": 0.0,
		"marked": false, "on_list": false, "quantity": 0.0, "unit": "piece", "list": "todo.bring_zuhause",
	})

	// Removing the choice clears the list.
	start = time.Now()
	if rec := putTarget(h, `{"id": null}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	checkEventMessage(t, msgs, start, "vorrat/events/shopping.snapshot", "shopping.snapshot", snapshot("todo.bring_zuhause", `[
		{"product_id": "p2", "name": "kidneybohnen", "on_list": false, "missing": 0, "quantity": 0, "unit": "piece"},
		{"product_id": "p3", "name": "Kidneybohnen", "on_list": false, "missing": 4, "quantity": 0, "unit": "piece"},
		{"product_id": "p4", "name": "Mehl", "on_list": false, "missing": 0, "quantity": 0, "unit": "piece"}
	]`))
	checkNoMessage(t, msgs, 200*time.Millisecond)
}

// jsonEqual reports whether the JSON got equals the JSON want.
func jsonEqual(t *testing.T, got, want string) bool {
	t.Helper()
	var g any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		return false
	}
	return reflect.DeepEqual(g, decodeJSON(t, want))
}
