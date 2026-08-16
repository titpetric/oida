package oida

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// format is the response representation selected for a front end request.
type format int

const (
	formatHTML format = iota
	formatJSON
	formatText
)

// negotiate selects the response representation. The format query parameter
// overrides the Accept header, which is what tests use.
func negotiate(r *http.Request) format {
	switch r.URL.Query().Get("format") {
	case "json":
		return formatJSON
	case "text":
		return formatText
	case "html":
		return formatHTML
	}

	accept := strings.ToLower(r.Header.Get("Accept"))
	switch {
	case strings.Contains(accept, "application/json"), strings.Contains(accept, "text/json"):
		return formatJSON
	case strings.Contains(accept, "text/plain"):
		return formatText
	case strings.HasPrefix(strings.ToLower(r.UserAgent()), "curl/"):
		return formatText
	default:
		return formatHTML
	}
}

// writeText renders a page as fixed width plain text, for terminals and curl.
func writeText(w http.ResponseWriter, page Page) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writeHeaderText(w, page.Snapshot)

	switch page.View {
	case ViewHosts:
		writeHostsText(w, page.Snapshot.Statistics)
	case ViewLive:
		writeTraceTableText(w, "Traces in flight", page.Snapshot.Live)
		writeStateText(w, page.Snapshot.StateTime)
	case ViewStats:
		writeStatsText(w, page.Snapshot.Statistics)
	case ViewDetail:
		writeDetailText(w, page)
	default:
		writeTraceTableText(w, "Recorded traces", page.Snapshot.Log)
	}
}

// writeHeaderText renders the process summary.
func writeHeaderText(w io.Writer, s Snapshot) {
	service := s.Service
	if service == "" {
		service = "unnamed service"
	}
	fmt.Fprintf(w, "oida · %s\n", service)
	fmt.Fprintf(w, "Uptime: %s  PID: %d  Go: %s  GOMAXPROCS: %d  Goroutines: %d\n",
		uptimeText(s.Uptime), s.PID, s.GoVersion, s.GOMAXPROCS, s.Goroutines)
	fmt.Fprintf(w, "Requests: %d seen, %d recorded, %d dropped, %d in flight\n",
		s.Total, s.Sampled, s.Dropped, s.Active)
	fmt.Fprintf(w, "SLA: %.4f%% over %d recorded traces, %d failed\n",
		s.SLA, s.Sampled, s.Errors)
	fmt.Fprintf(w, "Heap: %s / next GC %s  GC: %d cycles, %.2f%% CPU\n",
		bytesText(s.Memory.HeapAlloc), bytesText(s.Memory.NextGC), s.Memory.NumGC, s.Memory.GCCPUFraction*100)
	fmt.Fprintf(w, "Pool estimate: %d samples, %s per trace, %d before GC, %d within limit\n\n",
		s.Pool.Samples, bytesText(s.Pool.AverageAllocatedBytes), s.Pool.BeforeNextGC, s.Pool.WithinMemoryLimit)
}

// writeTraceTableText renders a list of traces.
func writeTraceTableText(w io.Writer, title string, traces []Trace) {
	fmt.Fprintf(w, "%s (%d):\n", title, len(traces))
	fmt.Fprintln(w, "TRACE-ID                   S  TIME          NAME                                     STATUS  DURATION      SPANS  BYTES      HEAP       ALLOCATED  REMOTE")
	for _, trace := range traces {
		var status int
		var bytes int64
		remote := ""
		if trace.HTTP != nil {
			status, bytes, remote = trace.HTTP.Status, trace.HTTP.ResponseBytes, trace.HTTP.RemoteAddress
		}
		fmt.Fprintf(w, "%-26s %-2s %-13s %-40s %-7d %-13s %-6d %-10d %-10s %-10s %s\n",
			trace.ID, trace.State, timeText(trace.StartedAt), truncate(trace.Name, 40), status,
			durationText(trace.Duration), len(trace.Spans), bytes,
			signedBytesText(trace.Memory.HeapDelta), bytesText(trace.Memory.AllocatedBytes), remote)
	}
	fmt.Fprintln(w)
}

