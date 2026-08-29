package oida

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/titpetric/oida/model"
)

// RequestIDHeader carries the trace identifier on the request and the response.
const RequestIDHeader = "Request-Id"

// TracingMiddleware returns middleware recording every sampled request into the
// tracer resolved from opts. It is compatible with chi's Use, with alice, and
// with any func(http.Handler) http.Handler chain.
func TracingMiddleware(opts Options) func(http.Handler) http.Handler {
	opts = opts.WithDefaults()
	tracer := MustResolve(opts)
	sampler := opts.sampler()

	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !tracer.Enabled() || opts.ignored(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if !sampler.Sample(r) {
				tracer.countUnsampled(r.Host)
				next.ServeHTTP(w, r)
				return
			}
			serveTraced(tracer, opts, next, w, r)
		})
	}
}

// serveTraced records one request.
func serveTraced(tracer *Tracer, opts Options, next http.Handler, w http.ResponseWriter, r *http.Request) {
	id := requestID(r, opts)
	if id == "" {
		next.ServeHTTP(w, r)
		return
	}

	r.Header.Set(RequestIDHeader, id)
	w.Header().Set(RequestIDHeader, id)

	info := &HTTPInfo{
		Method:        r.Method,
		URI:           r.URL.RequestURI(),
		Host:          r.Host,
		Protocol:      r.Proto,
		RemoteAddress: remoteAddr(r),
		UserAgent:     r.UserAgent(),
	}
	trace := tracer.begin(id, r.Method+" "+r.URL.Path, info)
	trace.SetState(StateReading)

	ctx, span := trace.StartSpan(WithTrace(r.Context(), trace), r.Method+" "+r.URL.Path, KindHTTP)
	r = r.WithContext(ctx)

	writer := &responseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
		onWrite:        func() { trace.SetState(StateWriting) },
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("panic: %v", recovered)
			span.RecordError(err)
			finishRequest(tracer, opts, trace, writer, r)
			panic(recovered)
		}
		finishRequest(tracer, opts, trace, writer, r)
	}()

	trace.SetState(StateProcessing)
	next.ServeHTTP(writer, r)
}

// finishRequest records the response metadata and completes the trace.
func finishRequest(tracer *Tracer, opts Options, trace *Trace, writer *responseWriter, r *http.Request) {
	route := routePattern(r, opts)
	trace.SetResponse(writer.status, writer.bytes, route)
	if route != "" {
		trace.SetName(r.Method + " " + route)
	}
	if writer.status >= http.StatusInternalServerError && trace.Err() == nil {
		trace.RecordError(fmt.Errorf("http %d", writer.status))
	}
	tracer.Finish(trace)
}

// requestID returns the identifier to record the request under.
func requestID(r *http.Request, opts Options) string {
	if opts.TrustRequestID {
		if given := strings.TrimSpace(r.Header.Get(RequestIDHeader)); model.ValidID(given) {
			return given
		}
	}
	id, err := model.NewID(opts.now())
	if err != nil {
		return ""
	}
	return id
}

// routePattern returns the routed pattern of the request. A configured
// RouteFunc decides on its own, including when it returns nothing: a service
// mounting a catch-all knows that pattern is not worth grouping by, and the
// fallback would put it back. Without one, the pattern the router recorded on
// the request is used.
func routePattern(r *http.Request, opts Options) string {
	if opts.RouteFunc != nil {
		return opts.RouteFunc(r)
	}
	if r.Pattern != "" {
		if _, pattern, ok := strings.Cut(r.Pattern, " "); ok {
			return pattern
		}
		return r.Pattern
	}
	return ""
}
