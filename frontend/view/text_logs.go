package view

import (
	"fmt"
	"io"
	"strings"
)

// writeLogsText renders the log lines a trace recorded, for the plain text
// detail view. A trace with no logs prints nothing.
func writeLogsText(w io.Writer, page Page) {
	trace := page.Trace
	if trace == nil || len(trace.Logs) == 0 {
		return
	}

	fmt.Fprintln(w, "LEVEL   OFFSET        SPAN                           MESSAGE")
	for _, entry := range trace.Logs {
		parts := make([]string, 0, 1+len(entry.Attributes))
		parts = append(parts, entry.Message)
		for _, key := range sortedKeys(entry.Attributes) {
			parts = append(parts, key+"="+attributeValue(entry.Attributes, key))
		}
		fmt.Fprintf(w, "%-7s %-13s %-30s %s\n",
			entry.Level, durationText(page.LogOffset(entry)),
			truncate(page.LogSpanName(entry), 30), strings.Join(parts, " "))
	}
	fmt.Fprintln(w)

	if trace.DroppedLogs > 0 {
		fmt.Fprintf(w, "%d log entries were dropped: this trace hit Options.MaxSpansPerTrace.\n\n", trace.DroppedLogs)
	}
}
