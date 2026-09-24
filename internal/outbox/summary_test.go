package outbox_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/outbox"
)

// summaryPayload is the summary of insertSnapshotProducts with Mehl to
// review.
const summaryPayload = `{"product_count":8,"shopping_count":5,"empty_count":3,"review_count":1,"shopping":[` +
	`{"name":"apfelsaft","missing":0,"quantity":0,"unit":"piece"},` +
	`{"name":"Jever Pilsener","missing":17,"quantity":1,"unit":"crate"},` +
	`{"name":"kidneybohnen","missing":4,"quantity":4,"unit":"piece"},` +
	`{"name":"Kidneybohnen","missing":3,"quantity":3,"unit":"piece"},` +
	`{"name":"Milch","missing":2,"quantity":2,"unit":"piece"}` +
	`],"shopping_truncated":false}`

// emptySummaryPayload is the summary without products.
const emptySummaryPayload = `{"product_count":0,"shopping_count":0,"empty_count":0,"review_count":0,"shopping":[],"shopping_truncated":false}`

// runSummary starts s.Run with delay and ends it when the test ends.
func runSummary(t *testing.T, s *outbox.SummaryPublisher, delay time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx, delay)
	}()
	t.Cleanup(func() {
		cancel()
		receive(t, done, "Run to return")
	})
}

// checkSummaryCall receives the next call of Publish of client and checks
// that it publishes payload retained on vorrat/state/summary.
func checkSummaryCall(t *testing.T, client *fakeClient, payload string) {
	t.Helper()
	m := receive(t, client.calls, "the summary")
	if m.topic != "vorrat/state/summary" || !m.retain {
		t.Errorf("topic, retain = %q, %v, want vorrat/state/summary, true", m.topic, m.retain)
	}
	if m.payload != payload {
		t.Errorf("payload:\n got %s\nwant %s", m.payload, payload)
	}
}

// checkNoCall asserts that client gets no call of Publish for d.
func checkNoCall(t *testing.T, client *fakeClient, d time.Duration) {
	t.Helper()
	select {
	case m := <-client.calls:
		t.Fatalf("Publish(%s, %s), want no call", m.topic, m.payload)
	case <-time.After(d):
	}
}

// checkWarning asserts that logs holds exactly one warning with msg and an
// error.
func checkWarning(t *testing.T, logs *logBuffer, msg string) {
	t.Helper()
	out := strings.TrimSpace(logs.String())
	var line map[string]any
	if err := json.Unmarshal([]byte(out), &line); err != nil {
		t.Fatalf("log = %q, want one JSON line: %v", out, err)
	}
	if e, _ := line["error"].(string); line["level"] != "WARN" || line["msg"] != msg || e == "" {
		t.Errorf("log = %s, want a warning %q with error", out, msg)
	}
}

func TestSummaryPublish(t *testing.T) {
	sqlDB := openDB(t)
	insertSnapshotProducts(t, sqlDB)
	exec(t, sqlDB, "UPDATE products SET needs_review = 1 WHERE id = 'p6'")
	client := newFakeClient(mqtt.StateConnected)
	logger, logs := newLogger()
	s := outbox.NewSummaryPublisher(sqlDB, client, "vorrat", logger)

	s.Publish(testContext(t))

	checkSummaryCall(t, client, summaryPayload)
	checkNoCall(t, client, 0)
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want none", out)
	}
}

func TestSummaryPublishWithoutProducts(t *testing.T) {
	client := newFakeClient(mqtt.StateConnected)
	logger, _ := newLogger()
	s := outbox.NewSummaryPublisher(openDB(t), client, "vorrat", logger)

	s.Publish(testContext(t))

	checkSummaryCall(t, client, emptySummaryPayload)
}

// Without a connection the summary is skipped; the next connection
// publishes it.
func TestSummaryPublishWithoutConnection(t *testing.T) {
	client := newFakeClient(mqtt.StateConnecting)
	logger, logs := newLogger()
	s := outbox.NewSummaryPublisher(openDB(t), client, "vorrat", logger)

	s.Publish(testContext(t))

	checkNoCall(t, client, 0)
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want none", out)
	}
}

func TestSummaryPublishError(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		client := newFakeClient(mqtt.StateConnected)
		client.set(mqtt.StateConnected, true)
		logger, logs := newLogger()
		s := outbox.NewSummaryPublisher(openDB(t), client, "vorrat", logger)

		s.Publish(testContext(t))

		checkSummaryCall(t, client, emptySummaryPayload)
		checkWarning(t, logs, "publish summary")
	})
	t.Run("database", func(t *testing.T) {
		sqlDB := openDB(t)
		client := newFakeClient(mqtt.StateConnected)
		logger, logs := newLogger()
		s := outbox.NewSummaryPublisher(sqlDB, client, "vorrat", logger)
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("close database: %v", err)
		}

		s.Publish(testContext(t))

		checkNoCall(t, client, 0)
		checkWarning(t, logs, "publish summary")
	})
	t.Run("ended context", func(t *testing.T) {
		client := newFakeClient(mqtt.StateConnected)
		logger, logs := newLogger()
		s := outbox.NewSummaryPublisher(openDB(t), client, "vorrat", logger)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		s.Publish(ctx)

		checkNoCall(t, client, 0)
		if out := logs.String(); out != "" {
			t.Errorf("log = %s, want none", out)
		}
	})
}

// Several requests in quick succession result in one publish, delay after
// the last one; a later request results in another one.
func TestSummaryPublisherCoalescesRequests(t *testing.T) {
	const delay = 300 * time.Millisecond
	sqlDB := openDB(t)
	client := newFakeClient(mqtt.StateConnected)
	logger, logs := newLogger()
	s := outbox.NewSummaryPublisher(sqlDB, client, "vorrat", logger)
	runSummary(t, s, delay)

	s.Request()
	time.Sleep(delay / 3)
	s.Request()
	time.Sleep(delay / 3)
	s.Request()
	last := time.Now()

	checkSummaryCall(t, client, emptySummaryPayload)
	if d := time.Since(last); d < delay {
		t.Errorf("published %v after the last request, want at least %v", d, delay)
	}
	checkNoCall(t, client, 2*delay)

	insertSnapshotProducts(t, sqlDB)
	exec(t, sqlDB, "UPDATE products SET needs_review = 1 WHERE id = 'p6'")
	s.Request()
	checkSummaryCall(t, client, summaryPayload)
	checkNoCall(t, client, 2*delay)
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want none", out)
	}
}

// Request never blocks, also before Run.
func TestSummaryPublisherRequestBeforeRun(t *testing.T) {
	client := newFakeClient(mqtt.StateConnected)
	logger, _ := newLogger()
	s := outbox.NewSummaryPublisher(openDB(t), client, "vorrat", logger)
	for range 3 {
		s.Request()
	}

	runSummary(t, s, 10*time.Millisecond)

	checkSummaryCall(t, client, emptySummaryPayload)
	checkNoCall(t, client, 100*time.Millisecond)
}

// Without a connection Run skips the publish.
func TestSummaryPublisherWithoutConnection(t *testing.T) {
	client := newFakeClient(mqtt.StateConnecting)
	logger, logs := newLogger()
	s := outbox.NewSummaryPublisher(openDB(t), client, "vorrat", logger)
	runSummary(t, s, 10*time.Millisecond)

	s.Request()

	receive(t, client.states, "a look at the state")
	checkNoCall(t, client, 100*time.Millisecond)
	if out := logs.String(); out != "" {
		t.Errorf("log = %s, want none", out)
	}
}