// writeStateText renders the lifetime time in state.
func writeStateText(w io.Writer, durations []StateDuration) {
	fmt.Fprintln(w, "Lifetime trace state time:")
	for _, state := range durations {
		fmt.Fprintf(w, "%-12s (%s) %-13s %6.2f%%\n", state.Label, state.State, durationText(state.Duration), state.Share)
	}
	fmt.Fprintln(w)
}

// writeHostsText renders the per host traffic overview.
func writeHostsText(w io.Writer, stats Stats) {
	fmt.Fprintf(w, "Hosts (%d):\n", len(stats.Hosts))
	fmt.Fprintln(w, "REQUESTS  RECORDED  ERRORS  ROUTES  SHARE    AVG TIME      MAX TIME      HOST")
	for _, host := range stats.Hosts {
		fmt.Fprintf(w, "%-9d %-9d %-7d %-7d %6.2f%%  %-13s %-13s %s\n",
			host.Requests, host.Traces, host.Errors, host.Routes, host.Share,
			durationText(host.AverageDuration), durationText(host.MaxDuration), host.Host)
	}
	fmt.Fprintln(w)
}

// writeStatsText renders the rolling statistics.
func writeStatsText(w io.Writer, stats Stats) {
	fmt.Fprintf(w, "Top %d of %d traces in a %d trace window:\n", stats.TopLimit, stats.WindowSize, stats.WindowLimit)
	fmt.Fprintln(w, "SHARE    COUNT  ERRORS  AVG TIME      MAX TIME      AVG BYTES  AVG ALLOC  AVG SPANS  NAME")
	for _, stat := range stats.Top {
		fmt.Fprintf(w, "%6.2f%%  %-5d  %-6d  %-13s %-13s %-10s %-10s %-10s %s\n",
			stat.Share, stat.Count, stat.Errors, durationText(stat.AverageDuration), durationText(stat.MaxDuration),
			bytesText(stat.AverageResponseBytes), bytesText(stat.AverageAllocatedBytes),
			countText(stat.AverageSpans), stat.Name)
	}
	fmt.Fprintln(w)
}

// writeDetailText renders one trace with its spans.
func writeDetailText(w io.Writer, page Page) {
	trace := page.Trace
	if trace == nil {
		return
	}

	fmt.Fprintf(w, "%s\nTrace %s · %s · %s\n", trace.Name, trace.ID, trace.State.Label(), durationText(trace.Duration))
	if trace.HTTP != nil {
		fmt.Fprintf(w, "HTTP %d · %s · %s · %s\n", trace.HTTP.Status, trace.HTTP.Method, trace.HTTP.URI, trace.HTTP.RemoteAddress)
	}
	if trace.Error != "" {
		fmt.Fprintf(w, "Error: %s\n", trace.Error)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Timeline:")
	for _, segment := range page.Segments {
		fmt.Fprintf(w, "%-12s at %-13s %-13s %6.2f%%\n",
			segment.Kind, durationText(segment.Offset), durationText(segment.Duration), segment.Share)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "KIND          OFFSET        DURATION        SOURCE                         NAME")
	for _, row := range page.Rows {
		duration := preciseText(row.Duration)
		if row.Open {
			duration = "(open)"
		}
		fmt.Fprintf(w, "%-13s %-13s %-15s %-30s %s%s\n",
			row.Kind, durationText(row.Offset), duration, sourceText(row.Filename, row.Line),
			strings.Repeat("  ", min(row.Depth, 12)), row.Name)
		for key, value := range row.Attributes {
			fmt.Fprintf(w, "%s%s = %s\n", strings.Repeat(" ", 74), key, attributeText(value))
		}
		if row.Error != "" {
			fmt.Fprintf(w, "%s! %s\n", strings.Repeat(" ", 74), row.Error)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Memory: heap %s · allocated %s · %d allocations · %d GC cycles / %s\n",
		signedBytesText(trace.Memory.HeapDelta), bytesText(trace.Memory.AllocatedBytes),
		trace.Memory.Allocations, trace.Memory.GCCycles, durationText(trace.Memory.GCPause))
	fmt.Fprintln(w, "Memory deltas are process wide and overlap when traces run concurrently.")
}

// truncate shortens a value to width, marking the cut with an ellipsis.
func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "…"
}
