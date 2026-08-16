package oida

import (
	"math"
	"net/http"
	"strings"
	"time"
)

// Options configures telemetry behaviour, the debug front end and the
// middleware. Take NewOptions and override what you need, so fields added in
// later versions keep their defaults.
type Options struct {
	// Path is the mount path of the debug front end.
	Path string `yaml:"path"`

	// ServiceName is displayed in the front end and recorded on every trace.
	ServiceName string `yaml:"service_name"`

	// Enabled records traces. A disabled tracer passes requests through.
	Enabled bool `yaml:"enabled"`

	// RingBufferSize is the number of completed traces retained.
	RingBufferSize int `yaml:"ring_buffer_size"`

	// TopRequests is the maximum number of groups in rolling statistics.
	TopRequests int `yaml:"top_requests"`

	// MaxSpansPerTrace bounds the spans recorded in a single trace. Excess
	// spans are counted in Trace.DroppedSpans. Zero means unlimited.
	MaxSpansPerTrace int `yaml:"max_spans_per_trace"`

	// SampleRate is the fraction of requests traced, between 0 and 1. It is
	// ignored when Sampler is set.
	SampleRate float64 `yaml:"sample_rate"`

	// TrackMemoryUse records process-wide allocation changes for each trace.
	TrackMemoryUse bool `yaml:"track_memory_use"`

	// TrustRequestID reuses a client supplied Request-Id header. Only enable
	// this behind a trusted proxy.
	TrustRequestID bool `yaml:"trust_request_id"`

	// IgnorePaths lists request paths that are never traced. Entries ending in
	// "/*" match by prefix.
	IgnorePaths []string `yaml:"ignore_paths"`

	// RefreshInterval is the fallback auto refresh interval of the live view in
	// seconds, used when the browser cannot stream. Zero disables it.
	RefreshInterval int `yaml:"refresh_interval"`

	// LiveStream serves the live view over server sent events, so recorded
	// traces appear as they happen instead of on a timer.
	LiveStream bool `yaml:"live_stream"`

	// Sampler decides whether a request is traced. It replaces SampleRate.
	Sampler Sampler `yaml:"-"`

	// Storage retains completed traces. Defaults to StorageMemory sized by
	// RingBufferSize; StorageDisk retains them across restarts.
	Storage Storage `yaml:"-"`

	// RouteFunc returns the routed pattern of a request, so statistics group
	// /users/1 and /users/2 into GET /users/{id}. With chi:
	//
	//	opts.RouteFunc = func(r *http.Request) string {
	//		return chi.RouteContext(r.Context()).RoutePattern()
	//	}
	//
	// A nil function falls back to the pattern set by http.ServeMux.
	RouteFunc func(r *http.Request) string `yaml:"-"`

	// OnError receives storage and recording errors. The package never writes
	// to stdout or stderr, so this is the only way to observe them.
	OnError func(error) `yaml:"-"`

	// Authorize gates access to the debug front end. A nil function allows
	// every request.
	Authorize func(r *http.Request) bool `yaml:"-"`

	// Clock is the time source of the tracer. Defaults to time.Now.
	Clock func() time.Time `yaml:"-"`

	// Tracer is the recorder used by TracingMiddleware, Handler and Mount. A
	// nil tracer resolves the process default.
	Tracer *Tracer `yaml:"-"`
}

// DefaultPath is the default mount path of the debug front end.
const DefaultPath = "/debug/oida"

// NewOptions returns the default options.
func NewOptions() Options {
	return Options{
		Path:             DefaultPath,
		Enabled:          true,
		RingBufferSize:   200,
		TopRequests:      20,
		MaxSpansPerTrace: 1000,
		SampleRate:       1,
		TrackMemoryUse:   true,
		RefreshInterval:  5,
		LiveStream:       true,
		IgnorePaths: []string{
			"/healthz",
			"/readyz",
			"/metrics",
			"/favicon.ico",
		},
	}
}

// WithDefaults returns a copy of the options with zero values replaced by their
// defaults. It is applied before validation, so a zero Options is usable.
func (o Options) WithDefaults() Options {
	defaults := NewOptions()
	if o.Path == "" {
		o.Path = defaults.Path
	}
	o.Path = strings.TrimSuffix(o.Path, "/")
	if o.RingBufferSize == 0 {
		o.RingBufferSize = defaults.RingBufferSize
	}
	if o.TopRequests == 0 {
		o.TopRequests = defaults.TopRequests
	}
	if o.MaxSpansPerTrace == 0 {
		o.MaxSpansPerTrace = defaults.MaxSpansPerTrace
	}
	if o.SampleRate == 0 && o.Sampler == nil {
		o.SampleRate = defaults.SampleRate
	}
	if o.RefreshInterval == 0 {
		o.RefreshInterval = defaults.RefreshInterval
	}
	if o.IgnorePaths == nil {
		o.IgnorePaths = defaults.IgnorePaths
	}
	if o.Clock == nil {
		o.Clock = time.Now
	}
	return o
}

// Validate reports whether the options are usable. Every failure wraps
// ErrInvalidOptions.
func (o Options) Validate() error {
	if o.Path == "" || !strings.HasPrefix(o.Path, "/") {
		return ErrInvalidPath
	}
	if math.IsNaN(o.SampleRate) || o.SampleRate < 0 || o.SampleRate > 1 {
		return ErrInvalidSampleRate
	}
	if o.RingBufferSize < 0 {
		return invalidOption("ring_buffer_size", "must not be negative")
	}
	if o.TopRequests < 0 {
		return invalidOption("top_requests", "must not be negative")
	}
	if o.MaxSpansPerTrace < 0 {
		return invalidOption("max_spans_per_trace", "must not be negative")
	}
	if o.RefreshInterval < 0 {
		return invalidOption("refresh_interval", "must not be negative")
	}
	return nil
}

// sampler returns the configured sampler, or a rate sampler for SampleRate.
func (o Options) sampler() Sampler {
	if o.Sampler != nil {
		return o.Sampler
	}
	return NewRateSampler(o.SampleRate)
}

// now returns the current time from the configured clock.
func (o Options) now() time.Time {
	if o.Clock == nil {
		return time.Now()
	}
	return o.Clock()
}

// authorized reports whether r may access the debug front end.
func (o Options) authorized(r *http.Request) bool {
	if o.Authorize == nil {
		return true
	}
	return o.Authorize(r)
}

// ignored reports whether a request path is excluded from tracing. The debug
// front end is always excluded so it does not trace itself.
func (o Options) ignored(path string) bool {
	if path == o.Path || strings.HasPrefix(path, o.Path+"/") {
		return true
	}
	for _, pattern := range o.IgnorePaths {
		if prefix, ok := strings.CutSuffix(pattern, "/*"); ok {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return true
			}
			continue
		}
		if path == pattern {
			return true
		}
	}
	return false
}
