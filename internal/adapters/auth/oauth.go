package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/pjover/espigol/internal/config"
)

// Authenticator handles the login flow (dev or Google OAuth).
type Authenticator interface {
	// Login handles GET /login: renders the email form (dev) or redirects to Google (prod).
	Login(w http.ResponseWriter, r *http.Request)
	// Complete handles the credential submission: POST /dev-login (dev) or GET /oauth2/callback (prod).
	// On success it creates a session, sets the cookie, and redirects to "/";
	// on an unregistered email it redirects to "/access-denied".
	Complete(w http.ResponseWriter, r *http.Request)
	// IsDev reports whether this is the dev authenticator (controls route registration).
	IsDev() bool
}

// emailFetcher is the interface used to fetch the user email after OAuth exchange.
// Abstracted so tests can inject a fake.
type emailFetcher interface {
	fetchEmail(ctx context.Context, tok *oauth2.Token) (string, error)
}

// googleEmailFetcher fetches the email from the Google userinfo endpoint.
type googleEmailFetcher struct {
	cfg *oauth2.Config
}

func (f *googleEmailFetcher) fetchEmail(ctx context.Context, tok *oauth2.Token) (string, error) {
	client := f.cfg.Client(ctx, tok)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", fmt.Errorf("fetching userinfo: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading userinfo body: %w", err)
	}
	var info struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("decoding userinfo: %w", err)
	}
	return info.Email, nil
}

const oauthStateCookie = "oauth_state"

// GoogleAuthenticator is the production OAuth2 authenticator backed by Google.
type GoogleAuthenticator struct {
	store    *SessionStore
	partners PartnerLookup
	secure   bool
	oauthCfg *oauth2.Config
	fetcher  emailFetcher
}

// IsDev reports that this is NOT the dev authenticator.
func (a *GoogleAuthenticator) IsDev() bool { return false }

// Login sets a random oauth_state cookie and redirects to Google's consent screen.
func (a *GoogleAuthenticator) Login(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		http.Error(w, "state generation error", http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})
	http.Redirect(w, r, a.oauthCfg.AuthCodeURL(state), http.StatusSeeOther)
}

// Complete verifies the state cookie, exchanges the code for a token, fetches the email,
// looks up the partner, and on hit creates a session+cookie+redirect; on miss redirects /access-denied.
func (a *GoogleAuthenticator) Complete(w http.ResponseWriter, r *http.Request) {
	// Verify state
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: "", Path: "/",
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})

	code := r.URL.Query().Get("code")
	tok, err := a.oauthCfg.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "oauth exchange error", http.StatusBadGateway)
		return
	}

	email, err := a.fetcher.fetchEmail(r.Context(), tok)
	if err != nil {
		http.Error(w, "userinfo error", http.StatusBadGateway)
		return
	}

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

// NewAuthenticator returns a DevAuthenticator when OAuth credentials are absent,
// or a GoogleAuthenticator (with secure cookies) when they are present.
func NewAuthenticator(cfg *config.Config, store *SessionStore, partners PartnerLookup) Authenticator {
	if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" {
		return &DevAuthenticator{
			store:    store,
			partners: partners,
			secure:   false, // dev = no HTTPS
		}
	}
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OAuth.ClientID,
		ClientSecret: cfg.OAuth.ClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  cfg.OAuth.RedirectURL,
		Scopes:       []string{"openid", "email"},
	}
	return &GoogleAuthenticator{
		store:    store,
		partners: partners,
		secure:   true,
		oauthCfg: oauthCfg,
		fetcher:  &googleEmailFetcher{cfg: oauthCfg},
	}
}
