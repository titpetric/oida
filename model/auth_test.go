package model

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// testHash returns a bcrypt hash of a password, at the cheapest cost.
func testHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(hash)
}

func TestAuthNil(t *testing.T) {
	auth, err := NewAuth(NewOptions("test"))
	if auth != nil || err != nil {
		t.Fatalf("unconfigured options built %+v, %v", auth, err)
	}

	// The nil Auth is what an unconfigured handler holds, so its answers are
	// the open ones.
	var none *Auth
	if !none.NetworkAllowed(httptest.NewRequest("GET", "/", nil)) {
		t.Error("a nil Auth denies a network")
	}
	if none.LoginRequired() {
		t.Error("a nil Auth requires a login")
	}
	if _, ok := none.RequestUser(httptest.NewRequest("GET", "/", nil)); ok {
		t.Error("a nil Auth names a user")
	}
	if err := none.Authenticate(context.Background(), "u", "p"); err == nil {
		t.Error("a nil Auth accepts credentials")
	}
	if _, err := none.Session("u"); err == nil {
		t.Error("a nil Auth issues a session")
	}
	if _, err := none.Verify("x.y.z"); err == nil {
		t.Error("a nil Auth verifies a token")
	}
}

func TestAuthNetworks(t *testing.T) {
	opts := NewOptions("test")
	opts.AllowedNetworks = []string{"127.0.0.0/8", "10.0.0.0/8", "2001:db8::/32"}
	auth, err := NewAuth(opts)
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}
	if auth.LoginRequired() {
		t.Error("an allow list alone requires a login")
	}

	cases := map[string]bool{
		"127.0.0.1:9000":         true,
		"10.20.30.40:1234":       true,
		"[::ffff:10.1.1.1]:5000": true, // an IPv4 peer on an IPv6 listener
		"[2001:db8::1]:443":      true,
		"192.168.1.1:9000":       false,
		"[2001:db9::1]:443":      false,
		"8.8.8.8":                false, // a bare address parses too
		"127.0.0.1":              true,
		"not-an-address":         false,
	}
	for remote, want := range cases {
		r := httptest.NewRequest("GET", "/debug/oida", nil)
		r.RemoteAddr = remote
		if got := auth.NetworkAllowed(r); got != want {
			t.Errorf("%s: allowed = %v, want %v", remote, got, want)
		}
	}
}

func TestAuthInvalidNetworkFailsClosed(t *testing.T) {
	var reported error
	opts := NewOptions("test")
	opts.AllowedNetworks = []string{"not-a-cidr", "10.0.0.0/8"}
	opts.OnError = func(err error) { reported = err }

	auth, err := NewAuth(opts)
	if err == nil || !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("NewAuth error = %v, want ErrInvalidOptions", err)
	}
	if reported == nil {
		t.Error("the dropped entry was not reported to OnError")
	}

	// The invalid entry is dropped, the valid one stays in force.
	r := httptest.NewRequest("GET", "/debug/oida", nil)
	r.RemoteAddr = "10.1.2.3:1000"
	if !auth.NetworkAllowed(r) {
		t.Error("the valid network entry was dropped with the invalid one")
	}

	if err := opts.Validate(); err == nil || !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("Validate = %v, want ErrInvalidOptions", err)
	}
}

