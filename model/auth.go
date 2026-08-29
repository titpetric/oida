package model

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// SessionCookie is the name of the front end session cookie.
const SessionCookie = "oida_session"

// SessionTTL is how long an issued session token stays valid.
const SessionTTL = 12 * time.Hour

// Auth evaluates the authentication options: the network allow list, the
// configured users and the token verification behind the session cookie and
// the Authorization header. The front end builds one per handler with NewAuth.
//
// Every method tolerates a nil receiver, which is what an unconfigured
// handler holds: a nil Auth allows every network and requires no login.
type Auth struct {
	networks  []netip.Prefix
	users     map[string]string
	authorize func(ctx context.Context, username, password string) error
	secret    []byte
	preshared bool
	clock     func() time.Time
}

// NewAuth builds the authentication state out of the options. Invalid network
// entries and an unreadable users file are dropped, reported through
// Options.OnError and returned, so the caller decides whether to treat them
// as fatal; what loaded stays in force. Dropping an allow list entry only
// narrows access, so a typo fails closed.
//
// It returns nil when no authentication option is set.
func NewAuth(opts Options) (*Auth, error) {
	if len(opts.AllowedNetworks) == 0 && len(opts.Users) == 0 &&
		opts.UsersFile == "" && opts.AuthorizeUser == nil && opts.SigningSecret == "" {
		return nil, nil
	}

	auth := &Auth{
		users:     map[string]string{},
		authorize: opts.AuthorizeUser,
		secret:    []byte(opts.SigningSecret),
		preshared: opts.SigningSecret != "",
		clock:     opts.Clock,
	}

	var failures []error
	for _, network := range opts.AllowedNetworks {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(network))
		if err != nil {
			failures = append(failures, invalidOption("allowed_networks", fmt.Sprintf("%q is not a CIDR prefix", network)))
			continue
		}
		auth.networks = append(auth.networks, prefix)
	}

	if opts.UsersFile != "" {
		if err := auth.loadUsersFile(opts.UsersFile); err != nil {
			failures = append(failures, invalidOption("users_file", err.Error()))
		}
	}
	for username, hash := range opts.Users {
		auth.users[username] = hash
	}

	// Without a pre-shared secret the sessions are signed with a generated
	// one: logins keep working, they just do not survive a restart, and no
	// externally minted bearer token can verify.
	if len(auth.secret) == 0 {
		auth.secret = make([]byte, 32)
		if _, err := rand.Read(auth.secret); err != nil {
			failures = append(failures, err)
		}
	}

	err := errors.Join(failures...)
	if err != nil && opts.OnError != nil {
		opts.OnError(err)
	}
	return auth, err
}

// loadUsersFile reads an .htpasswd style file: one username:hash per line,
// blank lines and # comments skipped.
func (a *Auth) loadUsersFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		username, hash, ok := strings.Cut(line, ":")
		if !ok || username == "" {
			continue
		}
		a.users[username] = hash
	}
	return nil
}

// LoginRequired reports whether requests have to carry a session or a bearer
// token: any configured user, the authorize callback or a pre-shared secret
// makes credentials the way in. A bare AllowedNetworks list requires none.
func (a *Auth) LoginRequired() bool {
	return a != nil && (len(a.users) > 0 || a.authorize != nil || a.preshared)
}

// NetworkAllowed reports whether the request comes from an allowed network.
// An empty allow list allows every network; an unparsable peer address is
// denied.
func (a *Auth) NetworkAllowed(r *http.Request) bool {
	if a == nil || len(a.networks) == 0 {
		return true
	}
	addr, err := peerAddr(r.RemoteAddr)
	if err != nil {
		return false
	}
	for _, network := range a.networks {
		if network.Contains(addr) {
			return true
		}
	}
	return false
}

// peerAddr parses the peer of a request: host:port as the server sets it, or
// a bare address. An IPv4 peer on an IPv6 listener is unmapped, so
// ::ffff:127.0.0.1 matches 127.0.0.0/8.
func peerAddr(remote string) (netip.Addr, error) {
	if addrPort, err := netip.ParseAddrPort(remote); err == nil {
		return addrPort.Addr().Unmap(), nil
	}
	addr, err := netip.ParseAddr(remote)
	if err != nil {
		return netip.Addr{}, err
	}
	return addr.Unmap(), nil
}

// Authenticate checks a login against the configured users, then against the
// AuthorizeUser callback. Password hashes are bcrypt; a user with any other
// hash format never matches.
func (a *Auth) Authenticate(ctx context.Context, username, password string) error {
	if a == nil {
		return ErrInvalidCredentials
	}
	if hash, ok := a.users[username]; ok {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err == nil {
			return nil
		}
	}
	if a.authorize != nil {
		return a.authorize(ctx, username, password)
	}
	return ErrInvalidCredentials
}

// Session issues the session token of a logged in user: an HS256 JWT with the
// username as its subject, valid for twelve hours.
func (a *Auth) Session(username string) (string, error) {
	if a == nil {
		return "", ErrInvalidToken
	}
	now := a.now()
	header := encodeSegment(map[string]any{"alg": "HS256", "typ": "JWT"})
	claims := encodeSegment(map[string]any{
		"sub": username,
		"iat": now.Unix(),
		"exp": now.Add(SessionTTL).Unix(),
	})
	signed := header + "." + claims
	return signed + "." + a.sign(signed), nil
}

// Verify checks a token against the signing secret and returns the username
// it names. Only HS256 is accepted, and an expiry claim is honoured.
func (a *Auth) Verify(token string) (string, error) {
	if a == nil {
		return "", ErrInvalidToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidToken
	}
	if !hmac.Equal([]byte(a.sign(parts[0]+"."+parts[1])), []byte(parts[2])) {
		return "", ErrInvalidToken
	}

	var alg struct {
		Alg string `json:"alg"`
	}
	if err := decodeSegment(parts[0], &alg); err != nil || alg.Alg != "HS256" {
		return "", ErrInvalidToken
	}

	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := decodeSegment(parts[1], &claims); err != nil {
		return "", ErrInvalidToken
	}
	if claims.Exp != 0 && a.now().Unix() >= claims.Exp {
		return "", ErrInvalidToken
	}
	return claims.Sub, nil
}

// RequestUser returns the username a request carries, in its session cookie
// or in an Authorization bearer token.
func (a *Auth) RequestUser(r *http.Request) (string, bool) {
	if a == nil {
		return "", false
	}
	if cookie, err := r.Cookie(SessionCookie); err == nil {
		if username, err := a.Verify(cookie.Value); err == nil {
			return username, true
		}
	}
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		if username, err := a.Verify(strings.TrimSpace(token)); err == nil {
			return username, true
		}
	}
	return "", false
}

// sign returns the base64url HMAC-SHA256 of the signing input.
func (a *Auth) sign(signed string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(signed))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// now returns the current time from the configured clock.
func (a *Auth) now() time.Time {
	if a == nil || a.clock == nil {
		return time.Now()
	}
	return a.clock()
}

// encodeSegment renders one JWT segment: JSON, base64url, no padding.
func encodeSegment(claims map[string]any) string {
	data, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(data)
}

// decodeSegment parses one JWT segment into out.
func decodeSegment(segment string, out any) error {
	data, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
