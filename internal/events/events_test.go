package events_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/schmitz-chris/stashbert/internal/events"
)

var (
	_ events.Publisher = events.Nop{}
	_ events.Publisher = (*events.Recorder)(nil)
)

func TestNew(t *testing.T) {
	data := events.StockData{ProductID: "p1", MovementID: "m1", Delta: -1, StockAfter: 2}

	before := time.Now()
	e := events.New(events.TypeStockConsumed, data)
	after := time.Now()

	id, err := uuid.Parse(e.ID)
	if err != nil {
		t.Fatalf("ID %q is not a UUID: %v", e.ID, err)
	}
	if id.Version() != 7 {
		t.Errorf("ID version = %d, want 7", id.Version())
	}
	if e.ID != strings.ToLower(e.ID) {
		t.Errorf("ID %q is not lower case", e.ID)
	}
	if e.Type != events.TypeStockConsumed {
		t.Errorf("Type = %q, want %q", e.Type, events.TypeStockConsumed)
	}
	if e.Source != "stashbert" {
		t.Errorf("Source = %q, want %q", e.Source, "stashbert")
	}
	if e.Time.Location() != time.UTC {
		t.Errorf("Time location = %v, want UTC", e.Time.Location())
	}
	if e.Time.Before(before) || e.Time.After(after) {
		t.Errorf("Time = %v, want between %v and %v", e.Time, before, after)
	}
	if e.Data != data {
		t.Errorf("Data = %#v, want %#v", e.Data, data)
	}

	if other := events.New(events.TypeStockConsumed, data); other.ID == e.ID {
		t.Errorf("two events share the ID %q", e.ID)
	}
}

func TestTypes(t *testing.T) {
	tests := []struct{ got, want string }{
		{events.TypeProductCreated, "product.created"},
		{events.TypeStockAdded, "stock.added"},
		{events.TypeStockConsumed, "stock.consumed"},
		{events.TypeStockAdjusted, "stock.adjusted"},
		{events.TypeProductEmpty, "product.empty"},
		{events.TypeShoppingChanged, "shopping.changed"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("type constant = %q, want %q", tt.got, tt.want)
		}
	}
}

func TestDataJSON(t *testing.T) {
	tests := []struct {
		data any
		want string
	}{
		{
			events.ProductCreatedData{ProductID: "p1", Name: "Kidneybohnen", Origin: "manual"},
			`{"product_id":"p1","name":"Kidneybohnen","origin":"manual"}`,
		},
		{
			events.StockData{ProductID: "p1", MovementID: "m1", Delta: -1, StockAfter: 2},
			`{"product_id":"p1","movement_id":"m1","delta":-1,"stock_after":2}`,
		},
		{
			events.ProductEmptyData{ProductID: "p1", Name: "Kidneybohnen"},
			`{"product_id":"p1","name":"Kidneybohnen"}`,
		},
		{
			events.ShoppingChangedData{ProductID: "p1", Name: "Kidneybohnen", MissingBefore: 0, MissingAfter: 4},
			`{"product_id":"p1","name":"Kidneybohnen","missing_before":0,"missing_after":4}`,
		},
	}
	for _, tt := range tests {
		got, err := json.Marshal(tt.data)
		if err != nil {
			t.Fatalf("Marshal(%#v): %v", tt.data, err)
		}
		if string(got) != tt.want {
			t.Errorf("Marshal(%#v) = %s, want %s", tt.data, got, tt.want)
		}
	}
}

func TestRecorder(t *testing.T) {
	ctx := t.Context()
	var r events.Recorder
	if got := r.Events(); len(got) != 0 {
		t.Fatalf("new Recorder has %d events, want 0", len(got))
	}

	first := events.New(events.TypeStockAdded, nil)
	second := events.New(events.TypeShoppingChanged, nil)
	r.Publish(ctx, first)
	r.Publish(ctx, second)

	got := r.Events()
	if len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("Events() = %v, want [%v %v]", got, first, second)
	}

	got[0] = events.Event{}
	if again := r.Events(); again[0].ID != first.ID {
		t.Errorf("changing the result of Events() changed the Recorder")
	}
}

func TestRecorderConcurrent(t *testing.T) {
	ctx := t.Context()
	var r events.Recorder
	const n = 100

	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			r.Publish(ctx, events.New(events.TypeStockAdded, nil))
			_ = r.Events()
		})
	}
	wg.Wait()

	if got := len(r.Events()); got != n {
		t.Errorf("recorded %d events, want %d", got, n)
	}
}
