package internal

import (
	"strings"

	"github.com/titpetric/oida/model"
)

// IgnoredPath reports whether a request path is excluded from tracing. The
// debug front end is always excluded so it does not trace itself.
func IgnoredPath(o model.Options, path string) bool {
	if path == o.Path || strings.HasPrefix(path, o.Path+"/") {
		return true
	}
	for _, pattern := range o.IgnorePaths {
		if prefix, ok := strings.CutSuffix(pattern, "/*"); ok {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return true
			}
			continue
		}
		if path == pattern {
			return true
		}
	}
	return false
}
