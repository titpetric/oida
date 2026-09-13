package model

import (
	"math"
	"runtime/metrics"
	"sync"
)

// memCounters is one reading of the allocation counters a memory-tracked
// trace diffs: the runtime/metrics equivalents of the five MemStats fields
// TrackMemory used to read. runtime.ReadMemStats stops the world, which
// billed every tracked trace around ten microseconds per reading; these
// counters read without one.
type memCounters struct {
	heapBytes  uint64
	allocBytes uint64
	allocs     uint64
	gcCycles   uint64
	pauseNs    uint64
}

// memSamples is the pooled sample set readMemCounters reads into. The pause
// histogram's buckets are reused across reads once allocated, so a reading
// settles at zero allocations.
var memSamples = sync.Pool{
	New: func() any {
		s := make([]metrics.Sample, 5)
		s[0].Name = "/memory/classes/heap/objects:bytes"
		s[1].Name = "/gc/heap/allocs:bytes"
		s[2].Name = "/gc/heap/allocs:objects"
		s[3].Name = "/gc/cycles/total:gc-cycles"
		s[4].Name = "/sched/pauses/total/gc:seconds"
		return &s
	},
}

// memorySamples is the pooled sample set ReadMemory reads into, the
// dashboard's counterpart of memSamples.
var memorySamples = sync.Pool{
	New: func() any {
		s := make([]metrics.Sample, 10)
		s[0].Name = "/memory/classes/heap/objects:bytes"
		s[1].Name = "/memory/classes/heap/unused:bytes"
		s[2].Name = "/gc/heap/objects:objects"
		s[3].Name = "/memory/classes/heap/stacks:bytes"
		s[4].Name = "/memory/classes/total:bytes"
		s[5].Name = "/gc/heap/goal:bytes"
		s[6].Name = "/gc/cycles/total:gc-cycles"
		s[7].Name = "/sched/pauses/total/gc:seconds"
		s[8].Name = "/cpu/classes/gc/total:cpu-seconds"
		s[9].Name = "/cpu/classes/total:cpu-seconds"
		return &s
	},
}

// ReadMemory reads the process memory and GC pressure the dashboard shows,
// through runtime/metrics: unlike runtime.ReadMemStats, the read does not
// stop the world, so an open live view does not pause the process it
// watches. The pause total and the GC CPU fraction are computed from
// counters and are indicative, the way MemoryUse documents. Limit is the
// caller's to fill.
func ReadMemory() Memory {
	samples := memorySamples.Get().(*[]metrics.Sample)
	metrics.Read(*samples)
	s := *samples
	heapObjects := s[0].Value.Uint64()
	memory := Memory{
		HeapAlloc:    heapObjects,
		HeapInuse:    heapObjects + s[1].Value.Uint64(),
		HeapObjects:  s[2].Value.Uint64(),
		StackInuse:   s[3].Value.Uint64(),
		System:       s[4].Value.Uint64(),
		NextGC:       s[5].Value.Uint64(),
		NumGC:        uint32(s[6].Value.Uint64()),
		GCPauseTotal: pauseTotalNs(s[7].Value.Float64Histogram()),
	}
	if total := s[9].Value.Float64(); total > 0 {
		memory.GCCPUFraction = s[8].Value.Float64() / total
	}
	memorySamples.Put(samples)
	return memory
}

// readMemCounters reads the current allocation counters.
func readMemCounters() memCounters {
	samples := memSamples.Get().(*[]metrics.Sample)
	metrics.Read(*samples)
	out := memCounters{
		heapBytes:  (*samples)[0].Value.Uint64(),
		allocBytes: (*samples)[1].Value.Uint64(),
		allocs:     (*samples)[2].Value.Uint64(),
		gcCycles:   (*samples)[3].Value.Uint64(),
		pauseNs:    pauseTotalNs((*samples)[4].Value.Float64Histogram()),
	}
	memSamples.Put(samples)
	return out
}

// pauseTotalNs approximates the cumulative GC pause time from the pause
// histogram: each bucket's count at the bucket's midpoint. MemoryUse values
// are indicative by contract, and a delta across one trace is almost always
// zero pauses or one.
func pauseTotalNs(h *metrics.Float64Histogram) uint64 {
	if h == nil {
		return 0
	}
	var total float64
	for i, count := range h.Counts {
		if count == 0 {
			continue
		}
		lo, hi := h.Buckets[i], h.Buckets[i+1]
		if math.IsInf(lo, -1) {
			lo = hi
		}
		if math.IsInf(hi, 1) {
			hi = lo
		}
		total += float64(count) * (lo + hi) / 2
	}
	return uint64(total * 1e9)
}
