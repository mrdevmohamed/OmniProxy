// Package ring provides the bounded event ring that FFI/mobile transport entry
// points drain. Producers (core goroutines) append; transports swap the batch
// out on poll. No platform logic lives here.
package ring

import (
	"sync"

	"omniproxy/core/api"
)

// EventRing is a bounded FIFO of bridge events. It is safe for concurrent use.
type EventRing struct {
	mu     sync.Mutex
	events []api.Event
	cap    int
}

// New returns an empty ring that keeps at most cap events (default 512),
// dropping the oldest when full.
func New(cap int) *EventRing {
	if cap <= 0 {
		cap = 512
	}
	return &EventRing{cap: cap}
}

// Push appends one event, dropping the oldest entry when the ring is full.
func (r *EventRing) Push(e api.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	if len(r.events) > r.cap {
		r.events = append([]api.Event(nil), r.events[len(r.events)-r.cap:]...)
	}
}

// Drain returns and clears all buffered events (may be empty).
func (r *EventRing) Drain() []api.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	batch := r.events
	r.events = nil
	return batch
}
