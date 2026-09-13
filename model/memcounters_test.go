package model

import (
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
