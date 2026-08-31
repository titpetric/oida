package oida

import (
	"fmt"
	"net/http"

	"github.com/titpetric/oida/internal"
	"github.com/titpetric/oida/model"
)

// RequestIDHeader carries the trace identifier on the request and the response.
const RequestIDHeader = model.RequestIDHeader

// serveTraced records one request.
func serveTraced(tracer *Tracer, opts Options, next http.Handler, w http.ResponseWriter, r *http.Request) {
	id := internal.RequestID(r, opts)
	if id == "" {
		next.ServeHTTP(w, r)
		return
	}

	r.Header.Set(RequestIDHeader, id)
	w.Header().Set(RequestIDHeader, id)

	info := &model.HTTPInfo{
		Method:        r.Method,
		URI:           r.URL.RequestURI(),
		Host:          r.Host,
		Protocol:      r.Proto,
		RemoteAddress: internal.RemoteAddr(r),
		UserAgent:     r.UserAgent(),
	}
	trace := tracer.begin(id, r.Method+" "+r.URL.Path, info)
	trace.SetState(StateReading)

	ctx, span := trace.StartSpan(WithTrace(r.Context(), trace), r.Method+" "+r.URL.Path, KindHTTP)
	r = r.WithContext(ctx)

	writer := internal.NewResponseWriter(w, func() { trace.SetState(StateWriting) })

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
func finishRequest(tracer *Tracer, opts Options, trace *Trace, writer *internal.ResponseWriter, r *http.Request) {
	route := internal.RoutePattern(r, opts)
	trace.SetResponse(writer.Status(), writer.Bytes(), route)
	if route != "" {
		trace.SetName(r.Method + " " + route)
	}
	if writer.Status() >= http.StatusInternalServerError && trace.Err() == nil {
		trace.RecordError(fmt.Errorf("http %d", writer.Status()))
	}
	tracer.Finish(trace)
}
