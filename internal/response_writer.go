package internal

import (
	"io"
	"net/http"

	"github.com/titpetric/oida/model"
)

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
)

// NewResponseWriter wraps w to record what the handler answered with. The
// trace moves to StateWriting once, when the first byte or the status goes
// out; a trace field instead of a callback keeps the wrapper one allocation.
func NewResponseWriter(w http.ResponseWriter, trace *model.Trace) *ResponseWriter {
	return &ResponseWriter{ResponseWriter: w, status: http.StatusOK, trace: trace}
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

// Flush forwards a flush to the wrapped writer.
func (w *ResponseWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
