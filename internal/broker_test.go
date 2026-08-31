package internal

import "testing"

func TestBrokerCoalescesNotifications(t *testing.T) {
	broker := NewBroker()

	events, cancel := broker.Subscribe()
	defer cancel()

	// The channel holds one pending notification, so a burst wakes a
	// subscriber once rather than once per event.
	for range 5 {
		broker.Notify()
	}
	if got := len(events); got != 1 {
		t.Fatalf("five notifications left %d pending, want 1", got)
	}

	<-events
	if got := len(events); got != 0 {
		t.Fatalf("reading the notification left %d pending", got)
	}
}

func TestBrokerReleasesSubscribers(t *testing.T) {
	broker := NewBroker()

	_, cancel := broker.Subscribe()
	if got := broker.Len(); got != 1 {
		t.Fatalf("subscribing left %d subscribers, want 1", got)
	}

	cancel()
	if got := broker.Len(); got != 0 {
		t.Fatalf("cancel left %d subscribers", got)
	}
	cancel() // idempotent
}

func TestBrokerNilIsUsable(t *testing.T) {
	// A nil broker is what a zero value carries, and notifying one is a
	// no-op rather than a panic.
	var broker *Broker

	events, cancel := broker.Subscribe()
	if events != nil {
		t.Fatal("a nil broker handed out a channel")
	}
	broker.Notify()
	cancel()
}
