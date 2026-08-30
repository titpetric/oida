package frontend

import (
	"time"

	"github.com/titpetric/oida/model"
)

// LogCount returns the number of log entries of the detail view trace.
func (p Page) LogCount() int {
	if p.Trace == nil {
		return 0
	}
	return len(p.Trace.Logs)
}

// LogOffset returns when an entry was written, relative to the trace start.
func (p Page) LogOffset(entry model.LogEntry) time.Duration {
	if p.Trace == nil || entry.Time.IsZero() {
		return 0
	}
	return entry.Time.Sub(p.Trace.StartedAt)
}

// LogSpanName returns the name of the span an entry was written under, or
// nothing when the entry was written outside any open span.
func (p Page) LogSpanName(entry model.LogEntry) string {
	if span := p.LogSpan(entry); span != nil {
		return span.Name
	}
	return ""
}

// LogSpan returns the span an entry was written under, or nil when the entry
// was written outside one or the span was dropped.
func (p Page) LogSpan(entry model.LogEntry) *model.Span {
	if p.Trace == nil {
		return nil
	}
	return p.Trace.Spans.Find(entry.SpanID)
}

// logLevelClass returns the CSS class of a log level.
func logLevelClass(level string) string {
	if level == model.LevelError {
		return "log-level log-level-error"
	}
	return "log-level"
}
