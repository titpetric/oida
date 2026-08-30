package frontend

import (
	"context"

	"github.com/titpetric/oida/model"
)

// nopRecorder backs the front end while no tracer exists: everything reads
// empty and nothing records. It keeps a dashboard mounted before the tracer is
// configured serving pages instead of panicking on a nil interface.
type nopRecorder struct{}

var _ model.Recorder = nopRecorder{}

// StartTrace implements model.Recorder. It never records.
func (nopRecorder) StartTrace(ctx context.Context, name string) (context.Context, *model.Trace, error) {
	return ctx, nil, model.ErrDisabled
}

// Finish implements model.Recorder.
func (nopRecorder) Finish(*model.Trace) {}

// Snapshot implements model.Recorder.
func (nopRecorder) Snapshot() model.Snapshot { return model.Snapshot{} }

// Traces implements model.Recorder.
func (nopRecorder) Traces() []model.Trace { return nil }

// Trace implements model.Recorder.
func (nopRecorder) Trace(string) (model.Trace, bool) { return model.Trace{}, false }

// Live implements model.Recorder.
func (nopRecorder) Live() []model.Trace { return nil }

// Subscribe implements model.Recorder. The channel is nil, so a subscriber
// blocks until its context ends rather than busy looping.
func (nopRecorder) Subscribe() (<-chan struct{}, func()) { return nil, func() {} }

// Options implements model.Recorder.
func (nopRecorder) Options() model.Options { return model.NewOptions("") }

// Enabled implements model.Recorder.
func (nopRecorder) Enabled() bool { return false }

// SetEnabled implements model.Recorder.
func (nopRecorder) SetEnabled(bool) {}

// Reset implements model.Recorder.
func (nopRecorder) Reset() {}

// ReportError implements model.Recorder.
func (nopRecorder) ReportError(error) {}
