package frontend

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/titpetric/oida"
)

// durationText renders a duration at microsecond resolution.
func durationText(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.Round(time.Microsecond).String()
}

// preciseText renders a duration at three significant figures in the unit that
// suits its magnitude, so a table of durations stays the same width and stays
// readable from 300ns to 30s.
func preciseText(d time.Duration) string {
	switch {
	case d <= 0:
		return "0"
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fµs", float64(d)/float64(time.Microsecond))
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// uptimeText renders a long duration the way a person says it. Nobody reads
// "17m2.353374s"; the seconds only matter while the process is young.
func uptimeText(d time.Duration) string {
	switch {
	case d <= 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		hours := int(d.Hours())
		text := plural(hours, "hour")
		if minutes := int(d.Minutes()) - hours*60; minutes > 0 {
			text += " " + plural(minutes, "minute")
		}
		return text
	default:
		days := int(d.Hours()) / 24
		text := plural(days, "day")
		if hours := int(d.Hours()) - days*24; hours > 0 {
			text += " " + plural(hours, "hour")
		}
		return text
	}
}

// plural renders a count with its unit, pluralised.
func plural(count int, unit string) string {
	if count == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(count) + " " + unit + "s"
}

// ageText renders how long ago something happened, as a debugger reads it: how
// stale is this row?
func ageText(at, now time.Time) string {
	if at.IsZero() {
		return ""
	}
	since := now.Sub(at)
	switch {
	case since < 0:
		return "now"
	case since < time.Second:
		return "just now"
	case since < time.Minute:
		return fmt.Sprintf("%ds ago", int(since.Seconds()))
	case since < time.Hour:
		return fmt.Sprintf("%dm ago", int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(since.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(since.Hours()/24))
	}
}

// shortID returns the tail of a trace ID, which is what the eye needs to tell
// two rows apart. The full value stays one copy button away.
func shortID(id string) string {
	if len(id) <= 6 {
		return id
	}
	return id[len(id)-6:]
}

// axisTicks returns the labels of a five step time axis across a duration.
func axisTicks(total time.Duration) []AxisTick {
	if total <= 0 {
		return nil
	}
	ticks := make([]AxisTick, 0, 5)
	for i := range 5 {
		share := float64(i) * 25
		ticks = append(ticks, AxisTick{
			Share: share,
			Label: durationText(time.Duration(float64(total) * share / 100)),
		})
	}
	return ticks
}

// percentText renders a share with two decimals.
func percentText(share float64) string {
	return fmt.Sprintf("%.2f%%", share)
}

// countText renders a float count with one decimal, or a dash for zero.
func countText(value float64) string {
	if value == 0 {
		return "/"
	}
	return fmt.Sprintf("%.1f", value)
}

// timeText renders a wall clock timestamp.
func timeText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05.000")
}

// bytesText renders a byte count in binary units.
func bytesText(n uint64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatUint(n, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for value := n / unit; value >= unit; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// signedBytesText renders a signed byte delta.
func signedBytesText(n int64) string {
	if n < 0 {
		if n == -1<<63 {
			return "-" + bytesText(1<<63)
		}
		return "-" + bytesText(uint64(-n))
	}
	return bytesText(uint64(n))
}

// attributeText renders an attribute value for display.
func attributeText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case error:
		return typed.Error()
	case time.Duration:
		return durationText(typed)
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}

// attributeValue renders one attribute in the unit its key implies: a memory
// limit recorded as 1048576 reads as a megabyte. The exact figure is in the
// JSON.
func attributeValue(attributes oida.Attributes, key string) string {
	if oida.IsBytes(key) {
		if size, ok := attributes.Int64(key); ok {
			return signedBytesText(size)
		}
	}
	return attributeText(attributes[key])
}

// attributeLabel renders an attribute key as a row label: "memory_limit" reads
// as "Memory limit". Unknown keys read the same way.
func attributeLabel(key string) string {
	label := strings.ReplaceAll(key, "_", " ")
	if label == "" {
		return key
	}
	return strings.ToUpper(label[:1]) + label[1:]
}

// spanColumns is the width of the span table, for the row that says there are
// no spans in it. Memory and source are drawn only when recorded.
func spanColumns(page Page) string {
	columns := 5
	if page.Memory.Spans {
		columns++
	}
	if page.Sources {
		columns++
	}
	return strconv.Itoa(columns)
}

// hasSources reports whether any span recorded where it was started.
func hasSources(rows []SpanRow) bool {
	for _, row := range rows {
		if row.SourceText() != "" {
			return true
		}
	}
	return false
}

// memoryText renders a span memory reading, or nothing when the span reported
// none.
func memoryText(row SpanRow) string {
	if !row.HasMemory {
		return ""
	}
	return signedBytesText(row.Memory)
}

// sortedKeys returns the attribute keys in a stable order, so a rendered span
// does not reshuffle between refreshes.
func sortedKeys(attributes oida.Attributes) []string {
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// keyList renders the attribute keys as the summary of the disclosure, so a
// span reads as a name plus what is known about it, not as a wall of chips.
func keyList(attributes oida.Attributes) string {
	return strings.Join(sortedKeys(attributes), ", ")
}

// queryKeys are the attribute names that carry a statement worth showing on the
// span row itself.
var queryKeys = []string{"query", "sql", "statement", "cql"}

// isQueryKey reports whether an attribute holds a statement, which is rendered
// as code rather than as a plain value.
func isQueryKey(key string) bool {
	return slices.Contains(queryKeys, strings.ToLower(key))
}

// shapeTitle summarises where a trace spent its time, for the tooltip on the
// shape bar: "database 62% · external 30%".
func shapeTitle(trace oida.Trace) string {
	segments := Timeline(trace)
	if len(segments) == 0 {
		return durationText(trace.Duration)
	}

	shares := make(map[oida.Kind]float64, len(segments))
	order := make([]oida.Kind, 0, len(segments))
	for _, segment := range segments {
		if _, seen := shares[segment.Kind]; !seen {
			order = append(order, segment.Kind)
		}
		shares[segment.Kind] += segment.Share
	}
	sort.SliceStable(order, func(i, j int) bool { return shares[order[i]] > shares[order[j]] })

	parts := make([]string, 0, len(order))
	for _, kind := range order {
		parts = append(parts, fmt.Sprintf("%s %.0f%%", kind, shares[kind]))
	}
	return durationText(trace.Duration) + ": " + strings.Join(parts, " · ")
}

// matches reports whether a trace contains the query anywhere a person would
// expect to find it: the trace name, its identifier, the request line, and the
// name, source, error and attribute values of every span. Searching for
// "select" has to find the trace that ran the query, not just the ones whose
// route happens to contain the word.
func matches(trace oida.Trace, query string) bool {
	if query == "" {
		return true
	}
	query = strings.ToLower(query)

	if contains(trace.Name, query) || contains(trace.ID, query) || contains(trace.Error, query) {
		return true
	}
	if trace.HTTP != nil {
		if contains(trace.HTTP.URI, query) || contains(trace.HTTP.Route, query) ||
			contains(trace.HTTP.Host, query) || contains(trace.HTTP.RemoteAddress, query) {
			return true
		}
	}
	for key, value := range trace.Attributes {
		if contains(key, query) || contains(attributeText(value), query) {
			return true
		}
	}

	for _, span := range trace.Spans {
		if span == nil {
			continue
		}
		if contains(span.Name, query) || contains(string(span.Kind), query) ||
			contains(span.Error, query) || contains(span.Filename, query) {
			return true
		}
		for key, value := range span.Attributes {
			if contains(key, query) || contains(attributeText(value), query) {
				return true
			}
		}
	}
	return false
}

// contains reports a case insensitive substring match against a lowered query.
func contains(value, lowered string) bool {
	return value != "" && strings.Contains(strings.ToLower(value), lowered)
}

// slowest returns the longest duration in a set of traces, used to scale the
// inline bars so rows compare against each other.
func slowest(traces []oida.Trace) time.Duration {
	var longest time.Duration
	for _, trace := range traces {
		if trace.Duration > longest {
			longest = trace.Duration
		}
	}
	return longest
}

// recent returns at most n traces from the front of a newest-first list.
func recent(traces []oida.Trace, n int) []oida.Trace {
	if n > 0 && len(traces) > n {
		return traces[:n]
	}
	return traces
}

// remoteText returns the peer a trace came from. Work that did not arrive over
// the network came from inside the process, and says so rather than showing an
// empty cell.
func remoteText(trace oida.Trace) string {
	if trace.HTTP == nil || trace.HTTP.RemoteAddress == "" {
		return "internal"
	}
	return trace.HTTP.RemoteAddress
}

// ariaSelected renders the aria-selected attribute value.
func ariaSelected(selected bool) string {
	if selected {
		return "true"
	}
	return "false"
}

// slaClass colours the SLA tile. A number is not an alert until it is one.
func slaClass(sla float64) string {
	switch {
	case sla >= 99.9:
		return "sla ok"
	case sla >= 99:
		return "sla warn"
	default:
		return "sla bad"
	}
}

// traceDot returns the health dot of a trace: green for a delivered response,
// orange for a client error, red for a server error or a recorded failure.
func traceDot(trace oida.Trace) string {
	if trace.HTTP != nil && trace.HTTP.Status > 0 {
		return statusDot(trace.HTTP.Status)
	}
	if trace.Error != "" || trace.State == oida.StateError {
		return "dot bad"
	}
	return "dot ok"
}

// statusDot returns the health dot of an HTTP status code.
func statusDot(status int) string {
	switch {
	case status >= 500:
		return "dot bad"
	case status >= 400:
		return "dot warn"
	case status >= 200:
		return "dot ok"
	default:
		return "dot"
	}
}

// hostDot returns the health dot of a host: red once anything it served failed.
func hostDot(host oida.HostStat) string {
	if host.Errors > 0 {
		return "dot bad"
	}
	return "dot ok"
}

// hostHealth is the tooltip behind the host dot.
func hostHealth(host oida.HostStat) string {
	if host.Errors == 0 {
		return "no failures recorded"
	}
	return plural(int(host.Errors), "failure") + " recorded"
}

// statusClass returns the CSS class for an HTTP status code.
func statusClass(status int) string {
	switch {
	case status >= 500:
		return "status s5xx"
	case status >= 400:
		return "status s4xx"
	case status >= 300:
		return "status s3xx"
	case status >= 200:
		return "status s2xx"
	default:
		return "status"
	}
}

// kindStyle returns the badge style of a span kind. Badges carry dark text on
// the kind colour, which holds contrast in both themes.
func kindStyle(kind oida.Kind) templ.SafeCSS {
	return templ.SafeCSS("background:" + kind.Color() + ";color:#07090c")
}

// kindBackground returns the legend swatch style of a span kind.
func kindBackground(kind oida.Kind) templ.SafeCSS {
	return templ.SafeCSS("background:" + kind.Color())
}

// kindBorder returns the row accent style of a span kind.
func kindBorder(kind oida.Kind) templ.SafeCSS {
	return templ.SafeCSS("border-left:4px solid " + kind.Color())
}

// segmentStyle positions one timeline segment.
func segmentStyle(segment Segment) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("left:%s;width:%s;background:%s",
		cssPercent(segment.OffsetShare), cssPercent(max(segment.Share, 0.2)), segment.Kind.Color()))
}

// rowBarStyle positions the inline duration bar of a span row.
func rowBarStyle(row SpanRow) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("left:%s;width:%s;background:%s",
		cssPercent(row.OffsetShare), cssPercent(max(row.Share, 0.4)), row.Kind.Color()))
}

// offsetStyle positions an axis tick.
func offsetStyle(share float64) templ.SafeCSS {
	return templ.SafeCSS("left:" + cssPercent(share))
}

// treeGlyph returns the tree connector of a span row.
func treeGlyph(row SpanRow) string {
	if row.Last {
		return "└─"
	}
	return "├─"
}

// shareStyle sizes a proportional bar.
func shareStyle(share float64) templ.SafeCSS {
	return templ.SafeCSS("width:" + cssPercent(share))
}

// indentStyle indents a span row by its depth.
func indentStyle(depth int) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("margin-left:%.1fem", float64(min(depth, 12))*1.2))
}

// stateClass returns the CSS class of a scoreboard state.
func stateClass(state oida.State) string {
	return "state-" + strings.ToLower(state.Label())
}

// cssPercent renders a clamped percentage for use in a style attribute.
func cssPercent(share float64) string {
	switch {
	case share != share: // NaN
		return "0%"
	case share < 0:
		share = 0
	case share > 100:
		share = 100
	}
	return strconv.FormatFloat(share, 'f', 4, 64) + "%"
}
