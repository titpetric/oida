package view

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
)

// MemoryPoint is one reading on the memory graph: the memory a span reported
// in use when it finished, placed at the moment it finished.
type MemoryPoint struct {
	// Name is the span the reading came from, and Offset when the span
	// finished, relative to the start of the trace.
	Name   string        `json:"name"`
	Offset time.Duration `json:"offset_ns"`
	Bytes  int64         `json:"bytes"`

	// X is the offset as a percentage of the trace width, and Y the reading
	// as a percentage down from the top of the plot, both ready to draw.
	X float64 `json:"x_percent"`
	Y float64 `json:"y_percent"`
}

// MemorySeries is the data behind the memory graph: the readings the spans
// reported, ordered by when they were taken.
type MemorySeries struct {
	Points []MemoryPoint `json:"points"`

	// Ceiling is the top of the value scale, in bytes: the limit when it sits
	// close enough to the readings to share a scale with them, the peak
	// reading otherwise.
	Ceiling int64 `json:"ceiling_bytes"`

	// Limited reports whether the limit is drawn as a reference line, and
	// LimitY where the line sits, as a percentage down from the top.
	Limited bool    `json:"-"`
	LimitY  float64 `json:"-"`
}

// Last returns the newest reading, which is the one the graph labels.
func (s MemorySeries) Last() MemoryPoint {
	if len(s.Points) == 0 {
		return MemoryPoint{}
	}
	return s.Points[len(s.Points)-1]
}

// memoryHeadroom is the empty share at the top of the memory plot, so the
// largest reading stays clear of the frame.
const memoryHeadroom = 8.0

// MemorySeries returns the readings behind the memory graph: one point per
// span that reported memory_usage, placed where the span finished and ordered
// by it. A reading is the memory in use at that moment, so the line holds flat
// between readings and moves where one was taken.
//
// The scale runs to the limit when the limit is within reach of the readings,
// and to the peak reading when it is not: a transaction far under its limit
// still draws its own shape rather than a sliver along the floor. Past the
// reach, the head line above the graph carries the limit in words instead.
func (p Page) MemorySeries() MemorySeries {
	series := MemorySeries{}
	if !p.Memory.Spans || p.Memory.Peak <= 0 {
		return series
	}

	series.Ceiling = p.Memory.Peak
	if p.Memory.HasLimit && p.Memory.Limit > 0 && p.Memory.Limit <= 2*p.Memory.Peak {
		series.Ceiling = max(p.Memory.Peak, p.Memory.Limit)
		series.Limited = true
	}

	scale := (100 - memoryHeadroom) / float64(series.Ceiling)
	if series.Limited {
		series.LimitY = 100 - float64(p.Memory.Limit)*scale
	}

	for _, row := range p.Rows {
		if !row.HasMemory {
			continue
		}
		series.Points = append(series.Points, MemoryPoint{
			Name:   row.Name,
			Offset: row.Offset + row.Duration,
			Bytes:  row.Memory,
			X:      min(row.OffsetShare+row.Share, 100),
			Y:      100 - float64(max(row.Memory, 0))*scale,
		})
	}
	sort.SliceStable(series.Points, func(i, j int) bool {
		return series.Points[i].Offset < series.Points[j].Offset
	})
	return series
}

// linePath draws the step line of the memory graph: flat from one reading to
// the next, then straight to the new level at the moment the reading was
// taken, which is the place the eye is after. The line runs the whole trace:
// it opens at the first reading's level, since that is the earliest the trace
// knows, and holds the last level to the end.
func (s MemorySeries) linePath() string {
	if len(s.Points) == 0 {
		return ""
	}
	var path strings.Builder
	path.WriteString("M 0 " + svgNumber(s.Points[0].Y))
	for _, point := range s.Points {
		path.WriteString(" H " + svgNumber(point.X) + " V " + svgNumber(point.Y))
	}
	path.WriteString(" H 100")
	return path.String()
}

// areaPath closes the step line down to the baseline, for the wash under the
// line.
func (s MemorySeries) areaPath() string {
	if len(s.Points) == 0 {
		return ""
	}
	var path strings.Builder
	path.WriteString("M 0 100 V " + svgNumber(s.Points[0].Y))
	for _, point := range s.Points {
		path.WriteString(" H " + svgNumber(point.X) + " V " + svgNumber(point.Y))
	}
	path.WriteString(" H 100 V 100 Z")
	return path.String()
}

// hitX is the left edge of the hover target of one reading, which runs from
// the previous reading to this one: the stretch of the line it explains.
func (s MemorySeries) hitX(i int) string {
	if i == 0 {
		return "0"
	}
	return svgNumber(s.Points[i-1].X)
}

// hitWidth is the width of the hover target of one reading. The last stretch
// runs to the end of the trace, where the line holds the final level.
func (s MemorySeries) hitWidth(i int) string {
	from := 0.0
	if i > 0 {
		from = s.Points[i-1].X
	}
	to := s.Points[i].X
	if i == len(s.Points)-1 {
		to = 100
	}
	return svgNumber(max(to-from, 0))
}

// at is the memory in use at a share of the trace width: the newest reading
// taken by then, and the first reading before any was, which is how the line
// itself opens.
func (s MemorySeries) at(share float64) int64 {
	if len(s.Points) == 0 {
		return 0
	}
	bytes := s.Points[0].Bytes
	for _, point := range s.Points {
		if point.X > share {
			break
		}
		bytes = point.Bytes
	}
	return bytes
}

// limitY is where the reference line sits, as a plot coordinate.
func (s MemorySeries) limitY() string {
	return svgNumber(s.LimitY)
}

// limitStyle places the label of the limit line at its height.
func (s MemorySeries) limitStyle() templ.SafeCSS {
	return templ.SafeCSS("top:" + cssPercent(s.LimitY))
}

// title is the native tooltip of one reading.
func (p MemoryPoint) title() string {
	return p.Name + ": " + signedBytesText(p.Bytes) + " at " + durationText(p.Offset)
}

// overlayStyle places a marker of the graph at a reading.
func (p MemoryPoint) overlayStyle() templ.SafeCSS {
	return templ.SafeCSS("left:" + cssPercent(p.X) + ";top:" + cssPercent(p.Y))
}

// summary reads the graph in one line: the peak the spans reached, and the
// limit the transaction ran under when it recorded one.
func (b MemoryBudget) summary() string {
	summary := "peak " + signedBytesText(b.Peak)
	if b.HasLimit && b.Limit > 0 {
		summary += " of a " + signedBytesText(b.Limit) + " limit"
	}
	return summary
}

// svgNumber renders a coordinate of the memory plot, which draws in a
// 100 by 100 viewBox, so a percentage is a coordinate.
func svgNumber(value float64) string {
	switch {
	case value != value: // NaN
		return "0"
	case value < 0:
		value = 0
	case value > 100:
		value = 100
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}
