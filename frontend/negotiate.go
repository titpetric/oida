package frontend

import (
	"net/http"
	"strings"
)

// format is the response representation selected for a front end request.
type format int

const (
	formatHTML format = iota
	formatJSON
	formatText
)

// negotiate selects the response representation. The format query parameter
// overrides the Accept header, which is what tests use.
func negotiate(r *http.Request) format {
	switch r.URL.Query().Get("format") {
	case "json":
		return formatJSON
	case "text":
		return formatText
	case "html":
		return formatHTML
	}

	accept := strings.ToLower(r.Header.Get("Accept"))
	switch {
	case strings.Contains(accept, "application/json"), strings.Contains(accept, "text/json"):
		return formatJSON
	case strings.Contains(accept, "text/plain"):
		return formatText
	case strings.HasPrefix(strings.ToLower(r.UserAgent()), "curl/"):
		return formatText
	default:
		return formatHTML
	}
}
