package outbox_test

import (
	"database/sql"
	"encoding/json"
	"maps"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/outbox"
)

// storeTarget stores the chosen target list id with name as settings.
func storeTarget(t *testing.T, sqlDB *sql.DB, id, name string) {
	t.Helper()
	exec(t, sqlDB, "INSERT INTO settings (key, value) VALUES ('shopping_target_id', ?), ('shopping_target_name', ?)", id, name)
}

// settings returns all rows of the table settings.
func settings(t *testing.T, sqlDB *sql.DB) map[string]string {
	t.Helper()
	rows, err := sqlDB.QueryContext(testContext(t), "SELECT key, value FROM settings")
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

// TestSetShoppingTarget switches the target list and checks the stored
// choice and the snapshots in the outbox: first the old list cleared, then
// the new one filled (architecture.md, 11.7).
func TestSetShoppingTarget(t *testing.T) {
	zuhause := &domain.ShoppingTarget{ID: "todo.bring_zuhause", Name: "Zuhause"}
	tests := []struct {
		name     string
		stored   *domain.ShoppingTarget // stored choice before, nil for none
		target   *domain.ShoppingTarget // new choice, nil to remove it
		settings map[string]string      // stored afterwards
		// snapshots is the data of the snapshots in the outbox in their order.
		snapshots []string
	}{
		{
			name:     "first choice",
			target:   zuhause,
			settings: map[string]string{"shopping_target_id": "todo.bring_zuhause", "shopping_target_name": "Zuhause"},
			snapshots: []string{
				`{"list":"todo.bring_zuhause","items":` + snapshotItems + `}`,
			},
		},
		{
			name:     "switch to another list",
			stored:   &domain.ShoppingTarget{ID: "todo.stashbert_test", Name: "StashBert Test"},
			target:   zuhause,
			settings: map[string]string{"shopping_target_id": "todo.bring_zuhause", "shopping_target_name": "Zuhause"},
			snapshots: []string{
				`{"list":"todo.stashbert_test","items":` + clearedItems + `}`,
				`{"list":"todo.bring_zuhause","items":` + snapshotItems + `}`,
			},
		},
		{
			name:     "remove the choice",
			stored:   zuhause,
			settings: map[string]string{},
			snapshots: []string{
				`{"list":"todo.bring_zuhause","items":` + clearedItems + `}`,
			},
		},
		{
			name:   "same list again, even with another name",
			stored: zuhause,
			target: &domain.ShoppingTarget{ID: "todo.bring_zuhause", Name: "Daheim"},
			// Nothing changes.
			settings: map[string]string{"shopping_target_id": "todo.bring_zuhause", "shopping_target_name": "Zuhause"},
		},
		{
			name:     "remove without a choice",
			settings: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB := openDB(t)
			insertSnapshotProducts(t, sqlDB)
			if tt.stored != nil {
				storeTarget(t, sqlDB, tt.stored.ID, tt.stored.Name)
			}
			var wakes, reported atomic.Int64
			logger, logs := newLogger()
			w := outbox.NewWriter(sqlDB, func() { wakes.Add(1) }, func() { reported.Add(1) }, logger)
			start := time.Now()

			if err := w.SetShoppingTarget(testContext(t), tt.target); err != nil {
				t.Fatalf("SetShoppingTarget: %v", err)
			}

			if got := settings(t, sqlDB); !maps.Equal(got, tt.settings) {
				t.Errorf("settings = %v, want %v", got, tt.settings)
			}
			got := entries(t, sqlDB)
			if len(got) != len(tt.snapshots) {
				t.Fatalf("outbox has %d entries, want %d: %+v", len(got), len(tt.snapshots), got)
			}
			for i, data := range tt.snapshots {
				checkSnapshotEntry(t, got[i], start, data)
				if i > 0 && got[i].seq <= got[i-1].seq {
					t.Errorf("seq of snapshot %d = %d, not after %d", i, got[i].seq, got[i-1].seq)
				}
			}
			wantWakes := int64(0)
			if len(tt.snapshots) > 0 {
				wantWakes = 1
			}
			if n := wakes.Load(); n != wantWakes {
				t.Errorf("wake called %d times, want %d", n, wantWakes)
			}
			// A switch is no event for the summary.
			if n := reported.Load(); n != 0 {
				t.Errorf("onEvent called %d times, want 0", n)
			}
			if logs.String() != "" {
				t.Errorf("log = %s, want none", logs)
			}
		})
	}
}

// After a switch, shopping.changed carries the new list, and after
// removing the choice an empty one (architecture.md, 11.3 and 11.7).
func TestSetShoppingTargetListInShoppingChanged(t *testing.T) {
	sqlDB := openDB(t)
	insertProduct(t, sqlDB, product{id: "p1", name: "Kidneybohnen", stock: 2, target: 5})
	logger, logs := newLogger()
	w := outbox.NewWriter(sqlDB, func() {}, func() {}, logger)
	changed := events.ShoppingChangedData{ProductID: "p1", Name: "Kidneybohnen", MissingBefore: 0, MissingAfter: 3}

	for _, target := range []*domain.ShoppingTarget{
		{ID: "todo.stashbert_test", Name: "StashBert Test"},
		{ID: "todo.bring_zuhause", Name: "Zuhause"},
		nil,
	} {
		if err := w.SetShoppingTarget(testContext(t), target); err != nil {
			t.Fatalf("SetShoppingTarget(%v): %v", target, err)
		}
		exec(t, sqlDB, "DELETE FROM outbox")

		w.Publish(testContext(t), events.New(events.TypeShoppingChanged, changed))

		got := entries(t, sqlDB)
		if len(got) != 1 {
			t.Fatalf("outbox = %+v, want one entry; log:\n%s", got, logs)
		}
		var e struct{ Data struct{ List *string } }
		if err := json.Unmarshal([]byte(got[0].payload), &e); err != nil {
			t.Fatalf("decode payload %s: %v", got[0].payload, err)
		}
		want := ""
		if target != nil {
			want = target.ID
		}
		if e.Data.List == nil || *e.Data.List != want {
			t.Errorf("after choosing %v: payload %s, want list %q", target, got[0].payload, want)
		}
	}
}

