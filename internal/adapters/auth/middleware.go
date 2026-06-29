package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

// CookieName is the name of the session cookie. Exported for use by handlers
// that need to read the raw token (e.g. for CSRF derivation).
const CookieName = "espigol_session"

const cookieName = CookieName

// PartnerLookup is the minimal repository interface required by auth.
// Exported so Task 6 wire/deps can reference auth.PartnerLookup.
type PartnerLookup interface {
	FindByID(ctx context.Context, id int) (model.Partner, bool, error)
	FindByEmail(ctx context.Context, email string) (model.Partner, bool, error)
}

type ctxKey int

const partnerKey ctxKey = 0

func withPartner(ctx context.Context, p model.Partner) context.Context {
	return context.WithValue(ctx, partnerKey, p)
}

// PartnerFrom returns the authenticated partner from the request context.
func PartnerFrom(ctx context.Context) (model.Partner, bool) {
	p, ok := ctx.Value(partnerKey).(model.Partner)
	return p, ok
}

// RequireAuth loads the session→partner into the request context or redirects to /login.
func RequireAuth(store *SessionStore, partners PartnerLookup, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		sess, ok, err := store.Get(r.Context(), c.Value)
		if err != nil {
			http.Error(w, "session error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		p, ok, err := partners.FindByID(r.Context(), sess.PartnerID)
		if err != nil || !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(withPartner(r.Context(), p)))
	})
}

// SetSessionCookie writes the session cookie to w.
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionTTL),
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
