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

	opts := oida.NewOptions()
	opts.ServiceName = "oida-example"
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

	tracer, err := oida.Configure(opts)
	if err != nil {
		return err
	}
	opts.Tracer = tracer

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(oida.TracingMiddleware(opts))

	if err := oida.Mount(r, opts); err != nil {
		return err
	}

	r.Get("/", index)
	r.Get("/users/{id}", getUser)
	r.Get("/report", report)

	go generateLoad(ctx, tracer)

	addr := os.Getenv("OIDA_ADDR")
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
	_, span := oida.Start(r.Context(), "render index", oida.KindTemplate)
	defer span.End()

	fmt.Fprintln(w, "oida example · try /users/42, /report, /debug/oida")
}

// getUser reads a user through a cache and a database lookup.
func getUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := oida.Start(r.Context(), "getUser")
	defer span.End()

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
	defer span.End()

	hit := rand.IntN(3) == 0
	span.SetAttribute("key", "user:"+id)
	span.SetAttribute("hit", hit)
	time.Sleep(time.Duration(rand.IntN(400)) * time.Microsecond)
	if !hit {
		return "", false
	}
	return "user " + id + " (cached)", true
}

// loadUser records a database span.
func loadUser(ctx context.Context, id string) (string, error) {
	ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
	defer span.End()

	span.SetAttribute("query", "SELECT id, email FROM users WHERE id = ?")
	span.SetAttribute("args", id)
	time.Sleep(time.Duration(2+rand.IntN(6)) * time.Millisecond)

	if id == "0" {
		return "", errors.New("user 0 does not exist")
	}
	return "user " + id, nil
}

// report fans out concurrent work below one parent span.
func report(w http.ResponseWriter, r *http.Request) {
	ctx, span := oida.Start(r.Context(), "build report")
	defer span.End()

	done := make(chan struct{}, 3)
	for i := range 3 {
		go func() {
			defer func() { done <- struct{}{} }()
			_, worker := oida.Start(ctx, fmt.Sprintf("shard %d", i), oida.KindDatabase)
			defer worker.End()
			worker.SetAttribute("shard", i)
			time.Sleep(time.Duration(3+rand.IntN(10)) * time.Millisecond)
		}()
	}
	for range 3 {
		<-done
	}

	if err := oida.Do(ctx, "GET pricing-api", func(context.Context) error {
		time.Sleep(time.Duration(5+rand.IntN(20)) * time.Millisecond)
		return nil
	}, oida.KindExternal); err != nil {
		span.RecordError(err)
	}

	fmt.Fprintln(w, "report built")
}

// generateLoad records a background trace on an interval, so the front end has
// something to show without any traffic. The interval comes from
// OIDA_DEMO_INTERVAL; set it to 0 to record only real requests.
func generateLoad(ctx context.Context, tracer *oida.Tracer) {
	interval := 5 * time.Minute
	if value := os.Getenv("OIDA_DEMO_INTERVAL"); value != "" {
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
				_, span := oida.Start(ctx, "REFRESH MATERIALIZED VIEW daily_totals", oida.KindDatabase)
				defer span.End()
				time.Sleep(time.Duration(10+rand.IntN(40)) * time.Millisecond)
				return nil
			})
		}
	}
}
