package model

import (
	"sort"
	"time"
)

// Statistics aggregates the rolling window of completed traces. Traces group by
// host and by routed pattern where one is known, so /users/1 and /users/2
// aggregate into GET /users/{id}. Lifetime request counts per host come from
// the tracer, because requests the sampler rejected never became traces.
func Statistics(window []Trace, windowLimit, topLimit int, requests map[string]uint64) Stats {
	result := Stats{
		WindowSize:  len(window),
		WindowLimit: windowLimit,
		TopLimit:    topLimit,
		Hosts:       hostStatistics(window, requests),
	}
	if len(window) == 0 {
		return result
	}

	grouped := make(map[string]*Statistic, len(window))
	order := make([]string, 0, len(window))
	for _, trace := range window {
		name, host := groupKey(trace)
		key := host + "\x00" + name
		stat := grouped[key]
		if stat == nil {
			stat = &Statistic{Name: name, Host: host}
			grouped[key] = stat
			order = append(order, key)
		}

		stat.Count++
		if trace.Error != "" || trace.State == StateError {
			stat.Errors++
		}
		stat.totalDuration += trace.Duration
		if trace.Duration > stat.MaxDuration {
			stat.MaxDuration = trace.Duration
		}
		if trace.HTTP != nil && trace.HTTP.ResponseBytes > 0 {
			stat.totalResponseBytes += uint64(trace.HTTP.ResponseBytes)
		}
		stat.totalAllocated += trace.Memory.AllocatedBytes
		stat.totalSpans += uint64(len(trace.Spans))
	}

	result.Top = make([]Statistic, 0, len(grouped))
	for _, key := range order {
		stat := grouped[key]
		stat.Share = float64(stat.Count) * 100 / float64(len(window))
		stat.AverageDuration = stat.totalDuration / time.Duration(stat.Count)
		stat.AverageResponseBytes = stat.totalResponseBytes / stat.Count
		stat.AverageAllocatedBytes = stat.totalAllocated / stat.Count
		stat.AverageSpans = float64(stat.totalSpans) / float64(stat.Count)
		result.Top = append(result.Top, *stat)
	}

	sort.SliceStable(result.Top, func(i, j int) bool {
		left, right := result.Top[i], result.Top[j]
		switch {
		case left.Count != right.Count:
			return left.Count > right.Count
		case left.totalDuration != right.totalDuration:
			return left.totalDuration > right.totalDuration
		case left.Name != right.Name:
			return left.Name < right.Name
		default:
			return left.Host < right.Host
		}
	})
	if topLimit > 0 && len(result.Top) > topLimit {
		result.Top = result.Top[:topLimit]
	}
	return result
}

// hostStatistics aggregates the window by host, and folds in the lifetime
// request counts the tracer keeps. A host with requests but no traces is still
// listed: that is what heavy sampling looks like.
func hostStatistics(window []Trace, requests map[string]uint64) []HostStat {
	if len(window) == 0 && len(requests) == 0 {
		return nil
	}

	grouped := make(map[string]*HostStat, len(requests)+1)
	routes := make(map[string]map[string]struct{}, len(requests)+1)

	host := func(name string) *HostStat {
		stat := grouped[name]
		if stat == nil {
			stat = &HostStat{Host: name}
			grouped[name] = stat
			routes[name] = make(map[string]struct{})
		}
		return stat
	}
	for name, count := range requests {
		host(name).Requests = count
	}

	for _, trace := range window {
		name := TraceHost(trace)
		stat := host(name)
		stat.Traces++
		if trace.Error != "" || trace.State == StateError {
			stat.Errors++
		}
		stat.totalDuration += trace.Duration
		stat.totalSpans += uint64(len(trace.Spans))
		if trace.Duration > stat.MaxDuration {
			stat.MaxDuration = trace.Duration
		}
		route, _ := groupKey(trace)
		routes[name][route] = struct{}{}
	}

	out := make([]HostStat, 0, len(grouped))
	for name, stat := range grouped {
		stat.Routes = len(routes[name])
		if stat.Traces > 0 {
			stat.AverageDuration = stat.totalDuration / time.Duration(stat.Traces)
			stat.AverageSpans = float64(stat.totalSpans) / float64(stat.Traces)
			if len(window) > 0 {
				stat.Share = float64(stat.Traces) * 100 / float64(len(window))
			}
		}
		out = append(out, *stat)
	}

	sort.SliceStable(out, func(i, j int) bool {
		switch {
		case out[i].Requests != out[j].Requests:
			return out[i].Requests > out[j].Requests
		case out[i].Traces != out[j].Traces:
			return out[i].Traces > out[j].Traces
		default:
			return out[i].Host < out[j].Host
		}
	})
	return out
}

// TraceHost returns the host a trace belongs to. Background traces have none,
// so they group under a stable placeholder rather than an empty string.
func TraceHost(trace Trace) string {
	if trace.HTTP == nil || trace.HTTP.Host == "" {
		return BackgroundHost
	}
	return trace.HTTP.Host
}

// BackgroundHost is the host label of traces that did not arrive over the
// network: cron ticks, queue consumers, startup work.
const BackgroundHost = "internal"

// StateDurations converts accumulated per state time into display order with
// shares.
func StateDurations(durations map[State]time.Duration) []StateDuration {
	var total time.Duration
	for _, duration := range durations {
		total += duration
	}
	result := make([]StateDuration, 0, len(states))
	for _, state := range states {
		duration := durations[state]
		if duration <= 0 {
			continue
		}
		entry := StateDuration{
			State:    state,
			Label:    state.Label(),
			Duration: duration,
		}
		if total > 0 {
			entry.Share = float64(duration) * 100 / float64(total)
		}
		result = append(result, entry)
	}
	return result
}

// groupKey returns the statistics group of a trace.
func groupKey(trace Trace) (name, host string) {
	if trace.HTTP == nil {
		return trace.Name, ""
	}
	if trace.HTTP.Route != "" {
		return trace.HTTP.Method + " " + trace.HTTP.Route, trace.HTTP.Host
	}
	return trace.Name, trace.HTTP.Host
}
