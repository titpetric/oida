// Command std serves an instrumented service on the standard library mux.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", getUser)

	if err := oida.Mount(mux, tracer); err != nil {
		return err
	}

	return http.ListenAndServe(":8080", tracer.Middleware(mux))
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
