package frontend

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/titpetric/oida/model"
)

// serveUnauthorized answers a request that carries no valid credentials. A
// browser is sent to the sign in screen with the requested page in ?back, so
// a successful login returns to it; an API caller, asking for JSON or plain
// text, gets the 401 it can act on.
func (h *handler) serveUnauthorized(w http.ResponseWriter, r *http.Request) {
	if negotiate(r) == formatHTML {
		target := h.opts.Path + "/login"
		if back := r.URL.RequestURI(); backTarget(h.opts.Path, back) == back {
			target += "?back=" + url.QueryEscape(back)
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	http.Error(w, "unauthorized: sign in at "+h.opts.Path+"/login or send Authorization: Bearer <token>", http.StatusUnauthorized)
}

// backTarget returns the page a successful login lands on: the ?back target
// when it is a location inside the dashboard, the overview otherwise. Only a
// relative path under the mount qualifies, so the parameter cannot redirect
// off the dashboard or to another site.
func backTarget(base string, back string) string {
	if strings.HasPrefix(back, base+"/") && !strings.Contains(back, "//") && !strings.Contains(back, "\\") {
		return back
	}
	return base
}

// serveLogin renders the sign in screen and handles its submission. A
// successful login sets the session cookie and lands on the ?back target,
// or the overview; a failed one re-renders the form with a 401, keeping the
// username and the target.
func (h *handler) serveLogin(w http.ResponseWriter, r *http.Request) {
	// The page carries no snapshot. The requester has not proven they may
	// see one.
	page := Page{View: ViewLogin, Path: h.opts.Path, Title: "Sign in"}
	page.LoginBack = r.URL.Query().Get("back")

	if r.Method == http.MethodPost {
		username := strings.TrimSpace(r.PostFormValue("username"))
		password := r.PostFormValue("password")
		page.LoginBack = r.PostFormValue("back")

		err := h.auth.Authenticate(r.Context(), username, password)
		if err == nil {
			var token string
			if token, err = h.auth.Session(username); err == nil {
				http.SetCookie(w, &http.Cookie{
					Name:     model.SessionCookie,
					Value:    token,
					Path:     h.opts.Path,
					MaxAge:   int(model.SessionTTL.Seconds()),
					HttpOnly: true,
					Secure:   r.TLS != nil,
					SameSite: http.SameSiteLaxMode,
				})
				http.Redirect(w, r, backTarget(h.opts.Path, page.LoginBack), http.StatusSeeOther)
				return
			}
			h.tracer().ReportError(err)
		}

		page.LoginUsername = username
		page.LoginError = "That username and password were not accepted. Check both and try again."
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
	}

	h.render(w, r, Login(page))
}
