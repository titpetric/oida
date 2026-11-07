package oida

import (
	"context"

	"github.com/titpetric/oida/internal"
	"github.com/titpetric/oida/model"
)

// Collection is the batch of transactions a client posts, and Collected the
// answer to one of them: the trace context the client nests its next
// transaction and its next call below. Both live in the model package so the
// front end can decode them.
type (
	Collection = model.Collection
	Collected  = model.Collected
	Receipt    = model.Receipt

	// Transaction is one unit of work a client measured and posted.
	Transaction = model.Transaction

	// TransactionSpan is one operation inside a Transaction.
	TransactionSpan = model.TransactionSpan
)

// Record retains a transaction a client measured and posted, and returns the
// trace context naming it.
//
// The client keeps its own times and its own identifiers: a browser opens the
// trace, names the span its calls will carry, and the recorder records what
// it reports. Nothing else in this package trusts a client that way, so the
// route in front of this is off until a deployment turns it on.
func (t *Tracer) Record(ctx context.Context, tx Transaction) (Collected, error) {
	if t == nil || !t.Enabled() {
		return Collected{}, ErrDisabled
	}
	if err := tx.Validate(); err != nil {
		return Collected{}, err
	}
	id, err := model.NewID(internal.ClockNow(t.opts))
	if err != nil {
		return Collected{}, err
	}

	trace := tx.Trace(id, internal.TraceOptionsFor(t.opts))
	t.count(trace)
	if err := t.storage.Save(ctx, trace); err != nil {
		t.onError(err)
		return Collected{}, err
	}
	t.events.Notify()
	return model.NewCollected(id, trace.SpanContext(1)), nil
}

// count records a finished transaction in the lifetime counters, the way
// begin and Finish do between them for a trace the tracer measured itself.
func (t *Tracer) count(trace *Trace) {
	durations := trace.StateTimes()

	t.mu.Lock()
	defer t.mu.Unlock()
	t.total++
	t.sampled++
	t.requests[model.TraceHost(trace)]++
	if trace.Failed() {
		t.failed++
	}
	for i, duration := range durations {
		if duration != 0 {
			t.stateTime[model.States()[i]] += duration
		}
	}
}
