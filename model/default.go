package model

import "sync"

var (
	defaultMu       sync.RWMutex
	defaultRecorder Recorder
)

// DefaultRecorder returns the process wide recorder, or nil when none has been
// configured. The root package fills the slot: Configure stores the tracer it
// builds, and the first resolution of the default creates one.
func DefaultRecorder() Recorder {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultRecorder
}

// SetDefaultRecorder replaces the process wide recorder. The front end reads
// the slot on every request, so a replacement takes effect immediately.
func SetDefaultRecorder(r Recorder) {
	defaultMu.Lock()
	defaultRecorder = r
	defaultMu.Unlock()
}
