package auth

import (
	"html/template"
	"net/http"
)

const devLoginHTML = `<!DOCTYPE html>
<html lang="ca">
<head><meta charset="utf-8"><title>Dev Login</title></head>
<body>
<h1>Dev Login</h1>
<form method="POST" action="/dev-login">
  <label>Email: <input type="email" name="email" autofocus></label>
  <button type="submit">Login</button>
</form>
</body>
</html>`

var devLoginTmpl = template.Must(template.New("devlogin").Parse(devLoginHTML))

// DevAuthenticator is the development authenticator used when OAuth creds are absent.
type DevAuthenticator struct {
	store    *SessionStore
	partners PartnerLookup
	secure   bool
}

// IsDev reports that this is the dev authenticator.
func (a *DevAuthenticator) IsDev() bool { return true }

// Login renders a minimal email form for the dev flow.
func (a *DevAuthenticator) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := devLoginTmpl.Execute(w, nil); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Complete reads the email from the POST form, looks up the partner, and on a hit
// creates a session + sets the cookie and redirects to "/". On a miss it redirects
// to "/access-denied".
func (a *DevAuthenticator) Complete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")
	p, ok, err := a.partners.FindByEmail(r.Context(), email)
	if err != nil {
		http.Error(w, "lookup error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Redirect(w, r, "/access-denied", http.StatusSeeOther)
		return
	}
	token, err := a.store.Create(r.Context(), p.ID(), p.Email())
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	SetSessionCookie(w, token, a.secure)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
