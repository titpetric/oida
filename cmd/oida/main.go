// Command oida runs a demo chi/v5 service instrumented with the oida package,
// so the front end can be explored without wiring it into a real service. The
// debug front end is served at http://localhost:8080/debug/oida.
package main

import (
	"context"
	"errors"
	"fmt"
	rand "math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/titpetric/oida"
)

// Build information, filled in by the linker.
var (
	Version    = "dev"
	Commit     = ""
	CommitTime = ""
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "oida:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Printf("oida %s %s %s\n", Version, Commit, CommitTime)
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts := oida.NewOptions("oida-example")
	opts.RingBufferSize = 500
	opts.RouteFunc = func(r *http.Request) string {
		if route := chi.RouteContext(r.Context()); route != nil {
			return route.RoutePattern()
		}
		return ""
	}
	opts.OnError = func(err error) {
		fmt.Fprintln(os.Stderr, "oida:", err)
	}

	tracer, err := oida.New(opts)
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(tracer.Middleware)
	r.Use(trackMemory)

	// The tracer is an http.Handler serving the debug front end.
	r.Mount(opts.Path, tracer)

	r.Get("/", index)
	r.Get("/users/{id}", getUser)
	r.Get("/report", report)

	go generateLoad(ctx, tracer)

	addr := strings.TrimSpace(os.Getenv("OIDA_ADDR"))
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	fmt.Println("listening on " + addr + " · front end at " + opts.Path)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// index renders the landing page.
func index(w http.ResponseWriter, r *http.Request) {
	ctx, span := oida.Start(r.Context(), "render index", oida.KindTemplate)
	defer endSpan(ctx, span)

	fmt.Fprintln(w, "oida example · try /users/42, /report, /debug/oida")
}

// getUser reads a user through a cache and a database lookup.
func getUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := oida.Start(r.Context(), "getUser")
	defer endSpan(ctx, span)

	id := chi.URLParam(r, "id")
	span.SetAttribute("user_id", id)

	if user, ok := lookupCache(ctx, id); ok {
		fmt.Fprintln(w, user)
		return
	}

	user, err := loadUser(ctx, id)
	if err != nil {
		span.RecordError(err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	fmt.Fprintln(w, user)
}

// lookupCache records a cache span that misses two times out of three.
func lookupCache(ctx context.Context, id string) (string, bool) {
	_, span := oida.Start(ctx, "cache: user", oida.KindCache)
	defer endSpan(ctx, span)

	hit := rand.IntN(3) == 0
	span.SetAttribute("key", "user:"+id)
	span.SetAttribute("hit", hit)
	time.Sleep(time.Duration(rand.IntN(400)) * time.Microsecond)
	if !hit {
		span.Info("cache miss, falling back to the database", "key", "user:"+id)
		return "", false
	}
	span.Info("cache hit", "key", "user:"+id)
	return "user " + id + " (cached)", true
}

// loadUser records a database span.
func loadUser(ctx context.Context, id string) (string, error) {
	ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
	defer endSpan(ctx, span)

	span.SetAttribute("query", "SELECT id, email FROM users WHERE id = ?")
	span.SetAttribute("args", id)
	time.Sleep(time.Duration(2+rand.IntN(6)) * time.Millisecond)

	if id == "0" {
		span.Error("user lookup returned no rows", "user_id", id)
		return "", errors.New("user 0 does not exist")
	}
	span.Info("user loaded", "user_id", id, "rows", 1)
	return "user " + id, nil
}

// report fans out concurrent work below one parent span, and writes the log a
// typical web request would: session, flags, cache, queries, the external
// call, rendering, compression. The entries land on the trace's Log tab.
func report(w http.ResponseWriter, r *http.Request) {
	ctx, span := oida.Start(r.Context(), "build report")
	defer endSpan(ctx, span)

	trace := oida.TraceFromContext(ctx)
	trace.Info("session accepted", "user_id", 1042, "roles", "analyst")
	trace.Info("feature flags loaded", "flags", 12, "source", "cache")

	span.Info("report cache missed, rebuilding", "key", "report:daily")

	done := make(chan struct{}, 3)
	for i := range 3 {
		go func() {
			defer func() { done <- struct{}{} }()
			_, worker := oida.Start(ctx, fmt.Sprintf("shard %d", i), oida.KindDatabase)
			defer endSpan(ctx, worker)
			worker.SetAttribute("shard", i)
			elapsed := 3 + rand.IntN(10)
			time.Sleep(time.Duration(elapsed) * time.Millisecond)
			worker.Info("shard queried", "shard", i, "rows", 380+rand.IntN(60))
			if elapsed > 10 {
				worker.Error("shard exceeded its budget", "shard", i, "budget_ms", 10, "took_ms", elapsed)
			}
		}()
	}
	for range 3 {
		<-done
	}

	// Logged on the trace: the entry attributes itself to the innermost open
	// span, which is the report span holding this work.
	trace.Info("report shards merged", "shards", 3, "rows", 1204)

	if err := do(ctx, "GET pricing-api", func(ctx context.Context) error {
		oida.SpanFromContext(ctx).Info("pricing api responded", "status", 200, "currency", "EUR")
		time.Sleep(time.Duration(5+rand.IntN(20)) * time.Millisecond)
		return nil
	}, oida.KindExternal); err != nil {
		span.RecordError(err)
	}

	trace.Info("totals computed", "rows", 1204, "sum", "48210.50")
	span.Info("report rendered", "template", "report.html", "bytes", 48512)
	trace.Info("response compressed", "encoding", "gzip", "ratio", "3.1")
	trace.Info("audit event queued", "topic", "reports", "partition", 3)

	fmt.Fprintln(w, "report built")
}

// generateLoad records a background trace on an interval, so the front end has
// something to show without any traffic. The interval comes from
// OIDA_DEMO_INTERVAL; set it to 0 to record only real requests.
func generateLoad(ctx context.Context, tracer *oida.Tracer) {
	interval := 5 * time.Minute
	if value := strings.TrimSpace(os.Getenv("OIDA_DEMO_INTERVAL")); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return
		}
		interval = parsed
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = tracer.Observe(ctx, "cron: refresh materialized views", func(ctx context.Context) error {
				ctx = withBudget(ctx)
				defer closeBudget(ctx)

				_, span := oida.Start(ctx, "REFRESH MATERIALIZED VIEW daily_totals", oida.KindDatabase)
				defer endSpan(ctx, span)
				time.Sleep(time.Duration(10+rand.IntN(40)) * time.Millisecond)
				return nil
			})
		}
	}
}
