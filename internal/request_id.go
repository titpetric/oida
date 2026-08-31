package internal

import (
	"net/http"
	"strings"

	"github.com/titpetric/oida/model"
)

// RequestID returns the identifier to record the request under.
func RequestID(r *http.Request, opts model.Options) string {
	if opts.TrustRequestID {
		if given := strings.TrimSpace(r.Header.Get(model.RequestIDHeader)); model.ValidID(given) {
			return given
		}
	}
	id, err := model.NewID(ClockNow(opts))
	if err != nil {
		return ""
	}
	return id
}
