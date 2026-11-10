package model

import "strings"

// SessionStateKey is the tracestate member a session travels in: the member
// value is the session ULID. A browser puts it on every call it makes, so a
// request recorded by a service joins the session the page is part of.
const SessionStateKey = "oida"

// ValidSessionID reports whether id is a usable session identifier. A session
// is an opaque ULID; anything else is dropped rather than recorded.
func ValidSessionID(id string) bool {
	return ValidID(id)
}

// SessionTraceID returns the trace identifier a session is traced under: the
// 128 bits of its ULID, so the first traceparent of a session is the session.
func SessionTraceID(id string) string {
	if !ValidSessionID(id) {
		return ""
	}
	return TraceIDHex(id)
}

// SessionFromState returns the session a tracestate names, empty when it
// holds none or holds one that is no ULID.
func SessionFromState(state string) string {
	for member := range strings.SplitSeq(state, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(member), "=")
		if ok && strings.TrimSpace(key) == SessionStateKey && ValidSessionID(value) {
			return value
		}
	}
	return ""
}
