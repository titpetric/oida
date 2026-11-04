package model

import (
	"context"
	"time"
)

// StartDebug is Start for a debug span: one the trace keeps only when it ran
// at least Options.DebugSpanMinDuration. Use it where the call is instrumented
// because it might be slow, not because it is interesting.
func StartDebug(ctx context.Context, name string, kind ...Kind) (context.Context, *Span) {
	ctx, span := Start(ctx, name, kind...)
	span.SetDebug()
	return ctx, span
}

// StartDebugSpan is StartSpan for a debug span: one the trace keeps only when
// it ran at least Options.DebugSpanMinDuration.
func StartDebugSpan(ctx context.Context, name string, kind ...Kind) *Span {
	span := StartSpan(ctx, name, kind...)
	span.SetDebug()
	return span
}

// SetDebug marks the span as debug detail, which subjects it to
// Options.DebugSpanMinDuration: ending shorter than that drops it from the
// trace. Mark before the span ends, which is where the filter runs.
func (s *Span) SetDebug() {
	if s == nil {
		return
	}
	s.lock()
	defer s.unlock()
	s.Debug = true
}

// filterDebugSpan drops a debug span that ran shorter than the configured
// minimum, counting it in DroppedSpans. Span.End calls it as it records the
// duration, which is the first moment the answer exists.
//
// A trace that has finished keeps what it holds: Finish ends every span still
// open, and removing entries from under the list it is walking would skip the
// neighbours of what went.
func (t *Trace) filterDebugSpan(span *Span, duration time.Duration) {
	if t == nil || span == nil || t.debugMinSpan <= 0 || duration >= t.debugMinSpan {
		return
	}
	t.lock()
	defer t.unlock()
	if t.finished {
		return
	}
	// Backwards: a leaf span is the last entry at the moment it ends, which
	// is the case the filter exists for, and nothing shifts for it. A span
	// whose children outlived it shifts them one place left.
	for i := len(t.Spans) - 1; i >= 0; i-- {
		if t.Spans[i] != span {
			continue
		}
		t.Spans = append(t.Spans[:i], t.Spans[i+1:]...)
		t.DroppedSpans++
		return
	}
}
