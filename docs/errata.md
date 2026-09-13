# Errata

Platform behaviour oida works around, with the decision behind each workaround. An entry stays until the platform moves.

## Windows clocks are too coarse to time a span

Reported in [issue #14](https://github.com/titpetric/oida/issues/14). The monotonic reading of `time.Now()` on Windows comes from the interrupt timer, which ticks every 0.5 to 15.6 milliseconds. A span that starts and ends inside one tick measured zero, and a zero duration is what the dashboard renders as a span still open.

### What oida does

Recorded clock readings of one trace are increment-only: a reading equal to or behind the last recorded one advances by a nanosecond (`model.clockStep`). Span starts, span ends, state changes and the finish each record a reading, so within a trace they are strictly ordered and every recorded duration is positive, whatever the clock underneath resolves. Readings that observe without recording, `Elapsed` and the snapshot copies, do not advance the sequence.

The values stay indicative, which is the contract `MemoryUse` already documents for the memory columns: on a coarse clock, a sub-tick span reads as the nanoseconds the recording sequence assigned it, not as a measurement.

### What oida does not do, and why

`QueryPerformanceCounter` and `GetSystemTimePreciseAsFileTime` read sub-microsecond time on Windows at 20 to 40 nanoseconds a call, against about 5 for `time.Now()`. oida does not call them: [golang/go#67066](https://github.com/golang/go/issues/67066) proposes moving `time.Now()` itself to `QueryPerformanceCounter`, and a workaround shipped here would outlive the problem. The decision is revisited when that issue resolves, or when a feature needs real sub-tick measurements, such as dropping spans below a duration threshold, which cannot be read off a stepped sequence.

A process that needs precise readings today sets `Options.Clock`:

```go
//go:build windows

package telemetry

import (
	"time"

	"golang.org/x/sys/windows"
)

// preciseNow reads the system time at the highest precision Windows
// reports, under a microsecond. The returned time carries no monotonic
// reading, so subtracting two readings compares the precise wall values
// instead of the coarse interrupt time.
func preciseNow() time.Time {
	var ft windows.Filetime
	windows.GetSystemTimePreciseAsFileTime(&ft)
	return time.Unix(0, ft.Nanoseconds())
}
```

The increment-only sequence applies on top of whatever clock is configured, so a clock that stands still or steps backwards still records ordered spans.
