package model

// Collected is what the recorder answers one posted transaction with: the
// trace context naming it, so the client nests its next transaction and its
// next call below this one.
type Collected struct {
	TraceContext

	// ID is the identifier the dashboard lists the transaction under.
	ID string `json:"id,omitempty"`

	// TraceParent is the header the client sends to name this transaction as
	// the parent of the work it causes.
	TraceParent string `json:"traceparent,omitempty"`

	// Error is why the transaction was not recorded, empty for one that was.
	Error string `json:"error,omitempty"`
}

// NewCollected returns the answer to a recorded transaction.
func NewCollected(id string, context TraceContext) Collected {
	return Collected{TraceContext: context, ID: id, TraceParent: context.TraceParent()}
}
