package internal

import (
	"net/http"
	"strings"

	"github.com/titpetric/oida/model"
)

// RoutePattern returns the routed pattern of the request. A configured
// RouteFunc decides on its own, including when it returns nothing: a service
// mounting a catch-all knows that pattern is not worth grouping by, and the
// fallback would put it back. Without one, the pattern the router recorded on
// the request is used.
func RoutePattern(r *http.Request, opts model.Options) string {
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
