package internal

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/titpetric/oida/model"
)

// ErrNotHijackable is what a hijack of a writer that cannot be hijacked
// answers, where the wrapper used to answer a panic from the handler asserting
// http.Hijacker on it.
var ErrNotHijackable = errors.New("oida: the response writer does not support hijacking")

// ResponseWriter records the status and the number of bytes written while
// preserving the optional interfaces of the wrapped writer.
type ResponseWriter struct {
	http.ResponseWriter

	status int
	bytes  int64
	trace  *model.Trace
	wrote  bool
}

var (
	_ http.ResponseWriter = (*ResponseWriter)(nil)
	_ http.Flusher        = (*ResponseWriter)(nil)
	_ io.ReaderFrom       = (*ResponseWriter)(nil)
	_ http.Hijacker       = (*ResponseWriter)(nil)
)

// writerPool recycles response writers across requests.
var writerPool = sync.Pool{
	New: func() any { return new(ResponseWriter) },
}

// NewResponseWriter wraps w to record what the handler answered with. The
// trace moves to StateWriting once, when the first byte or the status goes
// out. The wrapper comes from a pool; the middleware returns it with Release
// once the handler is done with it.
func NewResponseWriter(w http.ResponseWriter, trace *model.Trace) *ResponseWriter {
	rw := writerPool.Get().(*ResponseWriter)
	*rw = ResponseWriter{ResponseWriter: w, status: http.StatusOK, trace: trace}
	return rw
}

// Release returns the wrapper to the pool. After Release the wrapper is
// recycled memory, the way the http.ResponseWriter it wraps is invalid after
// the handler returns.
func (w *ResponseWriter) Release() {
	if w == nil {
		return
	}
	*w = ResponseWriter{}
	writerPool.Put(w)
}

// Status returns the status the handler wrote.
func (w *ResponseWriter) Status() int {
	return w.status
}

// Bytes returns the number of bytes the handler wrote.
func (w *ResponseWriter) Bytes() int64 {
	return w.bytes
}

// Unwrap lets http.ResponseController reach the optional interfaces implemented
// by the original response writer.
func (w *ResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// WriteHeader records the response status once.
func (w *ResponseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.status = status
	w.trace.SetState(model.StateWriting)
	w.ResponseWriter.WriteHeader(status)
}

// Write counts the response body.
func (w *ResponseWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

// ReadFrom counts a body streamed with io.Copy.
func (w *ResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if reader, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := reader.ReadFrom(r)
		w.bytes += n
		return n, err
	}
	n, err := io.Copy(w.ResponseWriter, r)
	w.bytes += n
	return n, err
}

// Hijack hands the connection over, which is how a websocket or any other
// protocol upgrade is served.
//
// The assertion has to hold on the wrapper itself and not only through Unwrap:
// golang.org/x/net/websocket asserts http.Hijacker on the writer it is given,
// and a traced route that upgrades panicked in the middleware without this.
//
// The handler writes the switch itself, over the connection it now owns, so
// the status is recorded here and no header goes out: 101, and the bytes of
// the stream after it are the connection's and not the response's.
func (w *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, ErrNotHijackable
	}
	conn, buffered, err := hijacker.Hijack()
	if err != nil {
		return conn, buffered, err
	}
	if !w.wrote {
		w.wrote = true
		w.status = http.StatusSwitchingProtocols
		w.trace.SetState(model.StateWriting)
	}
	return conn, buffered, nil
}

// Flush forwards a flush to the wrapped writer.
func (w *ResponseWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
