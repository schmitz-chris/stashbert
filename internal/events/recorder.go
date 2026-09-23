package events

import (
	"context"
	"slices"
	"sync"
)

// Recorder is a Publisher that keeps all published events in order. It is
// safe for concurrent use and meant for tests of other packages. The zero
// value is ready to use.
type Recorder struct {
	mu     sync.Mutex
	events []Event
}

// Publish appends e to the recorded events.
func (r *Recorder) Publish(_ context.Context, e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// Events returns a copy of the recorded events in publishing order.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}
