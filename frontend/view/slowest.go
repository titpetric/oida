package view

import (
	"time"

	"github.com/titpetric/oida/model"
)

// Slowest returns the longest duration in a set of traces, used to scale the
// inline bars so rows compare against each other.
func Slowest(traces []model.Trace) time.Duration {
	var longest time.Duration
	for _, trace := range traces {
		if trace.Duration > longest {
			longest = trace.Duration
		}
	}
	return longest
}
