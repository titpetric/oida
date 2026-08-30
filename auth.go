package oida

import "github.com/titpetric/oida/model"

// Auth evaluates the authentication options: the network allow list, the
// configured users, and the token verification behind the session cookie and
// the Authorization header. It lives in the model package so the front end
// can enforce it; the alias keeps it spelled oida.Auth.
type Auth = model.Auth

// NewAuth builds the authentication state out of the options, or nil when no
// authentication option is set: no allow list, no users and no signing secret
// leaves the front end open. Session mints a token from it, which is how a
// deployment issues one for a job that reads the dashboard API.
func NewAuth(opts Options) (*Auth, error) {
	return model.NewAuth(opts)
}

// SessionCookie is the name of the front end session cookie.
const SessionCookie = model.SessionCookie

// SessionTTL is how long an issued session token stays valid.
const SessionTTL = model.SessionTTL

// The errors authentication returns, aliased like the other error values so
// errors.Is works with either spelling.
var (
	// ErrInvalidCredentials is returned when a login does not match any
	// configured user.
	ErrInvalidCredentials = model.ErrInvalidCredentials

	// ErrInvalidToken is returned when a session cookie or bearer token does
	// not verify against the signing secret, or has expired.
	ErrInvalidToken = model.ErrInvalidToken
)
