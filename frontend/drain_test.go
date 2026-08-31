package frontend

import "testing"

// TestDrain covers a channel that is closed as well as one that is merely
// empty.
//
// The event stream reaches it with whatever the broker has pending at that
// instant, so which branch a run lands on is the scheduler's business: the
// closed channel is the shutdown path, and a live run only takes it if the
// timing falls that way.
func TestDrain(t *testing.T) {
	// Nothing pending: the default arm returns at once.
	drain(make(chan struct{}))

	// Pending notifications are taken together, so a burst redraws once.
	events := make(chan struct{}, 3)
	events <- struct{}{}
	events <- struct{}{}
	drain(events)
	if len(events) != 0 {
		t.Errorf("drain() left %d notifications pending", len(events))
	}

	// A closed channel reads forever, and is the arm that ends the loop.
	closed := make(chan struct{}, 1)
	closed <- struct{}{}
	close(closed)
	drain(closed)
}
