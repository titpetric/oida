package oida

import (
	"context"
	"testing"
)

func TestSubscribeCoalescesNotifications(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	events, cancel := tracer.Subscribe()
	defer cancel()

	for range 5 {
		if err := tracer.Observe(t.Context(), "job", func(context.Context) error { return nil }); err != nil {
			t.Fatalf("Observe: %v", err)
		}
	}

	select {
	case <-events:
	default:
		t.Fatal("no notification was delivered")
	}

	// The buffer holds one pending notification, so a burst wakes once more at
	// most rather than once per trace.
	woken := 1
	for {
		select {
		case <-events:
			woken++
			if woken > 2 {
				t.Fatalf("burst of 5 traces produced %d notifications", woken)
			}
			continue
		default:
		}
		break
	}

	cancel()
	if got := tracer.events.len(); got != 0 {
		t.Fatalf("cancel left %d subscribers", got)
	}
	cancel() // idempotent
}
