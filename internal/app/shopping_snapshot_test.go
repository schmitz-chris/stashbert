package app_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
)

// snapshotRequests counts the requests for shopping.snapshot.
type snapshotRequests struct{ n atomic.Int64 }

func (r *snapshotRequests) Request() { r.n.Add(1) }

// sendSnapshot sends POST /api/v1/shopping-list/snapshot without a body to h.
func sendSnapshot(h http.Handler) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/shopping-list/snapshot", nil))
	return rec
}

// Every request is passed on; the snapshotter makes one snapshot of the
// requests within 2 s.
func TestSendShoppingSnapshot(t *testing.T) {
	var requests snapshotRequests
	h, _ := newAppWithDeps(t, app.Deps{
		Publisher: events.Nop{}, Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(), Snapshots: &requests,
	})

	for i := range 2 {
		rec := sendSnapshot(h)

		if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
			t.Errorf("status = %d, body %q, want %d without body", rec.Code, rec.Body.String(), http.StatusAccepted)
		}
		if n := requests.n.Load(); n != int64(i+1) {
			t.Errorf("requests = %d, want %d", n, i+1)
		}
	}
}

func TestSendShoppingSnapshotWithoutMQTT(t *testing.T) {
	h := newHandler(t)

	rec := sendSnapshot(h)

	checkProblemCode(t, rec, http.StatusConflict, "mqtt_disabled")
}
