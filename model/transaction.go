package model

import (
	"fmt"
	"time"
)

// The bounds a posted transaction is held to, so an untrusted client cannot
// spend the recorder's memory on one request.
const (
	maxTransactionName  = 256
	maxTransactionSpans = 256
	maxTransactionAttrs = 64

	// maxClockSkew is how far a client start may sit from the recorder's own
	// clock. A browser clock is the user's to set, and a transaction dated
	// next year would sort above every recorded trace forever.
	maxClockSkew = 24 * time.Hour
)

// Transaction is one unit of work a client recorded and posted: a browser
// pageload, a user action, or a process reporting work of its own. It is the
// wire form of a Trace, with times the client measured rather than times the
// recorder took.
type Transaction struct {
	// TraceID is the 32 hex digit trace the transaction belongs to. Empty
	// opens a trace of its own.
	TraceID string `json:"trace_id,omitempty"`

	// SpanID is the 16 hex digit span the client already named its work
	// under, so the calls it made while the work ran arrive naming the same
	// parent. Empty takes the identifier the recorder derives.
	SpanID string `json:"span_id,omitempty"`

	// ParentID is the 16 hex digit span the transaction nests under.
	ParentID string `json:"parent_id,omitempty"`

	// TraceState is the tracestate the client carried, kept untouched.
	TraceState string `json:"tracestate,omitempty"`

	// Name is what the transaction measured, which the dashboard lists it
	// under.
	Name string `json:"name"`

	// Kind classifies the work, defaulting to KindBrowser.
	Kind Kind `json:"kind,omitempty"`

	// Service names the client the transaction came from, defaulting to the
	// recorder's own service name.
	Service string `json:"service,omitempty"`

	// StartedAt is the epoch millisecond the work began on the client clock.
	StartedAt float64 `json:"started_at"`

	// Duration is how long the work took, in milliseconds.
	Duration float64 `json:"duration_ms"`

	// Attributes is what the client recorded about the transaction as a
	// whole: the URL of a pageload, the element of an action.
	Attributes Attributes `json:"attributes,omitempty"`

	// HTTP is the request the transaction is about, which puts a browser
	// transaction on the host it was recorded for. The recorder overwrites
	// the fields it knows better than the client.
	HTTP *HTTPInfo `json:"http,omitempty"`

	// Spans are the operations inside the transaction, each timed from its
	// start.
	Spans []TransactionSpan `json:"spans,omitempty"`

	// Error is the failure the client recorded, empty for work that
	// succeeded.
	Error string `json:"error,omitempty"`
}

// TransactionSpan is one operation inside a Transaction, offset from the
// transaction start so a client needs no second clock.
type TransactionSpan struct {
	// Name is what the span measured.
	Name string `json:"name"`

	// Kind classifies the work, defaulting to the transaction's kind.
	Kind Kind `json:"kind,omitempty"`

	// Start is the offset from the transaction start, in milliseconds.
	Start float64 `json:"start_ms"`

	// Duration is how long the operation took, in milliseconds.
	Duration float64 `json:"duration_ms"`

	// Parent is the one based index of the span in Spans this one nests
	// under. Zero nests it under the transaction's own span.
	Parent int `json:"parent,omitempty"`

	// Attributes is what the client recorded about the operation.
	Attributes Attributes `json:"attributes,omitempty"`

	// Error is the failure the client recorded on the operation.
	Error string `json:"error,omitempty"`
}

// Validate reports whether the transaction is one the recorder will record.
// Every failure wraps ErrInvalidTransaction.
func (tx Transaction) Validate() error {
	switch {
	case tx.Name == "":
		return fmt.Errorf("%w: a transaction needs a name", ErrInvalidTransaction)
	case len(tx.Name) > maxTransactionName:
		return fmt.Errorf("%w: the name runs past %d bytes", ErrInvalidTransaction, maxTransactionName)
	case tx.Duration < 0 || tx.StartedAt < 0:
		return fmt.Errorf("%w: the times must not be negative", ErrInvalidTransaction)
	case len(tx.Spans) > maxTransactionSpans:
		return fmt.Errorf("%w: %d spans is past the %d one transaction may carry", ErrInvalidTransaction, len(tx.Spans), maxTransactionSpans)
	case tx.TraceID != "" && !ValidTraceID(tx.TraceID):
		return fmt.Errorf("%w: %q is no trace identifier", ErrInvalidTransaction, tx.TraceID)
	case tx.ParentID != "" && !ValidSpanID(tx.ParentID):
		return fmt.Errorf("%w: %q is no span identifier", ErrInvalidTransaction, tx.ParentID)
	case tx.SpanID != "" && !ValidSpanID(tx.SpanID):
		return fmt.Errorf("%w: %q is no span identifier", ErrInvalidTransaction, tx.SpanID)
	case tx.SpanID != "" && tx.SpanID == tx.ParentID:
		return fmt.Errorf("%w: a transaction cannot nest under itself", ErrInvalidTransaction)
	}
	for i, span := range tx.Spans {
		switch {
		case span.Name == "" || len(span.Name) > maxTransactionName:
			return fmt.Errorf("%w: span %d needs a name of at most %d bytes", ErrInvalidTransaction, i+1, maxTransactionName)
		case span.Duration < 0 || span.Start < 0:
			return fmt.Errorf("%w: the times of span %d must not be negative", ErrInvalidTransaction, i+1)
		case span.Parent < 0 || span.Parent > i:
			return fmt.Errorf("%w: span %d nests under %d, which is not a span before it", ErrInvalidTransaction, i+1, span.Parent)
		}
	}
	return nil
}

