package model

import (
	"runtime"
	"testing"
)

// TestReadMemory pins the metric names: a renamed or missing metric comes
// back as a bad value whose reader panics, and a live process reports
// nonzero sizes.
func TestReadMemory(t *testing.T) {
	memory := ReadMemory()

	if memory.HeapAlloc == 0 || memory.System == 0 || memory.NextGC == 0 {
		t.Errorf("ReadMemory() = %+v, want nonzero heap, system and goal", memory)
	}
	if memory.HeapInuse < memory.HeapAlloc {
		t.Errorf("HeapInuse = %d below HeapAlloc = %d", memory.HeapInuse, memory.HeapAlloc)
	}
	if memory.Limit != 0 {
		t.Errorf("Limit = %d, want 0, the caller fills it", memory.Limit)
	}
}

// TestReadMemoryMatchesMemStats pins ReadMemory against the runtime's other
// accounting of the same statistics. The runtime's own tests assert exact
// correspondence for these fields; here the two readings are taken
// back to back in a live process, so each comparison carries slack for the
// allocation drift between them.
func TestReadMemoryMatchesMemStats(t *testing.T) {
	within := func(name string, got, want uint64) {
		t.Helper()
		slack := max(want/8, 512*1024)
		diff := max(got, want) - min(got, want)
		if diff > slack {
			t.Errorf("%s = %d, want %d within %d", name, got, want, slack)
		}
	}

	runtime.GC()
	var mstats runtime.MemStats
	runtime.ReadMemStats(&mstats)
	memory := ReadMemory()

	within("HeapAlloc", memory.HeapAlloc, mstats.HeapAlloc)
	within("HeapInuse", memory.HeapInuse, mstats.HeapInuse)
	within("HeapObjects", memory.HeapObjects, mstats.HeapObjects)
	within("StackInuse", memory.StackInuse, mstats.StackInuse)
	within("System", memory.System, mstats.Sys)
	within("NextGC", memory.NextGC, mstats.NextGC)
	if memory.NumGC < mstats.NumGC || memory.NumGC > mstats.NumGC+2 {
		t.Errorf("NumGC = %d, want %d or up to two cycles later", memory.NumGC, mstats.NumGC)
	}
	// The pause total is summed from histogram bucket midpoints, so it is
	// compared loosely; the fraction is an estimate on both sides.
	within("GCPauseTotal", memory.GCPauseTotal, mstats.PauseTotalNs)
	if memory.GCCPUFraction < 0 || memory.GCCPUFraction > 1 {
		t.Errorf("GCCPUFraction = %v, want within [0,1]", memory.GCCPUFraction)
	}
}
