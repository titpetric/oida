package model

// logsEnabled reports whether the trace captures log entries. The flag is set
// once when the trace is created and never written after, so it is read
// without the lock.
func (t *Trace) logsEnabled() bool {
	return t != nil && t.captureLogs
}

// appendLog records one entry, honouring the log limit. A spanID of zero means
// the innermost open span at the time of the call. Entries share the span
// limit of the trace: excess entries are counted in DroppedLogs, never
// printed.
func (t *Trace) appendLog(level string, spanID int, message string, args []any) {
	if t == nil {
		return
	}
	t.lock()
	defer t.unlock()

	if t.finished {
		return
	}
	if t.maxSpans > 0 && len(t.Logs) >= t.maxSpans {
		t.DroppedLogs++
		return
	}

	if spanID == 0 {
		if span := t.openSpan(); span != nil {
			spanID = span.ID
		}
	}
	entry := LogEntry{
		Time:       t.time(),
		Level:      level,
		Message:    message,
		SpanID:     spanID,
		RequestID:  t.ID,
		Attributes: logAttributes(args),
	}
	t.Logs = append(t.Logs, entry)
	t.UpdatedAt = entry.Time
}

// openSpan returns the innermost open span: the most recently started span
// that has not ended. Callers hold the trace lock.
func (t *Trace) openSpan() *Span {
	for i := len(t.Spans) - 1; i >= 0; i-- {
		if span := t.Spans[i]; span != nil && !span.Ended() {
			return span
		}
	}
	return nil
}
