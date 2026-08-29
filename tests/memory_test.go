package tests_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/titpetric/oida/tests"
)

// memoryDetail records one /memory trace and returns its detail page. The
// handler reads 30 KiB and 50 KiB off its two spans, so against the default
// mebibyte limit the plot scales to the 50 KiB peak: the readings sit at
// 44.80 and 8.00 from the top, whatever the spans' timing was.
func memoryDetail(t *testing.T, query string) string {
	t.Helper()

	server := tests.NewHTTPServer(t)
	client := server.Client()

	if status, _ := get(t, client, server.URL+"/memory"+query, nil); status != http.StatusOK {
		t.Fatalf("memory request status %d", status)
	}

	id := traceID(t, client, server.URL)
	status, body := get(t, client, server.URL+tests.Path+"/trace/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("detail status %d", status)
	}
	return body
}

func TestMemoryGraph(t *testing.T) {
	body := memoryDetail(t, "")

	for _, want := range []string{
		`class="memory-graph"`,
		"<h2>Memory</h2>",
		"peak 50.0 KiB of a 1.0 MiB limit", // the head line reading the graph out
		"44.80",                            // the 30 KiB reading on the 50 KiB scale
		"V 8.00",                           // the step up to the peak
		"load rows: 30.0 KiB at ",          // the hover title of the first stretch
		">50.0 KiB</span>",                 // the label on the newest reading
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail view misses %q", want)
		}
	}

	// A mebibyte is far above the 50 KiB peak: the limit stays in the head
	// line and no reference line is drawn.
	if strings.Contains(body, `class="limit"`) {
		t.Error("the limit line is drawn far off the scale of the readings")
	}
}

func TestMemoryLimitLine(t *testing.T) {
	// An 80 KiB limit is within twice the 50 KiB peak, so it shares the scale.
	body := memoryDetail(t, "?limit=81920")

	for _, want := range []string{
		`class="limit"`,
		"limit 80.0 KiB",
		"peak 50.0 KiB of a 80.0 KiB limit",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail view misses %q", want)
		}
	}
}

func TestMemoryAbsent(t *testing.T) {
	server := tests.NewHTTPServer(t)
	client := server.Client()

	if status, _ := get(t, client, server.URL+"/users/3", nil); status != http.StatusOK {
		t.Fatalf("seed request status %d", status)
	}

	id := traceID(t, client, server.URL)
	_, body := get(t, client, server.URL+tests.Path+"/trace/"+id, nil)

	if strings.Contains(body, `class="memory-graph"`) {
		t.Error("the memory graph is drawn for a trace whose spans reported no memory")
	}
	if strings.Contains(body, "Memory in use") {
		t.Error("the memory column is drawn for a trace whose spans reported no memory")
	}
}
