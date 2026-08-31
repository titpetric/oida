// Command gorilla serves an instrumented service on a gorilla/mux router.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/titpetric/oida"
)

// UserList is what the handler returns.
type UserList []string

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	opts := oida.NewOptions("billing-api")
	opts.Enabled = true

	tracer, err := oida.New(opts)
	if err != nil {
		return err
	}

	r := mux.NewRouter()
	r.Use(tracer.Middleware)
	r.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)

	// gorilla returns a *mux.Route from Handle and matches its paths exactly,
	// so the dashboard is registered by prefix through the adapter.
	mount := oida.RouterFunc(func(pattern string, h http.Handler) {
		r.PathPrefix(pattern).Handler(h)
	})
	if err := oida.Mount(mount, tracer); err != nil {
		return err
	}

	return http.ListenAndServe(":8080", r)
}

func getUser(w http.ResponseWriter, r *http.Request) {
	users, err := listUsers(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(users)
}

func listUsers(ctx context.Context) (UserList, error) {
	_, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
	defer span.End()

	span.SetAttribute("limit", 100)

	users := UserList{"alice", "bob"} // implementation...

	span.SetAttribute("rows", len(users))
	return users, nil
}