// A failed switch returns the error, changes nothing and does not wake the
// deliverer.
func TestSetShoppingTargetError(t *testing.T) {
	sqlDB := openDB(t)
	logger, _ := newLogger()
	var wakes atomic.Int64
	w := outbox.NewWriter(sqlDB, func() { wakes.Add(1) }, func() {}, logger)
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	if err := w.SetShoppingTarget(testContext(t), &domain.ShoppingTarget{ID: "todo.bring_zuhause", Name: "Zuhause"}); err == nil {
		t.Error("SetShoppingTarget = nil, want an error")
	}
	if n := wakes.Load(); n != 0 {
		t.Errorf("wake called %d times, want 0", n)
	}
}

// Targets takes valid offers on <prefix>/in/targets, also retained ones,
// and keeps the last valid offer when an invalid one arrives
// (architecture.md, 11.7).
func TestTargetsReceive(t *testing.T) {
	logger, logs := newLogger()
	targets := outbox.NewTargets(outbox.NewWriter(openDB(t), func() {}, func() {}, logger), newFakeClient(mqtt.StateConnected), "vorrat", logger)
	checkOffer := func(want ...domain.ShoppingTarget) {
		t.Helper()
		if want == nil {
			want = []domain.ShoppingTarget{}
		}
		if got := targets.Offer(); got == nil || !reflect.DeepEqual(got, want) {
			t.Errorf("offer = %#v, want %#v", got, want)
		}
	}
	test := domain.ShoppingTarget{ID: "todo.stashbert_test", Name: "StashBert Test"}
	zuhause := domain.ShoppingTarget{ID: "todo.bring_zuhause", Name: "Zuhause"}

	// Before the first offer.
	checkOffer()

	// Home Assistant publishes the offer retained, so it arrives with the
	// retain flag after subscribing.
	targets.Receive(mqtt.Message{Topic: "vorrat/in/targets", Payload: []byte(`[{"id":"todo.stashbert_test","name":"StashBert Test"}]`), Retained: true})
	checkOffer(test)

	// Other topics are ignored.
	for _, m := range []mqtt.Message{
		{Topic: "stashbert/in/targets", Payload: []byte("[]")},
		{Topic: "vorrat/in/targets/x", Payload: []byte("[]")},
		{Topic: "vorrat/in/snapshot", Payload: []byte("[]")},
	} {
		targets.Receive(m)
	}
	checkOffer(test)
	if logs.String() != "" {
		t.Errorf("log = %s, want none", logs)
	}

	// An invalid offer is logged and ignored.
	targets.Receive(mqtt.Message{Topic: "vorrat/in/targets", Payload: []byte(`[{"id":"","name":"Leer"}]`)})
	checkOffer(test)
	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(logs.String())), &line); err != nil {
		t.Fatalf("log = %q, want one JSON line: %v", logs, err)
	}
	if msg, _ := line["error"].(string); line["level"] != "WARN" || line["msg"] != "ignore invalid target lists" ||
		line["topic"] != "vorrat/in/targets" || msg == "" {
		t.Errorf("log = %s, want a warning with topic and error", logs)
	}

	// A new offer replaces the old one, also without the retain flag.
	targets.Receive(mqtt.Message{Topic: "vorrat/in/targets", Payload: []byte(`[{"id":"todo.bring_zuhause","name":"Zuhause"},{"id":"todo.stashbert_test","name":"StashBert Test"}]`)})
	checkOffer(zuhause, test)

	// The offer is a copy.
	targets.Offer()[0].Name = "Geändert"
	checkOffer(zuhause, test)

	// An empty offer is valid.
	targets.Receive(mqtt.Message{Topic: "vorrat/in/targets", Payload: []byte(`[]`), Retained: true})
	checkOffer()
}

// Targets reports the state of the client and stores the choice with the
// writer.
func TestTargetsStateAndSetTarget(t *testing.T) {
	sqlDB := openDB(t)
	logger, _ := newLogger()
	client := newFakeClient(mqtt.StateConnecting)
	targets := outbox.NewTargets(outbox.NewWriter(sqlDB, func() {}, func() {}, logger), client, "vorrat", logger)

	if s := targets.State(); s != mqtt.StateConnecting {
		t.Errorf("State = %s, want %s", s, mqtt.StateConnecting)
	}
	client.set(mqtt.StateConnected, false)
	if s := targets.State(); s != mqtt.StateConnected {
		t.Errorf("State = %s, want %s", s, mqtt.StateConnected)
	}

	if err := targets.SetTarget(testContext(t), &domain.ShoppingTarget{ID: "todo.bring_zuhause", Name: "Zuhause"}); err != nil {
		t.Fatalf("SetTarget: %v", err)
	}
	want := map[string]string{"shopping_target_id": "todo.bring_zuhause", "shopping_target_name": "Zuhause"}
	if got := settings(t, sqlDB); !maps.Equal(got, want) {
		t.Errorf("settings = %v, want %v", got, want)
	}
	if n := len(entries(t, sqlDB)); n != 1 {
		t.Errorf("outbox has %d entries, want the snapshot", n)
	}
	// Nothing is published directly; the deliverer does that.
	select {
	case m := <-client.calls:
		t.Errorf("published %+v, want nothing", m)
	default:
	}
}
