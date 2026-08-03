package api

import "sync"

// Event is one asynchronous bridge event (contract §4).
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// EventSink receives events. Implementations must be safe for concurrent use.
type EventSink interface {
	SendEvent(Event)
}

// FuncEventSink adapts a callback to EventSink.
type FuncEventSink func(Event)

// SendEvent implements EventSink.
func (f FuncEventSink) SendEvent(e Event) { f(e) }

// EventBus fans events out to subscribed sinks. Delivery to a sink happens on
// the publisher's goroutine; sinks must not block or recurse. Subscriptions
// are tracked by opaque handles so any EventSink (including funcs) is safe.
type EventBus struct {
	mu   sync.Mutex
	next uint64
	subs map[uint64]*subscription
}

// subscription is one registered sink.
type subscription struct {
	id   uint64
	sink EventSink
}

// NewEventBus returns an empty bus.
func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[uint64]*subscription)}
}

// Subscribe registers a sink and returns its handle (nil when sink is nil).
func (b *EventBus) Subscribe(sink EventSink) *subscription {
	if sink == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	sub := &subscription{id: b.next, sink: sink}
	b.subs[sub.id] = sub
	return sub
}

// Unsubscribe removes a subscription by handle (idempotent).
func (b *EventBus) Unsubscribe(sub *subscription) {
	if sub == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, sub.id)
}

// Publish delivers e to every subscribed sink.
func (b *EventBus) Publish(e Event) {
	b.mu.Lock()
	sinks := make([]EventSink, 0, len(b.subs))
	for _, sub := range b.subs {
		sinks = append(sinks, sub.sink)
	}
	b.mu.Unlock()
	for _, s := range sinks {
		s.SendEvent(e)
	}
}
