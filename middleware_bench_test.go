package oida

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// benchWriter is a no-op http.ResponseWriter, so the middleware benchmarks
// measure the tracer rather than a response recorder.
type benchWriter struct{ h http.Header }

func (w *benchWriter) Header() http.Header         { return w.h }
func (w *benchWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *benchWriter) WriteHeader(int)             {}

func newBenchTracer(b *testing.B, apply func(*Options)) *Tracer {
	b.Helper()
	opts := NewOptions("bench")
	opts.Enabled = true
	opts.TrackMemoryUse = false
	opts.ReadEnv = false
	opts.OnError = func(err error) { b.Errorf("oida: %v", err) }
	if apply != nil {
		apply(&opts)
	}
	tracer, err := New(opts)
	if err != nil {
		b.Fatal(err)
	}
	return tracer
}

// benchmarkMiddleware drives one request shape through the middleware over a
// handler that does nothing, which is the overhead a traced service pays on
// top of its own work.
func benchmarkMiddleware(b *testing.B, path string, apply func(*Options)) {
	b.Helper()
	tracer := newBenchTracer(b, apply)
	handler := tracer.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	r := httptest.NewRequest(http.MethodGet, path, nil)
	w := &benchWriter{h: make(http.Header, 4)}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		handler.ServeHTTP(w, r)
	}
}

// BenchmarkMiddlewareDisabled is the floor: a nil-enabled tracer passing the
// request through.
func BenchmarkMiddlewareDisabled(b *testing.B) {
	benchmarkMiddleware(b, "/users/42", func(o *Options) { o.Enabled = false })
}

// BenchmarkMiddlewareIgnored is a path on the ignore list: recording skipped
// after the path check.
func BenchmarkMiddlewareIgnored(b *testing.B) {
	benchmarkMiddleware(b, "/healthz", nil)
}

// BenchmarkMiddlewareUnsampled is a request the sampler rejects: counted,
// not recorded.
func BenchmarkMiddlewareUnsampled(b *testing.B) {
	benchmarkMiddleware(b, "/users/42", func(o *Options) { o.SampleRate = 0 })
}

// BenchmarkMiddlewareRecorded is the full recording path: id, trace, root
// span, response wrap, finish, snapshot into the ring.
func BenchmarkMiddlewareRecorded(b *testing.B) {
	benchmarkMiddleware(b, "/users/42", nil)
}

// BenchmarkMiddlewareRecordedMemory is the recording path with memory
// tracking, the shipped default: two runtime.ReadMemStats per trace.
func BenchmarkMiddlewareRecordedMemory(b *testing.B) {
	benchmarkMiddleware(b, "/users/42", func(o *Options) { o.TrackMemoryUse = true })
}

// BenchmarkMiddlewareRecordedSpan adds one instrumented span below the root,
// the shape of a handler that records a database call.
func BenchmarkMiddlewareRecordedSpan(b *testing.B) {
	tracer := newBenchTracer(b, nil)
	handler := tracer.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, span := Start(r.Context(), "SELECT users", KindDatabase)
		span.End()
	}))
	r := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w := &benchWriter{h: make(http.Header, 4)}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		handler.ServeHTTP(w, r)
	}
}
