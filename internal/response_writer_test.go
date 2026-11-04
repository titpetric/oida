package internal

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/oida/model"
)

// The wrapper records what the handler answered with: the status once, the
// bytes as they go out.
func TestResponseWriter(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewResponseWriter(recorder, model.NewTrace("01", "GET /", model.TraceOptions{}))
	defer writer.Release()

	writer.WriteHeader(http.StatusTeapot)
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte("brewing")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if writer.Status() != http.StatusTeapot {
		t.Fatalf("status %d, want the first one written", writer.Status())
	}
	if writer.Bytes() != 7 {
		t.Fatalf("bytes %d, want 7", writer.Bytes())
	}
}

// A hijack of a writer that cannot be hijacked is an error and not a panic,
// which is what the handler asserting http.Hijacker on the wrapper used to get.
func TestResponseWriterHijackUnsupported(t *testing.T) {
	writer := NewResponseWriter(httptest.NewRecorder(), model.NewTrace("01", "GET /", model.TraceOptions{}))
	defer writer.Release()

	conn, buffered, err := writer.Hijack()
	if !errors.Is(err, ErrNotHijackable) {
		t.Fatalf("hijack answered %v, want ErrNotHijackable", err)
	}
	if conn != nil || buffered != nil {
		t.Fatal("a refused hijack hands over nothing")
	}
}

// A hijack that the wrapped writer takes is handed over, and the status is the
// switch the handler is about to write over the connection itself.
func TestResponseWriterHijack(t *testing.T) {
	hijacker := &hijackRecorder{ResponseWriter: httptest.NewRecorder()}
	writer := NewResponseWriter(hijacker, model.NewTrace("01", "GET /", model.TraceOptions{}))
	defer writer.Release()

	if _, _, err := writer.Hijack(); err != nil {
		t.Fatalf("hijack: %v", err)
	}
	if !hijacker.called {
		t.Fatal("the hijack did not reach the wrapped writer")
	}
	if writer.Status() != http.StatusSwitchingProtocols {
		t.Fatalf("status %d, want 101", writer.Status())
	}
	// And no header went out: the handler owns the connection and writes the
	// switch over it.
	if hijacker.ResponseWriter.(*httptest.ResponseRecorder).Flushed {
		t.Fatal("the wrapper answered the request it handed over")
	}
}

// hijackRecorder is a response writer whose hijack succeeds, handing over
// nothing: what is under test is that the call arrives.
type hijackRecorder struct {
	http.ResponseWriter

	called bool
}

func (w *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.called = true
	return nil, nil, nil
}
