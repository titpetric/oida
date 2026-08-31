package view

import (
	"time"

	"github.com/titpetric/oida/model"
)

// FeedRows is the number of traces the live feed holds, which the view states
// under the table and the handler cuts the feed to.
const FeedRows = 25

// View identifies one rendered page of the debug front end.
type View string

// The pages the front end serves. Page.URL builds a link to any of them.
const (
	// ViewHosts is the landing page: which domains this process serves, and
	// how much traffic each one carries. Everything else is a drill down.
	ViewHosts  View = "hosts"
	ViewList   View = "list"
	ViewLive   View = "live"
	ViewStats  View = "stats"
	ViewDetail View = "detail"

	// ViewLogin is the sign in screen, rendered when authentication is
	// configured and the request carries no valid session or bearer token. It
	// draws none of the recorded data.
	ViewLogin View = "login"
)

// listKinds are the kinds offered in the front end filter, in display order.
var listKinds = []model.Kind{
	model.KindHTTP,
	model.KindDatabase,
	model.KindExternal,
	model.KindTemplate,
	model.KindCache,
	model.KindQueue,
	model.KindInternal,
}

// Segment is one point-in-time region of a trace where a span kind was the
// innermost active span.
type Segment struct {
	Kind        model.Kind    `json:"kind"`
	Offset      time.Duration `json:"offset_ns"`
	Duration    time.Duration `json:"duration_ns"`
	OffsetShare float64       `json:"offset_percent"`
	Share       float64       `json:"share_percent"`
}

// SelectOption is one choice in a SelectField.
type SelectOption struct {
	Value string `json:"value"`
	Label string `json:"label"`

	// Note is the secondary text on the right of the option, such as a count.
	Note string `json:"note,omitempty"`
}

// SelectField is a dropdown on the filter bar. The control is rendered rather
// than delegated to a native select, because a native select cannot be styled
// consistently across platforms, least of all its open menu.
type SelectField struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Label   string         `json:"label"`
	Value   string         `json:"value"`
	Options []SelectOption `json:"options"`
}

// Selected returns the label of the chosen option, falling back to the first
// one, so the closed control always names its own state.
func (f SelectField) Selected() string {
	for _, option := range f.Options {
		if option.Value == f.Value {
			return option.Label
		}
	}
	if len(f.Options) > 0 {
		return f.Options[0].Label
	}
	return ""
}

// IsSelected reports whether an option is the chosen one.
func (f SelectField) IsSelected(option SelectOption) bool {
	return option.Value == f.Value
}

// WaveSpan is one span in the shape the drawing wants: where it ran as a
// fraction of the trace, how deep it sat, and what colour it draws in.
//
// Every span is handed over, parents and root included, because the drawing is
// of the stack: a span that wraps the whole trace is the reason there is a
// floor under everything else. The legend answers the other question, where the
// time went, and answers it from the sweep instead.
type WaveSpan struct {
	Name   string  `json:"name"`
	Kind   string  `json:"kind"`
	Color  string  `json:"color"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Depth  int     `json:"depth"`
	Failed bool    `json:"failed,omitempty"`
}

// WaveTrace is the payload behind the drawing: the spans, how long the trace
// took, and how deep it nested. The duration lets the drawing work in time
// rather than in pixels; the depth lets it scale itself to the trace it has
// rather than to a number someone guessed.
type WaveTrace struct {
	Milliseconds float64    `json:"ms"`
	Depth        int        `json:"depth"`
	Spans        []WaveSpan `json:"spans"`
}

// AxisTick is one labelled step of the time axis under a timeline.
type AxisTick struct {
	Share float64 `json:"share_percent"`
	Label string  `json:"label"`
}

// SpanRow is the flattened render model of one span within a trace.
type SpanRow struct {
	model.Span

	Offset      time.Duration `json:"offset_ns"`
	OffsetShare float64       `json:"offset_percent"`
	Share       float64       `json:"share_percent"`
	Open        bool          `json:"open,omitempty"`
	Last        bool          `json:"-"`

	// Memory is the memory_usage the span reported, and HasMemory whether it
	// reported one. Zero bytes in use and no reading are different answers.
	Memory    int64 `json:"memory_usage_bytes,omitempty"`
	HasMemory bool  `json:"-"`
}

// MemoryBudget is the memory_limit and memory_usage a trace recorded, read off
// the trace and off its spans. Every part of it is optional.
type MemoryBudget struct {
	// Limit is the ceiling the transaction ran under, Used what it had in use
	// when it finished, and Share the second as a percentage of the first.
	Limit int64   `json:"limit_bytes,omitempty"`
	Used  int64   `json:"used_bytes,omitempty"`
	Share float64 `json:"used_percent,omitempty"`

	HasLimit bool `json:"-"`
	HasUsed  bool `json:"-"`

	// Peak is the largest reading any span reported, and Spans whether any of
	// them did. The per span bars are drawn against the peak and not against
	// the limit, which would draw a whole trace as identical slivers.
	Peak  int64 `json:"peak_bytes,omitempty"`
	Spans bool  `json:"-"`
}

// TraceMemory reads the memory a trace reported, from its attributes and from
// the rows flattened out of its spans.
func TraceMemory(trace model.Trace, rows []SpanRow) MemoryBudget {
	budget := MemoryBudget{}
	budget.Limit, budget.HasLimit = trace.Attributes.Int64(model.AttrMemoryLimit)
	budget.Used, budget.HasUsed = trace.Attributes.Int64(model.AttrMemoryUsage)

	for _, row := range rows {
		if !row.HasMemory {
			continue
		}
		budget.Spans = true
		budget.Peak = max(budget.Peak, row.Memory)
	}

	if budget.HasLimit && budget.Limit > 0 && budget.HasUsed {
		budget.Share = float64(budget.Used) * 100 / float64(budget.Limit)
	}
	return budget
}

// Known reports whether the trace recorded any memory at all.
func (b MemoryBudget) Known() bool {
	return b.HasLimit || b.HasUsed || b.Spans
}

// ShareOf returns a reading as a percentage of the largest one in the trace.
func (b MemoryBudget) ShareOf(size int64) float64 {
	if b.Peak <= 0 || size <= 0 {
		return 0
	}
	return float64(size) * 100 / float64(b.Peak)
}
