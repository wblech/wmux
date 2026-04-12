package event

import "sync"

const (
	// subscriberBufferSize is the channel buffer for each subscriber.
	// Events are dropped (non-blocking send) if the subscriber can't keep up.
	subscriberBufferSize = 256
)

// Subscription represents a single event subscriber.
type Subscription struct {
	ch     chan Event
	bus    *Bus
	filter map[Type]struct{} // nil = all events
	once   sync.Once
}

// Events returns the channel on which events are delivered.
func (s *Subscription) Events() <-chan Event {
	return s.ch
}

// Unsubscribe removes this subscription from the bus and closes the channel.
func (s *Subscription) Unsubscribe() {
	s.once.Do(func() {
		if s.bus != nil {
			s.bus.removeSub(s)
		}
		close(s.ch)
	})
}

// Bus is a synchronous fan-out event dispatcher.
type Bus struct {
	mu     sync.RWMutex
	subs   []*Subscription
	closed bool
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		mu:     sync.RWMutex{},
		subs:   nil,
		closed: false,
	}
}

// Subscribe registers a subscriber that receives all events.
func (b *Bus) Subscribe() *Subscription {
	return b.subscribe(nil)
}

// SubscribeTypes registers a subscriber that receives only the specified event types.
func (b *Bus) SubscribeTypes(types ...Type) *Subscription {
	filter := make(map[Type]struct{}, len(types))
	for _, t := range types {
		filter[t] = struct{}{}
	}
	return b.subscribe(filter)
}

func (b *Bus) subscribe(filter map[Type]struct{}) *Subscription {
	sub := &Subscription{
		ch:     make(chan Event, subscriberBufferSize),
		bus:    b,
		filter: filter,
		once:   sync.Once{},
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(sub.ch)
		return sub
	}
	b.subs = append(b.subs, sub)
	b.mu.Unlock()

	return sub
}

// Publish sends an event to all matching subscribers.
// Events are delivered non-blocking — if a subscriber's buffer is full, the event is dropped.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for _, sub := range b.subs {
		if sub.filter != nil {
			if _, ok := sub.filter[e.Type]; !ok {
				continue
			}
		}

		select {
		case sub.ch <- e:
		default:
			// Slow subscriber — drop event.
		}
	}
}

// Close shuts down the bus and closes all subscriber channels.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true

	for _, sub := range b.subs {
		sub.once.Do(func() {
			close(sub.ch)
		})
	}

	b.subs = nil
}

// removeSub removes a subscription from the bus's subscriber list.
func (b *Bus) removeSub(sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, s := range b.subs {
		if s == sub {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}
