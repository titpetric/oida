package oida

import (
	"io"
	"net/http"
)

// responseWriter records the status and the number of bytes written while
// preserving the optional interfaces of the wrapped writer.
type responseWriter struct {
	http.ResponseWriter

	status  int
	bytes   int64
	onWrite func()
	wrote   bool
}

var (
	_ http.ResponseWriter = (*responseWriter)(nil)
	_ http.Flusher        = (*responseWriter)(nil)
	_ io.ReaderFrom       = (*responseWriter)(nil)
)

// Unwrap lets http.ResponseController reach the optional interfaces implemented
// by the original response writer.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// WriteHeader records the response status once.
func (w *responseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.status = status
	if w.onWrite != nil {
		w.onWrite()
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write counts the response body.
func (w *responseWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

// ReadFrom counts a body streamed with io.Copy.
func (w *responseWriter) ReadFrom(r io.Reader) (int64, error) {
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
func (w *responseWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
