package oida

import "net/http"

// StartRequest is Start for code holding an *http.Request rather than a
// context. It returns a request carrying the span, so spans started from the
// returned request nest below this one.
//
//	r, span := oida.StartRequest(r, "user.Handler")
//	defer span.End()
//
// When the request carries no trace it is returned unchanged along with a nil
// span, so the unsampled path allocates nothing.
func StartRequest(r *http.Request, name string, kind ...Kind) (*http.Request, *Span) {
	ctx, span := Start(r.Context(), name, kind...)
	if span == nil {
		return r, nil
	}
	return r.WithContext(ctx), span
}