// Trace returns the finished trace of the transaction, under the given ID.
//
// It joins the trace the client named, or opens one keyed on the hex form of
// its own ID. A start further from now than maxClockSkew gives way to the
// recorder's clock, which a wrong client clock would otherwise reorder.
func (tx Transaction) Trace(id string, opts TraceOptions) *Trace {
	now := opts.now()
	started := time.UnixMilli(int64(tx.StartedAt)).UTC()
	if tx.StartedAt == 0 || started.Sub(now).Abs() > maxClockSkew {
		started = now.Add(-milliseconds(tx.Duration))
	}

	trace := &Trace{
		ID:           id,
		TraceID:      firstString(tx.TraceID, TraceIDHex(id)),
		SpanID:       firstString(tx.SpanID, SpanIDFor(id, rootSpan)),
		ParentSpanID: tx.ParentID,
		TraceState:   CleanTraceState(tx.TraceState),
		Name:         tx.Name,
		Service:      firstString(tx.Service, opts.Service),
		State:        StateProcessing,
		StartedAt:    started,
		UpdatedAt:    started.Add(milliseconds(tx.Duration)),
		Duration:     milliseconds(tx.Duration),
		ErrorText:    tx.Error,
		Attributes:   capped(tx.Attributes),
		finished:     true,
	}
	if tx.Error != "" {
		trace.State = StateError
	}
	trace.stateTime[stateIndex(trace.State)] = trace.Duration
	trace.Spans = tx.spans(started, trace)
	if tx.HTTP != nil {
		http := *tx.HTTP
		trace.HTTP = &http
	}
	return trace
}

// spans returns the span list of a recorded transaction: its own span first,
// then the client's, each below the span its index names.
func (tx Transaction) spans(started time.Time, trace *Trace) Spans {
	kind := tx.Kind
	if kind == "" {
		kind = KindBrowser
	}
	spans := make(Spans, 0, len(tx.Spans)+1)
	spans = append(spans, &Span{
		ID: 1, TraceID: trace.ID, Name: tx.Name, Kind: kind,
		StartedAt: started, Duration: milliseconds(tx.Duration),
		Attributes: capped(tx.Attributes), ErrorText: tx.Error, ended: true,
	})
	for i, span := range tx.Spans {
		own := span.Kind
		if own == "" {
			own = kind
		}
		spans = append(spans, &Span{
			ID: i + 2, ParentID: span.Parent + 1, TraceID: trace.ID,
			Name: span.Name, Kind: own, Depth: tx.depth(i),
			StartedAt: started.Add(milliseconds(span.Start)), Duration: milliseconds(span.Duration),
			Attributes: capped(span.Attributes), ErrorText: span.Error, ended: true,
		})
	}
	return spans
}

// depth returns how deep a client span sits below the transaction's own,
// counting the chain of parents Validate proved finite.
func (tx Transaction) depth(i int) int {
	depth := 1
	for parent := tx.Spans[i].Parent; parent > 0; parent = tx.Spans[parent-1].Parent {
		depth++
	}
	return depth
}

// milliseconds returns a client duration as a Duration, clamped at zero.
func milliseconds(value float64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value * float64(time.Millisecond))
}

// capped returns at most maxTransactionAttrs of the attributes a client
// recorded, so one transaction cannot carry an unbounded map.
func capped(attributes Attributes) Attributes {
	if len(attributes) <= maxTransactionAttrs {
		return attributes
	}
	out := make(Attributes, maxTransactionAttrs)
	for key, value := range attributes {
		if len(out) == maxTransactionAttrs {
			break
		}
		out[key] = value
	}
	return out
}

// firstString returns the first value that is not empty.
func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
