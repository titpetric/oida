package internal

import (
	"math"
	"os"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
)

// MemoryLimit returns the smallest memory limit the process is subject to, or
// zero when none can be determined.
func MemoryLimit() uint64 {
	var limits []uint64
	if limit := debug.SetMemoryLimit(-1); limit > 0 && limit != math.MaxInt64 {
		limits = append(limits, uint64(limit))
	}
	for _, path := range []string{
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
	} {
		value, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(value)) == "max" {
			continue
		}
		if limit, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64); err == nil && limit > 0 {
			limits = append(limits, limit)
		}
	}
	if value, err := os.ReadFile("/proc/meminfo"); err == nil {
		for line := range strings.SplitSeq(string(value), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				if kib, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					limits = append(limits, kib*1024)
				}
				break
			}
		}
	}
	if len(limits) == 0 {
		return 0
	}
	return slices.Min(limits)
}