func TestAuthAuthenticate(t *testing.T) {
	opts := NewOptions("test")
	opts.Users = map[string]string{"admin": testHash(t, "hunter2")}
	auth, err := NewAuth(opts)
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}
	if !auth.LoginRequired() {
		t.Fatal("configured users do not require a login")
	}

	ctx := context.Background()
	if err := auth.Authenticate(ctx, "admin", "hunter2"); err != nil {
		t.Errorf("the right password is rejected: %v", err)
	}
	if err := auth.Authenticate(ctx, "admin", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("a wrong password returns %v, want ErrInvalidCredentials", err)
	}
	if err := auth.Authenticate(ctx, "nobody", "hunter2"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("an unknown user returns %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthUsersFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "htpasswd")
	content := strings.Join([]string{
		"# a comment",
		"",
		"admin:" + testHash(t, "hunter2"),
		"viewer:{SHA}unsupported",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := NewOptions("test")
	opts.UsersFile = path
	auth, err := NewAuth(opts)
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}

	ctx := context.Background()
	if err := auth.Authenticate(ctx, "admin", "hunter2"); err != nil {
		t.Errorf("the file user is rejected: %v", err)
	}
	// Only bcrypt hashes match, so the {SHA} user never authenticates.
	if err := auth.Authenticate(ctx, "viewer", "unsupported"); err == nil {
		t.Error("a non-bcrypt hash authenticated")
	}

	missing := NewOptions("test")
	missing.UsersFile = filepath.Join(t.TempDir(), "nope")
	if _, err := NewAuth(missing); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("a missing users file returns %v, want ErrInvalidOptions", err)
	}
}

func TestAuthAuthorizeUserCallback(t *testing.T) {
	opts := NewOptions("test")
	opts.Users = map[string]string{"admin": testHash(t, "hunter2")}
	opts.AuthorizeUser = func(ctx context.Context, username, password string) error {
		if username == "ldap" && password == "secret" {
			return nil
		}
		return ErrInvalidCredentials
	}
	auth, err := NewAuth(opts)
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}

	ctx := context.Background()
	if err := auth.Authenticate(ctx, "ldap", "secret"); err != nil {
		t.Errorf("the callback user is rejected: %v", err)
	}
	if err := auth.Authenticate(ctx, "admin", "hunter2"); err != nil {
		t.Errorf("the static user is rejected with a callback set: %v", err)
	}
	if err := auth.Authenticate(ctx, "ldap", "wrong"); err == nil {
		t.Error("the callback rejection was ignored")
	}
}

func TestAuthTokenRoundTrip(t *testing.T) {
	opts := NewOptions("test")
	opts.SigningSecret = "pre-shared"
	auth, err := NewAuth(opts)
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}
	if !auth.LoginRequired() {
		t.Error("a pre-shared secret does not require credentials")
	}

	token, err := auth.Session("admin")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	username, err := auth.Verify(token)
	if err != nil || username != "admin" {
		t.Fatalf("Verify = %q, %v", username, err)
	}

	// The same secret verifies a token minted elsewhere, a different one does
	// not.
	other := NewOptions("test")
	other.SigningSecret = "pre-shared"
	twin, _ := NewAuth(other)
	if _, err := twin.Verify(token); err != nil {
		t.Errorf("the same secret rejects the token: %v", err)
	}

	stranger := NewOptions("test")
	stranger.SigningSecret = "different"
	odd, _ := NewAuth(stranger)
	if _, err := odd.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("a different secret returns %v, want ErrInvalidToken", err)
	}

	for _, tampered := range []string{
		"",
		"only.two",
		token + "x",
		strings.Replace(token, ".", "x", 1),
	} {
		if _, err := auth.Verify(tampered); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%q verifies", tampered)
		}
	}
}

func TestAuthTokenExpiry(t *testing.T) {
	now := time.Now()
	opts := NewOptions("test")
	opts.SigningSecret = "pre-shared"
	opts.Clock = func() time.Time { return now }
	auth, err := NewAuth(opts)
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}

	token, err := auth.Session("admin")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if _, err := auth.Verify(token); err != nil {
		t.Fatalf("a fresh token is rejected: %v", err)
	}

	now = now.Add(SessionTTL + time.Second)
	if _, err := auth.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("an expired token returns %v, want ErrInvalidToken", err)
	}
}

func TestAuthRequestUser(t *testing.T) {
	opts := NewOptions("test")
	opts.SigningSecret = "pre-shared"
	auth, err := NewAuth(opts)
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}
	token, err := auth.Session("admin")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	bare := httptest.NewRequest("GET", "/debug/oida", nil)
	if _, ok := auth.RequestUser(bare); ok {
		t.Error("a request without credentials names a user")
	}

	cookie := httptest.NewRequest("GET", "/debug/oida", nil)
	cookie.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	if username, ok := auth.RequestUser(cookie); !ok || username != "admin" {
		t.Errorf("cookie user = %q, %v", username, ok)
	}

	bearer := httptest.NewRequest("GET", "/debug/oida", nil)
	bearer.Header.Set("Authorization", "Bearer "+token)
	if username, ok := auth.RequestUser(bearer); !ok || username != "admin" {
		t.Errorf("bearer user = %q, %v", username, ok)
	}

	wrong := httptest.NewRequest("GET", "/debug/oida", nil)
	wrong.Header.Set("Authorization", "Bearer nope")
	if _, ok := auth.RequestUser(wrong); ok {
		t.Error("an invalid bearer token names a user")
	}
}
