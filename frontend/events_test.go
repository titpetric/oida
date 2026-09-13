package frontend_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
)

func TestEventStreamPushesLiveSection(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	server := httptest.NewServer(frontend.Handler(tracer))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	stream, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+oida.DefaultPath+"/live/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	response, err := server.Client().Do(stream)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer response.Body.Close()

	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type %q", got)
	}

	reader := bufio.NewReader(response.Body)
	first := readEvent(t, reader)
	if !strings.Contains(first, "<h2>Feed</h2>") {
		t.Fatalf("first event is not the live section: %q", truncate(first, 200))
	}

	// A trace recorded now must show up on the stream without a new request.
	if err := tracer.Observe(t.Context(), "GET /users/{id}", func(ctx context.Context) error {
		oida.StartSpan(ctx, "SELECT users", oida.KindDatabase).End()
		return nil
	}); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	pushed := readEvent(t, reader)
	if !strings.Contains(pushed, "GET /users/{id}") {
		t.Fatalf("stream did not push the new trace: %q", truncate(pushed, 400))
	}
}

func TestEventStreamDisabled(t *testing.T) {
	tracer, _ := newTestTracer(t, func(o *oida.Options) { o.LiveStream = false })

	response := request(t, frontend.Handler(tracer), oida.DefaultPath+"/live/events", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled stream returned %d, want 404", response.Code)
	}

	body := request(t, frontend.Handler(tracer), oida.DefaultPath+"/live", nil).Body.String()
	if strings.Contains(body, "data-events") {
		t.Error("the live view advertises a stream while streaming is off")
	}
	if !strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("the live view has no refresh fallback while streaming is off")
	}
}

func TestLiveViewStreamOff(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	streaming := request(t, frontend.Handler(tracer), oida.DefaultPath+"/live", nil).Body.String()
	if !strings.Contains(streaming, "data-events") {
		t.Fatal("the live view does not advertise the stream by default")
	}

	// ?stream=off renders the same page without holding a connection open,
	// which is what screenshots and scrapers need.
	static := request(t, frontend.Handler(tracer), oida.DefaultPath+"/live?stream=off", nil).Body.String()
	if strings.Contains(static, "data-events") {
		t.Error("stream=off still advertises the event stream")
	}
	if !strings.Contains(static, `http-equiv="refresh"`) {
		t.Error("stream=off did not fall back to the meta refresh")
	}
	if !strings.Contains(static, "<h2>Feed</h2>") {
		t.Error("stream=off changed the rendered content")
	}
}

func TestLiveFeedHoldsRunningAndCompletedTraces(t *testing.T) {
	tracer, clock := newTestTracer(t, nil)

	// One trace still running, one already finished.
	_, running, err := tracer.StartTrace(t.Context(), "GET /slow")
	if err != nil {
		t.Fatalf("StartTrace: %v", err)
	}
	clock.Advance(40 * time.Millisecond)
	if err := tracer.Observe(t.Context(), "GET /fast", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	body := request(t, frontend.Handler(tracer), oida.DefaultPath+"/live?stream=off", nil).Body.String()
	for _, want := range []string{"GET /slow", "GET /fast", `class="status running"`} {
		if !strings.Contains(body, want) {
			t.Errorf("feed misses %q", want)
		}
	}

	// Newest first: the finished trace started later, so it leads the feed.
	if strings.Index(body, "GET /fast") > strings.Index(body, "GET /slow") {
		t.Error("feed is not ordered newest first")
	}

	tracer.Finish(running)
	body = request(t, frontend.Handler(tracer), oida.DefaultPath+"/live?stream=off", nil).Body.String()
	if strings.Contains(body, `class="status running"`) {
		t.Error("a completed trace is still marked as running")
	}
	// One row per trace: the feed merges in flight and completed, it does not
	// list a trace twice. Counted by row target, because the name also appears
	// in the cell's title attribute.
	if got := strings.Count(body, `data-href="`+oida.DefaultPath+"/trace/"+running.ID+`"`); got != 1 {
		t.Errorf("the completed trace appears in %d rows, want 1", got)
	}
}

// readEvent reads one server sent event and returns its payload.
func readEvent(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	var payload strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, ":"):
			continue // heartbeat
		case strings.HasPrefix(line, "data: "):
			payload.WriteString(strings.TrimPrefix(line, "data: "))
			payload.WriteString("\n")
		case line == "":
			if payload.Len() > 0 {
				return payload.String()
			}
		}
	}
}
