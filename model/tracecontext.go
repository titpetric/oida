package model

import (
	"strconv"
	"strings"
)

// The headers W3C Trace Context propagates a trace with.
const (
	// TraceParentHeader carries the trace identifier, the caller's span and
	// the sampling flag.
	TraceParentHeader = "traceparent"

	// TraceStateHeader carries vendor state a receiver passes on untouched.
	TraceStateHeader = "tracestate"
)

// TraceContextVersion is the traceparent version this package writes.
const TraceContextVersion = "00"

// The hex lengths the traceparent fields are fixed at.
const (
	traceIDLength     = 32
	spanIDLength      = 16
	traceParentLength = 55
)

// FlagSampled is the trace-flags bit a caller sets for a trace it recorded.
// The other seven bits are reserved and travel unread.
const FlagSampled byte = 0x01

// traceStateMembers is the number of tracestate list members the
// specification allows; the rest are dropped.
const traceStateMembers = 32

// TraceContext is the W3C trace context of one transaction: the trace it
// belongs to, one span of that trace, and the vendor state travelling beside
// them. The zero value names no trace.
type TraceContext struct {
	// TraceID is the 32 hex digit identifier every transaction of one trace
	// shares.
	TraceID string `json:"trace_id,omitempty"`

	// SpanID is the 16 hex digit identifier of the span this context names:
	// the caller's span on a parsed header, the named span on one written.
	SpanID string `json:"span_id,omitempty"`

	// Flags is the trace-flags byte, FlagSampled set for a recorded trace.
	Flags byte `json:"flags,omitempty"`

	// State is the tracestate header, kept as it arrived.
	State string `json:"tracestate,omitempty"`
}

// ParseTraceParent returns the trace context a traceparent header carries, or
// ErrInvalidTraceParent.
//
// Version 00 is read in full, and a higher version as its first three fields,
// which the version format reserves for exactly that; the trailing fields are
// dropped. An all zero trace or span identifier is refused, and so is version
// ff.
func ParseTraceParent(value string) (TraceContext, error) {
	value = strings.TrimSpace(value)
	if len(value) < traceParentLength || value[2] != '-' {
		return TraceContext{}, ErrInvalidTraceParent
	}
	version := value[0:2]
	switch {
	case !hexOnly(version) || version == "ff":
		return TraceContext{}, ErrInvalidTraceParent
	case version == TraceContextVersion && len(value) != traceParentLength:
		return TraceContext{}, ErrInvalidTraceParent
	case len(value) > traceParentLength && value[traceParentLength] != '-':
		return TraceContext{}, ErrInvalidTraceParent
	}

	fields := value[3:traceParentLength]
	traceID, spanID, flags := fields[0:traceIDLength], fields[traceIDLength+1:traceIDLength+1+spanIDLength], fields[traceIDLength+spanIDLength+2:]
	if fields[traceIDLength] != '-' || fields[traceIDLength+spanIDLength+1] != '-' {
		return TraceContext{}, ErrInvalidTraceParent
	}
	if !ValidTraceID(traceID) || !ValidSpanID(spanID) || !hexOnly(flags) {
		return TraceContext{}, ErrInvalidTraceParent
	}

	parsed, err := strconv.ParseUint(flags, 16, 8)
	if err != nil {
		return TraceContext{}, ErrInvalidTraceParent
	}
	return TraceContext{TraceID: traceID, SpanID: spanID, Flags: byte(parsed)}, nil
}

// TraceParent returns the traceparent header of the context, empty for a
// context naming no trace and span.
func (c TraceContext) TraceParent() string {
	if !c.Valid() {
		return ""
	}
	flags := []byte{hexDigits[c.Flags>>4], hexDigits[c.Flags&0x0f]}
	return TraceContextVersion + "-" + c.TraceID + "-" + c.SpanID + "-" + string(flags)
}

