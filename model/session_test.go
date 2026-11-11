package model

import "testing"

const sessionULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

func TestSessionFromState(t *testing.T) {
	cases := map[string]string{
		"oida=" + sessionULID:             sessionULID,
		"rojo=1, oida=" + sessionULID:     sessionULID,
		"oida=" + sessionULID + ",rojo=1": sessionULID,
		"oida=not-a-ulid":                 "",
		"rojo=1":                          "",
		"":                                "",
		"xoida=" + sessionULID:            "",
		"oida=" + sessionULID + "extra":   "",
	}
	for state, want := range cases {
		if got := SessionFromState(state); got != want {
			t.Errorf("SessionFromState(%q) = %q, want %q", state, got, want)
		}
	}
}

func TestSessionTraceID(t *testing.T) {
	hex := SessionTraceID(sessionULID)
	if !ValidTraceID(hex) || IDFromTraceID(hex) != sessionULID {
		t.Fatalf("SessionTraceID = %q, which does not round trip", hex)
	}
	if SessionTraceID("nope") != "" {
		t.Error("a session that is no ULID has no trace")
	}
}

func TestTransactionSession(t *testing.T) {
	opts := TraceOptions{}
	tx := Transaction{Name: "pageload /", SessionID: sessionULID, TraceID: SessionTraceID(sessionULID)}
	if got := tx.Trace("01K0RECORDED0000000000000A", opts).SessionID; got != sessionULID {
		t.Errorf("session = %q, want %q", got, sessionULID)
	}
	tx = Transaction{Name: "x", SessionID: "bad id"}
	if got := tx.Trace("01K0RECORDED0000000000000A", opts).SessionID; got != "" {
		t.Errorf("an invalid session is dropped, got %q", got)
	}
	if err := tx.Validate(); err != nil {
		t.Errorf("an invalid session must not fail the transaction: %v", err)
	}
	tx = Transaction{Name: "x", TraceState: "oida=" + sessionULID}
	if got := tx.Trace("01K0RECORDED0000000000000A", opts).SessionID; got != sessionULID {
		t.Errorf("the tracestate names the session, got %q", got)
	}
}

func TestSetParentSession(t *testing.T) {
	trace := &Trace{}
	trace.SetParent(TraceContext{TraceID: SessionTraceID(sessionULID), SpanID: "00f067aa0ba902b7", State: "oida=" + sessionULID})
	if trace.SessionID != sessionULID {
		t.Errorf("SessionID = %q", trace.SessionID)
	}
}
