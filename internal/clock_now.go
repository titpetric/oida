package internal

import (
	"time"

	"github.com/titpetric/oida/model"
)

// ClockNow returns the current time from the configured clock.
func ClockNow(o model.Options) time.Time {
	if o.Clock == nil {
		return time.Now()
	}
	return o.Clock()
}
