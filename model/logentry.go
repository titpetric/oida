package model

import (
	"fmt"
	"maps"
	"strings"
	"time"
)

// Log levels recorded by Trace.Info and Trace.Error. The set is closed: a log
// line either informs or reports a failure, and anything richer belongs in
// attributes.
const (
	LevelInfo  = "info"
	LevelError = "error"
)

// LogEntry is one log line recorded on a trace. Entries live in one slice on
// the trace, in write order; SpanID links each entry to the span that was
// active when it was written, and is zero for an entry written outside any
// open span.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	SpanID  int       `json:"span_id,omitempty"`

	// RequestID is the ID of the trace the entry was recorded on, the value
	// the Request-Id header carries. Every entry of a trace shares it, and it
	// keeps an entry attributable once the log is read apart from its trace.
	RequestID string `json:"request_id,omitempty"`

	// Attributes carry the slog-style key/value arguments of the call. The
	// message is stored verbatim, never formatted.
	Attributes Attributes `json:"attributes,omitempty"`
}

// clone returns a deep copy of the entry, safe to hand to snapshot consumers.
func (e LogEntry) clone() LogEntry {
	copied := e
	if e.Attributes != nil {
		copied.Attributes = maps.Clone(e.Attributes)
	}
	return copied
}

// formatLogText renders a message and its slog-style arguments as one line,
// "message key=value key=value", in argument order. It is what Error records
// through RecordError when log capture is disabled, so the text survives even
// though no entry is written.
func formatLogText(message string, args []any) string {
	if len(args) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString(message)
	for i := 0; i < len(args); i += 2 {
		b.WriteByte(' ')
		if i+1 >= len(args) {
			fmt.Fprintf(&b, "!BADKEY=%v", args[i])
			break
		}
		fmt.Fprintf(&b, "%v=%v", args[i], args[i+1])
	}
	return b.String()
}

// logAttributes converts slog-style variadic arguments into attributes: args
// are consumed in pairs, the first of each pair as the key and the second as
// the value. A key that is not a string is rendered with fmt.Sprint, and a
// dangling final argument is kept under "!BADKEY", the way log/slog keeps it.
func logAttributes(args []any) Attributes {
	if len(args) == 0 {
		return nil
	}
	attributes := make(Attributes, (len(args)+1)/2)
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			attributes["!BADKEY"] = args[i]
			break
		}
		key, ok := args[i].(string)
		if !ok {
			key = fmt.Sprint(args[i])
		}
		attributes[key] = args[i+1]
	}
	return attributes
}
