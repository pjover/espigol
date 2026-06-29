package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestRequireAuth_RedirectsWhenNoSession(t *testing.T) {
	store, q, _ := newStore(t, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	partners := persistence.NewPartnerRepository(q)
	h := auth.RequireAuth(store, partners, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Errorf("want redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("redirect to %q, want /login", loc)
	}
}

func TestRequireAuth_AttachesPartner(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, q, _ := newStore(t, now) // newStore seeds partner 1 (s1@e.test)
	partners := persistence.NewPartnerRepository(q)
	token, _ := store.Create(context.Background(), 1, "s1@e.test")

	var got model.Partner
	var ok bool
	h := auth.RequireAuth(store, partners, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = auth.PartnerFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "espigol_session", Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !ok || got.ID() != 1 {
		t.Errorf("expected authed partner 1; code=%d ok=%v id=%d", rec.Code, ok, got.ID())
	}
}

func TestNewAuthenticator_SelectsDev_WhenCredsEmpty(t *testing.T) {
	store, _, _ := newStore(t, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	cfg := &config.Config{}
	// empty OAuth creds → DevAuthenticator
	a := auth.NewAuthenticator(cfg, store, nil)
	if !a.IsDev() {
		t.Error("expected dev authenticator when OAuth creds are empty")
	}
}

func TestNewAuthenticator_SelectsGoogle_WhenCredsPresent(t *testing.T) {
	store, _, _ := newStore(t, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	cfg := &config.Config{}
	cfg.OAuth.ClientID = "client-id"
	cfg.OAuth.ClientSecret = "client-secret"
	cfg.OAuth.RedirectURL = "https://example.com/oauth2/callback"
	a := auth.NewAuthenticator(cfg, store, nil)
	if a.IsDev() {
		t.Error("expected Google authenticator when OAuth creds are present")
	}
}

var _ = sqlc.New