// Sampled reports whether the caller recorded the trace.
func (c TraceContext) Sampled() bool {
	return c.Flags&FlagSampled == FlagSampled
}

// Valid reports whether the context names a trace and a span.
func (c TraceContext) Valid() bool {
	return ValidTraceID(c.TraceID) && ValidSpanID(c.SpanID)
}

// WithSpan returns the context naming another span of the same trace.
func (c TraceContext) WithSpan(spanID string) TraceContext {
	c.SpanID = spanID
	return c
}

// TraceParentOf returns the sampled traceparent header naming a span of a
// trace.
func TraceParentOf(traceID, spanID string) string {
	return TraceContext{TraceID: traceID, SpanID: spanID, Flags: FlagSampled}.TraceParent()
}

// CleanTraceState returns the tracestate a receiver carries through: at most
// 32 key/value members in arrival order.
//
// A member the specification does not allow is dropped rather than failing
// the header, so one bad entry cannot cost the trace its context.
func CleanTraceState(value string) string {
	if value == "" {
		return ""
	}
	members := make([]string, 0, traceStateMembers)
	for member := range strings.SplitSeq(value, ",") {
		member = strings.TrimSpace(member)
		key, _, ok := strings.Cut(member, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		members = append(members, member)
		if len(members) == traceStateMembers {
			break
		}
	}
	return strings.Join(members, ",")
}

// SetParent records the trace context a caller propagated: the trace to
// join, the span that called, and the vendor state to carry through.
func (t *Trace) SetParent(c TraceContext) {
	if t == nil || !c.Valid() {
		return
	}
	t.lock()
	defer t.unlock()
	t.TraceID, t.ParentSpanID, t.TraceState = c.TraceID, c.SpanID, CleanTraceState(c.State)
}

// Parent returns the trace context the trace joined: its trace, the caller's
// span and the state that came with it.
func (t *Trace) Parent() TraceContext {
	if t == nil {
		return TraceContext{}
	}
	t.lock()
	defer t.unlock()
	return TraceContext{TraceID: t.TraceID, SpanID: t.ParentSpanID, Flags: FlagSampled, State: t.TraceState}
}

// SpanContext returns the trace context an outbound call propagates.
func (t *Trace) SpanContext(spanID int) TraceContext {
	return t.Parent().WithSpan(SpanIDHex(spanID))
}

// ValidTraceID reports whether id is 32 hex digits and not all zeroes.
func ValidTraceID(id string) bool {
	return len(id) == traceIDLength && hexOnly(id) && strings.Trim(id, "0") != ""
}

// ValidSpanID reports whether id is 16 hex digits and not all zeroes.
func ValidSpanID(id string) bool {
	return len(id) == spanIDLength && hexOnly(id) && strings.Trim(id, "0") != ""
}

// SpanIDHex returns the 16 hex digit identifier of a span sequence number,
// empty for a number no span carries.
func SpanIDHex(id int) string {
	if id <= 0 {
		return ""
	}
	out := []byte("0000000000000000")
	for i, value := spanIDLength-1, uint64(id); value > 0 && i >= 0; i, value = i-1, value>>4 {
		out[i] = hexDigits[value&0x0f]
	}
	return string(out)
}

// SpanIDValue returns the span sequence number a 16 hex digit identifier
// encodes, zero for one that encodes none.
func SpanIDValue(id string) int {
	value, err := strconv.ParseUint(id, 16, 64)
	if !ValidSpanID(id) || err != nil || value > maxSpanID {
		return 0
	}
	return int(value)
}

// maxSpanID bounds SpanIDValue, so a hostile identifier cannot overflow the
// int it decodes into.
const maxSpanID = 1 << 31

// hexDigits is the lowercase alphabet every identifier is written in.
const hexDigits = "0123456789abcdef"

// hexOnly reports whether value is lowercase hexadecimal throughout.
func hexOnly(value string) bool {
	for i := range len(value) {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return len(value) > 0
}
