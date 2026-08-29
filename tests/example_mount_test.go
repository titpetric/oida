package tests_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/titpetric/oida"
)

// Example_mountServeMux mounts the tracer's dashboard on the standard library
// mux: the tracer is an http.Handler, so Handle is the whole integration.
func Example_mountServeMux() {
	opts := oida.NewOptions("billing-api")
	opts.Enabled = true

	tracer, err := oida.New(opts)
	if err != nil {
		fmt.Println(err)
		return
	}

	mux := http.NewServeMux()
	mux.Handle("/debug/oida/", tracer)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/oida/", nil))
	fmt.Println(response.Code)
	// Output: 200
}
