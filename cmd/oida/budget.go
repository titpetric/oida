package main

import (
	"context"
	rand "math/rand/v2"
	"net/http"
	"sync/atomic"

	"github.com/titpetric/oida"
)

// The Go runtime cannot say what one request allocated, so the demo fakes a
// runtime that can: every span reports 10 KiB more in use than the one before
// it, under a limit of one megabyte.
const (
	// memoryLimit is what a demo transaction is allowed to use, in bytes.
	memoryLimit int64 = 1 << 20

	// memoryStep is what every span of it costs, in bytes.
	memoryStep int64 = 10 << 10
)

// budget is the memory one demo transaction has taken so far. Every method is
// safe to call on a nil budget.
type budget struct {
	used atomic.Int64
}

// budgetKey carries the budget of the current transaction.
type budgetKey struct{}

// withBudget starts a budget for the trace in ctx, records the limit that trace
// runs under, and returns the context its spans charge against.
func withBudget(ctx context.Context) context.Context {
	oida.TraceFromContext(ctx).SetAttribute(oida.AttrMemoryLimit, memoryLimit)
	return context.WithValue(ctx, budgetKey{}, &budget{})
}

// budgetFrom returns the budget in ctx, or nil.
func budgetFrom(ctx context.Context) *budget {
	value, _ := ctx.Value(budgetKey{}).(*budget)
	return value
}

// charge takes one span worth of memory and returns what is now in use.
func (b *budget) charge() int64 {
	if b == nil {
		return 0
	}
	return b.used.Add(memoryStep)
}

// total returns what the transaction has in use.
func (b *budget) total() int64 {
	if b == nil {
		return 0
	}
	return b.used.Load()
}

// endSpan closes a span, recording what the transaction had in use when it
// finished. The demo uses it instead of a bare span.End().
//
// The memory is charged either way and the reading is taken two times out of
// three, because instrumentation that reports on every span is the easy case.
// A gap in the column is a span that allocated without saying so, and the next
// reading steps by more than one span's worth.
func endSpan(ctx context.Context, span *oida.Span) {
	b := budgetFrom(ctx)
	if b == nil {
		span.End()
		return
	}
	used := b.charge()
	if rand.IntN(3) > 0 {
		span.SetAttribute(oida.AttrMemoryUsage, used)
	}
	span.End()
}

// do is oida.Do with the budget attached. oida.Do ends the span itself, which
// leaves nowhere to record the reading.
func do(ctx context.Context, name string, fn func(context.Context) error, kind ...oida.Kind) error {
	ctx, span := oida.Start(ctx, name, kind...)
	err := fn(ctx)
	span.RecordError(err)
	endSpan(ctx, span)
	return err
}

// closeBudget records the memory the transaction ended with: one last step for
// the span wrapping the request, and the total on the trace.
func closeBudget(ctx context.Context) {
	b := budgetFrom(ctx)
	if b == nil {
		return
	}
	if span := oida.SpanFromContext(ctx); span != nil {
		span.SetAttribute(oida.AttrMemoryUsage, b.charge())
	}
	oida.TraceFromContext(ctx).SetAttribute(oida.AttrMemoryUsage, b.total())
}

// trackMemory gives every traced request a budget and closes it once the
// response is written. Ignored paths carry no trace, and recording on a nil
// trace does nothing.
func trackMemory(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := withBudget(r.Context())
		defer closeBudget(ctx)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
