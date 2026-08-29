package tests_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/tests"
)

// protectedTracer returns a tracer whose front end sits behind one configured
// user and a pre-shared signing secret. The tracer is the handler: it serves
// the dashboard itself.
func protectedTracer(t *testing.T, apply func(*oida.Options)) *oida.Tracer {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	opts := oida.NewOptions("login-test")
	opts.Enabled = true
	opts.TrackMemoryUse = false
	opts.Users = map[string]string{"admin": string(hash)}
	opts.SigningSecret = "pre-shared"
	opts.OnError = func(err error) { t.Errorf("oida: %v", err) }
	if apply != nil {
		apply(&opts)
	}

	tracer, err := oida.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tracer
}

// loginGet performs one front end request and returns the response.
func loginGet(t *testing.T, handler http.Handler, target string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, target, nil)
	for key, value := range header {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// postLogin submits the sign in form.
func postLogin(t *testing.T, handler http.Handler, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	return postLoginBack(t, handler, username, password, "")
}

// postLoginBack submits the sign in form with a back target.
func postLoginBack(t *testing.T, handler http.Handler, username, password, back string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{"username": {username}, "password": {password}}
	if back != "" {
		form.Set("back", back)
	}
	r := httptest.NewRequest(http.MethodPost, oida.DefaultPath+"/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestLogin(t *testing.T) {
	tracer := protectedTracer(t, nil)

	// A browser without credentials is redirected to the sign in screen; an
	// API caller gets the 401 it can act on.
	browser := loginGet(t, tracer, oida.DefaultPath+"/traces", nil)
	if browser.Code != http.StatusSeeOther {
		t.Fatalf("browser status %d, want 303", browser.Code)
	}
	if got := browser.Header().Get("Location"); got != oida.DefaultPath+"/login?back=%2Fdebug%2Foida%2Ftraces" {
		t.Fatalf("browser redirected to %q", got)
	}
	if code := loginGet(t, tracer, oida.DefaultPath+"/traces", map[string]string{"Accept": "application/json"}).Code; code != http.StatusUnauthorized {
		t.Fatalf("api status %d, want 401", code)
	}

	// The sign in screen and the assets behind it stay reachable, and the
	// screen renders none of the recorded data.
	page := loginGet(t, tracer, oida.DefaultPath+"/login", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("login status %d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{
		`<label for="oida-username">Username</label>`,
		`<label for="oida-password">Password</label>`,
		`action="/debug/oida/login"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("login screen misses %q", want)
		}
	}
	if strings.Contains(body, "goroutines") {
		t.Error("the login screen renders the process facts")
	}
	if code := loginGet(t, tracer, oida.DefaultPath+"/assets/oida.css", nil).Code; code != http.StatusOK {
		t.Errorf("stylesheet status %d", code)
	}

	// A wrong password re-renders the form: a 401, the username kept, the
	// error saying what to do next.
	failed := postLogin(t, tracer, "admin", "wrong")
	if failed.Code != http.StatusUnauthorized {
		t.Fatalf("failed login status %d, want 401", failed.Code)
	}
	if !strings.Contains(failed.Body.String(), "not accepted") || !strings.Contains(failed.Body.String(), `value="admin"`) {
		t.Error("the failed login does not keep the username and explain the failure")
	}

	// The right password sets the session cookie and lands on the overview,
	// and the cookie opens the front end.
	passed := postLogin(t, tracer, "admin", "hunter2")
	if passed.Code != http.StatusSeeOther {
		t.Fatalf("login status %d, want 303", passed.Code)
	}
	if got := passed.Header().Get("Location"); got != oida.DefaultPath {
		t.Fatalf("login redirected to %q", got)
	}

	var session *http.Cookie
	for _, cookie := range passed.Result().Cookies() {
		if cookie.Name == oida.SessionCookie {
			session = cookie
		}
	}
	if session == nil {
		t.Fatal("the login set no session cookie")
	}
	if !session.HttpOnly || session.Path != oida.DefaultPath {
		t.Errorf("cookie flags: %+v", session)
	}

	r := httptest.NewRequest(http.MethodGet, oida.DefaultPath+"/traces", nil)
	r.AddCookie(session)
	w := httptest.NewRecorder()
	tracer.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status with session %d", w.Code)
	}
}

func TestLoginBack(t *testing.T) {
	tracer := protectedTracer(t, nil)

	// The redirect to the sign in screen carries the requested page, query
	// and all, so a successful login returns to it.
	browser := loginGet(t, tracer, oida.DefaultPath+"/traces?kind=db", nil)
	want := oida.DefaultPath + "/login?back=" + url.QueryEscape(oida.DefaultPath+"/traces?kind=db")
	if got := browser.Header().Get("Location"); got != want {
		t.Fatalf("browser redirected to %q, want %q", got, want)
	}

	// The screen keeps the target in the form, and the login lands on it.
	page := loginGet(t, tracer, oida.DefaultPath+"/login?back="+url.QueryEscape(oida.DefaultPath+"/traces?kind=db"), nil)
	if !strings.Contains(page.Body.String(), `name="back" value="/debug/oida/traces?kind=db"`) {
		t.Error("the form does not carry the back target")
	}
	passed := postLoginBack(t, tracer, "admin", "hunter2", oida.DefaultPath+"/traces?kind=db")
	if got := passed.Header().Get("Location"); got != oida.DefaultPath+"/traces?kind=db" {
		t.Fatalf("login redirected to %q, want the back target", got)
	}

	// A target outside the dashboard cannot leave it: another site, a
	// protocol relative address or another path all land on the overview.
	for _, hostile := range []string{
		"https://evil.example/phish",
		"//evil.example/phish",
		"/etc/passwd",
		oida.DefaultPath + "//evil.example",
	} {
		passed := postLoginBack(t, tracer, "admin", "hunter2", hostile)
		if got := passed.Header().Get("Location"); got != oida.DefaultPath {
			t.Errorf("back=%q redirected to %q, want the overview", hostile, got)
		}
	}
}

func TestLoginBearer(t *testing.T) {
	tracer := protectedTracer(t, nil)

	// A token minted elsewhere with the pre-shared secret opens the API.
	opts := oida.NewOptions("minter")
	opts.Enabled = true
	opts.SigningSecret = "pre-shared"
	auth, err := oida.NewAuth(opts)
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}
	token, err := auth.Session("ci")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	granted := loginGet(t, tracer, oida.DefaultPath+"/traces?format=json", map[string]string{
		"Authorization": "Bearer " + token,
	})
	if granted.Code != http.StatusOK {
		t.Fatalf("bearer status %d", granted.Code)
	}

	denied := loginGet(t, tracer, oida.DefaultPath+"/traces?format=json", map[string]string{
		"Authorization": "Bearer not-a-token",
	})
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("invalid bearer status %d, want 401", denied.Code)
	}
}

func TestLoginNetworks(t *testing.T) {
	opts := oida.NewOptions("network-test")
	opts.Enabled = true
	opts.TrackMemoryUse = false
	opts.AllowedNetworks = []string{"10.0.0.0/8"}
	opts.OnError = func(err error) { t.Errorf("oida: %v", err) }

	tracer, err := oida.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// httptest requests come from 192.0.2.1, outside the allow list, and the
	// denial reads like any other missing page.
	if code := loginGet(t, tracer, oida.DefaultPath+"/traces", nil).Code; code != http.StatusNotFound {
		t.Fatalf("outside status %d, want 404", code)
	}

	r := httptest.NewRequest(http.MethodGet, oida.DefaultPath+"/traces", nil)
	r.RemoteAddr = "10.1.2.3:9000"
	w := httptest.NewRecorder()
	tracer.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("inside status %d", w.Code)
	}

	// An allow list alone asks for no credentials.
	if strings.Contains(w.Body.String(), "Sign in") {
		t.Error("the allow list alone put up the login screen")
	}
}

func TestLoginRouteWithoutAuth(t *testing.T) {
	server := tests.NewServer(t)

	// Without authentication configured the login path is any other unknown
	// route.
	if code := loginGet(t, server, tests.Path+"/login", nil).Code; code != http.StatusNotFound {
		t.Fatalf("login without auth returned %d, want 404", code)
	}
}
