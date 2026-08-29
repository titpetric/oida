package oida

import (
	"net/http"

	"github.com/titpetric/oida/frontend"
)

var _ http.Handler = (*Tracer)(nil)

// ServeHTTP serves the debug front end of the tracer, so a tracer mounts like
// any other handler:
//
//	mux := http.NewServeMux()
//	mux.Handle("/debug/oida/", tracer)
//
//	r := chi.NewRouter()
//	r.Mount("/debug/oida", tracer)
//
// A path that does not start with Options.Path is treated as already relative,
// the shape http.StripPrefix delivers. A nil tracer serves 404, not a panic.
func (t *Tracer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t == nil {
		http.NotFound(w, r)
		return
	}
	t.handlerOnce.Do(func() {
		t.handler = frontend.HandlerFor(t)
	})
	t.handler.ServeHTTP(w, r)
}
