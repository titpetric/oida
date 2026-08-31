package oida

import (
	"reflect"
	"regexp"
	"runtime"
	"strings"
)

// symbolCleanup drops the punctuation the runtime puts around method values on
// pointer receivers, so (*UserStore).Get reads as UserStore.Get.
var symbolCleanup = regexp.MustCompile(`[()*]`)

// symbolName returns a span name read from a function, a value or a string.
// The import path is trimmed to its last element, so the result is the package,
// type and function names joined with a dot: billing.UserStore.GetUsers.
func symbolName(in any) string {
	raw := readSymbolName(in)
	if idx := strings.LastIndex(raw, "/"); idx != -1 {
		raw = raw[idx+1:]
	}
	// Method values carry a -fm suffix naming the wrapper the runtime made.
	raw = strings.TrimSuffix(raw, "-fm")
	return symbolCleanup.ReplaceAllString(raw, "")
}

func readSymbolName(in any) string {
	if in == nil {
		return "<nil>"
	}

	// A string is already a name, so it passes through.
	if s, ok := in.(string); ok && s != "" {
		return s
	}

	v := reflect.ValueOf(in)
	t := v.Type()

	// A func is named by the symbol its code address belongs to.
	if t.Kind() == reflect.Func {
		if fn := runtime.FuncForPC(v.Pointer()); fn != nil {
			return fn.Name()
		}
	}

	return t.String()
}
