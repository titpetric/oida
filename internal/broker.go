package internal

import "sync"

// Broker fans out change notifications to live view subscribers. Sends are non
// blocking: a subscriber that is behind coalesces updates instead of slowing
// down the request that produced them.
type Broker struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

// NewBroker returns an empty broker.
func NewBroker() *Broker {
	return &Broker{subscribers: make(map[chan struct{}]struct{})}
}

// Subscribe returns a channel notified on every change, and a function that
// releases it. The release function is idempotent.
func (b *Broker) Subscribe() (<-chan struct{}, func()) {
	if b == nil {
		return nil, func() {}
	}

	events := make(chan struct{}, 1)

	b.mu.Lock()
	b.subscribers[events] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	return events, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, events)
			b.mu.Unlock()
			close(events)
		})
	}
}

// Notify wakes every subscriber. Subscribers with a pending notification are
// skipped, so a burst of traces produces one redraw, not one per trace.
func (b *Broker) Notify() {
	if b == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for events := range b.subscribers {
		select {
		case events <- struct{}{}:
		default:
		}
	}
}

// Len returns the number of active subscribers.
func (b *Broker) Len() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers)
}
