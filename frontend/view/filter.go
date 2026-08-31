package view

import "github.com/titpetric/oida/model"

// Filter applies the list filters of a page.
func Filter(traces []model.Trace, page Page) []model.Trace {
	if page.Query == "" && page.Kind == "" && page.Host == "" &&
		(page.Status == "" || page.Status == "all") {
		return traces
	}

	out := make([]model.Trace, 0, len(traces))
	for _, trace := range traces {
		if !matches(trace, page.Query) {
			continue
		}
		if page.Kind != "" && !trace.HasKind(page.Kind) {
			continue
		}
		if page.Host != "" && model.TraceHost(trace) != page.Host {
			continue
		}
		if page.Status == "error" && trace.ErrorText == "" && trace.State != model.StateError {
			continue
		}
		out = append(out, trace)
	}
	return out
}
