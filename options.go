package oida

import "github.com/titpetric/oida/model"

// Options configures telemetry behaviour, the debug front end and the
// middleware. It lives in the model package so the front end can read it
// without depending on the recorder; the alias keeps it spelled oida.Options.
type Options = model.Options

// DefaultPath is the default mount path of the debug front end.
const DefaultPath = model.DefaultPath

// NewOptions returns the default options for the named service.
func NewOptions(serviceName string) Options {
	return model.NewOptions(serviceName)
}
